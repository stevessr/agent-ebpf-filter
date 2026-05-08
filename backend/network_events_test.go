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
		Type:       25,
		TagID:      0,
		Retval:     -1,
		Extra1:     62,
		Extra2:     9,
		DurationNs: 987654321,
	}

	out := buildKernelEvent(event)
	if out == nil {
		t.Fatal("buildKernelEvent returned nil")
	}
	if out.DurationNs != event.DurationNs {
		t.Fatalf("DurationNs = %d, want %d", out.DurationNs, event.DurationNs)
	}
	if !strings.Contains(out.ExtraInfo, "kill(62)") {
		t.Fatalf("ExtraInfo = %q, want syscall name and number", out.ExtraInfo)
	}
}
