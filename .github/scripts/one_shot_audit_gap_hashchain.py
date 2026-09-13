from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, text):
    Path(path).write_text(text)


def replace_once(path, old, new):
    text = read(path)
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one match, got {count}: {old[:160]!r}")
    write(path, text.replace(old, new, 1))


# CapturedEventRecord carries persistence-level audit chain metadata.
replace_once(
    "backend/core/state_types.go",
    '''type CapturedEventRecord struct {\n\tReceivedAt time.Time         `json:"receivedAt"`\n\tEvent      *pb.Event         `json:"event"`\n\tEnvelope   *pb.EventEnvelope `json:"-"`\n}\n''',
    '''type CapturedEventRecord struct {\n\tReceivedAt       time.Time         `json:"receivedAt"`\n\tEvent            *pb.Event         `json:"event"`\n\tEnvelope         *pb.EventEnvelope `json:"-"`\n\tAuditChainVersion string            `json:"auditChainVersion,omitempty"`\n\tAuditChainID      string            `json:"auditChainId,omitempty"`\n\tAuditSequence     uint64            `json:"auditSequence,omitempty"`\n\tAuditPrevHash     string            `json:"auditPrevHash,omitempty"`\n\tAuditHash         string            `json:"auditHash,omitempty"`\n}\n''',
)

# Efficient N-delta counters reuse the existing bounded AgentSight counter map.
replace_once(
    "backend/app/observability/metrics_collector.go",
    '''func RecordAgentSightCounter(name string) {\n\tcollectorMetricsStore.recordAgentSightCounter(name)\n}\n\nfunc (s *collectorMetricsState) recordAgentSightCounter(name string) {\n\tname = StringsTrimDefault(name, "unknown")\n\ts.mu.Lock()\n\ts.agentSightCountersTotal[name]++\n\ts.mu.Unlock()\n}\n''',
    '''func RecordAgentSightCounter(name string) {\n\tcollectorMetricsStore.recordAgentSightCounterN(name, 1)\n}\n\nfunc RecordAgentSightCounterN(name string, delta uint64) {\n\tcollectorMetricsStore.recordAgentSightCounterN(name, delta)\n}\n\nfunc (s *collectorMetricsState) recordAgentSightCounter(name string) {\n\ts.recordAgentSightCounterN(name, 1)\n}\n\nfunc (s *collectorMetricsState) recordAgentSightCounterN(name string, delta uint64) {\n\tif delta == 0 {\n\t\treturn\n\t}\n\tname = StringsTrimDefault(name, "unknown")\n\ts.mu.Lock()\n\ts.agentSightCountersTotal[name] += delta\n\ts.mu.Unlock()\n}\n''',
)

replace_once(
    "backend/app/collectormetricsbridge.go",
    '''func (metricsStoreBridge) RecordAgentSightCounter(name string) {\n\tobservability.RecordAgentSightCounter(name)\n}\n''',
    '''func (metricsStoreBridge) RecordAgentSightCounter(name string) {\n\tobservability.RecordAgentSightCounter(name)\n}\n\nfunc (metricsStoreBridge) RecordAgentSightCounterN(name string, delta uint64) {\n\tobservability.RecordAgentSightCounterN(name, delta)\n}\n''',
)

# Hook sequence continuity analysis immediately after raw provenance is copied.
replace_once(
    "backend/app/kernelriskbridge.go",
    '''\tevent.AuditFlags = raw.AuditFlags\n\tcollectorMetricsStore.RecordKernelCaptureTiming(observation.DelayNS, observation.Clock)\n''',
    '''\tevent.AuditFlags = raw.AuditFlags\n\trecordKernelSequenceObservation(raw)\n\tcollectorMetricsStore.RecordKernelCaptureTiming(observation.DelayNS, observation.Clock)\n''',
)

