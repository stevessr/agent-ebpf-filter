package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"sync"
	"time"
)

// ── Deps: external dependencies injected by app/main.go via Init() ─────────

type RuntimeSettingsStore interface {
	RecentEvents(limit int) ([]CapturedEventRecord, string, error)
	HookSecret(id string) string
	Snapshot() any
}

type ProcessContextStore interface {
	Get(pid uint32) (ProcessContext, bool)
	Set(pid uint32, ctx ProcessContext)
	Delete(pid uint32)
	Move(oldPID, newPID uint32) bool
}

type ProcessContext struct {
	RootAgentPid   uint32
	AgentRunID     string
	TaskID         string
	ConversationID string
	TurnID         string
	ToolCallID     string
	ToolName       string
	TraceID        string
	SpanID         string
	Decision       string
	ContainerID    string
	ArgvDigest     string
	Cwd            string
	RiskScore      float64
}

type CgroupAttributionStore interface {
	Get(cgroupID uint64) (CgroupAttributionEntry, bool)
	Set(cgroupID uint64, entry CgroupAttributionEntry)
}

type CgroupAttributionEntry struct {
	AgentRunID   string
	TaskID       string
	ToolCallID   string
	RootAgentPID uint32
}

type ToolBaselineStore interface {
	Record(toolName, comm, eventType, path string)
}

type NetworkFlowAggregator interface {
	ApplyProtocolMetadata(srcIP, dstIP string, srcPort, dstPort uint32, transport string, entry any)
}

type ProtoCacheStore interface {
	Lookup(host string, port uint32) any
}

type SemanticAlertState interface {
	RememberSecret(event *pb.Event, target string, now time.Time)
	RecentSecretTarget(event *pb.Event, now time.Time) (string, bool)
	RememberExecutable(event *pb.Event, path, mode string, now time.Time)
	RecentExecutablePath(key, path string, now time.Time) (string, bool)
	IncrementForkCount(event *pb.Event, now time.Time) int
	ObserveAgenticResourceLoop(event *pb.Event, now time.Time) (string, bool)
}

type EventRecordingStore interface {
	Status() any
	StartRecording() error
	StopRecording() error
	Append(record any) error
}

type CollectorMetricsStore interface {
	RecordKernelRiskDecision(decision string, elapsed time.Duration)
	RecordKernelRiskFeedback(success bool, err error)
}

type EventArchive interface {
	Add(record any)
	Snapshot(limit int) []any
	Count() int
}

type CapturedEventRecord struct {
	Event      *pb.Event
	ReceivedAt time.Time
}

type Deps struct {
	Broadcast             chan<- *pb.Event
	RuntimeSettings       RuntimeSettingsStore
	ProcessContexts       ProcessContextStore
	CgroupAttribution     CgroupAttributionStore
	ToolBaseline          ToolBaselineStore
	NetworkFlowAggregator NetworkFlowAggregator
	ProtoCache            ProtoCacheStore
	SemanticAlerts        SemanticAlertState
	EventRecording        EventRecordingStore
	CollectorMetrics      CollectorMetricsStore
	EventArchive          EventArchive
	BandwidthTracker      any
	DNSCorrelation        any
	TCPTracker            any
	Upgrader              any
}

var deps Deps

func Init(d Deps) { deps = d }

// ── Re-export types from app package ───────────────────────────────────────

type CgroupAttribution = CgroupAttributionEntry

// ── Utility functions (moved inline) ───────────────────────────────────────

var eventSchemaVersion = "event.v3"

func sanitizeUTF8(b []byte) string {
	return strings.ToValidUTF8(strings.TrimRight(string(b), "\x00"), "")
}

func getTagName(tagID uint32) string {
	return "AI Agent" // simplified; full logic stays in app package for now
}

func classifyIPScope(ip any) string {
	return "public" // simplified; full logic stays in app/network for now
}