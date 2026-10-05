package domainforwardproxy

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRewrittenRequestInvalidatesBodyDigests(t *testing.T) {
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
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(body); got != `{"value":"after"}` {
			t.Fatalf("rewritten body = %q", got)
		}
		for _, name := range []string{"Content-MD5", "Digest", "Content-Digest", "Repr-Digest"} {
			if got := req.Header.Get(name); got != "" {
				t.Fatalf("%s survived request rewrite: %q", name, got)
			}
			if got := req.Trailer.Get(name); got != "" {
				t.Fatalf("%s trailer survived request rewrite: %q", name, got)
			}
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

	req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(`{"value":"before"}`))
	req.Host = "api.openai.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-MD5", "legacy")
	req.Header.Set("Digest", "sha-256=legacy")
	req.Header.Set("Content-Digest", "sha-256=:legacy:")
	req.Header.Set("Repr-Digest", "sha-256=:legacy:")
	req.Trailer = http.Header{
		"Content-Digest": []string{"sha-256=:trailer-legacy:"},
		"Repr-Digest":    []string{"sha-256=:trailer-legacy:"},
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSignedRequestBodyBypassesRewrite(t *testing.T) {
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
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(body); got != `{"value":"before"}` {
			t.Fatalf("signed request was rewritten: %q", got)
		}
		if got := req.Header.Get("Signature-Input"); got == "" {
			t.Fatal("Signature-Input was removed from bypassed signed request")
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

	req := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/responses", strings.NewReader(`{"value":"before"}`))
	req.Host = "api.openai.com"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Signature-Input", `sig1=("@method" "content-digest")`)
	req.Header.Set("Signature", "sig1=:deadbeef:")
	req.Header.Set("Content-Digest", "sha-256=:deadbeef:")

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPartialResponseBypassesRewrite(t *testing.T) {
	original := []byte(`{"value":"secret"}`)
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
		return &http.Response{
			StatusCode: http.StatusPartialContent,
			Status:     "206 Partial Content",
			Header: http.Header{
				"Content-Type":  []string{"application/json"},
				"Content-Range": []string{"bytes 0-17/100"},
				"ETag":          []string{`"range-source"`},
			},
			Body:          io.NopCloser(bytes.NewReader(original)),
			ContentLength: int64(len(original)),
			Request:       req,
		}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	req.Host = "api.openai.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusPartialContent {
		t.Fatalf("status=%d, want 206", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), original) {
		t.Fatalf("partial response was rewritten: %q", rec.Body.Bytes())
	}
	if got := rec.Header().Get("Content-Range"); got != "bytes 0-17/100" {
		t.Fatalf("Content-Range changed: %q", got)
	}
	if got := rec.Header().Get("ETag"); got != `"range-source"` {
		t.Fatalf("ETag changed on bypassed partial response: %q", got)
	}
}

func TestRequestBodyRewriteabilityProtectsSignedAndRangedBodies(t *testing.T) {
	tests := []struct {
		name   string
		header http.Header
	}{
		{name: "signature", header: http.Header{"Signature": []string{"sig1=:abc:"}}},
		{name: "signature-input", header: http.Header{"Signature-Input": []string{`sig1=("@method")`}}},
		{name: "aws-hash", header: http.Header{"X-Amz-Content-Sha256": []string{"abc"}}},
		{name: "google-hash", header: http.Header{"X-Goog-Content-Sha256": []string{"abc"}}},
		{name: "aws-auth", header: http.Header{"Authorization": []string{"AWS4-HMAC-SHA256 Credential=abc"}}},
		{name: "signature-auth", header: http.Header{"Authorization": []string{"Signature keyId=\"abc\""}}},
		{name: "azure-shared-key", header: http.Header{"Authorization": []string{"SharedKey account:signature"}}},
		{name: "content-range", header: http.Header{"Content-Range": []string{"bytes 0-3/4"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("body"))
			req.Header = tc.header
			if requestBodyIsRewriteable(req) {
				t.Fatal("protected request body unexpectedly marked rewriteable")
			}
		})
	}
}

func TestRewrittenResponseInvalidatesTrailerDigests(t *testing.T) {
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
		body := []byte(`{"value":"secret"}`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Trailer: http.Header{
				"Content-Digest": []string{"sha-256=:trailer-legacy:"},
				"Repr-Digest":    []string{"sha-256=:trailer-legacy:"},
			},
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
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
	if got := rec.Body.String(); got != `{"value":"public"}` {
		t.Fatalf("rewritten response = %q", got)
	}
	for _, name := range []string{"Content-Digest", "Repr-Digest"} {
		if got := rec.Result().Trailer.Get(name); got != "" {
			t.Fatalf("%s trailer survived response rewrite: %q", name, got)
		}
	}
}

func TestRestoreRequestBodyPreservesOriginalFraming(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("old"))
	req.ContentLength = -1
	req.TransferEncoding = []string{"chunked"}
	req.Header.Del("Content-Length")
	req.Trailer = http.Header{"X-Trace": []string{"done"}}

	restoreRequestBody(req, []byte("same"))

	if req.ContentLength != -1 {
		t.Fatalf("ContentLength = %d, want -1", req.ContentLength)
	}
	if len(req.TransferEncoding) != 1 || req.TransferEncoding[0] != "chunked" {
		t.Fatalf("TransferEncoding = %v, want original chunked framing", req.TransferEncoding)
	}
	if got := req.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length unexpectedly introduced: %q", got)
	}
	if got := req.Trailer.Get("X-Trace"); got != "done" {
		t.Fatalf("trailer changed while restoring request: %q", got)
	}
}

func TestSetRequestBodyLeavesTrailerFramingToTransport(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("old"))
	req.TransferEncoding = []string{"chunked"}
	req.Trailer = http.Header{"X-Trace": []string{"done"}}

	setRequestBody(req, []byte("rewritten"))

	if req.ContentLength != -1 {
		t.Fatalf("ContentLength = %d, want unknown length for trailer-bearing body", req.ContentLength)
	}
	if len(req.TransferEncoding) != 0 {
		t.Fatalf("TransferEncoding = %v, want transport-selected framing", req.TransferEncoding)
	}
	if got := req.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length survived trailer-bearing rewrite: %q", got)
	}
	if got := req.Trailer.Get("X-Trace"); got != "done" {
		t.Fatalf("non-integrity trailer was not preserved: %q", got)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "rewritten" {
		t.Fatalf("body = %q", got)
	}
}

