from pathlib import Path
import re


def read(path):
    return Path(path).read_text()


def write(path, text):
    Path(path).write_text(text)


def replace_once(path, old, new):
    text = read(path)
    if text.count(old) != 1:
        raise SystemExit(f"{path}: expected exactly one match, got {text.count(old)} for {old[:80]!r}")
    write(path, text.replace(old, new, 1))


# ---------------------------------------------------------------------------
# Kernel ABI: allocate CPU-local sequence before reserve so reserve failures
# become visible sequence holes; attach self-reported loss + generation.
# ---------------------------------------------------------------------------
path = "backend/ebpf/agent_tracker_common.h"
replace_once(path,
"""    u64 kernel_timestamp_ns; // bpf_ktime_get_ns(), CLOCK_MONOTONIC domain
    u64 kernel_sequence;     // CPU-local monotonic sequence
    u32 kernel_cpu;
    u32 audit_flags;
};
""",
"""    u64 kernel_timestamp_ns; // bpf_ktime_get_ns(), CLOCK_MONOTONIC domain
    u64 kernel_sequence;     // CPU-local attempt sequence; reserve failures create holes
    u32 kernel_cpu;
    u32 audit_flags;
    u64 kernel_audit_generation;      // userspace-rotated tracker generation
    u64 kernel_dropped_since_last;    // reserve failures since previous successful reserve on this CPU
    u64 kernel_reserve_failures_total; // CPU-local cumulative reserve failures snapshot
};
""")
replace_once(path,
"""struct collector_stats {
    u64 ringbuf_events_total;
    u64 ringbuf_reserve_failed_total;
    u64 event_sequence; // per-CPU, used with kernel_cpu as an audit ordering tuple
};

struct {
""",
"""struct collector_stats {
    u64 ringbuf_events_total;
    u64 ringbuf_reserve_failed_total;
    u64 event_sequence; // per-CPU attempt sequence; increments before ringbuf reserve
    u64 pending_dropped_events; // reserve failures not yet reported by a successful event
    u64 audit_generation; // rotated when tracker programs are reloaded/re-attached
};

_Static_assert(sizeof(struct event) == 680, "event ABI size changed; update Go BpfEvent intentionally");
_Static_assert(sizeof(struct collector_stats) == 40, "collector_stats ABI size changed; regenerate bindings");

struct {
""")
old = """static __always_inline void account_ringbuf_reserve_failed(void) {
    u32 key = 0;
    struct collector_stats *stats = bpf_map_lookup_elem(&collector_stats, &key);
    if (stats) {
        stats->ringbuf_reserve_failed_total++;
    }
}

#define AUDIT_FLAG_KERNEL_TIMESTAMP (1U << 0)
#define AUDIT_FLAG_CPU_SEQUENCE     (1U << 1)

static __always_inline struct event *reserve_event(void) {
    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        account_ringbuf_reserve_failed();
        return 0;
    }

    // Capture provenance immediately after reserve so time includes as little
    // probe-side formatting work as possible. Sequence is deliberately per-CPU
    // to avoid a cross-CPU atomic hotspot on every observed syscall.
    e->kernel_timestamp_ns = bpf_ktime_get_ns();
    e->kernel_cpu = bpf_get_smp_processor_id();
    e->kernel_sequence = 0;
    e->audit_flags = AUDIT_FLAG_KERNEL_TIMESTAMP;

    u32 key = 0;
    struct collector_stats *stats = bpf_map_lookup_elem(&collector_stats, &key);
    if (stats) {
        stats->event_sequence++;
        e->kernel_sequence = stats->event_sequence;
        e->audit_flags |= AUDIT_FLAG_CPU_SEQUENCE;
    }
    return e;
}
"""
new = """static __always_inline void account_ringbuf_reserve_failed(struct collector_stats *stats) {
    if (stats) {
        stats->ringbuf_reserve_failed_total++;
        stats->pending_dropped_events++;
    }
}

#define AUDIT_FLAG_KERNEL_TIMESTAMP (1U << 0)
#define AUDIT_FLAG_CPU_SEQUENCE     (1U << 1)
#define AUDIT_FLAG_RESERVE_GAP      (1U << 2)
#define AUDIT_FLAG_GENERATION       (1U << 3)
#define AUDIT_FLAG_RESERVE_TOTAL    (1U << 4)

static __always_inline struct event *reserve_event(void) {
    // Assign provenance before ringbuf reserve. A failed reserve therefore
    // consumes a CPU-local sequence number, making loss visible as a hole in
    // the next successfully delivered event without any cross-CPU atomic.
    u64 timestamp_ns = bpf_ktime_get_ns();
    u32 cpu = bpf_get_smp_processor_id();
    u64 sequence = 0;
    u64 generation = 0;
    u32 key = 0;
    struct collector_stats *stats = bpf_map_lookup_elem(&collector_stats, &key);
    if (stats) {
        stats->event_sequence++;
        sequence = stats->event_sequence;
        generation = stats->audit_generation;
    }

    struct event *e = bpf_ringbuf_reserve(&events, sizeof(*e), 0);
    if (!e) {
        account_ringbuf_reserve_failed(stats);
        return 0;
    }

    e->kernel_timestamp_ns = timestamp_ns;
    e->kernel_cpu = cpu;
    e->kernel_sequence = sequence;
    e->audit_flags = AUDIT_FLAG_KERNEL_TIMESTAMP;
    e->kernel_audit_generation = generation;
    e->kernel_dropped_since_last = 0;
    e->kernel_reserve_failures_total = 0;

    if (stats) {
        if (sequence != 0) {
            e->audit_flags |= AUDIT_FLAG_CPU_SEQUENCE;
        }
        if (generation != 0) {
            e->audit_flags |= AUDIT_FLAG_GENERATION;
        }
        e->kernel_reserve_failures_total = stats->ringbuf_reserve_failed_total;
        e->audit_flags |= AUDIT_FLAG_RESERVE_TOTAL;
        if (stats->pending_dropped_events != 0) {
            e->kernel_dropped_since_last = stats->pending_dropped_events;
            e->audit_flags |= AUDIT_FLAG_RESERVE_GAP;
            stats->pending_dropped_events = 0;
        }
    }
    return e;
}
"""
replace_once(path, old, new)

