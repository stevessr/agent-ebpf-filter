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
