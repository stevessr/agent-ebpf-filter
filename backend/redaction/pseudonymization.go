package redaction

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"sync"
)

// PseudonymEngine implements GDPR-compliant pseudonymization with reversible token replacement.
// Unlike anonymization, pseudonymization allows re-identification through a secure mapping.
type PseudonymEngine struct {
	mu           sync.RWMutex
	hmacKey      []byte                       // Secret key for HMAC-SHA256
	mapping      map[string]string            // original → pseudonym
	reverseMap   map[string]string            // pseudonym → original
	enabled      bool
	preserveCase bool                         // Preserve original case in pseudonyms
}

// PseudonymConfig configures the pseudonymization engine.
type PseudonymConfig struct {
	HMACKey      []byte // Secret key (32 bytes recommended for SHA256)
	Enabled      bool
	PreserveCase bool
}

// NewPseudonymEngine creates a new pseudonymization engine.
// If no HMAC key is provided, a random key is generated.
func NewPseudonymEngine(config PseudonymConfig) (*PseudonymEngine, error) {
	key := config.HMACKey
	if len(key) == 0 {
		// Generate random 32-byte key
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, fmt.Errorf("failed to generate random key: %w", err)
		}
	}

	if len(key) < 16 {
		return nil, fmt.Errorf("HMAC key must be at least 16 bytes, got %d", len(key))
	}

	return &PseudonymEngine{
		hmacKey:      key,
		mapping:      make(map[string]string),
		reverseMap:   make(map[string]string),
		enabled:      config.Enabled,
		preserveCase: config.PreserveCase,
	}, nil
}

// SetEnabled enables or disables pseudonymization.
func (pe *PseudonymEngine) SetEnabled(enabled bool) {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.enabled = enabled
}

// IsEnabled returns whether pseudonymization is enabled.
func (pe *PseudonymEngine) IsEnabled() bool {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return pe.enabled
}

// Pseudonymize replaces a sensitive value with a consistent pseudonym.
// The same input always produces the same pseudonym (deterministic).
func (pe *PseudonymEngine) Pseudonymize(value string, category FieldCategory) string {
	if !pe.enabled || value == "" {
		return value
	}

	pe.mu.Lock()
	defer pe.mu.Unlock()

	// Check if we already have a pseudonym for this value
	if pseudonym, exists := pe.mapping[value]; exists {
		return pseudonym
	}

	// Generate new pseudonym
	pseudonym := pe.generatePseudonym(value, category)

	// Store bidirectional mapping
	pe.mapping[value] = pseudonym
	pe.reverseMap[pseudonym] = value

	return pseudonym
}

// PseudonymizeBatch processes multiple values efficiently.
func (pe *PseudonymEngine) PseudonymizeBatch(values []string, category FieldCategory) []string {
	if !pe.enabled || len(values) == 0 {
		return values
	}

	result := make([]string, len(values))
	for i, value := range values {
		result[i] = pe.Pseudonymize(value, category)
	}
	return result
}

// Depseudonymize reverses the pseudonymization (requires proper authorization).
// Returns the original value or an error if not found.
func (pe *PseudonymEngine) Depseudonymize(pseudonym string) (string, error) {
	if !pe.enabled {
		return pseudonym, nil
	}

	pe.mu.RLock()
	defer pe.mu.RUnlock()

	if original, exists := pe.reverseMap[pseudonym]; exists {
		return original, nil
	}

	return "", fmt.Errorf("pseudonym not found in mapping")
}

// generatePseudonym creates a deterministic pseudonym using HMAC-SHA256.
// Format: <category_prefix>_<hmac_hash_base64>
func (pe *PseudonymEngine) generatePseudonym(value string, category FieldCategory) string {
	// Compute HMAC-SHA256
	h := hmac.New(sha256.New, pe.hmacKey)
	h.Write([]byte(value))
	hash := h.Sum(nil)

	// Use first 12 bytes for compactness (96 bits, sufficient for uniqueness)
	shortHash := hash[:12]

	// Encode as base64 (URL-safe, no padding)
	encoded := base64.RawURLEncoding.EncodeToString(shortHash)

	// Add category prefix for readability
	prefix := categoryPrefix(category)

	return fmt.Sprintf("%s_%s", prefix, encoded)
}

