package app

import (
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/internal/agentscope"
	"agent-ebpf-filter/pb"
)

// agentRootScopeStore keeps a bounded mapping of *observed* Agent roots to the
// command which owns a forked process. A child cannot announce its own owner:
// only a direct event where pid == root_agent_pid can establish a name.
// This store is used for scope admission only; it does not modify audit data.
type agentRootScopeEntry struct {
	Comm     string
	RunID    string
	Observed time.Time
}

type agentRootScopeStore struct {
	mu    sync.RWMutex
	items map[uint32]agentRootScopeEntry
}

const (
	agentRootScopeMaxEntries = 4096
	agentRootScopeTTL        = 24 * time.Hour
)

func newAgentRootScopeStore() *agentRootScopeStore {
	return &agentRootScopeStore{items: make(map[uint32]agentRootScopeEntry)}
}

var agentRootScopes = newAgentRootScopeStore()

// Observe runs once after event context enrichment but before capture
// filtering, including when a root's first event is not admitted.
func (s *agentRootScopeStore) Observe(event *pb.Event) {
	if s == nil || event == nil || event.Pid == 0 || event.RootAgentPid == 0 ||
		event.RootAgentPid != event.Pid || strings.TrimSpace(event.Comm) == "" {
		return
	}
	comm := strings.TrimSpace(event.Comm)
	run := strings.TrimSpace(event.AgentRunId)
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = make(map[uint32]agentRootScopeEntry)
	}
	if event.Type == "process_exit" || event.Type == "exit" {
		// No run token means we cannot identify the root after PID reuse.
		// Runs with stable IDs may continue to emit child events after exit.
		if run == "" {
			delete(s.items, event.Pid)
		}
		return
	}
	if old, ok := s.items[event.Pid]; ok && old.RunID != "" && run == "" {
		// Do not downgrade an established run identity on an event which
		// lacks the corresponding ID.
		return
	}
	if len(s.items) >= agentRootScopeMaxEntries {
		for pid, item := range s.items {
			if now.Sub(item.Observed) >= agentRootScopeTTL {
				delete(s.items, pid)
			}
		}
		if len(s.items) >= agentRootScopeMaxEntries {
			// Admission must fail conservatively rather than use a
			// silently unbounded cache.
			return
		}
	}
	s.items[event.Pid] = agentRootScopeEntry{Comm: comm, RunID: run, Observed: now}
}

// OwnerComm never guesses from the command of a shell/VM child. Unknown and
// mismatched roots use ordinary process/tag filtering only.
func (s *agentRootScopeStore) OwnerComm(event *pb.Event) string {
	if s == nil || event == nil || event.RootAgentPid == 0 ||
		event.RootAgentPid == event.Pid {
		return ""
	}
	s.mu.RLock()
	entry, ok := s.items[event.RootAgentPid]
	s.mu.RUnlock()
	if !ok || time.Since(entry.Observed) > agentRootScopeTTL {
		return ""
	}
	run := strings.TrimSpace(event.AgentRunId)
	if entry.RunID != run {
		// A missing run token is also insufficient to connect an event
		// to a specific named run after PID recycling.
		return ""
	}
	return entry.Comm
}

// This app adapter supplies only the verified root identity from the bounded
// ancestry cache; the policy matcher itself has no dependency on protobuf.
func agentScopeAllowsWithRoot(list agentScopeList, event *pb.Event, owner string) bool {
	if event == nil {
		return false
	}
	return agentscope.AllowsWithOwner(list, event.Comm, event.Tag, owner)
}
