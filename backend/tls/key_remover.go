package tls

import (
	"bytes"
	"regexp"
)

// SensitiveDataType represents the type of sensitive data detected.
type SensitiveDataType string

// SensitivePattern defines a pattern for detecting sensitive data.
type SensitivePattern struct {
	Type        SensitiveDataType
	Pattern     *regexp.Regexp
	Replacement string
	Priority    int // Higher priority patterns are checked first

	// Markers are cheap fixed substrings every match of Pattern must
	// contain, so a marker miss rules the pattern out without a regex
	// scan. FoldMarkers are the same but case-insensitive, checked against
	// a lowercased copy of the data.
	Markers     []string
	FoldMarkers []string

	replacement []byte // Replacement precomputed at construction
}

var (
	// PEM format patterns
	pemPrivateKeyPattern  = regexp.MustCompile(`(?s)-----BEGIN[A-Z\s]*PRIVATE KEY-----.*?-----END[A-Z\s]*PRIVATE KEY-----`)
	pemCertificatePattern = regexp.MustCompile(`(?s)-----BEGIN CERTIFICATE-----.*?-----END CERTIFICATE-----`)
	pemPublicKeyPattern   = regexp.MustCompile(`(?s)-----BEGIN PUBLIC KEY-----.*?-----END PUBLIC KEY-----`)

	// SSH key patterns
	sshPrivateKeyPattern = regexp.MustCompile(`(?s)-----BEGIN OPENSSH PRIVATE KEY-----.*?-----END OPENSSH PRIVATE KEY-----`)
	sshRSAPattern        = regexp.MustCompile(`ssh-rsa\s+[A-Za-z0-9+/]{100,}={0,2}`)
	sshED25519Pattern    = regexp.MustCompile(`ssh-ed25519\s+[A-Za-z0-9+/]{40,}={0,2}`)

	// API key patterns. api[_-]?key already matches "apikey", so the plain
	// alternative is not repeated.
	genericAPIKeyPattern = regexp.MustCompile(`(?i)(?:['\"]?)(api[_-]?key|access[_-]?key|secret[_-]?key)(?:['\"]?)[\s:=]+['\"]?([A-Za-z0-9_\-]{20,})['\"]?`)
	awsAccessKeyPattern  = regexp.MustCompile(`AKIA[0-9A-Z]{16}`)
	awsSecretKeyPattern  = regexp.MustCompile(`(?i)(?:['\"]?)(aws[_-]?secret[_-]?access[_-]?key)(?:['\"]?)[\s:=]+['\"]?([A-Za-z0-9/+=]{40})['\"]?`)

	// JWT token pattern
	jwtPattern = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`)

	// Bearer token pattern
	bearerTokenPattern = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9_\-\.]{20,}`)

	// Password patterns
	passwordPattern = regexp.MustCompile(`(?i)(?:['\"]?)(password|passwd|pwd)(?:['\"]?)[\s:=]+['\"]?([^\s'\"]{6,})['\"]?`)
)

