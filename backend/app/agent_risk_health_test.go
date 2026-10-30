package app

import (
	"testing"
	"time"

	"agent-ebpf-filter/internal/agentpipeline"
	"agent-ebpf-filter/pb"
)

func TestCollectorHealthReportsAgentRiskPipelineStatus(t *testing.T) {
	old := unifiedAgentRiskPipeline
	unifiedAgentRiskPipeline = agentpipeline.New(8, time.Minute)
	t.Cleanup(func() { unifiedAgentRiskPipeline = old })
	buildUnifiedAgentRiskAlerts(&pb.Event{
		Type: "native_hook", AgentRunId: "health-run", Pid: 111,
		ToolCallId: "tool", ToolName: "read",
		ExtraInfo: "hook_event=PreToolUse side_effect_scope=read_only",
	})
	snap := collectorMetricsStore.Snapshot()
	if snap.AgentRiskPipeline == nil || snap.AgentRiskPipeline.Sessions != 1 ||
		snap.AgentRiskPipeline.ActiveCalls != 1 || snap.AgentRiskPipeline.MaxSessions != 8 {
		t.Fatalf("collector health lacks unified risk status: %+v", snap.AgentRiskPipeline)
	}
}
