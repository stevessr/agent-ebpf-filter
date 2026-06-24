package app

import (
	codexhandlers "agent-ebpf-filter/codex/capture/handlers"
)

// ---- moved from backend/zz_merged_backend.go section capturesinkcodex.go ----

type codexCaptureSink struct {
	store       *TLSCaptureStore
	broadcaster *tlsCaptureBroadcaster
}

func (s codexCaptureSink) HandleCaptureEvent(event codexhandlers.Event) {
	tlsEvent := TLSPlaintextEvent{
		Type:           event.Type,
		Timestamp:      event.Timestamp,
		PID:            event.PID,
		TGID:           event.TGID,
		Comm:           event.Comm,
		Direction:      event.Direction,
		Lib:            event.Lib,
		Function:       event.Function,
		CapturedLen:    event.CapturedLen,
		OriginalLen:    event.OriginalLen,
		Method:         event.Method,
		URL:            event.URL,
		Host:           event.Host,
		StatusCode:     event.StatusCode,
		Headers:        event.Headers,
		Body:           event.Body,
		BodySize:       event.BodySize,
		ContentType:    event.ContentType,
		RawAvailable:   event.RawAvailable,
		Truncated:      event.Truncated,
		RedactionState: event.RedactionState,
		SSEEvent:       event.SSEEvent,
		SSEDataDigest:  event.SSEDataDigest,
		SSEDataCount:   event.SSEDataCount,
		RootAgentPID:   event.RootAgentPID,
		AgentRunID:     event.AgentRunID,
		TaskID:         event.TaskID,
		ConversationID: event.ConversationID,
		TurnID:         event.TurnID,
		ToolCallID:     event.ToolCallID,
		ToolName:       event.ToolName,
		TraceID:        event.TraceID,
		SpanID:         event.SpanID,
		MessageRole:    event.MessageRole,
		PromptDigest:   event.PromptDigest,
		PromptLen:      event.PromptLen,
		Vendor:         event.Vendor,
	}

	dispatchTLSAgentEvent(&tlsEvent, tlsAgentLoopDetector, broadcast)
	if s.store != nil {
		s.store.Add(tlsEvent)
	}
	if s.broadcaster != nil {
		s.broadcaster.Broadcast(tlsEvent)
	}
}
