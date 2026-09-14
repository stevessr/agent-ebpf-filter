package captureprofile

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
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
	Transport   string
	Family      string
	RemoteIP    string
	RemotePort  uint32
	Process     string
}

// Profile describes one API fingerprint. Profiles are data rather than code so
// the matcher can be reused by TLS uprobes, kernel plaintext socket prefixes,
// proxy integrations, and future capture sources.
type Profile struct {
	ID              string   `json:"id"`
	Vendor          string   `json:"vendor"`
	Product         string   `json:"product,omitempty"`
	Operation       string   `json:"operation,omitempty"`
	Sources         []string `json:"sources,omitempty"`
	Protocols       []string `json:"protocols,omitempty"`
	Directions      []string `json:"directions,omitempty"`
	Methods         []string `json:"methods,omitempty"`
	HostSuffixes    []string `json:"host_suffixes,omitempty"`
	HostContains    []string `json:"host_contains,omitempty"`
	PathPrefixes    []string `json:"path_prefixes,omitempty"`
	PathContains    []string `json:"path_contains,omitempty"`
	RequiredHeaders []string `json:"required_headers,omitempty"`
	ContentTypes    []string `json:"content_types,omitempty"`
	Transports      []string `json:"transports,omitempty"`
	Families        []string `json:"families,omitempty"`
	RemotePorts     []uint32 `json:"remote_ports,omitempty"`
	RemoteCIDRs     []string `json:"remote_cidrs,omitempty"`
	Processes       []string `json:"processes,omitempty"`
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
	profiles   []Profile
	index      *profileDispatchIndex
	generation uint64
}

// Registry uses immutable snapshots so capture hot paths only perform an atomic
// load. Replace validates/copies rules before publishing them, enabling future
// hot-reload endpoints without putting locks in packet processing.
type Registry struct {
	snapshot   atomic.Pointer[profileSnapshot]
	generation atomic.Uint64
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
	index, err := compileProfileDispatch(validated)
	if err != nil {
		return err
	}
	generation := r.generation.Add(1)
	index.stats.Generation = generation
	r.snapshot.Store(&profileSnapshot{profiles: validated, index: index, generation: generation})
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
	if snapshot == nil || snapshot.index == nil {
		return Match{}, false
	}
	return matchIndexedSnapshot(snapshot, observation, true)
}

// MatchCompact runs the same matcher as Match but omits diagnostic MatchedBy
// materialization. Production capture paths use this to avoid per-event
// diagnostic slice allocation; control-plane preview keeps using Match.
func (r *Registry) MatchCompact(observation Observation) (Match, bool) {
	if r == nil {
		return Match{}, false
	}
	snapshot := r.snapshot.Load()
	if snapshot == nil || snapshot.index == nil {
		return Match{}, false
	}
	return matchIndexedSnapshot(snapshot, observation, false)
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
	profile.Sources = normalizeList(profile.Sources)
	profile.Protocols = normalizeList(profile.Protocols)
	profile.Directions = normalizeList(profile.Directions)
	profile.Methods = normalizeUpperList(profile.Methods)
	profile.HostSuffixes = normalizeList(profile.HostSuffixes)
	profile.HostContains = normalizeList(profile.HostContains)
	profile.PathPrefixes = normalizePathList(profile.PathPrefixes)
	profile.PathContains = normalizePathList(profile.PathContains)
	profile.RequiredHeaders = normalizeList(profile.RequiredHeaders)
	profile.ContentTypes = normalizeList(profile.ContentTypes)
	profile.Transports = normalizeList(profile.Transports)
	profile.Families = normalizeList(profile.Families)
	profile.RemotePorts = normalizeUint32List(profile.RemotePorts)
	profile.RemoteCIDRs = normalizeList(profile.RemoteCIDRs)
	profile.Processes = normalizeList(profile.Processes)
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
	observation.Transport = strings.ToLower(strings.TrimSpace(observation.Transport))
	observation.Family = strings.ToLower(strings.TrimSpace(observation.Family))
	observation.RemoteIP = strings.TrimSpace(observation.RemoteIP)
	observation.Process = strings.ToLower(strings.TrimSpace(observation.Process))
	return observation
}

