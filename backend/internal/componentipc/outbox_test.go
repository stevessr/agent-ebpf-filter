package componentipc

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"agent-ebpf-filter/udsframe"
)

func TestOutboxLimitsAndCopiesBorrowedPayload(t *testing.T) {
	client, server := handshakePair(t, RoleCollector, RoleEngine)
	box, err := NewOutbox(client, 1)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("before")
	if got := box.TryEnqueue(KindEvent, payload); got != Enqueued {
		t.Fatalf("accepted status %v", got)
	}
	payload[0] = 'X'
	if got := box.TryEnqueue(KindEvent, []byte("overflow")); got != QueueSaturated {
		t.Fatalf("saturation status %v", got)
	}
	if got := box.TryEnqueue(KindAlert, []byte("forge")); got != InvalidMessage {
		t.Fatalf("role-denied status %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- box.Run(ctx) }()
	message, err := server.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if string(message.Payload) != "before" {
		t.Fatalf("accepted payload was mutated: %q", message.Payload)
	}
	// Receive unblocks the writer, but delivery metrics update only after
	// WriteTyped has fully returned; synchronize before checking counters.
	deadline := time.Now().Add(2 * time.Second)
	for box.Stats().Sent != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if box.Stats().Sent != 1 {
		t.Fatal("outbox did not account for delivered frame")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run should stop on cancel: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("outbox did not stop")
	}
	stats := box.Stats()
	if stats.Accepted != 1 || stats.Dropped != 2 || stats.Sent != 1 || stats.Queued != 0 || stats.Accepting {
		t.Fatalf("unexpected final statistics: %+v", stats)
	}
	if got := box.TryEnqueue(KindEvent, []byte("late")); got != QueueClosed {
		t.Fatalf("stopped outbox accepted new data: %v", got)
	}
}

func TestOutboxBoundsAndLifecycle(t *testing.T) {
	if _, err := NewOutbox(nil, 1); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("nil session allowed: %v", err)
	}
	client, _ := handshakePair(t, RoleCollector, RoleEngine)
	if _, err := NewOutbox(client, MaxPendingMessages+1); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("unbounded pending queue allowed: %v", err)
	}
	box, err := NewOutbox(client, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := box.TryEnqueue(KindEvent, []byte("pending")); got != Enqueued {
		t.Fatalf("initial enqueue: %v", got)
	}
	box.Stop()
	box.Stop()
	if stats := box.Stats(); stats.Dropped != 1 || stats.Accepted != 1 || stats.Queued != 0 {
		t.Fatalf("shutdown must account for undelivered items: %+v", stats)
	}
	if got := box.TryEnqueue(KindEvent, []byte("event")); got != QueueClosed {
		t.Fatalf("stopped queue result %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := box.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled run %v", err)
	}
	if err := box.Run(context.Background()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("second run started: %v", err)
	}
}

func TestReceiveRejectsUnauthorizedAndOversizedWire(t *testing.T) {
	for _, test := range []struct {
		name string
		wire func(net.Conn) error
		want error
	}{
		{"unauthorized", func(c net.Conn) error {
			return udsframe.WriteTyped(c, byte(KindAlert), []byte("forged"))
		}, ErrFrameDenied},
		{"oversized", func(c net.Conn) error {
			var header [4]byte
			header[0], header[1], header[2], header[3] = 0, 16, 0, 2
			_, err := c.Write(header[:])
			return err
		}, udsframe.ErrInvalidPayloadSize},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, c := net.Pipe()
			defer s.Close()
			defer c.Close()
			receiver := &Session{conn: s, peer: RoleCollector, local: RoleEngine}
			ch := make(chan error, 1)
			go func() { ch <- test.wire(c) }()
			_, err := receiver.Receive()
			if !errors.Is(err, test.want) {
				t.Fatalf("wire rejection = %v, want %v", err, test.want)
			}
			if wireErr := <-ch; wireErr != nil {
				t.Fatal(wireErr)
			}
		})
	}
}

func TestOutboxStopWakesIdleRun(t *testing.T) {
	client, _ := handshakePair(t, RoleCollector, RoleEngine)
	box, err := NewOutbox(client, 4)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- box.Run(context.Background()) }()
	box.Stop()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("stopped worker should exit normally: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not wake an idle Run worker")
	}
	if stats := box.Stats(); stats.Accepting || stats.Queued != 0 {
		t.Fatalf("Stop left live queue: %+v", stats)
	}
}

func TestOutboxConcurrentShutdownAccountsForEveryAttempt(t *testing.T) {
	client, _ := handshakePair(t, RoleCollector, RoleEngine)
	box, err := NewOutbox(client, 8)
	if err != nil {
		t.Fatal(err)
	}
	const producers, attempts = 20, 32
	var wg sync.WaitGroup
	for i := 0; i < producers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < attempts; j++ {
				box.TryEnqueue(KindEvent, []byte("event"))
			}
		}()
	}
	box.Stop()
	wg.Wait()
	box.Stop() // idempotent even after racing producers
	stats := box.Stats()
	if stats.Accepted+stats.Dropped != producers*attempts+stats.Accepted {
		// Stop counts every unsent accepted item as dropped in addition
		// to any admission rejection. No worker is active in this test.
		t.Fatalf("accounting mismatch: %+v", stats)
	}
	if stats.Dropped != producers*attempts || stats.Sent != 0 || stats.Failed != 0 || stats.Queued != 0 || stats.Accepting {
		t.Fatalf("shutdown failed to account for pending frames: %+v", stats)
	}
	if err := box.Run(context.Background()); err != nil {
		t.Fatalf("Run after Stop should return without hanging: %v", err)
	}
}
