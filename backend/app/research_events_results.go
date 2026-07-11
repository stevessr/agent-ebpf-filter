package app

import (
	"agent-ebpf-filter/app/platform"
	"sort"
	"strings"
	"time"
)

func collectResearchEvents(filter ResearchSourceFilter, timerange ResearchTimeRange, limit int, tlsStore *TLSCaptureStore, entry *researchTaskEntry) ([]ResearchEvent, error) {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	filter = normalizeResearchSourceFilter(filter)
	timerange = normalizeResearchTimeRange(timerange)
	if limit <= 0 {
		limit = filter.Limit
	}
	if limit <= 0 {
		limit = settings.MaxSessionEvents
	}
	if limit > settings.MaxSessionEvents {
		limit = settings.MaxSessionEvents
	}
	if limit > researchMaxTaskLimit && settings.MaxSessionEvents <= researchMaxTaskLimit {
		limit = researchMaxTaskLimit
	}
	records, _, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		return nil, err
	}
	events := make([]ResearchEvent, 0, len(records))
	for index, record := range records {
		if entry != nil && index%256 == 0 && entry.isCanceled() {
			return nil, errResearchTaskCanceled
		}
		if event, ok := researchEventFromCapturedRecord(record); ok && researchEventMatches(event, filter, timerange) {
			events = append(events, event)
		}
	}
	if researchIncludeTLS(filter) && tlsStore != nil {
		for _, tlsEvent := range tlsStore.Recent(limit) {
			if entry != nil && entry.isCanceled() {
				return nil, errResearchTaskCanceled
			}
			if event, ok := researchEventFromTLS(tlsEvent); ok && researchEventMatches(event, filter, timerange) {
				events = append(events, event)
			}
		}
	}
	if researchIncludeUploaded(filter) {
		for _, uploaded := range agentSightUploadedEvents.Recent(limit) {
			if entry != nil && entry.isCanceled() {
				return nil, errResearchTaskCanceled
			}
			if event, ok := researchEventFromAgentSight(uploaded); ok && researchEventMatches(event, filter, timerange) {
				events = append(events, event)
			}
		}
	}
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp == events[j].Timestamp {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp < events[j].Timestamp
	})
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	return events, nil
}

func researchEventFromCapturedRecord(record CapturedEventRecord) (ResearchEvent, bool) {
	sample, ok := researchEventSampleFromRecord(record)
	if !ok {
		return ResearchEvent{}, false
	}
	record = normalizeCapturedEventRecord(record)
	event := record.Event
	features := researchFeaturesFromEvent(event, record.Envelope)
	redaction := ""
	if event != nil {
		redaction = strings.TrimSpace(event.GetRedactionLevel())
	}
	if redaction == "" {
		redaction = envelopeRedactionState(record.Envelope)
	}
	if redaction == "" {
		redaction = "metadata_only"
	}
	return ResearchEvent{
		ID:             sample.ID,
		Timestamp:      sample.Timestamp,
		Time:           sample.Time,
		Source:         sample.Source,
		EventType:      sample.EventType,
		PID:            sample.PID,
		PPID:           sample.PPID,
		Comm:           sample.Comm,
		TraceID:        sample.TraceID,
		SpanID:         sample.SpanID,
		Target:         sample.Target,
		RiskScore:      sample.RiskScore,
		Decision:       sample.Decision,
		RedactionLevel: redaction,
		Features:       features,
	}, true
}

