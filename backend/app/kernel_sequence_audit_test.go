package app

import (
	"testing"

	"agent-ebpf-filter/core"
)

func sequencedRaw(cpu uint32, seq, ts uint64) *core.BpfEvent {
	return &core.BpfEvent{KernelCPU: cpu, KernelSequence: seq, KernelTimestampNs: ts, AuditFlags: auditFlagCPUSequence}
}

func TestKernelSequenceAuditDetectsGapPerCPU(t *testing.T) {
	var state kernelSequenceAuditState
	if got, _ := state.Observe(sequencedRaw(2, 10, 100)); got != kernelSequenceFirst {
		t.Fatalf("first observation = %v", got)
	}
	if got, _ := state.Observe(sequencedRaw(3, 90, 101)); got != kernelSequenceFirst {
		t.Fatalf("other CPU first observation = %v", got)
	}
	got, missing := state.Observe(sequencedRaw(2, 14, 110))
	if got != kernelSequenceGap || missing != 3 {
		t.Fatalf("gap observation = %v missing=%d, want gap/3", got, missing)
	}
}

func TestKernelSequenceAuditSeparatesReloadFromOutOfOrder(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRaw(1, 50, 1000))
	if got, _ := state.Observe(sequencedRaw(1, 1, 2000)); got != kernelSequenceReset {
		t.Fatalf("forward-time rewind = %v, want reset", got)
	}
	if got, _ := state.Observe(sequencedRaw(1, 1, 1500)); got != kernelSequenceOutOfOrder {
		t.Fatalf("older duplicate = %v, want out-of-order", got)
	}
	if got, _ := state.Observe(sequencedRaw(1, 2, 2100)); got != kernelSequenceContiguous {
		t.Fatalf("post-reset next = %v, want contiguous", got)
	}
}
