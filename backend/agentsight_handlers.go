package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
)

const (
	agentSightDefaultLimit = 500
	agentSightMaxLimit     = 5000
)

var agentSightUploadedEvents = newAgentSightEventStore(2000)

type agentSightExportEvent struct {
	ID        string         `json:"id,omitempty"`
	Timestamp int64          `json:"timestamp"`
	Source    string         `json:"source"`
	PID       uint32         `json:"pid"`
	PPID      uint32         `json:"ppid,omitempty"`
	Comm      string         `json:"comm"`
	TraceID   string         `json:"trace_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
	Data      map[string]any `json:"data"`
}

type agentSightEventQuery struct {
	Limit      int
	Filters    recentEventFilters
	Search     string
	IncludeTLS bool
	Sources    []string
	EventTypes []string
	PIDs       []uint32
	RunnerID   string
}

func registerAgentSightRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	router.GET("/agentsight/runners", handleAgentSightRunners(tlsStore))
	router.GET("/agentsight/events", handleAgentSightEvents(tlsStore, false))
	router.POST("/agentsight/events", handleAgentSightEventsUpload)
	router.GET("/agentsight/events.jsonl", handleAgentSightEvents(tlsStore, true))
	router.GET("/agentsight/events/stats", handleAgentSightEventsStats(tlsStore, ""))
	router.GET("/agentsight/events/runners/:id/stats", handleAgentSightRunnerStats(tlsStore))
	router.POST("/agentsight/events/query", handleAgentSightEventsQuery(tlsStore))
	router.GET("/agentsight/events/stream", handleAgentSightEventsStream(tlsStore))
	router.GET("/agentsight/stream/merged", handleAgentSightEventsStream(tlsStore))
	router.GET("/agentsight/stream/runner/:id", handleAgentSightRunnerStream(tlsStore))
}

func registerAgentSightCompatibilityRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	router.GET("/runners", handleAgentSightRunners(tlsStore))
	router.GET("/events", handleAgentSightEvents(tlsStore, true))
	router.POST("/events", handleAgentSightEventsUpload)
	router.GET("/events/stats", handleAgentSightEventsStats(tlsStore, ""))
	router.GET("/events/runners/:id/stats", handleAgentSightRunnerStats(tlsStore))
	router.POST("/events/query", handleAgentSightEventsQuery(tlsStore))
	router.GET("/events/stream", handleAgentSightEventsStream(tlsStore))
	router.GET("/stream/merged", handleAgentSightEventsStream(tlsStore))
	router.GET("/stream/runner/:id", handleAgentSightRunnerStream(tlsStore))
}

type agentSightEventStore struct {
	mu     sync.RWMutex
	events []agentSightExportEvent
	max    int
}

func newAgentSightEventStore(max int) *agentSightEventStore {
	if max <= 0 {
		max = 1000
	}
	return &agentSightEventStore{max: max}
}

func (s *agentSightEventStore) Add(events ...agentSightExportEvent) {
	if s == nil || len(events) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
	if len(s.events) > s.max {
		copy(s.events, s.events[len(s.events)-s.max:])
		s.events = s.events[:s.max]
	}
}

func (s *agentSightEventStore) Recent(limit int) []agentSightExportEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	if limit == 0 {
		return nil
	}
	out := make([]agentSightExportEvent, limit)
	copy(out, s.events[len(s.events)-limit:])
	return out
}

func (s *agentSightEventStore) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = nil
}

func handleAgentSightEvents(tlsStore *TLSCaptureStore, forceJSONL bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		events, source, err := collectAgentSightEvents(c, tlsStore)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		writeAgentSightEvents(c, events, source, forceJSONL)
	}
}

func handleAgentSightEventsQuery(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, err := agentSightQueryFromJSONRequest(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		events, source, err := collectAgentSightEventsForQuery(query, tlsStore)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"source": source,
			"events": events,
			"stats":  buildAgentSightEventsStats(events, query.Limit, query.RunnerID),
		})
	}
}

func handleAgentSightEventsStream(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		seen := make(map[string]struct{})
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		emitSnapshot := func() {
			events, _, err := collectAgentSightEvents(c, tlsStore)
			if err != nil {
				c.SSEvent("error", gin.H{"error": err.Error()})
				return
			}
			for _, event := range events {
				key := event.ID
				if key == "" {
					key = agentSightStableID("event", event.Timestamp, event.Source, event.PID, event.Comm, event.Data)
				}
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				c.SSEvent("event", event)
			}
		}

		c.Stream(func(_ io.Writer) bool {
			emitSnapshot()
			select {
			case <-c.Request.Context().Done():
				return false
			case <-ticker.C:
				return true
			}
		})
	}
}

func handleAgentSightRunnerStream(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.URL.RawQuery = appendQueryValue(c.Request.URL.RawQuery, "runner", c.Param("id"))
		handleAgentSightEventsStream(tlsStore)(c)
	}
}

func handleAgentSightEventsUpload(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	events, err := parseAgentSightUploadPayload(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agentSightUploadedEvents.Add(events...)
	c.JSON(http.StatusOK, gin.H{"imported": len(events)})
}

func handleAgentSightRunners(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		events, _, err := collectAgentSightEventsForQuery(agentSightEventQuery{Limit: agentSightMaxLimit, IncludeTLS: true}, tlsStore)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"runners": buildAgentSightRunners(events, tlsStore),
		})
	}
}

func handleAgentSightEventsStats(tlsStore *TLSCaptureStore, runnerID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := agentSightQueryFromRequest(c)
		query.Limit = agentSightMaxLimit
		query.RunnerID = firstNonEmpty(runnerID, query.RunnerID)
		events, _, err := collectAgentSightEventsForQuery(query, tlsStore)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildAgentSightEventsStats(events, query.Limit, query.RunnerID))
	}
}

func handleAgentSightRunnerStats(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		runnerID := strings.TrimSpace(c.Param("id"))
		handleAgentSightEventsStats(tlsStore, runnerID)(c)
	}
}

func collectAgentSightEvents(c *gin.Context, tlsStore *TLSCaptureStore) ([]agentSightExportEvent, string, error) {
	query := agentSightQueryFromRequest(c)
	return collectAgentSightEventsForQuery(query, tlsStore)
}

func collectAgentSightEventsForQuery(query agentSightEventQuery, tlsStore *TLSCaptureStore) ([]agentSightExportEvent, string, error) {
	records, source, err := runtimeSettingsStore.RecentEvents(query.Limit)
	if err != nil {
		return nil, "", err
	}
	archiveFilters := query.Filters
	// AgentSight presents semantic sources such as "file", "process", and
	// "http_parser", while the raw EventEnvelope source for many kernel events is
	// "ebpf_ringbuf". Apply the source filter after conversion so compatibility
	// clients can use the same source vocabulary as docs/ref/agentsight.
	archiveFilters.Source = ""
	records = filterRecentEventRecords(records, archiveFilters)

	tlsCount := 0
	if tlsStore != nil {
		tlsCount = tlsStore.Count()
	}
	events := make([]agentSightExportEvent, 0, len(records)+tlsCount)
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
	for _, event := range agentSightUploadedEvents.Recent(query.Limit) {
		if agentSightExportEventMatches(event, query) {
			events = append(events, event)
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

func agentSightQueryFromRequest(c *gin.Context) agentSightEventQuery {
	query := agentSightEventQuery{
		Limit:      agentSightDefaultLimit,
		Filters:    recentEventFiltersFromRequest(c),
		Search:     firstNonEmpty(c.Query("filter"), c.Query("q"), c.Query("search")),
		IncludeTLS: true,
		Sources:    splitAgentSightCSV(firstNonEmpty(c.Query("sources"), c.Query("source[]"))),
		EventTypes: splitAgentSightCSV(firstNonEmpty(c.Query("event_types"), c.Query("eventTypes"), c.Query("event_type[]"))),
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

func agentSightQueryFromJSONRequest(c *gin.Context) (agentSightEventQuery, error) {
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
	query.Search = firstNonEmpty(body.Filter, body.Search, body.Query, query.Search)
	query.Filters.Source = firstNonEmpty(body.Source, query.Filters.Source)
	query.Sources = append(query.Sources, normalizeAgentSightTerms(body.Sources)...)
	query.Filters.Type = firstNonEmpty(body.Type, query.Filters.Type)
	query.Filters.EventType = firstNonEmpty(body.EventType, body.EventTypeCamel, query.Filters.EventType)
	query.EventTypes = append(query.EventTypes, normalizeAgentSightTerms(body.EventTypes)...)
	if body.PID != 0 {
		query.Filters.PID = body.PID
	}
	query.PIDs = append(query.PIDs, body.PIDs...)
	query.Filters.Comm = firstNonEmpty(body.Comm, query.Filters.Comm)
	query.Filters.TraceID = firstNonEmpty(body.TraceID, body.TraceIDCamel, query.Filters.TraceID)
	query.Filters.SpanID = firstNonEmpty(body.SpanID, body.SpanIDCamel, query.Filters.SpanID)
	query.Filters.RedactionState = firstNonEmpty(body.RedactionState, query.Filters.RedactionState)
	if parsed := parseAgentSightTimeAny(body.Since); !parsed.IsZero() {
		query.Filters.Since = parsed
	}
	if parsed := parseAgentSightTimeAny(body.Until); !parsed.IsZero() {
		query.Filters.Until = parsed
	}
	if body.IncludeTLS != nil {
		query.IncludeTLS = *body.IncludeTLS
	}
	if body.IncludeTLSCamel != nil {
		query.IncludeTLS = *body.IncludeTLSCamel
	}
	query.RunnerID = firstNonEmpty(body.Runner, body.RunnerID, query.RunnerID)
	return query, nil
}

type agentSightRunnerStatus struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Enabled        bool           `json:"enabled"`
	Running        bool           `json:"running"`
	State          string         `json:"state"`
	EventCount     int            `json:"event_count"`
	LastEventTs    int64          `json:"last_event_timestamp,omitempty"`
	LastEventISO   string         `json:"last_event_time,omitempty"`
	SupportedStart bool           `json:"supported_start"`
	SupportedStop  bool           `json:"supported_stop"`
	Details        map[string]any `json:"details,omitempty"`
}

type agentSightEventsStats struct {
	Total              int            `json:"total"`
	Limit              int            `json:"limit"`
	Runner             string         `json:"runner,omitempty"`
	BySource           map[string]int `json:"by_source"`
	ByType             map[string]int `json:"by_type"`
	ByRunner           map[string]int `json:"by_runner"`
	ByComm             map[string]int `json:"by_comm"`
	EarliestTimestamp  int64          `json:"earliest_timestamp,omitempty"`
	LatestTimestamp    int64          `json:"latest_timestamp,omitempty"`
	EarliestTime       string         `json:"earliest_time,omitempty"`
	LatestTime         string         `json:"latest_time,omitempty"`
	GeneratedTimestamp int64          `json:"generated_timestamp"`
	GeneratedTime      string         `json:"generated_time"`
}

func buildAgentSightRunners(events []agentSightExportEvent, tlsStore *TLSCaptureStore) []agentSightRunnerStatus {
	stats := buildAgentSightEventsStats(events, agentSightMaxLimit, "")
	settings := runtimeSettingsStore.Snapshot()
	collector := collectorMetricsStore.Snapshot()
	now := time.Now().UTC()
	runners := []agentSightRunnerStatus{
		{
			ID:          "process",
			Name:        "Process/File/eBPF runner",
			Type:        "process",
			Enabled:     true,
			Running:     collector.CollectorMapAvailable || stats.ByRunner["process"] > 0,
			State:       agentSightRunnerState(collector.CollectorMapAvailable || stats.ByRunner["process"] > 0),
			EventCount:  stats.ByRunner["process"],
			LastEventTs: lastAgentSightRunnerEventTs(events, "process"),
			Details: map[string]any{
				"collectorMapAvailable": collector.CollectorMapAvailable,
				"captureHealthy":        collector.CaptureHealthy,
				"ringbufEventsTotal":    collector.RingbufEventsTotal,
			},
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

func buildAgentSightEventsStats(events []agentSightExportEvent, limit int, runnerID string) agentSightEventsStats {
	now := time.Now().UTC()
	stats := agentSightEventsStats{
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
		if eventType := firstNonEmpty(stringFromMap(event.Data, "event_type"), stringFromMap(event.Data, "eventType"), stringFromMap(event.Data, "type")); eventType != "" {
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

func lastAgentSightRunnerEventTs(events []agentSightExportEvent, runnerID string) int64 {
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

func tlsStoreCount(store *TLSCaptureStore) int {
	if store == nil {
		return 0
	}
	return store.Count()
}

func tlsStoreLibraries(store *TLSCaptureStore) []TLSLibraryStatus {
	if store == nil {
		return nil
	}
	return store.LibraryStatuses()
}

func writeAgentSightEvents(c *gin.Context, events []agentSightExportEvent, source string, forceJSONL bool) {
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

func agentSightEventsJSONL(events []agentSightExportEvent) string {
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

func parseAgentSightUploadPayload(body []byte) ([]agentSightExportEvent, error) {
	raw := strings.TrimSpace(string(body))
	if raw == "" {
		return nil, fmt.Errorf("empty AgentSight event payload")
	}
	var decoded any
	if err := json.Unmarshal([]byte(raw), &decoded); err == nil {
		return agentSightEventsFromDecodedPayload(decoded)
	}
	events := make([]agentSightExportEvent, 0)
	for index, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var item any
		if err := json.Unmarshal([]byte(line), &item); err != nil {
			return nil, fmt.Errorf("line %d: %w", index+1, err)
		}
		event, err := agentSightEventFromDecodedPayload(item, index)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", index+1, err)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no AgentSight events found")
	}
	return events, nil
}

func agentSightEventsFromDecodedPayload(decoded any) ([]agentSightExportEvent, error) {
	switch typed := decoded.(type) {
	case []any:
		events := make([]agentSightExportEvent, 0, len(typed))
		for index, item := range typed {
			event, err := agentSightEventFromDecodedPayload(item, index)
			if err != nil {
				return nil, err
			}
			events = append(events, event)
		}
		return events, nil
	case map[string]any:
		if nested, ok := typed["events"]; ok {
			return agentSightEventsFromDecodedPayload(nested)
		}
		if nested, ok := typed["records"]; ok {
			return agentSightEventsFromDecodedPayload(nested)
		}
		event, err := agentSightEventFromDecodedPayload(typed, 0)
		if err != nil {
			return nil, err
		}
		return []agentSightExportEvent{event}, nil
	default:
		return nil, fmt.Errorf("unsupported AgentSight payload type %T", decoded)
	}
}

func agentSightEventFromDecodedPayload(decoded any, index int) (agentSightExportEvent, error) {
	values, ok := decoded.(map[string]any)
	if !ok {
		return agentSightExportEvent{}, fmt.Errorf("event must be an object")
	}
	data := mapFromAny(values["data"])
	if data == nil {
		data = map[string]any{}
		if values["data"] != nil {
			data["value"] = values["data"]
		}
	}
	timestamp := parseAgentSightTimestamp(firstNonNil(values["timestamp"], data["timestamp"]), time.Now().Add(time.Duration(index)*time.Millisecond).UnixMilli())
	source := firstNonEmpty(stringFromMap(values, "source"), stringFromMap(data, "source"), "imported")
	pid := uint32FromAny(firstNonNil(values["pid"], data["pid"]))
	ppid := uint32FromAny(firstNonNil(values["ppid"], data["ppid"], data["parent_pid"], data["parentPid"]))
	comm := firstNonEmpty(stringFromMap(values, "comm"), stringFromMap(data, "comm"), "imported")
	traceID := firstNonEmpty(stringFromMap(values, "trace_id"), stringFromMap(values, "traceId"), stringFromMap(data, "trace_id"), stringFromMap(data, "traceId"))
	spanID := firstNonEmpty(stringFromMap(values, "span_id"), stringFromMap(values, "spanId"), stringFromMap(data, "span_id"), stringFromMap(data, "spanId"))
	if data["event_type"] == nil && data["eventType"] != nil {
		data["event_type"] = data["eventType"]
	}
	if data["runner"] == nil {
		data["runner"] = "uploaded"
	}
	id := firstNonEmpty(stringFromMap(values, "id"), agentSightStableID("import", timestamp, source, pid, comm, data))
	return agentSightExportEvent{
		ID:        id,
		Timestamp: timestamp,
		Source:    source,
		PID:       pid,
		PPID:      ppid,
		Comm:      comm,
		TraceID:   traceID,
		SpanID:    spanID,
		Data:      data,
	}, nil
}

func agentSightEventFromCapturedRecord(record CapturedEventRecord) agentSightExportEvent {
	record = normalizeCapturedEventRecord(record)
	envelope := record.Envelope
	event := record.Event

	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() && envelope != nil && envelope.GetTimestampNs() > 0 {
		timestamp = time.Unix(0, int64(envelope.GetTimestampNs())).UTC()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	envelopeMap := eventEnvelopeToJSONValue(envelope)
	eventMap := agentSightProtoMap(event)
	payloadMap, payloadKey := agentSightEnvelopePayload(envelopeMap)
	data := make(map[string]any, len(payloadMap)+len(eventMap)+8)
	for key, value := range payloadMap {
		data[key] = value
	}
	for key, value := range eventMap {
		if _, exists := data[key]; !exists {
			data[key] = value
		}
	}

	eventType := envelopeEventTypeName(envelope, event)
	if eventType == "" {
		eventType = firstNonEmpty(stringFromMap(data, "event_type"), stringFromMap(data, "eventType"), stringFromMap(data, "type"))
	}
	data["event_type"] = eventType
	data["type"] = firstNonEmpty(stringFromMap(data, "type"), eventType)
	data["payload"] = payloadKey
	if len(eventMap) > 0 {
		data["legacy_event"] = eventMap
	}
	if len(envelopeMap) > 0 {
		data["envelope"] = envelopeMap
	}

	source := agentSightSourceFromEnvelope(envelope, event, payloadKey)
	pid := uint32FromEnvelopeOrEvent(envelope, event, "pid")
	ppid := uint32FromEnvelopeOrEvent(envelope, event, "ppid")
	comm := firstNonEmpty(envelopeString(envelope, "comm"), event.GetComm(), stringFromMap(data, "comm"), "unknown")
	traceID := firstNonEmpty(envelopeString(envelope, "trace_id"), event.GetTraceId(), stringFromMap(data, "trace_id"), stringFromMap(data, "traceId"))
	spanID := firstNonEmpty(envelopeString(envelope, "span_id"), event.GetSpanId(), stringFromMap(data, "span_id"), stringFromMap(data, "spanId"))

	id := ""
	if envelope != nil {
		id = envelope.GetEventId()
	}
	if id == "" {
		id = agentSightStableID("event", timestamp.UnixMilli(), source, pid, comm, data)
	}

	return agentSightExportEvent{
		ID:        id,
		Timestamp: timestamp.UnixMilli(),
		Source:    source,
		PID:       pid,
		PPID:      ppid,
		Comm:      comm,
		TraceID:   traceID,
		SpanID:    spanID,
		Data:      data,
	}
}

func agentSightEventFromTLSPlaintext(event TLSPlaintextEvent) agentSightExportEvent {
	timestamp := event.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	data := agentSightStructMap(event)
	data["timestamp"] = timestamp.UnixMilli()
	data["type"] = firstNonEmpty(event.Type, "tls_plaintext")
	data["event_type"] = agentSightTLSEventType(event)
	data["status_code"] = event.StatusCode
	data["path"] = firstNonEmpty(event.URL, stringFromMap(data, "path"))
	data["agent_run_id"] = event.AgentRunID
	data["task_id"] = event.TaskID
	data["trace_id"] = event.TraceID
	data["span_id"] = event.SpanID

	source := agentSightSourceFromTLS(event)
	id := agentSightStableID("tls", timestamp.UnixMilli(), source, event.PID, event.Comm, event.Type, event.Method, event.URL, event.StatusCode, event.PromptDigest)
	return agentSightExportEvent{
		ID:        id,
		Timestamp: timestamp.UnixMilli(),
		Source:    source,
		PID:       event.PID,
		Comm:      firstNonEmpty(event.Comm, "tls"),
		TraceID:   event.TraceID,
		SpanID:    event.SpanID,
		Data:      data,
	}
}

func agentSightExportEventMatches(event agentSightExportEvent, query agentSightEventQuery) bool {
	if query.RunnerID != "" && !strings.EqualFold(agentSightRunnerIDForEvent(event), query.RunnerID) {
		return false
	}
	if len(query.Sources) > 0 && !agentSightStringInList(event.Source, query.Sources) {
		return false
	}
	if query.Filters.Source != "" && !strings.EqualFold(event.Source, query.Filters.Source) {
		return false
	}
	if len(query.PIDs) > 0 && !agentSightUint32InList(event.PID, query.PIDs) {
		return false
	}
	if query.Filters.PID != 0 && event.PID != query.Filters.PID {
		return false
	}
	if query.Filters.Comm != "" && !strings.Contains(strings.ToLower(event.Comm), strings.ToLower(query.Filters.Comm)) {
		return false
	}
	if query.Filters.TraceID != "" && event.TraceID != query.Filters.TraceID {
		return false
	}
	if query.Filters.SpanID != "" && event.SpanID != query.Filters.SpanID {
		return false
	}
	if query.Filters.Type != "" && !strings.EqualFold(stringFromMap(event.Data, "type"), query.Filters.Type) {
		return false
	}
	eventType := firstNonEmpty(stringFromMap(event.Data, "event_type"), stringFromMap(event.Data, "eventType"), stringFromMap(event.Data, "type"))
	if len(query.EventTypes) > 0 && !agentSightStringInList(eventType, query.EventTypes) {
		return false
	}
	if query.Filters.EventType != "" && !strings.EqualFold(eventType, query.Filters.EventType) {
		return false
	}
	if query.Filters.RedactionState != "" {
		redaction := firstNonEmpty(stringFromMap(event.Data, "redaction_state"), stringFromMap(event.Data, "redactionState"))
		if !strings.EqualFold(redaction, query.Filters.RedactionState) {
			return false
		}
	}
	eventTime := time.UnixMilli(event.Timestamp)
	if !query.Filters.Since.IsZero() && eventTime.Before(query.Filters.Since) {
		return false
	}
	if !query.Filters.Until.IsZero() && eventTime.After(query.Filters.Until) {
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

func agentSightRunnerIDForEvent(event agentSightExportEvent) string {
	if strings.EqualFold(stringFromMap(event.Data, "runner"), "uploaded") || strings.HasPrefix(event.ID, "import-") {
		return "uploaded"
	}
	switch strings.ToLower(event.Source) {
	case "ssl", "http_parser", "sse_processor":
		return "tls"
	case "stdio", "mcp":
		return "stdio"
	case "system":
		return "system"
	case "agent", "wrapper", "native_hook", "policy", "otel", "semantic_alert":
		return "agent"
	case "process", "file", "network", "ebpf_ringbuf":
		return "process"
	default:
		eventType := strings.ToLower(firstNonEmpty(stringFromMap(event.Data, "event_type"), stringFromMap(event.Data, "type")))
		switch {
		case strings.Contains(eventType, "tls") || strings.Contains(eventType, "http") || strings.Contains(eventType, "sse"):
			return "tls"
		case strings.Contains(eventType, "stdio") || strings.Contains(eventType, "mcp"):
			return "stdio"
		case strings.Contains(eventType, "system"):
			return "system"
		case strings.Contains(eventType, "policy") || strings.Contains(eventType, "alert") || strings.Contains(eventType, "wrapper") || strings.Contains(eventType, "hook"):
			return "agent"
		default:
			return "process"
		}
	}
}

func agentSightProtoMap(message proto.Message) map[string]any {
	if message == nil {
		return nil
	}
	payload, err := eventEnvelopeJSONMarshaller.Marshal(message)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return decoded
}

func agentSightStructMap(value any) map[string]any {
	payload, err := json.Marshal(value)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return decoded
}

func agentSightEnvelopePayload(envelope map[string]any) (map[string]any, string) {
	for _, key := range []string{
		"tls_event", "tlsEvent",
		"http_event", "httpEvent",
		"sse_event", "sseEvent",
		"stdio_event", "stdioEvent",
		"system_metric_event", "systemMetricEvent",
		"otel_span_event", "otelSpanEvent",
		"agentsight_alert_event", "agentsightAlertEvent",
		"network_event", "networkEvent",
		"process_event", "processEvent",
		"file_event", "fileEvent",
		"policy_event", "policyEvent",
		"wrapper_event", "wrapperEvent",
		"hook_event", "hookEvent",
		"mcp_event", "mcpEvent",
		"exec_event", "execEvent",
	} {
		if payload, ok := envelope[key].(map[string]any); ok {
			return payload, key
		}
	}
	return map[string]any{}, ""
}

func agentSightSourceFromEnvelope(envelope *pb.EventEnvelope, event *pb.Event, payloadKey string) string {
	switch payloadKey {
	case "tls_event", "tlsEvent":
		return "ssl"
	case "http_event", "httpEvent":
		return "http_parser"
	case "sse_event", "sseEvent":
		return "sse_processor"
	case "stdio_event", "stdioEvent", "mcp_event", "mcpEvent":
		return "stdio"
	case "system_metric_event", "systemMetricEvent":
		return "system"
	case "otel_span_event", "otelSpanEvent":
		return "otel"
	case "agentsight_alert_event", "agentsightAlertEvent", "policy_event", "policyEvent":
		return "policy"
	case "network_event", "networkEvent":
		return "network"
	case "process_event", "processEvent", "exec_event", "execEvent":
		return "process"
	case "file_event", "fileEvent":
		return "file"
	case "wrapper_event", "wrapperEvent", "hook_event", "hookEvent":
		return "agent"
	}
	if event != nil {
		eventType := strings.ToLower(event.GetType() + " " + event.GetEventType().String())
		switch {
		case strings.Contains(eventType, "tls"):
			return "ssl"
		case strings.Contains(eventType, "http"):
			return "http_parser"
		case strings.Contains(eventType, "sse"):
			return "sse_processor"
		case strings.Contains(eventType, "stdio") || strings.Contains(eventType, "mcp"):
			return "stdio"
		case strings.Contains(eventType, "system"):
			return "system"
		case strings.Contains(eventType, "alert") || strings.Contains(eventType, "policy") || strings.TrimSpace(event.GetDecision()) != "":
			return "policy"
		case strings.Contains(eventType, "network") || strings.Contains(eventType, "tcp") || strings.Contains(eventType, "dns") || strings.Contains(eventType, "socket"):
			return "network"
		case strings.Contains(eventType, "exec") || strings.Contains(eventType, "clone") || strings.Contains(eventType, "fork") || strings.Contains(eventType, "exit") || strings.Contains(eventType, "wait"):
			return "process"
		case strings.Contains(eventType, "open") || strings.Contains(eventType, "read") || strings.Contains(eventType, "write") || strings.Contains(eventType, "file") || strings.Contains(eventType, "unlink") || strings.Contains(eventType, "rename") || strings.Contains(eventType, "chmod") || strings.Contains(eventType, "chown") || strings.Contains(eventType, "mkdir") || strings.Contains(eventType, "link") || strings.Contains(eventType, "mknod"):
			return "file"
		case strings.Contains(eventType, "wrapper") || strings.Contains(eventType, "hook"):
			return "agent"
		}
	}
	if envelope != nil && strings.TrimSpace(envelope.GetSource()) != "" {
		return envelope.GetSource()
	}
	return determineEventEnvelopeSource(event)
}

func agentSightSourceFromTLS(event TLSPlaintextEvent) string {
	switch event.Type {
	case "http_request", "http_response":
		return "http_parser"
	case "sse_message":
		return "sse_processor"
	default:
		return "ssl"
	}
}

func agentSightTLSEventType(event TLSPlaintextEvent) string {
	switch event.Type {
	case "http_request", "http_response":
		return "HTTP_MESSAGE"
	case "sse_message":
		return "SSE_MESSAGE"
	default:
		return "TLS_PLAINTEXT"
	}
}

func uint32FromEnvelopeOrEvent(envelope *pb.EventEnvelope, event *pb.Event, field string) uint32 {
	switch field {
	case "pid":
		if envelope != nil && envelope.GetPid() != 0 {
			return envelope.GetPid()
		}
		if event != nil {
			return event.GetPid()
		}
	case "ppid":
		if envelope != nil && envelope.GetPpid() != 0 {
			return envelope.GetPpid()
		}
		if event != nil {
			return event.GetPpid()
		}
	}
	return 0
}

func envelopeString(envelope *pb.EventEnvelope, field string) string {
	if envelope == nil {
		return ""
	}
	switch field {
	case "comm":
		return envelope.GetComm()
	case "trace_id":
		return envelope.GetTraceId()
	case "span_id":
		return envelope.GetSpanId()
	}
	return ""
}

func stringFromMap(values map[string]any, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case fmt.Stringer:
		return typed.String()
	default:
		return fmt.Sprint(typed)
	}
}

func mapFromAny(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func splitAgentSightCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeAgentSightTerms(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t'
	}))
}

func normalizeAgentSightTerms(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func parseAgentSightPIDList(raw string) []uint32 {
	parts := splitAgentSightCSV(raw)
	out := make([]uint32, 0, len(parts))
	for _, part := range parts {
		if parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32); err == nil && parsed > 0 {
			out = append(out, uint32(parsed))
		}
	}
	return out
}

func agentSightStringInList(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func agentSightUint32InList(value uint32, candidates []uint32) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func parseAgentSightTimeAny(value any) time.Time {
	if value == nil {
		return time.Time{}
	}
	switch typed := value.(type) {
	case string:
		return parseRecentEventTime(typed)
	case float64:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	case int64:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	case int:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	case json.Number:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	default:
		return time.Time{}
	}
}

func agentSightTimeFromMillis(millis int64) time.Time {
	if millis <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis).UTC()
}

func appendQueryValue(rawQuery, key, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return rawQuery
	}
	if strings.TrimSpace(rawQuery) == "" {
		return key + "=" + value
	}
	return rawQuery + "&" + key + "=" + value
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func uint32FromAny(value any) uint32 {
	switch typed := value.(type) {
	case uint32:
		return typed
	case uint64:
		return uint32(typed)
	case int:
		if typed > 0 {
			return uint32(typed)
		}
	case int64:
		if typed > 0 {
			return uint32(typed)
		}
	case float64:
		if typed > 0 {
			return uint32(typed)
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return uint32(parsed)
		}
	case string:
		if parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 32); err == nil {
			return uint32(parsed)
		}
	}
	return 0
}

func parseAgentSightTimestamp(value any, fallback int64) int64 {
	var numeric float64
	switch typed := value.(type) {
	case int64:
		numeric = float64(typed)
	case int:
		numeric = float64(typed)
	case uint64:
		numeric = float64(typed)
	case float64:
		numeric = typed
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return fallback
		}
		numeric = parsed
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return fallback
		}
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil {
			numeric = parsed
			break
		}
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed.UTC().UnixMilli()
		}
		return fallback
	default:
		return fallback
	}
	if numeric <= 0 {
		return fallback
	}
	switch {
	case numeric > 1_000_000_000_000_000:
		return int64(numeric / 1_000_000)
	case numeric > 10_000_000_000_000:
		return int64(numeric / 1_000)
	default:
		return int64(numeric)
	}
}

func agentSightStableID(prefix string, parts ...any) string {
	hash := sha256.New()
	for _, part := range parts {
		payload, err := json.Marshal(part)
		if err != nil {
			payload = []byte(fmt.Sprint(part))
		}
		_, _ = hash.Write(payload)
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "-" + hex.EncodeToString(hash.Sum(nil))[:20]
}
