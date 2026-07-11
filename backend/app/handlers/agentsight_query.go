package handlers

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/tls"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func collectAgentSightEvents(c *gin.Context, tlsStore *tls.TLSCaptureStore) ([]AgentSightExportEvent, string, error) {
	query := agentSightQueryFromRequest(c)
	return collectAgentSightEventsForQuery(query, tlsStore)
}

func collectAgentSightEventsForQuery(query AgentSightEventQuery, tlsStore *tls.TLSCaptureStore) ([]AgentSightExportEvent, string, error) {
	records, source, err := Deps.RuntimeSettings.RecentEvents(query.Limit)
	if err != nil {
		return nil, "", err
	}
	tlsCount := 0
	if tlsStore != nil {
		tlsCount = tlsStore.Count()
	}
	events := make([]AgentSightExportEvent, 0, len(records)+tlsCount)
	for _, record := range records {
		converted := agentSightEventFromCapturedRecord(record)
		if agentSightExportEventMatches(converted, query) {
			events = append(events, converted)
		}
	}
	if query.IncludeTLS && tlsStore != nil {
		for _, event := range tlsStore.Recent(query.Limit) {
			converted := agentSightEventFromTLSPlaintext(event)
			if agentSightExportEventMatches(converted, query) {
				events = append(events, converted)
			}
		}
	}
	for _, event := range Deps.AgentSightUploadedEvents.Recent(query.Limit) {
		if e, ok := event.(AgentSightExportEvent); ok && agentSightExportEventMatches(e, query) {
			events = append(events, e)
		}
	}

	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp == events[j].Timestamp {
			return events[i].ID < events[j].ID
		}
		return events[i].Timestamp < events[j].Timestamp
	})
	if len(events) > query.Limit {
		events = events[len(events)-query.Limit:]
	}
	return events, source, nil
}

// ── Query helpers ────────────────────────────────────────────────────

func agentSightQueryFromRequest(c *gin.Context) AgentSightEventQuery {
	query := AgentSightEventQuery{
		Limit:      agentSightDefaultLimit,
		Filters:    recentEventFiltersFromRequest(c),
		Search:     platform.FirstNonEmpty(c.Query("filter"), c.Query("q"), c.Query("search")),
		IncludeTLS: true,
		Sources:    splitAgentSightCSV(platform.FirstNonEmpty(c.Query("sources"), c.Query("source[]"))),
		EventTypes: splitAgentSightCSV(platform.FirstNonEmpty(c.Query("event_types"), c.Query("eventTypes"), c.Query("event_type[]"))),
		RunnerID:   strings.TrimSpace(c.Query("runner")),
	}
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			query.Limit = parsed
		}
	}
	if query.Limit > agentSightMaxLimit {
		query.Limit = agentSightMaxLimit
	}
	if raw := strings.TrimSpace(c.Query("include_tls")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			query.IncludeTLS = parsed
		}
	}
	if raw := strings.TrimSpace(c.Query("includeTLS")); raw != "" {
		if parsed, err := strconv.ParseBool(raw); err == nil {
			query.IncludeTLS = parsed
		}
	}
	if raw := strings.TrimSpace(c.Query("pids")); raw != "" {
		query.PIDs = parseAgentSightPIDList(raw)
	}
	if query.RunnerID == "" {
		query.RunnerID = strings.TrimSpace(c.Query("runner_id"))
	}
	return query
}

// recentEventFiltersFromRequest delegates to the app-level function via Deps.
func recentEventFiltersFromRequest(c *gin.Context) any {
	return Deps.RecentEventFiltersFromRequest(c)
}

