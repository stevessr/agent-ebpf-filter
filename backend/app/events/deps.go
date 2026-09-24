package events

import (
	"context"
	"strings"
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

// NetworkSink receives the network side effects of kernel event decoding:
// TCP state, bandwidth, flow context, protocol detection and DNS correlation.
// The app package implements it over its network manager; tests use no-op or
// recording implementations.
type NetworkSink interface {
	RecordBandwidthBytes(srcIP, dstIP string, dstPort uint32, protocol, direction string, bytes uint64, comm string, pid uint32)
	RecordTCPConnect(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string)
	RecordTCPClose(srcIP, dstIP string, srcPort, dstPort uint32)
	RecordTCPStateChange(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string)
	// RecordFlowContext attaches event to the flow identified by the tuple.
	RecordFlowContext(srcIP, dstIP string, srcPort, dstPort uint32, event *pb.Event, state string)
	// ApplyFlowProtocolMetadata records a protocol detection result on a flow.
	ApplyFlowProtocolMetadata(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, entry *ProtoDetectionEntry)
	// DetectAndRecordProtocol fingerprints a captured payload. data is a view
	// into the ring-buffer sample and must not be retained.
	DetectAndRecordProtocol(dstIP string, dstPort uint32, data []byte) *ProtoDetectionEntry
	// LookupDNS resolves an IP back to a recently queried domain.
	LookupDNS(ip string) (string, bool)
}

// NoopNetworkSink discards every network side effect.
type NoopNetworkSink struct{}

func (NoopNetworkSink) RecordBandwidthBytes(string, string, uint32, string, string, uint64, string, uint32) {
}
func (NoopNetworkSink) RecordTCPConnect(string, string, uint32, uint32, uint32, string) {}
func (NoopNetworkSink) RecordTCPClose(string, string, uint32, uint32)                   {}
func (NoopNetworkSink) RecordTCPStateChange(string, string, uint32, uint32, uint8, uint8, uint32, string) {
}
func (NoopNetworkSink) RecordFlowContext(string, string, uint32, uint32, *pb.Event, string) {}
func (NoopNetworkSink) ApplyFlowProtocolMetadata(string, string, uint32, uint32, string, *ProtoDetectionEntry) {
}
func (NoopNetworkSink) DetectAndRecordProtocol(string, uint32, []byte) *ProtoDetectionEntry {
	return nil
}
func (NoopNetworkSink) LookupDNS(string) (string, bool) { return "", false }

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

// NoopCollectorMetrics discards kernel-risk metrics; it is the default so
// the risk pass can run in tests without the observability store.
type NoopCollectorMetrics struct{}

func (NoopCollectorMetrics) RecordKernelRiskDecision(string, time.Duration) {}
func (NoopCollectorMetrics) RecordKernelRiskFeedback(bool, error)           {}

// KernelRiskEnforcer applies kernel-risk feedback actions. The app package
// implements it over the cgroup and LSM sandboxes.
type KernelRiskEnforcer interface {
	BlockIP(ip string) error
	BlockPort(port uint16) error
	BlockFileName(name string) error
	BlockExecPath(path string) error
	BlockExecName(name string) error
}

// ── Dependency injection ───────────────────────────────────────────────

// Deps holds all dependencies injected by the parent app package at init
// time. Every field must be set before any event processing begins.
var Deps struct {
	// Event processing closures (used by events_network.go, event_flows.go)
	GetTagName                           func(id uint32) string
	SyscallName                          func(nr uint32) string
	ApplyBestEffortProcessContextToEvent func(event *pb.Event)
	// KernelRisk is the kernel-risk pass run on every decoded kernel event.
	// It defaults to ApplyKernelRiskDecision; tests may replace it.
	KernelRisk func(raw *BpfEvent, event *pb.Event)

	// Network is where decoded network events report TCP state, bandwidth,
	// flow context, protocol detection and DNS correlation.
	Network NetworkSink

	// Graph execution / envelope event dependencies
	Upgrader                           *websocket.Upgrader
	ReadCapturedEvents                 func(path string, limit int) ([]CapturedEventRecord, error)
	ReadCapturedEventsContext          func(context.Context, string, int) ([]CapturedEventRecord, error)
	RuntimeSettingsRecentEvents        func(limit int) ([]CapturedEventRecord, string, error)
	RuntimeSettingsRecentEventsContext func(context.Context, int) ([]CapturedEventRecord, string, error)
	RuntimeSettingsSnapshot            func() RuntimeSettings
	// KernelRiskFeedbackGate answers the per-event "is feedback even on"
	// question without copying the full RuntimeSettings.
	KernelRiskFeedbackGate func() (policyManagement bool, feedback KernelRiskFeedbackSettings)
	CollectorMetrics       CollectorMetricsStore

	// Enforcer applies kernel-risk feedback actions.
	Enforcer KernelRiskEnforcer

	// Process context / cgroup attribution (used by context_event.go)
	ProcessContexts         *ProcessContextStore
	CgroupAttributionEnrich func(cgroupID uint64) (agentRunID, taskID, toolCallID string)
	CgroupAttributionSet    func(cgroupID uint64, entry CgroupAttributionEntry)

	// Semantic alerts (used by alerts_semantic.go, alertsdetectsemantic.go)
	SemanticAlertsState *SemanticAlertState
	ToolBaselineObserve func(toolName, comm, eventType string) (string, bool)

	// Event schema version (used by alerts_semantic.go)
	EventSchemaVersion string
}

func init() {
	Deps.KernelRisk = ApplyKernelRiskDecision
	Deps.CollectorMetrics = NoopCollectorMetrics{}
	Deps.Network = NoopNetworkSink{}
}

// trimDefault returns value without surrounding whitespace, or fallback when
// nothing is left.
func trimDefault(value, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
