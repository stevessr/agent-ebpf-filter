package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// ---- moved from backend/zz_merged_backend.go section ws_api.go ----

func serveEventsWS(c *gin.Context) {
	servePassiveProtoWS(c, AppCtx.Clients, &AppCtx.ClientsMu)
}

func serveEventEnvelopesWS(c *gin.Context) {
	servePassiveProtoWS(c, AppCtx.EnvelopeClients, &AppCtx.EnvelopeClientsMu)
}

func servePassiveProtoWS(c *gin.Context, target map[*websocket.Conn]bool, mu *sync.Mutex) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	mu.Lock()
	target[conn] = true
	mu.Unlock()

	go func(conn *websocket.Conn) {
		defer func() {
			mu.Lock()
			delete(target, conn)
			mu.Unlock()
			_ = conn.Close()
		}()

		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}(conn)
}

func broadcastProtoMessage(target map[*websocket.Conn]bool, mu *sync.Mutex, data []byte) {
	mu.Lock()
	for conn := range target {
		if conn == nil {
			delete(target, conn)
			continue
		}
		if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
			conn.Close()
			delete(target, conn)
		}
	}
	mu.Unlock()
}

func startEventBroadcaster() {
	go func() {
		eventBatch := make([]*pb.Event, 0, 50)
		envelopeBatch := make([]*pb.EventEnvelope, 0, 50)
		batchTicker := time.NewTicker(50 * time.Millisecond)
		defer batchTicker.Stop()

		flushBatch := func() {
			if len(eventBatch) > 0 {
				events := make([]*pb.Event, len(eventBatch))
				copy(events, eventBatch)
				eventBatch = eventBatch[:0]
				msg := &pb.EventBatch{Events: events}
				data, err := proto.Marshal(msg)
				if err != nil {
					log.Printf("[ERROR] failed to marshal EventBatch: %v", err)
				} else {
					broadcastProtoMessage(AppCtx.Clients, &AppCtx.ClientsMu, data)
				}
			}
			if len(envelopeBatch) > 0 {
				envelopes := make([]*pb.EventEnvelope, len(envelopeBatch))
				copy(envelopes, envelopeBatch)
				envelopeBatch = envelopeBatch[:0]
				msg := &pb.EventEnvelopeBatch{Envelopes: envelopes}
				data, err := proto.Marshal(msg)
				if err != nil {
					log.Printf("[ERROR] failed to marshal EventEnvelopeBatch: %v", err)
				} else {
					broadcastProtoMessage(AppCtx.EnvelopeClients, &AppCtx.EnvelopeClientsMu, data)
				}
			}
		}

		appendRecord := func(record CapturedEventRecord) {
			if record.Event != nil {
				eventBatch = append(eventBatch, record.Event)
			}
			if record.Envelope != nil {
				envelopeBatch = append(envelopeBatch, record.Envelope)
			}
		}

		for {
			select {
			case event := <-AppCtx.Broadcast:
				event = enrichEventContext(event)
				appendRecord(recordCapturedEvent(event))
				for _, alert := range buildSemanticAlerts(event) {
					alert = enrichEventContext(alert)
					appendRecord(recordCapturedEvent(alert))
				}
				if len(eventBatch) >= 50 || len(envelopeBatch) >= 50 {
					flushBatch()
				}
			case <-batchTicker.C:
				flushBatch()
			}
		}
	}()
}

type recentEventFilters struct {
	Type           string
	EventType      string
	Source         string
	PID            uint32
	Comm           string
	TraceID        string
	SpanID         string
	RedactionState string
	Since          time.Time
	Until          time.Time
}

func recentEventFiltersFromRequest(c *gin.Context) recentEventFilters {
	filters := recentEventFilters{
		Type:           strings.TrimSpace(c.Query("type")),
		EventType:      strings.TrimSpace(c.Query("event_type")),
		Source:         strings.TrimSpace(c.Query("source")),
		Comm:           strings.TrimSpace(c.Query("comm")),
		TraceID:        strings.TrimSpace(c.Query("trace_id")),
		SpanID:         strings.TrimSpace(c.Query("span_id")),
		RedactionState: strings.TrimSpace(c.Query("redaction_state")),
	}
	if filters.EventType == "" {
		filters.EventType = strings.TrimSpace(c.Query("eventType"))
	}
	if raw := strings.TrimSpace(c.Query("pid")); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
			filters.PID = uint32(parsed)
		}
	}
	filters.Since = parseRecentEventTime(c.Query("since"))
	filters.Until = parseRecentEventTime(c.Query("until"))
	return filters
}

