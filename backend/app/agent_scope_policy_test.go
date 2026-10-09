package app

import (
	"strings"
	"testing"
)

func TestAgentScopeDefaultsArePermissive(t *testing.T) {
	cfg := defaultAgentScopePolicy()
	if !agentScopeAllows(cfg.Capture, "codex", "") ||
		!agentScopeAllows(cfg.Monitor, "codex", "") {
		t.Fatal("default policies must preserve existing capture/monitor behavior")
	}
}

func TestAgentScopeIndependentCaptureAndMonitoring(t *testing.T) {
	cfg := agentScopePolicy{
		Capture: agentScopeList{Mode: "whitelist", Entries: []string{"codex", "Claude Code"}},
		Monitor: agentScopeList{Mode: "blacklist", Entries: []string{"claude code"}},
	}
	normalized, err := validateAgentScopePolicy(cfg)
	if err != nil { t.Fatal(err) }
	if !agentScopeAllows(normalized.Capture, "codex", "") || !agentScopeAllows(normalized.Monitor, "codex", "") {
		t.Fatal("codex should be captured and monitored")
	}
	if !agentScopeAllows(normalized.Capture, "node", "Claude Code") {
		t.Fatal("resolved Agent tag should be eligible for capture")
	}
	if agentScopeAllows(normalized.Monitor, "node", "Claude Code") {
		t.Fatal("monitor blacklist must operate independently from capture whitelist")
	}
	if agentScopeAllows(normalized.Capture, "bash", "") {
		t.Fatal("unknown commands must not pass a capture whitelist")
	}
}

func TestAgentScopeEmptyWhitelistRejectsAll(t *testing.T) {
	empty := agentScopeList{Mode: "whitelist"}
	if agentScopeAllows(empty, "codex", "Codex") || agentScopeAllows(empty, "", "") {
		t.Fatal("empty whitelist must fail closed")
	}
	if !agentScopeAllows(agentScopeList{Mode: "blacklist"}, "", "") {
		t.Fatal("empty blacklist must preserve original behavior")
	}
}

func TestAgentScopeNormalizeAndValidate(t *testing.T) {
	list, err := validateAgentScopeList(agentScopeList{
		Mode: "blacklist", Entries: []string{" Codex ", "codex", "Claude Code"},
	})
	if err != nil { t.Fatal(err) }
	if len(list.Entries) != 2 || list.Entries[0] != "codex" || list.Entries[1] != "claude code" {
		t.Fatalf("incorrect normalized entries: %#v", list.Entries)
	}
	for _, bad := range []agentScopeList{
		{Mode: "unknown"},
		{Mode: "whitelist", Entries: []string{""}},
		{Mode: "blacklist", Entries: []string{"bad\nname"}},
		{Mode: "blacklist", Entries: []string{strings.Repeat("x", 129)}},
		{Mode: "whitelist", Entries: make([]string, agentScopeMaxEntries+1)},
	} {
		if _, err := validateAgentScopeList(bad); err == nil {
			t.Fatalf("invalid policy accepted: %#v", bad)
		}
	}
}
