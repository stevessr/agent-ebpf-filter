from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    p = Path(path)
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing replacement anchor in {path}: {old[:120]!r}")
    write(path, text.replace(old, new, 1))


write("backend/app/captureprofile/profile.go", r'''package captureprofile

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync/atomic"
)

// Observation is the protocol-neutral metadata available after a capture
// source has recovered an application request. It intentionally excludes the
// request body and credentials: API fingerprinting should be possible from
// host/path/method/header names alone.
type Observation struct {
	Source      string
	Protocol    string
	Direction   string
	Method      string
	Host        string
	Path        string
	Headers     map[string]string
	ContentType string
}

// Profile describes one API fingerprint. Profiles are data rather than code so
// the matcher can be reused by TLS uprobes, kernel plaintext socket prefixes,
// proxy integrations, and future capture sources.
type Profile struct {
	ID              string   `json:"id"`
	Vendor          string   `json:"vendor"`
	Product         string   `json:"product,omitempty"`
	Operation       string   `json:"operation,omitempty"`
	Protocols       []string `json:"protocols,omitempty"`
	Methods         []string `json:"methods,omitempty"`
	HostSuffixes    []string `json:"host_suffixes,omitempty"`
	HostContains    []string `json:"host_contains,omitempty"`
	PathPrefixes    []string `json:"path_prefixes,omitempty"`
	PathContains    []string `json:"path_contains,omitempty"`
	RequiredHeaders []string `json:"required_headers,omitempty"`
	ContentTypes    []string `json:"content_types,omitempty"`
	MinScore        int      `json:"min_score,omitempty"`
}

type Match struct {
	ProfileID  string
	Vendor     string
	Product    string
	Operation  string
	Confidence uint32
	Score      int
	MatchedBy  []string
}

type profileSnapshot struct {
	profiles []Profile
}

// Registry uses immutable snapshots so capture hot paths only perform an atomic
// load. Replace validates/copies rules before publishing them, enabling future
// hot-reload endpoints without putting locks in packet processing.
type Registry struct {
	snapshot atomic.Pointer[profileSnapshot]
}

func NewRegistry(profiles []Profile) *Registry {
	r := &Registry{}
	if err := r.Replace(profiles); err != nil {
		panic(err)
	}
	return r
}

func (r *Registry) Replace(profiles []Profile) error {
	if r == nil {
		return fmt.Errorf("nil API profile registry")
	}
	validated := make([]Profile, 0, len(profiles))
	seen := make(map[string]struct{}, len(profiles))
	for _, profile := range profiles {
		profile = normalizeProfile(profile)
		if profile.ID == "" {
			return fmt.Errorf("API profile id is required")
		}
		if _, ok := seen[profile.ID]; ok {
			return fmt.Errorf("duplicate API profile id %q", profile.ID)
		}
		seen[profile.ID] = struct{}{}
		if profile.MinScore <= 0 {
			profile.MinScore = 30
		}
		validated = append(validated, profile)
	}
	sort.SliceStable(validated, func(i, j int) bool { return validated[i].ID < validated[j].ID })
	r.snapshot.Store(&profileSnapshot{profiles: validated})
	return nil
}

func (r *Registry) Profiles() []Profile {
	if r == nil || r.snapshot.Load() == nil {
		return nil
	}
	profiles := r.snapshot.Load().profiles
	out := make([]Profile, len(profiles))
	copy(out, profiles)
	return out
}

func (r *Registry) Match(observation Observation) (Match, bool) {
	if r == nil {
		return Match{}, false
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil {
		return Match{}, false
	}
	observation = normalizeObservation(observation)
	best := Match{}
	matched := false
	for _, profile := range snapshot.profiles {
		candidate, ok := matchProfile(profile, observation)
		if !ok {
			continue
		}
		if !matched || candidate.Score > best.Score || (candidate.Score == best.Score && candidate.ProfileID < best.ProfileID) {
			best = candidate
			matched = true
		}
	}
	return best, matched
}

func ParseJSON(data []byte) ([]Profile, error) {
	var profiles []Profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("decode API capture profiles: %w", err)
	}
	validator := NewRegistry(nil)
	if err := validator.Replace(profiles); err != nil {
		return nil, err
	}
	return validator.Profiles(), nil
}

func LoadJSON(path string) ([]Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read API capture profiles: %w", err)
	}
	return ParseJSON(data)
}

func normalizeProfile(profile Profile) Profile {
	profile.ID = strings.TrimSpace(profile.ID)
	profile.Vendor = strings.TrimSpace(profile.Vendor)
	profile.Product = strings.TrimSpace(profile.Product)
	profile.Operation = strings.TrimSpace(profile.Operation)
	profile.Protocols = normalizeList(profile.Protocols)
	profile.Methods = normalizeUpperList(profile.Methods)
	profile.HostSuffixes = normalizeList(profile.HostSuffixes)
	profile.HostContains = normalizeList(profile.HostContains)
	profile.PathPrefixes = normalizePathList(profile.PathPrefixes)
	profile.PathContains = normalizePathList(profile.PathContains)
	profile.RequiredHeaders = normalizeList(profile.RequiredHeaders)
	profile.ContentTypes = normalizeList(profile.ContentTypes)
	return profile
}

func normalizeObservation(observation Observation) Observation {
	observation.Source = strings.ToLower(strings.TrimSpace(observation.Source))
	observation.Protocol = strings.ToLower(strings.TrimSpace(observation.Protocol))
	observation.Direction = strings.ToLower(strings.TrimSpace(observation.Direction))
	observation.Method = strings.ToUpper(strings.TrimSpace(observation.Method))
	observation.Host = NormalizeHost(observation.Host)
	observation.Path = RequestPath(observation.Path)
	observation.ContentType = strings.ToLower(strings.TrimSpace(observation.ContentType))
	return observation
}

func matchProfile(profile Profile, observation Observation) (Match, bool) {
	score := 0
	matchedBy := make([]string, 0, 6)

	if len(profile.Protocols) != 0 {
		if !containsFold(profile.Protocols, observation.Protocol) {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "protocol")
	}
	if len(profile.Methods) != 0 {
		if observation.Method == "" || !containsFold(profile.Methods, observation.Method) {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "method")
	}

	hostMatched := false
	if observation.Host != "" {
		for _, suffix := range profile.HostSuffixes {
			if hostSuffixMatch(observation.Host, suffix) {
				hostMatched = true
				break
			}
		}
		if !hostMatched {
			for _, fragment := range profile.HostContains {
				if fragment != "" && strings.Contains(observation.Host, fragment) {
					hostMatched = true
					break
				}
			}
		}
	}
	if len(profile.HostSuffixes)+len(profile.HostContains) != 0 && !hostMatched {
		return Match{}, false
	}
	if hostMatched {
		score += 60
		matchedBy = append(matchedBy, "host")
	}

	pathMatched := false
	if observation.Path != "" {
		for _, prefix := range profile.PathPrefixes {
			if prefix != "" && strings.HasPrefix(observation.Path, prefix) {
				pathMatched = true
				break
			}
		}
		if !pathMatched {
			for _, fragment := range profile.PathContains {
				if fragment != "" && strings.Contains(observation.Path, fragment) {
					pathMatched = true
					break
				}
			}
		}
	}
	if len(profile.PathPrefixes)+len(profile.PathContains) != 0 && !pathMatched {
		return Match{}, false
	}
	if pathMatched {
		score += 30
		matchedBy = append(matchedBy, "path")
	}

	for _, header := range profile.RequiredHeaders {
		if !hasHeader(observation.Headers, header) {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "header:"+header)
	}
	if len(profile.ContentTypes) != 0 {
		matchedContentType := false
		for _, contentType := range profile.ContentTypes {
			if contentType != "" && strings.Contains(observation.ContentType, contentType) {
				matchedContentType = true
				break
			}
		}
		if !matchedContentType {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "content-type")
	}
	if score < profile.MinScore {
		return Match{}, false
	}
	confidence := uint32(score)
	if confidence > 100 {
		confidence = 100
	}
	return Match{
		ProfileID:  profile.ID,
		Vendor:     profile.Vendor,
		Product:    profile.Product,
		Operation:  profile.Operation,
		Confidence: confidence,
		Score:      score,
		MatchedBy:  matchedBy,
	}, true
}

func NormalizeHost(host string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	if strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil {
			host = parsed.Host
		}
	}
	if colon := strings.LastIndex(host, ":"); colon > 0 && !strings.Contains(host[colon+1:], "]") {
		if strings.Count(host, ":") == 1 {
			host = host[:colon]
		}
	}
	return strings.TrimSuffix(strings.Trim(host, "[]"), ".")
}

// RequestPath drops query/fragment data before matching or persistence. API
// keys are frequently placed in query strings, while provider fingerprints are
// generally stable on the path component.
func RequestPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if parsed, err := url.Parse(raw); err == nil {
		if parsed.Path != "" {
			return parsed.EscapedPath()
		}
	}
	if index := strings.IndexAny(raw, "?#"); index >= 0 {
		raw = raw[:index]
	}
	return raw
}

func ParseHTTP1RequestLine(line string) (method, path string, ok bool) {
	line = strings.TrimSpace(strings.TrimRight(line, "\x00"))
	if line == "" {
		return "", "", false
	}
	if index := strings.IndexAny(line, "\r\n"); index >= 0 {
		line = line[:index]
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", "", false
	}
	method = strings.ToUpper(parts[0])
	if !isHTTPMethod(method) {
		return "", "", false
	}
	path = RequestPath(parts[1])
	if path == "" {
		return "", "", false
	}
	return method, path, true
}

func isHTTPMethod(method string) bool {
	switch method {
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT":
		return true
	default:
		return false
	}
}

func hasHeader(headers map[string]string, wanted string) bool {
	for key := range headers {
		if strings.EqualFold(strings.TrimSpace(key), wanted) {
			return true
		}
	}
	return false
}

func hostSuffixMatch(host, suffix string) bool {
	host = NormalizeHost(host)
	suffix = NormalizeHost(suffix)
	if host == "" || suffix == "" {
		return false
	}
	if strings.HasPrefix(suffix, ".") {
		return strings.HasSuffix(host, suffix)
	}
	return host == suffix || strings.HasSuffix(host, "."+suffix)
}

func normalizeList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizeUpperList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToUpper(strings.TrimSpace(item))
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func normalizePathList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = RequestPath(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

func BuiltinProfiles() []Profile {
	return []Profile{
		{ID: "anthropic.messages", Vendor: "anthropic", Product: "messages", Operation: "messages.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.anthropic.com"}, PathPrefixes: []string{"/v1/messages"}, MinScore: 90},
		{ID: "aws-bedrock.converse", Vendor: "aws-bedrock", Product: "bedrock-runtime", Operation: "converse", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostContains: []string{"bedrock-runtime"}, PathContains: []string{"/converse"}, MinScore: 90},
		{ID: "azure-openai.deployments", Vendor: "azure-openai", Product: "azure-openai", Operation: "deployment.request", Protocols: []string{"http1", "http2"}, HostSuffixes: []string{"openai.azure.com"}, PathContains: []string{"/openai/deployments/"}, MinScore: 90},
		{ID: "cohere.chat", Vendor: "cohere", Product: "chat", Operation: "chat", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.cohere.com"}, PathPrefixes: []string{"/v2/chat", "/v1/chat"}, MinScore: 90},
		{ID: "google-gemini.generate", Vendor: "google", Product: "gemini", Operation: "generateContent", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"generativelanguage.googleapis.com"}, PathContains: []string{":generateContent", ":streamGenerateContent"}, MinScore: 90},
		{ID: "mistral.chat", Vendor: "mistral", Product: "chat", Operation: "chat.completions", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.mistral.ai"}, PathPrefixes: []string{"/v1/chat/completions"}, MinScore: 90},
		{ID: "ollama.chat", Vendor: "ollama", Product: "ollama", Operation: "chat", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, PathPrefixes: []string{"/api/chat"}, MinScore: 35},
		{ID: "ollama.generate", Vendor: "ollama", Product: "ollama", Operation: "generate", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, PathPrefixes: []string{"/api/generate"}, MinScore: 35},
		{ID: "openai.chat-completions", Vendor: "openai", Product: "chat-completions", Operation: "chat.completions.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.openai.com"}, PathPrefixes: []string{"/v1/chat/completions"}, MinScore: 90},
		{ID: "openai.embeddings", Vendor: "openai", Product: "embeddings", Operation: "embeddings.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.openai.com"}, PathPrefixes: []string{"/v1/embeddings"}, MinScore: 90},
		{ID: "openai.responses", Vendor: "openai", Product: "responses", Operation: "responses.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.openai.com"}, PathPrefixes: []string{"/v1/responses"}, MinScore: 90},
		{ID: "openrouter.chat", Vendor: "openrouter", Product: "chat-completions", Operation: "chat.completions.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"openrouter.ai"}, PathPrefixes: []string{"/api/v1/chat/completions"}, MinScore: 90},
		// Path-only compatibility profiles intentionally use a generic vendor.
		// They are used by local/plaintext proxies where the Host header is not
		// available to the kernel prefix sampler.
		{ID: "openai-compatible.chat-completions", Vendor: "openai-compatible", Product: "chat-completions", Operation: "chat.completions.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, PathPrefixes: []string{"/v1/chat/completions"}, MinScore: 35},
		{ID: "openai-compatible.responses", Vendor: "openai-compatible", Product: "responses", Operation: "responses.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, PathPrefixes: []string{"/v1/responses"}, MinScore: 35},
		{ID: "anthropic-compatible.messages", Vendor: "anthropic-compatible", Product: "messages", Operation: "messages.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, PathPrefixes: []string{"/v1/messages"}, MinScore: 35},
	}
}

var Default = NewRegistry(BuiltinProfiles())
''')

