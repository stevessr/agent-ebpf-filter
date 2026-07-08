package rules

import (
	"agent-ebpf-filter/redaction"
	"regexp"
	"strings"
)

// ── Context-based secret detection patterns ──────────────────────────────────
//
// Detects secrets expressed in natural language or structured assignment
// contexts that are not caught by inline credential patterns (credential.go).
// Examples:
//
//	password = "hunter2"
//	api_key: sk-abc123...
//	the secret token is ghp_xxxxx
//	my password is correcthorsebatterystaple

var (
	// Context assignment: key: value or key = value with secret-semantic keys
	reContextAssignment = regexp.MustCompile(`(?i)((?:password|passwd|secret|token|api[_-]?key|access[_-]?(?:key|token)|auth[_-]?(?:key|token)|client[_-]?secret|private[_-]?key)\s*[:=]\s*)(['"]?)([^\s,;\"')]{8,})`)

	// Natural language: "the X is Y", "X = Y", "set X to Y"
	reContextNaturalLang = regexp.MustCompile(`(?i)((?:(?:the|my|your|our|a|is|set)\s+)?(?:password|passwd|secret|token|api[_-]?key|access[_-]?(?:key|token)|auth[_-]?(?:key|token))\s+(?:is|was|will\s+be|has\s+been|:=|==?)\s+)(['"]?)([^\s,;\"')]{8,})`)

	// URL-embedded credentials: https://user:pass@host
	reURLCredentials = regexp.MustCompile(`([a-z]+://)[^/\s:@]+(:[^/\s@]+)?@`)

	// Variable assignment in code/config context (value = "long_string")
	reVarAssignment = regexp.MustCompile(`(?i)((?:const|let|var|export|set|env)\s+)?(?:[\w_-]*(?:password|passwd|secret|token|api[_-]?key|access[_-]?(?:key|token)|auth[_-]?(?:key|token)|credential)[\w_-]*\s*[:=]\s*)(['"]?)([^\s,;\"')]{8,})`)
)

// ── Gitleaks-style keyword patterns (simplified subset) ─────────────────────
//
// These mirror the gitleaks approach: keyword pre-filter + entropy threshold.
// Only the most common secret formats are included here to avoid false positives.
// The full gitleaks rule set (222 rules) is available in docs/ref/privacy-filter/rules/gitleaks.toml.

type gitleaksRule struct {
	ID          string
	Description string
	Regex       *regexp.Regexp
	Keywords    []string // pre-filter keywords
	SecretGroup int      // regex capture group for the secret value
	Entropy     float64  // minimum entropy threshold
}