func agentSightQueryFromJSONRequest(c *gin.Context) (AgentSightEventQuery, error) {
	query := agentSightQueryFromRequest(c)
	var body struct {
		Limit           int      `json:"limit"`
		Filter          string   `json:"filter"`
		Search          string   `json:"search"`
		Query           string   `json:"query"`
		Source          string   `json:"source"`
		Sources         []string `json:"sources"`
		Type            string   `json:"type"`
		EventType       string   `json:"event_type"`
		EventTypeCamel  string   `json:"eventType"`
		EventTypes      []string `json:"event_types"`
		PID             uint32   `json:"pid"`
		PIDs            []uint32 `json:"pids"`
		Comm            string   `json:"comm"`
		TraceID         string   `json:"trace_id"`
		TraceIDCamel    string   `json:"traceId"`
		SpanID          string   `json:"span_id"`
		SpanIDCamel     string   `json:"spanId"`
		RedactionState  string   `json:"redaction_state"`
		Since           any      `json:"since"`
		Until           any      `json:"until"`
		IncludeTLS      *bool    `json:"include_tls"`
		IncludeTLSCamel *bool    `json:"includeTLS"`
		Runner          string   `json:"runner"`
		RunnerID        string   `json:"runner_id"`
	}
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&body); err != nil {
			return query, err
		}
	}
	if body.Limit > 0 {
		query.Limit = body.Limit
		if query.Limit > agentSightMaxLimit {
			query.Limit = agentSightMaxLimit
		}
	}
	query.Search = platform.FirstNonEmpty(body.Filter, body.Search, body.Query, query.Search)
	query.Filters = setRecentEventFilterSource(body.Source, query.Filters)
	query.Sources = append(query.Sources, normalizeAgentSightTerms(body.Sources)...)
	query.Filters = setRecentEventFilterType(body.Type, query.Filters)
	query.Filters = setRecentEventFilterEventType(platform.FirstNonEmpty(body.EventType, body.EventTypeCamel), query.Filters)
	query.EventTypes = append(query.EventTypes, normalizeAgentSightTerms(body.EventTypes)...)
	if body.PID != 0 {
		query.Filters = setRecentEventFilterPID(body.PID, query.Filters)
	}
	query.PIDs = append(query.PIDs, body.PIDs...)
	query.Filters = setRecentEventFilterComm(body.Comm, query.Filters)
	query.Filters = setRecentEventFilterTraceID(platform.FirstNonEmpty(body.TraceID, body.TraceIDCamel), query.Filters)
	query.Filters = setRecentEventFilterSpanID(platform.FirstNonEmpty(body.SpanID, body.SpanIDCamel), query.Filters)
	query.Filters = setRecentEventFilterRedactionState(body.RedactionState, query.Filters)
	if parsed := parseAgentSightTimeAny(body.Since); !parsed.IsZero() {
		query.Filters = setRecentEventFilterSince(parsed, query.Filters)
	}
	if parsed := parseAgentSightTimeAny(body.Until); !parsed.IsZero() {
		query.Filters = setRecentEventFilterUntil(parsed, query.Filters)
	}
	if body.IncludeTLS != nil {
		query.IncludeTLS = *body.IncludeTLS
	}
	if body.IncludeTLSCamel != nil {
		query.IncludeTLS = *body.IncludeTLSCamel
	}
	query.RunnerID = platform.FirstNonEmpty(body.Runner, body.RunnerID, query.RunnerID)
	return query, nil
}

// ── Builder functions ────────────────────────────────────────────────

