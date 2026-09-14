package app

import (
	"testing"

	bpf "agent-ebpf-filter/ebpf"
)

func TestApplyKernelAuditGenerationResetsOnlyContinuityState(t *testing.T) {
	values := []bpf.AgentTrackerCollectorStats{
		{RingbufEventsTotal: 10, RingbufReserveFailedTotal: 3, EventSequence: 11, PendingDroppedEvents: 2, AuditGeneration: 7},
		{RingbufEventsTotal: 20, RingbufReserveFailedTotal: 4, EventSequence: 22, PendingDroppedEvents: 1, AuditGeneration: 7},
	}
	applyKernelAuditGeneration(values, 99)
	for i, value := range values {
		if value.EventSequence != 0 || value.PendingDroppedEvents != 0 || value.AuditGeneration != 99 {
			t.Fatalf("slot %d continuity state = %+v", i, value)
		}
	}
	if values[0].RingbufEventsTotal != 10 || values[0].RingbufReserveFailedTotal != 3 || values[1].RingbufEventsTotal != 20 || values[1].RingbufReserveFailedTotal != 4 {
		t.Fatalf("cumulative counters were not preserved: %+v", values)
	}
}
