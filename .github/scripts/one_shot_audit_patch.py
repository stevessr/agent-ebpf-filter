from pathlib import Path
import re


def read(path):
    return Path(path).read_text()


def write(path, text):
    Path(path).write_text(text)


def replace_once(path, old, new):
    text = read(path)
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one exact match, got {count}\n--- OLD ---\n{old[:500]}")
    write(path, text.replace(old, new, 1))


def sub_once(path, pattern, repl):
    text = read(path)
    new, count = re.subn(pattern, repl, text, count=1, flags=re.S)
    if count != 1:
        raise SystemExit(f"{path}: expected one regex match, got {count}: {pattern}")
    write(path, new)


# Raw eBPF tracker ABI: append-only audit provenance.
replace_once(
    "backend/ebpf/agent_tracker_common.h",
    """    u64 extra3;\n    char extra4[MAX_PATH_LEN];\n};\n""",
    """    u64 extra3;\n    char extra4[MAX_PATH_LEN];\n    // Append-only audit provenance. Keep legacy field offsets stable.\n    u64 kernel_timestamp_ns; // bpf_ktime_get_ns(), CLOCK_MONOTONIC domain\n    u64 kernel_sequence;     // CPU-local monotonic sequence\n    u32 kernel_cpu;\n    u32 audit_flags;\n};\n""",
)
replace_once(
    "backend/ebpf/agent_tracker_common.h",
    """struct collector_stats {\n    u64 ringbuf_events_total;\n    u64 ringbuf_reserve_failed_total;\n};\n""",
    """struct collector_stats {\n    u64 ringbuf_events_total;\n    u64 ringbuf_reserve_failed_total;\n    u64 event_sequence; // per-CPU, used with kernel_cpu as an audit ordering tuple\n};\n""",
)
replace_once(
    "backend/ebpf/agent_tracker_common.h",
    """static __always_inline struct event *reserve_event(void) {\n    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);\n    if (!e) {\n        account_ringbuf_reserve_failed();\n    }\n    return e;\n}\n""",
    """#define AUDIT_FLAG_KERNEL_TIMESTAMP (1U << 0)\n#define AUDIT_FLAG_CPU_SEQUENCE     (1U << 1)\n\nstatic __always_inline struct event *reserve_event(void) {\n    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);\n    if (!e) {\n        account_ringbuf_reserve_failed();\n        return 0;\n    }\n\n    // Capture provenance immediately after reserve so time includes as little\n    // probe-side formatting work as possible. Sequence is deliberately per-CPU\n    // to avoid a cross-CPU atomic hotspot on every observed syscall.\n    e->kernel_timestamp_ns = bpf_ktime_get_ns();\n    e->kernel_cpu = bpf_get_smp_processor_id();\n    e->kernel_sequence = 0;\n    e->audit_flags = AUDIT_FLAG_KERNEL_TIMESTAMP;\n\n    u32 key = 0;\n    struct collector_stats *stats = bpf_map_lookup_elem(&collector_stats, &key);\n    if (stats) {\n        stats->event_sequence++;\n        e->kernel_sequence = stats->event_sequence;\n        e->audit_flags |= AUDIT_FLAG_CPU_SEQUENCE;\n    }\n    return e;\n}\n""",
)

# Go mirror of the raw ABI.
replace_once(
    "backend/core/types.go",
    """\tExtra3                                 uint64\n\tExtra4                                 [256]byte\n}\n""",
    """\tExtra3                                 uint64\n\tExtra4                                 [256]byte\n\tKernelTimestampNs                      uint64\n\tKernelSequence                         uint64\n\tKernelCPU                              uint32\n\tAuditFlags                             uint32\n}\n""",
)

# Public schema.
replace_once(
    "proto/tracker_events.proto",
    """  string redaction_level = 69;\n  repeated string sanitized_fields = 70;\n}\n""",
    """  string redaction_level = 69;\n  repeated string sanitized_fields = 70;\n  uint64 kernel_timestamp_ns = 71;\n  uint64 kernel_sequence = 72;\n  uint32 kernel_cpu = 73;\n  string kernel_clock = 74;\n  uint64 ingest_timestamp_ns = 75;\n  uint64 capture_delay_ns = 76;\n  uint64 capture_timestamp_ns = 77;\n  uint32 audit_flags = 78;\n}\n""",
)
replace_once(
    "proto/tracker_events.proto",
    """    AgentSightAlertEvent agentsight_alert_event = 41;\n  }\n}\n""",
    """    AgentSightAlertEvent agentsight_alert_event = 41;\n  }\n  uint64 kernel_timestamp_ns = 42;\n  uint64 kernel_sequence = 43;\n  uint32 kernel_cpu = 44;\n  string kernel_clock = 45;\n  uint64 ingest_timestamp_ns = 46;\n  uint64 capture_delay_ns = 47;\n  uint64 capture_timestamp_ns = 48;\n  uint32 audit_flags = 49;\n}\n""",
)

Path("backend/app/kernel_audit_clock.go").write_text(r'''package app

import (
    "time"

    "golang.org/x/sys/unix"
)

type kernelCaptureObservation struct {
    CapturedAt time.Time
    IngestedAt time.Time
    DelayNS    uint64
    Clock      string
}

// alignKernelKtime is pure so audit clock behavior is deterministic in tests.
// bpf_ktime_get_ns() and CLOCK_MONOTONIC share the same boot-relative domain.
func alignKernelKtime(rawNS uint64, observedAt time.Time, monotonicNowNS uint64) kernelCaptureObservation {
    if observedAt.IsZero() {
        observedAt = time.Now().UTC()
    } else {
        observedAt = observedAt.UTC()
    }
    out := kernelCaptureObservation{
        CapturedAt: observedAt,
        IngestedAt: observedAt,
        Clock:      "unknown",
    }
    if rawNS == 0 || monotonicNowNS == 0 || rawNS > monotonicNowNS {
        return out
    }

    delta := monotonicNowNS - rawNS
    if delta > uint64(^uint64(0)>>1) {
        return out
    }
    out.DelayNS = delta
    out.CapturedAt = observedAt.Add(-time.Duration(delta))
    out.Clock = "monotonic"
    return out
}

func observeKernelKtime(rawNS uint64, observedAt time.Time) kernelCaptureObservation {
    if observedAt.IsZero() {
        observedAt = time.Now().UTC()
    } else {
        observedAt = observedAt.UTC()
    }
    var mono unix.Timespec
    if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &mono); err != nil || mono.Sec < 0 || mono.Nsec < 0 {
        return alignKernelKtime(rawNS, observedAt, 0)
    }
    monoNS := uint64(mono.Sec)*uint64(time.Second) + uint64(mono.Nsec)
    return alignKernelKtime(rawNS, observedAt, monoNS)
}
''')

