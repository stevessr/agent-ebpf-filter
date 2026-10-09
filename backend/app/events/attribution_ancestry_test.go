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

func TestPropagateAgentContextAtFork(t *testing.T) {
	store := NewProcessContextStore()
	parent := ProcessContext{RootAgentPid: 100, AgentRunID: "codex-run"}
	fork := &pb.Event{Pid: 100, Type: "process_fork", ExtraInfo: "child_pid=101"}
	propagateAgentContextOnFork(fork, parent, store)
	child, ok := store.Get(101)
	if !ok || child.RootAgentPid != 100 || child.AgentRunID != "codex-run" {
		t.Fatalf("child did not inherit Agent identity at fork: %+v %t", child, ok)
	}

	// A child that registers an independent run retains its own attribution.
	store.Set(102, ProcessContext{RootAgentPid: 102, AgentRunID: "child-run"})
	propagateAgentContextOnFork(&pb.Event{Pid: 100, Type: "process_fork", ExtraInfo: "child_pid=102"}, parent, store)
	registered, _ := store.Get(102)
	if registered.AgentRunID != "child-run" {
		t.Fatalf("fork overwrote an explicitly registered child: %+v", registered)
	}

	for _, extra := range []string{"child_pid=100", "child_pid=0", "child_pid=bogus", ""} {
		propagateAgentContextOnFork(&pb.Event{Pid: 100, Type: "process_fork", ExtraInfo: extra}, parent, store)
	}
	if _, found := store.Get(0); found {
		t.Fatal("invalid fork child became a process context")
	}
}

func TestAgentCLISeedsOnlyKnownAgentRoot(t *testing.T) {
	if !shouldSeedAgentRootAtFork(&pb.Event{Pid: 100, Type: "process_fork", Tag: "Agent CLI", Comm: "codex"}) {
		t.Fatal("Codex selected by comm should establish its own root context")
	}
	for _, test := range []pb.Event{
		{Pid: 102, Type: "process_fork", Tag: "Shell", Comm: "bash"},
		{Pid: 103, Type: "process_fork", Tag: "Runtime", Comm: "python"},
		{Pid: 104, Type: "process_fork", Tag: "Agent CLI", Comm: "gh"},
		{Pid: 105, Type: "file_write", Tag: "Agent CLI", Comm: "codex"},
	} {
		if shouldSeedAgentRootAtFork(&test) {
			t.Fatalf("unverified process was used as Agent root: %+v", test)
		}
	}
}

func TestAgentCustomTagCanSeedRootAtFork(t *testing.T) {
	if !shouldSeedAgentRootAtFork(&pb.Event{Pid: 150, Type: "process_fork", Comm: "codex", Tag: "My Agent"}) {
		t.Fatal("registered Codex with a custom user label must still seed its descendant tree")
	}
	if shouldSeedAgentRootAtFork(&pb.Event{Pid: 151, Type: "process_fork", Comm: "python", Tag: "My Agent"}) {
		t.Fatal("custom tag must never promote a generic interpreter to Agent root")
	}
}

func TestCgroupAttributionRetainsRootAgentPID(t *testing.T) {
	oldGet, oldEnrich := Deps.CgroupAttributionGet, Deps.CgroupAttributionEnrich
	defer func() {
		Deps.CgroupAttributionGet = oldGet
		Deps.CgroupAttributionEnrich = oldEnrich
	}()
	Deps.CgroupAttributionGet = func(id uint64) (CgroupAttributionEntry, bool) {
		if id != 99 {
			return CgroupAttributionEntry{}, false
		}
		return CgroupAttributionEntry{RootAgentPID: 100, AgentRunID: "run-1", TaskID: "task", ToolCallID: "tool"}, true
	}
	Deps.CgroupAttributionEnrich = nil
	ctx, ok := contextFromAgentCgroup(99)
	if !ok || ctx.RootAgentPid != 100 || ctx.AgentRunID != "run-1" || ctx.ToolCallID != "tool" {
		t.Fatalf("cgroup lookup lost Agent root: ok=%v ctx=%+v", ok, ctx)
	}
	event := &pb.Event{Pid: 200, Comm: "python", Type: "file_write"}
	ApplyProcessContextToEvent(event, ctx)
	if event.RootAgentPid != 100 || event.AgentRunId != "run-1" {
		t.Fatalf("python edit was not attributed to cgroup Agent: %+v", event)
	}
	if _, ok := contextFromAgentCgroup(0); ok {
		t.Fatal("zero cgroup ID cannot establish Agent identity")
	}
}

func TestCgroupAttributionLegacyFallbackWithoutRoot(t *testing.T) {
	oldGet, oldEnrich := Deps.CgroupAttributionGet, Deps.CgroupAttributionEnrich
	defer func() {
		Deps.CgroupAttributionGet = oldGet
		Deps.CgroupAttributionEnrich = oldEnrich
	}()
	Deps.CgroupAttributionGet = nil
	Deps.CgroupAttributionEnrich = func(uint64) (string, string, string) {
		return "legacy-run", "legacy-task", "legacy-tool"
	}
	ctx, ok := contextFromAgentCgroup(44)
	if !ok || ctx.AgentRunID != "legacy-run" || ctx.RootAgentPid != 0 {
		t.Fatalf("legacy attribution compatibility regression: %+v ok=%v", ctx, ok)
	}
}
