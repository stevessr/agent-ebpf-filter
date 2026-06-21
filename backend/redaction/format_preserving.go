package redaction

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
)

// FormatPreservingMasker masks data while preserving format and length.
// Useful for testing environments where data structure must be maintained.
type FormatPreservingMasker struct {
	rng *rand.Rand
}

// NewFormatPreservingMasker creates a new format-preserving masker.
func NewFormatPreservingMasker() *FormatPreservingMasker {
	return &FormatPreservingMasker{
		rng: rand.New(rand.NewSource(42)), // Fixed seed for deterministic results
	}
}

// MaskEmail masks an email while preserving format.
// Example: user@example.com → fake_ab12@example.com
func (fpm *FormatPreservingMasker) MaskEmail(email string) string {
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return email
	}

	// Generate fake username with random suffix
	fakeUser := fmt.Sprintf("fake_%s", randomAlphanumeric(4))

	return fmt.Sprintf("%s@%s", fakeUser, parts[1])
}

// MaskPhone masks a phone number while preserving format.
// Example: +1-234-567-8900 → +1-555-000-1234
func (fpm *FormatPreservingMasker) MaskPhone(phone string) string {
	// Preserve format characters (-, +, spaces, parentheses)
	format := extractFormat(phone)

	// Replace digits with fake ones
	masked := ""
	digitCount := 0

	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			// Use 555 prefix for first 3 digits, then random
			if digitCount < 3 {
				masked += "5"
			} else {
				masked += fmt.Sprintf("%d", fpm.rng.Intn(10))
			}
			digitCount++
		} else {
			masked += string(ch)
		}
	}

	return masked
}

// MaskCreditCard masks a credit card number, showing first 4 and last 4 digits.
// Example: 4532-1234-5678-9010 → 4532-****-****-9010
func (fpm *FormatPreservingMasker) MaskCreditCard(cc string) string {
	// Remove non-digits
	digits := extractDigits(cc)

	if len(digits) < 8 {
		return cc // Too short
	}

	// Keep first 4 and last 4
	first4 := digits[:4]
	last4 := digits[len(digits)-4:]

	// Determine format
	if strings.Contains(cc, "-") {
		// Format: XXXX-XXXX-XXXX-XXXX
		return fmt.Sprintf("%s-****-****-%s", first4, last4)
	} else if strings.Contains(cc, " ") {
		// Format: XXXX XXXX XXXX XXXX
		return fmt.Sprintf("%s **** **** %s", first4, last4)
	} else {
		// No separator
		return fmt.Sprintf("%s********%s", first4, last4)
	}
}

// MaskSSN masks a social security number.
// Example: 123-45-6789 → ***-**-6789
func (fpm *FormatPreservingMasker) MaskSSN(ssn string) string {
	// Preserve format
	if strings.Contains(ssn, "-") {
		// Format: XXX-XX-XXXX
		parts := strings.Split(ssn, "-")
		if len(parts) == 3 {
			return fmt.Sprintf("***-**-%s", parts[2])
		}
	}

	// No format, just mask first 5 digits
	if len(ssn) >= 9 {
		return "*****" + ssn[5:]
	}

	return ssn
}

// MaskIPv4 masks an IPv4 address while keeping format.
// Example: 192.168.1.100 → 10.0.0.123
func (fpm *FormatPreservingMasker) MaskIPv4(ip string) string {
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		return ip
	}

	// Use 10.0.0.x pattern (private IP)
	return fmt.Sprintf("10.0.0.%d", fpm.rng.Intn(256))
}

// MaskUsername generates a fake username of similar length.
// Example: alice_smith → fake_user_7a3b
func (fpm *FormatPreservingMasker) MaskUsername(username string) string {
	// Preserve length approximately
	length := len(username)
	if length < 4 {
		length = 4
	} else if length > 20 {
		length = 20
	}

	return fmt.Sprintf("fake_user_%s", randomAlphanumeric(length-10))
}

// Helper functions

func extractFormat(s string) string {
	format := ""
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			format += "X"
		} else {
			format += string(ch)
		}
	}
	return format
}

func extractDigits(s string) string {
	digits := ""
	for _, ch := range s {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}
	return digits
}

func randomAlphanumeric(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[rand.Intn(len(charset))]
	}
	return string(result)
}

// MaskByType automatically detects type and applies appropriate masking.
func (fpm *FormatPreservingMasker) MaskByType(value string) string {
	// Email detection
	if emailRegex.MatchString(value) {
		return fpm.MaskEmail(value)
	}

	// Phone detection
	if phoneRegex.MatchString(value) {
		return fpm.MaskPhone(value)
	}

	// Credit card detection
	if ccRegex.MatchString(value) {
		return fpm.MaskCreditCard(value)
	}

	// SSN detection
	if ssnRegex.MatchString(value) {
		return fpm.MaskSSN(value)
	}

	// IPv4 detection
	if ipv4Regex.MatchString(value) {
		return fpm.MaskIPv4(value)
	}

	// Default: use consistent hash-based masking
	return fmt.Sprintf("[MASKED_%s]", randomAlphanumeric(8))
}

// Patterns for auto-detection
var (
	emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	phoneRegex = regexp.MustCompile(`^\+?[\d\s\-\(\)]{10,}$`)
	ccRegex    = regexp.MustCompile(`^[\d\s\-]{13,19}$`)
	ssnRegex   = regexp.MustCompile(`^\d{3}-?\d{2}-?\d{4}$`)
	ipv4Regex  = regexp.MustCompile(`^\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)
)
