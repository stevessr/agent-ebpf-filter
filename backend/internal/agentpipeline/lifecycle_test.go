package agentpipeline

import (
	"testing"
	"time"
)

func TestCompletedToolDoesNotLeakPermissionsPastGrace(t *testing.T) {
	now := time.Now().UTC()
	p := New(4, time.Minute)
	pre := Message{At: now, Kind: "tool_call", AgentRunID: "run", ToolCallID: "id", TurnID: "1", SideEffectScope: "read_only"}
	p.Process(pre)
	p.Process(Message{At: now.Add(time.Second), Kind: "tool_result", AgentRunID: "run", ToolCallID: "id", TurnID: "1"})
	if got := p.Process(Message{At: now.Add(5*time.Second), AgentRunID: "run", ToolCallID: "id", TurnID: "1", Action: "write"}); len(got) != 0 {
		t.Fatalf("completed tool leaked read-only declaration: %+v", got)
	}
	if status := p.Stats(); status.ActiveCalls != 0 || status.CompletedCalls != 1 {
		t.Fatalf("unexpected lifecycle stats: %+v", status)
	}
}

func TestGraceForLateSyscallAndDuplicateToolResult(t *testing.T) {
	now := time.Now().UTC()
	p := New(2, time.Minute)
	pre := Message{At: now, Kind: "tool_call", AgentRunID: "r", ToolCallID: "t", SideEffectScope: "readonly"}
	p.Process(pre)
	result := Message{At: now.Add(time.Second), Kind: "tool_result", AgentRunID: "r", ToolCallID: "t"}
	p.Process(result)
	result.At = now.Add(4 * time.Second)
	p.Process(result)
	if got := p.Process(Message{At: now.Add(4500*time.Millisecond), AgentRunID: "r", ToolCallID: "t", Action: "write"}); len(got) != 0 {
		t.Fatalf("duplicated result extended completion grace: %+v", got)
	}
}

func TestNewTurnReusedToolCallIDStartsClean(t *testing.T) {
	p := New(2, time.Minute)
	now := time.Now().UTC()
	p.Process(Message{At: now, AgentRunID: "r", Kind: "tool_call", ToolCallID: "same", TurnID: "A", SideEffectScope: "read"})
	p.Process(Message{At: now.Add(time.Second), AgentRunID: "r", Kind: "tool_call", ToolCallID: "same", TurnID: "B", ToolName: "writer"})
	if got := p.Process(Message{At: now.Add(2*time.Second), AgentRunID: "r", Action: "write", ToolCallID: "same", TurnID: "B"}); len(got) != 0 {
		t.Fatalf("old tool restrictions inherited by new turn: %+v", got)
	}
	if got := p.Process(Message{At: now.Add(3*time.Second), AgentRunID: "r", Action: "write", ToolCallID: "same", TurnID: "A"}); len(got) != 0 {
		t.Fatalf("old turn matched new tool call: %+v", got)
	}
}

func TestSessionEndReleasesContextWithoutCreatingNewState(t *testing.T) {
	p := New(2, time.Minute)
	now := time.Now().UTC()
	if got := p.Process(Message{At: now, AgentRunID: "run", Action: "execute"}); len(got) != 0 || p.Stats().Sessions != 0 {
		t.Fatalf("orphan syscall allocated state: %+v", p.Stats())
	}
	p.Process(Message{At: now, AgentRunID: "run", Kind: "model_input"})
	if p.Stats().Sessions != 0 {
		t.Fatal("benign model input allocated state")
	}
	p.Process(Message{At: now, AgentRunID: "run", Kind: "tool_call", ToolCallID: "tool", SideEffectScope: "read_only"})
	p.Process(Message{At: now.Add(time.Second), AgentRunID: "run", Kind: "session_end"})
	if status := p.Stats(); status.Sessions != 0 || status.ActiveCalls != 0 {
		t.Fatalf("session end left tool state: %+v", status)
	}
	if got := p.Process(Message{At: now.Add(2*time.Second), AgentRunID: "run", ToolCallID: "tool", Action: "execute"}); len(got) != 0 {
		t.Fatalf("stale tool state after session exit: %+v", got)
	}
}

func TestHighRiskInputCanInitializeOtherwiseUnknownSession(t *testing.T) {
	p := New(3, time.Minute)
	now := time.Now().UTC()
	p.Process(Message{At: now, AgentRunID: "r", Kind: "tool_result", UntrustedInput: true, HighRiskInput: true, TurnID: "T"})
	if p.Stats().TaintedSessions != 1 {
		t.Fatalf("no taint evidence recorded: %+v", p.Stats())
	}
	got := p.Process(Message{At: now.Add(time.Second), AgentRunID: "r", TurnID: "T", Action: "egress"})
	if len(got) != 1 || got[0].Code != "UNTRUSTED_INPUT_ACTION_CHAIN" {
		t.Fatalf("missing independent high-risk chain: %+v", got)
	}
}