// DefaultSensitivePatterns returns the default set of patterns for detecting sensitive data.
func DefaultSensitivePatterns() []SensitivePattern {
	return []SensitivePattern{
		// PEM keys and certificates (highest priority)
		{Type: TypePrivateKey, Pattern: sshPrivateKeyPattern, Replacement: "[SSH_PRIVATE_KEY_REMOVED]", Priority: 105, // SSH 先于通用
			Markers: []string{"$$PRIVATEKEY", "-----BEGIN"}},
		{
			Type: TypePrivateKey, Pattern: pemPrivateKeyPattern, Replacement: "[PRIVATE_KEY_REMOVED]", Priority: 100,
			Markers: []string{"-----BEGIN"},
		},
		{
			Type: TypeCertificate, Pattern: pemCertificatePattern, Replacement: "[CERTIFICATE_REMOVED]", Priority: 90,
			Markers: []string{"-----BEGIN"},
		},
		{
			Type: TypeCertificate, Pattern: pemPublicKeyPattern, Replacement: "[PUBLIC_KEY_REMOVED]", Priority: 85,
			Markers: []string{"-----BEGIN"},
		},

		// SSH keys
		{
			Type: TypeSSHKey, Pattern: sshRSAPattern, Replacement: "[SSH_RSA_KEY_REMOVED]", Priority: 95,
			Markers: []string{"ssh-rsa"},
		},
		{
			Type: TypeSSHKey, Pattern: sshED25519Pattern, Replacement: "[SSH_ED25519_KEY_REMOVED]", Priority: 95,
			Markers: []string{"ssh-ed25519"},
		},

		// AWS credentials
		{
			Type: TypeAWSCredential, Pattern: awsAccessKeyPattern, Replacement: "[AWS_ACCESS_KEY_REMOVED]", Priority: 90,
			Markers: []string{"AKIA"},
		},
		{
			Type: TypeAWSCredential, Pattern: awsSecretKeyPattern, Replacement: "[AWS_SECRET_KEY_REMOVED]", Priority: 90,
			FoldMarkers: []string{"aws"},
		},

		// Tokens
		{
			Type: TypeJWTToken, Pattern: jwtPattern, Replacement: "[JWT_TOKEN_REMOVED]", Priority: 80,
			Markers: []string{"eyJ"},
		},
		{
			Type: TypeBearerToken, Pattern: bearerTokenPattern, Replacement: "[BEARER_TOKEN_REMOVED]", Priority: 75,
			FoldMarkers: []string{"bearer"},
		},

		// API keys and passwords
		{
			Type: TypeAPIKey, Pattern: genericAPIKeyPattern, Replacement: "${1}=[API_KEY_REMOVED]", Priority: 70,
			FoldMarkers: []string{"api", "access", "secret"},
		},
		{
			Type: TypePassword, Pattern: passwordPattern, Replacement: "${1}=[PASSWORD_REMOVED]", Priority: 65,
			FoldMarkers: []string{"password", "passwd", "pwd"},
		},
	}
}

// DetectionResult represents a detected sensitive data match.
type DetectionResult struct {
	Type     SensitiveDataType
	Start    int
	End      int
	Matched  string
	Redacted string
}

// markerView lazily builds the lowercased copy of the data for fold-marker
// checks: at most one allocation per scan.
type markerView struct {
	data  []byte
	lower []byte
	built bool
}

func (v *markerView) get() []byte {
	if !v.built {
		v.lower = bytes.ToLower(v.data)
		v.built = true
	}
	return v.lower
}

// patternCouldMatch reports whether data could contain a match of the
// pattern: every match must contain one of the pattern's fixed markers, so a
// marker miss rules the pattern out without running its regex. Case-sensitive
// markers are checked against data, fold markers against the lowercased view.
// Patterns without markers are never ruled out.
func patternCouldMatch(p *SensitivePattern, data []byte, view *markerView) bool {
	if len(p.Markers) == 0 && len(p.FoldMarkers) == 0 {
		return true
	}
	for _, marker := range p.Markers {
		if bytes.Contains(data, []byte(marker)) {
			return true
		}
	}
	for _, marker := range p.FoldMarkers {
		if bytes.Contains(view.get(), []byte(marker)) {
			return true
		}
	}
	return false
}

// KeyRemover handles detection and removal of sensitive data from TLS traffic.
type KeyRemover struct {
	patterns []SensitivePattern
	enabled  bool
}

// NewKeyRemover creates a new KeyRemover with default patterns.
func NewKeyRemover() *KeyRemover {
	patterns := DefaultSensitivePatterns()
	// Sort by priority (highest first)
	for i := range len(patterns) - 1 {
		for j := i + 1; j < len(patterns); j++ {
			if patterns[j].Priority > patterns[i].Priority {
				patterns[i], patterns[j] = patterns[j], patterns[i]
			}
		}
	}
	// Precompute replacement bytes so the hot path never converts per call.
	for i := range patterns {
		patterns[i].replacement = []byte(patterns[i].Replacement)
	}
	return &KeyRemover{
		patterns: patterns,
		enabled:  true,
	}
}

// SetEnabled enables or disables key removal.
func (kr *KeyRemover) SetEnabled(enabled bool) {
	kr.enabled = enabled
}

