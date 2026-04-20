package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"agent-ebpf-filter/ebpf"

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
		fmt.Println("Root privileges required for eBPF operations. Re-running with sudo...")
		cmd := exec.Command("sudo", os.Args...)
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
	var objs ebpf.AgentTrackerObjects
	if err := ebpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
		log.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	// Attach tracepoint to sys_enter_execve
	tpExecve, err := link.Tracepoint("syscalls", "sys_enter_execve", objs.TracepointSyscallsSysEnterExecve, nil)
	if err != nil {
		log.Fatalf("opening tracepoint sys_enter_execve: %s", err)
	}
	defer tpExecve.Close()

	// Attach tracepoint to sys_enter_openat
	tpOpenat, err := link.Tracepoint("syscalls", "sys_enter_openat", objs.TracepointSyscallsSysEnterOpenat, nil)
	if err != nil {
		log.Fatalf("opening tracepoint sys_enter_openat: %s", err)
	}
	defer tpOpenat.Close()

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

			// Parse the ringbuf event entry into a bpfEvent structure.
			if err := binary.Read(bytes.NewBuffer(record.RawSample), binary.LittleEndian, &event); err != nil {
				log.Printf("parsing ringbuf event: %s", err)
				continue
			}

			// Convert C strings to Go strings
			comm := string(bytes.TrimRight(event.Comm[:], "\x00"))
			path := string(bytes.TrimRight(event.Path[:], "\x00"))
			
			evtType := "execve"
			if event.Type == 1 {
				evtType = "openat"
			}

			wsEvent := WsEvent{
				PID:  event.PID,
				Type: evtType,
				Comm: comm,
				Path: path,
			}

			// Non-blocking send to broadcast channel
			select {
			case broadcast <- wsEvent:
			default:
				// Drop event if channel is full
			}
		}
	}()

	r := gin.Default()

	// Serve static files from frontend/dist
	r.StaticFile("/", "../frontend/dist/index.html")
	r.Static("/assets", "../frontend/dist/assets")
	r.NoRoute(func(c *gin.Context) {
		c.File("../frontend/dist/index.html")
	})

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

		// Register PID in eBPF map
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

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("Server listening on :%s\n", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}