write("backend/app/captureprofile/profile_test.go", r'''package captureprofile

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
''')

write("backend/app/tls/api_fingerprint.go", r'''package tls

import (
	"strings"

	"agent-ebpf-filter/app/captureprofile"
)

func tlsCaptureProtocol(event *TLSPlaintextEvent) string {
	if event == nil {
		return ""
	}
	if strings.HasPrefix(event.Type, "http2_") {
		return "http2"
	}
	if event.Type == "http_request" || event.Type == "http_response" {
		return "http1"
	}
	return ""
}

// annotateTLSAPIFingerprint turns the protocol parser output into a common API
// capture identity. The matcher only sees sanitized metadata; bodies and secret
// query parameters are never part of the fingerprint input.
func annotateTLSAPIFingerprint(event *TLSPlaintextEvent) {
	if event == nil {
		return
	}
	protocol := tlsCaptureProtocol(event)
	if protocol == "" {
		return
	}
	event.CaptureSource = "tls_plaintext"
	event.AppProtocol = protocol
	event.RequestPath = captureprofile.RequestPath(event.URL)

	match, ok := captureprofile.Default.Match(captureprofile.Observation{
		Source:      event.CaptureSource,
		Protocol:    protocol,
		Direction:   event.Direction,
		Method:      event.Method,
		Host:        event.Host,
		Path:        event.RequestPath,
		Headers:     event.Headers,
		ContentType: event.ContentType,
	})
	if !ok {
		return
	}
	event.APIProfile = match.ProfileID
	event.APIProduct = match.Product
	event.APIOperation = match.Operation
	event.APIConfidence = match.Confidence
	if event.Vendor == "" || strings.HasSuffix(match.Vendor, "-compatible") == false {
		// A host-specific fingerprint is stronger than the historical substring
		// vendor inference. Path-only compatible profiles only fill an empty value.
		if event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {
			event.Vendor = match.Vendor
		}
	}
}
''')

