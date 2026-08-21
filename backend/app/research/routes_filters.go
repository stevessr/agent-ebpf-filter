package research

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/platform"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const researchControlRequestMaxBytes int64 = 64 << 10

func bindResearchJSON(c *gin.Context, target any) (int, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, researchControlRequestMaxBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return http.StatusRequestEntityTooLarge, err
		}
		return http.StatusBadRequest, err
	}
	return http.StatusOK, nil
}

func handleResearchSessionsList(c *gin.Context) {
	sessions, err := researchSessionsStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func handleResearchSessionsCreate(c *gin.Context) {
	var req researchCreateSessionRequest
	if c.Request.Body != nil && c.Request.ContentLength != 0 {
		if status, err := bindResearchJSON(c, &req); err != nil {
			c.JSON(status, gin.H{"error": "invalid research session payload"})
			return
		}
	}
	session, err := researchSessionsStore.Create(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, session)
}

func handleResearchSessionGet(c *gin.Context) {
	session, err := researchSessionsStore.Get(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, session)
}

func handleResearchSessionDelete(c *gin.Context) {
	if err := researchSessionsStore.Delete(c.Param("id")); err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

func handleResearchSessionTask(tlsStore *TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req researchTaskRequest
		if c.Request.Body != nil && c.Request.ContentLength != 0 {
			if status, err := bindResearchJSON(c, &req); err != nil {
				c.JSON(status, gin.H{"error": "invalid research task payload"})
				return
			}
		}
		task, err := researchTaskStore.Submit(c.Param("id"), req, tlsStore)
		if err != nil {
			if errors.Is(err, errResearchQueueFull) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
				return
			}
			if errors.Is(err, os.ErrNotExist) {
				c.JSON(http.StatusNotFound, gin.H{"error": "research session not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusAccepted, task)
	}
}

func handleResearchTaskGet(c *gin.Context) {
	task, ok := researchTaskStore.Get(c.Param("taskId"))
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "research task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

func handleResearchTasksStatus(c *gin.Context) {
	c.JSON(http.StatusOK, researchTaskStore.Status())
}

func handleResearchTaskCancel(c *gin.Context) {
	task := researchTaskStore.Cancel(c.Param("taskId"))
	status := http.StatusAccepted
	if task.Error == "task not found" {
		status = http.StatusNotFound
	}
	c.JSON(status, task)
}

func handleResearchSessionEvents(c *gin.Context) {
	events, err := researchSessionsStore.LoadEvents(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	filter := researchSourceFilterFromQuery(c)
	timerange := researchTimeRangeFromQuery(c)
	filtered := make([]ResearchEvent, 0, len(events))
	for _, event := range events {
		if researchEventMatches(event, filter, timerange) {
			filtered = append(filtered, event)
		}
	}
	offset := parseResearchIntQuery(c, "offset", 0, 0, len(filtered))
	limit := parseResearchIntQuery(c, "limit", researchDefaultPageLimit, 1, researchMaxPageLimit)
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	page := []ResearchEvent{}
	if offset < len(filtered) {
		page = filtered[offset:end]
	}
	c.JSON(http.StatusOK, gin.H{"events": page, "total": len(filtered), "offset": offset, "limit": limit})
}

func handleResearchSessionResults(c *gin.Context) {
	results, err := researchSessionsStore.LoadResults(c.Param("id"))
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, results)
}

func handleResearchSessionExport(c *gin.Context) {
	format := normalizeResearchFormat(c.Query("format"))
	if format == "" {
		format = "bundle"
	}
	if format != "jsonl" && format != "csv" && format != "bundle" && format != "json" && format != "security_json" && format != "security_jsonl" && format != "security_csv" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported export format", "supported": []string{"jsonl", "csv", "json", "bundle", "security-json", "security-jsonl", "security-csv"}})
		return
	}
	ref, payload, err := researchSessionsStore.ExportArtifact(c.Param("id"), format)
	if err != nil {
		researchWriteStoreError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", ref.Name))
	c.Header("X-Research-Artifact-SHA256", ref.SHA256)
	c.Data(http.StatusOK, ref.ContentType, payload)
}

func registerResearchRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	researchTaskStore.Start(snapshotRuntimeSettings().ResearchProcessing.QueueSize)
	router.GET("/sessions", handleResearchSessionsList)
	router.POST("/sessions", handleResearchSessionsCreate)
	router.GET("/sessions/:id", handleResearchSessionGet)
	router.DELETE("/sessions/:id", handleResearchSessionDelete)
	router.POST("/sessions/:id/tasks", handleResearchSessionTask(tlsStore))
	router.GET("/tasks/status", handleResearchTasksStatus)
	router.GET("/tasks/:taskId", handleResearchTaskGet)
	router.POST("/tasks/:taskId/cancel", handleResearchTaskCancel)
	router.GET("/sessions/:id/events", handleResearchSessionEvents)
	router.GET("/sessions/:id/results", handleResearchSessionResults)
	router.GET("/sessions/:id/training", handleResearchSessionTraining)
	router.POST("/sessions/:id/training/import", handleResearchSessionTrainingImport)
	router.GET("/sessions/:id/export", handleResearchSessionExport)
}

func researchWriteStoreError(c *gin.Context, err error) {
	if errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusNotFound, gin.H{"error": "research session not found"})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func normalizeResearchAction(action string) string {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "", "scan", "scan_recent":
		return "scan_recent"
	case "build", "build_session":
		return "build_session"
	case "compare", "compare_windows":
		return "compare_windows"
	case "export", "export_bundle":
		return "export_bundle"
	case "reset", "reset_session":
		return "reset_session"
	case "security", "security_eval", "security_evaluation":
		return "security_eval"
	case "cancel":
		return "cancel"
	default:
		return strings.ToLower(strings.TrimSpace(action))
	}
}

func researchFormatsFromRequest(req researchTaskRequest) []string {
	formats := append([]string(nil), req.Formats...)
	if strings.TrimSpace(req.Format) != "" {
		formats = append(formats, req.Format)
	}
	return normalizeResearchFormats(formats)
}

func normalizeResearchExportFormats(raw string) string {
	formats := splitResearchFormats(raw)
	if len(formats) == 0 {
		formats = splitResearchFormats(researchProcessingDefaultExportFormats)
	}
	return strings.Join(formats, ",")
}

func splitResearchFormats(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeResearchFormats(strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n' }))
}

func normalizeResearchFormats(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		format := normalizeResearchFormat(value)
		if format == "" {
			continue
		}
		if _, ok := seen[format]; ok {
			continue
		}
		seen[format] = struct{}{}
		out = append(out, format)
	}
	return out
}

func normalizeResearchFormat(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ndjson", "jsonl":
		return "jsonl"
	case "csv":
		return "csv"
	case "zip", "bundle":
		return "bundle"
	case "json":
		return "json"
	case "security_json", "security-json", "security", "security_eval", "security-eval", "security_evaluation", "security-evaluation":
		return "security_json"
	case "security_jsonl", "security-jsonl", "security_ndjson", "security-ndjson":
		return "security_jsonl"
	case "security_csv", "security-csv":
		return "security_csv"
	default:
		return ""
	}
}

func normalizeResearchSession(session *ResearchSession) {
	if session == nil {
		return
	}
	session.ID = sanitizeResearchIDPart(session.ID)
	if strings.TrimSpace(session.ID) == "" || session.ID == "unknown" {
		session.ID = researchGenerateID("rs")
	}
	session.Name = strings.TrimSpace(session.Name)
	if session.Name == "" {
		session.Name = session.ID
	}
	session.Tags = normalizeResearchTags(session.Tags)
	session.SourceFilter = normalizeResearchSourceFilter(session.SourceFilter)
	session.TimeRange = normalizeResearchTimeRange(session.TimeRange)
	if session.Status == "" {
		session.Status = researchSessionEmpty
	}
	if session.Summary.SchemaVersion == "" {
		session.Summary.SchemaVersion = researchSchemaVersion
	}
	if session.ArtifactRefs == nil {
		session.ArtifactRefs = map[string]ResearchArtifactRef{}
	}
	if session.CreatedAt.IsZero() {
		session.CreatedAt = time.Now().UTC()
	}
	if session.UpdatedAt.IsZero() {
		session.UpdatedAt = session.CreatedAt
	}
}

func normalizeResearchTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := map[string]struct{}{}
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
	}
	sort.Strings(out)
	return out
}

