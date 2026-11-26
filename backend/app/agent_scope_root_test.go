package app

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestAgentScopeDescendantsHonorRootWhitelistAndBlacklist(t *testing.T) {
	store := newAgentRootScopeStore()
	root := &pb.Event{Pid: 100, RootAgentPid: 100, Comm: "codex", Tag: "Agent CLI", AgentRunId: "run-c"}
	viaFish := &pb.Event{Pid: 110, RootAgentPid: 100, Comm: "fish", Tag: "Agent CLI", AgentRunId: "run-c"}
	viaPython := &pb.Event{Pid: 111, RootAgentPid: 100, Comm: "python", Tag: "Agent CLI", AgentRunId: "run-c"}
	store.Observe(root)
	for _, child := range []*pb.Event{viaFish, viaPython} {
		name := store.OwnerComm(child)
		if name != "codex" {
			t.Fatalf("child lost verified Agent owner: owner=%q event=%+v", name, child)
		}
		if !agentScopeAllowsWithRoot(agentScopeList{Mode: "whitelist", Entries: []string{"codex"}}, child, name) {
			t.Fatalf("Agent whitelist dropped delegated interpreter event: %+v", child)
		}
		if agentScopeAllowsWithRoot(agentScopeList{Mode: "blacklist", Entries: []string{"codex"}}, child, name) {
			t.Fatalf("Agent blacklist leaked delegated interpreter event: %+v", child)
		}
	}
}

func TestAgentScopeNoCrossRunOrUnobservedRootAttribution(t *testing.T) {
	store := newAgentRootScopeStore()
	store.Observe(&pb.Event{Pid: 100, RootAgentPid: 100, Comm: "codex", AgentRunId: "old"})
	cases := []*pb.Event{
		{Pid: 200, RootAgentPid: 100, Comm: "bash", AgentRunId: "new"},
		{Pid: 201, RootAgentPid: 300, Comm: "python", AgentRunId: "old"},
		{Pid: 202, RootAgentPid: 100, Comm: "node", AgentRunId: ""},
	}
	for _, event := range cases[:2] {
		if name := store.OwnerComm(event); name != "" {
			t.Fatalf("unverified Agent root was accepted: %q", name)
		}
	}
	// Missing run identifier is not enough to validate the cached
	// run identity, especially when a PID could have been reused.
	if name := store.OwnerComm(cases[2]); name != "" {
		t.Fatalf("unversioned child inherited a named Agent run: %q", name)
	}
	if name := store.OwnerComm(&pb.Event{Pid: 401, RootAgentPid: 401, Comm: "python"}); name != "" {
		t.Fatalf("direct process unexpectedly has a second Agent owner: %q", name)
	}
}

func TestAgentScopeRootCacheDoesNotTrustChildAssertions(t *testing.T) {
	store := newAgentRootScopeStore()
	store.Observe(&pb.Event{Pid: 102, RootAgentPid: 100, Comm: "codex", AgentRunId: "fake"})
	if got := store.OwnerComm(&pb.Event{Pid: 103, RootAgentPid: 100, Comm: "bash", AgentRunId: "fake"}); got != "" {
		t.Fatalf("child spoofed a root identity: %q", got)
	}
}

func TestAgentScopeRootCacheExpiresAndBounds(t *testing.T) {
	now := time.Now()
	store := newAgentRootScopeStoreWithClock(func() time.Time { return now })
	store.Observe(&pb.Event{Pid: 100, RootAgentPid: 100, Comm: "codex"})
	now = now.Add(agentRootScopeTTL + time.Minute)
	if got := store.OwnerComm(&pb.Event{Pid: 101, RootAgentPid: 100}); got != "" {
		t.Fatalf("stale root was trusted: %q", got)
	}
	if agentScopeAllowsWithRoot(agentScopeList{Mode: "whitelist", Entries: []string{"codex"}}, &pb.Event{Pid: 101, Comm: "bash"}, "") {
		t.Fatal("root whitelist accepted an unattributed ordinary shell")
	}
}

// Both legal snapshots have equal predicates for this event. A mixed read
// would produce an impossible pair under concurrent publication.
func TestAgentScopeAdmissionUsesOneSnapshot(t *testing.T) {
	agentScopes.writeMu.Lock()
	defer agentScopes.writeMu.Unlock()
	old := currentAgentScopePolicy()
	defer agentScopes.snapshot.Store(old)
	allow := agentScopePolicy{
		Capture: agentScopeList{Mode: agentScopeBlacklist},
		Monitor: agentScopeList{Mode: agentScopeBlacklist},
	}
	deny := agentScopePolicy{
		Capture: agentScopeList{Mode: agentScopeWhitelist},
		Monitor: agentScopeList{Mode: agentScopeWhitelist},
	}
	agentScopes.snapshot.Store(&allow)
	event := &pb.Event{Comm: "axiomatic-fixture"}
	if got := evaluateAgentEventScope(event); !got.Capture || !got.Monitor {
		t.Fatalf("allow snapshot: %+v", got)
	}
	agentScopes.snapshot.Store(&deny)
	if got := evaluateAgentEventScope(event); got.Capture || got.Monitor {
		t.Fatalf("deny snapshot: %+v", got)
	}
	if got := evaluateAgentEventScope(nil); got.Capture || got.Monitor {
		t.Fatalf("nil event admission: %+v", got)
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				agentScopes.snapshot.Store(&allow)
				agentScopes.snapshot.Store(&deny)
			}
		}
	}()
	defer func() { close(stop); <-done }()
	for i := 0; i < 10000; i++ {
		got := evaluateAgentEventScope(event)
		if got.Capture != got.Monitor {
			t.Fatalf("mixed scope snapshot: %+v", got)
		}
	}
}
