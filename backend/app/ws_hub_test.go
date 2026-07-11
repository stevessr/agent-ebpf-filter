package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/gorilla/websocket"
)

type recordingProtoClient struct {
	mu        sync.Mutex
	messages  [][]byte
	deadlines []time.Time
	writeErr  error
	closed    int
}

func (client *recordingProtoClient) WriteMessage(messageType int, data []byte) error {
	if client.writeErr != nil {
		return client.writeErr
	}
	if messageType != websocket.BinaryMessage {
		return errors.New("unexpected websocket message type")
	}
	client.mu.Lock()
	client.messages = append(client.messages, append([]byte(nil), data...))
	client.mu.Unlock()
	return nil
}

func (client *recordingProtoClient) SetWriteDeadline(deadline time.Time) error {
	client.mu.Lock()
	client.deadlines = append(client.deadlines, deadline)
	client.mu.Unlock()
	return nil
}

func (client *recordingProtoClient) Close() error {
	client.mu.Lock()
	client.closed++
	client.mu.Unlock()
	return nil
}

func (client *recordingProtoClient) snapshot() ([][]byte, int, int) {
	client.mu.Lock()
	defer client.mu.Unlock()
	messages := make([][]byte, len(client.messages))
	for i, message := range client.messages {
		messages[i] = append([]byte(nil), message...)
	}
	return messages, len(client.deadlines), client.closed
}

type blockingProtoClient struct {
	mu         sync.Mutex
	startOnce  sync.Once
	closeOnce  sync.Once
	started    chan struct{}
	closed     chan struct{}
	closeCount int
}

func newBlockingProtoClient() *blockingProtoClient {
	return &blockingProtoClient{
		started: make(chan struct{}),
		closed:  make(chan struct{}),
	}
}

func (client *blockingProtoClient) WriteMessage(int, []byte) error {
	client.startOnce.Do(func() { close(client.started) })
	<-client.closed
	return errors.New("blocking client closed")
}

func (client *blockingProtoClient) SetWriteDeadline(time.Time) error { return nil }

func (client *blockingProtoClient) Close() error {
	client.mu.Lock()
	client.closeCount++
	client.mu.Unlock()
	client.closeOnce.Do(func() { close(client.closed) })
	return nil
}

func (client *blockingProtoClient) closes() int {
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.closeCount
}

func waitForProtoClient(t *testing.T, client *recordingProtoClient, messages, closes int) ([][]byte, int, int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		gotMessages, gotDeadlines, gotCloses := client.snapshot()
		if len(gotMessages) >= messages && gotCloses >= closes {
			return gotMessages, gotDeadlines, gotCloses
		}
		if time.Now().After(deadline) {
			t.Fatalf("client messages/closes = %d/%d, want at least %d/%d", len(gotMessages), gotCloses, messages, closes)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestProtoClientHubBroadcastAndClose(t *testing.T) {
	hub := newProtoClientHub()
	client := &recordingProtoClient{}
	hub.addClient(client)

	if writeErrors := hub.Broadcast([]byte("batch-one")); writeErrors != 0 {
		t.Fatalf("Broadcast write errors = %d, want 0", writeErrors)
	}
	messages, deadlineCount, closeCount := waitForProtoClient(t, client, 1, 0)
	if string(messages[0]) != "batch-one" || deadlineCount != 1 || closeCount != 0 {
		t.Fatalf("client snapshot = messages:%q deadlines:%d closes:%d", messages, deadlineCount, closeCount)
	}

	hub.Close()
	hub.Close()
	_, _, closeCount = waitForProtoClient(t, client, 1, 1)
	if closeCount != 1 || hub.ClientCount() != 0 {
		t.Fatalf("hub close count/clients = %d/%d, want 1/0", closeCount, hub.ClientCount())
	}
}

func TestProtoClientHubSlowClientDoesNotBlockHealthyClient(t *testing.T) {
	hub := newProtoClientHub()
	defer hub.Close()
	slow := newBlockingProtoClient()
	healthy := &recordingProtoClient{}
	hub.addClient(slow)
	hub.addClient(healthy)

	started := time.Now()
	if writeErrors := hub.Broadcast([]byte("batch")); writeErrors != 0 {
		t.Fatalf("initial Broadcast write errors = %d", writeErrors)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Broadcast blocked for %s on slow client", elapsed)
	}
	select {
	case <-slow.started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow client writer did not start")
	}
	messages, _, _ := waitForProtoClient(t, healthy, 1, 0)
	if string(messages[0]) != "batch" {
		t.Fatalf("healthy client message = %q", messages[0])
	}
}

func TestProtoClientHubDropsFullAndFailedClients(t *testing.T) {
	t.Run("full queue", func(t *testing.T) {
		hub := newProtoClientHub()
		slow := newBlockingProtoClient()
		hub.addClient(slow)
		hub.Broadcast([]byte("initial"))
		select {
		case <-slow.started:
		case <-time.After(2 * time.Second):
			t.Fatal("slow client writer did not start")
		}

		writeErrors := 0
		for i := 0; i < protoClientQueueSize+1; i++ {
			writeErrors += hub.Broadcast([]byte("queued"))
		}
		if writeErrors == 0 || hub.ClientCount() != 0 || slow.closes() != 1 {
			t.Fatalf("full queue errors/clients/closes = %d/%d/%d", writeErrors, hub.ClientCount(), slow.closes())
		}
	})

	t.Run("write failure", func(t *testing.T) {
		hub := newProtoClientHub()
		client := &recordingProtoClient{writeErr: errors.New("write failed")}
		hub.addClient(client)
		hub.Broadcast([]byte("batch"))
		waitForProtoClient(t, client, 0, 1)
		if hub.ClientCount() != 0 {
			t.Fatalf("failed client count = %d, want 0", hub.ClientCount())
		}
		if writeErrors := hub.Broadcast([]byte("drain-errors")); writeErrors != 1 {
			t.Fatalf("asynchronous write errors = %d, want 1", writeErrors)
		}
	})
}

func TestEventBroadcasterStopsClientHubsWithContext(t *testing.T) {
	originalContext := AppCtx
	appContext := newAppContext()
	appContext.Broadcast = make(chan *pb.Event)
	appContext.EventClientHub = newProtoClientHub()
	appContext.EnvelopeClientHub = newProtoClientHub()
	AppCtx = appContext
	t.Cleanup(func() {
		appContext.Network.Close()
		AppCtx = originalContext
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runEventBroadcaster(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("event broadcaster did not stop after cancellation")
	}
	appContext.EventClientHub.mu.Lock()
	eventsClosed := appContext.EventClientHub.closed
	appContext.EventClientHub.mu.Unlock()
	appContext.EnvelopeClientHub.mu.Lock()
	envelopesClosed := appContext.EnvelopeClientHub.closed
	appContext.EnvelopeClientHub.mu.Unlock()
	if !eventsClosed || !envelopesClosed {
		t.Fatalf("client hubs closed = events:%v envelopes:%v", eventsClosed, envelopesClosed)
	}
}
