package main

import (
	"strings"
	"testing"
)

func TestAgentSessionKeepsInterpreterFileEditsUnderSameAgent(t *testing.T) {
	events := []eventSummary{
		{EventID: "write", PID: 103, PPID: 102, RootAgentPID: 100, Comm: "python", Type: "file_write", Target: "/tmp/example.py", ToolName: "shell", AgentRunID: "run-a", ReceivedAtMS: 400},
		{EventID: "script", PID: 102, PPID: 101, RootAgentPID: 100, Comm: "node", Type: "process_exec", AgentRunID: "run-a", ReceivedAtMS: 300},
		{EventID: "shell", PID: 101, PPID: 100, RootAgentPID: 100, Comm: "fish", Type: "process_exec", AgentRunID: "run-a", ReceivedAtMS: 200},
		{EventID: "root", PID: 100, Comm: "codex", Tag: "Codex", Type: "hook", AgentRunID: "run-a", ReceivedAtMS: 100},
	}
	rows := aggregateAgentSessions(events)
	if len(rows) != 1 || rows[0].Events != 4 {
		t.Fatalf("indirect activities split into different sessions: %+v", rows)
	}
	if !strings.HasPrefix(rows[0].Label, "Codex") {
		t.Fatalf("the file edit was credited to an interpreter, not Codex: %+v", rows[0])
	}
	if !strings.Contains(rows[0].LastAction, "修改文件") ||
		!strings.Contains(rows[0].LastAction, "经 python") {
		t.Fatalf("the file modification or real executor is hidden: %+v", rows[0])
	}
	if got := eventAction(events[0]); got != "修改文件" {
		t.Fatalf("inherited shell tool name hid file modification: %q", got)
	}
	if eventSessionKey(events[0]) != eventSessionKey(events[3]) {
		t.Fatal("child syscall and native Agent event have different session keys")
	}
}

func TestDirectAgentWithoutRunStillSharesRootPIDSession(t *testing.T) {
	root := eventSummary{PID: 10, Comm: "claude", Tag: "Claude Code"}
	child := eventSummary{PID: 13, PPID: 11, RootAgentPID: 10, Comm: "pwsh", Type: "file_write"}
	if eventSessionKey(root) != eventSessionKey(child) {
		t.Fatalf("direct Agent and pwsh edit should share a root-PID key: %q != %q", eventSessionKey(root), eventSessionKey(child))
	}
	events := []eventSummary{root, child}
	if got := aggregateAgentSessions(events); len(got) != 1 || got[0].Events != 2 {
		t.Fatalf("did not merge direct and delegated events: %+v", got)
	}
}

func TestUnattributedShellIsNotAnAgent(t *testing.T) {
	plain := eventSummary{PID: 123, PPID: 10, Comm: "bash", Type: "file_write", Target: "/tmp/x"}
	if buildAgentOwnershipIndex([]eventSummary{plain}, nil).attribution(plain).IsAgent {
		t.Fatal("ordinary bash was attributed to an Agent without evidence")
	}
	if got := aggregateAgentSessions([]eventSummary{plain}); len(got) != 0 {
		t.Fatalf("ordinary bash created Agent session: %+v", got)
	}
	if got := aggregateAgentRecognition([]eventSummary{plain}, registrySnapshot{}); len(got) != 0 {
		t.Fatalf("ordinary bash created Agent recognition: %+v", got)
	}
}

func TestRecognitionCountsIndirectEditsAgainstRootAgent(t *testing.T) {
	events := []eventSummary{
		{PID: 101, PPID: 100, RootAgentPID: 100, Comm: "bash", Type: "file_write", ReceivedAtMS: 20},
		{PID: 102, PPID: 101, RootAgentPID: 100, Comm: "python", Type: "file_write", ReceivedAtMS: 30},
		{PID: 100, Comm: "codex", Tag: "Codex", HasAgentContext: true, ReceivedAtMS: 10},
	}
	rows := aggregateAgentRecognition(events, registrySnapshot{})
	if len(rows) != 1 || rows[0].Comm != "codex" || rows[0].PID != 100 || rows[0].Events != 3 {
		t.Fatalf("indirect edit must count once against root Agent, not bash/python: %+v", rows)
	}
}

