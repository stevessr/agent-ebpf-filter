package main

import (
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type tlsCaptureBroadcaster struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]*sync.Mutex
}

func (b *tlsCaptureBroadcaster) Serve(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	writeMu := &sync.Mutex{}
	b.mu.Lock()
	b.clients[conn] = writeMu
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.clients, conn)
		b.mu.Unlock()
		_ = conn.Close()
	}()

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (b *tlsCaptureBroadcaster) Broadcast(event TLSPlaintextEvent) {
	type client struct {
		conn    *websocket.Conn
		writeMu *sync.Mutex
	}

	b.mu.Lock()
	clients := make([]client, 0, len(b.clients))
	for conn, writeMu := range b.clients {
		clients = append(clients, client{conn: conn, writeMu: writeMu})
	}
	b.mu.Unlock()

	for _, client := range clients {
		client.writeMu.Lock()
		err := client.conn.WriteJSON(event)
		client.writeMu.Unlock()
		if err != nil {
			_ = client.conn.Close()
			b.mu.Lock()
			delete(b.clients, client.conn)
			b.mu.Unlock()
		}
	}
}

type tlsGoBinaryRegistrar interface {
	AttachGoUprobes(binPath string, pid int) error
}

func newTLSCaptureBroadcaster() *tlsCaptureBroadcaster {
	return &tlsCaptureBroadcaster{clients: make(map[*websocket.Conn]*sync.Mutex)}
}

func registerTLSCaptureRoutes(router gin.IRouter, manager tlsGoBinaryRegistrar, store *TLSCaptureStore) {
	router.GET("/tls-capture/recent", handleTLSCaptureRecent(store))
	router.GET("/tls-capture/libraries", handleTLSCaptureLibraries(store))
	router.POST("/tls-capture/go-binary", handleTLSCaptureGoBinary(manager))
}

func handleTLSCaptureRecent(store *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 100
		if raw := c.Query("limit"); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 1000 {
				limit = parsed
			}
		}
		c.JSON(http.StatusOK, gin.H{"events": store.Recent(limit)})
	}
}

func handleTLSCaptureLibraries(store *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"libraries": store.LibraryStatuses()})
	}
}

func handleTLSCaptureGoBinary(manager tlsGoBinaryRegistrar) gin.HandlerFunc {
	type request struct {
		Path string `json:"path"`
		PID  int    `json:"pid"`
	}
	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil || req.Path == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
			return
		}
		if manager == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture manager is not available"})
			return
		}
		if err := manager.AttachGoUprobes(req.Path, req.PID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "attached"})
	}
}