# ---------------------------------------------------------------------------
# Go raw ABI + audit flag constants.
# ---------------------------------------------------------------------------
path = "backend/core/types.go"
replace_once(path,
"""\tKernelTimestampNs                      uint64
\tKernelSequence                         uint64
\tKernelCPU                              uint32
\tAuditFlags                             uint32
}
""",
"""\tKernelTimestampNs                      uint64
\tKernelSequence                         uint64
\tKernelCPU                              uint32
\tAuditFlags                             uint32
\tKernelAuditGeneration                  uint64
\tKernelDroppedSinceLast                 uint64
\tKernelReserveFailuresTotal             uint64
}

const (
\tBpfAuditFlagKernelTimestamp uint32 = 1 << iota
\tBpfAuditFlagCPUSequence
\tBpfAuditFlagReserveGap
\tBpfAuditFlagGeneration
\tBpfAuditFlagReserveTotal
)
""")

write("backend/core/types_abi_test.go", '''package core

import (
    "testing"
    "unsafe"
)

func TestBpfEventAuditABIGuard(t *testing.T) {
    var event BpfEvent
    if got := unsafe.Sizeof(event); got != 680 {
        t.Fatalf("BpfEvent size = %d, want 680", got)
    }
    if got := unsafe.Offsetof(event.KernelAuditGeneration); got != 656 {
        t.Fatalf("KernelAuditGeneration offset = %d, want 656", got)
    }
    if got := unsafe.Offsetof(event.KernelDroppedSinceLast); got != 664 {
        t.Fatalf("KernelDroppedSinceLast offset = %d, want 664", got)
    }
    if got := unsafe.Offsetof(event.KernelReserveFailuresTotal); got != 672 {
        t.Fatalf("KernelReserveFailuresTotal offset = %d, want 672", got)
    }
}
''')

# ---------------------------------------------------------------------------
# Public event schema.
# ---------------------------------------------------------------------------
path = "proto/tracker_events.proto"
replace_once(path,
"""  uint64 capture_timestamp_ns = 77;
  uint32 audit_flags = 78;
}
""",
"""  uint64 capture_timestamp_ns = 77;
  uint32 audit_flags = 78;
  uint64 kernel_audit_generation = 79;
  uint64 kernel_dropped_since_last = 80;
  uint64 kernel_reserve_failures_total = 81;
}
""")
replace_once(path,
"""  uint64 capture_timestamp_ns = 48;
  uint32 audit_flags = 49;
}
""",
"""  uint64 capture_timestamp_ns = 48;
  uint32 audit_flags = 49;
  uint64 kernel_audit_generation = 50;
  uint64 kernel_dropped_since_last = 51;
  uint64 kernel_reserve_failures_total = 52;
}
""")

