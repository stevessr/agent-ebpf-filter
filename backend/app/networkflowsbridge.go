package app

// networkflowsbridge.go — bridges between app/events subpackage and remaining
// callers in the app package (runtime__jobs_background.go, tests, etc.).
//
// This file also wires up the events.Deps struct at init time so that the
// events subpackage can call back into the app package for all shared state.

import (
	appnetwork "agent-ebpf-filter/app/network"
	"agent-ebpf-filter/pb"

	"agent-ebpf-filter/app/events"
)

var fallbackNetworkMetrics = appnetwork.NewManager()

// appNetworkSink routes the events package's network side effects to the
// AppContext network manager, falling back to the package-level trackers
// before the context is bound (early startup and unit tests).
type appNetworkSink struct{}

func (appNetworkSink) RecordBandwidthBytes(srcIP, dstIP string, dstPort uint32, protocol, direction string, byteCount uint64, comm string, pid uint32) {
	if manager := currentNetworkManager(); manager != nil {
		manager.RecordBandwidthBytes(srcIP, dstIP, dstPort, protocol, direction, byteCount, comm, pid)
		return
	}
	fallbackNetworkMetrics.RecordBandwidthBytes(srcIP, dstIP, dstPort, protocol, direction, byteCount, comm, pid)
}

func (appNetworkSink) RecordTCPConnect(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string) {
	if manager := currentNetworkManager(); manager != nil {
		manager.RecordTCPConnect(srcIP, dstIP, srcPort, dstPort, pid, comm)
		return
	}
	tcpTracker.RecordConnect(srcIP, dstIP, srcPort, dstPort, pid, comm)
}

func (appNetworkSink) RecordTCPClose(srcIP, dstIP string, srcPort, dstPort uint32) {
	if manager := currentNetworkManager(); manager != nil {
		manager.RecordTCPClose(srcIP, dstIP, srcPort, dstPort)
		return
	}
	tcpTracker.RecordClose(srcIP, dstIP, srcPort, dstPort)
}

func (appNetworkSink) RecordTCPStateChange(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string) {
	if manager := currentNetworkManager(); manager != nil {
		manager.RecordTCPStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, pid, comm)
		return
	}
	tcpTracker.RecordStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, pid, comm)
}

func (appNetworkSink) RecordFlowContext(srcIP, dstIP string, srcPort, dstPort uint32, event *pb.Event, state string) {
	recordNetworkFlowContextFromEvent(srcIP, dstIP, srcPort, dstPort, event, state)
}

func (appNetworkSink) ApplyFlowProtocolMetadata(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, entry *events.ProtoDetectionEntry) {
	aggregator := currentNetworkFlowAggregator()
	if entry == nil {
		aggregator.ApplyProtocolMetadata(srcIP, dstIP, srcPort, dstPort, protocol, nil)
		return
	}
	aggregator.ApplyProtocolMetadata(srcIP, dstIP, srcPort, dstPort, protocol, &protoDetectionEntry{
		AppProtocol: AppProtocol(entry.AppProtocol),
		SNI:         entry.SNI,
		ALPN:        entry.ALPN,
		HTTPHost:    entry.HTTPHost,
		HTTPMethod:  entry.HTTPMethod,
	})
}

func (appNetworkSink) DetectAndRecordProtocol(dstIP string, dstPort uint32, data []byte) *events.ProtoDetectionEntry {
	entry := detectAndRecordProtocol(dstIP, dstPort, data)
	if entry == nil {
		return nil
	}
	return &events.ProtoDetectionEntry{
		AppProtocol: events.AppProtocol(entry.AppProtocol),
		SNI:         entry.SNI,
		ALPN:        entry.ALPN,
		HTTPHost:    entry.HTTPHost,
		HTTPMethod:  entry.HTTPMethod,
	}
}

func (appNetworkSink) LookupDNS(ip string) (string, bool) {
	return currentDNSCorrelation().LookupIP(ip)
}

// sandboxEnforcer applies kernel-risk feedback through the cgroup and LSM
// sandboxes.
type sandboxEnforcer struct{}

func (sandboxEnforcer) BlockIP(ip string) error         { return blockIP(ip) }
func (sandboxEnforcer) BlockPort(port uint16) error     { return blockPort(port) }
func (sandboxEnforcer) BlockFileName(name string) error { return blockLsmFileName(name) }
func (sandboxEnforcer) BlockExecPath(path string) error { return blockLsmExecPath(path) }
func (sandboxEnforcer) BlockExecName(name string) error { return blockLsmExecName(name) }

// ── Bridge functions (called by remaining app-package code) ─────────────

// sanitizeUTF8 bridges to events.SanitizeUTF8. Both callers
// (runtime__jobs_background.go, tls__fragmentassemblertls.go) and the
// events subpackage use this same helper.
func sanitizeUTF8(b []byte) string { return events.SanitizeUTF8(b) }

func buildKernelEvent(event bpfEvent) *pb.Event {
	return events.BuildKernelEvent(events.BpfEvent(event))
}

func buildKernelEventFromRaw(event *bpfEvent) *pb.Event {
	return events.BuildKernelEventFromRaw((*events.BpfEvent)(event))
}

func kernelEventTypeName(eventType uint32) string { return events.KernelEventTypeName(eventType) }

func isNetworkEventType(eventType string) bool { return events.IsNetworkEventType(eventType) }

// ── Deps wiring ────────────────────────────────────────────────────────

func init() {
	// Functional callbacks (wrappers around existing app-package functions)
	events.Deps.GetTagName = getTagName
	events.Deps.SyscallName = syscallName
	events.Deps.ApplyBestEffortProcessContextToEvent = applyBestEffortProcessContextToEvent
	events.Deps.KernelRisk = func(raw *events.BpfEvent, event *pb.Event) {
		applyKernelRiskDecision((*bpfEvent)(raw), event)
	}
	events.Deps.Network = appNetworkSink{}

	events.Deps.RuntimeSettingsSnapshot = func() events.RuntimeSettings {
		return runtimeSettingsStore.Snapshot()
	}
	events.Deps.KernelRiskFeedbackGate = runtimeSettingsStore.KernelRiskFeedbackGate

	// Collector metrics (kernel risk)
	events.Deps.CollectorMetrics = collectorMetricsStore
	events.Deps.Enforcer = sandboxEnforcer{}

	// Process context / cgroup attribution (context_event.go)
	events.Deps.ProcessContexts = trackedProcessContexts
	events.Deps.CgroupAttributionEnrich = enrichEventWithCgroupContext
	events.Deps.CgroupAttributionSet = func(cgroupID uint64, entry events.CgroupAttributionEntry) {
		cgroupAttribution.Set(cgroupID, cgroupAttributionEntry{
			CgroupID:     entry.CgroupID,
			AgentRunID:   entry.AgentRunID,
			TaskID:       entry.TaskID,
			ToolCallID:   entry.ToolCallID,
			RootAgentPID: entry.RootAgentPID,
			CreatedAt:    entry.CreatedAt,
		})
	}
	// Semantic alerts
	events.Deps.SemanticAlertsState = semanticAlertsState
	events.Deps.ToolBaselineObserve = func(toolName, comm, eventType string) (string, bool) {
		return toolBaseline.Observe(toolName, comm, eventType)
	}
	events.Deps.EventSchemaVersion = eventSchemaVersion
}