func researchEventFromTLS(event TLSPlaintextEvent) (ResearchEvent, bool) {
	timestamp := event.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	eventType := "TLS_PLAINTEXT"
	source := "ssl"
	switch event.Type {
	case "http_request", "http_response":
		eventType = "HTTP_MESSAGE"
		source = "http_parser"
	case "sse_message":
		eventType = "SSE_MESSAGE"
		source = "sse_processor"
	}
	target := platform.FirstNonEmpty(event.URL, event.Host)
	features := researchSafeFeatureMap(map[string]any{
		"runner":          "tls",
		"type":            event.Type,
		"direction":       event.Direction,
		"library":         event.Lib,
		"method":          event.Method,
		"url":             event.URL,
		"host":            event.Host,
		"status":          event.StatusCode,
		"bodySize":        event.BodySize,
		"capturedLen":     event.CapturedLen,
		"originalLen":     event.OriginalLen,
		"contentType":     event.ContentType,
		"truncated":       event.Truncated,
		"rootAgentPid":    event.RootAgentPID,
		"agentRunId":      event.AgentRunID,
		"taskId":          event.TaskID,
		"toolCallId":      event.ToolCallID,
		"toolName":        event.ToolName,
		"promptDigest":    event.PromptDigest,
		"promptLen":       event.PromptLen,
		"vendor":          event.Vendor,
		"loopAlert":       event.LoopAlert,
		"redaction_state": event.RedactionState,
	})
	return ResearchEvent{
		ID:             researchStableID("tls", timestamp.UnixMilli(), source, event.PID, event.Comm, event.Type, target),
		Timestamp:      timestamp.UnixMilli(),
		Time:           timestamp.Format(time.RFC3339Nano),
		Source:         source,
		EventType:      eventType,
		PID:            event.PID,
		Comm:           platform.FirstNonEmpty(event.Comm, "tls"),
		TraceID:        event.TraceID,
		SpanID:         event.SpanID,
		Target:         target,
		RedactionLevel: platform.FirstNonEmpty(event.RedactionState, "sanitized"),
		Features:       features,
	}, true
}

func researchEventFromAgentSight(uploaded agentSightExportEvent) (ResearchEvent, bool) {
	if uploaded.Timestamp <= 0 {
		uploaded.Timestamp = time.Now().UTC().UnixMilli()
	}
	data := researchSafeFeatureMap(uploaded.Data)
	eventType := platform.FirstNonEmpty(researchStringFromMap(data, "event_type"), researchStringFromMap(data, "eventType"), researchStringFromMap(data, "type"), "agentsight_event")
	target := platform.FirstNonEmpty(researchStringFromMap(data, "target"), researchStringFromMap(data, "path"), researchStringFromMap(data, "url"), researchStringFromMap(data, "host"), researchStringFromMap(data, "domain"))
	redaction := platform.FirstNonEmpty(researchStringFromMap(data, "redaction_state"), researchStringFromMap(data, "redactionLevel"), "sanitized")
	id := strings.TrimSpace(uploaded.ID)
	if id == "" {
		id = researchStableID("agentsight", uploaded.Timestamp, uploaded.Source, uploaded.PID, uploaded.Comm, data)
	}
	return ResearchEvent{
		ID:             id,
		Timestamp:      uploaded.Timestamp,
		Time:           time.UnixMilli(uploaded.Timestamp).UTC().Format(time.RFC3339Nano),
		Source:         platform.FirstNonEmpty(uploaded.Source, "uploaded"),
		EventType:      eventType,
		PID:            uploaded.PID,
		PPID:           uploaded.PPID,
		Comm:           uploaded.Comm,
		TraceID:        uploaded.TraceID,
		SpanID:         uploaded.SpanID,
		Target:         target,
		RiskScore:      researchFloatFromAny(data["riskScore"]),
		Decision:       researchStringFromMap(data, "decision"),
		RedactionLevel: redaction,
		Features:       data,
	}, true
}