# ---------------------------------------------------------------------------
# Kernel event annotation.
# ---------------------------------------------------------------------------
path = "backend/app/kernelriskbridge.go"
replace_once(path,
"""\tevent.CaptureTimestampNs = uint64(capturedAt.UnixNano())
\tevent.AuditFlags = raw.AuditFlags
\trecordKernelSequenceObservation(raw)
""",
"""\tevent.CaptureTimestampNs = uint64(capturedAt.UnixNano())
\tevent.AuditFlags = raw.AuditFlags
\tevent.KernelAuditGeneration = raw.KernelAuditGeneration
\tevent.KernelDroppedSinceLast = raw.KernelDroppedSinceLast
\tevent.KernelReserveFailuresTotal = raw.KernelReserveFailuresTotal
\trecordKernelSequenceObservation(raw)
""")

# ---------------------------------------------------------------------------
# Generation-aware continuity and kernel-vs-userspace loss attribution.
# ---------------------------------------------------------------------------
write("backend/app/kernel_sequence_audit.go", '''package app

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
''')

write("backend/app/kernel_sequence_audit_test.go", '''package app

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
''')

# ---------------------------------------------------------------------------
# Envelope propagation and stable kernel-provenance IDs.
# ---------------------------------------------------------------------------
path = "backend/app/events/envelope_event.go"
replace_once(path,
"""\t\tif cloned.GetAuditFlags() == 0 {
\t\t\tcloned.AuditFlags = legacy.GetAuditFlags()
\t\t}
""",
"""\t\tif cloned.GetAuditFlags() == 0 {
\t\t\tcloned.AuditFlags = legacy.GetAuditFlags()
\t\t}
\t\tif cloned.GetKernelAuditGeneration() == 0 {
\t\t\tcloned.KernelAuditGeneration = legacy.GetKernelAuditGeneration()
\t\t}
\t\tif cloned.GetKernelDroppedSinceLast() == 0 {
\t\t\tcloned.KernelDroppedSinceLast = legacy.GetKernelDroppedSinceLast()
\t\t}
\t\tif cloned.GetKernelReserveFailuresTotal() == 0 {
\t\t\tcloned.KernelReserveFailuresTotal = legacy.GetKernelReserveFailuresTotal()
\t\t}
""")
replace_once(path,
"""\t\tCaptureTimestampNs: event.GetCaptureTimestampNs(),
\t\tAuditFlags:         event.GetAuditFlags(),
\t\tLegacyEvent:        event,
""",
"""\t\tCaptureTimestampNs:        event.GetCaptureTimestampNs(),
\t\tAuditFlags:                event.GetAuditFlags(),
\t\tKernelAuditGeneration:     event.GetKernelAuditGeneration(),
\t\tKernelDroppedSinceLast:    event.GetKernelDroppedSinceLast(),
\t\tKernelReserveFailuresTotal: event.GetKernelReserveFailuresTotal(),
\t\tLegacyEvent:               event,
""")
text = read(path)
pattern = re.compile(r"func buildEventEnvelopeID\(record CapturedEventRecord, event \*pb\.Event\) string \{.*?\n\}\n\nfunc buildExecEnvelopePayload", re.S)
replacement = '''func buildEventEnvelopeID(record CapturedEventRecord, event *pb.Event) string {
    if event == nil {
        return ""
    }

    // Kernel provenance is stable across persistence/replay and across
    // userspace scheduling delays. Prefer it whenever an explicit generation
    // and CPU-local sequence are available.
    if event.GetKernelAuditGeneration() != 0 && event.GetKernelSequence() != 0 {
        parts := []string{
            "kernel-v2",
            fmt.Sprintf("%d", event.GetKernelAuditGeneration()),
            strconvFormatUint32(event.GetKernelCpu()),
            fmt.Sprintf("%d", event.GetKernelSequence()),
            fmt.Sprintf("%d", event.GetKernelTimestampNs()),
            event.GetType(),
            strconvFormatUint32(event.GetPid()),
            strconvFormatUint32(tgidOrPid(event)),
        }
        sum := sha256.Sum256([]byte(strings.Join(parts, "\\x00")))
        return "evt_" + hex.EncodeToString(sum[:12])
    }

    timestamp := record.ReceivedAt.UTC()
    if timestamp.IsZero() {
        timestamp = time.Unix(0, 0).UTC()
    }
    parts := []string{
        strconvFormatInt(timestamp.UnixNano()),
        DetermineEnvelopeSource(event),
        event.GetType(),
        strconvFormatUint32(event.GetPid()),
        strconvFormatUint32(event.GetPpid()),
        event.GetComm(),
        event.GetPath(),
        event.GetNetEndpoint(),
        event.GetTraceId(),
        event.GetToolCallId(),
        event.GetDecision(),
        event.GetExtraInfo(),
    }
    sum := sha256.Sum256([]byte(strings.Join(parts, "\\x00")))
    return "evt_" + hex.EncodeToString(sum[:12])
}

func buildExecEnvelopePayload'''
text2, n = pattern.subn(replacement, text, count=1)
if n != 1:
    raise SystemExit(f"{path}: failed to replace buildEventEnvelopeID")
