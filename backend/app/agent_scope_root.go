package app

import (
	"time"

	"agent-ebpf-filter/internal/agentidentity"
	"agent-ebpf-filter/internal/agentscope"
	"agent-ebpf-filter/pb"
)

// The app owns protobuf adaptation; root identity caching is independent of
// both protobuf and transport. This component is for capture/monitor admission,
// not an authoritative historical process tree.
type agentRootScopeStore struct {
	cache *agentidentity.Store
}

const (
	agentRootScopeMaxEntries = agentidentity.MaxRoots
	agentRootScopeTTL        = agentidentity.RootTTL
)

func newAgentRootScopeStore() *agentRootScopeStore {
	return &agentRootScopeStore{cache: agentidentity.NewStore()}
}

func newAgentRootScopeStoreWithClock(clock func() time.Time) *agentRootScopeStore {
	return &agentRootScopeStore{cache: agentidentity.NewStoreWithClock(clock)}
}

var agentRootScopes = newAgentRootScopeStore()

func agentRootEvidence(event *pb.Event) agentidentity.Evidence {
	return agentidentity.Evidence{
		PID:       event.GetPid(),
		RootPID:   event.GetRootAgentPid(),
		Comm:      event.GetComm(),
		RunID:     event.GetAgentRunId(),
		EventType: event.GetType(),
	}
}

// Observe runs after event context enrichment but before capture filtering;
// only observed root events may establish the identity.
func (s *agentRootScopeStore) Observe(event *pb.Event) {
	if s == nil || s.cache == nil || event == nil {
		return
	}
	s.cache.Observe(agentRootEvidence(event))
}

func (s *agentRootScopeStore) OwnerComm(event *pb.Event) string {
	if s == nil || s.cache == nil || event == nil {
		return ""
	}
	return s.cache.OwnerComm(agentRootEvidence(event))
}

// A verified root name participates in both allow and block admission lists.
func agentScopeAllowsWithRoot(list agentScopeList, event *pb.Event, owner string) bool {
	if event == nil {
		return false
	}
	return agentscope.AllowsWithOwner(list, event.Comm, event.Tag, owner)
}