func researchFeaturesFromEvent(event any, envelope any) map[string]any {
	pbEvent, _ := event.(interface {
		GetTag() string
		GetPath() string
		GetExtraPath() string
		GetNetDirection() string
		GetNetEndpoint() string
		GetNetBytes() uint32
		GetNetFamily() string
		GetRetval() int64
		GetExtraInfo() string
		GetBytes() uint64
		GetMode() string
		GetDomain() string
		GetSockType() string
		GetProtocol() uint32
		GetUid() uint32
		GetGid() uint32
		GetUidArg() uint32
		GetGidArg() uint32
		GetDurationNs() uint64
		GetSchemaVersion() string
		GetCgroupId() uint64
		GetRootAgentPid() uint32
		GetAgentRunId() string
		GetConversationId() string
		GetTurnId() string
		GetToolCallId() string
		GetToolName() string
		GetTaskId() string
		GetTgid() uint32
		GetFlowId() string
		GetSrcIp() string
		GetSrcPort() uint32
		GetDstIp() string
		GetDstPort() uint32
		GetTransport() string
		GetAppProtocol() string
		GetServiceName() string
		GetDnsName() string
		GetSni() string
		GetHttpHost() string
		GetTlsAlpn() string
		GetInterfaceName() string
		GetBytesIn() uint64
		GetBytesOut() uint64
		GetPacketsIn() uint64
		GetPacketsOut() uint64
		GetIpScope() string
		GetSanitizedFields() []string
	})
	if pbEvent == nil {
		return map[string]any{}
	}
	features := map[string]any{
		"tag":             pbEvent.GetTag(),
		"path":            pbEvent.GetPath(),
		"extraPath":       pbEvent.GetExtraPath(),
		"netDirection":    pbEvent.GetNetDirection(),
		"netEndpoint":     pbEvent.GetNetEndpoint(),
		"netBytes":        pbEvent.GetNetBytes(),
		"netFamily":       pbEvent.GetNetFamily(),
		"retval":          pbEvent.GetRetval(),
		"extraInfo":       pbEvent.GetExtraInfo(),
		"bytes":           pbEvent.GetBytes(),
		"mode":            pbEvent.GetMode(),
		"domain":          pbEvent.GetDomain(),
		"sockType":        pbEvent.GetSockType(),
		"protocol":        pbEvent.GetProtocol(),
		"uid":             pbEvent.GetUid(),
		"gid":             pbEvent.GetGid(),
		"uidArg":          pbEvent.GetUidArg(),
		"gidArg":          pbEvent.GetGidArg(),
		"durationNs":      pbEvent.GetDurationNs(),
		"schemaVersion":   pbEvent.GetSchemaVersion(),
		"cgroupId":        pbEvent.GetCgroupId(),
		"rootAgentPid":    pbEvent.GetRootAgentPid(),
		"agentRunId":      pbEvent.GetAgentRunId(),
		"conversationId":  pbEvent.GetConversationId(),
		"turnId":          pbEvent.GetTurnId(),
		"toolCallId":      pbEvent.GetToolCallId(),
		"toolName":        pbEvent.GetToolName(),
		"taskId":          pbEvent.GetTaskId(),
		"tgid":            pbEvent.GetTgid(),
		"flowId":          pbEvent.GetFlowId(),
		"srcIp":           pbEvent.GetSrcIp(),
		"srcPort":         pbEvent.GetSrcPort(),
		"dstIp":           pbEvent.GetDstIp(),
		"dstPort":         pbEvent.GetDstPort(),
		"transport":       pbEvent.GetTransport(),
		"appProtocol":     pbEvent.GetAppProtocol(),
		"serviceName":     pbEvent.GetServiceName(),
		"dnsName":         pbEvent.GetDnsName(),
		"sni":             pbEvent.GetSni(),
		"httpHost":        pbEvent.GetHttpHost(),
		"tlsAlpn":         pbEvent.GetTlsAlpn(),
		"interfaceName":   pbEvent.GetInterfaceName(),
		"bytesIn":         pbEvent.GetBytesIn(),
		"bytesOut":        pbEvent.GetBytesOut(),
		"packetsIn":       pbEvent.GetPacketsIn(),
		"packetsOut":      pbEvent.GetPacketsOut(),
		"ipScope":         pbEvent.GetIpScope(),
		"sanitizedFields": pbEvent.GetSanitizedFields(),
	}
	return researchSafeFeatureMap(features)
}

func buildResearchResults(sessionID string, events []ResearchEvent, compare *ResearchWindowCompare) ResearchResults {
	results, _ := buildResearchResultsWithCancel(sessionID, events, compare, nil)
	return results
}

