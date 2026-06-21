package redaction

import (
	"testing"
)

func TestPathMapper_ExactMatch(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true, CaseSensitive: true})

	rule := PathMappingRule{
		Pattern:     "/home/alice/secret.txt",
		Replacement: "/home/user/file.txt",
		Priority:    100,
		Type:        PathRuleExact,
	}
	pm.AddRule(rule)

	// Outgoing: real → sanitized
	sanitized := pm.MapOutgoing("/home/alice/secret.txt")
	if sanitized != "/home/user/file.txt" {
		t.Errorf("Expected /home/user/file.txt, got %s", sanitized)
	}

	// Incoming: sanitized → real
	real := pm.MapIncoming("/home/user/file.txt")
	if real != "/home/alice/secret.txt" {
		t.Errorf("Expected /home/alice/secret.txt, got %s", real)
	}
}

func TestPathMapper_PrefixMatch(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	rule := PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	}
	pm.AddRule(rule)

	// Outgoing
	tests := []struct {
		input    string
		expected string
	}{
		{"/home/alice/documents/file.txt", "/home/user/documents/file.txt"},
		{"/home/alice/projects/app/main.go", "/home/user/projects/app/main.go"},
		{"/home/bob/file.txt", "/home/bob/file.txt"}, // No match
	}

	for _, tc := range tests {
		result := pm.MapOutgoing(tc.input)
		if result != tc.expected {
			t.Errorf("Outgoing %s: expected %s, got %s", tc.input, tc.expected, result)
		}
	}

	// Incoming (reverse)
	real := pm.MapIncoming("/home/user/documents/file.txt")
	if real != "/home/alice/documents/file.txt" {
		t.Errorf("Expected /home/alice/documents/file.txt, got %s", real)
	}
}

func TestPathMapper_SuffixMatch(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	rule := PathMappingRule{
		Pattern:     ".secret",
		Replacement: ".txt",
		Priority:    100,
		Type:        PathRuleSuffix,
	}
	pm.AddRule(rule)

	// Outgoing
	sanitized := pm.MapOutgoing("/home/user/passwords.secret")
	if sanitized != "/home/user/passwords.txt" {
		t.Errorf("Expected /home/user/passwords.txt, got %s", sanitized)
	}

	// Incoming
	real := pm.MapIncoming("/home/user/passwords.txt")
	if real != "/home/user/passwords.secret" {
		t.Errorf("Expected /home/user/passwords.secret, got %s", real)
	}
}

func TestPathMapper_WildcardMatch(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	rule := PathMappingRule{
		Pattern:     "/home/*/documents/*.pdf",
		Replacement: "/home/user/docs/file.pdf",
		Priority:    100,
		Type:        PathRuleWildcard,
	}
	pm.AddRule(rule)

	// Outgoing
	tests := []struct {
		input  string
		expect string
	}{
		{"/home/alice/documents/report.pdf", "/home/user/docs/file.pdf"},
		{"/home/bob/documents/invoice.pdf", "/home/user/docs/file.pdf"},
		{"/home/alice/projects/file.pdf", "/home/alice/projects/file.pdf"}, // No match
	}

	for _, tc := range tests {
		result := pm.MapOutgoing(tc.input)
		if result != tc.expect {
			t.Errorf("Input %s: expected %s, got %s", tc.input, tc.expect, result)
		}
	}
}

func TestPathMapper_Priority(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	// Add rules with different priorities
	pm.AddRule(PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/low-priority/",
		Priority:    50,
		Type:        PathRulePrefix,
	})

	pm.AddRule(PathMappingRule{
		Pattern:     "/home/alice/secret/",
		Replacement: "/home/high-priority/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	// Higher priority should match first
	result := pm.MapOutgoing("/home/alice/secret/file.txt")
	if result != "/home/high-priority/file.txt" {
		t.Errorf("Expected high-priority match, got %s", result)
	}

	// Lower priority match
	result = pm.MapOutgoing("/home/alice/other/file.txt")
	if result != "/home/low-priority/other/file.txt" {
		t.Errorf("Expected low-priority match, got %s", result)
	}
}

func TestPathMapper_CaseInsensitive(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true, CaseSensitive: false})

	rule := PathMappingRule{
		Pattern:     "/HOME/ALICE/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	}
	pm.AddRule(rule)

	// Should match regardless of case
	tests := []string{
		"/home/alice/file.txt",
		"/HOME/ALICE/file.txt",
		"/Home/Alice/file.txt",
	}

	for _, input := range tests {
		result := pm.MapOutgoing(input)
		if result != "/home/user/file.txt" {
			t.Errorf("Case-insensitive failed for %s: got %s", input, result)
		}
	}
}

