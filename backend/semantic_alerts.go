package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
)

var secretPathHints = []string{
	"/.ssh/",
	"id_rsa",
	"id_ed25519",
	".aws/credentials",
	".git-credentials",
	".npmrc",
	".pypirc",
	".netrc",
	"/etc/shadow",
	"/etc/passwd",
	"/kube/config",
	"/.env",
	"/secrets/",
}

var readOnlyToolHints = []string{
	"read",
	"view",
	"list",
	"glob",
	"grep",
	"search",
	"inspect",
	"find",
	"cat",
}

var prReviewToolHints = []string{
	"review",
	"pr_",
	"pull_request",
	"diff",
	"patch",
	"approve",
}

var browserFrontendToolHints = []string{
	"browser",
	"frontend",
	"ui_",
	"playwright",
	"selenium",
	"puppeteer",
	"cypress",
	"chrome",
	"navigate",
	"screenshot",
}

var ideHandoffToolHints = []string{
	"handoff",
	"ide_",
	"open_in_",
	"editor",
}

var remoteDevboxToolHints = []string{
	"devbox",
	"remote_",
	"ssh_",
	"ssm_",
}

var riskyExecComms = map[string]string{
	"curl":   "network download tool",
	"wget":   "network download tool",
	"nc":     "raw network tool",
	"netcat": "raw network tool",
	"socat":  "socket relay tool",
	"ssh":    "remote shell tool",
	"scp":    "remote copy tool",
	"rsync":  "remote sync tool",
}

var expectedNetworkHints = []string{
	"install",
	"update",
	"fetch",
	"clone",
	"pull",
	"download",
	"dependency",
	"npm",
	"pnpm",
	"yarn",
	"bun",
	"pip",
	"uv",
	"cargo",
	"gem",
}

var workspaceEscapeHints = []string{
	"/etc/",
	"/root/",
	"/proc/",
	"/sys/",
	"/dev/",
	"/var/run/docker.sock",
}

const (
	semanticSecretCorrelationTTL = 30 * time.Second
	semanticExecCorrelationTTL   = 30 * time.Second
	semanticForkWindow           = 2 * time.Second
	semanticForkStormThreshold   = 8
)

type semanticSecretObservation struct {
	SeenAt time.Time
	Target string
}

type semanticExecObservation struct {
	SeenAt time.Time
	Path   string
	Mode   string
}

type semanticForkObservation struct {
	WindowStart time.Time
	Count       int
}

type semanticAlertState struct {
	mu            sync.Mutex
	recentSecrets map[string]semanticSecretObservation
	recentExecs   map[string]semanticExecObservation
	forkWindows   map[string]semanticForkObservation
}

func newSemanticAlertState() *semanticAlertState {
	return &semanticAlertState{
		recentSecrets: make(map[string]semanticSecretObservation),
		recentExecs:   make(map[string]semanticExecObservation),
		forkWindows:   make(map[string]semanticForkObservation),
	}
}

var semanticAlertsState = newSemanticAlertState()

func resetSemanticAlertState() {
	semanticAlertsState = newSemanticAlertState()
}