write("backend/app/tls/api_fingerprint_test.go", r'''package tls

import "testing"

func TestAnnotateTLSAPIFingerprintHTTP1(t *testing.T) {
	event := &TLSPlaintextEvent{
		Type: "http_request", Direction: "send", Method: "POST",
		Host: "api.anthropic.com", URL: "/v1/messages?api_key=secret",
	}
	annotateTLSAPIFingerprint(event)
	if event.CaptureSource != "tls_plaintext" || event.AppProtocol != "http1" {
		t.Fatalf("capture metadata = %+v", event)
	}
	if event.RequestPath != "/v1/messages" || event.APIProfile != "anthropic.messages" || event.Vendor != "anthropic" {
		t.Fatalf("fingerprint = %+v", event)
	}
	if event.APIConfidence < 90 {
		t.Fatalf("confidence = %d", event.APIConfidence)
	}
}

func TestAnnotateTLSAPIFingerprintHTTP2(t *testing.T) {
	event := &TLSPlaintextEvent{
		Type: "http2_headers", Direction: "send", Method: "POST",
		Host: "generativelanguage.googleapis.com", URL: "/v1beta/models/gemini:streamGenerateContent?key=secret",
	}
	annotateTLSAPIFingerprint(event)
	if event.AppProtocol != "http2" || event.RequestPath != "/v1beta/models/gemini:streamGenerateContent" || event.APIProfile != "google-gemini.generate" {
		t.Fatalf("fingerprint = %+v", event)
	}
}
''')

