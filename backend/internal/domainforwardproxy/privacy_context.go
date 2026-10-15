package domainforwardproxy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// PrivacyJSONFieldRule defines an exact RFC 6901 JSON Pointer policy. It is
// configured by the local operator, not by incoming untrusted tool metadata.
// allow explicitly declassifies this one scalar; redact forces minimization,
// drop removes this map property, and block denies the entire request.
type PrivacyJSONFieldRule struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

const (
	maxContextJSONDepth = 64
	maxContextJSONNodes = 16384
	maxContextJSONRules = 64
)

// Keys are normalized using ASCII alphanumeric folding: task_history,
// Task-History and taskHistory all resolve to the same information-flow label.
// Generic "context", "history" or "messages" are deliberately NOT denied.
var privacyMemoryKeys = map[string]bool{
	"taskhistory":            true,
	"conversationhistory":    true,
	"chathistory":            true,
	"sessionmemory":          true,
	"agentmemory":            true,
	"fullcontext":            true,
	"memorydump":             true,
	"fullconversation":       true,
	"conversationtranscript": true,
	"toolcallhistory":        true,
	"toolresultshistory":     true,
	"systemprompt":           true,
	"developermessage":       true,
}
var privacySensitiveKeys = map[string]bool{
	"password":             true,
	"passwd":               true,
	"passphrase":           true,
	"apikey":               true,
	"apiaccesskey":         true,
	"authtoken":            true,
	"accesstoken":          true,
	"refreshtoken":         true,
	"clientsecret":         true,
	"privatekey":           true,
	"secretkey":            true,
	"ssn":                  true,
	"socialsecuritynumber": true,
	"bankaccount":          true,
	"creditcard":           true,
	"sessioncookie":        true,
}

func NormalizeContextSettings(settings *PrivacySettings) {
	mode := strings.ToLower(strings.TrimSpace(settings.ContextMode))
	if mode != "off" && mode != "strict" {
		mode = "strict"
	}
	settings.ContextMode = mode
	if len(settings.JSONFieldRules) > maxContextJSONRules {
		settings.JSONFieldRules = settings.JSONFieldRules[:maxContextJSONRules]
	}
	rules := make([]PrivacyJSONFieldRule, 0, len(settings.JSONFieldRules))
	seen := make(map[string]bool)
	for _, rule := range settings.JSONFieldRules {
		rule.Path = strings.TrimSpace(rule.Path)
		rule.Action = strings.ToLower(strings.TrimSpace(rule.Action))
		if !validPrivacyJSONPointer(rule.Path) || seen[rule.Path] {
			continue
		}
		if rule.Action != "allow" && rule.Action != "redact" &&
			rule.Action != "block" && rule.Action != "drop" {
			rule.Action = "block"
		}
		seen[rule.Path] = true
		rules = append(rules, rule)
	}
	settings.JSONFieldRules = rules
}

func validPrivacyJSONPointer(path string) bool {
	if len(path) < 2 || len(path) > 512 || path[0] != '/' {
		return false
	}
	for i := 0; i < len(path); i++ {
		if path[i] == '~' {
			if i+1 >= len(path) || (path[i+1] != '0' && path[i+1] != '1') {
				return false
			}
			i++
		}
	}
	return true
}

func privacyPointerPart(part string) string {
	part = strings.ReplaceAll(part, "~", "~0")
	part = strings.ReplaceAll(part, "/", "~1")
	return part
}

func privacyFoldKey(key string) string {
	var out strings.Builder
	for _, char := range key {
		switch {
		case char >= 'A' && char <= 'Z':
			out.WriteByte(byte(char - 'A' + 'a'))
		case char >= 'a' && char <= 'z', char >= '0' && char <= '9':
			out.WriteByte(byte(char))
		}
	}
	return out.String()
}

type contextualJSONState struct {
	engine   *PrivacyEngine
	prefix   string
	session  *PrivacySession
	reverse  map[string]string
	report   PrivacyReport
	count    int
	nodes    int
	explicit map[string]string
}

