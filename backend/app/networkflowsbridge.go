package app

// networkflowsbridge.go — bridges between app/events subpackage and remaining
// callers in the app package (runtime__jobs_background.go, tests, etc.).
//
// This file also wires up the events.Deps struct at init time so that the
// events subpackage can call back into the app package for all shared state.

import (
	"net"

	appnetwork "agent-ebpf-filter/app/network"
	"agent-ebpf-filter/pb"

	"agent-ebpf-filter/app/events"
	netcore "agent-ebpf-filter/internal/network"
)

var fallbackNetworkMetrics = appnetwork.NewManager()

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
	events.Deps.RecordNetworkFlowContextFromEvent = recordNetworkFlowContextFromEvent
	events.Deps.ApplyKernelRiskDecision = func(raw *events.BpfEvent, event *pb.Event) {
		applyKernelRiskDecision((*bpfEvent)(raw), event)
	}
	events.Deps.MakeFlowKey = func(srcIP, dstIP string, srcPort, dstPort uint32, protocol string) events.FlowKey {
		return events.FlowKey(makeFlowKey(srcIP, dstIP, srcPort, dstPort, protocol))
	}
	events.Deps.LookupServiceByPort = lookupServiceByPort
	events.Deps.ClassifyIPScope = func(ip net.IP) events.IPScope {
		return events.IPScope(netcore.ClassifyIPScope(ip))
	}
	events.Deps.DetectAppProtocol = func(port uint32, domain string) string {
		return detectAppProtocol(port, domain)
	}

	// detectAndRecordProtocol returns app's *protoDetectionEntry; wrap it.
	events.Deps.DetectAndRecordProtocol = func(dstIP string, dstPort uint32, data []byte) *events.ProtoDetectionEntry {
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

	// Global-object method wrappers
	events.Deps.BandwidthTrackerRecordBytes = func(srcIP, dstIP string, dstPort uint32, protocol, direction string, byteCount uint64, comm string, pid uint32) {
		if manager := currentNetworkManager(); manager != nil {
			manager.RecordBandwidthBytes(srcIP, dstIP, dstPort, protocol, direction, byteCount, comm, pid)
			return
		}
		fallbackNetworkMetrics.RecordBandwidthBytes(srcIP, dstIP, dstPort, protocol, direction, byteCount, comm, pid)
	}
	events.Deps.TCPTrackerRecordConnect = func(srcIP, dstIP string, srcPort, dstPort uint32, pid uint32, comm string) {
		if manager := currentNetworkManager(); manager != nil {
			manager.RecordTCPConnect(srcIP, dstIP, srcPort, dstPort, pid, comm)
			return
		}
		tcpTracker.RecordConnect(srcIP, dstIP, srcPort, dstPort, pid, comm)
	}
	events.Deps.TCPTrackerRecordClose = func(srcIP, dstIP string, srcPort, dstPort uint32) {
		if manager := currentNetworkManager(); manager != nil {
			manager.RecordTCPClose(srcIP, dstIP, srcPort, dstPort)
			return
		}
		tcpTracker.RecordClose(srcIP, dstIP, srcPort, dstPort)
	}
	events.Deps.TCPTrackerRecordStateChange = func(srcIP, dstIP string, srcPort, dstPort uint32, oldState, newState uint8, pid uint32, comm string) {
		if manager := currentNetworkManager(); manager != nil {
			manager.RecordTCPStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, pid, comm)
			return
		}
		tcpTracker.RecordStateChange(srcIP, dstIP, srcPort, dstPort, oldState, newState, pid, comm)
	}

	// ApplyProtocolMetadata takes app's *protoDetectionEntry, not *events.ProtoDetectionEntry.
	events.Deps.FlowAggregatorApplyProtocolMetadata = func(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, entry *events.ProtoDetectionEntry) {
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
	events.Deps.DNSCorrelationLookupIP = func(ip string) (string, bool) {
		return currentDNSCorrelation().LookupIP(ip)
	}

	// Graph execution / envelope event dependencies
	events.Deps.Upgrader = &upgrader
	events.Deps.ReadCapturedEvents = readCapturedEventsFile
	events.Deps.ReadCapturedEventsContext = readCapturedEventsFileContext
	events.Deps.RuntimeSettingsRecentEvents = runtimeSettingsStore.RecentEvents
	events.Deps.RuntimeSettingsRecentEventsContext = runtimeSettingsStore.RecentEventsContext
	events.Deps.RuntimeSettingsSnapshot = func() events.RuntimeSettings {
		return runtimeSettingsStore.Snapshot()
	}

	// Collector metrics (kernel risk)
	events.Deps.CollectorMetrics = collectorMetricsStore
	events.Deps.StringsTrimDefault = stringsTrimDefault

	// Kernel-risk feedback enforcement
	events.Deps.BlockIP = blockIP
	events.Deps.BlockPort = blockPort
	events.Deps.BlockLsmFileName = blockLsmFileName
	events.Deps.BlockLsmExecPath = blockLsmExecPath
	events.Deps.BlockLsmExecName = blockLsmExecName

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
	events.Deps.ToolBaselineRecord = toolBaseline.Record

	// Semantic alerts
	events.Deps.SemanticAlertsState = semanticAlertsState
	events.Deps.ToolBaselineDetectDrift = toolBaseline.detectDrift
	events.Deps.EventSchemaVersion = eventSchemaVersion
}