func buildSemanticAlerts(event *pb.Event) []*pb.Event {
	if event == nil || event.GetType() == "semantic_alert" {
		return nil
	}

	now := time.Now().UTC()
	readonlyTool := toolNameLooksReadOnly(event.GetToolName())
	alerts := make([]*pb.Event, 0, 3)
	seen := make(map[string]struct{})
	addAlert := func(code, target, reason string, minimumRisk float64) {
		if _, ok := seen[code]; ok {
			return
		}
		seen[code] = struct{}{}
		alerts = append(alerts, newSemanticAlertEvent(event, code, target, reason, minimumRisk))
	}

	if target, ok := extractSecretTarget(event); ok {
		semanticAlertsState.rememberSecret(event, target, now)
		addAlert("SECRET_ACCESS", target, "observed access to a secret-like path", 0.96)
		if readonlyTool {
			addAlert("SEMANTIC_MISMATCH", target, fmt.Sprintf("tool %q looks read-only but secret-like data was accessed", event.GetToolName()), 0.98)
		}
	}

	if target, ok := extractWorkspaceEscapeTarget(event); ok {
		addAlert("WORKSPACE_ESCAPE", target, "observed file access outside the current workspace / cwd boundary", 0.95)
		if readonlyTool {
			addAlert("SEMANTIC_MISMATCH", target, fmt.Sprintf("tool %q crossed the workspace boundary", event.GetToolName()), 0.97)
		}
	}

	if target, reason, ok := detectSuspiciousShellTransport(event); ok {
		addAlert("SUSPICIOUS_SHELL_PIPELINE", target, reason, 0.97)
		if readonlyTool {
			addAlert("TOOL_BEHAVIOR_DRIFT", target, fmt.Sprintf("tool %q spawned a suspicious shell transport", event.GetToolName()), 0.98)
		}
	}

	if target, ok := recentExecutableAfterChmod(event, now); ok {
		addAlert("TOOL_BEHAVIOR_DRIFT", target, "observed chmod+x followed by execution within the same agent context", 0.95)
		if readonlyTool {
			addAlert("SEMANTIC_MISMATCH", target, fmt.Sprintf("tool %q created an executable payload and ran it", event.GetToolName()), 0.98)
		}
	}

	if endpoint, ok := extractNetworkTarget(event); ok && networkEgressShouldAlert(event) {
		addAlert("UNEXPECTED_NETWORK_EGRESS", endpoint, "observed outbound network activity", 0.93)
		if readonlyTool {
			addAlert("SEMANTIC_MISMATCH", endpoint, fmt.Sprintf("tool %q looks read-only but opened a network egress path", event.GetToolName()), 0.97)
		}
		if secretTarget, secretSeen := semanticAlertsState.recentSecretTarget(event, now); secretSeen {
			addAlert("TOKEN_EXFIL_RISK", secretTarget, fmt.Sprintf("secret-like data at %q was accessed before outbound network activity to %q", secretTarget, endpoint), 0.99)
		}
	}

	if reason, ok := riskyChildProcessReason(event); ok {
		target := strings.TrimSpace(event.GetComm())
		if target == "" {
			target = strings.TrimSpace(event.GetPath())
		}
		addAlert("UNEXPECTED_CHILD_PROCESS", target, reason, 0.94)
		if readonlyTool {
			addAlert("TOOL_BEHAVIOR_DRIFT", target, fmt.Sprintf("tool %q spawned %q (%s)", event.GetToolName(), target, reason), 0.97)
		}
	}

	if target, ok := observeForkStorm(event, now); ok {
		addAlert("RESOURCE_WASTING_LOOP", target, "observed repeated fork/clone activity suggesting a lightweight fork storm or runaway loop", 0.94)
	}

	// Codex-specific workflow semantic checks
	if reason, ok := detectPRReviewAnomaly(event); ok {
		addAlert("SEMANTIC_MISMATCH", firstNonEmpty(event.GetToolCallId(), event.GetPath()), reason, 0.96)
	}
	if reason, ok := detectBrowserTaskAnomaly(event); ok {
		addAlert("TOOL_BEHAVIOR_DRIFT", firstNonEmpty(event.GetComm(), event.GetPath()), reason, 0.97)
	}
	if reason, ok := detectIDEHandoffAnomaly(event); ok {
		addAlert("SEMANTIC_MISMATCH", event.GetPath(), reason, 0.98)
	}
	if reason, ok := detectRemoteDevboxAnomaly(event); ok {
		addAlert("UNEXPECTED_NETWORK_EGRESS", event.GetNetEndpoint(), reason, 0.96)
	}

	// Per-tool baseline drift detection
	if event.GetToolName() != "" && event.GetComm() != "" {
		if reason, ok := toolBaseline.detectDrift(event.GetToolName(), event.GetComm(), event.GetType()); ok {
			addAlert("TOOL_BEHAVIOR_DRIFT", firstNonEmpty(event.GetComm(), event.GetPath()), reason, 0.91)
		}
	}

	return alerts
}

func newSemanticAlertEvent(source *pb.Event, code, target, reason string, minimumRisk float64) *pb.Event {
	risk := source.GetRiskScore()
	if risk < minimumRisk {
		risk = minimumRisk
	}
	return &pb.Event{
		Pid:            source.GetPid(),
		Ppid:           source.GetPpid(),
		Uid:            source.GetUid(),
		Gid:            source.GetGid(),
		Type:           "semantic_alert",
		EventType:      pb.EventType_SEMANTIC_ALERT,
		Tag:            "Security",
		Comm:           code,
		Path:           target,
		ExtraInfo:      fmt.Sprintf("source=%s tool=%s comm=%s reason=%s", source.GetType(), source.GetToolName(), source.GetComm(), reason),
		SchemaVersion:  eventSchemaVersion,
		CgroupId:       source.GetCgroupId(),
		RootAgentPid:   source.GetRootAgentPid(),
		AgentRunId:     source.GetAgentRunId(),
		TaskId:         source.GetTaskId(),
		ConversationId: source.GetConversationId(),
		TurnId:         source.GetTurnId(),
		ToolCallId:     source.GetToolCallId(),
		ToolName:       source.GetToolName(),
		TraceId:        source.GetTraceId(),
		SpanId:         source.GetSpanId(),
		Decision:       "ALERT",
		RiskScore:      risk,
		ContainerId:    source.GetContainerId(),
		ArgvDigest:     source.GetArgvDigest(),
		Cwd:            source.GetCwd(),
		NetEndpoint:    source.GetNetEndpoint(),
		NetDirection:   source.GetNetDirection(),
		NetFamily:      source.GetNetFamily(),
	}
}

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

