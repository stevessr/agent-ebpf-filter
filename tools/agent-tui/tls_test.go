package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rivo/tview"
)

func sampleTLSEvent(typ, comm, host, method string, status int) *TLSEvent {
	return &TLSEvent{
		Type: typ, Timestamp: time.Unix(1_700_000_000, 0).UTC(), PID: 4242, TGID: 4242, Comm: comm,
		Direction: "send", Host: host, Method: method, URL: "/v1/messages", StatusCode: status,
		BodySize: 512, CapturedLen: 512, Vendor: "anthropic", ContentType: "application/json",
	}
}

func TestTLSModelStatsAndFilter(t *testing.T) {
	m := NewTLSModel(3)
	m.OnTLSHistory([]*TLSEvent{
		sampleTLSEvent("http_request", "node", "api.anthropic.com", "POST", 0),
		sampleTLSEvent("http_response", "node", "api.anthropic.com", "", 200),
	})
	m.OnTLSEvents([]*TLSEvent{
		sampleTLSEvent("http_request", "curl", "api.openai.com", "GET", 0),
		nil,
		sampleTLSEvent("http_response", "curl", "api.openai.com", "", 429),
	})
	m.OnTLSHistory([]*TLSEvent{sampleTLSEvent("http_request", "late", "x", "GET", 0)})

	s := m.Summary(5)
	if s.Total != 4 || s.Requests != 2 || s.Responses != 2 || s.Retained != 3 || s.Evicted != 1 {
		t.Fatalf("summary = %+v", s)
	}
	if s.Bytes != 4*512 || len(s.TopHosts) != 2 || s.TopVendors[0].Key != "anthropic" {
		t.Fatalf("summary histograms = %+v", s)
	}

	rows := m.Snapshot(nil)
	if len(rows) != 3 || rows[0].Type != "http_response" || rows[2].StatusCode != 429 {
		t.Fatalf("snapshot = %+v", rows)
	}
	cases := map[string]int{
		"openai":                        2,
		"host:anthropic":                1,
		"method:get":                    1,
		"status:4":                      1,
		"status:2":                      1,
		"comm:curl -type:http_response": 1,
		"vendor:anthropic":              3,
		"pid:4242":                      3,
		"dir:send":                      3,
		"url:/v1":                       3,
		"type:sse":                      0,
		"unknownkey:x":                  0,
	}
	for raw, want := range cases {
		m.SetFilter(ParseFilter(raw))
		if got := len(m.Snapshot(rows)); got != want {
			t.Fatalf("filter %q matched %d rows, want %d", raw, got, want)
		}
	}
	m.Clear()
	if m.Summary(0).Total != 0 || len(m.Snapshot(nil)) != 0 {
		t.Fatal("Clear left data behind")
	}
}

type fakeTLSBackend struct {
	server   *httptest.Server
	upgrader websocket.Upgrader
	events   chan *TLSEvent
	mu       sync.Mutex
	conns    int
	disabled bool
}

func newFakeTLSBackend(t *testing.T, disabled bool) *fakeTLSBackend {
	t.Helper()
	fb := &fakeTLSBackend{events: make(chan *TLSEvent, 8), disabled: disabled}
	mux := http.NewServeMux()
	mux.HandleFunc("/tls-capture/recent", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"events": []*TLSEvent{
			sampleTLSEvent("http_request", "node", "api.anthropic.com", "POST", 0),
			sampleTLSEvent("http_response", "node", "api.anthropic.com", "", 200),
		}})
	})
	mux.HandleFunc("/ws/tls-capture", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-KEY") != "secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if fb.disabled {
			http.Error(w, `{"error":"tls_capture is disabled"}`, http.StatusForbidden)
			return
		}
		conn, err := fb.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		fb.mu.Lock()
		fb.conns++
		fb.mu.Unlock()
		for ev := range fb.events {
			data, _ := json.Marshal(ev)
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	})
	fb.server = httptest.NewServer(mux)
	t.Cleanup(fb.server.Close)
	return fb
}

func TestTLSStreamBackfillsThenStreams(t *testing.T) {
	fb := newFakeTLSBackend(t, false)
	model := NewTLSModel(100)
	stream := NewTLSStream(Config{BackendURL: fb.server.URL, Token: "secret", History: 100, Backfill: 10}, model)
	stream.minBackoff, stream.maxBackoff = 10*time.Millisecond, 20*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		stream.Run(ctx)
		close(done)
	}()

	waitUntil(t, func() bool { return model.Summary(0).State == StateConnected }, "tls connection")
	if rows := model.Snapshot(nil); len(rows) != 2 || rows[0].Method != "POST" {
		t.Fatalf("backfilled rows = %+v", rows)
	}
	fb.events <- sampleTLSEvent("sse_message", "node", "api.anthropic.com", "", 0)
	waitUntil(t, func() bool { return model.Summary(0).Total == 3 }, "live tls event")
	if rows := model.Snapshot(nil); rows[2].Type != "sse_message" {
		t.Fatalf("live row = %+v", rows[2])
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("tls stream did not stop")
	}
}

