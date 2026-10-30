package agentpipeline

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestReadOnlyToolCallCorrelatesWithSyscall(t *testing.T) {
	p := New(2, time.Minute)
	now := time.Now()
	base := Message{At: now, AgentRunID: "run1", ToolCallID: "call", ToolName: "query", Kind: "tool_call", SideEffectScope: "read_only"}
	if got := p.Process(base); len(got) != 0 {
		t.Fatalf("unexpected findings: %v", got)
	}
	action := Message{At: now.Add(time.Second), AgentRunID: "run1", ToolCallID: "call", Kind: "file", Action: "write", Target: "/tmp/output"}
	findings := p.Process(action)
	if len(findings) != 1 || findings[0].Code != "TOOL_SCOPE_VIOLATION" || findings[0].RiskScore != 96 {
		t.Fatalf("findings=%v", findings)
	}
	if got := p.Process(action); len(got) != 0 {
		t.Fatalf("duplicate alerts: %v", got)
	}
	action.AgentRunID = "run2"
	if got := p.Process(action); len(got) != 0 {
		t.Fatalf("cross-run leak: %v", got)
	}
}

func TestHighRiskUntrustedInputChainOnlyWithExplicitEvidence(t *testing.T) {
	p := New(3, time.Minute)
	t0 := time.Now()
	input := Message{At: t0, AgentRunID: "A", TurnID: "T", Kind: "tool_result", UntrustedInput: true}
	p.Process(input)
	action := Message{At: t0.Add(time.Second), AgentRunID: "A", TurnID: "T", Kind: "process", Action: "execute"}
	if got := p.Process(action); len(got) != 0 {
		t.Fatalf("weak evidence: %v", got)
	}
	input.HighRiskInput = true
	p.Process(input)
	if got := p.Process(action); len(got) != 1 || got[0].Code != "UNTRUSTED_INPUT_ACTION_CHAIN" {
		t.Fatalf("missing chain: %v", got)
	}
	if got := p.Process(action); len(got) != 0 {
		t.Fatalf("duplicate chain: %v", got)
	}
}

func TestSessionIsolationExpiryBoundsAndPlanMode(t *testing.T) {
	p := New(2, time.Second)
	t0 := time.Now()
	p.Process(Message{At: t0, AgentRunID: "old", Kind: "tool_call", ToolCallID: "id", PermissionMode: "plan"})
	if got := p.Process(Message{At: t0.Add(2 * time.Second), AgentRunID: "old", ToolCallID: "id", Action: "egress"}); len(got) != 0 {
		t.Fatalf("expired: %v", got)
	}
	p.Process(Message{At: t0.Add(3 * time.Second), AgentRunID: "old", ToolCallID: "id", Kind: "tool_call", PermissionMode: "plan"})
	got := p.Process(Message{At: t0.Add(4 * time.Second), AgentRunID: "old", ToolCallID: "id", Action: "egress"})
	if len(got) != 1 || got[0].Code != "PLAN_MODE_SIDE_EFFECT" {
		t.Fatalf("plan: %v", got)
	}
	p.Process(Message{At: t0.Add(5 * time.Second), AgentRunID: "next", Kind: "other"})
	p.Process(Message{At: t0.Add(6 * time.Second), AgentRunID: "last", Kind: "other"})
	if stats := p.Stats(); stats.Sessions > 2 || stats.Evictions < 1 {
		t.Fatalf("state unbounded: %+v", stats)
	}
}

func TestNoIdentityOrUntrustedDataPersisted(t *testing.T) {
	p := New(1, time.Minute)
	if got := p.Process(Message{Kind: "tool_call", ToolCallID: "same", SideEffectScope: "read_only"}); len(got) != 0 || p.Stats().Sessions != 0 {
		t.Fatalf("unscoped: %v", got)
	}
	long := strings.Repeat("x", 10000)
	p.Process(Message{Kind: "tool_call", AgentRunID: long, ToolCallID: long, SideEffectScope: "readonly", Target: long})
	got := p.Process(Message{Kind: "file", Action: "write", AgentRunID: long, ToolCallID: long, Target: long})
	if len(got) != 1 || len(got[0].Target) > 256 {
		t.Fatalf("oversized finding: %v", got)
	}
}

func TestConcurrentProcessors(t *testing.T) {
	p := New(10, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				p.Process(Message{AgentRunID: "same", Kind: "model_input", HighRiskInput: true})
			}
		}()
	}
	wg.Wait()
	if p.Stats().Sessions != 1 {
		t.Fatalf("stats: %+v", p.Stats())
	}
}

func TestMultiAgentRootAndTurnSeparation(t *testing.T) {
	p := New(10, time.Minute)
	now := time.Now()
	p.Process(Message{At: now, AgentRunID: "shared-run", RootPID: 101, ToolCallID: "same-call", Kind: "tool_call", SideEffectScope: "read"})
	if got := p.Process(Message{At: now.Add(time.Second), AgentRunID: "shared-run", RootPID: 202, ToolCallID: "same-call", Action: "execute"}); len(got) != 0 {
		t.Fatalf("different root was attributed to the wrong agent: %v", got)
	}
	if got := p.Process(Message{At: now.Add(time.Second), AgentRunID: "shared-run", RootPID: 101, ToolCallID: "same-call", Action: "execute"}); len(got) != 1 {
		t.Fatalf("same-root correlation failed: %v", got)
	}
	p.Process(Message{At: now.Add(2 * time.Second), AgentRunID: "shared-run", RootPID: 101, TurnID: "turn-A", Kind: "model_input", UntrustedInput: true, HighRiskInput: true})
	if got := p.Process(Message{At: now.Add(3 * time.Second), AgentRunID: "shared-run", RootPID: 101, TurnID: "turn-B", Action: "write"}); len(got) != 0 {
		t.Fatalf("risk signal leaked into another turn: %v", got)
	}
}

func TestExpiredCallDoesNotSurviveBusySession(t *testing.T) {
	p := New(3, time.Second)
	start := time.Now()
	p.Process(Message{At: start, AgentRunID: "busy", ToolCallID: "old", Kind: "tool_call", SideEffectScope: "read_only"})
	for i := 1; i <= 4; i++ {
		p.Process(Message{At: start.Add(time.Duration(i)*400*time.Millisecond), AgentRunID: "busy", Kind: "model_input"})
	}
	if got := p.Process(Message{At: start.Add(1700 * time.Millisecond), AgentRunID: "busy", ToolCallID: "old", Action: "write"}); len(got) != 0 {
		t.Fatalf("stale tool scope reused: %v", got)
	}
}
