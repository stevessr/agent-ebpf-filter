package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
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
	"github.com/shirou/gopsutil/v3/process"
	"google.golang.org/protobuf/proto"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	udsPath = "/tmp/agent-ebpf.sock"
)

type bpfEvent struct {
	PID, PPID, UID, Type, TagID uint32
	Comm                        [16]byte
	Path                        [256]byte
}

type WrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"` // ALLOW, BLOCK, REWRITE, ALERT
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
}

var (
	tagsMu      sync.RWMutex
	tagMap      = map[uint32]string{0: "Unknown", 1: "AI Agent", 2: "Git", 3: "Build Tool", 4: "Package Manager", 5: "Runtime", 6: "System Tool", 7: "Network Tool", 8: "Security"}
	tagNameToID = map[string]uint32{"AI Agent": 1, "Git": 2, "Build Tool": 3, "Package Manager": 4, "Runtime": 5, "System Tool": 6, "Network Tool": 7, "Security": 8}
	nextTagID   uint32 = 9

	rulesMu      sync.RWMutex
	wrapperRules = make(map[string]WrapperRule)
)

func getTagName(id uint32) string {
	tagsMu.RLock()
	defer tagsMu.RUnlock()
	if name, ok := tagMap[id]; ok { return name }
	return fmt.Sprintf("Tag-%d", id)
}

func getTagID(name string) uint32 {
	tagsMu.Lock()
	defer tagsMu.Unlock()
	if id, ok := tagNameToID[name]; ok { return id }
	id := nextTagID
	tagMap[id] = name
	tagNameToID[name] = id
	nextTagID++
	return id
}

func getGPUPidMap() map[int32]struct{ mem, gpu uint32 } {
	gpuMap := make(map[int32]struct{ mem, gpu uint32 })
	cmd := exec.Command("nvidia-smi", "--query-compute-apps=pid,used_memory,gpu_index", "--format=csv,noheader,nounits")
	output, _ := cmd.Output()
	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		if len(line) == 0 { continue }
		parts := bytes.Split(line, []byte(", "))
		if len(parts) >= 2 {
			var pid int32
			var mem, gpu uint32
			fmt.Sscanf(string(parts[0]), "%d", &pid)
			fmt.Sscanf(string(parts[1]), "%d", &mem)
			if len(parts) > 2 { fmt.Sscanf(string(parts[2]), "%d", &gpu) }
			gpuMap[pid] = struct{ mem, gpu uint32 }{mem, gpu}
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
	if err != nil {
		log.Printf("UDS Listen error: %v", err)
		return
	}
	_ = os.Chmod(udsPath, 0666)
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil { continue }
		go handleUDSConn(conn, broadcast)
	}
}

func handleUDSConn(conn net.Conn, broadcast chan *pb.Event) {
	defer conn.Close()
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil { return }
		req := &pb.WrapperRequest{}
		if err := proto.Unmarshal(buf[:n], req); err != nil { continue }

		resp := &pb.WrapperResponse{Action: pb.WrapperResponse_ALLOW}
		
		rulesMu.RLock()
		rule, ok := wrapperRules[req.Comm]
		rulesMu.RUnlock()

		if ok {
			switch rule.Action {
			case "BLOCK":
				resp.Action = pb.WrapperResponse_BLOCK
				resp.Message = "Command blocked by security policy"
			case "ALERT":
				resp.Action = pb.WrapperResponse_ALERT
				resp.Message = "Alert: sensitive command execution"
			case "REWRITE":
				resp.Action = pb.WrapperResponse_REWRITE
				resp.RewrittenArgs = rule.RewrittenCmd
			}
		}

		// Report to eBPF stream as a special "WRAPPER" event
		evt := &pb.Event{
			Pid: req.Pid, Comm: req.Comm, Type: "wrapper_intercept",
			Tag: "Wrapper", Path: strings.Join(append([]string{req.Comm}, req.Args...), " "),
		}
		broadcast <- evt

		out, _ := proto.Marshal(resp)
		_, _ = conn.Write(out)
	}
}

