package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	bpf "agent-ebpf-filter/ebpf"
	"agent-ebpf-filter/pb"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	ps "github.com/shirou/gopsutil/v3/process"
	gnet "github.com/shirou/gopsutil/v3/net"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const udsPath = "/tmp/agent-ebpf.sock"

type bpfEvent struct {
	PID, PPID, UID, Type, TagID uint32
	Comm                        [16]byte
	Path                        [256]byte
}

type WrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
}

type ExportConfig struct {
	Tags  []string          `json:"tags"`
	Comms map[string]string `json:"comms"`
	Paths map[string]string `json:"paths"`
}

var (
	tagsMu      sync.RWMutex
	tagMap      = map[uint32]string{0: "Unknown", 1: "AI Agent", 2: "Git", 3: "Build Tool", 4: "Package Manager", 5: "Runtime", 6: "System Tool", 7: "Network Tool", 8: "Security"}
	tagNameToID = map[string]uint32{"AI Agent": 1, "Git": 2, "Build Tool": 3, "Package Manager": 4, "Runtime": 5, "System Tool": 6, "Network Tool": 7, "Security": 8}
	nextTagID   uint32 = 9

	rulesMu      sync.RWMutex
	wrapperRules = make(map[string]WrapperRule)
)

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if os.Getenv("GIN_MODE") != "release" && (os.Getenv("DISABLE_AUTH") == "true" || os.Getenv("DISABLE_AUTH") == "") {
			c.Next()
			return
		}
		apiKey := c.GetHeader("X-API-KEY")
		expectedKey := os.Getenv("AGENT_API_KEY")
		if expectedKey == "" { expectedKey = "agent-secret-123" }
		if apiKey != expectedKey {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}
		c.Next()
	}
}

func getTagName(id uint32) string {
	tagsMu.RLock(); defer tagsMu.RUnlock()
	if name, ok := tagMap[id]; ok { return name }
	return fmt.Sprintf("Tag-%d", id)
}

func getTagID(name string) uint32 {
	tagsMu.Lock(); defer tagsMu.Unlock()
	if id, ok := tagNameToID[name]; ok { return id }
	id := nextTagID; tagMap[id] = name; tagNameToID[name] = id; nextTagID++
	return id
}

type gpuInfo struct{ mem, gpu uint32 }

func getGPUPidMap() map[int32]gpuInfo {
	gpuMap := make(map[int32]gpuInfo)
	cmd := exec.Command("nvidia-smi", "--query-compute-apps=pid,used_memory,gpu_index", "--format=csv,noheader,nounits")
	output, _ := cmd.Output()
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 { continue }
		parts := bytes.Split(line, []byte(", "))
		if len(parts) >= 2 {
			var pid int32; var mem, gpu uint32
			fmt.Sscanf(string(parts[0]), "%d", &pid)
			fmt.Sscanf(string(parts[1]), "%d", &mem)
			if len(parts) > 2 { fmt.Sscanf(string(parts[2]), "%d", &gpu) }
			gpuMap[pid] = gpuInfo{mem, gpu}
		}
	}
	return gpuMap
}

func getGlobalGPUStatus() []*pb.GPUStatus {
	var gpus []*pb.GPUStatus
	cmd := exec.Command("nvidia-smi", "--query-gpu=index,name,utilization.gpu,utilization.memory,memory.total,memory.used,temperature.gpu", "--format=csv,noheader,nounits")
	output, _ := cmd.Output()
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 { continue }
		parts := bytes.Split(line, []byte(", "))
		if len(parts) == 7 {
			var idx, ug, um, mt, mu, temp uint32
			fmt.Sscanf(string(parts[0]), "%d", &idx)
			fmt.Sscanf(string(parts[2]), "%d", &ug)
			fmt.Sscanf(string(parts[3]), "%d", &um)
			fmt.Sscanf(string(parts[4]), "%d", &mt)
			fmt.Sscanf(string(parts[5]), "%d", &mu)
			fmt.Sscanf(string(parts[6]), "%d", &temp)
			gpus = append(gpus, &pb.GPUStatus{Index: idx, Name: string(parts[1]), UtilGpu: ug, UtilMem: um, MemTotal: mt, MemUsed: mu, Temp: temp})
		}
	}
	return gpus
}

