package handlers

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/pb"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
)

func parseAgentSightUploadPayload(body []byte) ([]AgentSightExportEvent, error) {
	raw := bytes.TrimSpace(body)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty AgentSight event payload")
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err == nil {
		return agentSightEventsFromDecodedPayload(decoded, AgentSightUploadMaxEvents)
	}
	events := make([]AgentSightExportEvent, 0)
	for index, line := range bytes.Split(raw, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var item any
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("line %d: %w", index+1, err)
		}
		event, err := agentSightEventFromDecodedPayload(item, index)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", index+1, err)
		}
		if len(events) >= AgentSightUploadMaxEvents {
			return nil, fmt.Errorf("%w: maximum %d events", errAgentSightUploadEventLimit, AgentSightUploadMaxEvents)
		}
		events = append(events, event)
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("no AgentSight events found")
	}
	return events, nil
}

func agentSightEventsFromDecodedPayload(decoded any, maxEvents int) ([]AgentSightExportEvent, error) {
	switch typed := decoded.(type) {
	case []any:
		if len(typed) > maxEvents {
			return nil, fmt.Errorf("%w: maximum %d events", errAgentSightUploadEventLimit, maxEvents)
		}
		events := make([]AgentSightExportEvent, 0, len(typed))
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
			return agentSightEventsFromDecodedPayload(nested, maxEvents)
		}
		if nested, ok := typed["records"]; ok {
			return agentSightEventsFromDecodedPayload(nested, maxEvents)
		}
		event, err := agentSightEventFromDecodedPayload(typed, 0)
		if err != nil {
			return nil, err
		}
		return []AgentSightExportEvent{event}, nil
	default:
		return nil, fmt.Errorf("unsupported AgentSight payload type %T", decoded)
	}
}