# Recording state exposes chain status and uses chained marshaling.
replace_once(
    "backend/app/recording/recording_event.go",
    '''\tLastFlushedAt string `json:"lastFlushedAt,omitempty"`\n\tLastError     string `json:"lastError,omitempty"`\n}\n''',
    '''\tLastFlushedAt    string `json:"lastFlushedAt,omitempty"`\n\tLastError        string `json:"lastError,omitempty"`\n\tAuditChainVersion string `json:"auditChainVersion,omitempty"`\n\tAuditChainID      string `json:"auditChainId,omitempty"`\n\tAuditSequence     uint64 `json:"auditSequence,omitempty"`\n\tAuditLastHash     string `json:"auditLastHash,omitempty"`\n}\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''\tterminalErr error\n}\n''',
    '''\tterminalErr error\n\tauditChain  *AuditChain\n}\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''\tif err := ctx.Err(); err != nil {\n\t\t_ = file.Close()\n\t\treturn stopStatus, err\n\t}\n\n\tqueue := make(chan CapturedEventRecord, eventRecordingQueueSize)\n''',
    '''\tif err := ctx.Err(); err != nil {\n\t\t_ = file.Close()\n\t\treturn stopStatus, err\n\t}\n\tauditChain, err := NewAuditChain()\n\tif err != nil {\n\t\t_ = file.Close()\n\t\treturn Status{}, fmt.Errorf("initialize audit chain: %w", err)\n\t}\n\n\tqueue := make(chan CapturedEventRecord, eventRecordingQueueSize)\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''\ts.terminalErr = nil\n\tstatus := s.statusLocked()\n''',
    '''\ts.terminalErr = nil\n\ts.auditChain = auditChain\n\tstatus := s.statusLocked()\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''\tgo s.runGeneration(file, info.Size(), queue, stopCh, done)\n''',
    '''\tgo s.runGeneration(file, info.Size(), queue, stopCh, done, auditChain)\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''\tif !s.lastFlushAt.IsZero() {\n\t\tstatus.LastFlushedAt = s.lastFlushAt.UTC().Format(time.RFC3339Nano)\n\t}\n\treturn status\n}\n''',
    '''\tif !s.lastFlushAt.IsZero() {\n\t\tstatus.LastFlushedAt = s.lastFlushAt.UTC().Format(time.RFC3339Nano)\n\t}\n\tif s.auditChain != nil {\n\t\tchain := s.auditChain.Status()\n\t\tstatus.AuditChainVersion = chain.Version\n\t\tstatus.AuditChainID = chain.ChainID\n\t\tstatus.AuditSequence = chain.Sequence\n\t\tstatus.AuditLastHash = chain.LastHash\n\t}\n\treturn status\n}\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''\tdone chan struct{},\n) {\n''',
    '''\tdone chan struct{},\n\tauditChain *AuditChain,\n) {\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''\tprocess := func(record CapturedEventRecord) error {\n\t\tpayload, err := MarshalRecord(record)\n''',
    '''\tprocess := func(record CapturedEventRecord) error {\n\t\tpayload, err := auditChain.MarshalRecord(record)\n''',
)
replace_once(
    "backend/app/recording/recording_event.go",
    '''func MarshalRecord(record CapturedEventRecord) ([]byte, error) {\n\tif record.Event == nil {\n\t\treturn nil, errors.New("event recording record has no event")\n\t}\n\trecord = events.NormalizeCapturedEventRecord(record)\n\tpayload, err := json.Marshal(record)\n''',
    '''func MarshalRecord(record CapturedEventRecord) ([]byte, error) {\n\tif record.Event == nil {\n\t\treturn nil, errors.New("event recording record has no event")\n\t}\n\trecord = events.NormalizeCapturedEventRecord(record)\n\treturn marshalNormalizedRecord(record)\n}\n\nfunc marshalNormalizedRecord(record CapturedEventRecord) ([]byte, error) {\n\tpayload, err := json.Marshal(record)\n''',
)