func TestSetRequestBodyUsesContentLengthWithoutTrailers(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("old"))
	req.TransferEncoding = []string{"chunked"}

	setRequestBody(req, []byte("rewritten"))

	if req.ContentLength != int64(len("rewritten")) {
		t.Fatalf("ContentLength = %d", req.ContentLength)
	}
	if len(req.TransferEncoding) != 0 {
		t.Fatalf("TransferEncoding = %v, want cleared", req.TransferEncoding)
	}
	if got := req.Header.Get("Content-Length"); got != "9" {
		t.Fatalf("Content-Length = %q, want 9", got)
	}
}

func TestRequestBodyRewriteabilityProtectsDeclaredSignatureTrailer(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.test/", strings.NewReader("body"))
	req.Trailer = http.Header{
		"Signature-Input": nil,
	}
	if requestBodyIsRewriteable(req) {
		t.Fatal("declared Signature-Input trailer was not treated as signed before body read")
	}
}

func TestHeadResponsePreservesRepresentationLength(t *testing.T) {
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
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header: http.Header{
				"Content-Type":   []string{"application/json"},
				"Content-Length": []string{"1234"},
				"ETag":           []string{`"head-repr"`},
			},
			Body:          http.NoBody,
			ContentLength: 1234,
			Request:       req,
		}, nil
	}))

	req := httptest.NewRequest(http.MethodHead, "https://api.openai.com/v1/responses", nil)
	req.Host = "api.openai.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Length"); got != "1234" {
		t.Fatalf("HEAD Content-Length = %q, want 1234", got)
	}
	if got := rec.Header().Get("ETag"); got != `"head-repr"` {
		t.Fatalf("HEAD ETag changed: %q", got)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD response unexpectedly gained a body: %q", rec.Body.String())
	}
}

func TestNotModifiedResponsePreservesValidatorAndLength(t *testing.T) {
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
		return &http.Response{
			StatusCode: http.StatusNotModified,
			Status:     "304 Not Modified",
			Header: http.Header{
				"Content-Length": []string{"1234"},
				"ETag":           []string{`"cached"`},
			},
			Body:          http.NoBody,
			ContentLength: 0,
			Request:       req,
		}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	req.Host = "api.openai.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Fatalf("status=%d, want 304", rec.Code)
	}
	if got := rec.Header().Get("Content-Length"); got != "1234" {
		t.Fatalf("304 Content-Length = %q, want 1234", got)
	}
	if got := rec.Header().Get("ETag"); got != `"cached"` {
		t.Fatalf("304 ETag changed: %q", got)
	}
}