Path("backend/app/kernel_audit_clock_test.go").write_text(r'''package app

import (
    "testing"
    "time"
)

func TestAlignKernelKtime(t *testing.T) {
    observed := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
    got := alignKernelKtime(9_500_000_000, observed, 10_000_000_000)
    if got.Clock != "monotonic" {
        t.Fatalf("clock = %q, want monotonic", got.Clock)
    }
    if got.DelayNS != uint64(500*time.Millisecond) {
        t.Fatalf("delay = %d, want %d", got.DelayNS, 500*time.Millisecond)
    }
    wantCapture := observed.Add(-500 * time.Millisecond)
    if !got.CapturedAt.Equal(wantCapture) {
        t.Fatalf("captured = %v, want %v", got.CapturedAt, wantCapture)
    }
}

func TestAlignKernelKtimeRejectsFutureMonotonicValue(t *testing.T) {
    observed := time.Date(2026, 9, 13, 15, 0, 0, 0, time.UTC)
    got := alignKernelKtime(11_000, observed, 10_000)
    if got.Clock != "unknown" || got.DelayNS != 0 || !got.CapturedAt.Equal(observed) {
        t.Fatalf("future monotonic timestamp should fall back to ingest time: %+v", got)
    }
}
''')

old_annotate = r'''// annotateKernelAuditTiming records the userspace observation window for an
// eBPF ring-buffer event using fields that are already part of the event schema.
//
// This deliberately does NOT claim LastSeenMs is a raw kernel timestamp. It is
// the time the decoded sample reached the backend. For generic syscall records
// the tracker records enter/exit DurationNs, allowing FirstSeenMs to be a
// best-effort start estimate. Flow-level network events may already carry
// authoritative first/last timestamps; those are never overwritten.
func annotateKernelAuditTiming(raw *core.BpfEvent, event *pb.Event, observedAt time.Time) {
	if raw == nil || event == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	observedMS := uint64(observedAt.UnixMilli())
	if event.GetLastSeenMs() == 0 {
		event.LastSeenMs = observedMS
	}
	if event.GetFirstSeenMs() != 0 {
		return
	}

	startedAt := observedAt
	// Only TYPE_GENERIC_SYSCALL has duration_ns measured from bpf_ktime_get_ns
	// enter→exit correlation today. Other event types are intentionally ignored:
	// notably tcp_state_change historically packs old/new TCP states into the
	// same raw DurationNs slot.
	if event.GetType() == "syscall" && raw.DurationNs > 0 {
		duration := time.Duration(raw.DurationNs)
		if duration > 0 && duration <= maxKernelAuditDuration {
			startedAt = observedAt.Add(-duration)
		}
	}
	event.FirstSeenMs = uint64(startedAt.UnixMilli())
}
'''
new_annotate = r'''// annotateKernelAuditTiming preserves raw kernel provenance and derives a
// wall-clock capture time without confusing userspace ingest time with probe
// time. Existing flow-level first/last timestamps remain authoritative.
func annotateKernelAuditTiming(raw *core.BpfEvent, event *pb.Event, observedAt time.Time) {
	if raw == nil || event == nil {
		return
	}
	observation := observeKernelKtime(raw.KernelTimestampNs, observedAt)
	capturedAt := observation.CapturedAt

	event.KernelTimestampNs = raw.KernelTimestampNs
	event.KernelSequence = raw.KernelSequence
	event.KernelCpu = raw.KernelCPU
	event.KernelClock = observation.Clock
	event.IngestTimestampNs = uint64(observation.IngestedAt.UnixNano())
	event.CaptureDelayNs = observation.DelayNS
	event.CaptureTimestampNs = uint64(capturedAt.UnixNano())
	event.AuditFlags = raw.AuditFlags
	collectorMetricsStore.RecordKernelCaptureTiming(observation.DelayNS, observation.Clock)

	if event.GetLastSeenMs() == 0 {
		event.LastSeenMs = uint64(capturedAt.UnixMilli())
	}
	if event.GetFirstSeenMs() != 0 {
		return
	}

	startedAt := capturedAt
	if event.GetType() == "syscall" && raw.DurationNs > 0 {
		duration := time.Duration(raw.DurationNs)
		if duration > 0 && duration <= maxKernelAuditDuration {
			startedAt = capturedAt.Add(-duration)
		}
	}
	event.FirstSeenMs = uint64(startedAt.UnixMilli())
}
'''
replace_once("backend/app/kernelriskbridge.go", old_annotate, new_annotate)

