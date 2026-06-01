package app

import (
	netcore "agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/pb"
	"net"
	"strconv"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section flow.go ----

type NetworkFlowSummary = netcore.NetworkFlowSummary

type flowKey = netcore.FlowKey

func makeFlowKey(srcIP, dstIP string, srcPort, dstPort uint32, protocol string) flowKey {
	return netcore.MakeFlowKey(srcIP, dstIP, srcPort, dstPort, protocol)
}

type flowAggregator struct {
	inner *netcore.FlowAggregator
}

func newFlowAggregator() *flowAggregator {
	return &flowAggregator{inner: netcore.NewFlowAggregator(dnsCorrelation)}
}

func (f *flowAggregator) RecordConnection(srcIP, dstIP string, srcPort, dstPort uint32, protocol, comm string, pid uint32, direction string, state string) {
	if f == nil || f.inner == nil {
		return
	}
	f.inner.RecordConnection(srcIP, dstIP, srcPort, dstPort, protocol, comm, pid, direction, state)
}

func (f *flowAggregator) RecordConnectionContext(srcIP, dstIP string, srcPort, dstPort uint32, protocol, comm string, pid uint32, direction string, state string, event *pb.Event) {
	if f == nil || f.inner == nil {
		return
	}
	f.inner.RecordConnectionContext(srcIP, dstIP, srcPort, dstPort, protocol, comm, pid, direction, state, flowEventContextFromProto(event))
}

func (f *flowAggregator) ApplyProtocolMetadata(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, entry *protoDetectionEntry) {
	if f == nil || f.inner == nil {
		return
	}
	f.inner.ApplyProtocolMetadata(srcIP, dstIP, srcPort, dstPort, protocol, protocolMetadataFromEntry(entry))
}

func (f *flowAggregator) Snapshot() []NetworkFlowSummary {
	if f == nil || f.inner == nil {
		return nil
	}
	return f.inner.Snapshot()
}

func (f *flowAggregator) EvictOlderThan(maxAge time.Duration) {
	if f == nil || f.inner == nil {
		return
	}
	f.inner.EvictOlderThan(maxAge)
}

func (f *flowAggregator) Get(flowID string) (NetworkFlowSummary, bool) {
	if f == nil || f.inner == nil {
		return NetworkFlowSummary{}, false
	}
	return f.inner.Get(flowID)
}

type networkFlowQuery = netcore.FlowQuery

type networkFlowQueryResult = netcore.FlowQueryResult

func (f *flowAggregator) Query(q networkFlowQuery) networkFlowQueryResult {
	if f == nil || f.inner == nil {
		return networkFlowQueryResult{}
	}
	return f.inner.Query(q)
}

var networkFlowAggregator = newFlowAggregator()

func startFlowAggregatorGC() {
	ticker := time.NewTicker(2 * time.Minute)
	go func() {
		for range ticker.C {
			networkFlowAggregator.EvictOlderThan(10 * time.Minute)
		}
	}()
}

// Collectors integration

func recordNetworkFlowFromEvent(srcIP, dstIP string, srcPort, dstPort uint32, comm string, pid uint32, direction, state string) {
	protocol := "TCP"
	networkFlowAggregator.RecordConnection(srcIP, dstIP, srcPort, dstPort, protocol, comm, pid, direction, state)
}

func recordNetworkFlowContextFromEvent(srcIP, dstIP string, srcPort, dstPort uint32, event *pb.Event, state string) {
	if event == nil {
		return
	}
	protocol := "TCP"
	if event.GetType() == "network_sendto" || event.GetType() == "network_recvfrom" {
		protocol = "UDP"
	}
	networkFlowAggregator.RecordConnectionContext(srcIP, dstIP, srcPort, dstPort, protocol, event.GetComm(), event.GetPid(), event.GetNetDirection(), state, event)
}

func enrichEndpointWithContext(endpoint string) string {
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}

	// DNS enrichment
	if domain, ok := dnsCorrelation.LookupIP(host); ok {
		if portStr != "" {
			return net.JoinHostPort(domain, portStr)
		}
		return domain
	}

	return endpoint
}

func classifyEndpointScope(endpoint string) IPScope {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}
	ip := net.ParseIP(host)
	return classifyIPScope(ip)
}

// Parse a network endpoint string into IP scope, service, domain, GeoIP, and risk info
func analyzeEndpoint(endpoint string) (scope IPScope, service string, domain string, risk float64) {
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		host = endpoint
	}

	// Scope classification
	ip := net.ParseIP(host)
	scope = classifyIPScope(ip)
	risk = ipScopeRiskScore(scope)

	// DNS enrichment
	if d, ok := dnsCorrelation.LookupIP(host); ok {
		domain = d
	}

	// GeoIP enrichment for public IPs
	if scope == ScopePublic {
		if record, ok := geoipDB.Lookup(host); ok && record.CountryCode != "XX" {
			if isHighRiskCountry(record.CountryCode) {
				risk = maxFloat64(risk, 0.85)
			}
		}
	}

	// Service name
	if portStr != "" {
		if p, err := strconv.Atoi(portStr); err == nil {
			service = lookupService(uint16(p))
			if isSuspiciousPortService(service) {
				risk = maxFloat64(risk, 0.80)
			}
		}
	}

	return
}

func flowEventContextFromProto(event *pb.Event) *netcore.FlowEventContext {
	if event == nil {
		return nil
	}
	return &netcore.FlowEventContext{
		NetBytes:    uint64(event.GetNetBytes()),
		AgentRunID:  event.GetAgentRunId(),
		TaskID:      event.GetTaskId(),
		ToolCallID:  event.GetToolCallId(),
		TraceID:     event.GetTraceId(),
		SpanID:      event.GetSpanId(),
		ContainerID: event.GetContainerId(),
		Decision:    event.GetDecision(),
		RiskScore:   event.GetRiskScore(),
	}
}

func protocolMetadataFromEntry(entry *protoDetectionEntry) *netcore.ProtocolMetadata {
	if entry == nil {
		return nil
	}
	return &netcore.ProtocolMetadata{
		AppProtocol: string(entry.AppProtocol),
		SNI:         entry.SNI,
		ALPN:        entry.ALPN,
		HTTPHost:    entry.HTTPHost,
		HTTPMethod:  entry.HTTPMethod,
	}
}
