package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"bytes"
	"path/filepath"
	"strings"
	"time"
)

// ── Core helper functions ─────────────────────────────────────────────

func toolNameLooksReadOnly(toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}
	for _, hint := range ReadOnlyToolHints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func extractSecretTarget(event *pb.Event) (string, bool) {
	for _, candidate := range []string{event.GetPath(), event.GetExtraPath()} {
		if isSecretLikePath(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// isSecretLikePath reports whether path contains one of SecretPathHints,
// ignoring case. The path is folded into a stack buffer instead of through
// strings.ToLower so the check does not allocate on the per-event path.
func isSecretLikePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	var scratch [512]byte
	lower := lowerJoinedFields(scratch[:0], path)
	for _, hint := range SecretPathHints {
		if bytes.Contains(lower, []byte(hint)) {
			return true
		}
	}
	return false
}

func extractNetworkTarget(event *pb.Event) (string, bool) {
	switch event.GetType() {
	case "network_connect", "network_sendto":
		endpoint := strings.TrimSpace(event.GetNetEndpoint())
		if endpoint == "" {
			endpoint = strings.TrimSpace(event.GetPath())
		}
		if endpoint != "" {
			return endpoint, true
		}
	}
	return "", false
}

func networkEgressShouldAlert(event *pb.Event) bool {
	if event == nil {
		return false
	}
	if endpoint := strings.ToLower(strings.TrimSpace(event.GetNetEndpoint())); endpoint != "" {
		for _, hint := range []string{"127.0.0.1", "localhost"} {
			if strings.Contains(endpoint, hint) {
				return false
			}
		}
	}
	commandish := strings.ToLower(strings.TrimSpace(strings.Join([]string{
		event.GetToolName(),
		event.GetComm(),
		event.GetPath(),
		event.GetExtraInfo(),
	}, " ")))
	for _, hint := range ExpectedNetworkHints {
		if strings.Contains(commandish, hint) {
			return false
		}
	}
	return true
}

func riskyChildProcessReason(event *pb.Event) (string, bool) {
	switch event.GetType() {
	case "execve", "process_exec":
		comm := strings.ToLower(strings.TrimSpace(event.GetComm()))
		if reason, ok := RiskyExecComms[comm]; ok {
			return reason, true
		}
	}
	return "", false
}

func extractWorkspaceEscapeTarget(event *pb.Event) (string, bool) {
	if event == nil || !isFileLikeEvent(event.GetType()) {
		return "", false
	}
	for _, candidate := range []string{event.GetPath(), event.GetExtraPath()} {
		if isWorkspaceEscapePath(candidate, event.GetCwd()) {
			return candidate, true
		}
	}
	return "", false
}

func isWorkspaceEscapePath(path, cwd string) bool {
	trimmedPath := strings.TrimSpace(path)
	trimmedCwd := strings.TrimSpace(cwd)
	if trimmedPath == "" || trimmedCwd == "" {
		return false
	}
	normalizedCwd := filepath.Clean(trimmedCwd)
	if !filepath.IsAbs(normalizedCwd) {
		return false
	}
	normalizedPath := trimmedPath
	if !filepath.IsAbs(normalizedPath) {
		normalizedPath = filepath.Join(normalizedCwd, normalizedPath)
	}
	normalizedPath = filepath.Clean(normalizedPath)
	if pathWithinBase(normalizedPath, normalizedCwd) {
		return false
	}
	lower := strings.ToLower(normalizedPath)
	if isSecretLikePath(lower) {
		return true
	}
	for _, hint := range WorkspaceEscapeHints {
		if strings.HasPrefix(lower, hint) {
			return true
		}
	}
	return false
}

func pathWithinBase(path, base string) bool {
	cleanPath := filepath.Clean(path)
	cleanBase := filepath.Clean(base)
	if cleanPath == cleanBase {
		return true
	}
	baseWithSep := cleanBase + string(filepath.Separator)
	return strings.HasPrefix(cleanPath, baseWithSep)
}

func isFileLikeEvent(eventType string) bool {
	switch eventType {
	case "openat", "open", "read", "write", "chmod", "chown", "rename", "link", "symlink", "mknod", "mkdir", "unlink", "unlinkat":
		return true
	default:
		return false
	}
}

func detectSuspiciousShellTransport(event *pb.Event) (string, string, bool) {
	if event == nil {
		return "", "", false
	}
	var scratch [384]byte
	lower := lowerJoinedFields(scratch[:0], event.GetComm(), event.GetPath(), event.GetExtraInfo())
	contains := func(pattern string) bool { return bytes.Contains(lower, []byte(pattern)) }
	switch {
	case (contains("curl") || contains("wget")) &&
		(contains("| sh") || contains("| bash")):
		return platform.FirstNonEmpty(event.GetPath(), event.GetComm()), "observed a curl/wget pipeline into a shell", true
	case contains("bash -i >& /dev/tcp") ||
		contains("bash -i > /dev/tcp") ||
		contains("nc -e") ||
		contains("socat exec:") ||
		contains("/dev/tcp/"):
		return platform.FirstNonEmpty(event.GetPath(), event.GetComm()), "observed a reverse-shell-like shell transport pattern", true
	default:
		return "", "", false
	}
}

// lowerJoinedFields appends strings.ToLower(strings.Join(fields, " ")) to dst.
// ASCII input is folded in place without allocating; anything else goes
// through strings.ToLower so Unicode case mapping stays identical.
func lowerJoinedFields(dst []byte, fields ...string) []byte {
	for i, field := range fields {
		if i > 0 {
			dst = append(dst, ' ')
		}
		if isASCIIString(field) {
			for j := 0; j < len(field); j++ {
				c := field[j]
				if 'A' <= c && c <= 'Z' {
					c += 'a' - 'A'
				}
				dst = append(dst, c)
			}
			continue
		}
		dst = append(dst, strings.ToLower(field)...)
	}
	return dst
}

func isASCIIString(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func recentExecutableAfterChmod(event *pb.Event, now time.Time) (string, bool) {
	if event == nil {
		return "", false
	}
	contextKey := semanticAlertContextKey(event)
	if contextKey == "" {
		return "", false
	}
	switch event.GetType() {
	case "chmod":
		if modeLooksExecutable(event.GetMode()) {
			Deps.SemanticAlertsState.RememberExecutable(event, platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath()), event.GetMode(), now)
		}
	case "execve", "process_exec":
		if path := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath()); path != "" {
			if matchedPath, ok := Deps.SemanticAlertsState.RecentExecutablePath(contextKey, path, now); ok {
				return matchedPath, true
			}
		}
	}
	return "", false
}

func modeLooksExecutable(mode string) bool {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "7") || strings.Contains(trimmed, "5") || strings.Contains(strings.ToLower(trimmed), "x")
}

func observeForkStorm(event *pb.Event, now time.Time) (string, bool) {
	if event == nil {
		return "", false
	}
	switch event.GetType() {
	case "process_fork", "clone":
	default:
		return "", false
	}
	if count := Deps.SemanticAlertsState.IncrementForkCount(event, now); count >= SemanticForkStormThreshold {
		return platform.FirstNonEmpty(event.GetToolCallId(), event.GetAgentRunId(), event.GetComm(), event.GetPath()), true
	}
	return "", false
}