replace_once("backend/app/tls/capturetypestls.go", '''\tVendor       string `json:"vendor,omitempty"`\n\tLoopAlert    bool   `json:"loop_alert,omitempty"`\n''', '''\tVendor       string `json:"vendor,omitempty"`\n\tLoopAlert    bool   `json:"loop_alert,omitempty"`\n\n\t// Generalized API-capture metadata. These fields are derived from\n\t// sanitized protocol metadata and are shared across TLS libraries.\n\tCaptureSource string `json:"capture_source,omitempty"`\n\tAppProtocol   string `json:"app_protocol,omitempty"`\n\tRequestPath   string `json:"request_path,omitempty"`\n\tAPIProfile    string `json:"api_profile,omitempty"`\n\tAPIProduct    string `json:"api_product,omitempty"`\n\tAPIOperation  string `json:"api_operation,omitempty"`\n\tAPIConfidence uint32 `json:"api_confidence,omitempty"`\n''')

replace_once("backend/app/tls/agentstreamlooptls.go", '''\tenrichTLSEventWithAgentContext(event)\n\tannotateTLSAgentMessage(event)\n''', '''\tenrichTLSEventWithAgentContext(event)\n\tannotateTLSAPIFingerprint(event)\n\tannotateTLSAgentMessage(event)\n''')

replace_once("backend/app/tls/agentstreamlooptls.go", '''\t\tHttpHost:       host,\n\t\tNetEndpoint:    host,\n\t\tNetDirection:   source.Direction,\n\t\tAppProtocol:    "tls_plaintext",\n\t\tServiceName:    source.Vendor,\n''', '''\t\tHttpHost:       host,\n\t\tNetEndpoint:    host,\n\t\tNetDirection:   source.Direction,\n\t\tAppProtocol:    source.AppProtocol,\n\t\tServiceName:    source.Vendor,\n\t\tCaptureSource:  source.CaptureSource,\n\t\tApiProfile:     source.APIProfile,\n\t\tApiVendor:      source.Vendor,\n\t\tApiProduct:     source.APIProduct,\n\t\tApiOperation:   source.APIOperation,\n\t\tHttpMethod:     source.Method,\n\t\tHttpPath:       source.RequestPath,\n\t\tApiConfidence: source.APIConfidence,\n''')

replace_once("proto/tracker_common.proto", '''  OBSERVE_NAVIGATE = 42;\n}''', '''  OBSERVE_NAVIGATE = 42;\n  SOCKET_HTTP = 43;\n}''')

replace_once("proto/tracker_events.proto", '''  uint64 kernel_reserve_failures_total = 81;\n}''', '''  uint64 kernel_reserve_failures_total = 81;\n  string capture_source = 82;\n  string api_profile = 83;\n  string api_vendor = 84;\n  string api_product = 85;\n  string api_operation = 86;\n  string http_method = 87;\n  string http_path = 88;\n  uint32 api_confidence = 89;\n  uint32 kernel_payload_prefix_len = 90;\n  uint32 kernel_capture_flags = 91;\n  int32 kernel_socket_fd = 92;\n}''')

replace_once("proto/tracker_events.proto", '''  string vendor = 15;\n}\n\nmessage HTTPEvent''', '''  string vendor = 15;\n  string capture_source = 16;\n  string app_protocol = 17;\n  string request_path = 18;\n  string api_profile = 19;\n  string api_product = 20;\n  string api_operation = 21;\n  uint32 api_confidence = 22;\n}\n\nmessage HTTPEvent''')

# Add socket fd provenance map and bounded HTTP request-line capture helpers.
replace_once("backend/ebpf/agent_tracker_common.h", '''#define TYPE_DNS_QUERY 34\n''', '''#define TYPE_DNS_QUERY 34\n#define TYPE_SOCKET_HTTP 43\n\n#define SOCKET_CAPTURE_HTTP1_REQUEST_LINE (1U << 0)\n''')

