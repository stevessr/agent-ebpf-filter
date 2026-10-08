package tls

import (
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

type TLSCaptureRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Scope       string   `json:"scope"`
	Comms       []string `json:"comms,omitempty"`
	Paths       []string `json:"paths,omitempty"`
	Hosts       []string `json:"hosts,omitempty"`
	Methods     []string `json:"methods,omitempty"`
	Libraries   []string `json:"libraries,omitempty"`
	Directions  []string `json:"directions,omitempty"`
	Description string   `json:"description,omitempty"`
}

type TLSCaptureRuleStore struct {
	mu    sync.RWMutex
	rules []TLSCaptureRule
}

func NewTLSCaptureRuleStore() *TLSCaptureRuleStore {
	return &TLSCaptureRuleStore{rules: defaultTLSCaptureRules()}
}

func defaultTLSCaptureRules() []TLSCaptureRule {
	return nil
}

func (s *TLSCaptureRuleStore) List() []TLSCaptureRule {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TLSCaptureRule, len(s.rules))
	copy(out, s.rules)
	return out
}

func (s *TLSCaptureRuleStore) Replace(rules []TLSCaptureRule) []TLSCaptureRule {
	if s == nil {
		return normalizeTLSCaptureRules(rules)
	}
	normalized := normalizeTLSCaptureRules(rules)
	s.mu.Lock()
	s.rules = normalized
	s.mu.Unlock()
	return normalized
}

func (s *TLSCaptureRuleStore) Allows(event TLSPlaintextEvent) bool {
	rules := s.List()
	if len(rules) == 0 {
		return true // No rules configured — allow all events
	}
	for _, rule := range rules {
		if tlsCaptureRuleMatches(rule, event) {
			return true
		}
	}
	return false
}

func normalizeTLSCaptureRules(rules []TLSCaptureRule) []TLSCaptureRule {
	if len(rules) == 0 {
		return nil
	}
	out := make([]TLSCaptureRule, 0, len(rules))
	seen := make(map[string]struct{}, len(rules))
	for _, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		rule.Name = strings.TrimSpace(rule.Name)
		rule.Scope = strings.TrimSpace(strings.ToLower(rule.Scope))
		rule.Description = strings.TrimSpace(rule.Description)
		if rule.ID == "" {
			rule.ID = tlsCaptureRuleID(rule)
		}
		if rule.Name == "" {
			rule.Name = rule.ID
		}
		if rule.Scope == "" {
			rule.Scope = "custom"
		}
		rule.Comms = normalizeTLSRuleValues(rule.Comms, false)
		rule.Paths = normalizeTLSExecutablePaths(rule.Paths)
		rule.Hosts = normalizeTLSRuleValues(rule.Hosts, true)
		rule.Methods = normalizeTLSRuleValues(rule.Methods, true)
		rule.Libraries = normalizeTLSRuleValues(rule.Libraries, true)
		rule.Directions = normalizeTLSRuleValues(rule.Directions, true)
		if _, ok := seen[rule.ID]; ok {
			continue
		}
		seen[rule.ID] = struct{}{}
		out = append(out, rule)
	}
	if len(out) == 0 {
		return defaultTLSCaptureRules()
	}
	return out
}

func tlsCaptureRuleID(rule TLSCaptureRule) string {
	base := strings.ToLower(rule.Name)
	base = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return '-'
	}, base)
	base = strings.Trim(base, "-")
	if base == "" {
		return "custom-rule"
	}
	return base
}

func normalizeTLSRuleValues(values []string, lower bool) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if lower {
			trimmed = strings.ToLower(trimmed)
		}
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// ExecutablePaths returns the enabled executable-path allowlist used before
// any TLS uprobe is attached. Empty means auto-discovery is fail-closed.
func (s *TLSCaptureRuleStore) ExecutablePaths() []string {
	if s == nil {
		return nil
	}
	rules := s.List()
	out := make([]string, 0)
	seen := make(map[string]struct{})
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		for _, path := range rule.Paths {
			if _, ok := seen[path]; ok {
				continue
			}
			seen[path] = struct{}{}
			out = append(out, path)
		}
	}
	return out
}

