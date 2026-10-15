package domainforwardproxy

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// PrivacySettings is a local middleware policy independent of event redaction.
// TLS interception and raw request logging are never implicitly enabled.
type PrivacySettings struct {
	Enabled         bool                   `json:"enabled"`
	Action          string                 `json:"action"`
	RestoreResponse bool                   `json:"restoreResponse"`
	MaxBodyBytes    int64                  `json:"maxBodyBytes"`
	Categories      []string               `json:"categories,omitempty"`
	Allowlist       []string               `json:"allowlist,omitempty"`
	ContextMode     string                 `json:"contextMode,omitempty"`
	JSONFieldRules  []PrivacyJSONFieldRule `json:"jsonFieldRules,omitempty"`
}

const defaultPrivacyMaxBytes = int64(1 << 20)
const privacyMaxBytes = int64(8 << 20)
const maxPrivacyFindings = 128

var (
	privacyEmail       = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	privacyPhone       = regexp.MustCompile(`\b\+?[0-9][0-9\-(). ]{8,}[0-9]\b`)
	privacyPaymentCard = regexp.MustCompile(`\b(?:[0-9][ -]?){12,18}[0-9]\b`)
	privacySecret      = regexp.MustCompile(`(?i)\b(?:sk-[A-Za-z0-9_\-]{16,}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|(?:api[_-]?key|access[_-]?token|password|secret)\s*[:=]\s*["']?[A-Za-z0-9_./+\-]{12,})`)
)

type PrivacyFinding struct {
	Category string `json:"category"`
}

type PrivacyReport struct {
	Findings []PrivacyFinding `json:"findings"`
	Action   string           `json:"action"`
	Blocked  bool             `json:"blocked"`
	Redacted bool             `json:"redacted"`
}

type privacyMatch struct {
	start, end int
	category   string
}

type PrivacyEngine struct {
	settings PrivacySettings
	patterns []struct {
		category string
		pattern  *regexp.Regexp
	}
	allowlist map[string]struct{}
	configurationError error
}

// PrivacySession contains plaintext only in process memory for one HTTP
// request. No global or disk-backed placeholder mapping is maintained.
type PrivacySession struct {
	replacements map[string]string
}

func NormalizePrivacySettings(s *PrivacySettings) {
	if s == nil {
		return
	}
	s.Action = strings.ToLower(strings.TrimSpace(s.Action))
	if s.Action != "warn" && s.Action != "block" && s.Action != "redact" {
		s.Action = "block" // Never silently allow a misspelled policy action.
	}
	if s.MaxBodyBytes <= 0 {
		s.MaxBodyBytes = defaultPrivacyMaxBytes
	}
	if s.MaxBodyBytes > privacyMaxBytes {
		s.MaxBodyBytes = privacyMaxBytes
	}
	if len(s.Categories) == 0 {
		s.Categories = []string{"secret", "email", "phone", "payment_card"}
	}
	categories := make([]string, 0, 3)
	seen := map[string]bool{}
	for _, category := range s.Categories {
		category = strings.ToLower(strings.TrimSpace(category))
		if (category == "secret" || category == "email" || category == "phone" || category == "payment_card") && !seen[category] {
			categories = append(categories, category)
			seen[category] = true
		}
	}
	if len(categories) == 0 {
		categories = []string{"secret", "email", "phone", "payment_card"}
	}
	s.Categories = categories
	NormalizeContextSettings(s)
	if len(s.Allowlist) > 100 {
		s.Allowlist = s.Allowlist[:100]
	}
}

func NewPrivacyEngine(settings PrivacySettings) *PrivacyEngine {
	NormalizePrivacySettings(&settings)
	engine := &PrivacyEngine{settings: settings, allowlist: make(map[string]struct{})}
	for _, rule := range settings.JSONFieldRules {
		if rule.Action == "__invalid__" || !validPrivacyJSONPointer(rule.Path) {
			engine.configurationError = errors.New("invalid privacy JSON disclosure rule")
			break
		}
	}
	for _, value := range settings.Allowlist {
		if value != "" && len(value) <= 512 {
			engine.allowlist[value] = struct{}{}
		}
	}
	for _, category := range settings.Categories {
		var pattern *regexp.Regexp
		switch category {
		case "secret":
			pattern = privacySecret
		case "email":
			pattern = privacyEmail
		case "phone":
			pattern = privacyPhone
		case "payment_card":
			pattern = privacyPaymentCard
		}
		if pattern != nil {
			engine.patterns = append(engine.patterns, struct {
				category string
				pattern  *regexp.Regexp
			}{category, pattern})
		}
	}
	return engine
}

