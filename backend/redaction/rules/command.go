package rules

import (
	"agent-ebpf-filter/redaction"
	"regexp"
	"strings"
)

var (
	commandSensitiveNameRegexps = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(?:^|[\s'"` + "`" + `])(?:--?(?:password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key|auth[_-]?token|client[_-]?secret|client[_-]?id|refresh[_-]?token|bearer|cookie|authorization))(?:=|\s+|$)`),
		regexp.MustCompile(`(?i)\b(?:password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key|auth[_-]?token|client[_-]?secret|client[_-]?id|refresh[_-]?token|bearer|cookie|authorization)\b`),
	}

	commandSensitiveValueRegexps = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key|authorization|cookie)[\s:=]+[^\s"'` + "`" + `]+`),
		regexp.MustCompile(`(?i)\b(--?(?:password|passwd|pwd|token|secret|apikey|api-key|access[_-]?key))(?:[=\s]+)[^\s"'` + "`" + `]+`),
		regexp.MustCompile(`(?i)\b(https?://)[^/\s:@]+(?::[^/\s@]+)?@`),
		regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}\b`),
		regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),
		regexp.MustCompile(`(?i)\bAuthorization[\s:=]+(?:Bearer\s+|Basic\s+)?[^\s"'` + "`" + `]+(?:\s+[^\s"'` + "`" + `]+)?`),
		regexp.MustCompile(`(?i)\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]+`),
	}
)

func RedactCommandLine(cmdline string, level redaction.RedactionLevel) string {
	if cmdline == "" || level == redaction.RedactionLevelNone {
		return cmdline
	}
	parts := strings.Fields(cmdline)
	return strings.Join(RedactArgs(parts, level), " ")
}

func RedactArgs(args []string, level redaction.RedactionLevel) []string {
	if len(args) == 0 || level == redaction.RedactionLevelNone {
		return args
	}

	redacted := make([]string, 0, len(args))
	redactedCommand := false
	for i, arg := range args {
		switch level {
		case redaction.RedactionLevelStrict:
			if i == 0 {
				redacted = append(redacted, arg)
			} else {
				redacted = append(redacted, "***")
			}
		case redaction.RedactionLevelStandard:
			if i == 0 {
				redacted = append(redacted, arg)
				continue
			}
			if isSensitiveArg(arg) || looksLikeCredentialValue(arg) {
				redacted = append(redacted, "***")
				redactedCommand = true
				continue
			}
			redacted = append(redacted, redactCommandArgText(arg))
		case redaction.RedactionLevelBasic:
			if i == 0 {
				redacted = append(redacted, arg)
				continue
			}
			if isSensitiveArg(arg) {
				redacted = append(redacted, "***")
				redactedCommand = true
				continue
			}
			redacted = append(redacted, redactBasicCommandArg(arg))
		default:
			if i == 0 {
				redacted = append(redacted, arg)
			} else {
				redacted = append(redacted, arg)
			}
		}
	}

	if level == redaction.RedactionLevelStrict {
		return redacted[:min(len(redacted), 1)]
	}
	if redactedCommand && len(redacted) > 1 {
		for i := 1; i < len(redacted); i++ {
			if redacted[i] == "" {
				redacted[i] = "***"
			}
		}
	}
	return redacted
}

func isSensitiveArg(arg string) bool {
	if arg == "" {
		return false
	}
	lower := strings.ToLower(arg)
	for _, re := range commandSensitiveNameRegexps {
		if re.MatchString(lower) {
			return true
		}
	}
	return strings.Contains(lower, "password") || strings.Contains(lower, "passwd") || strings.Contains(lower, "pwd") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "apikey") || strings.Contains(lower, "api-key") || strings.Contains(lower, "authorization") || strings.Contains(lower, "cookie")
}

func redactBasicCommandArg(arg string) string {
	if isSensitiveArg(arg) {
		return "***"
	}
	return redactCommandArgText(arg)
}

func redactCommandArgText(text string) string {
	if text == "" {
		return text
	}
	out := text
	for _, re := range commandSensitiveValueRegexps {
		out = re.ReplaceAllStringFunc(out, func(match string) string {
			lower := strings.ToLower(match)
			switch {
			case strings.Contains(lower, "http://") || strings.Contains(lower, "https://"):
				if idx := strings.Index(match, "://"); idx >= 0 {
					return match[:idx+3] + "***@"
				}
				return "***"
			case strings.Contains(lower, "@") && !strings.Contains(lower, "authorization"):
				return "***@***"
			case strings.Contains(lower, "akia"):
				return "AKIA****************"
			case strings.Contains(lower, "password"), strings.Contains(lower, "passwd"), strings.Contains(lower, "pwd"),
				strings.Contains(lower, "token"), strings.Contains(lower, "secret"), strings.Contains(lower, "apikey"),
				strings.Contains(lower, "api-key"), strings.Contains(lower, "authorization"), strings.Contains(lower, "cookie"):
				if strings.Contains(lower, "bearer ") || strings.Contains(lower, "basic ") {
					return "Authorization: ***"
				}
				if idx := strings.IndexAny(match, " =:"); idx >= 0 {
					return match[:idx+1] + "***"
				}
				return "***"
			default:
				return "***"
			}
		})
	}
	return out
}

func looksLikeCredentialValue(arg string) bool {
	if arg == "" {
		return false
	}
	lower := strings.ToLower(arg)
	return strings.Contains(lower, "bearer ") || strings.Contains(lower, "basic ") || strings.Contains(lower, "authorization:") || strings.Contains(arg, "=") && (strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password"))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
