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

func TestBuiltinMiniMaxAPIsAreProviderScoped(t *testing.T) {
	tests := []struct {
		name, host, path, want string
	}{
		{"global anthropic", "api.minimax.io", "/anthropic/v1/messages?api_key=secret", "minimax.messages"},
		{"cn anthropic", "api.minimaxi.com", "/anthropic/v1/messages", "minimax.messages"},
		{"global chat", "api.minimax.io", "/v1/chat/completions", "minimax.chat-completions"},
		{"cn legacy chat", "api.minimaxi.com", "/v1/text/chatcompletion_v2", "minimax.chat-completions"},
		{"global responses", "api.minimax.io", "/v1/responses", "minimax.responses"},
		{"cn responses", "api.minimaxi.com", "/v1/responses", "minimax.responses"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, protocol := range []string{"http1", "http2"} {
				match, ok := Default.MatchCompact(Observation{Protocol: protocol, Method: "POST", Host: tc.host, Path: tc.path})
				if !ok || match.ProfileID != tc.want || match.Vendor != "minimax" || match.Confidence < 90 {
					t.Fatalf("%s %s %s: ok=%v match=%+v", protocol, tc.host, tc.path, ok, match)
				}
			}
		})
	}
	// Compatible routes alone do not establish that a request is to MiniMax.
	for _, host := range []string{"private.example", "notapi.minimax.io.attacker.example"} {
		match, ok := Default.Match(Observation{Protocol: "http2", Method: "POST", Host: host, Path: "/v1/responses"})
		if ok && match.Vendor == "minimax" {
			t.Fatalf("non-MiniMax host %q incorrectly attributed: %+v", host, match)
		}
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

func TestParseHTTP1ResponseLine(t *testing.T) {
	line, ok := ParseHTTP1StartLine("HTTP/1.1 429 Too Many Requests\r\n")
	if !ok || line.Kind != "response" || line.Status != 429 {
		t.Fatalf("response line = %+v ok=%v", line, ok)
	}
}

func TestParseGRPCPath(t *testing.T) {
	service, method, ok := ParseGRPCPath("/google.ai.generativelanguage.v1beta.GenerativeService/GenerateContent?key=secret")
	if !ok || service != "google.ai.generativelanguage.v1beta.GenerativeService" || method != "GenerateContent" {
		t.Fatalf("grpc service=%q method=%q ok=%v", service, method, ok)
	}
}

func TestProfileSourceAndDirectionSelectors(t *testing.T) {
	registry := NewRegistry([]Profile{{
		ID: "acme.inbound", Vendor: "acme", Sources: []string{"kernel_socket_prefix"},
		Protocols: []string{"http1"}, Directions: []string{"incoming"}, Methods: []string{"POST"},
		PathPrefixes: []string{"/hook"}, MinScore: 40,
	}})
	if _, ok := registry.Match(Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Path: "/hook"}); ok {
		t.Fatal("outgoing observation unexpectedly matched inbound profile")
	}
	if match, ok := registry.Match(Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "incoming", Method: "POST", Path: "/hook"}); !ok || match.ProfileID != "acme.inbound" {
		t.Fatalf("inbound match failed: ok=%v match=%+v", ok, match)
	}
}

func TestMergeProfilesCustomOverridesBuiltinID(t *testing.T) {
	merged := MergeProfiles(BuiltinProfiles(), []Profile{{
		ID: "openai.responses", Vendor: "private-gateway", Product: "responses", Operation: "proxy.responses",
		Protocols: []string{"http2"}, HostSuffixes: []string{"gateway.example"}, PathPrefixes: []string{"/v1/responses"}, MinScore: 90,
	}})
	registry := NewRegistry(merged)
	match, ok := registry.Match(Observation{Protocol: "http2", Method: "POST", Host: "gateway.example", Path: "/v1/responses"})
	if !ok || match.Vendor != "private-gateway" || match.Operation != "proxy.responses" {
		t.Fatalf("overlay override failed: ok=%v match=%+v", ok, match)
	}
}

