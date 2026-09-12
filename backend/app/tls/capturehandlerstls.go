package tls

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/internal/binaryresolver"
	"agent-ebpf-filter/internal/wsfanout"
)

// TLSBroadcaster fans TLSPlaintextEvents out to /ws/tls-capture subscribers
// as JSON text frames. It is a thin policy layer over wsfanout.Hub: the hub
// owns the per-client queues and writer goroutines, while this type adds the
// runtime accept gate and the drop/failure counters exposed by Status().
type TLSBroadcaster struct {
	mu           sync.Mutex
	hub          *wsfanout.Hub
	upgrade      tlsBroadcastUpgradeFunc
	accepting    bool
	enabledCheck func() bool

	queueFullDropsTotal        atomic.Uint64
	writeFailuresTotal         atomic.Uint64
	writeDeadlineFailuresTotal atomic.Uint64
}

type TLSBroadcastStatus struct {
	ActiveClients              int    `json:"activeClients"`
	QueuedEvents               int    `json:"queuedEvents"`
	QueueCapacity              int    `json:"queueCapacity"`
	QueueFullDropsTotal        uint64 `json:"queueFullDropsTotal"`
	WriteFailuresTotal         uint64 `json:"writeFailuresTotal"`
	WriteDeadlineFailuresTotal uint64 `json:"writeDeadlineFailuresTotal"`
}

// tlsBroadcastClient is the connection surface the broadcaster writes to.
// *websocket.Conn satisfies it; tests use in-memory fakes.
type tlsBroadcastClient = wsfanout.Conn

type tlsBroadcastConnection interface {
	tlsBroadcastClient
	ReadMessage() (messageType int, payload []byte, err error)
}

type tlsBroadcastUpgradeFunc func(http.ResponseWriter, *http.Request, http.Header) (tlsBroadcastConnection, error)

const (
	tlsBroadcastQueueSize    = 64
	tlsBroadcastWriteTimeout = 2 * time.Second
)

func (b *TLSBroadcaster) SetEnabledCheck(enabled func() bool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.enabledCheck = enabled
	b.mu.Unlock()
}

func (b *TLSBroadcaster) SetAccepting(accepting bool) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.accepting = accepting
	b.mu.Unlock()
}

// tryAddClient registers conn unless the runtime gate is closed, in which
// case the connection is closed immediately and ok is false.
func (b *TLSBroadcaster) tryAddClient(conn tlsBroadcastClient) (*wsfanout.Client, bool) {
	b.mu.Lock()
	accepting := b.accepting && (b.enabledCheck == nil || b.enabledCheck())
	b.mu.Unlock()
	if !accepting {
		_ = conn.Close()
		return nil, false
	}
	return b.hub.Add(conn), true
}

func (b *TLSBroadcaster) addClient(conn tlsBroadcastClient) *wsfanout.Client {
	client, _ := b.tryAddClient(conn)
	return client
}

func (b *TLSBroadcaster) removeClient(client *wsfanout.Client) {
	b.hub.Remove(client)
}

func (b *TLSBroadcaster) recordDrop(reason wsfanout.DropReason) {
	var counter *atomic.Uint64
	var metric string
	switch reason {
	case wsfanout.DropQueueFull:
		counter, metric = &b.queueFullDropsTotal, "tls.broadcast.queue_full"
	case wsfanout.DropDeadlineFailure:
		counter, metric = &b.writeDeadlineFailuresTotal, "tls.broadcast.write_deadline_failure"
	case wsfanout.DropWriteFailure:
		counter, metric = &b.writeFailuresTotal, "tls.broadcast.write_failure"
	default:
		return
	}
	counter.Add(1)
	if metrics := deps.CollectorMetrics; metrics != nil {
		metrics.RecordAgentSightCounter(metric)
	}
}

