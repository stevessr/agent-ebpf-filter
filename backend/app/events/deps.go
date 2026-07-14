package events

import (
	"context"
	"net"
	"time"

	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/internal/protocoldetect"
	"agent-ebpf-filter/pb"

	"github.com/gorilla/websocket"
)

// ── Type re-exports (same aliases as the parent app package) ────────────

type BpfEvent = core.BpfEvent
type IPScope = network.IPScope
type FlowKey = network.FlowKey
type AppProtocol = protocoldetect.AppProtocol
type CapturedEventRecord = core.CapturedEventRecord
type RuntimeSettings = core.RuntimeSettings
type KernelRiskFeedbackSettings = core.KernelRiskFeedbackSettings

// ProtoDetectionEntry mirrors the protoDetectionEntry type in the parent app
// package so the events subpackage does not need to import app.
type ProtoDetectionEntry struct {
	AppProtocol AppProtocol
	SNI         string
	ALPN        string
	HTTPHost    string
	HTTPMethod  string
}

// CgroupAttributionEntry is used by context_event.go for cgroup-to-agent-run mapping.
type CgroupAttributionEntry struct {
	CgroupID     uint64
	AgentRunID   string
	TaskID       string
	ToolCallID   string
	RootAgentPID uint32
	CreatedAt    time.Time
}

// CollectorMetricsStore provides metrics recording for the events subpackage.
type CollectorMetricsStore interface {
	RecordKernelRiskDecision(decision string, elapsed time.Duration)
	RecordKernelRiskFeedback(success bool, err error)
}

// ── Dependency injection ───────────────────────────────────────────────

// Deps holds all dependencies injected by the parent app package at init
// time. Every field must be set before any event processing begins.
var Deps struct {
	// Network/event processing closures (used by events_network.go, event_flows.go)
	GetTagName                           func(id uint32) string
	SyscallName                          func(nr uint32) string
	ApplyBestEffortProcessContextToEvent func(event *pb.Event)
	RecordNetworkFlowContextFromEvent    func(srcIP, dstIP string, srcPort, dstPort uint32, event *pb.Event, state string)
	DetectAndRecordProtocol              func(dstIP string, dstPort uint32, data []byte) *ProtoDetectionEntry
	ApplyKernelRiskDecision              func(raw *BpfEvent, event *pb.Event)
	MakeFlowKey                          func(srcIP, dstIP string, srcPort, dstPort uint32, protocol string) FlowKey
	LookupServiceByPort                  func(port uint32) string
	ClassifyIPScope                      func(ip net.IP) IPScope
	DetectAppProtocol                    func(port uint32, domain string) string

	// Global-object method closures (bandwidth, TCP tracker, flow aggregator, DNS)
	BandwidthTrackerRecordBytes         func(srcIP, dstIP string, dstPort uint32, protocol, direction string, bytes uint64, comm string, pid uint32)
	TCPTrackerRecordConnect             func(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string)
	TCPTrackerRecordClose               func(srcIP, dstIP string, srcPort, dstPort uint32)
	TCPTrackerRecordStateChange         func(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string)
	FlowAggregatorApplyProtocolMetadata func(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, entry *ProtoDetectionEntry)
	DNSCorrelationLookupIP              func(ip string) (string, bool)

	// Graph execution / envelope event dependencies
	Upgrader                    *websocket.Upgrader
	ReadCapturedEvents          func(path string, limit int) ([]CapturedEventRecord, error)
	ReadCapturedEventsContext   func(context.Context, string, int) ([]CapturedEventRecord, error)
	RuntimeSettingsRecentEvents func(limit int) ([]CapturedEventRecord, string, error)
	RuntimeSettingsSnapshot     func() RuntimeSettings
	CollectorMetrics            CollectorMetricsStore
	StringsTrimDefault          func(value, fallback string) string

	// Kernel-risk feedback enforcement closures
	BlockIP          func(ipStr string) error
	BlockPort        func(port uint16) error
	BlockLsmFileName func(name string) error
	BlockLsmExecPath func(path string) error
	BlockLsmExecName func(name string) error

	// Process context / cgroup attribution (used by context_event.go)
	ProcessContexts         *ProcessContextStore
	CgroupAttributionEnrich func(cgroupID uint64) (agentRunID, taskID, toolCallID string)
	CgroupAttributionSet    func(cgroupID uint64, entry CgroupAttributionEntry)
	ToolBaselineRecord      func(toolName, comm, eventType, path string)

	// Semantic alerts (used by alerts_semantic.go, alertsdetectsemantic.go)
	SemanticAlertsState     *SemanticAlertState
	ToolBaselineDetectDrift func(toolName, comm, eventType string) (string, bool)

	// Event schema version (used by alerts_semantic.go)
	EventSchemaVersion string
}
