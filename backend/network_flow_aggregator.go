package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
)

// Network Flow Summary

type NetworkFlowSummary struct {
	FlowID        string   `json:"flowId"`
	Protocol      string   `json:"protocol"`
	Transport     string   `json:"transport"`
	SrcIP         string   `json:"srcIp"`
	SrcPort       uint32   `json:"srcPort"`
	DstIP         string   `json:"dstIp"`
	DstPort       uint32   `json:"dstPort"`
	DstService    string   `json:"dstService,omitempty"`
	DstDomain     string   `json:"dstDomain,omitempty"`
	DNSName       string   `json:"dnsName,omitempty"`
	SNI           string   `json:"sni,omitempty"`
	HTTPHost      string   `json:"httpHost,omitempty"`
	HTTPMethod    string   `json:"httpMethod,omitempty"`
	TLSALPN       string   `json:"tlsAlpn,omitempty"`
	IPScope       string   `json:"ipScope"`
	Direction     string   `json:"direction"`
	State         string   `json:"state,omitempty"`
	BytesIn       uint64   `json:"bytesIn"`
	BytesOut      uint64   `json:"bytesOut"`
	PacketsIn     uint64   `json:"packetsIn"`
	PacketsOut    uint64   `json:"packetsOut"`
	CurrentBpsIn  float64  `json:"currentBpsIn"`
	CurrentBpsOut float64  `json:"currentBpsOut"`
	PeakBpsIn     float64  `json:"peakBpsIn"`
	PeakBpsOut    float64  `json:"peakBpsOut"`
	ProcessPIDs   []uint32 `json:"processPids"`
	ProcessComms  []string `json:"processComms"`
	AgentRunIDs   []string `json:"agentRunIds,omitempty"`
	TaskIDs       []string `json:"taskIds,omitempty"`
	ToolCallIDs   []string `json:"toolCallIds,omitempty"`
	TraceIDs      []string `json:"traceIds,omitempty"`
	SpanIDs       []string `json:"spanIds,omitempty"`
	ContainerIDs  []string `json:"containerIds,omitempty"`
	Decisions     []string `json:"decisions,omitempty"`
	FirstSeen     int64    `json:"firstSeen"`
	LastSeen      int64    `json:"lastSeen"`
	DurationMs    int64    `json:"durationMs"`
	StaleLevel    string   `json:"staleLevel"`
	Historic      bool     `json:"historic"`
	RiskScore     float64  `json:"riskScore"`
	RiskLevel     string   `json:"riskLevel"`
	RiskReasons   []string `json:"riskReasons,omitempty"`
	AppProtocol   string   `json:"appProtocol,omitempty"`
}

type flowKey struct {
	Protocol string
	SrcIP    string
	SrcPort  uint32
	DstIP    string
	DstPort  uint32
}

func makeFlowKey(srcIP, dstIP string, srcPort, dstPort uint32, protocol string) flowKey {
	return flowKey{
		Protocol: protocol,
		SrcIP:    srcIP,
		SrcPort:  srcPort,
		DstIP:    dstIP,
		DstPort:  dstPort,
	}
}

func (k flowKey) ID() string {
	return fmt.Sprintf("%s:%s:%d->%s:%d", strings.ToUpper(k.Protocol), k.SrcIP, k.SrcPort, k.DstIP, k.DstPort)
}

type flowAggregator struct {
	mu    sync.RWMutex
	flows map[flowKey]*NetworkFlowSummary
}

func newFlowAggregator() *flowAggregator {
	return &flowAggregator{
		flows: make(map[flowKey]*NetworkFlowSummary),
	}
}

func (f *flowAggregator) RecordConnection(srcIP, dstIP string, srcPort, dstPort uint32, protocol, comm string, pid uint32, direction string, state string) {
	f.RecordConnectionContext(srcIP, dstIP, srcPort, dstPort, protocol, comm, pid, direction, state, nil)
}

