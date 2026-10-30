package events

import (
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestSemanticSecretCorrelationDoesNotCrossAgentRunOrRoot(t *testing.T) {
	state := NewSemanticAlertState()
	now := time.Now().UTC()
	original := &pb.Event{AgentRunId: "run-A", RootAgentPid: 100, ToolCallId: "shared-tool"}
	state.RememberSecret(original, "/home/user/.ssh/id_ed25519", now)
	for _, unrelated := range []*pb.Event{
		{AgentRunId: "run-B", RootAgentPid: 100, ToolCallId: "shared-tool"},
		{AgentRunId: "run-A", RootAgentPid: 200, ToolCallId: "shared-tool"},
		{AgentRunId: "run-B", RootAgentPid: 200, ToolCallId: "shared-tool"},
	} {
		if target, ok := state.RecentSecretTarget(unrelated, now.Add(time.Second)); ok {
			t.Fatalf("different agent inherited secret %q for %+v", target, unrelated)
		}
	}
	if target, ok := state.RecentSecretTarget(original, now.Add(time.Second)); !ok || target == "" {
		t.Fatalf("matching agent lost correlated secret: %q %v", target, ok)
	}
}

func TestSemanticTaskTraceScopeAndPIDFallback(t *testing.T) {
	for _, pair := range [][2]*pb.Event{
		{
			{AgentRunId: "A", TaskId: "task", TraceId: "shared-trace"},
			{AgentRunId: "B", TaskId: "task", TraceId: "shared-trace"},
		},
		{
			{Pid: 111, ToolCallId: "same-call"},
			{Pid: 222, ToolCallId: "same-call"},
		},
		{
			{AgentRunId: "a|b", ToolCallId: "c"},
			{AgentRunId: "a", ToolCallId: "b|c"},
		},
	} {
		left, _ := semanticAlertContextKeyBounded(pair[0])
		right, _ := semanticAlertContextKeyBounded(pair[1])
		if left == right || left == "" || right == "" {
			t.Fatalf("collision left=%q right=%q", left, right)
		}
	}
}

func TestScopedSemanticIdentifiersRemainBounded(t *testing.T) {
	long := strings.Repeat("a", SemanticStateMaxContextBytes*100)
	e := &pb.Event{AgentRunId: long + "A", ToolCallId: long + "B", RootAgentPid: 100}
	key1, truncated := semanticAlertContextKeyBounded(e)
	key2, _ := semanticAlertContextKeyBounded(e)
	e.ToolCallId = long + "C"
	key3, _ := semanticAlertContextKeyBounded(e)
	if !truncated || len(key1) > SemanticStateMaxContextBytes || key1 != key2 || key1 == key3 {
		t.Fatalf("bounded scoped key broken; len=%d truncated=%v", len(key1), truncated)
	}
}
