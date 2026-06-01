package main

import (
	bpf "agent-ebpf-filter/ebpf"
	"agent-ebpf-filter/internal/behavior"
	"agent-ebpf-filter/pb"
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/ulikunitz/xz"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"math/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"
)

// ---- merged from agentstreamtls_test.go ----

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
	state := newTLSAgentLoopState()
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
	state := newTLSAgentLoopState()
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
	state := newTLSAgentLoopState()
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

	state := newTLSAgentLoopState()
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
		dispatchTLSAgentEvent(ev, state, bridge)
	}
	ev := emit()
	dispatchTLSAgentEvent(ev, state, bridge)

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

// ---- merged from alertssemantic_test.go ----

func TestSemanticAlertsDetectAgenticResourceLoopFromSafeMetadata(t *testing.T) {
	resetSemanticAlertState()

	base := &pb.Event{
		Pid:          100,
		Tgid:         100,
		RootAgentPid: 100,
		AgentRunId:   "run-loop",
		ToolName:     "chat",
		Comm:         "agent",
	}
	promptDigest := digestHookText("repeat this prompt")

	for i := 0; i < semanticPromptLoopThreshold; i++ {
		event := cloneProtoEvent(base)
		event.Type = "native_hook"
		event.EventType = pb.EventType_NATIVE_HOOK
		event.ExtraInfo = "prompt_digest=" + promptDigest + " prompt_len=18"
		if alerts := buildSemanticAlerts(event); hasSemanticAlertCode(alerts, "RESOURCE_WASTING_LOOP") {
			t.Fatalf("prompt metadata alone should not produce resource loop alert")
		}
	}
	for i := 0; i < semanticAPILoopThreshold; i++ {
		event := cloneProtoEvent(base)
		event.Type = "network_connect"
		event.EventType = pb.EventType_NETWORK_CONNECT
		event.NetEndpoint = "api.openai.example:443"
		event.DstPort = 443
		_ = buildSemanticAlerts(event)
	}

	var finalAlerts []*pb.Event
	for i := 0; i < semanticFileIOLoopThreshold; i++ {
		event := cloneProtoEvent(base)
		event.Type = "openat"
		event.EventType = pb.EventType_OPENAT
		event.Path = "/tmp/agent-cache.json"
		finalAlerts = buildSemanticAlerts(event)
	}

	alert := findSemanticAlertCode(finalAlerts, "RESOURCE_WASTING_LOOP")
	if alert == nil {
		t.Fatalf("expected RESOURCE_WASTING_LOOP after repeated prompt metadata, API calls, and file I/O")
	}
	if !strings.Contains(alert.GetExtraInfo(), "repeated prompt metadata") {
		t.Fatalf("alert reason did not describe agentic loop correlation: %q", alert.GetExtraInfo())
	}
	if alert.GetTgid() != base.GetTgid() {
		t.Fatalf("alert tgid = %d, want %d", alert.GetTgid(), base.GetTgid())
	}
}

func TestSemanticAlertsDetectMultiAgentFileContention(t *testing.T) {
	resetSemanticAlertState()

	first := &pb.Event{
		Pid:        201,
		Tgid:       201,
		Type:       "write",
		EventType:  pb.EventType_WRITE,
		Path:       "/workspace/shared-plan.md",
		AgentRunId: "run-a",
		ToolName:   "editor",
		Comm:       "agent-a",
	}
	if alerts := buildSemanticAlerts(first); hasSemanticAlertCode(alerts, "MULTI_AGENT_FILE_CONTENTION") {
		t.Fatalf("first writer should not produce contention alert")
	}

	second := &pb.Event{
		Pid:        301,
		Tgid:       301,
		Type:       "write",
		EventType:  pb.EventType_WRITE,
		Path:       "/workspace/shared-plan.md",
		AgentRunId: "run-b",
		ToolName:   "editor",
		Comm:       "agent-b",
	}
	alert := findSemanticAlertCode(buildSemanticAlerts(second), "MULTI_AGENT_FILE_CONTENTION")
	if alert == nil {
		t.Fatalf("expected MULTI_AGENT_FILE_CONTENTION for different agent run touching same path")
	}
	if alert.GetPath() != "/workspace/shared-plan.md" {
		t.Fatalf("alert path = %q", alert.GetPath())
	}
	if !strings.Contains(alert.GetExtraInfo(), "run-a") || !strings.Contains(alert.GetExtraInfo(), "run-b") {
		t.Fatalf("alert reason should include both agent contexts: %q", alert.GetExtraInfo())
	}
}

func findSemanticAlertCode(alerts []*pb.Event, code string) *pb.Event {
	for _, alert := range alerts {
		if alert.GetType() == "semantic_alert" && alert.GetComm() == code {
			return alert
		}
	}
	return nil
}

func hasSemanticAlertCode(alerts []*pb.Event, code string) bool {
	return findSemanticAlertCode(alerts, code) != nil
}

// ---- merged from apiexternal_test.go ----

func TestExternalAPIRoutesExposeHealthAndOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerExternalAPIRoutes(router.Group("/api/v1"))

	for _, tc := range []struct {
		path string
		key  string
	}{
		{path: "/api/v1/health", key: "apiVersion"},
		{path: "/api/v1/openapi.json", key: "openapi"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", tc.path, rec.Code, rec.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s returned invalid JSON: %v", tc.path, err)
		}
		if _, ok := body[tc.key]; !ok {
			t.Fatalf("%s response missing %q: %+v", tc.path, tc.key, body)
		}
	}
}

func TestExternalOpenAPISpecUsesLowercaseHTTPMethods(t *testing.T) {
	paths, ok := buildExternalOpenAPISpec()["paths"].(gin.H)
	if !ok {
		t.Fatal("OpenAPI spec missing paths")
	}
	health, ok := paths["/health"].(gin.H)
	if !ok {
		t.Fatal("OpenAPI spec missing /health")
	}
	if _, ok := health["get"]; !ok {
		t.Fatalf("expected lowercase get method, got %+v", health)
	}
	if _, ok := health["GET"]; ok {
		t.Fatalf("OpenAPI method keys must be lowercase, got %+v", health)
	}
}

// ---- merged from capturehandlerstls_test.go ----

func TestHandleTLSCaptureRecentReturnsStoredEventsWithoutAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewTLSCaptureStore(10)
	store.Add(TLSPlaintextEvent{Type: "tls_plaintext", PID: 42, Comm: "curl", Timestamp: time.Unix(1, 0).UTC()})

	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, store, NewTLSCaptureRuleStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/tls-capture/recent?limit=5", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Events []TLSPlaintextEvent `json:"events"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(resp.Events) != 1 || resp.Events[0].PID != 42 {
		t.Fatalf("events = %#v", resp.Events)
	}
}

func TestHandleTLSCaptureGoBinaryRejectsMissingPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, NewTLSCaptureStore(10), NewTLSCaptureRuleStore())

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/tls-capture/go-binary", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestHandleTLSCaptureRulesRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rules := NewTLSCaptureRuleStore()
	r := gin.New()
	registerTLSCaptureRoutes(r.Group("/"), nil, NewTLSCaptureStore(10), rules)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/tls-capture/rules", strings.NewReader(`{"rules":[{"id":"node-api","name":"Node API","enabled":true,"scope":"custom","comms":["node"],"hosts":["api.example.com"]}]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("put status = %d body = %s", w.Code, w.Body.String())
	}

	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/tls-capture/rules", nil)
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Rules []TLSCaptureRule `json:"rules"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if len(resp.Rules) != 1 || resp.Rules[0].ID != "node-api" || len(resp.Rules[0].Hosts) != 1 {
		t.Fatalf("rules = %#v", resp.Rules)
	}
}

func TestTLSCaptureBroadcasterServeAndBroadcast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	broadcaster := newTLSCaptureBroadcaster()
	r := gin.New()
	r.GET("/ws/tls-capture", broadcaster.Serve)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/tls-capture"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	event := TLSPlaintextEvent{Type: "tls_plaintext", PID: 99, Comm: "curl", Timestamp: time.Unix(2, 0).UTC()}
	broadcaster.Broadcast(event)

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var got TLSPlaintextEvent
	if err := conn.ReadJSON(&got); err != nil {
		t.Fatalf("read json: %v", err)
	}
	if got.PID != event.PID || got.Type != event.Type || got.Comm != event.Comm || !got.Timestamp.Equal(event.Timestamp) {
		t.Fatalf("event = %#v", got)
	}
}

func TestTLSCaptureBroadcasterConcurrentBroadcastsDeliverEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	broadcaster := newTLSCaptureBroadcaster()
	r := gin.New()
	r.GET("/ws/tls-capture", broadcaster.Serve)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/tls-capture"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	const broadcasters = 8
	const eventsPerBroadcaster = 4
	const totalEvents = broadcasters * eventsPerBroadcaster

	start := make(chan struct{})
	errCh := make(chan error, totalEvents)
	for i := 0; i < broadcasters; i++ {
		go func(base int) {
			<-start
			for j := 0; j < eventsPerBroadcaster; j++ {
				broadcaster.Broadcast(TLSPlaintextEvent{
					Type:      "tls_plaintext",
					PID:       uint32(100 + base + j),
					Comm:      "curl",
					Timestamp: time.Unix(int64(base*eventsPerBroadcaster+j+1), 0).UTC(),
				})
			}
			errCh <- nil
		}(i * eventsPerBroadcaster)
	}

	close(start)
	for i := 0; i < broadcasters; i++ {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("broadcast error: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("broadcast goroutine %d timed out", i)
		}
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	seen := make(map[uint32]struct{}, totalEvents)
	for len(seen) < totalEvents {
		var got TLSPlaintextEvent
		if err := conn.ReadJSON(&got); err != nil {
			t.Fatalf("read json after %d events: %v", len(seen), err)
		}
		seen[got.PID] = struct{}{}
	}
}

// ---- merged from capturerulestls_test.go ----

func TestDefaultTLSCaptureRuleAllowsAgentContextOnly(t *testing.T) {
	rules := NewTLSCaptureRuleStore()
	pid := uint32(4242)
	trackedProcessContexts.Set(pid, processContext{AgentRunID: "run-ssl"})
	t.Cleanup(func() { trackedProcessContexts.Delete(pid) })

	if !rules.Allows(TLSPlaintextEvent{PID: pid, TGID: pid, Comm: "claude"}) {
		t.Fatal("default rule should allow agent CLI tagged process")
	}
	if rules.Allows(TLSPlaintextEvent{PID: 9999, TGID: 9999, Comm: "curl"}) {
		t.Fatal("default rule should reject untagged process")
	}
}

func TestCustomTLSCaptureRuleMatchesCommonFields(t *testing.T) {
	rules := NewTLSCaptureRuleStore()
	rules.Replace([]TLSCaptureRule{
		{ID: "custom", Name: "Custom", Enabled: true, Scope: "custom", Comms: []string{"node"}, Hosts: []string{"api.example.com"}, Methods: []string{"POST"}, Libraries: []string{"OpenSSL"}, Directions: []string{"send"}},
	})

	allowed := TLSPlaintextEvent{Comm: "node", Host: "api.example.com", Method: "POST", Lib: "openssl", Direction: "send"}
	if !rules.Allows(allowed) {
		t.Fatal("custom rule should match normalized fields")
	}
	blocked := allowed
	blocked.Host = "other.example.com"
	if rules.Allows(blocked) {
		t.Fatal("custom rule should reject non-matching host")
	}
}

// ---- merged from capturestoretls_test.go ----

func TestTLSCaptureStoreKeepsOnlyMostRecentEvents(t *testing.T) {
	store := NewTLSCaptureStore(2)
	store.Add(TLSPlaintextEvent{PID: 1, Timestamp: time.Unix(1, 0)})
	store.Add(TLSPlaintextEvent{PID: 2, Timestamp: time.Unix(2, 0)})
	store.Add(TLSPlaintextEvent{PID: 3, Timestamp: time.Unix(3, 0)})

	recent := store.Recent(10)
	if len(recent) != 2 {
		t.Fatalf("recent len = %d, want 2", len(recent))
	}
	if recent[0].PID != 2 || recent[1].PID != 3 {
		t.Fatalf("recent = %#v, want PIDs 2 and 3", recent)
	}
}

func TestTLSCaptureStoreTracksLibraryStatuses(t *testing.T) {
	store := NewTLSCaptureStore(10)
	store.SetLibraryStatus(TLSLibraryStatus{Name: "OpenSSL", Path: "/usr/lib/libssl.so", Attached: true, Available: true})
	store.SetLibraryStatus(TLSLibraryStatus{Name: "GnuTLS", Path: "/usr/lib/libgnutls.so", Error: "missing symbol"})

	statuses := store.LibraryStatuses()
	if len(statuses) != 2 {
		t.Fatalf("statuses len = %d, want 2", len(statuses))
	}

	var seenOpenSSL, seenGnuTLS bool
	for _, status := range statuses {
		switch status.Name {
		case "OpenSSL":
			seenOpenSSL = status.Attached && status.Available && status.Path == "/usr/lib/libssl.so"
		case "GnuTLS":
			seenGnuTLS = status.Error == "missing symbol" && status.Path == "/usr/lib/libgnutls.so"
		}
	}
	if !seenOpenSSL || !seenGnuTLS {
		t.Fatalf("statuses = %#v", statuses)
	}
}

func TestTLSCaptureStoreLibraryStatusesAreSortedByNameThenPath(t *testing.T) {
	store := NewTLSCaptureStore(10)
	store.SetLibraryStatus(TLSLibraryStatus{Name: "OpenSSL", Path: "/opt/libssl.so"})
	store.SetLibraryStatus(TLSLibraryStatus{Name: "GnuTLS", Path: "/usr/lib/libgnutls.so"})
	store.SetLibraryStatus(TLSLibraryStatus{Name: "OpenSSL", Path: "/usr/lib/libssl.so"})

	statuses := store.LibraryStatuses()
	if len(statuses) != 3 {
		t.Fatalf("statuses len = %d, want 3", len(statuses))
	}

	want := []TLSLibraryStatus{
		{Name: "GnuTLS", Path: "/usr/lib/libgnutls.so"},
		{Name: "OpenSSL", Path: "/opt/libssl.so"},
		{Name: "OpenSSL", Path: "/usr/lib/libssl.so"},
	}
	for i, status := range statuses {
		if status.Name != want[i].Name || status.Path != want[i].Path {
			t.Fatalf("status[%d] = %#v, want %#v", i, status, want[i])
		}
	}
}

// ---- merged from commandsafety_test.go ----

func TestSplitCommandLinePreservesQuotedCommandArgument(t *testing.T) {
	got := splitCommandLine(`sudo bash -c "rm -rf /tmp/demo"`)
	want := []string{"sudo", "bash", "-c", "rm -rf /tmp/demo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitCommandLine() = %#v, want %#v", got, want)
	}
}

func TestCommandCandidateFromWrapperRecord(t *testing.T) {
	record := CapturedEventRecord{
		ReceivedAt: time.Unix(1700000000, 0).UTC(),
		Event: &pb.Event{
			Type:      "wrapper_intercept",
			EventType: pb.EventType_WRAPPER_INTERCEPT,
			Comm:      "rm",
			Path:      "rm -rf /tmp/demo",
			Behavior:  &pb.BehaviorClassification{PrimaryCategory: "FILE_DELETE"},
		},
	}

	got, ok := commandCandidateFromRecord(record, "memory")
	if !ok {
		t.Fatal("commandCandidateFromRecord() did not recognize wrapper event")
	}
	if got.Comm != "rm" || !reflect.DeepEqual(got.Args, []string{"-rf", "/tmp/demo"}) {
		t.Fatalf("candidate command = %q %#v", got.Comm, got.Args)
	}
	if got.Category != "FILE_DELETE" || got.Source != "memory" {
		t.Fatalf("candidate metadata = category %q source %q", got.Category, got.Source)
	}
}

// ---- merged from contextevent_test.go ----

func TestBuildProcessContextFromRegisterDefaultsRootPID(t *testing.T) {
	ctx := buildProcessContextFromRegister(registerPayload{
		PID:        321,
		ToolName:   "codex",
		AgentRunID: "run-1",
	})
	if ctx.RootAgentPid != 321 {
		t.Fatalf("RootAgentPid = %d, want 321", ctx.RootAgentPid)
	}
	if ctx.ToolName != "codex" {
		t.Fatalf("ToolName = %q, want codex", ctx.ToolName)
	}
	if ctx.ArgvDigest == "" {
		t.Fatal("ArgvDigest should be populated")
	}
}

func TestEnrichEventContextInheritsFromParentPID(t *testing.T) {
	trackedProcessContexts = newProcessContextStore()
	trackedProcessContexts.Set(100, processContext{
		RootAgentPid: 100,
		AgentRunID:   "run-42",
		ToolCallID:   "tool-7",
		TraceID:      "trace-9",
	})

	event := &pb.Event{Pid: 101, Ppid: 100, Type: "execve"}
	enrichEventContext(event)

	if event.RootAgentPid != 100 {
		t.Fatalf("RootAgentPid = %d, want 100", event.RootAgentPid)
	}
	if event.AgentRunId != "run-42" {
		t.Fatalf("AgentRunId = %q, want run-42", event.AgentRunId)
	}
	if event.ToolCallId != "tool-7" {
		t.Fatalf("ToolCallId = %q, want tool-7", event.ToolCallId)
	}
	if event.TraceId != "trace-9" {
		t.Fatalf("TraceId = %q, want trace-9", event.TraceId)
	}
	if _, ok := trackedProcessContexts.Get(101); !ok {
		t.Fatal("expected child PID context to be cached after enrichment")
	}
}

func TestEnrichEventContextMovesExecContext(t *testing.T) {
	trackedProcessContexts = newProcessContextStore()
	trackedProcessContexts.Set(200, processContext{RootAgentPid: 200, AgentRunID: "run-exec"})

	event := &pb.Event{Pid: 201, Type: "process_exec", ExtraInfo: "old_pid=200"}
	enrichEventContext(event)

	if event.AgentRunId != "run-exec" {
		t.Fatalf("AgentRunId = %q, want run-exec", event.AgentRunId)
	}
	if _, ok := trackedProcessContexts.Get(200); ok {
		t.Fatal("old pid context should be moved away")
	}
	if _, ok := trackedProcessContexts.Get(201); !ok {
		t.Fatal("new pid context should exist after exec move")
	}
}

// ---- merged from datasetmanagement_test.go ----

func TestTrainingDataStoreClearResetsSamples(t *testing.T) {
	store := newTrainingDataStore(8)
	store.Add(TrainingSample{
		Comm:      "echo",
		Args:      []string{"hello"},
		Timestamp: time.Unix(1700000000, 0).UTC(),
	})
	store.Add(TrainingSample{
		Comm:      "rm",
		Args:      []string{"-rf", "/tmp/demo"},
		Timestamp: time.Unix(1700000001, 0).UTC(),
	})

	total, labeled := store.Status()
	if total != 2 || labeled != 0 {
		t.Fatalf("status before clear = %d/%d, want 2/0", total, labeled)
	}

	cleared := store.Clear()
	if cleared != 2 {
		t.Fatalf("Clear() = %d, want 2", cleared)
	}

	total, labeled = store.Status()
	if total != 0 || labeled != 0 {
		t.Fatalf("status after clear = %d/%d, want 0/0", total, labeled)
	}

	if samples := store.AllSamples(); len(samples) != 0 {
		t.Fatalf("AllSamples() after clear = %d, want 0", len(samples))
	}
}

func TestPullRemoteDatasetFromContentSupportsImportAll(t *testing.T) {
	raw := []byte(`{
		"rows": [
			{"commandLine": "rm -rf /tmp/demo", "label": "BLOCK"},
			{"commandLine": "echo hello", "label": "ALLOW"}
		]
	}`)

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		Content:    string(raw),
		SourceName: "export.json",
		Format:     "auto",
		Limit:      1,
		LabelMode:  "preserve",
		ImportAll:  true,
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Source != "export.json" {
		t.Fatalf("Source = %q, want export.json", resp.Source)
	}
	if resp.Format != "json" {
		t.Fatalf("Format = %q, want json", resp.Format)
	}
	if resp.ContentType != "application/json" {
		t.Fatalf("ContentType = %q, want application/json", resp.ContentType)
	}
	if resp.Total != 2 {
		t.Fatalf("Total = %d, want 2", resp.Total)
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("Rows length = %d, want 2", len(resp.Rows))
	}
	if resp.Truncated {
		t.Fatalf("Truncated = true, want false for ImportAll")
	}
	if resp.Rows[0].Label != "BLOCK" || resp.Rows[1].Label != "ALLOW" {
		t.Fatalf("rows labels = %#v %#v", resp.Rows[0].Label, resp.Rows[1].Label)
	}
}

func TestHandleMLDatasetExportAndClear(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newTrainingDataStore(8)
	tmpDir := t.TempDir()
	globalTrainingStore.dataDir = tmpDir
	globalTrainingStore.persistPath = filepath.Join(tmpDir, "ml_training_data.bin")
	t.Cleanup(func() {
		globalTrainingStore = oldStore
	})

	globalTrainingStore.Add(TrainingSample{
		Label:        1,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		UserLabel:    "manual",
	})

	exportRec := httptest.NewRecorder()
	exportCtx, _ := gin.CreateTestContext(exportRec)
	exportCtx.Request = httptest.NewRequest(http.MethodGet, "/config/ml/datasets/export", nil)
	handleMLDatasetExportGet(exportCtx)

	if exportRec.Code != http.StatusOK {
		t.Fatalf("export status = %d, want %d", exportRec.Code, http.StatusOK)
	}
	var exportResp remoteDatasetResponse
	if err := json.Unmarshal(exportRec.Body.Bytes(), &exportResp); err != nil {
		t.Fatalf("unmarshal export response: %v", err)
	}
	if exportResp.Source != "local-training-store" {
		t.Fatalf("export source = %q, want local-training-store", exportResp.Source)
	}
	if exportResp.Total != 1 || len(exportResp.Rows) != 1 {
		t.Fatalf("export rows = %d/%d, want 1/1", len(exportResp.Rows), exportResp.Total)
	}
	if exportResp.Rows[0].CommandLine != "rm -rf /tmp/demo" || exportResp.Rows[0].Label != "BLOCK" {
		t.Fatalf("export row = %#v", exportResp.Rows[0])
	}

	clearRec := httptest.NewRecorder()
	clearCtx, _ := gin.CreateTestContext(clearRec)
	clearCtx.Request = httptest.NewRequest(http.MethodDelete, "/config/ml/datasets", nil)
	handleMLDatasetClearDelete(clearCtx)

	if clearRec.Code != http.StatusOK {
		t.Fatalf("clear status = %d, body = %s", clearRec.Code, clearRec.Body.String())
	}
	if total, labeled := globalTrainingStore.Status(); total != 0 || labeled != 0 {
		t.Fatalf("store status after clear = %d/%d, want 0/0", total, labeled)
	}
}

func TestHandleMLSamplesPostPreservesCommandLine(t *testing.T) {
	oldStore := globalTrainingStore
	globalTrainingStore = newTrainingDataStore(8)
	t.Cleanup(func() {
		globalTrainingStore = oldStore
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/config/ml/samples", strings.NewReader(`{
		"commandLine": "bash -c \"rm -rf /tmp/demo\"",
		"label": "BLOCK"
	}`))
	handleMLSamplesPost(ctx)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	items := globalTrainingStore.AllSamplesWithIndex()
	if len(items) != 1 {
		t.Fatalf("sample count = %d, want 1", len(items))
	}
	if got := items[0].Sample.CommandLine; got != `bash -c "rm -rf /tmp/demo"` {
		t.Fatalf("stored commandLine = %q", got)
	}

	exportRec := httptest.NewRecorder()
	exportCtx, _ := gin.CreateTestContext(exportRec)
	exportCtx.Request = httptest.NewRequest(http.MethodGet, "/config/ml/samples", nil)
	handleMLSamplesGet(exportCtx)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("export status = %d, body = %s", exportRec.Code, exportRec.Body.String())
	}
	var payload struct {
		Samples []struct {
			CommandLine string `json:"commandLine"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(exportRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if len(payload.Samples) != 1 || payload.Samples[0].CommandLine != `bash -c "rm -rf /tmp/demo"` {
		t.Fatalf("response samples = %#v", payload.Samples)
	}
}

func TestTrainingDataStorePersistenceRestoresArgs(t *testing.T) {
	store := newTrainingDataStore(8)
	tmpDir := t.TempDir()
	store.dataDir = tmpDir
	store.persistPath = filepath.Join(tmpDir, "ml_training_data.bin")
	store.Add(TrainingSample{
		Label:        1,
		CommandLine:  `bash -c "rm -rf /tmp/demo"`,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		UserLabel:    "manual",
	})
	if err := store.Flush(); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	loaded := newTrainingDataStore(8)
	loaded.dataDir = tmpDir
	loaded.persistPath = filepath.Join(tmpDir, "ml_training_data.bin")
	if err := loaded.loadFromDisk(); err != nil {
		t.Fatalf("loadFromDisk() error = %v", err)
	}

	items := loaded.AllSamplesWithIndex()
	if len(items) != 1 {
		t.Fatalf("loaded sample count = %d, want 1", len(items))
	}
	gotArgs := items[0].Sample.Args
	wantArgs := []string{"-rf", "/tmp/demo"}
	if len(gotArgs) != len(wantArgs) {
		t.Fatalf("loaded args = %#v, want %#v", gotArgs, wantArgs)
	}
	for i := range wantArgs {
		if gotArgs[i] != wantArgs[i] {
			t.Fatalf("loaded args[%d] = %q, want %q", i, gotArgs[i], wantArgs[i])
		}
	}
	if got := items[0].Sample.CommandLine; got != `bash -c "rm -rf /tmp/demo"` {
		t.Fatalf("loaded commandLine = %q, want raw commandLine", got)
	}
}

