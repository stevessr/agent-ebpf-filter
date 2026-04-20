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
	"github.com/shirou/gopsutil/v3/process"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow any origin for development
	},
}

// Event matching the C struct
type bpfEvent struct {
	PID   uint32
	PPID  uint32
	UID   uint32
	Type  uint32
	TagID uint32
	Comm  [16]byte
	Path  [256]byte
}

type WsEvent struct {
	PID  uint32 `json:"pid"`
	PPID uint32 `json:"ppid"`
	UID  uint32 `json:"uid"`
	Type string `json:"type"`
	Tag  string `json:"tag"`
	Comm string `json:"comm"`
	Path string `json:"path"`
}

var (
	tagsMu sync.RWMutex
	tagMap = map[uint32]string{
		0: "Unknown", 1: "AI Agent", 2: "Git",
		3: "Build Tool", 4: "Package Manager", 5: "Runtime",
		6: "System Tool", 7: "Network Tool", 8: "Security",
	}
	tagNameToID = map[string]uint32{
		"AI Agent": 1, "Git": 2, "Build Tool": 3,
		"Package Manager": 4, "Runtime": 5, "System Tool": 6,
		"Network Tool": 7, "Security": 8,
	}
	nextTagID uint32 = 9
)

func getTagName(id uint32) string {
	tagsMu.RLock()
	defer tagsMu.RUnlock()
	if name, ok := tagMap[id]; ok {
		return name
	}
	return fmt.Sprintf("Tag-%d", id)
}

func getTagID(name string) uint32 {
	tagsMu.Lock()
	defer tagsMu.Unlock()
	if id, ok := tagNameToID[name]; ok {
		return id
	}
	id := nextTagID
	tagMap[id] = name
	tagNameToID[name] = id
	nextTagID++
	return id
}

type ExportConfig struct {
	Tags  []string          `json:"tags"`
	Comms map[string]string `json:"comms"`
	Paths map[string]string `json:"paths"`
}

func getGPUPidMap() map[int32]uint32 {
	gpuMap := make(map[int32]uint32)
	// Query NVIDIA GPU for compute applications PIDs and their VRAM usage
	cmd := exec.Command("nvidia-smi", "--query-compute-apps=pid,used_memory", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return gpuMap
	}

	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}
		parts := bytes.Split(line, []byte(", "))
		if len(parts) == 2 {
			var pid int32
			var mem uint32
			fmt.Sscanf(string(parts[0]), "%d", &pid)
			fmt.Sscanf(string(parts[1]), "%d", &mem)
			gpuMap[pid] = mem
		}
	}
	return gpuMap
}

