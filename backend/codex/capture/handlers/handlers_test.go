package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type captureStore struct {
	events []Event
}

func (s *captureStore) HandleCaptureEvent(event Event) {
	s.events = append(s.events, event)
}

func TestHandleCodexCaptureStoresSanitizedPlaintext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &captureStore{}
	r := gin.New()
	RegisterRoutes(r.Group("/"), store)

	body := `{"phase":"request","direction":"send","method":"POST","url":"https://api.openai.com/v1/responses?api_key=secret","host":"api.openai.com","pid":4242,"comm":"codex","content_type":"application/json","headers":{"Authorization":"Bearer sk-secret","Content-Type":"application/json"},"body":"{\"model\":\"gpt-5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello codex\"}],\"api_key\":\"secret\"}"}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/codex/capture", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
	if len(store.events) != 1 {
		t.Fatalf("events = %#v", store.events)
	}
	event := store.events[0]
	if event.Vendor != "codex" || event.Lib != "codex-reqwest" || event.PID != 4242 {
		t.Fatalf("event identity = %#v", event)
	}
	if event.Headers["authorization"] != RedactedValue {
		t.Fatalf("authorization not redacted: %#v", event.Headers)
	}
	if strings.Contains(event.URL, "secret") || !strings.Contains(event.URL, "api_key=") {
		t.Fatalf("url not sanitized: %q", event.URL)
	}
	if strings.Contains(event.Body, "\"api_key\": \"secret\"") || !strings.Contains(event.Body, RedactedValue) {
		t.Fatalf("body not sanitized: %q", event.Body)
	}
	if event.PromptDigest == "" || event.MessageRole != "user" || event.PromptLen == 0 {
		t.Fatalf("prompt metadata missing: %#v", event)
	}
}

func TestHandleCodexCaptureRejectsInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterRoutes(r.Group("/"), &captureStore{})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/codex/capture", strings.NewReader(`{`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestBuildCodexCaptureEventDefaultsWebsocketRequest(t *testing.T) {
	event := BuildEvent(CaptureRequest{
		Phase: "websocket_request",
		URL:   "wss://chatgpt.com/backend-api/codex/ws?token=secret",
		Body:  `{"input":"hello"}`,
		PID:   7,
	})
	if event.Type != "websocket_request" || event.Method != "WEBSOCKET" || event.Direction != "send" {
		t.Fatalf("event = %#v", event)
	}
	if event.Host != "chatgpt.com" {
		t.Fatalf("host = %q", event.Host)
	}
	if strings.Contains(event.URL, "secret") {
		t.Fatalf("url not sanitized: %q", event.URL)
	}
}

func TestCodexCaptureResponseShape(t *testing.T) {
	event := BuildEvent(CaptureRequest{Phase: "response", Status: 201, PID: 8})
	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(payload), `"type":"http_response"`) {
		t.Fatalf("payload = %s", payload)
	}
}

func TestBuildCodexCaptureResponsesWebsocketArrayInput(t *testing.T) {
	event := BuildEvent(CaptureRequest{
		Phase:       "websocket_request",
		Direction:   "send",
		URL:         "wss://api.openai.com/v1/responses",
		Host:        "api.openai.com",
		ContentType: "application/json",
		Body: `{"type":"response.create","stream_id":"lane-1","previous_response_id":"resp_prev","model":"gpt-5.6","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"upstream context"}]}]}`,
		PID: 9,
	})
	if event.Type != "websocket_request" || event.Direction != "send" {
		t.Fatalf("direction/type = %q/%q", event.Direction, event.Type)
	}
	if event.ProtocolEvent != "response.create" || event.StreamID != "lane-1" || event.PreviousResponseID != "resp_prev" {
		t.Fatalf("responses metadata missing: %#v", event)
	}
	if event.MessageRole != "user" || event.PromptLen != len("upstream context") || event.PromptDigest == "" {
		t.Fatalf("responses input context missing: %#v", event)
	}
	if event.ContextDigest == "" || event.ContextItems != 1 || event.ContextLen <= event.PromptLen {
		t.Fatalf("complete upstream context metadata missing: %#v", event)
	}
}

func TestBuildCodexCaptureResponsesWebsocketResponseDelta(t *testing.T) {
	event := BuildEvent(CaptureRequest{
		Phase:       "websocket_response",
		Direction:   "recv",
		URL:         "wss://api.openai.com/v1/responses",
		ContentType: "application/json",
		Body:        `{"type":"response.output_text.delta","stream_id":"lane-1","delta":"hello"}`,
		PID:         10,
	})
	if event.Type != "websocket_response" || event.Direction != "recv" {
		t.Fatalf("direction/type = %q/%q", event.Direction, event.Type)
	}
	if event.ProtocolEvent != "response.output_text.delta" || event.StreamID != "lane-1" {
		t.Fatalf("responses metadata missing: %#v", event)
	}
	if event.MessageRole != "assistant" || event.PromptLen != len("hello") || event.PromptDigest == "" {
		t.Fatalf("responses output context missing: %#v", event)
	}
}

func TestBuildCodexCaptureResponsesContextIncludesAllInputItems(t *testing.T) {
	build := func(system string) Event {
		return BuildEvent(CaptureRequest{
			Phase:       "request",
			Direction:   "send",
			URL:         "https://api.openai.com/v1/responses",
			Host:        "api.openai.com",
			ContentType: "application/json",
			Body: `{"model":"gpt-5.6","input":[{"type":"message","role":"system","content":[{"type":"input_text","text":"` + system + `"}]},{"type":"function_call_output","call_id":"call_1","output":"tool-result"},{"type":"message","role":"user","content":[{"type":"input_text","text":"latest-user"}]}]}`,
			PID: 11,
		})
	}

	first := build("system-one")
	second := build("system-two")
	if first.PromptDigest == "" || first.PromptDigest != second.PromptDigest {
		t.Fatalf("latest prompt digest should stay stable: %q vs %q", first.PromptDigest, second.PromptDigest)
	}
	if first.ContextDigest == "" || second.ContextDigest == "" || first.ContextDigest == second.ContextDigest {
		t.Fatalf("full context digest did not include earlier input items: %q vs %q", first.ContextDigest, second.ContextDigest)
	}
	if first.ContextItems != 3 || second.ContextItems != 3 {
		t.Fatalf("context item counts = %d/%d, want 3/3", first.ContextItems, second.ContextItems)
	}
	if first.ContextLen <= first.PromptLen {
		t.Fatalf("context len=%d prompt len=%d; full context was not retained", first.ContextLen, first.PromptLen)
	}
}
