package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	gnet "github.com/shirou/gopsutil/v3/net"
	ps "github.com/shirou/gopsutil/v3/process"
	"google.golang.org/protobuf/proto"
)

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

	broadcast := make(chan *pb.Event, 100)
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

	go startUDSServer(broadcast)

	r := gin.Default()
	r.Use(clusterGatewayMiddleware())
	clients := make(map[*websocket.Conn]bool)
	var clientsMu sync.Mutex
	go func() {
		batch := make([]*pb.Event, 0, 50)
		batchTicker := time.NewTicker(50 * time.Millisecond)
		defer batchTicker.Stop()
		flushBatch := func() {
			if len(batch) == 0 {
				return
			}
			events := make([]*pb.Event, len(batch))
			copy(events, batch)
			batch = batch[:0]
			msg := &pb.EventBatch{Events: events}
			data, _ := proto.Marshal(msg)
			clientsMu.Lock()
			for c := range clients {
				if c == nil {
					delete(clients, c)
					continue
				}
				if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
					c.Close()
					delete(clients, c)
				}
			}
			clientsMu.Unlock()
		}
		for {
			select {
			case event := <-broadcast:
				recordCapturedEvent(event)
				batch = append(batch, event)
				if len(batch) >= 50 {
					flushBatch()
				}
			case <-batchTicker.C:
				flushBatch()
			}
		}
	}()

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

	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()

		go func(conn *websocket.Conn) {
			defer func() {
				clientsMu.Lock()
				delete(clients, conn)
				clientsMu.Unlock()
				_ = conn.Close()
			}()

			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}(conn)
	})

	r.GET("/ws/system", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		intervalStr := c.DefaultQuery("interval", "2000")
		iv, _ := time.ParseDuration(intervalStr + "ms")
		if iv < 500*time.Millisecond {
			iv = 500 * time.Millisecond
		}
		ticker := time.NewTicker(iv)
		defer ticker.Stop()

		coreTypes := getCoreTypes()
		lastFaults, err := readVMFaultCounters()
		if err != nil {
			lastFaults = vmFaultCounters{}
		}
		lastFaultTime := time.Now()
		type procCPUSample struct {
			createTime int64
			totalCPU   float64
			sampleTime time.Time
		}
		procCPUSamples := make(map[int32]procCPUSample)
		cpuScale := float64(runtime.NumCPU())
		if cpuScale <= 0 {
			cpuScale = 1
		}
		for range ticker.C {
			now := time.Now()
			gm, gs := getGPUMetrics()
			vm, _ := mem.VirtualMemory()
			cc, _ := cpu.Percent(0, false)
			cp, _ := cpu.Percent(0, true)
			netIO, _ := gnet.IOCounters(true)
			diskIO, _ := disk.IOCounters()
			pbIO := &pb.IOInfo{}
			vmFaults, faultErr := readVMFaultCounters()
			faultInfo := &pb.FaultInfo{}
			currentPIDs := make(map[int32]struct{})
			if faultErr == nil {
				pageFaults := vmFaults.pageFaults
				majorFaults := vmFaults.majorFaults
				minorFaults := uint64(0)
				if pageFaults >= majorFaults {
					minorFaults = pageFaults - majorFaults
				}
				faultInfo.PageFaults = pageFaults
				faultInfo.MajorFaults = majorFaults
				faultInfo.MinorFaults = minorFaults

				dt := now.Sub(lastFaultTime).Seconds()
				if dt > 0 {
					pageDelta := deltaUint64(pageFaults, lastFaults.pageFaults)
					majorDelta := deltaUint64(majorFaults, lastFaults.majorFaults)
					swapInDelta := deltaUint64(vmFaults.swapIn, lastFaults.swapIn)
					swapOutDelta := deltaUint64(vmFaults.swapOut, lastFaults.swapOut)

					faultInfo.PageFaultRate = float64(pageDelta) / dt
					faultInfo.MajorFaultRate = float64(majorDelta) / dt
					faultInfo.MinorFaultRate = faultInfo.PageFaultRate - faultInfo.MajorFaultRate
					if faultInfo.MinorFaultRate < 0 {
						faultInfo.MinorFaultRate = 0
					}
					faultInfo.SwapIn = vmFaults.swapIn
					faultInfo.SwapOut = vmFaults.swapOut
					faultInfo.SwapInRate = float64(swapInDelta) / dt
					faultInfo.SwapOutRate = float64(swapOutDelta) / dt
				}
				lastFaults = vmFaults
				lastFaultTime = now
			}

			for _, n := range netIO {
				pbIO.Networks = append(pbIO.Networks, &pb.NetworkInterface{Name: n.Name, RecvBytes: n.BytesRecv, SentBytes: n.BytesSent})
				pbIO.TotalNetRecvBytes += n.BytesRecv
				pbIO.TotalNetSentBytes += n.BytesSent
			}
			for name, d := range diskIO {
				pbIO.Disks = append(pbIO.Disks, &pb.DiskDevice{Name: name, ReadBytes: d.ReadBytes, WriteBytes: d.WriteBytes})
				pbIO.TotalReadBytes += d.ReadBytes
				pbIO.TotalWriteBytes += d.WriteBytes
			}
			cpuInfo := &pb.CPUInfo{Total: cc[0], Cores: cp}
			for i, usage := range cp {
				ct := pb.CPUInfo_Core_PERFORMANCE
				if i < len(coreTypes) {
					ct = coreTypes[i]
				}
				cpuInfo.CoreDetails = append(cpuInfo.CoreDetails, &pb.CPUInfo_Core{Index: uint32(i), Usage: usage, Type: ct})
			}
			stats := &pb.SystemStats{Gpus: gs, Cpu: cpuInfo, Memory: &pb.MemoryInfo{Total: vm.Total, Used: vm.Used, Percent: float32(vm.UsedPercent)}, Io: pbIO, Faults: faultInfo}
			psList, _ := ps.Processes()
			for _, p := range psList {
				n, _ := p.Name()
				pp, _ := p.Ppid()
				ct, _ := p.CreateTime()
				ccp := 0.0
				if times, err := p.Times(); err == nil {
					totalCPU := times.Total()
					if prev, ok := procCPUSamples[p.Pid]; ok && prev.createTime == ct {
						dt := now.Sub(prev.sampleTime).Seconds()
						if dt > 0 {
							ccp = ((totalCPU - prev.totalCPU) / dt) * 100 / cpuScale
							if ccp < 0 || math.IsNaN(ccp) || math.IsInf(ccp, 0) {
								ccp = 0
							}
						}
					}
					if ct > 0 {
						procCPUSamples[p.Pid] = procCPUSample{createTime: ct, totalCPU: totalCPU, sampleTime: now}
					}
				}
				mp, _ := p.MemoryPercent()
				u, _ := p.Username()
				cmdl, _ := p.Cmdline()
				gmem, gid, gutil := uint32(0), uint32(0), uint32(0)
				if info, ok := gm[p.Pid]; ok {
					gmem, gid, gutil = info.mem, info.gpu, info.util
				}
				minorFaults, majorFaults := uint64(0), uint64(0)
				if faults, err := p.PageFaults(); err == nil && faults != nil {
					minorFaults = faults.MinorFaults
					majorFaults = faults.MajorFaults
				}
				currentPIDs[p.Pid] = struct{}{}
				stats.Processes = append(stats.Processes, &pb.Process{Pid: p.Pid, Ppid: pp, Name: n, Cpu: ccp, Mem: mp, User: u, GpuMem: gmem, GpuId: gid, GpuUtil: gutil, Cmdline: cmdl, CreateTime: ct, MinorFaults: minorFaults, MajorFaults: majorFaults})
			}
			for pid := range procCPUSamples {
				if _, ok := currentPIDs[pid]; !ok {
					delete(procCPUSamples, pid)
				}
			}
			data, _ := proto.Marshal(stats)
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	})

	r.POST("/shell-sessions", handleCreateShellSession)
	r.GET("/shell-sessions", handleListShellSessions)
	r.DELETE("/shell-sessions/:id", handleDeleteShellSession)
	r.POST("/shell-sessions/:id/input", handleSendShellSessionInput)
	r.GET("/ws/shell", serveShellWS)

	r.GET("/events/recent", func(c *gin.Context) {
		limit := 50
		if l := c.Query("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
				limit = parsed
			}
		}
		typeFilter := c.Query("type")
		records, source, err := runtimeSettingsStore.RecentEvents(limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if typeFilter != "" {
			filtered := make([]CapturedEventRecord, 0, len(records))
			for _, r := range records {
				if r.Event != nil && r.Event.Type == typeFilter {
					filtered = append(filtered, r)
				}
			}
			records = filtered
		}
		c.JSON(http.StatusOK, gin.H{"source": source, "events": records})
	})
	r.GET("/ws/shell-sessions", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		notifyCh := shellSessions.subscribe()
		defer shellSessions.unsubscribe(notifyCh)

		done := make(chan struct{})
		go func() {
			defer close(done)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					return
				}
			}
		}()

		sendList := func() {
			list := shellSessions.List()
			data, err := json.Marshal(list)
			if err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}

		sendList()

		for {
			select {
			case <-notifyCh:
				sendList()
			case <-done:
				return
			}
		}
	})

	r.POST("/hooks/event", func(c *gin.Context) {
		var payload map[string]interface{}
		if err := c.ShouldBindJSON(&payload); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}

		toolName, _ := payload["tool_name"].(string)
		hookEvent, _ := payload["hook_event_name"].(string)
		toolInput, _ := payload["tool_input"].(map[string]interface{})

		if toolName == "" {
			toolName, _ = payload["tool"].(string)
		}
		if hookEvent == "" {
			hookEvent, _ = payload["event"].(string)
		}

		path := ""
		if toolInput != nil {
			if cmd, ok := toolInput["command"].(string); ok {
				path = cmd
			} else if fp, ok := toolInput["file_path"].(string); ok {
				path = fp
			} else if args, ok := toolInput["arguments"].([]interface{}); ok && len(args) > 0 {
				path = fmt.Sprintf("%v", args)
			}
		}

		tag := "Native Hook"
		sourceCLI := strings.ToLower(strings.TrimSpace(c.GetHeader("X-Agent-CLI")))
		ua := strings.ToLower(c.GetHeader("User-Agent"))
		if sourceCLI == "claude" || strings.Contains(ua, "claude") {
			tag = "Claude Code"
		} else if sourceCLI == "gemini" || strings.Contains(ua, "gemini") {
			tag = "Gemini CLI"
		} else if sourceCLI == "codex" || strings.Contains(ua, "codex") {
			tag = "Codex"
		} else if sourceCLI == "copilot" || strings.Contains(ua, "copilot") || strings.Contains(ua, "gh-copilot") {
			tag = "GitHub Copilot"
		} else if sourceCLI == "kiro" || strings.Contains(ua, "kiro") {
			tag = "Kiro CLI"
		} else {
			if hookEvent == "BeforeTool" {
				tag = "Gemini CLI"
			} else if hookEvent == "preToolUse" {
				tag = "GitHub Copilot"
			} else if hookEvent == "agentSpawn" || hookEvent == "userPromptSubmit" || hookEvent == "stop" {
				tag = "Kiro CLI"
			} else if hookEvent == "PreToolUse" {
			}
		}

		broadcast <- &pb.Event{
			Type:      "native_hook",
			EventType: pb.EventType_NATIVE_HOOK,
			Tag:       tag,
			Comm: fmt.Sprintf("%s:%s", hookEvent, toolName),
			Path: path,
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/register", func(c *gin.Context) {
		if trackerMaps.AgentPids == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "agent pid map not initialized"})
			return
		}
		var req struct {
			PID uint32 `json:"pid"`
			Tag string `json:"tag,omitempty"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pid"})
			return
		}
		tag := req.Tag
		if tag == "" {
			tag = "AI Agent"
		}
		if err := trackerMaps.AgentPids.Put(req.PID, getTagID(tag)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/unregister", func(c *gin.Context) {
		if trackerMaps.AgentPids == nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "agent pid map not initialized"})
			return
		}
		var req struct {
			PID uint32 `json:"pid"`
		}
		if err := c.ShouldBindJSON(&req); err != nil || req.PID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pid"})
			return
		}
		_ = trackerMaps.AgentPids.Delete(req.PID)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	r.POST("/cluster/heartbeat", clusterHeartbeatHandler)
	r.POST("/cluster/register", clusterHeartbeatHandler)

	api := r.Group("/", authMiddleware())
	{
		config := api.Group("/config")
		{
			config.GET("/tags", func(c *gin.Context) {
				tagsMu.RLock()
				defer tagsMu.RUnlock()
				t := []string{}
				for _, n := range tagMap {
					t = append(t, n)
				}
				c.JSON(200, t)
			})
			config.POST("/tags", func(c *gin.Context) {
				var r struct {
					Name string `json:"name"`
				}
				_ = c.ShouldBindJSON(&r)
				getTagID(r.Name)
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/comms", func(c *gin.Context) {
				items := []gin.H{}
				iter := objs.TrackedComms.Iterate()
				var k [16]byte
				var tid uint32
				for iter.Next(&k, &tid) {
					items = append(items, gin.H{"comm": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)})
				}
				c.JSON(200, items)
			})
			config.POST("/comms", func(c *gin.Context) {
				var r struct {
					Comm, Tag string `json:"comm" json:"tag"`
				}
				_ = c.ShouldBindJSON(&r)
				var k [16]byte
				copy(k[:], r.Comm)
				_ = objs.TrackedComms.Put(k, getTagID(r.Tag))
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/comms/:comm", func(c *gin.Context) {
				var k [16]byte
				copy(k[:], c.Param("comm"))
				_ = objs.TrackedComms.Delete(k)
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/paths", func(c *gin.Context) {
				items := []gin.H{}
				iter := objs.TrackedPaths.Iterate()
				var k [256]byte
				var tid uint32
				for iter.Next(&k, &tid) {
					items = append(items, gin.H{"path": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)})
				}
				c.JSON(200, items)
			})
			config.POST("/paths", func(c *gin.Context) {
				var r struct {
					Path, Tag string `json:"path" json:"tag"`
				}
				_ = c.ShouldBindJSON(&r)
				var k [256]byte
				copy(k[:], r.Path)
				_ = objs.TrackedPaths.Put(k, getTagID(r.Tag))
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/paths/*path", func(c *gin.Context) {
				p := c.Param("path")
				if len(p) > 0 && p[0] == '/' {
					p = p[1:]
				}
				var k [256]byte
				copy(k[:], p)
				_ = objs.TrackedPaths.Delete(k)
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/prefixes", func(c *gin.Context) {
				items := []gin.H{}
				if objs.TrackedPrefixes == nil {
					c.JSON(200, items)
					return
				}
				iter := objs.TrackedPrefixes.Iterate()
				var k struct {
					PrefixLen uint32
					Data      [64]byte
				}
				var tid uint32
				for iter.Next(&k, &tid) {
					prefix := string(bytes.TrimRight(k.Data[:], "\x00"))
					prefixLen := k.PrefixLen / 8
					if prefixLen > 0 && uint32(len(prefix)) > prefixLen {
						prefix = prefix[:prefixLen]
					}
					items = append(items, gin.H{"prefix": prefix, "tag": getTagName(tid)})
				}
				c.JSON(200, items)
			})
			config.POST("/prefixes", func(c *gin.Context) {
				var r struct {
					Prefix string `json:"prefix"`
					Tag    string `json:"tag"`
				}
				_ = c.ShouldBindJSON(&r)
				if r.Prefix == "" {
					c.JSON(400, gin.H{"error": "prefix is required"})
					return
				}
				var k struct {
					PrefixLen uint32
					Data      [64]byte
				}
				plen := len(r.Prefix)
				if plen > 63 {
					plen = 63
				}
				k.PrefixLen = uint32(plen * 8)
				copy(k.Data[:], r.Prefix[:plen])
				_ = objs.TrackedPrefixes.Put(k, getTagID(r.Tag))
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/prefixes", func(c *gin.Context) {
				prefix := c.Query("prefix")
				if prefix == "" {
					c.JSON(400, gin.H{"error": "prefix query parameter is required"})
					return
				}
				var k struct {
					PrefixLen uint32
					Data      [64]byte
				}
				plen := len(prefix)
				if plen > 63 {
					plen = 63
				}
				k.PrefixLen = uint32(plen * 8)
				copy(k.Data[:], prefix[:plen])
				_ = objs.TrackedPrefixes.Delete(k)
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/rules", func(c *gin.Context) { rulesMu.RLock(); defer rulesMu.RUnlock(); c.JSON(200, wrapperRules) })
			config.POST("/rules", func(c *gin.Context) {
				var r WrapperRule
				_ = c.ShouldBindJSON(&r)
				rulesMu.Lock()
				wrapperRules[r.Comm] = r
				rulesMu.Unlock()
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/rules/:comm", func(c *gin.Context) {
				rulesMu.Lock()
				delete(wrapperRules, c.Param("comm"))
				rulesMu.Unlock()
				c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/runtime", func(c *gin.Context) {
				c.JSON(http.StatusOK, buildRuntimeConfigResponse())
			})
			config.PUT("/runtime", func(c *gin.Context) {
				var req RuntimeSettings
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": "invalid runtime settings"})
					return
				}
				settings, err := runtimeSettingsStore.Replace(req)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				applyRetentionConfig(settings)
				c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
			})
			config.POST("/access-token", func(c *gin.Context) {
				settings, err := runtimeSettingsStore.RotateAccessToken()
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, buildRuntimeConfigResponseFromSettings(settings))
			})
			config.GET("/export", func(c *gin.Context) {
				runtimeSnapshot := runtimeSettingsStore.Snapshot()
				cfg := ExportConfig{Comms: make(map[string]string), Paths: make(map[string]string), Rules: make(map[string]WrapperRule), Runtime: &runtimeSnapshot}
				tagsMu.RLock()
				for _, n := range tagMap {
					cfg.Tags = append(cfg.Tags, n)
				}
				tagsMu.RUnlock()
				var k16 [16]byte
				var k256 [256]byte
				var tid uint32
				i1 := objs.TrackedComms.Iterate()
				for i1.Next(&k16, &tid) {
					cfg.Comms[string(bytes.TrimRight(k16[:], "\x00"))] = getTagName(tid)
				}
				i2 := objs.TrackedPaths.Iterate()
				for i2.Next(&k256, &tid) {
					cfg.Paths[string(bytes.TrimRight(k256[:], "\x00"))] = getTagName(tid)
				}
				rulesMu.RLock()
				for comm, rule := range wrapperRules {
					cfg.Rules[comm] = rule
				}
				rulesMu.RUnlock()
				c.JSON(200, cfg)
			})
			config.POST("/import", func(c *gin.Context) {
				var cfg ExportConfig
				_ = c.ShouldBindJSON(&cfg)
				if cfg.Runtime != nil {
					if _, err := runtimeSettingsStore.Replace(*cfg.Runtime); err != nil {
						c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
						return
					}
				}
				for _, t := range cfg.Tags {
					getTagID(t)
				}
				for comm, tag := range cfg.Comms {
					var k [16]byte
					copy(k[:], comm)
					_ = objs.TrackedComms.Put(k, getTagID(tag))
				}
				for p, tag := range cfg.Paths {
					var k [256]byte
					copy(k[:], p)
					_ = objs.TrackedPaths.Put(k, getTagID(tag))
				}
				rulesMu.Lock()
				wrapperRules = make(map[string]WrapperRule, len(cfg.Rules))
				for comm, rule := range cfg.Rules {
					wrapperRules[comm] = rule
				}
				rulesMu.Unlock()
				c.JSON(200, gin.H{"status": "ok"})
			})
			api.Any("/mcp", gin.WrapH(buildMCPHandler()))
			cluster := api.Group("/cluster")
			{
				cluster.GET("/state", clusterStateHandler)
				cluster.GET("/nodes", clusterNodesHandler)
			}
			hooks := config.Group("/hooks")
			{
				hooks.GET("", func(c *gin.Context) {
					res := []gin.H{}
					for _, h := range availableHooks {
						res = append(res, gin.H{
							"id": h.ID, "name": h.Name, "description": h.Description,
							"target_cmd": h.TargetCmd, "hook_type": h.HookType,
							"installed": isHookInstalled(h),
						})
					}
					c.JSON(200, res)
				})
				hooks.POST("", func(c *gin.Context) {
					var req struct {
						ID         string `json:"id"`
						Install    bool   `json:"install"`
						UseWrapper bool   `json:"use_wrapper"`
					}
					if err := c.ShouldBindJSON(&req); err != nil {
						c.JSON(400, gin.H{"error": "invalid request"})
						return
					}
					var target HookDef
					found := false
					for _, h := range availableHooks {
						if h.ID == req.ID {
							target = h
							found = true
							break
						}
					}
					if !found {
						c.JSON(404, gin.H{"error": "hook not found"})
						return
					}

					effectiveType := target.HookType
					if req.UseWrapper {
						effectiveType = HookTypeWrapper
					}

					if req.Install {
						if effectiveType == HookTypeNative {
							if err := installNativeHook(target); err != nil {
								c.JSON(500, gin.H{"error": err.Error()})
								return
							}
						} else {
							p := getShellConfigPath()
							b, _ := os.ReadFile(p)
							content := string(b)
							aliasLine := fmt.Sprintf("\nalias %s='agent-wrapper %s' # agent-ebpf-hook\n", target.TargetCmd, target.TargetCmd)
							if !strings.Contains(content, fmt.Sprintf("alias %s=", target.TargetCmd)) {
								f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0644)
								if err != nil {
									c.JSON(500, gin.H{"error": err.Error()})
									return
								}
								f.WriteString(aliasLine)
								f.Close()
							}
						}
					} else {
						if target.HookType == HookTypeNative {
							_ = uninstallNativeHook(target)
						}
						p := getShellConfigPath()
						b, _ := os.ReadFile(p)
						lines := strings.Split(string(b), "\n")
						newLines := []string{}
						for _, l := range lines {
							if !strings.Contains(l, fmt.Sprintf("alias %s=", target.TargetCmd)) {
								newLines = append(newLines, l)
							}
						}
						_ = os.WriteFile(p, []byte(strings.Join(newLines, "\n")), 0644)
					}
					c.JSON(200, gin.H{"status": "ok"})
				})
				hooks.GET("/:id/raw", func(c *gin.Context) {
					id := c.Param("id")
					var target HookDef
					found := false
					for _, h := range availableHooks {
						if h.ID == id {
							target = h
							found = true
							break
						}
					}
					if !found || target.HookType != HookTypeNative {
						c.JSON(404, gin.H{"error": "native hook not found"})
						return
					}
					if target.ID == "kiro" {
						if err := ensureKiroManagedAgentExists(); err != nil {
							c.JSON(500, gin.H{"error": err.Error()})
							return
						}
					}
					b, err := os.ReadFile(target.NativeConfigPath)
					if err != nil {
						if os.IsNotExist(err) {
							c.JSON(200, gin.H{"content": "{}", "path": target.NativeConfigPath, "format": target.ConfigFormat})
							return
						}
						c.JSON(500, gin.H{"error": err.Error()})
						return
					}
					c.JSON(200, gin.H{"content": string(b), "path": target.NativeConfigPath, "format": target.ConfigFormat})
				})
				hooks.POST("/:id/raw", func(c *gin.Context) {
					id := c.Param("id")
					var req struct {
						Content string `json:"content"`
					}
					if err := c.ShouldBindJSON(&req); err != nil {
						c.JSON(400, gin.H{"error": "invalid request"})
						return
					}
					var target HookDef
					found := false
					for _, h := range availableHooks {
						if h.ID == id {
							target = h
							found = true
							break
						}
					}
					if !found || target.HookType != HookTypeNative {
						c.JSON(404, gin.H{"error": "native hook not found"})
						return
					}
					var js map[string]interface{}
					if err := json.Unmarshal([]byte(req.Content), &js); err != nil {
						c.JSON(400, gin.H{"error": "invalid JSON: " + err.Error()})
						return
					}

					if err := os.MkdirAll(filepath.Dir(target.NativeConfigPath), 0755); err != nil {
						c.JSON(500, gin.H{"error": err.Error()})
						return
					}
					if err := os.WriteFile(target.NativeConfigPath, []byte(req.Content), 0644); err != nil {
						c.JSON(500, gin.H{"error": err.Error()})
						return
					}
					c.JSON(200, gin.H{"status": "ok"})
				})
			}
		}
		system := api.Group("/system")
		{
			system.GET("/ls", func(c *gin.Context) {
				p := c.DefaultQuery("path", "/")
				e, _ := os.ReadDir(p)
				l := []gin.H{}
				for _, v := range e {
					fp := p
					if fp == "/" {
						fp = "/" + v.Name()
					} else {
						fp = fp + "/" + v.Name()
					}
					l = append(l, gin.H{"name": v.Name(), "isDir": v.IsDir(), "path": fp})
				}
				c.JSON(200, l)
			})
			system.GET("/file-preview", func(c *gin.Context) {
				targetPath := strings.TrimSpace(c.Query("path"))
				if targetPath == "" {
					c.JSON(400, gin.H{"error": "path is required"})
					return
				}

				preview, err := buildFilePreview(targetPath)
				if err != nil {
					if os.IsNotExist(err) {
						c.JSON(404, gin.H{"error": "path not found"})
						return
					}
					c.JSON(500, gin.H{"error": err.Error()})
					return
				}
				c.JSON(200, preview)
			})
			system.GET("/env", handleListLaunchEnvEntries)
			system.POST("/run", func(c *gin.Context) {
				var r struct {
					Comm string   `json:"comm"`
					Args []string `json:"args"`
				}
				if err := c.ShouldBindJSON(&r); err == nil {
					wb := resolveWrapperPath()
					if wb == "" {
						c.JSON(500, gin.H{"error": "wrapper not found"})
						return
					}
					cmd := exec.Command(wb, append([]string{r.Comm}, r.Args...)...)
					cmd.Env = os.Environ()
					dropPrivileges(cmd)
					if err := cmd.Start(); err != nil {
						c.JSON(500, gin.H{"error": err.Error()})
						return
					}
					c.JSON(200, gin.H{"status": "started", "pid": cmd.Process.Pid})
				}
			})
			data := api.Group("/data")
			{
				data.POST("/clear-events", func(c *gin.Context) {
					capturedEventArchive.Clear()
					if err := runtimeSettingsStore.TruncateEventLog(); err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, gin.H{"status": "ok"})
				})
				data.POST("/clear-events-memory", func(c *gin.Context) {
					capturedEventArchive.Clear()
					c.JSON(http.StatusOK, gin.H{"status": "ok"})
				})
				data.POST("/clear-events-persisted", func(c *gin.Context) {
					if err := runtimeSettingsStore.TruncateEventLog(); err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
						return
					}
					c.JSON(http.StatusOK, gin.H{"status": "ok"})
				})
			}
			api.POST("/shell-sessions/cleanup", func(c *gin.Context) {
				shellSessions.ClearClosed()
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})
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
			actualPort = startPort + i
			l.Close()
			break
		}
	}
	clusterManagerStore.ConfigurePort(actualPort)
	writePortFile(actualPort)
	startClusterHeartbeatLoop()
	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
