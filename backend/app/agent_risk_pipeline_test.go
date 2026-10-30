package app

import (
	"testing"
	"time"

	"agent-ebpf-filter/internal/agentpipeline"
	"agent-ebpf-filter/pb"
)

func TestUnifiedAgentRiskFromNativeHookToSyscall(t *testing.T) {
	old := unifiedAgentRiskPipeline
	unifiedAgentRiskPipeline = agentpipeline.New(16, time.Minute)
	t.Cleanup(func() { unifiedAgentRiskPipeline = old })
	pre := &pb.Event{
		Pid: 1001, RootAgentPid: 1001, Type: "native_hook",
		AgentRunId: "run-A", TaskId: "task", ToolCallId: "call-1", ToolName: "read",
		ExtraInfo: "hook_event=PreToolUse side_effect_scope=read_only",
	}
	if got := buildUnifiedAgentRiskAlerts(pre); len(got) != 0 {
		t.Fatalf("tool declaration emitted alerts: %v", got)
	}
	exec := &pb.Event{
		Pid: 1002, RootAgentPid: 1001, Type: "execve", Path: "/bin/sh",
		AgentRunId: "run-A", TaskId: "task", ToolCallId: "call-1",
	}
	alerts := buildUnifiedAgentRiskAlerts(exec)
	if len(alerts) != 1 || alerts[0].GetComm() != "TOOL_SCOPE_VIOLATION" ||
		alerts[0].GetRiskScore() < 96 || alerts[0].GetDecision() != "ALERT" {
		t.Fatalf("missing normalized alert: %+v", alerts)
	}
	exec.AgentRunId = "run-B"
	if got := buildUnifiedAgentRiskAlerts(exec); len(got) != 0 {
		t.Fatalf("cross-Agent alert: %v", got)
	}
}

func TestUnifiedAgentRiskSkipsBlockedSideEffects(t *testing.T) {
	old := unifiedAgentRiskPipeline
	unifiedAgentRiskPipeline = agentpipeline.New(16, time.Minute)
	t.Cleanup(func() { unifiedAgentRiskPipeline = old })
	pre := &pb.Event{Pid: 10, Type: "native_hook", ToolCallId: "x", ExtraInfo: "hook_event=tool_call permission_mode=plan"}
	buildUnifiedAgentRiskAlerts(pre)
	blocked := &pb.Event{Pid: 10, Type: "network_connect", ToolCallId: "x", NetEndpoint: "example.com:443", Decision: "BLOCK"}
	if got := buildUnifiedAgentRiskAlerts(blocked); len(got) != 0 {
		t.Fatalf("blocked event should not be a side effect: %v", got)
	}
}
