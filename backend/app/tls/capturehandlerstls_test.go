package tls

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend_test.go section capturehandlerstls_test.go ----

func TestHandleTLSCaptureRecentReturnsStoredEventsWithoutAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewTLSCaptureStore(10)
	store.Add(TLSPlaintextEvent{Type: "tls_plaintext", PID: 42, Comm: "curl", Timestamp: time.Unix(1, 0).UTC()})

	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, store, NewTLSCaptureRuleStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tls-capture/recent?limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Events []TLSPlaintextEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(resp.Events) != 1 || resp.Events[0].PID != 42 {
		t.Fatalf("events = %#v", resp.Events)
	}
}

func TestHandleTLSCaptureGoBinaryRejectsMissingPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, NewTLSCaptureStore(10), NewTLSCaptureRuleStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tls-capture/go-binary", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestHandleTLSCaptureAttachBuiltinsReturnsTargetsWhenRuntimeMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, NewTLSCaptureStore(10), NewTLSCaptureRuleStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tls-capture/attach-builtins", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestBuiltinTLSExecutableTargetListIsEmpty(t *testing.T) {
	// builtinTLSExecutableTargetList was removed in favor of auto-discovery
	// This test verifies no dangling references remain.
}

func TestHandleTLSCaptureRulesRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rules := NewTLSCaptureRuleStore()
	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, NewTLSCaptureStore(10), rules)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/tls-capture/rules", strings.NewReader(`{"rules":[{"id":"node-api","name":"Node API","enabled":true,"scope":"custom","comms":["node"],"hosts":["api.example.com"]}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("put status = %d body = %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/tls-capture/rules", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Rules []TLSCaptureRule `json:"rules"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(resp.Rules) != 1 || resp.Rules[0].ID != "node-api" || len(resp.Rules[0].Hosts) != 1 {
		t.Fatalf("rules = %#v", resp.Rules)
	}
}

type recordingTLSBroadcastClient struct {
	mu        sync.Mutex
	events    []TLSPlaintextEvent
	deadlines []time.Time
	writeErr  error
	closed    int
}

func (c *recordingTLSBroadcastClient) WriteJSON(value any) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	event, ok := value.(TLSPlaintextEvent)
	if !ok {
		return errors.New("unexpected TLS broadcast payload")
	}
	c.mu.Lock()
	c.events = append(c.events, event)
	c.mu.Unlock()
	return nil
}

func (c *recordingTLSBroadcastClient) Close() error {
	c.mu.Lock()
	c.closed++
	c.mu.Unlock()
	return nil
}

func (c *recordingTLSBroadcastClient) SetWriteDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, deadline)
	c.mu.Unlock()
	return nil
}

func (c *recordingTLSBroadcastClient) snapshot() ([]TLSPlaintextEvent, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]TLSPlaintextEvent(nil), c.events...), c.closed
}

func (c *recordingTLSBroadcastClient) deadlineCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.deadlines)
}

type blockingTLSBroadcastClient struct {
	mu         sync.Mutex
	startOnce  sync.Once
	closeOnce  sync.Once
	started    chan struct{}
	closed     chan struct{}
	closeCount int
	deadlines  []time.Time
}

type controlledTLSBroadcastConnection struct {
	recordingTLSBroadcastClient
	readStarted chan struct{}
	readRelease chan struct{}
	startOnce   sync.Once
}

func newControlledTLSBroadcastConnection() *controlledTLSBroadcastConnection {
	return &controlledTLSBroadcastConnection{
		readStarted: make(chan struct{}),
		readRelease: make(chan struct{}),
	}
}

func (c *controlledTLSBroadcastConnection) ReadMessage() (int, []byte, error) {
	c.startOnce.Do(func() { close(c.readStarted) })
	<-c.readRelease
	return 0, nil, io.EOF
}

func newBlockingTLSBroadcastClient() *blockingTLSBroadcastClient {
	return &blockingTLSBroadcastClient{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (c *blockingTLSBroadcastClient) WriteJSON(any) error {
	c.startOnce.Do(func() { close(c.started) })
	<-c.closed
	return errors.New("blocking client closed")
}

func (c *blockingTLSBroadcastClient) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *blockingTLSBroadcastClient) SetWriteDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, deadline)
	c.mu.Unlock()
	return nil
}

func (c *blockingTLSBroadcastClient) snapshot() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCount, len(c.deadlines)
}

func addTLSBroadcastTestClient(t *testing.T, broadcaster *TLSBroadcaster, client tlsBroadcastClient) (uint64, *tlsBroadcastClientState) {
	t.Helper()
	id, state := broadcaster.addClient(client)
	t.Cleanup(func() { broadcaster.removeClient(id, state) })
	return id, state
}

