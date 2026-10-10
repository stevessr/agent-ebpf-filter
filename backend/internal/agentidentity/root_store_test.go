package agentidentity

import (
	"testing"
	"time"
)

func TestVerifiedDescendantAndCrossRun(t *testing.T) {
	s := NewStore()
	s.Observe(Evidence{PID: 100, RootPID: 100, Comm: " codex ", RunID: "run-a"})
	child := Evidence{PID: 101, RootPID: 100, Comm: "fish", RunID: "run-a"}
	if owner := s.OwnerComm(child); owner != "codex" {
		t.Fatalf("verified owner %q", owner)
	}
	child.RunID = "run-b"
	if got := s.OwnerComm(child); got != "" {
		t.Fatalf("cross-run attribution: %q", got)
	}
	child.RunID = ""
	if got := s.OwnerComm(child); got != "" {
		t.Fatalf("unversioned child inherited run: %q", got)
	}
}

func TestChildCannotRegisterAsRoot(t *testing.T) {
	s := NewStore()
	s.Observe(Evidence{PID: 101, RootPID: 100, Comm: "malicious", RunID: "r"})
	if got := s.OwnerComm(Evidence{PID: 102, RootPID: 100, RunID: "r"}); got != "" {
		t.Fatalf("untrusted child's claim leaked: %q", got)
	}
}

func TestExpirationWithoutSleep(t *testing.T) {
	now := time.Unix(100000, 0)
	s := NewStoreWithClock(func() time.Time { return now })
	s.Observe(Evidence{PID: 100, RootPID: 100, Comm: "codex"})
	child := Evidence{PID: 101, RootPID: 100}
	if got := s.OwnerComm(child); got != "codex" {
		t.Fatalf("expected root before TTL, got %q", got)
	}
	now = now.Add(RootTTL + time.Second)
	if got := s.OwnerComm(child); got != "" {
		t.Fatalf("stale root accepted: %q", got)
	}
}

func TestExitAndPIDReuse(t *testing.T) {
	s := NewStore()
	root := Evidence{PID: 100, RootPID: 100, Comm: "codex"}
	child := Evidence{PID: 101, RootPID: 100}
	s.Observe(root)
	s.Observe(Evidence{PID: 100, RootPID: 100, Comm: "codex", EventType: "exit"})
	if got := s.OwnerComm(child); got != "" {
		t.Fatalf("exited unversioned root accepted: %q", got)
	}

	root.RunID = "run-old"
	s.Observe(root)
	s.Observe(Evidence{PID: 100, RootPID: 100, Comm: "codex", EventType: "exit", RunID: "run-old"})
	child.RunID = "run-old"
	if got := s.OwnerComm(child); got != "codex" {
		t.Fatalf("named run should remain valid on root exit, got %q", got)
	}
	// An event missing run ID may not silently downgrade the verified root.
	s.Observe(Evidence{PID: 100, RootPID: 100, Comm: "other"})
	if got := s.OwnerComm(child); got != "codex" {
		t.Fatalf("unversioned event replaced root: %q", got)
	}
	s.Observe(Evidence{PID: 100, RootPID: 100, Comm: "other", RunID: "run-new"})
	if got := s.OwnerComm(child); got != "" {
		t.Fatalf("old run survived PID reuse: %q", got)
	}
}

func TestBoundedCapacityAndEviction(t *testing.T) {
	now := time.Unix(100000, 0)
	s := NewStoreWithClock(func() time.Time { return now })
	for i := uint32(1); i <= MaxRoots; i++ {
		s.Observe(Evidence{PID: i, RootPID: i, Comm: "agent"})
	}
	s.Observe(Evidence{PID: MaxRoots + 1, RootPID: MaxRoots + 1, Comm: "ignored"})
	if got := s.OwnerComm(Evidence{PID: MaxRoots + 2, RootPID: MaxRoots + 1}); got != "" {
		t.Fatalf("capacity exceeded: %q", got)
	}
	now = now.Add(RootTTL + time.Second)
	s.Observe(Evidence{PID: MaxRoots + 1, RootPID: MaxRoots + 1, Comm: "new"})
	if got := s.OwnerComm(Evidence{PID: MaxRoots + 2, RootPID: MaxRoots + 1}); got != "new" {
		t.Fatalf("expired entries were not reclaimed: %q", got)
	}
}
