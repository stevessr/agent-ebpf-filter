package tls

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/internal/binaryresolver"
)

// ---- moved from backend/zz_merged_backend.go section capturehandlerstls.go ----

type TLSBroadcaster struct {
	mu                         sync.Mutex
	nextClientID               uint64
	clients                    map[uint64]*tlsBroadcastClientState
	upgrade                    tlsBroadcastUpgradeFunc
	queueFullDropsTotal        uint64
	writeFailuresTotal         uint64
	writeDeadlineFailuresTotal uint64
	accepting                  bool
	enabledCheck               func() bool
}

type TLSBroadcastStatus struct {
	ActiveClients              int    `json:"activeClients"`
	QueuedEvents               int    `json:"queuedEvents"`
	QueueCapacity              int    `json:"queueCapacity"`
	QueueFullDropsTotal        uint64 `json:"queueFullDropsTotal"`
	WriteFailuresTotal         uint64 `json:"writeFailuresTotal"`
	WriteDeadlineFailuresTotal uint64 `json:"writeDeadlineFailuresTotal"`
}

type tlsBroadcastClient interface {
	WriteJSON(v any) error
	Close() error
}

type tlsBroadcastDeadlineClient interface {
	SetWriteDeadline(deadline time.Time) error
}

type tlsBroadcastConnection interface {
	tlsBroadcastClient
	ReadMessage() (messageType int, payload []byte, err error)
}

type tlsBroadcastUpgradeFunc func(http.ResponseWriter, *http.Request, http.Header) (tlsBroadcastConnection, error)

const (
	tlsBroadcastQueueSize    = 64
	tlsBroadcastWriteTimeout = 2 * time.Second
)

type tlsBroadcastClientState struct {
	conn      tlsBroadcastClient
	mu        sync.Mutex
	queue     chan TLSPlaintextEvent
	done      chan struct{}
	closeOnce sync.Once
	dead      bool
}

type tlsBroadcastEnqueueResult uint8

const (
	tlsBroadcastEnqueueAccepted tlsBroadcastEnqueueResult = iota
	tlsBroadcastEnqueueDead
	tlsBroadcastEnqueueFull
)

func newTLSBroadcastClientState(conn tlsBroadcastClient) *tlsBroadcastClientState {
	return &tlsBroadcastClientState{
		conn:  conn,
		queue: make(chan TLSPlaintextEvent, tlsBroadcastQueueSize),
		done:  make(chan struct{}),
	}
}

func (state *tlsBroadcastClientState) enqueue(event TLSPlaintextEvent) tlsBroadcastEnqueueResult {
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.dead {
		return tlsBroadcastEnqueueDead
	}
	select {
	case state.queue <- event:
		return tlsBroadcastEnqueueAccepted
	default:
		return tlsBroadcastEnqueueFull
	}
}

func (state *tlsBroadcastClientState) isDead() bool {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.dead
}

func (state *tlsBroadcastClientState) close() {
	state.closeOnce.Do(func() {
		state.mu.Lock()
		state.dead = true
		close(state.done)
		state.mu.Unlock()
		_ = state.conn.Close()
	})
}

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

func (b *TLSBroadcaster) tryAddClient(conn tlsBroadcastClient) (uint64, *tlsBroadcastClientState, bool) {
	state := newTLSBroadcastClientState(conn)
	b.mu.Lock()
	if !b.accepting || (b.enabledCheck != nil && !b.enabledCheck()) {
		b.mu.Unlock()
		state.close()
		return 0, state, false
	}
	if b.clients == nil {
		b.clients = make(map[uint64]*tlsBroadcastClientState)
	}
	b.nextClientID++
	id := b.nextClientID
	b.clients[id] = state
	b.mu.Unlock()
	go b.runClient(id, state)
	return id, state, true
}

func (b *TLSBroadcaster) addClient(conn tlsBroadcastClient) (uint64, *tlsBroadcastClientState) {
	id, state, _ := b.tryAddClient(conn)
	return id, state
}

