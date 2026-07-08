package tls

import (
	"agent-ebpf-filter/internal/binaryresolver"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ---- moved from backend/zz_merged_backend.go section capturehandlerstls.go ----

type TLSBroadcaster struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]*sync.Mutex
}

func (b *TLSBroadcaster) Serve(c *gin.Context) {
	conn, err := deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
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

func (b *TLSBroadcaster) Broadcast(event TLSPlaintextEvent) {
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

type tlsCaptureRuntime interface {
	AttachDefaults() error
	AttachBuiltinExecutables(pid int) ([]TLSBuiltinExecutableAttachStatus, error)
	AttachLibrary(path, library string) error
	EnsureStarted() (*TLSProbeManager, error)
	Status() map[string]any
	AttachedPIDs() []AttachedPIDInfo
	ProbeHitCounters() map[string]uint64
	ReadLoopStatsSnapshot() ReadLoopStats
}

func NewTLSCaptureBroadcaster() *TLSBroadcaster {
	return &TLSBroadcaster{clients: make(map[*websocket.Conn]*sync.Mutex)}
}

func RegisterTLSCaptureRoutes(router gin.IRouter, runtime tlsCaptureRuntime, store *TLSCaptureStore, rules *TLSCaptureRuleStore) {
	router.GET("/tls-capture/recent", handleTLSCaptureRecent(store))
	router.GET("/tls-capture/libraries", handleTLSCaptureLibraries(store))
	router.GET("/tls-capture/status", handleTLSCaptureStatus(runtime, store))
	router.POST("/tls-capture/start", handleTLSCaptureStart(runtime))
	router.POST("/tls-capture/attach-defaults", handleTLSCaptureAttachDefaults(runtime))
	router.POST("/tls-capture/attach-builtins", handleTLSCaptureAttachBuiltins(runtime))
	router.GET("/tls-capture/rules", handleTLSCaptureRulesGet(rules))
	router.PUT("/tls-capture/rules", handleTLSCaptureRulesPut(rules))
	router.POST("/tls-capture/library", handleTLSCaptureLibrary(runtime))
	router.POST("/tls-capture/go-binary", handleTLSCaptureGoBinary(runtime))
	router.POST("/tls-capture/executable", handleTLSCaptureExecutable(runtime))
	router.GET("/tls-capture/attached-pids", handleTLSCaptureAttachedPIDs(runtime))
}

func handleTLSCaptureAttachedPIDs(runtime tlsCaptureRuntime) gin.HandlerFunc {
	return func(c *gin.Context) {
		pids := runtime.AttachedPIDs()
		c.JSON(http.StatusOK, pids)
	}
}

func handleTLSCaptureRecent(store *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := parseEventLimitQuery(c.Query("limit"), 100)
		events := store.Recent(limit)
		if filter := c.Query("filter"); filter != "" {
			events = agentSightHTTPFilter.Filter(events, filter)
		}
		c.JSON(http.StatusOK, gin.H{"events": events})
	}
}

func filterTLSCaptureEvents(events []TLSPlaintextEvent, filter string) []TLSPlaintextEvent {
	terms := strings.Fields(filter)
	if len(terms) == 0 {
		return events
	}
	out := make([]TLSPlaintextEvent, 0, len(events))
	for _, event := range events {
		if tlsCaptureEventMatchesFilter(event, terms) {
			out = append(out, event)
		}
	}
	return out
}

func tlsCaptureEventMatchesFilter(event TLSPlaintextEvent, terms []string) bool {
	for _, term := range terms {
		key, value, ok := strings.Cut(term, ":")
		if !ok {
			value = term
			key = "text"
		}
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		var haystack string
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "host":
			haystack = event.Host
		case "method":
			haystack = event.Method
		case "status":
			haystack = strconv.Itoa(event.StatusCode)
		case "type":
			haystack = event.Type
		case "comm", "process":
			haystack = event.Comm
		case "vendor":
			haystack = event.Vendor
		case "redaction":
			haystack = event.RedactionState
		default:
			haystack = strings.Join([]string{event.Type, event.Comm, event.Host, event.Method, event.URL, event.ContentType, event.Vendor, event.RedactionState}, " ")
		}
		if !strings.Contains(strings.ToLower(haystack), value) {
			return false
		}
	}
	return true
}