write(path, text2)

path = "backend/app/events/envelopeevent_test.go"
text = read(path)
text += '''

func TestKernelEnvelopeIDStableAcrossUserspaceIngestTimes(t *testing.T) {
    base := &pb.Event{
        Pid: 321, Tgid: 321, Type: "openat", Comm: "python",
        KernelTimestampNs: 9000,
        KernelSequence: 77,
        KernelCpu: 3,
        KernelAuditGeneration: 42,
        KernelDroppedSinceLast: 2,
        KernelReserveFailuresTotal: 9,
    }
    first := NormalizeCapturedEventRecord(CapturedEventRecord{
        ReceivedAt: time.Unix(100, 0).UTC(), Event: CloneProtoEvent(base),
    })
    second := NormalizeCapturedEventRecord(CapturedEventRecord{
        ReceivedAt: time.Unix(200, 0).UTC(), Event: CloneProtoEvent(base),
    })
    if first.Envelope.GetEventId() != second.Envelope.GetEventId() {
        t.Fatalf("kernel event ID changed with ingest time: %q != %q", first.Envelope.GetEventId(), second.Envelope.GetEventId())
    }
    if first.Envelope.GetKernelAuditGeneration() != 42 || first.Envelope.GetKernelDroppedSinceLast() != 2 || first.Envelope.GetKernelReserveFailuresTotal() != 9 {
        t.Fatalf("kernel audit provenance not propagated: %+v", first.Envelope)
    }

    changed := CloneProtoEvent(base)
    changed.KernelSequence++
    third := NormalizeCapturedEventRecord(CapturedEventRecord{
        ReceivedAt: time.Unix(100, 0).UTC(), Event: changed,
    })
    if first.Envelope.GetEventId() == third.Envelope.GetEventId() {
        t.Fatal("distinct kernel sequence produced the same event ID")
    }
}
'''
write(path, text)

