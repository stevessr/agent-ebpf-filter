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
