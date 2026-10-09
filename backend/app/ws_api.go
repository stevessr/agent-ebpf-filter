package app

import (
	"agent-ebpf-filter/app/events"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/internal/wsfanout"
	"agent-ebpf-filter/pb"
)

const (
	passiveProtoWSReadLimit = 1024
	passiveProtoWSPongWait  = 75 * time.Second

	// broadcastBatchSize and broadcastFlushInterval bound the latency added by
	// batching events into one websocket frame.
	broadcastBatchSize     = 50
	broadcastFlushInterval = 50 * time.Millisecond
)

func serveEventsWS(c *gin.Context) {
	ac := Ctx(c)
	servePassiveProtoWS(c, ac.EventClientHub)
}

func serveEventEnvelopesWS(c *gin.Context) {
	ac := Ctx(c)
	servePassiveProtoWS(c, ac.EnvelopeClientHub)
}

func serveEventSummariesWS(c *gin.Context) {
	ac := Ctx(c)
	servePassiveProtoWS(c, ac.SummaryClientHub)
}

func servePassiveProtoWS(c *gin.Context, hub *wsfanout.Hub) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	conn.SetReadLimit(passiveProtoWSReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(passiveProtoWSPongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(passiveProtoWSPongWait))
	})
	client := hub.Add(conn)
	defer hub.Remove(client)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func runEventBroadcaster(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	appContext := AppCtx
	if appContext == nil {
		return
	}
	defer appContext.EventClientHub.Close()
	defer appContext.EnvelopeClientHub.Close()
	defer appContext.SummaryClientHub.Close()
	eventBatch := make([]*pb.Event, 0, broadcastBatchSize)
	envelopeBatch := make([]*pb.EventEnvelope, 0, broadcastBatchSize)
	summaryBatch := make([]renewEventSummary, 0, broadcastBatchSize)
	batchTicker := time.NewTicker(broadcastFlushInterval)
	defer batchTicker.Stop()

	// publish marshals one batch and hands the single encoded payload to the
	// hub, which shares it across every subscriber. When nobody is subscribed
	// the batch is dropped without paying for the marshal at all.
	publish := func(hub *wsfanout.Hub, msg proto.Message, marshalErrors, writeErrors *int) {
		if hub.Len() == 0 {
			return
		}
		data, err := proto.Marshal(msg)
		if err != nil {
			*marshalErrors++
			log.Printf("[ERROR] failed to marshal %T: %v", msg, err)
			return
		}
		*writeErrors += hub.Broadcast(wsfanout.NewBinaryMessage(data))
	}

	flushBatch := func() {
		eventCount := len(eventBatch)
		envelopeCount := len(envelopeBatch)
		if eventCount == 0 && envelopeCount == 0 {
			return
		}
		started := time.Now()
		marshalErrors := 0
		writeErrors := 0
		if eventCount > 0 {
			publish(appContext.EventClientHub, &pb.EventBatch{Events: eventBatch}, &marshalErrors, &writeErrors)
			clear(eventBatch)
			eventBatch = eventBatch[:0]
		}
		if envelopeCount > 0 {
			publish(appContext.EnvelopeClientHub, &pb.EventEnvelopeBatch{Envelopes: envelopeBatch}, &marshalErrors, &writeErrors)
			clear(envelopeBatch)
			envelopeBatch = envelopeBatch[:0]
		}
		if len(summaryBatch) > 0 {
			if appContext.SummaryClientHub.Len() > 0 {
				data, err := json.Marshal(gin.H{"events": summaryBatch})
				if err != nil {
					marshalErrors++
					log.Printf("[ERROR] failed to marshal Renew summaries: %v", err)
				} else {
					writeErrors += appContext.SummaryClientHub.Broadcast(wsfanout.NewTextMessage(data))
				}
			}
			clear(summaryBatch)
			summaryBatch = summaryBatch[:0]
		}
		collectorMetricsStore.RecordBroadcastFlush(eventCount, envelopeCount, marshalErrors, writeErrors, time.Since(started))
	}

	appendRecord := func(record CapturedEventRecord) {
		if record.Event != nil {
			eventBatch = append(eventBatch, record.Event)
		}
		if record.Envelope != nil {
			envelopeBatch = append(envelopeBatch, record.Envelope)
		}
		if summary, ok := buildRenewEventSummary(record); ok {
			summaryBatch = append(summaryBatch, summary)
		}
	}

	for {
		select {
		case <-ctx.Done():
			flushBatch()
			return
		case event, ok := <-appContext.Broadcast:
			if !ok {
				flushBatch()
				return
			}
			collectorMetricsStore.RecordBroadcastReceived()
			event = enrichEventContext(event)
			// Semantic alerts must see the event before recordCapturedEvent
			// redacts it in place; the broadcaster owns the event outright
			// once it leaves the queue (see enqueueBroadcastEvent).
			// Capture admission runs after attribution, before analysis or retention.
			if !captureAgentEvent(event) {
				continue
			}
			monitorAllowed := monitorAgentEvent(event)
			var alerts []*pb.Event
			if monitorAllowed {
				alerts = buildSemanticAlerts(event)
			}
			if !shouldIgnoreEventPath(event) {
				appendRecord(recordCapturedEventWithMonitoring(event, monitorAllowed))
			}
			for _, alert := range alerts {
				alert = enrichEventContext(alert)
				// Derived alerts inherit the original Agent's captured and
				// monitored scope even when using a synthetic Security tag.
				appendRecord(recordCapturedEventWithMonitoring(alert, monitorAllowed))
			}
			if len(eventBatch) >= broadcastBatchSize || len(envelopeBatch) >= broadcastBatchSize || len(summaryBatch) >= broadcastBatchSize {
				flushBatch()
			}
		case <-batchTicker.C:
			flushBatch()
		}
	}
}

