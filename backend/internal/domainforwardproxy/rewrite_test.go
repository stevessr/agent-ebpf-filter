package domainforwardproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRewriteKernelModelMappingPreservesNestedModel(t *testing.T) {
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled: true,
		ModelRules: []ModelRewriteRule{{
			Host: "api.openai.com",
			From: "client-model",
			To:   "fast-model",
		}},
	})
	input := []byte(`{"model":"client-model","input":[{"type":"message","content":[{"model":"nested-user-data","text":"hello"}]}]}`)
	got, mapping, changed := kernel.RewriteRequest("api.openai.com", "/v1/responses", "application/json", input)
	if !changed || mapping == nil || mapping.Client != "client-model" || mapping.Upstream != "fast-model" {
		t.Fatalf("mapping = %#v changed=%v", mapping, changed)
	}
	body := string(got)
	if !strings.Contains(body, `"model":"fast-model"`) {
		t.Fatalf("top-level model was not rewritten: %s", body)
	}
	if !strings.Contains(body, `"model":"nested-user-data"`) {
		t.Fatalf("nested user data was unexpectedly rewritten: %s", body)
	}

	response := []byte(`{"type":"response.completed","stream_id":"lane","response":{"id":"resp_1","model":"fast-model","output":[]}}`)
	restored, ok := kernel.RewriteResponse("api.openai.com", "/v1/responses", "application/json", response, mapping)
	if !ok || !strings.Contains(string(restored), `"model":"client-model"`) {
		t.Fatalf("response model was not restored: %s", restored)
	}
}

func TestHandlerRewritesRequestAndResponseBodies(t *testing.T) {
	settings := DomainForwardProxySettings{
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host:     "api.openai.com",
			Upstream: "https://upstream.test",
		}},
		Rewrite: BodyRewriteSettings{
			Enabled: true,
			ModelRules: []ModelRewriteRule{{
				Host: "api.openai.com",
				From: "client-model",
				To:   "fast-model",
			}},
			Rules: []BodyRewriteRule{
				{ID: "request-marker", Enabled: true, Direction: "request", Host: "api.openai.com", Find: "before", Replace: "after"},
				{ID: "response-marker", Enabled: true, Direction: "response", Host: "api.openai.com", Find: "private-marker", Replace: "public-marker"},
			},
		},
	}
	handler := NewHandlerWithTransport(settings, testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatalf("read upstream request: %v", err)
		}
		text := string(body)
		if !strings.Contains(text, `"model":"fast-model"`) || !strings.Contains(text, "after") {
			t.Fatalf("upstream body not rewritten: %s", text)
		}
		response := `{"model":"fast-model","value":"private-marker"}`
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(strings.NewReader(response)),
			ContentLength: int64(len(response)),
			Request:       req,
		}, nil
	}))

	req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(`{"model":"client-model","input":"before"}`))
	req.Host = "api.openai.com"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, `"model":"client-model"`) || !strings.Contains(body, "public-marker") {
		t.Fatalf("client response not restored/rewritten: %s", body)
	}
}

func TestRewriteHostWildcard(t *testing.T) {
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled: true,
		ModelRules: []ModelRewriteRule{{
			Host: "*.openai.com",
			From: "a",
			To:   "b",
		}},
	})
	got, _, changed := kernel.RewriteRequest("api.openai.com", "/", "application/json", []byte(`{"model":"a"}`))
	if !changed || string(got) != `{"model":"b"}` {
		t.Fatalf("rewrite = %s changed=%v", got, changed)
	}
	if _, _, changed := kernel.RewriteRequest("example.com", "/", "application/json", []byte(`{"model":"a"}`)); changed {
		t.Fatal("rule escaped its host scope")
	}
}

func BenchmarkRewriteKernelModelNoMatch(b *testing.B) {
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled: true,
		ModelRules: []ModelRewriteRule{{From: "client-model", To: "fast-model"}},
	})
	body := []byte(`{"input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for i := 0; i < b.N; i++ {
		_, _, _ = kernel.RewriteRequest("api.openai.com", "/v1/responses", "application/json", body)
	}
}

func BenchmarkRewriteKernelModelMatch(b *testing.B) {
	kernel := NewRewriteKernel(BodyRewriteSettings{
		Enabled: true,
		ModelRules: []ModelRewriteRule{{From: "client-model", To: "fast-model"}},
	})
	body := []byte(`{"model":"client-model","input":[{"role":"user","content":[{"type":"input_text","text":"hello"}]}]}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for i := 0; i < b.N; i++ {
		_, _, _ = kernel.RewriteRequest("api.openai.com", "/v1/responses", "application/json", body)
	}
}