func TestTLSStreamReportsDisabledCapture(t *testing.T) {
	fb := newFakeTLSBackend(t, true)
	model := NewTLSModel(10)
	stream := NewTLSStream(Config{BackendURL: fb.server.URL, Token: "secret", History: 10}, model)
	stream.minBackoff, stream.maxBackoff = 10*time.Millisecond, 20*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stream.Run(ctx)
	waitUntil(t, func() bool {
		s := model.Summary(0)
		return s.State == StateDisconnected && strings.Contains(s.StateDetail, "disabled")
	}, "disabled state")
}

func TestTLSViewRendering(t *testing.T) {
	ev := sampleTLSEvent("http_request", "node", "api.anthropic.com", "POST", 0)
	ev.Headers = map[string]string{"content-type": "application/json", "authorization": "[REDACTED]"}
	ev.Body = `{"model":"claude","messages":[]}`
	content := &tlsContent{rows: []*TLSEvent{ev, sampleTLSEvent("http_response", "node", "api.anthropic.com", "", 503)}}
	if content.GetRowCount() != 3 || content.GetColumnCount() != len(tlsColumns) {
		t.Fatalf("rows/cols = %d/%d", content.GetRowCount(), content.GetColumnCount())
	}
	row := func(r int) []string {
		out := make([]string, len(tlsColumns))
		for c := range tlsColumns {
			out[c] = content.GetCell(r, c).Text
		}
		return out
	}
	first := row(1)
	if first[1] != "→" || first[3] != "node" || first[5] != "POST" || first[6] != "" || first[7] != "512 B" || first[8] != "api.anthropic.com" || !strings.Contains(first[9], "/v1/messages") {
		t.Fatalf("request row = %q", first)
	}
	if second := row(2); second[6] != "503" {
		t.Fatalf("response row = %q", second)
	}
	if content.GetCell(3, 0) != nil || content.GetCell(1, len(tlsColumns)) != nil {
		t.Fatal("out-of-range cells were not nil")
	}

	detail := describeTLSEvent(ev)
	for _, want := range []string{"POST /v1/messages", "api.anthropic.com", "authorization:", tview.Escape("[REDACTED]"), `{"model":"claude"`, "anthropic"} {
		if !strings.Contains(detail, want) {
			t.Fatalf("detail missing %q:\n%s", want, detail)
		}
	}
	if humanBytes(0) != "" || humanBytes(1023) != "1023 B" || humanBytes(2048) != "2.0 K" || humanBytes(3<<20) != "3.0 M" {
		t.Fatal("humanBytes formatting changed")
	}
}

func TestUISwitchesBetweenEventAndTLSViews(t *testing.T) {
	model := NewModel(10)
	tlsModel := NewTLSModel(10)
	tlsModel.OnTLSEvents([]*TLSEvent{sampleTLSEvent("http_request", "node", "api.anthropic.com", "POST", 0)})
	tlsModel.OnTLSState(StateConnected, "")
	ui := NewUI(Config{BackendURL: "http://test"}, model, tlsModel)
	ui.refresh()
	if ui.active != viewEvents || len(ui.tlsContent.rows) != 1 {
		t.Fatalf("initial view = %v, tls rows = %d", ui.active, len(ui.tlsContent.rows))
	}
	ui.switchView(viewTLS)
	header := ui.header.GetText(true)
	if ui.active != viewTLS || !strings.Contains(header, "captured 1") || !strings.Contains(header, "req/resp 1/0") {
		t.Fatalf("tls header = %q", header)
	}
	if sidebar := ui.sidebar.GetText(true); !strings.Contains(sidebar, "api.anthropic.com") || !strings.Contains(sidebar, "top hosts") {
		t.Fatalf("tls sidebar = %q", sidebar)
	}
	// A filter must apply immediately even while the view is paused.
	ui.tls.follow = false
	ui.setFilter(ParseFilter("host:openai"))
	ui.refresh()
	if len(ui.tlsContent.rows) != 0 || !strings.Contains(ui.header.GetText(true), "filter host:openai") {
		t.Fatalf("tls filter not applied: rows=%d header=%q", len(ui.tlsContent.rows), ui.header.GetText(true))
	}
	tlsModel.OnTLSEvents([]*TLSEvent{sampleTLSEvent("http_request", "curl", "api.openai.com", "GET", 0)})
	ui.refresh()
	if len(ui.tlsContent.rows) != 0 {
		t.Fatal("paused TLS view advanced without a filter change")
	}
	ui.switchView(viewEvents)
	if ui.active != viewEvents || !ui.activeFilter().IsZero() {
		t.Fatal("event view inherited the TLS filter")
	}
}
