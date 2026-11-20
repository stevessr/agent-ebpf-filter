package handlers

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/tls"
	"time"
)

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
	if event.ProtocolEvent != "" {
		data["protocol_event"] = event.ProtocolEvent
	}
	if event.StreamID != "" {
		data["stream_id"] = event.StreamID
	}
	if event.ResponseID != "" {
		data["response_id"] = event.ResponseID
	}
	if event.PreviousResponseID != "" {
		data["previous_response_id"] = event.PreviousResponseID
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
	if event.ContextDigest != "" {
		data["context_digest"] = event.ContextDigest
	}
	if event.ContextLen != 0 {
		data["context_len"] = event.ContextLen
	}
	if event.ContextItems != 0 {
		data["context_items"] = event.ContextItems
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