# Envelope capture timestamp/provenance.
replace_once(
    "backend/app/events/envelope_event.go",
    """\ttimestamp := record.ReceivedAt.UTC()\n\tif timestamp.IsZero() {\n\t\ttimestamp = time.Now().UTC()\n\t}\n\tenvelope := &pb.EventEnvelope{\n\t\tSchemaVersion:  eventEnvelopeSchemaVersion,\n\t\tTimestampNs:    uint64(timestamp.UnixNano()),\n""",
    """\ttimestamp := record.ReceivedAt.UTC()\n\tif timestamp.IsZero() {\n\t\ttimestamp = time.Now().UTC()\n\t}\n\ttimestampNS := uint64(timestamp.UnixNano())\n\tif event.GetCaptureTimestampNs() != 0 {\n\t\ttimestampNS = event.GetCaptureTimestampNs()\n\t}\n\tenvelope := &pb.EventEnvelope{\n\t\tSchemaVersion:  eventEnvelopeSchemaVersion,\n\t\tTimestampNs:    timestampNS,\n""",
)
replace_once(
    "backend/app/events/envelope_event.go",
    """\t\tRiskScore:      event.GetRiskScore(),\n\t\tEventType:      event.GetEventType(),\n\t\tLegacyEvent:    event,\n""",
    """\t\tRiskScore:          event.GetRiskScore(),\n\t\tEventType:          event.GetEventType(),\n\t\tKernelTimestampNs:  event.GetKernelTimestampNs(),\n\t\tKernelSequence:     event.GetKernelSequence(),\n\t\tKernelCpu:          event.GetKernelCpu(),\n\t\tKernelClock:        event.GetKernelClock(),\n\t\tIngestTimestampNs:  event.GetIngestTimestampNs(),\n\t\tCaptureDelayNs:     event.GetCaptureDelayNs(),\n\t\tCaptureTimestampNs: event.GetCaptureTimestampNs(),\n\t\tAuditFlags:         event.GetAuditFlags(),\n\t\tLegacyEvent:        event,\n""",
)
replace_once(
    "backend/app/events/envelope_event.go",
    """\tif cloned.GetTimestampNs() == 0 {\n\t\ttimestamp := record.ReceivedAt.UTC()\n\t\tif timestamp.IsZero() {\n\t\t\ttimestamp = time.Now().UTC()\n\t\t}\n\t\tcloned.TimestampNs = uint64(timestamp.UnixNano())\n\t}\n""",
    """\tlegacy := firstNonNilEvent(record.Event, cloned.GetLegacyEvent())\n\tif cloned.GetTimestampNs() == 0 {\n\t\tif legacy != nil && legacy.GetCaptureTimestampNs() != 0 {\n\t\t\tcloned.TimestampNs = legacy.GetCaptureTimestampNs()\n\t\t} else {\n\t\t\ttimestamp := record.ReceivedAt.UTC()\n\t\t\tif timestamp.IsZero() {\n\t\t\t\ttimestamp = time.Now().UTC()\n\t\t\t}\n\t\t\tcloned.TimestampNs = uint64(timestamp.UnixNano())\n\t\t}\n\t}\n\tif legacy != nil {\n\t\tif cloned.GetKernelTimestampNs() == 0 { cloned.KernelTimestampNs = legacy.GetKernelTimestampNs() }\n\t\tif cloned.GetKernelSequence() == 0 { cloned.KernelSequence = legacy.GetKernelSequence() }\n\t\tif cloned.GetKernelCpu() == 0 { cloned.KernelCpu = legacy.GetKernelCpu() }\n\t\tif strings.TrimSpace(cloned.GetKernelClock()) == \"\" { cloned.KernelClock = legacy.GetKernelClock() }\n\t\tif cloned.GetIngestTimestampNs() == 0 { cloned.IngestTimestampNs = legacy.GetIngestTimestampNs() }\n\t\tif cloned.GetCaptureDelayNs() == 0 { cloned.CaptureDelayNs = legacy.GetCaptureDelayNs() }\n\t\tif cloned.GetCaptureTimestampNs() == 0 { cloned.CaptureTimestampNs = legacy.GetCaptureTimestampNs() }\n\t\tif cloned.GetAuditFlags() == 0 { cloned.AuditFlags = legacy.GetAuditFlags() }\n\t}\n""",
)