# Runtime event log uses the same efficient hash chain and reports its head.
replace_once(
    "backend/app/runtime_event_log_writer.go",
    '''\tLastFlushedAt  time.Time `json:"lastFlushedAt,omitempty"`\n\tLastError      string    `json:"lastError,omitempty"`\n}\n''',
    '''\tLastFlushedAt    time.Time `json:"lastFlushedAt,omitempty"`\n\tLastError        string    `json:"lastError,omitempty"`\n\tAuditChainVersion string   `json:"auditChainVersion,omitempty"`\n\tAuditChainID      string   `json:"auditChainId,omitempty"`\n\tAuditSequence     uint64   `json:"auditSequence,omitempty"`\n\tAuditLastHash     string   `json:"auditLastHash,omitempty"`\n}\n''',
)
replace_once(
    "backend/app/runtime_event_log_writer.go",
    '''\tstopRequested  bool\n}\n''',
    '''\tstopRequested  bool\n\tauditChain     *recording.AuditChain\n}\n''',
)
replace_once(
    "backend/app/runtime_event_log_writer.go",
    '''\tif _, err := file.Stat(); err != nil {\n\t\t_ = file.Close()\n\t\treturn nil, err\n\t}\n\twriter := &runtimeEventLogWriter{\n''',
    '''\tif _, err := file.Stat(); err != nil {\n\t\t_ = file.Close()\n\t\treturn nil, err\n\t}\n\tauditChain, err := recording.NewAuditChain()\n\tif err != nil {\n\t\t_ = file.Close()\n\t\treturn nil, err\n\t}\n\twriter := &runtimeEventLogWriter{\n''',
)
replace_once(
    "backend/app/runtime_event_log_writer.go",
    '''\t\taccepting:     true,\n\t}\n''',
    '''\t\taccepting:     true,\n\t\tauditChain:    auditChain,\n\t}\n''',
)
replace_once(
    "backend/app/runtime_event_log_writer.go",
    '''\treturn runtimeEventLogStatus{\n\t\tActive:         w.accepting,\n''',
    '''\tchain := recording.AuditChainStatus{}\n\tif w.auditChain != nil {\n\t\tchain = w.auditChain.Status()\n\t}\n\treturn runtimeEventLogStatus{\n\t\tActive:         w.accepting,\n''',
)
replace_once(
    "backend/app/runtime_event_log_writer.go",
    '''\t\tLastFlushedAt:  w.lastFlushedAt,\n\t\tLastError:      w.lastError,\n\t}\n''',
    '''\t\tLastFlushedAt:    w.lastFlushedAt,\n\t\tLastError:        w.lastError,\n\t\tAuditChainVersion: chain.Version,\n\t\tAuditChainID:      chain.ChainID,\n\t\tAuditSequence:     chain.Sequence,\n\t\tAuditLastHash:     chain.LastHash,\n\t}\n''',
)
replace_once(
    "backend/app/runtime_event_log_writer.go",
    '''\tprocessRecord := func(record CapturedEventRecord) error {\n\t\tstartedAt := time.Now()\n\t\tpayload, err := recording.MarshalRecord(record)\n''',
    '''\tprocessRecord := func(record CapturedEventRecord) error {\n\t\tstartedAt := time.Now()\n\t\tpayload, err := w.auditChain.MarshalRecord(record)\n''',
)

# Add kernel sequence continuity detector.
write("backend/app/kernel_sequence_audit.go", r'''package app

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
''')

write("backend/app/kernel_sequence_audit_test.go", r'''package app

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
''')

