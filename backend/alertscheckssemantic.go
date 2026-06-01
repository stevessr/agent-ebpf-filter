package main

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
)

// Codex-specific workflow semantic checks

func detectPRReviewAnomaly(event *pb.Event) (string, bool) {
	if !toolNameMatchesHints(event.GetToolName(), prReviewToolHints) {
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

// ── Semantic alert state methods ──────────────────────────────────────

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

func (s *semanticAlertState) observeAgenticResourceLoop(event *pb.Event, now time.Time) (string, string, bool) {
	if s == nil || event == nil {
		return "", "", false
	}
	key := semanticAlertContextKey(event)
	if key == "" {
		return "", "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	observation := s.agenticLoopWindows[key]
	if observation.WindowStart.IsZero() || now.Sub(observation.WindowStart) > semanticAgenticLoopWindow {
		observation = semanticAgenticLoopObservation{WindowStart: now}
	}

	changed := false
	if digest := extraInfoField(event.GetExtraInfo(), "prompt_digest"); digest != "" {
		if observation.PromptDigest == digest {
			observation.PromptRepeats++
		} else {
			observation.PromptDigest = digest
			observation.PromptRepeats = 1
			observation.Alerted = false
		}
		observation.LastTarget = "prompt:" + digest
		changed = true
	}
	if isAPILikeNetworkEvent(event) {
		observation.APICalls++
		observation.LastTarget = firstNonEmpty(event.GetNetEndpoint(), event.GetSni(), event.GetHttpHost(), event.GetDnsName(), event.GetPath())
		changed = true
	}
	if isLowValueFileIOEvent(event) {
		observation.FileOps++
		if target := firstNonEmpty(event.GetPath(), event.GetExtraPath(), event.GetComm()); target != "" {
			observation.LastTarget = target
		}
		changed = true
	}

	if !changed {
		s.agenticLoopWindows[key] = observation
		return "", "", false
	}

	if !observation.Alerted &&
		observation.PromptRepeats >= semanticPromptLoopThreshold &&
		observation.APICalls >= semanticAPILoopThreshold &&
		observation.FileOps >= semanticFileIOLoopThreshold {
		observation.Alerted = true
		s.agenticLoopWindows[key] = observation
		target := firstNonEmpty(observation.LastTarget, observation.PromptDigest, key)
		reason := fmt.Sprintf("observed repeated prompt metadata (%d repeats) with %d API egress events and %d low-level file I/O events within %s",
			observation.PromptRepeats, observation.APICalls, observation.FileOps, semanticAgenticLoopWindow)
		return target, reason, true
	}

	s.agenticLoopWindows[key] = observation
	return "", "", false
}

func (s *semanticAlertState) observeMultiAgentFileContention(event *pb.Event, now time.Time) (string, string, bool) {
	if s == nil || event == nil {
		return "", "", false
	}
	path, ok := semanticFileMutationPath(event)
	if !ok {
		return "", "", false
	}
	actor := semanticAgentIdentity(event)
	if actor == "" {
		return "", "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	previous, seen := s.recentFileMutations[path]
	current := semanticFileMutationObservation{
		SeenAt: now,
		Actor:  actor,
		Op:     event.GetType(),
		Path:   path,
	}
	s.recentFileMutations[path] = current
	if !seen || previous.Actor == "" || previous.Actor == actor || now.Sub(previous.SeenAt) > semanticFileContentionTTL {
		return "", "", false
	}

	reason := fmt.Sprintf("agent context %s performed %s on a path touched by %s via %s within %s",
		actor, event.GetType(), previous.Actor, previous.Op, semanticFileContentionTTL)
	return path, reason, true
}
