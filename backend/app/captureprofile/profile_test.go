package captureprofile

import "testing"

func TestBuiltinSpecificProviderBeatsCompatibilityProfile(t *testing.T) {
	match, ok := Default.Match(Observation{Protocol: "http2", Method: "POST", Host: "api.openai.com", Path: "/v1/responses?api_key=secret"})
	if !ok {
		t.Fatal("expected match")
	}
	if match.ProfileID != "openai.responses" || match.Vendor != "openai" || match.Operation != "responses.create" {
		t.Fatalf("unexpected match: %+v", match)
	}
}

func TestBuiltinPathOnlyCompatibilityMatch(t *testing.T) {
	match, ok := Default.Match(Observation{Protocol: "http1", Method: "POST", Path: "/v1/chat/completions?token=secret"})
	if !ok {
		t.Fatal("expected compatibility match")
	}
	if match.ProfileID != "openai-compatible.chat-completions" || match.Vendor != "openai-compatible" {
		t.Fatalf("unexpected match: %+v", match)
	}
}

func TestBuiltinGeminiPathContains(t *testing.T) {
	match, ok := Default.Match(Observation{Protocol: "http2", Method: "POST", Host: "generativelanguage.googleapis.com", Path: "/v1beta/models/gemini-2.5-pro:generateContent"})
	if !ok || match.ProfileID != "google-gemini.generate" {
		t.Fatalf("unexpected Gemini match: ok=%v match=%+v", ok, match)
	}
}

func TestRequestPathDropsSensitiveQuery(t *testing.T) {
	if got := RequestPath("/v1/responses?api_key=top-secret&safe=yes"); got != "/v1/responses" {
		t.Fatalf("RequestPath = %q", got)
	}
}

func TestParseHTTP1RequestLine(t *testing.T) {
	method, path, ok := ParseHTTP1RequestLine("POST /v1/messages?x=secret HTTP/1.1\r\n")
	if !ok || method != "POST" || path != "/v1/messages" {
		t.Fatalf("parsed method=%q path=%q ok=%v", method, path, ok)
	}
}

func TestRegistryReplaceCustomProfile(t *testing.T) {
	profiles, err := ParseJSON([]byte(`[{"id":"acme.jobs","vendor":"acme","product":"jobs","operation":"create","protocols":["http1"],"methods":["POST"],"host_suffixes":["api.acme.test"],"path_prefixes":["/v9/jobs"],"min_score":90}]`))
	if err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(nil)
	if err := registry.Replace(profiles); err != nil {
		t.Fatal(err)
	}
	match, ok := registry.Match(Observation{Protocol: "http1", Method: "POST", Host: "api.acme.test", Path: "/v9/jobs/123"})
	if !ok || match.ProfileID != "acme.jobs" || match.Vendor != "acme" {
		t.Fatalf("custom match failed: ok=%v match=%+v", ok, match)
	}
}
