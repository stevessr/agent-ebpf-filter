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
