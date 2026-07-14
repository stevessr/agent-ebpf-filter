package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ---- moved from app/alerts_semantic.go ----

var SecretPathHints = []string{
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

var ReadOnlyToolHints = []string{
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

var PRReviewToolHints = []string{
	"review",
	"pr_",
	"pull_request",
	"diff",
	"patch",
	"approve",
}

var BrowserFrontendToolHints = []string{
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

var IDEHandoffToolHints = []string{
	"handoff",
	"ide_",
	"open_in_",
	"editor",
}

var RemoteDevboxToolHints = []string{
	"devbox",
	"remote_",
	"ssh_",
	"ssm_",
}

var RiskyExecComms = map[string]string{
	"curl":   "network download tool",
	"wget":   "network download tool",
	"nc":     "raw network tool",
	"netcat": "raw network tool",
	"socat":  "socket relay tool",
	"ssh":    "remote shell tool",
	"scp":    "remote copy tool",
	"rsync":  "remote sync tool",
}

var ExpectedNetworkHints = []string{
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

var WorkspaceEscapeHints = []string{
	"/etc/",
	"/root/",
	"/proc/",
	"/sys/",
	"/dev/",
	"/var/run/docker.sock",
}

type SemanticSecretObservation struct {
	SeenAt time.Time
	Target string
}

type SemanticExecObservation struct {
	SeenAt time.Time
	Path   string
	Mode   string
}

type SemanticForkObservation struct {
	WindowStart time.Time
	Count       int
}

type SemanticAgenticLoopObservation struct {
	WindowStart   time.Time
	PromptDigest  string
	PromptRepeats int
	APICalls      int
	FileOps       int
	LastTarget    string
	Alerted       bool
}

type SemanticFileMutationObservation struct {
	SeenAt time.Time
	Actor  string
	Op     string
	Path   string
}

type SemanticAlertState struct {
	mu                            sync.Mutex
	recentSecrets                 *boundedSemanticStateMap[SemanticSecretObservation]
	recentExecs                   *boundedSemanticStateMap[SemanticExecObservation]
	forkWindows                   *boundedSemanticStateMap[SemanticForkObservation]
	agenticLoopWindows            *boundedSemanticStateMap[SemanticAgenticLoopObservation]
	recentFileMutations           *boundedSemanticStateMap[SemanticFileMutationObservation]
	expiredEvictionsTotal         uint64
	capacityEvictionsTotal        uint64
	truncatedStateValuesTotal     uint64
	ignoredOversizedMetadataTotal uint64
	lastSweepAt                   time.Time
}

func NewSemanticAlertState() *SemanticAlertState {
	return &SemanticAlertState{
		recentSecrets:       newBoundedSemanticStateMap[SemanticSecretObservation](SemanticStateMaxContextEntries),
		recentExecs:         newBoundedSemanticStateMap[SemanticExecObservation](SemanticStateMaxContextEntries),
		forkWindows:         newBoundedSemanticStateMap[SemanticForkObservation](SemanticStateMaxContextEntries),
		agenticLoopWindows:  newBoundedSemanticStateMap[SemanticAgenticLoopObservation](SemanticStateMaxContextEntries),
		recentFileMutations: newBoundedSemanticStateMap[SemanticFileMutationObservation](SemanticStateMaxFileEntries),
	}
}

func (s *SemanticAlertState) RememberSecret(event *pb.Event, target string, now time.Time) {
	if s == nil {
		return
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return
	}
	target, targetTruncated := boundSemanticStateString(target, SemanticStateMaxValueBytes)
	s.mu.Lock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated, targetTruncated)
	s.noteCapacityEvictionLocked(s.recentSecrets.Set(key, SemanticSecretObservation{SeenAt: now, Target: target}))
	s.mu.Unlock()
}

func (s *SemanticAlertState) RecentSecretTarget(event *pb.Event, now time.Time) (string, bool) {
	if s == nil {
		return "", false
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated)
	observation, ok := s.recentSecrets.Get(key)
	if !ok {
		return "", false
	}
	if semanticStateExpired(now, observation.SeenAt, SemanticSecretCorrelationTTL) {
		s.recentSecrets.Delete(key)
		s.expiredEvictionsTotal++
		return "", false
	}
	return observation.Target, observation.Target != ""
}

func (s *SemanticAlertState) RememberExecutable(event *pb.Event, path, mode string, now time.Time) {
	if s == nil {
		return
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" || strings.TrimSpace(path) == "" {
		return
	}
	path, pathTruncated := boundSemanticStateString(path, SemanticStateMaxPathBytes)
	mode, modeTruncated := boundSemanticStateString(mode, SemanticStateMaxModeBytes)
	s.mu.Lock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated, pathTruncated, modeTruncated)
	s.noteCapacityEvictionLocked(s.recentExecs.Set(key, SemanticExecObservation{SeenAt: now, Path: filepath.Clean(path), Mode: mode}))
	s.mu.Unlock()
}

func (s *SemanticAlertState) RecentExecutablePath(key, path string, now time.Time) (string, bool) {
	if s == nil || key == "" || strings.TrimSpace(path) == "" {
		return "", false
	}
	key, keyTruncated := boundSemanticStateString(key, SemanticStateMaxContextBytes)
	path, pathTruncated := boundSemanticStateString(path, SemanticStateMaxPathBytes)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated, pathTruncated)
	observation, ok := s.recentExecs.Get(key)
	if !ok {
		return "", false
	}
	if semanticStateExpired(now, observation.SeenAt, SemanticExecCorrelationTTL) {
		s.recentExecs.Delete(key)
		s.expiredEvictionsTotal++
		return "", false
	}
	cleanPath := filepath.Clean(path)
	return observation.Path, observation.Path == cleanPath
}

func (s *SemanticAlertState) IncrementForkCount(event *pb.Event, now time.Time) int {
	if s == nil {
		return 0
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated)
	observation, exists := s.forkWindows.Get(key)
	if !exists || semanticStateExpired(now, observation.WindowStart, SemanticForkWindow) {
		if exists {
			s.expiredEvictionsTotal++
		}
		observation = SemanticForkObservation{WindowStart: now, Count: 1}
	} else {
		observation.Count++
	}
	s.noteCapacityEvictionLocked(s.forkWindows.Set(key, observation))
	return observation.Count
}

func (s *SemanticAlertState) ObserveAgenticResourceLoop(event *pb.Event, now time.Time) (string, string, bool) {
	if s == nil || event == nil {
		return "", "", false
	}
	promptDigest, oversizedMetadata := extraInfoFieldBounded(event.GetExtraInfo(), "prompt_digest", SemanticPromptDigestMaxBytes)
	apiLike := isAPILikeNetworkEvent(event)
	fileIO := isLowValueFileIOEvent(event)
	if oversizedMetadata {
		s.mu.Lock()
		s.ignoredOversizedMetadataTotal++
		s.mu.Unlock()
	}
	if promptDigest == "" && !apiLike && !fileIO {
		return "", "", false
	}
	key, keyTruncated := semanticAlertContextKeyBounded(event)
	if key == "" {
		return "", "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(keyTruncated)

	observation, exists := s.agenticLoopWindows.Get(key)
	if !exists || semanticStateExpired(now, observation.WindowStart, SemanticAgenticLoopWindow) {
		if exists {
			s.expiredEvictionsTotal++
		}
		observation = SemanticAgenticLoopObservation{WindowStart: now}
	}

	if promptDigest != "" {
		if observation.PromptDigest == promptDigest {
			observation.PromptRepeats++
		} else {
			observation.PromptDigest = promptDigest
			observation.PromptRepeats = 1
			observation.Alerted = false
		}
		observation.LastTarget, _ = boundSemanticStateString("prompt:"+promptDigest, SemanticStateMaxValueBytes)
	}
	if apiLike {
		observation.APICalls++
		target, truncated := boundSemanticStateString(
			platform.FirstNonEmpty(event.GetNetEndpoint(), event.GetSni(), event.GetHttpHost(), event.GetDnsName(), event.GetPath()),
			SemanticStateMaxValueBytes,
		)
		s.noteTruncationsLocked(truncated)
		observation.LastTarget = target
	}
	if fileIO {
		observation.FileOps++
		if target := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath(), event.GetComm()); target != "" {
			boundedTarget, truncated := boundSemanticStateString(target, SemanticStateMaxValueBytes)
			s.noteTruncationsLocked(truncated)
			observation.LastTarget = boundedTarget
		}
	}

	if !observation.Alerted &&
		observation.PromptRepeats >= SemanticPromptLoopThreshold &&
		observation.APICalls >= SemanticAPILoopThreshold &&
		observation.FileOps >= SemanticFileIOLoopThreshold {
		observation.Alerted = true
		s.noteCapacityEvictionLocked(s.agenticLoopWindows.Set(key, observation))
		target := platform.FirstNonEmpty(observation.LastTarget, observation.PromptDigest, key)
		reason := fmt.Sprintf("observed repeated prompt metadata (%d repeats) with %d API egress events and %d low-level file I/O events within %s",
			observation.PromptRepeats, observation.APICalls, observation.FileOps, SemanticAgenticLoopWindow)
		return target, reason, true
	}

	s.noteCapacityEvictionLocked(s.agenticLoopWindows.Set(key, observation))
	return "", "", false
}

func (s *SemanticAlertState) ObserveMultiAgentFileContention(event *pb.Event, now time.Time) (string, string, bool) {
	if s == nil || event == nil {
		return "", "", false
	}
	path, pathTruncated, ok := semanticFileMutationPath(event)
	if !ok {
		return "", "", false
	}
	actor, actorTruncated := semanticAgentIdentity(event)
	if actor == "" {
		return "", "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	s.noteTruncationsLocked(pathTruncated, actorTruncated)

	previous, seen := s.recentFileMutations.Get(path)
	current := SemanticFileMutationObservation{
		SeenAt: now,
		Actor:  actor,
		Op:     event.GetType(),
		Path:   path,
	}
	s.noteCapacityEvictionLocked(s.recentFileMutations.Set(path, current))
	if seen && semanticStateExpired(now, previous.SeenAt, SemanticFileContentionTTL) {
		s.expiredEvictionsTotal++
		seen = false
	}
	if !seen || previous.Actor == "" || previous.Actor == actor {
		return "", "", false
	}

	reason := fmt.Sprintf("agent context %s performed %s on a path touched by %s via %s within %s",
		actor, event.GetType(), previous.Actor, previous.Op, SemanticFileContentionTTL)
	return path, reason, true
}

func (s *SemanticAlertState) EvictExpired(now time.Time) SemanticAlertStateStatus {
	if s == nil {
		return SemanticAlertStateStatus{MaxEntries: SemanticStateMaxEntries}
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	evicted := 0
	evicted += s.recentSecrets.DeleteIf(func(value SemanticSecretObservation) bool {
		return semanticStateExpired(now, value.SeenAt, SemanticSecretCorrelationTTL)
	})
	evicted += s.recentExecs.DeleteIf(func(value SemanticExecObservation) bool {
		return semanticStateExpired(now, value.SeenAt, SemanticExecCorrelationTTL)
	})
	evicted += s.forkWindows.DeleteIf(func(value SemanticForkObservation) bool {
		return semanticStateExpired(now, value.WindowStart, SemanticForkWindow)
	})
	evicted += s.agenticLoopWindows.DeleteIf(func(value SemanticAgenticLoopObservation) bool {
		return semanticStateExpired(now, value.WindowStart, SemanticAgenticLoopWindow)
	})
	evicted += s.recentFileMutations.DeleteIf(func(value SemanticFileMutationObservation) bool {
		return semanticStateExpired(now, value.SeenAt, SemanticFileContentionTTL)
	})
	s.expiredEvictionsTotal += uint64(evicted)
	s.lastSweepAt = now
	return s.statusLocked()
}

func (s *SemanticAlertState) Status() SemanticAlertStateStatus {
	if s == nil {
		return SemanticAlertStateStatus{MaxEntries: SemanticStateMaxEntries}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureMapsLocked()
	return s.statusLocked()
}

func (s *SemanticAlertState) statusLocked() SemanticAlertStateStatus {
	status := SemanticAlertStateStatus{
		RecentSecrets:                 s.recentSecrets.Len(),
		RecentExecutables:             s.recentExecs.Len(),
		ForkWindows:                   s.forkWindows.Len(),
		AgenticLoopWindows:            s.agenticLoopWindows.Len(),
		RecentFileMutations:           s.recentFileMutations.Len(),
		MaxEntries:                    SemanticStateMaxEntries,
		ExpiredEvictionsTotal:         s.expiredEvictionsTotal,
		CapacityEvictionsTotal:        s.capacityEvictionsTotal,
		TruncatedStateValuesTotal:     s.truncatedStateValuesTotal,
		IgnoredOversizedMetadataTotal: s.ignoredOversizedMetadataTotal,
		LastSweepAt:                   s.lastSweepAt,
	}
	status.Entries = status.RecentSecrets + status.RecentExecutables + status.ForkWindows + status.AgenticLoopWindows + status.RecentFileMutations
	return status
}

func (s *SemanticAlertState) ensureMapsLocked() {
	if s.recentSecrets == nil {
		s.recentSecrets = newBoundedSemanticStateMap[SemanticSecretObservation](SemanticStateMaxContextEntries)
	}
	if s.recentExecs == nil {
		s.recentExecs = newBoundedSemanticStateMap[SemanticExecObservation](SemanticStateMaxContextEntries)
	}
	if s.forkWindows == nil {
		s.forkWindows = newBoundedSemanticStateMap[SemanticForkObservation](SemanticStateMaxContextEntries)
	}
	if s.agenticLoopWindows == nil {
		s.agenticLoopWindows = newBoundedSemanticStateMap[SemanticAgenticLoopObservation](SemanticStateMaxContextEntries)
	}
	if s.recentFileMutations == nil {
		s.recentFileMutations = newBoundedSemanticStateMap[SemanticFileMutationObservation](SemanticStateMaxFileEntries)
	}
}

func (s *SemanticAlertState) noteCapacityEvictionLocked(evicted bool) {
	if evicted {
		s.capacityEvictionsTotal++
	}
}

func (s *SemanticAlertState) noteTruncationsLocked(values ...bool) {
	for _, truncated := range values {
		if truncated {
			s.truncatedStateValuesTotal++
		}
	}
}

func BuildSemanticAlerts(event *pb.Event) []*pb.Event {
	if event == nil || event.GetType() == "semantic_alert" {
		return nil
	}

	now := time.Now().UTC()
	readonlyTool := toolNameLooksReadOnly(event.GetToolName())
	var alerts []*pb.Event
	addAlert := func(code, target, reason string, minimumRisk float64) {
		for _, alert := range alerts {
			if alert.GetComm() == code {
				return
			}
		}
		alerts = append(alerts, newSemanticAlertEvent(event, code, target, reason, minimumRisk))
	}

	if target, ok := extractSecretTarget(event); ok {
		Deps.SemanticAlertsState.RememberSecret(event, target, now)
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
		if secretTarget, secretSeen := Deps.SemanticAlertsState.RecentSecretTarget(event, now); secretSeen {
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

	if target, reason, ok := Deps.SemanticAlertsState.ObserveAgenticResourceLoop(event, now); ok {
		addAlert("RESOURCE_WASTING_LOOP", target, reason, 0.95)
	}

	if target, reason, ok := Deps.SemanticAlertsState.ObserveMultiAgentFileContention(event, now); ok {
		addAlert("MULTI_AGENT_FILE_CONTENTION", target, reason, 0.96)
	}

	// Codex-specific workflow semantic checks
	if reason, ok := detectPRReviewAnomaly(event); ok {
		addAlert("SEMANTIC_MISMATCH", platform.FirstNonEmpty(event.GetToolCallId(), event.GetPath()), reason, 0.96)
	}
	if reason, ok := detectBrowserTaskAnomaly(event); ok {
		addAlert("TOOL_BEHAVIOR_DRIFT", platform.FirstNonEmpty(event.GetComm(), event.GetPath()), reason, 0.97)
	}
	if reason, ok := detectIDEHandoffAnomaly(event); ok {
		addAlert("SEMANTIC_MISMATCH", event.GetPath(), reason, 0.98)
	}
	if reason, ok := detectRemoteDevboxAnomaly(event); ok {
		addAlert("UNEXPECTED_NETWORK_EGRESS", event.GetNetEndpoint(), reason, 0.96)
	}

	// Per-tool baseline drift detection
	if event.GetToolName() != "" && event.GetComm() != "" {
		if reason, ok := Deps.ToolBaselineDetectDrift(event.GetToolName(), event.GetComm(), event.GetType()); ok {
			addAlert("TOOL_BEHAVIOR_DRIFT", platform.FirstNonEmpty(event.GetComm(), event.GetPath()), reason, 0.91)
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
		Tgid:           source.GetTgid(),
		Ppid:           source.GetPpid(),
		Uid:            source.GetUid(),
		Gid:            source.GetGid(),
		Type:           "semantic_alert",
		EventType:      pb.EventType_SEMANTIC_ALERT,
		Tag:            "Security",
		Comm:           code,
		Path:           target,
		ExtraInfo:      fmt.Sprintf("source=%s tool=%s comm=%s reason=%s", source.GetType(), source.GetToolName(), source.GetComm(), reason),
		SchemaVersion:  Deps.EventSchemaVersion,
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

func ResetSemanticAlertState() {
	Deps.SemanticAlertsState = NewSemanticAlertState()
}
