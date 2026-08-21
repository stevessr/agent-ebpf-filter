package tasks

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func startBackendTaskRuntimeForTest(t *testing.T, runtime *Runtime, queueSize int) {
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
	runtime := New("unit", 8, func(entry *Entry) error {
		if payload, ok := entry.Payload().(string); !ok || payload != "payload" {
			t.Fatalf("payload mismatch: %#v", entry.Payload())
		}
		entry.SetProgress(0.5)
		time.Sleep(2 * time.Millisecond)
		close(done)
		return nil
	})
	startBackendTaskRuntimeForTest(t, runtime, 4)

	entry := NewEntry("task-1", "unit", "payload")
	entry.queuedAt = time.Now().UTC().Add(-10 * time.Millisecond)
	if err := runtime.Submit(entry); err != nil {
		t.Fatalf("submit: %v", err)
	}

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("runtime task did not run")
	}
	waitForBackendTaskStatus(t, entry, StatusSucceeded)

	snapshot := entry.Snapshot()
	if snapshot.Status != StatusSucceeded || snapshot.Progress != 1 || snapshot.StartedAt == nil || snapshot.FinishedAt == nil {
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
	runtime := New("unit", 8, nil)
	runtime.queue = make(chan *Entry, 1)
	runtime.started = true

	first := NewEntry("task-1", "unit", nil)
	if err := runtime.Submit(first); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	second := NewEntry("task-2", "unit", nil)
	if err := runtime.Submit(second); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("second submit error = %v, want %v", err, ErrQueueFull)
	}
	if _, ok := runtime.Cancel(first.id); !ok {
		t.Fatal("expected cancel to find first task")
	}
	snapshot := first.Snapshot()
	if snapshot.Status != StatusCanceled || snapshot.Progress != 1 || snapshot.FinishedAt == nil {
		t.Fatalf("canceled snapshot mismatch: %+v", snapshot)
	}
	stats := runtime.Stats()
	if stats.RejectedTotal != 1 || stats.LastRejectReason != "queue_full" {
		t.Fatalf("queue full stats mismatch: %+v", stats)
	}
}

