package app

import (
	"testing"
	"time"

	"agent-ebpf-filter/internal/agentpipeline"
	"agent-ebpf-filter/pb"
)

func TestUnifiedRiskHookAliasAndSessionLifecycle(t *testing.T) {
	old := unifiedAgentRiskPipeline
	unifiedAgentRiskPipeline = agentpipeline.New(16, time.Minute)
	t.Cleanup(func() { unifiedAgentRiskPipeline = old })
	pre := &pb.Event{Type: "native_hook", Pid: 100, RootAgentPid: 100, AgentRunId: "run", ToolCallId: "call", ExtraInfo: "hook_event=BeforeTool side_effect_scope=read_only"}
	buildUnifiedAgentRiskAlerts(pre)
	if s := unifiedAgentRiskPipeline.Stats(); s.ActiveCalls != 1 {
		t.Fatalf("BeforeTool did not open a call: %+v", s)
	}
	end := &pb.Event{Type: "native_hook", Pid: 100, RootAgentPid: 100, AgentRunId: "run", ExtraInfo: "hook_event=SessionEnd"}
	buildUnifiedAgentRiskAlerts(end)
	if s := unifiedAgentRiskPipeline.Stats(); s.Sessions != 0 {
		t.Fatalf("SessionEnd left stale state: %+v", s)
	}
}

func TestUnifiedRiskWriteRequiresBytesAndCompletedAction(t *testing.T) {
	old := unifiedAgentRiskPipeline
	unifiedAgentRiskPipeline = agentpipeline.New(16, time.Minute)
	t.Cleanup(func() { unifiedAgentRiskPipeline = old })
	pre := &pb.Event{Type: "native_hook", Pid: 10, AgentRunId: "run", ToolCallId: "w", ExtraInfo: "hook_event=PreToolUse side_effect_scope=read_only"}
	buildUnifiedAgentRiskAlerts(pre)
	write := &pb.Event{Type: "write", Pid: 10, AgentRunId: "run", ToolCallId: "w", Retval: 0}
	if got := buildUnifiedAgentRiskAlerts(write); len(got) != 0 {
		t.Fatalf("zero-byte write inferred as side effect: %+v", got)
	}
	write.Retval = 5
	if got := buildUnifiedAgentRiskAlerts(write); len(got) != 1 || got[0].GetComm() != "TOOL_SCOPE_VIOLATION" {
		t.Fatalf("successful write did not correlate: %+v", got)
	}
}

func TestRootExitClearsAgentStateButChildExitDoesNot(t *testing.T) {
	old := unifiedAgentRiskPipeline
	unifiedAgentRiskPipeline = agentpipeline.New(16, time.Minute)
	t.Cleanup(func() { unifiedAgentRiskPipeline = old })
	buildUnifiedAgentRiskAlerts(&pb.Event{Type: "native_hook", Pid: 100, RootAgentPid: 100, AgentRunId: "run", ToolCallId: "t", ExtraInfo: "hook_event=tool_call side_effect_scope=read_only"})
	buildUnifiedAgentRiskAlerts(&pb.Event{Type: "process_exit", Pid: 200, RootAgentPid: 100, AgentRunId: "run"})
	if s := unifiedAgentRiskPipeline.Stats(); s.ActiveCalls != 1 {
		t.Fatalf("child exit removed parent run: %+v", s)
	}
	buildUnifiedAgentRiskAlerts(&pb.Event{Type: "process_exit", Pid: 100, RootAgentPid: 100, AgentRunId: "run"})
	if s := unifiedAgentRiskPipeline.Stats(); s.Sessions != 0 {
		t.Fatalf("root exit did not close run: %+v", s)
	}
}
