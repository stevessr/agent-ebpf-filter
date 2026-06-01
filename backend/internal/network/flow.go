package network

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
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

func (k FlowKey) ID() string {
	return fmt.Sprintf("%s:%s:%d->%s:%d", strings.ToUpper(k.Protocol), k.SrcIP, k.SrcPort, k.DstIP, k.DstPort)
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

type FlowQuery struct {
	Filter       string
	Sort         string
	ShowHistoric bool
	Limit        int
	Cursor       string
	PID          uint32
	Domain       string
	Service      string
	Scope        string
}

type FlowQueryResult struct {
	Flows      []NetworkFlowSummary `json:"flows"`
	Total      int                  `json:"total"`
	NextCursor string               `json:"nextCursor,omitempty"`
}

func (f *FlowAggregator) Query(q FlowQuery) FlowQueryResult {
	flows := f.Snapshot()
	filtered := make([]NetworkFlowSummary, 0, len(flows))
	for _, flow := range flows {
		if !q.ShowHistoric && flow.Historic {
			continue
		}
		if q.PID != 0 && !flowHasPID(flow, q.PID) {
			continue
		}
		if q.Domain != "" && !strings.Contains(strings.ToLower(flow.DstDomain), strings.ToLower(q.Domain)) {
			continue
		}
		if q.Service != "" && !strings.EqualFold(flow.DstService, q.Service) {
			continue
		}
		if q.Scope != "" && !strings.EqualFold(flow.IPScope, q.Scope) {
			continue
		}
		if !flowMatchesFilter(flow, q.Filter) {
			continue
		}
		filtered = append(filtered, flow)
	}
	sortNetworkFlows(filtered, q.Sort)
	total := len(filtered)
	start := decodeFlowCursor(q.Cursor)
	if start > total {
		start = total
	}
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	end := start + limit
	if end > total {
		end = total
	}
	next := ""
	if end < total {
		next = encodeFlowCursor(end)
	}
	return FlowQueryResult{Flows: filtered[start:end], Total: total, NextCursor: next}
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
	flow.DurationMs = maxInt64(0, flow.LastSeen-flow.FirstSeen)
	ageMs := maxInt64(0, nowMs-flow.LastSeen)
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
	if flow == nil {
		return
	}
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
		flow.RiskScore = maxFloat64(flow.RiskScore, 0.80)
		addReason("suspicious service/port: " + flow.DstService)
	}
	endpoint := strings.ToLower(flow.DstIP + ":" + strconv.FormatUint(uint64(flow.DstPort), 10))
	if flow.DstDomain != "" || flow.DNSName != "" || flow.SNI != "" || flow.HTTPHost != "" {
		endpoint += " " + strings.Join([]string{flow.DstDomain, flow.DNSName, flow.SNI, flow.HTTPHost}, " ")
	}
	if isSuspiciousEndpoint(endpoint) {
		flow.RiskScore = maxFloat64(flow.RiskScore, 0.90)
		addReason("suspicious endpoint pattern")
	}
	if strings.EqualFold(flow.AppProtocol, "SSH") && strings.EqualFold(flow.IPScope, string(ScopePublic)) {
		flow.RiskScore = maxFloat64(flow.RiskScore, 0.75)
		addReason("public SSH flow")
	}
	if flow.BytesOut > 10*1024*1024 {
		flow.RiskScore = maxFloat64(flow.RiskScore, 0.65)
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

func flowHasPID(flow NetworkFlowSummary, pid uint32) bool {
	for _, existing := range flow.ProcessPIDs {
		if existing == pid {
			return true
		}
	}
	return false
}

func flowMatchesFilter(flow NetworkFlowSummary, filter string) bool {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return true
	}
	for _, token := range strings.Fields(filter) {
		key, value, ok := strings.Cut(token, ":")
		if !ok {
			haystack := strings.ToLower(strings.Join([]string{flow.FlowID, flow.SrcIP, flow.DstIP, flow.DstDomain, flow.DstService, strings.Join(flow.ProcessComms, " ")}, " "))
			if !strings.Contains(haystack, strings.ToLower(token)) {
				return false
			}
			continue
		}
		if !flowMatchesFilterToken(flow, strings.ToLower(key), strings.ToLower(value)) {
			return false
		}
	}
	return true
}

func flowMatchesFilterToken(flow NetworkFlowSummary, key, value string) bool {
	switch key {
	case "port", "dport":
		return strconv.FormatUint(uint64(flow.DstPort), 10) == value
	case "sport":
		return strconv.FormatUint(uint64(flow.SrcPort), 10) == value
	case "src":
		return strings.Contains(strings.ToLower(flow.SrcIP), value)
	case "dst":
		return strings.Contains(strings.ToLower(flow.DstIP), value) || strings.Contains(strings.ToLower(flow.DstDomain), value)
	case "process", "comm":
		return strings.Contains(strings.ToLower(strings.Join(flow.ProcessComms, " ")), value)
	case "pid":
		pid, err := strconv.ParseUint(value, 10, 32)
		return err == nil && flowHasPID(flow, uint32(pid))
	case "agent":
		return strings.Contains(strings.ToLower(strings.Join(flow.AgentRunIDs, " ")), value)
	case "task":
		return strings.Contains(strings.ToLower(strings.Join(flow.TaskIDs, " ")), value)
	case "tool":
		return strings.Contains(strings.ToLower(strings.Join(flow.ToolCallIDs, " ")), value)
	case "host", "sni", "domain":
		return strings.Contains(strings.ToLower(strings.Join([]string{flow.DstDomain, flow.DNSName, flow.SNI, flow.HTTPHost}, " ")), value)
	case "service", "app":
		return strings.Contains(strings.ToLower(flow.DstService+" "+flow.AppProtocol), value)
	case "state":
		return strings.Contains(strings.ToLower(flow.State), value)
	case "proto", "transport":
		return strings.EqualFold(flow.Transport, value) || strings.EqualFold(flow.Protocol, value)
	case "scope":
		return strings.EqualFold(flow.IPScope, value)
	case "risk":
		minRisk, err := strconv.ParseFloat(value, 64)
		return err == nil && flow.RiskScore >= minRisk
	default:
		return false
	}
}

func sortNetworkFlows(flows []NetworkFlowSummary, sortKey string) {
	desc := true
	sortKey = strings.TrimSpace(sortKey)
	if strings.HasPrefix(sortKey, "-") {
		sortKey = strings.TrimPrefix(sortKey, "-")
		desc = true
	} else if strings.HasPrefix(sortKey, "+") {
		sortKey = strings.TrimPrefix(sortKey, "+")
		desc = false
	}
	if sortKey == "" {
		sortKey = "lastSeen"
	}
	sort.SliceStable(flows, func(i, j int) bool {
		var less bool
		switch sortKey {
		case "risk":
			less = flows[i].RiskScore < flows[j].RiskScore
		case "bandwidth", "bytes":
			less = flows[i].BytesIn+flows[i].BytesOut < flows[j].BytesIn+flows[j].BytesOut
		case "firstSeen":
			less = flows[i].FirstSeen < flows[j].FirstSeen
		case "dst":
			less = flows[i].DstIP < flows[j].DstIP
		default:
			less = flows[i].LastSeen < flows[j].LastSeen
		}
		if desc {
			return !less
		}
		return less
	})
}

func encodeFlowCursor(offset int) string {
	payload, _ := json.Marshal(map[string]int{"offset": offset})
	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeFlowCursor(cursor string) int {
	if strings.TrimSpace(cursor) == "" {
		return 0
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0
	}
	var payload struct {
		Offset int `json:"offset"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil || payload.Offset < 0 {
		return 0
	}
	return payload.Offset
}

func isSuspiciousEndpoint(endpoint string) bool {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	suspiciousPatterns := []string{
		".ngrok.io", ".serveo.net", ".localhost.run",
		":4444", ":1337", ":31337", ":6666", ":6667",
		"pastebin", "termbin", "ix.io",
	}
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(endpoint, pattern) {
			return true
		}
	}
	return false
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func maxFloat64(values ...float64) float64 {
	max := 0.0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}
