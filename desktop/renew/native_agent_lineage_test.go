package main

import (
	"testing"
	"time"
)

func lineageProcess(pid, ppid int, name string, created int64) systemProcess {
	return systemProcess{PID: pid, PPID: ppid, Name: name, CreateTime: created}
}

func TestAgentLineageTracksNestedAgentAndOrdinaryChildren(t *testing.T) {
	processes := []systemProcess{
		lineageProcess(15, 12, "python", 130),
		lineageProcess(30, 20, "git", 142),
		lineageProcess(10, 1, "codex", 100),
		lineageProcess(20, 15, "claude", 140),
		lineageProcess(12, 10, "bash", 110),
	}
	graph := buildAgentLineage(processes, nil, 10, 72, time.Unix(170, 0))
	if len(graph.Nodes) != 5 || graph.Truncated {
		t.Fatalf("unexpected lineage size: len=%d truncated=%t", len(graph.Nodes), graph.Truncated)
	}
	wantPIDs := []int{10, 12, 15, 20, 30}
	wantParents := []int{-1, 0, 1, 2, 3}
	wantRoles := []string{"agent", "process", "process", "subagent-candidate", "process"}
	for i, got := range graph.Nodes {
		if got.Process.PID != wantPIDs[i] || got.Parent != wantParents[i] || got.Role != wantRoles[i] {
			t.Errorf("node %d: %+v, want pid=%d parent=%d role=%s", i, got, wantPIDs[i], wantParents[i], wantRoles[i])
		}
	}
}

func TestLineageSkipsImpossibleReusedPIDParent(t *testing.T) {
	processes := []systemProcess{
		lineageProcess(10, 1, "codex", 200),
		lineageProcess(11, 10, "bash", 120), // child older than parent: recycled parent PID
		lineageProcess(12, 10, "sh", 210),
	}
	graph := buildAgentLineage(processes, nil, 10, 72, time.Unix(230, 0))
	if len(graph.Nodes) != 2 || graph.Nodes[1].Process.PID != 12 {
		t.Fatalf("invalid lineage joined reused PID: %+v", graph.Nodes)
	}
}

func TestLineageBoundedAndCycleSafe(t *testing.T) {
	processes := []systemProcess{
		lineageProcess(10, 12, "codex", 100),
		lineageProcess(11, 10, "bash", 110),
		lineageProcess(12, 11, "sh", 120),
		lineageProcess(13, 10, "python", 121),
	}
	graph := buildAgentLineage(processes, nil, 10, 2, time.Unix(140, 0))
	if len(graph.Nodes) != 2 || !graph.Truncated {
		t.Fatalf("graph must be bounded and mark truncation: %+v", graph)
	}
	if graph.Nodes[0].Process.PID != 10 || graph.Nodes[1].Process.PID != 11 {
		t.Fatalf("unstable order: %+v", graph.Nodes)
	}
}

func TestLineageEventAttributionRequiresCurrentProcessGeneration(t *testing.T) {
	p := lineageProcess(42, 1, "bash", 200)
	old := eventSummary{PID: 42, RootAgentPID: 42, Tag: "codex", ReceivedAtMS: 100000}
	recent := eventSummary{PID: 42, RootAgentPID: 42, Tag: "codex", ReceivedAtMS: 200500}
	if agentLabelForLiveProcess(p, []eventSummary{old}) != "" {
		t.Fatal("historical event linked to reused PID")
	}
	if agentLabelForLiveProcess(p, []eventSummary{recent}) != "Codex" {
		t.Fatal("valid current event should label active agent")
	}
	graph := buildAgentLineage([]systemProcess{p}, []eventSummary{old, recent}, 42, 72, time.Unix(220, 0))
	if len(graph.Nodes) != 1 || graph.Nodes[0].Events != 1 {
		t.Fatalf("historical event leaked into current counters: %+v", graph)
	}
}

func TestAgentLineageRootsDeterministic(t *testing.T) {
	processes := []systemProcess{
		lineageProcess(13, 1, "python", 102),
		lineageProcess(10, 1, "codex", 100),
		lineageProcess(12, 1, "claude", 102),
		lineageProcess(11, 1, "gemini", 90),
	}
	roots := agentLineageRoots(processes, nil)
	if len(roots) != 3 || roots[0].PID != 12 || roots[1].PID != 10 || roots[2].PID != 11 {
		t.Fatalf("root ordering: %+v", roots)
	}
}

func TestLineageWaterfallGeometryRespectsTimestamps(t *testing.T) {
	graph := buildAgentLineage([]systemProcess{
		lineageProcess(10, 1, "codex", 100),
		lineageProcess(11, 10, "bash", 110),
		lineageProcess(12, 10, "python", 120),
	}, nil, 10, 72, time.Unix(140, 0))
	got := waterfallBarGeometry(graph, 100)
	if len(got) != 3 || got[0] != 0 || got[1] != 25 || got[2] != 50 {
		t.Fatalf("waterfall must be monotone: %v", got)
	}
	points := radialLineagePoints(graph, 800, 540)
	if len(points) != 3 || points[0].X != 400 || points[0].Y != 270 {
		t.Fatalf("radial center/layout invalid: %+v", points)
	}
}

