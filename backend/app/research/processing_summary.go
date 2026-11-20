package research

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/app/platform"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func researchProcessingSummarySettingsFrom(settings ResearchProcessingSettings) researchProcessingSummarySettings {
	normalizeResearchProcessingSettings(&settings)
	return researchProcessingSummarySettings{
		TimelineBucketSeconds: settings.TimelineBucketSeconds,
		TopK:                  settings.TopK,
		RecentSamples:         settings.RecentSamples,
	}
}

func ptrTimeIfSet(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func durationMilliseconds(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(d.Microseconds()) / 1000
}

func throughputPerSecond(total uint64, startedAt, now time.Time) float64 {
	if total == 0 || startedAt.IsZero() || now.IsZero() || !now.After(startedAt) {
		return 0
	}
	return float64(total) / now.Sub(startedAt).Seconds()
}

func researchEventSampleFromRecord(record CapturedEventRecord) (researchEventSample, bool) {
	record = events.NormalizeCapturedEventRecord(record)
	event := record.Event
	if event == nil {
		return researchEventSample{}, false
	}
	envelope := record.Envelope
	observedAt := record.ReceivedAt.UTC()
	if observedAt.IsZero() && envelope != nil && envelope.GetTimestampNs() > 0 {
		observedAt = time.Unix(0, int64(envelope.GetTimestampNs())).UTC()
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	source := events.DetermineEnvelopeSource(event)
	if envelope != nil && strings.TrimSpace(envelope.GetSource()) != "" {
		source = envelope.GetSource()
	}
	if strings.TrimSpace(source) == "" {
		source = "unknown"
	}
	eventType := events.EnvelopeEventTypeName(envelope, event)
	if eventType == "" {
		eventType = platform.FirstNonEmpty(event.GetType(), event.GetEventType().String())
	}
	pid := event.GetPid()
	ppid := event.GetPpid()
	comm := strings.TrimSpace(event.GetComm())
	traceID := strings.TrimSpace(event.GetTraceId())
	spanID := strings.TrimSpace(event.GetSpanId())
	if envelope != nil {
		if pid == 0 {
			pid = envelope.GetPid()
		}
		if ppid == 0 {
			ppid = envelope.GetPpid()
		}
		comm = platform.FirstNonEmpty(comm, envelope.GetComm())
		traceID = platform.FirstNonEmpty(traceID, envelope.GetTraceId())
		spanID = platform.FirstNonEmpty(spanID, envelope.GetSpanId())
	}
	target := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath(), event.GetNetEndpoint(), event.GetDnsName(), event.GetSni(), event.GetDomain(), event.GetServiceName())
	titleParts := []string{source, eventType}
	if comm != "" {
		titleParts = append(titleParts, comm)
	}
	if target != "" {
		titleParts = append(titleParts, target)
	}
	id := ""
	if envelope != nil {
		id = strings.TrimSpace(envelope.GetEventId())
	}
	if id == "" {
		id = fmt.Sprintf("research-%d-%s-%d-%s", observedAt.UnixMilli(), sanitizeResearchIDPart(source), pid, sanitizeResearchIDPart(eventType))
	}
	return researchEventSample{
		ID:        id,
		Timestamp: observedAt.UnixMilli(),
		Time:      observedAt.Format(time.RFC3339Nano),
		Source:    strings.ToLower(strings.TrimSpace(source)),
		EventType: strings.TrimSpace(eventType),
		PID:       pid,
		PPID:      ppid,
		Comm:      comm,
		TraceID:   traceID,
		SpanID:    spanID,
		Title:     strings.Join(nonEmptyResearchParts(titleParts...), " · "),
		Target:    target,
		RiskScore: event.GetRiskScore(),
		Decision:  strings.TrimSpace(event.GetDecision()),
	}, true
}

func buildResearchProcessingSummary(events []researchEventSample, settings ResearchProcessingSettings) researchProcessingSummary {
	normalizeResearchProcessingSettings(&settings)
	now := time.Now().UTC()
	summary := researchProcessingSummary{
		Total:              len(events),
		BySource:           []researchCount{},
		ByType:             []researchCount{},
		ByComm:             []researchCount{},
		ByPID:              []researchCount{},
		ByTrace:            []researchCount{},
		Timeline:           []researchTimelineBucket{},
		TopProcesses:       []researchProcessSummary{},
		TopTraces:          []researchTraceSummary{},
		RecentSamples:      []researchEventSample{},
		GeneratedTimestamp: now.UnixMilli(),
		GeneratedTime:      now.Format(time.RFC3339Nano),
	}
	if len(events) == 0 {
		return summary
	}
	bySource := map[string]int{}
	byType := map[string]int{}
	byComm := map[string]int{}
	byPID := map[string]int{}
	byTrace := map[string]int{}
	processes := map[uint32]*mutableResearchProcess{}
	traces := map[string]*mutableResearchTrace{}
	timeline := map[int64]int{}
	bucketMs := int64(settings.TimelineBucketSeconds) * 1000
	if bucketMs <= 0 {
		bucketMs = int64(researchProcessingDefaultTimelineBucketSeconds) * 1000
	}
	for _, event := range events {
		if summary.EarliestTimestamp == 0 || event.Timestamp < summary.EarliestTimestamp {
			summary.EarliestTimestamp = event.Timestamp
		}
		if event.Timestamp > summary.LatestTimestamp {
			summary.LatestTimestamp = event.Timestamp
		}
		ml.IncrementResearchCount(bySource, event.Source)
		ml.IncrementResearchCount(byType, event.EventType)
		ml.IncrementResearchCount(byComm, event.Comm)
		if event.PID != 0 {
			ml.IncrementResearchCount(byPID, strconv.FormatUint(uint64(event.PID), 10)+":"+event.Comm)
			proc := processes[event.PID]
			if proc == nil {
				proc = &mutableResearchProcess{summary: researchProcessSummary{PID: event.PID, PPID: event.PPID, Comm: platform.FirstNonEmpty(event.Comm, "unknown")}, sources: map[string]struct{}{}, eventTypes: map[string]struct{}{}, childPids: map[uint32]struct{}{}}
				processes[event.PID] = proc
			}
			if proc.summary.PPID == 0 {
				proc.summary.PPID = event.PPID
			}
			proc.summary.EventCount++
			if proc.summary.FirstSeen == 0 || event.Timestamp < proc.summary.FirstSeen {
				proc.summary.FirstSeen = event.Timestamp
			}
			if event.Timestamp > proc.summary.LastSeen {
				proc.summary.LastSeen = event.Timestamp
			}
			if event.Source != "" {
				proc.sources[event.Source] = struct{}{}
			}
			if event.EventType != "" {
				proc.eventTypes[event.EventType] = struct{}{}
			}
		}
		if event.TraceID != "" {
			ml.IncrementResearchCount(byTrace, event.TraceID)
			trace := traces[event.TraceID]
			if trace == nil {
				trace = &mutableResearchTrace{summary: researchTraceSummary{TraceID: event.TraceID}, sources: map[string]struct{}{}, comms: map[string]struct{}{}}
				traces[event.TraceID] = trace
			}
			trace.summary.EventCount++
			if trace.summary.FirstSeen == 0 || event.Timestamp < trace.summary.FirstSeen {
				trace.summary.FirstSeen = event.Timestamp
			}
			if event.Timestamp > trace.summary.LastSeen {
				trace.summary.LastSeen = event.Timestamp
			}
			if event.Source != "" {
				trace.sources[event.Source] = struct{}{}
			}
			if event.Comm != "" {
				trace.comms[event.Comm] = struct{}{}
			}
		}
		bucket := event.Timestamp / bucketMs * bucketMs
		timeline[bucket]++
	}
	for _, proc := range processes {
		if proc.summary.PPID != 0 {
			if parent := processes[proc.summary.PPID]; parent != nil {
				parent.childPids[proc.summary.PID] = struct{}{}
			}
		}
	}
	summary.BySource = ml.TopResearchCounts(bySource, settings.TopK)
	summary.ByType = ml.TopResearchCounts(byType, settings.TopK)
	summary.ByComm = ml.TopResearchCounts(byComm, settings.TopK)
	summary.ByPID = ml.TopResearchCounts(byPID, settings.TopK)
	summary.ByTrace = ml.TopResearchCounts(byTrace, settings.TopK)
	summary.TopProcesses = topResearchProcesses(processes, settings.TopK)
	summary.TopTraces = topResearchTraces(traces, settings.TopK)
	summary.Timeline = researchTimelineBuckets(timeline, bucketMs)
	summary.RecentSamples = recentResearchSamples(events, settings.RecentSamples)
	if summary.EarliestTimestamp > 0 {
		summary.EarliestTime = time.UnixMilli(summary.EarliestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if summary.LatestTimestamp > 0 {
		summary.LatestTime = time.UnixMilli(summary.LatestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	return summary
}

type mutableResearchProcess struct {
	summary    researchProcessSummary
	sources    map[string]struct{}
	eventTypes map[string]struct{}
	childPids  map[uint32]struct{}
}

type mutableResearchTrace struct {
	summary researchTraceSummary
	sources map[string]struct{}
	comms   map[string]struct{}
}

func topResearchProcesses(processes map[uint32]*mutableResearchProcess, limit int) []researchProcessSummary {
	items := make([]researchProcessSummary, 0, len(processes))
	for _, proc := range processes {
		item := proc.summary
		item.Sources = sortedResearchStrings(proc.sources, 12)
		item.EventTypes = sortedResearchStrings(proc.eventTypes, 12)
		item.ChildPids = sortedResearchUint32s(proc.childPids, 24)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EventCount == items[j].EventCount {
			return items[i].PID < items[j].PID
		}
		return items[i].EventCount > items[j].EventCount
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func topResearchTraces(traces map[string]*mutableResearchTrace, limit int) []researchTraceSummary {
	items := make([]researchTraceSummary, 0, len(traces))
	for _, trace := range traces {
		item := trace.summary
		item.Sources = sortedResearchStrings(trace.sources, 12)
		item.Comms = sortedResearchStrings(trace.comms, 12)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EventCount == items[j].EventCount {
			return items[i].TraceID < items[j].TraceID
		}
		return items[i].EventCount > items[j].EventCount
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func researchTimelineBuckets(counts map[int64]int, bucketMs int64) []researchTimelineBucket {
	starts := make([]int64, 0, len(counts))
	for start := range counts {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	items := make([]researchTimelineBucket, 0, len(starts))
	for _, start := range starts {
		items = append(items, researchTimelineBucket{Start: start, End: start + bucketMs, Time: time.UnixMilli(start).UTC().Format(time.RFC3339Nano), Count: counts[start]})
	}
	return items
}

func recentResearchSamples(events []researchEventSample, limit int) []researchEventSample {
	if limit <= 0 {
		limit = researchProcessingDefaultRecentSamples
	}
	if len(events) <= limit {
		out := make([]researchEventSample, len(events))
		copy(out, events)
		return out
	}
	out := make([]researchEventSample, limit)
	copy(out, events[len(events)-limit:])
	return out
}

func sortedResearchStrings(set map[string]struct{}, limit int) []string {
	items := make([]string, 0, len(set))
	for value := range set {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func sortedResearchUint32s(set map[uint32]struct{}, limit int) []uint32 {
	items := make([]uint32, 0, len(set))
	for value := range set {
		if value != 0 {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func cloneResearchProcessingSummary(summary researchProcessingSummary) researchProcessingSummary {
	cloned := summary
	cloned.BySource = append([]researchCount(nil), summary.BySource...)
	cloned.ByType = append([]researchCount(nil), summary.ByType...)
	cloned.ByComm = append([]researchCount(nil), summary.ByComm...)
	cloned.ByPID = append([]researchCount(nil), summary.ByPID...)
	cloned.ByTrace = append([]researchCount(nil), summary.ByTrace...)
	cloned.Timeline = append([]researchTimelineBucket(nil), summary.Timeline...)
	cloned.TopProcesses = append([]researchProcessSummary(nil), summary.TopProcesses...)
	cloned.TopTraces = append([]researchTraceSummary(nil), summary.TopTraces...)
	cloned.RecentSamples = append([]researchEventSample(nil), summary.RecentSamples...)
	return cloned
}

func sanitizeResearchIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func nonEmptyResearchParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}
