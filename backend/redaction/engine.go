package redaction

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// RedactionStats tracks how much data has been redacted.
type RedactionStats struct {
	EventsProcessed uint64 `json:"eventsProcessed"`
	ValuesRedacted  uint64 `json:"valuesRedacted"`
	RulesApplied    uint64 `json:"rulesApplied"`
}

type compiledRule struct {
	rule     RedactionRule
	patterns []*regexp.Regexp
}

// RedactionEngine applies policy-driven masking to event payloads.
type RedactionEngine struct {
	policy RedactionPolicy

	mu           sync.RWMutex
	rules        []RedactionRule
	compiled     []compiledRule
	ruleCache    map[string]string
	batchCache    map[string]interface{}
	stats        RedactionStats
	placeholder  string
	excludeSet   map[FieldCategory]struct{}
}

// NewRedactionEngine constructs an engine and precomputes policy state.
func NewRedactionEngine(policy RedactionPolicy) *RedactionEngine {
	engine := &RedactionEngine{
		policy:      policy,
		ruleCache:   make(map[string]string),
		batchCache:  make(map[string]interface{}),
		excludeSet:  make(map[FieldCategory]struct{}, len(policy.ExcludeCategories)),
		placeholder: policy.DefaultPlaceholder,
	}
	if engine.placeholder == "" {
		engine.placeholder = "[REDACTED]"
	}
	for _, cat := range policy.ExcludeCategories {
		engine.excludeSet[cat] = struct{}{}
	}
	engine.rules = engine.loadEffectiveRulesLocked()
	engine.compiled = compileRules(engine.rules)
	return engine
}

// GetEffectiveRules returns the active rule set in deterministic order.
func (e *RedactionEngine) GetEffectiveRules() []RedactionRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]RedactionRule, len(e.rules))
	copy(out, e.rules)
	return out
}

// ApplyRules redacts a single value for the provided category.
func (e *RedactionEngine) ApplyRules(value string, category FieldCategory) string {
	if value == "" {
		return value
	}
	if e.isExcluded(category) {
		return value
	}

	cacheKey := string(category) + "\x00" + value
	e.mu.RLock()
	if cached, ok := e.ruleCache[cacheKey]; ok {
		e.mu.RUnlock()
		return cached
	}
	rules := e.compiled
	placeholder := e.placeholder
	preserveLengths := e.policy.PreserveLengths
	e.mu.RUnlock()

	redacted := value
	applied := 0
	for _, cr := range rules {
		if !ruleAppliesToCategory(cr.rule, category) {
			continue
		}
		if !ruleMatchesValue(cr, redacted) {
			continue
		}
		replacement := cr.rule.ReplaceWith
		if replacement == "" {
			replacement = placeholder
		}
		redacted = applyReplacement(redacted, replacement, preserveLengths)
		applied++
	}

	if applied > 0 && redacted != value {
		e.mu.Lock()
		e.ruleCache[cacheKey] = redacted
		e.mu.Unlock()
		atomic.AddUint64(&e.stats.ValuesRedacted, 1)
		atomic.AddUint64(&e.stats.RulesApplied, uint64(applied))
	}
	return redacted
}

// RedactEvent redacts all string fields in a payload while preserving shape.
func (e *RedactionEngine) RedactEvent(evt interface{}) (interface{}, error) {
	if evt == nil {
		return nil, nil
	}

	b, err := json.Marshal(evt)
	if err != nil {
		return nil, err
	}

	cacheKey := string(b)
	e.mu.RLock()
	if cached, ok := e.batchCache[cacheKey]; ok {
		e.mu.RUnlock()
		atomic.AddUint64(&e.stats.EventsProcessed, 1)
		return cached, nil
	}
	rules := e.compiled
	placeholder := e.placeholder
	preserveLengths := e.policy.PreserveLengths
	e.mu.RUnlock()

	var data interface{}
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, err
	}

	redacted := redactAny(data, rules, placeholder, preserveLengths, e.isExcluded)

	out, err := json.Marshal(redacted)
	if err != nil {
		return nil, err
	}

	var result interface{}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, err
	}

	e.mu.Lock()
	e.batchCache[cacheKey] = result
	e.mu.Unlock()
	atomic.AddUint64(&e.stats.EventsProcessed, 1)
	return result, nil
}

// PolicyLevel returns the current redaction level.
func (e *RedactionEngine) PolicyLevel() RedactionLevel {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.policy.Level
}

func (e *RedactionEngine) isExcluded(category FieldCategory) bool {
	e.mu.RLock()
	_, ok := e.excludeSet[category]
	e.mu.RUnlock()
	return ok
}

func (e *RedactionEngine) loadEffectiveRulesLocked() []RedactionRule {
	level := e.policy.Level
	defaults := defaultRulesForLevel(level)
	custom := make([]RedactionRule, 0, len(defaults)+len(e.policy.Rules))
	custom = append(custom, defaults...)
	for _, rule := range e.policy.Rules {
		if !rule.Enabled {
			continue
		}
		custom = append(custom, rule)
	}
	sort.SliceStable(custom, func(i, j int) bool { return custom[i].ID < custom[j].ID })
	return custom
}