# Collector metrics.
replace_once(
    "backend/app/observability/metrics_collector.go",
    """type bpfCollectorStats struct {\n\tRingbufEventsTotal        uint64\n\tRingbufReserveFailedTotal uint64\n}\n""",
    """type bpfCollectorStats struct {\n\tRingbufEventsTotal        uint64\n\tRingbufReserveFailedTotal uint64\n\tEventSequence             uint64\n}\n""",
)
replace_once(
    "backend/app/observability/metrics_collector.go",
    """\tRingbufZeroCopyDecodeTotal     uint64\n\tRingbufCopyDecodeTotal         uint64\n\tKernelRiskEvaluationsTotal     uint64\n""",
    """\tRingbufZeroCopyDecodeTotal     uint64\n\tRingbufCopyDecodeTotal         uint64\n\tKernelCaptureDelaySamples      uint64\n\tKernelCaptureDelayLastNs       uint64\n\tKernelCaptureDelayMaxNs        uint64\n\tKernelCaptureClockUnknown      uint64\n\tKernelRiskEvaluationsTotal     uint64\n""",
)
replace_once(
    "backend/app/observability/metrics_collector.go",
    """\tRingbufCopyDecodeTotal         uint64            `json:\"ringbufCopyDecodeTotal\"`\n\tEventsByTypeTotal              map[string]uint64 `json:\"eventsByTypeTotal\"`\n""",
    """\tRingbufCopyDecodeTotal         uint64            `json:\"ringbufCopyDecodeTotal\"`\n\tKernelSequencedEventsTotal     uint64            `json:\"kernelSequencedEventsTotal\"`\n\tKernelCaptureDelaySamples      uint64            `json:\"kernelCaptureDelaySamples\"`\n\tKernelCaptureDelayLastNs       uint64            `json:\"kernelCaptureDelayLastNs\"`\n\tKernelCaptureDelayMaxNs        uint64            `json:\"kernelCaptureDelayMaxNs\"`\n\tKernelCaptureClockUnknown      uint64            `json:\"kernelCaptureClockUnknownTotal\"`\n\tEventsByTypeTotal              map[string]uint64 `json:\"eventsByTypeTotal\"`\n""",
)
replace_once(
    "backend/app/observability/metrics_collector.go",
    """\tringbufZeroCopyDecodeTotal     uint64\n\tringbufCopyDecodeTotal         uint64\n\tkernelRiskEvaluationsTotal     uint64\n""",
    """\tringbufZeroCopyDecodeTotal     uint64\n\tringbufCopyDecodeTotal         uint64\n\tkernelCaptureDelaySamples      uint64\n\tkernelCaptureDelayLastNs       uint64\n\tkernelCaptureDelayMaxNs        uint64\n\tkernelCaptureClockUnknown      uint64\n\tkernelRiskEvaluationsTotal     uint64\n""",
)
replace_once(
    "backend/app/observability/metrics_collector.go",
    """func (s *collectorMetricsState) RecordRingbufDecode(zeroCopy bool) {\n\ts.recordRingbufDecode(zeroCopy)\n}\n\nfunc RecordKernelRiskDecision(decision string, duration time.Duration) {\n""",
    """func (s *collectorMetricsState) RecordRingbufDecode(zeroCopy bool) {\n\ts.recordRingbufDecode(zeroCopy)\n}\n\nfunc RecordKernelCaptureTiming(delayNS uint64, clock string) {\n\tcollectorMetricsStore.recordKernelCaptureTiming(delayNS, clock)\n}\n\nfunc (s *collectorMetricsState) recordKernelCaptureTiming(delayNS uint64, clock string) {\n\ts.mu.Lock()\n\ts.kernelCaptureDelaySamples++\n\ts.kernelCaptureDelayLastNs = delayNS\n\tif delayNS > s.kernelCaptureDelayMaxNs {\n\t\ts.kernelCaptureDelayMaxNs = delayNS\n\t}\n\tif strings.TrimSpace(clock) != \"monotonic\" {\n\t\ts.kernelCaptureClockUnknown++\n\t}\n\ts.mu.Unlock()\n}\n\nfunc (s *collectorMetricsState) RecordKernelCaptureTiming(delayNS uint64, clock string) {\n\ts.recordKernelCaptureTiming(delayNS, clock)\n}\n\nfunc RecordKernelRiskDecision(decision string, duration time.Duration) {\n""",
)
replace_once(
    "backend/app/observability/metrics_collector.go",
    """\t\tRingbufZeroCopyDecodeTotal:     s.ringbufZeroCopyDecodeTotal,\n\t\tRingbufCopyDecodeTotal:         s.ringbufCopyDecodeTotal,\n\t\tKernelRiskEvaluationsTotal:     s.kernelRiskEvaluationsTotal,\n""",
    """\t\tRingbufZeroCopyDecodeTotal:     s.ringbufZeroCopyDecodeTotal,\n\t\tRingbufCopyDecodeTotal:         s.ringbufCopyDecodeTotal,\n\t\tKernelCaptureDelaySamples:      s.kernelCaptureDelaySamples,\n\t\tKernelCaptureDelayLastNs:       s.kernelCaptureDelayLastNs,\n\t\tKernelCaptureDelayMaxNs:        s.kernelCaptureDelayMaxNs,\n\t\tKernelCaptureClockUnknown:      s.kernelCaptureClockUnknown,\n\t\tKernelRiskEvaluationsTotal:     s.kernelRiskEvaluationsTotal,\n""",
)
replace_once(
    "backend/app/observability/metrics_collector.go",
    """\t\tRingbufZeroCopyDecodeTotal:     raw.RingbufZeroCopyDecodeTotal,\n\t\tRingbufCopyDecodeTotal:         raw.RingbufCopyDecodeTotal,\n\t\tEventsByTypeTotal:              eventsByType,\n""",
    """\t\tRingbufZeroCopyDecodeTotal:     raw.RingbufZeroCopyDecodeTotal,\n\t\tRingbufCopyDecodeTotal:         raw.RingbufCopyDecodeTotal,\n\t\tKernelSequencedEventsTotal:     bpfStats.EventSequence,\n\t\tKernelCaptureDelaySamples:      raw.KernelCaptureDelaySamples,\n\t\tKernelCaptureDelayLastNs:       raw.KernelCaptureDelayLastNs,\n\t\tKernelCaptureDelayMaxNs:        raw.KernelCaptureDelayMaxNs,\n\t\tKernelCaptureClockUnknown:      raw.KernelCaptureClockUnknown,\n\t\tEventsByTypeTotal:              eventsByType,\n""",
)
replace_once(
    "backend/app/observability/metrics_collector.go",
    """\t\ttotal.RingbufEventsTotal += value.RingbufEventsTotal\n\t\ttotal.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal\n""",
    """\t\ttotal.RingbufEventsTotal += value.RingbufEventsTotal\n\t\ttotal.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal\n\t\ttotal.EventSequence += value.EventSequence\n""",
)
replace_once(
    "backend/app/collectormetricsbridge.go",
    """func (metricsStoreBridge) RecordRingbufDecode(zeroCopy bool) {\n\tobservability.RecordRingbufDecode(zeroCopy)\n}\n\nfunc (metricsStoreBridge) RecordKernelRiskDecision(decision string, elapsed time.Duration) {\n""",
    """func (metricsStoreBridge) RecordRingbufDecode(zeroCopy bool) {\n\tobservability.RecordRingbufDecode(zeroCopy)\n}\n\nfunc (metricsStoreBridge) RecordKernelCaptureTiming(delayNS uint64, clock string) {\n\tobservability.RecordKernelCaptureTiming(delayNS, clock)\n}\n\nfunc (metricsStoreBridge) RecordKernelRiskDecision(decision string, elapsed time.Duration) {\n""",
)