# ---------------------------------------------------------------------------
# Collector health: expose attempt sequence, pending kernel-reported drops,
# generation consistency, and attribution counters.
# ---------------------------------------------------------------------------
path = "backend/app/observability/metrics_collector.go"
replace_once(path,
"""type bpfCollectorStats struct {
\tRingbufEventsTotal        uint64
\tRingbufReserveFailedTotal uint64
\tEventSequence             uint64
}
""",
"""type bpfCollectorStats struct {
\tRingbufEventsTotal        uint64
\tRingbufReserveFailedTotal uint64
\tEventSequence             uint64
\tPendingDroppedEvents      uint64
\tAuditGeneration           uint64
}
""")
replace_once(path,
"""\tKernelSequencedEventsTotal     uint64            `json:\"kernelSequencedEventsTotal\"`
\tKernelCaptureDelaySamples      uint64            `json:\"kernelCaptureDelaySamples\"`
""",
"""\tKernelSequencedEventsTotal     uint64            `json:\"kernelSequencedEventsTotal\"`
\tKernelSequenceAttemptsTotal    uint64            `json:\"kernelSequenceAttemptsTotal\"`
\tKernelPendingDroppedEvents     uint64            `json:\"kernelPendingDroppedEvents\"`
\tKernelAuditGeneration          uint64            `json:\"kernelAuditGeneration\"`
\tKernelAuditGenerationConsistent bool             `json:\"kernelAuditGenerationConsistent\"`
\tKernelReportedReserveGapEvents uint64            `json:\"kernelReportedReserveGapEventsTotal\"`
\tKernelReportedReserveDropped   uint64            `json:\"kernelReportedReserveDroppedEventsTotal\"`
\tKernelSequenceUnexplainedMissing uint64          `json:\"kernelSequenceUnexplainedMissingEventsTotal\"`
\tKernelCaptureDelaySamples      uint64            `json:\"kernelCaptureDelaySamples\"`
""")
replace_once(path,
"""func (s *collectorMetricsState) snapshot() CollectorHealthResponse {
\tbpfStats, mapAvailable := loadCollectorStatsSnapshot()
\traw := s.rawSnapshot()
""",
"""func (s *collectorMetricsState) snapshot() CollectorHealthResponse {
\tbpfStats, mapAvailable, generationConsistent := loadCollectorStatsSnapshot()
\traw := s.rawSnapshot()
""")
replace_once(path,
"""\t\tKernelSequencedEventsTotal:     bpfStats.EventSequence,
\t\tKernelCaptureDelaySamples:      raw.KernelCaptureDelaySamples,
""",
"""\t\tKernelSequencedEventsTotal:       bpfStats.RingbufEventsTotal,
\t\tKernelSequenceAttemptsTotal:      bpfStats.EventSequence,
\t\tKernelPendingDroppedEvents:       bpfStats.PendingDroppedEvents,
\t\tKernelAuditGeneration:            bpfStats.AuditGeneration,
\t\tKernelAuditGenerationConsistent:  generationConsistent,
\t\tKernelReportedReserveGapEvents:   raw.AgentSightCountersTotal[\"kernel_reported_reserve_gap_events\"],
\t\tKernelReportedReserveDropped:     raw.AgentSightCountersTotal[\"kernel_reported_reserve_dropped_events\"],
\t\tKernelSequenceUnexplainedMissing: raw.AgentSightCountersTotal[\"kernel_sequence_unexplained_missing_events\"],
\t\tKernelCaptureDelaySamples:        raw.KernelCaptureDelaySamples,
""")
replace_once(path,
"""\t\tCaptureHealthy:                 !mapAvailable || bpfStats.RingbufReserveFailedTotal == 0,
""",
"""\t\tCaptureHealthy: !mapAvailable || (bpfStats.RingbufReserveFailedTotal == 0 && generationConsistent && raw.AgentSightCountersTotal[\"kernel_sequence_unexplained_missing_events\"] == 0 && raw.AgentSightCountersTotal[\"kernel_sequence_out_of_order\"] == 0),
""")
old = """func loadCollectorStatsSnapshot() (bpfCollectorStats, bool) {
\tif deps.TrackerMaps == nil {
\t\treturn bpfCollectorStats{}, false
\t}
\tcollectorStatsMap := deps.TrackerMaps.GetCollectorStats()
\tif collectorStatsMap == nil {
\t\treturn bpfCollectorStats{}, false
\t}

\tcpuCount, err := ebpf.PossibleCPU()
\tif err != nil || cpuCount <= 0 {
\t\treturn bpfCollectorStats{}, false
\t}

\tvalues := make([]bpfCollectorStats, cpuCount)
\tkey := uint32(0)
\tif err := collectorStatsMap.Lookup(&key, &values); err != nil {
\t\treturn bpfCollectorStats{}, false
\t}

\tvar total bpfCollectorStats
\tfor _, value := range values {
\t\ttotal.RingbufEventsTotal += value.RingbufEventsTotal
\t\ttotal.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal
\t\ttotal.EventSequence += value.EventSequence
\t}
\treturn total, true
}
"""
new = """func loadCollectorStatsSnapshot() (bpfCollectorStats, bool, bool) {
\tif deps.TrackerMaps == nil {
\t\treturn bpfCollectorStats{}, false, true
\t}
\tcollectorStatsMap := deps.TrackerMaps.GetCollectorStats()
\tif collectorStatsMap == nil {
\t\treturn bpfCollectorStats{}, false, true
\t}

\tcpuCount, err := ebpf.PossibleCPU()
\tif err != nil || cpuCount <= 0 {
\t\treturn bpfCollectorStats{}, false, true
\t}

\tvalues := make([]bpfCollectorStats, cpuCount)
\tkey := uint32(0)
\tif err := collectorStatsMap.Lookup(&key, &values); err != nil {
\t\treturn bpfCollectorStats{}, false, true
\t}

\tvar total bpfCollectorStats
\tgenerationConsistent := true
\tfor _, value := range values {
\t\ttotal.RingbufEventsTotal += value.RingbufEventsTotal
\t\ttotal.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal
\t\ttotal.EventSequence += value.EventSequence
\t\ttotal.PendingDroppedEvents += value.PendingDroppedEvents
\t\tif value.AuditGeneration != 0 {
\t\t\tif total.AuditGeneration == 0 {
\t\t\t\ttotal.AuditGeneration = value.AuditGeneration
\t\t\t} else if total.AuditGeneration != value.AuditGeneration {
\t\t\t\tgenerationConsistent = false
\t\t\t}
\t\t}
\t}
\treturn total, true, generationConsistent
}
"""
replace_once(path, old, new)

