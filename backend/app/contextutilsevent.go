package app

import (
	"fmt"
	"strings"
)

// ---- moved from backend/zz_merged_backend.go section contextutilsevent.go ----

func payloadString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					return trimmed
				}
			case fmt.Stringer:
				if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
					return trimmed
				}
			case float64:
				return strings.TrimSpace(fmt.Sprintf("%.0f", typed))
			case int:
				return fmt.Sprintf("%d", typed)
			case int64:
				return fmt.Sprintf("%d", typed)
			case uint32:
				return fmt.Sprintf("%d", typed)
			}
		}
	}
	return ""
}

func payloadUint32(payload map[string]interface{}, keys ...string) uint32 {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case float64:
				if typed > 0 {
					return uint32(typed)
				}
			case int:
				if typed > 0 {
					return uint32(typed)
				}
			case int64:
				if typed > 0 {
					return uint32(typed)
				}
			case uint32:
				return typed
			case string:
				var parsed uint32
				if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%d", &parsed); err == nil && parsed > 0 {
					return parsed
				}
			}
		}
	}
	return 0
}

func payloadFloat64(payload map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case float64:
				return typed
			case float32:
				return float64(typed)
			case int:
				return float64(typed)
			case int64:
				return float64(typed)
			case string:
				var parsed float64
				if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &parsed); err == nil {
					return parsed
				}
			}
		}
	}
	return 0
}

func parseUintField(extraInfo, key string) uint32 {
	needle := key + "="
	for _, part := range strings.Fields(strings.ReplaceAll(extraInfo, ",", " ")) {
		if strings.HasPrefix(part, needle) {
			var parsed uint32
			if _, err := fmt.Sscanf(strings.TrimPrefix(part, needle), "%d", &parsed); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func parseFloatField(extraInfo, key string) float64 {
	needle := key + "="
	for _, part := range strings.Fields(strings.ReplaceAll(extraInfo, ",", " ")) {
		if strings.HasPrefix(part, needle) {
			var parsed float64
			if _, err := fmt.Sscanf(strings.TrimPrefix(part, needle), "%f", &parsed); err == nil {
				return parsed
			}
		}
	}
	return 0
}

func parseStringField(extraInfo, key string) string {
	needle := key + "="
	for _, part := range strings.Fields(strings.ReplaceAll(extraInfo, ",", " ")) {
		if strings.HasPrefix(part, needle) {
			return strings.TrimSpace(strings.TrimPrefix(part, needle))
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func maxFloat64(values ...float64) float64 {
	max := 0.0
	for _, value := range values {
		if value > max {
			max = value
		}
	}
	return max
}
