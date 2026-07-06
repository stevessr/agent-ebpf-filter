package tls

import (
	"agent-ebpf-filter/pb"
	"strings"
	"testing"
	"time"
)

// ---- moved from backend/zz_merged_backend_test.go section agentstreamtls_test.go ----

func TestExtractAgentMessageMetaOpenAI(t *testing.T) {
	body := `{
		"model": "gpt-4o-mini",
		"messages": [
			{"role": "system", "content": "you are a helpful agent"},
			{"role": "user", "content": "please refactor this function"}
		]
	}`
	digest, role, vendor, length := extractAgentMessageMeta(body, "application/json", "api.openai.com", "/v1/chat/completions", "send")
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest = %q", digest)
	}
	if role != "user" {
		t.Fatalf("role = %q want user", role)
	}
	if vendor != "openai" {
		t.Fatalf("vendor = %q want openai", vendor)
	}
	if length != len("please refactor this function") {
		t.Fatalf("length = %d", length)
	}
}

func TestExtractAgentMessageMetaAnthropic(t *testing.T) {
	body := `{
		"model": "claude-opus-4",
		"messages": [
			{"role": "user", "content": [{"type": "text", "text": "hello claude"}]}
		]
	}`
	digest, role, vendor, length := extractAgentMessageMeta(body, "application/json", "api.anthropic.com", "/v1/messages", "send")
	if !strings.HasPrefix(digest, "sha256:") {
		t.Fatalf("digest = %q", digest)
	}
	if role != "user" {
		t.Fatalf("role = %q", role)
	}
	if vendor != "anthropic" {
		t.Fatalf("vendor = %q", vendor)
	}
	if length != len("hello claude") {
		t.Fatalf("length = %d", length)
	}
}

func TestExtractAgentMessageMetaGemini(t *testing.T) {
	body := `{
		"contents": [
			{"role": "user", "parts": [{"text": "write a haiku about ebpf"}]}
		]
	}`
	digest, role, vendor, _ := extractAgentMessageMeta(body, "application/json", "generativelanguage.googleapis.com", "/v1beta/models/gemini-pro:generateContent", "send")
	if digest == "" {
		t.Fatal("digest empty")
	}
	if role != "user" {
		t.Fatalf("role = %q", role)
	}
	if vendor != "google" {
		t.Fatalf("vendor = %q", vendor)
	}
}

func TestExtractAgentMessageMetaResponseChoice(t *testing.T) {
	body := `{
		"choices": [
			{"message": {"role": "assistant", "content": "here is the answer"}}
		]
	}`
	digest, role, vendor, _ := extractAgentMessageMeta(body, "application/json", "api.openai.com", "/v1/chat/completions", "recv")
	if digest == "" {
		t.Fatal("digest empty")
	}
	if role != "assistant" {
		t.Fatalf("role = %q", role)
	}
	if vendor != "openai" {
		t.Fatalf("vendor = %q", vendor)
	}
}

func TestExtractAgentMessageMetaFallbackRaw(t *testing.T) {
	body := "free-form text not JSON"
	digest, role, _, length := extractAgentMessageMeta(body, "text/plain", "example.com", "/", "send")
	if digest == "" {
		t.Fatal("digest empty")
	}
	if role != "raw" {
		t.Fatalf("role = %q want raw", role)
	}
	if length != len(body) {
		t.Fatalf("length = %d", length)
	}
}

func TestExtractAgentMessageMetaEmptyBody(t *testing.T) {
	digest, role, _, length := extractAgentMessageMeta("", "application/json", "api.openai.com", "/", "send")
	if digest != "" || role != "" || length != 0 {
		t.Fatalf("unexpected non-empty result: digest=%q role=%q length=%d", digest, role, length)
	}
}

func TestEnrichTLSEventWithAgentContext(t *testing.T) {
	const pid uint32 = 9911
	trackedProcessContexts.Set(pid, processContext{
		RootAgentPid:   pid,
		AgentRunID:     "run-abc",
		TaskID:         "task-1",
		ToolCallID:     "call-9",
		ToolName:       "code_editor",
		TraceID:        "trace-x",
		SpanID:         "span-y",
		ConversationID: "conv-7",
		TurnID:         "turn-2",
	})
	t.Cleanup(func() { trackedProcessContexts.Delete(pid) })

	event := &TLSPlaintextEvent{PID: pid, TGID: pid}
	enrichTLSEventWithAgentContext(event)
	if event.AgentRunID != "run-abc" || event.TaskID != "task-1" || event.ToolCallID != "call-9" {
		t.Fatalf("context not applied: %+v", event)
	}
	if event.RootAgentPID != pid {
		t.Fatalf("root pid = %d want %d", event.RootAgentPID, pid)
	}
}

