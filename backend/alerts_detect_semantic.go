package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
)

// ── Core helper functions ─────────────────────────────────────────────

func toolNameLooksReadOnly(toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}
	for _, hint := range readOnlyToolHints {
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

func isSecretLikePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	if lower == "" {
		return false
	}
	for _, hint := range secretPathHints {
		if strings.Contains(lower, hint) {
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
	for _, hint := range expectedNetworkHints {
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
		if reason, ok := riskyExecComms[comm]; ok {
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
	for _, hint := range workspaceEscapeHints {
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

func isLowValueFileIOEvent(event *pb.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetType() {
	case "read", "write":
		return true
	case "openat", "open":
		path := strings.ToLower(strings.TrimSpace(event.GetPath()))
		if path == "" || isSecretLikePath(path) {
			return false
		}
		return true
	default:
		return false
	}
}

func isAPILikeNetworkEvent(event *pb.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetType() {
	case "network_connect", "network_sendto", "tcp_connect", "dns_query":
	default:
		return false
	}
	target := strings.ToLower(strings.Join([]string{
		event.GetNetEndpoint(),
		event.GetSni(),
		event.GetHttpHost(),
		event.GetDnsName(),
		event.GetServiceName(),
		event.GetPath(),
	}, " "))
	if target == "" {
		return false
	}
	for _, local := range []string{"127.0.0.1", "localhost", "::1"} {
		if strings.Contains(target, local) {
			return false
		}
	}
	for _, hint := range []string{
		"api",
		"openai",
		"anthropic",
		"claude",
		"gemini",
		"generativelanguage",
		"azure.com",
		"bedrock",
		"cohere",
		"mistral",
		"ollama",
	} {
		if strings.Contains(target, hint) {
			return true
		}
	}
	return event.GetDstPort() == 443 && firstNonEmpty(event.GetSni(), event.GetHttpHost(), event.GetDnsName()) != ""
}

func semanticFileMutationPath(event *pb.Event) (string, bool) {
	if event == nil {
		return "", false
	}
	switch event.GetType() {
	case "write", "chmod", "chown", "rename", "link", "symlink", "mknod", "mkdir", "unlink", "unlinkat":
	default:
		return "", false
	}
	path := firstNonEmpty(event.GetPath(), event.GetExtraPath())
	if path == "" {
		return "", false
	}
	lower := strings.ToLower(strings.TrimSpace(path))
	if lower == "write" || strings.HasPrefix(lower, "socket ") {
		return "", false
	}
	return normalizeSemanticPath(path, event.GetCwd()), true
}

func normalizeSemanticPath(path, cwd string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	if !filepath.IsAbs(trimmed) {
		base := strings.TrimSpace(cwd)
		if filepath.IsAbs(base) {
			trimmed = filepath.Join(base, trimmed)
		}
	}
	return filepath.Clean(trimmed)
}

func semanticAgentIdentity(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if value := strings.TrimSpace(event.GetAgentRunId()); value != "" {
		return "agent_run:" + value
	}
	if value := strings.TrimSpace(event.GetTaskId()); value != "" {
		return "task:" + value
	}
	if value := strings.TrimSpace(event.GetToolCallId()); value != "" {
		return "tool_call:" + value
	}
	if value := strings.TrimSpace(event.GetTraceId()); value != "" {
		return "trace:" + value
	}
	if event.GetRootAgentPid() > 0 {
		return fmt.Sprintf("root_pid:%d", event.GetRootAgentPid())
	}
	if event.GetPid() > 0 {
		return fmt.Sprintf("pid:%d", event.GetPid())
	}
	return ""
}

func extraInfoField(extraInfo, key string) string {
	needle := key + "="
	for _, part := range strings.Fields(strings.ReplaceAll(extraInfo, ",", " ")) {
		if strings.HasPrefix(part, needle) {
			return strings.TrimSpace(strings.TrimPrefix(part, needle))
		}
	}
	return ""
}

func detectSuspiciousShellTransport(event *pb.Event) (string, string, bool) {
	if event == nil {
		return "", "", false
	}
	lower := strings.ToLower(strings.Join([]string{
		event.GetComm(),
		event.GetPath(),
		event.GetExtraInfo(),
	}, " "))
	switch {
	case (strings.Contains(lower, "curl") || strings.Contains(lower, "wget")) &&
		(strings.Contains(lower, "| sh") || strings.Contains(lower, "| bash")):
		return firstNonEmpty(event.GetPath(), event.GetComm()), "observed a curl/wget pipeline into a shell", true
	case strings.Contains(lower, "bash -i >& /dev/tcp") ||
		strings.Contains(lower, "bash -i > /dev/tcp") ||
		strings.Contains(lower, "nc -e") ||
		strings.Contains(lower, "socat exec:") ||
		strings.Contains(lower, "/dev/tcp/"):
		return firstNonEmpty(event.GetPath(), event.GetComm()), "observed a reverse-shell-like shell transport pattern", true
	default:
		return "", "", false
	}
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
			semanticAlertsState.rememberExecutable(event, firstNonEmpty(event.GetPath(), event.GetExtraPath()), event.GetMode(), now)
		}
	case "execve", "process_exec":
		if path := firstNonEmpty(event.GetPath(), event.GetExtraPath()); path != "" {
			if matchedPath, ok := semanticAlertsState.recentExecutablePath(contextKey, path, now); ok {
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
	if count := semanticAlertsState.incrementForkCount(event, now); count >= semanticForkStormThreshold {
		return firstNonEmpty(event.GetToolCallId(), event.GetAgentRunId(), event.GetComm(), event.GetPath()), true
	}
	return "", false
}

func observeAgenticResourceLoop(event *pb.Event, now time.Time) (string, string, bool) {
	return semanticAlertsState.observeAgenticResourceLoop(event, now)
}

func observeMultiAgentFileContention(event *pb.Event, now time.Time) (string, string, bool) {
	return semanticAlertsState.observeMultiAgentFileContention(event, now)
}

func semanticAlertContextKey(event *pb.Event) string {
	if event == nil {
		return ""
	}
	taskTraceKey := ""
	if taskID := strings.TrimSpace(event.GetTaskId()); taskID != "" || strings.TrimSpace(event.GetTraceId()) != "" {
		taskTraceKey = taskID + "|" + strings.TrimSpace(event.GetTraceId())
	}
	candidates := []string{
		strings.TrimSpace(event.GetToolCallId()),
		taskTraceKey,
		strings.TrimSpace(event.GetAgentRunId()),
	}
	if event.GetRootAgentPid() > 0 {
		candidates = append(candidates, fmt.Sprintf("pid:%d", event.GetRootAgentPid()))
	}
	if event.GetPid() > 0 {
		candidates = append(candidates, fmt.Sprintf("pid:%d", event.GetPid()))
	}
	return firstNonEmpty(candidates...)
}
