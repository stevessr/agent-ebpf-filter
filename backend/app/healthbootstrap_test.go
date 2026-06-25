package app

import (
	"agent-ebpf-filter/app/runtime"
	"testing"
)

// Tests for TracepointBootstrapStatus — now delegated to runtime subpackage.

func TestBuildTracepointBootstrapStatus(t *testing.T) {
	// Use the runtime subpackage directly since buildTracepointBootstrapStatus
	// is unexported there. We test through RecordTracepointBootstrapStatus + Snapshot.
	runtime.RecordTracepointBootstrapStatus(5, []string{"syscalls/sys_enter_lstat", "sched/sched_process_free"})
	status := runtime.SnapshotBootstrapTracepointStatus()
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
	original := runtime.SnapshotBootstrapTracepointStatus()

	runtime.RecordTracepointBootstrapStatus(2, []string{"syscalls/sys_enter_execve"})
	snapshot := runtime.SnapshotBootstrapTracepointStatus()
	if snapshot.Status != "partial" {
		t.Fatalf("expected partial status, got %q", snapshot.Status)
	}
	if len(snapshot.SkippedTracepoints) != 1 || snapshot.SkippedTracepoints[0] != "syscalls/sys_enter_execve" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	snapshot.SkippedTracepoints[0] = "mutated"
	again := runtime.SnapshotBootstrapTracepointStatus()
	if again.SkippedTracepoints[0] != "syscalls/sys_enter_execve" {
		t.Fatalf("snapshot should be copied defensively, got %+v", again.SkippedTracepoints)
	}

	// Restore original state
	t.Cleanup(func() {
		runtime.RecordTracepointBootstrapStatus(original.CompiledCount, original.SkippedTracepoints)
	})
}
