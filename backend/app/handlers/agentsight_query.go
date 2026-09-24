package handlers

import (
	"agent-ebpf-filter/app/events"
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/tls"
)

func collectAgentSightEvents(c *gin.Context, tlsStore *tls.TLSCaptureStore) ([]AgentSightExportEvent, string, error) {
	query := agentSightQueryFromRequest(c)
	return collectAgentSightEventsForQuery(c.Request.Context(), query, tlsStore)
}

func collectAgentSightEventsForQuery(ctx context.Context, query AgentSightEventQuery, tlsStore *tls.TLSCaptureStore) ([]AgentSightExportEvent, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	records, source, err := Deps.RuntimeSettings.RecentEventsContext(ctx, query.Limit)
	if err != nil {
		return nil, "", err
	}
	tlsCount := 0
	if tlsStore != nil {
		tlsCount = tlsStore.Count()
	}
	events := make([]AgentSightExportEvent, 0, len(records)+tlsCount)
	for index, record := range records {
		if index%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
		}
		converted := agentSightEventFromCapturedRecord(record)
		if agentSightExportEventMatches(converted, query) {
			events = append(events, converted)
		}
	}
	if query.IncludeTLS && tlsStore != nil {
		for index, event := range tlsStore.Recent(query.Limit) {
			if index%128 == 0 {
				if err := ctx.Err(); err != nil {
					return nil, "", err
				}
			}
			converted := agentSightEventFromTLSPlaintext(event)
			if agentSightExportEventMatches(converted, query) {
				events = append(events, converted)
			}
		}
	}
	for index, event := range Deps.AgentSightUploadedEvents.Recent(query.Limit) {
		if index%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, "", err
			}
		}
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
	if err := ctx.Err(); err != nil {
		return nil, "", err
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

func recentEventFiltersFromRequest(c *gin.Context) events.RecentEventFilters {
	return events.RecentEventFiltersFromRequest(c)
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
	// Body fields override the matching query parameters when present.
	filters := &query.Filters
	filters.Source = platform.FirstNonEmpty(body.Source, filters.Source)
	query.Sources = append(query.Sources, normalizeAgentSightTerms(body.Sources)...)
	filters.Type = platform.FirstNonEmpty(body.Type, filters.Type)
	filters.EventType = platform.FirstNonEmpty(body.EventType, body.EventTypeCamel, filters.EventType)
	query.EventTypes = append(query.EventTypes, normalizeAgentSightTerms(body.EventTypes)...)
	if body.PID != 0 {
		filters.PID = body.PID
	}
	query.PIDs = append(query.PIDs, body.PIDs...)
	filters.Comm = platform.FirstNonEmpty(body.Comm, filters.Comm)
	filters.TraceID = platform.FirstNonEmpty(body.TraceID, body.TraceIDCamel, filters.TraceID)
	filters.SpanID = platform.FirstNonEmpty(body.SpanID, body.SpanIDCamel, filters.SpanID)
	filters.RedactionState = platform.FirstNonEmpty(body.RedactionState, filters.RedactionState)
	if parsed := parseAgentSightTimeAny(body.Since); !parsed.IsZero() {
		filters.Since = parsed
	}
	if parsed := parseAgentSightTimeAny(body.Until); !parsed.IsZero() {
		filters.Until = parsed
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
	filters := query.Filters
	if filters.Source != "" && !strings.EqualFold(event.Source, filters.Source) {
		return false
	}
	if len(query.PIDs) > 0 && !agentSightUint32InList(event.PID, query.PIDs) {
		return false
	}
	if filters.PID != 0 && event.PID != filters.PID {
		return false
	}
	if filters.Comm != "" && !strings.Contains(strings.ToLower(event.Comm), strings.ToLower(filters.Comm)) {
		return false
	}
	if filters.TraceID != "" && event.TraceID != filters.TraceID {
		return false
	}
	if filters.SpanID != "" && event.SpanID != filters.SpanID {
		return false
	}
	if filters.Type != "" && !strings.EqualFold(stringFromMap(event.Data, "type"), filters.Type) {
		return false
	}
	eventType := platform.FirstNonEmpty(stringFromMap(event.Data, "event_type"), stringFromMap(event.Data, "eventType"), stringFromMap(event.Data, "type"))
	if len(query.EventTypes) > 0 && !agentSightStringInList(eventType, query.EventTypes) {
		return false
	}
	if filters.EventType != "" && !strings.EqualFold(eventType, filters.EventType) {
		return false
	}
	if filters.RedactionState != "" {
		redaction := platform.FirstNonEmpty(stringFromMap(event.Data, "redaction_state"), stringFromMap(event.Data, "redactionState"))
		if !strings.EqualFold(redaction, filters.RedactionState) {
			return false
		}
	}
	eventTime := time.UnixMilli(event.Timestamp)
	if !filters.Since.IsZero() && eventTime.Before(filters.Since) {
		return false
	}
	if !filters.Until.IsZero() && eventTime.After(filters.Until) {
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

// ── Runner ID assignment ─────────────────────────────────────────────
