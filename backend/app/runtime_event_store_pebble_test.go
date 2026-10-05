package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestRuntimeEventStoreRoundTripByID(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "events.pebble")
	store, resolved, err := openRuntimeEventStoreWithin(root, path)
	if err != nil {
		t.Fatalf("openRuntimeEventStoreWithin() error = %v", err)
	}
	if resolved != path {
		t.Fatalf("resolved path = %q, want %q", resolved, path)
	}

	record := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Date(2026, 10, 5, 1, 30, 0, 123, time.UTC),
		Event: &pb.Event{
			Pid:       4242,
			Ppid:      1,
			Uid:       1000,
			Type:      "execve",
			EventType: pb.EventType_EXECVE,
			Comm:      "codex",
			Path:      "/usr/bin/codex",
			Tag:       "AI Agent",
		},
	})
	eventID := record.Envelope.GetEventId()
	if eventID == "" {
		t.Fatal("normalized event id is empty")
	}

	accepted, err := store.Enqueue(record)
	if err != nil || !accepted {
		t.Fatalf("Enqueue() = %t, %v", accepted, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := store.FlushContext(ctx); err != nil {
		t.Fatalf("FlushContext() error = %v", err)
	}

	recent, err := store.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(recent) != 1 {
		t.Fatalf("Recent() len = %d, want 1", len(recent))
	}
	if got := recent[0].Envelope.GetEventId(); got != eventID {
		t.Fatalf("Recent() event id = %q, want %q", got, eventID)
	}

	got, err := store.GetByID(ctx, eventID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if got.Event.GetPid() != 4242 || got.Event.GetPath() != "/usr/bin/codex" {
		t.Fatalf("GetByID() event = %+v", got.Event)
	}

	if err := store.StopContext(ctx); err != nil {
		t.Fatalf("StopContext() before reopen error = %v", err)
	}

	reopened, _, err := openRuntimeEventStoreWithin(root, path)
	if err != nil {
		t.Fatalf("reopen event store error = %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if err := reopened.StopContext(cleanupCtx); err != nil {
			t.Errorf("reopened StopContext() error = %v", err)
		}
	})

	got, err = reopened.GetByID(ctx, eventID)
	if err != nil {
		t.Fatalf("GetByID() after reopen error = %v", err)
	}
	if got.Event.GetPid() != 4242 || got.Envelope.GetEventId() != eventID {
		t.Fatalf("GetByID() after reopen = %+v", got)
	}

	if err := reopened.Clear(ctx); err != nil {
		t.Fatalf("Clear() error = %v", err)
	}
	recent, err = reopened.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent() after Clear error = %v", err)
	}
	if len(recent) != 0 {
		t.Fatalf("Recent() after Clear len = %d, want 0", len(recent))
	}
	if _, err := reopened.GetByID(ctx, eventID); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("GetByID() after Clear error = %v, want os.ErrNotExist", err)
	}
}