func TestEnrichTLSEventFallsBackToTGID(t *testing.T) {
	const tgid uint32 = 4242
	trackedProcessContexts.Set(tgid, processContext{
		AgentRunID: "tgid-run",
	})
	t.Cleanup(func() { trackedProcessContexts.Delete(tgid) })

	event := &TLSPlaintextEvent{PID: 1, TGID: tgid}
	enrichTLSEventWithAgentContext(event)
	if event.AgentRunID != "tgid-run" {
		t.Fatalf("fallback to tgid failed: %+v", event)
	}
}

func TestTLSAgentLoopStateAlertsAfterRepeat(t *testing.T) {
	state := NewAgentLoopState()
	state.limit = 3
	clock := time.Unix(1700000000, 0).UTC()
	state.now = func() time.Time { return clock }

	source := func() *TLSPlaintextEvent {
		return &TLSPlaintextEvent{
			PID:          7,
			AgentRunID:   "run-loop",
			Direction:    "send",
			Host:         "api.openai.com",
			URL:          "/v1/chat/completions",
			Lib:          "openssl",
			Vendor:       "openai",
			PromptDigest: "sha256:deadbeef",
			MessageRole:  "user",
		}
	}

	for i := 1; i < 3; i++ {
		if alert := state.Observe(source()); alert != nil {
			t.Fatalf("unexpected alert on iter %d: %+v", i, alert)
		}
	}
	alert := state.Observe(source())
	if alert == nil {
		t.Fatal("expected alert after threshold")
	}
	if alert.Type != "semantic_alert" {
		t.Fatalf("alert type = %q", alert.Type)
	}
	if alert.Comm != "RESOURCE_WASTING_LOOP" {
		t.Fatalf("alert comm = %q", alert.Comm)
	}
	if alert.AgentRunId != "run-loop" {
		t.Fatalf("alert agent run id = %q", alert.AgentRunId)
	}

	if again := state.Observe(source()); again != nil {
		t.Fatal("alert should fire once per window")
	}
}

func TestTLSAgentLoopStateResetsOnDigestChange(t *testing.T) {
	state := NewAgentLoopState()
	state.limit = 2
	clock := time.Unix(1700000000, 0).UTC()
	state.now = func() time.Time { return clock }

	first := &TLSPlaintextEvent{PID: 1, AgentRunID: "run-1", PromptDigest: "sha256:aaaa"}
	if state.Observe(first) != nil {
		t.Fatal("first observation should not alert")
	}
	second := &TLSPlaintextEvent{PID: 1, AgentRunID: "run-1", PromptDigest: "sha256:bbbb"}
	if state.Observe(second) != nil {
		t.Fatal("digest change should reset window")
	}
	third := &TLSPlaintextEvent{PID: 1, AgentRunID: "run-1", PromptDigest: "sha256:bbbb"}
	if state.Observe(third) == nil {
		t.Fatal("expected alert after threshold on new digest")
	}
}

func TestTLSAgentLoopStateExpiresWindow(t *testing.T) {
	state := NewAgentLoopState()
	state.limit = 2
	clock := time.Unix(1700000000, 0).UTC()
	state.now = func() time.Time { return clock }

	first := &TLSPlaintextEvent{PID: 1, AgentRunID: "run-x", PromptDigest: "sha256:abcd"}
	state.Observe(first)

	clock = clock.Add(2 * tlsAgentLoopDefaultWindow)
	second := &TLSPlaintextEvent{PID: 1, AgentRunID: "run-x", PromptDigest: "sha256:abcd"}
	if alert := state.Observe(second); alert != nil {
		t.Fatalf("expired window should reset: %+v", alert)
	}
}

func TestConvertTLSToProtoEventEmitsContext(t *testing.T) {
	event := TLSPlaintextEvent{
		PID:          77,
		TGID:         77,
		Comm:         "claude",
		Direction:    "send",
		Lib:          "openssl",
		Method:       "POST",
		URL:          "/v1/messages",
		Host:         "api.anthropic.com",
		Vendor:       "anthropic",
		AgentRunID:   "run-1",
		TaskID:       "task-5",
		ToolName:     "code_editor",
		PromptDigest: "sha256:cafef00d",
		PromptLen:    42,
		MessageRole:  "user",
	}
	proto := convertTLSToProtoEvent(event)
	if proto == nil {
		t.Fatal("proto event nil")
	}
	if proto.Type != "tls_plaintext" {
		t.Fatalf("type = %q", proto.Type)
	}
	if proto.AgentRunId != "run-1" || proto.TaskId != "task-5" {
		t.Fatalf("context missing: %+v", proto)
	}
	if !strings.Contains(proto.ExtraInfo, "prompt_digest=sha256:cafef00d") {
		t.Fatalf("extra info missing digest: %q", proto.ExtraInfo)
	}
	if !strings.Contains(proto.ExtraInfo, "vendor=anthropic") {
		t.Fatalf("extra info missing vendor: %q", proto.ExtraInfo)
	}
	if proto.HttpHost != "api.anthropic.com" {
		t.Fatalf("http host = %q", proto.HttpHost)
	}
}