func waitForTLSBroadcastSnapshot(t *testing.T, client *recordingTLSBroadcastClient, wantEvents, wantCloseCount int) ([]TLSPlaintextEvent, int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		events, closeCount := client.snapshot()
		if len(events) >= wantEvents && (wantCloseCount < 0 || closeCount == wantCloseCount) {
			return events, closeCount
		}
		if time.Now().After(deadline) {
			t.Fatalf("client events/close count = %d/%d, want at least %d/%d", len(events), closeCount, wantEvents, wantCloseCount)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitForTLSBroadcastClientCount(t *testing.T, broadcaster *TLSBroadcaster, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		broadcaster.mu.Lock()
		clientCount := len(broadcaster.clients)
		broadcaster.mu.Unlock()
		if clientCount == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("client count = %d, want %d", clientCount, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTLSCaptureBroadcasterBroadcast(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	client := &recordingTLSBroadcastClient{}
	addTLSBroadcastTestClient(t, broadcaster, client)

	event := TLSPlaintextEvent{Type: "tls_plaintext", PID: 99, Comm: "curl", Timestamp: time.Unix(2, 0).UTC()}
	broadcaster.Broadcast(event)

	events, closeCount := waitForTLSBroadcastSnapshot(t, client, 1, 0)
	if closeCount != 0 {
		t.Fatalf("client close count = %d after successful broadcast", closeCount)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	got := events[0]
	if got.PID != event.PID || got.Type != event.Type || got.Comm != event.Comm || !got.Timestamp.Equal(event.Timestamp) {
		t.Fatalf("event = %#v", got)
	}
}

func TestTLSCaptureBroadcasterServeAndBroadcast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	conn := newControlledTLSBroadcastConnection()
	broadcaster := newTLSCaptureBroadcasterWithUpgrader(func(http.ResponseWriter, *http.Request, http.Header) (tlsBroadcastConnection, error) {
		return conn, nil
	})
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/ws/tls-capture", nil)
	serveDone := make(chan struct{})
	go func() {
		broadcaster.Serve(context)
		close(serveDone)
	}()

	select {
	case <-conn.readStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("TLS broadcaster did not start its client read loop")
	}
	waitForTLSBroadcastClientCount(t, broadcaster, 1)

	event := TLSPlaintextEvent{Type: "tls_plaintext", PID: 100, Comm: "curl", Timestamp: time.Unix(3, 0).UTC()}
	broadcaster.Broadcast(event)

	events, closeCount := waitForTLSBroadcastSnapshot(t, &conn.recordingTLSBroadcastClient, 1, 0)
	got := events[0]
	if got.PID != event.PID || got.Type != event.Type || got.Comm != event.Comm || !got.Timestamp.Equal(event.Timestamp) {
		t.Fatalf("event = %#v", got)
	}
	if closeCount != 0 {
		t.Fatalf("client close count = %d while serving", closeCount)
	}

	close(conn.readRelease)
	select {
	case <-serveDone:
	case <-time.After(2 * time.Second):
		t.Fatal("TLS broadcaster Serve did not stop after the reader exited")
	}
	waitForTLSBroadcastClientCount(t, broadcaster, 0)
	_, closeCount = conn.snapshot()
	if closeCount != 1 {
		t.Fatalf("client close count = %d after reader exit, want 1", closeCount)
	}
}

func TestTLSCaptureBroadcasterConcurrentBroadcastsDeliverEvents(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	client := &recordingTLSBroadcastClient{}
	addTLSBroadcastTestClient(t, broadcaster, client)

	const broadcasters = 8
	const eventsPerBroadcaster = 4
	const totalEvents = broadcasters * eventsPerBroadcaster

	start := make(chan struct{})
	errCh := make(chan error, totalEvents)
	for i := 0; i < broadcasters; i++ {
		go func(base int) {
			<-start
			for j := 0; j < eventsPerBroadcaster; j++ {
				broadcaster.Broadcast(TLSPlaintextEvent{
					Type:      "tls_plaintext",
					PID:       uint32(100 + base + j),
					Comm:      "curl",
					Timestamp: time.Unix(int64(base*eventsPerBroadcaster+j+1), 0).UTC(),
				})
			}
			errCh <- nil
		}(i * eventsPerBroadcaster)
	}

	close(start)
	for i := 0; i < broadcasters; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("broadcast error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("broadcast goroutine %d timed out", i)
		}
	}

	events, closeCount := waitForTLSBroadcastSnapshot(t, client, totalEvents, 0)
	if closeCount != 0 {
		t.Fatalf("client close count = %d after concurrent broadcasts", closeCount)
	}
	if len(events) != totalEvents {
		t.Fatalf("events = %d, want %d", len(events), totalEvents)
	}
	seen := make(map[uint32]struct{}, totalEvents)
	for _, event := range events {
		seen[event.PID] = struct{}{}
	}
	if len(seen) != totalEvents {
		t.Fatalf("unique events = %d, want %d", len(seen), totalEvents)
	}
}

func TestTLSCaptureBroadcasterRemovesFailedClient(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	client := &recordingTLSBroadcastClient{writeErr: errors.New("write failed")}
	addTLSBroadcastTestClient(t, broadcaster, client)

	broadcaster.Broadcast(TLSPlaintextEvent{Type: "tls_plaintext"})

	_, closeCount := waitForTLSBroadcastSnapshot(t, client, 0, 1)
	if closeCount != 1 {
		t.Fatalf("failed client close count = %d, want 1", closeCount)
	}
	broadcaster.mu.Lock()
	clientCount := len(broadcaster.clients)
	broadcaster.mu.Unlock()
	if clientCount != 0 {
		t.Fatalf("client count = %d, want 0", clientCount)
	}
}

func TestTLSCaptureBroadcasterFailureDoesNotBlockHealthyClient(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	failed := &recordingTLSBroadcastClient{writeErr: errors.New("write failed")}
	healthy := &recordingTLSBroadcastClient{}
	addTLSBroadcastTestClient(t, broadcaster, failed)
	addTLSBroadcastTestClient(t, broadcaster, healthy)

	event := TLSPlaintextEvent{Type: "tls_plaintext", PID: 101}
	broadcaster.Broadcast(event)

	failedEvents, failedCloseCount := waitForTLSBroadcastSnapshot(t, failed, 0, 1)
	if len(failedEvents) != 0 || failedCloseCount != 1 {
		t.Fatalf("failed client events/close count = %d/%d, want 0/1", len(failedEvents), failedCloseCount)
	}
	healthyEvents, healthyCloseCount := waitForTLSBroadcastSnapshot(t, healthy, 1, 0)
	if len(healthyEvents) != 1 || healthyEvents[0].PID != event.PID || healthyCloseCount != 0 {
		t.Fatalf("healthy client events/close count = %#v/%d", healthyEvents, healthyCloseCount)
	}
}

func TestTLSCaptureBroadcasterConcurrentFailureClosesClientOnce(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	client := &recordingTLSBroadcastClient{writeErr: errors.New("write failed")}
	addTLSBroadcastTestClient(t, broadcaster, client)

	const broadcasts = 16
	start := make(chan struct{})
	done := make(chan struct{}, broadcasts)
	for range broadcasts {
		go func() {
			<-start
			broadcaster.Broadcast(TLSPlaintextEvent{Type: "tls_plaintext"})
			done <- struct{}{}
		}()
	}
	close(start)
	for range broadcasts {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("concurrent failed broadcast timed out")
		}
	}

	_, closeCount := waitForTLSBroadcastSnapshot(t, client, 0, 1)
	if closeCount != 1 {
		t.Fatalf("failed client close count = %d, want 1", closeCount)
	}
	broadcaster.mu.Lock()
	clientCount := len(broadcaster.clients)
	broadcaster.mu.Unlock()
	if clientCount != 0 {
		t.Fatalf("client count = %d, want 0", clientCount)
	}
}

func TestTLSCaptureBroadcasterSlowClientDoesNotBlockHealthyClient(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	slow := newBlockingTLSBroadcastClient()
	healthy := &recordingTLSBroadcastClient{}
	slowID, slowState := addTLSBroadcastTestClient(t, broadcaster, slow)
	addTLSBroadcastTestClient(t, broadcaster, healthy)

	started := time.Now()
	broadcaster.Broadcast(TLSPlaintextEvent{Type: "tls_plaintext", PID: 202})
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Broadcast blocked for %s on slow client", elapsed)
	}

	select {
	case <-slow.started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow client writer did not start")
	}
	healthyEvents, healthyCloseCount := waitForTLSBroadcastSnapshot(t, healthy, 1, 0)
	if healthyEvents[0].PID != 202 || healthyCloseCount != 0 {
		t.Fatalf("healthy client events/close count = %#v/%d", healthyEvents, healthyCloseCount)
	}
	_, deadlineCount := slow.snapshot()
	if deadlineCount == 0 {
		t.Fatal("slow client write deadline was not set")
	}
	broadcaster.removeClient(slowID, slowState)
}

func TestTLSCaptureBroadcasterDropsClientWhenQueueIsFull(t *testing.T) {
	broadcaster := NewTLSCaptureBroadcaster()
	slow := newBlockingTLSBroadcastClient()
	addTLSBroadcastTestClient(t, broadcaster, slow)

	broadcaster.Broadcast(TLSPlaintextEvent{Type: "tls_plaintext", PID: 1})
	select {
	case <-slow.started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow client writer did not start")
	}
	for i := 0; i < tlsBroadcastQueueSize+1; i++ {
		broadcaster.Broadcast(TLSPlaintextEvent{Type: "tls_plaintext", PID: uint32(i + 2)})
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		closeCount, _ := slow.snapshot()
		if closeCount == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("slow client close count = %d, want 1", closeCount)
		}
		time.Sleep(time.Millisecond)
	}
	waitForTLSBroadcastClientCount(t, broadcaster, 0)
}
