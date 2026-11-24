package app

import (
	"sync"

	"agent-ebpf-filter/core"
)

type kernelSequenceObservation uint8

const (
	kernelSequenceUntracked kernelSequenceObservation = iota
	kernelSequenceFirst
	kernelSequenceContiguous
	kernelSequenceGap
	kernelSequenceReset
	kernelSequenceOutOfOrder
)

type kernelSequenceCursor struct {
	sequence             uint64
	timestamp            uint64
	generation           uint64
	reserveFailuresTotal uint64
	reserveTotalValid    bool
}

type kernelSequenceResult struct {
	Observation               kernelSequenceObservation
	Missing                   uint64
	ReserveFailuresDelta      uint64
	ReserveFailuresDeltaKnown bool
}

type kernelSequenceAuditState struct {
	mu    sync.Mutex
	byCPU map[uint32]kernelSequenceCursor
}

func (s *kernelSequenceAuditState) Observe(raw *core.BpfEvent) kernelSequenceResult {
	if raw == nil || raw.KernelSequence == 0 || raw.AuditFlags&core.BpfAuditFlagCPUSequence == 0 {
		return kernelSequenceResult{Observation: kernelSequenceUntracked}
	}
	cpu := raw.KernelCPU
	current := kernelSequenceCursor{
		sequence:             raw.KernelSequence,
		timestamp:            raw.KernelTimestampNs,
		generation:           raw.KernelAuditGeneration,
		reserveFailuresTotal: raw.KernelReserveFailuresTotal,
		reserveTotalValid:    raw.AuditFlags&core.BpfAuditFlagReserveTotal != 0,
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byCPU == nil {
		s.byCPU = make(map[uint32]kernelSequenceCursor)
	}
	previous, ok := s.byCPU[cpu]
	if !ok {
		s.byCPU[cpu] = current
		return kernelSequenceResult{Observation: kernelSequenceFirst}
	}

	// Explicit generations identify reloads, including upgrades from legacy
	// zero-generation events. Reject late records from a previous generation:
	// generation IDs are random, so time (not numeric ID order) establishes
	// whether the new sample actually belongs after the live cursor.
	if current.generation != 0 && current.generation != previous.generation {
		if current.timestamp != 0 && previous.timestamp != 0 && current.timestamp <= previous.timestamp {
			return kernelSequenceResult{Observation: kernelSequenceOutOfOrder}
		}
		s.byCPU[cpu] = current
		return kernelSequenceResult{Observation: kernelSequenceReset}
	}

	reserveDelta, reserveDeltaKnown := uint64(0), false
	if current.reserveTotalValid && previous.reserveTotalValid && current.reserveFailuresTotal >= previous.reserveFailuresTotal {
		reserveDelta = current.reserveFailuresTotal - previous.reserveFailuresTotal
		reserveDeltaKnown = true
	}

	switch {
	case current.sequence > previous.sequence && current.sequence-previous.sequence == 1:
		s.byCPU[cpu] = current
		return kernelSequenceResult{
			Observation:               kernelSequenceContiguous,
			ReserveFailuresDelta:      reserveDelta,
			ReserveFailuresDeltaKnown: reserveDeltaKnown,
		}
	case current.sequence > previous.sequence:
		missing := current.sequence - previous.sequence - 1
		s.byCPU[cpu] = current
		return kernelSequenceResult{
			Observation:               kernelSequenceGap,
			Missing:                   missing,
			ReserveFailuresDelta:      reserveDelta,
			ReserveFailuresDeltaKnown: reserveDeltaKnown,
		}
	case current.generation == 0 && previous.generation == 0 && current.timestamp > previous.timestamp:
		// Compatibility path for events captured before explicit audit
		// generations existed.
		s.byCPU[cpu] = current
		return kernelSequenceResult{Observation: kernelSequenceReset}
	default:
		// Same-generation rewinds are stale/replayed samples, even when their
		// timestamp is newer. Never rewind the live cursor or its cumulative
		// reserve-failure baseline.
		return kernelSequenceResult{Observation: kernelSequenceOutOfOrder}
	}
}

var kernelSequenceAudit kernelSequenceAuditState

func recordKernelSequenceObservation(raw *core.BpfEvent) {
	if raw == nil {
		return
	}

	result := kernelSequenceAudit.Observe(raw)
	if result.Observation == kernelSequenceOutOfOrder {
		// A replayed sample may carry the same pending-drop count as the
		// original. Never count it twice as fresh kernel ring-buffer loss.
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_out_of_order", 1)
		return
	}

	reportedDropped := uint64(0)
	if raw.AuditFlags&core.BpfAuditFlagReserveGap != 0 {
		reportedDropped = raw.KernelDroppedSinceLast
		collectorMetricsStore.RecordAgentSightCounterN("kernel_reported_reserve_gap_events", 1)
		collectorMetricsStore.RecordAgentSightCounterN("kernel_reported_reserve_dropped_events", reportedDropped)
	}

	switch result.Observation {
	case kernelSequenceGap:
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_gap_events", 1)
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_missing_events", result.Missing)

		// Prefer the cumulative reserve-failure delta when both visible events
		// carry it. Unlike dropped_since_last, this survives losing the successful
		// event that originally acknowledged/reset the kernel pending-drop count.
		explained := reportedDropped
		if result.ReserveFailuresDeltaKnown {
			explained = result.ReserveFailuresDelta
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_reserve_delta_samples", 1)
			if explained > reportedDropped {
				collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_reserve_delta_recovered_events", explained-reportedDropped)
			}
		}
		if explained > result.Missing {
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_reserve_delta_exceeds_gap_events", 1)
			explained = result.Missing
		}
		if explained != 0 {
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_ringbuf_explained_missing_events", explained)
		}
		if result.Missing > explained {
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_unexplained_missing_events", result.Missing-explained)
		}
	case kernelSequenceContiguous:
		if result.ReserveFailuresDeltaKnown && result.ReserveFailuresDelta != 0 {
			// With attempt sequence allocated before reserve, any reserve failure
			// must consume a sequence number. A reserve delta without a sequence
			// hole therefore signals an ABI/state inconsistency worth surfacing.
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_reserve_without_gap_events", 1)
		}
	case kernelSequenceFirst:
		if reportedDropped != 0 {
			// Loss may have happened before userspace established its first
			// cursor. Preserve it instead of silently hiding the pre-baseline
			// pressure behind a "first sample" classification.
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_prebaseline_dropped_events", reportedDropped)
		}
	case kernelSequenceReset:
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_resets", 1)
	}
}
