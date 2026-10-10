package eventnoise

import (
	"reflect"
	"testing"
)

func TestNormalizeAndBoundary(t *testing.T) {
	if got, _ := NormalizeIgnoredPaths(nil); got != nil {
		t.Fatal("nil should stay nil")
	}
	got, err := NormalizeIgnoredPaths([]string{" /proc/ ", "/tmp/cache/", "/sys/../proc", ""})
	if err != nil || !reflect.DeepEqual(got, []string{"/proc", "/tmp/cache"}) {
		t.Fatalf("normalized %v error %v", got, err)
	}
	for _, bad := range []string{"proc", "/"} {
		if _, err := NormalizeIgnoredPaths([]string{bad}); err == nil {
			t.Fatalf("expected bad path rejection for %s", bad)
		}
	}
	if !MatchesPrefix("/proc/1/status", []string{"/proc"}) ||
		MatchesPrefix("/procfs/1", []string{"/proc"}) ||
		MatchesPrefix("proc/1", []string{"/proc"}) {
		t.Fatal("path boundary failure")
	}
}

func TestAttentionEventsNeverSuppressed(t *testing.T) {
	for _, event := range []Event{
		{Path: "/proc/1/status", Decision: "BLOCK"},
		{Path: "/proc/1/status", Decision: "DENY"},
		{Path: "/proc/1/status", Decision: "ALERT"},
		{Path: "/proc/1/status", RiskScore: 60},
		{Path: "/proc/1/status", Type: "semantic_alert"},
		{Path: "/proc/1/status", Type: "agentsight_alert"},
	} {
		if ShouldIgnore(event, []string{"/proc"}) {
			t.Fatalf("suppressed attention event: %+v", event)
		}
	}
}

func TestNoiseKeepsMutationsAndExplicitEmptyConfiguration(t *testing.T) {
	cases := []struct {
		event Event
		ignore bool
	}{
		{Event{Path: "/sys/class/net/eth0", Type: "stat"}, true},
		{Event{Path: "/sys/class/net/eth0", Type: "unlink"}, false},
		{Event{Path: "/usr/bin/python", Type: "read"}, true},
		{Event{Path: "/usr/bin/python", Type: "execve"}, false},
		{Event{Path: "/usr/bin/python", Type: "write", Decision: "ALERT"}, false},
		{Event{Path: "/tmp/data", Type: "unlink"}, true}, // configured ignores retain prior semantics
		{Event{Path: "/proc/1/status", Type: "read"}, true},
	}
	for _, tc := range cases {
		if got := ShouldIgnore(tc.event, []string{"/proc", "/tmp"}); got != tc.ignore {
			t.Fatalf("ShouldIgnore(%+v) = %t, want %t", tc.event, got, tc.ignore)
		}
		if ShouldIgnore(tc.event, []string{}) {
			t.Fatalf("explicit [] ignored event %+v", tc.event)
		}
	}
}