# TLS kernel diagnostics/hot path.
replace_once(
    "backend/ebpf/agent_tls_capture.c",
    """#define TLS_DIAG_PERF_OUTPUT_FAIL 100\n#define TLS_DIAG_PROBE_READ_FAIL 101\n#define TLS_DIAG_PERF_SUBMIT_OK 102\n#define TLS_PROBE_HIT_SLOTS 128\n""",
    """#define TLS_DIAG_PERF_OUTPUT_FAIL 100\n#define TLS_DIAG_PROBE_READ_FAIL 101\n#define TLS_DIAG_PERF_SUBMIT_OK 102\n#define TLS_DIAG_RETPROBE_MISS 103\n#define TLS_DIAG_LENGTH_CLAMP 104\n#define TLS_DIAG_PARTIAL_CAPTURE 105\n#define TLS_DIAG_RETPROBE_STORE_FAIL 106\n#define TLS_DIAG_TRUNCATED_PAYLOAD 107\n#define TLS_PROBE_HIT_SLOTS 128\n""",
)
replace_once(
    "backend/ebpf/agent_tls_capture.c",
    """// Function counters occupy 1..13 and diagnostics occupy 100..102. Keep this\n// array large enough for both groups; the previous 16-slot array made every\n""",
    """// Function counters occupy 1..13 and diagnostics occupy 100..107. Keep this\n// array large enough for both groups; the previous 16-slot array made every\n""",
)
replace_once(
    "backend/ebpf/agent_tls_capture.c",
    """static __always_inline void inc_probe_hit(__u8 function) {\n\t__u32 idx = (__u32)function;\n\t__u64 *cnt = bpf_map_lookup_elem(&tls_probe_hits, &idx);\n\tif (cnt) {\n\t\t__sync_fetch_and_add(cnt, 1);\n\t}\n}\n""",
    """static __always_inline void inc_probe_hit(__u8 function) {\n\t__u32 idx = (__u32)function;\n\t__u64 *cnt = bpf_map_lookup_elem(&tls_probe_hits, &idx);\n\tif (cnt) {\n\t\t__sync_fetch_and_add(cnt, 1);\n\t}\n}\n\nstatic __always_inline void inc_tls_diag(__u32 idx) {\n\t__u64 *cnt = bpf_map_lookup_elem(&tls_probe_hits, &idx);\n\tif (cnt) {\n\t\t__sync_fetch_and_add(cnt, 1);\n\t}\n}\n\nstatic __always_inline __u32 clamp_retprobe_len(__u64 observed, __u32 capacity) {\n\tif (observed == 0 || capacity == 0) {\n\t\treturn 0;\n\t}\n\tif (observed > (__u64)capacity) {\n\t\tinc_tls_diag(TLS_DIAG_LENGTH_CLAMP);\n\t\treturn capacity;\n\t}\n\treturn (__u32)observed;\n}\n""",
)

sub_once(
    "backend/ebpf/agent_tls_capture.c",
    r"static __always_inline int emit_tls_fragment_uncounted\(void \*ctx, __u64 connection_id, const void \*buf, __u32 original_len, __u8 lib, __u8 dir, __u8 function\)\n\{.*?\n\}\n\nstatic __always_inline int emit_tls_fragment\(",
    r'''static __always_inline int emit_tls_fragment_uncounted(void *ctx, __u64 connection_id, const void *buf, __u32 original_len, __u8 lib, __u8 dir, __u8 function)
{
	__u32 zero = 0;

	if (!buf || original_len == 0) {
		return 0;
	}

	__u32 total_len = original_len;
	__u8 flags = 0;
	if (total_len > TLS_MAX_CAPTURE_SIZE) {
		total_len = TLS_MAX_CAPTURE_SIZE;
		flags |= TLS_FLAG_TRUNCATED;
		inc_tls_diag(TLS_DIAG_TRUNCATED_PAYLOAD);
	}

	__u64 pid_tgid = bpf_get_current_pid_tgid();
	__u64 now_ns = bpf_ktime_get_ns();
	__u32 frag_count32 = (total_len + TLS_FRAG_SIZE - 1) / TLS_FRAG_SIZE;
	if (frag_count32 == 0 || frag_count32 > TLS_MAX_FRAGS) {
		return 0;
	}

	struct tls_fragment *scratch = bpf_map_lookup_elem(&tls_scratch, &zero);
	if (!scratch) {
		return 0;
	}

	scratch->timestamp_ns = now_ns;
	scratch->connection_id = connection_id;
	scratch->pid = (__u32)pid_tgid;
	scratch->tgid = (__u32)(pid_tgid >> 32);
	scratch->total_len = total_len;
	scratch->original_len = original_len;
	scratch->frag_count = (__u16)frag_count32;
	scratch->lib_type = lib;
	scratch->direction = dir;
	scratch->flags = flags;
	scratch->function = function;
	bpf_get_current_comm(&scratch->comm, sizeof(scratch->comm));

	for (__u32 i = 0; i < frag_count32; i++) {
		__u32 offset = i * TLS_FRAG_SIZE;
		__u32 chunk = total_len - offset;
		if (chunk > TLS_FRAG_SIZE) {
			chunk = TLS_FRAG_SIZE;
		}
		scratch->data_len = chunk;
		scratch->frag_index = (__u16)i;

		if (bpf_probe_read_user(scratch->data, chunk, (const char *)buf + offset) < 0) {
			inc_tls_diag(TLS_DIAG_PROBE_READ_FAIL);
			if (i > 0) inc_tls_diag(TLS_DIAG_PARTIAL_CAPTURE);
			break;
		}

		__u64 sample_size = (__u64)TLS_FRAGMENT_WIRE_HEADER_SIZE + (__u64)chunk;
		long ret = bpf_perf_event_output(ctx, &tls_events, BPF_F_CURRENT_CPU, scratch, sample_size);
		if (ret < 0) {
			inc_tls_diag(TLS_DIAG_PERF_OUTPUT_FAIL);
			if (i > 0) inc_tls_diag(TLS_DIAG_PARTIAL_CAPTURE);
			break;
		}
		inc_tls_diag(TLS_DIAG_PERF_SUBMIT_OK);
	}

	return 0;
}

static __always_inline int emit_tls_fragment(''',
)