type recentEventFilters = events.RecentEventFilters

var recentEventFiltersFromRequest = events.RecentEventFiltersFromRequest

var parseRecentEventTime = events.ParseRecentEventTime

func filterRecentEventRecords(records []CapturedEventRecord, filters recentEventFilters) []CapturedEventRecord {
	if filters.IsZero() {
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

var envelopeEventTypeName = events.EnvelopeEventTypeName

var envelopeRedactionState = events.EnvelopeRedactionState

const maxRecentEventLimit = 1000

func parseEventLimitQuery(raw string, defaultLimit int) int {
	if defaultLimit <= 0 || defaultLimit > maxRecentEventLimit {
		defaultLimit = 50
	}
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return defaultLimit
	}
	switch value {
	case "0", "all", "unlimited", "none":
		return maxRecentEventLimit
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 0 {
		return defaultLimit
	}
	if parsed == 0 || parsed > maxRecentEventLimit {
		return maxRecentEventLimit
	}
	return parsed
}

type renewEventSummary struct {
	RootAgentPID   uint32  `json:"rootAgentPid,omitempty"`
	Key            string  `json:"key"`
	EventID        string  `json:"eventId"`
	PID            uint32  `json:"pid"`
	PPID           uint32  `json:"ppid"`
	UID            uint32  `json:"uid"`
	Type           string  `json:"type"`
	EventType      int32   `json:"eventType"`
	Tag            string  `json:"tag"`
	Comm           string  `json:"comm"`
	Target         string  `json:"target,omitempty"`
	Network        bool    `json:"network,omitempty"`
	HasAgentContext bool   `json:"hasAgentContext,omitempty"`
	Path           string  `json:"path"`
	ExtraPath      string  `json:"extraPath,omitempty"`
	NetDirection   string  `json:"netDirection,omitempty"`
	NetEndpoint    string  `json:"netEndpoint,omitempty"`
	NetBytes       uint64  `json:"netBytes,omitempty"`
	Domain         string  `json:"domain,omitempty"`
	Decision       string  `json:"decision,omitempty"`
	RiskScore      float64 `json:"riskScore,omitempty"`
	AgentRunID     string  `json:"agentRunId,omitempty"`
	ConversationID string  `json:"conversationId,omitempty"`
	TurnID         string  `json:"turnId,omitempty"`
	ToolCallID     string  `json:"toolCallId,omitempty"`
	ToolName       string  `json:"toolName,omitempty"`
	TraceID        string  `json:"traceId,omitempty"`
	SpanID         string  `json:"spanId,omitempty"`
	Time           string  `json:"time"`
	ReceivedAtMS   int64   `json:"receivedAtMs"`
}

type renewDesktopEventSummary struct {
	EventID          string  `json:"eventId"`
	ReceivedAtMS     int64   `json:"receivedAtMs"`
	PID              uint32  `json:"pid"`
	PPID             uint32  `json:"ppid"`
	RootAgentPID     uint32  `json:"rootAgentPid,omitempty"`
	EventType        int32   `json:"eventType"`
	Type             string  `json:"type"`
	Tag              string  `json:"tag,omitempty"`
	Comm             string  `json:"comm,omitempty"`
	Target           string  `json:"target,omitempty"`
	Network          bool    `json:"network,omitempty"`
	NetBytes         uint64  `json:"netBytes,omitempty"`
	Decision         string  `json:"decision,omitempty"`
	RiskScore        float64 `json:"riskScore,omitempty"`
	AgentRunID       string  `json:"agentRunId,omitempty"`
	ConversationID   string  `json:"conversationId,omitempty"`
	ToolName         string  `json:"toolName,omitempty"`
	HasAgentContext  bool    `json:"hasAgentContext,omitempty"`
}

func buildRenewDesktopEventSummary(record CapturedEventRecord) (renewDesktopEventSummary, bool) {
	return buildRenewDesktopEventSummaryNormalized(normalizeCapturedEventRecord(record))
}

func buildRenewDesktopEventSummaryNormalized(record CapturedEventRecord) (renewDesktopEventSummary, bool) {
	if record.Event == nil || record.Envelope == nil {
		return renewDesktopEventSummary{}, false
	}
	event := record.Event
	envelope := record.Envelope
	eventID := strings.TrimSpace(envelope.GetEventId())
	if eventID == "" {
		return renewDesktopEventSummary{}, false
	}

	agentRunID := platform.FirstNonEmpty(event.GetAgentRunId(), envelope.GetAgentRunId())
	conversationID := platform.FirstNonEmpty(event.GetConversationId(), envelope.GetConversationId())
	toolCallID := platform.FirstNonEmpty(event.GetToolCallId(), envelope.GetToolCallId())
	toolName := platform.FirstNonEmpty(event.GetToolName(), envelope.GetToolName())
	tag := event.GetTag()
	rootAgentPID := event.GetRootAgentPid()
	target := platform.FirstNonEmpty(
		event.GetPath(),
		event.GetNetEndpoint(),
		event.GetDomain(),
		event.GetExtraPath(),
		toolName,
	)
	eventType := event.GetType()
	network := strings.TrimSpace(event.GetNetEndpoint()) != "" || strings.TrimSpace(event.GetDomain()) != ""
	if !network {
		lowerType := strings.ToLower(eventType)
		network = strings.Contains(lowerType, "network") ||
			strings.Contains(lowerType, "connect") ||
			strings.Contains(lowerType, "socket") ||
			strings.Contains(lowerType, "tcp") ||
			strings.Contains(lowerType, "dns")
	}
	hasAgentContext := agentRunID != "" ||
		conversationID != "" ||
		toolCallID != "" ||
		rootAgentPID > 0 ||
		(strings.TrimSpace(tag) != "" && !strings.EqualFold(tag, "Unknown"))

	return renewDesktopEventSummary{
		EventID:         eventID,
		ReceivedAtMS:    record.ReceivedAt.UnixMilli(),
		PID:             event.GetPid(),
		PPID:            event.GetPpid(),
		RootAgentPID:    rootAgentPID,
		EventType:       int32(event.GetEventType()),
		Type:            eventType,
		Tag:             tag,
		Comm:            event.GetComm(),
		Target:          target,
		Network:         network,
		NetBytes:        uint64(event.GetNetBytes()),
		Decision:        platform.FirstNonEmpty(event.GetDecision(), envelope.GetPolicyDecision()),
		RiskScore:       max(event.GetRiskScore(), envelope.GetRiskScore()),
		AgentRunID:      agentRunID,
		ConversationID:  conversationID,
		ToolName:        toolName,
		HasAgentContext: hasAgentContext,
	}, true
}

func desktopEventSummaryProto(summary renewDesktopEventSummary) *pb.DesktopEventSummary {
	return &pb.DesktopEventSummary{
		EventId:          summary.EventID,
		ReceivedAtMs:     summary.ReceivedAtMS,
		Pid:              summary.PID,
		Ppid:             summary.PPID,
		RootAgentPid:     summary.RootAgentPID,
		EventType:        summary.EventType,
		Type:             summary.Type,
		Tag:              summary.Tag,
		Comm:             summary.Comm,
		Target:           summary.Target,
		Network:          summary.Network,
		NetBytes:         summary.NetBytes,
		Decision:         summary.Decision,
		RiskScore:        summary.RiskScore,
		AgentRunId:       summary.AgentRunID,
		ConversationId:   summary.ConversationID,
		ToolName:         summary.ToolName,
		HasAgentContext:  summary.HasAgentContext,
	}
}

func buildRenewEventSummary(record CapturedEventRecord) (renewEventSummary, bool) {
	record = normalizeCapturedEventRecord(record)
	if record.Event == nil || record.Envelope == nil {
		return renewEventSummary{}, false
	}
	event := record.Event
	envelope := record.Envelope
	eventID := strings.TrimSpace(envelope.GetEventId())
	if eventID == "" {
		return renewEventSummary{}, false
	}
	compact, _ := buildRenewDesktopEventSummaryNormalized(record)
	return renewEventSummary{
		Key:            eventID,
		EventID:        eventID,
		PID:            event.GetPid(),
		PPID:           event.GetPpid(),
		UID:            event.GetUid(),
		Type:           event.GetType(),
		EventType:      int32(event.GetEventType()),
		Tag:            event.GetTag(),
		Comm:           event.GetComm(),
		Target:         compact.Target,
		Network:        compact.Network,
		HasAgentContext: compact.HasAgentContext,
		Path:           event.GetPath(),
		ExtraPath:      event.GetExtraPath(),
		NetDirection:   event.GetNetDirection(),
		NetEndpoint:    event.GetNetEndpoint(),
		NetBytes:       uint64(event.GetNetBytes()),
		Domain:         event.GetDomain(),
		Decision:       platform.FirstNonEmpty(event.GetDecision(), envelope.GetPolicyDecision()),
		RiskScore:      max(event.GetRiskScore(), envelope.GetRiskScore()),
		RootAgentPID:   event.GetRootAgentPid(),
		AgentRunID:     platform.FirstNonEmpty(event.GetAgentRunId(), envelope.GetAgentRunId()),
		ConversationID: platform.FirstNonEmpty(event.GetConversationId(), envelope.GetConversationId()),
		TurnID:         platform.FirstNonEmpty(event.GetTurnId(), envelope.GetTurnId()),
		ToolCallID:     platform.FirstNonEmpty(event.GetToolCallId(), envelope.GetToolCallId()),
		ToolName:       platform.FirstNonEmpty(event.GetToolName(), envelope.GetToolName()),
		TraceID:        platform.FirstNonEmpty(event.GetTraceId(), envelope.GetTraceId()),
		SpanID:         platform.FirstNonEmpty(event.GetSpanId(), envelope.GetSpanId()),
		Time:           record.ReceivedAt.UTC().Format(time.RFC3339Nano),
		ReceivedAtMS:   record.ReceivedAt.UnixMilli(),
	}, true
}

func handleRecentEventSummaries(c *gin.Context) {
	limit := parseEventLimitQuery(c.Query("limit"), 100)
	cursor := strings.TrimSpace(c.Query("cursor"))
	filters := recentEventFiltersFromRequest(c)
	records, source, nextCursor, err := runtimeSettingsStore.EventPageContext(c.Request.Context(), limit, cursor)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		if errors.Is(err, errInvalidEventStoreCursor) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event history cursor"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	records = filterRecentEventRecords(records, filters)
	if strings.EqualFold(strings.TrimSpace(c.Query("compact")), "1") ||
		strings.EqualFold(strings.TrimSpace(c.Query("compact")), "true") {
		summaries := make([]renewDesktopEventSummary, 0, len(records))
		for _, record := range records {
			if summary, ok := buildRenewDesktopEventSummary(record); ok {
				summaries = append(summaries, summary)
			}
		}
		c.JSON(http.StatusOK, gin.H{
			"source":     source,
			"events":     summaries,
			"nextCursor": nextCursor,
			"hasMore":    nextCursor != "",
		})
		return
	}

	summaries := make([]renewEventSummary, 0, len(records))
	for _, record := range records {
		if summary, ok := buildRenewEventSummary(record); ok {
			summaries = append(summaries, summary)
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"source":     source,
		"events":     summaries,
		"nextCursor": nextCursor,
		"hasMore":    nextCursor != "",
	})
}

func handleEventByID(c *gin.Context) {
	c.Header("Cache-Control", "no-store, private")
	c.Header("Pragma", "no-cache")
	eventID := strings.TrimSpace(c.Param("id"))
	if eventID == "" || len(eventID) > 256 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event id"})
		return
	}
	record, err := runtimeSettingsStore.EventByIDContext(c.Request.Context(), eventID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
			return
		}
		if c.Request.Context().Err() != nil {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	record = normalizeCapturedEventRecord(record)
	values := buildCapturedEventJSONRecords([]CapturedEventRecord{record})
	if len(values) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "event not found"})
		return
	}
	c.JSON(http.StatusOK, values[0])
}

func handleRecentEvents(c *gin.Context) {
	limit := parseEventLimitQuery(c.Query("limit"), 50)
	filters := recentEventFiltersFromRequest(c)
	records, source, err := runtimeSettingsStore.RecentEventsContext(c.Request.Context(), limit)
	if err != nil {
		if c.Request.Context().Err() != nil {
			return
		}
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
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
