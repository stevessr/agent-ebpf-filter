package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

const maxBodySize = 16 * 1024

const RedactedValue = "***REDACTED***"

var sensitiveQueryKeys = map[string]struct{}{
	"access_token":  {},
	"api_key":       {},
	"apikey":        {},
	"auth":          {},
	"authorization": {},
	"bearer":        {},
	"client_secret": {},
	"key":           {},
	"password":      {},
	"secret":        {},
	"session":       {},
	"token":         {},
}

var bearerTokenPattern = regexp.MustCompile(`(?i)\b(bearer\s+)[A-Za-z0-9._~+/=-]+`)
var inlineSecretPattern = regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|auth[_-]?token|client[_-]?secret|password|secret|token)=([^\s&]+)`)

func sanitizeHeaders(headers http.Header) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	redacted := map[string]string{
		"authorization":       RedactedValue,
		"proxy-authorization": RedactedValue,
		"x-api-key":           RedactedValue,
		"cookie":              RedactedValue,
		"set-cookie":          RedactedValue,
	}
	out := make(map[string]string, len(headers))
	for key, values := range headers {
		lower := strings.ToLower(key)
		if replacement, ok := redacted[lower]; ok {
			out[lower] = replacement
			continue
		}
		out[lower] = strings.Join(values, ", ")
	}
	return out
}

func sanitizeURL(rawURL string) string {
	if strings.TrimSpace(rawURL) == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return sanitizeInlineSecrets(rawURL)
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		if isSensitiveKey(key) {
			query.Set(key, RedactedValue)
			changed = true
		}
	}
	if !changed {
		return sanitizeInlineSecrets(rawURL)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func sanitizeBody(body, contentType string) string {
	if strings.TrimSpace(body) == "" {
		return body
	}
	if looksLikeJSON(contentType, []byte(body)) {
		var payload any
		if err := json.Unmarshal([]byte(body), &payload); err == nil {
			if sanitizeJSONValue(&payload) {
				if redacted, err := json.MarshalIndent(payload, "", "  "); err == nil {
					return string(redacted)
				}
			}
		}
	}
	if strings.Contains(strings.ToLower(contentType), "x-www-form-urlencoded") {
		if values, err := url.ParseQuery(body); err == nil {
			changed := false
			for key := range values {
				if isSensitiveKey(key) {
					values.Set(key, RedactedValue)
					changed = true
				}
			}
			if changed {
				return values.Encode()
			}
		}
	}
	return sanitizeInlineSecrets(body)
}

func sanitizeJSONValue(value *any) bool {
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
			if sanitizeJSONValue(&childValue) {
				typed[key] = childValue
				changed = true
			}
		}
		return changed
	case []any:
		changed := false
		for i, child := range typed {
			childValue := child
			if sanitizeJSONValue(&childValue) {
				typed[i] = childValue
				changed = true
			}
		}
		return changed
	case string:
		redacted := sanitizeInlineSecrets(typed)
		if redacted != typed {
			*value = redacted
			return true
		}
	}
	return false
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if _, ok := sensitiveQueryKeys[normalized]; ok {
		return true
	}
	return strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "api_key")
}

func sanitizeInlineSecrets(value string) string {
	encodedRedactedValue := url.QueryEscape(RedactedValue)
	placeholder := "__CODEX_CAPTURE_REDACTED_VALUE__"
	protected := strings.ReplaceAll(value, encodedRedactedValue, placeholder)
	redacted := bearerTokenPattern.ReplaceAllString(protected, `${1}`+RedactedValue)
	redacted = inlineSecretPattern.ReplaceAllString(redacted, `${1}=`+RedactedValue)
	redacted = strings.ReplaceAll(redacted, placeholder, encodedRedactedValue)
	return redacted
}

func annotateSSEEvent(event *Event) {
	if event == nil || !strings.Contains(strings.ToLower(event.ContentType), "text/event-stream") {
		return
	}
	event.Type = "sse_message"
	var dataParts []string
	for _, line := range strings.Split(event.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "event:") && event.SSEEvent == "" {
			event.SSEEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		}
		if strings.HasPrefix(trimmed, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(trimmed, "data:"))
			if data != "" {
				dataParts = append(dataParts, data)
			}
		}
	}
	if len(dataParts) > 0 {
		event.SSEDataCount = len(dataParts)
		sum := sha256.Sum256([]byte(strings.Join(dataParts, "\n")))
		event.SSEDataDigest = "sha256:" + hex.EncodeToString(sum[:8])
	}
}

func formatBody(body []byte, contentType string) (string, bool) {
	if len(body) == 0 {
		return "", false
	}

	formatted := body
	if looksLikeJSON(contentType, body) {
		if pretty, err := prettyPrintJSON(body); err == nil {
			formatted = pretty
		}
	}

	truncated := len(body) > maxBodySize || len(formatted) > maxBodySize
	if len(formatted) > maxBodySize {
		formatted = formatted[:maxBodySize]
	}
	return string(formatted), truncated
}

func prettyPrintJSON(body []byte) ([]byte, error) {
	var out bytes.Buffer
	if err := json.Indent(&out, body, "", "  "); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func looksLikeJSON(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "json") {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}
