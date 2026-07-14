package events

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
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
	key, _ := semanticAlertContextKeyBounded(event)
	return key
}

func semanticAlertContextKeyBounded(event *pb.Event) (string, bool) {
	if event == nil {
		return "", false
	}
	if toolCallID := strings.TrimSpace(event.GetToolCallId()); toolCallID != "" {
		return boundSemanticStateString(toolCallID, SemanticStateMaxContextBytes)
	}
	taskID := strings.TrimSpace(event.GetTaskId())
	traceID := strings.TrimSpace(event.GetTraceId())
	if taskID != "" || traceID != "" {
		return boundSemanticStatePair(taskID, traceID, SemanticStateMaxContextBytes)
	}
	if agentRunID := strings.TrimSpace(event.GetAgentRunId()); agentRunID != "" {
		return boundSemanticStateString(agentRunID, SemanticStateMaxContextBytes)
	}
	if event.GetRootAgentPid() > 0 {
		return fmt.Sprintf("pid:%d", event.GetRootAgentPid()), false
	}
	if event.GetPid() > 0 {
		return fmt.Sprintf("pid:%d", event.GetPid()), false
	}
	return "", false
}

func extraInfoFieldBounded(extraInfo, key string, maxValueBytes int) (string, bool) {
	if extraInfo == "" || key == "" || maxValueBytes <= 0 {
		return "", false
	}
	scanLimit := len(extraInfo)
	if scanLimit > SemanticExtraInfoMaxScanBytes {
		scanLimit = SemanticExtraInfoMaxScanBytes
	}
	needle := key + "="
	for offset := 0; offset < scanLimit; {
		for offset < scanLimit {
			width, separator := semanticFieldSeparatorWidth(extraInfo[offset:])
			if !separator {
				break
			}
			offset += width
		}
		start := offset
		for offset < scanLimit {
			width, separator := semanticFieldSeparatorWidth(extraInfo[offset:])
			if separator {
				break
			}
			offset += width
		}
		if start == offset {
			continue
		}
		complete := offset < scanLimit || scanLimit == len(extraInfo)
		field := extraInfo[start:offset]
		if strings.HasPrefix(field, needle) {
			if !complete {
				return "", true
			}
			value := strings.TrimSpace(strings.TrimPrefix(field, needle))
			if value == "" {
				return "", len(extraInfo) > scanLimit
			}
			if len(value) > maxValueBytes {
				return "", true
			}
			return value, len(extraInfo) > scanLimit
		}
		if !complete {
			return "", true
		}
	}
	return "", len(extraInfo) > scanLimit
}

func semanticFieldSeparatorWidth(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	if value[0] < utf8.RuneSelf {
		switch value[0] {
		case ' ', '\t', '\n', '\r', '\v', '\f', ',':
			return 1, true
		default:
			return 1, false
		}
	}
	runeValue, width := utf8.DecodeRuneInString(value)
	return width, unicode.IsSpace(runeValue)
}

func isLowValueFileIOEvent(event *pb.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetType() {
	case "read", "write":
		return true
	case "openat", "open":
		path := strings.TrimSpace(event.GetPath())
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
	targets := [...]string{
		event.GetNetEndpoint(),
		event.GetSni(),
		event.GetHttpHost(),
		event.GetDnsName(),
		event.GetServiceName(),
		event.GetPath(),
	}
	hasTarget := false
	for _, target := range targets {
		if target == "" {
			continue
		}
		hasTarget = true
		for _, local := range []string{"127.0.0.1", "localhost", "::1"} {
			if semanticContainsFold(target, local) {
				return false
			}
		}
	}
	if !hasTarget {
		return false
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
		for _, target := range targets {
			if semanticContainsFold(target, hint) {
				return true
			}
		}
	}
	return event.GetDstPort() == 443 && platform.FirstNonEmpty(event.GetSni(), event.GetHttpHost(), event.GetDnsName()) != ""
}

func semanticContainsFold(value, needle string) bool {
	if needle == "" {
		return true
	}
	if len(needle) > len(value) {
		return false
	}
	for offset := 0; offset <= len(value)-len(needle); offset++ {
		if strings.EqualFold(value[offset:offset+len(needle)], needle) {
			return true
		}
	}
	return false
}

func semanticFileMutationPath(event *pb.Event) (string, bool, bool) {
	if event == nil {
		return "", false, false
	}
	switch event.GetType() {
	case "write", "chmod", "chown", "rename", "link", "symlink", "mknod", "mkdir", "unlink", "unlinkat":
	default:
		return "", false, false
	}
	path := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath())
	if path == "" {
		return "", false, false
	}
	trimmedPath := strings.TrimSpace(path)
	if strings.EqualFold(trimmedPath, "write") ||
		(len(trimmedPath) >= len("socket ") && strings.EqualFold(trimmedPath[:len("socket ")], "socket ")) {
		return "", false, false
	}
	normalized, truncated := normalizeSemanticPath(trimmedPath, event.GetCwd())
	return normalized, truncated, normalized != ""
}

func normalizeSemanticPath(path, cwd string) (string, bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", false
	}
	if !filepath.IsAbs(trimmed) {
		base := strings.TrimSpace(cwd)
		if filepath.IsAbs(base) {
			if len(base)+1+len(trimmed) > SemanticStateMaxPathBytes {
				return semanticBoundWithDigest(trimmed, SemanticStateMaxPathBytes, semanticStateDigest(base, trimmed)), true
			}
			trimmed = filepath.Join(base, trimmed)
		}
	}
	trimmed, truncated := boundSemanticStateString(trimmed, SemanticStateMaxPathBytes)
	if trimmed == "" {
		return "", truncated
	}
	return filepath.Clean(trimmed), truncated
}

func semanticAgentIdentity(event *pb.Event) (string, bool) {
	if event == nil {
		return "", false
	}
	if value := strings.TrimSpace(event.GetAgentRunId()); value != "" {
		return boundSemanticStatePrefixed("agent_run:", value, SemanticStateMaxContextBytes)
	}
	if value := strings.TrimSpace(event.GetTaskId()); value != "" {
		return boundSemanticStatePrefixed("task:", value, SemanticStateMaxContextBytes)
	}
	if value := strings.TrimSpace(event.GetToolCallId()); value != "" {
		return boundSemanticStatePrefixed("tool_call:", value, SemanticStateMaxContextBytes)
	}
	if value := strings.TrimSpace(event.GetTraceId()); value != "" {
		return boundSemanticStatePrefixed("trace:", value, SemanticStateMaxContextBytes)
	}
	if event.GetRootAgentPid() > 0 {
		return fmt.Sprintf("root_pid:%d", event.GetRootAgentPid()), false
	}
	if event.GetPid() > 0 {
		return fmt.Sprintf("pid:%d", event.GetPid()), false
	}
	return "", false
}