func TestBuildLLMProductionDatasetCleansTrainingSamples(t *testing.T) {
	oldStore := globalTrainingStore
	oldConfig := mlConfig
	globalTrainingStore = newTrainingDataStore(8)
	mlConfig.LlmSystemPrompt = "SYSTEM PROMPT"
	t.Cleanup(func() {
		globalTrainingStore = oldStore
		mlConfig = oldConfig
	})

	globalTrainingStore.Add(TrainingSample{
		Label:        1,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000000, 0).UTC(),
		UserLabel:    "manual",
	})
	globalTrainingStore.Add(TrainingSample{
		Label:        1,
		Comm:         "rm",
		Args:         []string{"-rf", "/tmp/demo"},
		Category:     "FILE_DELETE",
		AnomalyScore: 0.82,
		Timestamp:    time.Unix(1700000001, 0).UTC(),
		UserLabel:    "manual",
	})
	globalTrainingStore.Add(TrainingSample{
		Label:        3,
		Comm:         "git",
		Args:         []string{"status"},
		Category:     "SAFE",
		AnomalyScore: 0.12,
		Timestamp:    time.Unix(1700000002, 0).UTC(),
		UserLabel:    "remote-heuristic",
	})
	globalTrainingStore.Add(TrainingSample{
		Label:        -1,
		Comm:         "echo",
		Args:         []string{"hello"},
		Category:     "SAFE",
		AnomalyScore: 0.01,
		Timestamp:    time.Unix(1700000003, 0).UTC(),
		UserLabel:    "remote-import",
	})

	resp, err := buildLLMProductionDataset(llmProductionDatasetRequest{
		Limit:          10,
		AllowHeuristic: false,
		Deduplicate:    true,
	})
	if err != nil {
		t.Fatalf("buildLLMProductionDataset() error = %v", err)
	}
	if resp.Source != "local-training-store" {
		t.Fatalf("Source = %q, want local-training-store", resp.Source)
	}
	if resp.Included != 1 {
		t.Fatalf("Included = %d, want 1", resp.Included)
	}
	if resp.SkippedDuplicates != 1 {
		t.Fatalf("SkippedDuplicates = %d, want 1", resp.SkippedDuplicates)
	}
	if resp.SkippedHeuristic != 1 {
		t.Fatalf("SkippedHeuristic = %d, want 1", resp.SkippedHeuristic)
	}
	if resp.SkippedUnlabeled != 1 {
		t.Fatalf("SkippedUnlabeled = %d, want 1", resp.SkippedUnlabeled)
	}
	if len(resp.Rows) != 1 {
		t.Fatalf("Rows length = %d, want 1", len(resp.Rows))
	}

	row := resp.Rows[0]
	if row.Label != "BLOCK" {
		t.Fatalf("row.Label = %q, want BLOCK", row.Label)
	}
	if row.Messages[0].Content != "SYSTEM PROMPT" {
		t.Fatalf("system prompt = %q, want SYSTEM PROMPT", row.Messages[0].Content)
	}
	if row.Messages[1].Role != "user" || row.Messages[2].Role != "assistant" {
		t.Fatalf("unexpected message roles = %#v", row.Messages)
	}
	if row.TargetRiskScore != 95 {
		t.Fatalf("TargetRiskScore = %v, want 95", row.TargetRiskScore)
	}
	if row.TargetConfidence < 0.98 {
		t.Fatalf("TargetConfidence = %v, want >= 0.98", row.TargetConfidence)
	}
	if row.Completion == "" || row.Prompt == "" {
		t.Fatalf("prompt/completion should not be empty: %#v", row)
	}
}

// ---- merged from detectionprotocol_test.go ----

func TestExtractDNSQueries(t *testing.T) {
	query := []byte{
		0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
	}
	queries := extractDNSQueries(query)
	if len(queries) != 1 || queries[0] != "example.com" {
		t.Fatalf("queries = %#v, want example.com", queries)
	}
	entry := detectAndRecordProtocol("8.8.8.8", 53, query)
	if entry == nil || entry.AppProtocol != AppProtoDNS || entry.HTTPHost != "example.com" {
		t.Fatalf("entry = %#v, want DNS example.com", entry)
	}
}

func TestCorrelateDNSResponseRecordsAnswers(t *testing.T) {
	orig := dnsCorrelation
	dnsCorrelation = newDNSCache()
	defer func() { dnsCorrelation = orig }()

	response := []byte{
		0x12, 0x34, 0x81, 0x80, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
		0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x3c,
		0x00, 0x04, 93, 184, 216, 34,
	}
	correlateDNSResponse("8.8.8.8", response)
	domain, ok := dnsCorrelation.LookupIP("93.184.216.34")
	if !ok || domain != "example.com" {
		t.Fatalf("LookupIP = %q %v, want example.com true", domain, ok)
	}
	ip, ok := dnsCorrelation.LookupDomain("example.com")
	if !ok || ip != "93.184.216.34" {
		t.Fatalf("LookupDomain = %q %v, want 93.184.216.34 true", ip, ok)
	}
}

func TestFingerprintQUIC(t *testing.T) {
	quicInitial := []byte{
		0xc0, 0x00, 0x00, 0x00, 0x01,
		0x08, 1, 2, 3, 4, 5, 6, 7, 8,
		0x00,
	}
	if proto := fingerprintProtocol(quicInitial, 443); proto != AppProtoQUIC {
		t.Fatalf("fingerprint = %q, want QUIC", proto)
	}
}

func TestFingerprintNTP(t *testing.T) {
	ntp := make([]byte, 48)
	ntp[0] = 0x23
	if proto := fingerprintProtocol(ntp, 123); proto != AppProtoNTP {
		t.Fatalf("fingerprint NTP = %q, want NTP", proto)
	}
	ver, stratum := extractNTPInfo(ntp)
	if ver == "" || stratum == "" {
		t.Fatalf("extractNTPInfo empty")
	}
}

func TestFingerprintSNMP(t *testing.T) {
	snmp := []byte{
		0x30, 0x26, 0x02, 0x01, 0x01,
		0x04, 0x06, 'p', 'u', 'b', 'l', 'i', 'c',
	}
	if proto := fingerprintProtocol(snmp, 161); proto != AppProtoSNMP {
		t.Fatalf("fingerprint SNMP = %q", proto)
	}
	ver, comm := extractSNMPInfo(snmp)
	if ver != "SNMPv2c" || comm != "public" {
		t.Fatalf("extractSNMPInfo = %q %q", ver, comm)
	}
}

func TestFingerprintSSDP(t *testing.T) {
	ssdp := []byte("NOTIFY * HTTP/1.1\r\nHost: 239.255.255.250:1900\r\n")
	if proto := fingerprintProtocol(ssdp, 1900); proto != AppProtoSSDP {
		t.Fatalf("fingerprint SSDP = %q", proto)
	}
}

func TestFingerprintNetBIOS(t *testing.T) {
	nbns := make([]byte, 50)
	nbns[2], nbns[3] = 0x00, 0x00
	nbns[4], nbns[5] = 0x00, 0x01
	nbns[12] = 0x20
	nameBytes := []byte("WORKGROUP")
	for i := 0; i < 16; i++ {
		c := byte('A')
		if i < len(nameBytes) {
			c = nameBytes[i]
		}
		nbns[13+i*2] = 'A' + (c>>4)&0x0f
		nbns[13+i*2+1] = 'A' + c&0x0f
	}
	if proto := fingerprintProtocol(nbns, 137); proto != AppProtoNetBIOS {
		t.Fatalf("fingerprint NetBIOS = %q", proto)
	}
}

func TestFingerprintLLMNR(t *testing.T) {
	llmnr := make([]byte, 12)
	llmnr[4], llmnr[5] = 0x00, 0x01
	if proto := fingerprintProtocol(llmnr, 5355); proto != AppProtoLLMNR {
		t.Fatalf("fingerprint LLMNR = %q", proto)
	}
}

func TestQUICVersion(t *testing.T) {
	v1 := extractQUICVersion([]byte{0xc0, 0x00, 0x00, 0x00, 0x01})
	if v1 != "QUIC v1" {
		t.Fatalf("QUIC version = %q", v1)
	}
}

// ---- merged from enrichmenthandlers_test.go ----

func TestHandleNetworkFlowsSupportsFilterAndFlowDetail(t *testing.T) {
	orig := networkFlowAggregator
	networkFlowAggregator = newFlowAggregator()
	defer func() { networkFlowAggregator = orig }()

	networkFlowAggregator.RecordConnectionContext("10.0.0.2", "93.184.216.34", 42000, 443, "TCP", "curl", 123, "outgoing", "ESTABLISHED", &pb.Event{
		Pid:        123,
		Comm:       "curl",
		NetBytes:   512,
		AgentRunId: "run-flow-test",
		ToolCallId: "tool-flow-test",
	})
	networkFlowAggregator.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 42000, 443, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoTLS,
		SNI:         "api.example.com",
		ALPN:        "h2",
	})

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/network/flows", handleNetworkFlows)
	router.GET("/network/flows/:flowID", handleNetworkFlowByID)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/network/flows?filter=process:curl+sni:api.example.com&showHistoric=true&limit=10", nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flows status = %d body=%s", rec.Code, rec.Body.String())
	}
	var list networkFlowQueryResult
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode flows response: %v", err)
	}
	if list.Total != 1 || len(list.Flows) != 1 {
		t.Fatalf("flows total=%d len=%d, want 1", list.Total, len(list.Flows))
	}
	flow := list.Flows[0]
	if flow.SNI != "api.example.com" || flow.TLSALPN != "h2" || len(flow.AgentRunIDs) != 1 || flow.AgentRunIDs[0] != "run-flow-test" {
		t.Fatalf("flow metadata = sni=%q alpn=%q agents=%#v", flow.SNI, flow.TLSALPN, flow.AgentRunIDs)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/network/flows/"+flow.FlowID, nil)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("flow detail status = %d body=%s", rec.Code, rec.Body.String())
	}
	var detail NetworkFlowSummary
	if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detail.FlowID != flow.FlowID || detail.SNI != "api.example.com" {
		t.Fatalf("detail = id %q sni %q, want %q/api.example.com", detail.FlowID, detail.SNI, flow.FlowID)
	}
}

func TestHandleNetworkFlowJSONLExportIncludesAttributionAndDPI(t *testing.T) {
	orig := networkFlowAggregator
	networkFlowAggregator = newFlowAggregator()
	defer func() { networkFlowAggregator = orig }()

	networkFlowAggregator.RecordConnectionContext("10.0.0.2", "93.184.216.34", 42000, 80, "TCP", "curl", 123, "outgoing", "ESTABLISHED", &pb.Event{
		Pid:        123,
		Comm:       "curl",
		NetBytes:   256,
		AgentRunId: "run-jsonl",
		ToolCallId: "tool-jsonl",
	})
	networkFlowAggregator.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 42000, 80, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoHTTP,
		HTTPHost:    "example.com",
		HTTPMethod:  "GET",
	})

	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/network/export/jsonl?filter=host:example.com&showHistoric=true", nil)
	handleNetworkFlowJSONLExport(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("jsonl status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/x-ndjson") {
		t.Fatalf("Content-Type = %q, want application/x-ndjson", got)
	}
	scanner := bufio.NewScanner(strings.NewReader(rec.Body.String()))
	if !scanner.Scan() {
		t.Fatalf("expected one JSONL row, body=%q", rec.Body.String())
	}
	var row NetworkFlowSummary
	if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
		t.Fatalf("decode jsonl row: %v", err)
	}
	if row.HTTPHost != "example.com" || row.HTTPMethod != "GET" || len(row.AgentRunIDs) != 1 || row.AgentRunIDs[0] != "run-jsonl" {
		t.Fatalf("jsonl row metadata = host=%q method=%q agents=%#v", row.HTTPHost, row.HTTPMethod, row.AgentRunIDs)
	}
	if scanner.Scan() {
		t.Fatalf("expected one JSONL row, got extra %q", scanner.Text())
	}
}

func TestHandleDNSCacheReturnsEntries(t *testing.T) {
	orig := dnsCorrelation
	dnsCorrelation = newDNSCache()
	defer func() { dnsCorrelation = orig }()
	dnsCorrelation.Record("example.com", "93.184.216.34")

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/network/dns-cache", nil)
	handleDNSCache(ctx)
	if rec.Code != http.StatusOK {
		t.Fatalf("dns cache status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "example.com") || !strings.Contains(rec.Body.String(), "93.184.216.34") {
		t.Fatalf("dns cache body missing entry: %s", rec.Body.String())
	}
}

// ---- merged from envelopeevent_test.go ----

func TestNormalizeCapturedEventRecordBuildsWrapperEnvelope(t *testing.T) {
	record := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.Unix(1710000000, 123).UTC(),
		Event: &pb.Event{
			Pid:        321,
			Ppid:       123,
			Type:       "wrapper_intercept",
			EventType:  pb.EventType_WRAPPER_INTERCEPT,
			Comm:       "git",
			Path:       "git status --short",
			AgentRunId: "run-1",
			TaskId:     "task-7",
			ToolCallId: "tool-3",
			ToolName:   "bash",
			TraceId:    "trace-8",
			Decision:   "ALERT",
			RiskScore:  88,
			Cwd:        "/workspace/demo",
		},
	})

	if record.Envelope == nil {
		t.Fatal("expected envelope to be populated")
	}
	if record.Envelope.GetSchemaVersion() != eventEnvelopeSchemaVersion {
		t.Fatalf("schema version = %q, want %q", record.Envelope.GetSchemaVersion(), eventEnvelopeSchemaVersion)
	}
	if record.Envelope.GetSource() != "wrapper" {
		t.Fatalf("source = %q, want wrapper", record.Envelope.GetSource())
	}
	if record.Envelope.GetTaskId() != "task-7" {
		t.Fatalf("task id = %q, want task-7", record.Envelope.GetTaskId())
	}
	if record.Envelope.GetCwd() != "/workspace/demo" {
		t.Fatalf("cwd = %q, want /workspace/demo", record.Envelope.GetCwd())
	}
	if record.Envelope.GetEventId() == "" {
		t.Fatal("expected deterministic event id")
	}
	wrapperPayload := record.Envelope.GetWrapperEvent()
	if wrapperPayload == nil {
		t.Fatal("expected wrapper payload")
	}
	if wrapperPayload.GetCommandLine() != "git status --short" {
		t.Fatalf("command line = %q, want git status --short", wrapperPayload.GetCommandLine())
	}
	if wrapperPayload.GetToolName() != "bash" {
		t.Fatalf("tool name = %q, want bash", wrapperPayload.GetToolName())
	}
}

func TestBuildCapturedEventJSONRecordsIncludesEnvelope(t *testing.T) {
	records := buildCapturedEventJSONRecords([]CapturedEventRecord{{
		ReceivedAt: time.Unix(1710000001, 0).UTC(),
		Event: &pb.Event{
			Pid:        99,
			Type:       "openat",
			EventType:  pb.EventType_OPENAT,
			Comm:       "python",
			Path:       "/workspace/app.py",
			AgentRunId: "run-json",
			TaskId:     "task-json",
			Cwd:        "/workspace",
		},
	}})

	if len(records) != 1 {
		t.Fatalf("json record count = %d, want 1", len(records))
	}
	envelope, ok := records[0]["Envelope"].(map[string]any)
	if !ok || envelope == nil {
		t.Fatalf("expected JSON envelope map, got %#v", records[0]["Envelope"])
	}
	if envelope["task_id"] != "task-json" {
		t.Fatalf("json task_id = %#v, want task-json", envelope["task_id"])
	}
	if envelope["cwd"] != "/workspace" {
		t.Fatalf("json cwd = %#v, want /workspace", envelope["cwd"])
	}
	filePayload, ok := envelope["file_event"].(map[string]any)
	if !ok || filePayload == nil {
		t.Fatalf("expected file_event payload, got %#v", envelope["file_event"])
	}
	if filePayload["path"] != "/workspace/app.py" {
		t.Fatalf("file_event.path = %#v, want /workspace/app.py", filePayload["path"])
	}
}

// ---- merged from eventsnetwork_test.go ----

func TestKernelEventTypeNameMatchesProtoNetworkEnums(t *testing.T) {
	tests := map[pb.EventType]string{
		pb.EventType_SEMANTIC_ALERT:   "semantic_alert",
		pb.EventType_TCP_CONNECT:      "tcp_connect",
		pb.EventType_TCP_CLOSE:        "tcp_close",
		pb.EventType_TCP_STATE_CHANGE: "tcp_state_change",
		pb.EventType_DNS_QUERY:        "dns_query",
	}
	for eventType, want := range tests {
		if got := kernelEventTypeName(uint32(eventType)); got != want {
			t.Fatalf("kernelEventTypeName(%d) = %q, want %q", eventType, got, want)
		}
	}
}

func TestFlowEventsAreNetworkEvents(t *testing.T) {
	for _, eventType := range []string{"tcp_connect", "tcp_close", "tcp_state_change", "dns_query"} {
		if !isNetworkEventType(eventType) {
			t.Fatalf("%s should be classified as a network event", eventType)
		}
	}
}

func TestBuildKernelEventRecordsUDPFlow(t *testing.T) {
	orig := networkFlowAggregator
	networkFlowAggregator = newFlowAggregator()
	defer func() { networkFlowAggregator = orig }()

	event := bpfEvent{
		PID:          42,
		Type:         uint32(pb.EventType_NETWORK_SENDTO),
		NetFamily:    2,
		NetDirection: 1,
		NetBytes:     29,
		NetPort:      53,
	}
	copy(event.Comm[:], []byte("dig"))
	copy(event.NetAddr[:4], []byte{8, 8, 8, 8})
	copy(event.Extra4[:], []byte{
		0x12, 0x34, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm', 0x00,
		0x00, 0x01, 0x00, 0x01,
	})

	out := buildKernelEvent(event)
	if out == nil || out.NetEndpoint != "8.8.8.8:53" {
		t.Fatalf("NetEndpoint = %q, want 8.8.8.8:53", out.GetNetEndpoint())
	}
	if out.GetFlowId() != "UDP:local:0->8.8.8.8:53" || out.GetTransport() != "UDP" || out.GetDnsName() != "example.com" {
		t.Fatalf("proto flow fields = id %q transport %q dns %q", out.GetFlowId(), out.GetTransport(), out.GetDnsName())
	}
	result := networkFlowAggregator.Query(networkFlowQuery{ShowHistoric: true, Filter: "proto:udp host:example.com", Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("UDP flow query total=%d len=%d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.Protocol != "UDP" || flow.DstIP != "8.8.8.8" || flow.DstPort != 53 || flow.DNSName != "example.com" {
		t.Fatalf("flow = protocol %q dst %s:%d dns %q", flow.Protocol, flow.DstIP, flow.DstPort, flow.DNSName)
	}
}

func TestBuildKernelEventRecordsGenericTCPConnectFlowAndState(t *testing.T) {
	origAgg := networkFlowAggregator
	origTCP := tcpTracker
	networkFlowAggregator = newFlowAggregator()
	tcpTracker = newTCPStateTracker()
	defer func() {
		networkFlowAggregator = origAgg
		tcpTracker = origTCP
	}()

	event := bpfEvent{
		PID:          42,
		Type:         uint32(pb.EventType_NETWORK_CONNECT),
		TagID:        7,
		NetFamily:    2,
		NetDirection: 1,
		NetPort:      443,
		Retval:       0,
	}
	copy(event.Comm[:], []byte("curl"))
	copy(event.NetAddr[:4], []byte{93, 184, 216, 34})

	out := buildKernelEvent(event)
	if out == nil || out.GetFlowId() != "TCP:local:0->93.184.216.34:443" {
		t.Fatalf("FlowId = %q, want generic TCP connect flow", out.GetFlowId())
	}

	result := networkFlowAggregator.Query(networkFlowQuery{ShowHistoric: true, Filter: "proto:tcp dport:443", Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("TCP flow query total=%d len=%d, want 1", result.Total, len(result.Flows))
	}
	if got := result.Flows[0].State; got != "ESTABLISHED" {
		t.Fatalf("flow state = %q, want ESTABLISHED", got)
	}

	conns := tcpTracker.Snapshot()
	if len(conns) != 1 || conns[0].State != TCPStateEstablished {
		t.Fatalf("tcp conns = %#v, want one ESTABLISHED connection", conns)
	}
}

func TestBuildKernelEventCopiesDurationNs(t *testing.T) {
	event := bpfEvent{
		PID:        42,
		PPID:       7,
		UID:        1000,
		GID:        1001,
		Type:       25,
		TagID:      0,
		Retval:     -1,
		Extra1:     62,
		Extra2:     9,
		CgroupID:   123456,
		DurationNs: 987654321,
	}

	out := buildKernelEvent(event)
	if out == nil {
		t.Fatal("buildKernelEvent returned nil")
	}
	if out.DurationNs != event.DurationNs {
		t.Fatalf("DurationNs = %d, want %d", out.DurationNs, event.DurationNs)
	}
	if out.Gid != event.GID {
		t.Fatalf("Gid = %d, want %d", out.Gid, event.GID)
	}
	if out.CgroupId != event.CgroupID {
		t.Fatalf("CgroupId = %d, want %d", out.CgroupId, event.CgroupID)
	}
	if !strings.Contains(out.ExtraInfo, "kill(62)") {
		t.Fatalf("ExtraInfo = %q, want syscall name and number", out.ExtraInfo)
	}
}

func TestBuildKernelEventProcessFork(t *testing.T) {
	event := bpfEvent{
		PID:    100,
		PPID:   50,
		Type:   26,
		Extra1: 101,
	}
	copy(event.Path[:], []byte("python3"))

	out := buildKernelEvent(event)
	if out == nil {
		t.Fatal("buildKernelEvent returned nil")
	}
	if out.Type != "process_fork" {
		t.Fatalf("Type = %q, want process_fork", out.Type)
	}
	if !strings.Contains(out.ExtraInfo, "child_pid=101") {
		t.Fatalf("ExtraInfo = %q, want child pid", out.ExtraInfo)
	}
}

// ---- merged from exportotel_test.go ----

type captureSpanExporter struct {
	mu    sync.Mutex
	names []string
}

func (c *captureSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, span := range spans {
		c.names = append(c.names, span.Name())
	}
	return nil
}

func (c *captureSpanExporter) Shutdown(context.Context) error {
	return nil
}

func (c *captureSpanExporter) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.names...)
}

func TestOTelExporterBuildsHierarchyAndChildSpans(t *testing.T) {
	state := newOTelExporterState()
	defer state.Close()

	exporter := &captureSpanExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	state.mu.Lock()
	state.enabled = true
	state.ready = true
	state.endpoint = "http://collector:4318"
	state.serviceName = "agent-ebpf-filter-test"
	state.tp = provider
	state.tracer = provider.Tracer("test")
	state.mu.Unlock()

	baseTime := time.Unix(1_700_000_000, 0).UTC()
	execRecord := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: baseTime,
		Event: &pb.Event{
			Pid:          321,
			Ppid:         320,
			Type:         "execve",
			Path:         "/usr/bin/git",
			Comm:         "git",
			ToolName:     "shell",
			ToolCallId:   "tool-1",
			TaskId:       "task-1",
			AgentRunId:   "run-1",
			TraceId:      "trace-1",
			RootAgentPid: 321,
			DurationNs:   12_000,
		},
	})
	state.handleRecord(execRecord)

	exitRecord := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: baseTime.Add(250 * time.Millisecond),
		Event: &pb.Event{
			Pid:          321,
			Ppid:         320,
			Type:         "process_exit",
			Comm:         "git",
			ToolName:     "shell",
			ToolCallId:   "tool-1",
			TaskId:       "task-1",
			AgentRunId:   "run-1",
			TraceId:      "trace-1",
			RootAgentPid: 321,
			ExtraInfo:    "status=0",
		},
	})
	state.handleRecord(exitRecord)
	state.endIdleSpans(baseTime.Add(2 * time.Minute))

	names := exporter.Names()
	for _, expected := range []string{"agent.run", "codex.task", "tool.call", "process.exec", "process.exit"} {
		if !slices.Contains(names, expected) {
			t.Fatalf("expected exported spans to include %q, got %v", expected, names)
		}
	}
}

func TestBuildOTLPHTTPOptionsSupportsExplicitPath(t *testing.T) {
	opts, err := buildOTLPHTTPOptions("https://collector.example.com/custom/traces", map[string]string{
		"Authorization": "Bearer token",
	})
	if err != nil {
		t.Fatalf("buildOTLPHTTPOptions() error = %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("expected options to be returned")
	}
}

// ---- merged from flowstore_test.go ----

func TestFlowAggregatorKeysByFiveTupleAndKeepsAgentContext(t *testing.T) {
	agg := newFlowAggregator()
	ev := &pb.Event{
		Pid:        123,
		Comm:       "curl",
		NetBytes:   256,
		AgentRunId: "run-1",
		TaskId:     "task-1",
		ToolCallId: "tool-1",
		TraceId:    "trace-1",
		SpanId:     "span-1",
		Decision:   "ALLOW",
	}
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", "curl", 123, "outgoing", "ESTABLISHED", ev)
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40001, 443, "TCP", "curl", 123, "outgoing", "ESTABLISHED", ev)

	result := agg.Query(networkFlowQuery{ShowHistoric: true, Limit: 10})
	if result.Total != 2 {
		t.Fatalf("Total = %d, want 2 distinct source-port flows", result.Total)
	}
	for _, flow := range result.Flows {
		if flow.FlowID == "" {
			t.Fatal("FlowID should be populated")
		}
		if len(flow.AgentRunIDs) != 1 || flow.AgentRunIDs[0] != "run-1" {
			t.Fatalf("AgentRunIDs = %#v, want run-1", flow.AgentRunIDs)
		}
	}
}

func TestFlowQueryFilterSortCursorAndHistoric(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", "curl", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "curl", NetBytes: 100})
	agg.RecordConnectionContext("10.0.0.2", "192.0.2.10", 40001, 22, "TCP", "ssh", 11, "outgoing", "CLOSED", &pb.Event{Pid: 11, Comm: "ssh", NetBytes: 200})

	activeOnly := agg.Query(networkFlowQuery{Limit: 10})
	if activeOnly.Total != 1 {
		t.Fatalf("active-only Total = %d, want 1", activeOnly.Total)
	}
	all := agg.Query(networkFlowQuery{ShowHistoric: true, Filter: "process:ssh state:closed", Sort: "risk", Limit: 1})
	if all.Total != 1 || len(all.Flows) != 1 {
		t.Fatalf("historic filtered result = total %d len %d, want 1", all.Total, len(all.Flows))
	}
	if !all.Flows[0].Historic {
		t.Fatal("closed flow should be marked historic")
	}
	page := agg.Query(networkFlowQuery{ShowHistoric: true, Limit: 1})
	if page.Total != 2 || page.NextCursor == "" || len(page.Flows) != 1 {
		t.Fatalf("page = total %d next %q len %d, want cursor page", page.Total, page.NextCursor, len(page.Flows))
	}
}

func TestFlowAggregatorAppliesProtocolMetadata(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 80, "TCP", "curl", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "curl", NetBytes: 128})
	agg.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 40000, 80, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoHTTP,
		HTTPHost:    "example.com",
		HTTPMethod:  "GET",
	})

	result := agg.Query(networkFlowQuery{ShowHistoric: true, Filter: "host:example.com", Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("filtered result = total %d len %d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.AppProtocol != "HTTP" || flow.HTTPHost != "example.com" || flow.HTTPMethod != "GET" || flow.DstDomain != "example.com" {
		t.Fatalf("flow protocol metadata = app=%q host=%q method=%q domain=%q", flow.AppProtocol, flow.HTTPHost, flow.HTTPMethod, flow.DstDomain)
	}
}

func TestFlowAggregatorAppliesTLSMetadata(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", "curl", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "curl", NetBytes: 128})
	agg.ApplyProtocolMetadata("10.0.0.2", "93.184.216.34", 40000, 443, "TCP", &protoDetectionEntry{
		AppProtocol: AppProtoTLS,
		SNI:         "api.example.com",
		ALPN:        "h2, http/1.1",
	})

	result := agg.Query(networkFlowQuery{ShowHistoric: true, Filter: "sni:api.example.com", Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("filtered result = total %d len %d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.AppProtocol != "TLS" || flow.SNI != "api.example.com" || flow.TLSALPN != "h2, http/1.1" || flow.DstDomain != "api.example.com" {
		t.Fatalf("flow TLS metadata = app=%q sni=%q alpn=%q domain=%q", flow.AppProtocol, flow.SNI, flow.TLSALPN, flow.DstDomain)
	}
}

func TestFlowRiskReasonsForSuspiciousEndpointAndVolume(t *testing.T) {
	agg := newFlowAggregator()
	agg.RecordConnectionContext("10.0.0.2", "203.0.113.10", 40000, 4444, "TCP", "nc", 10, "outgoing", "ESTABLISHED", &pb.Event{Pid: 10, Comm: "nc", NetBytes: 12 * 1024 * 1024})
	result := agg.Query(networkFlowQuery{ShowHistoric: true, Limit: 10})
	if result.Total != 1 || len(result.Flows) != 1 {
		t.Fatalf("result total=%d len=%d, want 1", result.Total, len(result.Flows))
	}
	flow := result.Flows[0]
	if flow.RiskScore < 0.80 || flow.RiskLevel != "high" {
		t.Fatalf("risk = %.2f/%s, want high >=0.80", flow.RiskScore, flow.RiskLevel)
	}
	joined := strings.Join(flow.RiskReasons, "; ")
	for _, want := range []string{"suspicious IP scope", "suspicious endpoint pattern", "large outbound volume"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("RiskReasons = %#v, missing %q", flow.RiskReasons, want)
		}
	}
}

// ---- merged from forwardproxydomain_test.go ----

func TestDomainForwardProxyRoutesByHost(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "host=%s path=%s query=%s forwarded=%s route=%s", r.Host, r.URL.Path, r.URL.RawQuery, r.Header.Get("X-Forwarded-Host"), r.Header.Get("X-Agent-Forward-Route"))
	}))
	defer upstream.Close()

	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes: []DomainForwardRoute{{
			Host:     "Example.TEST",
			Upstream: upstream.URL + "/base",
		}},
	})

	req := httptest.NewRequest(http.MethodGet, "http://example.test/hello?x=1", nil)
	req.Host = "example.test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		"host=example.test",
		"path=/base/hello",
		"query=x=1",
		"forwarded=example.test",
		"route=example.test",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response %q does not contain %q", body, want)
		}
	}
}