func main() {
	if os.Geteuid() != 0 {
		executable, _ := os.Executable()
		isDesktop := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
		sudoCmd := "sudo"
		if isDesktop {
			if _, err := exec.LookPath("pkexec"); err == nil {
				sudoCmd = "pkexec"
			}
		}
		fmt.Printf("Root privileges required for eBPF operations. Re-running with %s...\n", sudoCmd)
		cmd := exec.Command(sudoCmd, append([]string{executable}, os.Args[1:]...)...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Printf("Error: could not elevate privileges: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Kill existing instances to avoid conflicts
	currentPid := int32(os.Getpid())
	procs, _ := process.Processes()
	for _, p := range procs {
		if p.Pid == currentPid {
			continue
		}
		name, _ := p.Name()
		if name == "agent-ebpf-filter" || name == "main" {
			_ = p.Kill()
		}
	}

	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal("Failed to remove memlock:", err)
	}

	var objs bpf.AgentTrackerObjects
	if err := bpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
		log.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	attachTracepoint := func(category, name string, prog *ebpf.Program) link.Link {
		l, err := link.Tracepoint(category, name, prog, nil)
		if err != nil {
			log.Fatalf("opening tracepoint %s/%s: %s", category, name, err)
		}
		return l
	}

	links := []link.Link{
		attachTracepoint("syscalls", "sys_enter_execve", objs.TracepointSyscallsSysEnterExecve),
		attachTracepoint("syscalls", "sys_enter_openat", objs.TracepointSyscallsSysEnterOpenat),
		attachTracepoint("syscalls", "sys_enter_connect", objs.TracepointSyscallsSysEnterConnect),
		attachTracepoint("syscalls", "sys_enter_mkdirat", objs.TracepointSyscallsSysEnterMkdirat),
		attachTracepoint("syscalls", "sys_enter_unlinkat", objs.TracepointSyscallsSysEnterUnlinkat),
		attachTracepoint("syscalls", "sys_enter_ioctl", objs.TracepointSyscallsSysEnterIoctl),
		attachTracepoint("syscalls", "sys_enter_bind", objs.TracepointSyscallsSysEnterBind),
	}
	for _, l := range links {
		defer l.Close()
	}

	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		log.Fatalf("opening ringbuf reader: %s", err)
	}
	defer rd.Close()

	broadcast := make(chan *pb.Event, 100)
	go func() {
		var event bpfEvent
		types := map[uint32]string{
			0: "execve", 1: "openat", 2: "network_connect",
			3: "mkdir", 4: "unlink", 5: "ioctl", 6: "network_bind",
		}
		for {
			record, err := rd.Read()
			if err != nil {
				if err == ringbuf.ErrClosed {
					return
				}
				continue
			}
			if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
				continue
			}
			comm := string(bytes.TrimRight(event.Comm[:], "\x00"))
			path := string(bytes.TrimRight(event.Path[:], "\x00"))
			evtType, ok := types[event.Type]
			if !ok {
				evtType = "unknown"
			}
			pbEvent := &pb.Event{
				Pid:  event.PID,
				Ppid: event.PPID,
				Uid:  event.UID,
				Type: evtType,
				Tag:  getTagName(event.TagID),
				Comm: comm,
				Path: path,
			}
			select {
			case broadcast <- pbEvent:
			default:
			}
		}
	}()

	r := gin.Default()
	clients := make(map[*websocket.Conn]bool)
	var clientsMu sync.Mutex

	go func() {
		for event := range broadcast {
			data, _ := proto.Marshal(event)
			clientsMu.Lock()
			for client := range clients {
				if err := client.WriteMessage(websocket.BinaryMessage, data); err != nil {
					client.Close()
					delete(clients, client)
				}
			}
			clientsMu.Unlock()
		}
	}()

	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()
	})

	r.GET("/ws/system", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		intervalStr := c.DefaultQuery("interval", "2000")
		intervalMs, _ := time.ParseDuration(intervalStr + "ms")
		if intervalMs < 500*time.Millisecond {
			intervalMs = 500 * time.Millisecond
		}

		ticker := time.NewTicker(intervalMs)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				gpuMap := getGPUPidMap()
				ps, _ := process.Processes()
				pbList := &pb.ProcessList{}
				for _, p := range ps {
					n, _ := p.Name()
					pp, _ := p.Ppid()
					cp, _ := p.CPUPercent()
					mp, _ := p.MemoryPercent()
					u, _ := p.Username()

					gpuMem := uint32(0)
					if mem, ok := gpuMap[p.Pid]; ok {
						gpuMem = mem
					}

					pbList.Processes = append(pbList.Processes, &pb.Process{
						Pid: p.Pid, Ppid: pp, Name: n, Cpu: cp, Mem: mp, User: u, GpuMem: gpuMem,
					})
				}
				data, _ := proto.Marshal(pbList)
				if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
					return
				}
			}
		}
	})

	config := r.Group("/config")
	{
		config.GET("/tags", func(c *gin.Context) {
			tagsMu.RLock()
			defer tagsMu.RUnlock()
			var tags []string
			for _, name := range tagMap {
				tags = append(tags, name)
			}
			c.JSON(200, tags)
		})
		config.POST("/tags", func(c *gin.Context) {
			var req struct{ Name string `json:"name"` }
			if err := c.ShouldBindJSON(&req); err == nil {
				getTagID(req.Name)
				c.JSON(200, gin.H{"message": "Tag created"})
			}
		})
		config.GET("/comms", func(c *gin.Context) {
			type Item struct{ Comm string `json:"comm"`; Tag string `json:"tag"` }
			var items []Item
			var key [16]byte
			var tagID uint32
			iter := objs.TrackedComms.Iterate()
			for iter.Next(&key, &tagID) {
				items = append(items, Item{string(bytes.TrimRight(key[:], "\x00")), getTagName(tagID)})
			}
			c.JSON(200, items)
		})
		config.POST("/comms", func(c *gin.Context) {
			var req struct{ Comm string `json:"comm"`; Tag string `json:"tag"` }
			if err := c.ShouldBindJSON(&req); err == nil {
				var key [16]byte
				copy(key[:], req.Comm)
				tagID := getTagID(req.Tag)
				objs.TrackedComms.Put(key, tagID)
				c.JSON(200, gin.H{"message": "Added"})
			}
		})
		config.DELETE("/comms/:comm", func(c *gin.Context) {
			var key [16]byte
			copy(key[:], c.Param("comm"))
			objs.TrackedComms.Delete(key)
			c.JSON(200, gin.H{"message": "Removed"})
		})
		config.GET("/paths", func(c *gin.Context) {
			type Item struct{ Path string `json:"path"`; Tag string `json:"tag"` }
			var items []Item
			var key [256]byte
			var tagID uint32
			iter := objs.TrackedPaths.Iterate()
			for iter.Next(&key, &tagID) {
				items = append(items, Item{string(bytes.TrimRight(key[:], "\x00")), getTagName(tagID)})
			}
			c.JSON(200, items)
		})
		config.POST("/paths", func(c *gin.Context) {
			var req struct{ Path string `json:"path"`; Tag string `json:"tag"` }
			if err := c.ShouldBindJSON(&req); err != nil {
				var key [256]byte
				copy(key[:], req.Path)
				tagID := getTagID(req.Tag)
				objs.TrackedPaths.Put(key, tagID)
				c.JSON(200, gin.H{"message": "Added"})
			}
		})
		config.DELETE("/paths/*path", func(c *gin.Context) {
			path := c.Param("path")
			if len(path) > 0 && path[0] == '/' { path = path[1:] }
			var key [256]byte
			copy(key[:], path)
			objs.TrackedPaths.Delete(key)
			c.JSON(200, gin.H{"message": "Removed"})
		})
		config.GET("/export", func(c *gin.Context) {
			cfg := ExportConfig{Comms: make(map[string]string), Paths: make(map[string]string)}
			tagsMu.RLock()
			for _, n := range tagMap { cfg.Tags = append(cfg.Tags, n) }
			tagsMu.RUnlock()
			var k16 [16]byte
			var k256 [256]byte
			var tid uint32
			i1 := objs.TrackedComms.Iterate()
			for i1.Next(&k16, &tid) { cfg.Comms[string(bytes.TrimRight(k16[:], "\x00"))] = getTagName(tid) }
			i2 := objs.TrackedPaths.Iterate()
			for i2.Next(&k256, &tid) { cfg.Paths[string(bytes.TrimRight(k256[:], "\x00"))] = getTagName(tid) }
			c.JSON(200, cfg)
		})
		config.POST("/import", func(c *gin.Context) {
			var cfg ExportConfig
			if err := c.ShouldBindJSON(&cfg); err == nil {
				for _, t := range cfg.Tags { getTagID(t) }
				for comm, tag := range cfg.Comms {
					var k [16]byte
					copy(k[:], comm)
					tid := getTagID(tag)
					objs.TrackedComms.Put(k, tid)
				}
				for p, tag := range cfg.Paths {
					var k [256]byte
					copy(k[:], p)
					tid := getTagID(tag)
					objs.TrackedPaths.Put(k, tid)
				}
				c.JSON(200, gin.H{"message": "Imported"})
			}
		})
	}

	r.StaticFile("/", "../frontend/dist/index.html")
	r.Static("/assets", "../frontend/dist/assets")
	r.NoRoute(func(c *gin.Context) { c.File("../frontend/dist/index.html") })

	commonCLIs := map[string]string{
		"git": "Git", "npm": "Package Manager", "bun": "Package Manager",
		"pnpm": "Package Manager", "yarn": "Package Manager", "node": "Runtime",
		"python": "Runtime", "python3": "Runtime", "go": "Build Tool",
		"cargo": "Build Tool", "rustc": "Build Tool", "gcc": "Build Tool",
		"g++": "Build Tool", "clang": "Build Tool", "make": "Build Tool",
		"cmake": "Build Tool", "docker": "System Tool", "kubectl": "Network Tool",
	}
	for cli, tag := range commonCLIs {
		var key [16]byte
		copy(key[:], cli)
		_ = objs.TrackedComms.Put(key, getTagID(tag))
	}
	sensitivePaths := map[string]string{
		"/etc/shadow": "Security", "/etc/passwd": "Security",
		"/etc/sudoers": "Security", "/etc/hosts": "Security",
	}
	for p, tag := range sensitivePaths {
		var key [256]byte
		copy(key[:], p)
		_ = objs.TrackedPaths.Put(key, getTagID(tag))
	}

	startPort, maxTries, actualPort := 8080, 10, 8080
	var ln net.Listener
	for i := 0; i < maxTries; i++ {
		l, err := net.Listen("tcp", fmt.Sprintf(":%d", startPort+i))
		if err == nil {
			actualPort, ln = startPort+i, l
			break
		}
	}
	if ln != nil {
		ln.Close()
		_ = os.WriteFile(".port", []byte(fmt.Sprintf("%d", actualPort)), 0644)
		fmt.Printf("Server listening on :%d\n", actualPort)
		r.Run(fmt.Sprintf(":%d", actualPort))
	}
}
