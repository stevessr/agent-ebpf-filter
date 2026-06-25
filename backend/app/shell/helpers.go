package shell

import (
	"strings"
)

// PB_SchemaVersion is the event schema version string used when emitting stdio events.
const PB_SchemaVersion = "event.v3"

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func stringsTrimToDefault(value, fallback string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return fallback
	}
	return v
}

func normalizeShellSessionKind(value string) string {
	v := strings.TrimSpace(strings.ToLower(value))
	switch v {
	case KindShell, KindTmux, KindScript, KindWrapper:
		return v
	case "":
		return KindShell
	default:
		// Treat unrecognized kinds as shell.
		return KindShell
	}
}
