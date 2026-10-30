package app

import (
	"sync"
	"strings"
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/internal/agentpipeline"
	"agent-ebpf-filter/pb"
)

// The existing single-consumer broadcaster is the message bus. This stage
// consumes its admitted, attributed and monitored events in causal order.
var unifiedAgentRiskPipeline = agentpipeline.New(4096, 5*time.Minute)
var unifiedAgentRiskPipelineMu sync.RWMutex

func agentRiskPipelineSnapshot() agentpipeline.Status {
	unifiedAgentRiskPipelineMu.RLock()
	p := unifiedAgentRiskPipeline
	unifiedAgentRiskPipelineMu.RUnlock()
	return p.Stats()
}

func resetUnifiedAgentRiskPipeline() {
	unifiedAgentRiskPipelineMu.Lock()
	unifiedAgentRiskPipeline = agentpipeline.New(4096, 5*time.Minute)
	unifiedAgentRiskPipelineMu.Unlock()
}

func buildUnifiedAgentRiskAlerts(event *pb.Event) []*pb.Event {
	if event == nil || event.GetType() == "semantic_alert" {
		return nil
	}
	msg := normalizeAgentRiskMessage(event)
	// The hot path must not allocate per-session state for benign file reads
	// or unrelated kernel samples. Only messages used by correlation enter it.
	if msg.Kind == "tool_call" && msg.ToolCallID == "" {
		return nil
	}
	if msg.Kind != "tool_call" && msg.Kind != "tool_result" && msg.Kind != "model_input" && msg.Kind != "session_end" &&
		msg.Action != "write" && msg.Action != "execute" && msg.Action != "egress" {
		return nil
	}
	unifiedAgentRiskPipelineMu.RLock()
	pipeline := unifiedAgentRiskPipeline
	unifiedAgentRiskPipelineMu.RUnlock()
	findings := pipeline.Process(msg)
	if len(findings) == 0 {
		return nil
	}
	alerts := make([]*pb.Event, 0, len(findings))
	for _, finding := range findings {
		alerts = append(alerts, events.NewUnifiedRiskAlert(event, finding.Code, finding.Target, finding.Reason, finding.RiskScore))
	}
	return alerts
}

func normalizeAgentRiskMessage(event *pb.Event) agentpipeline.Message {
	m := agentpipeline.Message{
		At: time.Now().UTC(),
		Kind: "other",
		AgentRunID: event.GetAgentRunId(),
		ConversationID: event.GetConversationId(),
		TaskID: event.GetTaskId(),
		TurnID: event.GetTurnId(),
		TraceID: event.GetTraceId(),
		ToolCallID: event.GetToolCallId(),
		ToolName: event.GetToolName(),
		RootPID: event.GetRootAgentPid(),
		PID: event.GetPid(),
		Target: event.GetPath(),
	}
	// Never treat unsuccessful or blocked syscalls as completed side effects.
	blocked := strings.EqualFold(event.GetDecision(), "BLOCK") || event.GetRetval() < 0
	switch event.GetType() {
	case "native_hook":
		switch strings.ToLower(agentRiskExtraField(event.GetExtraInfo(), "hook_event")) {
		case "pretooluse", "beforetool", "pretool", "tool_call", "before_tool_call", "pretoolusehook", "before_tool_use":
			m.Kind = "tool_call"
		case "posttooluse", "posttoolusefailure", "aftertool", "tool_result", "after_tool_call", "after_tool_use", "post_tool_use":
			m.Kind = "tool_result"
		case "userpromptsubmit", "userpromptsubmitted", "prompt_submit", "prompt", "beforeagent", "beforemodel":
			m.Kind = "model_input"
		case "sessionend", "session_end", "sessionstop", "session_stop":
			m.Kind = "session_end"
		}
		m.SideEffectScope = agentRiskExtraField(event.GetExtraInfo(), "side_effect_scope")
		m.PermissionMode = agentRiskExtraField(event.GetExtraInfo(), "permission_mode")
	case "mcp_call", "mcp_tool_call", "mcp_request":
		m.Kind = "tool_call"
	case "mcp_result", "mcp_tool_result", "mcp_response":
		m.Kind = "tool_result"
	case "tls_plaintext":
		m.Kind = "model_input"
		if strings.EqualFold(agentRiskExtraField(event.GetExtraInfo(), "role"), "tool") {
			m.Kind = "tool_result"
		}
	case "process_exit", "exit":
		// A child exit must not terminate its parent Agent's entire run.
		if (event.GetRootAgentPid() != 0 && event.GetPid() == event.GetRootAgentPid()) ||
			(event.GetRootAgentPid() == 0 && event.GetAgentRunId() == "" && event.GetConversationId() == "" && event.GetTraceId() == "") {
			m.Kind = "session_end"
		}
	case "execve", "process_exec":
		m.Kind = "process"
		if !blocked {
			m.Action = "execute"
		}
	case "open", "openat", "read":
		m.Kind = "file"
		m.Action = "read"
	case "write", "chmod", "chown", "rename", "link", "symlink", "mknod", "mkdir", "unlink", "unlinkat":
		m.Kind = "file"
		// Zero-byte writes do not demonstrate an actual data mutation.
		if !blocked && (event.GetType() != "write" || event.GetRetval() > 0) {
			m.Action = "write"
		}
	case "network_connect", "network_sendto", "tcp_connect":
		m.Kind = "network"
		m.Target = event.GetNetEndpoint()
		if !blocked {
			m.Action = "egress"
		}
	}
	// Kernel file/process/network records have no model-input provenance.
	// Do not scan metadata on the high-volume syscall path.
	if m.Kind != "tool_call" && m.Kind != "tool_result" && m.Kind != "model_input" {
		return m
	}
	source := strings.ToLower(agentRiskExtraField(event.GetExtraInfo(), "source"))
	role := strings.ToLower(agentRiskExtraField(event.GetExtraInfo(), "role"))
	risk := strings.ToLower(agentRiskExtraField(event.GetExtraInfo(), "risk_level"))
	m.UntrustedInput = source == "external" || source == "untrusted" || source == "tool_output" || role == "tool"
	m.HighRiskInput = risk == "high" || risk == "critical" ||
		strings.EqualFold(agentRiskExtraField(event.GetExtraInfo(), "prompt_injection_detected"), "true")
	return m
}

// Read only bounded, single-token metadata emitted by source adapters. This
// parser does not scan or persist prompt, response, tool input or TLS bodies.
func agentRiskExtraField(info, field string) string {
	if len(info) > 4096 {
		info = info[:4096]
	}
	needle := field + "="
	for _, token := range strings.Fields(info) {
		if strings.HasPrefix(token, needle) {
			return strings.TrimPrefix(token, needle)
		}
	}
	return ""
}