var gitleaksRules = []gitleaksRule{
	{
		ID: "aws-access-token", Description: "AWS Access Key ID",
		Regex:       regexp.MustCompile(`(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}`),
		Keywords:    []string{"AKIA", "ASIA"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "aws-secret-key", Description: "AWS Secret Access Key",
		Regex:       regexp.MustCompile(`(?i)aws[_-]?(?:secret|access)[_-]?key\s*[:=]\s*['"]?([A-Za-z0-9/+]{40})`),
		Keywords:    []string{"aws_secret", "aws_secret_access", "AWS_SECRET_ACCESS_KEY"},
		SecretGroup: 1, Entropy: 4.5,
	},
	{
		ID: "google-api-key", Description: "Google API Key",
		Regex:       regexp.MustCompile(`AIza[0-9A-Za-z\-_]{35}`),
		Keywords:    []string{"AIza"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "github-token", Description: "GitHub Personal Access Token",
		Regex:       regexp.MustCompile(`ghp_[0-9A-Za-z]{36,}`),
		Keywords:    []string{"ghp_"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "github-oauth", Description: "GitHub OAuth Access Token",
		Regex:       regexp.MustCompile(`gho_[0-9A-Za-z]{36,}`),
		Keywords:    []string{"gho_"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "jwt-token", Description: "JSON Web Token (JWT)",
		Regex:       regexp.MustCompile(`eyJ[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+\.[A-Za-z0-9-_]+`),
		Keywords:    []string{"eyJ"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "slack-token", Description: "Slack Bot / App Token",
		Regex:       regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]{10,}`),
		Keywords:    []string{"xoxb-", "xoxa-", "xoxp-", "xoxr-", "xoxs-"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "openai-api-key", Description: "OpenAI API Key",
		Regex:       regexp.MustCompile(`sk-[A-Za-z0-9]{20,}`),
		Keywords:    []string{"sk-"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "private-key", Description: "Private/SSH Key (PEM)",
		Regex:       regexp.MustCompile(`-----BEGIN\s+(?:RSA|DSA|EC|OPENSSH|SSH2|PGP)\s+PRIVATE KEY-----`),
		Keywords:    []string{"BEGIN PRIVATE KEY", "BEGIN RSA PRIVATE KEY", "BEGIN OPENSSH PRIVATE KEY"},
		SecretGroup: 0, Entropy: 0,
	},
	{
		ID: "generic-api-key", Description: "Generic API Key (high-entropy, keyword-adjacent)",
		Regex:       regexp.MustCompile(`(?i)(?:api[_-]?key|apikey|api[_-]?secret)\s*[:=]\s*['"]?((?:[A-Za-z0-9+/]{20,}|[A-Za-z0-9_\-]{20,}))`),
		Keywords:    []string{"api_key", "apikey", "api-key", "api_secret", "api-secret"},
		SecretGroup: 1, Entropy: 3.5,
	},
}

// ── Redaction functions ──────────────────────────────────────────────────────

// RedactContextSecret detects secrets expressed in natural language context
// (e.g., "password is X", "api_key: X") and replaces the value portion.
//
// Levels:
//   - none: no redaction
//   - basic: redacts clear-keyword patterns (password=, api_key:)
//   - standard: adds URL credentials, natural language patterns
//   - strict: standard + variable assignments in code config context
func RedactContextSecret(text string, level redaction.RedactionLevel) string {
	if strings.TrimSpace(text) == "" || level == redaction.RedactionLevelNone {
		return text
	}

	redacted := text
	if level == redaction.RedactionLevelBasic || level == redaction.RedactionLevelStandard || level == redaction.RedactionLevelStrict {
		redacted = redactByRegex(redacted, reContextAssignment, "[REDACTED_CONTEXT_SECRET]")
		redacted = redactByRegex(redacted, reURLCredentials, "${1}[REDACTED_CREDS]@")
	}
	if level == redaction.RedactionLevelStandard || level == redaction.RedactionLevelStrict {
		redacted = redactByRegex(redacted, reContextNaturalLang, "[REDACTED_CONTEXT_SECRET]")
	}
	if level == redaction.RedactionLevelStrict {
		redacted = redactByRegex(redacted, reVarAssignment, "[REDACTED_CONTEXT_SECRET]")
	}
	return redacted
}

// RedactGitleaks detects known secret formats using gitleaks-style rules.
// Only runs rules whose keywords appear in the text (pre-filter for speed).
func RedactGitleaks(text string, level redaction.RedactionLevel) string {
	if strings.TrimSpace(text) == "" || level == redaction.RedactionLevelNone || level == redaction.RedactionLevelBasic {
		return text
	}

	redacted := text
	lower := strings.ToLower(redacted)

	for _, rule := range gitleaksRules {
		// Keyword pre-filter: skip if none of the keywords appear
		hasKeyword := len(rule.Keywords) == 0
		for _, kw := range rule.Keywords {
			if strings.Contains(lower, strings.ToLower(kw)) {
				hasKeyword = true
				break
			}
		}
		if !hasKeyword {
			continue
		}

		// Run regex and replace matches
		redacted = rule.Regex.ReplaceAllStringFunc(redacted, func(match string) string {
			if rule.Entropy > 0 && rule.SecretGroup > 0 {
				groups := rule.Regex.FindStringSubmatch(match)
				if len(groups) > rule.SecretGroup {
					entropy := shannonEntropy(groups[rule.SecretGroup])
					if entropy < rule.Entropy {
						return match
					}
				}
			}
			return "[REDACTED_GITLEAKS]"
		})
	}
	return redacted
}

// redactByRegex replaces all matches of a regex, preserving the prefix group (group 1)
// and replacing the rest. If the regex has exactly 1 capture group (the whole match),
// the entire match is replaced.
func redactByRegex(text string, re *regexp.Regexp, replacement string) string {
	return re.ReplaceAllStringFunc(text, func(match string) string {
		groups := re.FindStringSubmatch(match)
		if len(groups) >= 2 && groups[1] != "" {
			// Preserve the key/prefix portion, replace the value
			prefixLen := len(groups[1])
			// Check if there's a quote group
			if len(groups) >= 3 && groups[2] != "" {
				prefixLen += len(groups[2])
			}
			return match[:prefixLen] + replacement
		}
		return replacement
	})
}