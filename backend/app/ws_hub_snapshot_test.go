package app

import (
	"sync"
	"testing"
	"time"
)

type snapshotNoopProtoClient struct{}

func (*snapshotNoopProtoClient) WriteMessage(int, []byte) error       { return nil }
func (*snapshotNoopProtoClient) SetWriteDeadline(time.Time) error     { return nil }
func (*snapshotNoopProtoClient) Close() error                         { return nil }

func TestProtoClientHubSnapshotConcurrentBroadcastAndMembership(t *testing.T) {
	hub := newProtoClientHub()
	defer hub.Close()

	const iterations = 1000
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		payload := []byte("snapshot-race")
		for iteration := 0; iteration < iterations; iteration++ {
			hub.Broadcast(payload)
		}
	}()
	go func() {
		defer wg.Done()
		for iteration := 0; iteration < iterations; iteration++ {
			id, state := hub.addClient(&snapshotNoopProtoClient{})
			hub.removeClient(id, state)
		}
	}()
	wg.Wait()
}

func BenchmarkProtoClientHubBroadcastSnapshot(b *testing.B) {
	hub := newProtoClientHub()
	state := newProtoClientState(nil)
	hub.mu.Lock()
	hub.clients[1] = state
	hub.publishClientSnapshotLocked()
	hub.mu.Unlock()

	payload := []byte("protobuf-batch")
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if writeErrors := hub.Broadcast(payload); writeErrors != 0 {
			b.Fatalf("Broadcast write errors = %d, want 0", writeErrors)
		}
		<-state.queue
	}
}

func BenchmarkProtoClientHubBroadcastRecipientCopyBaseline(b *testing.B) {
	hub := newProtoClientHub()
	state := newProtoClientState(nil)
	hub.mu.Lock()
	hub.clients[1] = state
	hub.publishClientSnapshotLocked()
	hub.mu.Unlock()

	type client struct {
		id    uint64
		state *protoClientState
	}
	payload := []byte("protobuf-batch")
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		hub.mu.Lock()
		clients := make([]client, 0, len(hub.clients))
		for id, state := range hub.clients {
			clients = append(clients, client{id: id, state: state})
		}
		hub.mu.Unlock()
		for _, client := range clients {
			if !client.state.enqueue(payload) {
				b.Fatal("client queue unexpectedly full")
			}
			<-client.state.queue
		}
	}
}
