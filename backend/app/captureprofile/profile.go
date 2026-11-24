package captureprofile

import (
	"fmt"
	"sort"
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

func BuiltinProfiles() []Profile {
	return []Profile{
		{ID: "anthropic.messages", Vendor: "anthropic", Product: "messages", Operation: "messages.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.anthropic.com"}, PathPrefixes: []string{"/v1/messages"}, MinScore: 90},
		{ID: "aws-bedrock.converse", Vendor: "aws-bedrock", Product: "bedrock-runtime", Operation: "converse", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostContains: []string{"bedrock-runtime"}, PathContains: []string{"/converse"}, MinScore: 90},
		{ID: "azure-openai.deployments", Vendor: "azure-openai", Product: "azure-openai", Operation: "deployment.request", Protocols: []string{"http1", "http2"}, HostSuffixes: []string{"openai.azure.com"}, PathContains: []string{"/openai/deployments/"}, MinScore: 90},
		{ID: "cohere.chat", Vendor: "cohere", Product: "chat", Operation: "chat", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.cohere.com"}, PathPrefixes: []string{"/v2/chat", "/v1/chat"}, MinScore: 90},
		{ID: "google-gemini.generate", Vendor: "google", Product: "gemini", Operation: "generateContent", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"generativelanguage.googleapis.com"}, PathContains: []string{":generateContent", ":streamGenerateContent"}, MinScore: 90},
		// MiniMax Code's builtin minimax_api provider is anthropic-messages and
		// uses https://api.minimax.io/anthropic (global) or
		// https://api.minimaxi.com/anthropic (CN). Attribute the provider only
		// when the source-backed host + wire path are both present.
		{ID: "minimax.messages", Vendor: "minimax", Product: "messages", Operation: "messages.create", Protocols: []string{"http1", "http2"}, Methods: []string{"POST"}, HostSuffixes: []string{"api.minimax.io", "api.minimaxi.com"}, PathPrefixes: []string{"/anthropic/v1/messages"}, MinScore: 90},
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
