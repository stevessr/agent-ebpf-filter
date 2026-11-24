package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type CaptureRequest struct {
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

type Event struct {
	Type           string            `json:"type"`
	Timestamp      time.Time         `json:"timestamp"`
	PID            uint32            `json:"pid"`
	TGID           uint32            `json:"tgid"`
	Comm           string            `json:"comm"`
	Direction      string            `json:"direction"`
	Lib            string            `json:"lib"`
	Function       string            `json:"function,omitempty"`
	CapturedLen    int               `json:"captured_len"`
	OriginalLen    int               `json:"original_len"`
	Method         string            `json:"method,omitempty"`
	URL            string            `json:"url,omitempty"`
	Host           string            `json:"host,omitempty"`
	StatusCode     int               `json:"status,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	Body           string            `json:"body,omitempty"`
	BodySize       int               `json:"body_size"`
	ContentType    string            `json:"content_type,omitempty"`
	RawAvailable   bool              `json:"raw_available"`
	Truncated      bool              `json:"truncated"`
	RedactionState string            `json:"redaction_state,omitempty"`
	SSEEvent       string            `json:"sse_event,omitempty"`
	SSEDataDigest  string            `json:"sse_data_digest,omitempty"`
	SSEDataCount   int               `json:"sse_data_count,omitempty"`

	RootAgentPID   uint32 `json:"root_agent_pid,omitempty"`
	AgentRunID     string `json:"agent_run_id,omitempty"`
	TaskID         string `json:"task_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	ToolCallID     string `json:"tool_call_id,omitempty"`
	ToolName       string `json:"tool_name,omitempty"`
	TraceID        string `json:"trace_id,omitempty"`
	SpanID         string `json:"span_id,omitempty"`

	MessageRole        string `json:"message_role,omitempty"`
	PromptDigest       string `json:"prompt_digest,omitempty"`
	PromptLen          int    `json:"prompt_len,omitempty"`
	ContextDigest      string `json:"context_digest,omitempty"`
	ContextLen         int    `json:"context_len,omitempty"`
	ContextItems       int    `json:"context_items,omitempty"`
	Vendor             string `json:"vendor,omitempty"`
	ProtocolEvent      string `json:"protocol_event,omitempty"`
	StreamID           string `json:"stream_id,omitempty"`
	ResponseID         string `json:"response_id,omitempty"`
	PreviousResponseID string `json:"previous_response_id,omitempty"`
}

type CaptureSink interface {
	HandleCaptureEvent(Event)
}

type CaptureSinkFunc func(Event)

func (f CaptureSinkFunc) HandleCaptureEvent(event Event) {
	if f != nil {
		f(event)
	}
}

func RegisterRoutes(router gin.IRouter, sink CaptureSink) {
	router.POST("/codex/capture", HandleCapture(sink))
}

func HandleCapture(sink CaptureSink) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req CaptureRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid json"})
			return
		}

		event := BuildEvent(req)
		if sink != nil {
			sink.HandleCaptureEvent(event)
		}
		c.JSON(http.StatusOK, gin.H{"status": "ok", "redaction_state": event.RedactionState, "body_size": event.BodySize})
	}
}

func BuildEvent(req CaptureRequest) Event {
	body := req.Body
	contentType := firstNonEmpty(req.ContentType, headerValue(req.Headers, "content-type"))
	formattedBody, truncated := formatBody([]byte(body), contentType)
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

	direction := normalizeDirection(req.Direction, phase)
	eventType := "http_request"
	if req.Status != 0 || strings.Contains(phase, "response") || direction == "recv" {
		eventType = "http_response"
	}
	if strings.Contains(phase, "websocket") {
		if direction == "recv" {
			eventType = "websocket_response"
		} else {
			eventType = "websocket_request"
		}
	}

	event := Event{
		Type:           eventType,
		Timestamp:      time.Now().UTC(),
		PID:            req.PID,
		TGID:           firstNonZero(req.TGID, req.PID),
		Comm:           firstNonEmpty(req.Comm, "codex"),
		Direction:      direction,
		Lib:            "codex-reqwest",
		Function:       firstNonEmpty(phase, "send"),
		CapturedLen:    len([]byte(body)),
		OriginalLen:    bodySize,
		Method:         method,
		URL:            sanitizeURL(req.URL),
		Host:           firstNonEmpty(req.Host, hostFromURL(req.URL)),
		StatusCode:     req.Status,
		Headers:        sanitizeHeaders(headerMapFromStrings(req.Headers)),
		Body:           sanitizeBody(formattedBody, contentType),
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
	annotateSSEEvent(&event)
	annotateResponsesMetadata(&event, body)
	annotateResponsesContextMetadata(&event, body, contentType)
	annotateAgentMessageFromBody(&event, body, contentType)
	return event
}

func normalizeDirection(direction, phase string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "recv", "receive", "response", "in", "inbound":
		return "recv"
	case "send", "request", "out", "outbound":
		return "send"
	}
	if strings.Contains(phase, "response") {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func firstNonZero(values ...uint32) uint32 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