sub_once(
    "backend/ebpf/agent_tls_capture.c",
    r"static __always_inline int save_retprobe_ctx\(__u64 connection_id, void \*buf, const void \*len_ptr, __u32 len, __u8 lib, __u8 dir, __u8 function\)\n\{.*?\n\}\n\nstatic __always_inline int load_retprobe_ctx",
    r'''static __always_inline int save_retprobe_ctx(__u64 connection_id, void *buf, const void *len_ptr, __u32 len, __u8 lib, __u8 dir, __u8 function)
{
	inc_probe_hit(function);
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	struct retprobe_ctx rc = {
		.buf = (__u64)buf,
		.len_ptr = (__u64)len_ptr,
		.connection_id = connection_id,
		.len = len,
		.lib_type = lib,
		.direction = dir,
		.function = function,
	};
	long ret = bpf_map_update_elem(&retprobe_buf, &pid_tgid, &rc, BPF_ANY);
	if (ret < 0) inc_tls_diag(TLS_DIAG_RETPROBE_STORE_FAIL);
	return (int)ret;
}

static __always_inline int load_retprobe_ctx''',
)
sub_once(
    "backend/ebpf/agent_tls_capture.c",
    r"static __always_inline int load_retprobe_ctx\(struct retprobe_ctx \*out\)\n\{.*?\n\}\n\nstatic __always_inline int emit_retprobe_payload\(void \*ctx, __u32 len\)\n\{.*?\n\}\n",
    r'''static __always_inline int load_retprobe_ctx(struct retprobe_ctx *out)
{
	__u64 pid_tgid = bpf_get_current_pid_tgid();
	struct retprobe_ctx *rc = bpf_map_lookup_elem(&retprobe_buf, &pid_tgid);
	if (!rc) {
		inc_tls_diag(TLS_DIAG_RETPROBE_MISS);
		return 0;
	}
	*out = *rc;
	bpf_map_delete_elem(&retprobe_buf, &pid_tgid);
	return 1;
}

static __always_inline int emit_retprobe_payload_signed(void *ctx, __s64 result)
{
	struct retprobe_ctx rc = {};
	if (!load_retprobe_ctx(&rc)) {
		return 0;
	}
	if (result <= 0) {
		return 0;
	}
	__u32 safe_len = clamp_retprobe_len((__u64)result, rc.len);
	if (safe_len == 0) {
		return 0;
	}
	return emit_tls_fragment_uncounted(ctx, rc.connection_id, (const void *)rc.buf, safe_len, rc.lib_type, rc.direction, rc.function);
}
''',
)

# Consume retprobe context even on failed reads.
old_recv = """\t__s32 ret = (__s32)PT_REGS_RC(ctx);\n\tif (ret <= 0) {\n\t\treturn 0;\n\t}\n\treturn emit_retprobe_payload(ctx, (__u32)ret);\n"""
tls_c = read("backend/ebpf/agent_tls_capture.c")
if tls_c.count(old_recv) != 3:
    raise SystemExit(f"expected SSL/GnuTLS/NSS recv return bodies = 3, got {tls_c.count(old_recv)}")
tls_c = tls_c.replace(old_recv, """\t__s32 ret = (__s32)PT_REGS_RC(ctx);\n\treturn emit_retprobe_payload_signed(ctx, (__s64)ret);\n""", 3)
write("backend/ebpf/agent_tls_capture.c", tls_c)
replace_once(
    "backend/ebpf/agent_tls_capture.c",
    """\t__s64 n = (__s64)ctx->ax;\n\tif (n <= 0) {\n\t\treturn 0;\n\t}\n\treturn emit_retprobe_payload(ctx, (__u32)n);\n""",
    """\t__s64 n = (__s64)ctx->ax;\n\treturn emit_retprobe_payload_signed(ctx, n);\n""",
)

old_write_ex = """\t__u64 written = 0;\n\tif (rc.len_ptr && bpf_probe_read_user(&written, sizeof(written), (const void *)rc.len_ptr) == 0 && written > 0) {\n\t\treturn emit_tls_fragment_uncounted(ctx, rc.connection_id, (const void *)rc.buf, (__u32)written, rc.lib_type, rc.direction, rc.function);\n\t}\n\treturn emit_tls_fragment_uncounted(ctx, rc.connection_id, (const void *)rc.buf, rc.len, rc.lib_type, rc.direction, rc.function);\n"""
new_write_ex = """\tif (!rc.len_ptr) {\n\t\tinc_tls_diag(TLS_DIAG_PROBE_READ_FAIL);\n\t\treturn 0;\n\t}\n\t__u64 written = 0;\n\tif (bpf_probe_read_user(&written, sizeof(written), (const void *)rc.len_ptr) < 0) {\n\t\tinc_tls_diag(TLS_DIAG_PROBE_READ_FAIL);\n\t\treturn 0;\n\t}\n\t__u32 safe_len = clamp_retprobe_len(written, rc.len);\n\tif (safe_len == 0) return 0;\n\treturn emit_tls_fragment_uncounted(ctx, rc.connection_id, (const void *)rc.buf, safe_len, rc.lib_type, rc.direction, rc.function);\n"""
tls_c = read("backend/ebpf/agent_tls_capture.c")
if tls_c.count(old_write_ex) != 2:
    raise SystemExit(f"expected SSL_write_ex and SSL_write_ex2 bodies = 2, got {tls_c.count(old_write_ex)}")
