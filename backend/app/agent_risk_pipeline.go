package app

import (
	"strings"
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/internal/agentpipeline"
	"agent-ebpf-filter/pb"
)

// The existing single-consumer broadcaster is the message bus. This stage
// consumes its admitted, attributed and monitored events in causal order.
var unifiedAgentRiskPipeline = agentpipeline.New(4096, 5*time.Minute)

func resetUnifiedAgentRiskPipeline() {
	unifiedAgentRiskPipeline = agentpipeline.New(4096, 5*time.Minute)
}

func buildUnifiedAgentRiskAlerts(event *pb.Event) []*pb.Event {
	if event == nil || event.GetType() == "semantic_alert" {
		return nil
	}
	msg := normalizeAgentRiskMessage(event)
	if msg.Kind == "other" && msg.Action == "" {
		return nil
	}
	findings := unifiedAgentRiskPipeline.Process(msg)
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
		case "pretooluse", "beforetool", "pretool", "tool_call", "before_tool_call", "pretoolusehook":
			m.Kind = "tool_call"
		case "posttooluse", "posttoolusefailure", "aftertool", "tool_result", "after_tool_call":
			m.Kind = "tool_result"
		case "userpromptsubmit", "prompt_submit", "prompt", "beforeagent":
			m.Kind = "model_input"
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
		if !blocked {
			m.Action = "write"
		}
	case "network_connect", "network_sendto", "tcp_connect":
		m.Kind = "network"
		m.Target = event.GetNetEndpoint()
		if !blocked {
			m.Action = "egress"
		}
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