replace_once("backend/ebpf/agent_tracker_common.h", '''struct {\n    __uint(type, BPF_MAP_TYPE_HASH);\n    __uint(max_entries, 10240);\n    __type(key, u64);\n    __type(value, struct exit_meta);\n} exit_ctx SEC(".maps");\n''', '''struct {\n    __uint(type, BPF_MAP_TYPE_HASH);\n    __uint(max_entries, 10240);\n    __type(key, u64);\n    __type(value, struct exit_meta);\n} exit_ctx SEC(".maps");\n\nstruct socket_fd_key {\n    u32 tgid;\n    s32 fd;\n};\n\nstruct socket_fd_meta {\n    u32 family;\n    u32 sock_type;\n    u32 protocol;\n    u32 remote_port;\n    char remote_addr[16];\n};\n\n// LRU bounds stale descriptors when a process exits or uses close paths that\n// are not observed. Keys include TGID so fd reuse in another process cannot\n// inherit provenance.\nstruct {\n    __uint(type, BPF_MAP_TYPE_LRU_HASH);\n    __uint(max_entries, 16384);\n    __type(key, struct socket_fd_key);\n    __type(value, struct socket_fd_meta);\n} socket_fds SEC(".maps");\n''')

replace_once("backend/ebpf/agent_tracker_common.h", '''static __always_inline void store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {\n    bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);\n}\n''', r'''static __always_inline void store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);
}

static __always_inline struct socket_fd_meta *lookup_socket_fd(u32 tgid, s32 fd) {
    struct socket_fd_key key = {.tgid = tgid, .fd = fd};
    return bpf_map_lookup_elem(&socket_fds, &key);
}

static __always_inline void remember_socket_fd(u32 tgid, s32 fd, u32 family, u32 sock_type, u32 protocol) {
    if (fd < 0) return;
    struct socket_fd_key key = {.tgid = tgid, .fd = fd};
    struct socket_fd_meta value = {
        .family = family,
        .sock_type = sock_type,
        .protocol = protocol,
    };
    bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY);
}

static __always_inline void update_socket_fd_remote(u32 tgid, s32 fd, struct exit_meta *meta) {
    if (fd < 0 || !meta) return;
    struct socket_fd_key key = {.tgid = tgid, .fd = fd};
    struct socket_fd_meta value = {};
    struct socket_fd_meta *existing = bpf_map_lookup_elem(&socket_fds, &key);
    if (existing) __builtin_memcpy(&value, existing, sizeof(value));
    value.family = meta->net_family;
    value.remote_port = meta->net_port;
    __builtin_memcpy(value.remote_addr, meta->net_addr, sizeof(value.remote_addr));
    bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY);
}

static __always_inline void forget_socket_fd(u32 tgid, s32 fd) {
    struct socket_fd_key key = {.tgid = tgid, .fd = fd};
    bpf_map_delete_elem(&socket_fds, &key);
}

static __always_inline void fill_network_meta_from_socket(struct exit_meta *meta, struct socket_fd_meta *socket, u32 bytes) {
    if (!meta || !socket) return;
    meta->net_family = socket->family;
    meta->net_direction = NET_DIR_OUTGOING;
    meta->net_bytes = bytes;
    meta->net_port = socket->remote_port;
    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));
}

static __always_inline int looks_like_http1_method(const char *head, u32 len) {
    if (!head || len < 4) return 0;
    if (head[0] == 'G' && head[1] == 'E' && head[2] == 'T' && head[3] == ' ') return 1;
    if (head[0] == 'P' && head[1] == 'O' && head[2] == 'S' && head[3] == 'T' && len >= 5 && head[4] == ' ') return 1;
    if (head[0] == 'P' && head[1] == 'U' && head[2] == 'T' && head[3] == ' ') return 1;
    if (head[0] == 'H' && head[1] == 'E' && head[2] == 'A' && head[3] == 'D' && len >= 5 && head[4] == ' ') return 1;
    if (head[0] == 'P' && head[1] == 'A' && head[2] == 'T' && head[3] == 'C' && len >= 6 && head[4] == 'H' && head[5] == ' ') return 1;
    if (head[0] == 'D' && head[1] == 'E' && head[2] == 'L' && head[3] == 'E' && len >= 7 && head[4] == 'T' && head[5] == 'E' && head[6] == ' ') return 1;
    if (head[0] == 'O' && head[1] == 'P' && head[2] == 'T' && head[3] == 'I' && len >= 8 && head[4] == 'O' && head[5] == 'N' && head[6] == 'S' && head[7] == ' ') return 1;
    if (head[0] == 'C' && head[1] == 'O' && head[2] == 'N' && head[3] == 'N' && len >= 8 && head[4] == 'E' && head[5] == 'C' && head[6] == 'T' && head[7] == ' ') return 1;
    return 0;
}

// Copy only the HTTP/1 request line, never headers/body. This keeps the kernel
// sampler useful for request-path discovery while avoiding Authorization/Cookie
// material. Query strings are removed in userspace before persistence.
static __always_inline u32 capture_http1_request_line(char *dst, const void *user_buf, u32 len) {
    if (!dst || !user_buf || len < 4) return 0;
    char head[8] = {};
    u32 head_len = len < sizeof(head) ? len : sizeof(head);
    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return 0;
    if (!looks_like_http1_method(head, head_len)) return 0;

    u32 capture_len = len;
    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;
    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) return 0;
#pragma clang loop unroll(disable)
    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {
        if ((u32)i >= capture_len) break;
        if (dst[i] == '\r' || dst[i] == '\n') {
            dst[i] = '\0';
            return (u32)i;
        }
    }
    dst[capture_len] = '\0';
    return capture_len;
}
''')

# Connect now remembers the descriptor and updates its remote endpoint after success.
replace_once("backend/ebpf/agent_tracker_common.h", '''    meta.type = TYPE_CONNECT;\n    meta.tag_id = tag_id;\n    fill_network_meta(&meta, (const void *)ctx->args[1], NET_DIR_OUTGOING, 0);\n''', '''    meta.type = TYPE_CONNECT;\n    meta.tag_id = tag_id;\n    meta.extra1 = (u32)ctx->args[0]; // fd for socket provenance\n    fill_network_meta(&meta, (const void *)ctx->args[1], NET_DIR_OUTGOING, 0);\n''')