write("backend/app/recording/audit_chain.go", r'''package recording

import (
    "crypto/rand"
    "crypto/sha256"
    "encoding/binary"
    "encoding/hex"
    "errors"
    "fmt"
    "hash"
    "sync"

    "agent-ebpf-filter/app/events"
    "google.golang.org/protobuf/proto"
)

const AuditChainVersion = "sha256-envelope-v1"

var auditChainDomain = []byte("agent-ebpf-filter/audit-chain/v1\x00")

type AuditChainStatus struct {
    Version  string `json:"version"`
    ChainID  string `json:"chainId"`
    Sequence uint64 `json:"sequence"`
    LastHash string `json:"lastHash,omitempty"`
}

type AuditChain struct {
    mu       sync.RWMutex
    chainID  string
    sequence uint64
    lastHash [sha256.Size]byte
    hasLast  bool
}

func NewAuditChain() (*AuditChain, error) {
    seed := make([]byte, 16)
    if _, err := rand.Read(seed); err != nil {
        return nil, fmt.Errorf("generate audit chain id: %w", err)
    }
    return &AuditChain{chainID: hex.EncodeToString(seed)}, nil
}

func (c *AuditChain) Status() AuditChainStatus {
    if c == nil {
        return AuditChainStatus{}
    }
    c.mu.RLock()
    defer c.mu.RUnlock()
    status := AuditChainStatus{Version: AuditChainVersion, ChainID: c.chainID, Sequence: c.sequence}
    if c.hasLast {
        status.LastHash = hex.EncodeToString(c.lastHash[:])
    }
    return status
}

func (c *AuditChain) MarshalRecord(record CapturedEventRecord) ([]byte, error) {
    if c == nil {
        return MarshalRecord(record)
    }
    if record.Event == nil {
        return nil, errors.New("event recording record has no event")
    }
    record = events.NormalizeCapturedEventRecord(record)

    c.mu.Lock()
    defer c.mu.Unlock()
    sequence := c.sequence + 1
    previous := ""
    if c.hasLast {
        previous = hex.EncodeToString(c.lastHash[:])
    }
    digest, err := auditRecordHash(record, c.chainID, sequence, previous)
    if err != nil {
        return nil, err
    }
    record.AuditChainVersion = AuditChainVersion
    record.AuditChainID = c.chainID
    record.AuditSequence = sequence
    record.AuditPrevHash = previous
    record.AuditHash = hex.EncodeToString(digest[:])

    payload, err := marshalNormalizedRecord(record)
    if err != nil {
        return nil, err
    }
    c.sequence = sequence
    c.lastHash = digest
    c.hasLast = true
    return payload, nil
}

func auditRecordHash(record CapturedEventRecord, chainID string, sequence uint64, previous string) ([sha256.Size]byte, error) {
    record = events.NormalizeCapturedEventRecord(record)
    if record.Envelope == nil {
        return [sha256.Size]byte{}, errors.New("audit chain record has no envelope")
    }
    envelopeBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(record.Envelope)
    if err != nil {
        return [sha256.Size]byte{}, fmt.Errorf("marshal audit envelope: %w", err)
    }
    h := sha256.New()
    _, _ = h.Write(auditChainDomain)
    writeAuditPart(h, []byte(chainID))
    writeAuditUint64(h, sequence)
    writeAuditPart(h, []byte(previous))
    writeAuditUint64(h, uint64(record.ReceivedAt.UTC().UnixNano()))
    writeAuditPart(h, envelopeBytes)
    var out [sha256.Size]byte
    copy(out[:], h.Sum(nil))
    return out, nil
}

func writeAuditUint64(h hash.Hash, value uint64) {
    var buf [8]byte
    binary.BigEndian.PutUint64(buf[:], value)
    _, _ = h.Write(buf[:])
}

func writeAuditPart(h hash.Hash, value []byte) {
    writeAuditUint64(h, uint64(len(value)))
    _, _ = h.Write(value)
}

type AuditVerification struct {
    Valid          bool   `json:"valid"`
    CheckedRecords int    `json:"checkedRecords"`
    LegacyRecords  int    `json:"legacyRecords"`
    Segments       int    `json:"segments"`
    LastHash       string `json:"lastHash,omitempty"`
    ErrorIndex     int    `json:"errorIndex"`
    Error          string `json:"error,omitempty"`
}

func VerifyAuditChain(records []CapturedEventRecord) AuditVerification {
    result := AuditVerification{Valid: true, ErrorIndex: -1}
    var chainID string
    var sequence uint64
    var lastHash string
    active := false

    fail := func(index int, err error) AuditVerification {
        result.Valid = false
        result.ErrorIndex = index
        result.Error = err.Error()
        return result
    }

    for index, record := range records {
        if record.AuditChainVersion == "" {
            result.LegacyRecords++
            active = false
            chainID, sequence, lastHash = "", 0, ""
            continue
        }
        if record.AuditChainVersion != AuditChainVersion {
            return fail(index, fmt.Errorf("unsupported audit chain version %q", record.AuditChainVersion))
        }
        if record.AuditChainID == "" || record.AuditSequence == 0 || record.AuditHash == "" {
            return fail(index, errors.New("incomplete audit chain metadata"))
        }
        if record.AuditSequence == 1 {
            if record.AuditPrevHash != "" {
                return fail(index, errors.New("audit chain genesis has previous hash"))
            }
            result.Segments++
            chainID, sequence, lastHash = record.AuditChainID, 0, ""
            active = true
        } else {
            if !active || record.AuditChainID != chainID {
                return fail(index, errors.New("audit chain segment starts without genesis"))
            }
            if record.AuditSequence != sequence+1 {
                return fail(index, fmt.Errorf("audit sequence gap: got %d want %d", record.AuditSequence, sequence+1))
            }
            if record.AuditPrevHash != lastHash {
                return fail(index, errors.New("audit previous hash mismatch"))
            }
        }
        digest, err := auditRecordHash(record, record.AuditChainID, record.AuditSequence, record.AuditPrevHash)
        if err != nil {
            return fail(index, err)
        }
        expected := hex.EncodeToString(digest[:])
        if record.AuditHash != expected {
            return fail(index, errors.New("audit record hash mismatch"))
        }
        result.CheckedRecords++
        chainID = record.AuditChainID
        sequence = record.AuditSequence
        lastHash = record.AuditHash
        result.LastHash = lastHash
        active = true
    }
    return result
}
''')

