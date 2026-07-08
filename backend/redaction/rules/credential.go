package rules

import (
	"agent-ebpf-filter/redaction"
	"encoding/json"
	"net/url"
	"regexp"
	"strings"
)

var sensitiveKeyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:^|[_\-\s])password(?:$|[_\-\s])`),
	regexp.MustCompile(`(?i)(?:^|[_\-\s])token(?:$|[_\-\s])`),
	regexp.MustCompile(`(?i)(?:^|[_\-\s])api[_\-]?key(?:$|[_\-\s])`),
	regexp.MustCompile(`(?i)(?:^|[_\-\s])authorization(?:$|[_\-\s])`),
	regexp.MustCompile(`(?i)(?:^|[_\-\s])bearer(?:$|[_\-\s])`),
	regexp.MustCompile(`(?i)(?:^|[_\-\s])secret(?:$|[_\-\s])`),
}

var inlineCredentialPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]+`),
	regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret|password|secret|token)\s*[:=]\s*([^\s,;"']+)`),
}

// RedactCredentials redacts credential-like values in free-form text.
func RedactCredentials(text string, level redaction.RedactionLevel) string {
	if strings.TrimSpace(text) == "" || level == redaction.RedactionLevelNone {
		return text
	}

	redacted := text
	for _, re := range inlineCredentialPatterns {
		redacted = re.ReplaceAllStringFunc(redacted, func(match string) string {
			lower := strings.ToLower(match)
			switch {
			case strings.Contains(lower, "bearer "):
				if idx := strings.Index(lower, "bearer "); idx >= 0 {
					return match[:idx+7] + RedactedValue
				}
				return "Bearer " + RedactedValue
			default:
				sep := strings.IndexAny(match, "=:")
				if sep < 0 {
					return RedactedValue
				}
				return match[:sep+1] + " " + RedactedValue
			}
		})
	}
	return redacted
}

// sanitizeHeaders redacts sensitive header values.
func sanitizeHeaders(headers map[string]string, level redaction.RedactionLevel) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	if level == redaction.RedactionLevelNone {
		out := make(map[string]string, len(headers))
		for k, v := range headers {
			out[strings.ToLower(strings.TrimSpace(k))] = v
		}
		return out
	}

	out := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.ToLower(strings.TrimSpace(k))
		if key == "" {
			continue
		}
		if isSensitiveKey(key) {
			out[key] = RedactedValue
			continue
		}
		out[key] = RedactCredentials(v, level)
	}
	return out
}

// sanitizeJSON redacts sensitive fields within a JSON string while preserving shape.
func sanitizeJSON(jsonStr string, level redaction.RedactionLevel) string {
	if strings.TrimSpace(jsonStr) == "" || level == redaction.RedactionLevelNone {
		return jsonStr
	}

	var payload any
	if err := json.Unmarshal([]byte(jsonStr), &payload); err != nil {
		return RedactCredentials(jsonStr, level)
	}
	if redactJSONValue(&payload, level) {
		if out, err := json.MarshalIndent(payload, "", "  "); err == nil {
			return string(out)
		}
	}
	return RedactCredentials(jsonStr, level)
}

// isSensitiveKey reports whether a key name appears credential-related.
func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	if normalized == "" {
		return false
	}
	if normalized == "authorization" || normalized == "bearer" {
		return true
	}
	for _, re := range sensitiveKeyPatterns {
		if re.MatchString("_" + normalized + "_") {
			return true
		}
	}
	return strings.Contains(normalized, "password") || strings.Contains(normalized, "token") || strings.Contains(normalized, "api_key") || strings.Contains(normalized, "secret")
}

func redactJSONValue(value *any, level redaction.RedactionLevel) bool {
	switch typed := (*value).(type) {
	case map[string]any:
		changed := false
		for key, child := range typed {
			if isSensitiveKey(key) {
				typed[key] = RedactedValue
				changed = true
				continue
			}
			childValue := child
			if redactJSONValue(&childValue, level) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i, child := range typed {
			childValue := child
			if redactJSONValue(&childValue, level) {
				typed[i] = childValue
				changed = true
			}
		}
		return changed
	case string:
		redacted := RedactCredentials(typed, level)
		if redacted != typed {
			*value = redacted
			return true
		}
	}
	return false
}

func sanitizeURLValues(rawURL string, level redaction.RedactionLevel) string {
	if strings.TrimSpace(rawURL) == "" || level == redaction.RedactionLevelNone {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return RedactCredentials(rawURL, level)
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		if isSensitiveKey(key) {
			query.Set(key, RedactedValue)
			changed = true
		}
	}
	if changed {
		parsed.RawQuery = query.Encode()
		return parsed.String()
	}
	return RedactCredentials(rawURL, level)
}
