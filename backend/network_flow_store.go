package main

import (
	"encoding/base64"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

type networkFlowQuery struct {
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

type networkFlowQueryResult struct {
	Flows      []NetworkFlowSummary `json:"flows"`
	Total      int                  `json:"total"`
	NextCursor string               `json:"nextCursor,omitempty"`
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
		flow.FlowID = makeFlowKey(flow.SrcIP, flow.DstIP, flow.SrcPort, flow.DstPort, flow.Protocol).ID()
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
	scopeRisk := ipScopeRiskScore(IPScope(flow.IPScope))
	if scopeRisk > flow.RiskScore {
		flow.RiskScore = scopeRisk
	}
	if scopeRisk >= 0.70 {
		addReason("suspicious IP scope: " + flow.IPScope)
	}
	if isSuspiciousPortService(flow.DstService) {
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

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func (f *flowAggregator) Get(flowID string) (NetworkFlowSummary, bool) {
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

func (f *flowAggregator) Query(q networkFlowQuery) networkFlowQueryResult {
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
	return networkFlowQueryResult{Flows: filtered[start:end], Total: total, NextCursor: next}
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
