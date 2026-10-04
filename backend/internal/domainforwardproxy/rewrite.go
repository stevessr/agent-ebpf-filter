package domainforwardproxy

import (
	"bytes"
	"encoding/json"
	"mime"
	"strings"
)

const (
	defaultRewriteBodyBytes = int64(4 << 20)
	maxRewriteBodyBytes     = int64(64 << 20)
)

// BodyRewriteRule is a bounded literal rewrite rule applied to textual payloads.
// Direction accepts request, response or both. Host supports exact and *.suffix
// matching; empty Host applies to every configured forwarding route.
type BodyRewriteRule struct {
	ID          string `json:"id,omitempty"`
	Enabled     bool   `json:"enabled"`
	Direction   string `json:"direction"`
	Host        string `json:"host,omitempty"`
	PathPrefix  string `json:"pathPrefix,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	Find        string `json:"find"`
	Replace     string `json:"replace"`
}

// ModelRewriteRule redirects the top-level Responses/Chat model field without
// decoding the entire request. Host is optional and supports *.suffix.
type ModelRewriteRule struct {
	Host string `json:"host,omitempty"`
	From string `json:"from"`
	To   string `json:"to"`
}

type BodyRewriteSettings struct {
	Enabled      bool               `json:"enabled"`
	MaxBodyBytes int64              `json:"maxBodyBytes"`
	Rules        []BodyRewriteRule  `json:"rules,omitempty"`
	ModelRules   []ModelRewriteRule `json:"modelRules,omitempty"`
}

type modelRewrite struct {
	Client   string
	Upstream string
}

type compiledModelRewrite struct {
	host string
	from string
	to   string
}

// RewriteKernel is the in-process inference/rewrite hot path. It deliberately
// has a cheap byte scan before JSON work so non-candidate streaming frames pass
// through without allocating a generic object graph.
type RewriteKernel struct {
	enabled      bool
	maxBodyBytes int64
	modelRules   []compiledModelRewrite
	bodyRules    []BodyRewriteRule
}

func NewRewriteKernel(settings BodyRewriteSettings) *RewriteKernel {
	maxBytes := settings.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = defaultRewriteBodyBytes
	}
	if maxBytes > maxRewriteBodyBytes {
		maxBytes = maxRewriteBodyBytes
	}
	kernel := &RewriteKernel{
		enabled:      settings.Enabled,
		maxBodyBytes: maxBytes,
	}
	for _, rule := range settings.ModelRules {
		from := strings.TrimSpace(rule.From)
		to := strings.TrimSpace(rule.To)
		if from == "" || to == "" || from == to {
			continue
		}
		kernel.modelRules = append(kernel.modelRules, compiledModelRewrite{
			host: NormalizeDomainPattern(rule.Host),
			from: from,
			to:   to,
		})
	}
	for _, rule := range settings.Rules {
		rule.Direction = normalizeRewriteDirection(rule.Direction)
		rule.Host = NormalizeDomainPattern(rule.Host)
		rule.PathPrefix = strings.TrimSpace(rule.PathPrefix)
		rule.ContentType = strings.ToLower(strings.TrimSpace(rule.ContentType))
		if !rule.Enabled || rule.Find == "" {
			continue
		}
		kernel.bodyRules = append(kernel.bodyRules, rule)
	}
	return kernel
}

func (k *RewriteKernel) MaxBodyBytes() int64 {
	if k == nil || k.maxBodyBytes <= 0 {
		return defaultRewriteBodyBytes
	}
	return k.maxBodyBytes
}

func (k *RewriteKernel) RewriteRequest(host, path, contentType string, body []byte) ([]byte, *modelRewrite, bool) {
	if k == nil || !k.enabled || len(body) == 0 {
		return body, nil, false
	}
	changed := false
	var mapping *modelRewrite
	out := body
	if bytes.Contains(out, []byte(`"model"`)) {
		for _, rule := range k.modelRules {
			if !rewriteHostMatches(rule.host, host) {
				continue
			}
			rewritten, ok := rewriteTopLevelJSONStringField(out, "model", rule.from, rule.to)
			if ok {
				out = rewritten
				mapping = &modelRewrite{Client: rule.from, Upstream: rule.to}
				changed = true
				break
			}
		}
	}
	if len(k.bodyRules) > 0 && isTextualPayload(contentType, out) {
		if rewritten, ok := k.applyLiteralRules("request", host, path, contentType, out); ok {
			out = rewritten
			changed = true
		}
	}
	return out, mapping, changed
}

func (k *RewriteKernel) RewriteResponse(host, path, contentType string, body []byte, mapping *modelRewrite) ([]byte, bool) {
	if k == nil || !k.enabled || len(body) == 0 {
		return body, false
	}
	changed := false
	out := body
	if mapping != nil && mapping.Client != "" && mapping.Upstream != "" && bytes.Contains(out, []byte(`"model"`)) {
		if rewritten, ok := rewriteResponsesModel(out, mapping.Upstream, mapping.Client); ok {
			out = rewritten
			changed = true
		}
	}
	if len(k.bodyRules) > 0 && isTextualPayload(contentType, out) {
		if rewritten, ok := k.applyLiteralRules("response", host, path, contentType, out); ok {
			out = rewritten
			changed = true
		}
	}
	return out, changed
}

func (k *RewriteKernel) applyLiteralRules(direction, host, path, contentType string, body []byte) ([]byte, bool) {
	out := body
	changed := false
	for _, rule := range k.bodyRules {
		if rule.Direction != "both" && rule.Direction != direction {
			continue
		}
		if !rewriteHostMatches(rule.Host, host) {
			continue
		}
		if rule.PathPrefix != "" && !strings.HasPrefix(path, rule.PathPrefix) {
			continue
		}
		if rule.ContentType != "" && !strings.Contains(strings.ToLower(contentType), rule.ContentType) {
			continue
		}
		needle := []byte(rule.Find)
		if !bytes.Contains(out, needle) {
			continue
		}
		out = bytes.ReplaceAll(out, needle, []byte(rule.Replace))
		changed = true
	}
	return out, changed
}

func normalizeRewriteDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "request", "response":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "both"
	}
}

func rewriteHostMatches(pattern, host string) bool {
	if pattern == "" {
		return true
	}
	host = NormalizeForwardHost(host)
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return host == pattern
}

func isTextualPayload(contentType string, body []byte) bool {
	mediaType, _, _ := mime.ParseMediaType(contentType)
	mediaType = strings.ToLower(mediaType)
	if strings.HasPrefix(mediaType, "text/") ||
		strings.Contains(mediaType, "json") ||
		strings.Contains(mediaType, "xml") ||
		mediaType == "application/x-www-form-urlencoded" {
		return true
	}
	trimmed := bytes.TrimSpace(body)
	return len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')
}

// rewriteTopLevelJSONStringField finds a JSON string field at object depth 1.
// This avoids unmarshalling large prompt/tool payloads on the common model-map
// hot path while refusing to touch nested user/tool data with the same key.
func rewriteTopLevelJSONStringField(body []byte, field, from, to string) ([]byte, bool) {
	keyStart, valueStart, valueEnd, ok := topLevelJSONStringField(body, field)
	if !ok {
		return body, false
	}
	_ = keyStart
	var current string
	if err := json.Unmarshal(body[valueStart:valueEnd], &current); err != nil || current != from {
		return body, false
	}
	replacement, _ := json.Marshal(to)
	out := make([]byte, 0, len(body)-((valueEnd-valueStart)-len(replacement)))
	out = append(out, body[:valueStart]...)
	out = append(out, replacement...)
	out = append(out, body[valueEnd:]...)
	return out, true
}

func topLevelJSONStringField(body []byte, field string) (keyStart, valueStart, valueEnd int, ok bool) {
	depth := 0
	inString := false
	escaped := false
	expectingKey := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c != '"' {
				continue
			}
			inString = false
			if depth != 1 || !expectingKey {
				continue
			}
			rawKey := body[keyStart : i+1]
			var key string
			if json.Unmarshal(rawKey, &key) != nil || key != field {
				expectingKey = false
				continue
			}
			j := i + 1
			for j < len(body) && isJSONSpace(body[j]) {
				j++
			}
			if j >= len(body) || body[j] != ':' {
				return 0, 0, 0, false
			}
			j++
			for j < len(body) && isJSONSpace(body[j]) {
				j++
			}
			if j >= len(body) || body[j] != '"' {
				return 0, 0, 0, false
			}
			end := scanJSONStringEnd(body, j)
			if end <= j {
				return 0, 0, 0, false
			}
			return keyStart, j, end, true
		}
		switch c {
		case '{', '[':
			depth++
			expectingKey = depth == 1 && c == '{'
		case '}', ']':
			depth--
			expectingKey = false
		case ',':
			expectingKey = depth == 1
		case '"':
			inString = true
			escaped = false
			keyStart = i
		default:
			if depth == 1 && !isJSONSpace(c) && c != ':' {
				expectingKey = false
			}
		}
	}
	return 0, 0, 0, false
}

func scanJSONStringEnd(body []byte, start int) int {
	escaped := false
	for i := start + 1; i < len(body); i++ {
		if escaped {
			escaped = false
			continue
		}
		switch body[i] {
		case '\\':
			escaped = true
		case '"':
			return i + 1
		}
	}
	return -1
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\n' || c == '\r' || c == '\t'
}

// rewriteResponsesModel handles both ordinary response objects and streaming
// Responses events where the response object is nested under "response".
func rewriteResponsesModel(body []byte, from, to string) ([]byte, bool) {
	if rewritten, ok := rewriteTopLevelJSONStringField(body, "model", from, to); ok {
		return rewritten, true
	}
	var envelope map[string]json.RawMessage
	if json.Unmarshal(body, &envelope) != nil {
		return body, false
	}
	raw := envelope["response"]
	if len(raw) == 0 || !bytes.Contains(raw, []byte(`"model"`)) {
		return body, false
	}
	rewritten, ok := rewriteTopLevelJSONStringField(raw, "model", from, to)
	if !ok {
		return body, false
	}
	envelope["response"] = rewritten
	out, err := json.Marshal(envelope)
	if err != nil {
		return body, false
	}
	return out, true
}
