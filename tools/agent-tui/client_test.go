package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"agent-ebpf-filter/pb"
)

// fakeBackend serves /events/recent and /ws the way the real backend does:
// protobuf history over HTTP and EventBatch frames over websocket, both
// gated on X-API-KEY.
type fakeBackend struct {
	token    string
	upgrader websocket.Upgrader
	mu       sync.Mutex
	conns    int
	server   *httptest.Server
	batches  chan *pb.EventBatch
}

func newFakeBackend(t *testing.T, token string) *fakeBackend {
	t.Helper()
	fb := &fakeBackend{token: token, batches: make(chan *pb.EventBatch, 16)}
	mux := http.NewServeMux()
	mux.HandleFunc("/events/recent", func(w http.ResponseWriter, r *http.Request) {
		if !fb.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !strings.Contains(r.Header.Get("Accept"), "application/x-protobuf") {
			http.Error(w, "expected protobuf accept header", http.StatusNotAcceptable)
			return
		}
		history := &pb.EventHistoryResponse{Source: "memory", Events: []*pb.CapturedEventRecord{
			{Event: sampleEvent("openat", "node", "/old-1", 1), Timestamp: 1_000},
			{Event: sampleEvent("openat", "node", "/old-2", 2), Timestamp: 2_000},
		}}
		data, err := proto.Marshal(history)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(data)
	})
	mux.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		if !fb.authorized(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
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
		for batch := range fb.batches {
			data, err := proto.Marshal(batch)
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
				return
			}
		}
	})
	fb.server = httptest.NewServer(mux)
	t.Cleanup(fb.server.Close)
	return fb
}

func (fb *fakeBackend) authorized(r *http.Request) bool {
	return fb.token == "" || r.Header.Get("X-API-KEY") == fb.token
}

func (fb *fakeBackend) connections() int {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	return fb.conns
}

func waitUntil(t *testing.T, cond func() bool, what string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStreamBackfillsThenStreamsWithAuth(t *testing.T) {
	fb := newFakeBackend(t, "secret")
	model := NewModel(100)
	stream := NewStream(Config{BackendURL: fb.server.URL, Token: "secret", History: 100, Backfill: 50}, model)
	stream.minBackoff, stream.maxBackoff = 10*time.Millisecond, 50*time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		stream.Run(ctx)
		close(done)
	}()

	waitUntil(t, func() bool { return model.Summary(time.Now(), 0).State == StateConnected }, "connection")
	rows := model.Snapshot(nil)
	if len(rows) != 2 || rows[0].ev.GetPath() != "/old-1" || !rows[0].at.Equal(time.UnixMilli(1_000)) {
		t.Fatalf("backfilled rows = %v", pids(rows))
	}

	fb.batches <- &pb.EventBatch{Events: []*pb.Event{sampleEvent("execve", "bash", "/live", 3)}}
	waitUntil(t, func() bool { return model.Summary(time.Now(), 0).Total == 3 }, "live event")
	rows = model.Snapshot(rows)
	if rows[2].ev.GetPath() != "/live" || time.Since(rows[2].at) > time.Minute {
		t.Fatalf("live row = %+v", rows[2])
	}

	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("stream did not stop after cancellation")
	}
	if state := model.Summary(time.Now(), 0); state.State != StateDisconnected || state.StateDetail != "stopped" {
		t.Fatalf("final state = %+v", state)
	}
}

func TestStreamReconnectsAfterServerDrop(t *testing.T) {
	fb := newFakeBackend(t, "")
	model := NewModel(100)
	stream := NewStream(Config{BackendURL: fb.server.URL, History: 100}, model)
	stream.minBackoff, stream.maxBackoff = 10*time.Millisecond, 20*time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stream.Run(ctx)

	waitUntil(t, func() bool { return fb.connections() == 1 }, "first connection")
	// Closing the batch channel ends the server-side handler, dropping the socket.
	close(fb.batches)
	waitUntil(t, func() bool { return fb.connections() >= 2 }, "reconnect")
	waitUntil(t, func() bool {
		s := model.Summary(time.Now(), 0)
		return s.State == StateConnected || strings.Contains(s.StateDetail, "retry")
	}, "state update")
}

func TestStreamReportsUnauthorized(t *testing.T) {
	fb := newFakeBackend(t, "secret")
	model := NewModel(10)
	stream := NewStream(Config{BackendURL: fb.server.URL, Token: "wrong", History: 10}, model)
	stream.minBackoff, stream.maxBackoff = 10*time.Millisecond, 20*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go stream.Run(ctx)
	waitUntil(t, func() bool {
		s := model.Summary(time.Now(), 0)
		return s.State == StateDisconnected && strings.Contains(s.StateDetail, "401")
	}, "401 status")
	if fb.connections() != 0 {
		t.Fatal("unauthorized client was upgraded")
	}
}

func TestWSURLDerivedFromBackend(t *testing.T) {
	for backend, want := range map[string]string{
		"http://127.0.0.1:8080":       "ws://127.0.0.1:8080/ws",
		"https://edge.example/prefix": "wss://edge.example/prefix/ws",
	} {
		got, err := NewStream(Config{BackendURL: backend}, nil).wsURL()
		if err != nil || got != want {
			t.Fatalf("wsURL(%q) = %q, %v; want %q", backend, got, err, want)
		}
	}
}