replace_once("backend/ebpf/agent_tracker_common.h", '''    struct event *e = reserve_event();\n    if (!e) return 0;\n    fill_from_exit_meta(e, pid_tgid, &meta);\n    e->retval = ctx->ret;\n\n    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);\n''', '''    if (ctx->ret == 0) {\n        update_socket_fd_remote((u32)(pid_tgid >> 32), (s32)meta.extra1, &meta);\n    }\n\n    struct event *e = reserve_event();\n    if (!e) return 0;\n    fill_from_exit_meta(e, pid_tgid, &meta);\n    e->retval = ctx->ret;\n\n    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);\n''')

# Replace generic socket/write handlers with provenance-aware versions.
replace_once("backend/ebpf/agent_tracker_syscalls.h", '''DEFINE_SIMPLE_ENTER_HANDLER(socket, TYPE_SOCKET, "socket create")\nDEFINE_GENERIC_EXIT_HANDLER(socket)\n''', r'''SEC("tracepoint/syscalls/sys_enter_socket")
int tracepoint__syscalls__sys_enter_socket(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {
        .type = TYPE_SOCKET,
        .tag_id = tag_id,
        .extra1 = (u32)ctx->args[0],
        .extra2 = (u32)ctx->args[1],
        .extra3 = (u32)ctx->args[2],
    };
    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket create", 14);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")
int tracepoint__syscalls__sys_exit_socket(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    if (ctx->ret >= 0) {
        remember_socket_fd((u32)(pid_tgid >> 32), (s32)ctx->ret, meta.extra1, meta.extra2, (u32)meta.extra3);
    }
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }
    submit_event(e);
    return 0;
}
''')

replace_once("backend/ebpf/agent_tracker_syscalls.h", '''DEFINE_SIMPLE_ENTER_HANDLER(write, TYPE_WRITE, "file write")\nDEFINE_GENERIC_EXIT_HANDLER(write)\n''', r'''SEC("tracepoint/syscalls/sys_enter_write")
int tracepoint__syscalls__sys_enter_write(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;

    s32 fd = (s32)ctx->args[0];
    u32 requested = (u32)ctx->args[2];
    struct socket_fd_meta *socket = lookup_socket_fd(tgid, fd);
    struct exit_meta meta = {.type = TYPE_WRITE, .tag_id = tag_id, .extra1 = (u32)fd, .extra3 = requested};
    if (socket) fill_network_meta_from_socket(&meta, socket, requested);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        if (socket) {
            u32 captured = capture_http1_request_line(pd->extra4, (const void *)ctx->args[1], requested);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                __builtin_memcpy(pd->path, "socket write", 13);
            }
        } else {
            __builtin_memcpy(pd->path, "file write", 11);
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}
DEFINE_GENERIC_EXIT_HANDLER(write)
''')

replace_once("backend/ebpf/agent_tracker_syscalls.h", '''    struct exit_meta meta = {.type = TYPE_SENDTO, .tag_id = tag_id};\n    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);\n    meta.extra3 = (u32)ctx->args[2];\n''', '''    struct exit_meta meta = {.type = TYPE_SENDTO, .tag_id = tag_id};\n    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);\n    meta.extra1 = (u32)ctx->args[0];\n    meta.extra3 = (u32)ctx->args[2];\n''')

replace_once("backend/ebpf/agent_tracker_syscalls.h", '''        __builtin_memcpy(pd->path, "socket sendto", 14);\n        u32 data_len = (u32)ctx->args[2];\n        if (tag_id != 0 && data_len > 0 && data_len <= (MAX_PATH_LEN - 1)) {\n            u32 capture_len = data_len & (MAX_PATH_LEN - 1);\n            bpf_probe_read_user(pd->extra4, capture_len, (const void *)ctx->args[1]);\n            pd->extra4[capture_len] = '\\0';\n        }\n''', '''        __builtin_memcpy(pd->path, "socket sendto", 14);\n        u32 data_len = (u32)ctx->args[2];\n        u32 captured = capture_http1_request_line(pd->extra4, (const void *)ctx->args[1], data_len);\n        if (captured > 0) {\n            meta.type = TYPE_SOCKET_HTTP;\n            meta.extra2 = captured;\n            __builtin_memcpy(pd->path, "socket http", 12);\n        }\n''')

# Track close so fd reuse cannot inherit old socket provenance.
insert_anchor = '''DEFINE_GENERIC_EXIT_HANDLER(recvfrom)\n\n// Add remaining handlers using macros...\n'''
insert_close = r'''DEFINE_GENERIC_EXIT_HANDLER(recvfrom)

SEC("tracepoint/syscalls/sys_enter_close")
int tracepoint__syscalls__sys_enter_close(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {.type = TYPE_GENERIC_SYSCALL, .tag_id = tag_id, .extra1 = 3, .extra2 = (u32)ctx->args[0]};
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_close")
int tracepoint__syscalls__sys_exit_close(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    if (ctx->ret == 0) forget_socket_fd((u32)(pid_tgid >> 32), (s32)meta.extra2);
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    submit_event(e);
    return 0;
}

// Add remaining handlers using macros...
'''
replace_once("backend/ebpf/agent_tracker_syscalls.h", insert_anchor, insert_close)