// Apply inspects JSON string values (not keys) or plain text. Unsupported
// content types, malformed JSON and oversized payloads fail closed.
func (e *PrivacyEngine) Apply(contentType string, body []byte) ([]byte, *PrivacySession, PrivacyReport, error) {
	report := PrivacyReport{Action: e.settings.Action}
	if !e.settings.Enabled { return body, nil, report, nil }
	if e.configurationError != nil { return nil, nil, report, e.configurationError }
	if len(body) == 0 {
		return body, nil, report, nil
	}
	if int64(len(body)) > e.settings.MaxBodyBytes {
		return nil, nil, report, errors.New("privacy inspection body limit exceeded")
	}
	lower := strings.ToLower(contentType)
	isJSON := strings.Contains(lower, "json")
	isText := strings.HasPrefix(lower, "text/plain")
	if !isJSON && !isText {
		return nil, nil, report, errors.New("privacy inspection does not support content type")
	}
	if isJSON && !json.Valid(body) {
		return nil, nil, report, errors.New("invalid JSON for privacy inspection")
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, report, errors.New("privacy session initialization failed")
	}
	prefix := "[[AE_PRIV_" + hex.EncodeToString(nonce) + "_"
	session := &PrivacySession{replacements: make(map[string]string)}
	reverse := make(map[string]string)
	if isJSON {
		return e.applyContextualJSON(body, prefix, session, report)
	}
	count := 0
	transform := func(value string) (string, bool) {
		matches := e.detect(value)
		if len(matches) == 0 {
			return value, false
		}
		var builder strings.Builder
		start := 0
		for _, m := range matches {
			if m.start < start {
				continue
			}
			count++
			if count <= maxPrivacyFindings {
				report.Findings = append(report.Findings, PrivacyFinding{Category: m.category})
			}
			if count > maxPrivacyFindings {
				continue
			}
			if e.settings.Action != "redact" {
				continue
			}
			builder.WriteString(value[start:m.start])
			original := value[m.start:m.end]
			placeholder, exists := reverse[original]
			if !exists {
				placeholder = fmt.Sprintf("%s%d]]", prefix, len(reverse)+1)
				reverse[original] = placeholder
				session.replacements[placeholder] = original
			}
			builder.WriteString(placeholder)
			start = m.end
		}
		if e.settings.Action != "redact" {
			return value, false
		}
		builder.WriteString(value[start:])
		return builder.String(), true
	}

	var out []byte
	if isText {
		changed, _ := transform(string(body))
		out = []byte(changed)
	} else {
		out = make([]byte, 0, len(body)+64)
		offset := 0
		for offset < len(body) {
			start, end, isKey, ok := nextJSONString(body, offset)
			if !ok {
				break
			}
			out = append(out, body[offset:start]...)
			token := body[start:end]
			if !isKey {
				var value string
				if json.Unmarshal(token, &value) != nil {
					return nil, nil, report, errors.New("privacy JSON string decode failed")
				}
				rewritten, changed := transform(value)
				if changed {
					encoded, _ := json.Marshal(rewritten)
					token = encoded
				}
			}
			out = append(out, token...)
			offset = end
		}
		out = append(out, body[offset:]...)
	}
	if count > maxPrivacyFindings {
		return nil, nil, report, errors.New("privacy finding limit exceeded")
	}
	if len(report.Findings) == 0 {
		return body, nil, report, nil
	}
	if e.settings.Action == "block" {
		report.Blocked = true
		return nil, nil, report, nil
	}
	if e.settings.Action == "warn" {
		return body, nil, report, nil
	}
	report.Redacted = true
	return out, session, report, nil
}

func (e *PrivacyEngine) detect(value string) []privacyMatch {
	candidates := make([]privacyMatch, 0, 8)
	for _, item := range e.patterns {
		for _, offsets := range item.pattern.FindAllStringIndex(value, maxPrivacyFindings+1) {
			raw := value[offsets[0]:offsets[1]]
			if item.category == "payment_card" && !looksLikePaymentCard(raw) {
				continue
			}
			if item.category == "phone" {
				digits := 0
				for _, char := range raw {
					if char >= '0' && char <= '9' {
						digits++
					}
				}
				if digits < 10 || digits > 15 {
					continue
				}
				// Long unformatted digit runs are often IDs, epoch values or
				// telemetry counters. Require explicit phone punctuation.
				if digits > 11 && !strings.ContainsAny(raw, "+()- ") {
					continue
				}
			}
			if _, allowed := e.allowlist[raw]; allowed {
				continue
			}
			candidates = append(candidates, privacyMatch{offsets[0], offsets[1], item.category})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].start != candidates[j].start {
			return candidates[i].start < candidates[j].start
		}
		if candidates[i].end != candidates[j].end {
			return candidates[i].end > candidates[j].end
		}
		return candidates[i].category == "payment_card" && candidates[j].category != "payment_card"
	})
	kept := candidates[:0]
	end := 0
	for _, m := range candidates {
		if m.start < end {
			continue
		}
		kept = append(kept, m)
		end = m.end
	}
	return kept
}

func (s *PrivacySession) Restore(body []byte) ([]byte, bool) {
	if s == nil || len(s.replacements) == 0 || len(body) == 0 {
		return body, false
	}
	keys := make([]string, 0, len(s.replacements))
	for key := range s.replacements {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys)*2)
	for _, key := range keys {
		pairs = append(pairs, key, s.replacements[key])
	}
	restored := strings.NewReplacer(pairs...).Replace(string(body))
	return []byte(restored), restored != string(body)
}