func TestDomainForwardProxyRejectsUnknownHostUnlessAllowed(t *testing.T) {
	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes:        []DomainForwardRoute{{Host: "known.test", Upstream: "http://127.0.0.1:1"}},
	})

	req := httptest.NewRequest(http.MethodGet, "http://unknown.test/", nil)
	req.Host = "unknown.test"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	if !strings.Contains(rec.Body.String(), "no forwarding route") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestDomainForwardProxyAllowAnyHostBuildsHostTarget(t *testing.T) {
	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		AllowAnyHost:  true,
		DefaultScheme: "https",
	})

	target, route, err := handler.targetForHost("Service.Example:443")
	if err != nil {
		t.Fatalf("targetForHost returned error: %v", err)
	}
	if got, want := target.String(), "https://service.example"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
	if route.Host != "service.example" {
		t.Fatalf("route host = %q", route.Host)
	}
}

func TestDomainForwardProxyWildcardAndHostPlaceholder(t *testing.T) {
	handler := newDomainForwardProxyHandler(DomainForwardProxySettings{
		DefaultScheme: "http",
		Routes: []DomainForwardRoute{{
			Host:     "*.example.test",
			Upstream: "http://upstream.internal/{host}",
		}},
	})

	target, route, err := handler.targetForHost("api.example.test")
	if err != nil {
		t.Fatalf("targetForHost returned error: %v", err)
	}
	if route.Host != "*.example.test" {
		t.Fatalf("route host = %q", route.Host)
	}
	if got, want := target.String(), "http://upstream.internal/api.example.test"; got != want {
		t.Fatalf("target = %q, want %q", got, want)
	}
}

func TestNormalizeDomainForwardProxySettingsDefaults(t *testing.T) {
	settings := DomainForwardProxySettings{
		HTTPPort:           -1,
		HTTPSPort:          70000,
		DefaultScheme:      "ftp",
		DNSResolver:        "1.1.1.1",
		DialTimeoutSeconds: 999,
		Routes: []DomainForwardRoute{
			{Host: "Example.Test:8443", Upstream: " backend:8080 "},
			{Host: "example.test", Upstream: "duplicate"},
			{Host: ""},
		},
	}
	normalizeDomainForwardProxySettings(&settings)

	if settings.HTTPPort != 80 || settings.HTTPSPort != 443 {
		t.Fatalf("ports = %d/%d, want 80/443", settings.HTTPPort, settings.HTTPSPort)
	}
	if settings.DefaultScheme != "https" {
		t.Fatalf("default scheme = %q", settings.DefaultScheme)
	}
	if settings.DNSResolver != "1.1.1.1:53" {
		t.Fatalf("dns resolver = %q", settings.DNSResolver)
	}
	if settings.DialTimeoutSeconds != 120 {
		t.Fatalf("dial timeout = %d", settings.DialTimeoutSeconds)
	}
	if len(settings.Routes) != 1 {
		t.Fatalf("route count = %d", len(settings.Routes))
	}
	if settings.Routes[0].Host != "example.test" || settings.Routes[0].Upstream != "backend:8080" {
		t.Fatalf("route = %+v", settings.Routes[0])
	}
}

func TestDomainForwardProxyStatusEndpointReturnsCopy(t *testing.T) {
	oldService := domainForwardProxyService
	service := newDomainForwardProxyRuntime()
	domainForwardProxyService = service
	t.Cleanup(func() { domainForwardProxyService = oldService })

	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest(http.MethodGet, "/system/domain-forward/status", nil)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = req
	handleDomainForwardProxyStatus(c)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// ---- merged from fragmentassemblertls_test.go ----

func newTestTLSFragment(index, count int, totalLen int, data string) tlsFragment {
	return newTestTLSFragmentAt(index, count, totalLen, data, uint64(time.Now().UnixNano()))
}

func newTestTLSFragmentAt(index, count int, totalLen int, data string, timestampNS uint64) tlsFragment {
	var frag tlsFragment
	frag.TimestampNS = timestampNS
	frag.PID = 1234
	frag.TGID = 5678
	frag.DataLen = uint32(len(data))
	frag.TotalLen = uint32(totalLen)
	frag.OriginalLen = uint32(totalLen)
	frag.FragIndex = uint16(index)
	frag.FragCount = uint16(count)
	frag.LibType = tlsLibOpenSSL
	frag.Direction = tlsDirectionSend
	copy(frag.Comm[:], []byte("curl"))
	copy(frag.Data[:], []byte(data))
	return frag
}

func TestFragmentAssemblerReassemblesOutOfOrderFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)

	timestamp := uint64(time.Now().UnixNano())
	frags := []tlsFragment{
		newTestTLSFragmentAt(1, 3, 12, "fghij", timestamp),
		newTestTLSFragmentAt(0, 3, 12, "abcde", timestamp),
		newTestTLSFragmentAt(2, 3, 12, "kl", timestamp),
	}

	var completed *completedTLSFragment
	var ok bool
	for _, frag := range frags {
		completed, ok = assembler.Add(frag)
	}

	if !ok {
		t.Fatalf("expected completed fragment")
	}
	if completed == nil {
		t.Fatalf("expected completed fragment payload")
	}
	if got := string(completed.Payload); got != "abcdefghijkl" {
		t.Fatalf("unexpected payload: %q", got)
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments, got %d", got)
	}
}

func TestFragmentAssemblerCleansExpiredPendingBuffers(t *testing.T) {
	assembler := NewFragmentAssembler(time.Millisecond)
	frag := newTestTLSFragment(0, 2, 10, "abcde")
	frag.TimestampNS = uint64(time.Now().Add(-time.Second).UnixNano())

	if completed, ok := assembler.Add(frag); ok || completed != nil {
		t.Fatalf("expected incomplete fragment to stay pending")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment, got %d", got)
	}
	if cleaned := assembler.CleanupExpired(time.Now()); cleaned != 1 {
		t.Fatalf("expected one expired fragment cleaned, got %d", cleaned)
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments after cleanup, got %d", got)
	}
}

func TestFragmentAssemblerRejectsInvalidFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	frag := newTestTLSFragment(0, 0, 10, "abcde")

	if completed, ok := assembler.Add(frag); ok || completed != nil {
		t.Fatalf("expected invalid fragment to be rejected")
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments, got %d", got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment, got %d", got)
	}
}

func TestFragmentAssemblerDropsDuplicateFragmentIndexWithoutOverwriting(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	frag0 := newTestTLSFragmentAt(0, 2, 10, "abcde", timestamp)
	frag1 := newTestTLSFragmentAt(0, 2, 10, "vwxyz", timestamp)
	frag2 := newTestTLSFragmentAt(1, 2, 10, "fghij", timestamp)

	if completed, ok := assembler.Add(frag0); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment, got %d", got)
	}

	if completed, ok := assembler.Add(frag1); ok || completed != nil {
		t.Fatalf("expected duplicate fragment index to be rejected")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment after duplicate, got %d", got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment after duplicate, got %d", got)
	}

	if completed, ok := assembler.Add(frag2); !ok || completed == nil {
		t.Fatalf("expected completed fragment after second unique fragment")
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected no pending fragments after completion, got %d", got)
	}
}

func TestFragmentAssemblerDistinguishesPendingEntriesByPID(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	fragA := newTestTLSFragmentAt(0, 2, 10, "abcde", timestamp)
	fragA.PID = 1111
	fragA.TGID = 5678
	fragB := newTestTLSFragmentAt(0, 2, 10, "vwxyz", timestamp)
	fragB.PID = 2222
	fragB.TGID = 5678

	if completed, ok := assembler.Add(fragA); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	if completed, ok := assembler.Add(fragB); ok || completed != nil {
		t.Fatalf("expected second fragment with different PID to remain pending")
	}
	if got := assembler.Pending(); got != 2 {
		t.Fatalf("expected two pending fragments for distinct PIDs, got %d", got)
	}
}

func TestFragmentAssemblerDropsOldestPendingWhenCapExceeded(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	base := uint64(time.Now().UnixNano())

	for i := 0; i < tlsMaxPendingFragments+1; i++ {
		frag := newTestTLSFragmentAt(0, 2, 10, "abcde", base+uint64(i))
		frag.PID = uint32(1000 + i)
		frag.TGID = 5678
		if completed, ok := assembler.Add(frag); ok || completed != nil {
			t.Fatalf("expected fragment %d to remain pending", i)
		}
	}

	if got := assembler.Pending(); got != tlsMaxPendingFragments {
		t.Fatalf("expected pending fragments to be capped at %d, got %d", tlsMaxPendingFragments, got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment from cap enforcement, got %d", got)
	}

	first := newTestTLSFragmentAt(1, 2, 10, "fghij", base)
	first.PID = 1000
	first.TGID = 5678
	if completed, ok := assembler.Add(first); ok || completed != nil {
		t.Fatalf("expected evicted oldest fragment key to start a new pending assembly")
	}
	if got := assembler.Pending(); got != tlsMaxPendingFragments {
		t.Fatalf("expected pending fragments to stay capped at %d after reinserting evicted key, got %d", tlsMaxPendingFragments, got)
	}
}

func TestFragmentAssemblerDropsPendingWhenCountOrLengthMismatchAppearsForSameKey(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	frag0 := newTestTLSFragment(0, 2, 10, "abcde")
	frag0.TimestampNS = timestamp
	frag1 := newTestTLSFragment(1, 3, 12, "fghij")
	frag1.TimestampNS = timestamp

	if completed, ok := assembler.Add(frag0); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("expected one pending fragment, got %d", got)
	}

	if completed, ok := assembler.Add(frag1); ok || completed != nil {
		t.Fatalf("expected mismatched fragment to be rejected")
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("expected pending fragment to be deleted after mismatch, got %d", got)
	}
	if got := assembler.Dropped(); got != 1 {
		t.Fatalf("expected one dropped fragment after mismatch, got %d", got)
	}
}

func TestTLSFragmentLayoutMatchesGeneratedBPFStruct(t *testing.T) {
	if got, want := unsafe.Sizeof(tlsFragment{}), unsafe.Sizeof(bpf.AgentTlsCaptureTlsFragment{}); got != want {
		t.Fatalf("unexpected tlsFragment size: got %d want %d", got, want)
	}
}

func TestFragmentAssemblerPreservesCaptureMetadata(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	frag := newTestTLSFragment(0, 1, 5, "abcde")
	frag.OriginalLen = 20
	frag.Flags = tlsFlagTruncated
	frag.Function = tlsFuncSSLReadEx

	completed, ok := assembler.Add(frag)
	if !ok || completed == nil {
		t.Fatalf("expected completed fragment")
	}
	if completed.OriginalLen != 20 {
		t.Fatalf("original_len = %d, want 20", completed.OriginalLen)
	}
	if completed.Flags&tlsFlagTruncated == 0 {
		t.Fatalf("expected truncated flag to be preserved")
	}
	if completed.Function != tlsFuncSSLReadEx {
		t.Fatalf("function = %d, want %d", completed.Function, tlsFuncSSLReadEx)
	}

	event := parseTLSPlaintext(*completed)
	if event.CapturedLen != 5 || event.OriginalLen != 20 || !event.Truncated {
		t.Fatalf("unexpected capture metadata: captured=%d original=%d truncated=%v", event.CapturedLen, event.OriginalLen, event.Truncated)
	}
	if event.Function != "SSL_read_ex" {
		t.Fatalf("function = %q, want SSL_read_ex", event.Function)
	}
}

func TestCompletedFragmentPayloadIsCopied(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())
	frag0 := newTestTLSFragmentAt(0, 2, 6, "abc", timestamp)
	frag0.DataLen = 3
	frag0.TotalLen = 6
	frag1 := newTestTLSFragmentAt(1, 2, 6, "def", timestamp)
	frag1.DataLen = 3
	frag1.TotalLen = 6

	if completed, ok := assembler.Add(frag0); ok || completed != nil {
		t.Fatalf("expected first fragment to remain pending")
	}
	completed, ok := assembler.Add(frag1)
	if !ok || completed == nil {
		t.Fatalf("expected completed fragment")
	}
	if got := string(completed.Payload); got != "abcdef" {
		t.Fatalf("unexpected payload: %q", got)
	}
	orig := append([]byte(nil), completed.Payload...)
	frag0.Data[0] = 'z'
	frag1.Data[0] = 'y'
	if !bytes.Equal(completed.Payload, orig) {
		t.Fatalf("payload changed after source fragment mutation: %q", string(completed.Payload))
	}
}

func TestFragmentAssemblerRemoveByTGIDCleansPendingFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())
	fragA := newTestTLSFragmentAt(0, 2, 10, "aaaaa", timestamp)
	fragA.TGID = 100
	fragB := newTestTLSFragmentAt(0, 2, 10, "bbbbb", timestamp+1)
	fragB.TGID = 200
	if _, ok := assembler.Add(fragA); ok {
		t.Fatalf("expected fragA to remain pending")
	}
	if _, ok := assembler.Add(fragB); ok {
		t.Fatalf("expected fragB to remain pending")
	}
	if got := assembler.Pending(); got != 2 {
		t.Fatalf("pending = %d, want 2", got)
	}
	removed := assembler.RemoveByTGID(100)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("pending after remove = %d, want 1", got)
	}
	removed = assembler.RemoveByTGID(999)
	if removed != 0 {
		t.Fatalf("removed nonexistent = %d, want 0", removed)
	}
}

func TestTLSPipelineAssembleAndParseHTTPRequestIntegration(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())

	body := `{"model":"claude-opus-4-7","messages":["hello"]}`
	httpPayload := fmt.Sprintf("POST /v1/messages HTTP/1.1\r\nHost: api.anthropic.com\r\nAuthorization: Bearer sk-ant-secret\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	totalLen := uint32(len(httpPayload))
	split := len(httpPayload) / 2

	frag0 := newTestTLSFragmentAt(0, 2, int(totalLen), httpPayload[:split], timestamp)
	frag0.LibType = tlsLibGo
	frag0.DataLen = uint32(split)
	frag1 := newTestTLSFragmentAt(1, 2, int(totalLen), httpPayload[split:], timestamp)
	frag1.LibType = tlsLibGo
	frag1.DataLen = uint32(len(httpPayload) - split)

	assembler.Add(frag0)
	completed, ok := assembler.Add(frag1)
	if !ok || completed == nil {
		t.Fatalf("expected completed fragment from pipeline")
	}

	event := parseTLSPlaintext(*completed)
	if event.Type != "http_request" && event.Type != "tls_plaintext" {
		t.Fatalf("unexpected event type: %q", event.Type)
	}
	if event.Method != "POST" {
		t.Fatalf("method = %q, want POST", event.Method)
	}
	if event.URL != "/v1/messages" {
		t.Fatalf("url = %q, want /v1/messages", event.URL)
	}
	if event.Host != "api.anthropic.com" {
		t.Fatalf("host = %q, want api.anthropic.com", event.Host)
	}
	if event.Direction != "send" {
		t.Fatalf("direction = %q, want send", event.Direction)
	}
	if event.Lib != "go" {
		t.Fatalf("lib = %q, want go", event.Lib)
	}
	if event.Headers["authorization"] != "***REDACTED***" {
		t.Fatalf("authorization not redacted: %q", event.Headers["authorization"])
	}
	if event.ContentType != "application/json" {
		t.Fatalf("content_type = %q, want application/json", event.ContentType)
	}
	if event.BodySize <= 0 {
		t.Fatalf("body_size = %d, want >0", event.BodySize)
	}
}

func TestFragmentAssemblerRemoveByPIDCleansPendingFragments(t *testing.T) {
	assembler := NewFragmentAssembler(10 * time.Second)
	timestamp := uint64(time.Now().UnixNano())
	fragA := newTestTLSFragmentAt(0, 2, 10, "aaaaa", timestamp)
	fragA.PID = 42
	fragB := newTestTLSFragmentAt(0, 2, 10, "bbbbb", timestamp+1)
	fragB.PID = 43
	if _, ok := assembler.Add(fragA); ok {
		t.Fatalf("expected fragA to remain pending")
	}
	if _, ok := assembler.Add(fragB); ok {
		t.Fatalf("expected fragB to remain pending")
	}
	if got := assembler.Pending(); got != 2 {
		t.Fatalf("pending = %d, want 2", got)
	}
	removed := assembler.RemoveByPID(42)
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if got := assembler.Pending(); got != 1 {
		t.Fatalf("pending after remove = %d, want 1", got)
	}
}

// ---- merged from graphexecution_test.go ----

func TestBuildExecutionGraphIncludesProcessTreeResourcesAndPolicy(t *testing.T) {
	now := time.Unix(1710000000, 0).UTC()
	records := []CapturedEventRecord{
		{
			ReceivedAt: now,
			Event: &pb.Event{
				Pid:          100,
				Ppid:         1,
				Comm:         "codex",
				Type:         "process_fork",
				ExtraInfo:    "child_pid=101",
				AgentRunId:   "run-1",
				ToolCallId:   "tool-1",
				ToolName:     "bash",
				TraceId:      "trace-1",
				Decision:     "ALLOW",
				RiskScore:    12,
				CgroupId:     55,
				RootAgentPid: 100,
			},
		},
		{
			ReceivedAt: now.Add(time.Second),
			Event: &pb.Event{
				Pid:        101,
				Ppid:       100,
				Comm:       "bash",
				Type:       "execve",
				Path:       "/usr/bin/git",
				AgentRunId: "run-1",
				ToolCallId: "tool-1",
				ToolName:   "bash",
				TraceId:    "trace-1",
				Decision:   "ALLOW",
				RiskScore:  15,
			},
		},
		{
			ReceivedAt: now.Add(2 * time.Second),
			Event: &pb.Event{
				Pid:          101,
				Ppid:         100,
				Comm:         "git",
				Type:         "openat",
				Path:         "/workspace/package.json",
				AgentRunId:   "run-1",
				ToolCallId:   "tool-1",
				TraceId:      "trace-1",
				RiskScore:    18,
				RootAgentPid: 100,
			},
		},
		{
			ReceivedAt: now.Add(3 * time.Second),
			Event: &pb.Event{
				Pid:         101,
				Ppid:        100,
				Comm:        "git",
				Type:        "network_connect",
				NetEndpoint: "github.com:443",
				Domain:      "github.com",
				AgentRunId:  "run-1",
				ToolCallId:  "tool-1",
				TraceId:     "trace-1",
				RiskScore:   22,
			},
		},
		{
			ReceivedAt: now.Add(4 * time.Second),
			Event: &pb.Event{
				Pid:        101,
				Ppid:       100,
				Comm:       "SECRET_ACCESS",
				Type:       "semantic_alert",
				Path:       "/home/steve/.ssh/id_rsa",
				ExtraInfo:  "tool declared read_file but accessed secret",
				AgentRunId: "run-1",
				ToolCallId: "tool-1",
				TraceId:    "trace-1",
				Decision:   "ALERT",
				RiskScore:  96,
			},
		},
	}

	graph := buildExecutionGraph(records, executionGraphFilters{AgentRunID: "run-1"})
	if graph.EventCount != len(records) {
		t.Fatalf("EventCount = %d, want %d", graph.EventCount, len(records))
	}
	if len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		t.Fatalf("expected non-empty graph, got %d nodes / %d edges", len(graph.Nodes), len(graph.Edges))
	}
	assertGraphNodeKind(t, graph.Nodes, "agent_run")
	assertGraphNodeKind(t, graph.Nodes, "tool_call")
	assertGraphNodeKind(t, graph.Nodes, "process")
	assertGraphNodeKind(t, graph.Nodes, "file")
	assertGraphNodeKind(t, graph.Nodes, "network")
	assertGraphNodeKind(t, graph.Nodes, "policy_alert")
	assertGraphNodeKind(t, graph.Nodes, "policy_decision")
	assertGraphNodeKind(t, graph.Nodes, "syscall")
	assertGraphEdgeKind(t, graph.Edges, "spawned")
	assertGraphEdgeKind(t, graph.Edges, "parent_process")
	assertGraphEdgeKind(t, graph.Edges, "execed")
	assertGraphEdgeKind(t, graph.Edges, "opened")
	assertGraphEdgeKind(t, graph.Edges, "connected")
	assertGraphEdgeKind(t, graph.Edges, "alerted")
}

func TestBuildExecutionGraphAddsProcessCallChainFallbackEdges(t *testing.T) {
	base := time.Unix(1710000000, 0).UTC()
	records := []CapturedEventRecord{
		{ReceivedAt: base, Event: &pb.Event{Pid: 301, Ppid: 300, Comm: "python", Type: "openat", Path: "/tmp/a"}},
		{ReceivedAt: base.Add(time.Second), Event: &pb.Event{Pid: 302, Ppid: 301, Comm: "bash", Type: "process_exec", ExtraInfo: "old_pid=201"}},
	}

	graph := buildExecutionGraph(records, executionGraphFilters{})
	assertGraphEdgeKind(t, graph.Edges, "parent_process")
	assertGraphEdgeKind(t, graph.Edges, "exec_chain")
	assertGraphNodeLabelContains(t, graph.Nodes, "pid 300")
	assertGraphNodeLabelContains(t, graph.Nodes, "pid 201")
}

func TestBuildExecutionGraphFilters(t *testing.T) {
	base := time.Unix(1710000000, 0).UTC()
	records := []CapturedEventRecord{
		{ReceivedAt: base, Event: &pb.Event{Pid: 10, Comm: "git", Type: "openat", Path: "/workspace/a.txt", AgentRunId: "run-a", ToolCallId: "tool-a", TraceId: "trace-a", RiskScore: 20}},
		{ReceivedAt: base.Add(2 * time.Hour), Event: &pb.Event{Pid: 11, Comm: "curl", Type: "network_connect", NetEndpoint: "evil.example:443", Domain: "evil.example", AgentRunId: "run-b", ToolCallId: "tool-b", TraceId: "trace-b", RiskScore: 95}},
	}

	since := base.Add(time.Hour)
	graph := buildExecutionGraph(records, executionGraphFilters{Since: &since, Domain: "evil", RiskMin: 90})
	if graph.EventCount != 1 {
		t.Fatalf("EventCount = %d, want 1", graph.EventCount)
	}
	assertGraphNodeLabelContains(t, graph.Nodes, "evil.example:443")
	for _, node := range graph.Nodes {
		if node.Kind == "file" && node.Label == "/workspace/a.txt" {
			t.Fatalf("unexpected file node from filtered-out record")
		}
	}
}

func TestBuildExecutionGraphProcessTreeFilterIncludesDescendants(t *testing.T) {
	base := time.Unix(1710000000, 0).UTC()
	pid := uint32(100)
	records := []CapturedEventRecord{
		{ReceivedAt: base, Event: &pb.Event{Pid: 100, Ppid: 1, Comm: "agent", Type: "process_fork", ExtraInfo: "child_pid=101"}},
		{ReceivedAt: base.Add(time.Second), Event: &pb.Event{Pid: 101, Ppid: 100, Comm: "bash", Type: "process_fork", ExtraInfo: "child_pid=102"}},
		{ReceivedAt: base.Add(2 * time.Second), Event: &pb.Event{Pid: 102, Ppid: 101, Comm: "curl", Type: "network_connect", NetEndpoint: "api.example:443"}},
		{ReceivedAt: base.Add(3 * time.Second), Event: &pb.Event{Pid: 200, Ppid: 1, Comm: "unrelated", Type: "openat", Path: "/tmp/other"}},
	}

	graph := buildExecutionGraph(records, executionGraphFilters{PID: &pid, ProcessTree: true})
	if graph.EventCount != 3 {
		t.Fatalf("EventCount = %d, want descendant tree events only", graph.EventCount)
	}
	assertGraphNodeLabelContains(t, graph.Nodes, "curl")
	assertGraphNodeLabelContains(t, graph.Nodes, "api.example:443")
	assertGraphEdgeKind(t, graph.Edges, "child_process")
	for _, node := range graph.Nodes {
		if node.Label == "unrelated" || node.Label == "/tmp/other" {
			t.Fatalf("unexpected unrelated node %#v", node)
		}
	}
}

func TestBuildExecutionGraphProcessTreeIncludesMonitoredProcessItself(t *testing.T) {
	pid := uint32(12345)
	graph := buildExecutionGraph(nil, executionGraphFilters{PID: &pid, ProcessTree: true})
	if graph.EventCount != 0 {
		t.Fatalf("EventCount = %d, want no matched events", graph.EventCount)
	}
	assertGraphNodeLabelContains(t, graph.Nodes, "pid 12345")
	for _, node := range graph.Nodes {
		if node.ID == "proc:12345" {
			if node.Metadata["monitored"] != "true" {
				t.Fatalf("monitored metadata = %q, want true", node.Metadata["monitored"])
			}
			return
		}
	}
	t.Fatalf("missing monitored process node")
}

func assertGraphNodeKind(t *testing.T, nodes []ExecutionGraphNode, kind string) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind {
			return
		}
	}
	t.Fatalf("missing node kind %q", kind)
}

func assertGraphEdgeKind(t *testing.T, edges []ExecutionGraphEdge, kind string) {
	t.Helper()
	for _, edge := range edges {
		if edge.Kind == kind {
			return
		}
	}
	t.Fatalf("missing edge kind %q", kind)
}

func assertGraphNodeLabelContains(t *testing.T, nodes []ExecutionGraphNode, want string) {
	t.Helper()
	for _, node := range nodes {
		if node.Label == want {
			return
		}
	}
	t.Fatalf("missing node label %q", want)
}

// ---- merged from handlersagentsight_test.go ----