func semanticAlertContextKey(event *pb.Event) string {
	if event == nil {
		return ""
	}
	taskTraceKey := ""
	if taskID := strings.TrimSpace(event.GetTaskId()); taskID != "" || strings.TrimSpace(event.GetTraceId()) != "" {
		taskTraceKey = taskID + "|" + strings.TrimSpace(event.GetTraceId())
	}
	return firstNonEmpty(
		strings.TrimSpace(event.GetToolCallId()),
		taskTraceKey,
		strings.TrimSpace(event.GetAgentRunId()),
		fmt.Sprintf("pid:%d", event.GetRootAgentPid()),
		fmt.Sprintf("pid:%d", event.GetPid()),
	)
}

func (s *semanticAlertState) rememberSecret(event *pb.Event, target string, now time.Time) {
	if s == nil {
		return
	}
	key := semanticAlertContextKey(event)
	if key == "" {
		return
	}
	s.mu.Lock()
	s.recentSecrets[key] = semanticSecretObservation{SeenAt: now, Target: strings.TrimSpace(target)}
	s.mu.Unlock()
}

func (s *semanticAlertState) recentSecretTarget(event *pb.Event, now time.Time) (string, bool) {
	if s == nil {
		return "", false
	}
	key := semanticAlertContextKey(event)
	if key == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	observation, ok := s.recentSecrets[key]
	if !ok {
		return "", false
	}
	if now.Sub(observation.SeenAt) > semanticSecretCorrelationTTL {
		delete(s.recentSecrets, key)
		return "", false
	}
	return observation.Target, observation.Target != ""
}

func (s *semanticAlertState) rememberExecutable(event *pb.Event, path, mode string, now time.Time) {
	if s == nil {
		return
	}
	key := semanticAlertContextKey(event)
	if key == "" || strings.TrimSpace(path) == "" {
		return
	}
	s.mu.Lock()
	s.recentExecs[key] = semanticExecObservation{SeenAt: now, Path: filepath.Clean(path), Mode: mode}
	s.mu.Unlock()
}

func (s *semanticAlertState) recentExecutablePath(key, path string, now time.Time) (string, bool) {
	if s == nil || key == "" || strings.TrimSpace(path) == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	observation, ok := s.recentExecs[key]
	if !ok {
		return "", false
	}
	if now.Sub(observation.SeenAt) > semanticExecCorrelationTTL {
		delete(s.recentExecs, key)
		return "", false
	}
	cleanPath := filepath.Clean(path)
	return observation.Path, observation.Path == cleanPath
}

func (s *semanticAlertState) incrementForkCount(event *pb.Event, now time.Time) int {
	if s == nil {
		return 0
	}
	key := semanticAlertContextKey(event)
	if key == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	observation := s.forkWindows[key]
	if observation.WindowStart.IsZero() || now.Sub(observation.WindowStart) > semanticForkWindow {
		observation = semanticForkObservation{WindowStart: now, Count: 1}
	} else {
		observation.Count++
	}
	s.forkWindows[key] = observation
	return observation.Count
}

// Codex-specific workflow semantic checks

func detectPRReviewAnomaly(event *pb.Event) (string, bool) {
	if !toolNameMatchesHints(event.GetToolName(), prReviewToolHints) {
		return "", false
	}
	switch event.GetType() {
	case "execve", "process_exec":
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

func detectBrowserTaskAnomaly(event *pb.Event) (string, bool) {
	if !toolNameMatchesHints(event.GetToolName(), browserFrontendToolHints) {
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
	if !toolNameMatchesHints(event.GetToolName(), ideHandoffToolHints) {
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
	if !toolNameMatchesHints(event.GetToolName(), remoteDevboxToolHints) {
		return "", false
	}
	switch event.GetType() {
	case "network_connect", "network_sendto":
		endpoint := strings.TrimSpace(event.GetNetEndpoint())
		if isNonLocalhostEndpoint(endpoint) {
			// For remote devbox tools, network egress is expected but monitor for suspicious endpoints
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
