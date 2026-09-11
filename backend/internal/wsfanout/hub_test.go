package wsfanout

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type recordingConn struct {
	mu        sync.Mutex
	messages  [][]byte
	deadlines []time.Time
	writeErr  error
	closed    int
}

func (c *recordingConn) WriteMessage(messageType int, data []byte) error {
	if c.writeErr != nil {
		return c.writeErr
	}
	if messageType != websocket.BinaryMessage {
		return errors.New("unexpected websocket message type")
	}
	c.mu.Lock()
	c.messages = append(c.messages, append([]byte(nil), data...))
	c.mu.Unlock()
	return nil
}

func (c *recordingConn) SetWriteDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, deadline)
	c.mu.Unlock()
	return nil
}

func (c *recordingConn) Close() error {
	c.mu.Lock()
	c.closed++
	c.mu.Unlock()
	return nil
}

func (c *recordingConn) snapshot() (messages [][]byte, deadlines, closes int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	messages = make([][]byte, len(c.messages))
	for i, message := range c.messages {
		messages[i] = append([]byte(nil), message...)
	}
	return messages, len(c.deadlines), c.closed
}

type blockingConn struct {
	mu         sync.Mutex
	startOnce  sync.Once
	closeOnce  sync.Once
	started    chan struct{}
	closed     chan struct{}
	closeCount int
}

func newBlockingConn() *blockingConn {
	return &blockingConn{started: make(chan struct{}), closed: make(chan struct{})}
}

func (c *blockingConn) WriteMessage(int, []byte) error {
	c.startOnce.Do(func() { close(c.started) })
	<-c.closed
	return errors.New("blocking client closed")
}

func (c *blockingConn) SetWriteDeadline(time.Time) error { return nil }

func (c *blockingConn) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (c *blockingConn) closes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCount
}