func startUDSServer(broadcast chan *pb.Event) {
	_ = os.Remove(udsPath)
	l, err := net.Listen("unix", udsPath)
	if err != nil { return }
	_ = os.Chmod(udsPath, 0666)
	defer l.Close()
	for {
		conn, err := l.Accept()
		if err != nil { continue }
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 4096)
			for {
				n, err := c.Read(buf)
				if err != nil { return }
				req := &pb.WrapperRequest{}
				if err := proto.Unmarshal(buf[:n], req); err != nil { continue }
				resp := &pb.WrapperResponse{Action: pb.WrapperResponse_ALLOW}
				rulesMu.RLock(); rule, ok := wrapperRules[req.Comm]; rulesMu.RUnlock()
				if ok {
					switch rule.Action {
					case "BLOCK": resp.Action = pb.WrapperResponse_BLOCK; resp.Message = "Blocked by policy"
					case "ALERT": resp.Action = pb.WrapperResponse_ALERT; resp.Message = "Security alert"
					case "REWRITE": resp.Action = pb.WrapperResponse_REWRITE; resp.RewrittenArgs = rule.RewrittenCmd
					}
				}
				broadcast <- &pb.Event{Pid: req.Pid, Comm: req.Comm, Type: "wrapper_intercept", Tag: "Wrapper", Path: strings.Join(append([]string{req.Comm}, req.Args...), " ")}
				out, _ := proto.Marshal(resp); _, _ = c.Write(out)
			}
		}(conn)
	}
}

