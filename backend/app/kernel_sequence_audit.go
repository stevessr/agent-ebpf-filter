package app

import (
	"sync"

	"agent-ebpf-filter/core"
)

const auditFlagCPUSequence uint32 = 1 << 1

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
	sequence  uint64
	timestamp uint64
}

type kernelSequenceAuditState struct {
	mu    sync.Mutex
	byCPU map[uint32]kernelSequenceCursor
}

func (s *kernelSequenceAuditState) Observe(raw *core.BpfEvent) (kernelSequenceObservation, uint64) {
	if raw == nil || raw.KernelSequence == 0 || raw.AuditFlags&auditFlagCPUSequence == 0 {
		return kernelSequenceUntracked, 0
	}
	cpu := raw.KernelCPU
	current := kernelSequenceCursor{sequence: raw.KernelSequence, timestamp: raw.KernelTimestampNs}

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
	case current.timestamp > previous.timestamp:
		// Sequence moved backwards while boot-relative time moved forward: the
		// per-CPU map was most likely recreated after tracker reload/re-attach.
		s.byCPU[cpu] = current
		s.mu.Unlock()
		return kernelSequenceReset, 0
	default:
		// Keep the newest cursor; an old/replayed sample must not rewind gap
		// detection for subsequent live events.
		s.mu.Unlock()
		return kernelSequenceOutOfOrder, 0
	}
}

var kernelSequenceAudit kernelSequenceAuditState

func recordKernelSequenceObservation(raw *core.BpfEvent) {
	observation, missing := kernelSequenceAudit.Observe(raw)
	switch observation {
	case kernelSequenceGap:
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_gap_events", 1)
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_missing_events", missing)
	case kernelSequenceReset:
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_resets", 1)
	case kernelSequenceOutOfOrder:
		collectorMetricsStore.RecordAgentSightCounterN("kernel_sequence_out_of_order", 1)
	}
}
