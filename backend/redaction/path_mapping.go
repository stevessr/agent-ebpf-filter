package redaction

import (
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// PathMapper provides bidirectional path mapping for outgoing and incoming data.
// Outgoing: real paths → sanitized paths (before sending)
// Incoming: sanitized paths → real paths (when receiving)
type PathMapper struct {
	mu              sync.RWMutex
	rules           []PathMappingRule
	reverseMap      map[string]string // sanitized → real (for exact matches)
	enabled         bool
	caseSensitive   bool
}

// PathMappingRule defines a path transformation rule.
type PathMappingRule struct {
	// Pattern to match (supports wildcards: * and **)
	Pattern string

	// Replacement for outgoing mapping
	Replacement string

	// Priority (higher = checked first)
	Priority int

	// Type of rule
	Type PathRuleType

	// Compiled regex (internal)
	regex *regexp.Regexp
}

// PathRuleType indicates how to apply the rule.
type PathRuleType string

const (
	PathRuleExact      PathRuleType = "exact"       // Exact match
	PathRulePrefix     PathRuleType = "prefix"      // Prefix match
	PathRuleSuffix     PathRuleType = "suffix"      // Suffix match
	PathRuleWildcard   PathRuleType = "wildcard"    // Wildcard (* and **)
	PathRuleRegex      PathRuleType = "regex"       // Regular expression
)

// PathMapperConfig configures the path mapper.
type PathMapperConfig struct {
	Enabled       bool
	CaseSensitive bool
	DefaultRules  []PathMappingRule
}

// NewPathMapper creates a new bidirectional path mapper.
func NewPathMapper(config PathMapperConfig) *PathMapper {
	pm := &PathMapper{
		rules:         make([]PathMappingRule, 0),
		reverseMap:    make(map[string]string),
		enabled:       config.Enabled,
		caseSensitive: config.CaseSensitive,
	}

	// Add default rules
	if len(config.DefaultRules) > 0 {
		for _, rule := range config.DefaultRules {
			pm.AddRule(rule)
		}
	}

	return pm
}

// AddRule adds a new mapping rule.
func (pm *PathMapper) AddRule(rule PathMappingRule) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Compile regex if needed
	if rule.Type == PathRuleRegex {
		regex, err := regexp.Compile(rule.Pattern)
		if err != nil {
			return err
		}
		rule.regex = regex
	} else if rule.Type == PathRuleWildcard {
		// Convert wildcard to regex
		regex, err := wildcardToRegex(rule.Pattern)
		if err != nil {
			return err
		}
		rule.regex = regex
	}

	// Normalize case if not case-sensitive
	if !pm.caseSensitive {
		rule.Pattern = strings.ToLower(rule.Pattern)
		rule.Replacement = strings.ToLower(rule.Replacement)
	}

	// Add to rules (maintain priority order)
	inserted := false
	for i, existing := range pm.rules {
		if rule.Priority > existing.Priority {
			pm.rules = append(pm.rules[:i], append([]PathMappingRule{rule}, pm.rules[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		pm.rules = append(pm.rules, rule)
	}

	// Add to reverse map for exact matches
	if rule.Type == PathRuleExact {
		pm.reverseMap[rule.Replacement] = rule.Pattern
	}

	return nil
}

// RemoveRule removes a mapping rule by pattern.
func (pm *PathMapper) RemoveRule(pattern string) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if !pm.caseSensitive {
		pattern = strings.ToLower(pattern)
	}

	for i, rule := range pm.rules {
		if rule.Pattern == pattern {
			// Remove from reverse map if exact
			if rule.Type == PathRuleExact {
				delete(pm.reverseMap, rule.Replacement)
			}

			// Remove from rules
			pm.rules = append(pm.rules[:i], pm.rules[i+1:]...)
			return true
		}
	}
	return false
}

// MapOutgoing maps a real path to a sanitized path (for outgoing data).
func (pm *PathMapper) MapOutgoing(realPath string) string {
	if !pm.enabled || realPath == "" {
		return realPath
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	checkPath := realPath
	if !pm.caseSensitive {
		checkPath = strings.ToLower(realPath)
	}

	// Try rules in priority order
	for _, rule := range pm.rules {
		switch rule.Type {
		case PathRuleExact:
			if checkPath == rule.Pattern {
				return rule.Replacement
			}

		case PathRulePrefix:
			if strings.HasPrefix(checkPath, rule.Pattern) {
				// Replace prefix
				return rule.Replacement + realPath[len(rule.Pattern):]
			}

		case PathRuleSuffix:
			if strings.HasSuffix(checkPath, rule.Pattern) {
				// Replace suffix
				return realPath[:len(realPath)-len(rule.Pattern)] + rule.Replacement
			}

		case PathRuleWildcard, PathRuleRegex:
			if rule.regex != nil && rule.regex.MatchString(checkPath) {
				// Use regex replacement
				return rule.regex.ReplaceAllString(realPath, rule.Replacement)
			}
		}
	}

	// No match, return original
	return realPath
}

// MapIncoming maps a sanitized path back to the real path (for incoming data).
func (pm *PathMapper) MapIncoming(sanitizedPath string) string {
	if !pm.enabled || sanitizedPath == "" {
		return sanitizedPath
	}

	pm.mu.RLock()
	defer pm.mu.RUnlock()

	checkPath := sanitizedPath
	if !pm.caseSensitive {
		checkPath = strings.ToLower(sanitizedPath)
	}

	// Try exact reverse map first (fastest)
	if realPath, exists := pm.reverseMap[checkPath]; exists {
		return realPath
	}

	// Try rules in reverse (sanitized → real)
	for _, rule := range pm.rules {
		switch rule.Type {
		case PathRulePrefix:
			// If sanitized has the replacement prefix, restore real prefix
			if strings.HasPrefix(checkPath, rule.Replacement) {
				return rule.Pattern + sanitizedPath[len(rule.Replacement):]
			}

		case PathRuleSuffix:
			// If sanitized has the replacement suffix, restore real suffix
			if strings.HasSuffix(checkPath, rule.Replacement) {
				return sanitizedPath[:len(sanitizedPath)-len(rule.Replacement)] + rule.Pattern
			}

		case PathRuleWildcard, PathRuleRegex:
			// For regex/wildcard, try to match replacement pattern and reverse
			// (This is approximate - may not always work perfectly)
			if strings.Contains(sanitizedPath, rule.Replacement) {
				// Simple string replacement (best effort)
				return strings.ReplaceAll(sanitizedPath, rule.Replacement, rule.Pattern)
			}
		}
	}

	// No match, return original
	return sanitizedPath
}

// MapOutgoingBatch maps multiple paths efficiently.
func (pm *PathMapper) MapOutgoingBatch(paths []string) []string {
	if !pm.enabled || len(paths) == 0 {
		return paths
	}

	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = pm.MapOutgoing(path)
	}
	return result
}

// MapIncomingBatch maps multiple sanitized paths back.
func (pm *PathMapper) MapIncomingBatch(paths []string) []string {
	if !pm.enabled || len(paths) == 0 {
		return paths
	}

	result := make([]string, len(paths))
	for i, path := range paths {
		result[i] = pm.MapIncoming(path)
	}
	return result
}

// GetRules returns all mapping rules.
func (pm *PathMapper) GetRules() []PathMappingRule {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rules := make([]PathMappingRule, len(pm.rules))
	copy(rules, pm.rules)
	return rules
}

// ClearRules removes all mapping rules.
func (pm *PathMapper) ClearRules() {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	pm.rules = make([]PathMappingRule, 0)
	pm.reverseMap = make(map[string]string)
}

// SetEnabled enables or disables path mapping.
func (pm *PathMapper) SetEnabled(enabled bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.enabled = enabled
}

// IsEnabled returns whether path mapping is enabled.
func (pm *PathMapper) IsEnabled() bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.enabled
}

// ExportRules exports all rules for persistence.
func (pm *PathMapper) ExportRules() []PathMappingRule {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	exported := make([]PathMappingRule, len(pm.rules))
	for i, rule := range pm.rules {
		// Don't export compiled regex (will be recompiled on import)
		exported[i] = PathMappingRule{
			Pattern:     rule.Pattern,
			Replacement: rule.Replacement,
			Priority:    rule.Priority,
			Type:        rule.Type,
		}
	}
	return exported
}

// ImportRules imports previously exported rules.
func (pm *PathMapper) ImportRules(rules []PathMappingRule) error {
	pm.ClearRules()
	for _, rule := range rules {
		if err := pm.AddRule(rule); err != nil {
			return err
		}
	}
	return nil
}

// wildcardToRegex converts a wildcard pattern to regex.
// * matches any characters except /
// ** matches any characters including /
func wildcardToRegex(pattern string) (*regexp.Regexp, error) {
	// Escape regex special characters except * and **
	escaped := regexp.QuoteMeta(pattern)

	// Replace escaped wildcards with regex equivalents
	escaped = strings.ReplaceAll(escaped, "\\*\\*", ".*")     // ** → .*
	escaped = strings.ReplaceAll(escaped, "\\*", "[^/]*")     // * → [^/]*

	// Anchor to start and end
	regexPattern := "^" + escaped + "$"

	return regexp.Compile(regexPattern)
}

// DefaultPathMappingRules returns common path mapping rules.
func DefaultPathMappingRules() []PathMappingRule {
	return []PathMappingRule{
		{
			Pattern:     "/home/*/",
			Replacement: "/home/user/",
			Priority:    100,
			Type:        PathRuleWildcard,
		},
		{
			Pattern:     "/Users/*/",
			Replacement: "/Users/user/",
			Priority:    100,
			Type:        PathRuleWildcard,
		},
		{
			Pattern:     filepath.Join(homeDir(), ""),
			Replacement: "~/",
			Priority:    90,
			Type:        PathRulePrefix,
		},
	}
}

// homeDir returns the user's home directory (helper).
func homeDir() string {
	if home := filepath.Clean("/home/user"); home != "" {
		return home
	}
	return ""
}

// PathMappingStats tracks mapping statistics.
type PathMappingStats struct {
	TotalOutgoing    int64
	TotalIncoming    int64
	OutgoingMapped   int64
	IncomingMapped   int64
	OutgoingUnmapped int64
	IncomingUnmapped int64
}

// StatsTracker tracks path mapping statistics.
type PathMappingStatsTracker struct {
	mu    sync.RWMutex
	stats PathMappingStats
}

// NewPathMappingStatsTracker creates a new stats tracker.
func NewPathMappingStatsTracker() *PathMappingStatsTracker {
	return &PathMappingStatsTracker{}
}

// RecordOutgoing records an outgoing mapping operation.
func (pst *PathMappingStatsTracker) RecordOutgoing(mapped bool) {
	pst.mu.Lock()
	defer pst.mu.Unlock()

	pst.stats.TotalOutgoing++
	if mapped {
		pst.stats.OutgoingMapped++
	} else {
		pst.stats.OutgoingUnmapped++
	}
}

// RecordIncoming records an incoming mapping operation.
func (pst *PathMappingStatsTracker) RecordIncoming(mapped bool) {
	pst.mu.Lock()
	defer pst.mu.Unlock()

	pst.stats.TotalIncoming++
	if mapped {
		pst.stats.IncomingMapped++
	} else {
		pst.stats.IncomingUnmapped++
	}
}

// GetStats returns a copy of current statistics.
func (pst *PathMappingStatsTracker) GetStats() PathMappingStats {
	pst.mu.RLock()
	defer pst.mu.RUnlock()
	return pst.stats
}

// Reset resets all statistics.
func (pst *PathMappingStatsTracker) Reset() {
	pst.mu.Lock()
	defer pst.mu.Unlock()
	pst.stats = PathMappingStats{}
}
