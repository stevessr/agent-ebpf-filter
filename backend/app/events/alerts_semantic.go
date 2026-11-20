package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/internal/heuristics"
	"agent-ebpf-filter/pb"
	"fmt"
	"strings"
	"time"
)

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

// BuildSemanticAlerts retains the standard strict thresholds for unverified activity.
func BuildSemanticAlerts(event *pb.Event) []*pb.Event {
	return BuildSemanticAlertsWithCloneContext(event, false)
}

// BuildSemanticAlertsWithCloneContext keeps all independent security checks
// active while allowing a higher (not disabled) fork-storm threshold for an
// anonymously verified public clone associated with this exact process.
func BuildSemanticAlertsWithCloneContext(event *pb.Event, verifiedPublicClone bool) []*pb.Event {
	if event == nil || event.GetType() == "semantic_alert" {
		return nil
	}

	now := time.Now().UTC()
	readonlyTool := toolNameLooksReadOnly(event.GetToolName())
	// The secret-path scan is the most expensive predicate here; evaluate it
	// once for event.Path and share the verdict with every check below.
	pathIsSecret := isSecretLikePath(event.GetPath())
	var alerts []*pb.Event
	addAlert := func(code, target, reason string, minimumRisk float64) {
		for _, alert := range alerts {
			if alert.GetComm() == code {
				return
			}
		}
		alerts = append(alerts, newSemanticAlertEvent(event, code, target, reason, minimumRisk))
	}

	if target, ok := extractSecretTargetWith(event, pathIsSecret); ok {
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

	// Post-factum kernel exec evidence: alert only; no kernel enforcement is claimed.
	if event.GetType() == "execve" || event.GetType() == "execveat" || event.GetType() == "process_exec" {
		target := strings.TrimSpace(event.GetPath())
		if target == "" {
			target = strings.TrimSpace(event.GetComm())
		}
		if finding, ok := heuristics.DetectWebShellSpawn(target, nil, event.GetPpid()); ok {
			addAlert(finding.Code, target, finding.Reason(), 0.98)
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

	forkThreshold := SemanticForkStormThreshold
	if verifiedPublicClone {
		forkThreshold *= 8
	}
	if target, ok := observeForkStorm(event, now, forkThreshold, verifiedPublicClone); ok {
		addAlert("RESOURCE_WASTING_LOOP", target, "observed repeated fork/clone activity suggesting a lightweight fork storm or runaway loop", 0.94)
	}

	if target, reason, ok := Deps.SemanticAlertsState.observeAgenticResourceLoop(event, now, isLowValueFileIOEventWith(event, pathIsSecret)); ok {
		addAlert("RESOURCE_WASTING_LOOP", target, reason, 0.95)
	}

	if target, reason, ok := Deps.SemanticAlertsState.ObserveMultiAgentFileContention(event, now); ok {
		// Interleaved writes alone are investigation evidence, not proof of
		// simultaneous modification or malicious data races.
		addAlert("MULTI_AGENT_FILE_CONTENTION", target, reason, 0.70)
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
		if Deps.ToolBaselineAssess != nil {
			if reason, riskFloor, ok := Deps.ToolBaselineAssess(event.GetToolName(), event.GetComm(), event.GetType()); ok {
				addAlert("TOOL_BEHAVIOR_DRIFT", platform.FirstNonEmpty(event.GetComm(), event.GetPath()), reason, riskFloor)
			}
		} else if Deps.ToolBaselineObserve != nil {
			if reason, ok := Deps.ToolBaselineObserve(event.GetToolName(), event.GetComm(), event.GetType()); ok {
				addAlert("TOOL_BEHAVIOR_DRIFT", platform.FirstNonEmpty(event.GetComm(), event.GetPath()), reason, 0.91)
			}
		}
	}

	return alerts
}

func newSemanticAlertEvent(source *pb.Event, code, target, reason string, minimumRisk float64) *pb.Event {
	// The semantic rule thresholds are probabilities (0..1), while protobuf
	// Event.RiskScore and all dashboard severity bands use a 0..100 score.
	// Comparing them directly kept weak source scores (for example 12) on
	// high-confidence semantic ALERT events instead of the intended 96.
	minRiskScore := minimumRisk * 100
	risk := source.GetRiskScore()
	if risk < minRiskScore {
		risk = minRiskScore
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
