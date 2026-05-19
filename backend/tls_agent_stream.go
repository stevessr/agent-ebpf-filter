package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
)

const (
	tlsAgentLoopDefaultWindow = 30 * time.Second
	tlsAgentLoopRepeatLimit   = 5
	tlsAgentLoopAlertMinRisk  = 0.97
	tlsPromptDigestBytes      = 8
)

// tlsAgentBridge is the channel used to feed TLS-derived pb.Events into the
// main event pipeline. main wires this to the global broadcast channel; tests
// can swap it to capture emitted events without depending on the broadcaster.
var tlsAgentBridge chan<- *pb.Event = broadcast

// enrichTLSEventWithAgentContext copies the tracked process context for the
// PID/TGID owning the TLS event into the event itself, allowing downstream
// consumers (frontend, semantic alerting, execution graph) to correlate the
// plaintext traffic back to a specific Agent run, task, or tool call.
func enrichTLSEventWithAgentContext(event *TLSPlaintextEvent) {
	if event == nil {
		return
	}
	ctx, ok := lookupTLSProcessContext(event.PID, event.TGID)
	if !ok {
		return
	}
	if event.RootAgentPID == 0 {
		event.RootAgentPID = ctx.RootAgentPid
	}
	if event.AgentRunID == "" {
		event.AgentRunID = ctx.AgentRunID
	}
	if event.TaskID == "" {
		event.TaskID = ctx.TaskID
	}
	if event.ConversationID == "" {
		event.ConversationID = ctx.ConversationID
	}
	if event.TurnID == "" {
		event.TurnID = ctx.TurnID
	}
	if event.ToolCallID == "" {
		event.ToolCallID = ctx.ToolCallID
	}
	if event.ToolName == "" {
		event.ToolName = ctx.ToolName
	}
	if event.TraceID == "" {
		event.TraceID = ctx.TraceID
	}
	if event.SpanID == "" {
		event.SpanID = ctx.SpanID
	}
}

func lookupTLSProcessContext(pid, tgid uint32) (processContext, bool) {
	if ctx, ok := trackedProcessContexts.Get(pid); ok {
		return ctx, true
	}
	if tgid != 0 && tgid != pid {
		if ctx, ok := trackedProcessContexts.Get(tgid); ok {
			return ctx, true
		}
	}
	return processContext{}, false
}

// annotateTLSAgentMessage populates the prompt digest / role / vendor fields
// for chat-completion-shaped HTTP requests and responses. Only metadata is
// retained; the plaintext body stays inside the in-memory TLS capture store
// (which is bounded and explicitly opt-in via tlsCaptureEnabled).
func annotateTLSAgentMessage(event *TLSPlaintextEvent) {
	if event == nil || event.Body == "" {
		return
	}
	digest, role, vendor, length := extractAgentMessageMeta(event.Body, event.ContentType, event.Host, event.URL, event.Direction)
	if digest == "" {
		return
	}
	event.PromptDigest = digest
	event.MessageRole = role
	event.Vendor = vendor
	event.PromptLen = length
}

// extractAgentMessageMeta inspects an HTTP body and returns a digest of the
// prompt/response text plus a coarse role label. It recognises OpenAI,
// Anthropic, Google Gemini, Ollama, and generic JSON shapes; if the body
// does not look like a structured agent message but still has content, it
// falls back to digesting the raw body to support local proxy traffic.
func extractAgentMessageMeta(body, contentType, host, url, direction string) (digest, role, vendor string, length int) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", "", "", 0
	}

	if !looksLikeTLSAgentJSON(contentType, trimmed) {
		return digestPromptText(trimmed), "raw", inferTLSVendor(host, url), len(trimmed)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return digestPromptText(trimmed), "raw", inferTLSVendor(host, url), len(trimmed)
	}

	vendor = inferTLSVendor(host, url)
	if extracted, role := extractAgentPromptFromPayload(payload, direction); extracted != "" {
		return digestPromptText(extracted), role, vendor, len(extracted)
	}
	return digestPromptText(trimmed), "raw", vendor, len(trimmed)
}

