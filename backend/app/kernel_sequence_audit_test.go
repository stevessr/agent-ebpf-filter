package app

import (
	"os"
	"strings"
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

// TestKernelSequenceAuditBeforeReaderFilters prevents a silent regression:
// CPU-local kernel sequence holes must reflect delivered raw records, not
// intentional user-space command/type filtering after ringbuf decode.
func TestKernelSequenceAuditBeforeReaderFilters(t *testing.T) {
	readerSource, err := os.ReadFile("jobs_background.go")
	if err != nil {
		t.Fatal(err)
	}
	reader := string(readerSource)
	start := strings.Index(reader, "func startKernelEventReader(")
	if start < 0 {
		t.Fatal("kernel reader function missing")
	}
	reader = reader[start:]
	audit := strings.Index(reader, "recordKernelSequenceObservation(event)")
	selfFilter := strings.Index(reader, "if event.PID == selfPid")
	commFilter := strings.Index(reader, "if commDisabled(event.Comm[:])")
	typeFilter := strings.Index(reader, "if eventTypeDisabled(event.Type)")
	if audit < 0 || selfFilter <= audit || commFilter <= audit || typeFilter <= audit {
		t.Fatalf("kernel sequence audit must precede user-space admission: audit=%d self=%d comm=%d type=%d",
			audit, selfFilter, commFilter, typeFilter)
	}

	riskSource, err := os.ReadFile("kernelriskbridge.go")
	if err != nil {
		t.Fatal(err)
	}
	// The risk pass runs only for admitted events, so it must not advance
	// the same CPU-local cursor again for a second time.
	start = strings.Index(string(riskSource), "func annotateKernelAuditTiming(")
	end := strings.Index(string(riskSource), "func applyKernelRiskDecision(")
	if start < 0 || end <= start {
		t.Fatal("kernel timing annotation function missing")
	}
	if strings.Contains(string(riskSource)[start:end], "recordKernelSequenceObservation(") {
		t.Fatal("kernel risk/timing path must not double-audit the reader sample")
	}
}