func TestAgentSightEventsExportReferenceJSONL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreAgentSightTestState(t)

	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000001000).UTC(),
		Event: &pb.Event{
			Pid:       123,
			Ppid:      1,
			Type:      "openat",
			EventType: pb.EventType_OPENAT,
			Comm:      "python",
			Path:      "/workspace/app.py",
			TraceId:   "trace-file",
		},
	}))

	tlsStore := NewTLSCaptureStore(10)
	tlsStore.Add(TLSPlaintextEvent{
		Type:           "http_request",
		Timestamp:      time.UnixMilli(1710000002000).UTC(),
		PID:            456,
		Comm:           "node",
		Method:         "POST",
		URL:            "/v1/messages",
		Host:           "api.anthropic.com",
		BodySize:       256,
		RedactionState: "sanitized",
		TraceID:        "trace-http",
	})

	router := gin.New()
	registerAgentSightRoutes(router.Group(""), tlsStore)

	req := httptest.NewRequest(http.MethodGet, "/agentsight/events?format=jsonl&limit=10", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agentsight/events returned %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "application/x-ndjson") {
		t.Fatalf("content type = %q, want application/x-ndjson", got)
	}

	events := decodeAgentSightJSONLLines(t, rec.Body.String())
	if len(events) != 2 {
		t.Fatalf("event count = %d, want 2: %#v", len(events), events)
	}
	if events[0].Source != "file" {
		t.Fatalf("first source = %q, want file", events[0].Source)
	}
	if events[0].Data["path"] != "/workspace/app.py" {
		t.Fatalf("file path = %#v, want /workspace/app.py", events[0].Data["path"])
	}
	if events[0].Data["event_type"] != "OPENAT" {
		t.Fatalf("file event_type = %#v, want OPENAT", events[0].Data["event_type"])
	}
	if events[1].Source != "http_parser" {
		t.Fatalf("second source = %q, want http_parser", events[1].Source)
	}
	if events[1].Data["host"] != "api.anthropic.com" {
		t.Fatalf("http host = %#v, want api.anthropic.com", events[1].Data["host"])
	}
	if events[1].Data["event_type"] != "HTTP_MESSAGE" {
		t.Fatalf("http event_type = %#v, want HTTP_MESSAGE", events[1].Data["event_type"])
	}
}

func TestAgentSightCompatibilityAndExternalRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreAgentSightTestState(t)

	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000100000).UTC(),
		Event: &pb.Event{
			Pid:       777,
			Type:      "execve",
			EventType: pb.EventType_EXECVE,
			Comm:      "bash",
			Path:      "/usr/bin/bash",
		},
	}))

	router := gin.New()
	registerAgentSightCompatibilityRoutes(router.Group("/api"), nil)
	registerExternalAPIRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodPost, "/api/events", strings.NewReader(`{"timestamp":1710000101001,"source":"process","pid":888,"comm":"claude","data":{"event":"FILE_OPEN","filepath":"/tmp/imported.txt"}}`))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/events returned %d: %s", rec.Code, rec.Body.String())
	}
	var uploadResp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("upload response decode failed: %v", err)
	}
	if uploadResp["imported"] != float64(1) {
		t.Fatalf("upload imported = %#v, want 1", uploadResp["imported"])
	}

	req = httptest.NewRequest(http.MethodGet, "/api/events?source=process", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/events returned %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/plain") {
		t.Fatalf("compat content type = %q, want text/plain", got)
	}
	compatEvents := decodeAgentSightJSONLLines(t, rec.Body.String())
	if len(compatEvents) != 2 || compatEvents[0].PID != 777 || compatEvents[1].PID != 888 {
		t.Fatalf("compat events = %#v, want PID 777 and uploaded PID 888", compatEvents)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/agentsight/events?format=array&source=process", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/v1/agentsight/events returned %d: %s", rec.Code, rec.Body.String())
	}
	var externalEvents []agentSightExportEvent
	if err := json.Unmarshal(rec.Body.Bytes(), &externalEvents); err != nil {
		t.Fatalf("external array decode failed: %v", err)
	}
	if len(externalEvents) != 2 || externalEvents[0].Comm != "bash" || externalEvents[1].Comm != "claude" {
		t.Fatalf("external events = %#v, want bash and uploaded claude events", externalEvents)
	}
}

