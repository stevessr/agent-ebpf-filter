package events

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestResolveAncestorAgentContextThroughScriptInterpreters(t *testing.T) {
	contexts := NewProcessContextStore()
	contexts.Set(100, ProcessContext{RootAgentPid: 100, AgentRunID: "agent-run", ToolCallID: "shell-call"})
	parents := map[uint32]uint32{500: 400, 400: 300, 300: 200, 200: 100, 100: 1}
	parentOf := func(pid uint32) (uint32, bool) { parent, ok := parents[pid]; return parent, ok }
	// Agent(100) -> fish(200) -> pwsh(300) -> node(400) -> python(500).
	ctx, ok := resolveAncestorAgentContext(500, 400, contexts, parentOf)
	if !ok || ctx.RootAgentPid != 100 || ctx.AgentRunID != "agent-run" {
		t.Fatalf("did not resolve multiple interpreter layers: ok=%v ctx=%+v", ok, ctx)
	}
	file := &pb.Event{Pid: 500, Ppid: 400, Type: "file_write", Path: "/tmp/script-modified.txt"}
	ApplyProcessContextToEvent(file, ctx)
	if file.RootAgentPid != 100 || file.AgentRunId != "agent-run" || file.ToolCallId != "shell-call" {
		t.Fatalf("file write lost source Agent attribution: %+v", file)
	}
}

func TestAncestorResolutionNeverInventsOwnership(t *testing.T) {
	contexts := NewProcessContextStore()
	contexts.Set(100, ProcessContext{RootAgentPid: 100, AgentRunID: "other-run"})
	parents := map[uint32]uint32{500: 400, 400: 1}
	parentOf := func(pid uint32) (uint32, bool) { parent, ok := parents[pid]; return parent, ok }
	if ctx, ok := resolveAncestorAgentContext(500, 400, contexts, parentOf); ok {
		t.Fatalf("unrelated interpreter was attributed to Agent: %+v", ctx)
	}
	// Reparenting and PID cycles fail closed; no traversal into unrelated
	// ancestry, and no unbounded recursion.
	parents[400] = 500
	if _, ok := resolveAncestorAgentContext(500, 400, contexts, parentOf); ok {
		t.Fatal("cyclic process ancestry created an Agent attribution")
	}
}

func TestAncestorFallbackOnlyForFileAndExecEvents(t *testing.T) {
	for _, typ := range []string{"file_write", "process_exec", "write", "renameat2", "openat"} {
		if !shouldResolveAgentAncestor(&pb.Event{Pid: 500, Type: typ}) {
			t.Fatalf("%s should allow bounded ancestry recovery", typ)
		}
	}
	for _, typ := range []string{"network_connect", "system_metric", "agentsight_alert"} {
		if shouldResolveAgentAncestor(&pb.Event{Pid: 500, Type: typ}) {
			t.Fatalf("%s should not trigger procfs walking", typ)
		}
	}
}

func TestAncestorResolutionBounded(t *testing.T) {
	store := NewProcessContextStore()
	store.Set(10, ProcessContext{AgentRunID: "far-away"})
	parents := map[uint32]uint32{}
	for pid := uint32(200); pid > 10; pid-- {
		parents[pid] = pid - 1
	}
	reader := func(pid uint32) (uint32, bool) { value, ok := parents[pid]; return value, ok }
	if _, ok := resolveAncestorAgentContext(201, 200, store, reader); ok {
		t.Fatal("an overly deep /proc ancestry chain should not be followed")
	}
}
