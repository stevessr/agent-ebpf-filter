package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"fmt"
	"path/filepath"
	"strings"
)

// ---- moved from app/alertscheckssemantic.go ----

// Codex-specific workflow semantic checks

func detectPRReviewAnomaly(event *pb.Event) (string, bool) {
	if !toolNameMatchesHints(event.GetToolName(), PRReviewToolHints) {
		return "", false
	}
	switch event.GetType() {
	case "execve", "process_exec":
		if isPRReviewReadOnlyExec(event.GetComm(), event.GetPath()) {
			return "", false
		}
		return fmt.Sprintf("PR review tool %q spawned a process (%s)", event.GetToolName(), event.GetComm()), true
	case "network_connect", "network_sendto":
		endpoint := strings.TrimSpace(event.GetNetEndpoint())
		if endpoint != "" && !strings.Contains(endpoint, "127.0.0.1") && !strings.Contains(endpoint, "localhost") {
			return fmt.Sprintf("PR review tool %q opened unexpected network egress to %s", event.GetToolName(), endpoint), true
		}
	case "write", "chmod", "unlink", "unlinkat":
		return fmt.Sprintf("PR review tool %q modified filesystem (%s %s)", event.GetToolName(), event.GetType(), event.GetPath()), true
	}
	return "", false
}

func isPRReviewReadOnlyExec(comm, path string) bool {
	lowerComm := strings.ToLower(strings.TrimSpace(comm))
	lowerPath := strings.ToLower(strings.TrimSpace(filepath.Base(path)))
	for _, allowed := range []string{"rg", "grep", "git", "diff", "cat", "sed", "awk", "find", "ls"} {
		if lowerComm == allowed || lowerPath == allowed {
			return true
		}
	}
	return false
}

func detectBrowserTaskAnomaly(event *pb.Event) (string, bool) {
	if !toolNameMatchesHints(event.GetToolName(), BrowserFrontendToolHints) {
		return "", false
	}
	switch event.GetType() {
	case "execve", "process_exec":
		comm := strings.ToLower(strings.TrimSpace(event.GetComm()))
		for _, risky := range []string{"nc", "netcat", "socat", "ssh", "nohup", "disown"} {
			if comm == risky || strings.HasPrefix(comm, risky) {
				return fmt.Sprintf("browser/frontend tool %q spawned risky process %q", event.GetToolName(), event.GetComm()), true
			}
		}
	case "network_connect", "network_sendto":
		endpoint := strings.TrimSpace(event.GetNetEndpoint())
		if isNonLocalhostEndpoint(endpoint) {
			return fmt.Sprintf("browser/frontend tool %q opened unexpected network egress to %s", event.GetToolName(), endpoint), true
		}
	}
	return "", false
}

func detectIDEHandoffAnomaly(event *pb.Event) (string, bool) {
	if !toolNameMatchesHints(event.GetToolName(), IDEHandoffToolHints) {
		return "", false
	}
	if target, ok := extractSecretTarget(event); ok {
		return fmt.Sprintf("IDE handoff tool %q accessed secret-like path %s", event.GetToolName(), target), true
	}
	if target, ok := extractWorkspaceEscapeTarget(event); ok {
		return fmt.Sprintf("IDE handoff tool %q escaped workspace boundary to %s", event.GetToolName(), target), true
	}
	return "", false
}

func detectRemoteDevboxAnomaly(event *pb.Event) (string, bool) {
	if !toolNameMatchesHints(event.GetToolName(), RemoteDevboxToolHints) {
		return "", false
	}
	switch event.GetType() {
	case "network_connect", "network_sendto":
		endpoint := strings.TrimSpace(event.GetNetEndpoint())
		if isNonLocalhostEndpoint(endpoint) {
			if isSuspiciousEndpoint(endpoint) {
				return fmt.Sprintf("remote devbox tool %q connected to suspicious endpoint %s", event.GetToolName(), endpoint), true
			}
		}
	case "execve", "process_exec":
		comm := strings.ToLower(strings.TrimSpace(event.GetComm()))
		for _, risky := range []string{"nc", "socat", "reverse", "backdoor"} {
			if strings.Contains(comm, risky) {
				return fmt.Sprintf("remote devbox tool %q spawned suspicious process %q", event.GetToolName(), event.GetComm()), true
			}
		}
	}
	return "", false
}

func toolNameMatchesHints(toolName string, hints []string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	if lower == "" {
		return false
	}
	for _, hint := range hints {
		if strings.Contains(lower, hint) {
			return true
		}
	}
	return false
}

func isNonLocalhostEndpoint(endpoint string) bool {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return false
	}
	for _, hint := range []string{"127.0.0.1", "localhost", "::1", "0.0.0.0"} {
		if strings.Contains(endpoint, hint) {
			return false
		}
	}
	return true
}

func isSuspiciousEndpoint(endpoint string) bool {
	endpoint = strings.ToLower(strings.TrimSpace(endpoint))
	suspiciousPatterns := []string{
		".ngrok.io", ".serveo.net", ".localhost.run",
		":4444", ":1337", ":31337", ":6666", ":6667",
		"pastebin", "termbin", "ix.io",
	}
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(endpoint, pattern) {
			return true
		}
	}
	return false
}

// ── Semantic alert state helpers (extracted from SemanticAlertState methods) ──

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
	return platform.FirstNonEmpty(candidates...)
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
	return event.GetDstPort() == 443 && platform.FirstNonEmpty(event.GetSni(), event.GetHttpHost(), event.GetDnsName()) != ""
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
	path := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath())
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