# Prometheus metrics for the new kernel provenance layer.
path = "backend/app/observability/metrics_prometheus.go"
replace_once(path,
"""\twritePrometheusHeader(&b, \"agent_ebpf_ringbuf_copy_decode_total\", \"counter\", \"Total eBPF ring buffer samples decoded through the endian/alignment-safe copy fallback path.\")
\twritePrometheusSample(&b, \"agent_ebpf_ringbuf_copy_decode_total\", nil, float64(health.RingbufCopyDecodeTotal))
""",
"""\twritePrometheusHeader(&b, \"agent_ebpf_ringbuf_copy_decode_total\", \"counter\", \"Total eBPF ring buffer samples decoded through the endian/alignment-safe copy fallback path.\")
\twritePrometheusSample(&b, \"agent_ebpf_ringbuf_copy_decode_total\", nil, float64(health.RingbufCopyDecodeTotal))
\twritePrometheusHeader(&b, \"agent_ebpf_kernel_sequence_attempts_total\", \"counter\", \"CPU-local kernel audit sequence attempts, including ringbuf reserve failures.\")
\twritePrometheusSample(&b, \"agent_ebpf_kernel_sequence_attempts_total\", nil, float64(health.KernelSequenceAttemptsTotal))
\twritePrometheusHeader(&b, \"agent_ebpf_kernel_pending_dropped_events\", \"gauge\", \"Kernel reserve failures not yet reported by a subsequent successful event.\")
\twritePrometheusSample(&b, \"agent_ebpf_kernel_pending_dropped_events\", nil, float64(health.KernelPendingDroppedEvents))
\twritePrometheusHeader(&b, \"agent_ebpf_kernel_reported_reserve_dropped_total\", \"counter\", \"Ringbuf reserve drops self-reported in event audit provenance.\")
\twritePrometheusSample(&b, \"agent_ebpf_kernel_reported_reserve_dropped_total\", nil, float64(health.KernelReportedReserveDropped))
\twritePrometheusHeader(&b, \"agent_ebpf_kernel_sequence_unexplained_missing_total\", \"counter\", \"Sequence holes not explained by kernel ringbuf reserve failures.\")
\twritePrometheusSample(&b, \"agent_ebpf_kernel_sequence_unexplained_missing_total\", nil, float64(health.KernelSequenceUnexplainedMissing))
\twritePrometheusHeader(&b, \"agent_ebpf_kernel_audit_generation_consistent\", \"gauge\", \"Whether all per-CPU collector slots report the same audit generation.\")
\tif health.KernelAuditGenerationConsistent {
\t\twritePrometheusSample(&b, \"agent_ebpf_kernel_audit_generation_consistent\", nil, 1)
\t} else {
\t\twritePrometheusSample(&b, \"agent_ebpf_kernel_audit_generation_consistent\", nil, 0)
\t}
""")
replace_once(path,
"""\twritePrometheusHeader(&b, \"agent_ebpf_capture_healthy\", \"gauge\", \"Whether capture currently reports no ring buffer drops.\")
""",
"""\twritePrometheusHeader(&b, \"agent_ebpf_capture_healthy\", \"gauge\", \"Whether capture reports no kernel reserve loss, unexplained sequence loss, out-of-order samples, or generation divergence.\")
""")

