package app

import (
	"errors"
	"testing"
	"time"
)

func TestBackendTaskRuntimeCompletesAndTracksStats(t *testing.T) {
	done := make(chan struct{})
	runtime := newBackendTaskRuntime("unit", 8, func(entry *backendTaskRuntimeEntry) error {
		if payload, ok := entry.Payload().(string); !ok || payload != "payload" {
			t.Fatalf("payload mismatch: %#v", entry.Payload())
		}
		entry.SetProgress(0.5)
		close(done)
		return nil
	})
	runtime.Start(4)

	entry := newBackendTaskRuntimeEntry("task-1", "unit", "payload")
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
	stats := runtime.Stats()
	if stats.EnqueuedTotal != 1 || stats.CompletedTotal != 1 || stats.FailedTotal != 0 || stats.CanceledTotal != 0 {
		t.Fatalf("stats mismatch: %+v", stats)
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