func normalizeResearchSourceFilter(filter ResearchSourceFilter) ResearchSourceFilter {
	filter.Sources = normalizeResearchTerms(filter.Sources)
	filter.EventTypes = normalizeResearchTerms(filter.EventTypes)
	filter.Comms = normalizeResearchTerms(filter.Comms)
	filter.TraceID = strings.TrimSpace(filter.TraceID)
	filter.SpanID = strings.TrimSpace(filter.SpanID)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Limit < 0 {
		filter.Limit = 0
	}
	if filter.Limit > researchMaxTaskLimit {
		filter.Limit = researchMaxTaskLimit
	}
	if len(filter.PIDs) > 0 {
		seen := map[uint32]struct{}{}
		out := make([]uint32, 0, len(filter.PIDs))
		for _, pid := range filter.PIDs {
			if pid == 0 {
				continue
			}
			if _, ok := seen[pid]; ok {
				continue
			}
			seen[pid] = struct{}{}
			out = append(out, pid)
		}
		sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
		filter.PIDs = out
	}
	return filter
}

func normalizeResearchTerms(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeResearchTimeRange(timerange ResearchTimeRange) ResearchTimeRange {
	if timerange.Since <= 0 && strings.TrimSpace(timerange.SinceTime) != "" {
		if parsed := events.ParseRecentEventTime(timerange.SinceTime); !parsed.IsZero() {
			timerange.Since = parsed.UnixMilli()
		}
	}
	if timerange.Until <= 0 && strings.TrimSpace(timerange.UntilTime) != "" {
		if parsed := events.ParseRecentEventTime(timerange.UntilTime); !parsed.IsZero() {
			timerange.Until = parsed.UnixMilli()
		}
	}
	if timerange.Since > 0 {
		timerange.SinceTime = time.UnixMilli(timerange.Since).UTC().Format(time.RFC3339Nano)
	}
	if timerange.Until > 0 {
		timerange.UntilTime = time.UnixMilli(timerange.Until).UTC().Format(time.RFC3339Nano)
	}
	return timerange
}

func mergeResearchSourceFilter(base, override ResearchSourceFilter) ResearchSourceFilter {
	if len(override.Sources) > 0 {
		base.Sources = override.Sources
	}
	if len(override.EventTypes) > 0 {
		base.EventTypes = override.EventTypes
	}
	if len(override.Comms) > 0 {
		base.Comms = override.Comms
	}
	if len(override.PIDs) > 0 {
		base.PIDs = override.PIDs
	}
	if strings.TrimSpace(override.TraceID) != "" {
		base.TraceID = override.TraceID
	}
	if strings.TrimSpace(override.SpanID) != "" {
		base.SpanID = override.SpanID
	}
	if strings.TrimSpace(override.Query) != "" {
		base.Query = override.Query
	}
	if override.Limit > 0 {
		base.Limit = override.Limit
	}
	if override.IncludeTLS != nil {
		base.IncludeTLS = override.IncludeTLS
	}
	if override.IncludeUploaded != nil {
		base.IncludeUploaded = override.IncludeUploaded
	}
	return normalizeResearchSourceFilter(base)
}

func mergeResearchTimeRange(base, override ResearchTimeRange) ResearchTimeRange {
	if override.Since > 0 || strings.TrimSpace(override.SinceTime) != "" {
		base.Since = override.Since
		base.SinceTime = override.SinceTime
	}
	if override.Until > 0 || strings.TrimSpace(override.UntilTime) != "" {
		base.Until = override.Until
		base.UntilTime = override.UntilTime
	}
	return normalizeResearchTimeRange(base)
}

func researchIncludeTLS(filter ResearchSourceFilter) bool {
	if filter.IncludeTLS != nil {
		return *filter.IncludeTLS
	}
	return true
}

func researchIncludeUploaded(filter ResearchSourceFilter) bool {
	if filter.IncludeUploaded != nil {
		return *filter.IncludeUploaded
	}
	return true
}

func researchEventMatches(event ResearchEvent, filter ResearchSourceFilter, timerange ResearchTimeRange) bool {
	filter = normalizeResearchSourceFilter(filter)
	timerange = normalizeResearchTimeRange(timerange)
	if len(filter.Sources) > 0 && !researchStringInList(event.Source, filter.Sources) {
		return false
	}
	if len(filter.EventTypes) > 0 && !researchStringInList(event.EventType, filter.EventTypes) {
		return false
	}
	if len(filter.Comms) > 0 && !researchStringInList(event.Comm, filter.Comms) {
		return false
	}
	if len(filter.PIDs) > 0 && !researchUint32InList(event.PID, filter.PIDs) {
		return false
	}
	if filter.TraceID != "" && event.TraceID != filter.TraceID {
		return false
	}
	if filter.SpanID != "" && event.SpanID != filter.SpanID {
		return false
	}
	if timerange.Since > 0 && event.Timestamp < timerange.Since {
		return false
	}
	if timerange.Until > 0 && event.Timestamp > timerange.Until {
		return false
	}
	if filter.Query != "" {
		haystack, _ := json.Marshal(event)
		if !strings.Contains(strings.ToLower(string(haystack)), strings.ToLower(filter.Query)) {
			return false
		}
	}
	return true
}

func filterResearchEventsByRange(events []ResearchEvent, timerange ResearchTimeRange) []ResearchEvent {
	out := make([]ResearchEvent, 0, len(events))
	for _, event := range events {
		if researchEventMatches(event, ResearchSourceFilter{}, timerange) {
			out = append(out, event)
		}
	}
	return out
}

func researchSourceFilterFromQuery(c *gin.Context) ResearchSourceFilter {
	filter := ResearchSourceFilter{
		Sources:    splitResearchQueryList(platform.FirstNonEmpty(c.Query("source"), c.Query("sources"))),
		EventTypes: splitResearchQueryList(platform.FirstNonEmpty(c.Query("eventType"), c.Query("event_type"), c.Query("type"))),
		Comms:      splitResearchQueryList(platform.FirstNonEmpty(c.Query("comm"), c.Query("comms"))),
		TraceID:    strings.TrimSpace(platform.FirstNonEmpty(c.Query("traceId"), c.Query("trace_id"))),
		SpanID:     strings.TrimSpace(platform.FirstNonEmpty(c.Query("spanId"), c.Query("span_id"))),
		Query:      strings.TrimSpace(platform.FirstNonEmpty(c.Query("q"), c.Query("query"), c.Query("search"))),
	}
	if raw := strings.TrimSpace(c.Query("pid")); raw != "" {
		filter.PIDs = parseResearchPIDs(raw)
	}
	return normalizeResearchSourceFilter(filter)
}

func researchTimeRangeFromQuery(c *gin.Context) ResearchTimeRange {
	return normalizeResearchTimeRange(ResearchTimeRange{SinceTime: c.Query("since"), UntilTime: c.Query("until")})
}

func splitResearchQueryList(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\t' })
}

func parseResearchPIDs(raw string) []uint32 {
	parts := splitResearchQueryList(raw)
	out := make([]uint32, 0, len(parts))
	for _, part := range parts {
		if parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32); err == nil && parsed > 0 {
			out = append(out, uint32(parsed))
		}
	}
	return out
}

func parseResearchIntQuery(c *gin.Context, key string, fallback, min, max int) int {
	value := fallback
	if raw := strings.TrimSpace(c.Query(key)); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			value = parsed
		}
	}
	if value < min {
		value = min
	}
	if max > 0 && value > max {
		value = max
	}
	return value
}

func researchStringInList(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func researchUint32InList(value uint32, candidates []uint32) bool {
	for _, candidate := range candidates {
		if candidate == value {
			return true
		}
	}
	return false
}

// RegisterRoutes wires the research workbench API onto the router; the
// transport entry point kept importable from the app layer.
func RegisterRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	registerResearchRoutes(router, tlsStore)
}