func TestRegistryIndexedSelectorsAndMatchAll(t *testing.T) {
	registry := NewRegistry([]Profile{
		{ID: "generic", Vendor: "generic", Protocols: []string{"http1"}, Methods: []string{"POST"}, PathPrefixes: []string{"/v1"}, MinScore: 35},
		{ID: "scoped", Vendor: "scoped", Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"}, Directions: []string{"outgoing"}, Methods: []string{"POST"}, Transports: []string{"tcp"}, Families: []string{"ipv4"}, RemotePorts: []uint32{8443}, RemoteCIDRs: []string{"10.0.0.0/8"}, Processes: []string{"curl"}, PathPrefixes: []string{"/v1"}, MinScore: 70},
	})
	observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", RemoteIP: "10.1.2.3", RemotePort: 8443, Process: "curl", Path: "/v1/jobs"}
	match, ok := registry.Match(observation)
	if !ok || match.ProfileID != "scoped" {
		t.Fatalf("indexed scoped match failed: ok=%v match=%+v", ok, match)
	}
	matches := registry.MatchAll(observation)
	if len(matches) != 2 || matches[0].ProfileID != "scoped" || matches[1].ProfileID != "generic" {
		t.Fatalf("unexpected MatchAll: %+v", matches)
	}
	stats := registry.Stats()
	if stats.Profiles != 2 || stats.DispatchBuckets == 0 || stats.Generation == 0 {
		t.Fatalf("unexpected registry stats: %+v", stats)
	}
}

func TestRegistryRejectsInvalidNetworkSelectors(t *testing.T) {
	registry := NewRegistry(nil)
	if err := registry.Replace([]Profile{{ID: "bad-port", Vendor: "x", RemotePorts: []uint32{70000}}}); err == nil {
		t.Fatal("expected invalid remote port to be rejected")
	}
	if err := registry.Replace([]Profile{{ID: "bad-cidr", Vendor: "x", RemoteCIDRs: []string{"not-a-cidr"}}}); err == nil {
		t.Fatal("expected invalid CIDR to be rejected")
	}
}

func TestRegistryMatchCompactEquivalent(t *testing.T) {
	registry := NewRegistry([]Profile{{
		ID: "compact", Vendor: "acme", Product: "jobs", Operation: "create",
		Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"}, Methods: []string{"POST"},
		HostSuffixes: []string{"api.example.test"}, PathPrefixes: []string{"/v1/jobs"}, MinScore: 90,
	}})
	observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Method: "POST", Host: "api.example.test", Path: "/v1/jobs/42"}
	detailed, ok := registry.Match(observation)
	if !ok || len(detailed.MatchedBy) == 0 {
		t.Fatalf("detailed match missing diagnostics: ok=%v match=%+v", ok, detailed)
	}
	compact, ok := registry.MatchCompact(observation)
	if !ok {
		t.Fatal("compact match failed")
	}
	if compact.ProfileID != detailed.ProfileID || compact.Score != detailed.Score || compact.Confidence != detailed.Confidence || compact.Vendor != detailed.Vendor || compact.Product != detailed.Product || compact.Operation != detailed.Operation {
		t.Fatalf("compact mismatch: detailed=%+v compact=%+v", detailed, compact)
	}
	if compact.MatchedBy != nil {
		t.Fatalf("compact match should omit diagnostics: %+v", compact.MatchedBy)
	}
}

func TestRequestPathFastPathAndAbsoluteURL(t *testing.T) {
	cases := map[string]string{
		"/v1/jobs/42":                                 "/v1/jobs/42",
		"/v1/jobs/42?token=secret#fragment":           "/v1/jobs/42",
		"v1/jobs/42?token=secret":                     "v1/jobs/42",
		"https://api.example.test/v1/jobs/42?token=x": "/v1/jobs/42",
		"//api.example.test/v1/jobs/42?token=x":       "/v1/jobs/42",
	}
	for input, want := range cases {
		if got := RequestPath(input); got != want {
			t.Fatalf("RequestPath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRegistryMatchCompactZeroAllocsForPreparedPath(t *testing.T) {
	registry := NewRegistry([]Profile{{
		ID: "zero-alloc", Vendor: "acme", Product: "jobs", Operation: "create",
		Sources: []string{"kernel_socket_prefix"}, Protocols: []string{"http1"},
		Directions: []string{"outgoing"}, Methods: []string{"POST"}, Transports: []string{"tcp"},
		HostSuffixes: []string{"target.example.test"}, PathPrefixes: []string{"/v1/jobs"}, MinScore: 90,
	}})
	observation := Observation{
		Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST",
		Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42",
	}
	allocs := testing.AllocsPerRun(1000, func() {
		match, ok := registry.MatchCompact(observation)
		if !ok || match.ProfileID != "zero-alloc" {
			panic("compact target profile did not match")
		}
	})
	if allocs != 0 {
		t.Fatalf("MatchCompact hot path allocations = %.2f, want 0", allocs)
	}
}
