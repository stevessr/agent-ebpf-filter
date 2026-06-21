package redaction

import (
	"strings"
	"testing"
)

func TestNewPseudonymEngine(t *testing.T) {
	t.Run("with custom key", func(t *testing.T) {
		key := []byte("this-is-a-test-key-32-bytes!")
		pe, err := NewPseudonymEngine(PseudonymConfig{
			HMACKey: key,
			Enabled: true,
		})
		if err != nil {
			t.Fatalf("Failed to create engine: %v", err)
		}
		if !pe.IsEnabled() {
			t.Error("Engine should be enabled")
		}
	})

	t.Run("with auto-generated key", func(t *testing.T) {
		pe, err := NewPseudonymEngine(PseudonymConfig{Enabled: true})
		if err != nil {
			t.Fatalf("Failed to create engine: %v", err)
		}
		if len(pe.hmacKey) != 32 {
			t.Errorf("Expected 32-byte key, got %d", len(pe.hmacKey))
		}
	})

	t.Run("with short key", func(t *testing.T) {
		_, err := NewPseudonymEngine(PseudonymConfig{
			HMACKey: []byte("short"),
			Enabled: true,
		})
		if err == nil {
			t.Error("Should reject short keys")
		}
	})
}

func TestPseudonymize_Consistency(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	original := "sensitive-value-123"

	// First pseudonymization
	pseudo1 := pe.Pseudonymize(original, FieldCategoryIdentifier)

	// Second pseudonymization of same value
	pseudo2 := pe.Pseudonymize(original, FieldCategoryIdentifier)

	if pseudo1 != pseudo2 {
		t.Errorf("Pseudonymization not consistent: %s != %s", pseudo1, pseudo2)
	}

	if pseudo1 == original {
		t.Error("Pseudonym should be different from original")
	}
}

func TestPseudonymize_Categories(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	testCases := []struct {
		value    string
		category FieldCategory
		prefix   string
	}{
		{"/home/user/file.txt", FieldCategoryPath, "PATH_"},
		{"192.168.1.1", FieldCategoryNetwork, "NET_"},
		{"curl http://api.example.com", FieldCategoryCommand, "CMD_"},
		{"password123", FieldCategoryCredential, "CRED_"},
		{"user-id-456", FieldCategoryIdentifier, "ID_"},
	}

	for _, tc := range testCases {
		t.Run(string(tc.category), func(t *testing.T) {
			pseudonym := pe.Pseudonymize(tc.value, tc.category)

			if !strings.HasPrefix(pseudonym, tc.prefix) {
				t.Errorf("Expected prefix %s, got: %s", tc.prefix, pseudonym)
			}

			if pseudonym == tc.value {
				t.Error("Pseudonym should differ from original")
			}
		})
	}
}

func TestDepseudonymize(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	original := "secret-data-789"
	category := FieldCategoryIdentifier

	// Pseudonymize
	pseudonym := pe.Pseudonymize(original, category)

	// Depseudonymize
	recovered, err := pe.Depseudonymize(pseudonym)
	if err != nil {
		t.Fatalf("Depseudonymization failed: %v", err)
	}

	if recovered != original {
		t.Errorf("Expected %s, got %s", original, recovered)
	}
}

func TestDepseudonymize_NotFound(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	_, err := pe.Depseudonymize("ID_nonexistent")
	if err == nil {
		t.Error("Should return error for unknown pseudonym")
	}
}

func TestPseudonymizeBatch(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	values := []string{
		"/home/user1/file.txt",
		"/home/user2/file.txt",
		"/home/user1/file.txt", // Duplicate
	}

	results := pe.PseudonymizeBatch(values, FieldCategoryPath)

	// Check all were processed
	if len(results) != len(values) {
		t.Errorf("Expected %d results, got %d", len(values), len(results))
	}

	// Check consistency for duplicates
	if results[0] != results[2] {
		t.Error("Duplicate values should have same pseudonym")
	}

	// Check different values have different pseudonyms
	if results[0] == results[1] {
		t.Error("Different values should have different pseudonyms")
	}
}

func TestPseudonymEngine_Disabled(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: false})

	original := "sensitive-value"
	result := pe.Pseudonymize(original, FieldCategoryIdentifier)

	if result != original {
		t.Error("Disabled engine should return original value")
	}
}

func TestPseudonymEngine_EnableDisable(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	original := "test-value"

	// Enabled - should pseudonymize
	pseudo := pe.Pseudonymize(original, FieldCategoryIdentifier)
	if pseudo == original {
		t.Error("Should pseudonymize when enabled")
	}

	// Disable
	pe.SetEnabled(false)
	result := pe.Pseudonymize("another-value", FieldCategoryIdentifier)
	if result != "another-value" {
		t.Error("Should return original when disabled")
	}

	// Re-enable
	pe.SetEnabled(true)
	if !pe.IsEnabled() {
		t.Error("Should be enabled")
	}
}

func TestGetMappingSize(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	if pe.GetMappingSize() != 0 {
		t.Error("Initial mapping should be empty")
	}

	pe.Pseudonymize("value1", FieldCategoryIdentifier)
	pe.Pseudonymize("value2", FieldCategoryIdentifier)
	pe.Pseudonymize("value1", FieldCategoryIdentifier) // Duplicate

	if size := pe.GetMappingSize(); size != 2 {
		t.Errorf("Expected 2 mappings, got %d", size)
	}
}

