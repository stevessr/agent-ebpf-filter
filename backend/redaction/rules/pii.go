package rules

import (
	"agent-ebpf-filter/redaction"
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ── PII regex patterns (adapted from privacy-filter) ─────────────────────────

var (
	reEmail = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)

	// Chinese mobile: optional +86 prefix, then 1[3-9]XXXXXXXXX
	rePhoneCN = regexp.MustCompile(`(?:\+?86[-\s]?)?1[3-9][0-9]{9}`)

	// Chinese 18-digit ID card: [1-9] + 16 digits + [0-9Xx]
	reIDCard = regexp.MustCompile(`[1-9][0-9]{16}[0-9Xx]`)

	// Bank card: 13-19 digits (Luhn-checked in code)
	reBankCard = regexp.MustCompile(`[0-9]{13,19}`)

	// IPv4 address
	reIPv4 = regexp.MustCompile(`\b(?:(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(?:25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)
)

// ── PII redaction ────────────────────────────────────────────────────────────

// RedactPII detects and replaces structured PII in free-form text.
//
// Levels:
//   - none/basic: no PII redaction
//   - standard: redacts email, phone, ID card, bank card (Luhn), IPv4
//   - strict: same as standard
func RedactPII(text string, level redaction.RedactionLevel) string {
	if strings.TrimSpace(text) == "" || level == redaction.RedactionLevelNone || level == redaction.RedactionLevelBasic {
		return text
	}

	redacted := text
	redacted = redactEmails(redacted)
	redacted = redactPhoneCN(redacted)
	redacted = redactIDCard(redacted)
	redacted = redactBankCard(redacted)
	redacted = redactIPv4(redacted)
	return redacted
}

func redactEmails(text string) string {
	return reEmail.ReplaceAllStringFunc(text, func(match string) string {
		// Skip emails in SSH/git command context: user@host followed by :path
		lower := strings.ToLower(match)
		if strings.Contains(lower, "ssh ") || strings.Contains(lower, "scp ") ||
			strings.Contains(lower, "rsync ") || strings.Contains(lower, "sftp ") {
			return match
		}
		return "[email]"
	})
}

func redactPhoneCN(text string) string {
	return rePhoneCN.ReplaceAllString(text, "[phone]")
}

func redactIDCard(text string) string {
	return reIDCard.ReplaceAllString(text, "[idcard]")
}

func redactBankCard(text string) string {
	return reBankCard.ReplaceAllStringFunc(text, func(match string) string {
		if !luhnValid(match) {
			return match
		}
		return "[bankcard]"
	})
}

func redactIPv4(text string) string {
	return reIPv4.ReplaceAllString(text, "[ip]")
}

// ── Luhn algorithm ───────────────────────────────────────────────────────────

// luhnValid checks a digit string for Luhn validity (ISO/IEC 7812).
func luhnValid(s string) bool {
	if len(s) < 13 || len(s) > 19 {
		return false
	}
	// Must be all digits
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	sum := 0
	alternate := false
	for i := len(s) - 1; i >= 0; i-- {
		n := int(s[i] - '0')
		if alternate {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alternate = !alternate
	}
	return sum%10 == 0
}

// ── Entropy detection ────────────────────────────────────────────────────────

// RedactHighEntropy detects high-entropy strings (potential secrets) in text.
//
// Levels:
//   - none/basic: no entropy redaction
//   - standard: redacts strings with entropy >= 4.0 and length >= 20
//   - strict: redacts strings with entropy >= 3.5 and length >= 16
func RedactHighEntropy(text string, level redaction.RedactionLevel) string {
	if strings.TrimSpace(text) == "" || level == redaction.RedactionLevelNone || level == redaction.RedactionLevelBasic {
		return text
	}

	minEntropy := 4.0
	minLength := 20
	if level == redaction.RedactionLevelStrict {
		minEntropy = 3.5
		minLength = 16
	}

	return reEntropyToken.ReplaceAllStringFunc(text, func(match string) string {
		// Skip hex hashes, UUIDs, template variables, paths, URLs
		if isFalsePositiveEntropy(match) {
			return match
		}
		entropy := shannonEntropy(match)
		if entropy >= minEntropy && utf8.RuneCountInString(match) >= minLength {
			return "[secret]"
		}
		return match
	})
}

// reEntropyToken matches potential high-entropy tokens: base64-like strings of 12+ chars
var reEntropyToken = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{12,}`)

func isFalsePositiveEntropy(s string) bool {
	// Hex hash (md5/sha1/sha256 lengths)
	if isHexHash(s) {
		return true
	}
	// UUID
	if isUUID(s) {
		return true
	}
	// Template variable: {{...}}, ${...}, %{...}
	if isTemplateVar(s) {
		return true
	}
	// Path or URL
	if strings.Contains(s, "/") || strings.Contains(s, "\\") {
		return true
	}
	// Common words (all lowercase, dictionary-like)
	if isLikelyWord(s) {
		return true
	}
	return false
}

func isHexHash(s string) bool {
	l := len(s)
	if l != 32 && l != 40 && l != 64 {
		return false
	}
	for _, r := range s {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return false
		}
	}
	return true
}

func isUUID(s string) bool {
	if len(s) != 36 {
		return false
	}
	return s[8] == '-' && s[13] == '-' && s[18] == '-' && s[23] == '-'
}

func isTemplateVar(s string) bool {
	return strings.HasPrefix(s, "{{") || strings.HasPrefix(s, "${") ||
		strings.HasPrefix(s, "%{")
}

var commonWords = map[string]struct{}{
	"the": {}, "this": {}, "that": {}, "with": {}, "from": {},
	"have": {}, "been": {}, "were": {}, "they": {}, "will": {},
	"would": {}, "could": {}, "should": {}, "their": {}, "there": {},
	"about": {}, "which": {}, "where": {}, "after": {}, "still": {},
	"between": {}, "through": {}, "another": {}, "yourselves": {},
	"yourself": {}, "ourselves": {}, "something": {}, "everything": {},
	"together": {}, "important": {}, "difference": {}, "application": {},
	"configuration": {}, "development": {}, "environment": {},
	"implementation": {}, "authentication": {}, "authorization": {},
	"administrator": {}, "documentation": {}, "recommendation": {},
}

func isLikelyWord(s string) bool {
	lower := strings.ToLower(s)
	if len(lower) < 3 || len(lower) > 30 {
		return false
	}
	if _, ok := commonWords[lower]; ok {
		return true
	}
	// All lowercase letters only (dictionary word)
	for _, r := range lower {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

// shannonEntropy calculates the Shannon entropy of a string.
func shannonEntropy(s string) float64 {
	if len(s) == 0 {
		return 0
	}
	freq := make(map[rune]float64)
	for _, r := range s {
		freq[r]++
	}
	var entropy float64
	length := float64(len(s))
	for _, count := range freq {
		p := count / length
		entropy -= p * math.Log2(p)
	}
	return entropy
}