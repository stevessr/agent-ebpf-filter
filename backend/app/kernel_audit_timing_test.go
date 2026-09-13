package app

import (
	"testing"
	"time"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
)

func TestAnnotateKernelAuditTimingEstimatesSyscallStart(t *testing.T) {
	observed := time.Date(2026, 9, 13, 14, 0, 0, 500*int(time.Millisecond), time.UTC)
	raw := &core.BpfEvent{DurationNs: uint64(250 * time.Millisecond)}
	event := &pb.Event{Type: "syscall"}

	annotateKernelAuditTiming(raw, event, observed)

	if event.GetLastSeenMs() != uint64(observed.UnixMilli()) {
		t.Fatalf("last_seen_ms = %d, want %d", event.GetLastSeenMs(), observed.UnixMilli())
	}
	wantStart := uint64(observed.Add(-250 * time.Millisecond).UnixMilli())
	if event.GetFirstSeenMs() != wantStart {
		t.Fatalf("first_seen_ms = %d, want %d", event.GetFirstSeenMs(), wantStart)
	}
}

func TestAnnotateKernelAuditTimingPreservesFlowTimestamps(t *testing.T) {
	raw := &core.BpfEvent{DurationNs: uint64(time.Second)}
	event := &pb.Event{
		Type:        "network_connect",
		FirstSeenMs: 100,
		LastSeenMs:  200,
	}

	annotateKernelAuditTiming(raw, event, time.Now().UTC())

	if event.GetFirstSeenMs() != 100 || event.GetLastSeenMs() != 200 {
		t.Fatalf("flow timestamps were overwritten: first=%d last=%d", event.GetFirstSeenMs(), event.GetLastSeenMs())
	}
}

func TestAnnotateKernelAuditTimingDoesNotInterpretTCPStateAsDuration(t *testing.T) {
	observed := time.Date(2026, 9, 13, 14, 1, 2, 345*int(time.Millisecond), time.UTC)
	// tcp_state_change packs old/new state into DurationNs in the raw ABI.
	raw := &core.BpfEvent{DurationNs: uint64(1)<<32 | 4}
	event := &pb.Event{Type: "tcp_state_change"}

	annotateKernelAuditTiming(raw, event, observed)

	want := uint64(observed.UnixMilli())
	if event.GetFirstSeenMs() != want || event.GetLastSeenMs() != want {
		t.Fatalf("tcp state timing should use observation time only: first=%d last=%d want=%d", event.GetFirstSeenMs(), event.GetLastSeenMs(), want)
	}
}
