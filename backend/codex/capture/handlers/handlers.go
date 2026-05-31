package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

const maxBodySize = 16 * 1024

const RedactedValue = "***REDACTED***"

var sensitiveQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"auth":          {},
	"authorization": {},
	"bearer":        {},
	"client_secret": {},
	"key":           {},
	"password":      {},
	"secret":        {},
	"session":       {},
	"token":         {},
}

var bearerTokenPattern = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]+`)
var inlineSecretPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret|password|secret|token)=([^\s&]+)`)

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

	MessageRole  string `json:"message_role,omitempty"`
	PromptDigest string `json:"prompt_digest,omitempty"`
	PromptLen    int    `json:"prompt_len,omitempty"`
	Vendor       string `json:"vendor,omitempty"`
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

	eventType := "http_request"
	if req.Status != 0 || strings.Contains(phase, "response") {
		eventType = "http_response"
	}
	if strings.Contains(phase, "websocket") {
		eventType = "websocket_request"
	}

	event := Event{
		Type:           eventType,
		Timestamp:      time.Now().UTC(),
		PID:            req.PID,
		TGID:           firstNonZero(req.TGID, req.PID),
		Comm:           firstNonEmpty(req.Comm, "codex"),
		Direction:      normalizeDirection(req.Direction, phase),
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
	annotateAgentMessage(&event)
	return event
}

func normalizeDirection(direction, phase string) string {
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

func sanitizeHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	redacted := map[string]string{
		"authorization":       RedactedValue,
		"proxy-authorization": RedactedValue,
		"x-api-key":           RedactedValue,
		"cookie":              RedactedValue,
		"set-cookie":          RedactedValue,
	}
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		lower := strings.ToLower(key)
		if replacement, ok := redacted[lower]; ok {
			out[lower] = replacement
			continue
		}
		out[lower] = strings.Join(values, ", ")
	}
	return out
}

func sanitizeURL(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return sanitizeInlineSecrets(rawURL)
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		if isSensitiveKey(key) {
			query.Set(key, RedactedValue)
			changed = true
		}
	}
	if !changed {
		return sanitizeInlineSecrets(rawURL)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sanitizeBody(body, contentType string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}
	if looksLikeJSON(contentType, []byte(body)) {
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err == nil {
			if sanitizeJSONValue(&payload) {
				if redacted, err := json.MarshalIndent(payload, "", "  "); err == nil {
					return string(redacted)
				}
			}
		}
	}
	if strings.Contains(strings.ToLower(contentType), "x-www-form-urlencoded") {
		if values, err := url.ParseQuery(body); err == nil {
			changed := false
			for key := range values {
				if isSensitiveKey(key) {
					values.Set(key, RedactedValue)
					changed = true
				}
			}
			if changed {
				return values.Encode()
			}
		}
	}
	return sanitizeInlineSecrets(body)
}

func sanitizeJSONValue(value *any) bool {
	switch typed := (*value).(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			if isSensitiveKey(key) {
				typed[key] = RedactedValue
				changed = true
				continue
			}
			childValue := child
			if sanitizeJSONValue(&childValue) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i, child := range typed {
			childValue := child
			if sanitizeJSONValue(&childValue) {
				typed[i] = childValue
				changed = true
			}
		}
		return changed
	case string:
		redacted := sanitizeInlineSecrets(typed)
		if redacted != typed {
			*value = redacted
			return true
		}
	}
	return false
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if _, ok := sensitiveQueryKeys[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "api_key")
}

func sanitizeInlineSecrets(value string) string {
	encodedRedactedValue := url.QueryEscape(RedactedValue)
	placeholder := "__CODEX_CAPTURE_REDACTED_VALUE__"
	protected := strings.ReplaceAll(value, encodedRedactedValue, placeholder)
	redacted := bearerTokenPattern.ReplaceAllString(protected, `${1}`+RedactedValue)
	redacted = inlineSecretPattern.ReplaceAllString(redacted, `${1}=`+RedactedValue)
	redacted = strings.ReplaceAll(redacted, placeholder, encodedRedactedValue)
	return redacted
}

func annotateSSEEvent(event *Event) {
	if event == nil || !strings.Contains(strings.ToLower(event.ContentType), "text/event-stream") {
		return
	}
	event.Type = "sse_message"
	var dataParts []string
	for _, line := range strings.Split(event.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "event:") && event.SSEEvent == "" {
			event.SSEEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		}
		if strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data != "" {
				dataParts = append(dataParts, data)
			}
		}
	}
	if len(dataParts) > 0 {
		event.SSEDataCount = len(dataParts)
		sum := sha256.Sum256([]byte(strings.Join(dataParts, "\n")))
		event.SSEDataDigest = "sha256:" + hex.EncodeToString(sum[:8])
	}
}

func formatBody(body []byte, contentType string) (string, bool) {
	if len(body) == 0 {
		return "", false
	}

	formatted := body
	if looksLikeJSON(contentType, body) {
		if pretty, err := prettyPrintJSON(body); err == nil {
			formatted = pretty
		}
	}

	truncated := len(body) > maxBodySize || len(formatted) > maxBodySize
	if len(formatted) > maxBodySize {
		formatted = formatted[:maxBodySize]
	}
	return string(formatted), truncated
}

