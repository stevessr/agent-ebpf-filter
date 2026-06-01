package main

import "testing"

func TestBuildTracepointBootstrapStatus(t *testing.T) {
	status := buildTracepointBootstrapStatus(5, []string{"syscalls/sys_enter_lstat", "sched/sched_process_free"})
	if status.Status != "partial" {
		t.Fatalf("expected partial status, got %q", status.Status)
	}
	if status.CompiledCount != 5 || status.AttachedCount != 3 || status.SkippedCount != 2 {
		t.Fatalf("unexpected counts: %+v", status)
	}
	if len(status.SkippedTracepoints) != 2 {
		t.Fatalf("expected 2 skipped tracepoints, got %d", len(status.SkippedTracepoints))
	}
	if status.SkippedTracepoints[0] != "sched/sched_process_free" || status.SkippedTracepoints[1] != "syscalls/sys_enter_lstat" {
		t.Fatalf("expected skipped tracepoints to be sorted, got %+v", status.SkippedTracepoints)
	}
	if status.Message == "" {
		t.Fatal("expected a human-readable status message")
	}
}

func TestRecordTracepointBootstrapStatusCopiesSnapshot(t *testing.T) {
	original := bootstrapTracepointStatusStore.Snapshot()
	t.Cleanup(func() {
		bootstrapTracepointStatusStore.mu.Lock()
		bootstrapTracepointStatusStore.status = original
		bootstrapTracepointStatusStore.mu.Unlock()
	})

	recordTracepointBootstrapStatus(2, []string{"syscalls/sys_enter_execve"})
	snapshot := bootstrapTracepointStatusStore.Snapshot()
	if snapshot.Status != "partial" {
		t.Fatalf("expected partial status, got %q", snapshot.Status)
	}
	if len(snapshot.SkippedTracepoints) != 1 || snapshot.SkippedTracepoints[0] != "syscalls/sys_enter_execve" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	snapshot.SkippedTracepoints[0] = "mutated"
	again := bootstrapTracepointStatusStore.Snapshot()
	if again.SkippedTracepoints[0] != "syscalls/sys_enter_execve" {
		t.Fatalf("snapshot should be copied defensively, got %+v", again.SkippedTracepoints)
	}
}