write("backend/ebpf/agent_tls_capture.c", tls_c.replace(old_write_ex, new_write_ex, 2))
replace_once(
    "backend/ebpf/agent_tls_capture.c",
    """\t__u64 read_len = 0;\n\tif (bpf_probe_read_user(&read_len, sizeof(read_len), (const void *)rc.len_ptr) < 0 || read_len == 0) {\n\t\treturn 0;\n\t}\n\treturn emit_tls_fragment_uncounted(ctx, rc.connection_id, (const void *)rc.buf, (__u32)read_len, rc.lib_type, rc.direction, rc.function);\n""",
    """\t__u64 read_len = 0;\n\tif (bpf_probe_read_user(&read_len, sizeof(read_len), (const void *)rc.len_ptr) < 0) {\n\t\tinc_tls_diag(TLS_DIAG_PROBE_READ_FAIL);\n\t\treturn 0;\n\t}\n\t__u32 safe_len = clamp_retprobe_len(read_len, rc.len);\n\tif (safe_len == 0) return 0;\n\treturn emit_tls_fragment_uncounted(ctx, rc.connection_id, (const void *)rc.buf, safe_len, rc.lib_type, rc.direction, rc.function);\n""",
)

replace_once(
    "backend/app/tls/probemanager_lifecycletls.go",
    """\t\tdiagLabels := map[uint32]string{100: \"perf_output_fail\", 101: \"probe_read_fail\", 102: \"perf_submit_ok\"}\n""",
    """\t\tdiagLabels := map[uint32]string{\n\t\t\t100: \"perf_output_fail\",\n\t\t\t101: \"probe_read_fail\",\n\t\t\t102: \"perf_submit_ok\",\n\t\t\t103: \"retprobe_miss\",\n\t\t\t104: \"return_length_clamp\",\n\t\t\t105: \"partial_capture\",\n\t\t\t106: \"retprobe_store_fail\",\n\t\t\t107: \"truncated_payload\",\n\t\t}\n""",
)

# TLS read-loop diagnostics.
replace_once(
    "backend/app/tls/probemanagertls.go",
    """type ReadLoopStats struct {\n\tTotalFrags     int64\n\tDroppedFrags   int64\n\tCompletedFrags int64\n\tHTTPEvents     int64\n\tRawEvents      int64\n\tLastFragmentNS int64\n}\n\ntype readLoopAtomicStats struct {\n\ttotalFrags     atomic.Int64\n\tdroppedFrags   atomic.Int64\n\tcompletedFrags atomic.Int64\n\thttpEvents     atomic.Int64\n\trawEvents      atomic.Int64\n\tlastFragmentNS atomic.Int64\n}\n""",
    """type ReadLoopStats struct {\n\tTotalFrags       int64\n\tDroppedFrags     int64\n\tPerfLostSamples  int64\n\tDecodeErrors     int64\n\tAssemblerDropped int64\n\tCompletedFrags   int64\n\tHTTPEvents       int64\n\tRawEvents        int64\n\tLastFragmentNS   int64\n}\n\ntype readLoopAtomicStats struct {\n\ttotalFrags      atomic.Int64\n\tdroppedFrags    atomic.Int64\n\tperfLostSamples atomic.Int64\n\tdecodeErrors    atomic.Int64\n\tcompletedFrags  atomic.Int64\n\thttpEvents      atomic.Int64\n\trawEvents       atomic.Int64\n\tlastFragmentNS  atomic.Int64\n}\n""",
)
replace_once(
    "backend/app/tls/probemanagertls.go",
    """\treturn ReadLoopStats{\n\t\tTotalFrags:     s.totalFrags.Load(),\n\t\tDroppedFrags:   s.droppedFrags.Load(),\n\t\tCompletedFrags: s.completedFrags.Load(),\n\t\tHTTPEvents:     s.httpEvents.Load(),\n\t\tRawEvents:      s.rawEvents.Load(),\n\t\tLastFragmentNS: s.lastFragmentNS.Load(),\n\t}\n""",
    """\treturn ReadLoopStats{\n\t\tTotalFrags:      s.totalFrags.Load(),\n\t\tDroppedFrags:    s.droppedFrags.Load(),\n\t\tPerfLostSamples: s.perfLostSamples.Load(),\n\t\tDecodeErrors:    s.decodeErrors.Load(),\n\t\tCompletedFrags:  s.completedFrags.Load(),\n\t\tHTTPEvents:      s.httpEvents.Load(),\n\t\tRawEvents:       s.rawEvents.Load(),\n\t\tLastFragmentNS:  s.lastFragmentNS.Load(),\n\t}\n""",
)
replace_once(
    "backend/app/tls/probemanager_readlooptls.go",
    """\t\tif rec.LostSamples > 0 {\n\t\t\tm.readLoopStats.droppedFrags.Add(int64(rec.LostSamples))\n\t\t\tlog.Printf(\"[tls] ReadLoop: kernel perf buffer lost %d samples\", rec.LostSamples)\n\t\t}\n""",
    """\t\tif rec.LostSamples > 0 {\n\t\t\tm.readLoopStats.droppedFrags.Add(int64(rec.LostSamples))\n\t\t\tm.readLoopStats.perfLostSamples.Add(int64(rec.LostSamples))\n\t\t\tlog.Printf(\"[tls] ReadLoop: kernel perf buffer lost %d samples\", rec.LostSamples)\n\t\t}\n""",
)
replace_once(
    "backend/app/tls/probemanager_readlooptls.go",
    """\t\tif err != nil {\n\t\t\tm.readLoopStats.droppedFrags.Add(1)\n\t\t\tif totalFrags <= 5 {\n""",
    """\t\tif err != nil {\n\t\t\tm.readLoopStats.droppedFrags.Add(1)\n\t\t\tm.readLoopStats.decodeErrors.Add(1)\n\t\t\tif totalFrags <= 5 {\n""",
)
replace_once(
    "backend/app/tls/probemanager_lifecycletls.go",
    """func (m *TLSProbeManager) ReadLoopStatsSnapshot() ReadLoopStats {\n\tif m == nil {\n\t\treturn ReadLoopStats{}\n\t}\n\treturn m.readLoopStats.Snapshot()\n}\n""",
    """func (m *TLSProbeManager) ReadLoopStatsSnapshot() ReadLoopStats {\n\tif m == nil {\n\t\treturn ReadLoopStats{}\n\t}\n\tstats := m.readLoopStats.Snapshot()\n\tif m.assembler != nil {\n\t\tstats.AssemblerDropped = int64(m.assembler.Dropped())\n\t}\n\treturn stats\n}\n""",
)
replace_once(
    "backend/app/tls/probemanagertls_test.go",
    """\tmanager.readLoopStats.droppedFrags.Add(1)\n\tmanager.readLoopStats.completedFrags.Add(2)\n""",
    """\tmanager.readLoopStats.droppedFrags.Add(1)\n\tmanager.readLoopStats.perfLostSamples.Add(4)\n\tmanager.readLoopStats.decodeErrors.Add(2)\n\tmanager.readLoopStats.completedFrags.Add(2)\n""",
)
replace_once(
    "backend/app/tls/probemanagertls_test.go",
    """\tif stats.TotalFrags != 3 || stats.DroppedFrags != 1 || stats.CompletedFrags != 2 || stats.HTTPEvents != 1 || stats.RawEvents != 1 || stats.LastFragmentNS != 42 {\n""",
    """\tif stats.TotalFrags != 3 || stats.DroppedFrags != 1 || stats.PerfLostSamples != 4 || stats.DecodeErrors != 2 || stats.CompletedFrags != 2 || stats.HTTPEvents != 1 || stats.RawEvents != 1 || stats.LastFragmentNS != 42 {\n""",
)