func TestBackendTaskRuntimeRejectsDuplicateIDWithoutLosingOriginal(t *testing.T) {
	runtime := New("unit", 8, nil)
	runtime.queue = make(chan *Entry, 2)
	runtime.started = true

	original := NewEntry("task-1", "unit", "original")
	if err := runtime.Submit(original); err != nil {
		t.Fatalf("submit original: %v", err)
	}
	duplicate := NewEntry("task-1", "unit", "duplicate")
	if err := runtime.Submit(duplicate); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("duplicate submit error = %v, want %v", err, ErrDuplicateID)
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
	runtime := New("unit", 8, func(entry *Entry) error {
		if entry.id == "panic" {
			panic("boom")
		}
		return nil
	})
	startBackendTaskRuntimeForTest(t, runtime, 2)

	panicked := NewEntry("panic", "unit", nil)
	next := NewEntry("next", "unit", nil)
	if err := runtime.Submit(panicked); err != nil {
		t.Fatalf("submit panic task: %v", err)
	}
	if err := runtime.Submit(next); err != nil {
		t.Fatalf("submit next task: %v", err)
	}

	waitForBackendTaskStatus(t, panicked, StatusFailed)
	waitForBackendTaskStatus(t, next, StatusSucceeded)
	if message := panicked.Snapshot().Error; !strings.Contains(message, ErrHandlerPanic.Error()) || !strings.Contains(message, "boom") {
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
	runtime := New("unit", 1, func(entry *Entry) error {
		if entry.id == "first" {
			close(started)
			<-release
		}
		return nil
	})
	startBackendTaskRuntimeForTest(t, runtime, 2)

	first := NewEntry("first", "unit", nil)
	second := NewEntry("second", "unit", nil)
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
	waitForBackendTaskStatus(t, first, StatusSucceeded)
	waitForBackendTaskStatus(t, second, StatusSucceeded)
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
	entry := NewEntry("done", "unit", nil)
	entry.finish(StatusSucceeded, 1, "")
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

func TestBackendTaskRuntimeCompletionHonorsPendingCancel(t *testing.T) {
	runtime := New("unit", 8, nil)
	entry := NewEntry("cancel-race", "unit", nil)
	if !entry.markRunning() {
		t.Fatal("entry did not start")
	}
	runtime.tasks[entry.id] = entry

	entry.Cancel()
	runtime.completeEntry(entry, StatusSucceeded, 1, "", false)

	snapshot := entry.Snapshot()
	if snapshot.Status != StatusCanceled || snapshot.Progress != 1 || snapshot.FinishedAt == nil {
		t.Fatalf("cancel-vs-success completion = %+v", snapshot)
	}
	stats := runtime.Stats()
	if stats.CompletedTotal != 1 || stats.CanceledTotal != 1 || stats.FailedTotal != 0 {
		t.Fatalf("cancel-vs-success stats = %+v", stats)
	}
}

func TestBackendTaskRuntimeCompletionIsAccountedOnce(t *testing.T) {
	runtime := New("unit", 8, nil)
	entry := NewEntry("complete-once", "unit", nil)
	if !entry.markRunning() {
		t.Fatal("entry did not start")
	}
	runtime.tasks[entry.id] = entry

	const attempts = 64
	var callers sync.WaitGroup
	callers.Add(attempts)
	for index := 0; index < attempts; index++ {
		go func() {
			defer callers.Done()
			runtime.completeEntry(entry, StatusSucceeded, 1, "", false)
		}()
	}
	callers.Wait()

	stats := runtime.Stats()
	if stats.CompletedTotal != 1 || stats.FailedTotal != 0 || stats.CanceledTotal != 0 {
		t.Fatalf("duplicate completion stats = %+v", stats)
	}
	if runtime.terminalHead != entry || runtime.terminalTail != entry || entry.terminalNext != nil {
		t.Fatal("terminal retention queue contains duplicate completion entries")
	}
}

func TestBackendTaskRuntimeDoesNotPruneCanceledQueuedEntryBeforeDrain(t *testing.T) {
	runtime := New("unit", 1, nil)
	runtime.queue = make(chan *Entry, 2)
	runtime.started = true

	first := NewEntry("first", "unit", nil)
	if err := runtime.Submit(first); err != nil {
		t.Fatalf("submit first: %v", err)
	}
	first.Cancel()
	second := NewEntry("second", "unit", nil)
	if err := runtime.Submit(second); err != nil {
		t.Fatalf("submit second: %v", err)
	}
	if got, ok := runtime.Get(first.id); !ok || got != first {
		t.Fatal("canceled queued task was pruned before the worker drained it")
	}
	if got := runtime.Stats().TrackedTotal; got != 2 {
		t.Fatalf("tracked tasks before drain = %d, want 2", got)
	}

	runtime.completeEntry(first, StatusCanceled, 1, "", false)
	if _, ok := runtime.Get(first.id); ok {
		t.Fatal("drained terminal task was not pruned")
	}
	if got, ok := runtime.Get(second.id); !ok || got != second {
		t.Fatal("queued nonterminal task was pruned")
	}
}

func TestBackendTaskRuntimeShutdownCancelsAndRejectsTasks(t *testing.T) {
	started := make(chan struct{})
	runtime := New("shutdown", 8, func(entry *Entry) error {
		if entry.id == "running" {
			close(started)
			<-entry.cancel
			return ErrCanceled
		}
		return nil
	})
	runtime.Start(2)
	running := NewEntry("running", "unit", nil)
	queued := NewEntry("queued", "unit", nil)
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
	if running.Snapshot().Status != StatusCanceled || queued.Snapshot().Status != StatusCanceled {
		t.Fatalf("shutdown task statuses = %q/%q", running.Snapshot().Status, queued.Snapshot().Status)
	}
	stats := runtime.Stats()
	if !stats.Closed || stats.Started || stats.CanceledTotal != 2 {
		t.Fatalf("shutdown stats = %+v", stats)
	}
	if err := runtime.Submit(NewEntry("late", "unit", nil)); !errors.Is(err, ErrClosed) {
		t.Fatalf("late submit error = %v, want %v", err, ErrClosed)
	}
	if stats = runtime.Stats(); stats.LastRejectReason != "runtime_closed" || stats.RejectedTotal != 1 {
		t.Fatalf("closed runtime rejection stats = %+v", stats)
	}
	runtime.Start(1)
	if runtime.Stats().Started {
		t.Fatal("closed runtime restarted")
	}
}

func waitForBackendTaskStatus(t *testing.T, entry *Entry, status string) {
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

func BenchmarkBackendTaskRuntimeSteadyStateRetention(b *testing.B) {
	for _, limit := range []int{64, 2048} {
		b.Run(strconv.Itoa(limit), func(b *testing.B) {
			runtime := New("benchmark", limit, nil)
			complete := func(id string) {
				entry := NewEntry(id, "benchmark", nil)
				entry.markRunning()
				runtime.mu.Lock()
				runtime.tasks[id] = entry
				runtime.mu.Unlock()
				runtime.completeEntry(entry, StatusSucceeded, 1, "", false)
			}
			for index := 0; index < limit; index++ {
				complete("warm-" + strconv.Itoa(index))
			}

			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				complete("task-" + strconv.Itoa(iteration))
			}
		})
	}
}
