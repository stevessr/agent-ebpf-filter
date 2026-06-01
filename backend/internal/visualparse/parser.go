package visualparse

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var visualLLMAllowedTriggers = map[string]string{
	"process":             "process",
	"bprm":                "process",
	"bprm_check":          "process",
	"bprm_check_security": "process",
	"file_open":           "file_open",
	"open":                "file_open",
	"read":                "file_open",
	"mkdir":               "mkdir",
	"inode_mkdir":         "mkdir",
	"file_create":         "file_create",
	"create":              "file_create",
	"inode_create":        "file_create",
	"rmdir":               "rmdir",
	"inode_rmdir":         "rmdir",
	"symlink":             "symlink",
	"inode_symlink":       "symlink",
	"unlink":              "unlink",
	"delete":              "unlink",
	"inode_unlink":        "unlink",
	"socket":              "socket_connect",
	"connect":             "socket_connect",
	"socket_connect":      "socket_connect",
	"inode_mknod":         "inode_mknod",
	"mknod":               "inode_mknod",
	"file_mprotect":       "file_mprotect",
	"mprotect":            "file_mprotect",
	"rwx":                 "file_mprotect",
	"inode_rename":        "inode_rename",
	"rename":              "inode_rename",
}

var visualLLMAllowedConditionFields = map[string]string{
	"comm":     "comm",
	"command":  "comm",
	"process":  "comm",
	"pid":      "pid",
	"uid":      "uid",
	"user":     "uid",
	"gid":      "gid",
	"group":    "gid",
	"basename": "basename",
	"file":     "basename",
	"filename": "basename",
	"name":     "basename",
	"port":     "port",
	"ipv4":     "ipv4",
	"ip":       "ipv4",
	"address":  "ipv4",
}

type visualBlocksLLMCompileRequest struct {
	Prompt  string         `json:"prompt"`
	Current map[string]any `json:"current,omitempty"`
}

type LogicNode struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Field    string      `json:"field,omitempty"`
	Operator string      `json:"operator,omitempty"`
	Value    string      `json:"value,omitempty"`
	Children []LogicNode `json:"children,omitempty"`
}

type CompileResponse struct {
	Trigger    string    `json:"trigger"`
	Action     string    `json:"action"`
	Conditions LogicNode `json:"conditions"`
	MapMode    string    `json:"mapMode"`
	MapKey     string    `json:"mapKey"`
	MapLimit   int       `json:"mapLimit"`
	Reasoning  string    `json:"reasoning,omitempty"`
	Warnings   []string  `json:"warnings,omitempty"`
	Model      string    `json:"model,omitempty"`
	RawContent string    `json:"rawContent,omitempty"`
}

func extractJSONObject(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return ""
	}
	return content[start : end+1]
}

func extractLLMString(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := raw[key]; ok {
			return strings.TrimSpace(valueToVisualString(value))
		}
	}
	return ""
}

func extractLLMFloat(raw map[string]any, keys ...string) float64 {
	for _, key := range keys {
		switch value := raw[key].(type) {
		case float64:
			return value
		case json.Number:
			f, _ := value.Float64()
			return f
		case string:
			var f float64
			_, _ = fmt.Sscanf(value, "%f", &f)
			return f
		}
	}
	return 0
}

