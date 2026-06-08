package redaction

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
)

// ConsistentRedactor ensures the same sensitive value is always masked with the same result.
// This maintains data relationships while protecting privacy.
type ConsistentRedactor struct {
	mu    sync.RWMutex
	cache map[string]string // original → consistent masked value
}

// NewConsistentRedactor creates a new consistent redactor.
func NewConsistentRedactor() *ConsistentRedactor {
	return &ConsistentRedactor{
		cache: make(map[string]string),
	}
}

// Redact masks a value consistently across multiple occurrences.
func (cr *ConsistentRedactor) Redact(value string, category FieldCategory) string {
	if value == "" {
		return value
	}

	cr.mu.RLock()
	if masked, exists := cr.cache[value]; exists {
		cr.mu.RUnlock()
		return masked
	}
	cr.mu.RUnlock()

	// Generate consistent mask
	masked := cr.generateConsistentMask(value, category)

	cr.mu.Lock()
	cr.cache[value] = masked
	cr.mu.Unlock()

	return masked
}

// generateConsistentMask creates a deterministic mask based on the value.
func (cr *ConsistentRedactor) generateConsistentMask(value string, category FieldCategory) string {
	// Use SHA256 hash for deterministic result
	h := sha256.New()
	h.Write([]byte(value))
	hash := h.Sum(nil)

	// Use first 8 bytes for a short identifier
	shortHash := hex.EncodeToString(hash[:8])

	// Add category prefix
	prefix := categoryPrefix(category)

	return fmt.Sprintf("[%s_%s]", prefix, shortHash)
}

// RedactBatch processes multiple values efficiently.
func (cr *ConsistentRedactor) RedactBatch(values []string, category FieldCategory) []string {
	if len(values) == 0 {
		return values
	}

	result := make([]string, len(values))
	for i, value := range values {
		result[i] = cr.Redact(value, category)
	}
	return result
}

// GetCacheSize returns the number of cached mappings.
func (cr *ConsistentRedactor) GetCacheSize() int {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	return len(cr.cache)
}

// ClearCache removes all cached mappings.
func (cr *ConsistentRedactor) ClearCache() {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.cache = make(map[string]string)
}

// HasMapping checks if a value has been redacted before.
func (cr *ConsistentRedactor) HasMapping(value string) bool {
	cr.mu.RLock()
	defer cr.mu.RUnlock()
	_, exists := cr.cache[value]
	return exists
}

// ExportCache exports the cache for persistence.
func (cr *ConsistentRedactor) ExportCache() map[string]string {
	cr.mu.RLock()
	defer cr.mu.RUnlock()

	exported := make(map[string]string, len(cr.cache))
	for k, v := range cr.cache {
		exported[k] = v
	}
	return exported
}

// ImportCache imports a previously exported cache.
func (cr *ConsistentRedactor) ImportCache(cache map[string]string) {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	for k, v := range cache {
		cr.cache[k] = v
	}
}
