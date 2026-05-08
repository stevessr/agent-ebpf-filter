package main

import (
	"strings"
	"testing"
)

func TestBuildKernelEventCopiesDurationNs(t *testing.T) {
	event := bpfEvent{
		PID:        42,
		PPID:       7,
		UID:        1000,
		GID:        1001,
		Type:       25,
		TagID:      0,
		Retval:     -1,
		Extra1:     62,
		Extra2:     9,
		CgroupID:   123456,
		DurationNs: 987654321,
	}

	out := buildKernelEvent(event)
	if out == nil {
		t.Fatal("buildKernelEvent returned nil")
	}
	if out.DurationNs != event.DurationNs {
		t.Fatalf("DurationNs = %d, want %d", out.DurationNs, event.DurationNs)
	}
	if out.Gid != event.GID {
		t.Fatalf("Gid = %d, want %d", out.Gid, event.GID)
	}
	if out.CgroupId != event.CgroupID {
		t.Fatalf("CgroupId = %d, want %d", out.CgroupId, event.CgroupID)
	}
	if !strings.Contains(out.ExtraInfo, "kill(62)") {
		t.Fatalf("ExtraInfo = %q, want syscall name and number", out.ExtraInfo)
	}
}

func TestBuildKernelEventProcessFork(t *testing.T) {
	event := bpfEvent{
		PID:    100,
		PPID:   50,
		Type:   26,
		Extra1: 101,
	}
	copy(event.Path[:], []byte("python3"))

	out := buildKernelEvent(event)
	if out == nil {
		t.Fatal("buildKernelEvent returned nil")
	}
	if out.Type != "process_fork" {
		t.Fatalf("Type = %q, want process_fork", out.Type)
	}
	if !strings.Contains(out.ExtraInfo, "child_pid=101") {
		t.Fatalf("ExtraInfo = %q, want child pid", out.ExtraInfo)
	}
}