func waitFor(t *testing.T, conn *recordingConn, messages, closes int) ([][]byte, int, int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		gotMessages, gotDeadlines, gotCloses := conn.snapshot()
		if len(gotMessages) >= messages && gotCloses >= closes {
			return gotMessages, gotDeadlines, gotCloses
		}
		if time.Now().After(deadline) {
			t.Fatalf("client messages/closes = %d/%d, want at least %d/%d", len(gotMessages), gotCloses, messages, closes)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestHubBroadcastAndClose(t *testing.T) {
	hub := New(Options{})
	conn := &recordingConn{}
	client := hub.Add(conn)
	if !client.Alive() {
		t.Fatal("freshly added client is not alive")
	}

	if failures := hub.Broadcast(NewBinaryMessage([]byte("batch-one"))); failures != 0 {
		t.Fatalf("Broadcast failures = %d, want 0", failures)
	}
	messages, deadlines, closes := waitFor(t, conn, 1, 0)
	if string(messages[0]) != "batch-one" || deadlines != 1 || closes != 0 {
		t.Fatalf("client snapshot = messages:%q deadlines:%d closes:%d", messages, deadlines, closes)
	}

	hub.Close()
	hub.Close()
	_, _, closes = waitFor(t, conn, 1, 1)
	if closes != 1 || hub.Len() != 0 || !hub.Closed() || client.Alive() {
		t.Fatalf("after close: closes=%d len=%d closed=%v alive=%v", closes, hub.Len(), hub.Closed(), client.Alive())
	}
	if late := hub.Add(&recordingConn{}); late.Alive() {
		t.Fatal("closed hub accepted a client")
	}
}

func TestHubSlowClientDoesNotBlockHealthyClient(t *testing.T) {
	hub := New(Options{})
	defer hub.Close()
	slow := newBlockingConn()
	healthy := &recordingConn{}
	hub.Add(slow)
	hub.Add(healthy)

	started := time.Now()
	if failures := hub.Broadcast(NewBinaryMessage([]byte("batch"))); failures != 0 {
		t.Fatalf("initial Broadcast failures = %d", failures)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Broadcast blocked for %s on slow client", elapsed)
	}
	select {
	case <-slow.started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow client writer did not start")
	}
	messages, _, _ := waitFor(t, healthy, 1, 0)
	if string(messages[0]) != "batch" {
		t.Fatalf("healthy client message = %q", messages[0])
	}
}

func TestHubDropsFullAndFailedClients(t *testing.T) {
	t.Run("full queue", func(t *testing.T) {
		hub := New(Options{QueueSize: 4})
		slow := newBlockingConn()
		hub.Add(slow)
		hub.Broadcast(NewBinaryMessage([]byte("initial")))
		select {
		case <-slow.started:
		case <-time.After(2 * time.Second):
			t.Fatal("slow client writer did not start")
		}

		failures := 0
		for i := 0; i < hub.opts.QueueSize+1; i++ {
			failures += hub.Broadcast(NewBinaryMessage([]byte("queued")))
		}
		if failures == 0 || hub.Len() != 0 || slow.closes() != 1 {
			t.Fatalf("full queue failures/len/closes = %d/%d/%d", failures, hub.Len(), slow.closes())
		}
	})

	t.Run("write failure", func(t *testing.T) {
		hub := New(Options{})
		conn := &recordingConn{writeErr: errors.New("write failed")}
		hub.Add(conn)
		hub.Broadcast(NewBinaryMessage([]byte("batch")))
		waitFor(t, conn, 0, 1)
		if hub.Len() != 0 {
			t.Fatalf("failed client count = %d, want 0", hub.Len())
		}
		if failures := hub.Broadcast(NewBinaryMessage([]byte("drain-errors"))); failures != 1 {
			t.Fatalf("asynchronous write failures = %d, want 1", failures)
		}
	})
}

func TestHubEmptyMessageIsIgnored(t *testing.T) {
	hub := New(Options{})
	defer hub.Close()
	conn := &recordingConn{}
	hub.Add(conn)
	if failures := hub.Broadcast(NewBinaryMessage(nil)); failures != 0 {
		t.Fatalf("empty Broadcast failures = %d", failures)
	}
	if failures := hub.Broadcast(nil); failures != 0 {
		t.Fatalf("nil Broadcast failures = %d", failures)
	}
	time.Sleep(10 * time.Millisecond)
	if messages, _, _ := conn.snapshot(); len(messages) != 0 {
		t.Fatalf("empty broadcast delivered %d messages", len(messages))
	}
}

// TestHubSharesPreparedFrameAcrossRealConnections drives real gorilla
// connections so the WritePreparedMessage path is exercised end to end.
func TestHubSharesPreparedFrameAcrossRealConnections(t *testing.T) {
	hub := New(Options{PingPeriod: -1})
	defer hub.Close()
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		client := hub.Add(conn)
		defer hub.Remove(client)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	url := "ws" + strings.TrimPrefix(server.URL, "http")
	const clients = 3
	conns := make([]*websocket.Conn, 0, clients)
	for i := 0; i < clients; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("Dial() error = %v", err)
		}
		defer conn.Close()
		conns = append(conns, conn)
	}
	deadline := time.Now().Add(2 * time.Second)
	for hub.Len() != clients && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if hub.Len() != clients {
		t.Fatalf("registered clients = %d, want %d", hub.Len(), clients)
	}

	msg := NewBinaryMessage([]byte("shared-frame"))
	if failures := hub.Broadcast(msg); failures != 0 {
		t.Fatalf("Broadcast failures = %d", failures)
	}
	for i, conn := range conns {
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("client %d ReadMessage() error = %v", i, err)
		}
		if messageType != websocket.BinaryMessage || string(payload) != "shared-frame" {
			t.Fatalf("client %d got type=%d payload=%q", i, messageType, payload)
		}
	}
	if msg.prepared == nil {
		t.Fatal("prepared frame was not built for real connections")
	}
}