write("backend/app/recording/audit_chain_test.go", r'''package recording

import (
    "encoding/json"
    "testing"
    "time"

    "agent-ebpf-filter/pb"
)

func TestAuditChainDetectsTamperAndDeletion(t *testing.T) {
    chain, err := NewAuditChain()
    if err != nil {
        t.Fatal(err)
    }
    records := make([]CapturedEventRecord, 0, 3)
    for i := 1; i <= 3; i++ {
        payload, err := chain.MarshalRecord(CapturedEventRecord{
            ReceivedAt: time.Unix(int64(i), 0).UTC(),
            Event:      &pb.Event{Pid: uint32(i), Type: "openat", Path: "/tmp/test"},
        })
        if err != nil {
            t.Fatal(err)
        }
        var record CapturedEventRecord
        if err := json.Unmarshal(payload, &record); err != nil {
            t.Fatal(err)
        }
        records = append(records, record)
    }
    if got := VerifyAuditChain(records); !got.Valid || got.CheckedRecords != 3 || got.Segments != 1 {
        t.Fatalf("valid chain verification = %+v", got)
    }

    tampered := append([]CapturedEventRecord(nil), records...)
    tampered[1].Event = pbCloneEventForAuditTest(tampered[1].Event)
    tampered[1].Event.Path = "/tmp/evil"
    if got := VerifyAuditChain(tampered); got.Valid || got.ErrorIndex != 1 {
        t.Fatalf("tampered chain verification = %+v", got)
    }

    deleted := []CapturedEventRecord{records[0], records[2]}
    if got := VerifyAuditChain(deleted); got.Valid || got.ErrorIndex != 1 {
        t.Fatalf("deleted record verification = %+v", got)
    }
}

func pbCloneEventForAuditTest(in *pb.Event) *pb.Event {
    if in == nil {
        return nil
    }
    out := *in
    return &out
}

func TestVerifyAuditChainAllowsLegacyRecordsButMarksThem(t *testing.T) {
    records := []CapturedEventRecord{{ReceivedAt: time.Unix(1, 0).UTC(), Event: &pb.Event{Pid: 1, Type: "execve"}}}
    got := VerifyAuditChain(records)
    if !got.Valid || got.LegacyRecords != 1 || got.CheckedRecords != 0 {
        t.Fatalf("legacy verification = %+v", got)
    }
}
''')

# Documentation: append concise operational semantics.
doc = read("docs/backend/capture-timing-audit.md")
doc += r'''

## Sequence continuity and tamper-evident persistence

Main-tracker audit ordering is checked per CPU using `(kernel_cpu, kernel_sequence)`.
The collector reports continuity findings through `agentSightCountersTotal`:

- `kernel_sequence_gap_events`: observed forward sequence jumps;
- `kernel_sequence_missing_events`: total sequence numbers skipped by those jumps;
- `kernel_sequence_resets`: sequence rewinds while kernel monotonic time advances, normally a tracker/map reload;
- `kernel_sequence_out_of_order`: stale/duplicate samples that must not rewind the live cursor.

JSONL event persistence now starts a random audit-chain segment for every writer generation.
Each persisted `CapturedEventRecord` carries `auditChainVersion`, `auditChainId`,
`auditSequence`, `auditPrevHash`, and `auditHash`. The SHA-256 digest binds the chain
metadata, receive timestamp, and a deterministic protobuf encoding of `EventEnvelope`.
This avoids a second JSON serialization in the hot persistence path while detecting
record mutation and deletion inside a chain segment. `VerifyAuditChain` validates loaded
records; legacy unchained records remain readable and are reported separately rather
than being silently described as verified.

The chain is tamper-evident, not a signature: an attacker able to rewrite the complete
file can recompute an unkeyed chain. For stronger external attestation, persist the
reported chain head in an independent trusted store or sign checkpoints.
'''
write("docs/backend/capture-timing-audit.md", doc)
