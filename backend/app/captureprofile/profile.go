package captureprofile

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
