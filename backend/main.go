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
	var req struct {
		PID uint32 `json:"pid"`
		Tag string `json:"tag,omitempty"`
	}
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
			_ = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			if event.PID == selfPid {
				continue
			}
			broadcast <- buildKernelEvent(event)
		}
	}()

	startEventBroadcaster()
	go startUDSServer(broadcast)

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

	r.GET("/ws", serveEventsWS)
	r.GET("/ws/system", serveSystemStatsWS)
	r.GET("/ws/camera", serveCameraWS)
	r.GET("/ws/sensors", serveSensorsWS)
	r.GET("/ws/microphone", serveMicrophoneWS)
	r.POST("/shell-sessions", handleCreateShellSession)
	r.GET("/shell-sessions", handleListShellSessions)
	r.DELETE("/shell-sessions/:id", handleDeleteShellSession)
	r.POST("/shell-sessions/:id/input", handleSendShellSessionInput)
	r.GET("/ws/shell", serveShellWS)
	r.GET("/events/recent", handleRecentEvents)
	r.GET("/ws/shell-sessions", serveShellSessionsWS)

	r.POST("/hooks/event", handleNativeHookEvent)
	r.POST("/register", handleRegister)
	r.POST("/unregister", handleUnregister)
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
		api.POST("/shell-sessions/cleanup", handleShellSessionsCleanup)
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

	commonCLIs := map[string]string{"git": "Git", "npm": "Package Manager", "bun": "Package Manager", "pnpm": "Package Manager", "yarn": "Package Manager", "node": "Runtime", "python": "Runtime", "python3": "Runtime", "go": "Build Tool", "cargo": "Build Tool", "rustc": "Build Tool", "gcc": "Build Tool", "g++": "Build Tool", "clang": "Build Tool", "make": "Build Tool", "cmake": "Build Tool", "docker": "System Tool", "kubectl": "Network Tool"}
	for cl, t := range commonCLIs {
		var k [16]byte
		copy(k[:], cl)
		_ = objs.TrackedComms.Put(k, getTagID(t))
	}

	startPort, maxTries, actualPort := 8080, 10, 8080
	for i := 0; i < maxTries; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort+i))
		if err == nil {
			actualPort = startPort+i
			l.Close()
			break
		}
	}
	clusterManagerStore.ConfigurePort(actualPort)
	writePortFile(actualPort)
	startClusterHeartbeatLoop()
	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
