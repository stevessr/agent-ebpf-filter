package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

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
	PID  uint32
	Type uint32
	Comm [16]byte
	Path [256]byte
}

type WsEvent struct {
	PID  uint32 `json:"pid"`
	Type string `json:"type"`
	Comm string `json:"comm"`
	Path string `json:"path"`
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
	}

	r.POST("/register", func(c *gin.Context) {
		var req RegisterRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		val := uint8(1)
		if err := objs.AgentPids.Put(&req.PID, &val); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to update eBPF map: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("Registered PID %d", req.PID)})
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
	}

	config := r.Group("/config")
	{
		config.GET("/comms", func(c *gin.Context) {
			var comms []string
			var key [16]byte
			var val uint8
			iter := objs.TrackedComms.Iterate()
			for iter.Next(&key, &val) {
				comms = append(comms, string(bytes.TrimRight(key[:], "\x00")))
			}
			c.JSON(http.StatusOK, comms)
		})

		config.POST("/comms", func(c *gin.Context) {
			var req CommRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			var key [16]byte
			copy(key[:], req.Comm)
			val := uint8(1)
			if err := objs.TrackedComms.Put(key, val); err != nil {
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
	commonCLIs := []string{
		"git", "npm", "bun", "pnpm", "yarn", "node", "python", "python3", "go", "cargo", 
		"rustc", "gcc", "g++", "clang", "make", "cmake", "docker", "kubectl",
	}
	for _, cli := range commonCLIs {
		var key [16]byte
		copy(key[:], cli)
		val := uint8(1)
		_ = objs.TrackedComms.Put(key, val)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server listening on :%s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