func buildAgentSightRunners(events []AgentSightExportEvent, tlsStore *tls.TLSCaptureStore) []AgentSightRunnerStatus {
	stats := buildAgentSightEventsStats(events, agentSightMaxLimit, "")
	settings := Deps.RuntimeSettings.Snapshot()
	collector := Deps.CollectorHealth()
	now := time.Now().UTC()
	runners := []AgentSightRunnerStatus{
		{
			ID:          "process",
			Name:        "Process/File/eBPF runner",
			Type:        "process",
			Enabled:     true,
			Running:     collectorMapAvailable(collector) || stats.ByRunner["process"] > 0,
			State:       agentSightRunnerState(collectorMapAvailable(collector) || stats.ByRunner["process"] > 0),
			EventCount:  stats.ByRunner["process"],
			LastEventTs: lastAgentSightRunnerEventTs(events, "process"),
			Details:     collectorDetails(collector),
		},
		{
			ID:          "tls",
			Name:        "TLS/HTTP/SSE plaintext runner",
			Type:        "ssl",
			Enabled:     settings.TlsCaptureEnabled,
			Running:     tlsStore != nil && (tlsStore.Count() > 0 || len(tlsStore.LibraryStatuses()) > 0),
			State:       agentSightRunnerState(settings.TlsCaptureEnabled),
			EventCount:  stats.ByRunner["tls"],
			LastEventTs: lastAgentSightRunnerEventTs(events, "tls"),
			Details: map[string]any{
				"storeCount": tlsStoreCount(tlsStore),
				"libraries":  tlsStoreLibraries(tlsStore),
			},
		},
		{
			ID:          "stdio",
			Name:        "STDIO/MCP payload runner",
			Type:        "stdio",
			Enabled:     true,
			Running:     stats.ByRunner["stdio"] > 0,
			State:       agentSightRunnerState(stats.ByRunner["stdio"] > 0),
			EventCount:  stats.ByRunner["stdio"],
			LastEventTs: lastAgentSightRunnerEventTs(events, "stdio"),
		},
		{
			ID:          "system",
			Name:        "System resource runner",
			Type:        "system",
			Enabled:     true,
			Running:     stats.ByRunner["system"] > 0,
			State:       agentSightRunnerState(stats.ByRunner["system"] > 0),
			EventCount:  stats.ByRunner["system"],
			LastEventTs: lastAgentSightRunnerEventTs(events, "system"),
			Details: map[string]any{
				"websocket": "/ws/system",
			},
		},
		{
			ID:          "agent",
			Name:        "Wrapper/hook/policy/OTel runner",
			Type:        "agent",
			Enabled:     true,
			Running:     stats.ByRunner["agent"] > 0,
			State:       agentSightRunnerState(stats.ByRunner["agent"] > 0),
			EventCount:  stats.ByRunner["agent"],
			LastEventTs: lastAgentSightRunnerEventTs(events, "agent"),
		},
		{
			ID:          "uploaded",
			Name:        "Uploaded AgentSight trace store",
			Type:        "storage",
			Enabled:     true,
			Running:     stats.ByRunner["uploaded"] > 0,
			State:       agentSightRunnerState(stats.ByRunner["uploaded"] > 0),
			EventCount:  stats.ByRunner["uploaded"],
			LastEventTs: lastAgentSightRunnerEventTs(events, "uploaded"),
		},
	}
	for index := range runners {
		if runners[index].LastEventTs > 0 {
			runners[index].LastEventISO = time.UnixMilli(runners[index].LastEventTs).UTC().Format(time.RFC3339Nano)
		}
		if runners[index].Details == nil {
			runners[index].Details = map[string]any{}
		}
		runners[index].Details["reportedAt"] = now.Format(time.RFC3339Nano)
	}
	return runners
}

func collectorMapAvailable(collector any) bool {
	if m, ok := collector.(interface{ GetCollectorMapAvailable() bool }); ok {
		return m.GetCollectorMapAvailable()
	}
	return false
}

func collectorDetails(collector any) map[string]any {
	return map[string]any{
		"collectorMapAvailable": collectorMapAvailable(collector),
	}
}

