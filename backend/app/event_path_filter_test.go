package app

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestDefaultIgnoredEventPaths(t *testing.T) {
	paths := defaultIgnoredEventPaths()
	if len(paths) != 1 || paths[0] != "/proc" {
		t.Fatalf("default ignored paths = %v, want [/proc]", paths)
	}
}

func TestNormalizeIgnoredEventPaths(t *testing.T) {
	got, err := normalizeIgnoredEventPaths([]string{" /proc/ ", "/sys/../proc", "/tmp/cache/"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/proc", "/tmp/cache"}
	if len(got) != len(want) {
		t.Fatalf("normalized = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalized = %v, want %v", got, want)
		}
	}

	for _, value := range []string{"proc", "/"} {
		if _, err := normalizeIgnoredEventPaths([]string{value}); err == nil {
			t.Fatalf("normalizeIgnoredEventPaths(%q) succeeded, want error", value)
		}
	}
}

func TestPathMatchesIgnoredPrefixUsesPathBoundary(t *testing.T) {
	ignored := []string{"/proc"}
	for _, path := range []string{"/proc", "/proc/1/status", "/proc/self/fd/1"} {
		if !pathMatchesIgnoredPrefix(path, ignored) {
			t.Fatalf("%q should match /proc", path)
		}
	}
	for _, path := range []string{"/procfs", "/home/user/proc", "proc/1/status"} {
		if pathMatchesIgnoredPrefix(path, ignored) {
			t.Fatalf("%q should not match /proc", path)
		}
	}
}

func TestShouldIgnoreEventPathKeepsAttentionEvents(t *testing.T) {
	previous := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{
		settings: RuntimeSettings{IgnoredPaths: []string{"/proc"}},
	}
	t.Cleanup(func() { runtimeSettingsStore = previous })

	if !shouldIgnoreEventPath(&pb.Event{Path: "/proc/1/status"}) {
		t.Fatal("routine /proc event should be ignored")
	}
	if !shouldIgnoreEventPath(&pb.Event{ExtraPath: "/proc/self/fd/3"}) {
		t.Fatal("routine /proc extra path should be ignored")
	}
	if shouldIgnoreEventPath(&pb.Event{Path: "/procfs/status"}) {
		t.Fatal("/procfs must not be ignored by the /proc prefix")
	}

	attention := []*pb.Event{
		{Path: "/proc/1/status", Decision: "BLOCK"},
		{Path: "/proc/1/status", Decision: "DENY"},
		{Path: "/proc/1/status", Decision: "ALERT"},
		{Path: "/proc/1/status", RiskScore: 60},
		{Path: "/proc/1/status", Type: "semantic_alert"},
		{Path: "/proc/1/status", Type: "agentsight_alert"},
	}
	for _, event := range attention {
		if shouldIgnoreEventPath(event) {
			t.Fatalf("attention event was incorrectly ignored: %+v", event)
		}
	}
}