replace_once("backend/app/runtime_ebpf.go", '''var mapNames = []string{"agent_pids", "events", "collector_stats", "tracked_comms", "tracked_paths", "tracked_prefixes", "exit_ctx", "exit_path_buf", "exit_path_ctx"}\n''', '''var mapNames = []string{"agent_pids", "events", "collector_stats", "tracked_comms", "tracked_paths", "tracked_prefixes", "exit_ctx", "exit_path_buf", "exit_path_ctx", "socket_fds"}\n''')

replace_once("backend/app/runtime_ebpf.go", '''\t\t"tracked_prefixes": objs.TrackedPrefixes, "exit_ctx": objs.ExitCtx,\n\t\t"exit_path_buf": objs.ExitPathBuf, "exit_path_ctx": objs.ExitPathCtx,\n''', '''\t\t"tracked_prefixes": objs.TrackedPrefixes, "exit_ctx": objs.ExitCtx,\n\t\t"exit_path_buf": objs.ExitPathBuf, "exit_path_ctx": objs.ExitPathCtx,\n\t\t"socket_fds": objs.SocketFds,\n''')

replace_once("backend/app/runtime_ebpf.go", '''\t\tTrackedPrefixes: maps["tracked_prefixes"],\n\t}, nil\n''', '''\t\tTrackedPrefixes: maps["tracked_prefixes"],\n\t\tSocketFds:       maps["socket_fds"],\n\t}, nil\n''')

replace_once("backend/core/types.go", '''\tCollectorStats  *ebpf.Map\n}\n''', '''\tCollectorStats  *ebpf.Map\n\tSocketFds       *ebpf.Map\n}\n''')

# Generic API metadata on kernel socket HTTP events.
replace_once("backend/app/events/events_network.go", '''import (\n\t"fmt"\n''', '''import (\n\t"agent-ebpf-filter/app/captureprofile"\n\t"fmt"\n''')

replace_once("backend/app/events/events_network.go", '''\tcase 34:\n\t\treturn "dns_query"\n\tdefault:\n''', '''\tcase 34:\n\t\treturn "dns_query"\n\tcase 43:\n\t\treturn "socket_http"\n\tdefault:\n''')

replace_once("backend/app/events/events_network.go", '''\t\t"tcp_connect", "tcp_close", "tcp_state_change", "dns_query":\n''', '''\t\t"tcp_connect", "tcp_close", "tcp_state_change", "dns_query", "socket_http":\n''')

replace_once("backend/app/events/events_network.go", '''\tcase "dns_query":\n\t\tout.NetDirection = "outgoing"\n\t\tout.NetFamily = "AF_INET"\n\t\tout.NetEndpoint = fmt.Sprintf("dns:%d", event.NetPort)\n\t\tout.Domain = SanitizeUTF8(event.Path[:])\n\tdefault:\n''', r'''\tcase "dns_query":
		out.NetDirection = "outgoing"
		out.NetFamily = "AF_INET"
		out.NetEndpoint = fmt.Sprintf("dns:%d", event.NetPort)
		out.Domain = SanitizeUTF8(event.Path[:])
	case "socket_http":
		method, requestPath, ok := captureprofile.ParseHTTP1RequestLine(extraPath)
		out.CaptureSource = "kernel_socket_prefix"
		out.AppProtocol = "http1"
		out.KernelSocketFd = int32(event.Extra1)
		out.KernelPayloadPrefixLen = event.Extra2
		out.KernelCaptureFlags = 1
		out.Bytes = event.Extra3
		if ok {
			out.HttpMethod = method
			out.HttpPath = requestPath
			// Drop the raw request line immediately so query parameters never
			// leave the kernel-event normalization boundary.
			out.ExtraPath = method + " " + requestPath
			if match, matched := captureprofile.Default.Match(captureprofile.Observation{
				Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing",
				Method: method, Path: requestPath,
			}); matched {
				out.ApiProfile = match.ProfileID
				out.ApiVendor = match.Vendor
				out.ApiProduct = match.Product
				out.ApiOperation = match.Operation
				out.ApiConfidence = match.Confidence
				out.ServiceName = match.Vendor
			}
		}
		out.ExtraInfo = fmt.Sprintf("fd=%d capture=request-line prefix_len=%d requested=%d", int32(event.Extra1), event.Extra2, event.Extra3)
	default:
'''.replace('\\t','\t'))

write("backend/app/events/kernel_socket_http_test.go", r'''package events

import "testing"

func TestBuildKernelSocketHTTPEventSanitizesAndClassifiesPath(t *testing.T) {
	var raw BpfEvent
	raw.PID = 123
	raw.TGID = 123
	raw.Type = 43
	raw.Extra1 = 9
	raw.Extra2 = 53
	raw.Extra3 = 4096
	raw.NetFamily = 2
	raw.NetDirection = 1
	raw.NetPort = 8080
	raw.NetAddr[0] = 127
	raw.NetAddr[3] = 1
	copy(raw.Comm[:], "agent")
	copy(raw.Path[:], "socket http")
	copy(raw.Extra4[:], "POST /v1/responses?api_key=top-secret HTTP/1.1")

	event := BuildKernelEvent(raw)
	if event.GetType() != "socket_http" || event.GetCaptureSource() != "kernel_socket_prefix" || event.GetAppProtocol() != "http1" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.GetHttpMethod() != "POST" || event.GetHttpPath() != "/v1/responses" {
		t.Fatalf("method/path = %q %q", event.GetHttpMethod(), event.GetHttpPath())
	}
	if event.GetExtraPath() != "POST /v1/responses" {
		t.Fatalf("raw query leaked through ExtraPath: %q", event.GetExtraPath())
	}
	if event.GetApiProfile() != "openai-compatible.responses" || event.GetApiVendor() != "openai-compatible" {
		t.Fatalf("profile = %q vendor=%q", event.GetApiProfile(), event.GetApiVendor())
	}
	if event.GetKernelSocketFd() != 9 || event.GetKernelPayloadPrefixLen() != 53 || event.GetKernelCaptureFlags() != 1 {
		t.Fatalf("kernel capture metadata missing: %+v", event)
	}
}
''')

