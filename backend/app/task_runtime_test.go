package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func startBackendTaskRuntimeForTest(t *testing.T, runtime *backendTaskRuntime, queueSize int) {
	t.Helper()
	runtime.Start(queueSize)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := runtime.Shutdown(ctx); err != nil {
			t.Errorf("runtime shutdown: %v", err)
		}
	})
}

func TestBackendTaskRuntimeCompletesAndTracksStats(t *testing.T) {
	done := make(chan struct{})
	runtime := newBackendTaskRuntime("unit", 8, func(entry *backendTaskRuntimeEntry) error {
		if payload, ok := entry.Payload().(string); !ok || payload != "payload" {
			t.Fatalf("payload mismatch: %#v", entry.Payload())
		}
		entry.SetProgress(0.5)
		time.Sleep(2 * time.Millisecond)
		close(done)
		return nil
	})
	startBackendTaskRuntimeForTest(t, runtime, 4)

	entry := newBackendTaskRuntimeEntry("task-1", "unit", "payload")
	entry.queuedAt = time.Now().UTC().Add(-10 * time.Millisecond)
	if err := runtime.Submit(entry); err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime task did not run")
	}
	waitForBackendTaskStatus(t, entry, backendTaskStatusSucceeded)

	snapshot := entry.Snapshot()
	if snapshot.Status != backendTaskStatusSucceeded || snapshot.Progress != 1 || snapshot.StartedAt == nil || snapshot.FinishedAt == nil {
		t.Fatalf("snapshot mismatch: %+v", snapshot)
	}
	if snapshot.QueueLatencyMs <= 0 || snapshot.RunDurationMs <= 0 || snapshot.TotalDurationMs <= 0 {
		t.Fatalf("snapshot duration metrics missing: %+v", snapshot)
	}
	stats := runtime.Stats()
	if stats.EnqueuedTotal != 1 || stats.CompletedTotal != 1 || stats.FailedTotal != 0 || stats.CanceledTotal != 0 {
		t.Fatalf("stats mismatch: %+v", stats)
	}
	if stats.LastQueueLatencyMs <= 0 || stats.LastRunDurationMs <= 0 || stats.LastTotalDurationMs <= 0 || stats.AvgRunDurationMs <= 0 || stats.LastStartedAt == nil || stats.LastFinishedAt == nil {
		t.Fatalf("runtime duration stats missing: %+v", stats)
	}
}

func TestBackendTaskRuntimeQueueFullAndCancel(t *testing.T) {
	runtime := newBackendTaskRuntime("unit", 8, nil)
	runtime.queue = make(chan *backendTaskRuntimeEntry, 1)
	runtime.started = true

	first := newBackendTaskRuntimeEntry("task-1", "unit", nil)
	if err := runtime.Submit(first); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second := newBackendTaskRuntimeEntry("task-2", "unit", nil)
	if err := runtime.Submit(second); !errors.Is(err, errBackendTaskQueueFull) {
		t.Fatalf("second submit error = %v, want %v", err, errBackendTaskQueueFull)
	}
	if _, ok := runtime.Cancel(first.id); !ok {
		t.Fatal("expected cancel to find first task")
	}
	snapshot := first.Snapshot()
	if snapshot.Status != backendTaskStatusCanceled || snapshot.Progress != 1 || snapshot.FinishedAt == nil {
		t.Fatalf("canceled snapshot mismatch: %+v", snapshot)
	}
	stats := runtime.Stats()
	if stats.RejectedTotal != 1 || stats.LastRejectReason != "queue_full" {
		t.Fatalf("queue full stats mismatch: %+v", stats)
	}
}

func TestBackendTaskRuntimeRejectsDuplicateIDWithoutLosingOriginal(t *testing.T) {
	runtime := newBackendTaskRuntime("unit", 8, nil)
	runtime.queue = make(chan *backendTaskRuntimeEntry, 2)
	runtime.started = true

	original := newBackendTaskRuntimeEntry("task-1", "unit", "original")
	if err := runtime.Submit(original); err != nil {
		t.Fatalf("submit original: %v", err)
	}
	duplicate := newBackendTaskRuntimeEntry("task-1", "unit", "duplicate")
	if err := runtime.Submit(duplicate); !errors.Is(err, errBackendTaskDuplicateID) {
		t.Fatalf("duplicate submit error = %v, want %v", err, errBackendTaskDuplicateID)
	}

	got, ok := runtime.Get(original.id)
	if !ok || got != original {
		t.Fatalf("tracked task = %p, %v; want original %p", got, ok, original)
	}
	stats := runtime.Stats()
	if stats.EnqueuedTotal != 1 || stats.RejectedTotal != 1 || stats.LastRejectReason != "duplicate_id" {
		t.Fatalf("duplicate stats mismatch: %+v", stats)
	}
}