func TestConvertTLSToProtoEventNilWhenNoSignal(t *testing.T) {
	if proto := convertTLSToProtoEvent(TLSPlaintextEvent{PID: 1}); proto != nil {
		t.Fatalf("expected nil for empty plaintext signal, got %+v", proto)
	}
}

func TestDispatchTLSAgentEventBridgesAndDetectsLoop(t *testing.T) {
	const pid uint32 = 5151
	trackedProcessContexts.Set(pid, processContext{AgentRunID: "run-disp", ToolName: "agent_tool"})
	t.Cleanup(func() { trackedProcessContexts.Delete(pid) })

	state := NewAgentLoopState()
	state.limit = 2
	clock := time.Unix(1700000100, 0).UTC()
	state.now = func() time.Time { return clock }

	bridge := make(chan *pb.Event, 8)
	emit := func() *TLSPlaintextEvent {
		return &TLSPlaintextEvent{
			PID:         pid,
			TGID:        pid,
			Direction:   "send",
			Lib:         "openssl",
			Method:      "POST",
			URL:         "/v1/chat/completions",
			Host:        "api.openai.com",
			ContentType: "application/json",
			Body:        `{"messages":[{"role":"user","content":"loop me"}]}`,
		}
	}

	for i := 0; i < 2; i++ {
		ev := emit()
		DispatchTLSAgentEvent(ev, state, bridge)
	}
	ev := emit()
	DispatchTLSAgentEvent(ev, state, bridge)

	var sawTLS, sawAlert bool
	for {
		select {
		case got := <-bridge:
			switch got.Type {
			case "tls_plaintext":
				sawTLS = true
				if got.AgentRunId != "run-disp" {
					t.Fatalf("tls_plaintext missing agent context: %+v", got)
				}
				if !strings.Contains(got.ExtraInfo, "prompt_digest=sha256:") {
					t.Fatalf("tls_plaintext missing prompt digest extra info: %q", got.ExtraInfo)
				}
			case "semantic_alert":
				sawAlert = true
				if got.Comm != "RESOURCE_WASTING_LOOP" {
					t.Fatalf("alert comm = %q", got.Comm)
				}
				if got.AgentRunId != "run-disp" {
					t.Fatalf("alert missing agent run id: %+v", got)
				}
			}
		default:
			if !sawTLS {
				t.Fatal("expected at least one tls_plaintext pb.Event on bridge")
			}
			if !sawAlert {
				t.Fatal("expected RESOURCE_WASTING_LOOP alert on bridge")
			}
			return
		}
	}
}

type tlsBridgeMetricRecorder struct {
	queued  int
	dropped int
	reason  string
}

func (r *tlsBridgeMetricRecorder) RecordAgentSightCounter(string) {}

func (r *tlsBridgeMetricRecorder) RecordBroadcastEnqueue(accepted bool, reason string) {
	if accepted {
		r.queued++
		return
	}
	r.dropped++
	r.reason = reason
}

func TestSendTLSBridgeRecordsEnqueueMetrics(t *testing.T) {
	oldMetrics := deps.CollectorMetrics
	recorder := &tlsBridgeMetricRecorder{}
	deps.CollectorMetrics = recorder
	t.Cleanup(func() { deps.CollectorMetrics = oldMetrics })

	bridge := make(chan *pb.Event, 1)
	SendTLSBridge(bridge, &pb.Event{Type: "tls_plaintext"})
	SendTLSBridge(bridge, &pb.Event{Type: "tls_plaintext"})
	SendTLSBridge(nil, &pb.Event{Type: "tls_plaintext"})
	SendTLSBridge(bridge, nil)

	if recorder.queued != 1 || recorder.dropped != 3 || recorder.reason != "tls_bridge:nil_event" {
		t.Fatalf("tls bridge metrics mismatch: %+v", recorder)
	}
}

func TestAnnotateTLSAgentMessageNoBody(t *testing.T) {
	event := &TLSPlaintextEvent{}
	annotateTLSAgentMessage(event)
	if event.PromptDigest != "" || event.Vendor != "" {
		t.Fatalf("expected unchanged event: %+v", event)
	}
}

func TestInferTLSVendor(t *testing.T) {
	cases := map[string]string{
		"api.openai.com":                    "openai",
		"api.anthropic.com":                 "anthropic",
		"generativelanguage.googleapis.com": "google",
		"ollama.local":                      "ollama",
		"api.cohere.ai":                     "cohere",
		"api.mistral.ai":                    "mistral",
		"runtime.sagemaker.amazonaws.com":   "",
		"bedrock.us-east-1.amazonaws.com":   "aws-bedrock",
		"foo.azure.com":                     "azure",
		"unrelated.example.com":             "",
	}
	for host, want := range cases {
		if got := inferTLSVendor(host, ""); got != want {
			t.Errorf("inferTLSVendor(%q) = %q want %q", host, got, want)
		}
	}
}