func (s *contextualJSONState) finding(category string) error {
	s.count++
	if s.count > maxPrivacyFindings {
		return errors.New("privacy finding limit exceeded")
	}
	s.report.Findings = append(s.report.Findings, PrivacyFinding{Category: category})
	return nil
}

func (s *contextualJSONState) placeholder(raw string) string {
	if existing, ok := s.reverse[raw]; ok {
		return existing
	}
	value := fmt.Sprintf("%s%d]]", s.prefix, len(s.reverse)+1)
	s.reverse[raw] = value
	s.session.replacements[value] = raw
	return value
}

func (s *contextualJSONState) transformText(value string, force bool) (string, bool, error) {
	if value == "" {
		return value, false, nil
	}
	matches := s.engine.detect(value)
	if force {
		matches = []privacyMatch{{start: 0, end: len(value), category: "sensitive_field"}}
	}
	if len(matches) == 0 {
		return value, false, nil
	}
	var builder strings.Builder
	offset := 0
	for _, match := range matches {
		if match.start < offset {
			continue
		}
		if err := s.finding(match.category); err != nil {
			return "", false, err
		}
		if s.engine.settings.Action == "block" {
			s.report.Blocked = true
			return "", false, nil
		}
		if s.engine.settings.Action != "redact" {
			continue
		}
		builder.WriteString(value[offset:match.start])
		builder.WriteString(s.placeholder(value[match.start:match.end]))
		offset = match.end
	}
	if s.engine.settings.Action != "redact" {
		return value, false, nil
	}
	builder.WriteString(value[offset:])
	return builder.String(), true, nil
}

// walk enforces a bounded trusted-local policy before serialization. "force"
// marks values under sensitive property keys, preserving the provenance label
// across nested map/array levels rather than searching only standalone PII.
func (s *contextualJSONState) walk(node any, path string, force bool, depth int) (any, bool, error) {
	s.nodes++
	if depth > maxContextJSONDepth || s.nodes > maxContextJSONNodes {
		return nil, false, errors.New("privacy JSON nesting/node limit exceeded")
	}
	if rule := s.explicit[path]; rule != "" {
		switch rule {
		case "allow":
			// Only scalar-level declassification; nested children keep their
			// own classification and cannot be blanket-whitelisted.
			switch node.(type) {
			case string, json.Number, bool, nil:
				return node, false, nil
			}
		case "block":
			if err := s.finding("field_policy"); err != nil {
				return nil, false, err
			}
			s.report.Blocked = true
			return nil, false, nil
		case "redact":
			// A stronger field rule must never be silently downgraded by the
			// global warn mode. Deny when the active action cannot redact.
			if s.engine.settings.Action != "redact" {
				if err := s.finding("field_policy"); err != nil {
					return nil, false, err
				}
				s.report.Blocked = true
				return nil, false, nil
			}
			force = true
		case "drop":
			// Parent map removes this property instead of entering walk.
			return nil, false, errors.New("drop policy must target an object property")
		}
	}
	switch value := node.(type) {
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(value))
		changed := false
		for _, key := range keys {
			childPath := path + "/" + privacyPointerPart(key)
			action := s.explicit[childPath]
			if action == "drop" {
				if err := s.finding("field_drop"); err != nil {
					return nil, false, err
				}
				changed = true
				continue
			}
			folded := privacyFoldKey(key)
			if action != "allow" && s.engine.settings.ContextMode == "strict" && privacyMemoryKeys[folded] {
				if err := s.finding("memory_exfiltration"); err != nil {
					return nil, false, err
				}
				s.report.Blocked = true
				return nil, false, nil
			}
			sensitive := force
			if action != "allow" && s.engine.settings.ContextMode == "strict" && privacySensitiveKeys[folded] {
				sensitive = true
			}
			next, childChanged, err := s.walk(value[key], childPath, sensitive, depth+1)
			if err != nil {
				return nil, false, err
			}
			if s.report.Blocked {
				return nil, false, nil
			}
			out[key] = next
			changed = changed || childChanged
		}
		return out, changed, nil
	case []any:
		out := make([]any, len(value))
		changed := false
		for index, item := range value {
			next, childChanged, err := s.walk(item, fmt.Sprintf("%s/%d", path, index), force, depth+1)
			if err != nil {
				return nil, false, err
			}
			if s.report.Blocked {
				return nil, false, nil
			}
			out[index] = next
			changed = changed || childChanged
		}
		return out, changed, nil
	case string:
		return s.transformText(value, force)
	case json.Number:
		// Never replace a numeric JSON token with a quoted string: it silently
		// changes the application's schema. Deny sensitive numeric values.
		if force || len(s.engine.detect(string(value))) > 0 {
			if err := s.finding("numeric_sensitive"); err != nil {
				return nil, false, err
			}
			if s.engine.settings.Action != "warn" {
				s.report.Blocked = true
			}
		}
		return value, false, nil
	default:
		return node, false, nil
	}
}

