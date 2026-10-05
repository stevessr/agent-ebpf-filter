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

func TestRuntimeEventStorePrunesByCountAndRemovesIDIndex(t *testing.T) {
	root := t.TempDir()
	store, _, err := openRuntimeEventStoreWithin(root, filepath.Join(root, "events.pebble"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = store.StopContext(cleanupCtx)
	})

	base := time.Now().UTC().Add(-time.Minute)
	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		record := normalizeCapturedEventRecord(CapturedEventRecord{
			ReceivedAt: base.Add(time.Duration(i) * time.Second),
			Event: &pb.Event{
				Pid:       uint32(100 + i),
				Type:      "read",
				EventType: pb.EventType_READ,
				Comm:      "codex",
				Path:      "/tmp/prune-count",
			},
		})
		ids = append(ids, record.Envelope.GetEventId())
		if accepted, err := store.Enqueue(record); err != nil || !accepted {
			t.Fatalf("Enqueue(%d) = %t, %v", i, accepted, err)
		}
	}
	if err := store.FlushContext(ctx); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.Prune(ctx, 2, 0)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if deleted != 3 {
		t.Fatalf("Prune() deleted = %d, want 3", deleted)
	}
	recent, err := store.Recent(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].Event.GetPid() != 103 || recent[1].Event.GetPid() != 104 {
		t.Fatalf("remaining PIDs = %v, want [103 104]", replayRecordPIDs(recent))
	}
	for _, eventID := range ids[:3] {
		if _, err := store.GetByID(ctx, eventID); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("GetByID(%q) after prune error = %v, want os.ErrNotExist", eventID, err)
		}
	}
	if _, err := store.GetByID(ctx, ids[4]); err != nil {
		t.Fatalf("newest GetByID() error = %v", err)
	}
}

func TestRuntimeEventStorePrunesByAge(t *testing.T) {
	root := t.TempDir()
	store, _, err := openRuntimeEventStoreWithin(root, filepath.Join(root, "events.pebble"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		_ = store.StopContext(cleanupCtx)
	})

	now := time.Now().UTC()
	fixtures := []struct {
		pid uint32
		at  time.Time
	}{
		{pid: 201, at: now.Add(-3 * time.Hour)},
		{pid: 202, at: now.Add(-2 * time.Hour)},
		{pid: 203, at: now.Add(-10 * time.Minute)},
	}
	ids := make([]string, 0, len(fixtures))
	for _, fixture := range fixtures {
		record := normalizeCapturedEventRecord(CapturedEventRecord{
			ReceivedAt: fixture.at,
			Event: &pb.Event{
				Pid:       fixture.pid,
				Type:      "write",
				EventType: pb.EventType_WRITE,
				Comm:      "codex",
				Path:      "/tmp/prune-age",
			},
		})
		ids = append(ids, record.Envelope.GetEventId())
		if accepted, err := store.Enqueue(record); err != nil || !accepted {
			t.Fatalf("Enqueue(%d) = %t, %v", fixture.pid, accepted, err)
		}
	}
	if err := store.FlushContext(ctx); err != nil {
		t.Fatal(err)
	}

	deleted, err := store.Prune(ctx, 0, time.Hour)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("Prune() deleted = %d, want 2", deleted)
	}
	recent, err := store.Recent(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Event.GetPid() != 203 {
		t.Fatalf("remaining PIDs = %v, want [203]", replayRecordPIDs(recent))
	}
	for _, eventID := range ids[:2] {
		if _, err := store.GetByID(ctx, eventID); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("old GetByID(%q) error = %v, want os.ErrNotExist", eventID, err)
		}
	}
}