func looksLikeTLSAgentJSON(contentType, body string) bool {
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

func digestPromptText(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return "sha256:" + hex.EncodeToString(sum[:tlsPromptDigestBytes])
}

func inferTLSVendor(host, url string) string {
	lower := strings.ToLower(host + " " + url)
	switch {
	case strings.Contains(lower, "openai"):
		return "openai"
	case strings.Contains(lower, "anthropic"):
		return "anthropic"
	case strings.Contains(lower, "generativelanguage") || strings.Contains(lower, "gemini"):
		return "google"
	case strings.Contains(lower, "ollama"):
		return "ollama"
	case strings.Contains(lower, "cohere"):
		return "cohere"
	case strings.Contains(lower, "mistral"):
		return "mistral"
	case strings.Contains(lower, "bedrock"):
		return "aws-bedrock"
	case strings.Contains(lower, "azure"):
		return "azure"
	default:
		return ""
	}
}

// tlsAgentLoopState tracks repeated prompt digests within a short window per
// agent identity. Once the same digest repeats beyond the configured limit
// it produces a synthetic RESOURCE_WASTING_LOOP semantic_alert event, so that
// loops manifesting purely as encrypted API traffic (no fs/forks) still
// surface.
type tlsAgentLoopState struct {
	mu       sync.Mutex
	tracker  map[string]*tlsAgentLoopWindow
	now      func() time.Time
	window   time.Duration
	limit    int
	maxIdent int
}

type tlsAgentLoopWindow struct {
	WindowStart  time.Time
	LastSeen     time.Time
	PromptDigest string
	Repeats      int
	Alerted      bool
}

func newTLSAgentLoopState() *tlsAgentLoopState {
	return &tlsAgentLoopState{
		tracker:  make(map[string]*tlsAgentLoopWindow),
		now:      time.Now,
		window:   tlsAgentLoopDefaultWindow,
		limit:    tlsAgentLoopRepeatLimit,
		maxIdent: 1024,
	}
}

// Observe records the digest emitted by a TLS event and returns an alert if
// the repeat threshold has been crossed for this identity inside the window.
func (s *tlsAgentLoopState) Observe(event *TLSPlaintextEvent) *pb.Event {
	if s == nil || event == nil || event.PromptDigest == "" {
		return nil
	}
	identity := tlsAgentLoopIdentity(event)
	if identity == "" {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now().UTC()
	if len(s.tracker) >= s.maxIdent {
		s.evictExpiredLocked(now)
	}

	state, ok := s.tracker[identity]
	if !ok || state == nil || state.PromptDigest != event.PromptDigest || now.Sub(state.WindowStart) > s.window {
		s.tracker[identity] = &tlsAgentLoopWindow{
			WindowStart:  now,
			LastSeen:     now,
			PromptDigest: event.PromptDigest,
			Repeats:      1,
		}
		return nil
	}

	state.LastSeen = now
	state.Repeats++
	if state.Alerted || state.Repeats < s.limit {
		return nil
	}
	state.Alerted = true
	event.LoopAlert = true
	return buildTLSLoopAlertEvent(event, identity, state.Repeats, s.window)
}

func (s *tlsAgentLoopState) evictExpiredLocked(now time.Time) {
	for key, state := range s.tracker {
		if state == nil || now.Sub(state.LastSeen) > s.window*2 {
			delete(s.tracker, key)
		}
	}
}

func (s *tlsAgentLoopState) reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.tracker = make(map[string]*tlsAgentLoopWindow)
	s.mu.Unlock()
}

func tlsAgentLoopIdentity(event *TLSPlaintextEvent) string {
	if event == nil {
		return ""
	}
	if event.AgentRunID != "" {
		return "agent_run:" + event.AgentRunID
	}
	if event.TaskID != "" {
		return "task:" + event.TaskID
	}
	if event.ToolCallID != "" {
		return "tool_call:" + event.ToolCallID
	}
	if event.TraceID != "" {
		return "trace:" + event.TraceID
	}
	if event.RootAgentPID != 0 {
		return fmt.Sprintf("root_pid:%d", event.RootAgentPID)
	}
	if event.PID != 0 {
		return fmt.Sprintf("pid:%d", event.PID)
	}
	return ""
}

func buildTLSLoopAlertEvent(source *TLSPlaintextEvent, identity string, repeats int, window time.Duration) *pb.Event {
	if source == nil {
		return nil
	}
	target := firstNonEmpty(source.Host, source.URL, identity)
	reason := fmt.Sprintf("observed %d repeated TLS prompt requests with digest %s within %s (identity=%s, vendor=%s)",
		repeats, source.PromptDigest, window, identity, source.Vendor)
	endpoint := strings.TrimSpace(source.Host)
	return &pb.Event{
		Pid:            source.PID,
		Tgid:           source.TGID,
		Type:           "semantic_alert",
		EventType:      pb.EventType_SEMANTIC_ALERT,
		Tag:            "Security",
		Comm:           "RESOURCE_WASTING_LOOP",
		Path:           target,
		ExtraInfo:      fmt.Sprintf("source=tls_plaintext vendor=%s prompt_digest=%s repeats=%d window=%s role=%s", source.Vendor, source.PromptDigest, repeats, window, source.MessageRole),
		SchemaVersion:  eventSchemaVersion,
		RootAgentPid:   source.RootAgentPID,
		AgentRunId:     source.AgentRunID,
		TaskId:         source.TaskID,
		ConversationId: source.ConversationID,
		TurnId:         source.TurnID,
		ToolCallId:     source.ToolCallID,
		ToolName:       source.ToolName,
		TraceId:        source.TraceID,
		SpanId:         source.SpanID,
		Decision:       "ALERT",
		RiskScore:      tlsAgentLoopAlertMinRisk,
		NetEndpoint:    endpoint,
		NetDirection:   source.Direction,
		HttpHost:       source.Host,
		AppProtocol:    "tls_plaintext",
		ServiceName:    source.Vendor,
		ArgvDigest:     reason,
	}
}

// convertTLSToProtoEvent produces a pb.Event mirror of a TLS plaintext event so
// that the unified pipeline (enrichEventContext + buildSemanticAlerts +
// archive) can correlate prompt digests with kernel-level activity in the
// same agent context. The plaintext body is never copied into the pb.Event;
// only metadata + digest end up on the wire.
func convertTLSToProtoEvent(source TLSPlaintextEvent) *pb.Event {
	if source.PID == 0 && source.TGID == 0 {
		return nil
	}
	eventType := "tls_plaintext"
	if source.PromptDigest == "" && source.Method == "" && source.StatusCode == 0 {
		return nil
	}
	method := strings.TrimSpace(source.Method)
	url := strings.TrimSpace(source.URL)
	host := strings.TrimSpace(source.Host)
	extras := []string{
		"lib=" + source.Lib,
		"dir=" + source.Direction,
	}
	if source.Vendor != "" {
		extras = append(extras, "vendor="+source.Vendor)
	}
	if source.MessageRole != "" {
		extras = append(extras, "role="+source.MessageRole)
	}
	if source.PromptDigest != "" {
		extras = append(extras, "prompt_digest="+source.PromptDigest)
		extras = append(extras, fmt.Sprintf("prompt_len=%d", source.PromptLen))
	}
	if source.StatusCode != 0 {
		extras = append(extras, fmt.Sprintf("status=%d", source.StatusCode))
	}
	if source.BodySize != 0 {
		extras = append(extras, fmt.Sprintf("body_size=%d", source.BodySize))
	}
	if source.LoopAlert {
		extras = append(extras, "loop_alert=true")
	}
	return &pb.Event{
		Pid:            source.PID,
		Tgid:           source.TGID,
		Type:           eventType,
		EventType:      pb.EventType_NETWORK_CONNECT,
		Tag:            "AI Agent",
		Comm:           source.Comm,
		Path:           method,
		ExtraPath:      url,
		ExtraInfo:      strings.Join(extras, " "),
		SchemaVersion:  eventSchemaVersion,
		RootAgentPid:   source.RootAgentPID,
		AgentRunId:     source.AgentRunID,
		TaskId:         source.TaskID,
		ConversationId: source.ConversationID,
		TurnId:         source.TurnID,
		ToolCallId:     source.ToolCallID,
		ToolName:       source.ToolName,
		TraceId:        source.TraceID,
		SpanId:         source.SpanID,
		HttpHost:       host,
		NetEndpoint:    host,
		NetDirection:   source.Direction,
		AppProtocol:    "tls_plaintext",
		ServiceName:    source.Vendor,
	}
}

// dispatchTLSAgentEvent enriches the TLS event with agent context, derives
// prompt metadata, fires loop detection, and bridges the result into the main
// pb.Event pipeline. It is invoked from TLSProbeManager.ReadLoop after
// parsing each completed plaintext fragment.
func dispatchTLSAgentEvent(event *TLSPlaintextEvent, loopState *tlsAgentLoopState, bridge chan<- *pb.Event) {
	if event == nil {
		return
	}
	enrichTLSEventWithAgentContext(event)
	annotateTLSAgentMessage(event)

	if loopState != nil {
		if alert := loopState.Observe(event); alert != nil && bridge != nil {
			sendTLSBridge(bridge, alert)
		}
	}
	if proto := convertTLSToProtoEvent(*event); proto != nil && bridge != nil {
		sendTLSBridge(bridge, proto)
	}
}

func sendTLSBridge(bridge chan<- *pb.Event, event *pb.Event) {
	if bridge == nil || event == nil {
		return
	}
	select {
	case bridge <- event:
	default:
	}
}

var tlsAgentLoopDetector = newTLSAgentLoopState()
