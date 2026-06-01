package app

import (
	"slices"
	"strings"
	"sync"
)

// ---- moved from backend/zz_merged_backend.go section capturerulestls.go ----

type TLSCaptureRule struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Enabled     bool     `json:"enabled"`
	Scope       string   `json:"scope"`
	Comms       []string `json:"comms,omitempty"`
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
	return []TLSCaptureRule{
		{
			ID:          "agent-cli-tag",
			Name:        "Agent CLI tag",
			Enabled:     true,
			Scope:       "agent_cli_tag",
			Description: "Default Hook SSL rule: keep TLS plaintext only for processes registered by agent CLI hooks or wrappers.",
		},
	}
}

func (s *TLSCaptureRuleStore) List() []TLSCaptureRule {
	if s == nil {
		return defaultTLSCaptureRules()
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
	for _, rule := range rules {
		if tlsCaptureRuleMatches(rule, event) {
			return true
		}
	}
	return false
}

func normalizeTLSCaptureRules(rules []TLSCaptureRule) []TLSCaptureRule {
	if len(rules) == 0 {
		return defaultTLSCaptureRules()
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
