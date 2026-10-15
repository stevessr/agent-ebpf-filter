package domainforwardproxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func privacyHandler(action string, upstream http.RoundTripper) *Handler {
	return NewHandlerWithTransport(DomainForwardProxySettings{
		DefaultScheme: "https",
		AllowAnyHost:  false,
		Routes:        []DomainForwardRoute{{Host: "api.openai.com", Upstream: "https://api.openai.com"}},
		Privacy: PrivacySettings{
			Enabled:         true,
			Action:          action,
			RestoreResponse: true,
			Categories:      []string{"secret", "email", "phone"},
		},
	}, upstream)
}

func privacyRequest(body, contentType string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "https://api.openai.com/v1/chat/completions", strings.NewReader(body))
	request.Host = "api.openai.com"
	request.Header.Set("Content-Type", contentType)
	return request
}

func TestPrivacyEngineJSONValueOnlyAndEphemeralRestore(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	original := []byte(`{"email":"jane@example.com","message":"jane@example.com and sk-abcdefghijklmnopqrstuv"}`)
	output, session, report, err := engine.Apply("application/json", original)
	if err != nil || session == nil || !report.Redacted || len(report.Findings) != 3 {
		t.Fatalf("unexpected privacy result: report=%+v err=%v", report, err)
	}
	if bytes.Contains(output, []byte("jane@example.com")) || bytes.Contains(output, []byte("sk-abcdefghijklmnopqrstuv")) {
		t.Fatalf("original secrets leaked in redacted body")
	}
	var values map[string]string
	if json.Unmarshal(output, &values) != nil || values["email"] == "" {
		t.Fatalf("JSON values corrupted: %s", output)
	}
	restored, ok := session.Restore(output)
	if !ok || !bytes.Equal(restored, original) {
		t.Fatalf("round-trip restoration failed: %s", restored)
	}
	output2, session2, _, _ := engine.Apply("application/json", original)
	if bytes.Equal(output, output2) {
		t.Fatal("placeholders must be unpredictable per request")
	}
	wrong, changed := session2.Restore(output)
	if changed || !bytes.Equal(wrong, output) {
		t.Fatal("session mappings must never cross requests")
	}
}

func TestPrivacyProxyBlocksBeforeUpstream(t *testing.T) {
	var calls atomic.Int32
	transport := testRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	req := privacyRequest(`{"text":"email jane@example.com"}`, "application/json")
	rec := httptest.NewRecorder()
	privacyHandler("block", transport).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || calls.Load() != 0 {
		t.Fatalf("must block before upstream: status=%d calls=%d", rec.Code, calls.Load())
	}
}

func TestPrivacyProxyRedactsBeforeUpstreamAndRestoresResponse(t *testing.T) {
	var calls atomic.Int32
	transport := testRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		raw, _ := io.ReadAll(r.Body)
		if bytes.Contains(raw, []byte("jane@example.com")) {
			t.Error("upstream saw original email")
		}
		if r.ContentLength != int64(len(raw)) {
			t.Error("invalid rewritten content-length")
		}
		return &http.Response{
			StatusCode:    200,
			Header:        http.Header{"Content-Type": []string{"application/json"}},
			Body:          io.NopCloser(bytes.NewReader(raw)),
			ContentLength: int64(len(raw)),
		}, nil
	})
	rec := httptest.NewRecorder()
	privacyHandler("redact", transport).ServeHTTP(rec, privacyRequest(`{"text":"contact jane@example.com"}`, "application/json"))
	if rec.Code != 200 || calls.Load() != 1 {
		t.Fatalf("proxy returned %d (calls %d): %s", rec.Code, calls.Load(), rec.Body.String())
	}
	if got := rec.Body.String(); got != `{"text":"contact jane@example.com"}` {
		t.Errorf("response not restored: %s", got)
	}
}

func TestPrivacyProxyWarnDoesNotRedact(t *testing.T) {
	transport := testRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		if !bytes.Contains(raw, []byte("jane@example.com")) {
			t.Error("warn mode should pass original unmodified")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	rec := httptest.NewRecorder()
	privacyHandler("warn", transport).ServeHTTP(rec, privacyRequest(`{"email":"jane@example.com"}`, "application/json"))
	if rec.Code != 200 {
		t.Fatalf("warn must pass: %d", rec.Code)
	}
}

func TestPrivacyProxyRejectsUnsupportedOrUninspectableRequests(t *testing.T) {
	var calls atomic.Int32
	transport := testRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	h := privacyHandler("redact", transport)
	for _, tc := range []struct {
		body, typ string
		sign      bool
		want      int
	}{
		{`{"email":`, "application/json", false, http.StatusUnprocessableEntity},
		{"file upload", "application/octet-stream", false, http.StatusUnprocessableEntity},
		{`{"email":"jane@example.com"}`, "application/json", true, http.StatusUnprocessableEntity},
		{strings.Repeat("A", 1024*1024+1), "text/plain", false, http.StatusRequestEntityTooLarge},
	} {
		req := privacyRequest(tc.body, tc.typ)
		if tc.sign {
			req.Header.Set("Signature-Input", `sig1=("@method" "content-digest")`)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != tc.want {
			t.Errorf("status %d want %d: %s", rec.Code, tc.want, rec.Body.String())
		}
	}
	if calls.Load() != 0 {
		t.Fatal("rejected requests reached upstream")
	}
}

func TestPrivacyProxyRestoresSSEAcrossTransportChunks(t *testing.T) {
	transport := testRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		var payload map[string]string
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		event := "data: " + string(raw) + "\n\n"
		return &http.Response{
			StatusCode:    200,
			Header:        http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:          io.NopCloser(&slowByteReader{data: []byte(event)}),
			ContentLength: -1,
		}, nil
	})
	rec := httptest.NewRecorder()
	privacyHandler("redact", transport).ServeHTTP(rec, privacyRequest(`{"text":"jane@example.com"}`, "application/json"))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "jane@example.com") {
		t.Fatalf("streaming response restoration failed: %d %s", rec.Code, rec.Body.String())
	}
}

type slowByteReader struct{ data []byte }

func (s *slowByteReader) Read(p []byte) (int, error) {
	if len(s.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, s.data[:1])
	s.data = s.data[n:]
	return n, nil
}

func TestPrivacySettingsEqualityAndFailureDefault(t *testing.T) {
	a := DomainForwardProxySettings{Privacy: PrivacySettings{Enabled: true, Action: "redact"}}
	b := a
	b.Privacy.Action = "block"
	if SettingsEqual(a, b) {
		t.Fatal("privacy changes must trigger reload")
	}
	x := PrivacySettings{Enabled: true, Action: "typo"}
	NormalizePrivacySettings(&x)
	if x.Action != "block" {
		t.Fatal("invalid action should normalize to block")
	}
}
