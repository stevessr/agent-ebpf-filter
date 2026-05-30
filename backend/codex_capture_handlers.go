package main

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CodexCaptureRequest struct {
	Phase          string            `json:"phase"`
	Direction      string            `json:"direction"`
	Method         string            `json:"method"`
	URL            string            `json:"url"`
	Host           string            `json:"host"`
	Status         int               `json:"status"`
	Headers        map[string]string `json:"headers"`
	Body           string            `json:"body"`
	BodySize       int               `json:"body_size"`
	ContentType    string            `json:"content_type"`
	PID            uint32            `json:"pid"`
	TGID           uint32            `json:"tgid"`
	Comm           string            `json:"comm"`
	AgentRunID     string            `json:"agent_run_id"`
	TaskID         string            `json:"task_id"`
	ConversationID string            `json:"conversation_id"`
	TurnID         string            `json:"turn_id"`
	ToolCallID     string            `json:"tool_call_id"`
	ToolName       string            `json:"tool_name"`
	TraceID        string            `json:"trace_id"`
	SpanID         string            `json:"span_id"`
}

func registerCodexCaptureRoutes(router gin.IRouter, store *TLSCaptureStore, broadcaster *tlsCaptureBroadcaster) {
	router.POST("/codex/capture", handleCodexCapture(store, broadcaster))
}

func handleCodexCapture(store *TLSCaptureStore, broadcaster *tlsCaptureBroadcaster) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CodexCaptureRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}

		event := buildCodexCaptureEvent(req)
		dispatchTLSAgentEvent(&event, tlsAgentLoopDetector, broadcast)
		if store != nil {
			store.Add(event)
		}
		if broadcaster != nil {
			broadcaster.Broadcast(event)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "redaction_state": event.RedactionState, "body_size": event.BodySize})
	}
}

func buildCodexCaptureEvent(req CodexCaptureRequest) TLSPlaintextEvent {
	body := req.Body
	contentType := firstNonEmpty(req.ContentType, headerValue(req.Headers, "content-type"))
	formattedBody, truncated := formatTLSPlaintextBody([]byte(body), contentType)
	if body == "" {
		formattedBody = ""
		truncated = false
	}
	bodySize := req.BodySize
	if bodySize == 0 && body != "" {
		bodySize = len([]byte(body))
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	phase := strings.ToLower(strings.TrimSpace(req.Phase))
	if method == "" && strings.Contains(phase, "websocket") {
		method = "WEBSOCKET"
	}

	eventType := "http_request"
	if req.Status != 0 || strings.Contains(phase, "response") {
		eventType = "http_response"
	}
	if strings.Contains(phase, "websocket") {
		eventType = "websocket_request"
	}

	event := TLSPlaintextEvent{
		Type:           eventType,
		Timestamp:      time.Now().UTC(),
		PID:            req.PID,
		TGID:           firstNonZero(req.TGID, req.PID),
		Comm:           firstNonEmpty(req.Comm, "codex"),
		Direction:      normalizeCodexCaptureDirection(req.Direction, phase),
		Lib:            "codex-reqwest",
		Function:       firstNonEmpty(phase, "send"),
		CapturedLen:    len([]byte(body)),
		OriginalLen:    bodySize,
		Method:         method,
		URL:            sanitizeTLSURL(req.URL),
		Host:           firstNonEmpty(req.Host, hostFromURL(req.URL)),
		StatusCode:     req.Status,
		Headers:        sanitizeTLSHeaders(headerMapFromStrings(req.Headers)),
		Body:           sanitizeTLSBody(formattedBody, contentType),
		BodySize:       bodySize,
		ContentType:    contentType,
		RawAvailable:   body != "",
		Truncated:      truncated,
		RedactionState: "sanitized",
		RootAgentPID:   req.PID,
		AgentRunID:     strings.TrimSpace(req.AgentRunID),
		TaskID:         strings.TrimSpace(req.TaskID),
		ConversationID: strings.TrimSpace(req.ConversationID),
		TurnID:         strings.TrimSpace(req.TurnID),
		ToolCallID:     strings.TrimSpace(req.ToolCallID),
		ToolName:       strings.TrimSpace(req.ToolName),
		TraceID:        strings.TrimSpace(req.TraceID),
		SpanID:         strings.TrimSpace(req.SpanID),
		Vendor:         "codex",
	}
	annotateTLSSSEEvent(&event)
	return event
}

func normalizeCodexCaptureDirection(direction, phase string) string {
	direction = strings.ToLower(strings.TrimSpace(direction))
	switch direction {
	case "recv", "receive", "response", "in", "inbound":
		return "recv"
	case "send", "request", "out", "outbound":
		return "send"
	}
	if strings.Contains(strings.ToLower(phase), "response") {
		return "recv"
	}
	return "send"
}

func headerMapFromStrings(headers map[string]string) http.Header {
	if len(headers) == 0 {
		return nil
	}
	out := make(http.Header, len(headers))
	for key, value := range headers {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out.Set(key, value)
	}
	return out
}

func headerValue(headers map[string]string, name string) string {
	for key, value := range headers {
		if strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func hostFromURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return parsed.Host
}

func firstNonZero(values ...uint32) uint32 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