func TestBackendTaskRuntimeRecoversHandlerPanicAndContinues(t *testing.T) {
	runtime := newBackendTaskRuntime("unit", 8, func(entry *backendTaskRuntimeEntry) error {
		if entry.id == "panic" {
			panic("boom")
		}
		return nil
	})
	startBackendTaskRuntimeForTest(t, runtime, 2)

	panicked := newBackendTaskRuntimeEntry("panic", "unit", nil)
	next := newBackendTaskRuntimeEntry("next", "unit", nil)
	if err := runtime.Submit(panicked); err != nil {
		t.Fatalf("submit panic task: %v", err)
	}
	if err := runtime.Submit(next); err != nil {
		t.Fatalf("submit next task: %v", err)
	}

	waitForBackendTaskStatus(t, panicked, backendTaskStatusFailed)
	waitForBackendTaskStatus(t, next, backendTaskStatusSucceeded)
	if message := panicked.Snapshot().Error; !strings.Contains(message, errBackendTaskHandlerPanic.Error()) || !strings.Contains(message, "boom") {
		t.Fatalf("panic error = %q", message)
	}
	stats := runtime.Stats()
	if stats.CompletedTotal != 2 || stats.FailedTotal != 1 || stats.PanickedTotal != 1 {
		t.Fatalf("panic stats mismatch: %+v", stats)
	}
}

func TestBackendTaskRuntimeRetentionNeverEvictsActiveTasks(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	runtime := newBackendTaskRuntime("unit", 1, func(entry *backendTaskRuntimeEntry) error {
		if entry.id == "first" {
			close(started)
			<-release
		}
		return nil
	})
	startBackendTaskRuntimeForTest(t, runtime, 2)

	first := newBackendTaskRuntimeEntry("first", "unit", nil)
	second := newBackendTaskRuntimeEntry("second", "unit", nil)
	if err := runtime.Submit(first); err != nil {
		t.Fatalf("submit first: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}
	if err := runtime.Submit(second); err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if _, ok := runtime.Get("first"); !ok {
		t.Fatal("running task was evicted")
	}
	if _, ok := runtime.Get("second"); !ok {
		t.Fatal("queued task was evicted")
	}
	if got := runtime.Stats().TrackedTotal; got != 2 {
		t.Fatalf("tracked active tasks = %d, want 2", got)
	}

	close(release)
	waitForBackendTaskStatus(t, first, backendTaskStatusSucceeded)
	waitForBackendTaskStatus(t, second, backendTaskStatusSucceeded)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && runtime.Stats().TrackedTotal != 1 {
		time.Sleep(time.Millisecond)
	}
	if got := runtime.Stats().TrackedTotal; got != 1 {
		t.Fatalf("tracked terminal history = %d, want 1", got)
	}
	if _, ok := runtime.Get("first"); ok {
		t.Fatal("oldest terminal task was not pruned")
	}
	if got, ok := runtime.Get("second"); !ok || got != second {
		t.Fatal("newest terminal task should remain tracked")
	}
}

func TestBackendTaskRuntimeCancelDoesNotMutateTerminalTask(t *testing.T) {
	entry := newBackendTaskRuntimeEntry("done", "unit", nil)
	entry.finish(backendTaskStatusSucceeded, 1, "")
	before := entry.Snapshot()
	entry.Cancel()
	after := entry.Snapshot()

	if entry.IsCanceled() {
		t.Fatal("terminal task cancellation channel should remain open")
	}
	if after.Status != before.Status || after.FinishedAt == nil || !after.FinishedAt.Equal(*before.FinishedAt) {
		t.Fatalf("terminal task changed after cancel: before=%+v after=%+v", before, after)
	}
}

func TestBackendTaskRuntimeShutdownCancelsAndRejectsTasks(t *testing.T) {
	started := make(chan struct{})
	runtime := newBackendTaskRuntime("shutdown", 8, func(entry *backendTaskRuntimeEntry) error {
		if entry.id == "running" {
			close(started)
			<-entry.cancel
			return errBackendTaskCanceled
		}
		return nil
	})
	runtime.Start(2)
	running := newBackendTaskRuntimeEntry("running", "unit", nil)
	queued := newBackendTaskRuntimeEntry("queued", "unit", nil)
	if err := runtime.Submit(running); err != nil {
		t.Fatalf("submit running task: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("running task did not start")
	}
	if err := runtime.Submit(queued); err != nil {
		t.Fatalf("submit queued task: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := runtime.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if running.Snapshot().Status != backendTaskStatusCanceled || queued.Snapshot().Status != backendTaskStatusCanceled {
		t.Fatalf("shutdown task statuses = %q/%q", running.Snapshot().Status, queued.Snapshot().Status)
	}
	stats := runtime.Stats()
	if !stats.Closed || stats.Started || stats.CanceledTotal != 2 {
		t.Fatalf("shutdown stats = %+v", stats)
	}
	if err := runtime.Submit(newBackendTaskRuntimeEntry("late", "unit", nil)); !errors.Is(err, errBackendTaskRuntimeClosed) {
		t.Fatalf("late submit error = %v, want %v", err, errBackendTaskRuntimeClosed)
	}
	if stats = runtime.Stats(); stats.LastRejectReason != "runtime_closed" || stats.RejectedTotal != 1 {
		t.Fatalf("closed runtime rejection stats = %+v", stats)
	}
	runtime.Start(1)
	if runtime.Stats().Started {
		t.Fatal("closed runtime restarted")
	}
}

func waitForBackendTaskStatus(t *testing.T, entry *backendTaskRuntimeEntry, status string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if entry.Snapshot().Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("task status = %q, want %q", entry.Snapshot().Status, status)
}