# Documentation.
doc = read("docs/backend/capture-timing-audit.md")
old_doc = """## 主 tracker 的 audit observation window\n\n主 syscall tracker 当前 raw ABI 尚未增加独立的 kernel timestamp 字段。为避免把接收时间伪装成内核时间，后端为解码后的 eBPF 事件补充一个明确的 observation window：\n\n- `last_seen_ms`：若事件/flow 尚未提供该字段，写入后端解码/风险处理阶段的观察时间；\n- `first_seen_ms`：若没有更权威的 flow 时间，则默认等于 observation time；如果 eBPF enter/exit correlation 已提供可信的 `duration_ns`，则用 `observation - duration` 反推近似 syscall start；\n- 已由 network flow aggregator 生成的 first/last 时间不会被覆盖；\n- `tcp_state_change` 历史 ABI 会复用 `duration_ns` 打包 old/new state，因此明确排除在 duration-based start estimation 之外。\n\n因此：`first_seen_ms/last_seen_ms` 对主 tracker 是**审计观察窗口**，不是 raw kernel ktime。TLS 的 `probe_timestamp_ns` 才是当前链路里真正由 eBPF probe 直接提供的 monotonic timestamp。\n"""
new_doc = """## 主 tracker 的 raw kernel audit provenance\n\n主 tracker raw ABI 现在在结构体尾部追加审计字段，因此旧字段偏移保持不变：\n\n- `kernel_timestamp_ns`：`reserve_event()` 后立即读取 `bpf_ktime_get_ns()`；\n- `kernel_cpu`：事件发生 CPU；\n- `kernel_sequence`：该 CPU 上单调递增的事件序号，和 `kernel_cpu` 组成审计排序 tuple；\n- `audit_flags`：声明 timestamp / CPU-local sequence 是否有效；\n- `ingest_timestamp_ns`：Go 后端观察时刻；\n- `capture_delay_ns`：当前 `CLOCK_MONOTONIC - kernel_timestamp_ns`；\n- `capture_timestamp_ns`：将 monotonic probe time 对齐到 wall clock 后的纳秒时间。\n\n`first_seen_ms/last_seen_ms` 在没有更权威 flow 时间时现在以 **kernel capture time** 为基准，而不是 backend receive time。generic syscall 若有可信的 enter→exit `duration_ns`，`first_seen_ms` 再从 capture/exit time 向前推；`tcp_state_change` 因历史 ABI 复用 `duration_ns` 存 TCP state，仍不会被当成 syscall latency。已有 network flow aggregator 的 first/last 时间继续保持优先级，不会被覆盖。\n\n`kernel_sequence` 是 **CPU-local** 而非全局原子序号。这是刻意的性能设计：`(kernel_cpu, kernel_sequence)` 可用于同 CPU 严格排序和审计缺口定位，同时避免所有 CPU 为每条事件竞争同一个共享 counter。\n"""
if old_doc not in doc:
    raise SystemExit("capture timing doc: old tracker timing section not found")
doc = doc.replace(old_doc, new_doc, 1)
doc += r'''

## TLS probe diagnostics hardening

TLS eBPF 热路径新增并暴露以下安全/可靠性计数：

- `retprobe_miss`：返回探针找不到 entry context（LRU 淘汰、ABI/attach 不匹配等）；
- `retprobe_store_fail`：entry context 无法写入 retprobe map；
- `return_length_clamp`：返回长度超过 entry 时记录的 buffer capacity，已安全 clamp；
- `truncated_payload`：payload 超过最大 capture window，被按设计截断；
- `partial_capture`：一个多 fragment payload 已提交部分 fragment 后发生 read/perf output 失败；
- `perf_output_fail` / `probe_read_fail` / `perf_submit_ok`：原有底层诊断继续保留。

性能上，TLS fragment loop 现在只在每次 TLS 调用开始时填一次 timestamp/PID/TGID/comm/总长度等不变量；循环内只更新 fragment index、data length 和 payload。任一 `bpf_perf_event_output()` 失败后立即终止后续 fragment，避免继续读取用户内存和产生必然无法重组的尾部数据。

return-probe 路径现在无论调用成功、返回 0 还是失败都会消费 entry context，避免失败调用在 `retprobe_buf` 中留下陈旧状态；所有返回长度在读取 payload 前都会和 entry buffer capacity 校验。
'''
write("docs/backend/capture-timing-audit.md", doc)
