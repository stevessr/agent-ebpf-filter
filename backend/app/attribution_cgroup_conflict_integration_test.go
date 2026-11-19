package app

import (
	"testing"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
)

// Regression across the real app→events dependency boundary: two distinct
// registered Agent roots share an OS cgroup, and an unrelated process from
// that cgroup must NEVER inherit whichever Agent registered most recently.
func TestCgroupAttributionSharedRootDoesNotMislabelUnrelatedKernelEvent(t *testing.T) {
	t.Setenv("AGENT_EBPF_RESEARCH_ATTRIBUTION_MODE", "combined")
	oldContexts := events.Deps.ProcessContexts
	oldGet := events.Deps.CgroupAttributionGet
	oldSet := events.Deps.CgroupAttributionSet
	defer func() {
		events.Deps.ProcessContexts = oldContexts
		events.Deps.CgroupAttributionGet = oldGet
		events.Deps.CgroupAttributionSet = oldSet
	}()

	contexts := events.NewProcessContextStore()
	cgroups := newCgroupAttributionStore()
	events.Deps.ProcessContexts = contexts
	events.Deps.CgroupAttributionGet = func(cgroupID uint64) (events.CgroupAttributionEntry, bool) {
		entry, ok := cgroups.Get(cgroupID)
		if !ok {
			return events.CgroupAttributionEntry{}, false
		}
		return events.CgroupAttributionEntry{
			CgroupID: entry.CgroupID, AgentRunID: entry.AgentRunID,
			RootAgentPID: entry.RootAgentPID, TaskID: entry.TaskID,
			ToolCallID: entry.ToolCallID,
		}, true
	}
	events.Deps.CgroupAttributionSet = func(cgroupID uint64, entry events.CgroupAttributionEntry) {
		cgroups.Set(cgroupID, cgroupAttributionEntry{
			AgentRunID: entry.AgentRunID, RootAgentPID: entry.RootAgentPID,
			TaskID: entry.TaskID, ToolCallID: entry.ToolCallID,
		})
	}

	const sharedCgroup = uint64(4123)
	for _, root := range []struct {
		pid uint32
		run string
	}{
		{pid: 111, run: "agent-A"},
		{pid: 222, run: "agent-B"},
	} {
		contexts.Set(root.pid, events.ProcessContext{AgentRunID: root.run, RootAgentPid: root.pid})
		got := events.EnrichEventContext(&pb.Event{
			Pid: root.pid, Type: "openat", CgroupId: sharedCgroup,
		})
		if got.AgentRunId != root.run {
			t.Fatalf("explicitly registered root lost its direct attribution: %+v", got)
		}
	}
	if entry, ok := cgroups.Get(sharedCgroup); ok {
		t.Fatalf("conflicting registered roots must make cgroup owner ambiguous: %+v", entry)
	}

	unrelated := events.EnrichEventContext(&pb.Event{
		Pid: 333, Type: "network_connect", CgroupId: sharedCgroup,
	})
	if unrelated.AgentRunId != "" || unrelated.RootAgentPid != 0 {
		t.Fatalf("shared cgroup leaked wrong Agent ID into unrelated event: %+v", unrelated)
	}
}