func main() {
	if os.Geteuid() != 0 {
		executable, _ := os.Executable()
		isDesktop := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
		sudoCmd := "sudo"
		if isDesktop { if _, err := exec.LookPath("pkexec"); err == nil { sudoCmd = "pkexec" } }
		fmt.Printf("Root privileges required. Re-running with %s...\n", sudoCmd)
		cmd := exec.Command(sudoCmd, append([]string{executable}, os.Args[1:]...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		_ = cmd.Run()
		os.Exit(0)
	}

	currentPid := int32(os.Getpid())
	procs, _ := process.Processes()
	for _, p := range procs {
		if p.Pid == currentPid { continue }
		name, _ := p.Name()
		if name == "agent-ebpf-filter" || name == "main" { _ = p.Kill() }
	}

	_ = rlimit.RemoveMemlock()
	var objs bpf.AgentTrackerObjects
	_ = bpf.LoadAgentTrackerObjects(&objs, nil)
	defer objs.Close()

	attach := func(c, n string, p *ebpf.Program) link.Link {
		l, _ := link.Tracepoint(c, n, p, nil)
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
	for _, l := range links { defer l.Close() }

	rd, _ := ringbuf.NewReader(objs.Events)
	defer rd.Close()

	broadcast := make(chan *pb.Event, 100)
	go func() {
		var event bpfEvent
		types := map[uint32]string{0: "execve", 1: "openat", 2: "network_connect", 3: "mkdir", 4: "unlink", 5: "ioctl", 6: "network_bind"}
		for {
			record, err := rd.Read()
			if err != nil { return }
			_ = binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event)
			comm, path := string(bytes.TrimRight(event.Comm[:], "\x00")), string(bytes.TrimRight(event.Path[:], "\x00"))
			evtType := types[event.Type]
			if evtType == "" { evtType = "unknown" }
			broadcast <- &pb.Event{Pid: event.PID, Ppid: event.PPID, Uid: event.UID, Type: evtType, Tag: getTagName(event.TagID), Comm: comm, Path: path}
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
				if err := c.WriteMessage(websocket.BinaryMessage, data); err != nil {
					c.Close()
					delete(clients, c)
				}
			}
			clientsMu.Unlock()
		}
	}()

	r.GET("/ws", func(c *gin.Context) {
		conn, _ := upgrader.Upgrade(c.Writer, c.Request, nil)
		clientsMu.Lock()
		clients[conn] = true
		clientsMu.Unlock()
	})

	r.GET("/ws/system", func(c *gin.Context) {
		conn, _ := upgrader.Upgrade(c.Writer, c.Request, nil)
		defer conn.Close()
		ticker := time.NewTicker(2 * time.Second)
		for range ticker.C {
			gpuMap, gpus := getGPUPidMap(), getGlobalGPUStatus()
			ps, _ := process.Processes()
			stats := &pb.SystemStats{Gpus: gpus}
			for _, p := range ps {
				n, _ := p.Name(); pp, _ := p.Ppid(); cp, _ := p.CPUPercent(); mp, _ := p.MemoryPercent(); u, _ := p.Username()
				gm, gi := uint32(0), uint32(0)
				if info, ok := gpuMap[p.Pid]; ok { gm, gi = info.mem, info.gpu }
				stats.Processes = append(stats.Processes, &pb.Process{Pid: p.Pid, Ppid: pp, Name: n, Cpu: cp, Mem: mp, User: u, GpuMem: gm, GpuId: gi})
			}
			data, _ := proto.Marshal(stats)
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil { return }
		}
	})

	config := r.Group("/config")
	{
		config.GET("/tags", func(c *gin.Context) {
			tagsMu.RLock(); defer tagsMu.RUnlock()
			t := []string{}; for _, n := range tagMap { t = append(t, n) }
			c.JSON(200, t)
		})
		config.POST("/tags", func(c *gin.Context) {
			var r struct{ Name string `json:"name"` }; _ = c.ShouldBindJSON(&r)
			getTagID(r.Name); c.JSON(200, gin.H{"status": "ok"})
		})
		config.GET("/comms", func(c *gin.Context) {
			items := []gin.H{}
			iter := objs.TrackedComms.Iterate()
			var k [16]byte; var tid uint32
			for iter.Next(&k, &tid) { items = append(items, gin.H{"comm": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)}) }
			c.JSON(200, items)
		})
		config.POST("/comms", func(c *gin.Context) {
			var r struct{ Comm, Tag string `json:"comm" json:"tag"` }; _ = c.ShouldBindJSON(&r)
			var k [16]byte; copy(k[:], r.Comm); _ = objs.TrackedComms.Put(k, getTagID(r.Tag)); c.JSON(200, gin.H{"status": "ok"})
		})
		config.DELETE("/comms/:comm", func(c *gin.Context) {
			var k [16]byte; copy(k[:], c.Param("comm")); _ = objs.TrackedComms.Delete(k); c.JSON(200, gin.H{"status": "ok"})
		})
		config.GET("/paths", func(c *gin.Context) {
			items := []gin.H{}
			iter := objs.TrackedPaths.Iterate()
			var k [256]byte; var tid uint32
			for iter.Next(&k, &tid) { items = append(items, gin.H{"path": string(bytes.TrimRight(k[:], "\x00")), "tag": getTagName(tid)}) }
			c.JSON(200, items)
		})
		config.POST("/paths", func(c *gin.Context) {
			var r struct{ Path, Tag string `json:"path" json:"tag"` }; _ = c.ShouldBindJSON(&r)
			var k [256]byte; copy(k[:], r.Path); _ = objs.TrackedPaths.Put(k, getTagID(r.Tag)); c.JSON(200, gin.H{"status": "ok"})
		})
		config.DELETE("/paths/*path", func(c *gin.Context) {
			p := c.Param("path"); if len(p) > 0 && p[0] == '/' { p = p[1:] }
			var k [256]byte; copy(k[:], p); _ = objs.TrackedPaths.Delete(k); c.JSON(200, gin.H{"status": "ok"})
		})
		config.GET("/rules", func(c *gin.Context) {
			rulesMu.RLock(); defer rulesMu.RUnlock()
			c.JSON(200, wrapperRules)
		})
		config.POST("/rules", func(c *gin.Context) {
			var r WrapperRule; _ = c.ShouldBindJSON(&r)
			rulesMu.Lock(); wrapperRules[r.Comm] = r; rulesMu.Unlock()
			c.JSON(200, gin.H{"status": "ok"})
		})
		config.DELETE("/rules/:comm", func(c *gin.Context) {
			rulesMu.Lock(); delete(wrapperRules, c.Param("comm")); rulesMu.Unlock()
			c.JSON(200, gin.H{"status": "ok"})
		})
	}

	r.GET("/system/ls", func(c *gin.Context) {
		p := c.DefaultQuery("path", "/")
		e, _ := os.ReadDir(p)
		l := []gin.H{}
		for _, v := range e {
			fp := p; if fp == "/" { fp = "/" + v.Name() } else { fp = fp + "/" + v.Name() }
			l = append(l, gin.H{"name": v.Name(), "isDir": v.IsDir(), "path": fp})
		}
		c.JSON(200, l)
	})

	r.StaticFile("/", "../frontend/dist/index.html")
	r.Static("/assets", "../frontend/dist/assets")
	r.NoRoute(func(c *gin.Context) { c.File("../frontend/dist/index.html") })

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