func TestPathMapper_Disabled(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: false})

	pm.AddRule(PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	// Should return original when disabled
	input := "/home/alice/secret.txt"
	result := pm.MapOutgoing(input)
	if result != input {
		t.Error("Disabled mapper should return original path")
	}
}

func TestPathMapper_EnableDisable(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	pm.AddRule(PathMappingRule{
		Pattern:     "/secret/",
		Replacement: "/public/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	// Enabled
	result := pm.MapOutgoing("/secret/file.txt")
	if result == "/secret/file.txt" {
		t.Error("Should map when enabled")
	}

	// Disable
	pm.SetEnabled(false)
	result = pm.MapOutgoing("/secret/file.txt")
	if result != "/secret/file.txt" {
		t.Error("Should not map when disabled")
	}

	// Re-enable
	pm.SetEnabled(true)
	if !pm.IsEnabled() {
		t.Error("Should be enabled")
	}
}

func TestPathMapper_RemoveRule(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	pattern := "/home/alice/"
	pm.AddRule(PathMappingRule{
		Pattern:     pattern,
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	// Should map
	result := pm.MapOutgoing("/home/alice/file.txt")
	if result == "/home/alice/file.txt" {
		t.Error("Should map before removal")
	}

	// Remove rule
	removed := pm.RemoveRule(pattern)
	if !removed {
		t.Error("Rule should be removed")
	}

	// Should not map after removal
	result = pm.MapOutgoing("/home/alice/file.txt")
	if result != "/home/alice/file.txt" {
		t.Error("Should not map after removal")
	}
}

func TestPathMapper_Batch(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	pm.AddRule(PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	// Outgoing batch
	inputs := []string{
		"/home/alice/file1.txt",
		"/home/alice/file2.txt",
		"/home/bob/file3.txt",
	}

	results := pm.MapOutgoingBatch(inputs)

	if results[0] != "/home/user/file1.txt" {
		t.Errorf("Batch[0] failed: got %s", results[0])
	}
	if results[1] != "/home/user/file2.txt" {
		t.Errorf("Batch[1] failed: got %s", results[1])
	}
	if results[2] != "/home/bob/file3.txt" {
		t.Errorf("Batch[2] should not map: got %s", results[2])
	}

	// Incoming batch
	sanitized := []string{
		"/home/user/file1.txt",
		"/home/user/file2.txt",
	}

	recovered := pm.MapIncomingBatch(sanitized)

	if recovered[0] != "/home/alice/file1.txt" {
		t.Errorf("Incoming batch[0] failed: got %s", recovered[0])
	}
	if recovered[1] != "/home/alice/file2.txt" {
		t.Errorf("Incoming batch[1] failed: got %s", recovered[1])
	}
}

func TestPathMapper_ExportImport(t *testing.T) {
	pm1 := NewPathMapper(PathMapperConfig{Enabled: true})

	// Add some rules
	pm1.AddRule(PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	})
	pm1.AddRule(PathMappingRule{
		Pattern:     "/secret/file.txt",
		Replacement: "/public/file.txt",
		Priority:    90,
		Type:        PathRuleExact,
	})

	// Export
	exported := pm1.ExportRules()
	if len(exported) != 2 {
		t.Errorf("Expected 2 exported rules, got %d", len(exported))
	}

	// Import into new mapper
	pm2 := NewPathMapper(PathMapperConfig{Enabled: true})
	err := pm2.ImportRules(exported)
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}

	// Verify same behavior
	result1 := pm1.MapOutgoing("/home/alice/test.txt")
	result2 := pm2.MapOutgoing("/home/alice/test.txt")

	if result1 != result2 {
		t.Errorf("Imported mapper behaves differently: %s != %s", result1, result2)
	}
}

func TestPathMapper_GetRules(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	pm.AddRule(PathMappingRule{
		Pattern:     "/test1/",
		Replacement: "/mapped1/",
		Priority:    100,
		Type:        PathRulePrefix,
	})
	pm.AddRule(PathMappingRule{
		Pattern:     "/test2/",
		Replacement: "/mapped2/",
		Priority:    90,
		Type:        PathRulePrefix,
	})

	rules := pm.GetRules()
	if len(rules) != 2 {
		t.Errorf("Expected 2 rules, got %d", len(rules))
	}

	// Should be sorted by priority (highest first)
	if rules[0].Priority < rules[1].Priority {
		t.Error("Rules not sorted by priority")
	}
}

func TestPathMapper_ClearRules(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	pm.AddRule(PathMappingRule{
		Pattern:     "/test/",
		Replacement: "/mapped/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	if len(pm.GetRules()) == 0 {
		t.Error("Rules should exist before clear")
	}

	pm.ClearRules()

	if len(pm.GetRules()) != 0 {
		t.Error("Rules should be empty after clear")
	}

	// Should not map after clear
	result := pm.MapOutgoing("/test/file.txt")
	if result != "/test/file.txt" {
		t.Error("Should not map after rules cleared")
	}
}

func TestPathMapper_EmptyPath(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	pm.AddRule(PathMappingRule{
		Pattern:     "/test/",
		Replacement: "/mapped/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	// Empty string should remain empty
	if pm.MapOutgoing("") != "" {
		t.Error("Empty path should remain empty")
	}
	if pm.MapIncoming("") != "" {
		t.Error("Empty path should remain empty")
	}
}

func TestPathMapper_NoMatch(t *testing.T) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})

	pm.AddRule(PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	// No match should return original
	input := "/var/log/app.log"
	result := pm.MapOutgoing(input)
	if result != input {
		t.Errorf("No-match should return original, got %s", result)
	}
}

func TestPathMappingStatsTracker(t *testing.T) {
	tracker := NewPathMappingStatsTracker()

	// Record some operations
	tracker.RecordOutgoing(true)  // Mapped
	tracker.RecordOutgoing(true)  // Mapped
	tracker.RecordOutgoing(false) // Unmapped
	tracker.RecordIncoming(true)  // Mapped
	tracker.RecordIncoming(false) // Unmapped
	tracker.RecordIncoming(false) // Unmapped

	stats := tracker.GetStats()

	if stats.TotalOutgoing != 3 {
		t.Errorf("Expected 3 outgoing, got %d", stats.TotalOutgoing)
	}
	if stats.OutgoingMapped != 2 {
		t.Errorf("Expected 2 outgoing mapped, got %d", stats.OutgoingMapped)
	}
	if stats.OutgoingUnmapped != 1 {
		t.Errorf("Expected 1 outgoing unmapped, got %d", stats.OutgoingUnmapped)
	}
	if stats.TotalIncoming != 3 {
		t.Errorf("Expected 3 incoming, got %d", stats.TotalIncoming)
	}
	if stats.IncomingMapped != 1 {
		t.Errorf("Expected 1 incoming mapped, got %d", stats.IncomingMapped)
	}
	if stats.IncomingUnmapped != 2 {
		t.Errorf("Expected 2 incoming unmapped, got %d", stats.IncomingUnmapped)
	}

	// Reset
	tracker.Reset()
	stats = tracker.GetStats()
	if stats.TotalOutgoing != 0 {
		t.Error("Stats should be reset")
	}
}

func TestWildcardToRegex(t *testing.T) {
	tests := []struct {
		pattern string
		input   string
		match   bool
	}{
		{"/home/*/file.txt", "/home/alice/file.txt", true},
		{"/home/*/file.txt", "/home/bob/file.txt", true},
		{"/home/*/file.txt", "/home/alice/bob/file.txt", false}, // * doesn't match /
		{"/home/**/file.txt", "/home/alice/bob/file.txt", true}, // ** matches /
		{"*.txt", "file.txt", true},
		{"*.txt", "document.pdf", false},
	}

	for _, tc := range tests {
		regex, err := wildcardToRegex(tc.pattern)
		if err != nil {
			t.Errorf("Failed to compile %s: %v", tc.pattern, err)
			continue
		}

		match := regex.MatchString(tc.input)
		if match != tc.match {
			t.Errorf("Pattern %s, input %s: expected match=%v, got %v",
				tc.pattern, tc.input, tc.match, match)
		}
	}
}

func BenchmarkMapOutgoing(b *testing.B) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})
	pm.AddRule(PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	path := "/home/alice/documents/report.pdf"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.MapOutgoing(path)
	}
}

func BenchmarkMapIncoming(b *testing.B) {
	pm := NewPathMapper(PathMapperConfig{Enabled: true})
	pm.AddRule(PathMappingRule{
		Pattern:     "/home/alice/",
		Replacement: "/home/user/",
		Priority:    100,
		Type:        PathRulePrefix,
	})

	path := "/home/user/documents/report.pdf"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pm.MapIncoming(path)
	}
}
