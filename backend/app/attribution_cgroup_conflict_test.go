package app

import (
	"sync"
	"testing"
	"time"
)

func TestCgroupAttributionRejectsSharedAgentRoots(t *testing.T) {
	store := newCgroupAttributionStore()
	store.Set(33, cgroupAttributionEntry{AgentRunID: "agent-A", RootAgentPID: 100})
	first, ok := store.Get(33)
	if !ok || first.AgentRunID != "agent-A" {
		t.Fatalf("initial unique root was not found: %+v %v", first, ok)
	}
	store.Set(33, cgroupAttributionEntry{AgentRunID: "agent-B", RootAgentPID: 200})
	if entry, ok := store.Get(33); ok {
		t.Fatalf("shared cgroup must not return a winner: %+v", entry)
	}
	store.Set(33, cgroupAttributionEntry{AgentRunID: "agent-B", RootAgentPID: 200})
	if entry, ok := store.Get(33); ok {
		t.Fatalf("last writer reclaimed ambiguous cgroup: %+v", entry)
	}
}

func TestCgroupAttributionRetainsRootButNotConflictingTools(t *testing.T) {
	store := newCgroupAttributionStore()
	store.Set(33, cgroupAttributionEntry{
		AgentRunID: "run-1", RootAgentPID: 100, TaskID: "task-a", ToolCallID: "tool-a",
	})
	store.Set(33, cgroupAttributionEntry{
		AgentRunID: "run-1", RootAgentPID: 100, TaskID: "task-b", ToolCallID: "tool-b",
	})
	entry, ok := store.Get(33)
	if !ok || entry.AgentRunID != "run-1" || entry.RootAgentPID != 100 {
		t.Fatalf("known Agent root disappeared after distinct tool call: %+v %v", entry, ok)
	}
	if entry.TaskID != "" || entry.ToolCallID != "" {
		t.Fatalf("cgroup falsely attributed concurrent task or tool: %+v", entry)
	}
	store.Set(33, cgroupAttributionEntry{
		AgentRunID: "run-1", RootAgentPID: 100, TaskID: "task-b", ToolCallID: "tool-b",
	})
	entry, _ = store.Get(33)
	if entry.TaskID != "" || entry.ToolCallID != "" {
		t.Fatalf("later tool call reclaimed ambiguous task attribution: %+v", entry)
	}
}

func TestCgroupAttributionRootPIDCollisionIsAmbiguous(t *testing.T) {
	store := newCgroupAttributionStore()
	store.Set(1, cgroupAttributionEntry{AgentRunID: "same-name-run", RootAgentPID: 100})
	store.Set(1, cgroupAttributionEntry{AgentRunID: "same-name-run", RootAgentPID: 200})
	if _, ok := store.Get(1); ok {
		t.Fatal("identical run strings from distinct roots cannot establish shared cgroup ownership")
	}
}

func TestCgroupAttributionConcurrentWritersNeverPickLast(t *testing.T) {
	store := newCgroupAttributionStore()
	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(i int) {
			defer wg.Done()
			run := "run-A"
			root := uint32(10)
			if i%2 == 1 {
				run = "run-B"
				root = 20
			}
			store.Set(444, cgroupAttributionEntry{AgentRunID: run, RootAgentPID: root})
		}(i)
	}
	wg.Wait()
	if entry, ok := store.Get(444); ok {
		t.Fatalf("concurrent root conflict chose an arbitrary owner: %+v", entry)
	}
}

func TestCgroupAmbiguityExpiresWithGC(t *testing.T) {
	store := newCgroupAttributionStore()
	store.Set(55, cgroupAttributionEntry{
		AgentRunID: "A", RootAgentPID: 100, CreatedAt: time.Now().UTC().Add(-time.Hour),
	})
	store.Set(55, cgroupAttributionEntry{AgentRunID: "B", RootAgentPID: 200})
	if _, ok := store.Get(55); ok {
		t.Fatal("expected conflicting root tombstone")
	}
	// The tombstone must expire only after its latest conflict, not after
	// the first root registration.
	store.mu.Lock()
	aged := store.items[55]
	aged.CreatedAt = time.Now().UTC().Add(-time.Hour)
	store.items[55] = aged
	store.mu.Unlock()
	store.EvictOlderThan(30 * time.Minute)
	store.Set(55, cgroupAttributionEntry{AgentRunID: "C", RootAgentPID: 300})
	entry, ok := store.Get(55)
	if !ok || entry.AgentRunID != "C" {
		t.Fatalf("GC failed to allow freshly disjoint cgroup identity: %+v %v", entry, ok)
	}
}

func TestCgroupActiveAmbiguityRefreshesTTL(t *testing.T) {
	store := newCgroupAttributionStore()
	store.Set(999, cgroupAttributionEntry{
		AgentRunID: "A", RootAgentPID: 100,
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	})
	store.Set(999, cgroupAttributionEntry{AgentRunID: "B", RootAgentPID: 200})
	store.EvictOlderThan(30 * time.Minute)
	store.Set(999, cgroupAttributionEntry{AgentRunID: "C", RootAgentPID: 300})
	if entry, ok := store.Get(999); ok {
		t.Fatalf("active conflicting cgroup tombstone incorrectly expired: %+v", entry)
	}
}

func TestCgroupAttributionSkipsUnidentifiedRuns(t *testing.T) {
	store := newCgroupAttributionStore()
	store.Set(55, cgroupAttributionEntry{RootAgentPID: 333})
	if entry, ok := store.Get(55); ok {
		t.Fatalf("empty run must not establish owner: %+v", entry)
	}
}