func (b *TLSBroadcaster) Status() TLSBroadcastStatus {
	if b == nil {
		return TLSBroadcastStatus{QueueCapacity: tlsBroadcastQueueSize}
	}
	return TLSBroadcastStatus{
		ActiveClients:              b.hub.Len(),
		QueuedEvents:               b.hub.Queued(),
		QueueCapacity:              tlsBroadcastQueueSize,
		QueueFullDropsTotal:        b.queueFullDropsTotal.Load(),
		WriteFailuresTotal:         b.writeFailuresTotal.Load(),
		WriteDeadlineFailuresTotal: b.writeDeadlineFailuresTotal.Load(),
	}
}

// Close disconnects every subscriber and stops accepting new ones until
// SetAccepting(true) re-opens the gate (runtime TLS capture toggled back on).
func (b *TLSBroadcaster) Close() {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.accepting = false
	b.mu.Unlock()
	b.hub.DisconnectAll()
}

func (b *TLSBroadcaster) Serve(c *gin.Context) {
	upgrade := b.upgrade
	if upgrade == nil {
		upgrade = defaultTLSBroadcastUpgrade
	}
	conn, err := upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client, accepted := b.tryAddClient(conn)
	if !accepted {
		return
	}
	defer b.removeClient(client)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

// Broadcast encodes event once and shares the resulting text frame with
// every subscriber. Without subscribers nothing is encoded or allocated.
func (b *TLSBroadcaster) Broadcast(event TLSPlaintextEvent) {
	if b == nil || b.hub.Len() == 0 {
		return
	}
	b.publish(event)
}

// publish is split from Broadcast so that only this copy of the event is
// forced onto the heap: json encodes an addressable struct in place, whereas
// a by-value argument is first duplicated through reflect.New.
func (b *TLSBroadcaster) publish(event TLSPlaintextEvent) {
	data, err := json.Marshal(&event)
	if err != nil {
		if metrics := deps.CollectorMetrics; metrics != nil {
			metrics.RecordAgentSightCounter("tls.broadcast.marshal_failure")
		}
		return
	}
	b.hub.Broadcast(wsfanout.NewTextMessage(data))
}

type tlsCaptureRuntime interface {
	AttachDefaults() error
	AttachBuiltinExecutables(pid int) ([]TLSBuiltinExecutableAttachStatus, error)
	AttachLibrary(path, library string) error
	AttachExecutable(input string, pid int, libraryHint string) TLSExecutableAttachResult
	AttachGoUprobes(path string, pid int) error
	EnsureStarted() (*TLSProbeManager, error)
	Status() map[string]any
	AttachedPIDs() []AttachedPIDInfo
	ProbeHitCounters() map[string]uint64
	ReadLoopStatsSnapshot() ReadLoopStats
}

func NewTLSCaptureBroadcaster() *TLSBroadcaster {
	return newTLSCaptureBroadcasterWithUpgrader(defaultTLSBroadcastUpgrade)
}

func newTLSCaptureBroadcasterWithUpgrader(upgrade tlsBroadcastUpgradeFunc) *TLSBroadcaster {
	b := &TLSBroadcaster{upgrade: upgrade, accepting: true}
	b.hub = wsfanout.New(wsfanout.Options{
		QueueSize:    tlsBroadcastQueueSize,
		WriteTimeout: tlsBroadcastWriteTimeout,
		PingPeriod:   -1,
		OnDrop:       b.recordDrop,
	})
	return b
}

func defaultTLSBroadcastUpgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (tlsBroadcastConnection, error) {
	if deps.Upgrader == nil {
		return nil, errors.New("TLS capture websocket upgrader is not initialized")
	}
	return deps.Upgrader.Upgrade(w, r, responseHeader)
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
		resolved := binaryresolver.ResolveBinary(req.Path, "")
		attachPath := req.Path
		if resolved.Error == "" && resolved.RealPath != "" {
			attachPath = resolved.RealPath
		}
		if err := runtime.AttachGoUprobes(attachPath, req.PID); err != nil {
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
		result := runtime.AttachExecutable(req.Path, req.PID, req.Library)
		if result.Error != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": result.Error, "result": result})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "attached", "result": result})
	}
}
