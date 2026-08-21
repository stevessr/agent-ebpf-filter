package app

import (
	"agent-ebpf-filter/app/recording"

	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestRuntimeEventLogWriterFlushesAcceptedRecordsInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := startRuntimeEventLogWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = writer.StopContext(ctx)
	})

	const recordCount = 300
	for pid := 1; pid <= recordCount; pid++ {
		accepted, err := writer.Enqueue(CapturedEventRecord{
			ReceivedAt: time.Unix(int64(pid), 0).UTC(),
			Event:      &pb.Event{Pid: uint32(pid), Type: "openat", Path: "/tmp/test"},
		})
		if err != nil || !accepted {
			t.Fatalf("Enqueue(%d) = %t, %v", pid, accepted, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writer.FlushContext(ctx); err != nil {
		t.Fatalf("FlushContext() error = %v", err)
	}
	status := writer.Status()
	if status.EnqueuedTotal != recordCount || status.PersistedTotal != recordCount || status.Pending != 0 || status.FailedTotal != 0 || status.DroppedTotal != 0 {
		t.Fatalf("unexpected writer status %+v", status)
	}
	records, err := tailCapturedEventsFileAtRootContext(ctx, filepath.Dir(path), filepath.Base(path), 5)
	if err != nil {
		t.Fatalf("tailCapturedEventsFileContext() error = %v", err)
	}
	if got := replayRecordPIDs(records); len(got) != 5 || got[0] != 296 || got[4] != 300 {
		t.Fatalf("tail PIDs = %v, want [296 297 298 299 300]", got)
	}
}

func TestRuntimeEventLogWriterStopDrainsAcceptedRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := startRuntimeEventLogWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	for pid := 1; pid <= 50; pid++ {
		if accepted, err := writer.Enqueue(CapturedEventRecord{Event: &pb.Event{Pid: uint32(pid), Type: "read"}}); err != nil || !accepted {
			t.Fatalf("Enqueue(%d) = %t, %v", pid, accepted, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := writer.StopContext(ctx); err != nil {
		t.Fatalf("StopContext() error = %v", err)
	}
	status := writer.Status()
	if status.Active || status.Stopping || status.PersistedTotal != 50 || status.Pending != 0 {
		t.Fatalf("unexpected stopped writer status %+v", status)
	}
	records, err := tailCapturedEventsFileAtRootContext(ctx, filepath.Dir(path), filepath.Base(path), 100)
	if err != nil || len(records) != 50 {
		t.Fatalf("drained tail = %d records, %v", len(records), err)
	}
}

func TestRuntimeEventLogWriterQueueFullAndCanceledBarrier(t *testing.T) {
	writer := &runtimeEventLogWriter{
		queue:     make(chan runtimeEventLogItem, 1),
		stopCh:    make(chan struct{}),
		done:      make(chan struct{}),
		accepting: true,
	}
	record := CapturedEventRecord{Event: &pb.Event{Pid: 1, Type: "read"}}
	if accepted, err := writer.Enqueue(record); err != nil || !accepted {
		t.Fatalf("first Enqueue() = %t, %v", accepted, err)
	}
	if accepted, err := writer.Enqueue(record); accepted || !errors.Is(err, errRuntimeEventLogQueueFull) {
		t.Fatalf("full Enqueue() = %t, %v", accepted, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	flushDone := make(chan error, 1)
	go func() {
		flushDone <- writer.FlushContext(ctx)
	}()
	// A full queue must not make FlushContext hold the status/lifecycle lock.
	statusDone := make(chan runtimeEventLogStatus, 1)
	go func() {
		statusDone <- writer.Status()
	}()
	select {
	case <-statusDone:
	case <-time.After(250 * time.Millisecond):
		cancel()
		t.Fatal("Status() blocked behind a full flush barrier")
	}
	cancel()
	if err := <-flushDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("blocked FlushContext() error = %v, want context canceled", err)
	}
	status := writer.Status()
	if status.QueueLen != 1 || status.QueueCap != 1 || status.EnqueuedTotal != 1 || status.DroppedTotal != 1 || status.Pending != 1 {
		t.Fatalf("unexpected full writer status %+v", status)
	}
}

func TestRuntimeEventLogWriterReservesCapacityForFlushBarrier(t *testing.T) {
	writer := &runtimeEventLogWriter{
		queue:         make(chan runtimeEventLogItem, 2),
		stopCh:        make(chan struct{}),
		done:          make(chan struct{}),
		eventQueueCap: 1,
		accepting:     true,
	}
	record := CapturedEventRecord{Event: &pb.Event{Pid: 1, Type: "read"}}
	if accepted, err := writer.Enqueue(record); err != nil || !accepted {
		t.Fatalf("Enqueue() = %t, %v", accepted, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	flushDone := make(chan error, 1)
	go func() {
		flushDone <- writer.FlushContext(ctx)
	}()
	deadline := time.Now().Add(time.Second)
	for len(writer.queue) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(writer.queue) != 2 {
		cancel()
		t.Fatal("flush barrier did not use the reserved control slot")
	}
	status := writer.Status()
	if status.QueueLen != 1 || status.QueueCap != 1 {
		cancel()
		t.Fatalf("reserved barrier leaked into event queue status: %+v", status)
	}
	cancel()
	if err := <-flushDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("FlushContext() error = %v, want context canceled", err)
	}
	writer.mu.Lock()
	waiters := writer.flushWaiters
	writer.mu.Unlock()
	if waiters != 0 {
		t.Fatalf("flush waiters = %d, want 0", waiters)
	}
}

func TestRuntimeEventLogWriterContainsRecordAndTerminalFailures(t *testing.T) {
	t.Run("record", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "events-*.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		writer, err := startRuntimeEventLogWriter(file)
		if err != nil {
			t.Fatal(err)
		}
		if accepted, err := writer.Enqueue(CapturedEventRecord{Event: &pb.Event{Pid: 1, Type: "read", RiskScore: math.NaN()}}); err != nil || !accepted {
			t.Fatalf("Enqueue() = %t, %v", accepted, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := writer.FlushContext(ctx); err != nil {
			t.Fatalf("FlushContext() error = %v", err)
		}
		if status := writer.Status(); status.FailedTotal != 1 || status.PersistedTotal != 0 || status.Pending != 0 {
			t.Fatalf("unexpected record-failure status %+v", status)
		}
		if err := writer.StopContext(ctx); err != nil {
			t.Fatalf("StopContext() error = %v", err)
		}
	})

	t.Run("terminal", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "events-*.jsonl")
		if err != nil {
			t.Fatal(err)
		}
		writer, err := startRuntimeEventLogWriter(file)
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if accepted, err := writer.Enqueue(CapturedEventRecord{Event: &pb.Event{Pid: 1, Type: "read"}}); err != nil || !accepted {
			t.Fatalf("Enqueue() = %t, %v", accepted, err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := writer.FlushContext(ctx); err == nil {
			t.Fatal("FlushContext() succeeded after the file was closed")
		}
		if err := writer.StopContext(ctx); err == nil {
			t.Fatal("StopContext() omitted the terminal write error")
		}
		status := writer.Status()
		if status.Active || status.FailedTotal != 1 || status.LastError == "" {
			t.Fatalf("unexpected terminal-failure status %+v", status)
		}
	})
}

func TestRuntimeStateRecentEventsFlushesWriterAndHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := startRuntimeEventLogWriter(file)
	if err != nil {
		t.Fatal(err)
	}
	state := &runtimeState{
		settings:  RuntimeSettings{LogPersistenceEnabled: true, LogFilePath: path},
		logWriter: writer,
		logRoot:   filepath.Dir(path),
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.Shutdown(ctx)
	})
	for pid := 1; pid <= 3; pid++ {
		if accepted, err := state.enqueueEvent(CapturedEventRecord{Event: &pb.Event{Pid: uint32(pid), Type: "read"}}); err != nil || !accepted {
			t.Fatalf("enqueueEvent(%d) = %t, %v", pid, accepted, err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	records, source, err := state.RecentEventsContext(ctx, 2)
	if err != nil || source != "file" {
		t.Fatalf("RecentEventsContext() source=%q error=%v", source, err)
	}
	if got := replayRecordPIDs(records); len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Fatalf("recent PIDs = %v, want [2 3]", got)
	}
	canceled, cancelNow := context.WithCancel(context.Background())
	cancelNow()
	if _, _, err := state.RecentEventsContext(canceled, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled RecentEventsContext() error = %v", err)
	}
}

func TestRuntimeStateLogReconfigurationDrainsAndTruncatesGenerations(t *testing.T) {
	root := t.TempDir()
	state := &runtimeState{logRoot: root}
	configure := func(enabled bool, path string) {
		t.Helper()
		state.mu.Lock()
		state.settings.LogPersistenceEnabled = enabled
		state.settings.LogFilePath = path
		err := state.applyLoggingLocked()
		state.mu.Unlock()
		if err != nil {
			t.Fatalf("applyLoggingLocked(%t, %q) error = %v", enabled, path, err)
		}
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = state.Shutdown(ctx)
	})

	configure(true, "first.jsonl")
	firstWriter := state.logWriter
	configure(true, "first.jsonl")
	if state.logWriter != firstWriter {
		t.Fatal("unchanged logging configuration restarted the writer")
	}
	if accepted, err := state.enqueueEvent(CapturedEventRecord{Event: &pb.Event{Pid: 1, Type: "read"}}); err != nil || !accepted {
		t.Fatalf("first enqueueEvent() = %t, %v", accepted, err)
	}
	configure(true, "second.jsonl")
	if accepted, err := state.enqueueEvent(CapturedEventRecord{Event: &pb.Event{Pid: 2, Type: "read"}}); err != nil || !accepted {
		t.Fatalf("second enqueueEvent() = %t, %v", accepted, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := state.FlushEventLogContext(ctx); err != nil {
		t.Fatalf("FlushEventLogContext() error = %v", err)
	}
	first, err := tailCapturedEventsFileAtRootContext(ctx, root, "first.jsonl", 10)
	if err != nil || len(first) != 1 || first[0].Event.GetPid() != 1 {
		t.Fatalf("first generation = %v, %v", replayRecordPIDs(first), err)
	}
	second, err := tailCapturedEventsFileAtRootContext(ctx, root, "second.jsonl", 10)
	if err != nil || len(second) != 1 || second[0].Event.GetPid() != 2 {
		t.Fatalf("second generation = %v, %v", replayRecordPIDs(second), err)
	}

	if err := state.TruncateEventLog(); err != nil {
		t.Fatalf("TruncateEventLog() error = %v", err)
	}
	second, err = tailCapturedEventsFileAtRootContext(ctx, root, "second.jsonl", 10)
	if err != nil || len(second) != 0 {
		t.Fatalf("truncated generation = %v, %v", replayRecordPIDs(second), err)
	}
	configure(false, "second.jsonl")
	if accepted, err := state.enqueueEvent(CapturedEventRecord{Event: &pb.Event{Pid: 3, Type: "read"}}); err != nil || accepted {
		t.Fatalf("disabled enqueueEvent() = %t, %v", accepted, err)
	}
	if status := state.EventLogStatus(); status.Active || status.QueueCap != 0 {
		t.Fatalf("disabled event log status = %+v", status)
	}
}

func replayRecordPIDs(records []recording.CapturedEventRecord) []uint32 {
	pids := make([]uint32, 0, len(records))
	for _, record := range records {
		pids = append(pids, record.Event.GetPid())
	}
	return pids
}