func prettyPrintJSON(body []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func looksLikeJSON(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}

func annotateAgentMessage(event *Event) {
	if event == nil || event.Body == "" {
		return
	}
	digest, role, vendor, length := extractAgentMessageMeta(event.Body, event.ContentType, event.Host, event.URL, event.Direction)
	if digest == "" {
		return
	}
	event.PromptDigest = digest
	event.MessageRole = role
	if event.Vendor == "" {
		event.Vendor = vendor
	}
	event.PromptLen = length
}

func extractAgentMessageMeta(body, contentType, host, rawURL, direction string) (digest, role, vendor string, length int) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", "", "", 0
	}

	if !looksLikeAgentJSON(contentType, trimmed) {
		return digestPromptText(trimmed), "raw", inferVendor(host, rawURL), len(trimmed)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return digestPromptText(trimmed), "raw", inferVendor(host, rawURL), len(trimmed)
	}

	vendor = inferVendor(host, rawURL)
	if extracted, role := extractAgentPromptFromPayload(payload, direction); extracted != "" {
		return digestPromptText(extracted), role, vendor, len(extracted)
	}
	return digestPromptText(trimmed), "raw", vendor, len(trimmed)
}

func digestPromptText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "sha256:" + hex.EncodeToString(sum[:8])
}

func inferVendor(host, rawURL string) string {
	lower := strings.ToLower(host + " " + rawURL)
	switch {
	case strings.Contains(lower, "anthropic") || strings.Contains(lower, "claude"):
		return "anthropic"
	case strings.Contains(lower, "openai"):
		return "openai"
	case strings.Contains(lower, "googleapis") || strings.Contains(lower, "gemini"):
		return "google"
	case strings.Contains(lower, "ollama"):
		return "ollama"
	case strings.Contains(lower, "codex"):
		return "codex"
	default:
		return "unknown"
	}
}

func looksLikeAgentJSON(contentType, body string) bool {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "json") {
		return true
	}
	if body == "" {
		return false
	}
	first := body[0]
	return first == '{' || first == '['
}

func extractAgentPromptFromPayload(payload map[string]any, direction string) (string, string) {
	if payload == nil {
		return "", ""
	}

	if messages, ok := payload["messages"].([]any); ok && len(messages) > 0 {
		if text, role := extractAgentMessageFromList(messages, "user", "system", "tool"); text != "" {
			return text, role
		}
	}

	if contents, ok := payload["contents"].([]any); ok && len(contents) > 0 {
		if text, role := extractAgentMessageFromList(contents, "user", "system", "tool"); text != "" {
			return text, role
		}
	}

	if prompt, ok := payload["prompt"].(string); ok && strings.TrimSpace(prompt) != "" {
		return prompt, "prompt"
	}
	if input, ok := payload["input"].(string); ok && strings.TrimSpace(input) != "" {
		return input, "prompt"
	}

	if direction == "recv" {
		if text, role := extractAgentResponseFromPayload(payload); text != "" {
			return text, role
		}
	}
	return "", ""
}

func extractAgentMessageFromList(items []any, preferRoles ...string) (string, string) {
	preferred := map[string]struct{}{}
	for _, r := range preferRoles {
		preferred[r] = struct{}{}
	}

	var fallbackText, fallbackRole string
	for i := len(items) - 1; i >= 0; i-- {
		item, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		role, _ := item["role"].(string)
		text := stringifyAgentContent(item["content"])
		if text == "" {
			text = stringifyAgentContent(item["parts"])
		}
		if text == "" {
			continue
		}
		if _, want := preferred[role]; want {
			return text, role
		}
		if fallbackText == "" {
			fallbackText = text
			fallbackRole = role
		}
	}
	return fallbackText, fallbackRole
}

func stringifyAgentContent(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []any:
		var parts []string
		for _, item := range typed {
			switch entry := item.(type) {
			case string:
				if trimmed := strings.TrimSpace(entry); trimmed != "" {
					parts = append(parts, trimmed)
				}
			case map[string]any:
				if text, ok := entry["text"].(string); ok && strings.TrimSpace(text) != "" {
					parts = append(parts, strings.TrimSpace(text))
					continue
				}
				if input, ok := entry["input"].(string); ok && strings.TrimSpace(input) != "" {
					parts = append(parts, strings.TrimSpace(input))
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func extractAgentResponseFromPayload(payload map[string]any) (string, string) {
	if choices, ok := payload["choices"].([]any); ok && len(choices) > 0 {
		if choice, ok := choices[0].(map[string]any); ok {
			if message, ok := choice["message"].(map[string]any); ok {
				if text := stringifyAgentContent(message["content"]); text != "" {
					return text, "assistant"
				}
			}
			if delta, ok := choice["delta"].(map[string]any); ok {
				if text := stringifyAgentContent(delta["content"]); text != "" {
					return text, "assistant"
				}
			}
		}
	}
	if content, ok := payload["content"].([]any); ok && len(content) > 0 {
		if text := stringifyAgentContent(content); text != "" {
			return text, "assistant"
		}
	}
	if candidates, ok := payload["candidates"].([]any); ok && len(candidates) > 0 {
		if candidate, ok := candidates[0].(map[string]any); ok {
			if content, ok := candidate["content"].(map[string]any); ok {
				if text := stringifyAgentContent(content["parts"]); text != "" {
					return text, "assistant"
				}
			}
		}
	}
	if response, ok := payload["response"].(string); ok && strings.TrimSpace(response) != "" {
		return response, "assistant"
	}
	return "", ""
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