func buildAgentSightEventsStats(events []AgentSightExportEvent, limit int, runnerID string) AgentSightEventsStats {
	now := time.Now().UTC()
	stats := AgentSightEventsStats{
		Total:              len(events),
		Limit:              limit,
		Runner:             runnerID,
		BySource:           make(map[string]int),
		ByType:             make(map[string]int),
		ByRunner:           make(map[string]int),
		ByComm:             make(map[string]int),
		GeneratedTimestamp: now.UnixMilli(),
		GeneratedTime:      now.Format(time.RFC3339Nano),
	}
	for _, event := range events {
		stats.BySource[event.Source]++
		stats.ByRunner[agentSightRunnerIDForEvent(event)]++
		if event.Comm != "" {
			stats.ByComm[event.Comm]++
		}
		if eventType := platform.FirstNonEmpty(stringFromMap(event.Data, "event_type"), stringFromMap(event.Data, "eventType"), stringFromMap(event.Data, "type")); eventType != "" {
			stats.ByType[eventType]++
		}
		if event.Timestamp > 0 {
			if stats.EarliestTimestamp == 0 || event.Timestamp < stats.EarliestTimestamp {
				stats.EarliestTimestamp = event.Timestamp
			}
			if event.Timestamp > stats.LatestTimestamp {
				stats.LatestTimestamp = event.Timestamp
			}
		}
	}
	if stats.EarliestTimestamp > 0 {
		stats.EarliestTime = time.UnixMilli(stats.EarliestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if stats.LatestTimestamp > 0 {
		stats.LatestTime = time.UnixMilli(stats.LatestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	return stats
}

func lastAgentSightRunnerEventTs(events []AgentSightExportEvent, runnerID string) int64 {
	var latest int64
	for _, event := range events {
		if agentSightRunnerIDForEvent(event) == runnerID && event.Timestamp > latest {
			latest = event.Timestamp
		}
	}
	return latest
}

func agentSightRunnerState(running bool) string {
	if running {
		return "running"
	}
	return "idle"
}

func tlsStoreCount(store *tls.TLSCaptureStore) int {
	if store == nil {
		return 0
	}
	return store.Count()
}

func tlsStoreLibraries(store *tls.TLSCaptureStore) []tls.TLSLibraryStatus {
	if store == nil {
		return nil
	}
	return store.LibraryStatuses()
}

// ── Write helpers ────────────────────────────────────────────────────

func writeAgentSightEvents(c *gin.Context, events []AgentSightExportEvent, source string, forceJSONL bool) {
	format := strings.ToLower(strings.TrimSpace(c.Query("format")))
	if forceJSONL || format == "jsonl" || format == "ndjson" || format == "log" {
		contentType := "application/x-ndjson; charset=utf-8"
		if forceJSONL && c.FullPath() == "/api/events" {
			contentType = "text/plain; charset=utf-8"
		}
		c.Data(http.StatusOK, contentType, []byte(agentSightEventsJSONL(events)))
		return
	}
	if format == "array" {
		c.JSON(http.StatusOK, events)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"source": source,
		"events": events,
	})
}

func agentSightEventsJSONL(events []AgentSightExportEvent) string {
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	encoder.SetEscapeHTML(false)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			continue
		}
	}
	return builder.String()
}

// ── Upload parsing ───────────────────────────────────────────────────

func agentSightExportEventMatches(event AgentSightExportEvent, query AgentSightEventQuery) bool {
	if query.RunnerID != "" && !strings.EqualFold(agentSightRunnerIDForEvent(event), query.RunnerID) {
		return false
	}
	if len(query.Sources) > 0 && !agentSightStringInList(event.Source, query.Sources) {
		return false
	}
	// Filters.Source, Filters.Type, etc. are accessed via type assertion from the `any` field
	if filterStr := recentEventFilterField(query.Filters, "source"); filterStr != "" && !strings.EqualFold(event.Source, filterStr) {
		return false
	}
	if len(query.PIDs) > 0 && !agentSightUint32InList(event.PID, query.PIDs) {
		return false
	}
	if filterPID := recentEventFilterUint32(query.Filters, "pid"); filterPID != 0 && event.PID != filterPID {
		return false
	}
	if filterComm := recentEventFilterField(query.Filters, "comm"); filterComm != "" && !strings.Contains(strings.ToLower(event.Comm), strings.ToLower(filterComm)) {
		return false
	}
	if filterTrace := recentEventFilterField(query.Filters, "trace_id"); filterTrace != "" && event.TraceID != filterTrace {
		return false
	}
	if filterSpan := recentEventFilterField(query.Filters, "span_id"); filterSpan != "" && event.SpanID != filterSpan {
		return false
	}
	if filterType := recentEventFilterField(query.Filters, "type"); filterType != "" && !strings.EqualFold(stringFromMap(event.Data, "type"), filterType) {
		return false
	}
	eventType := platform.FirstNonEmpty(stringFromMap(event.Data, "event_type"), stringFromMap(event.Data, "eventType"), stringFromMap(event.Data, "type"))
	if len(query.EventTypes) > 0 && !agentSightStringInList(eventType, query.EventTypes) {
		return false
	}
	if filterET := recentEventFilterField(query.Filters, "event_type"); filterET != "" && !strings.EqualFold(eventType, filterET) {
		return false
	}
	if filterRedaction := recentEventFilterField(query.Filters, "redaction_state"); filterRedaction != "" {
		redaction := platform.FirstNonEmpty(stringFromMap(event.Data, "redaction_state"), stringFromMap(event.Data, "redactionState"))
		if !strings.EqualFold(redaction, filterRedaction) {
			return false
		}
	}
	eventTime := time.UnixMilli(event.Timestamp)
	if filterSince := recentEventFilterTime(query.Filters, "since"); !filterSince.IsZero() && eventTime.Before(filterSince) {
		return false
	}
	if filterUntil := recentEventFilterTime(query.Filters, "until"); !filterUntil.IsZero() && eventTime.After(filterUntil) {
		return false
	}
	if strings.TrimSpace(query.Search) != "" {
		haystack, _ := json.Marshal(event)
		if !strings.Contains(strings.ToLower(string(haystack)), strings.ToLower(strings.TrimSpace(query.Search))) {
			return false
		}
	}
	return true
}

// recentEventFilterField extracts a string field from the opaque filter type.
func recentEventFilterField(filters any, field string) string {
	if m, ok := filters.(map[string]any); ok {
		if v, ok := m[field]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

func recentEventFilterUint32(filters any, field string) uint32 {
	if m, ok := filters.(map[string]any); ok {
		if v, ok := m[field]; ok {
			switch t := v.(type) {
			case uint32:
				return t
			case int:
				return uint32(t)
			}
		}
	}
	return 0
}

func recentEventFilterTime(filters any, field string) time.Time {
	if m, ok := filters.(map[string]any); ok {
		if v, ok := m[field]; ok {
			if t, ok := v.(time.Time); ok {
				return t
			}
		}
	}
	return time.Time{}
}

// Filter builder helpers for JSON request parsing
func setRecentEventFilterSource(source string, filters any) any {
	if source == "" {
		return filters
	}
	if m, ok := filters.(map[string]any); ok {
		m["source"] = source
		return m
	}
	return map[string]any{"source": source}
}

func setRecentEventFilterType(typ string, filters any) any {
	if typ == "" {
		return filters
	}
	if m, ok := filters.(map[string]any); ok {
		m["type"] = typ
		return m
	}
	return map[string]any{"type": typ}
}

func setRecentEventFilterEventType(et string, filters any) any {
	if et == "" {
		return filters
	}
	if m, ok := filters.(map[string]any); ok {
		m["event_type"] = et
		return m
	}
	return map[string]any{"event_type": et}
}

func setRecentEventFilterPID(pid uint32, filters any) any {
	if m, ok := filters.(map[string]any); ok {
		m["pid"] = pid
		return m
	}
	return map[string]any{"pid": pid}
}

func setRecentEventFilterComm(comm string, filters any) any {
	if comm == "" {
		return filters
	}
	if m, ok := filters.(map[string]any); ok {
		m["comm"] = comm
		return m
	}
	return map[string]any{"comm": comm}
}

func setRecentEventFilterTraceID(traceID string, filters any) any {
	if traceID == "" {
		return filters
	}
	if m, ok := filters.(map[string]any); ok {
		m["trace_id"] = traceID
		return m
	}
	return map[string]any{"trace_id": traceID}
}

func setRecentEventFilterSpanID(spanID string, filters any) any {
	if spanID == "" {
		return filters
	}
	if m, ok := filters.(map[string]any); ok {
		m["span_id"] = spanID
		return m
	}
	return map[string]any{"span_id": spanID}
}

func setRecentEventFilterRedactionState(state string, filters any) any {
	if state == "" {
		return filters
	}
	if m, ok := filters.(map[string]any); ok {
		m["redaction_state"] = state
		return m
	}
	return map[string]any{"redaction_state": state}
}

func setRecentEventFilterSince(since time.Time, filters any) any {
	if m, ok := filters.(map[string]any); ok {
		m["since"] = since
		return m
	}
	return map[string]any{"since": since}
}

func setRecentEventFilterUntil(until time.Time, filters any) any {
	if m, ok := filters.(map[string]any); ok {
		m["until"] = until
		return m
	}
	return map[string]any{"until": until}
}

// ── Runner ID assignment ─────────────────────────────────────────────