func TestLineageDoesNotPromoteInheritedAgentRunToAgentRoot(t *testing.T) {
	processes := []systemProcess{
		lineageProcess(10, 1, "codex", 200),
		lineageProcess(11, 10, "bash", 210),
		lineageProcess(12, 11, "python", 220),
	}
	events := []eventSummary{
		{PID: 11, RootAgentPID: 10, AgentRunID: "run1", Tag: "codex", ReceivedAtMS: 211000},
		{PID: 12, RootAgentPID: 10, AgentRunID: "run1", Tag: "claude", ReceivedAtMS: 222000},
	}
	roots := agentLineageRoots(processes, events)
	if len(roots) != 1 || roots[0].PID != 10 {
		t.Fatalf("executor inherited parent context and was promoted to Agent: %+v", roots)
	}
	graph := buildAgentLineage(processes, events, 10, 72, time.Unix(240, 0))
	for _, n := range graph.Nodes[1:] {
		if n.Role != "process" {
			t.Fatalf("inherited event incorrectly implies subagent: %+v", n)
		}
	}
}

func TestLineageDirectNativeHookLabelsNonStandardExecutable(t *testing.T) {
	p := lineageProcess(42, 1, "node", 200)
	e := eventSummary{PID: 42, Type: "native_hook", Tag: "Claude Code", ReceivedAtMS: 201000}
	if got := agentLabelForLiveProcess(p, []eventSummary{e}); got != "Claude Code" {
		t.Fatalf("direct native hook should identify agent, got %q", got)
	}
	e.Type = "exec"
	if got := agentLabelForLiveProcess(p, []eventSummary{e}); got != "" {
		t.Fatalf("generic inherited tag cannot be trusted without root attribution: %q", got)
	}
}

func TestLineageDoesNotTurnRelayShellIntoAgent(t *testing.T) {
	p := lineageProcess(41, 1, "bash", 200)
	event := eventSummary{PID: 41, Type: "native_hook", Tag: "Claude Code", ReceivedAtMS: 201000}
	if agentLabelForLiveProcess(p, []eventSummary{event}) != "" {
		t.Fatal("native hook relay attributed to bash cannot establish an Agent root")
	}
	event.RootAgentPID = 41
	if got := agentLabelForLiveProcess(p, []eventSummary{event}); got != "Claude Code" {
		t.Fatalf("explicit root attribution must be retained, got %q", got)
	}
}

func TestLineageHookSignalsStaySeparateFromOSChildNodes(t *testing.T) {
	processes := []systemProcess{
		lineageProcess(10, 1, "claude", 200),
		lineageProcess(11, 10, "bash", 201),
	}
	events := []eventSummary{
		{PID: 10, Type: "native_hook", Comm: "SubagentStart:", Tag: "Claude Code", ReceivedAtMS: 202000},
		{PID: 10, Type: "native_hook", Comm: "SubagentStop:", Tag: "Claude Code", ReceivedAtMS: 208000},
		{PID: 11, RootAgentPID: 10, Type: "native_hook", Comm: "agentSpawn:task", Tag: "Kiro CLI", ReceivedAtMS: 209000},
		{PID: 11, RootAgentPID: 10, Type: "exec", Comm: "SubagentStart:", ReceivedAtMS: 210000},
		{PID: 10, Type: "native_hook", Comm: "PreToolUse:", ReceivedAtMS: 211000},
	}
	graph := buildAgentLineage(processes, events, 10, 72, time.Unix(220, 0))
	if len(graph.Nodes) != 2 {
		t.Fatalf("hooks must not fabricate more OS nodes: %+v", graph.Nodes)
	}
	signals := collectAgentLineageSignals(graph, events, 2)
	if len(signals) != 2 || signals[0].Name != "agentSpawn" || signals[1].Kind != "stop" {
		t.Fatalf("signals not bounded/sorted: %+v", signals)
	}
}

func TestLineageHookSignalsRejectPreviousPIDGeneration(t *testing.T) {
	p := lineageProcess(10, 1, "claude", 300)
	graph := buildAgentLineage([]systemProcess{p}, nil, 10, 72, time.Unix(320, 0))
	events := []eventSummary{
		{PID: 10, Type: "native_hook", Comm: "SubagentStart:", ReceivedAtMS: 100000},
		{PID: 10, Type: "native_hook", Comm: "SubagentStop:", ReceivedAtMS: 301000},
	}
	signals := collectAgentLineageSignals(graph, events, 20)
	if len(signals) != 1 || signals[0].Kind != "stop" {
		t.Fatalf("reused PID leaked historical subagent hook: %+v", signals)
	}
}

func BenchmarkAgentLineageEventIndex(b *testing.B) {
	processes := make([]systemProcess, 2000)
	for i := range processes {
		processes[i] = lineageProcess(i+10, 10, "bash", 200)
	}
	processes[0].Name = "codex"
	processes[0].PPID = 1
	events := make([]eventSummary, 1200)
	for i := range events {
		events[i] = eventSummary{
			PID: i%len(processes)+10,
			RootAgentPID: 10,
			ReceivedAtMS: 250000+int64(i),
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildAgentLineage(processes, events, 10, 72, time.Unix(280, 0))
	}
}
