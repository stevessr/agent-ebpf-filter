package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

func TestTLSCaptureBroadcasterServeAndBroadcast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	broadcaster := newTLSCaptureBroadcaster()
	r := gin.New()
	r.GET("/ws/tls-capture", broadcaster.Serve)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/tls-capture"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	event := TLSPlaintextEvent{Type: "tls_plaintext", PID: 99, Comm: "curl", Timestamp: time.Unix(2, 0).UTC()}
	broadcaster.Broadcast(event)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got TLSPlaintextEvent
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read json: %v", err)
	}
	if got.PID != event.PID || got.Type != event.Type || got.Comm != event.Comm || !got.Timestamp.Equal(event.Timestamp) {
		t.Fatalf("event = %#v", got)
	}
}

func TestTLSCaptureBroadcasterConcurrentBroadcastsDeliverEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	broadcaster := newTLSCaptureBroadcaster()
	r := gin.New()
	r.GET("/ws/tls-capture", broadcaster.Serve)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/tls-capture"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

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

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	seen := make(map[uint32]struct{}, totalEvents)
	for len(seen) < totalEvents {
		var got TLSPlaintextEvent
		if err := conn.ReadJSON(&got); err != nil {
			t.Fatalf("read json after %d events: %v", len(seen), err)
		}
		seen[got.PID] = struct{}{}
	}
}