# ---------------------------------------------------------------------------
# Explicit kernel audit generation rotation during tracker bootstrap.
# ---------------------------------------------------------------------------
path = "backend/app/runtime_ebpf.go"
replace_once(path,
"""import (
\t\"agent-ebpf-filter/app/platform\"
\tbpf \"agent-ebpf-filter/ebpf\"
\t\"errors\"
""",
"""import (
\t\"agent-ebpf-filter/app/platform\"
\tbpf \"agent-ebpf-filter/ebpf\"
\t\"crypto/rand\"
\t\"encoding/binary\"
\t\"errors\"
""")
replace_once(path,
"""\t\tif err := bpf.LoadAgentTrackerObjects(&objs, &ebpf.CollectionOptions{MapReplacements: replacements}); err == nil {
\t\t\tdefer objs.Close()
\t\t\t_ = os.RemoveAll(ebpfPinLinksDir)
\t\t\t_ = os.MkdirAll(ebpfPinLinksDir, 0755)
\t\t\tif err := pinLinks(&objs); err != nil {
""",
"""\t\tif err := bpf.LoadAgentTrackerObjects(&objs, &ebpf.CollectionOptions{MapReplacements: replacements}); err == nil {
\t\t\tdefer objs.Close()
\t\t\t_ = os.RemoveAll(ebpfPinLinksDir)
\t\t\t_ = os.MkdirAll(ebpfPinLinksDir, 0755)
\t\t\tgeneration, err := rotateKernelAuditGeneration(objs.CollectorStats)
\t\t\tif err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\tif err := pinLinks(&objs); err != nil {
""")
replace_once(path,
"""\t\t\tif err := ensurePinnedMapPermissions(); err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\treturn replacements, nil
""",
"""\t\t\tif err := ensurePinnedMapPermissions(); err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\tlog.Printf(\"[INFO] kernel audit generation rotated: %016x\", generation)
\t\t\treturn replacements, nil
""")
replace_once(path,
"""\tvar objs bpf.AgentTrackerObjects
\tif err := bpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
\t\treturn nil, fmt.Errorf(\"load eBPF objects: %w\", err)
\t}
\tdefer objs.Close()
\tif err := pinMaps(&objs); err != nil {
""",
"""\tvar objs bpf.AgentTrackerObjects
\tif err := bpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
\t\treturn nil, fmt.Errorf(\"load eBPF objects: %w\", err)
\t}
\tdefer objs.Close()
\tgeneration, err := rotateKernelAuditGeneration(objs.CollectorStats)
\tif err != nil {
\t\treturn nil, err
\t}
\tif err := pinMaps(&objs); err != nil {
""")
replace_once(path,
"""\tif err := ensurePinnedMapPermissions(); err != nil {
\t\treturn nil, err
\t}
\treturn loadPinnedMapHandles()
}

func pinMaps(objs *bpf.AgentTrackerObjects) error {
""",
"""\tif err := ensurePinnedMapPermissions(); err != nil {
\t\treturn nil, err
\t}
\tlog.Printf(\"[INFO] kernel audit generation initialized: %016x\", generation)
\treturn loadPinnedMapHandles()
}

func newKernelAuditGeneration() (uint64, error) {
\tvar seed [8]byte
\tif _, err := rand.Read(seed[:]); err != nil {
\t\treturn 0, fmt.Errorf(\"generate kernel audit generation: %w\", err)
\t}
\tgeneration := binary.LittleEndian.Uint64(seed[:])
\tif generation == 0 {
\t\tgeneration = 1
\t}
\treturn generation, nil
}

func applyKernelAuditGeneration(values []bpf.AgentTrackerCollectorStats, generation uint64) {
\tfor i := range values {
\t\t// Keep cumulative successful/reserve-failure counters across a compatible
\t\t// program reload, but start sequence continuity in an explicit new epoch.
\t\tvalues[i].EventSequence = 0
\t\tvalues[i].PendingDroppedEvents = 0
\t\tvalues[i].AuditGeneration = generation
\t}
}

func rotateKernelAuditGeneration(stats *ebpf.Map) (uint64, error) {
\tif stats == nil {
\t\treturn 0, errors.New(\"collector_stats map is nil\")
\t}
\tgeneration, err := newKernelAuditGeneration()
\tif err != nil {
\t\treturn 0, err
\t}
\tcpuCount, err := ebpf.PossibleCPU()
\tif err != nil || cpuCount <= 0 {
\t\treturn 0, fmt.Errorf(\"discover possible CPUs for audit generation: %w\", err)
\t}
\tvalues := make([]bpf.AgentTrackerCollectorStats, cpuCount)
\tkey := uint32(0)
\tif err := stats.Lookup(&key, &values); err != nil {
\t\treturn 0, fmt.Errorf(\"read collector_stats before audit generation rotation: %w\", err)
\t}
\tapplyKernelAuditGeneration(values, generation)
\tif err := stats.Update(&key, values, ebpf.UpdateAny); err != nil {
\t\treturn 0, fmt.Errorf(\"write kernel audit generation: %w\", err)
\t}
\treturn generation, nil
}

func pinMaps(objs *bpf.AgentTrackerObjects) error {
""")