// categoryPrefix returns a short prefix for each category.
func categoryPrefix(category FieldCategory) string {
	switch category {
	case FieldCategoryPath:
		return "PATH"
	case FieldCategoryCommand:
		return "CMD"
	case FieldCategoryNetwork:
		return "NET"
	case FieldCategoryCredential:
		return "CRED"
	case FieldCategoryIdentifier:
		return "ID"
	default:
		return "DATA"
	}
}

// GetMappingSize returns the number of stored mappings.
func (pe *PseudonymEngine) GetMappingSize() int {
	pe.mu.RLock()
	defer pe.mu.RUnlock()
	return len(pe.mapping)
}

// ClearMapping removes all stored mappings (use with caution).
func (pe *PseudonymEngine) ClearMapping() {
	pe.mu.Lock()
	defer pe.mu.Unlock()
	pe.mapping = make(map[string]string)
	pe.reverseMap = make(map[string]string)
}

// ExportMapping exports the mapping for backup/audit (requires secure storage).
func (pe *PseudonymEngine) ExportMapping() map[string]string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	exported := make(map[string]string, len(pe.mapping))
	for k, v := range pe.mapping {
		exported[k] = v
	}
	return exported
}

// ImportMapping imports a previously exported mapping.
func (pe *PseudonymEngine) ImportMapping(mapping map[string]string) {
	pe.mu.Lock()
	defer pe.mu.Unlock()

	for original, pseudonym := range mapping {
		pe.mapping[original] = pseudonym
		pe.reverseMap[pseudonym] = original
	}
}

// GetHMACKeyFingerprint returns a fingerprint of the HMAC key (for verification).
// Does NOT expose the key itself.
func (pe *PseudonymEngine) GetHMACKeyFingerprint() string {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	h := sha256.New()
	h.Write(pe.hmacKey)
	fingerprint := h.Sum(nil)
	return hex.EncodeToString(fingerprint[:8]) // First 8 bytes
}

// PseudonymStats tracks pseudonymization statistics.
type PseudonymStats struct {
	TotalPseudonymized   int64
	UniqueMappings       int64
	CacheHits            int64
	CacheMisses          int64
	DepseudonymRequests  int64
	DepseudonymSuccesses int64
}

// Stats tracks pseudonymization statistics.
type PseudonymStatTracker struct {
	mu    sync.RWMutex
	stats PseudonymStats
}

// NewPseudonymStatTracker creates a new statistics tracker.
func NewPseudonymStatTracker() *PseudonymStatTracker {
	return &PseudonymStatTracker{}
}

// RecordPseudonymize records a pseudonymization operation.
func (pst *PseudonymStatTracker) RecordPseudonymize(cacheHit bool) {
	pst.mu.Lock()
	defer pst.mu.Unlock()

	pst.stats.TotalPseudonymized++
	if cacheHit {
		pst.stats.CacheHits++
	} else {
		pst.stats.CacheMisses++
		pst.stats.UniqueMappings++
	}
}

// RecordDepseudonymize records a depseudonymization operation.
func (pst *PseudonymStatTracker) RecordDepseudonymize(success bool) {
	pst.mu.Lock()
	defer pst.mu.Unlock()

	pst.stats.DepseudonymRequests++
	if success {
		pst.stats.DepseudonymSuccesses++
	}
}

// GetStats returns a copy of current statistics.
func (pst *PseudonymStatTracker) GetStats() PseudonymStats {
	pst.mu.RLock()
	defer pst.mu.RUnlock()
	return pst.stats
}

// Reset resets all statistics.
func (pst *PseudonymStatTracker) Reset() {
	pst.mu.Lock()
	defer pst.mu.Unlock()
	pst.stats = PseudonymStats{}
}