func agentSightEventFromDecodedPayload(decoded any, index int) (AgentSightExportEvent, error) {
	values, ok := decoded.(map[string]any)
	if !ok {
		return AgentSightExportEvent{}, fmt.Errorf("event must be an object")
	}
	data := mapFromAny(values["data"])
	if data == nil {
		data = map[string]any{}
		if values["data"] != nil {
			data["value"] = values["data"]
		}
	}
	timestamp := parseAgentSightTimestamp(firstNonNil(values["timestamp"], data["timestamp"]), time.Now().Add(time.Duration(index)*time.Millisecond).UnixMilli())
	source := platform.FirstNonEmpty(stringFromMap(values, "source"), stringFromMap(data, "source"), "imported")
	pid := uint32FromAny(firstNonNil(values["pid"], data["pid"]))
	ppid := uint32FromAny(firstNonNil(values["ppid"], data["ppid"], data["parent_pid"], data["parentPid"]))
	comm := platform.FirstNonEmpty(stringFromMap(values, "comm"), stringFromMap(data, "comm"), "imported")
	traceID := platform.FirstNonEmpty(stringFromMap(values, "trace_id"), stringFromMap(values, "traceId"), stringFromMap(data, "trace_id"), stringFromMap(data, "traceId"))
	spanID := platform.FirstNonEmpty(stringFromMap(values, "span_id"), stringFromMap(values, "spanId"), stringFromMap(data, "span_id"), stringFromMap(data, "spanId"))
	if data["event_type"] == nil && data["eventType"] != nil {
		data["event_type"] = data["eventType"]
	}
	if data["runner"] == nil {
		data["runner"] = "uploaded"
	}
	id := platform.FirstNonEmpty(stringFromMap(values, "id"), agentSightStableID("import", timestamp, source, pid, comm, data))
	return AgentSightExportEvent{
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

func agentSightEventFromCapturedRecord(record CapturedEventRecord) AgentSightExportEvent {
	record = Deps.NormalizeCapturedEventRecord(record)
	envelope := record.Envelope
	event := record.Event

	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() && envelope != nil && envelope.GetTimestampNs() > 0 {
		timestamp = time.Unix(0, int64(envelope.GetTimestampNs())).UTC()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	// Envelope conversion already serializes legacy_event. Reuse that decoded
	// map instead of independently serializing the same *pb.Event a second time.
	envelopeMap := Deps.EventEnvelopeToJSONValue(envelope)
	eventMap := mapFromAny(envelopeMap["legacy_event"])
	if eventMap == nil {
		eventMap = mapFromAny(envelopeMap["legacyEvent"])
	}
	if eventMap == nil && event != nil {
		eventMap = agentSightProtoMap(event)
	}
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

	eventType := Deps.EnvelopeEventTypeName(envelope, event)
	if eventType == "" {
		eventType = platform.FirstNonEmpty(stringFromMap(data, "event_type"), stringFromMap(data, "eventType"), stringFromMap(data, "type"))
	}
	data["event_type"] = eventType
	data["type"] = platform.FirstNonEmpty(stringFromMap(data, "type"), eventType)
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
	comm := platform.FirstNonEmpty(envelopeString(envelope, "comm"), event.GetComm(), stringFromMap(data, "comm"), "unknown")
	traceID := platform.FirstNonEmpty(envelopeString(envelope, "trace_id"), event.GetTraceId(), stringFromMap(data, "trace_id"), stringFromMap(data, "traceId"))
	spanID := platform.FirstNonEmpty(envelopeString(envelope, "span_id"), event.GetSpanId(), stringFromMap(data, "span_id"), stringFromMap(data, "spanId"))

	id := ""
	if envelope != nil {
		id = envelope.GetEventId()
	}
	if id == "" {
		id = agentSightStableID("event", timestamp.UnixMilli(), source, pid, comm, data)
	}

	return AgentSightExportEvent{
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

func agentSightTLSData(event tls.TLSPlaintextEvent, timestamp time.Time) map[string]any {
	data := map[string]any{
		"type":          platform.FirstNonEmpty(event.Type, "tls_plaintext"),
		"timestamp":     timestamp.UnixMilli(),
		"pid":           event.PID,
		"tgid":          event.TGID,
		"comm":          event.Comm,
		"direction":     event.Direction,
		"lib":           event.Lib,
		"captured_len":  event.CapturedLen,
		"original_len":  event.OriginalLen,
		"body_size":     event.BodySize,
		"raw_available": event.RawAvailable,
		"truncated":     event.Truncated,
		"is_handshake":  event.IsHandshake,
		"status_code":   event.StatusCode,
		"event_type":    agentSightTLSEventType(event),
	}
	if event.Function != "" {
		data["function"] = event.Function
	}
	if event.Method != "" {
		data["method"] = event.Method
	}
	if event.URL != "" {
		data["url"] = event.URL
		data["path"] = event.URL
	}
	if event.Host != "" {
		data["host"] = event.Host
	}
	if len(event.Headers) > 0 {
		data["headers"] = event.Headers
	}
	if event.Body != "" {
		data["body"] = event.Body
	}
	if event.ContentType != "" {
		data["content_type"] = event.ContentType
	}
	if event.RawHexDump != "" {
		data["raw_hex_dump"] = event.RawHexDump
	}
	if event.RedactionState != "" {
		data["redaction_state"] = event.RedactionState
	}
	if event.SSEEvent != "" {
		data["sse_event"] = event.SSEEvent
	}
	if event.SSEDataDigest != "" {
		data["sse_data_digest"] = event.SSEDataDigest
	}
	if event.SSEDataCount != 0 {
		data["sse_data_count"] = event.SSEDataCount
	}
	if event.RootAgentPID != 0 {
		data["root_agent_pid"] = event.RootAgentPID
	}
	if event.AgentRunID != "" {
		data["agent_run_id"] = event.AgentRunID
	}
	if event.TaskID != "" {
		data["task_id"] = event.TaskID
	}
	if event.ConversationID != "" {
		data["conversation_id"] = event.ConversationID
	}
	if event.TurnID != "" {
		data["turn_id"] = event.TurnID
	}
	if event.ToolCallID != "" {
		data["tool_call_id"] = event.ToolCallID
	}
	if event.ToolName != "" {
		data["tool_name"] = event.ToolName
	}
	if event.TraceID != "" {
		data["trace_id"] = event.TraceID
	}
	if event.SpanID != "" {
		data["span_id"] = event.SpanID
	}
	if event.MessageRole != "" {
		data["message_role"] = event.MessageRole
	}
	if event.PromptDigest != "" {
		data["prompt_digest"] = event.PromptDigest
	}
	if event.PromptLen != 0 {
		data["prompt_len"] = event.PromptLen
	}
	if event.Vendor != "" {
		data["vendor"] = event.Vendor
	}
	if event.LoopAlert {
		data["loop_alert"] = true
	}
	if event.UID != 0 {
		data["uid"] = event.UID
	}
	if event.TID != 0 {
		data["tid"] = event.TID
	}
	if event.LatencyMs != 0 {
		data["latency_ms"] = event.LatencyMs
	}
	if event.DataType != "" {
		data["data_type"] = event.DataType
	}
	if event.DeltaNs != 0 {
		data["delta_ns"] = event.DeltaNs
	}
	return data
}

func agentSightEventFromTLSPlaintext(event tls.TLSPlaintextEvent) AgentSightExportEvent {
	timestamp := event.Timestamp.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	data := agentSightTLSData(event, timestamp)
	tls.EnrichTLSEventWithAIMetadata(data, event)

	source := agentSightSourceFromTLS(event)
	id := agentSightStableID("tls", timestamp.UnixMilli(), source, event.PID, event.Comm, event.Type, event.Method, event.URL, event.StatusCode, event.PromptDigest)
	return AgentSightExportEvent{
		ID:        id,
		Timestamp: timestamp.UnixMilli(),
		Source:    source,
		PID:       event.PID,
		Comm:      platform.FirstNonEmpty(event.Comm, "tls"),
		TraceID:   event.TraceID,
		SpanID:    event.SpanID,
		Data:      data,
	}
}

func agentSightRunnerIDForEvent(event AgentSightExportEvent) string {
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
		eventType := strings.ToLower(platform.FirstNonEmpty(stringFromMap(event.Data, "event_type"), stringFromMap(event.Data, "type")))
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
	raw, err := json.Marshal(message)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(raw, &decoded); err != nil {
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
	return ""
}

func agentSightSourceFromTLS(event tls.TLSPlaintextEvent) string {
	switch event.Type {
	case "http_request", "http_response":
		return "http_parser"
	case "sse_message":
		return "sse_processor"
	default:
		return "ssl"
	}
}

func agentSightTLSEventType(event tls.TLSPlaintextEvent) string {
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