func (e *PrivacyEngine) applyContextualJSON(
	body []byte, prefix string, session *PrivacySession, report PrivacyReport,
) ([]byte, *PrivacySession, PrivacyReport, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	nodeCount := 0
	doc, err := parseStrictPrivacyJSON(decoder, 0, &nodeCount)
	if err != nil {
		return nil, nil, report, errors.New("privacy JSON parse/duplicate-field validation failed")
	}
	// The caller checks json.Valid; this also rejects trailing JSON values.
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, nil, report, errors.New("privacy JSON has trailing data")
	}
	state := &contextualJSONState{
		engine:   e,
		prefix:   prefix,
		session:  session,
		reverse:  make(map[string]string),
		report:   report,
		explicit: make(map[string]string, len(e.settings.JSONFieldRules)),
	}
	for _, rule := range e.settings.JSONFieldRules {
		state.explicit[rule.Path] = rule.Action
	}
	rewritten, changed, err := state.walk(doc, "", false, 0)
	if err != nil {
		return nil, nil, state.report, err
	}
	if state.report.Blocked {
		return nil, nil, state.report, nil
	}
	if !changed {
		return body, nil, state.report, nil
	}
	output, err := json.Marshal(rewritten)
	if err != nil {
		return nil, nil, state.report, errors.New("privacy JSON serialization failed")
	}
	state.report.Redacted = true
	return output, session, state.report, nil
}


// parseStrictPrivacyJSON rejects duplicate object members before classification.
// Otherwise a last-key-wins map can hide an earlier task_history/secret value
// while a different upstream JSON parser may consume that earlier value.
func parseStrictPrivacyJSON(decoder *json.Decoder, depth int, visited *int) (any, error) {
	(*visited)++
	if depth > maxContextJSONDepth || *visited > maxContextJSONNodes {
		return nil, errors.New("privacy JSON depth/node limit exceeded")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := token.(json.Delim)
	if !ok { return token, nil }
	switch delim {
	case '{':
		object := make(map[string]any)
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil { return nil, err }
			key, ok := keyToken.(string)
			if !ok { return nil, errors.New("invalid privacy JSON object key") }
			if seen[key] { return nil, errors.New("duplicate privacy JSON member") }
			seen[key] = true
			next, err := parseStrictPrivacyJSON(decoder, depth+1, visited)
			if err != nil { return nil, err }
			object[key] = next
		}
		_, err := decoder.Token()
		return object, err
	case '[':
		list := make([]any, 0)
		for decoder.More() {
			next, err := parseStrictPrivacyJSON(decoder, depth+1, visited)
			if err != nil { return nil, err }
			list = append(list, next)
		}
		_, err := decoder.Token()
		return list, err
	default:
		return nil, errors.New("unexpected privacy JSON delimiter")
	}
}
