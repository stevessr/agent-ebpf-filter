package events

import (
	"testing"

	"agent-ebpf-filter/pb"
)

// These are research-only isolation tests; normal production mode has no
// AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE setting.
func TestResearchPIDOnlyNeverInheritsChildOwner(t *testing.T) {
	t.Setenv("AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE", "pid-only")
	old := Deps.ProcessContexts
	store := NewProcessContextStore()
	Deps.ProcessContexts = store
	defer func() { Deps.ProcessContexts = old }()
	store.Set(100, ProcessContext{RootAgentPid: 100, AgentRunID: "agent-root"})
	root := EnrichEventContext(&pb.Event{Pid: 100, Type: "openat"})
	if root.AgentRunId != "agent-root" {
		t.Fatalf("direct registered root not attributed: %+v", root)
	}
	fork := EnrichEventContext(&pb.Event{Pid: 100, Ppid: 1, Type: "process_fork", ExtraInfo: "child_pid=101"})
	if fork.AgentRunId != "agent-root" {
		t.Fatalf("root fork event lost its direct attribution: %+v", fork)
	}
	child := EnrichEventContext(&pb.Event{Pid: 101, Ppid: 100, Type: "openat", CgroupId: 77})
	if child.AgentRunId != "" || child.RootAgentPid != 0 {
		t.Fatalf("PID-only mode leaked parent or cgroup owner: %+v", child)
	}
	if _, exists := store.Get(101); exists {
		t.Fatal("PID-only mode illegally propagated a new child context")
	}
}

func TestResearchLifecycleExcludesCgroupFallback(t *testing.T) {
	t.Setenv("AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE", "lifecycle-only")
	oldStore, oldGet, oldSet := Deps.ProcessContexts, Deps.CgroupAttributionGet, Deps.CgroupAttributionSet
	defer func() {
		Deps.ProcessContexts, Deps.CgroupAttributionGet, Deps.CgroupAttributionSet = oldStore, oldGet, oldSet
	}()
	store := NewProcessContextStore()
	Deps.ProcessContexts = store
	Deps.CgroupAttributionGet = func(uint64) (CgroupAttributionEntry, bool) {
		t.Fatal("lifecycle-only must not consult cgroup")
		return CgroupAttributionEntry{}, false
	}
	Deps.CgroupAttributionSet = func(uint64, CgroupAttributionEntry) {
		t.Fatal("lifecycle-only must not mutate cgroup ownership")
	}
	store.Set(100, ProcessContext{AgentRunID: "root-100", RootAgentPid: 100})
	child := EnrichEventContext(&pb.Event{Pid: 101, Ppid: 100, Type: "openat", CgroupId: 77})
	if child.AgentRunId != "root-100" || child.RootAgentPid != 100 {
		t.Fatalf("trusted parent lineage not propagated: %+v", child)
	}
	unrelated := EnrichEventContext(&pb.Event{Pid: 999, Ppid: 0, Type: "network_connect", CgroupId: 77})
	if unrelated.AgentRunId != "" {
		t.Fatalf("lifecycle-only used shared cgroup: %+v", unrelated)
	}
}

func TestResearchCombinedStillSupportsUniqueCgroupFallback(t *testing.T) {
	t.Setenv("AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE", "combined")
	oldStore, oldGet, oldSet := Deps.ProcessContexts, Deps.CgroupAttributionGet, Deps.CgroupAttributionSet
	defer func() {
		Deps.ProcessContexts, Deps.CgroupAttributionGet, Deps.CgroupAttributionSet = oldStore, oldGet, oldSet
	}()
	Deps.ProcessContexts = NewProcessContextStore()
	Deps.CgroupAttributionGet = func(id uint64) (CgroupAttributionEntry, bool) {
		if id != 88 {
			return CgroupAttributionEntry{}, false
		}
		return CgroupAttributionEntry{RootAgentPID: 777, AgentRunID: "isolated-run"}, true
	}
	Deps.CgroupAttributionSet = func(uint64, CgroupAttributionEntry) {}
	got := EnrichEventContext(&pb.Event{Pid: 900, Type: "network_connect", CgroupId: 88})
	if got.AgentRunId != "isolated-run" || got.RootAgentPid != 777 {
		t.Fatalf("combined mode lost cgroup fallback: %+v", got)
	}
}

func TestResearchInvalidModeFailsClosed(t *testing.T) {
	t.Setenv("AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE", "typo")
	old := Deps.ProcessContexts
	defer func() { Deps.ProcessContexts = old }()
	Deps.ProcessContexts = NewProcessContextStore()
	Deps.ProcessContexts.Set(100, ProcessContext{AgentRunID: "should-not-assign"})
	if result := EnrichEventContext(&pb.Event{Pid: 100, Type: "openat"}); result.AgentRunId != "" {
		t.Fatalf("invalid research mode unexpectedly attributed event: %+v", result)
	}
}