func extractLLMStrings(raw map[string]any, keys ...string) []string {
	for _, key := range keys {
		value, ok := raw[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case []any:
			out := make([]string, 0, len(v))
			for _, item := range v {
				out = append(out, valueToVisualString(item))
			}
			return out
		case []string:
			return append([]string(nil), v...)
		case string:
			return []string{v}
		}
	}
	return nil
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func ParseContent(content string) (*CompileResponse, error) {
	jsonPayload := extractJSONObject(content)
	if jsonPayload == "" {
		return nil, errors.New("LLM response did not contain JSON")
	}

	var raw map[string]any
	if err := json.Unmarshal([]byte(jsonPayload), &raw); err != nil {
		return nil, err
	}

	trigger := normalizeVisualLLMTrigger(extractLLMString(raw, "trigger", "hook", "entry"))
	action := normalizeVisualLLMAction(extractLLMString(raw, "action", "response", "decision"))
	mapMode, mapKey, mapLimit := extractVisualLLMMap(raw)
	warnings := extractLLMStrings(raw, "warnings", "warning", "notes")

	conditionsRaw, ok := raw["conditions"]
	if !ok {
		conditionsRaw = firstPresent(raw, "conditionTree", "logic", "rules")
	}
	conditionCount := 0
	conditions, err := normalizeVisualLLMLogicNode(conditionsRaw, true, 0, &conditionCount)
	if err != nil {
		return nil, err
	}
	if conditionCount == 0 {
		conditions = defaultVisualLLMConditions()
		conditionCount = 1
	}
	if conditionCount > 8 {
		return nil, fmt.Errorf("LLM returned %d conditions; maximum is 8", conditionCount)
	}

	if trigger != "socket_connect" && visualLogicTreeUsesField(conditions, "port", "ipv4") {
		return nil, errors.New("LLM returned port/ipv4 conditions for a non-socket trigger")
	}
	if trigger == "unlink" && action == "BLOCK" {
		action = "ALERT"
		warnings = append(warnings, "unlink 走 kprobe/do_unlinkat，不能直接返回 EACCES，已将 BLOCK 调整为 ALERT")
	}
	if trigger == "unlink" && visualLogicTreeUsesField(conditions, "basename") {
		return nil, errors.New("LLM returned basename condition for unlink, but unlink kprobe has no safe basename context in this UI")
	}

	return &CompileResponse{
		Trigger:    trigger,
		Action:     action,
		Conditions: conditions,
		MapMode:    mapMode,
		MapKey:     mapKey,
		MapLimit:   mapLimit,
		Reasoning:  strings.TrimSpace(extractLLMString(raw, "reasoning", "explanation", "analysis")),
		Warnings:   compactStrings(warnings, 6),
	}, nil
}

func firstPresent(raw map[string]any, keys ...string) any {
	for _, key := range keys {
		if val, ok := raw[key]; ok {
			return val
		}
	}
	return nil
}

func extractVisualLLMMap(raw map[string]any) (string, string, int) {
	mapMode := extractLLMString(raw, "mapMode", "map_mode")
	mapKey := extractLLMString(raw, "mapKey", "map_key")
	mapLimit := int(extractLLMFloat(raw, "mapLimit", "map_limit", "limit"))
	if nested, ok := raw["map"].(map[string]any); ok {
		if mapMode == "" {
			mapMode = extractLLMString(nested, "mode", "mapMode", "map_mode")
		}
		if mapKey == "" {
			mapKey = extractLLMString(nested, "key", "mapKey", "map_key")
		}
		if mapLimit == 0 {
			mapLimit = int(extractLLMFloat(nested, "limit", "mapLimit", "map_limit"))
		}
	}
	if mapLimit <= 0 {
		mapLimit = 10
	}
	return normalizeVisualLLMMapMode(mapMode), normalizeVisualLLMMapKey(mapKey), clampInt(mapLimit, 1, 100000)
}

func normalizeVisualLLMLogicNode(raw any, root bool, depth int, count *int) (LogicNode, error) {
	if depth > 5 {
		return LogicNode{}, errors.New("LLM condition tree is nested deeper than 5")
	}
	if raw == nil {
		if root {
			return defaultVisualLLMConditions(), nil
		}
		return LogicNode{}, errors.New("condition node is empty")
	}

	switch val := raw.(type) {
	case []any:
		children := make([]LogicNode, 0, len(val))
		for _, childRaw := range val {
			child, err := normalizeVisualLLMLogicNode(childRaw, false, depth+1, count)
			if err != nil {
				return LogicNode{}, err
			}
			children = append(children, child)
		}
		return LogicNode{ID: visualNodeID("group", root, depth, *count), Type: "AND", Children: children}, nil
	case map[string]any:
		typeName := strings.ToUpper(strings.TrimSpace(extractLLMString(val, "type", "kind")))
		if typeName == "" && val["field"] != nil {
			typeName = "CONDITION"
		}
		if typeName == "CONDITION" || val["field"] != nil {
			*count = *count + 1
			field := normalizeVisualLLMConditionField(extractLLMString(val, "field", "key"))
			operator := normalizeVisualLLMOperator(extractLLMString(val, "operator", "op"))
			value := sanitizeVisualLLMValue(valueToVisualString(firstPresent(val, "value", "match", "text")))
			if value == "" {
				return LogicNode{}, fmt.Errorf("condition %d has empty value", *count)
			}
			id := strings.TrimSpace(extractLLMString(val, "id"))
			if id == "" {
				id = fmt.Sprintf("cond-llm-%d", *count)
			}
			return LogicNode{ID: sanitizeVisualNodeID(id, "cond"), Type: "CONDITION", Field: field, Operator: operator, Value: value}, nil
		}

		groupType := "AND"
		if typeName == "OR" || strings.EqualFold(extractLLMString(val, "operator", "op"), "OR") {
			groupType = "OR"
		}
		childrenRaw, ok := firstPresent(val, "children", "conditions", "rules").([]any)
		if !ok || len(childrenRaw) == 0 {
			return LogicNode{}, errors.New("logic group has no children")
		}
		children := make([]LogicNode, 0, len(childrenRaw))
		for _, childRaw := range childrenRaw {
			child, err := normalizeVisualLLMLogicNode(childRaw, false, depth+1, count)
			if err != nil {
				return LogicNode{}, err
			}
			children = append(children, child)
		}
		id := strings.TrimSpace(extractLLMString(val, "id"))
		if id == "" || root {
			id = visualNodeID("group", root, depth, *count)
		}
		if root {
			id = "root"
		}
		return LogicNode{ID: sanitizeVisualNodeID(id, "group"), Type: groupType, Children: children}, nil
	default:
		return LogicNode{}, fmt.Errorf("unsupported condition node shape %T", raw)
	}
}

func normalizeVisualLLMTrigger(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	key = strings.TrimPrefix(key, "lsm/")
	key = strings.TrimPrefix(key, "kprobe/")
	key = strings.ReplaceAll(key, "-", "_")
	key = strings.TrimPrefix(key, "sys_")
	if mapped, ok := visualLLMAllowedTriggers[key]; ok {
		return mapped
	}
	for k, mapped := range visualLLMAllowedTriggers {
		if strings.Contains(key, k) {
			return mapped
		}
	}
	return "process"
}

func normalizeVisualLLMAction(value string) string {
	s := strings.ToUpper(strings.TrimSpace(value))
	switch s {
	case "KILL", "SIGKILL", "TERMINATE":
		return "KILL"
	case "ALERT", "AUDIT", "LOG", "ALLOW":
		return "ALERT"
	default:
		return "BLOCK"
	}
}

func normalizeVisualLLMMapMode(value string) string {
	s := strings.ToUpper(strings.TrimSpace(value))
	s = strings.ReplaceAll(s, "-", "_")
	switch s {
	case "COUNTER", "RATE_LIMIT", "RATELIMIT":
		return "COUNTER"
	case "BLOCKLIST", "BLACKLIST", "DENYLIST":
		return "BLOCKLIST"
	default:
		return "NONE"
	}
}

func normalizeVisualLLMMapKey(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	switch s {
	case "uid", "user":
		return "uid"
	case "comm", "command", "process", "process_name":
		return "comm"
	default:
		return "pid"
	}
}

func normalizeVisualLLMConditionField(value string) string {
	key := strings.ToLower(strings.TrimSpace(value))
	key = strings.ReplaceAll(key, "-", "_")
	if mapped, ok := visualLLMAllowedConditionFields[key]; ok {
		return mapped
	}
	for k, mapped := range visualLLMAllowedConditionFields {
		if strings.Contains(key, k) {
			return mapped
		}
	}
	return "comm"
}

func normalizeVisualLLMOperator(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	s = strings.ReplaceAll(s, "-", "_")
	switch s {
	case "!=", "not_equals", "not_equal", "not", "exclude":
		return "!="
	case "starts_with", "prefix", "begins_with":
		return "starts_with"
	case "ends_with", "suffix":
		return "ends_with"
	default:
		return "=="
	}
}

func valueToVisualString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case json.Number:
		return v.String()
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%g", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func sanitizeVisualLLMValue(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\x00", "")
	value = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ", "\\", "", "\"", "", "'", "").Replace(value)
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) > 64 {
		value = string(runes[:64])
	}
	return value
}

func sanitizeVisualNodeID(id, prefix string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	var b strings.Builder
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == '_' || r == ' ' || r == '/' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return prefix + "-llm"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}

func visualNodeID(prefix string, root bool, depth, count int) string {
	if root {
		return "root"
	}
	return fmt.Sprintf("%s-llm-%d-%d", prefix, depth, count)
}

func defaultVisualLLMConditions() LogicNode {
	return LogicNode{
		ID:   "root",
		Type: "AND",
		Children: []LogicNode{
			{ID: "cond-llm-default", Type: "CONDITION", Field: "comm", Operator: "==", Value: "nc"},
		},
	}
}

func visualLogicTreeUsesField(node LogicNode, fields ...string) bool {
	if node.Type == "CONDITION" {
		for _, field := range fields {
			if node.Field == field {
				return true
			}
		}
		return false
	}
	for _, child := range node.Children {
		if visualLogicTreeUsesField(child, fields...) {
			return true
		}
	}
	return false
}

func compactStrings(values []string, limit int) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}
