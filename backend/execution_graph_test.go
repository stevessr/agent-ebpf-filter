package main

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestBuildExecutionGraphIncludesProcessTreeResourcesAndPolicy(t *testing.T) {
	now := time.Unix(1710000000, 0).UTC()
	records := []CapturedEventRecord{
		{
			ReceivedAt: now,
			Event: &pb.Event{
				Pid:          100,
				Ppid:         1,
				Comm:         "codex",
				Type:         "process_fork",
				ExtraInfo:    "child_pid=101",
				AgentRunId:   "run-1",
				ToolCallId:   "tool-1",
				ToolName:     "bash",
				TraceId:      "trace-1",
				Decision:     "ALLOW",
				RiskScore:    12,
				CgroupId:     55,
				RootAgentPid: 100,
			},
		},
		{
			ReceivedAt: now.Add(time.Second),
			Event: &pb.Event{
				Pid:        101,
				Ppid:       100,
				Comm:       "bash",
				Type:       "execve",
				Path:       "/usr/bin/git",
				AgentRunId: "run-1",
				ToolCallId: "tool-1",
				ToolName:   "bash",
				TraceId:    "trace-1",
				Decision:   "ALLOW",
				RiskScore:  15,
			},
		},
		{
			ReceivedAt: now.Add(2 * time.Second),
			Event: &pb.Event{
				Pid:          101,
				Ppid:         100,
				Comm:         "git",
				Type:         "openat",
				Path:         "/workspace/package.json",
				AgentRunId:   "run-1",
				ToolCallId:   "tool-1",
				TraceId:      "trace-1",
				RiskScore:    18,
				RootAgentPid: 100,
			},
		},
		{
			ReceivedAt: now.Add(3 * time.Second),
			Event: &pb.Event{
				Pid:         101,
				Ppid:        100,
				Comm:        "git",
				Type:        "network_connect",
				NetEndpoint: "github.com:443",
				Domain:      "github.com",
				AgentRunId:  "run-1",
				ToolCallId:  "tool-1",
				TraceId:     "trace-1",
				RiskScore:   22,
			},
		},
		{
			ReceivedAt: now.Add(4 * time.Second),
			Event: &pb.Event{
				Pid:        101,
				Ppid:       100,
				Comm:       "SECRET_ACCESS",
				Type:       "semantic_alert",
				Path:       "/home/steve/.ssh/id_rsa",
				ExtraInfo:  "tool declared read_file but accessed secret",
				AgentRunId: "run-1",
				ToolCallId: "tool-1",
				TraceId:    "trace-1",
				Decision:   "ALERT",
				RiskScore:  96,
			},
		},
	}

	graph := buildExecutionGraph(records, executionGraphFilters{AgentRunID: "run-1"})
	if graph.EventCount != len(records) {
		t.Fatalf("EventCount = %d, want %d", graph.EventCount, len(records))
	}
	if len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		t.Fatalf("expected non-empty graph, got %d nodes / %d edges", len(graph.Nodes), len(graph.Edges))
	}
	assertGraphNodeKind(t, graph.Nodes, "agent_run")
	assertGraphNodeKind(t, graph.Nodes, "tool_call")
	assertGraphNodeKind(t, graph.Nodes, "process")
	assertGraphNodeKind(t, graph.Nodes, "file")
	assertGraphNodeKind(t, graph.Nodes, "network")
	assertGraphNodeKind(t, graph.Nodes, "policy_alert")
	assertGraphNodeKind(t, graph.Nodes, "policy_decision")
	assertGraphNodeKind(t, graph.Nodes, "syscall")
	assertGraphEdgeKind(t, graph.Edges, "spawned")
	assertGraphEdgeKind(t, graph.Edges, "parent_process")
	assertGraphEdgeKind(t, graph.Edges, "execed")
	assertGraphEdgeKind(t, graph.Edges, "opened")
	assertGraphEdgeKind(t, graph.Edges, "connected")
	assertGraphEdgeKind(t, graph.Edges, "alerted")
}