// IsEnabled returns whether key removal is enabled.
func (kr *KeyRemover) IsEnabled() bool {
	return kr.enabled
}

// DetectSensitiveData scans data for sensitive patterns and returns all matches.
func (kr *KeyRemover) DetectSensitiveData(data []byte) []DetectionResult {
	if !kr.enabled || len(data) == 0 {
		return nil
	}

	var results []DetectionResult
	view := &markerView{data: data}

	for i := range kr.patterns {
		pattern := &kr.patterns[i]
		if !patternCouldMatch(pattern, data, view) {
			continue
		}
		matches := pattern.Pattern.FindAllIndex(data, -1)
		for _, match := range matches {
			if match[0] >= 0 && match[1] <= len(data) {
				matched := string(data[match[0]:match[1]])
				redacted := pattern.Pattern.ReplaceAllString(matched, pattern.Replacement)

				results = append(results, DetectionResult{
					Type:     pattern.Type,
					Start:    match[0],
					End:      match[1],
					Matched:  matched,
					Redacted: redacted,
				})
			}
		}
	}

	return results
}

// RemoveSensitiveData removes sensitive data from the input and returns the sanitized version.
func (kr *KeyRemover) RemoveSensitiveData(data []byte) []byte {
	if !kr.enabled || len(data) == 0 {
		return data
	}

	result := data
	view := &markerView{data: data}

	// Apply patterns in priority order. Each pattern is gated behind its
	// cheap markers: a marker miss skips the regex scan and its allocation
	// entirely, so clean payloads pay one lowercase copy and nothing else.
	for i := range kr.patterns {
		pattern := &kr.patterns[i]
		if !patternCouldMatch(pattern, data, view) {
			continue
		}
		result = pattern.Pattern.ReplaceAll(result, pattern.replacement)
	}

	return result
}

// RemoveSensitiveString is a convenience method for string input.
func (kr *KeyRemover) RemoveSensitiveString(data string) string {
	return string(kr.RemoveSensitiveData([]byte(data)))
}

// ContainsSensitiveData quickly checks if data contains any sensitive patterns.
func (kr *KeyRemover) ContainsSensitiveData(data []byte) bool {
	if !kr.enabled || len(data) == 0 {
		return false
	}

	// Cheap marker gate: every pattern's match must contain one of its
	// markers, so a full marker miss rules out every pattern without any
	// regex work. This covers the api/secret/password/bearer patterns the
	// PEM/SSH/AWS/JWT prefixes alone would miss.
	view := &markerView{data: data}
	for i := range kr.patterns {
		pattern := &kr.patterns[i]
		if !patternCouldMatch(pattern, data, view) {
			continue
		}
		if pattern.Pattern.Match(data) {
			return true
		}
	}

	return false
}

// GetStats returns statistics about detected sensitive data.
type RemovalStats struct {
	TotalScanned  int64
	TotalDetected int64
	ByType        map[SensitiveDataType]int64
	BytesRemoved  int64
}

// Stats tracks removal statistics.
type Stats struct {
	stats RemovalStats
}

// NewStats creates a new Stats tracker.
func NewStats() *Stats {
	return &Stats{
		stats: RemovalStats{
			ByType: make(map[SensitiveDataType]int64),
		},
	}
}

// RecordDetection records a detection event.
func (s *Stats) RecordDetection(dataType SensitiveDataType, bytesRemoved int) {
	s.stats.TotalScanned++
	s.stats.TotalDetected++
	s.stats.ByType[dataType]++
	s.stats.BytesRemoved += int64(bytesRemoved)
}

// GetStats returns a copy of current statistics.
func (s *Stats) GetStats() RemovalStats {
	stats := s.stats
	stats.ByType = make(map[SensitiveDataType]int64)
	for k, v := range s.stats.ByType {
		stats.ByType[k] = v
	}
	return stats
}

// Reset resets all statistics.
func (s *Stats) Reset() {
	s.stats = RemovalStats{
		ByType: make(map[SensitiveDataType]int64),
	}
}