func matchProfile(profile Profile, observation Observation) (Match, bool) {
	score := 0
	matchedBy := make([]string, 0, 8)

	if len(profile.Sources) != 0 {
		if !containsFold(profile.Sources, observation.Source) {
			return Match{}, false
		}
		score += 3
		matchedBy = append(matchedBy, "source")
	}
	if len(profile.Protocols) != 0 {
		if !containsFold(profile.Protocols, observation.Protocol) {
			return Match{}, false
		}
		score += 5
		matchedBy = append(matchedBy, "protocol")
	}
	if len(profile.Directions) != 0 {
		if !containsFold(profile.Directions, observation.Direction) {
			return Match{}, false
		}
		score += 3
		matchedBy = append(matchedBy, "direction")
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

type HTTP1StartLine struct {
	Kind   string
	Method string
	Path   string
	Status uint32
}

func ParseHTTP1StartLine(line string) (HTTP1StartLine, bool) {
	line = strings.TrimSpace(strings.TrimRight(line, "\x00"))
	if line == "" {
		return HTTP1StartLine{}, false
	}
	if index := strings.IndexAny(line, "\r\n"); index >= 0 {
		line = line[:index]
	}
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return HTTP1StartLine{}, false
	}
	if strings.HasPrefix(strings.ToUpper(parts[0]), "HTTP/1.") {
		status, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil || status < 100 || status > 999 {
			return HTTP1StartLine{}, false
		}
		return HTTP1StartLine{Kind: "response", Status: uint32(status)}, true
	}
	method := strings.ToUpper(parts[0])
	if !isHTTPMethod(method) {
		return HTTP1StartLine{}, false
	}
	path := RequestPath(parts[1])
	if path == "" {
		return HTTP1StartLine{}, false
	}
	return HTTP1StartLine{Kind: "request", Method: method, Path: path}, true
}

func ParseHTTP1RequestLine(line string) (method, path string, ok bool) {
	parsed, ok := ParseHTTP1StartLine(line)
	if !ok || parsed.Kind != "request" {
		return "", "", false
	}
	return parsed.Method, parsed.Path, true
}

// ParseGRPCPath normalizes the canonical HTTP/2 gRPC route
// /package.Service/Method without inspecting protobuf message bodies.
func ParseGRPCPath(raw string) (service, method string, ok bool) {
	path := strings.TrimPrefix(RequestPath(raw), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		return "", "", false
	}
	service = strings.TrimSpace(parts[0])
	method = strings.TrimSpace(parts[1])
	if service == "" || method == "" {
		return "", "", false
	}
	return service, method, true
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
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func normalizeUpperList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.ToUpper(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func normalizePathList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = RequestPath(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func normalizeUint32List(items []uint32) []uint32 {
	if len(items) == 0 {
		return nil
	}
	out := append([]uint32(nil), items...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	writeAt := 0
	for _, value := range out {
		if writeAt != 0 && out[writeAt-1] == value {
			continue
		}
		out[writeAt] = value
		writeAt++
	}
	return out[:writeAt]
}

func containsFold(items []string, value string) bool {
	for _, item := range items {
		if strings.EqualFold(item, value) {
			return true
		}
	}
	return false
}

// MergeProfiles overlays custom profiles by ID while retaining all built-ins
// that were not explicitly replaced. The result is validated atomically by the
// Registry before publication.
func MergeProfiles(base, overlay []Profile) []Profile {
	merged := make(map[string]Profile, len(base)+len(overlay))
	for _, profile := range base {
		profile = normalizeProfile(profile)
		if profile.ID != "" {
			merged[profile.ID] = profile
		}
	}
	for _, profile := range overlay {
		profile = normalizeProfile(profile)
		if profile.ID != "" {
			merged[profile.ID] = profile
		}
	}
	out := make([]Profile, 0, len(merged))
	for _, profile := range merged {
		out = append(out, profile)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func ReloadDefaultJSON(path string) error {
	overlay, err := LoadJSON(path)
	if err != nil {
		return err
	}
	return Default.Replace(MergeProfiles(BuiltinProfiles(), overlay))
}

// WatchDefaultJSON hashes the file contents instead of relying only on mtime,
// which makes atomic replace/ConfigMap style updates reliable. Invalid updates
// never replace the last known-good immutable registry snapshot.
func WatchDefaultJSON(ctx context.Context, path string, interval time.Duration, onError func(error)) {
	if ctx == nil || strings.TrimSpace(path) == "" {
		return
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	var lastAttempt [32]byte
	var attempted bool
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(path)
			if err != nil {
				if onError != nil {
					onError(fmt.Errorf("read API capture profiles: %w", err))
				}
				continue
			}
			digest := sha256.Sum256(data)
			if attempted && digest == lastAttempt {
				continue
			}
			lastAttempt = digest
			attempted = true
			overlay, err := ParseJSON(data)
			if err == nil {
				err = Default.Replace(MergeProfiles(BuiltinProfiles(), overlay))
			}
			if err != nil && onError != nil {
				onError(err)
			}
		}
	}
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