func (b *TLSBroadcaster) runClient(id uint64, state *tlsBroadcastClientState) {
	for {
		select {
		case <-state.done:
			return
		case event := <-state.queue:
			if state.isDead() {
				return
			}
			if deadlineClient, ok := state.conn.(tlsBroadcastDeadlineClient); ok {
				if err := deadlineClient.SetWriteDeadline(time.Now().Add(tlsBroadcastWriteTimeout)); err != nil {
					if !state.isDead() {
						b.recordWriteDeadlineFailure()
					}
					b.removeClient(id, state)
					return
				}
			}
			if err := state.conn.WriteJSON(event); err != nil {
				if !state.isDead() {
					b.recordWriteFailure()
				}
				b.removeClient(id, state)
				return
			}
		}
	}
}

func (b *TLSBroadcaster) recordQueueFullDrop() {
	b.mu.Lock()
	b.queueFullDropsTotal++
	b.mu.Unlock()
	if metrics := deps.CollectorMetrics; metrics != nil {
		metrics.RecordAgentSightCounter("tls.broadcast.queue_full")
	}
}

func (b *TLSBroadcaster) recordWriteFailure() {
	b.mu.Lock()
	b.writeFailuresTotal++
	b.mu.Unlock()
	if metrics := deps.CollectorMetrics; metrics != nil {
		metrics.RecordAgentSightCounter("tls.broadcast.write_failure")
	}
}

func (b *TLSBroadcaster) recordWriteDeadlineFailure() {
	b.mu.Lock()
	b.writeDeadlineFailuresTotal++
	b.mu.Unlock()
	if metrics := deps.CollectorMetrics; metrics != nil {
		metrics.RecordAgentSightCounter("tls.broadcast.write_deadline_failure")
	}
}

func (b *TLSBroadcaster) Status() TLSBroadcastStatus {
	if b == nil {
		return TLSBroadcastStatus{QueueCapacity: tlsBroadcastQueueSize}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	queuedEvents := 0
	for _, state := range b.clients {
		queuedEvents += len(state.queue)
	}
	return TLSBroadcastStatus{
		ActiveClients:              len(b.clients),
		QueuedEvents:               queuedEvents,
		QueueCapacity:              tlsBroadcastQueueSize,
		QueueFullDropsTotal:        b.queueFullDropsTotal,
		WriteFailuresTotal:         b.writeFailuresTotal,
		WriteDeadlineFailuresTotal: b.writeDeadlineFailuresTotal,
	}
}

func (b *TLSBroadcaster) removeClient(id uint64, state *tlsBroadcastClientState) {
	b.mu.Lock()
	if current, ok := b.clients[id]; ok && current == state {
		delete(b.clients, id)
	}
	b.mu.Unlock()
	state.close()
}

func (b *TLSBroadcaster) Close() {
	if b == nil {
		return
	}
	type client struct {
		id    uint64
		state *tlsBroadcastClientState
	}
	b.mu.Lock()
	b.accepting = false
	clients := make([]client, 0, len(b.clients))
	for id, state := range b.clients {
		clients = append(clients, client{id: id, state: state})
	}
	b.mu.Unlock()
	for _, client := range clients {
		b.removeClient(client.id, client.state)
	}
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

	id, state, accepted := b.tryAddClient(conn)
	if !accepted {
		return
	}
	defer b.removeClient(id, state)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (b *TLSBroadcaster) Broadcast(event TLSPlaintextEvent) {
	type client struct {
		id    uint64
		state *tlsBroadcastClientState
	}

	b.mu.Lock()
	clients := make([]client, 0, len(b.clients))
	for id, state := range b.clients {
		clients = append(clients, client{id: id, state: state})
	}
	b.mu.Unlock()

	for _, client := range clients {
		result := client.state.enqueue(event)
		switch result {
		case tlsBroadcastEnqueueAccepted:
			continue
		case tlsBroadcastEnqueueFull:
			b.recordQueueFullDrop()
		}
		b.removeClient(client.id, client.state)
	}
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
	return &TLSBroadcaster{
		clients:   make(map[uint64]*tlsBroadcastClientState),
		upgrade:   upgrade,
		accepting: true,
	}
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
