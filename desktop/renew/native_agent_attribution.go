package main

import (
	"fmt"
	"strings"
)

// agentOwnershipIndex resolves the actor from persisted Agent context, not the
// executable which happened to perform a syscall. bash/fish/python/pwsh/node
// are executors, not new Agent sessions. This is presentation-only: it never
// writes an inferred identity back into captured evidence.
type agentOwnershipIndex struct {
	roots map[int]agentRootIdentity
}

type agentRootIdentity struct {
	comm string
	tag  string
	seenAtMS int64
	agentRunID string
	// Live process names are only usable for events after process start. This
	// prevents an unrelated process reusing the same PID from naming history.
	startedAtMS int64
	live        bool
}

type eventAttribution struct {
	OwnerPID     int
	OwnerComm    string
	OwnerTag     string
	OwnerLabel   string
	ExecutorPID  int
	ExecutorComm string
	IsAgent      bool
	Indirect     bool
}

func buildAgentOwnershipIndex(events []eventSummary, processes []systemProcess) agentOwnershipIndex {
	index := agentOwnershipIndex{roots: make(map[int]agentRootIdentity)}
	// Events are generally newest-first. Prefer the newest root evidence, but
	// never use a child interpreter's comm as the root's identity.
	for i := len(events) - 1; i >= 0; i-- {
		e := events[i]
		if e.PID <= 0 || (e.RootAgentPID > 0 && e.RootAgentPID != e.PID) {
			continue
		}
		if !isAgentSummary(e) && harnessLabelFor(e.Tag, e.Comm) == "未识别" {
			continue
		}
		if strings.TrimSpace(e.Comm) != "" {
			index.roots[e.PID] = agentRootIdentity{comm: e.Comm, tag: e.Tag, seenAtMS: e.ReceivedAtMS, agentRunID: e.AgentRunID}
		}
	}
	// A verified current process can name its own PID; it cannot establish
	// historical ancestry or assign a child event without rootAgentPid.
	for _, p := range processes {
		if p.PID <= 0 || harnessLabelFor(p.Name) == "未识别" {
			continue
		}
		index.roots[p.PID] = agentRootIdentity{comm: p.Name, startedAtMS: p.CreateTime * 1000, live: true}
	}
	return index
}

func (index agentOwnershipIndex) attribution(e eventSummary) eventAttribution {
	a := eventAttribution{ExecutorPID: e.PID, ExecutorComm: e.Comm}
	if !isAgentSummary(e) {
		return a
	}
	a.IsAgent = true
	a.OwnerPID = e.RootAgentPID
	if a.OwnerPID <= 0 {
		a.OwnerPID = e.PID
	}
	a.Indirect = e.RootAgentPID > 0 && e.PID > 0 && e.RootAgentPID != e.PID
	a.OwnerTag = e.Tag
	if a.OwnerPID > 0 {
		if root, ok := index.roots[a.OwnerPID]; ok &&
			(!root.live || root.startedAtMS <= 0 || e.ReceivedAtMS <= 0 || root.startedAtMS <= e.ReceivedAtMS) &&
			(root.agentRunID == "" || e.AgentRunID == "" || root.agentRunID == e.AgentRunID) &&
			(root.live || root.seenAtMS <= 0 || e.ReceivedAtMS <= 0 || root.seenAtMS <= e.ReceivedAtMS ||
				(root.agentRunID != "" && root.agentRunID == e.AgentRunID)) {
			a.OwnerComm = root.comm
			if strings.TrimSpace(root.tag) != "" && !strings.EqualFold(root.tag, "Unknown") {
				a.OwnerTag = root.tag
			}
		}
	}
	if !a.Indirect && a.OwnerComm == "" {
		a.OwnerComm = e.Comm
	}
	a.OwnerLabel = harnessLabelFor(a.OwnerTag, a.OwnerComm)
	if a.OwnerLabel == "未识别" {
		switch {
		case a.OwnerComm != "":
			a.OwnerLabel = a.OwnerComm
		case a.OwnerTag != "" && !strings.EqualFold(a.OwnerTag, "Unknown"):
			a.OwnerLabel = a.OwnerTag
		case a.OwnerPID > 0:
			a.OwnerLabel = fmt.Sprintf("Agent PID %d", a.OwnerPID)
		default:
			a.OwnerLabel = "Agent（未识别）"
		}
	}
	return a
}

func attributionExecutorLabel(a eventAttribution) string {
	if !a.Indirect {
		return ""
	}
	comm := strings.TrimSpace(a.ExecutorComm)
	if comm == "" {
		comm = "未知执行进程"
	}
	if a.ExecutorPID > 0 {
		return fmt.Sprintf("经 %s · PID %d", comm, a.ExecutorPID)
	}
	return "经 " + comm
}

func eventSessionDisplayLabel(e eventSummary, a eventAttribution) string {
	if !a.IsAgent {
		return ""
	}
	label := a.OwnerLabel
	switch {
	case strings.TrimSpace(e.AgentRunID) != "":
		return label + " · " + e.AgentRunID
	case strings.TrimSpace(e.ConversationID) != "":
		return label + " · " + e.ConversationID
	case a.OwnerPID > 0:
		return fmt.Sprintf("%s · PID %d", label, a.OwnerPID)
	default:
		return label
	}
}