func TestClearMapping(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	original := "test-value"
	pseudo1 := pe.Pseudonymize(original, FieldCategoryIdentifier)

	pe.ClearMapping()

	if pe.GetMappingSize() != 0 {
		t.Error("Mapping should be empty after clear")
	}

	// After clear, same value should get different pseudonym
	pseudo2 := pe.Pseudonymize(original, FieldCategoryIdentifier)
	if pseudo1 == pseudo2 {
		t.Error("New pseudonym should differ after mapping clear")
	}
}

func TestExportImportMapping(t *testing.T) {
	pe1, _ := NewPseudonymEngine(PseudonymConfig{
		HMACKey: []byte("shared-key-for-export-test!!"),
		Enabled: true,
	})

	// Create some mappings
	values := []string{"val1", "val2", "val3"}
	for _, v := range values {
		pe1.Pseudonymize(v, FieldCategoryIdentifier)
	}

	// Export
	exported := pe1.ExportMapping()
	if len(exported) != 3 {
		t.Errorf("Expected 3 exported mappings, got %d", len(exported))
	}

	// Create new engine and import
	pe2, _ := NewPseudonymEngine(PseudonymConfig{
		HMACKey: []byte("shared-key-for-export-test!!"),
		Enabled: true,
	})
	pe2.ImportMapping(exported)

	// Verify imported mappings work
	for original, expectedPseudo := range exported {
		recovered, err := pe2.Depseudonymize(expectedPseudo)
		if err != nil {
			t.Errorf("Failed to depseudonymize imported mapping: %v", err)
		}
		if recovered != original {
			t.Errorf("Expected %s, got %s", original, recovered)
		}
	}
}

func TestGetHMACKeyFingerprint(t *testing.T) {
	key := []byte("test-key-for-fingerprint!!!!!")

	pe1, _ := NewPseudonymEngine(PseudonymConfig{HMACKey: key, Enabled: true})
	pe2, _ := NewPseudonymEngine(PseudonymConfig{HMACKey: key, Enabled: true})
	pe3, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true}) // Different key

	fp1 := pe1.GetHMACKeyFingerprint()
	fp2 := pe2.GetHMACKeyFingerprint()
	fp3 := pe3.GetHMACKeyFingerprint()

	// Same key should have same fingerprint
	if fp1 != fp2 {
		t.Error("Same keys should have same fingerprint")
	}

	// Different key should have different fingerprint
	if fp1 == fp3 {
		t.Error("Different keys should have different fingerprints")
	}

	// Fingerprint should not expose the key
	if len(fp1) < 16 {
		t.Error("Fingerprint too short")
	}
}

func TestPseudonymStatTracker(t *testing.T) {
	tracker := NewPseudonymStatTracker()

	// Record some operations
	tracker.RecordPseudonymize(false) // New mapping
	tracker.RecordPseudonymize(true)  // Cache hit
	tracker.RecordPseudonymize(true)  // Cache hit
	tracker.RecordDepseudonymize(true)
	tracker.RecordDepseudonymize(false)

	stats := tracker.GetStats()

	if stats.TotalPseudonymized != 3 {
		t.Errorf("Expected 3 pseudonymizations, got %d", stats.TotalPseudonymized)
	}
	if stats.CacheHits != 2 {
		t.Errorf("Expected 2 cache hits, got %d", stats.CacheHits)
	}
	if stats.UniqueMappings != 1 {
		t.Errorf("Expected 1 unique mapping, got %d", stats.UniqueMappings)
	}
	if stats.DepseudonymRequests != 2 {
		t.Errorf("Expected 2 depseudonym requests, got %d", stats.DepseudonymRequests)
	}
	if stats.DepseudonymSuccesses != 1 {
		t.Errorf("Expected 1 success, got %d", stats.DepseudonymSuccesses)
	}

	// Reset
	tracker.Reset()
	stats = tracker.GetStats()
	if stats.TotalPseudonymized != 0 {
		t.Error("Stats should be reset")
	}
}

func TestPseudonymize_EmptyValue(t *testing.T) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})

	result := pe.Pseudonymize("", FieldCategoryIdentifier)
	if result != "" {
		t.Error("Empty value should remain empty")
	}
}

func TestPseudonymize_Deterministic(t *testing.T) {
	key := []byte("deterministic-test-key-32byte")

	pe1, _ := NewPseudonymEngine(PseudonymConfig{HMACKey: key, Enabled: true})
	pe2, _ := NewPseudonymEngine(PseudonymConfig{HMACKey: key, Enabled: true})

	original := "test-value-for-determinism"

	pseudo1 := pe1.Pseudonymize(original, FieldCategoryIdentifier)
	pseudo2 := pe2.Pseudonymize(original, FieldCategoryIdentifier)

	// Same key + same value = same pseudonym (deterministic)
	if pseudo1 != pseudo2 {
		t.Errorf("Pseudonymization not deterministic: %s != %s", pseudo1, pseudo2)
	}
}

func BenchmarkPseudonymize(b *testing.B) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})
	value := "benchmark-test-value-12345"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pe.Pseudonymize(value, FieldCategoryIdentifier)
	}
}

func BenchmarkDepseudonymize(b *testing.B) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})
	value := "benchmark-test-value-12345"
	pseudonym := pe.Pseudonymize(value, FieldCategoryIdentifier)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pe.Depseudonymize(pseudonym)
	}
}

func BenchmarkPseudonymizeBatch(b *testing.B) {
	pe, _ := NewPseudonymEngine(PseudonymConfig{Enabled: true})
	values := []string{
		"value1", "value2", "value3", "value4", "value5",
		"value6", "value7", "value8", "value9", "value10",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pe.PseudonymizeBatch(values, FieldCategoryIdentifier)
	}
}