func compileRules(rules []RedactionRule) []compiledRule {
	compiled := make([]compiledRule, 0, len(rules))
	for _, rule := range rules {
		cr := compiledRule{rule: rule}
		for _, field := range rule.Fields {
			if field.Pattern == "" {
				continue
			}
			if re, err := regexp.Compile(field.Pattern); err == nil {
				cr.patterns = append(cr.patterns, re)
			}
		}
		compiled = append(compiled, cr)
	}
	return compiled
}

func defaultRulesForLevel(level RedactionLevel) []RedactionRule {
	rules := []RedactionRule{{ID: "mask-credential", Level: RedactionLevelBasic, Enabled: true, Categories: []FieldCategory{FieldCategoryCredential}, ReplaceWith: "[REDACTED]"}}
	switch level {
	case RedactionLevelNone:
		return nil
	case RedactionLevelBasic:
		return rules
	case RedactionLevelStandard:
		return append(rules,
			RedactionRule{ID: "mask-identifier", Level: RedactionLevelStandard, Enabled: true, Categories: []FieldCategory{FieldCategoryIdentifier}, ReplaceWith: "[REDACTED]"},
		)
	case RedactionLevelStrict:
		return append(rules,
			RedactionRule{ID: "mask-identifier", Level: RedactionLevelStandard, Enabled: true, Categories: []FieldCategory{FieldCategoryIdentifier}, ReplaceWith: "[REDACTED]"},
			RedactionRule{ID: "mask-path", Level: RedactionLevelStrict, Enabled: true, Categories: []FieldCategory{FieldCategoryPath, FieldCategoryCommand, FieldCategoryNetwork}, ReplaceWith: "[REDACTED]"},
		)
	default:
		return rules
	}
}

func ruleAppliesToCategory(rule RedactionRule, category FieldCategory) bool {
	if len(rule.Categories) == 0 {
		return true
	}
	for _, cat := range rule.Categories {
		if cat == category {
			return true
		}
	}
	return false
}

func ruleMatchesValue(rule compiledRule, value string) bool {
	if len(rule.patterns) == 0 {
		return true
	}
	for _, re := range rule.patterns {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

func applyReplacement(value, replacement string, preserveLengths bool) string {
	if !preserveLengths {
		return replacement
	}
	if len(value) <= len(replacement) {
		return replacement
	}
	return strings.Repeat("*", len(value))
}

func redactAny(v interface{}, rules []compiledRule, placeholder string, preserveLengths bool, isExcluded func(FieldCategory) bool) interface{} {
	switch x := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(x))
		for k, val := range x {
			out[k] = redactAnyWithKey(k, val, rules, placeholder, preserveLengths, isExcluded)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(x))
		for i, val := range x {
			out[i] = redactAny(val, rules, placeholder, preserveLengths, isExcluded)
		}
		return out
	default:
		return v
	}
}

func redactAnyWithKey(key string, val interface{}, rules []compiledRule, placeholder string, preserveLengths bool, isExcluded func(FieldCategory) bool) interface{} {
	if s, ok := val.(string); ok {
		category := inferCategory(key)
		if isExcluded(category) {
			return s
		}
		redacted := s
		for _, rule := range rules {
			if !ruleAppliesToCategory(rule.rule, category) {
				continue
			}
			if !ruleMatchesValue(rule, redacted) {
				continue
			}
			replacement := rule.rule.ReplaceWith
			if replacement == "" {
				replacement = placeholder
			}
			redacted = applyReplacement(redacted, replacement, preserveLengths)
		}
		return redacted
	}
	return redactAny(val, rules, placeholder, preserveLengths, isExcluded)
}

func inferCategory(key string) FieldCategory {
	lower := strings.ToLower(key)
	switch {
	case strings.Contains(lower, "path"), strings.Contains(lower, "file"), strings.Contains(lower, "dir"):
		return FieldCategoryPath
	case strings.Contains(lower, "cmd"), strings.Contains(lower, "comm"), strings.Contains(lower, "argv"), strings.Contains(lower, "command"):
		return FieldCategoryCommand
	case strings.Contains(lower, "ip"), strings.Contains(lower, "port"), strings.Contains(lower, "host"), strings.Contains(lower, "addr"), strings.Contains(lower, "net"):
		return FieldCategoryNetwork
	case strings.Contains(lower, "token"), strings.Contains(lower, "secret"), strings.Contains(lower, "password"), strings.Contains(lower, "passwd"), strings.Contains(lower, "cred"):
		return FieldCategoryCredential
	default:
		return FieldCategoryIdentifier
	}
}

func (e *RedactionEngine) String() string {
	return fmt.Sprintf("RedactionEngine(level=%s,rules=%d)", e.policy.Level, len(e.rules))
}
