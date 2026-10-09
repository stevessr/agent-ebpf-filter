package main

import "testing"

func TestAgentScopeNames(t *testing.T) {
	items := agentScopeNames(" Codex, claude；CODEX\n Gemini CLI")
	if len(items) != 3 || items[0] != "codex" || items[1] != "claude" || items[2] != "gemini cli" {
		t.Fatalf("names parsed incorrectly: %#v", items)
	}
	if got := appendScopeEntry("codex, Claude", "CODEX"); got != "codex, Claude" {
		t.Fatalf("duplicate entry inserted: %q", got)
	}
}

func TestAgentScopePresentationMatchesServerSemantics(t *testing.T) {
	capture := agentScopeList{Mode: "whitelist", Entries: []string{"codex"}}
	monitor := agentScopeList{Mode: "blacklist", Entries: []string{"codex"}}
	if !scopeMatches(capture, "Codex", "") || scopeMatches(monitor, "Codex", "") {
		t.Fatal("capture and monitor rule evaluation differs")
	}
	if scopeMatches(capture, "bash", "") || !scopeMatches(monitor, "bash", "") {
		t.Fatal("unlisted commands should follow selected mode")
	}
}

func TestAgentRecognitionDoesNotInventOrdinaryAgents(t *testing.T) {
	events := []eventSummary{
		{Comm: "bash", PID: 10, ReceivedAtMS: 100},
		{Comm: "codex", PID: 11, Tag: "Codex", HasAgentContext: true, ReceivedAtMS: 200},
	}
	rows := aggregateAgentRecognition(events, registrySnapshot{})
	if len(rows) != 1 || rows[0].Comm != "codex" || rows[0].PID != 11 {
		t.Fatalf("agent recognition rows: %#v", rows)
	}
}

func TestAgentRecognitionSurvivesCaptureWhitelistAndSeparatesPIDs(t *testing.T) {
	processes := []systemProcess{
		{PID: 101, Name: "codex", CPU: 1.5},
		{PID: 202, Name: "codex", CPU: 2.5},
		{PID: 303, Name: "bash"},
	}
	rows := aggregateAgentRecognitionWithProcesses(nil, registrySnapshot{}, processes)
	if len(rows) != 2 {
		t.Fatalf("expected both live Agent instances, not ordinary bash: %#v", rows)
	}
	pids := map[int]bool{}
	for _, row := range rows {
		pids[row.PID] = true
		if row.Source != "实时进程" || row.Label != "Codex" {
			t.Fatalf("unexpected process recognition: %#v", row)
		}
	}
	if !pids[101] || !pids[202] {
		t.Fatalf("lost one Agent PID: %#v", pids)
	}
}

func TestRegisteredAgentVisibleWithoutEventsOrProcess(t *testing.T) {
	registry := registrySnapshot{Comms: []trackedComm{
		{Comm: "my-agent", Tag: "Custom Agent", Disabled: true},
	}}
	rows := aggregateAgentRecognitionWithProcesses(nil, registry, nil)
	if len(rows) != 1 || rows[0].PID != 0 || rows[0].Source != "跟踪注册表" || !rows[0].Disabled {
		t.Fatalf("registered Agent missing: %#v", rows)
	}
}