// AllowsExecutablePath is intentionally stricter than the event-level rule
// matcher: TLS probe attachment is denied unless an enabled rule explicitly
// names the executable path (or a directory /** prefix). This keeps unrelated
// processes out of the eBPF TLS plaintext path entirely.
func (s *TLSCaptureRuleStore) AllowsExecutablePath(path string) bool {
	candidate := normalizeTLSExecutablePath(path)
	if candidate == "" {
		return false
	}
	for _, allowed := range s.ExecutablePaths() {
		if tlsExecutablePathMatches(candidate, allowed) {
			return true
		}
	}
	return false
}

func normalizeTLSExecutablePaths(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		normalized := normalizeTLSExecutablePath(value)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func normalizeTLSExecutablePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.HasSuffix(path, "/**") {
		base := strings.TrimSuffix(path, "/**")
		base = filepath.Clean(base)
		if base == "." || base == string(filepath.Separator) {
			return ""
		}
		return strings.TrimSuffix(base, string(filepath.Separator)) + "/**"
	}
	return filepath.Clean(path)
}

func tlsExecutablePathMatches(candidate, allowed string) bool {
	allowed = normalizeTLSExecutablePath(allowed)
	if allowed == "" {
		return false
	}
	if strings.HasSuffix(allowed, "/**") {
		prefix := strings.TrimSuffix(allowed, "/**")
		return candidate == prefix || strings.HasPrefix(candidate, prefix+string(filepath.Separator))
	}
	if candidate == allowed {
		return true
	}
	// Exact rules may be symlinks (for example /usr/local/bin/claude), while
	// /proc/<pid>/exe reports the resolved target. Resolve only existing exact
	// paths; wildcard directory rules remain lexical and predictable.
	if resolved, err := filepath.EvalSymlinks(allowed); err == nil {
		return normalizeTLSExecutablePath(resolved) == candidate
	}
	return false
}

func tlsCaptureRuleMatches(rule TLSCaptureRule, event TLSPlaintextEvent) bool {
	if !rule.Enabled {
		return false
	}
	if rule.Scope == "agent_cli_tag" && !tlsCaptureEventHasAgentContext(event) {
		return false
	}
	if len(rule.Comms) > 0 && !tlsValueMatchesAny(event.Comm, rule.Comms, false) {
		return false
	}
	if len(rule.Hosts) > 0 && !tlsValueMatchesAny(event.Host, rule.Hosts, true) {
		return false
	}
	if len(rule.Methods) > 0 && !slices.Contains(rule.Methods, strings.ToLower(event.Method)) {
		return false
	}
	if len(rule.Libraries) > 0 && !slices.Contains(rule.Libraries, strings.ToLower(event.Lib)) {
		return false
	}
	if len(rule.Directions) > 0 && !slices.Contains(rule.Directions, strings.ToLower(event.Direction)) {
		return false
	}
	return true
}

func tlsCaptureEventHasAgentContext(event TLSPlaintextEvent) bool {
	if event.RootAgentPID != 0 || event.AgentRunID != "" || event.TaskID != "" || event.ToolCallID != "" || event.TraceID != "" || event.ToolName != "" {
		return true
	}
	_, ok := lookupTLSProcessContext(event.PID, event.TGID)
	return ok
}

func tlsValueMatchesAny(value string, patterns []string, lower bool) bool {
	candidate := strings.TrimSpace(value)
	if lower {
		candidate = strings.ToLower(candidate)
	}
	for _, pattern := range patterns {
		if pattern == "*" || candidate == pattern || strings.Contains(candidate, pattern) {
			return true
		}
	}
	return false
}
