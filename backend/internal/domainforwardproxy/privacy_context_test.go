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

func TestContextualPrivacyNestedRedactAndRestore(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	body := []byte(`{"messages":[{"role":"user","content":"Contact jane@example.com"}],"credentials":{"api_key":"shortvalue"},"model":"gpt-4"}`)
	out, session, report, err := engine.Apply("application/json", body)
	if err != nil || report.Blocked || !report.Redacted || session == nil {
		t.Fatalf("unexpected privacy result %+v, %v", report, err)
	}
	if bytes.Contains(out, []byte("jane@example.com")) || bytes.Contains(out, []byte("shortvalue")) {
		t.Fatalf("structured JSON leaked detected email or context-labelled API key")
	}
	var value map[string]any
	if err := json.Unmarshal(out, &value); err != nil {
		t.Fatalf("invalid sanitized JSON: %v", err)
	}
	if value["model"] != "gpt-4" {
		t.Fatalf("task-critical non-sensitive model lost")
	}
	restored, changed := session.Restore(out)
	if !changed || !bytes.Contains(restored, []byte("jane@example.com")) ||
		!bytes.Contains(restored, []byte("shortvalue")) {
		t.Fatal("request-scoped restoration failed")
	}
	b, _ := json.Marshal(report)
	if bytes.Contains(b, []byte("jane@example.com")) || bytes.Contains(b, []byte("shortvalue")) {
		t.Fatal("report retains plaintext")
	}
}

