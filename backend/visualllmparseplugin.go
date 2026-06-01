package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

func parseVisualBlocksLLMContent(content string) (*visualBlocksLLMCompileResponse, error) {
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

	return &visualBlocksLLMCompileResponse{
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

func normalizeVisualLLMLogicNode(raw any, root bool, depth int, count *int) (visualBlocksLogicNode, error) {
	if depth > 5 {
		return visualBlocksLogicNode{}, errors.New("LLM condition tree is nested deeper than 5")
	}
	if raw == nil {
		if root {
			return defaultVisualLLMConditions(), nil
		}
		return visualBlocksLogicNode{}, errors.New("condition node is empty")
	}

	switch val := raw.(type) {
	case []any:
		children := make([]visualBlocksLogicNode, 0, len(val))
		for _, childRaw := range val {
			child, err := normalizeVisualLLMLogicNode(childRaw, false, depth+1, count)
			if err != nil {
				return visualBlocksLogicNode{}, err
			}
			children = append(children, child)
		}
		return visualBlocksLogicNode{ID: visualNodeID("group", root, depth, *count), Type: "AND", Children: children}, nil
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
				return visualBlocksLogicNode{}, fmt.Errorf("condition %d has empty value", *count)
			}
			id := strings.TrimSpace(extractLLMString(val, "id"))
			if id == "" {
				id = fmt.Sprintf("cond-llm-%d", *count)
			}
			return visualBlocksLogicNode{ID: sanitizeVisualNodeID(id, "cond"), Type: "CONDITION", Field: field, Operator: operator, Value: value}, nil
		}

		groupType := "AND"
		if typeName == "OR" || strings.EqualFold(extractLLMString(val, "operator", "op"), "OR") {
			groupType = "OR"
		}
		childrenRaw, ok := firstPresent(val, "children", "conditions", "rules").([]any)
		if !ok || len(childrenRaw) == 0 {
			return visualBlocksLogicNode{}, errors.New("logic group has no children")
		}
		children := make([]visualBlocksLogicNode, 0, len(childrenRaw))
		for _, childRaw := range childrenRaw {
			child, err := normalizeVisualLLMLogicNode(childRaw, false, depth+1, count)
			if err != nil {
				return visualBlocksLogicNode{}, err
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
		return visualBlocksLogicNode{ID: sanitizeVisualNodeID(id, "group"), Type: groupType, Children: children}, nil
	default:
		return visualBlocksLogicNode{}, fmt.Errorf("unsupported condition node shape %T", raw)
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

func defaultVisualLLMConditions() visualBlocksLogicNode {
	return visualBlocksLogicNode{
		ID:   "root",
		Type: "AND",
		Children: []visualBlocksLogicNode{
			{ID: "cond-llm-default", Type: "CONDITION", Field: "comm", Operator: "==", Value: "nc"},
		},
	}
}

func visualLogicTreeUsesField(node visualBlocksLogicNode, fields ...string) bool {
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
