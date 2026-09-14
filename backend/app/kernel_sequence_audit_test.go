package app

import (
	"testing"

	"agent-ebpf-filter/core"
)

func sequencedRaw(cpu uint32, seq, ts, generation uint64) *core.BpfEvent {
	flags := core.BpfAuditFlagCPUSequence
	if generation != 0 {
		flags |= core.BpfAuditFlagGeneration
	}
	return &core.BpfEvent{
		KernelCPU: cpu, KernelSequence: seq, KernelTimestampNs: ts,
		KernelAuditGeneration: generation, AuditFlags: flags,
	}
}

func TestKernelSequenceAuditDetectsGapPerCPU(t *testing.T) {
	var state kernelSequenceAuditState
	if got, _ := state.Observe(sequencedRaw(2, 10, 100, 7)); got != kernelSequenceFirst {
		t.Fatalf("first observation = %v", got)
	}
	if got, _ := state.Observe(sequencedRaw(3, 90, 101, 7)); got != kernelSequenceFirst {
		t.Fatalf("other CPU first observation = %v", got)
	}
	got, missing := state.Observe(sequencedRaw(2, 14, 110, 7))
	if got != kernelSequenceGap || missing != 3 {
		t.Fatalf("gap observation = %v missing=%d, want gap/3", got, missing)
	}
}

func TestKernelSequenceAuditUsesGenerationForReload(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRaw(1, 50, 1000, 11))
	if got, _ := state.Observe(sequencedRaw(1, 1, 2000, 12)); got != kernelSequenceReset {
		t.Fatalf("generation change = %v, want reset", got)
	}
	if got, _ := state.Observe(sequencedRaw(1, 1, 3000, 12)); got != kernelSequenceOutOfOrder {
		t.Fatalf("same-generation duplicate = %v, want out-of-order", got)
	}
	if got, _ := state.Observe(sequencedRaw(1, 2, 3100, 12)); got != kernelSequenceContiguous {
		t.Fatalf("post-reset next = %v, want contiguous", got)
	}
}

func TestKernelSequenceAuditLegacyResetFallback(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRaw(1, 50, 1000, 0))
	if got, _ := state.Observe(sequencedRaw(1, 1, 2000, 0)); got != kernelSequenceReset {
		t.Fatalf("legacy forward-time rewind = %v, want reset", got)
	}
}
