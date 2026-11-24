package network

import (
	"net"
	"strings"
	"sync"
	"time"
)

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

type FlowKey struct {
	Protocol string
	SrcIP    string
	SrcPort  uint32
	DstIP    string
	DstPort  uint32
}

func MakeFlowKey(srcIP, dstIP string, srcPort, dstPort uint32, protocol string) FlowKey {
	return FlowKey{
		Protocol: protocol,
		SrcIP:    srcIP,
		SrcPort:  srcPort,
		DstIP:    dstIP,
		DstPort:  dstPort,
	}
}

// appendUint renders v as decimal digits and appends the bytes to buf.
func appendUint(buf []byte, v uint32) []byte {
	if v == 0 {
		return append(buf, '0')
	}
	var digits [10]byte
	m := 0
	for v > 0 {
		digits[m] = '0' + byte(v%10)
		v /= 10
		m++
	}
	for i := m - 1; i >= 0; i-- {
		buf = append(buf, digits[i])
	}
	return buf
}

// ID renders the flow key as "PROTO:src:port->dst:port". Protocol is already
// an uppercase transport constant, so the string is assembled with one
// allocation instead of strings.ToUpper plus fmt.
func (k FlowKey) ID() string {
	// Worst case: upper(IPv6) + port + separators, twice, plus the protocol.
	const upper = 3 + 1 + 45 + 1 + 5 + 2 + 45 + 1 + 5
	var buf [upper]byte
	b := buf[:0]
	for i := range len(k.Protocol) {
		c := k.Protocol[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		b = append(b, c)
	}
	b = append(b, ':')
	b = append(b, k.SrcIP...)
	b = append(b, ':')
	b = appendUint(b, k.SrcPort)
	b = append(b, '-', '>')
	b = append(b, k.DstIP...)
	b = append(b, ':')
	b = appendUint(b, k.DstPort)
	return string(b)
}

type FlowEventContext struct {
	NetBytes    uint64
	AgentRunID  string
	TaskID      string
	ToolCallID  string
	TraceID     string
	SpanID      string
	ContainerID string
	Decision    string
	RiskScore   float64
}

type ProtocolMetadata struct {
	AppProtocol string
	SNI         string
	ALPN        string
	HTTPHost    string
	HTTPMethod  string
}

type FlowAggregator struct {
	mu    sync.RWMutex
	flows map[FlowKey]*NetworkFlowSummary
	dns   *DNSCache
}

func NewFlowAggregator(dns *DNSCache) *FlowAggregator {
	return &FlowAggregator{
		flows: make(map[FlowKey]*NetworkFlowSummary),
		dns:   dns,
	}
}

func (f *FlowAggregator) RecordConnection(srcIP, dstIP string, srcPort, dstPort uint32, protocol, comm string, pid uint32, direction string, state string) {
	f.RecordConnectionContext(srcIP, dstIP, srcPort, dstPort, protocol, comm, pid, direction, state, nil)
}

func (f *FlowAggregator) RecordConnectionContext(srcIP, dstIP string, srcPort, dstPort uint32, protocol, comm string, pid uint32, direction string, state string, event *FlowEventContext) {
	if f == nil {
		return
	}
	key := MakeFlowKey(srcIP, dstIP, srcPort, dstPort, protocol)
	now := time.Now().UTC().UnixMilli()

	f.mu.Lock()
	defer f.mu.Unlock()

	flow, ok := f.flows[key]
	if !ok {
		scope := ClassifyIPScope(net.ParseIP(dstIP))
		service := LookupService(uint16(dstPort))
		domain, _ := f.lookupDomain(dstIP)
		risk := IPScopeRiskScore(scope)
		riskReasons := make([]string, 0, 3)
		if risk >= 0.70 {
			riskReasons = append(riskReasons, "suspicious IP scope: "+string(scope))
		}
		if IsSuspiciousPortService(service) {
			risk = max(risk, 0.80)
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
			AppProtocol:  DetectAppProtocol(dstPort, domain),
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
			flow.BytesIn += event.NetBytes
			flow.PacketsIn++
		case "outgoing":
			flow.BytesOut += event.NetBytes
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
		addUniqueString(&flow.AgentRunIDs, event.AgentRunID)
		addUniqueString(&flow.TaskIDs, event.TaskID)
		addUniqueString(&flow.ToolCallIDs, event.ToolCallID)
		addUniqueString(&flow.TraceIDs, event.TraceID)
		addUniqueString(&flow.SpanIDs, event.SpanID)
		addUniqueString(&flow.ContainerIDs, event.ContainerID)
		addUniqueString(&flow.Decisions, event.Decision)
		if event.RiskScore > flow.RiskScore {
			flow.RiskScore = event.RiskScore
		}
	}
	updateFlowRisk(flow)
}

func (f *FlowAggregator) ApplyProtocolMetadata(srcIP, dstIP string, srcPort, dstPort uint32, protocol string, entry *ProtocolMetadata) {
	if f == nil || entry == nil {
		return
	}
	key := MakeFlowKey(srcIP, dstIP, srcPort, dstPort, protocol)
	f.mu.Lock()
	defer f.mu.Unlock()
	flow, ok := f.flows[key]
	if !ok {
		return
	}
	flow.AppProtocol = entry.AppProtocol
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
		if entry.AppProtocol == "DNS" || entry.AppProtocol == "mDNS" {
			flow.DNSName = entry.HTTPHost
		}
	}
	if entry.HTTPMethod != "" {
		flow.HTTPMethod = entry.HTTPMethod
	}
	updateFlowRisk(flow)
}

func (f *FlowAggregator) Snapshot() []NetworkFlowSummary {
	if f == nil {
		return nil
	}
	f.mu.RLock()
	defer f.mu.RUnlock()

	flows := make([]NetworkFlowSummary, 0, len(f.flows))
	now := time.Now().UTC().UnixMilli()
	for _, flow := range f.flows {
		flows = append(flows, finalizeNetworkFlowSummary(*flow, now))
	}
	return flows
}

func (f *FlowAggregator) EvictOlderThan(maxAge time.Duration) {
	if f == nil {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	cutoff := time.Now().UTC().Add(-maxAge).UnixMilli()
	for key, flow := range f.flows {
		if flow.LastSeen < cutoff {
			delete(f.flows, key)
		}
	}
}

func (f *FlowAggregator) Get(flowID string) (NetworkFlowSummary, bool) {
	if f == nil || strings.TrimSpace(flowID) == "" {
		return NetworkFlowSummary{}, false
	}
	f.mu.RLock()
	defer f.mu.RUnlock()
	now := time.Now().UTC().UnixMilli()
	for key, flow := range f.flows {
		id := flow.FlowID
		if id == "" {
			id = key.ID()
		}
		if id == flowID {
			return finalizeNetworkFlowSummary(*flow, now), true
		}
	}
	return NetworkFlowSummary{}, false
}

func (f *FlowAggregator) lookupDomain(ip string) (string, bool) {
	if f == nil || f.dns == nil {
		return "", false
	}
	return f.dns.LookupIP(ip)
}

func addUniqueString(values *[]string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}

func finalizeNetworkFlowSummary(flow NetworkFlowSummary, nowMs int64) NetworkFlowSummary {
	if flow.FlowID == "" {
		flow.FlowID = MakeFlowKey(flow.SrcIP, flow.DstIP, flow.SrcPort, flow.DstPort, flow.Protocol).ID()
	}
	if flow.Transport == "" {
		flow.Transport = flow.Protocol
	}
	if flow.LastSeen == 0 {
		flow.LastSeen = flow.FirstSeen
	}
	flow.DurationMs = max(int64(0), flow.LastSeen-flow.FirstSeen)
	ageMs := max(int64(0), nowMs-flow.LastSeen)
	switch {
	case flow.Historic:
		flow.StaleLevel = "historic"
	case ageMs > int64(2*time.Minute/time.Millisecond):
		flow.StaleLevel = "critical"
	case ageMs > int64(30*time.Second/time.Millisecond):
		flow.StaleLevel = "warning"
	default:
		flow.StaleLevel = "active"
	}
	updateFlowRisk(&flow)
	return flow
}

func updateFlowRisk(flow *NetworkFlowSummary) {
	reasons := append([]string(nil), flow.RiskReasons...)
	addReason := func(reason string) {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			return
		}
		for _, existing := range reasons {
			if existing == reason {
				return
			}
		}
		reasons = append(reasons, reason)
	}
	scopeRisk := IPScopeRiskScore(IPScope(flow.IPScope))
	if scopeRisk > flow.RiskScore {
		flow.RiskScore = scopeRisk
	}
	if scopeRisk >= 0.70 {
		addReason("suspicious IP scope: " + flow.IPScope)
	}
	if IsSuspiciousPortService(flow.DstService) {
		flow.RiskScore = max(flow.RiskScore, 0.80)
		addReason("suspicious service/port: " + flow.DstService)
	}
	if isSuspiciousFlowEndpoint(flow) {
		flow.RiskScore = max(flow.RiskScore, 0.90)
		addReason("suspicious endpoint pattern")
	}
	if strings.EqualFold(flow.AppProtocol, "SSH") && strings.EqualFold(flow.IPScope, string(ScopePublic)) {
		flow.RiskScore = max(flow.RiskScore, 0.75)
		addReason("public SSH flow")
	}
	if flow.BytesOut > 10*1024*1024 {
		flow.RiskScore = max(flow.RiskScore, 0.65)
		addReason("large outbound volume")
	}
	if flow.RiskScore >= 0.80 {
		flow.RiskLevel = "high"
	} else if flow.RiskScore >= 0.50 {
		flow.RiskLevel = "medium"
	} else if flow.RiskScore > 0 {
		flow.RiskLevel = "low"
	} else {
		flow.RiskLevel = "none"
	}
	flow.RiskReasons = reasons
}

func isSuspiciousFlowEndpoint(flow *NetworkFlowSummary) bool {
	if IsSuspiciousPortNumber(uint16(flow.DstPort)) {
		return true
	}
	for _, rawHost := range []string{flow.DstDomain, flow.DNSName, flow.SNI, flow.HTTPHost} {
		if IsSuspiciousDomainHost(rawHost) {
			return true
		}
	}
	return false
}
