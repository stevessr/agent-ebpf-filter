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

	bpf "agent-ebpf-filter/ebpf"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/ringbuf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // allow any origin for development
	},
}

// Event matching the C struct
type bpfEvent struct {
	PID   uint32
	Type  uint32
	TagID uint32
	Comm  [16]byte
	Path  [256]byte
}

type WsEvent struct {
	PID  uint32 `json:"pid"`
	Type string `json:"type"`
	Tag  string `json:"tag"`
	Comm string `json:"comm"`
	Path string `json:"path"`
}

var (
	tagsMu sync.RWMutex
	tagMap = map[uint32]string{
		0: "Unknown",
		1: "AI Agent",
		2: "Git",
		3: "Build Tool",
		4: "Package Manager",
		5: "Runtime",
		6: "System Tool",
		7: "Network Tool",
	}
	tagNameToID = map[string]uint32{
		"AI Agent":        1,
		"Git":             2,
		"Build Tool":      3,
		"Package Manager": 4,
		"Runtime":         5,
		"System Tool":     6,
		"Network Tool":    7,
	}
	nextTagID uint32 = 8
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

	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal("Failed to remove memlock:", err)
	}

	// Load pre-compiled programs and maps into the kernel.
	var objs bpf.AgentTrackerObjects
	if err := bpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
		log.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	// Attach tracepoints
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

	// Open a ringbuf reader from userspace RINGBUF map
	rd, err := ringbuf.NewReader(objs.Events)
	if err != nil {
		log.Fatalf("opening ringbuf reader: %s", err)
	}
	defer rd.Close()

	// Channel to broadcast events to websocket clients
	broadcast := make(chan WsEvent, 100)

	// Background task to read ringbuf events
	go func() {
		var event bpfEvent
		types := map[uint32]string{
			0: "execve",
			1: "openat",
			2: "network_connect",
			3: "mkdir",
			4: "unlink",
			5: "ioctl",
			6: "network_bind",
		}
		for {
			record, err := rd.Read()
			if err != nil {
				if err == ringbuf.ErrClosed {
					log.Println("Ringbuf closed")
					return
				}
				log.Printf("reading from ringbuf: %s", err)
				continue
			}

			if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
				log.Printf("parsing ringbuf event: %s", err)
				continue
			}

			comm := string(bytes.TrimRight(event.Comm[:], "\x00"))
			path := string(bytes.TrimRight(event.Path[:], "\x00"))
			
			evtType, ok := types[event.Type]
			if !ok {
				evtType = "unknown"
			}

			wsEvent := WsEvent{
				PID:  event.PID,
				Type: evtType,
				Tag:  getTagName(event.TagID),
				Comm: comm,
				Path: path,
			}

			select {
			case broadcast <- wsEvent:
			default:
			}
		}
	}()

	r := gin.Default()

	// Manage connected websocket clients
	clients := make(map[*websocket.Conn]bool)

	// Broadcast events to all clients
	go func() {
		for event := range broadcast {
			for client := range clients {
				err := client.WriteJSON(event)
				if err != nil {
					log.Printf("websocket write error: %v", err)
					client.Close()
					delete(clients, client)
				}
			}
		}
	}()

	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			log.Println(err)
			return
		}
		clients[conn] = true
	})

	type RegisterRequest struct {
		PID uint32 `json:"pid"`
		Tag string `json:"tag"`
	}

	r.POST("/register", func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		
		tagID := uint32(1) // Default "AI Agent"
		if req.Tag != "" {
			tagID = getTagID(req.Tag)
		}

		if err := objs.AgentPids.Put(&req.PID, &tagID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update eBPF map: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Registered PID %d with tag %s", req.PID, getTagName(tagID))})
	})
	
	r.POST("/unregister", func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if err := objs.AgentPids.Delete(&req.PID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update eBPF map: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Unregistered PID %d", req.PID)})
	})

	// --- Config Endpoints for Tracked Comms ---
	type CommRequest struct {
		Comm string `json:"comm"`
		Tag  string `json:"tag"`
	}

	config := r.Group("/config")
	{
		config.GET("/comms", func(c *gin.Context) {
			type TrackedItem struct {
				Comm string `json:"comm"`
				Tag  string `json:"tag"`
			}
			var items []TrackedItem
			var key [16]byte
			var tagID uint32
			iter := objs.TrackedComms.Iterate()
			for iter.Next(&key, &tagID) {
				items = append(items, TrackedItem{
					Comm: string(bytes.TrimRight(key[:], "\x00")),
					Tag:  getTagName(tagID),
				})
			}
			c.JSON(http.StatusOK, items)
		})

		config.POST("/comms", func(c *gin.Context) {
			var req CommRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			var key [16]byte
			copy(key[:], req.Comm)
			
			tagID := uint32(6) // Default "System Tool"
			if req.Tag != "" {
				tagID = getTagID(req.Tag)
			}

			if err := objs.TrackedComms.Put(key, tagID); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Tracked comm added"})
		})

		config.DELETE("/comms/:comm", func(c *gin.Context) {
			comm := c.Param("comm")
			var key [16]byte
			copy(key[:], comm)
			if err := objs.TrackedComms.Delete(key); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			c.JSON(http.StatusOK, gin.H{"message": "Tracked comm removed"})
		})
	}

	// Serve static files from frontend/dist (defined AFTER API routes)
	r.StaticFile("/", "../frontend/dist/index.html")
	r.Static("/assets", "../frontend/dist/assets")
	r.NoRoute(func(c *gin.Context) {
		c.File("../frontend/dist/index.html")
	})

	// Pre-load common coding CLIs
	commonCLIs := map[string]string{
		"git":     "Git",
		"npm":     "Package Manager",
		"bun":     "Package Manager",
		"pnpm":    "Package Manager",
		"yarn":    "Package Manager",
		"node":    "Runtime",
		"python":  "Runtime",
		"python3": "Runtime",
		"go":      "Build Tool",
		"cargo":   "Build Tool",
		"rustc":   "Build Tool",
		"gcc":     "Build Tool",
		"g++":     "Build Tool",
		"clang":   "Build Tool",
		"make":    "Build Tool",
		"cmake":   "Build Tool",
		"docker":  "System Tool",
		"kubectl": "Network Tool",
	}
	for cli, tag := range commonCLIs {
		var key [16]byte
		copy(key[:], cli)
		tagID := getTagID(tag)
		_ = objs.TrackedComms.Put(key, tagID)
	}

	startPort := 8080
	maxTries := 10
	var ln net.Listener
	var errBind error
	actualPort := startPort

	for i := 0; i < maxTries; i++ {
		addr := fmt.Sprintf(":%d", startPort+i)
		ln, errBind = net.Listen("tcp", addr)
		if errBind == nil {
			actualPort = startPort + i
			break
		}
		fmt.Printf("Port %d is in use, trying next...\n", startPort+i)
	}

	if ln == nil {
		log.Fatalf("Could not find an available port after %d tries: %v", maxTries, errBind)
	}
	ln.Close() // Close so Gin can bind it, or pass the listener to Gin

	// Write the port to a file so the frontend/vite can discover it
	_ = os.WriteFile(".port", []byte(fmt.Sprintf("%d", actualPort)), 0644)

	fmt.Printf("Server listening on :%d\n", actualPort)
	if err := r.Run(fmt.Sprintf(":%d", actualPort)); err != nil {
		log.Fatal(err)
	}
}