func TestUnknownRootDoesNotInheritInterpreterIdentity(t *testing.T) {
	leaf := eventSummary{PID: 310, RootAgentPID: 300, Comm: "node", Type: "file_write", ReceivedAtMS: 1000}
	owner := buildAgentOwnershipIndex([]eventSummary{leaf}, nil).attribution(leaf)
	if owner.OwnerLabel != "Agent PID 300" || owner.OwnerComm != "" || !owner.Indirect {
		t.Fatalf("unknown root was incorrectly named node: %+v", owner)
	}
	// A process with the same PID that started after the historical event is
	// not the source of the event, even if it happens to be a known Agent.
	live := []systemProcess{{PID: 300, Name: "codex", CreateTime: 30}}
	owner = buildAgentOwnershipIndex([]eventSummary{leaf}, live).attribution(leaf)
	if owner.OwnerLabel != "Agent PID 300" {
		t.Fatalf("PID-reused live Agent misnamed historical event: %+v", owner)
	}
}

func TestGenericTaggedShellDoesNotBecomeAnAgent(t *testing.T) {
	for _, input := range []eventSummary{
		{PID: 201, Comm: "fish", Tag: "Shell", HasAgentContext: true, Type: "file_write"},
		{PID: 202, Comm: "python", Tag: "Runtime", HasAgentContext: true, Type: "file_write"},
		{PID: 203, Comm: "node", Tag: "Runtime", HasAgentContext: true, Type: "file_write"},
		{PID: 204, Comm: "pwsh", Tag: "Runtime", HasAgentContext: true, Type: "file_write"},
	} {
		if isAgentSummary(input) {
			t.Fatalf("generic executor became independent Agent: %+v", input)
		}
	}
	if !isAgentSummary(eventSummary{PID: 205, RootAgentPID: 100, Comm: "bash", Tag: "Shell", HasAgentContext: true}) {
		t.Fatal("confirmed descendant must still belong to its Agent")
	}
}

func TestHistoricalPIDReuseDoesNotRenameEarlierAgentRun(t *testing.T) {
	events := []eventSummary{
		{PID: 300, Comm: "claude", AgentRunID: "new-run", ReceivedAtMS: 4000},
		{PID: 310, RootAgentPID: 300, Comm: "python", Type: "file_write", AgentRunID: "old-run", ReceivedAtMS: 1000},
	}
	owner := buildAgentOwnershipIndex(events, nil).attribution(events[1])
	if owner.OwnerComm != "" || owner.OwnerLabel != "Agent PID 300" {
		t.Fatalf("historical edit misidentified as reused Claude PID: %+v", owner)
	}
}

func TestFileEditCountersDoNotCountNetworkWrites(t *testing.T) {
	events := []eventSummary{
		{PID: 100, Comm: "codex", Tag: "Codex", AgentRunID: "run-1", ReceivedAtMS: 10},
		{PID: 201, RootAgentPID: 100, Comm: "python", AgentRunID: "run-1", Type: "write", Target: "file write", ToolName: "exec", ReceivedAtMS: 20},
		{PID: 202, RootAgentPID: 100, Comm: "node", AgentRunID: "run-1", Type: "write", Target: "socket write", ReceivedAtMS: 30},
		{PID: 203, RootAgentPID: 100, Comm: "fish", AgentRunID: "run-1", Type: "network_write", Network: true, ReceivedAtMS: 40},
		{PID: 204, RootAgentPID: 100, Comm: "bash", AgentRunID: "run-1", Type: "open", Target: "/workspace/file", ReceivedAtMS: 50},
		{PID: 205, RootAgentPID: 100, Comm: "pwsh", AgentRunID: "run-1", Type: "renameat2", Target: "/workspace/file", ReceivedAtMS: 60},
	}
	rows := aggregateAgentSessions(events)
	if len(rows) != 1 || rows[0].FileEdits != 2 || rows[0].DelegatedEdits != 2 {
		t.Fatalf("delegated file edits counted incorrectly: %+v", rows)
	}
	if got := eventAction(events[1]); got != "修改文件" {
		t.Fatalf("file syscall should override inherited tool name, got %q", got)
	}
	if got := eventAction(events[2]); got == "修改文件" {
		t.Fatal("socket write misclassified as a file edit")
	}
	if isFileMutationSummary(eventSummary{Type: "openai_request"}) {
		t.Fatal("substring matching must not invent file operations")
	}
}