write("backend/app/runtime_ebpf_audit_test.go", '''package app

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
''')

# ---------------------------------------------------------------------------
# Main tracker kernel verifier smoke check.
# ---------------------------------------------------------------------------
write("backend/cmd/agenttracker-verifier-check/main.go", '''package main

import (
    "fmt"
    "os"

    bpf "agent-ebpf-filter/ebpf"
    "github.com/cilium/ebpf"
    "github.com/cilium/ebpf/rlimit"
)

func main() {
    if err := rlimit.RemoveMemlock(); err != nil {
        fmt.Fprintf(os.Stderr, "remove memlock: %v\\n", err)
        os.Exit(1)
    }
    spec, err := bpf.LoadAgentTracker()
    if err != nil {
        fmt.Fprintf(os.Stderr, "load tracker spec: %v\\n", err)
        os.Exit(1)
    }
    collection, err := ebpf.NewCollection(spec)
    if err != nil {
        fmt.Fprintf(os.Stderr, "kernel verifier rejected tracker: %v\\n", err)
        os.Exit(1)
    }
    collection.Close()
}
''')

path = ".github/workflows/bpf-ts-smoke.yml"
replace_once(path,
"""      - name: Verify generated repository eBPF bindings are committed
        working-directory: .
        run: git diff --exit-code -- backend/ebpf/*_bpf*.go
      - name: Install compiler dependencies
""",
"""      - name: Verify generated repository eBPF bindings are committed
        working-directory: .
        run: git diff --exit-code -- backend/ebpf/*_bpf*.go
      - name: Verify main agent tracker with kernel verifier
        working-directory: backend
        run: |
          go build -o /tmp/agenttracker-verifier-check ./cmd/agenttracker-verifier-check
          sudo /tmp/agenttracker-verifier-check
      - name: Install compiler dependencies
""")

# ---------------------------------------------------------------------------
# Documentation.
# ---------------------------------------------------------------------------
path = "docs/backend/capture-timing-audit.md"
text = read(path)
text += '''

## Kernel loss provenance and explicit tracker generations

The main tracker now assigns the CPU-local audit sequence **before**
`bpf_ringbuf_reserve()`. A reserve failure therefore consumes a sequence number;
the next successful event exposes that failure as a real sequence hole instead
of leaving userspace to infer pressure only from an aggregate map counter.

Each per-CPU `collector_stats` slot also maintains:

- `pending_dropped_events`: reserve failures not yet acknowledged by a successful event;
- `audit_generation`: a random non-zero generation rotated by privileged bootstrap whenever tracker programs are reloaded/re-attached.

A successful event carries three append-only provenance fields:

- `kernel_audit_generation`;
- `kernel_dropped_since_last`;
- `kernel_reserve_failures_total`.

`AUDIT_FLAG_RESERVE_GAP`, `AUDIT_FLAG_GENERATION`, and
`AUDIT_FLAG_RESERVE_TOTAL` state which values are authoritative. This lets the
backend classify a sequence hole as kernel-ringbuf-explained versus unexplained
(userspace/ring-reader/decode loss), and makes tracker reloads explicit rather
than relying on sequence/timestamp heuristics.

The event ID path now prefers `(generation, cpu, sequence, kernel_timestamp_ns)`
when available, so persistence/replay or userspace scheduling delays do not
change the forensic identity of the same kernel event.

The repository additionally guards this ABI in three places: C `_Static_assert`
size checks, a Go `unsafe.Sizeof/Offsetof` test, and CI that both regenerates the
committed bpf2go bindings and loads the main tracker through the host kernel BPF
verifier.
'''
write(path, text)
