package domainforwardproxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func compressedTestBody(t *testing.T, encoding, body string) []byte {
	t.Helper()
	encoded, err := encodeBodyAfterRewrite(encoding, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func decompressedTestBody(t *testing.T, encoding string, body []byte) string {
	t.Helper()
	decoded, bounded, err := decodeBodyForRewrite(encoding, body, 1<<20)
	if err != nil || !bounded {
		t.Fatalf("decode %s: bounded=%v err=%v", encoding, bounded, err)
	}
	return string(decoded)
}

func testCompressionHandler(encoding string, direction string, transport http.RoundTripper) *Handler {
	return NewHandlerWithTransport(DomainForwardProxySettings{
		DefaultScheme: "https",
		Routes: []DomainForwardRoute{{
			Host: "api.openai.com", Upstream: "https://upstream.test",
		}},
		Rewrite: BodyRewriteSettings{
			Enabled: true,
			Rules: []BodyRewriteRule{{
				Enabled: true, Direction: direction,
				Find: "before", Replace: "after",
			}},
		},
	}, transport)
}

func TestHTTPSCompressionRoundTrip(t *testing.T) {
	for _, encoding := range []string{"identity", "gzip", "br", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			original := `{"text":"before","ok":true}`
			wire := compressedTestBody(t, encoding, original)
			if got := decompressedTestBody(t, encoding, wire); got != original {
				t.Fatalf("round trip body = %q", got)
			}
			if !bodyEncodingRewriteable(encoding) {
				t.Fatalf("%s should support rewriting", encoding)
			}
		})
	}
	for _, encoding := range []string{"gzip, br", "compress", "deflate", "br, gzip"} {
		if bodyEncodingRewriteable(encoding) {
			t.Fatalf("stacked or unsupported encoding %q should be pass-through", encoding)
		}
	}
}

func TestHandlerRewritesBrotliAndZstdRequests(t *testing.T) {
	for _, encoding := range []string{"br", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			const original = `{"text":"before"}`
			handler := testCompressionHandler(encoding, "request", testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if got := req.Header.Get("Content-Encoding"); got != encoding {
					t.Fatalf("outbound encoding = %q", got)
				}
				if got := req.Header.Get("Accept-Encoding"); got != "br, gzip, zstd" {
					t.Fatalf("Accept-Encoding lost: %q", got)
				}
				raw, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				if got := decompressedTestBody(t, encoding, raw); got != `{"text":"after"}` {
					t.Fatalf("rewritten request = %q", got)
				}
				if int64(len(raw)) != req.ContentLength {
					t.Fatalf("ContentLength = %d, want %d", req.ContentLength, len(raw))
				}
				return &http.Response{
					StatusCode: http.StatusOK, Header: make(http.Header),
					Body:          io.NopCloser(strings.NewReader("ok")),
					ContentLength: 2, Request: req,
				}, nil
			}))
			req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses",
				bytes.NewReader(compressedTestBody(t, encoding, original)))
			req.Host = "api.openai.com"
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Content-Encoding", encoding)
			req.Header.Set("Accept-Encoding", "br, gzip, zstd")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandlerRewritesBrotliAndZstdResponses(t *testing.T) {
	for _, encoding := range []string{"br", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			handler := testCompressionHandler(encoding, "response", testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				raw := compressedTestBody(t, encoding, `{"text":"before"}`)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":     []string{"application/json"},
						"Content-Encoding": []string{encoding},
						"Etag":             []string{`"original"`},
					},
					Body:          io.NopCloser(bytes.NewReader(raw)),
					ContentLength: int64(len(raw)), Request: req,
				}, nil
			}))
			req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
			req.Host = "api.openai.com"
			req.Header.Set("Accept-Encoding", "br, gzip, zstd")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Encoding"); got != encoding {
				t.Fatalf("response encoding = %q", got)
			}
			if got := decompressedTestBody(t, encoding, rec.Body.Bytes()); got != `{"text":"after"}` {
				t.Fatalf("rewritten response = %q", got)
			}
			if got := rec.Header().Get("ETag"); got != "" {
				t.Fatalf("stale validator after rewrite: %s", got)
			}
		})
	}
}

func TestCompressedBodyDecompressionBombFallsBack(t *testing.T) {
	for _, encoding := range []string{"gzip", "br", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			raw := compressedTestBody(t, encoding, strings.Repeat("A", 8192))
			decoded, bounded, err := decodeBodyForRewrite(encoding, raw, 64)
			if err != nil || bounded || decoded != nil {
				t.Fatalf("limit handling: decoded=%q bounded=%v err=%v", decoded, bounded, err)
			}
		})
	}
}

func TestCompressedSSEIsUnmodified(t *testing.T) {
	for _, encoding := range []string{"gzip", "br", "zstd"} {
		t.Run(encoding, func(t *testing.T) {
			event := "event: message\\ndata: before\\n\\n"
			raw := compressedTestBody(t, encoding, event)
			handler := testCompressionHandler(encoding, "response", testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":     []string{"text/event-stream"},
						"Content-Encoding": []string{encoding},
					},
					Body:          io.NopCloser(bytes.NewReader(raw)),
					ContentLength: -1, Request: req,
				}, nil
			}))
			req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
			req.Host = "api.openai.com"
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
			}
			if !bytes.Equal(rec.Body.Bytes(), raw) {
				t.Fatal("compressed SSE must be byte-for-byte unchanged")
			}
			if got := rec.Header().Get("Content-Encoding"); got != encoding {
				t.Fatalf("SSE Content-Encoding = %q", got)
			}
		})
	}
}
