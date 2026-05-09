package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/cilium/ebpf/ringbuf"
	"github.com/gin-gonic/gin"
	ps "github.com/shirou/gopsutil/v3/process"
)

func handleRegister(c *gin.Context) {
	if trackerMaps.AgentPids == nil {
		c.JSON(500, gin.H{"error": "agent pid map not initialized"})
		return
	}
	var req registerPayload
	if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
		c.JSON(400, gin.H{"error": "invalid pid"})
		return
	}
	tag := req.Tag
	if tag == "" {
		tag = "AI Agent"
	}
	if err := trackerMaps.AgentPids.Put(req.PID, getTagID(tag)); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	trackedProcessContexts.Set(req.PID, buildProcessContextFromRegister(req))
	c.JSON(200, gin.H{"status": "ok"})
}

func handleUnregister(c *gin.Context) {
	if trackerMaps.AgentPids == nil {
		c.JSON(500, gin.H{"error": "agent pid map not initialized"})
		return
	}
	var req struct {
		PID uint32 `json:"pid"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
		c.JSON(400, gin.H{"error": "invalid pid"})
		return
	}
	_ = trackerMaps.AgentPids.Delete(req.PID)
	trackedProcessContexts.Delete(req.PID)
	c.JSON(200, gin.H{"status": "ok"})
}

func handleClearEvents(c *gin.Context) {
	capturedEventArchive.Clear()
	if err := runtimeSettingsStore.TruncateEventLog(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func handleClearEventsMemory(c *gin.Context) {
	capturedEventArchive.Clear()
	c.JSON(200, gin.H{"status": "ok"})
}

func handleClearEventsPersisted(c *gin.Context) {
	if err := runtimeSettingsStore.TruncateEventLog(); err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "ok"})
}

func handleShellSessionsCleanup(c *gin.Context) {
	shellSessions.ClearClosed()
	c.JSON(200, gin.H{"status": "ok"})
}

func main() {
	if isBootstrapMode() {
		if err := bootstrapTrackerMaps(); err != nil {
			log.Fatalf("failed to bootstrap eBPF components: %v", err)
		}
		return
	}
	if relaunched, err := ensureBackendPrivileges(); err != nil {
		log.Fatalf("failed to elevate backend privileges: %v", err)
	} else if relaunched {
		return
	}

	refreshHooksPaths()
	if _, err := runtimeSettingsStore.LoadOrCreate(); err != nil {
		log.Printf("[WARN] failed to load runtime settings: %v", err)
	}
	defer otelExporterStore.Close()

	procsList, _ := ps.Processes()
	curr := int32(os.Getpid())
	for _, p := range procsList {
		if p.Pid != curr {
			if n, _ := p.Name(); n == "agent-ebpf-filter" || n == "main" {
				_ = p.Kill()
			}
		}
	}

	if err := ensureTrackerMapsLoaded(); err != nil {
		log.Fatalf("failed to initialize eBPF components: %v", err)
	}
	objs := &trackerMaps

	rd, _ := ringbuf.NewReader(objs.Events)
	defer rd.Close()

	go func() {
		var event bpfEvent
		selfPid := uint32(os.Getpid())
		for {
			record, err := rd.Read()
			if err != nil {
				return
			}
			if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
				log.Printf("[WARN] failed to decode eBPF event: %v (sample len=%d)", err, len(record.RawSample))
				continue
			}
			if event.PID == selfPid {
				continue
			}
			comm := sanitizeUTF8(event.Comm[:])
			if isCommDisabled(comm) {
				continue
			}
			if isEventTypeDisabled(event.Type) {
				continue
			}
			broadcast <- buildKernelEvent(event)
		}
	}()

	startEventBroadcaster()
	go startUDSServer(broadcast)
	startCgroupAttributionGC()
	startDNSCacheGC()
	startTCPStateTrackerGC()
	startFlowAggregatorGC()
	startExfilDetectionLoop()
	go func() {
		time.Sleep(100 * time.Millisecond)
		initGeoIPDatabase()
	}()
	go func() {
		ticker := time.NewTicker(3 * time.Minute)
		for range ticker.C {
			globalBandwidthTracker.EvictOlderThan(15 * time.Minute)
		}
	}()
	go func() {
		if err := ensureCgroupSandboxLoaded(); err != nil {
			log.Printf("[CGROUP-SANDBOX] not available: %v", err)
		} else {
			autoBlockHighRiskEndpoints()
		}
	}()

	ApplySandbox()

	r := gin.Default()
	r.Use(clusterGatewayMiddleware())

	// Periodic archive eviction based on MaxEventAge
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				settings := runtimeSettingsStore.Snapshot()
				if d, err := time.ParseDuration(settings.MaxEventAge); err == nil && d > 0 {
					capturedEventArchive.EvictOlderThan(time.Now().UTC().Add(-d))
				}
			}
		}
	}()

	r.GET("/ws", authMiddleware(), serveEventsWS)
	r.GET("/ws/system", authMiddleware(), serveSystemStatsWS)
	r.GET("/ws/camera", authMiddleware(), serveCameraWS)
	r.GET("/ws/sensors", authMiddleware(), serveSensorsWS)
	r.GET("/ws/microphone", authMiddleware(), serveMicrophoneWS)
	r.GET("/ws/ml-status", authMiddleware(), serveMLStatusWS)
	r.GET("/ws/envelopes", authMiddleware(), serveEventEnvelopesWS)
	r.GET("/ws/events/graph", authMiddleware(), serveExecutionGraphWS)
	r.POST("/shell-sessions", authMiddleware(), shellSessionsEnabledMiddleware(), handleCreateShellSession)
	r.GET("/shell-sessions", authMiddleware(), shellSessionsEnabledMiddleware(), handleListShellSessions)
	r.DELETE("/shell-sessions/:id", authMiddleware(), shellSessionsEnabledMiddleware(), handleDeleteShellSession)
	r.POST("/shell-sessions/:id/input", authMiddleware(), shellSessionsEnabledMiddleware(), handleSendShellSessionInput)
	r.GET("/ws/shell", authMiddleware(), shellSessionsEnabledMiddleware(), serveShellWS)
	r.GET("/events/recent", authMiddleware(), handleRecentEvents)
	r.GET("/events/graph", authMiddleware(), handleExecutionGraph)
	r.GET("/events/recording", authMiddleware(), handleEventRecordingStatus)
	r.POST("/events/recording/start", authMiddleware(), handleStartEventRecording)
	r.POST("/events/recording/stop", authMiddleware(), handleStopEventRecording)
	r.POST("/events/recording/replay", authMiddleware(), handleReplayEventRecording)
	r.GET("/network/flows", authMiddleware(), handleNetworkFlows)
	r.GET("/network/flows/:flowID", authMiddleware(), handleNetworkFlowByID)
	r.GET("/network/tcp-state", authMiddleware(), handleTCPState)
	r.GET("/network/analyze", authMiddleware(), handleNetworkAnalyze)
	r.GET("/network/dns-lookup", authMiddleware(), handleDNSLookup)
	r.GET("/network/dns-cache", authMiddleware(), handleDNSCache)
	r.GET("/network/interfaces", authMiddleware(), handleNetworkInterfaces)
	r.GET("/network/export/jsonl", authMiddleware(), handleNetworkFlowJSONLExport)
	r.POST("/network/export-pcap", authMiddleware(), handlePCAPExport)
	r.GET("/network/geoip", authMiddleware(), handleGeoIPLookup)
	r.GET("/sandbox/cgroup/status", authMiddleware(), handleCgroupSandboxStatus)
	r.POST("/sandbox/cgroup/block-cgroup", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockCgroup)
	r.POST("/sandbox/cgroup/unblock-cgroup", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxUnblockCgroup)
	r.POST("/sandbox/cgroup/block-ip", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockIP)
	r.POST("/sandbox/cgroup/block-port", authMiddleware(), policyManagementEnabledMiddleware(), handleCgroupSandboxBlockPort)
	r.GET("/metrics", authMiddleware(), handlePrometheusMetrics)
	r.GET("/ws/shell-sessions", authMiddleware(), shellSessionsEnabledMiddleware(), serveShellSessionsWS)

	r.POST("/hooks/event", hookIngressAuthMiddleware(), handleNativeHookEvent)
	r.POST("/register", authMiddleware(), handleRegister)
	r.POST("/unregister", authMiddleware(), handleUnregister)
	r.POST("/cluster/heartbeat", clusterHeartbeatHandler)
	r.POST("/cluster/register", clusterHeartbeatHandler)

	api := r.Group("/", authMiddleware())
	{
		registerConfigRoutes(api.Group("/config"))
		registerSystemRoutes(api.Group("/system"))

		data := api.Group("/data")
		{
			data.POST("/clear-events", handleClearEvents)
			data.POST("/clear-events-memory", handleClearEventsMemory)
			data.POST("/clear-events-persisted", handleClearEventsPersisted)
		}
		api.POST("/shell-sessions/cleanup", shellSessionsEnabledMiddleware(), handleShellSessionsCleanup)
		api.Any("/mcp", gin.WrapH(buildMCPHandler()))
		cluster := api.Group("/cluster")
		{
			cluster.GET("/state", clusterStateHandler)
			cluster.GET("/nodes", clusterNodesHandler)
		}
	}

	staticDir := "../frontend/dist"
	if _, err := os.Stat(staticDir); err != nil {
		staticDir = "./frontend/dist"
	}
	r.StaticFile("/", filepath.Join(staticDir, "index.html"))
	r.Static("/assets", filepath.Join(staticDir, "assets"))
	r.NoRoute(func(c *gin.Context) { c.File(filepath.Join(staticDir, "index.html")) })

	commonCLIs := map[string]string{
		// Git
		"git": "Git",
		// Language Pkg (npm/pip/cargo/uv etc.)
		"npm": "Language Pkg", "bun": "Language Pkg", "pnpm": "Language Pkg",
		"yarn": "Language Pkg", "pip": "Language Pkg", "pip3": "Language Pkg",
		"gem": "Language Pkg", "uv": "Language Pkg", "zig": "Language Pkg",
		// System Pkg (apt/pacman/dnf/brew etc.)
		"dpkg": "System Pkg", "apt": "System Pkg", "apt-get": "System Pkg",
		"snap": "System Pkg", "flatpak": "System Pkg",
		"pacman": "System Pkg", "yay": "System Pkg", "paru": "System Pkg",
		"dnf": "System Pkg", "yum": "System Pkg", "zypper": "System Pkg",
		"rpm": "System Pkg", "nix": "System Pkg", "brew": "System Pkg",
		// Container CLI
		"docker": "Container CLI", "podman": "Container CLI", "kubectl": "Container CLI",
		// Agent CLI
		"claude": "Agent CLI", "gemini": "Agent CLI", "codex": "Agent CLI",
		"kiro-cli": "Agent CLI", "gh": "Agent CLI", "cursor": "Agent CLI",
		// Build Tool
		"go": "Build Tool", "cargo": "Build Tool", "rustc": "Build Tool",
		"gcc": "Build Tool", "g++": "Build Tool", "clang": "Build Tool",
		"make": "Build Tool", "cmake": "Build Tool", "ninja": "Build Tool",
		"meson": "Build Tool", "gradle": "Build Tool", "mvn": "Build Tool",
		"lldb": "Build Tool", "gdb": "Build Tool",
		// Runtime
		"node": "Runtime", "python": "Runtime", "python3": "Runtime",
		"java": "Runtime", "javac": "Runtime", "ruby": "Runtime",
		"perl": "Runtime", "lua": "Runtime", "deno": "Runtime", "pwsh": "Runtime",
		"php": "Runtime", "dotnet": "Runtime", "erl": "Runtime", "ghc": "Runtime",
		// System Tool
		"systemctl": "System Tool", "journalctl": "System Tool",
		"ffmpeg": "System Tool", "tar": "System Tool", "gzip": "System Tool",
		"unzip": "System Tool",
		// Network Tool
		"ssh": "Network Tool", "scp": "Network Tool", "rsync": "Network Tool",
		"curl": "Network Tool", "wget": "Network Tool",
		// Shell (shadow-banned by default)
		"bash": "Shell", "zsh": "Shell", "fish": "Shell",
		"sh": "Shell", "dash": "Shell", "ash": "Shell",
	}
	for cl, t := range commonCLIs {
		var k [16]byte
		copy(k[:], cl)
		_ = objs.TrackedComms.Put(k, getTagID(t))
	}
	// Shadow-ban shell binaries by default (too noisy for debugging)
	for _, sh := range []string{"bash", "zsh", "fish", "sh", "dash", "ash"} {
		disabledComms[sh] = struct{}{}
	}

	startPort, maxTries, actualPort := 8080, 10, 8080
	for i := 0; i < maxTries; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort+i))
		if err == nil {
			actualPort = startPort + i
			l.Close()
			break
		}
	}
	clusterManagerStore.ConfigurePort(actualPort)
	writePortFile(actualPort)
	startClusterHeartbeatLoop()

	// Initialize ML behavior classifier (master node only)
	go func() {
		time.Sleep(1 * time.Second) // brief delay to let cluster role settle
		settings := runtimeSettingsStore.Snapshot()
		InitMLEngine(settings.MLConfig)
		StartMLEngine()
	}()

	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
