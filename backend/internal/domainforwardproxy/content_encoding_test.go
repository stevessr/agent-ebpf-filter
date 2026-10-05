package domainforwardproxy

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gzipTestBytes(t *testing.T, value string) []byte {
	t.Helper()
	var out bytes.Buffer
	writer := gzip.NewWriter(&out)
	if _, err := writer.Write([]byte(value)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func gunzipTestString(t *testing.T, payload []byte) string {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(decoded)
}

func TestHandlerRewritesGzipRequestBody(t *testing.T) {
	handler := NewHandlerWithTransport(DomainForwardProxySettings{
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host:     "api.openai.com",
			Upstream: "https://upstream.test",
		}},
		Rewrite: BodyRewriteSettings{
			Enabled: true,
			Rules: []BodyRewriteRule{{
				Enabled:   true,
				Direction: "request",
				Find:      "before",
				Replace:   "after",
			}},
		},
	}, testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if got := req.Header.Get("Content-Encoding"); got != "gzip" {
			t.Fatalf("Content-Encoding = %q, want gzip", got)
		}
		raw, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := gunzipTestString(t, raw); got != `{"input":"after"}` {
			t.Fatalf("rewritten request = %q", got)
		}
		return &http.Response{
			StatusCode:    http.StatusOK,
			Status:        "200 OK",
			Header:        make(http.Header),
			Body:          io.NopCloser(strings.NewReader("ok")),
			ContentLength: 2,
			Request:       req,
		}, nil
	}))

	compressed := gzipTestBytes(t, `{"input":"before"}`)
	req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", bytes.NewReader(compressed))
	req.Host = "api.openai.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandlerRewritesGzipResponseAndInvalidatesValidators(t *testing.T) {
	handler := NewHandlerWithTransport(DomainForwardProxySettings{
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host:     "api.openai.com",
			Upstream: "https://upstream.test",
		}},
		Rewrite: BodyRewriteSettings{
			Enabled: true,
			Rules: []BodyRewriteRule{{
				Enabled:   true,
				Direction: "response",
				Find:      "secret",
				Replace:   "public",
			}},
		},
	}, testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw := gzipTestBytes(t, `{"value":"secret"}`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header: http.Header{
				"Content-Type":     []string{"application/json"},
				"Content-Encoding": []string{"gzip"},
				"ETag":             []string{`"upstream"`},
				"Content-MD5":      []string{"deadbeef"},
			},
			Body:          io.NopCloser(bytes.NewReader(raw)),
			ContentLength: int64(len(raw)),
			Request:       req,
		}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	req.Host = "api.openai.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := gunzipTestString(t, rec.Body.Bytes()); got != `{"value":"public"}` {
		t.Fatalf("rewritten response = %q", got)
	}
	if got := rec.Header().Get("ETag"); got != "" {
		t.Fatalf("ETag survived rewritten response: %q", got)
	}
	if got := rec.Header().Get("Content-MD5"); got != "" {
		t.Fatalf("Content-MD5 survived rewritten response: %q", got)
	}
}

func TestHandlerPreservesValidatorsWhenResponseUnchanged(t *testing.T) {
	handler := NewHandlerWithTransport(DomainForwardProxySettings{
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host:     "api.openai.com",
			Upstream: "https://upstream.test",
		}},
		Rewrite: BodyRewriteSettings{
			Enabled: true,
			Rules: []BodyRewriteRule{{
				Enabled:   true,
				Direction: "response",
				Find:      "not-present",
				Replace:   "public",
			}},
		},
	}, testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		raw := gzipTestBytes(t, `{"value":"safe"}`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header: http.Header{
				"Content-Type":     []string{"application/json"},
				"Content-Encoding": []string{"gzip"},
				"ETag":             []string{`"upstream"`},
				"Content-MD5":      []string{"deadbeef"},
			},
			Body:          io.NopCloser(bytes.NewReader(raw)),
			ContentLength: int64(len(raw)),
			Request:       req,
		}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	req.Host = "api.openai.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := gunzipTestString(t, rec.Body.Bytes()); got != `{"value":"safe"}` {
		t.Fatalf("unchanged response = %q", got)
	}
	if got := rec.Header().Get("ETag"); got != `"upstream"` {
		t.Fatalf("ETag = %q, want preserved validator", got)
	}
	if got := rec.Header().Get("Content-MD5"); got != "deadbeef" {
		t.Fatalf("Content-MD5 = %q, want preserved validator", got)
	}
}

func TestGzipDecodedBodyLimitFallsBackWithoutRewrite(t *testing.T) {
	const original = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	raw := gzipTestBytes(t, original)
	if len(raw) >= 64 {
		t.Fatalf("fixture compressed size = %d, want < 64", len(raw))
	}
	decoded, bounded, err := decodeBodyForRewrite("gzip", raw, 64)
	if err != nil {
		t.Fatalf("decodeBodyForRewrite: %v", err)
	}
	if bounded || decoded != nil {
		t.Fatalf("expected decompressed over-limit fallback, bounded=%v decoded=%q", bounded, decoded)
	}
}