func buildResearchResultsWithCancel(sessionID string, events []ResearchEvent, compare *ResearchWindowCompare, entry *researchTaskEntry) (ResearchResults, error) {
	now := time.Now().UTC()
	samples := make([]researchEventSample, 0, len(events))
	byTarget := map[string]int{}
	byDecision := map[string]int{}
	riskAlerts := make([]ResearchRiskFinding, 0)
	for index, event := range events {
		if index%256 == 0 {
			if err := entry.checkCanceled(); err != nil {
				return ResearchResults{}, err
			}
		}
		samples = append(samples, researchSampleFromResearchEvent(event))
		incrementResearchCount(byTarget, event.Target)
		incrementResearchCount(byDecision, event.Decision)
		if event.RiskScore >= 80 || strings.EqualFold(event.Decision, "ALERT") || strings.EqualFold(event.Decision, "BLOCK") || strings.Contains(strings.ToLower(event.EventType), "alert") {
			riskAlerts = append(riskAlerts, ResearchRiskFinding{EventID: event.ID, Timestamp: event.Timestamp, Time: event.Time, Source: event.Source, EventType: event.EventType, PID: event.PID, Comm: event.Comm, Target: event.Target, RiskScore: event.RiskScore, Decision: event.Decision, TraceID: event.TraceID, Associated: researchRiskAssociation(event)})
		}
	}
	if err := entry.checkCanceled(); err != nil {
		return ResearchResults{}, err
	}
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	loopStatus := loopDetectionWorkerStore.Status()
	results := ResearchResults{
		SchemaVersion:      researchSchemaVersion,
		SessionID:          sessionID,
		GeneratedTimestamp: now.UnixMilli(),
		GeneratedTime:      now.Format(time.RFC3339Nano),
		Summary:            buildResearchProcessingSummary(samples, settings),
		TopTargets:         topResearchCounts(byTarget, settings.TopK),
		TopDecisions:       topResearchCounts(byDecision, settings.TopK),
		LoopFindings:       matchResearchLoopFindings(events, loopStatus.RecentFindings),
		RiskAlerts:         riskAlerts,
		KernelRiskFeedback: currentResearchKernelRiskFeedbackInfo(),
		CompareWindows:     compare,
	}
	return results, nil
}