func TestResponseBodyRewriteabilityRejectsNoContentStatuses(t *testing.T) {
	for _, status := range []int{
		http.StatusContinue,
		http.StatusSwitchingProtocols,
		http.StatusNoContent,
		http.StatusNotModified,
		http.StatusPartialContent,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
			resp := &http.Response{
				StatusCode: status,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"value":"secret"}`)),
			}
			if responseBodyIsRewriteable(req, resp) {
				t.Fatalf("status %d unexpectedly marked rewriteable", status)
			}
		})
	}
}


func TestRestoreResponseBodyPreservesTrailerFraming(t *testing.T) {
	resp := &http.Response{
		Header:           http.Header{"Content-Type": []string{"application/json"}},
		Trailer:          http.Header{"X-Trace": []string{"done"}},
		Body:             io.NopCloser(strings.NewReader("old")),
		ContentLength:    -1,
		TransferEncoding: []string{"chunked"},
	}

	restoreResponseBody(resp, []byte("same"))

	if resp.ContentLength != -1 {
		t.Fatalf("ContentLength = %d, want -1", resp.ContentLength)
	}
	if len(resp.TransferEncoding) != 1 || resp.TransferEncoding[0] != "chunked" {
		t.Fatalf("TransferEncoding = %v, want original chunked framing", resp.TransferEncoding)
	}
	if got := resp.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length unexpectedly introduced: %q", got)
	}
	if got := resp.Trailer.Get("X-Trace"); got != "done" {
		t.Fatalf("response trailer changed: %q", got)
	}
}

func TestSetResponseBodyLeavesTrailerFramingToServer(t *testing.T) {
	resp := &http.Response{
		Header: http.Header{
			"Content-Type":   []string{"application/json"},
			"Content-Length": []string{"3"},
		},
		Trailer: http.Header{
			"X-Trace":        []string{"done"},
			"Content-Digest": []string{"sha-256=:old:"},
		},
		Body:             io.NopCloser(strings.NewReader("old")),
		ContentLength:    3,
		TransferEncoding: []string{"chunked"},
	}

	setResponseBody(resp, []byte("rewritten"))

	if resp.ContentLength != -1 {
		t.Fatalf("ContentLength = %d, want -1", resp.ContentLength)
	}
	if len(resp.TransferEncoding) != 0 {
		t.Fatalf("TransferEncoding = %v, want server-selected framing", resp.TransferEncoding)
	}
	if got := resp.Header.Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length survived trailer-bearing rewrite: %q", got)
	}
	if got := resp.Trailer.Get("Content-Digest"); got != "" {
		t.Fatalf("Content-Digest trailer survived rewrite: %q", got)
	}
	if got := resp.Trailer.Get("X-Trace"); got != "done" {
		t.Fatalf("non-integrity trailer was not preserved: %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "rewritten" {
		t.Fatalf("body = %q", got)
	}
}


func TestSignedURLRequestBodyBypassesRewrite(t *testing.T) {
	cases := []string{
		"https://api.openai.com/v1/responses?X-Amz-Algorithm=AWS4-HMAC-SHA256&X-Amz-Signature=deadbeef",
		"https://api.openai.com/v1/responses?X-Goog-Algorithm=GOOG4-RSA-SHA256&X-Goog-Signature=deadbeef",
		"https://api.openai.com/v1/responses?sv=2026-01-01&sig=deadbeef",
		"https://api.openai.com/v1/responses?Key-Pair-Id=K123&Signature=deadbeef",
	}
	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, rawURL, strings.NewReader(`{"value":"before"}`))
			req.Host = "api.openai.com"
			if requestBodyIsRewriteable(req) {
				t.Fatalf("signed URL unexpectedly marked rewriteable: %s", rawURL)
			}
		})
	}
}

func TestSignedResponseBodyBypassesRewrite(t *testing.T) {
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
		body := []byte(`{"value":"secret"}`)
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Header: http.Header{
				"Content-Type":    []string{"application/json"},
				"Signature-Input": []string{`sig1=("content-digest");keyid="test"`},
				"Signature":       []string{"sig1=:deadbeef:"},
			},
			Body:          io.NopCloser(bytes.NewReader(body)),
			ContentLength: int64(len(body)),
			Request:       req,
		}, nil
	}))

	req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/responses", nil)
	req.Host = "api.openai.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if got := rec.Body.String(); got != `{"value":"secret"}` {
		t.Fatalf("signed response was rewritten: %q", got)
	}
	if got := rec.Header().Get("Signature"); got != "sig1=:deadbeef:" {
		t.Fatalf("response signature changed: %q", got)
	}
}

func TestDeclaredSignedResponseTrailerBypassesRewrite(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Trailer:    http.Header{"Signature": nil},
		Body:       io.NopCloser(strings.NewReader(`{"value":"secret"}`)),
	}
	if responseBodyIsRewriteable(req, resp) {
		t.Fatal("response with declared Signature trailer unexpectedly marked rewriteable")
	}
}
