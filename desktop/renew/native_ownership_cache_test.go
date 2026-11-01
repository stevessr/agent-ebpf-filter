package main

import (
	"strconv"
	"testing"
)

func TestEventOwnershipIndexIsReusedWithoutPerRowAllocations(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	for i := range 1200 {
		a.events = append(a.events, eventSummary{
			EventID: strconv.Itoa(i + 1),
			PID: i + 1000,
			Comm: "codex",
			Tag: "Codex",
			ReceivedAtMS: int64(i + 1),
		})
	}
	first := a.eventOwnershipIndex()
	if !a.ownershipCacheValid || len(first.roots) == 0 {
		t.Fatal("first rendered row must build a usable ownership cache")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		_ = a.eventOwnershipIndex()
	}); allocations > 0 {
		t.Fatalf("steady native row attribution must not rebuild ownership maps: %v allocations", allocations)
	}
	if a.ownershipCacheVersion != a.eventsVersion ||
		a.ownershipCacheCount != len(a.events) {
		t.Fatal("ownership cache must be tied to event summary version and count")
	}
}

func TestEventOwnershipCacheInvalidatesWhenEvidenceChanges(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	leaf := eventSummary{EventID: "leaf", PID: 301, RootAgentPID: 300, Comm: "node", Type: "file_write", AgentRunID: "run-a", ReceivedAtMS: 200}
	root := eventSummary{EventID: "root", PID: 300, Comm: "codex", AgentRunID: "run-a", ReceivedAtMS: 100}
	a.events = []eventSummary{leaf, root}
	if got := a.eventOwnershipIndex().attribution(leaf); got.OwnerComm != "codex" || !got.Indirect {
		t.Fatalf("index did not recognize the verified Agent root: %+v", got)
	}

	// Normal native IPC merges always change the version. A different run
	// using the same PID must not inherit the old Agent identity.
	a.events = []eventSummary{
		leaf,
		{EventID: "root", PID: 300, Comm: "claude", AgentRunID: "run-b", ReceivedAtMS: 300},
	}
	a.eventsVersion++
	if got := a.eventOwnershipIndex().attribution(leaf); got.OwnerComm != "" || got.OwnerLabel != "Agent PID 300" {
		t.Fatalf("new run was incorrectly credited for old child event: %+v", got)
	}

	// Direct array replacements in offline/test views may not increment
	// eventsVersion. The bounded-window endpoint signature covers those.
	a.events = []eventSummary{
		{EventID: "new-leaf", PID: 401, RootAgentPID: 400, Comm: "python", Type: "file_write", AgentRunID: "run-c", ReceivedAtMS: 500},
		{EventID: "new-root", PID: 400, Comm: "codex", AgentRunID: "run-c", ReceivedAtMS: 400},
	}
	if got := a.eventOwnershipIndex().attribution(a.events[0]); got.OwnerPID != 400 || got.OwnerComm != "codex" {
		t.Fatalf("new offline event window retained stale root evidence: %+v", got)
	}
}

func TestEventOwnershipCacheDoesNotConsumeUnverifiedLivePIDNames(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	leaf := eventSummary{EventID: "old", PID: 311, RootAgentPID: 300, Comm: "node", Type: "file_write", ReceivedAtMS: 1000}
	a.events = []eventSummary{leaf}
	a.system.Processes = []systemProcess{{PID: 300, Name: "codex", CreateTime: 30}}
	if got := a.eventOwnershipIndex().attribution(leaf); got.OwnerComm != "" || got.OwnerLabel != "Agent PID 300" {
		t.Fatalf("unverified live PID was incorrectly used for historical Agent attribution: %+v", got)
	}
}
