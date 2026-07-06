package tls

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section agentstreamlooptls.go ----

const (
	tlsAgentLoopDefaultWindow = 30 * time.Second
	tlsAgentLoopRepeatLimit   = 5
	tlsAgentLoopAlertMinRisk  = 0.97
	tlsPromptDigestBytes      = 8
)

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

// AgentLoopState tracks repeated prompt digests within a short window per
// agent identity. Once the same digest repeats beyond the configured limit
// it produces a synthetic RESOURCE_WASTING_LOOP semantic_alert event, so that
// loops manifesting purely as encrypted API traffic (no fs/forks) still
// surface.
type AgentLoopState struct {
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

func NewAgentLoopState() *AgentLoopState {
	return &AgentLoopState{
		tracker:  make(map[string]*tlsAgentLoopWindow),
		now:      time.Now,
		window:   tlsAgentLoopDefaultWindow,
		limit:    tlsAgentLoopRepeatLimit,
		maxIdent: 1024,
	}
}

// Observe records the digest emitted by a TLS event and returns an alert if
// the repeat threshold has been crossed for this identity inside the window.
func (s *AgentLoopState) Observe(event *TLSPlaintextEvent) *pb.Event {
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

func (s *AgentLoopState) evictExpiredLocked(now time.Time) {
	for key, state := range s.tracker {
		if state == nil || now.Sub(state.LastSeen) > s.window*2 {
			delete(s.tracker, key)
		}
	}
}

func (s *AgentLoopState) reset() {
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
	target := platform.FirstNonEmpty(source.Host, source.URL, identity)
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
		EventType:      pb.EventType_TLS_PLAINTEXT,
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

func convertTLSToOTelSpanEvent(source TLSPlaintextEvent) *pb.Event {
	if source.PID == 0 && source.TGID == 0 {
		return nil
	}
	if source.Vendor == "" && source.PromptDigest == "" {
		return nil
	}
	extra := []string{
		"kind=client",
		"provider=" + source.Vendor,
		"prompt_digest=" + source.PromptDigest,
		fmt.Sprintf("prompt_len=%d", source.PromptLen),
	}
	return &pb.Event{
		Pid:            source.PID,
		Tgid:           source.TGID,
		Type:           "otel_span",
		EventType:      pb.EventType_OTEL_SPAN,
		Tag:            "AgentSight OTEL",
		Comm:           source.Comm,
		Path:           platform.FirstNonEmpty(source.ToolName, source.Vendor, "genai.request"),
		ExtraInfo:      strings.Join(extra, " "),
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
		ServiceName:    source.Vendor,
	}
}

// DispatchTLSAgentEvent enriches the TLS event with agent context, derives
// prompt metadata, fires loop detection, and bridges the result into the main
// pb.Event pipeline. It is invoked from TLSProbeManager.ReadLoop after
// parsing each completed plaintext fragment.
func DispatchTLSAgentEvent(event *TLSPlaintextEvent, loopState *AgentLoopState, bridge chan<- *pb.Event) {
	if event == nil {
		return
	}
	enrichTLSEventWithAgentContext(event)
	annotateTLSAgentMessage(event)

	if loopState != nil {
		if alert := loopState.Observe(event); alert != nil && bridge != nil {
			SendTLSBridge(bridge, alert)
		}
	}
	if proto := convertTLSToProtoEvent(*event); proto != nil && bridge != nil {
		SendTLSBridge(bridge, proto)
		if otel := convertTLSToOTelSpanEvent(*event); otel != nil {
			SendTLSBridge(bridge, otel)
		}
	}
}

func SendTLSBridge(bridge chan<- *pb.Event, event *pb.Event) {
	if event == nil {
		recordTLSBridgeEnqueue(false, "tls_bridge:nil_event")
		return
	}
	if bridge == nil {
		recordTLSBridgeEnqueue(false, "tls_bridge:queue_unavailable")
		return
	}
	select {
	case bridge <- event:
		recordTLSBridgeEnqueue(true, "")
	default:
		recordTLSBridgeEnqueue(false, "tls_bridge:queue_full")
	}
}

func recordTLSBridgeEnqueue(accepted bool, reason string) {
	if deps.CollectorMetrics != nil {
		deps.CollectorMetrics.RecordBroadcastEnqueue(accepted, reason)
	}
}

var tlsAgentLoopDetector = NewAgentLoopState()
