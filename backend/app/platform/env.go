package platform

import (
	"os"
	"strconv"
	"strings"
)

// ── OS env helpers ────────────────────────────────────────────────────────

// FirstEnv returns the first non-empty env var value from the given keys.
func FirstEnv(keys ...string) (string, bool) {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value, true
		}
	}
	return "", false
}

// ApplyStringEnv sets dst to the first non-empty env var value.
func ApplyStringEnv(dst *string, keys ...string) {
	if value, ok := FirstEnv(keys...); ok {
		*dst = value
	}
}

// ApplyBoolEnv sets dst from the first non-empty env var value.
func ApplyBoolEnv(dst *bool, keys ...string) {
	if value, ok := FirstEnv(keys...); ok {
		*dst = parseBoolEnv(value)
	}
}

// ApplyIntEnv sets dst from the first non-empty env var value.
func ApplyIntEnv(dst *int, keys ...string) {
	if value, ok := FirstEnv(keys...); ok {
		if parsed, err := strconv.Atoi(value); err == nil {
			*dst = parsed
		}
	}
}

// ApplyFloatEnv sets dst from the first non-empty env var value.
func ApplyFloatEnv(dst *float64, keys ...string) {
	if value, ok := FirstEnv(keys...); ok {
		if parsed, err := strconv.ParseFloat(value, 64); err == nil {
			*dst = parsed
		}
	}
}

// ApplyModelTypeEnv sets a string-typed dst from the first non-empty env var.
func ApplyModelTypeEnv(dst *string, keys ...string) {
	ApplyStringEnv(dst, keys...)
}

func parseBoolEnv(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
