package app

import (
	"agent-ebpf-filter/pb"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section alerts_semantic.go ----

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
	semanticAgenticLoopWindow    = 10 * time.Second
	semanticPromptLoopThreshold  = 3
	semanticAPILoopThreshold     = 3
	semanticFileIOLoopThreshold  = 3
	semanticFileContentionTTL    = 15 * time.Second
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

type semanticAgenticLoopObservation struct {
	WindowStart   time.Time
	PromptDigest  string
	PromptRepeats int
	APICalls      int
	FileOps       int
	LastTarget    string
	Alerted       bool
}

type semanticFileMutationObservation struct {
	SeenAt time.Time
	Actor  string
	Op     string
	Path   string
}

type semanticAlertState struct {
	mu                  sync.Mutex
	recentSecrets       map[string]semanticSecretObservation
	recentExecs         map[string]semanticExecObservation
	forkWindows         map[string]semanticForkObservation
	agenticLoopWindows  map[string]semanticAgenticLoopObservation
	recentFileMutations map[string]semanticFileMutationObservation
}

func newSemanticAlertState() *semanticAlertState {
	return &semanticAlertState{
		recentSecrets:       make(map[string]semanticSecretObservation),
		recentExecs:         make(map[string]semanticExecObservation),
		forkWindows:         make(map[string]semanticForkObservation),
		agenticLoopWindows:  make(map[string]semanticAgenticLoopObservation),
		recentFileMutations: make(map[string]semanticFileMutationObservation),
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

	if target, reason, ok := observeAgenticResourceLoop(event, now); ok {
		addAlert("RESOURCE_WASTING_LOOP", target, reason, 0.95)
	}

	if target, reason, ok := observeMultiAgentFileContention(event, now); ok {
		addAlert("MULTI_AGENT_FILE_CONTENTION", target, reason, 0.96)
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
