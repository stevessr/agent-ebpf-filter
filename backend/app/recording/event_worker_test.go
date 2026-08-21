package recording

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestEventRecordingQueueIsBoundedAndInactivePathDoesNoWork(t *testing.T) {
	store := NewState()
	record := CapturedEventRecord{Event: &pb.Event{Pid: 1, Comm: "codex"}}
	store.Record(record)
	if status := store.Status(); status.EnqueuedTotal != 0 || status.DroppedTotal != 0 {
		t.Fatalf("inactive recording changed counters: %+v", status)
	}

	store.mu.Lock()
	store.started = true
	store.queue = make(chan CapturedEventRecord, 1)
	store.queueCap = 1
	store.mu.Unlock()
	store.Record(record)
	store.Record(record)
	status := store.Status()
	if !status.Active || status.QueueLen != 1 || status.QueueCap != 1 || status.EnqueuedTotal != 1 || status.DroppedTotal != 1 {
		t.Fatalf("unexpected bounded queue status: %+v", status)
	}
}

func TestEventRecordingStopDrainsEveryAcceptedRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	store := NewState()
	if _, err := store.startAtRoot(root, "events.jsonl", true); err != nil {
		t.Fatalf("startAtRoot() error = %v", err)
	}
	defer store.Stop()

	for index := 0; index < 750; index++ {
		store.Record(CapturedEventRecord{
			ReceivedAt: time.Now().UTC(),
			Event:      &pb.Event{Pid: uint32(index + 1), Comm: "codex", Type: "openat", Path: "/tmp/drain"},
		})
	}
	status, err := store.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status.Active || status.Stopping || status.Pending != 0 {
		t.Fatalf("recording still active after stop: %+v", status)
	}
	if uint64(status.Count)+status.FailedTotal != status.EnqueuedTotal {
		t.Fatalf("accepted records were lost: %+v", status)
	}
	if status.Count == 0 {
		t.Fatalf("no records were persisted: %+v", status)
	}

	records, _, err := ReadCapturedEventsFileAtRoot(root, "events.jsonl", 10000)
	if err != nil {
		t.Fatalf("read recording: %v", err)
	}
	if int64(len(records)) != status.Count {
		t.Fatalf("replayed records = %d, persisted count = %d", len(records), status.Count)
	}
}

func TestEventRecordingConcurrentStopIsLinearizedWithProducers(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	store := NewState()
	if _, err := store.startAtRoot(root, "events.jsonl", true); err != nil {
		t.Fatalf("startAtRoot() error = %v", err)
	}

	start := make(chan struct{})
	var producers sync.WaitGroup
	for producer := 0; producer < 8; producer++ {
		producers.Add(1)
		go func(producer int) {
			defer producers.Done()
			<-start
			for index := 0; index < 500; index++ {
				store.Record(CapturedEventRecord{
					ReceivedAt: time.Now().UTC(),
					Event:      &pb.Event{Pid: uint32(producer*1000 + index + 1), Comm: "codex", Type: "read"},
				})
			}
		}(producer)
	}
	close(start)
	deadline := time.Now().Add(time.Second)
	for store.Status().EnqueuedTotal == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	status, err := store.StopContext(shutdownCtx)
	cancel()
	producers.Wait()
	if err != nil {
		t.Fatalf("StopContext() error = %v", err)
	}
	status = store.Status()
	if status.Active || status.Stopping || status.Pending != 0 {
		t.Fatalf("concurrent stop left work pending: %+v", status)
	}
	if uint64(status.Count)+status.FailedTotal != status.EnqueuedTotal {
		t.Fatalf("concurrent stop lost accepted records: %+v", status)
	}
}

func TestEventRecordingRejectsOversizedRecordWithoutStoppingGeneration(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	store := NewState()
	if _, err := store.startAtRoot(root, "events.jsonl", true); err != nil {
		t.Fatalf("startAtRoot() error = %v", err)
	}
	store.Record(CapturedEventRecord{Event: &pb.Event{
		Pid:       1,
		Comm:      "codex",
		ExtraInfo: strings.Repeat("x", eventRecordingMaxRecordBytes+1),
	}})
	store.Record(CapturedEventRecord{Event: &pb.Event{Pid: 2, Comm: "codex", Type: "read"}})
	status, err := store.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status.FailedTotal != 1 || status.Count != 1 || status.EnqueuedTotal != 2 || status.Pending != 0 {
		t.Fatalf("unexpected oversized-record status: %+v", status)
	}
	if !strings.Contains(status.LastError, errEventRecordingRecordTooLarge.Error()) {
		t.Fatalf("last error = %q, want oversized record error", status.LastError)
	}
}

func TestEventRecordingStopsAtReplayFileLimit(t *testing.T) {
	root := filepath.Join(t.TempDir(), "recordings")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(root, "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if err := file.Truncate(eventReplayMaxFileBytes - 1); err != nil {
		_ = file.Close()
		t.Fatalf("Truncate() error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store := NewState()
	if _, err := store.startAtRoot(root, "events.jsonl", false); err != nil {
		t.Fatalf("startAtRoot() error = %v", err)
	}
	store.Record(CapturedEventRecord{Event: &pb.Event{Pid: 1, Comm: "codex", Type: "read"}})
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		status := store.Status()
		if !status.Active && !status.Stopping {
			break
		}
		time.Sleep(time.Millisecond)
	}
	status, err := store.Stop()
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Stop() error = %v, want file size error", err)
	}
	if status.Active || status.Stopping || status.Count != 0 || status.FailedTotal != 1 || status.Pending != 0 {
		t.Fatalf("unexpected file-limit status: %+v", status)
	}
}

func TestEventRecordingShutdownTimeoutKeepsStoppingGeneration(t *testing.T) {
	store := NewState()
	stopCh := make(chan struct{})
	done := make(chan struct{})
	store.mu.Lock()
	store.started = true
	store.queue = make(chan CapturedEventRecord, 1)
	store.queueCap = 1
	store.stopCh = stopCh
	store.done = done
	store.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := store.StopContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("StopContext() error = %v, want context cancellation", err)
	}
	if status.Active || !status.Stopping {
		t.Fatalf("timed-out shutdown status = %+v", status)
	}
	select {
	case <-stopCh:
	default:
		t.Fatal("stop channel was not closed")
	}
	if _, err := store.StopContext(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("second StopContext() error = %v, want context cancellation", err)
	}
	root := filepath.Join(t.TempDir(), "recordings")
	if _, err := store.startAtRootContext(ctx, root, "replacement.jsonl", true); !errors.Is(err, context.Canceled) {
		t.Fatalf("startAtRootContext() error = %v, want context cancellation", err)
	}
	store.mu.RLock()
	started, currentDone := store.started, store.done
	store.mu.RUnlock()
	if !started || currentDone != done {
		t.Fatal("timed-out generation was replaced")
	}
}
