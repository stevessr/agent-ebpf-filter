package events

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
)

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
	if toolCallID, truncated := boundSemanticStateString(event.GetToolCallId(), SemanticStateMaxContextBytes); toolCallID != "" {
		return toolCallID, truncated
	}
	if taskTrace, truncated := boundSemanticStatePair(event.GetTaskId(), event.GetTraceId(), SemanticStateMaxContextBytes); taskTrace != "" {
		return taskTrace, truncated
	}
	if agentRunID, truncated := boundSemanticStateString(event.GetAgentRunId(), SemanticStateMaxContextBytes); agentRunID != "" {
		return agentRunID, truncated
	}
	if event.GetRootAgentPid() > 0 {
		return pidContextKey(event.GetRootAgentPid()), false
	}
	if event.GetPid() > 0 {
		return pidContextKey(event.GetPid()), false
	}
	return "", false
}

func pidContextKey(pid uint32) string {
	var scratch [16]byte
	key := append(scratch[:0], "pid:"...)
	return string(strconv.AppendUint(key, uint64(pid), 10))
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
	return isLowValueFileIOEventWith(event, isSecretLikePath(event.GetPath()))
}

// isLowValueFileIOEventWith is isLowValueFileIOEvent with the secret-path
// verdict for event.Path already known.
func isLowValueFileIOEventWith(event *pb.Event, pathIsSecret bool) bool {
	if event == nil {
		return false
	}
	switch event.GetType() {
	case "read", "write":
		return true
	case "openat", "open":
		return strings.TrimSpace(event.GetPath()) != "" && !pathIsSecret
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

// File contention requires an actual, unambiguous filesystem target. Kernel
// write(2) only exposes an fd; event adapters sometimes report a description
// such as "file write" instead of a resolved filename. Those labels are not
// paths and must never become keys shared across unrelated processes.
func semanticFileMutationPath(event *pb.Event) (string, bool, bool) {
	if event == nil {
		return "", false, false
	}
	switch event.GetType() {
	case "write", "chmod", "chown", "rename", "link", "symlink", "mknod", "mkdir", "unlink", "unlinkat":
	default:
		return "", false, false
	}
	// As with Tetragon's return-value selectors, a failed mutating syscall
	// cannot establish a successful change to a shared resource.
	// Zero is intentionally permitted for metadata syscalls: success == 0.
	if event.GetRetval() < 0 {
		return "", false, false
	}
	for _, candidate := range []string{event.GetPath(), event.GetExtraPath()} {
		path := strings.TrimSpace(candidate)
		if path == "" || semanticFileTargetIsPlaceholder(path) {
			continue
		}
		if !filepath.IsAbs(path) && semanticFileTargetIsPlaceholder(event.GetCwd()) {
			continue
		}
		normalized, truncated := normalizeSemanticPath(path, event.GetCwd())
		// An unresolved relative filename is ambiguous across working
		// directories. A truncated/redacted key is not a proven same file.
		if truncated || !filepath.IsAbs(normalized) {
			continue
		}
		return normalized, false, true
	}
	return "", false, false
}

func semanticFileTargetIsPlaceholder(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	switch lower {
	case "write", "read", "file write", "file read", "file_write", "file_read",
		"file-write", "file-read", "file writev", "file readv",
		"file_writev", "file_readv", "unknown", "(unknown)", "<unknown>",
		"<redacted>", "[redacted]", "<custom_redacted>", "-":
		return true
	}
	// A redacted target or cwd cannot prove that two Agents wrote the same
	// inode: unrelated paths often collapse to one shared placeholder.
	if strings.Contains(lower, "[redacted]") || strings.Contains(lower, "<redacted") ||
		strings.Contains(lower, "<custom_redacted>") {
		return true
	}
	for _, prefix := range []string{"socket ", "socket:", "pipe:", "anon_inode:", "fd:", "fd="} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func normalizeSemanticPath(path, cwd string) (string, bool) {
	rawPath := path
	boundedInput := rawPath
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
			boundedInput = trimmed
		}
	}
	trimmed, truncated := boundSemanticStateString(boundedInput, SemanticStateMaxPathBytes)
	if trimmed == "" {
		return "", truncated
	}
	return filepath.Clean(trimmed), truncated
}

// A PID by itself is a process identity, not evidence of an Agent context.
// Tool call and trace identifiers may rotate within one Agent, so they are not
// valid evidence that two different Agents touched a file.
func semanticAgentIdentity(event *pb.Event) (string, bool) {
	if event == nil {
		return "", false
	}
	if event.GetRootAgentPid() > 0 {
		return fmt.Sprintf("root_pid:%d", event.GetRootAgentPid()), false
	}
	if value, truncated := boundSemanticStatePrefixed("agent_run:", event.GetAgentRunId(), SemanticStateMaxContextBytes); value != "" {
		return value, truncated
	}
	return "", false
}
