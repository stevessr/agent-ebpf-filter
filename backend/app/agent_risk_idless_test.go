package app

import (
	"testing"
	"time"

	"agent-ebpf-filter/internal/agentpipeline"
	"agent-ebpf-filter/pb"
)

func TestUnifiedRiskIgnoresIdlessToolAndBenignModelMessage(t *testing.T) {
	old := unifiedAgentRiskPipeline
	unifiedAgentRiskPipeline = agentpipeline.New(10, time.Minute)
	t.Cleanup(func() { unifiedAgentRiskPipeline = old })
	for _, event := range []*pb.Event{
		{Type: "mcp_tool_call", AgentRunId: "run", ToolName: "read"},
		{Type: "native_hook", AgentRunId: "run", ExtraInfo: "hook_event=BeforeModel"},
		{Type: "execve", AgentRunId: "run", Pid: 1},
	} {
		if got := buildUnifiedAgentRiskAlerts(event); len(got) != 0 {
			t.Fatalf("unexpected uncorrelated finding for %q: %+v", event.GetType(), got)
		}
	}
	if stats := unifiedAgentRiskPipeline.Stats(); stats.Sessions != 0 {
		t.Fatalf("idless/no-risk messages allocated sessions: %+v", stats)
	}
}