func TestAgentSightStatsRunnersAndAdvancedQuery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	restoreAgentSightTestState(t)

	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000200000).UTC(),
		Event: &pb.Event{
			Pid:       101,
			Type:      "openat",
			EventType: pb.EventType_OPENAT,
			Comm:      "python",
			Path:      "/workspace/secret.txt",
		},
	}))
	capturedEventArchive.Add(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: time.UnixMilli(1710000201000).UTC(),
		Event: &pb.Event{
			Pid:       202,
			Type:      "wrapper_intercept",
			EventType: pb.EventType_WRAPPER_INTERCEPT,
			Comm:      "claude",
			Path:      "claude --dangerously-skip-permissions",
		},
	}))
	tlsStore := NewTLSCaptureStore(10)
	tlsStore.Add(TLSPlaintextEvent{
		Type:      "sse_message",
		Timestamp: time.UnixMilli(1710000202000).UTC(),
		PID:       303,
		Comm:      "node",
		Host:      "api.openai.com",
		SSEEvent:  "content_block_delta",
	})

	router := gin.New()
	registerAgentSightRoutes(router.Group(""), tlsStore)
	registerAgentSightCompatibilityRoutes(router.Group("/api"), tlsStore)
	registerExternalAPIRoutes(router.Group("/api/v1"), tlsStore)

	req := httptest.NewRequest(http.MethodGet, "/agentsight/events/stats", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /agentsight/events/stats returned %d: %s", rec.Code, rec.Body.String())
	}
	var stats agentSightEventsStats
	if err := json.Unmarshal(rec.Body.Bytes(), &stats); err != nil {
		t.Fatalf("stats decode failed: %v", err)
	}
	if stats.Total != 3 || stats.ByRunner["process"] != 1 || stats.ByRunner["agent"] != 1 || stats.ByRunner["tls"] != 1 {
		t.Fatalf("stats = %+v, want process/agent/tls counts", stats)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/runners", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/runners returned %d: %s", rec.Code, rec.Body.String())
	}
	var runnersResp struct {
		Runners []agentSightRunnerStatus `json:"runners"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &runnersResp); err != nil {
		t.Fatalf("runners decode failed: %v", err)
	}
	if len(runnersResp.Runners) == 0 {
		t.Fatal("expected runner statuses")
	}
	if !agentSightTestRunnerPresent(runnersResp.Runners, "process") || !agentSightTestRunnerPresent(runnersResp.Runners, "tls") {
		t.Fatalf("runners = %+v, want process and tls runners", runnersResp.Runners)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/agentsight/events/query", strings.NewReader(`{"runner":"tls","event_types":["SSE_MESSAGE"],"filter":"openai","limit":20}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/v1/agentsight/events/query returned %d: %s", rec.Code, rec.Body.String())
	}
	var queryResp struct {
		Events []agentSightExportEvent `json:"events"`
		Stats  agentSightEventsStats   `json:"stats"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &queryResp); err != nil {
		t.Fatalf("query decode failed: %v", err)
	}
	if len(queryResp.Events) != 1 || queryResp.Events[0].Source != "sse_processor" || queryResp.Stats.ByRunner["tls"] != 1 {
		t.Fatalf("query response = %+v, want one tls/sse event", queryResp)
	}
}

func restoreAgentSightTestState(t *testing.T) {
	t.Helper()
	oldStore := runtimeSettingsStore
	oldArchive := capturedEventArchive
	oldUploaded := agentSightUploadedEvents
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{}}
	capturedEventArchive = newEventArchive(20)
	agentSightUploadedEvents = newAgentSightEventStore(20)
	t.Cleanup(func() {
		runtimeSettingsStore = oldStore
		capturedEventArchive = oldArchive
		agentSightUploadedEvents = oldUploaded
	})
}

func agentSightTestRunnerPresent(runners []agentSightRunnerStatus, id string) bool {
	for _, runner := range runners {
		if runner.ID == id {
			return true
		}
	}
	return false
}

func decodeAgentSightJSONLLines(t *testing.T, body string) []agentSightExportEvent {
	t.Helper()
	var events []agentSightExportEvent
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event agentSightExportEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("failed to decode JSONL line %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}

// ---- merged from healthbootstrap_test.go ----

func TestBuildTracepointBootstrapStatus(t *testing.T) {
	status := buildTracepointBootstrapStatus(5, []string{"syscalls/sys_enter_lstat", "sched/sched_process_free"})
	if status.Status != "partial" {
		t.Fatalf("expected partial status, got %q", status.Status)
	}
	if status.CompiledCount != 5 || status.AttachedCount != 3 || status.SkippedCount != 2 {
		t.Fatalf("unexpected counts: %+v", status)
	}
	if len(status.SkippedTracepoints) != 2 {
		t.Fatalf("expected 2 skipped tracepoints, got %d", len(status.SkippedTracepoints))
	}
	if status.SkippedTracepoints[0] != "sched/sched_process_free" || status.SkippedTracepoints[1] != "syscalls/sys_enter_lstat" {
		t.Fatalf("expected skipped tracepoints to be sorted, got %+v", status.SkippedTracepoints)
	}
	if status.Message == "" {
		t.Fatal("expected a human-readable status message")
	}
}

func TestRecordTracepointBootstrapStatusCopiesSnapshot(t *testing.T) {
	original := bootstrapTracepointStatusStore.Snapshot()
	t.Cleanup(func() {
		bootstrapTracepointStatusStore.mu.Lock()
		bootstrapTracepointStatusStore.status = original
		bootstrapTracepointStatusStore.mu.Unlock()
	})

	recordTracepointBootstrapStatus(2, []string{"syscalls/sys_enter_execve"})
	snapshot := bootstrapTracepointStatusStore.Snapshot()
	if snapshot.Status != "partial" {
		t.Fatalf("expected partial status, got %q", snapshot.Status)
	}
	if len(snapshot.SkippedTracepoints) != 1 || snapshot.SkippedTracepoints[0] != "syscalls/sys_enter_execve" {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}

	snapshot.SkippedTracepoints[0] = "mutated"
	again := bootstrapTracepointStatusStore.Snapshot()
	if again.SkippedTracepoints[0] != "syscalls/sys_enter_execve" {
		t.Fatalf("snapshot should be copied defensively, got %+v", again.SkippedTracepoints)
	}
}

// ---- merged from hooks_test.go ----

func TestBuildNativeHookExtraInfoRecordsSafePromptMetadata(t *testing.T) {
	payload := map[string]interface{}{
		"prompt":     "please inspect SECRET_TOKEN=abc123",
		"session_id": "session-1",
	}

	extra := buildNativeHookExtraInfo(payload, "UserPromptSubmit", "chat")

	if !strings.Contains(extra, "hook_event=UserPromptSubmit") {
		t.Fatalf("extra info missing hook event: %q", extra)
	}
	if !strings.Contains(extra, "prompt_digest=sha256:") || !strings.Contains(extra, "prompt_len=") {
		t.Fatalf("extra info missing prompt metadata: %q", extra)
	}
	if strings.Contains(extra, "SECRET_TOKEN") || strings.Contains(extra, "abc123") {
		t.Fatalf("extra info leaked raw prompt content: %q", extra)
	}
}

func TestBuildNativeHookExtraInfoFindsNestedResponseMetadata(t *testing.T) {
	payload := map[string]interface{}{
		"tool_response": map[string]interface{}{
			"response": "final answer text",
		},
	}

	extra := buildNativeHookExtraInfo(payload, "AfterAgent", "chat")

	if !strings.Contains(extra, "response_digest=sha256:") || !strings.Contains(extra, "response_len=17") {
		t.Fatalf("extra info missing nested response metadata: %q", extra)
	}
	if strings.Contains(extra, "final answer text") {
		t.Fatalf("extra info leaked raw response content: %q", extra)
	}
}

func TestAntigravityRelayScriptReturnsCLIRequiredJSON(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"antigravity": "secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	script := buildHookRelayScript(HookDef{ID: "antigravity"})

	for _, want := range []string{
		"X-Agent-CLI: antigravity",
		"X-Agent-Hook-Event: $hook_event",
		`PreToolUse)`,
		`"decision":"allow"`,
		`"injectSteps":[]`,
		`"terminationBehavior":""`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("antigravity relay script missing %q:\n%s", want, script)
		}
	}
}

func TestInstallAntigravityNativeHookWritesPluginHooksJSON(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{HookSecrets: map[string]string{"antigravity": "secret"}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	pluginDir := filepath.Join(t.TempDir(), ".gemini", "antigravity-cli", "plugins", hookMarker)
	h := HookDef{
		ID:               "antigravity",
		NativeConfigPath: filepath.Join(pluginDir, "hooks.json"),
		NativeHookEvent:  "PreToolUse",
		NativeMatcher:    "*",
		ConfigFormat:     ConfigFormatJSON,
	}

	if err := installAntigravityNativeHook(h); err != nil {
		t.Fatalf("install antigravity hook: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pluginDir, "plugin.json")); err != nil {
		t.Fatalf("plugin manifest was not written: %v", err)
	}
	if _, err := os.Stat(hookRelayScriptPath(h)); err != nil {
		t.Fatalf("relay script was not written: %v", err)
	}
	if !isNativeHookInstalled(h) {
		t.Fatalf("installed antigravity hook was not detected")
	}

	var cfg map[string]interface{}
	b, err := os.ReadFile(h.NativeConfigPath)
	if err != nil {
		t.Fatalf("read hooks.json: %v", err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatalf("parse hooks.json: %v", err)
	}
	hookDef, _ := cfg[hookMarker].(map[string]interface{})
	if hookDef == nil {
		t.Fatalf("hooks.json missing %s definition: %#v", hookMarker, cfg)
	}
	entries, _ := hookDef["PreToolUse"].([]interface{})
	if len(entries) != 1 {
		t.Fatalf("unexpected PreToolUse entries: %#v", hookDef["PreToolUse"])
	}
	matcher, _ := entries[0].(map[string]interface{})
	if got, _ := matcher["matcher"].(string); got != "*" {
		t.Fatalf("unexpected matcher: %#v", matcher)
	}
	hooks, _ := matcher["hooks"].([]interface{})
	if len(hooks) != 1 {
		t.Fatalf("unexpected command hooks: %#v", matcher["hooks"])
	}
	commandHook, _ := hooks[0].(map[string]interface{})
	command, _ := commandHook["command"].(string)
	if !strings.Contains(command, hookMarker+"-antigravity.sh") || !strings.Contains(command, "PreToolUse") {
		t.Fatalf("command does not call event-aware relay script: %q", command)
	}
	if got, _ := commandHook["timeout"].(float64); got != 5 {
		t.Fatalf("antigravity timeout should be seconds=5, got %#v", commandHook["timeout"])
	}

	if err := uninstallAntigravityNativeHook(h); err != nil {
		t.Fatalf("uninstall antigravity hook: %v", err)
	}
	if isNativeHookInstalled(h) {
		t.Fatalf("uninstalled antigravity hook still detected")
	}
	if _, err := os.Stat(hookRelayScriptPath(h)); !os.IsNotExist(err) {
		t.Fatalf("relay script should be removed on uninstall, stat err=%v", err)
	}
}

func TestAntigravityPayloadShapeFeedsPathAndContext(t *testing.T) {
	payload := map[string]interface{}{
		"conversationId": "conv-1",
		"toolCall": map[string]interface{}{
			"name": "run_command",
			"args": map[string]interface{}{
				"CommandLine": "npm test",
				"Cwd":         "/workspace/project",
			},
		},
	}
	toolCall, _ := payload["toolCall"].(map[string]interface{})
	toolInput, _ := toolCall["args"].(map[string]interface{})
	path := extractNativeHookPath(toolInput)
	if path != "npm test" {
		t.Fatalf("unexpected antigravity command path: %q", path)
	}
	_, ctx := buildProcessContextFromHookPayload(payload, "", path)
	if ctx.ToolName != "run_command" || ctx.ConversationID != "conv-1" || ctx.Cwd != "/workspace/project" {
		t.Fatalf("unexpected antigravity context: %#v", ctx)
	}
}

// ---- merged from httpparsertls_test.go ----

func testCompletedTLSFragment(payload string, direction uint8) completedTLSFragment {
	return completedTLSFragment{
		TimestampNS: uint64(time.Date(2026, time.May, 10, 12, 0, 0, 0, time.UTC).UnixNano()),
		PID:         4321,
		TGID:        8765,
		LibType:     tlsLibOpenSSL,
		Direction:   direction,
		Comm:        "curl",
		Payload:     []byte(payload),
	}
}

func TestParseTLSPlaintextHTTPRequestRedactsSensitiveHeaders(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /login HTTP/1.1",
		"Host: example.com",
		"Authorization: Bearer secret-token",
		"X-API-Key: abc123",
		"Cookie: session=super-secret",
		"Content-Type: application/json",
		"Content-Length: 22",
		"",
		`{"password":"hunter2"}`,
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if event.Method != "POST" {
		t.Fatalf("Method = %q, want POST", event.Method)
	}
	if event.URL != "/login" {
		t.Fatalf("URL = %q, want /login", event.URL)
	}
	if event.Host != "example.com" {
		t.Fatalf("Host = %q, want example.com", event.Host)
	}
	if got := event.Headers["authorization"]; got != "***REDACTED***" {
		t.Fatalf("authorization header = %q, want redacted", got)
	}
	if got := event.Headers["x-api-key"]; got != "***REDACTED***" {
		t.Fatalf("x-api-key header = %q, want redacted", got)
	}
	if got := event.Headers["cookie"]; got != "***REDACTED***" {
		t.Fatalf("cookie header = %q, want redacted", got)
	}
	if event.Body != "{\n  \"password\": \"***REDACTED***\"\n}" {
		t.Fatalf("Body = %q, want redacted pretty-printed JSON", event.Body)
	}
	if event.RawHexDump != "" {
		t.Fatalf("RawHexDump = %q, want empty for parsed HTTP", event.RawHexDump)
	}
	if !event.RawAvailable {
		t.Fatalf("RawAvailable = false, want true for parsed HTTP")
	}
}

func TestParseTLSPlaintextHTTPRequestRedactsSensitiveURLQuery(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"GET /v1/messages?api_key=abc&token=secret&safe=value HTTP/1.1",
		"Host: example.com",
		"Content-Length: 0",
		"",
		"",
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if strings.Contains(event.URL, "abc") || strings.Contains(event.URL, "secret") {
		t.Fatalf("URL = %q, want sensitive query values redacted", event.URL)
	}
	if !strings.Contains(event.URL, "api_key=%2A%2A%2AREDACTED%2A%2A%2A") || !strings.Contains(event.URL, "safe=value") {
		t.Fatalf("URL = %q, want redacted sensitive query and preserved safe query", event.URL)
	}
}

func TestParseTLSPlaintextSSEAnnotatesDigest(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"HTTP/1.1 200 OK",
		"Content-Type: text/event-stream",
		"Content-Length: 39",
		"",
		"event: completion\ndata: token=secret\n\n",
	}, "\r\n"), tlsDirectionRecv)

	event := parseTLSPlaintext(fragment)

	if event.Type != "sse_message" {
		t.Fatalf("Type = %q, want sse_message", event.Type)
	}
	if event.SSEEvent != "completion" {
		t.Fatalf("SSEEvent = %q, want completion", event.SSEEvent)
	}
	if event.SSEDataDigest == "" {
		t.Fatalf("SSEDataDigest empty, want digest")
	}
	if strings.Contains(event.Body, "secret") {
		t.Fatalf("Body = %q, want inline secret redacted", event.Body)
	}
}

func TestParseTLSPlaintextHTTPResponse(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"HTTP/1.1 201 Created",
		"Content-Type: application/json",
		"Set-Cookie: session=secret; HttpOnly",
		"Content-Length: 11",
		"",
		`{"ok":true}`,
	}, "\r\n"), tlsDirectionRecv)

	event := parseTLSPlaintext(fragment)

	if event.StatusCode != 201 {
		t.Fatalf("StatusCode = %d, want 201", event.StatusCode)
	}
	if got := event.Headers["set-cookie"]; got != "***REDACTED***" {
		t.Fatalf("set-cookie header = %q, want redacted", got)
	}
	if got := event.Body; !strings.Contains(got, "\n  \"ok\": true\n") {
		t.Fatalf("Body = %q, want pretty-printed JSON", got)
	}
	if event.Method != "" || event.URL != "" {
		t.Fatalf("unexpected request fields for response: method=%q url=%q", event.Method, event.URL)
	}
}

func TestParseTLSPlaintextNonHTTPUsesHexDump(t *testing.T) {
	fragment := completedTLSFragment{
		TimestampNS: uint64(time.Now().UnixNano()),
		PID:         1,
		TGID:        2,
		LibType:     tlsLibGo,
		Direction:   tlsDirectionSend,
		Comm:        "go-app",
		Payload: []byte(strings.Join([]string{
			"HELLO /not-http HTTP/1.1",
			"Header: value",
			"",
			"body",
		}, "\r\n")),
	}

	event := parseTLSPlaintext(fragment)

	if event.RawHexDump == "" {
		t.Fatalf("RawHexDump = %q, want hex dump", event.RawHexDump)
	}
	if event.Method != "" || event.URL != "" || len(event.Headers) != 0 {
		t.Fatalf("unexpected structured HTTP fields for non-HTTP payload: %+v", event)
	}
	if event.RawAvailable {
		t.Fatalf("RawAvailable = true, want false for non-HTTP payload")
	}
}

func TestParseTLSPlaintextRedactsProxyAuthorizationHeader(t *testing.T) {
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /proxy HTTP/1.1",
		"Host: example.com",
		"Proxy-Authorization: Basic secret",
		"pRoXy-aUtHoRiZaTiOn: Digest another-secret",
		"Content-Length: 0",
		"",
		"",
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if got := event.Headers["proxy-authorization"]; got != "***REDACTED***" {
		t.Fatalf("proxy-authorization header = %q, want redacted", got)
	}
}

func TestParseTLSPlaintextTruncatesLargeBody(t *testing.T) {
	largeBody := "{" + strings.Repeat("\"a\":\"xxxxxxxxxx\",", 2000) + "\"z\":\"end\"}"
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /bulk HTTP/1.1",
		"Host: example.com",
		"Content-Type: text/plain",
		"Content-Length: 25000",
		"",
		largeBody,
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if !event.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
	if event.BodySize <= tlsMaxBodySize {
		t.Fatalf("BodySize = %d, want larger than max body size", event.BodySize)
	}
	if len(event.Body) != tlsMaxBodySize {
		t.Fatalf("Body length = %d, want %d", len(event.Body), tlsMaxBodySize)
	}
}

func TestParseTLSPlaintextTruncatesBasedOnRawBodySize(t *testing.T) {
	rawJSON := "[\n" + strings.Repeat(" ", tlsMaxBodySize+256) + "1\n]"
	fragment := testCompletedTLSFragment(fmt.Sprintf(
		"POST /raw-size HTTP/1.1\r\nHost: example.com\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s",
		len(rawJSON),
		rawJSON,
	), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if !event.Truncated {
		t.Fatalf("Truncated = false, want true when raw body exceeds limit")
	}
	if len(event.Body) > tlsMaxBodySize {
		t.Fatalf("Body length = %d, want at most %d", len(event.Body), tlsMaxBodySize)
	}
	if event.BodySize <= tlsMaxBodySize {
		t.Fatalf("BodySize = %d, want larger than max body size", event.BodySize)
	}
}

func TestParseTLSPlaintextBoundsBodyReadToMaxPlusOne(t *testing.T) {
	body := strings.Repeat("x", tlsMaxBodySize+512)
	fragment := testCompletedTLSFragment(strings.Join([]string{
		"POST /bounded HTTP/1.1",
		"Host: example.com",
		"Content-Type: text/plain",
		"Content-Length: 999999",
		"",
		body,
	}, "\r\n"), tlsDirectionSend)

	event := parseTLSPlaintext(fragment)

	if event.BodySize != tlsMaxBodySize+1 {
		t.Fatalf("BodySize = %d, want %d", event.BodySize, tlsMaxBodySize+1)
	}
	if !event.Truncated {
		t.Fatalf("Truncated = false, want true")
	}
	if len(event.Body) != tlsMaxBodySize {
		t.Fatalf("Body length = %d, want %d", len(event.Body), tlsMaxBodySize)
	}
}

// ---- merged from httpstreamassemblertls_test.go ----

func TestTLSHTTPStreamAssemblerMergesSplitHTTPResponse(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	body := `{"ok":true,"message":"merged"}`
	payload := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	first := testCompletedTLSFragment(payload[:70], tlsDirectionRecv)
	second := testCompletedTLSFragment(payload[70:], tlsDirectionRecv)
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if got := assembler.Add(first); len(got) != 0 {
		t.Fatalf("first fragment emitted %d events, want 0", len(got))
	}
	events := assembler.Add(second)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Type != "http_response" {
		t.Fatalf("Type = %q, want http_response", event.Type)
	}
	if event.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, want 200", event.StatusCode)
	}
	if !strings.Contains(event.Body, `"message": "merged"`) {
		t.Fatalf("Body = %q, want merged JSON body", event.Body)
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

func TestTLSHTTPStreamAssemblerMergesSplitHTTPRequest(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	body := `{"prompt":"hello"}`
	payload := fmt.Sprintf("POST /v1/messages HTTP/1.1\r\nHost: api.example.com\r\nContent-Type: application/json\r\nContent-Length: %d\r\n\r\n%s", len(body), body)
	first := testCompletedTLSFragment(payload[:55], tlsDirectionSend)
	second := testCompletedTLSFragment(payload[55:], tlsDirectionSend)
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if got := assembler.Add(first); len(got) != 0 {
		t.Fatalf("first fragment emitted %d events, want 0", len(got))
	}
	events := assembler.Add(second)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Type != "http_request" || event.Method != "POST" || event.URL != "/v1/messages" || event.Host != "api.example.com" {
		t.Fatalf("unexpected request event: %+v", event)
	}
	if !strings.Contains(event.Body, `"prompt": "hello"`) {
		t.Fatalf("Body = %q, want pretty JSON", event.Body)
	}
}

func TestTLSHTTPStreamAssemblerDropsRawTLSRecords(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	fragment := testCompletedTLSFragment("not http plaintext", tlsDirectionRecv)

	if events := assembler.Add(fragment); len(events) != 0 {
		t.Fatalf("events = %d, want 0", len(events))
	}
	if got := assembler.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

func TestTLSHTTPStreamAssemblerCopiesRequestContextToResponse(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	request := testCompletedTLSFragment("GET /health HTTP/1.1\r\nHost: api.example.com\r\nContent-Length: 0\r\n\r\n", tlsDirectionSend)
	response := testCompletedTLSFragment("HTTP/1.1 204 No Content\r\nContent-Length: 0\r\n\r\n", tlsDirectionRecv)
	response.TimestampNS = request.TimestampNS + uint64(time.Millisecond)

	if events := assembler.Add(request); len(events) != 1 {
		t.Fatalf("request events = %d, want 1", len(events))
	}
	events := assembler.Add(response)
	if len(events) != 1 {
		t.Fatalf("response events = %d, want 1", len(events))
	}
	event := events[0]
	if event.Host != "api.example.com" || event.Method != "GET" || event.URL != "/health" {
		t.Fatalf("response context = method %q url %q host %q, want request context", event.Method, event.URL, event.Host)
	}
}

func TestTLSHTTPStreamAssemblerParsesChunkedResponseAfterTerminator(t *testing.T) {
	assembler := NewTLSHTTPStreamAssembler(10 * time.Second)
	payload := strings.Join([]string{
		"HTTP/1.1 200 OK",
		"Content-Type: text/plain",
		"Transfer-Encoding: chunked",
		"",
		"5",
		"hello",
		"6",
		" world",
		"0",
		"",
		"",
	}, "\r\n")
	first := testCompletedTLSFragment(payload[:90], tlsDirectionRecv)
	second := testCompletedTLSFragment(payload[90:], tlsDirectionRecv)
	second.TimestampNS = first.TimestampNS + uint64(time.Millisecond)

	if events := assembler.Add(first); len(events) != 0 {
		t.Fatalf("first events = %d, want 0", len(events))
	}
	events := assembler.Add(second)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if got := events[0].Body; got != "hello world" {
		t.Fatalf("Body = %q, want dechunked body", got)
	}
}

// ---- merged from modelregistry_test.go ----

// ── Helpers ────────────────────────────────────────────────────────

func seedRand() *rand.Rand { return rand.New(rand.NewSource(42)) }

func initMLTest(t *testing.T, nSamples int) {
	t.Helper()
	InitTrainingStore(100000)
	mlConfig = DefaultMLConfig()
	mlEnabled = true
	globalTrainer.ResetCancel()

	rng := seedRand()
	for i := 0; i < nSamples; i++ {
		var features [FeatureDim]float64
		for d := 0; d < FeatureDim; d++ {
			features[d] = rng.Float64()
		}
		globalTrainingStore.Add(TrainingSample{
			Features:  features,
			Label:     int32(i % 4),
			Comm:      fmt.Sprintf("cmd-%d", i%4),
			Args:      []string{fmt.Sprintf("arg-%d", i)},
			Timestamp: time.Now(),
			UserLabel: "test",
		})
	}
}

func tmpModelPath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(t.TempDir(), name+".bin")
}

// ── Decision Forest ────────────────────────────────────────────────

func TestDecisionForestTrainPredict(t *testing.T) {
	initMLTest(t, 300)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([]trainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
	}

	forest := buildAutoTuneForest(samples, 11, 5, 3, 42)
	if len(forest.Trees) != 11 {
		t.Fatalf("expected 11 trees, got %d", len(forest.Trees))
	}
	if !forest.IsTrained {
		t.Fatal("forest not marked as trained")
	}

	// Predict all samples — should not hang
	correct := 0
	for _, s := range samples {
		pred := forest.Predict(s.features)
		if pred.Action == s.label {
			correct++
		}
	}
	acc := float64(correct) / float64(len(samples))
	t.Logf("RF accuracy: %.2f%% (%d/%d)", acc*100, correct, len(samples))
	if acc < 0.1 {
		t.Logf("warn: accuracy very low (random features), but test passes if no hang")
	}
}

func TestDecisionForestSerializeRoundtrip(t *testing.T) {
	initMLTest(t, 200)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([]trainSample, len(labeled))
	for i, s := range labeled {
		samples[i] = trainSample{features: s.Features, label: s.Label}
	}

	forest := buildAutoTuneForest(samples, 7, 4, 2, 99)
	path := tmpModelPath(t, "rf_test")

	if err := forest.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	loaded, err := DeserializeForest(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if len(loaded.Trees) != len(forest.Trees) {
		t.Fatalf("tree count mismatch: %d vs %d", len(loaded.Trees), len(forest.Trees))
	}

	// Verify predictions match
	for i, s := range samples[:10] {
		orig := forest.Predict(s.features)
		reloaded := loaded.Predict(s.features)
		if orig.Action != reloaded.Action {
			t.Errorf("sample %d: action mismatch %d vs %d", i, orig.Action, reloaded.Action)
		}
	}
	t.Logf("RF roundtrip OK: %d trees", len(loaded.Trees))
}

// ── KNN ────────────────────────────────────────────────────────────

func TestKNNPredict(t *testing.T) {
	initMLTest(t, 500)

	labeled := globalTrainingStore.LabeledSamples()
	model := NewKNNModel(5, "euclidean", "uniform")
	model.NumClasses = 4
	model.Samples = make([][FeatureDim]float64, len(labeled))
	model.Labels = make([]int32, len(labeled))
	for i, s := range labeled {
		model.Samples[i] = s.Features
		model.Labels[i] = s.Label
	}

	pred := model.Predict(labeled[0].Features)
	if pred.Action < 0 || pred.Action > 3 {
		t.Errorf("invalid action: %d", pred.Action)
	}
	if pred.Confidence < 0 || pred.Confidence > 1 {
		t.Errorf("invalid confidence: %.3f", pred.Confidence)
	}
	t.Logf("KNN predict: action=%d, confidence=%.3f, anomaly=%.3f",
		pred.Action, pred.Confidence, pred.AnomalyScore)

	// Empty model should return safe defaults
	empty := NewKNNModel(3, "euclidean", "uniform")
	emptyPred := empty.Predict(labeled[0].Features)
	if emptyPred.Action != 0 {
		t.Errorf("empty model should return ALLOW(0), got %d", emptyPred.Action)
	}
}

func TestKNNSerializeRoundtrip(t *testing.T) {
	initMLTest(t, 200)

	labeled := globalTrainingStore.LabeledSamples()
	model := NewKNNModel(7, "manhattan", "distance")
	model.NumClasses = 4
	model.Samples = make([][FeatureDim]float64, len(labeled))
	model.Labels = make([]int32, len(labeled))
	for i, s := range labeled {
		model.Samples[i] = s.Features
		model.Labels[i] = s.Label
	}

	path := tmpModelPath(t, "knn_test")
	if err := model.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	loaded, err := DeserializeKNN(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.K != model.K {
		t.Fatalf("K mismatch: %d vs %d", loaded.K, model.K)
	}
	if loaded.Distance != model.Distance {
		t.Fatalf("distance mismatch: %s vs %s", loaded.Distance, model.Distance)
	}
	if len(loaded.Samples) != len(model.Samples) {
		t.Fatalf("sample count mismatch: %d vs %d", len(loaded.Samples), len(model.Samples))
	}

	// Verify predictions match
	for i := 0; i < 10; i++ {
		orig := model.Predict(labeled[i].Features)
		reloaded := loaded.Predict(labeled[i].Features)
		if orig.Action != reloaded.Action {
			t.Errorf("sample %d: action mismatch", i)
		}
	}
	t.Logf("KNN roundtrip OK: k=%d, samples=%d", loaded.K, len(loaded.Samples))
}

func TestKNNDistanceMetrics(t *testing.T) {
	initMLTest(t, 100)
	labeled := globalTrainingStore.LabeledSamples()

	euclidean := NewKNNModel(3, "euclidean", "uniform")
	manhattan := NewKNNModel(3, "manhattan", "uniform")
	for _, m := range []*KNNModel{euclidean, manhattan} {
		m.NumClasses = 4
		m.Samples = make([][FeatureDim]float64, len(labeled))
		m.Labels = make([]int32, len(labeled))
		for i, s := range labeled {
			m.Samples[i] = s.Features
			m.Labels[i] = s.Label
		}
	}

	// Both should produce valid predictions
	ePred := euclidean.Predict(labeled[0].Features)
	mPred := manhattan.Predict(labeled[0].Features)
	t.Logf("euclidean: action=%d conf=%.3f, manhattan: action=%d conf=%.3f",
		ePred.Action, ePred.Confidence, mPred.Action, mPred.Confidence)
}

// ── Logistic Regression ────────────────────────────────────────────

func TestLogisticTrainPredict(t *testing.T) {
	initMLTest(t, 500)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([][FeatureDim]float64, len(labeled))
	labels := make([]int32, len(labeled))
	for i, s := range labeled {
		samples[i] = s.Features
		labels[i] = s.Label
	}

	model := NewLogisticModel(0.01, "l2", 500)
	model.NumClasses = 4
	model.Train(samples, labels)

	if len(model.Weights) != 4 {
		t.Fatalf("expected 4 class weight sets, got %d", len(model.Weights))
	}

	// Predict all samples
	correct := 0
	for i, s := range samples {
		pred := model.Predict(s)
		if pred.Action == labels[i] {
			correct++
		}
	}
	acc := float64(correct) / float64(len(samples))
	t.Logf("Logistic accuracy: %.2f%% (%d/%d)", acc*100, correct, len(samples))
	if acc < 0.1 {
		t.Logf("warn: accuracy low (random features), but training should not hang")
	}
}

func TestLogisticSerializeRoundtrip(t *testing.T) {
	initMLTest(t, 200)

	labeled := globalTrainingStore.LabeledSamples()
	samples := make([][FeatureDim]float64, len(labeled))
	labels := make([]int32, len(labeled))
	for i, s := range labeled {
		samples[i] = s.Features
		labels[i] = s.Label
	}

	model := NewLogisticModel(0.01, "l2", 300)
	model.NumClasses = 4
	model.Train(samples, labels)

	path := tmpModelPath(t, "lr_test")
	if err := model.Serialize(path); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	loaded, err := DeserializeLogistic(path)
	if err != nil {
		t.Fatalf("deserialize: %v", err)
	}
	if loaded.NumClasses != model.NumClasses {
		t.Fatalf("class count mismatch: %d vs %d", loaded.NumClasses, model.NumClasses)
	}

	// Verify predictions match
	match := 0
	for i := 0; i < 20; i++ {
		orig := model.Predict(samples[i])
		reloaded := loaded.Predict(samples[i])
		if orig.Action == reloaded.Action {
			match++
		}
	}
	if match < 18 {
		t.Errorf("prediction mismatch: %d/20 match", match)
	}
	t.Logf("Logistic roundtrip OK: classes=%d, match=%d/20", loaded.NumClasses, match)
}

// ── Model Registry ──────────────────────────────────────────────────

func TestModelRegistry(t *testing.T) {
	for _, mt := range AllModelTypes() {
		m, err := NewModel(mt)
		if err != nil {
			t.Fatalf("NewModel(%s): %v", mt, err)
		}
		if m == nil {
			t.Fatalf("NewModel(%s) returned nil", mt)
		}
		if m.Type() != mt {
			t.Fatalf("type mismatch: expected %s, got %s", mt, m.Type())
		}
		t.Logf("Model %s: type=%s ✓", modelName(mt), m.Type())
	}

	_, err := NewModel("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown model type")
	}
}

func TestModelTypeNames(t *testing.T) {
	if modelName(ModelRandomForest) == string(ModelRandomForest) {
		t.Error("modelName should return human-readable name, not the constant")
	}
	types := AllModelTypes()
	if len(types) < 3 {
		t.Fatalf("expected at least 3 model types, got %d", len(types))
	}
}

func TestBuiltinModelCatalogCoversAllModelTypes(t *testing.T) {
	types := AllModelTypes()
	catalog := BuiltinModelCatalog()
	if len(types) < 30 {
		t.Fatalf("expected at least 30 built-in model profiles, got %d", len(types))
	}
	if len(catalog) != len(types) {
		t.Fatalf("catalog/model type mismatch: %d catalog vs %d model types", len(catalog), len(types))
	}
	seen := make(map[string]bool, len(catalog))
	for _, item := range catalog {
		if item.Value == "" || item.Label == "" || item.Base == "" || item.Category == "" {
			t.Fatalf("incomplete catalog item: %+v", item)
		}
		seen[item.Value] = true
	}
	for _, mt := range types {
		if !seen[string(mt)] {
			t.Fatalf("model type %s missing from built-in catalog", mt)
		}
	}
}

// ── Training Data Store ─────────────────────────────────────────────

func TestTrainingStoreAddAndQuery(t *testing.T) {
	store := initTestStore(500)
	total, labeled := store.Status()
	t.Logf("Store: total=%d, labeled=%d", total, labeled)
	if total < 1 {
		t.Fatal("store should have samples")
	}

	samples := store.LabeledSamples()
	if len(samples) != labeled {
		t.Fatalf("labeled count mismatch: %d vs %d", len(samples), labeled)
	}
}

func initTestStore(n int) *TrainingDataStore {
	InitTrainingStore(n + 10)
	rng := seedRand()
	for i := 0; i < n; i++ {
		var features [FeatureDim]float64
		for d := 0; d < FeatureDim; d++ {
			features[d] = rng.Float64()
		}
		globalTrainingStore.Add(TrainingSample{
			Features:    features,
			Label:       int32(i % 4),
			Comm:        "test",
			CommandLine: fmt.Sprintf("test-%d", i),
			Timestamp:   time.Now(),
			UserLabel:   "test",
		})
	}
	return globalTrainingStore
}

// ── Trainer with different model types ──────────────────────────────

func TestTrainerAllModelTypes(t *testing.T) {
	models := []struct {
		modelType ModelType
		nTrees    int
		maxDepth  int
		minLeaf   int
		balance   bool
	}{
		{ModelRandomForest, 5, 3, 2, false},
		{ModelRandomForestFast, 31, 8, 5, false},
		{ModelKNN, 5, 8, 0, false}, // K uses nTrees, distance uses maxDepth
		{ModelKNNDistance, 31, 8, 5, false},
		{ModelLogisticRegression, 10, 8, 500, true}, // LR=0.01, L2, 500 iters
		{ModelLogisticL1, 31, 8, 5, false},
		{ModelSVM, 5, 8, 500, false},
		{ModelRidge, 5, 8, 5, false},
		{ModelPerceptron, 20, 8, 500, false},
		{ModelPassiveAggressive, 10, 8, 500, false},
		{ModelNearestCentroid, 0, 4, 0, true}, // cosine + uniform-prior path
		{ModelNearestCentroidCosine, 31, 8, 5, false},
		{ModelAdaBoostFast, 31, 8, 5, false},
		{ModelEnsemble, 31, 8, 5, false},
	}

	for _, tc := range models {
		t.Run(string(tc.modelType), func(t *testing.T) {
			initMLTest(t, 300)
			cfg := DefaultMLConfig()
			cfg.ModelType = tc.modelType
			cfg.NumTrees = tc.nTrees
			cfg.MaxDepth = tc.maxDepth
			cfg.MinSamplesLeaf = tc.minLeaf
			cfg.BalanceClasses = tc.balance
			mlConfig = cfg

			globalTrainer.ResetCancel()
			model, result := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
			if result.Error != "" {
				t.Fatalf("TrainWithConfig failed: %s", result.Error)
			}
			if model == nil {
				t.Fatal("nil model returned")
			}
			if model.Type() != tc.modelType {
				t.Fatalf("wrong type: %s vs %s", model.Type(), tc.modelType)
			}

			// Test prediction doesn't hang
			labeled := globalTrainingStore.LabeledSamples()
			pred := model.Predict(labeled[0].Features)
			t.Logf("%s: accuracy=%.2f%%, pred action=%d conf=%.3f",
				model.Type(), result.Accuracy*100, pred.Action, pred.Confidence)

			// Serialize roundtrip
			path := tmpModelPath(t, string(tc.modelType)+"_trainer")
			if err := model.Serialize(path); err != nil {
				t.Fatalf("serialize: %v", err)
			}
			_, err := os.Stat(path)
			if err != nil {
				t.Fatalf("model file missing: %v", err)
			}
			loaded := tryLoadModel(path, tc.modelType)
			if loaded == nil {
				t.Fatalf("failed to reload %s model from %s", tc.modelType, path)
			}
			if loaded.Type() != tc.modelType {
				t.Fatalf("loaded wrong type: %s vs %s", loaded.Type(), tc.modelType)
			}
			t.Logf("model file: %s exists and reloads", path)
		})
	}
}

func TestMLCRuntimeStatusForForest(t *testing.T) {
	initMLTest(t, 300)
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRandomForest
	cfg.NumTrees = 5
	cfg.MaxDepth = 4
	cfg.MinSamplesLeaf = 2

	globalTrainer.ResetCancel()
	model, result := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
	if result.Error != "" {
		t.Fatalf("TrainWithConfig failed: %s", result.Error)
	}
	if model == nil {
		t.Fatal("nil model returned")
	}

	mlCRuntimeMu.Lock()
	mlCRuntimeAt = time.Time{}
	mlCRuntimeKey = ""
	mlCRuntimeMu.Unlock()

	status := buildMLCRuntimeStatus(model, globalTrainingStore)
	if !status.Available {
		t.Fatal("C runtime status should be available")
	}
	if !status.CSupported {
		t.Fatalf("random forest should have a C inference kernel: %+v", status)
	}
	if status.SampleCount == 0 {
		t.Fatal("expected benchmark samples")
	}
	if status.GoMsPerSample <= 0 || status.CMsPerSample <= 0 {
		t.Fatalf("expected positive runtime numbers: go=%f c=%f", status.GoMsPerSample, status.CMsPerSample)
	}
	if len(status.Backends) < 3 {
		t.Fatalf("expected C CPU, CUDA, and Intel iGPU backend entries, got %d", len(status.Backends))
	}
	backendIDs := map[string]bool{}
	for _, backend := range status.Backends {
		backendIDs[backend.ID] = true
	}
	for _, id := range []string{"c_cpu", "cuda", "intel_igpu"} {
		if !backendIDs[id] {
			t.Fatalf("missing backend %s in %+v", id, status.Backends)
		}
	}
	t.Logf("C runtime: go=%.6fms c=%.6fms speedup=%.2fx backend=%s",
		status.GoMsPerSample, status.CMsPerSample, status.Speedup, status.ActiveBackend)
}

// ── Trainer edge cases ──────────────────────────────────────────────

func TestTrainerEmptyStore(t *testing.T) {
	InitTrainingStore(10)
	store := globalTrainingStore
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRandomForest

	globalTrainer.ResetCancel()
	_, result := globalTrainer.TrainWithConfig(store, cfg)
	if result.Error == "" {
		t.Fatal("expected error for empty store")
	}
	t.Logf("empty store correctly rejected: %s", result.Error)
}

func TestTrainerDuplicateRun(t *testing.T) {
	initMLTest(t, 200)
	cfg := DefaultMLConfig()
	cfg.ModelType = ModelRandomForest
	cfg.NumTrees = 3
	mlConfig = cfg

	globalTrainer.ResetCancel()
	// First training should succeed
	_, result := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
	if result.Error != "" {
		t.Fatalf("first train failed: %s", result.Error)
	}

	// Second should also work (mutex is released by defer)
	globalTrainer.ResetCancel()
	_, result2 := globalTrainer.TrainWithConfig(globalTrainingStore, cfg)
	if result2.Error != "" {
		t.Fatalf("second train failed: %s", result2.Error)
	}
	t.Log("consecutive training runs OK")
}

// ---- merged from osenforcementobjects_test.go ----

func TestCgroupSandboxObjectSections(t *testing.T) {
	spec, err := bpf.LoadAgentCgroupSandbox()
	if err != nil {
		t.Fatalf("load cgroup sandbox spec: %v", err)
	}

	assertProgramSpec(t, spec, "cgroup_sandbox_connect4", ebpf.CGroupSockAddr, ebpf.AttachCGroupInet4Connect, "cgroup/connect4")
	assertProgramSpec(t, spec, "cgroup_sandbox_connect6", ebpf.CGroupSockAddr, ebpf.AttachCGroupInet6Connect, "cgroup/connect6")
	assertProgramSpec(t, spec, "cgroup_sandbox_sendmsg4", ebpf.CGroupSockAddr, ebpf.AttachCGroupUDP4Sendmsg, "cgroup/sendmsg4")
	assertProgramSpec(t, spec, "cgroup_sandbox_sendmsg6", ebpf.CGroupSockAddr, ebpf.AttachCGroupUDP6Sendmsg, "cgroup/sendmsg6")
	assertMapSpec(t, spec, "cgroup_blocklist", ebpf.Hash, 256, 8, 4)
	assertMapSpec(t, spec, "ip_blocklist", ebpf.Hash, 1024, 4, 4)
	assertMapSpec(t, spec, "ip6_blocklist", ebpf.Hash, 1024, 16, 4)
	assertMapSpec(t, spec, "port_blocklist", ebpf.Hash, 256, 4, 4)
	assertMapSpec(t, spec, "cgroup_sandbox_stats", ebpf.PerCPUArray, 1, 4, 24)
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect4", "cgroup_sandbox_stats")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "ip6_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_connect6", "cgroup_sandbox_stats")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg4", "cgroup_sandbox_stats")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "cgroup_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "ip_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "ip6_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "port_blocklist")
	assertProgramReferencesMap(t, spec, "cgroup_sandbox_sendmsg6", "cgroup_sandbox_stats")
}

func TestLsmEnforcerObjectSections(t *testing.T) {
	spec, err := bpf.LoadAgentLsmEnforcer()
	if err != nil {
		t.Fatalf("load BPF LSM enforcer spec: %v", err)
	}

	assertProgramSpec(t, spec, "lsm_enforce_bprm_check", ebpf.LSM, ebpf.AttachLSMMac, "lsm/bprm_check_security")
	assertProgramSpec(t, spec, "lsm_enforce_file_open", ebpf.LSM, ebpf.AttachLSMMac, "lsm/file_open")
	assertProgramSpec(t, spec, "lsm_enforce_file_permission", ebpf.LSM, ebpf.AttachLSMMac, "lsm/file_permission")
	assertProgramSpec(t, spec, "lsm_enforce_mmap_file", ebpf.LSM, ebpf.AttachLSMMac, "lsm/mmap_file")
	assertProgramSpec(t, spec, "lsm_enforce_file_mprotect", ebpf.LSM, ebpf.AttachLSMMac, "lsm/file_mprotect")
	assertProgramSpec(t, spec, "lsm_enforce_inode_setattr", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_setattr")
	assertProgramSpec(t, spec, "lsm_enforce_inode_create", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_create")
	assertProgramSpec(t, spec, "lsm_enforce_inode_link", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_link")
	assertProgramSpec(t, spec, "lsm_enforce_inode_unlink", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_unlink")
	assertProgramSpec(t, spec, "lsm_enforce_inode_symlink", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_symlink")
	assertProgramSpec(t, spec, "lsm_enforce_inode_mkdir", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_mkdir")
	assertProgramSpec(t, spec, "lsm_enforce_inode_rmdir", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_rmdir")
	assertProgramSpec(t, spec, "lsm_enforce_inode_mknod", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_mknod")
	assertProgramSpec(t, spec, "lsm_enforce_inode_rename", ebpf.LSM, ebpf.AttachLSMMac, "lsm/inode_rename")
	assertMapSpec(t, spec, "lsm_blocked_exec_paths", ebpf.Hash, 512, 256, 4)
	assertMapSpec(t, spec, "lsm_blocked_exec_names", ebpf.Hash, 512, 64, 4)
	assertMapSpec(t, spec, "lsm_blocked_file_names", ebpf.Hash, 512, 64, 4)
	assertMapSpec(t, spec, "lsm_enforcer_stats_map", ebpf.PerCPUArray, 1, 4, 32)
	assertProgramReferencesMap(t, spec, "lsm_enforce_bprm_check", "lsm_blocked_exec_paths")
	assertProgramReferencesMap(t, spec, "lsm_enforce_bprm_check", "lsm_blocked_exec_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_bprm_check", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_open", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_open", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_permission", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_permission", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_mmap_file", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_mmap_file", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_mprotect", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_file_mprotect", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_setattr", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_setattr", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_create", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_create", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_link", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_link", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_unlink", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_unlink", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_symlink", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_symlink", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mkdir", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mkdir", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rmdir", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rmdir", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mknod", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_mknod", "lsm_enforcer_stats_map")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rename", "lsm_blocked_file_names")
	assertProgramReferencesMap(t, spec, "lsm_enforce_inode_rename", "lsm_enforcer_stats_map")
}

func TestCgroupSandboxPolicySourceUsesHostOrderKeys(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("ebpf", "cgroup_sandbox.c"))
	if err != nil {
		t.Fatalf("read cgroup sandbox source: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"bpf_get_current_cgroup_id()",
		"bpf_ntohl(ctx->user_ip4)",
		"bpf_ntohl(ctx->user_ip6[0])",
		"ip6_blocklist",
		"ipv6_is_v4_mapped",
		"mapped_v4_is_blocked",
		"::ffff:a.b.c.d",
		"bpf_ntohs(ctx->user_port)",
		"SEC(\"cgroup/sendmsg4\")",
		"SEC(\"cgroup/sendmsg6\")",
		"unconnected UDP sendto()/sendmsg() gap",
		"return 0; // block",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("cgroup sandbox source missing %q", want)
		}
	}
}

func TestLsmPolicyKeys(t *testing.T) {
	execKey, err := lsmPathKeyFromString("/usr/bin/nc")
	if err != nil {
		t.Fatalf("lsmPathKeyFromString: %v", err)
	}
	if got := stringFromNULBytes(execKey.Path[:]); got != "/usr/bin/nc" {
		t.Fatalf("exec key = %q", got)
	}

	fileKey, err := lsmNameKeyFromString("/home/agent/.ssh/id_rsa")
	if err != nil {
		t.Fatalf("lsmNameKeyFromString: %v", err)
	}
	if got := stringFromNULBytes(fileKey.Name[:]); got != "id_rsa" {
		t.Fatalf("file key = %q", got)
	}

	execNameKey, err := lsmExecNameKeyFromString("/tmp/agent-os-block")
	if err != nil {
		t.Fatalf("lsmExecNameKeyFromString: %v", err)
	}
	if got := stringFromNULBytes(execNameKey.Name[:]); got != "agent-os-block" {
		t.Fatalf("exec name key = %q", got)
	}

	if _, err := lsmPathKeyFromString(strings.Repeat("x", 256)); err == nil {
		t.Fatal("expected overlong exec path error")
	}
	if _, err := lsmNameKeyFromString(strings.Repeat("x", 64)); err == nil {
		t.Fatal("expected overlong file name error")
	}
}

func TestLsmPolicySourceUsesCurrentHookArguments(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("ebpf", "lsm_enforcer.c"))
	if err != nil {
		t.Fatalf("read BPF LSM source: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"SEC(\"lsm/file_permission\")",
		"SEC(\"lsm/mmap_file\")",
		"SEC(\"lsm/file_mprotect\")",
		"SEC(\"lsm/inode_setattr\")",
		"struct file *file, int mask, int ret",
		"struct file *file, unsigned long reqprot",
		"struct vm_area_struct *vma, unsigned long reqprot",
		"BPF_CORE_READ(vma, vm_file)",
		"struct mnt_idmap *idmap, struct dentry *dentry",
		"SEC(\"lsm/inode_rename\")",
		"struct inode *new_dir, struct dentry *new_dentry, int ret",
		"BPF_CORE_READ(bprm, file, f_path.dentry, d_name.name)",
		"if (ret != 0)",
		"return -EACCES;",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("BPF LSM source missing hook contract %q", want)
		}
	}
	mmapHookStart := strings.Index(source, "SEC(\"lsm/mmap_file\")")
	if mmapHookStart < 0 {
		t.Fatal("BPF LSM source missing mmap_file hook")
	}
	mmapFileRead := strings.Index(source[mmapHookStart:], "BPF_CORE_READ(file, f_path.dentry, d_name.name)")
	mmapFileNilCheck := strings.Index(source[mmapHookStart:], "if (!file)")
	if mmapFileNilCheck < 0 || mmapFileRead < 0 || mmapFileNilCheck > mmapFileRead {
		t.Fatal("BPF LSM mmap_file must null-check file before reading through it")
	}
	if strings.Contains(source, "struct inode *new_dir, struct dentry *new_dentry, unsigned int flags, int ret") {
		t.Fatal("BPF LSM inode_rename signature must match the current vmlinux hook and not read ret from the wrong ctx slot")
	}
	if strings.Count(source, "BPF_CORE_READ(old_dentry, d_name.name)") < 2 {
		t.Fatal("BPF LSM should check old_dentry basenames for both hard-link source protection and rename-away protection")
	}
	if strings.Count(source, "BPF_CORE_READ(new_dentry, d_name.name)") < 2 {
		t.Fatal("BPF LSM should check new_dentry basenames for both hard-link destination protection and rename-into protection")
	}
}

func TestOSSmokeScriptExists(t *testing.T) {
	path := filepath.Join("..", "scripts", "os-enforcement-smoke.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, want := range []string{
		"/sandbox/lsm/block-exec-path",
		"/sandbox/lsm/block-exec-name",
		"/sandbox/lsm/block-file-name",
		"/sandbox/cgroup/block-port",
		`"attached":true`,
		"OS_SMOKE_START_BACKEND",
		"OS_SMOKE_PRIVILEGE_CMD",
		"build_backend_privilege_prefix",
		"custom_privilege_prefix",
		"cleanup_policy_entries",
		"assert_idempotent_unblock()",
		"assert_idempotent_unblock /sandbox/lsm/unblock-exec-path",
		"assert_idempotent_unblock /sandbox/lsm/unblock-exec-name",
		"assert_idempotent_unblock /sandbox/lsm/unblock-file-name",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-cgroup",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-pid",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-ip",
		"assert_idempotent_unblock /sandbox/cgroup/unblock-port",
		"blocked_exec_paths",
		"blocked_exec_names",
		"blocked_file_names",
		"blocked_cgroups",
		"blocked_ips",
		"blocked_ports",
		"BPF LSM exec basename symlink-alias block",
		"agent-ebpf-lsm-exec-alias",
		"expected exec basename to block symlink alias execution",
		"BPF LSM file_open write block",
		"expected file write open to be blocked",
		"BPF LSM file_permission existing-fd block",
		"expected existing file descriptor I/O to be blocked",
		"BPF LSM mmap_file existing-fd block",
		"expected existing file descriptor mmap to be blocked",
		"BPF LSM file_mprotect existing-map block",
		"expected existing file mapping mprotect to be blocked",
		"start_existing_fd_setattr_probe",
		"BPF LSM inode_setattr existing-fd ftruncate block",
		"expected existing file descriptor ftruncate to be blocked",
		"start_existing_fd_fchmod_probe",
		"BPF LSM inode_setattr existing-fd fchmod block",
		"expected existing file descriptor fchmod to be blocked",
		"BPF LSM inode_setattr block",
		"BPF LSM inode_unlink block",
		"BPF LSM inode_rmdir block",
		"BPF LSM inode_rename block",
		"BPF LSM inode_rename destination block",
		"agent-ebpf-lsm-rename-dst",
		"expected rename into blocked destination basename to be blocked",
		"/sandbox/cgroup/block-pid",
		"cgroup.procs",
		"json_field cgroupPath",
		"run_cgroup_pid_block_smoke",
		"cgroup/connect PID cgroup unblock-pid",
		"expected PID-cgroup connect to succeed after unblock-pid",
		"run_ip_block_smoke 127.0.0.2 IPv4-loopback-alias",
		"run_ip_block_smoke ::1 IPv6-loopback",
		"run_port_block_smoke ::1 IPv6",
		"python_udp_connect",
		"start_udp_sendto_probe",
		"run_udp_sendto_probe_in_cgroup",
		"expected baseline PID-cgroup UDP sendto to succeed",
		"cgroup/sendmsg PID cgroup UDP sendto block",
		"expected PID-cgroup UDP sendto to be blocked",
		"expected PID-cgroup UDP sendto to succeed after unblock-pid",
		"run_udp_ip_block_smoke 127.0.0.2 IPv4-UDP-loopback-alias",
		"run_udp_port_block_smoke 127.0.0.1 IPv4-UDP",
		"run_udp_ip_block_smoke ::1 IPv6-UDP-loopback",
		"run_udp_port_block_smoke ::1 IPv6-UDP",
		"python_udp_sendto",
		"run_udp_sendto_ip_block_smoke 127.0.0.2 IPv4-UDP-sendto-loopback-alias",
		"run_udp_sendto_port_block_smoke 127.0.0.1 IPv4-UDP-sendto",
		"run_udp_sendto_ip_block_smoke ::1 IPv6-UDP-sendto-loopback",
		"run_udp_sendto_port_block_smoke ::1 IPv6-UDP-sendto",
		"start_connected_udp_send_probe",
		"run_udp_existing_connected_send_ip_block_smoke 127.0.0.2 IPv4-UDP-existing-connected-loopback-alias",
		"run_udp_existing_connected_send_port_block_smoke 127.0.0.1 IPv4-UDP-existing-connected",
		"run_udp_existing_connected_send_ip_block_smoke ::1 IPv6-UDP-existing-connected-loopback",
		"run_udp_existing_connected_send_port_block_smoke ::1 IPv6-UDP-existing-connected",
		"ipv4_mapped_loopback_available",
		"run_ipv4_mapped_ip_block_smoke",
		"run_udp_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-loopback",
		"run_udp_sendto_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-sendto-loopback",
		"run_udp_existing_connected_send_ip_block_smoke ::ffff:127.0.0.1 IPv4-mapped-IPv6-UDP-existing-connected-loopback",
		"expected existing UDP connected-socket",
		"BPF LSM inode_create block",
		"BPF LSM inode_link block",
		"BPF LSM inode_link source block",
		"agent-ebpf-lsm-link-src-blocked",
		"expected hard link from blocked source basename to be blocked",
		"BPF LSM inode_symlink block",
		"BPF LSM inode_mkdir block",
		"BPF LSM inode_mknod block",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s missing %s", path, want)
		}
	}
}

func TestOSPreflightScriptExists(t *testing.T) {
	path := filepath.Join("..", "scripts", "os-enforcement-preflight.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, want := range []string{
		"/sys/fs/bpf",
		"/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps",
		"/sys/fs/cgroup/cgroup.controllers",
		"AGENT_CGROUP_SANDBOX_PATH",
		"OS_SMOKE_PRIVILEGE_CMD",
		"cgroup.procs",
		"cgroup2fs",
		"can create a temporary cgroup below the sandbox attach path",
		"run_privileged_preflight",
		"ip6_blocklist",
		"/sys/kernel/security/lsm",
		"pinned maps include expected entries",
		"running as root; sudo is not required",
		"sudo -n true",
		"custom OS_SMOKE_PRIVILEGE_CMD runs commands as root",
		"writable through passwordless sudo",
		"writable through OS_SMOKE_PRIVILEGE_CMD",
		"curl is available",
		"python3 is available",
		"check_object_section",
		"cgroup2fs",
		"cgroup.procs",
		"cgroup/connect6",
		"cgroup/sendmsg4",
		"cgroup/sendmsg6",
		"lsm/file_permission",
		"lsm/mmap_file",
		"lsm/file_mprotect",
		"lsm/inode_setattr",
		"lsm/inode_create",
		"lsm/inode_link",
		"lsm/inode_unlink",
		"lsm/inode_symlink",
		"lsm/inode_mkdir",
		"lsm/inode_rmdir",
		"lsm/inode_mknod",
		"lsm/inode_rename",
		"os-enforcement-smoke-start",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("%s missing %s", path, want)
		}
	}
}

func TestCgroupPIDResolutionHelpers(t *testing.T) {
	rel, err := unifiedCgroupRelativePath([]byte("12:cpu:/legacy\n0::/user.slice/test.scope\n"))
	if err != nil {
		t.Fatalf("unifiedCgroupRelativePath: %v", err)
	}
	if rel != "/user.slice/test.scope" {
		t.Fatalf("unified cgroup path = %q", rel)
	}

	root := t.TempDir()
	resolved, err := resolveCgroupPath(root, "/user.slice/test.scope")
	if err != nil {
		t.Fatalf("resolveCgroupPath: %v", err)
	}
	if want := filepath.Join(root, "user.slice", "test.scope"); resolved != want {
		t.Fatalf("resolved path = %q, want %q", resolved, want)
	}

	if got, err := resolveCgroupPath(root, "/"); err != nil || got != root {
		t.Fatalf("root cgroup path = %q, %v; want %q", got, err, root)
	}

	if got := ipv4StringFromBlockKey(0x7f000001); got != "127.0.0.1" {
		t.Fatalf("ipv4StringFromBlockKey = %q", got)
	}
	ip6Key, err := ip6BlockKeyFromIP(net.ParseIP("2001:db8::1"))
	if err != nil {
		t.Fatalf("ip6BlockKeyFromIP: %v", err)
	}
	if got := ip6StringFromBlockKey(ip6Key); got != "2001:db8::1" {
		t.Fatalf("ip6StringFromBlockKey = %q", got)
	}

	if got, err := parseCgroupID([]byte(`"18446744073709551615"`)); err != nil || got != ^uint64(0) {
		t.Fatalf("parse string cgroup id = %d, %v", got, err)
	}
	if got, err := parseCgroupID([]byte(`12345`)); err != nil || got != 12345 {
		t.Fatalf("parse numeric cgroup id = %d, %v", got, err)
	}
	if _, err := parseCgroupID([]byte(`0`)); err == nil {
		t.Fatal("expected zero cgroup id to be rejected")
	}
}

func TestCgroupSandboxAttachPathValidation(t *testing.T) {
	temp := t.TempDir()
	if err := validateCgroupSandboxAttachPath(temp); err == nil {
		t.Fatal("expected non-cgroup attach path to be rejected")
	}

	if st, err := os.Stat("/sys/fs/cgroup"); err == nil && st.IsDir() {
		err := validateCgroupSandboxAttachPath("/sys/fs/cgroup")
		if err != nil && strings.Contains(err.Error(), "not on a cgroup v2 filesystem") {
			t.Fatalf("/sys/fs/cgroup should be recognized as cgroup v2 when mounted: %v", err)
		}
	}
}

func TestCgroupSandboxPortValidation(t *testing.T) {
	if err := validateCgroupSandboxPort(1); err != nil {
		t.Fatalf("port 1 should be valid: %v", err)
	}
	if err := validateCgroupSandboxPort(65535); err != nil {
		t.Fatalf("port 65535 should be valid: %v", err)
	}
	if err := validateCgroupSandboxPort(0); err == nil {
		t.Fatal("port 0 should be rejected")
	}

	data, err := os.ReadFile(filepath.Join("cgroup", "sandbox", "handlers.go"))
	if err != nil {
		t.Fatalf("read cgroup/sandbox/handlers.go: %v", err)
	}
	source := string(data)
	for _, want := range []string{
		"validateCgroupSandboxPort(req.Port)",
		"c.JSON(http.StatusBadRequest",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("port handlers missing %q", want)
		}
	}
}

func TestCgroupSandboxIPValidation(t *testing.T) {
	if ip, text, err := parseCgroupSandboxIP(" ::1 "); err != nil || text != "::1" || ip.To16() == nil {
		t.Fatalf("parse IPv6 = %v %q %v, want ::1", ip, text, err)
	}
	if ip, text, err := parseCgroupSandboxIP(" ::ffff:127.0.0.1 "); err != nil || text != "127.0.0.1" || ip.To4() == nil {
		t.Fatalf("parse IPv4-mapped IPv6 = %v %q %v, want canonical 127.0.0.1", ip, text, err)
	}
	for _, fn := range []struct {
		name string
		call func(string) error
	}{
		{name: "parse", call: func(s string) error {
			_, _, err := parseCgroupSandboxIP(s)
			return err
		}},
		{name: "block", call: blockIP},
		{name: "unblock", call: unblockIP},
	} {
		if err := fn.call("not-an-ip"); err == nil {
			t.Fatalf("%s accepted invalid IP", fn.name)
		}
	}
}

func TestOSEnforcementMutationHandlersRejectInvalidInputBeforeLoad(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		body    string
	}{
		{name: "cgroup block pid", handler: handleCgroupSandboxBlockPID, body: `{"pid":0}`},
		{name: "cgroup unblock pid", handler: handleCgroupSandboxUnblockPID, body: `{"pid":0}`},
		{name: "cgroup block missing pid", handler: handleCgroupSandboxBlockPID, body: `{"pid":2147483647}`},
		{name: "cgroup unblock missing pid", handler: handleCgroupSandboxUnblockPID, body: `{"pid":2147483647}`},
		{name: "lsm block exec path", handler: handleLsmBlockExecPath, body: `{"path":""}`},
		{name: "lsm unblock exec path", handler: handleLsmUnblockExecPath, body: `{"path":""}`},
		{name: "lsm block exec name", handler: handleLsmBlockExecName, body: `{"name":"/"}`},
		{name: "lsm unblock exec name", handler: handleLsmUnblockExecName, body: `{"name":"/"}`},
		{name: "lsm block file name", handler: handleLsmBlockFileName, body: `{"name":""}`},
		{name: "lsm unblock file name", handler: handleLsmUnblockFileName, body: `{"name":""}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			c.Request.Header.Set("Content-Type", "application/json")
			tc.handler(c)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body = %s, want 400", w.Code, w.Body.String())
			}
		})
	}
}