func (f *flowAggregator) RecordConnectionContext(srcIP, dstIP string, srcPort, dstPort uint32, protocol, comm string, pid uint32, direction string, state string, event *pb.Event) {
	if f == nil {
		return
	}
	key := makeFlowKey(srcIP, dstIP, srcPort, dstPort, protocol)
	now := time.Now().UTC().UnixMilli()

	f.mu.Lock()
	defer f.mu.Unlock()

	flow, ok := f.flows[key]
	if !ok {
		scope := classifyIPScope(net.ParseIP(dstIP))
		service := lookupService(uint16(dstPort))
		domain, _ := dnsCorrelation.LookupIP(dstIP)
		risk := ipScopeRiskScore(scope)
		riskReasons := make([]string, 0, 3)
		if risk >= 0.70 {
			riskReasons = append(riskReasons, "suspicious IP scope: "+string(scope))
		}
		if isSuspiciousPortService(service) {
			risk = maxFloat64(risk, 0.80)
			riskReasons = append(riskReasons, "suspicious service/port: "+service)
		}

		flow = &NetworkFlowSummary{
			FlowID:       key.ID(),
			Protocol:     protocol,
			Transport:    protocol,
			SrcIP:        srcIP,
			SrcPort:      srcPort,
			DstIP:        dstIP,
			DstPort:      dstPort,
			DstService:   service,
			DstDomain:    domain,
			IPScope:      string(scope),
			Direction:    direction,
			State:        state,
			ProcessPIDs:  make([]uint32, 0),
			ProcessComms: make([]string, 0),
			FirstSeen:    now,
			LastSeen:     now,
			RiskScore:    risk,
			RiskReasons:  riskReasons,
			AppProtocol:  detectAppProtocol(dstPort, domain),
		}
		if domain != "" {
			flow.DNSName = domain
		}
		f.flows[key] = flow
	}

	previousBytesIn := flow.BytesIn
	previousBytesOut := flow.BytesOut
	previousLastSeen := flow.LastSeen
	flow.LastSeen = now
	if state != "" {
		flow.State = state
	}
	if flow.State == "CLOSED" || flow.State == "CLOSE" || strings.EqualFold(state, "closed") {
		flow.Historic = true
	}
	if event != nil {
		switch direction {
		case "incoming":
			flow.BytesIn += uint64(event.GetNetBytes())
			flow.PacketsIn++
		case "outgoing":
			flow.BytesOut += uint64(event.GetNetBytes())
			flow.PacketsOut++
		}
	}
	elapsedSinceUpdate := float64(now-previousLastSeen) / 1000
	if previousLastSeen > 0 && elapsedSinceUpdate > 0 {
		flow.CurrentBpsIn = float64(flow.BytesIn-previousBytesIn) / elapsedSinceUpdate
		flow.CurrentBpsOut = float64(flow.BytesOut-previousBytesOut) / elapsedSinceUpdate
		if flow.CurrentBpsIn > flow.PeakBpsIn {
			flow.PeakBpsIn = flow.CurrentBpsIn
		}
		if flow.CurrentBpsOut > flow.PeakBpsOut {
			flow.PeakBpsOut = flow.CurrentBpsOut
		}
	}

	// Deduplicate processes
	pidExists := false
	for _, p := range flow.ProcessPIDs {
		if p == pid {
			pidExists = true
			break
		}
	}
	if !pidExists && pid > 0 {
		flow.ProcessPIDs = append(flow.ProcessPIDs, pid)
		flow.ProcessComms = append(flow.ProcessComms, comm)
	}
	if event != nil {
		addUniqueString(&flow.AgentRunIDs, event.GetAgentRunId())
		addUniqueString(&flow.TaskIDs, event.GetTaskId())
		addUniqueString(&flow.ToolCallIDs, event.GetToolCallId())
		addUniqueString(&flow.TraceIDs, event.GetTraceId())
		addUniqueString(&flow.SpanIDs, event.GetSpanId())
		addUniqueString(&flow.ContainerIDs, event.GetContainerId())
		addUniqueString(&flow.Decisions, event.GetDecision())
		if event.GetRiskScore() > flow.RiskScore {
			flow.RiskScore = event.GetRiskScore()
		}
	}
	updateFlowRisk(flow)
}

func (f *flowAggregator) ApplyProtocolMetadata(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, entry *protoDetectionEntry) {
	if f == nil || entry == nil {
		return
	}
	key := makeFlowKey(srcIP, dstIP, srcPort, dstPort, protocol)
	f.mu.Lock()
	defer f.mu.Unlock()
	flow, ok := f.flows[key]
	if !ok {
		return
	}
	flow.AppProtocol = string(entry.AppProtocol)
	if entry.SNI != "" {
		flow.SNI = entry.SNI
		if flow.DstDomain == "" {
			flow.DstDomain = entry.SNI
		}
	}
	if entry.ALPN != "" {
		flow.TLSALPN = entry.ALPN
	}
	if entry.HTTPHost != "" {
		flow.HTTPHost = entry.HTTPHost
		if flow.DstDomain == "" || flow.DstDomain == flow.SNI {
			flow.DstDomain = entry.HTTPHost
		}
		if entry.AppProtocol == AppProtoDNS || entry.AppProtocol == AppProtomDNS {
			flow.DNSName = entry.HTTPHost
		}
	}
	if entry.HTTPMethod != "" {
		flow.HTTPMethod = entry.HTTPMethod
	}
	updateFlowRisk(flow)
}

func (f *flowAggregator) Snapshot() []NetworkFlowSummary {
	f.mu.RLock()
	defer f.mu.RUnlock()

	flows := make([]NetworkFlowSummary, 0, len(f.flows))
	now := time.Now().UTC().UnixMilli()
	for _, flow := range f.flows {
		flows = append(flows, finalizeNetworkFlowSummary(*flow, now))
	}
	return flows
}

func (f *flowAggregator) EvictOlderThan(maxAge time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().UTC().Add(-maxAge).UnixMilli()
	for key, flow := range f.flows {
		if flow.LastSeen < cutoff {
			delete(f.flows, key)
		}
	}
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
