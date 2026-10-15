package main

import (
	"sort"
	"strings"
	"time"
)

// The graph is an OS process snapshot, not a causal Agent SDK span graph.
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

type lineageEvidence struct {
	Label  string
	Events int
	Alerts int
}

// Native hooks can provide evidence of a logical SubAgent lifecycle, even
// when the agent runs in the same OS process. A bare start/stop event does
// not prove the duration, result, or parent call ID.
type agentLineageSignal struct {
	PID      int
	WhenMS   int64
	Name     string
	Provider string
	Kind     string // start or stop
}

func agentProcessName(p systemProcess) string {
	if v := strings.TrimSpace(p.Name); v != "" {
		return v
	}
	return "unknown"
}

// A PID is only a current identity if the event was emitted after that
// process started. A missing timestamp cannot establish that identity.
func eventMatchesLiveProcess(e eventSummary, p systemProcess) bool {
	if e.PID != p.PID || e.PID <= 0 {
		return false
	}
	if p.CreateTime > 0 && (e.ReceivedAtMS <= 0 || e.ReceivedAtMS < p.CreateTime*1000-2000) {
		return false
	}
	return true
}

func lineageDirectAgentEvidence(e eventSummary, p systemProcess) string {
	if !eventMatchesLiveProcess(e, p) {
		return ""
	}
	// AgentRunID is inherited by ordinary shell executors and cannot
	// establish a new Agent root. Explicit root attribution or a native hook
	// emitted by this PID is stronger evidence.
	if e.RootAgentPID != p.PID {
		if e.Type != "native_hook" || e.RootAgentPID > 0 {
			return ""
		}
		// A native Hook's relay may run under a shell launcher; the
		// best-effort $PPID cannot turn that shell into a new Agent.
		switch strings.ToLower(strings.TrimSpace(p.Name)) {
		case "sh", "bash", "dash", "zsh", "fish", "pwsh", "powershell", "curl":
			return ""
		}
	}
	if label := harnessLabelFor(e.Tag); label != "未识别" {
		return label
	}
	if e.Type == "native_hook" {
		if label := harnessLabelFor(e.Comm); label != "未识别" {
			return label
		}
	}
	return ""
}

func agentLabelForLiveProcess(p systemProcess, events []eventSummary) string {
	if label := harnessLabelFor(p.Name); label != "未识别" {
		return label
	}
	for _, e := range events {
		if label := lineageDirectAgentEvidence(e, p); label != "" {
			return label
		}
	}
	return ""
}

// Index once: O(processes + events) rather than rescanning each event
// for each process and once again for every visible graph node.
func indexLineageEvidence(processes []systemProcess, events []eventSummary) map[int]lineageEvidence {
	byPID := make(map[int]systemProcess, len(processes))
	out := make(map[int]lineageEvidence, len(processes))
	for _, p := range processes {
		if p.PID <= 0 {
			continue
		}
		byPID[p.PID] = p
		label := harnessLabelFor(p.Name)
		if label == "未识别" {
			label = ""
		}
		out[p.PID] = lineageEvidence{Label: label}
	}
	for _, e := range events {
		p, ok := byPID[e.PID]
		if !ok || !eventMatchesLiveProcess(e, p) {
			continue
		}
		facts := out[p.PID]
		facts.Events++
		if eventRisk(e) != "正常" || e.Type == "semantic_alert" || e.Type == "agentsight_alert" {
			facts.Alerts++
		}
		if facts.Label == "" {
			facts.Label = lineageDirectAgentEvidence(e, p)
		}
		out[p.PID] = facts
	}
	return out
}

func agentLineageRoots(processes []systemProcess, events []eventSummary) []systemProcess {
	facts := indexLineageEvidence(processes, events)
	out := make([]systemProcess, 0, 16)
	for _, p := range processes {
		if p.PID > 0 && facts[p.PID].Label != "" {
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
	// Suppress impossible links after PID reuse. If either start time is
	// missing, the link remains only a best-effort live PPID observation.
	if parent.CreateTime > 0 && child.CreateTime > 0 && child.CreateTime < parent.CreateTime {
		return false
	}
	return true
}

// buildAgentLineage follows one live snapshot, with cycle, depth and size
// limits. Exited processes and unknown causal RPC edges are not fabricated.
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
	facts := indexLineageEvidence(processes, events)
	root, ok := byPID[rootPID]
	if !ok || facts[rootPID].Label == "" {
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
		} else if facts[p.PID].Label != "" {
			role = "subagent-candidate"
		}
		evidence := facts[p.PID]
		index := len(graph.Nodes)
		graph.Nodes = append(graph.Nodes, agentLineageNode{
			Process: p, Parent: parent, Depth: depth, Role: role,
			Events: evidence.Events, Alerts: evidence.Alerts,
		})
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

func classifySubAgentHook(e eventSummary) (string, string, bool) {
	if !strings.EqualFold(e.Type, "native_hook") {
		return "", "", false
	}
	name, _, _ := strings.Cut(strings.TrimSpace(e.Comm), ":")
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "subagentstart", "agentspawn":
		return "start", name, true
	case "subagentstop":
		return "stop", name, true
	default:
		return "", "", false
	}
}

func collectAgentLineageSignals(graph agentLineage, events []eventSummary, limit int) []agentLineageSignal {
	if limit <= 0 {
		return nil
	}
	byPID := make(map[int]systemProcess, len(graph.Nodes))
	for _, node := range graph.Nodes {
		byPID[node.Process.PID] = node.Process
	}
	signals := make([]agentLineageSignal, 0)
	for _, e := range events {
		kind, name, ok := classifySubAgentHook(e)
		if !ok {
			continue
		}
		p, found := byPID[e.PID]
		if !found || !eventMatchesLiveProcess(e, p) || e.ReceivedAtMS <= 0 {
			continue
		}
		// A descendant hook may be attributed to this root explicitly. A
		// hook emitted by the graph's root process also qualifies.
		if e.PID != graph.RootPID && e.RootAgentPID != graph.RootPID {
			continue
		}
		signals = append(signals, agentLineageSignal{
			PID: e.PID, WhenMS: e.ReceivedAtMS, Name: name,
			Provider: e.Tag, Kind: kind,
		})
	}
	sort.Slice(signals, func(i, j int) bool {
		if signals[i].WhenMS != signals[j].WhenMS {
			return signals[i].WhenMS > signals[j].WhenMS
		}
		return signals[i].PID < signals[j].PID
	})
	if len(signals) > limit {
		signals = signals[:limit]
	}
	return signals
}
