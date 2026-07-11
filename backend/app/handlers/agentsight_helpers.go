package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func splitAgentSightCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeAgentSightTerms(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t'
	}))
}

func normalizeAgentSightTerms(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func parseAgentSightPIDList(raw string) []uint32 {
	parts := splitAgentSightCSV(raw)
	out := make([]uint32, 0, len(parts))
	for _, part := range parts {
		if parsed, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32); err == nil && parsed > 0 {
			out = append(out, uint32(parsed))
		}
	}
	return out
}

func agentSightStringInList(value string, candidates []string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func agentSightUint32InList(value uint32, candidates []uint32) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func parseAgentSightTimeAny(value any) time.Time {
	if value == nil {
		return time.Time{}
	}
	switch typed := value.(type) {
	case string:
		return Deps.ParseRecentEventTime(typed)
	case float64:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	case int64:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	case int:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	case json.Number:
		return agentSightTimeFromMillis(parseAgentSightTimestamp(typed, 0))
	default:
		return time.Time{}
	}
}

func agentSightTimeFromMillis(millis int64) time.Time {
	if millis <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(millis).UTC()
}

func appendQueryValue(rawQuery, key, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return rawQuery
	}
	if strings.TrimSpace(rawQuery) == "" {
		return key + "=" + value
	}
	return rawQuery + "&" + key + "=" + value
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func uint32FromAny(value any) uint32 {
	switch typed := value.(type) {
	case uint32:
		return typed
	case uint64:
		return uint32(typed)
	case int:
		if typed > 0 {
			return uint32(typed)
		}
	case int64:
		if typed > 0 {
			return uint32(typed)
		}
	case float64:
		if typed > 0 {
			return uint32(typed)
		}
	case json.Number:
		if parsed, err := typed.Int64(); err == nil && parsed > 0 {
			return uint32(parsed)
		}
	case string:
		if parsed, err := strconv.ParseUint(strings.TrimSpace(typed), 10, 32); err == nil {
			return uint32(parsed)
		}
	}
	return 0
}

func parseAgentSightTimestamp(value any, fallback int64) int64 {
	var numeric float64
	switch typed := value.(type) {
	case int64:
		numeric = float64(typed)
	case int:
		numeric = float64(typed)
	case uint64:
		numeric = float64(typed)
	case float64:
		numeric = typed
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return fallback
		}
		numeric = parsed
	case string:
		raw := strings.TrimSpace(typed)
		if raw == "" {
			return fallback
		}
		if parsed, err := strconv.ParseFloat(raw, 64); err == nil {
			numeric = parsed
			break
		}
		if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			return parsed.UTC().UnixMilli()
		}
		return fallback
	default:
		return fallback
	}
	if numeric <= 0 {
		return fallback
	}
	switch {
	case numeric > 1_000_000_000_000_000:
		return int64(numeric / 1_000_000)
	case numeric > 10_000_000_000_000:
		return int64(numeric / 1_000)
	default:
		return int64(numeric)
	}
}

func agentSightStableID(prefix string, parts ...any) string {
	hash := sha256.New()
	for _, part := range parts {
		payload, err := json.Marshal(part)
		if err != nil {
			payload = []byte(fmt.Sprint(part))
		}
		_, _ = hash.Write(payload)
		_, _ = hash.Write([]byte{0})
	}
	return prefix + "-" + hex.EncodeToString(hash.Sum(nil))[:20]
}