func parseRecentEventTime(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return parsed.UTC()
	}
	if millis, err := strconv.ParseInt(raw, 10, 64); err == nil && millis > 0 {
		if millis > 1_000_000_000_000_000 {
			return time.Unix(0, millis).UTC()
		}
		return time.UnixMilli(millis).UTC()
	}
	return time.Time{}
}

func filterRecentEventRecords(records []CapturedEventRecord, filters recentEventFilters) []CapturedEventRecord {
	if filters == (recentEventFilters{}) {
		return records
	}
	filtered := make([]CapturedEventRecord, 0, len(records))
	for _, record := range records {
		record = normalizeCapturedEventRecord(record)
		if recentEventRecordMatches(record, filters) {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func recentEventRecordMatches(record CapturedEventRecord, filters recentEventFilters) bool {
	event := record.Event
	envelope := record.Envelope
	if event == nil && envelope == nil {
		return false
	}
	if filters.Type != "" && (event == nil || event.GetType() != filters.Type) {
		return false
	}
	if filters.EventType != "" && !strings.EqualFold(envelopeEventTypeName(envelope, event), filters.EventType) {
		return false
	}
	source := determineEventEnvelopeSource(event)
	if envelope != nil && strings.TrimSpace(envelope.GetSource()) != "" {
		source = envelope.GetSource()
	}
	if filters.Source != "" && !strings.EqualFold(source, filters.Source) {
		return false
	}
	pid := uint32(0)
	comm := ""
	traceID := ""
	spanID := ""
	if envelope != nil {
		pid = envelope.GetPid()
		comm = envelope.GetComm()
		traceID = envelope.GetTraceId()
		spanID = envelope.GetSpanId()
	}
	if event != nil {
		if pid == 0 {
			pid = event.GetPid()
		}
		comm = platform.FirstNonEmpty(comm, event.GetComm())
		traceID = platform.FirstNonEmpty(traceID, event.GetTraceId())
		spanID = platform.FirstNonEmpty(spanID, event.GetSpanId())
	}
	if filters.PID != 0 && pid != filters.PID {
		return false
	}
	if filters.Comm != "" && !strings.Contains(strings.ToLower(comm), strings.ToLower(filters.Comm)) {
		return false
	}
	if filters.TraceID != "" && traceID != filters.TraceID {
		return false
	}
	if filters.SpanID != "" && spanID != filters.SpanID {
		return false
	}
	if filters.RedactionState != "" && !strings.EqualFold(envelopeRedactionState(envelope), filters.RedactionState) {
		return false
	}
	if !filters.Since.IsZero() && record.ReceivedAt.Before(filters.Since) {
		return false
	}
	if !filters.Until.IsZero() && record.ReceivedAt.After(filters.Until) {
		return false
	}
	return true
}

func envelopeEventTypeName(envelope *pb.EventEnvelope, event *pb.Event) string {
	if envelope != nil && envelope.GetEventType().String() != "" {
		return envelope.GetEventType().String()
	}
	if event != nil {
		return event.GetEventType().String()
	}
	return ""
}

func envelopeRedactionState(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return ""
	}
	switch payload := envelope.GetPayload().(type) {
	case *pb.EventEnvelope_TlsEvent:
		return payload.TlsEvent.GetRedactionState()
	case *pb.EventEnvelope_HttpEvent:
		return payload.HttpEvent.GetRedactionState()
	case *pb.EventEnvelope_SseEvent:
		return payload.SseEvent.GetRedactionState()
	case *pb.EventEnvelope_StdioEvent:
		return payload.StdioEvent.GetRedactionState()
	case *pb.EventEnvelope_AgentsightAlertEvent:
		return payload.AgentsightAlertEvent.GetRedactionState()
	}
	return ""
}

func parseEventLimitQuery(raw string, defaultLimit int) int {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return defaultLimit
	}
	switch value {
	case "0", "all", "unlimited", "none":
		return -1
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return defaultLimit
	}
	if parsed == 0 {
		return -1
	}
	return parsed
}

func handleRecentEvents(c *gin.Context) {
	limit := parseEventLimitQuery(c.Query("limit"), 50)
	filters := recentEventFiltersFromRequest(c)
	records, source, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	records = filterRecentEventRecords(records, filters)

	resp := &pb.EventHistoryResponse{Source: source}
	for _, record := range records {
		record = normalizeCapturedEventRecord(record)
		resp.Events = append(resp.Events, &pb.CapturedEventRecord{
			Event:     record.Event,
			Timestamp: record.ReceivedAt.UnixMilli(),
			Envelope:  record.Envelope,
		})
	}
	writeProtoOrJSON(c, http.StatusOK, resp, gin.H{"source": source, "events": buildCapturedEventJSONRecords(records)})
}
