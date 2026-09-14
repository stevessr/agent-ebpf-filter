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
	sequence   uint64
	timestamp  uint64
	generation uint64
}

type kernelSequenceAuditState struct {
	mu    sync.Mutex
	byCPU map[uint32]kernelSequenceCursor
}

func (s *kernelSequenceAuditState) Observe(raw *core.BpfEvent) (kernelSequenceObservation, uint64) {
	if raw == nil || raw.KernelSequence == 0 || raw.AuditFlags&core.BpfAuditFlagCPUSequence == 0 {
		return kernelSequenceUntracked, 0
	}
	cpu := raw.KernelCPU
	current := kernelSequenceCursor{
		sequence:   raw.KernelSequence,
		timestamp:  raw.KernelTimestampNs,
		generation: raw.KernelAuditGeneration,
	}

	s.mu.Lock()
	if s.byCPU == nil {
		s.byCPU = make(map[uint32]kernelSequenceCursor)
	}
	previous, ok := s.byCPU[cpu]
	if !ok {
		s.byCPU[cpu] = current
		s.mu.Unlock()
		return kernelSequenceFirst, 0
	}

	// A non-zero kernel audit generation is authoritative. Tracker reloads no
	// longer need to be inferred from a timestamp-forward sequence rewind.
	if current.generation != 0 && previous.generation != 0 && current.generation != previous.generation {
		s.byCPU[cpu] = current
		s.mu.Unlock()
		return kernelSequenceReset, 0
	}

	switch {
	case current.sequence == previous.sequence+1:
		s.byCPU[cpu] = current
		s.mu.Unlock()
		return kernelSequenceContiguous, 0
	case current.sequence > previous.sequence+1:
		missing := current.sequence - previous.sequence - 1
		s.byCPU[cpu] = current
		s.mu.Unlock()
		return kernelSequenceGap, missing
	case current.generation == 0 && previous.generation == 0 && current.timestamp > previous.timestamp:
		// Compatibility path for events captured before explicit audit
		// generations existed.
		s.byCPU[cpu] = current
		s.mu.Unlock()
		return kernelSequenceReset, 0
	default:
		// Same-generation rewinds are stale/replayed samples, even when their
		// timestamp is newer. Never rewind the live cursor.
		s.mu.Unlock()
		return kernelSequenceOutOfOrder, 0
	}
}

var kernelSequenceAudit kernelSequenceAuditState

func recordKernelSequenceObservation(raw *core.BpfEvent) {
	if raw == nil {
		return
	}

	reportedDropped := uint64(0)
	if raw.AuditFlags&core.BpfAuditFlagReserveGap != 0 {
		reportedDropped = raw.KernelDroppedSinceLast
		collectorMetricsStore.RecordAgentSightCounterN("kernel_reported_reserve_gap_events", 1)
		collectorMetricsStore.RecordAgentSightCounterN("kernel_reported_reserve_dropped_events", reportedDropped)
	}

	observation, missing := kernelSequenceAudit.Observe(raw)
	switch observation {
	case kernelSequenceGap:
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_gap_events", 1)
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_missing_events", missing)

		explained := reportedDropped
		if explained > missing {
			explained = missing
		}
		if explained != 0 {
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_ringbuf_explained_missing_events", explained)
		}
		if missing > explained {
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_unexplained_missing_events", missing-explained)
		}
		if reportedDropped != missing {
			collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_kernel_report_mismatch_events", 1)
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
	case kernelSequenceOutOfOrder:
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_out_of_order", 1)
	}
}