func handleTLSCaptureLibraries(store *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"libraries": store.LibraryStatuses()})
	}
}

func handleTLSCaptureStatus(runtime tlsCaptureRuntime, store *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := map[string]any{"enabled": false, "available": false, "error": "TLS capture manager is not started"}
		if runtime != nil {
			status = runtime.Status()
			status["probe_hits"] = runtime.ProbeHitCounters()
			status["readloop"] = runtime.ReadLoopStatsSnapshot()
		}
		status["libraries"] = store.LibraryStatuses()
		c.JSON(http.StatusOK, status)
	}
}

func handleTLSCaptureStart(runtime tlsCaptureRuntime) gin.HandlerFunc {
	return func(c *gin.Context) {
		if runtime == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture runtime is unavailable"})
			return
		}
		if _, err := runtime.EnsureStarted(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": runtime.Status()})
			return
		}
		c.JSON(http.StatusOK, runtime.Status())
	}
}

func handleTLSCaptureAttachDefaults(runtime tlsCaptureRuntime) gin.HandlerFunc {
	return func(c *gin.Context) {
		if runtime == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture runtime is unavailable"})
			return
		}
		if err := runtime.AttachDefaults(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": runtime.Status()})
			return
		}
		c.JSON(http.StatusOK, runtime.Status())
	}
}

func handleTLSCaptureAttachBuiltins(runtime tlsCaptureRuntime) gin.HandlerFunc {
	type request struct {
		PID int `json:"pid"`
	}
	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if runtime == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture runtime is unavailable"})
			return
		}
		statuses, err := runtime.AttachBuiltinExecutables(req.PID)
		payload := gin.H{"targets": []map[string]string{}, "statuses": statuses, "status": runtime.Status()}
		if err != nil {
			payload["error"] = err.Error()
			c.JSON(http.StatusBadRequest, payload)
			return
		}
		c.JSON(http.StatusOK, payload)
	}
}

func handleTLSCaptureRulesGet(rules *TLSCaptureRuleStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		if rules == nil {
			rules = NewTLSCaptureRuleStore()
		}
		c.JSON(http.StatusOK, gin.H{"rules": rules.List()})
	}
}

func handleTLSCaptureRulesPut(rules *TLSCaptureRuleStore) gin.HandlerFunc {
	type request struct {
		Rules []TLSCaptureRule `json:"rules"`
	}
	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if rules == nil {
			rules = NewTLSCaptureRuleStore()
		}
		c.JSON(http.StatusOK, gin.H{"rules": rules.Replace(req.Rules)})
	}
}

func handleTLSCaptureLibrary(runtime tlsCaptureRuntime) gin.HandlerFunc {
	type request struct {
		Path    string `json:"path"`
		Library string `json:"library"`
	}
	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
			return
		}
		if runtime == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture runtime is unavailable"})
			return
		}
		if err := runtime.AttachLibrary(req.Path, req.Library); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "status": runtime.Status()})
			return
		}
		c.JSON(http.StatusOK, runtime.Status())
	}
}

func handleTLSCaptureGoBinary(runtime tlsCaptureRuntime) gin.HandlerFunc {
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
		if runtime == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture runtime is unavailable"})
			return
		}
		manager, err := runtime.EnsureStarted()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": runtime.Status()})
			return
		}
		resolved := binaryresolver.ResolveBinary(req.Path, "")
		attachPath := req.Path
		if resolved.Error == "" && resolved.RealPath != "" {
			attachPath = resolved.RealPath
		}
		if err := manager.AttachGoUprobes(attachPath, req.PID); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "resolved": resolved})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "attached", "resolved": resolved})
	}
}

func handleTLSCaptureExecutable(runtime tlsCaptureRuntime) gin.HandlerFunc {
	type request struct {
		Path    string `json:"path"`
		PID     int    `json:"pid"`
		Library string `json:"library"`
	}
	return func(c *gin.Context) {
		var req request
		if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Path) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path is required"})
			return
		}
		if runtime == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "TLS capture runtime is unavailable"})
			return
		}
		manager, err := runtime.EnsureStarted()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "status": runtime.Status()})
			return
		}

		result := manager.AttachExecutable(req.Path, req.PID, req.Library)
		if result.Error != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": result.Error, "result": result})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "attached", "result": result})
	}
}