# Typed TLS envelope payload carries the generic profile as well.
replace_once("backend/app/events/envelope_event.go", '''\treturn &pb.TLSEvent{\n\t\tDirection:      event.GetNetDirection(),\n''', '''\treturn &pb.TLSEvent{\n\t\tDirection:      event.GetNetDirection(),\n''')

# Add fields near the end of the TLSEvent literal. The anchor is stable in the current tree.
replace_once("backend/app/events/envelope_event.go", '''\t\tVendor:         event.GetServiceName(),\n\t}\n}\n''', '''\t\tVendor:         event.GetServiceName(),\n\t\tCaptureSource:  event.GetCaptureSource(),\n\t\tAppProtocol:    event.GetAppProtocol(),\n\t\tRequestPath:    event.GetHttpPath(),\n\t\tApiProfile:     event.GetApiProfile(),\n\t\tApiProduct:     event.GetApiProduct(),\n\t\tApiOperation:   event.GetApiOperation(),\n\t\tApiConfidence:  event.GetApiConfidence(),\n\t}\n}\n''')

# Frontend type can render the metadata without casts.
replace_once("frontend/src/types/tls.ts", '''  vendor?: string;\n''', '''  vendor?: string;\n  capture_source?: string;\n  app_protocol?: string;\n  request_path?: string;\n  api_profile?: string;\n  api_product?: string;\n  api_operation?: string;\n  api_confidence?: number;\n''')

write("docs/backend/generic-api-capture.md", r'''# Generic API capture pipeline

The capture path is intentionally split into two layers.

## Kernel provenance layer

The main tracker keeps an LRU `socket_fds` map keyed by `(tgid, fd)`. `socket()` seeds the map, a successful `connect()` updates the remote endpoint, and `close()` deletes the descriptor. This lets the `write()` tracepoint distinguish a confirmed socket from an ordinary file descriptor without trying to infer descriptor type from payload bytes.

For confirmed sockets, the eBPF program performs a bounded HTTP/1 method check and copies **only the request line** (maximum 255 bytes) into the event scratch buffer. Headers and bodies are deliberately not copied, so Authorization/Cookie material is outside this kernel capture path. A matching event is emitted as `SOCKET_HTTP` with the fd, captured prefix length, requested byte count, endpoint metadata, and the same kernel audit generation/CPU sequence/loss provenance as every other tracker event.

`sendto()` uses the same request-line helper. Non-HTTP socket writes remain ordinary `write`/`sendto` events and do not carry payload prefixes.

The userspace normalization boundary immediately strips query and fragment data before the request line enters `pb.Event`, preventing API keys embedded in query strings from being persisted in `extra_path`.

## Protocol/API fingerprint layer

`backend/app/captureprofile` is a protocol-neutral immutable-snapshot matcher. Inputs are metadata only: capture source, protocol, method, host, path, header names and content type. Profiles are JSON-compatible data with host suffix/substring, path prefix/substring, method, protocol, required-header and content-type constraints.

The same matcher is used for:

- OpenSSL/BoringSSL plaintext capture;
- Go `crypto/tls` plaintext capture;
- GnuTLS/NSS/rustls plaintext capture;
- HTTP/1 parser output;
- HTTP/2 HPACK-decoded `:authority` / `:path` metadata;
- kernel plaintext `SOCKET_HTTP` request-line events;
- future proxy, QUIC decryption, language-runtime and SDK-specific capture sources.

Built-in profiles cover OpenAI, OpenAI-compatible endpoints, Anthropic, Anthropic-compatible endpoints, Google Gemini, Azure OpenAI, AWS Bedrock, OpenRouter, Mistral, Cohere and Ollama. The matcher publishes rule snapshots atomically through `Registry.Replace`, so a runtime configuration endpoint/file watcher can replace profiles without locks on the capture hot path.

Provider-specific host matches score above path-only compatibility rules. Consequently `/v1/responses` observed without a host is labeled `openai-compatible`, while the same path with `api.openai.com` is labeled `openai`.

## Privacy and trust boundaries

This feature is metadata-first, not a blanket packet dumper. Kernel capture does not copy headers or bodies. TLS capture keeps the existing bounded/opt-in plaintext policy and existing header/body redaction. API fingerprinting never needs credentials or prompt contents.

Kernel `SOCKET_HTTP` events inherit the tracker audit tuple (`kernel_audit_generation`, per-CPU sequence, capture timestamp, reserve-loss counters), and the persisted deterministic protobuf hash chain therefore binds the capture provenance and normalized API metadata together.

## Known next steps

The socket-fd map currently follows `socket`, `connect`, `write`, `sendto`, and `close`. Descriptor duplication (`dup*`), inherited sockets across fork, scatter/gather `writev/sendmsg`, and decrypted HTTP/3/QUIC are separate follow-ups. TLS/library capture already remains the authoritative source for encrypted HTTP request paths.
''')
