package domainforwardproxy

import (
	"io"
	"strings"
	"testing"
)

func TestSSERewriteBodyPreservesStreamingAndRestoresModel(t *testing.T) {
	kernel := NewRewriteKernel(BodyRewriteSettings{Enabled: true})
	source := io.NopCloser(strings.NewReader(
		"event: response.completed\n" +
			"data: {\"type\":\"response.completed\",\"response\":{\"model\":\"fast-model\"}}\n\n" +
			"data: [DONE]\n",
	))
	body := newSSERewriteBody(
		source,
		kernel,
		"api.openai.com",
		"/v1/responses",
		"text/event-stream",
		&modelRewrite{Client: "client-model", Upstream: "fast-model"},
	)
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, `"model":"client-model"`) {
		t.Fatalf("model was not restored: %s", text)
	}
	if !strings.Contains(text, "data: [DONE]\n") {
		t.Fatalf("DONE sentinel changed: %s", text)
	}
}

func TestSSERewriteBodyAppliesLiteralRulePerLine(t *testing.T) {
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled: true,
		Rules: []BodyRewriteRule{{
			Enabled:     true,
			Direction:   "response",
			ContentType: "text/event-stream",
			Find:        "secret",
			Replace:     "public",
		}},
	})
	source := io.NopCloser(strings.NewReader("data: {\"delta\":\"secret\"}\n\n"))
	body := newSSERewriteBody(
		source,
		kernel,
		"api.openai.com",
		"/v1/responses",
		"text/event-stream",
		nil,
	)
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "data: {\"delta\":\"public\"}\n\n" {
		t.Fatalf("unexpected SSE rewrite: %q", got)
	}
}


func TestSSERewriteBodyReassemblesMultiLineDataEvent(t *testing.T) {
	kernel := NewRewriteKernel(BodyRewriteSettings{Enabled: true})
	source := io.NopCloser(strings.NewReader(
		"event: response.completed\n" +
			"data: {\"type\":\"response.completed\",\n" +
			"data: \"response\":{\"model\":\"fast-model\"}}\n\n",
	))
	body := newSSERewriteBody(
		source,
		kernel,
		"api.openai.com",
		"/v1/responses",
		"text/event-stream",
		&modelRewrite{Client: "client-model", Upstream: "fast-model"},
	)
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, `"model":"client-model"`) {
		t.Fatalf("multi-line SSE model was not restored: %s", text)
	}
	if !strings.Contains(text, "event: response.completed\n") {
		t.Fatalf("event metadata changed: %s", text)
	}
}

func TestSSERewriteBodyUsesJSONPayloadTypeForInference(t *testing.T) {
	modelFile := writeNativeInferenceFixture(t, "<MASK>")
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled: true,
		Inference: NativeInferenceSettings{
			Enabled:       true,
			ModelFile:     modelFile,
			Direction:     "response",
			ContentType:   "application/json",
			MinTokenBytes: 3,
			MaxTokenBytes: 256,
		},
	})
	if err := kernel.InferenceError(); err != nil {
		t.Fatalf("inference error: %v", err)
	}
	source := io.NopCloser(strings.NewReader("data: {\"value\":\"secret@example.com\"}\n\n"))
	body := newSSERewriteBody(
		source,
		kernel,
		"api.openai.com",
		"/v1/responses",
		"text/event-stream",
		nil,
	)
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if !strings.Contains(string(got), `"value":"<MASK>"`) {
		t.Fatalf("native inference did not see JSON SSE payload: %s", got)
	}
}

func TestSSERewriteBodyPassesOversizedEventThrough(t *testing.T) {
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled:      true,
		MaxBodyBytes: 64,
		Rules: []BodyRewriteRule{{
			Enabled:   true,
			Direction: "response",
			Find:      "secret",
			Replace:   "public",
		}},
	})
	payload := strings.Repeat("secret-", 32)
	raw := "data: " + payload + "\n\n"
	body := newSSERewriteBody(
		io.NopCloser(strings.NewReader(raw)),
		kernel,
		"api.openai.com",
		"/v1/responses",
		"text/event-stream",
		nil,
	)
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != raw {
		t.Fatalf("oversized SSE event changed:\n got: %q\nwant: %q", got, raw)
	}
}
