package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func normalizeDatasetLabelValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return normalizeActionLabel(v)
	case json.Number:
		if n, err := strconv.Atoi(v.String()); err == nil {
			return actionLabel[int32(n)]
		}
	case float64:
		return actionLabel[int32(v)]
	case int:
		return actionLabel[int32(v)]
	case int64:
		return actionLabel[int32(v)]
	case uint32:
		return actionLabel[int32(v)]
	case uint64:
		return actionLabel[int32(v)]
	}
	return ""
}

func normalizeActionLabel(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "0", "ALLOW", "BENIGN", "SAFE", "NORMAL", "NORM", "PASSED", "PASS":
		return "ALLOW"
	case "1", "BLOCK", "DENY", "REJECT", "MALICIOUS", "MALWARE", "BAD", "ANOM", "ANOMALY", "ATTACK", "INTRUSION", "CMDI", "COMMAND INJECTION", "SQLI", "SQL INJECTION", "XSS", "CROSS-SITE SCRIPTING", "PATH-TRAVERSAL", "PATH_TRAVERSAL", "PATH TRAVERSAL":
		return "BLOCK"
	case "2", "REWRITE", "TRANSFORM", "MODIFY":
		return "REWRITE"
	case "3", "ALERT", "WARN", "WARNING", "SUSPICIOUS":
		return "ALERT"
	default:
		return ""
	}
}

func extractDatasetArgs(row map[string]any, commandLine string) []string {
	if args := extractDatasetStringSlice(row, "args", "argv", "arguments", "commandArgs"); len(args) > 0 {
		return args
	}
	if raw := firstAnyValue(row, "args", "argv", "arguments", "commandArgs"); raw != nil {
		if str, ok := raw.(string); ok && strings.TrimSpace(str) != "" {
			return splitCommandLine(str)
		}
	}
	if commandLine != "" {
		_, args := normalizeCommandInput(commandLine, "", nil)
		return args
	}
	return nil
}

func extractDatasetStringSlice(row map[string]any, keys ...string) []string {
	for _, key := range keys {
		raw, ok := row[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case []any:
			out := make([]string, 0, len(value))
			for _, item := range value {
				if s := fmt.Sprint(item); strings.TrimSpace(s) != "" {
					out = append(out, strings.TrimSpace(s))
				}
			}
			if len(out) > 0 {
				return out
			}
		case string:
			if strings.TrimSpace(value) != "" {
				return splitCommandLine(value)
			}
		}
	}
	return nil
}

func extractDatasetFloat(row map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		raw, ok := row[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case float64:
			return value, true
		case float32:
			return float64(value), true
		case int:
			return float64(value), true
		case int64:
			return float64(value), true
		case json.Number:
			if f, err := value.Float64(); err == nil {
				return f, true
			}
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return f, true
			}
		}
	}
	return 0, false
}

func extractDatasetTimestamp(row map[string]any) (time.Time, bool) {
	raw := firstAnyValue(row, "timestamp", "time", "createdAt", "created_at", "ts")
	if raw == nil {
		return time.Time{}, false
	}
	switch value := raw.(type) {
	case string:
		for _, layout := range []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05",
		} {
			if ts, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
				return ts, true
			}
		}
		if num, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil {
			return parseUnixTimestamp(num), true
		}
	case float64:
		return parseUnixTimestamp(int64(value)), true
	case json.Number:
		if n, err := value.Int64(); err == nil {
			return parseUnixTimestamp(n), true
		}
	case int64:
		return parseUnixTimestamp(value), true
	case int:
		return parseUnixTimestamp(int64(value)), true
	}
	return time.Time{}, false
}

func parseUnixTimestamp(v int64) time.Time {
	switch {
	case v > 1_000_000_000_000:
		return time.Unix(0, v*int64(time.Millisecond)).UTC()
	case v > 1_000_000_000:
		return time.Unix(v, 0).UTC()
	default:
		return time.Unix(v, 0).UTC()
	}
}

func firstStringValue(row map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := row[key]; ok && raw != nil {
			switch v := raw.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					return strings.TrimSpace(v)
				}
			case map[string]any, []any:
				// Special serialization: if it's an object/array, return as JSON string
				b, _ := json.Marshal(v)
				return string(b)
			default:
				s := fmt.Sprint(v)
				if strings.TrimSpace(s) != "" && s != "<nil>" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func firstAnyValue(row map[string]any, keys ...string) any {
	for _, key := range keys {
		if raw, ok := row[key]; ok {
			if raw != nil {
				return raw
			}
		}
	}
	return nil
}

func normalizeHeaderRow(headers []string) []string {
	out := make([]string, 0, len(headers))
	for _, header := range headers {
		header = strings.ToLower(strings.TrimSpace(header))
		header = strings.ReplaceAll(header, " ", "")
		header = strings.ReplaceAll(header, "-", "_")
		if header != "" {
			out = append(out, header)
		} else {
			out = append(out, "")
		}
	}
	return out
}