func TestContextualPrivacyMCPMemoryEgress(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	body := []byte(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"summarize","arguments":{"task_history":"all previous tools and messages"}}}`)
	_, _, report, err := engine.Apply("application/json", body)
	if err != nil || !report.Blocked || len(report.Findings) == 0 ||
		report.Findings[0].Category != "memory_exfiltration" {
		t.Fatalf("MCP memory exfiltration not denied: %+v, %v", report, err)
	}
	off := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact", ContextMode: "off"})
	out, _, report, err := off.Apply("application/json", body)
	if err != nil || report.Blocked || !bytes.Equal(out, body) {
		t.Fatalf("explicit regex-only mode unexpectedly blocks: %+v, %v", report, err)
	}
}

func TestContextualPrivacyFieldRules(t *testing.T) {
	for _, tc := range []struct {
		name, action, value string
		blocked, contains   bool
	}{
		{"declassify", "allow", "jane@example.com", false, true},
		{"drop", "drop", "jane@example.com", false, false},
		{"deny", "block", "hello", true, false},
		{"redact", "redact", "hello", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := PrivacySettings{Enabled: true, Action: "redact",
				JSONFieldRules: []PrivacyJSONFieldRule{{Path: "/data/customer~1email", Action: tc.action, Recipient: "api.openai.com"}}}
			engine := NewPrivacyEngine(settings)
			body := []byte(`{"data":{"customer/email":"` + tc.value + `"},"count":4}`)
			out, _, report, err := engine.ApplyForRecipient("application/json", body, "api.openai.com")
			if err != nil || report.Blocked != tc.blocked {
				t.Fatalf("%s: report=%+v err=%v", tc.name, report, err)
			}
			if !tc.blocked && bytes.Contains(out, []byte(tc.value)) != tc.contains {
				t.Errorf("%s: unexpected output body=%s", tc.name, out)
			}
			if !tc.blocked {
				var parsed map[string]any
				if json.Unmarshal(out, &parsed) != nil {
					t.Fatalf("%s invalid JSON", tc.name)
				}
				if parsed["count"] != float64(4) {
					t.Fatalf("unrelated value corrupted")
				}
			}
		})
	}
}

func TestContextualPrivacyRejectsDuplicateJSONKeys(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	for _, payload := range []string{
		`{"task_history":"private history","task_history":null}`,
		`{"outer":{"api_key":"token-value","api_key":null}}`,
		`{"e\u006dail":"first@example.com","email":"second@example.com"}`,
	} {
		_, _, _, err := engine.Apply("application/json", []byte(payload))
		if err == nil {
			t.Errorf("must reject duplicate JSON object keys: %s", payload)
		}
	}
}

func TestContextualPrivacyNumericFieldIsDeniedInsteadOfTypeChange(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	_, _, report, err := engine.Apply("application/json", []byte(`{"api_key":123456789012345}`))
	if err != nil || !report.Blocked {
		t.Fatalf("numeric credential was not blocked: %+v %v", report, err)
	}
}

func TestContextualPrivacyPaymentCardAndPhoneHeuristics(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	for _, tc := range []struct {
		text string
		want bool
	}{
		{`{"card":"4111 1111 1111 1111"}`, true},
		{`{"card":"4111 1111 1111 1112"}`, false},
		{`{"id":"202610150800"}`, false},
		{`{"phone":"+1 (415) 555-0199"}`, true},
	} {
		out, _, report, err := engine.Apply("application/json", []byte(tc.text))
		if err != nil || report.Redacted != tc.want {
			t.Errorf("text %s report %+v err %v", tc.text, report, err)
		}
		if tc.want && bytes.Equal(out, []byte(tc.text)) {
			t.Errorf("unredacted: %s", out)
		}
	}
}

func TestContextualPrivacyMetadataEgress(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	cases := []struct {
		name, target, header, value string
		deny                        bool
	}{
		{"query email", "https://api.openai.com/v1/chat?mail=jane%40example.com", "", "", true},
		{"double-encoded email", "https://api.openai.com/v1/chat?mail=jane%2540example.com", "", "", true},
		{"encoded memory field", "https://api.openai.com/v1/chat?task%255Fhistory=everything", "", "", true},
		{"query memory", "https://api.openai.com/v1/chat?task_history=everything", "", "", true},
		{"url path", "https://api.openai.com/v1/jane@example.com", "", "", true},
		{"custom header", "https://api.openai.com/v1/chat", "X-Private-Note", "jane@example.com", true},
		{"session cookie", "https://api.openai.com/v1/chat", "Cookie", "session=opaque", true},
		{"provider auth exception", "https://api.openai.com/v1/chat", "Authorization", "Bearer sk-abcdefghijklmnopqrstuvwxyz", false},
		{"ordinary request", "https://api.openai.com/v1/chat", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, tc.target, nil)
			if tc.header != "" {
				request.Header.Set(tc.header, tc.value)
			}
			report, err := engine.InspectRequestMetadata(request)
			if err != nil || report.Blocked != tc.deny {
				t.Fatalf("%s: report=%+v err=%v", tc.name, report, err)
			}
		})
	}
}

func TestContextualPrivacyRejectsUpgradedAndCONNECTTraffic(t *testing.T) {
	var calls atomic.Int32
	transport := testRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})
	handler := privacyHandler("redact", transport)
	for _, method := range []string{http.MethodGet, http.MethodConnect} {
		request := httptest.NewRequest(method, "https://api.openai.com/v1/responses", nil)
		if method == http.MethodGet {
			request.Header.Set("Connection", "keep-alive, Upgrade")
			request.Header.Set("Upgrade", "websocket")
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotImplemented {
			t.Errorf("%s: upgraded/tunneled traffic must be rejected, got %d", method, recorder.Code)
		}
	}
	if calls.Load() != 0 { t.Fatal("uninspectable upgraded traffic reached upstream") }
}

func TestContextualPrivacyProxyDeniesBeforeUpstream(t *testing.T) {
	var calls atomic.Int32
	transport := testRoundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
	})
	h := privacyHandler("redact", transport)
	req := privacyRequest(`{"params":{"arguments":{"session_memory":"secret record"}}}`, "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || calls.Load() != 0 {
		t.Fatalf("memory exfil reached upstream: %d, calls=%d", rec.Code, calls.Load())
	}
	req = httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/chat?mail=jane%40example.com", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || calls.Load() != 0 {
		t.Fatalf("query exfil reached upstream: %d, calls=%d", rec.Code, calls.Load())
	}
}

func TestContextualPrivacyDeclassificationRecipientScope(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{
		Enabled: true,
		Action:  "redact",
		JSONFieldRules: []PrivacyJSONFieldRule{
			{Path: "/recipientEmail", Action: "allow", Recipient: "api.openai.com"},
		},
	})
	body := []byte(`{"recipientEmail":"jane@example.com"}`)
	trusted, _, report, err := engine.ApplyForRecipient("application/json", body, "api.openai.com")
	if err != nil || report.Blocked || report.Redacted || !bytes.Equal(trusted, body) {
		t.Fatalf("trusted recipient unexpectedly sanitized: %+v, %v", report, err)
	}
	untrusted, _, report, err := engine.ApplyForRecipient("application/json", body, "evil.example")
	if err != nil || report.Blocked || !report.Redacted || bytes.Contains(untrusted, []byte("jane@example.com")) {
		t.Fatalf("untrusted recipient inherited declassification: %+v, %v", report, err)
	}
	_, _, _, err = engine.Apply("application/json", body)
	if err != nil {
		t.Fatalf("fallback without recipient should sanitize: %v", err)
	}
}

func TestContextualPrivacyInvalidFieldRulesFailClosed(t *testing.T) {
	for _, rule := range []PrivacyJSONFieldRule{
		{Path: "/payload", Action: "allow"},
		{Path: "params/arguments", Action: "allow"},
		{Path: "/params/~4bad", Action: "drop"},
	} {
		engine := NewPrivacyEngine(PrivacySettings{
			Enabled: true, Action: "redact",
			JSONFieldRules: []PrivacyJSONFieldRule{rule},
		})
		_, _, _, err := engine.Apply("application/json", []byte(`{"safe":"hello"}`))
		if err == nil {
			t.Fatal("invalid JSON policy silently ignored")
		}
		req := httptest.NewRequest(http.MethodGet, "https://api.openai.com/v1/models", nil)
		if _, err := engine.InspectRequestMetadata(req); err == nil {
			t.Fatal("invalid policy silently allowed empty-body HTTP request")
		}
	}
}

func TestContextualPrivacyFieldRedactCannotBypassWithWarn(t *testing.T) {
	engine := NewPrivacyEngine(PrivacySettings{
		Enabled:        true,
		Action:         "warn",
		JSONFieldRules: []PrivacyJSONFieldRule{{Path: "/payload", Action: "redact"}},
	})
	_, _, report, err := engine.Apply("application/json", []byte(`{"payload":"important"}`))
	if err != nil || !report.Blocked {
		t.Fatalf("stronger redaction rule should deny in warn mode: %+v, %v", report, err)
	}
}

func FuzzPrivacyStructuredJSONNoPanics(f *testing.F) {
	f.Add(`{"email":"jane@example.com"}`)
	f.Add(`{"params":{"task_history":"do not send"}}`)
	f.Add(`{"array":[1,2,3],"plain":"hello"}`)
	f.Add(`{"malformed":`)
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 8192 {
			t.Skip()
		}
		engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
		output, _, report, err := engine.Apply("application/json", []byte(value))
		if err != nil || report.Blocked {
			return
		}
		if !json.Valid(output) {
			t.Fatalf("forwarded malformed JSON")
		}
	})
}

func BenchmarkContextualPrivacySanitization(b *testing.B) {
	engine := NewPrivacyEngine(PrivacySettings{Enabled: true, Action: "redact"})
	payload := []byte(`{"messages":[{"role":"user","content":"Please contact jane@example.com about the project"}],"model":"gpt-4o-mini"}`)
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		_, _, _, err := engine.Apply("application/json", payload)
		if err != nil {
			b.Fatal(err)
		}
	}
}