func buildResearchSessionSummary(events []ResearchEvent, results ResearchResults) ResearchSessionSummary {
	now := time.Now().UTC()
	summary := ResearchSessionSummary{SchemaVersion: researchSchemaVersion, EventCount: len(events), GeneratedTimestamp: now.UnixMilli(), GeneratedTime: now.Format(time.RFC3339Nano), LoopFindings: len(results.LoopFindings), RiskAlerts: len(results.RiskAlerts)}
	if len(events) == 0 {
		return summary
	}
	bySource := map[string]int{}
	byType := map[string]int{}
	byComm := map[string]int{}
	for _, event := range events {
		if summary.EarliestTimestamp == 0 || event.Timestamp < summary.EarliestTimestamp {
			summary.EarliestTimestamp = event.Timestamp
		}
		if event.Timestamp > summary.LatestTimestamp {
			summary.LatestTimestamp = event.Timestamp
		}
		if event.RiskScore > summary.MaxRiskScore {
			summary.MaxRiskScore = event.RiskScore
		}
		incrementResearchCount(bySource, event.Source)
		incrementResearchCount(byType, event.EventType)
		incrementResearchCount(byComm, event.Comm)
	}
	if summary.EarliestTimestamp > 0 {
		summary.EarliestTime = time.UnixMilli(summary.EarliestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if summary.LatestTimestamp > 0 {
		summary.LatestTime = time.UnixMilli(summary.LatestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if top := topResearchCounts(bySource, 1); len(top) > 0 {
		summary.TopSource = top[0].Key
	}
	if top := topResearchCounts(byType, 1); len(top) > 0 {
		summary.TopEventType = top[0].Key
	}
	if top := topResearchCounts(byComm, 1); len(top) > 0 {
		summary.TopComm = top[0].Key
	}
	return summary
}

func buildResearchWindowCompare(events []ResearchEvent, leftRange, rightRange ResearchTimeRange) *ResearchWindowCompare {
	leftRange = normalizeResearchTimeRange(leftRange)
	rightRange = normalizeResearchTimeRange(rightRange)
	leftEvents := filterResearchEventsByRange(events, leftRange)
	rightEvents := filterResearchEventsByRange(events, rightRange)
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	leftSummary := buildResearchProcessingSummary(researchSamplesFromResearchEvents(leftEvents), settings)
	rightSummary := buildResearchProcessingSummary(researchSamplesFromResearchEvents(rightEvents), settings)
	return &ResearchWindowCompare{
		Left:   ResearchWindowSummary{Name: "left", TimeRange: leftRange, Summary: leftSummary},
		Right:  ResearchWindowSummary{Name: "right", TimeRange: rightRange, Summary: rightSummary},
		Deltas: researchSummaryDeltas(leftSummary, rightSummary),
	}
}

func researchSummaryDeltas(left, right researchProcessingSummary) []ResearchCountDelta {
	var deltas []ResearchCountDelta
	appendDeltas := func(category string, a, b []researchCount) {
		counts := map[string][2]int{}
		for _, item := range a {
			counts[item.Key] = [2]int{item.Count, counts[item.Key][1]}
		}
		for _, item := range b {
			counts[item.Key] = [2]int{counts[item.Key][0], item.Count}
		}
		keys := make([]string, 0, len(counts))
		for key := range counts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			pair := counts[key]
			deltas = append(deltas, ResearchCountDelta{Category: category, Key: key, Left: pair[0], Right: pair[1], Delta: pair[1] - pair[0]})
		}
	}
	appendDeltas("source", left.BySource, right.BySource)
	appendDeltas("eventType", left.ByType, right.ByType)
	appendDeltas("comm", left.ByComm, right.ByComm)
	sort.SliceStable(deltas, func(i, j int) bool {
		ai := deltas[i].Delta
		if ai < 0 {
			ai = -ai
		}
		aj := deltas[j].Delta
		if aj < 0 {
			aj = -aj
		}
		if ai == aj {
			if deltas[i].Category == deltas[j].Category {
				return deltas[i].Key < deltas[j].Key
			}
			return deltas[i].Category < deltas[j].Category
		}
		return ai > aj
	})
	if len(deltas) > 100 {
		deltas = deltas[:100]
	}
	return deltas
}

func researchSampleFromResearchEvent(event ResearchEvent) researchEventSample {
	return researchEventSample{ID: event.ID, Timestamp: event.Timestamp, Time: event.Time, Source: event.Source, EventType: event.EventType, PID: event.PID, PPID: event.PPID, Comm: event.Comm, TraceID: event.TraceID, SpanID: event.SpanID, Title: strings.Join(nonEmptyResearchParts(event.Source, event.EventType, event.Comm, event.Target), " · "), Target: event.Target, RiskScore: event.RiskScore, Decision: event.Decision}
}

func researchSamplesFromResearchEvents(events []ResearchEvent) []researchEventSample {
	samples := make([]researchEventSample, 0, len(events))
	for _, event := range events {
		samples = append(samples, researchSampleFromResearchEvent(event))
	}
	return samples
}

func matchResearchLoopFindings(events []ResearchEvent, findings []loopDetectionFinding) []loopDetectionFinding {
	if len(events) == 0 || len(findings) == 0 {
		return nil
	}
	pids := map[uint32]struct{}{}
	traces := map[string]struct{}{}
	comms := map[string]struct{}{}
	for _, event := range events {
		if event.PID != 0 {
			pids[event.PID] = struct{}{}
		}
		if event.TraceID != "" {
			traces[event.TraceID] = struct{}{}
		}
		if event.Comm != "" {
			comms[strings.ToLower(event.Comm)] = struct{}{}
		}
	}
	out := make([]loopDetectionFinding, 0)
	for _, finding := range findings {
		matched := false
		if finding.PID != 0 {
			_, matched = pids[finding.PID]
		}
		if !matched && finding.TraceID != "" {
			_, matched = traces[finding.TraceID]
		}
		if !matched && finding.Comm != "" {
			_, matched = comms[strings.ToLower(finding.Comm)]
		}
		if matched {
			out = append(out, finding)
		}
	}
	return out
}

func currentResearchKernelRiskFeedbackInfo() ResearchKernelRiskFeedbackInfo {
	settings := runtimeSettingsStore.Snapshot()
	normalizeKernelRiskFeedbackSettings(&settings.KernelRiskFeedback)
	return ResearchKernelRiskFeedbackInfo{Enabled: settings.KernelRiskFeedback.Enabled, PolicyGateEnabled: settings.PolicyManagementEnabled, MinRiskScore: settings.KernelRiskFeedback.MinRiskScore, EnforceNetwork: settings.KernelRiskFeedback.EnforceNetwork, EnforceFileNames: settings.KernelRiskFeedback.EnforceFileNames, EnforceExec: settings.KernelRiskFeedback.EnforceExec, MaxActionsPerMinute: settings.KernelRiskFeedback.MaxActionsPerMinute}
}