func getCoreTypes() []pb.CPUInfo_Core_Type {
	cores, _ := cpu.Counts(true)
	types := make([]pb.CPUInfo_Core_Type, cores)
	
	maxFreqs := make([]int64, cores)
	overallMax := int64(0)

	for i := 0; i < cores; i++ {
		// Try to read core_type if available (Intel Hybrid)
		data, err := os.ReadFile(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/topology/core_type", i))
		if err == nil {
			val := strings.TrimSpace(string(data))
			if val == "intel_atom" {
				types[i] = pb.CPUInfo_Core_EFFICIENCY
				continue
			} else if val == "intel_core" {
				types[i] = pb.CPUInfo_Core_PERFORMANCE
				continue
			}
		}

		// Fallback to frequency analysis
		freqData, err := os.ReadFile(fmt.Sprintf("/sys/devices/system/cpu/cpu%d/cpufreq/cpuinfo_max_freq", i))
		if err == nil {
			fmt.Sscanf(string(freqData), "%d", &maxFreqs[i])
			if maxFreqs[i] > overallMax {
				overallMax = maxFreqs[i]
			}
		}
	}

	// If we used frequency fallback
	if overallMax > 0 {
		for i := 0; i < cores; i++ {
			if types[i] != 0 { continue } // Already set
			// If freq is significantly lower (e.g. < 80% of max), likely E-core
			if maxFreqs[i] < (overallMax * 8 / 10) {
				types[i] = pb.CPUInfo_Core_EFFICIENCY
			} else {
				types[i] = pb.CPUInfo_Core_PERFORMANCE
			}
		}
	}

	return types
}

func main() {
	if os.Geteuid() != 0 {
		executable, _ := os.Executable(); isDesktop := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
		sudoCmd := "sudo"
		if isDesktop { if p, err := exec.LookPath("pkexec"); err == nil { sudoCmd = p } }
		cmd := exec.Command(sudoCmd, append([]string{executable}, os.Args[1:]...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		_ = cmd.Run(); os.Exit(0)
	}

	procsList, _ := ps.Processes(); curr := int32(os.Getpid())
	for _, p := range procsList {
		if p.Pid != curr {
			if n, _ := p.Name(); n == "agent-ebpf-filter" || n == "main" { _ = p.Kill() }
		}
	}

	_ = rlimit.RemoveMemlock()
	var objs bpf.AgentTrackerObjects
	if err := bpf.LoadAgentTrackerObjects(&objs, nil); err != nil { log.Fatalf("Load eBPF: %v", err) }
	defer objs.Close()

	attach := func(c, n string, p *ebpf.Program) link.Link {
		l, err := link.Tracepoint(c, n, p, nil)
		if err != nil { log.Printf("Link %s/%s: %v", c, n, err) }
		return l
	}
	links := []link.Link{
		attach("syscalls", "sys_enter_execve", objs.TracepointSyscallsSysEnterExecve),
		attach("syscalls", "sys_enter_openat", objs.TracepointSyscallsSysEnterOpenat),
		attach("syscalls", "sys_enter_connect", objs.TracepointSyscallsSysEnterConnect),
		attach("syscalls", "sys_enter_mkdirat", objs.TracepointSyscallsSysEnterMkdirat),
		attach("syscalls", "sys_enter_unlinkat", objs.TracepointSyscallsSysEnterUnlinkat),
		attach("syscalls", "sys_enter_ioctl", objs.TracepointSyscallsSysEnterIoctl),
		attach("syscalls", "sys_enter_bind", objs.TracepointSyscallsSysEnterBind),
	}
	for _, l := range links { if l != nil { defer l.Close() } }

	rd, _ := ringbuf.NewReader(objs.Events); defer rd.Close()

	broadcast := make(chan *pb.Event, 100)
	go func() {
		var event bpfEvent
		types := map[uint32]string{0: "execve", 1: "openat", 2: "network_connect", 3: "mkdir", 4: "unlink", 5: "ioctl", 6: "network_bind"}
		for {
			record, err := rd.Read(); if err != nil { return }
			_ = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			broadcast <- &pb.Event{Pid: event.PID, Ppid: event.PPID, Uid: event.UID, Type: types[event.Type], Tag: getTagName(event.TagID), Comm: string(bytes.TrimRight(event.Comm[:], "\x00")), Path: string(bytes.TrimRight(event.Path[:], "\x00"))}
		}
	}()

	go startUDSServer(broadcast)

	r := gin.Default()
	clients := make(map[*websocket.Conn]bool)
	var clientsMu sync.Mutex
	go func() {
		for event := range broadcast {
			data, _ := proto.Marshal(event)
			clientsMu.Lock()
			for c := range clients {
				if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil { c.Close(); delete(clients, c) }
			}
			clientsMu.Unlock()
		}
	}()

	r.GET("/ws", func(c *gin.Context) {
		conn, _ := upgrader.Upgrade(c.Writer, c.Request, nil)
		clientsMu.Lock(); clients[conn] = true; clientsMu.Unlock()
	})

	r.GET("/ws/system", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil { return }
		defer conn.Close()

		intervalStr := c.DefaultQuery("interval", "2000")
		iv, _ := time.ParseDuration(intervalStr + "ms")
		if iv < 500*time.Millisecond { iv = 500 * time.Millisecond }
		ticker := time.NewTicker(iv); defer ticker.Stop()

		coreTypes := getCoreTypes()

		for range ticker.C {
			gm, gs := getGPUPidMap(), getGlobalGPUStatus()
			vm, _ := mem.VirtualMemory()
			cc, _ := cpu.Percent(0, false)
			cp, _ := cpu.Percent(0, true)
			
			// Per-interface/disk I/O
			netIO, _ := gnet.IOCounters(true)
			diskIO, _ := disk.IOCounters()
			
			pbIO := &pb.IOInfo{}
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
				if i < len(coreTypes) { ct = coreTypes[i] }
				cpuInfo.CoreDetails = append(cpuInfo.CoreDetails, &pb.CPUInfo_Core{
					Index: uint32(i), Usage: usage, Type: ct,
				})
			}

			stats := &pb.SystemStats{
				Gpus: gs, Cpu: cpuInfo,
				Memory: &pb.MemoryInfo{Total: vm.Total, Used: vm.Used, Percent: float32(vm.UsedPercent)},
				Io: pbIO,
			}

			psList, _ := ps.Processes()
			for _, p := range psList {
				n, _ := p.Name(); pp, _ := p.Ppid(); ccp, _ := p.CPUPercent(); mp, _ := p.MemoryPercent(); u, _ := p.Username()
				gmem, gid := uint32(0), uint32(0)
				if info, ok := gm[p.Pid]; ok { gmem, gid = info.mem, info.gpu }
				stats.Processes = append(stats.Processes, &pb.Process{Pid: p.Pid, Ppid: pp, Name: n, Cpu: ccp, Mem: mp, User: u, GpuMem: gmem, GpuId: gid})
			}
			data, _ := proto.Marshal(stats); if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil { return }
		}
	})

	api := r.Group("/", authMiddleware())
	{
		config := api.Group("/config")
		{
			config.GET("/tags", func(c *gin.Context) {
				tagsMu.RLock(); defer tagsMu.RUnlock(); t := []string{}; for _, n := range tagMap { t = append(t, n) }; c.JSON(200, t)
			})
			config.POST("/tags", func(c *gin.Context) {
				var req struct{ Name string `json:"name"` }; _ = c.ShouldBindJSON(&req); getTagID(req.Name); c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/comms", func(c *gin.Context) {
				items := []gin.H{}; iter := objs.TrackedComms.Iterate(); var k [16]byte; var tid uint32
				for iter.Next(&k, &tid) { items = append(items, gin.H{"comm": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)}) }; c.JSON(200, items)
			})
			config.POST("/comms", func(c *gin.Context) {
				var req struct{ Comm, Tag string `json:"comm" json:"tag"` }; _ = c.ShouldBindJSON(&req); var k [16]byte; copy(k[:], req.Comm); _ = objs.TrackedComms.Put(k, getTagID(req.Tag)); c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/comms/:comm", func(c *gin.Context) {
				var k [16]byte; copy(k[:], c.Param("comm")); _ = objs.TrackedComms.Delete(k); c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/paths", func(c *gin.Context) {
				items := []gin.H{}; iter := objs.TrackedPaths.Iterate(); var k [256]byte; var tid uint32
				for iter.Next(&k, &tid) { items = append(items, gin.H{"path": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)}) }; c.JSON(200, items)
			})
			config.POST("/paths", func(c *gin.Context) {
				var req struct{ Path, Tag string `json:"path" json:"tag"` }; _ = c.ShouldBindJSON(&req); var k [256]byte; copy(k[:], req.Path); _ = objs.TrackedPaths.Put(k, getTagID(req.Tag)); c.JSON(200, gin.H{"status": "ok"})
			})
			config.DELETE("/paths/*path", func(c *gin.Context) {
				p := c.Param("path"); if len(p) > 0 && p[0] == '/' { p = p[1:] }; var k [256]byte; copy(k[:], p); _ = objs.TrackedPaths.Delete(k); c.JSON(200, gin.H{"status": "ok"})
			})
			config.GET("/rules", func(c *gin.Context) { rulesMu.RLock(); defer rulesMu.RUnlock(); c.JSON(200, wrapperRules) })
			config.POST("/rules", func(c *gin.Context) { var r WrapperRule; _ = c.ShouldBindJSON(&r); rulesMu.Lock(); wrapperRules[r.Comm] = r; rulesMu.Unlock(); c.JSON(200, gin.H{"status": "ok"}) })
			config.DELETE("/rules/:comm", func(c *gin.Context) { rulesMu.Lock(); delete(wrapperRules, c.Param("comm")); rulesMu.Unlock(); c.JSON(200, gin.H{"status": "ok"}) })
			config.GET("/export", func(c *gin.Context) {
				cfg := ExportConfig{Comms: make(map[string]string), Paths: make(map[string]string)}
				tagsMu.RLock(); for _, n := range tagMap { cfg.Tags = append(cfg.Tags, n) }; tagsMu.RUnlock()
				var k16 [16]byte; var k256 [256]byte; var tid uint32
				i1 := objs.TrackedComms.Iterate(); for i1.Next(&k16, &tid) { cfg.Comms[string(bytes.TrimRight(k16[:], "\x00"))] = getTagName(tid) }
				i2 := objs.TrackedPaths.Iterate(); for i2.Next(&k256, &tid) { cfg.Paths[string(bytes.TrimRight(k256[:], "\x00"))] = getTagName(tid) }
				c.JSON(200, cfg)
			})
			config.POST("/import", func(c *gin.Context) {
				var cfg ExportConfig; _ = c.ShouldBindJSON(&cfg)
				for _, t := range cfg.Tags { getTagID(t) }
				for comm, tag := range cfg.Comms { var k [16]byte; copy(k[:], comm); _ = objs.TrackedComms.Put(k, getTagID(tag)) }
				for p, tag := range cfg.Paths { var k [256]byte; copy(k[:], p); _ = objs.TrackedPaths.Put(k, getTagID(tag)) }
				c.JSON(200, gin.H{"status": "ok"})
			})
		}
		system := api.Group("/system")
		{
			system.GET("/ls", func(c *gin.Context) {
				p := c.DefaultQuery("path", "/"); e, _ := os.ReadDir(p); l := []gin.H{}
				for _, v := range e {
					fp := p; if fp == "/" { fp = "/" + v.Name() } else { fp = fp + "/" + v.Name() }
					l = append(l, gin.H{"name": v.Name(), "isDir": v.IsDir(), "path": fp})
				}
				c.JSON(200, l)
			})
			system.POST("/run", func(c *gin.Context) {
				var req struct { Comm string `json:"comm"`; Args []string `json:"args"` }
				if err := c.ShouldBindJSON(&req); err == nil {
					cwd, _ := os.Getwd(); execPath, _ := os.Executable()
					candidates := []string{filepath.Join(cwd, "..", "agent-wrapper"), filepath.Join(cwd, "agent-wrapper"), filepath.Join(filepath.Dir(execPath), "agent-wrapper"), filepath.Join(filepath.Dir(execPath), "..", "agent-wrapper"), "./agent-wrapper", "../agent-wrapper"}
					var wb string
					for _, cnd := range candidates { if info, err := os.Stat(cnd); err == nil && !info.IsDir() { wb = cnd; break } }
					if wb == "" { c.JSON(500, gin.H{"error": "wrapper not found"}); return }
					cmd := exec.Command(wb, append([]string{req.Comm}, req.Args...)...)
					cmd.Env = os.Environ()
					if err := cmd.Start(); err != nil { c.JSON(500, gin.H{"error": err.Error()}); return }
					c.JSON(200, gin.H{"status": "started", "pid": cmd.Process.Pid})
				}
			})
		}
	}

	staticDir := "../frontend/dist"
	if _, err := os.Stat(staticDir); err != nil { staticDir = "./frontend/dist" }
	r.StaticFile("/", filepath.Join(staticDir, "index.html"))
	r.Static("/assets", filepath.Join(staticDir, "assets"))
	r.NoRoute(func(c *gin.Context) { c.File(filepath.Join(staticDir, "index.html")) })

	commonCLIs := map[string]string{"git": "Git", "npm": "Package Manager", "bun": "Package Manager", "pnpm": "Package Manager", "yarn": "Package Manager", "node": "Runtime", "python": "Runtime", "python3": "Runtime", "go": "Build Tool", "cargo": "Build Tool", "rustc": "Build Tool", "gcc": "Build Tool", "g++": "Build Tool", "clang": "Build Tool", "make": "Build Tool", "cmake": "Build Tool", "docker": "System Tool", "kubectl": "Network Tool"}
	for cl, t := range commonCLIs { var k [16]byte; copy(k[:], cl); _ = objs.TrackedComms.Put(k, getTagID(t)) }
	
	startPort, maxTries, actualPort := 8080, 10, 8080
	for i := 0; i < maxTries; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort+i))
		if err == nil { actualPort = startPort + i; l.Close(); break }
	}
	_ = os.WriteFile(".port", []byte(fmt.Sprintf("%d", actualPort)), 0644)
	_ = r.Run(fmt.Sprintf(":%d", actualPort))
}
