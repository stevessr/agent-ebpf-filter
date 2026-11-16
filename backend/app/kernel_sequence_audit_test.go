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

func sequencedRawWithReserveTotal(cpu uint32, seq, ts, generation, reserveTotal uint64) *core.BpfEvent {
	raw := sequencedRaw(cpu, seq, ts, generation)
	raw.AuditFlags |= core.BpfAuditFlagReserveTotal
	raw.KernelReserveFailuresTotal = reserveTotal
	return raw
}

func TestKernelSequenceAuditDetectsGapPerCPU(t *testing.T) {
	var state kernelSequenceAuditState
	if got := state.Observe(sequencedRaw(2, 10, 100, 7)); got.Observation != kernelSequenceFirst {
		t.Fatalf("first observation = %+v", got)
	}
	if got := state.Observe(sequencedRaw(3, 90, 101, 7)); got.Observation != kernelSequenceFirst {
		t.Fatalf("other CPU first observation = %+v", got)
	}
	got := state.Observe(sequencedRaw(2, 14, 110, 7))
	if got.Observation != kernelSequenceGap || got.Missing != 3 {
		t.Fatalf("gap observation = %+v, want gap/3", got)
	}
}

func TestKernelSequenceAuditRecoversReserveDeltaAcrossLostCarrier(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRawWithReserveTotal(2, 10, 100, 7, 4))

	// Imagine an intermediate successful event carried dropped_since_last and
	// was itself lost before userspace observed it. The later event's local
	// dropped_since_last may be zero, but its cumulative reserve total still
	// recovers the kernel-side portion of the sequence gap.
	got := state.Observe(sequencedRawWithReserveTotal(2, 15, 150, 7, 7))
	if got.Observation != kernelSequenceGap || got.Missing != 4 {
		t.Fatalf("gap observation = %+v, want gap/4", got)
	}
	if !got.ReserveFailuresDeltaKnown || got.ReserveFailuresDelta != 3 {
		t.Fatalf("reserve delta = %+v, want known delta 3", got)
	}
}

func TestKernelSequenceAuditContiguousReserveDeltaIsVisibleAsInconsistency(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRawWithReserveTotal(4, 20, 100, 9, 2))
	got := state.Observe(sequencedRawWithReserveTotal(4, 21, 110, 9, 3))
	if got.Observation != kernelSequenceContiguous || !got.ReserveFailuresDeltaKnown || got.ReserveFailuresDelta != 1 {
		t.Fatalf("contiguous reserve delta = %+v", got)
	}
}

func TestKernelSequenceAuditUsesGenerationForReload(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRawWithReserveTotal(1, 50, 1000, 11, 8))
	if got := state.Observe(sequencedRawWithReserveTotal(1, 1, 2000, 12, 8)); got.Observation != kernelSequenceReset || got.ReserveFailuresDeltaKnown {
		t.Fatalf("generation change = %+v, want reset with no cross-generation delta", got)
	}
	if got := state.Observe(sequencedRawWithReserveTotal(1, 1, 3000, 12, 8)); got.Observation != kernelSequenceOutOfOrder {
		t.Fatalf("same-generation duplicate = %+v, want out-of-order", got)
	}
	if got := state.Observe(sequencedRawWithReserveTotal(1, 2, 3100, 12, 8)); got.Observation != kernelSequenceContiguous {
		t.Fatalf("post-reset next = %+v, want contiguous", got)
	}
}

func TestKernelSequenceAuditLegacyResetFallback(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRaw(1, 50, 1000, 0))
	if got := state.Observe(sequencedRaw(1, 1, 2000, 0)); got.Observation != kernelSequenceReset {
		t.Fatalf("legacy forward-time rewind = %+v, want reset", got)
	}
}

func TestKernelSequenceAuditRejectsLatePreviousGeneration(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRawWithReserveTotal(2, 50, 1000, 11, 8))
	if got := state.Observe(sequencedRawWithReserveTotal(2, 1, 2000, 12, 0)); got.Observation != kernelSequenceReset {
		t.Fatalf("new generation = %+v, want reset", got)
	}

	// A buffered event from the prior tracker must not reset the newer
	// cursor, even though generation IDs differ.
	if got := state.Observe(sequencedRawWithReserveTotal(2, 51, 1500, 11, 9)); got.Observation != kernelSequenceOutOfOrder {
		t.Fatalf("stale previous generation = %+v, want out of order", got)
	}
	if got := state.Observe(sequencedRawWithReserveTotal(2, 2, 2100, 12, 0)); got.Observation != kernelSequenceContiguous || !got.ReserveFailuresDeltaKnown || got.ReserveFailuresDelta != 0 {
		t.Fatalf("new generation cursor corrupted by stale event: %+v", got)
	}
}

func TestKernelSequenceAuditPromotesLegacyCursorOnGenerationUpgrade(t *testing.T) {
	var state kernelSequenceAuditState
	state.Observe(sequencedRaw(3, 99, 1000, 0))
	if got := state.Observe(sequencedRaw(3, 1, 2000, 81)); got.Observation != kernelSequenceReset {
		t.Fatalf("legacy-to-explicit generation = %+v, want reset", got)
	}
	if got := state.Observe(sequencedRaw(3, 2, 2100, 81)); got.Observation != kernelSequenceContiguous {
		t.Fatalf("upgraded cursor = %+v, want contiguous", got)
	}
}

func TestKernelSequenceAuditSequenceArithmeticDoesNotWrap(t *testing.T) {
	var state kernelSequenceAuditState
	const maxSequence = ^uint64(0)
	state.Observe(sequencedRaw(4, maxSequence, 1000, 27))
	if got := state.Observe(sequencedRaw(4, 1, 1100, 27)); got.Observation != kernelSequenceOutOfOrder {
		t.Fatalf("wrapped same-generation sequence = %+v, want out of order", got)
	}
	if got := state.Observe(sequencedRaw(4, maxSequence-1, 900, 27)); got.Observation != kernelSequenceOutOfOrder {
		t.Fatalf("stale sequence after max = %+v, want out of order", got)
	}
}

func TestKernelSequenceAuditDoesNotDoubleCountReplayedReserveGap(t *testing.T) {
	before := collectorMetricsStore.Snapshot().AgentSightCountersTotal
	// Use an isolated CPU ID so this global audit cursor cannot collide with
	// runtime and other ordinary per-CPU test samples.
	raw := sequencedRaw(1<<30, 200, 1000, 42)
	raw.AuditFlags |= core.BpfAuditFlagReserveGap
	raw.KernelDroppedSinceLast = 5

	recordKernelSequenceObservation(raw)
	recordKernelSequenceObservation(raw)

	after := collectorMetricsStore.Snapshot().AgentSightCountersTotal
	for _, tc := range []struct {
		name string
		want uint64
	}{
		{"kernel_reported_reserve_gap_events", 1},
		{"kernel_reported_reserve_dropped_events", 5},
		{"kernel_sequence_prebaseline_dropped_events", 5},
		{"kernel_sequence_out_of_order", 1},
	} {
		if delta := after[tc.name] - before[tc.name]; delta != tc.want {
			t.Errorf("%s delta = %d, want %d", tc.name, delta, tc.want)
		}
	}
}