func TestOSPolicyMapPinsAreRestrictive(t *testing.T) {
	if cgroupSandboxMapPinMode != 0600 {
		t.Fatalf("cgroup sandbox map pin mode = %v, want 0600", cgroupSandboxMapPinMode)
	}
	if lsmEnforcerMapPinMode != 0600 {
		t.Fatalf("BPF LSM map pin mode = %v, want 0600", lsmEnforcerMapPinMode)
	}
}

func TestOSEnforcementStartsWithoutDefaultBlockEntries(t *testing.T) {
	for _, path := range []string{"main.go", filepath.Join("cgroup", "sandbox", "control.go"), filepath.Join("lsm", "enforcer", "control.go")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "autoBlockHighRiskEndpoints") || strings.Contains(string(data), "highRiskPorts") {
			t.Fatalf("%s installs implicit OS block entries; OS enforcement should start from explicit UI/API map entries", path)
		}
	}
}

func TestOSEnforcementMutationRoutesArePolicyGated(t *testing.T) {
	data, err := os.ReadFile("zz_merged_backend.go")
	if err != nil {
		t.Fatalf("read zz_merged_backend.go: %v", err)
	}
	source := string(data)
	for _, route := range []string{
		"/sandbox/cgroup/block-cgroup",
		"/sandbox/cgroup/unblock-cgroup",
		"/sandbox/cgroup/block-pid",
		"/sandbox/cgroup/unblock-pid",
		"/sandbox/cgroup/block-ip",
		"/sandbox/cgroup/unblock-ip",
		"/sandbox/cgroup/block-port",
		"/sandbox/cgroup/unblock-port",
		"/sandbox/lsm/block-exec-path",
		"/sandbox/lsm/unblock-exec-path",
		"/sandbox/lsm/block-exec-name",
		"/sandbox/lsm/unblock-exec-name",
		"/sandbox/lsm/block-file-name",
		"/sandbox/lsm/unblock-file-name",
	} {
		want := `r.POST("` + route + `", authMiddleware(), policyManagementEnabledMiddleware(),`
		if !strings.Contains(source, want) {
			t.Fatalf("route %s is not registered with auth + policy management gate", route)
		}
	}
}

func TestOSEnforcementStatusRoutesRequireAuth(t *testing.T) {
	data, err := os.ReadFile("zz_merged_backend.go")
	if err != nil {
		t.Fatalf("read zz_merged_backend.go: %v", err)
	}
	source := string(data)
	for _, route := range []string{
		"/sandbox/cgroup/status",
		"/sandbox/lsm/status",
	} {
		want := `r.GET("` + route + `", authMiddleware(),`
		if !strings.Contains(source, want) {
			t.Fatalf("status route %s is not registered with auth middleware", route)
		}
	}

	for _, check := range []struct {
		path string
		want string
	}{
		{path: filepath.Join("..", "AGENTS.md"), want: "/sandbox/**"},
		{path: filepath.Join("..", "README.md"), want: "OS sandbox (`/sandbox/**`)"},
	} {
		doc, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		if !strings.Contains(string(doc), check.want) {
			t.Fatalf("%s missing auth coverage note %q", check.path, check.want)
		}
	}
}

func TestOSSecurityDocsDescribeCurrentKernelEnforcement(t *testing.T) {
	checks := []struct {
		path      string
		required  []string
		forbidden []string
	}{
		{
			path: filepath.Join("..", "docs", "threat-model.md"),
			required: []string{
				"Current OS-level enforcement focus",
				"cgroup/connect and cgroup/sendmsg programs can reject",
				"existing connected UDP sends",
				"IPv4-mapped IPv6 destinations",
				"BPF LSM programs can reject",
				"existing-fd `ftruncate` / `fchmod` via `setattr`",
				"not recursive workspace sandboxes",
				"escape defenses",
			},
		},
		{
			path: filepath.Join("..", "docs", "security-model.md"),
			required: []string{
				"`/sandbox/**`",
				"Kernel-enforced policy paths",
				"cgroup/connect and cgroup/sendmsg blocking for exact cgroup ids, IPv4/IPv6 destinations, and",
				"existing connected UDP send",
				"IPv4 block entries are also honored for IPv4-mapped IPv6 socket",
				"BPF LSM blocking for executable paths, executable basenames, and file or",
				"`file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`",
				"`inode_mknod`, and `inode_rename`",
				"existing-fd `ftruncate` / `fchmod`",
				"own cgroup / LSM policy-map mutation and attach lifecycle",
			},
			forbidden: []string{
				"apply future cgroup / LSM policy",
			},
		},
		{
			path: filepath.Join("..", "docs", "policy-semantics.md"),
			required: []string{
				"OS-level cgroup/connect + sendmsg policy",
				"TCP/UDP destination port",
				"unconnected UDP sendto/sendmsg",
				"existing connected UDP sends",
				"IPv4 block entries also deny IPv4-mapped IPv6 destinations",
				"API inputs in that form normalize to the equivalent IPv4 block key",
				"Existing TCP streams established before a matching block is added are not",
				"Existing-fd `ftruncate` / `fchmod`-style",
				"OS-level BPF LSM policy",
				"Matching LSM decisions return `EACCES`",
				"File/directory LSM matching is basename-based today",
				"not in the synchronous cgroup/LSM decision path",
			},
			forbidden: []string{
				"not kernel-enforced policy decisions yet.",
				"optional kernel blocking via cgroup hooks or BPF LSM",
			},
		},
		{
			path: filepath.Join("..", "README.md"),
			required: []string{
				"`GET /sandbox/cgroup/status`",
				"`checked` / `blocked` / `allowed`",
				"legacy `connect*` aliases",
				"IPv4-mapped IPv6-destination",
				"existing connected UDP sends",
			},
		},
		{
			path: "README.md",
			required: []string{
				"`GET /sandbox/cgroup/status` returns",
				"decision counters as `checked` / `blocked` / `allowed` plus legacy `connect*` aliases",
				"IPv4-mapped IPv6-destination",
				"existing connected UDP sends",
			},
		},
	}

	for _, check := range checks {
		data, err := os.ReadFile(check.path)
		if err != nil {
			t.Fatalf("read %s: %v", check.path, err)
		}
		doc := string(data)
		for _, want := range check.required {
			if !strings.Contains(doc, want) {
				t.Fatalf("%s missing OS enforcement doc marker %q", check.path, want)
			}
		}
		for _, bad := range check.forbidden {
			if strings.Contains(doc, bad) {
				t.Fatalf("%s still contains stale OS enforcement wording %q", check.path, bad)
			}
		}
	}
}

func TestOSFrontendSecuritySurfaceWiresSandboxEndpoints(t *testing.T) {
	composablePath := filepath.Join("..", "frontend", "src", "composables", "config", "useConfigSecurity.ts")
	composableData, err := os.ReadFile(composablePath)
	if err != nil {
		t.Fatalf("read %s: %v", composablePath, err)
	}
	composable := string(composableData)
	for _, want := range []string{
		"axios.get(\"/sandbox/cgroup/status\")",
		"\"/sandbox/cgroup/block-cgroup\"",
		"\"/sandbox/cgroup/unblock-cgroup\"",
		"\"/sandbox/cgroup/block-pid\"",
		"\"/sandbox/cgroup/unblock-pid\"",
		"\"/sandbox/cgroup/block-ip\"",
		"\"/sandbox/cgroup/unblock-ip\"",
		"\"/sandbox/cgroup/block-port\"",
		"\"/sandbox/cgroup/unblock-port\"",
		"axios.get(\"/sandbox/lsm/status\")",
		"\"/sandbox/lsm/block-exec-path\"",
		"\"/sandbox/lsm/unblock-exec-path\"",
		"\"/sandbox/lsm/block-exec-name\"",
		"\"/sandbox/lsm/unblock-exec-name\"",
		"\"/sandbox/lsm/block-file-name\"",
		"\"/sandbox/lsm/unblock-file-name\"",
		"blockedCgroups:",
		"blockedIPs:",
		"ip6Blocklist:",
		"blockedPorts:",
		"checked: 0",
		"blocked: 0",
		"allowed: 0",
		"blockedExecPaths:",
		"blockedExecNames:",
		"blockedFileNames:",
		"请输入 IPv4、IPv6 或 IPv4-mapped IPv6 地址",
		"CgroupSandboxSuccessText",
		"axios.post<CgroupSandboxActionResponse>",
		"data.ip || ip",
		"打开/读写/mmap/mprotect/setattr/创建/link/symlink/删除/mkdir/rmdir/mknod/rename basename",
		"fetchCgroupSandboxStatus,",
		"fetchLsmEnforcerStatus,",
	} {
		if !strings.Contains(composable, want) {
			t.Fatalf("%s missing frontend sandbox contract %q", composablePath, want)
		}
	}

	componentPath := filepath.Join("..", "frontend", "src", "components", "config", "ConfigSecurityTab.vue")
	componentData, err := os.ReadFile(componentPath)
	if err != nil {
		t.Fatalf("read %s: %v", componentPath, err)
	}
	component := string(componentData)
	for _, want := range []string{
		"OS-Level cgroup Network Interception",
		"TCP/UDP connected sockets",
		"UDP sendto/sendmsg",
		"IPv4-mapped IPv6 socket",
		"OS-Level BPF LSM File / Exec Interception",
		"1.2.3.4, ::ffff:1.2.3.4, or ::1",
		"cgroupSandboxStatus.stats.checked",
		"cgroupSandboxStatus.stats.blocked",
		"cgroupSandboxStatus.stats.allowed",
		"file_open",
		"file_permission",
		"mmap_file",
		"file_mprotect",
		"inode_setattr",
		"inode_create",
		"inode_link",
		"inode_unlink",
		"inode_symlink",
		"inode_mkdir",
		"inode_rmdir",
		"inode_mknod",
		"inode_rename",
		"打开、既有 fd 读写、mmap、mprotect、setattr、创建、link、symlink、删除、mkdir、rmdir、mknod 与 rename",
		"@click=\"blockCgroupID\"",
		"@click=\"blockCgroupPID\"",
		"@click=\"blockCgroupIP\"",
		"@click=\"blockCgroupPort\"",
		"@close.prevent=\"unblockCgroupIDFromTag(id)\"",
		"@close.prevent=\"unblockCgroupIPFromTag(ip)\"",
		"@close.prevent=\"unblockCgroupPortFromTag(port)\"",
		"@click=\"blockLsmExecPath\"",
		"@click=\"blockLsmExecName\"",
		"@click=\"blockLsmFileName\"",
		"@close.prevent=\"unblockLsmExecPath(path)\"",
		"@close.prevent=\"unblockLsmExecName(name)\"",
		"@close.prevent=\"unblockLsmFileName(name)\"",
	} {
		if !strings.Contains(component, want) {
			t.Fatalf("%s missing UI sandbox control %q", componentPath, want)
		}
	}

	viewPath := filepath.Join("..", "frontend", "src", "views", "config", "Config.vue")
	viewData, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatalf("read %s: %v", viewPath, err)
	}
	view := string(viewData)
	for _, want := range []string{
		"fetchCgroupSandboxStatus",
		"fetchLsmEnforcerStatus",
		"onMounted(() =>",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("%s missing Config page sandbox startup hook %q", viewPath, want)
		}
	}
}

