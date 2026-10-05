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

func TestRuntimeEventStorePruneKeepsNewestRecordsAndIndexes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "events.pebble")
	store, _, err := openRuntimeEventStoreWithin(root, path)
	if err != nil {
		t.Fatalf("openRuntimeEventStoreWithin() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := store.StopContext(stopCtx); err != nil {
			t.Errorf("StopContext() error = %v", err)
		}
	})

	base := time.Now().UTC().Add(-time.Hour)
	ids := make([]string, 0, 5)
	for i := 0; i < 5; i++ {
		record := normalizeCapturedEventRecord(CapturedEventRecord{
			ReceivedAt: base.Add(time.Duration(i) * time.Minute),
			Event: &pb.Event{
				Pid:       uint32(5000 + i),
				Type:      "execve",
				EventType: pb.EventType_EXECVE,
				Comm:      "agent",
				Path:      "/tmp/tool",
				Tag:       "AI Agent",
			},
		})
		ids = append(ids, record.Envelope.GetEventId())
		accepted, err := store.Enqueue(record)
		if err != nil || !accepted {
			t.Fatalf("Enqueue(%d) = %t, %v", i, accepted, err)
		}
	}
	if err := store.FlushContext(ctx); err != nil {
		t.Fatalf("FlushContext() error = %v", err)
	}

	deleted, err := store.Prune(ctx, 3, 0)
	if err != nil {
		t.Fatalf("Prune() error = %v", err)
	}
	if deleted != 2 {
		t.Fatalf("Prune() deleted = %d, want 2", deleted)
	}

	recent, err := store.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent() error = %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("Recent() len = %d, want 3", len(recent))
	}
	for i, record := range recent {
		wantID := ids[i+2]
		if got := record.Envelope.GetEventId(); got != wantID {
			t.Fatalf("Recent()[%d] id = %q, want %q", i, got, wantID)
		}
	}

	for _, prunedID := range ids[:2] {
		if _, err := store.GetByID(ctx, prunedID); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("GetByID(pruned %q) error = %v, want os.ErrNotExist", prunedID, err)
		}
	}
	for _, keptID := range ids[2:] {
		if _, err := store.GetByID(ctx, keptID); err != nil {
			t.Fatalf("GetByID(kept %q) error = %v", keptID, err)
		}
	}
}


func TestRuntimeEventStoreCursorPaging(t *testing.T) {
	root := t.TempDir()
	store, _, err := openRuntimeEventStoreWithin(root, filepath.Join(root, "events.pebble"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer stopCancel()
		if err := store.StopContext(stopCtx); err != nil {
			t.Errorf("StopContext() error = %v", err)
		}
	})

	base := time.Date(2026, 10, 5, 2, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		record := normalizeCapturedEventRecord(CapturedEventRecord{
			ReceivedAt: base.Add(time.Duration(i) * time.Second),
			Event: &pb.Event{
				Pid:       uint32(7000 + i),
				Type:      "execve",
				EventType: pb.EventType_EXECVE,
				Comm:      "agent",
				Path:      "/tmp/paged",
			},
		})
		accepted, err := store.Enqueue(record)
		if err != nil || !accepted {
			t.Fatalf("Enqueue(%d) = %t, %v", i, accepted, err)
		}
	}
	if err := store.FlushContext(ctx); err != nil {
		t.Fatal(err)
	}

	first, cursor, err := store.Page(ctx, 2, "")
	if err != nil {
		t.Fatalf("Page(first) error = %v", err)
	}
	if got := replayRecordPIDs(first); len(got) != 2 || got[0] != 7003 || got[1] != 7004 {
		t.Fatalf("first page PIDs = %v, want [7003 7004]", got)
	}
	if cursor == "" {
		t.Fatal("first page cursor is empty")
	}

	second, cursor, err := store.Page(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("Page(second) error = %v", err)
	}
	if got := replayRecordPIDs(second); len(got) != 2 || got[0] != 7001 || got[1] != 7002 {
		t.Fatalf("second page PIDs = %v, want [7001 7002]", got)
	}
	if cursor == "" {
		t.Fatal("second page cursor is empty")
	}

	third, cursor, err := store.Page(ctx, 2, cursor)
	if err != nil {
		t.Fatalf("Page(third) error = %v", err)
	}
	if got := replayRecordPIDs(third); len(got) != 1 || got[0] != 7000 {
		t.Fatalf("third page PIDs = %v, want [7000]", got)
	}
	if cursor != "" {
		t.Fatalf("third page cursor = %q, want empty", cursor)
	}

	if _, _, err := store.Page(ctx, 2, "not-a-valid-cursor!"); err == nil {
		t.Fatal("Page() accepted an invalid cursor")
	}
}