func TestBuildExecutionGraphAddsProcessCallChainFallbackEdges(t *testing.T) {
	base := time.Unix(1710000000, 0).UTC()
	records := []CapturedEventRecord{
		{ReceivedAt: base, Event: &pb.Event{Pid: 301, Ppid: 300, Comm: "python", Type: "openat", Path: "/tmp/a"}},
		{ReceivedAt: base.Add(time.Second), Event: &pb.Event{Pid: 302, Ppid: 301, Comm: "bash", Type: "process_exec", ExtraInfo: "old_pid=201"}},
	}

	graph := buildExecutionGraph(records, executionGraphFilters{})
	assertGraphEdgeKind(t, graph.Edges, "parent_process")
	assertGraphEdgeKind(t, graph.Edges, "exec_chain")
	assertGraphNodeLabelContains(t, graph.Nodes, "pid 300")
	assertGraphNodeLabelContains(t, graph.Nodes, "pid 201")
}

func TestBuildExecutionGraphFilters(t *testing.T) {
	base := time.Unix(1710000000, 0).UTC()
	records := []CapturedEventRecord{
		{ReceivedAt: base, Event: &pb.Event{Pid: 10, Comm: "git", Type: "openat", Path: "/workspace/a.txt", AgentRunId: "run-a", ToolCallId: "tool-a", TraceId: "trace-a", RiskScore: 20}},
		{ReceivedAt: base.Add(2 * time.Hour), Event: &pb.Event{Pid: 11, Comm: "curl", Type: "network_connect", NetEndpoint: "evil.example:443", Domain: "evil.example", AgentRunId: "run-b", ToolCallId: "tool-b", TraceId: "trace-b", RiskScore: 95}},
	}

	since := base.Add(time.Hour)
	graph := buildExecutionGraph(records, executionGraphFilters{Since: &since, Domain: "evil", RiskMin: 90})
	if graph.EventCount != 1 {
		t.Fatalf("EventCount = %d, want 1", graph.EventCount)
	}
	assertGraphNodeLabelContains(t, graph.Nodes, "evil.example:443")
	for _, node := range graph.Nodes {
		if node.Kind == "file" && node.Label == "/workspace/a.txt" {
			t.Fatalf("unexpected file node from filtered-out record")
		}
	}
}

func TestBuildExecutionGraphProcessTreeFilterIncludesDescendants(t *testing.T) {
	base := time.Unix(1710000000, 0).UTC()
	pid := uint32(100)
	records := []CapturedEventRecord{
		{ReceivedAt: base, Event: &pb.Event{Pid: 100, Ppid: 1, Comm: "agent", Type: "process_fork", ExtraInfo: "child_pid=101"}},
		{ReceivedAt: base.Add(time.Second), Event: &pb.Event{Pid: 101, Ppid: 100, Comm: "bash", Type: "process_fork", ExtraInfo: "child_pid=102"}},
		{ReceivedAt: base.Add(2 * time.Second), Event: &pb.Event{Pid: 102, Ppid: 101, Comm: "curl", Type: "network_connect", NetEndpoint: "api.example:443"}},
		{ReceivedAt: base.Add(3 * time.Second), Event: &pb.Event{Pid: 200, Ppid: 1, Comm: "unrelated", Type: "openat", Path: "/tmp/other"}},
	}

	graph := buildExecutionGraph(records, executionGraphFilters{PID: &pid, ProcessTree: true})
	if graph.EventCount != 3 {
		t.Fatalf("EventCount = %d, want descendant tree events only", graph.EventCount)
	}
	assertGraphNodeLabelContains(t, graph.Nodes, "curl")
	assertGraphNodeLabelContains(t, graph.Nodes, "api.example:443")
	assertGraphEdgeKind(t, graph.Edges, "child_process")
	for _, node := range graph.Nodes {
		if node.Label == "unrelated" || node.Label == "/tmp/other" {
			t.Fatalf("unexpected unrelated node %#v", node)
		}
	}
}

func assertGraphNodeKind(t *testing.T, nodes []ExecutionGraphNode, kind string) {
	t.Helper()
	for _, node := range nodes {
		if node.Kind == kind {
			return
		}
	}
	t.Fatalf("missing node kind %q", kind)
}

func assertGraphEdgeKind(t *testing.T, edges []ExecutionGraphEdge, kind string) {
	t.Helper()
	for _, edge := range edges {
		if edge.Kind == kind {
			return
		}
	}
	t.Fatalf("missing edge kind %q", kind)
}

func assertGraphNodeLabelContains(t *testing.T, nodes []ExecutionGraphNode, want string) {
	t.Helper()
	for _, node := range nodes {
		if node.Label == want {
			return
		}
	}
	t.Fatalf("missing node label %q", want)
}