func TestPinnedOSEnforcementPolicyIsPreservedOnReuseFailure(t *testing.T) {
	checks := []struct {
		paths    []string
		required []string
	}{
		{
			paths: []string{filepath.Join("cgroup", "sandbox", "control.go")},
			required: []string{
				"Preserve existing pinned policy maps",
				"link.LoadPinnedLink",
				"updatePinnedCgroupSandboxLinks",
				"ensureCgroupSandboxPinnedMapCompatibility",
				"if len(links) >= 4",
			},
		},
		{
			paths: []string{filepath.Join("lsm", "enforcer", "bootstrap.go")},
			required: []string{
				"Preserve pinned LSM policy maps",
				"link.LoadPinnedLink",
				"updatePinnedLsmEnforcerLinks",
				"if len(links) >= expectedLsmEnforcerLinks",
			},
		},
	}
	for _, check := range checks {
		source := readSourceFiles(t, check.paths...)
		if strings.Contains(source, "retrying fresh bootstrap") || strings.Contains(source, "fresh bootstrap:") {
			t.Fatalf("%s can fresh-bootstrap after pinned-map reuse failure; this risks deleting explicit OS policy pins", strings.Join(check.paths, ", "))
		}
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing %q", strings.Join(check.paths, ", "), want)
			}
		}
	}
}
func TestOSEnforcementAttachFailureCleansPartialPins(t *testing.T) {
	checks := []struct {
		paths    []string
		required []string
	}{
		{
			paths: []string{filepath.Join("cgroup", "sandbox", "control.go")},
			required: []string{
				"closeLinksAndRemovePins(links, pins)",
				"func closeLinksAndRemovePins",
				"os.Remove(pin)",
			},
		},
		{
			paths: []string{filepath.Join("lsm", "enforcer", "bootstrap.go")},
			required: []string{
				"closeLinksAndRemovePins(links, pins)",
			},
		},
	}
	for _, check := range checks {
		source := readSourceFiles(t, check.paths...)
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing partial-attach cleanup %q", strings.Join(check.paths, ", "), want)
			}
		}
	}
}
func TestOSEnforcementUnblockIgnoresMissingMapKeys(t *testing.T) {
	if err := ignoreMissingMapKey(ebpf.ErrKeyNotExist); err != nil {
		t.Fatalf("missing map key should be idempotent: %v", err)
	}
	sentinel := errors.New("sentinel")
	if err := ignoreMissingMapKey(sentinel); !errors.Is(err, sentinel) {
		t.Fatalf("non-missing map error = %v, want sentinel", err)
	}

	checks := []struct {
		paths    []string
		required []string
	}{
		{
			paths: []string{filepath.Join("cgroup", "sandbox", "ops.go")},
			required: []string{
				"ignoreMissingMapKey(snap.CgroupBlocklist.Delete",
				"ignoreMissingMapKey(snap.IPBlocklist.Delete",
				"ignoreMissingMapKey(snap.IP6Blocklist.Delete",
				"ignoreMissingMapKey(snap.PortBlocklist.Delete",
			},
		},
		{
			paths: []string{filepath.Join("lsm", "enforcer", "control.go")},
			required: []string{
				"ignoreMissingMapKey(snap.ExecPathBlocklist.Delete",
				"ignoreMissingMapKey(snap.ExecNameBlocklist.Delete",
				"ignoreMissingMapKey(snap.FileNameBlocklist.Delete",
			},
		},
	}
	for _, check := range checks {
		source := readSourceFiles(t, check.paths...)
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing idempotent unblock wrapper %q", strings.Join(check.paths, ", "), want)
			}
		}
	}
}
func TestOSEnforcementStatusUsesRuntimeSnapshots(t *testing.T) {
	checks := []struct {
		paths    []string
		required []string
	}{
		{
			paths: []string{
				filepath.Join("cgroup", "sandbox", "control.go"),
				filepath.Join("cgroup", "sandbox", "handlers.go"),
				filepath.Join("cgroup", "sandbox", "ops.go"),
			},
			required: []string{
				"sync.RWMutex",
				"currentCgroupSandboxSnapshot",
				"listBlockedCgroups(snap.CgroupBlocklist)",
				"getCgroupSandboxStats(snap.SandboxStats)",
				"`json:\"checked\"`",
				"total.Checked = total.ConnectChecked",
				"len(cgroupSandbox.Links) >= 4",
			},
		},
		{
			paths: []string{
				filepath.Join("lsm", "enforcer", "types.go"),
				filepath.Join("lsm", "enforcer", "control.go"),
			},
			required: []string{
				"sync.RWMutex",
				"currentLsmEnforcerSnapshot",
				"listLsmExecPaths(snap.ExecPathBlocklist)",
				"getLsmEnforcerStats(snap.Stats)",
				"len(lsmEnforcer.Links) >= expectedLsmEnforcerLinks",
			},
		},
	}
	for _, check := range checks {
		source := readSourceFiles(t, check.paths...)
		for _, want := range check.required {
			if !strings.Contains(source, want) {
				t.Fatalf("%s missing %q", strings.Join(check.paths, ", "), want)
			}
		}
	}
}
func readSourceFiles(t *testing.T, paths ...string) string {
	t.Helper()
	var builder strings.Builder
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func assertProgramSpec(t *testing.T, spec *ebpf.CollectionSpec, name string, typ ebpf.ProgramType, attach ebpf.AttachType, section string) {
	t.Helper()
	prog, ok := spec.Programs[name]
	if !ok {
		t.Fatalf("missing program %s", name)
	}
	if prog.Type != typ || prog.AttachType != attach || prog.SectionName != section {
		t.Fatalf("program %s = type %s attach %s section %q, want type %s attach %s section %q",
			name, prog.Type, prog.AttachType, prog.SectionName, typ, attach, section)
	}
	if len(prog.Instructions) == 0 {
		t.Fatalf("program %s has no instructions", name)
	}
}

func assertMapSpec(t *testing.T, spec *ebpf.CollectionSpec, name string, typ ebpf.MapType, maxEntries, keySize, valueSize uint32) {
	t.Helper()
	m, ok := spec.Maps[name]
	if !ok {
		t.Fatalf("missing map %s", name)
	}
	if m.Type != typ || m.MaxEntries != maxEntries || m.KeySize != keySize || m.ValueSize != valueSize {
		t.Fatalf("map %s = type %s max_entries %d key_size %d value_size %d, want type %s max_entries %d key_size %d value_size %d",
			name, m.Type, m.MaxEntries, m.KeySize, m.ValueSize, typ, maxEntries, keySize, valueSize)
	}
}

func assertProgramReferencesMap(t *testing.T, spec *ebpf.CollectionSpec, progName, mapName string) {
	t.Helper()
	prog, ok := spec.Programs[progName]
	if !ok {
		t.Fatalf("missing program %s", progName)
	}
	for _, ins := range prog.Instructions {
		if ins.Reference() == mapName {
			return
		}
	}
	t.Fatalf("program %s does not reference map %s", progName, mapName)
}

func stringFromNULBytes(b []byte) string {
	if idx := strings.IndexByte(string(b), 0); idx >= 0 {
		return string(b[:idx])
	}
	return string(b)
}

// ---- merged from probemanagertls_test.go ----

func TestFindFirstExistingPath(t *testing.T) {
	tmpDir := t.TempDir()
	existing := filepath.Join(tmpDir, "libssl.so")
	if err := os.WriteFile(existing, []byte(""), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, ok := findFirstExistingPath("/does/not/exist", existing, filepath.Join(tmpDir, "missing"))
	if !ok {
		t.Fatalf("expected to find existing path")
	}
	if got != existing {
		t.Fatalf("got path %q, want %q", got, existing)
	}

	if _, ok := findFirstExistingPath("/still/missing", filepath.Join(tmpDir, "also-missing")); ok {
		t.Fatalf("expected missing paths to return false")
	}
}

func TestTLSProgramForSymbol(t *testing.T) {
	tests := []struct {
		symbol  string
		program string
	}{
		{symbol: "SSL_write", program: "uprobe_ssl_write"},
		{symbol: "SSL_write_ex", program: "uprobe_ssl_write_ex"},
		{symbol: "SSL_read", program: "uprobe_ssl_read"},
		{symbol: "SSL_read_ex", program: "uprobe_ssl_read_ex"},
		{symbol: "gnutls_record_send", program: "uprobe_gnutls_record_send"},
		{symbol: "gnutls_record_recv", program: "uprobe_gnutls_record_recv"},
		{symbol: "PR_Write", program: "uprobe_pr_write"},
		{symbol: "PR_Read", program: "uprobe_pr_read"},
		{symbol: "crypto/tls.(*Conn).Write", program: "uprobe_crypto_tls_conn_write"},
		{symbol: "crypto/tls.(*Conn).Read", program: "uprobe_crypto_tls_conn_read"},
	}

	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			got, ok := tlsProgramForSymbol(tt.symbol)
			if !ok {
				t.Fatalf("expected program for symbol %q", tt.symbol)
			}
			if got != tt.program {
				t.Fatalf("got program %q, want %q", got, tt.program)
			}
		})
	}
}

func TestTLSReturnProgramForSymbol(t *testing.T) {
	tests := []struct {
		symbol  string
		program string
	}{
		{symbol: "SSL_read", program: "uretprobe_ssl_read"},
		{symbol: "SSL_read_ex", program: "uretprobe_ssl_read_ex"},
		{symbol: "SSL_write_ex", program: "uretprobe_ssl_write_ex"},
		{symbol: "gnutls_record_recv", program: "uretprobe_gnutls_record_recv"},
		{symbol: "PR_Read", program: "uretprobe_pr_read"},
		{symbol: "crypto/tls.(*Conn).Read", program: "uretprobe_crypto_tls_conn_read"},
	}
	for _, tt := range tests {
		t.Run(tt.symbol, func(t *testing.T) {
			got, ok := tlsReturnProgramForSymbol(tt.symbol)
			if !ok {
				t.Fatalf("expected return program for symbol %q", tt.symbol)
			}
			if got != tt.program {
				t.Fatalf("got return program %q, want %q", got, tt.program)
			}
		})
	}

	for _, symbol := range []string{"SSL_write", "gnutls_record_send", "PR_Write", "crypto/tls.(*Conn).Write"} {
		t.Run(symbol+" no return", func(t *testing.T) {
			if got, ok := tlsReturnProgramForSymbol(symbol); ok {
				t.Fatalf("return program for %q = %q, want none", symbol, got)
			}
		})
	}
}

func TestParseProcPID(t *testing.T) {
	pid, ok := parseProcPID("/proc/1234/exe")
	if !ok || pid != 1234 {
		t.Fatalf("pid = %d ok = %v", pid, ok)
	}

	if pid, ok := parseProcPID("/proc/self/exe"); ok || pid != 0 {
		t.Fatalf("self parsed as pid = %d ok = %v", pid, ok)
	}
}

func TestShouldAttachGoBinaryOnlyOncePerPIDPath(t *testing.T) {
	manager := &TLSProbeManager{attachedGo: make(map[string]bool)}
	if !manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("first attach should be allowed")
	}
	if manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("duplicate attach should be skipped")
	}
	if !manager.shouldAttachGoBinary("/tmp/app", 43) {
		t.Fatalf("different pid should be allowed")
	}
}

func TestForgetGoBinaryAttachAllowsRetryAfterFailure(t *testing.T) {
	manager := &TLSProbeManager{attachedGo: make(map[string]bool)}
	if !manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("first attach should be allowed")
	}
	manager.forgetGoBinaryAttach("/tmp/app", 42)
	if !manager.shouldAttachGoBinary("/tmp/app", 42) {
		t.Fatalf("attach should be retried after failure cleanup")
	}
}

func TestResolveShebangInterpreterUsesEnvArgument(t *testing.T) {
	interpreter := resolveShebangInterpreter("/usr/bin/env sh -c echo")
	if base := filepath.Base(interpreter); base != "sh" && base != "bash" {
		t.Fatalf("interpreter = %q, want a shell executable", interpreter)
	}
}

func TestResolveShebangInterpreterFallsBackToAbsoluteTarget(t *testing.T) {
	interpreter := resolveShebangInterpreter("/does/not/exist -S node")
	if interpreter != "/does/not/exist" {
		t.Fatalf("interpreter = %q, want absolute shebang target", interpreter)
	}
}

// ---- merged from recordingevent_test.go ----

func TestEventRecordingWritesAndReadsJSONL(t *testing.T) {
	path := t.TempDir() + "/events.jsonl"
	status, err := eventRecordingStore.Start(path, true)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer eventRecordingStore.Stop()
	if !status.Active {
		t.Fatalf("recording should be active")
	}
	eventRecordingStore.Record(CapturedEventRecord{
		ReceivedAt: time.Unix(1710000000, 0).UTC(),
		Event:      &pb.Event{Pid: 42, Ppid: 1, Comm: "codex", Type: "openat", Path: "/tmp/demo"},
	})
	status, err = eventRecordingStore.Stop()
	if err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if status.Count != 1 {
		t.Fatalf("recorded count = %d, want 1", status.Count)
	}

	records, err := readCapturedEventsFile(path, 100)
	if err != nil {
		t.Fatalf("readCapturedEventsFile() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("record count = %d, want 1", len(records))
	}
	if records[0].Event.GetPid() != 42 || records[0].Envelope == nil {
		t.Fatalf("unexpected replay record %#v", records[0])
	}
}

func TestSaveBrowserRecordingExport(t *testing.T) {
	path := t.TempDir() + "/browser-memory.json"
	payload := json.RawMessage(`{"version":1,"snapshots":[{"recordedAt":"now","graph":{"eventCount":1,"source":"browser_memory","nodes":[],"edges":[]}}]}`)
	gotPath, snapshots, err := saveBrowserRecordingExport(path, payload)
	if err != nil {
		t.Fatalf("saveBrowserRecordingExport() error = %v", err)
	}
	if gotPath != path {
		t.Fatalf("path = %q, want %q", gotPath, path)
	}
	if snapshots != 1 {
		t.Fatalf("snapshots = %d, want 1", snapshots)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !json.Valid(data) {
		t.Fatalf("saved payload is not valid JSON: %s", string(data))
	}
}

// ---- merged from remotedatasets_test.go ----

func TestParseRemoteDatasetRecordsJSONL(t *testing.T) {
	raw := []byte(`{"commandLine":"rm -rf /tmp/demo","label":"BLOCK"}
{"commandLine":"echo hello"}
`)

	records, format, err := parseRemoteDatasetRecords(raw, "auto", "inline.jsonl")
	if err != nil {
		t.Fatalf("parseRemoteDatasetRecords() error = %v", err)
	}
	if format != "jsonl" {
		t.Fatalf("format = %q, want jsonl", format)
	}
	if len(records) != 2 {
		t.Fatalf("records length = %d, want 2", len(records))
	}
	if records[0].Comm != "rm" || records[0].Label != "BLOCK" {
		t.Fatalf("first record = %#v", records[0])
	}
	if records[1].Comm != "echo" || strings.Join(records[1].Args, " ") != "hello" {
		t.Fatalf("second record = %#v", records[1])
	}
}

func TestPullRemoteDatasetFromHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"commandLine":"sudo systemctl disable firewalld","label":"ALERT"},
			{"commandLine":"ls -la /tmp","label":"ALLOW"}
		]`))
	}))
	defer srv.Close()

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		URL:       srv.URL,
		Format:    "auto",
		Limit:     10,
		LabelMode: "preserve",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Format != "json" {
		t.Fatalf("format = %q, want json", resp.Format)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Label != "ALERT" || resp.Rows[0].Comm != "sudo" {
		t.Fatalf("first row = %#v", resp.Rows[0])
	}
}

func TestPullRemoteDatasetFromClassicCSVWithCleaning(t *testing.T) {
	raw := []byte(`payload,length,attack_type,label
"c/ caridad s/n",14,norm,norm
"../etc/passwd",12,cmdi,anom
`)

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		Content:        string(raw),
		SourceName:     "HttpParamsDataset/payload_train.csv",
		Format:         "csv",
		Limit:          10,
		LabelMode:      "preserve",
		CleanSensitive: true,
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Format != "csv" {
		t.Fatalf("format = %q, want csv", resp.Format)
	}
	if len(resp.Rows) != 2 {
		t.Fatalf("rows length = %d, want 2", len(resp.Rows))
	}
	if resp.Rows[0].Label != "ALLOW" {
		t.Fatalf("first row label = %q, want ALLOW", resp.Rows[0].Label)
	}
	if resp.Rows[1].Label != "BLOCK" {
		t.Fatalf("second row label = %q, want BLOCK", resp.Rows[1].Label)
	}
	if strings.Contains(resp.Rows[1].CommandLine, "/etc/passwd") {
		t.Fatalf("sensitive path was not cleaned: %#v", resp.Rows[1])
	}
	if resp.Rows[1].Source != "HttpParamsDataset/payload_train.csv" {
		t.Fatalf("row source = %q, want dataset source", resp.Rows[1].Source)
	}
}

func TestPullRemoteDatasetRejectsHTMLLandingPage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html lang=en dir=ltr prefix=content: http://purl.org/rss/1.0/modules/content/ dc: http://purl.org/dc/terms/>
<head><title>Dataset</title></head>
<body>Download page</body>
</html>`))
	}))
	defer srv.Close()

	_, err := pullRemoteDataset(remoteDatasetRequest{
		URL:    srv.URL,
		Format: "auto",
		Limit:  10,
	})
	if err == nil {
		t.Fatalf("pullRemoteDataset() error = nil, want HTML landing page rejection")
	}
	if got := err.Error(); !strings.Contains(got, "HTML landing page") {
		t.Fatalf("error = %q, want HTML landing page rejection", got)
	}
}

func TestPullRemoteDatasetFromBase64ZipArchive(t *testing.T) {
	archiveBytes := buildZipArchive(t, map[string]string{
		"README.md":     "# Dataset\nThis is documentation and should be skipped.\n",
		"samples.jsonl": `{"commandLine":"rm -rf /tmp/demo","label":"BLOCK"}` + "\n" + `{"commandLine":"echo hello","label":"ALLOW"}` + "\n",
	})

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(archiveBytes),
		SourceName:    "classic.zip",
		Format:        "auto",
		Limit:         10,
		LabelMode:     "preserve",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Source != "classic.zip" {
		t.Fatalf("Source = %q, want classic.zip", resp.Source)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Comm != "rm" || resp.Rows[1].Comm != "echo" {
		t.Fatalf("rows = %#v %#v", resp.Rows[0], resp.Rows[1])
	}
}

func TestPullRemoteDatasetFromTarGzArchive(t *testing.T) {
	tarBytes := buildTarArchive(t, map[string]string{
		"commands.txt": "sudo systemctl disable firewalld\nls -la /tmp\n",
	})
	var gz bytes.Buffer
	gw := gzip.NewWriter(&gz)
	if _, err := gw.Write(tarBytes); err != nil {
		t.Fatalf("gzip write error = %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close error = %v", err)
	}

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(gz.Bytes()),
		SourceName:    "classic.tar.gz",
		Format:        "auto",
		Limit:         10,
		LabelMode:     "preserve",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Comm != "sudo" || resp.Rows[1].Comm != "ls" {
		t.Fatalf("rows = %#v %#v", resp.Rows[0], resp.Rows[1])
	}
}

func TestPullRemoteDatasetFromTarXzArchive(t *testing.T) {
	tarBytes := buildTarArchive(t, map[string]string{
		"commands.txt": "sudo systemctl disable firewalld\nls -la /tmp\n",
	})
	var xzBuf bytes.Buffer
	xw, err := xz.NewWriter(&xzBuf)
	if err != nil {
		t.Fatalf("xz writer error = %v", err)
	}
	if _, err := xw.Write(tarBytes); err != nil {
		t.Fatalf("xz write error = %v", err)
	}
	if err := xw.Close(); err != nil {
		t.Fatalf("xz close error = %v", err)
	}

	resp, err := pullRemoteDataset(remoteDatasetRequest{
		ContentBase64: base64.StdEncoding.EncodeToString(xzBuf.Bytes()),
		SourceName:    "classic.tar.xz",
		Format:        "auto",
		Limit:         10,
		LabelMode:     "preserve",
	})
	if err != nil {
		t.Fatalf("pullRemoteDataset() error = %v", err)
	}
	if resp.Total != 2 || len(resp.Rows) != 2 {
		t.Fatalf("response rows = %d/%d, want 2/2", len(resp.Rows), resp.Total)
	}
	if resp.Rows[0].Comm != "sudo" || resp.Rows[1].Comm != "ls" {
		t.Fatalf("rows = %#v %#v", resp.Rows[0], resp.Rows[1])
	}
}

func TestParseRemoteDatasetRecordsGTFOBinsAndLOLBAS(t *testing.T) {
	// GTFOBins style: real API shape uses top-level executables map.
	gtfoRaw := []byte(`{
		"functions": {
			"shell": { "label": "Shell" }
		},
		"contexts": {
			"sudo": { "label": "Sudo" }
		},
		"executables": {
			"7z": {
				"functions": {
					"file-read": [
						{ "code": "7z a -ttar -an -so /etc/shadow | 7z e -ttar -si -so" }
					]
				}
			},
			"comm": {
				"functions": {
					"shell": [
						{ "code": "comm /tmp/a /tmp/b" }
					]
				}
			}
		}
	}`)
	records, _, err := parseRemoteDatasetRecords(gtfoRaw, "auto", "GTFOBins")
	if err != nil {
		t.Fatalf("GTFOBins parse error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("GTFOBins record count = %d, want 2", len(records))
	}
	got := map[string]remoteDatasetRecord{}
	for _, rec := range records {
		if strings.HasPrefix(rec.CommandLine, "{") {
			t.Fatalf("GTFOBins record command line is still serialized JSON: %#v", rec)
		}
		got[rec.Comm] = rec
	}
	if got["7z"].Category != "file-read" || got["7z"].CommandLine != "7z a -ttar -an -so /etc/shadow | 7z e -ttar -si -so" {
		t.Fatalf("GTFOBins 7z record = %#v", got["7z"])
	}
	if got["comm"].Category != "shell" || got["comm"].CommandLine != "comm /tmp/a /tmp/b" {
		t.Fatalf("GTFOBins comm record = %#v", got["comm"])
	}

	// LOLBAS style
	lolbasRaw := []byte(`[
		{
			"Name": "7z.exe",
			"Commands": [
				{ "Command": "7z.exe a -ttar -an -so /etc/shadow", "Category": "Download" }
			]
		}
	]`)
	records, _, err = parseRemoteDatasetRecords(lolbasRaw, "auto", "LOLBAS")
	if err != nil {
		t.Fatalf("LOLBAS parse error = %v", err)
	}
	if len(records) != 1 || records[0].Comm != "7z.exe" || records[0].Category != "Download" {
		t.Fatalf("LOLBAS record = %#v", records[0])
	}
}

func TestParseRemoteDatasetRecordsSpecialSerialization(t *testing.T) {
	// Object that isn't expanded but is picked up as a value
	raw := []byte(`[
		{
			"comm": "test-binary",
			"metadata": { "author": "me", "version": 1.0 }
		}
	]`)
	records, _, err := parseRemoteDatasetRecords(raw, "auto", "inline.json")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	_ = records
	// If we looked for 'metadata' as a string, it should now be a JSON string
	val := firstStringValue(map[string]any{"m": map[string]any{"a": 1}}, "m")
	if val != `{"a":1}` {
		t.Fatalf("got %q, want {\"a\":1}", val)
	}
}

func TestParseRemoteDatasetRecordsTextNumericSequencePreserved(t *testing.T) {
	raw := []byte("1 2 3 4\n5 6 7\n")
	records, format, err := parseRemoteDatasetRecords(raw, "auto", "ADFA-LD.txt")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if format != "text" {
		t.Fatalf("format = %q, want text", format)
	}
	if len(records) != 2 {
		t.Fatalf("records length = %d, want 2", len(records))
	}
	if records[0].Comm != "syscall-seq" || strings.Join(records[0].Args, " ") != "1 2 3 4" {
		t.Fatalf("first record = %#v", records[0])
	}
	if records[1].Comm != "syscall-seq" || strings.Join(records[1].Args, " ") != "5 6 7" {
		t.Fatalf("second record = %#v", records[1])
	}
}

func TestParseRemoteDatasetRecordsSafetyNetRules(t *testing.T) {
	raw := []byte(`{
		"source": "github.com/kenryu42/claude-code-safety-net",
		"rules": [
			{
				"command": "git reset --hard HEAD~1",
				"action": "BLOCK",
				"priority": 200,
				"reason": "test"
			}
		]
	}`)
	records, format, err := parseRemoteDatasetRecords(raw, "auto", "Claude Code Safety Net")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if format != "json" {
		t.Fatalf("format = %q, want json", format)
	}
	if len(records) != 1 {
		t.Fatalf("records length = %d, want 1", len(records))
	}
	if records[0].Comm != "git" || strings.Join(records[0].Args, " ") != "reset --hard HEAD~1" {
		t.Fatalf("record = %#v", records[0])
	}
	if records[0].Label != "BLOCK" {
		t.Fatalf("label = %q, want BLOCK", records[0].Label)
	}
}

func TestParseRemoteDatasetRecordsTextSkipsCommentNoise(t *testing.T) {
	raw := []byte("/*\n* This file contains the system call numbers, based on the\n__SYSCALL(__NR_io_setup, sys_io_setup)\necho hello\n")
	records, format, err := parseRemoteDatasetRecords(raw, "auto", "noisy.txt")
	if err != nil {
		t.Fatalf("parse error = %v", err)
	}
	if format != "text" {
		t.Fatalf("format = %q, want text", format)
	}
	if len(records) != 1 {
		t.Fatalf("records length = %d, want 1", len(records))
	}
	if records[0].Comm != "echo" || strings.Join(records[0].Args, " ") != "hello" {
		t.Fatalf("record = %#v", records[0])
	}
}

func TestBuildRemoteDatasetSampleForceBlock(t *testing.T) {
	row := remoteDatasetRow{
		CommandLine: "openvt -- /bin/sh",
		Comm:        "openvt",
		Args:        []string{"--", "/bin/sh"},
		Label:       "ALLOW",
		Category:    "shell",
		Timestamp:   "2026-01-01T00:00:00Z",
		UserLabel:   "dataset",
	}
	sample := buildRemoteDatasetSample(row, "block", false)
	if sample.Label != 1 {
		t.Fatalf("sample.Label = %d, want BLOCK", sample.Label)
	}
	if sample.UserLabel != "remote-block" {
		t.Fatalf("sample.UserLabel = %q, want remote-block", sample.UserLabel)
	}
	if sample.CommandLine != row.CommandLine {
		t.Fatalf("sample.CommandLine = %q, want %q", sample.CommandLine, row.CommandLine)
	}
}

func TestBuildRemoteDatasetRowInfersLabelFromSource(t *testing.T) {
	record := remoteDatasetRecord{
		Row:         7,
		Source:      "mpsd/powershell_benign_dataset/sample.ps1",
		CommandLine: "Write-Host hello",
		Comm:        "Write-Host",
		Args:        []string{"hello"},
	}

	row := buildRemoteDatasetRow(record, "preserve", false)
	if row.Label != "ALLOW" {
		t.Fatalf("row.Label = %q, want ALLOW", row.Label)
	}
	if row.LabelSource != "source" {
		t.Fatalf("row.LabelSource = %q, want source", row.LabelSource)
	}
	if row.Source != record.Source {
		t.Fatalf("row.Source = %q, want %q", row.Source, record.Source)
	}
}

func TestBuildRemoteDatasetSampleCleansSensitiveValues(t *testing.T) {
	row := remoteDatasetRow{
		Source:      "HttpParamsDataset/payload_train.csv",
		CommandLine: "curl https://user:secret@example.com/path?token=abc123 -H \"Authorization: Bearer abc123\"",
		Comm:        "curl",
		Args:        []string{"https://user:secret@example.com/path?token=abc123", "-H", "Authorization: Bearer abc123"},
		Label:       "BLOCK",
		Category:    "NETWORK",
		Timestamp:   "2026-01-01T00:00:00Z",
		UserLabel:   "remote-source-label",
	}

	sample := buildRemoteDatasetSample(row, "preserve", true)
	if strings.Contains(sample.CommandLine, "secret") || strings.Contains(sample.CommandLine, "token=abc123") {
		t.Fatalf("sample.CommandLine still contains sensitive data: %q", sample.CommandLine)
	}
	if strings.Contains(strings.Join(sample.Args, " "), "secret") || strings.Contains(strings.Join(sample.Args, " "), "abc123") {
		t.Fatalf("sample.Args still contains sensitive data: %#v", sample.Args)
	}
	if !strings.Contains(sample.CommandLine, "***") {
		t.Fatalf("sample.CommandLine = %q, want masked content", sample.CommandLine)
	}
}

func TestNormalizeActionLabelClassicDatasetSynonyms(t *testing.T) {
	cases := map[string]string{
		"norm":                 "ALLOW",
		"BENIGN":               "ALLOW",
		"anom":                 "BLOCK",
		"cmdi":                 "BLOCK",
		"sql injection":        "BLOCK",
		"path traversal":       "BLOCK",
		"cross-site scripting": "BLOCK",
	}
	for input, want := range cases {
		if got := normalizeActionLabel(input); got != want {
			t.Fatalf("normalizeActionLabel(%q) = %q, want %q", input, got, want)
		}
	}
}

func buildZipArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatalf("zip create %q error = %v", name, err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %q error = %v", name, err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zip close error = %v", err)
	}
	return buf.Bytes()
}

func buildTarArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for name, content := range files {
		hdr := &tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar write header %q error = %v", name, err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatalf("tar write %q error = %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close error = %v", err)
	}
	return buf.Bytes()
}

// ---- merged from runtimeebpf_test.go ----

func TestIsMissingTracepointError(t *testing.T) {
	t.Parallel()

	err := fmt.Errorf("attach syscalls/sys_enter_lstat: reading file \"/sys/kernel/tracing/events/syscalls/sys_enter_lstat/id\": open /sys/kernel/tracing/events/syscalls/sys_enter_lstat/id: no such file or directory")
	if !isMissingTracepointError(err) {
		t.Fatalf("expected missing tracepoint error to be detected")
	}

	if isMissingTracepointError(errors.New("permission denied")) {
		t.Fatalf("unexpectedly classified a non-not-found error as missing tracepoint")
	}
}

// ---- merged from runtimereplaysuite_test.go ----

type runtimeReplayCatalog struct {
	Version   string                  `json:"version"`
	Scenarios []runtimeReplayScenario `json:"scenarios"`
}

