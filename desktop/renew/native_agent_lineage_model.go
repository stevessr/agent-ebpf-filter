package main

import (
	"sort"
	"strings"
	"time"
)

// Lineage shows currently observed OS process ancestry. It never claims that
// fork/exec by itself proves an application-level Agent -> SubAgent RPC.
type agentLineageNode struct {
	Process systemProcess
	Parent  int // index in Nodes, -1 for the focused root
	Depth   int
	Role    string // agent, subagent-candidate, process
	Events  int
	Alerts  int
}

type agentLineage struct {
	RootPID   int
	Nodes     []agentLineageNode
	Truncated bool
	Snapshot  time.Time
}

func agentProcessName(p systemProcess) string {
	if v := strings.TrimSpace(p.Name); v != "" {
		return v
	}
	return "unknown"
}

// Event context is only usable for the live process when it is not older than
// the current PID's start time. A reused PID must not inherit old Agent tags.
func eventMatchesLiveProcess(e eventSummary, p systemProcess) bool {
	if e.PID != p.PID {
		return false
	}
	if p.CreateTime > 0 && e.ReceivedAtMS > 0 &&
		e.ReceivedAtMS < p.CreateTime*1000-2000 {
		return false
	}
	return true
}

func agentLabelForLiveProcess(p systemProcess, events []eventSummary) string {
	if label := harnessLabelFor(p.Name); label != "未识别" {
		return label
	}
	for _, e := range events {
		if !eventMatchesLiveProcess(e, p) {
			continue
		}
		if e.RootAgentPID == p.PID || (e.PID == p.PID && e.AgentRunID != "") {
			if label := harnessLabelFor(e.Tag, e.Comm); label != "未识别" {
				return label
			}
		}
	}
	return ""
}

func agentLineageRoots(processes []systemProcess, events []eventSummary) []systemProcess {
	out := make([]systemProcess, 0, 16)
	for _, p := range processes {
		if p.PID > 0 && agentLabelForLiveProcess(p, events) != "" {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreateTime != out[j].CreateTime {
			return out[i].CreateTime > out[j].CreateTime
		}
		return out[i].PID < out[j].PID
	})
	return out
}

func validLineageEdge(parent, child systemProcess) bool {
	if parent.PID <= 0 || child.PID <= 0 || parent.PID == child.PID || child.PPID != parent.PID {
		return false
	}
	// Suppress impossible links after PID reuse; if either start time is
	// missing, PPID is only a live-snapshot observation, not a historical edge.
	if parent.CreateTime > 0 && child.CreateTime > 0 && child.CreateTime < parent.CreateTime {
		return false
	}
	return true
}

// buildAgentLineage follows one live snapshot, with cycle, depth and size
// limits. No history is fabricated for processes that have exited.
func buildAgentLineage(processes []systemProcess, events []eventSummary, rootPID, limit int, snapshot time.Time) agentLineage {
	graph := agentLineage{RootPID: rootPID, Snapshot: snapshot}
	if limit <= 0 {
		return graph
	}
	byPID := make(map[int]systemProcess, len(processes))
	for _, p := range processes {
		if p.PID > 0 {
			byPID[p.PID] = p
		}
	}
	root, ok := byPID[rootPID]
	if !ok || agentLabelForLiveProcess(root, events) == "" {
		return graph
	}
	children := make(map[int][]systemProcess)
	for _, p := range processes {
		parent, ok := byPID[p.PPID]
		if ok && validLineageEdge(parent, p) {
			children[parent.PID] = append(children[parent.PID], p)
		}
	}
	for pid := range children {
		sort.Slice(children[pid], func(i, j int) bool {
			a, b := children[pid][i], children[pid][j]
			if a.CreateTime != b.CreateTime {
				return a.CreateTime < b.CreateTime
			}
			return a.PID < b.PID
		})
	}

	seen := make(map[int]bool, min(limit, len(processes)))
	var visit func(p systemProcess, parent, depth int)
	visit = func(p systemProcess, parent, depth int) {
		if seen[p.PID] {
			return
		}
		if len(graph.Nodes) >= limit || depth > 6 {
			graph.Truncated = true
			return
		}
		seen[p.PID] = true
		role := "process"
		if parent < 0 {
			role = "agent"
		} else if agentLabelForLiveProcess(p, events) != "" {
			role = "subagent-candidate"
		}
		node := agentLineageNode{Process: p, Parent: parent, Depth: depth, Role: role}
		for _, e := range events {
			if !eventMatchesLiveProcess(e, p) {
				continue
			}
			node.Events++
			if e.RiskScore >= 60 || e.Type == "semantic_alert" || e.Type == "agentsight_alert" {
				node.Alerts++
			}
		}
		index := len(graph.Nodes)
		graph.Nodes = append(graph.Nodes, node)
		if depth == 6 {
			if len(children[p.PID]) > 0 {
				graph.Truncated = true
			}
			return
		}
		for _, child := range children[p.PID] {
			visit(child, index, depth+1)
		}
	}
	visit(root, -1, 0)
	return graph
}

func lineageIndexForPID(graph agentLineage, pid int) int {
	for i := range graph.Nodes {
		if graph.Nodes[i].Process.PID == pid {
			return i
		}
	}
	return -1
}

func lineageVisibleSpan(graph agentLineage) (int64, int64) {
	if len(graph.Nodes) == 0 {
		return 0, 0
	}
	lo, hi := int64(0), graph.Snapshot.Unix()
	for _, n := range graph.Nodes {
		start := n.Process.CreateTime
		if start > 0 && (lo == 0 || start < lo) {
			lo = start
		}
		if start > hi {
			hi = start
		}
	}
	if lo == 0 {
		lo = hi - 1
	}
	if hi <= lo {
		hi = lo + 1
	}
	return lo, hi
}