type runtimeReplayScenario struct {
	ID                       string                    `json:"id"`
	Class                    string                    `json:"class"`
	Description              string                    `json:"description"`
	Seed                     *runtimeReplaySeed        `json:"seed,omitempty"`
	Events                   []runtimeReplayEventInput `json:"events"`
	ExpectedAlertCodes       []string                  `json:"expectedAlertCodes,omitempty"`
	ExpectedWrapperAction    string                    `json:"expectedWrapperAction,omitempty"`
	ExpectContextInheritance bool                      `json:"expectContextInheritance,omitempty"`
}

type runtimeReplaySeed struct {
	WrapperRequest *runtimeReplayWrapperRequest `json:"wrapperRequest,omitempty"`
}

type runtimeReplayWrapperRequest struct {
	PID          uint32   `json:"pid"`
	Comm         string   `json:"comm"`
	Args         []string `json:"args"`
	User         string   `json:"user"`
	ToolName     string   `json:"toolName"`
	AgentRunID   string   `json:"agentRunId"`
	TaskID       string   `json:"taskId"`
	ToolCallID   string   `json:"toolCallId"`
	TraceID      string   `json:"traceId"`
	RootAgentPID uint32   `json:"rootAgentPid"`
	Cwd          string   `json:"cwd"`
}

type runtimeReplayEventInput struct {
	PID          uint32  `json:"pid"`
	PPID         uint32  `json:"ppid"`
	Type         string  `json:"type"`
	Comm         string  `json:"comm"`
	Path         string  `json:"path,omitempty"`
	ExtraPath    string  `json:"extraPath,omitempty"`
	NetEndpoint  string  `json:"netEndpoint,omitempty"`
	NetDirection string  `json:"netDirection,omitempty"`
	ExtraInfo    string  `json:"extraInfo,omitempty"`
	Mode         string  `json:"mode,omitempty"`
	Cwd          string  `json:"cwd,omitempty"`
	Decision     string  `json:"decision,omitempty"`
	RiskScore    float64 `json:"riskScore,omitempty"`
}

type runtimeReplayScenarioResult struct {
	ID                    string   `json:"id"`
	Class                 string   `json:"class"`
	Description           string   `json:"description"`
	ExpectedAlertCodes    []string `json:"expectedAlertCodes,omitempty"`
	ObservedAlertCodes    []string `json:"observedAlertCodes,omitempty"`
	MissingAlertCodes     []string `json:"missingAlertCodes,omitempty"`
	UnexpectedAlertCodes  []string `json:"unexpectedAlertCodes,omitempty"`
	ExpectedWrapperAction string   `json:"expectedWrapperAction,omitempty"`
	ObservedWrapperAction string   `json:"observedWrapperAction,omitempty"`
	ContextChecks         int      `json:"contextChecks"`
	ContextMatches        int      `json:"contextMatches"`
	EventCount            int      `json:"eventCount"`
}

type runtimeReplaySummary struct {
	Version                     string                        `json:"version"`
	GeneratedAt                 string                        `json:"generatedAt"`
	ScenarioCount               int                           `json:"scenarioCount"`
	ClassCounts                 map[string]int                `json:"classCounts"`
	PassedScenarios             int                           `json:"passedScenarios"`
	FailedScenarios             int                           `json:"failedScenarios"`
	ExpectedAlertCount          int                           `json:"expectedAlertCount"`
	ObservedAlertCount          int                           `json:"observedAlertCount"`
	FalsePositiveCount          int                           `json:"falsePositiveCount"`
	FalseNegativeCount          int                           `json:"falseNegativeCount"`
	TraceCorrelationAccuracy    float64                       `json:"traceCorrelationAccuracy"`
	EventLatencyP50Ns           int64                         `json:"eventLatencyP50Ns"`
	EventLatencyP95Ns           int64                         `json:"eventLatencyP95Ns"`
	EventLatencyP99Ns           int64                         `json:"eventLatencyP99Ns"`
	WrapperDecisionLatencyP50Ns int64                         `json:"wrapperDecisionLatencyP50Ns"`
	WrapperDecisionLatencyP95Ns int64                         `json:"wrapperDecisionLatencyP95Ns"`
	WrapperDecisionLatencyP99Ns int64                         `json:"wrapperDecisionLatencyP99Ns"`
	BlockLatencyP50Ns           int64                         `json:"blockLatencyP50Ns"`
	BlockLatencyP95Ns           int64                         `json:"blockLatencyP95Ns"`
	BlockLatencyP99Ns           int64                         `json:"blockLatencyP99Ns"`
	WallDurationNs              int64                         `json:"wallDurationNs"`
	MemoryAllocDeltaBytes       uint64                        `json:"memoryAllocDeltaBytes"`
	RingbufDropRate             float64                       `json:"ringbufDropRate"`
	Notes                       []string                      `json:"notes"`
	Scenarios                   []runtimeReplayScenarioResult `json:"scenarios"`
}

func TestRuntimeReplaySuite(t *testing.T) {
	catalog := loadRuntimeReplayCatalog(t)
	origTracked := trackedProcessContexts
	origSemanticState := semanticAlertsState
	origMLEnabled := mlEnabled
	origMLLoaded := mlModelLoaded
	origMLConfig := mlConfig
	defer func() {
		trackedProcessContexts = origTracked
		semanticAlertsState = origSemanticState
		mlEnabled = origMLEnabled
		mlModelLoaded = origMLLoaded
		mlConfig = origMLConfig
	}()

	mlEnabled = false
	mlModelLoaded = false
	mlConfig = DefaultMLConfig()

	var beforeMem, afterMem runtimeMemStats
	readRuntimeMemStats(&beforeMem)
	start := time.Now()

	results := make([]runtimeReplayScenarioResult, 0, len(catalog.Scenarios))
	classCounts := make(map[string]int)
	eventLatencies := make([]int64, 0, 256)
	wrapperLatencies := make([]int64, 0, len(catalog.Scenarios))
	blockLatencies := make([]int64, 0, len(catalog.Scenarios))

	expectedAlerts := 0
	observedAlerts := 0
	falsePositives := 0
	falseNegatives := 0
	contextChecks := 0
	contextMatches := 0
	failedScenarios := 0

	for _, scenario := range catalog.Scenarios {
		classCounts[scenario.Class]++
		result, metrics := runRuntimeReplayScenario(t, scenario)
		results = append(results, result)

		expectedAlerts += len(result.ExpectedAlertCodes)
		observedAlerts += len(result.ObservedAlertCodes)
		eventLatencies = append(eventLatencies, metrics.eventLatencies...)
		if metrics.wrapperLatency > 0 {
			wrapperLatencies = append(wrapperLatencies, metrics.wrapperLatency)
		}
		if metrics.blockLatency > 0 {
			blockLatencies = append(blockLatencies, metrics.blockLatency)
		}
		contextChecks += result.ContextChecks
		contextMatches += result.ContextMatches

		if result.Class == "benign" {
			falsePositives += len(result.ObservedAlertCodes)
		}
		falseNegatives += len(result.MissingAlertCodes)

		passed := len(result.MissingAlertCodes) == 0
		if result.Class == "benign" && len(result.ObservedAlertCodes) > 0 {
			passed = false
		}
		if strings.TrimSpace(result.ExpectedWrapperAction) != "" && result.ExpectedWrapperAction != result.ObservedWrapperAction {
			passed = false
		}
		if !passed {
			failedScenarios++
		}
	}

	readRuntimeMemStats(&afterMem)
	summary := runtimeReplaySummary{
		Version:                     catalog.Version,
		GeneratedAt:                 time.Now().UTC().Format(time.RFC3339Nano),
		ScenarioCount:               len(results),
		ClassCounts:                 classCounts,
		PassedScenarios:             len(results) - failedScenarios,
		FailedScenarios:             failedScenarios,
		ExpectedAlertCount:          expectedAlerts,
		ObservedAlertCount:          observedAlerts,
		FalsePositiveCount:          falsePositives,
		FalseNegativeCount:          falseNegatives,
		TraceCorrelationAccuracy:    ratio(contextMatches, contextChecks),
		EventLatencyP50Ns:           percentileNs(eventLatencies, 50),
		EventLatencyP95Ns:           percentileNs(eventLatencies, 95),
		EventLatencyP99Ns:           percentileNs(eventLatencies, 99),
		WrapperDecisionLatencyP50Ns: percentileNs(wrapperLatencies, 50),
		WrapperDecisionLatencyP95Ns: percentileNs(wrapperLatencies, 95),
		WrapperDecisionLatencyP99Ns: percentileNs(wrapperLatencies, 99),
		BlockLatencyP50Ns:           percentileNs(blockLatencies, 50),
		BlockLatencyP95Ns:           percentileNs(blockLatencies, 95),
		BlockLatencyP99Ns:           percentileNs(blockLatencies, 99),
		WallDurationNs:              time.Since(start).Nanoseconds(),
		MemoryAllocDeltaBytes:       deltaUint64(afterMem.Alloc, beforeMem.Alloc),
		RingbufDropRate:             0,
		Notes: []string{
			"Offline replay bypasses the live kernel ringbuf, so ringbufDropRate is 0 by construction for this suite.",
			"Wrapper decision latency is measured through resolveAction() with deterministic non-ML inputs.",
			"Context correlation accuracy checks that child events inherit agent_run_id/task_id/tool_call_id/trace_id/root_agent_pid via enrichEventContext().",
		},
		Scenarios: results,
	}

	writeRuntimeReplaySummary(t, summary)

	if failedScenarios > 0 || falsePositives > 0 || falseNegatives > 0 {
		t.Fatalf("runtime replay suite found regressions: failedScenarios=%d falsePositives=%d falseNegatives=%d", failedScenarios, falsePositives, falseNegatives)
	}
}

type runtimeReplayScenarioMetrics struct {
	eventLatencies []int64
	wrapperLatency int64
	blockLatency   int64
}

func runRuntimeReplayScenario(t *testing.T, scenario runtimeReplayScenario) (runtimeReplayScenarioResult, runtimeReplayScenarioMetrics) {
	t.Helper()
	trackedProcessContexts = newProcessContextStore()
	resetSemanticAlertState()

	result := runtimeReplayScenarioResult{
		ID:                 scenario.ID,
		Class:              scenario.Class,
		Description:        scenario.Description,
		ExpectedAlertCodes: append([]string(nil), scenario.ExpectedAlertCodes...),
		EventCount:         len(scenario.Events),
	}
	metrics := runtimeReplayScenarioMetrics{eventLatencies: make([]int64, 0, len(scenario.Events))}

	var seedReq *pb.WrapperRequest
	if scenario.Seed != nil && scenario.Seed.WrapperRequest != nil {
		seedReq = scenario.Seed.WrapperRequest.toProto()
		start := time.Now()
		action := simulateWrapperDecision(seedReq)
		metrics.wrapperLatency = time.Since(start).Nanoseconds()
		result.ObservedWrapperAction = action
		result.ExpectedWrapperAction = strings.TrimSpace(scenario.ExpectedWrapperAction)
		seedCtx := buildProcessContextFromWrapperRequest(seedReq, action, 0.1)
		trackedProcessContexts.Set(seedReq.Pid, seedCtx)
	}

	observedCodes := make(map[string]struct{})
	firstAlertLatency := int64(0)
	for index, raw := range scenario.Events {
		event := enrichEventContext(raw.toProto())
		record := normalizeCapturedEventRecord(CapturedEventRecord{
			ReceivedAt: time.Now().UTC(),
			Event:      event,
		})
		eventStart := time.Now()
		alerts := buildSemanticAlerts(record.Event)
		latency := time.Since(eventStart).Nanoseconds()
		metrics.eventLatencies = append(metrics.eventLatencies, latency)
		if len(alerts) > 0 && firstAlertLatency == 0 {
			firstAlertLatency = latency
		}

		for _, alert := range alerts {
			code := strings.TrimSpace(alert.GetComm())
			if code != "" {
				observedCodes[code] = struct{}{}
			}
		}

		if scenario.ExpectContextInheritance && seedReq != nil && raw.PID != seedReq.Pid {
			result.ContextChecks++
			if contextMatchesSeed(record.Envelope, seedReq) {
				result.ContextMatches++
			} else {
				t.Logf("context mismatch in scenario %s event %d: envelope=%+v seed=%+v", scenario.ID, index, record.Envelope, seedReq)
			}
		}
	}

	result.ObservedAlertCodes = sortedSetKeys(observedCodes)
	result.MissingAlertCodes = missingStrings(scenario.ExpectedAlertCodes, result.ObservedAlertCodes)
	if scenario.Class == "benign" {
		result.UnexpectedAlertCodes = append(result.UnexpectedAlertCodes, result.ObservedAlertCodes...)
	}
	if firstAlertLatency > 0 {
		metrics.blockLatency = firstAlertLatency
	} else if metrics.wrapperLatency > 0 && result.ObservedWrapperAction != "" && result.ObservedWrapperAction != "ALLOW" {
		metrics.blockLatency = metrics.wrapperLatency
	}
	return result, metrics
}

func loadRuntimeReplayCatalog(t *testing.T) runtimeReplayCatalog {
	t.Helper()
	path := filepath.Join("..", "benchmarks", "runtime-replay", "scenarios.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime replay catalog %s: %v", path, err)
	}
	var catalog runtimeReplayCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse runtime replay catalog %s: %v", path, err)
	}
	if len(catalog.Scenarios) == 0 {
		t.Fatalf("runtime replay catalog %s is empty", path)
	}
	return catalog
}

func (r *runtimeReplayWrapperRequest) toProto() *pb.WrapperRequest {
	if r == nil {
		return nil
	}
	return &pb.WrapperRequest{
		Pid:          r.PID,
		Comm:         r.Comm,
		Args:         append([]string(nil), r.Args...),
		User:         r.User,
		ToolName:     r.ToolName,
		AgentRunId:   r.AgentRunID,
		TaskId:       r.TaskID,
		ToolCallId:   r.ToolCallID,
		TraceId:      r.TraceID,
		RootAgentPid: r.RootAgentPID,
		Cwd:          r.Cwd,
	}
}

func (e runtimeReplayEventInput) toProto() *pb.Event {
	return &pb.Event{
		Pid:          e.PID,
		Ppid:         e.PPID,
		Type:         e.Type,
		Comm:         e.Comm,
		Path:         e.Path,
		ExtraPath:    e.ExtraPath,
		NetEndpoint:  e.NetEndpoint,
		NetDirection: e.NetDirection,
		ExtraInfo:    e.ExtraInfo,
		Mode:         e.Mode,
		Cwd:          e.Cwd,
		Decision:     e.Decision,
		RiskScore:    e.RiskScore,
	}
}

func simulateWrapperDecision(req *pb.WrapperRequest) string {
	if req == nil {
		return ""
	}
	classification := behavior.ClassifyBehavior(req.GetComm(), req.GetArgs())
	action, _ := resolveAction(req, "", 0, classification, 0, Prediction{}, DefaultMLConfig())
	return actionLabel[int32(action)]
}

func contextMatchesSeed(envelope *pb.EventEnvelope, seed *pb.WrapperRequest) bool {
	if envelope == nil || seed == nil {
		return false
	}
	rootAgentPID := uint32(0)
	if legacy := envelope.GetLegacyEvent(); legacy != nil {
		rootAgentPID = legacy.GetRootAgentPid()
	}
	return rootAgentPID == seed.GetRootAgentPid() &&
		envelope.GetAgentRunId() == seed.GetAgentRunId() &&
		envelope.GetTaskId() == seed.GetTaskId() &&
		envelope.GetToolCallId() == seed.GetToolCallId() &&
		envelope.GetTraceId() == seed.GetTraceId() &&
		envelope.GetToolName() == firstNonEmpty(seed.GetToolName(), seed.GetComm()) &&
		envelope.GetCwd() == seed.GetCwd()
}

func missingStrings(expected, observed []string) []string {
	seen := make(map[string]struct{}, len(observed))
	for _, item := range observed {
		seen[item] = struct{}{}
	}
	missing := make([]string, 0)
	for _, item := range expected {
		if _, ok := seen[item]; !ok {
			missing = append(missing, item)
		}
	}
	return missing
}

func sortedSetKeys(items map[string]struct{}) []string {
	out := make([]string, 0, len(items))
	for item := range items {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func percentileNs(values []int64, percentile int) int64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]int64(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	index := (len(sorted) - 1) * percentile / 100
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func ratio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

type runtimeMemStats struct {
	Alloc uint64
}

func readRuntimeMemStats(out *runtimeMemStats) {
	if out == nil {
		return
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	out.Alloc = stats.Alloc
}

func writeRuntimeReplaySummary(t *testing.T, summary runtimeReplaySummary) {
	t.Helper()
	data, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatalf("marshal runtime replay summary: %v", err)
	}
	target := strings.TrimSpace(os.Getenv("RUNTIME_REPLAY_OUT"))
	if target == "" {
		t.Logf("runtime replay summary:\n%s", string(data))
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		t.Fatalf("mkdir runtime replay report dir: %v", err)
	}
	if err := os.WriteFile(target, data, 0644); err != nil {
		t.Fatalf("write runtime replay summary: %v", err)
	}
	t.Logf("runtime replay summary written to %s", target)
}

// ---- merged from stateenvruntime_test.go ----

func TestSeedRuntimeSettingsFromEnvAppliesLLMAndBehavior(t *testing.T) {
	t.Setenv("AGENT_ACCESS_TOKEN", "dev-token")
	t.Setenv("AGENT_RUNTIME_SHELL_SESSIONS_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_SYSTEM_RUN_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_TLS_CAPTURE_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_OTLP_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_OTLP_ENDPOINT", "http://127.0.0.1:4318/v1/traces")
	t.Setenv("AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED", "true")
	t.Setenv("AGENT_RUNTIME_DOMAIN_HTTP_PORT", "18080")
	t.Setenv("AGENT_RUNTIME_DOMAIN_HTTPS_PORT", "18443")
	t.Setenv("AGENT_RUNTIME_DOMAIN_DEFAULT_SCHEME", "http")
	t.Setenv("AGENT_RUNTIME_DOMAIN_ALLOW_ANY_HOST", "true")
	t.Setenv("AGENT_ML_ENABLED", "false")
	t.Setenv("AGENT_ML_MODEL_TYPE", string(ModelLogisticRegression))
	t.Setenv("AGENT_ML_VALIDATION_SPLIT_RATIO", "0.35")
	t.Setenv("AGENT_LLM_ENABLED", "true")
	t.Setenv("AGENT_LLM_BASE_URL", "http://127.0.0.1:11434/v1")
	t.Setenv("AGENT_LLM_API_KEY", "local-key")
	t.Setenv("AGENT_LLM_MODEL", "qwen2.5-coder")
	t.Setenv("AGENT_LLM_TIMEOUT_SECONDS", "12")
	t.Setenv("AGENT_LLM_TEMPERATURE", "0.2")
	t.Setenv("AGENT_LLM_MAX_TOKENS", "777")
	t.Setenv("AGENT_LLM_SYSTEM_PROMPT", "strict json")

	settings := RuntimeSettings{}
	seedRuntimeSettingsFromEnv(&settings)
	if err := normalizeRuntimeSettings(&settings); err != nil {
		t.Fatalf("normalizeRuntimeSettings() error = %v", err)
	}

	if settings.AccessToken != "dev-token" {
		t.Fatalf("AccessToken = %q, want env token", settings.AccessToken)
	}
	if !settings.ShellSessionsEnabled || !settings.SystemRunEnabled || !settings.PolicyManagementEnabled || !settings.TlsCaptureEnabled {
		t.Fatalf("runtime behavior booleans were not seeded: %+v", settings)
	}
	if !settings.OtlpEnabled || settings.OtlpEndpoint != "http://127.0.0.1:4318/v1/traces" {
		t.Fatalf("OTLP env seed mismatch: enabled=%v endpoint=%q", settings.OtlpEnabled, settings.OtlpEndpoint)
	}
	if !settings.DomainForwardProxy.Enabled || settings.DomainForwardProxy.HTTPPort != 18080 || settings.DomainForwardProxy.HTTPSPort != 18443 {
		t.Fatalf("domain forward env seed mismatch: %+v", settings.DomainForwardProxy)
	}
	if settings.DomainForwardProxy.DefaultScheme != "http" || !settings.DomainForwardProxy.AllowAnyHost {
		t.Fatalf("domain forward scheme/host env seed mismatch: %+v", settings.DomainForwardProxy)
	}
	if settings.MLConfig.Enabled {
		t.Fatalf("MLConfig.Enabled = true, want AGENT_ML_ENABLED=false to be respected")
	}
	if settings.MLConfig.ModelType != ModelLogisticRegression || settings.MLConfig.ValidationSplitRatio != 0.35 {
		t.Fatalf("ML env seed mismatch: %+v", settings.MLConfig)
	}
	if !settings.MLConfig.LlmEnabled || settings.MLConfig.LlmBaseURL != "http://127.0.0.1:11434/v1" || settings.MLConfig.LlmModel != "qwen2.5-coder" {
		t.Fatalf("LLM env seed mismatch: %+v", settings.MLConfig)
	}
	if settings.MLConfig.LlmAPIKey != "local-key" || settings.MLConfig.LlmTimeoutSeconds != 12 || settings.MLConfig.LlmTemperature != 0.2 || settings.MLConfig.LlmMaxTokens != 777 {
		t.Fatalf("LLM numeric/secret env seed mismatch: %+v", settings.MLConfig)
	}
	if settings.MLConfig.LlmSystemPrompt != "strict json" {
		t.Fatalf("LlmSystemPrompt = %q, want env prompt", settings.MLConfig.LlmSystemPrompt)
	}
}

// ---- merged from sweep_test.go ----

func TestComprehensiveSweepProfilesCoverThousandPointsPerNumericParameter(t *testing.T) {
	profiles := profilesForMode("comprehensive")
	seen := make(map[ModelType]map[string]int)
	for _, profile := range profiles {
		if profile.ParameterName == "" {
			t.Fatalf("profile %s missing parameter metadata", profile.Name)
		}
		if profile.ParameterKind != "numeric" {
			if unique := uniqueIntCount(profile.XValues); profile.RequiredDiscretePoints != unique {
				t.Fatalf("%s categorical/fixed requirement=%d, want unique count %d", profile.Name, profile.RequiredDiscretePoints, unique)
			}
			continue
		}
		unique := uniqueIntCount(profile.XValues)
		if unique < 1000 {
			t.Fatalf("%s has %d unique points, want >=1000", profile.Name, unique)
		}
		if seen[profile.ModelType] == nil {
			seen[profile.ModelType] = make(map[string]int)
		}
		seen[profile.ModelType][profile.ParameterName] = unique
	}
	for _, modelType := range AllModelTypes() {
		for _, param := range numericSweepParametersForModel(modelType) {
			if seen[modelType][param] < 1000 {
				t.Fatalf("%s/%s coverage = %d, want >=1000 discrete points", modelType, param, seen[modelType][param])
			}
		}
	}
}

func TestComprehensiveSweepDefaultsToMultipleDatasets(t *testing.T) {
	samples := make([]TrainingSample, 0, 30)
	for i := 0; i < 12; i++ {
		samples = append(samples, sweepTestSample(0, "allow"))
	}
	for i := 0; i < 10; i++ {
		samples = append(samples, sweepTestSample(1, "block"))
	}
	for i := 0; i < 8; i++ {
		samples = append(samples, sweepTestSample(3, "alert"))
	}

	datasets := datasetProfilesForMode(samples, "comprehensive", nil)
	if len(datasets) < 2 {
		t.Fatalf("comprehensive datasets = %d, want at least 2", len(datasets))
	}
	if datasets[0].Name != "all" || len(datasets[0].Samples) != len(samples) {
		t.Fatalf("first dataset = %s/%d, want all/%d", datasets[0].Name, len(datasets[0].Samples), len(samples))
	}
	foundBalanced := false
	for _, dataset := range datasets {
		if dataset.Name == "label-balanced" {
			foundBalanced = true
			if len(dataset.Samples) != 24 {
				t.Fatalf("label-balanced samples = %d, want 24", len(dataset.Samples))
			}
		}
	}
	if !foundBalanced {
		t.Fatalf("expected label-balanced dataset, got %#v", datasets)
	}
}

func sweepTestSample(label int32, userLabel string) TrainingSample {
	return TrainingSample{
		Label:     label,
		UserLabel: userLabel,
		Timestamp: time.Now(),
		Comm:      "cmd",
		Args:      []string{fmt.Sprintf("%d", label)},
	}
}

func TestMLSweep(t *testing.T) {
	if os.Getenv("ML_SWEEP") != "1" {
		t.Skip("set ML_SWEEP=1 to run the offline ML sweep report generator")
	}
	if err := runMLSweepReport(); err != nil {
		t.Fatalf("ml sweep failed: %v", err)
	}
}

// ---- merged from visualllmplugin_test.go ----

func TestParseVisualBlocksLLMContentSocketCounter(t *testing.T) {
	content := `{"trigger":"socket_connect","action":"KILL","conditions":{"id":"root","type":"AND","children":[{"id":"cond-comm","type":"CONDITION","field":"comm","operator":"==","value":"python"},{"id":"cond-port","type":"CONDITION","field":"port","operator":"==","value":4444}]},"mapMode":"COUNTER","mapKey":"pid","mapLimit":3,"reasoning":"外连端口强杀"}`

	got, err := parseVisualBlocksLLMContent(content)
	if err != nil {
		t.Fatalf("parseVisualBlocksLLMContent() error = %v", err)
	}
	if got.Trigger != "socket_connect" || got.Action != "KILL" {
		t.Fatalf("trigger/action = %s/%s, want socket_connect/KILL", got.Trigger, got.Action)
	}
	if got.MapMode != "COUNTER" || got.MapKey != "pid" || got.MapLimit != 3 {
		t.Fatalf("map = %s/%s/%d, want COUNTER/pid/3", got.MapMode, got.MapKey, got.MapLimit)
	}
	if got.Conditions.ID != "root" || len(got.Conditions.Children) != 2 {
		t.Fatalf("conditions = %#v", got.Conditions)
	}
	if got.Conditions.Children[1].Field != "port" || got.Conditions.Children[1].Value != "4444" {
		t.Fatalf("second condition = %#v, want port 4444", got.Conditions.Children[1])
	}
}

func TestParseVisualBlocksLLMContentAdjustsUnlinkBlock(t *testing.T) {
	content := `{"trigger":"unlink","action":"BLOCK","conditions":{"id":"root","type":"AND","children":[{"type":"CONDITION","field":"comm","operator":"==","value":"rm"}]},"mapMode":"NONE","mapKey":"pid","mapLimit":10}`

	got, err := parseVisualBlocksLLMContent(content)
	if err != nil {
		t.Fatalf("parseVisualBlocksLLMContent() error = %v", err)
	}
	if got.Action != "ALERT" {
		t.Fatalf("action = %s, want ALERT", got.Action)
	}
	if len(got.Warnings) == 0 || !strings.Contains(got.Warnings[0], "unlink") {
		t.Fatalf("warnings = %#v, want unlink warning", got.Warnings)
	}
}

func TestParseVisualBlocksLLMContentRejectsSocketFieldOnProcess(t *testing.T) {
	content := `{"trigger":"process","action":"BLOCK","conditions":{"id":"root","type":"AND","children":[{"type":"CONDITION","field":"port","operator":"==","value":"4444"}]},"mapMode":"NONE","mapKey":"pid","mapLimit":10}`

	_, err := parseVisualBlocksLLMContent(content)
	if err == nil || !strings.Contains(err.Error(), "port/ipv4") {
		t.Fatalf("parseVisualBlocksLLMContent() error = %v, want port/ipv4 validation error", err)
	}
}
