// Package agentidentity maintains a bounded, evidence-backed cache of Agent
// root process identity. It is independent of protobuf, HTTP and the runtime.
// The cache is for scope admission only, not audit reconstruction or enforcement.
package agentidentity

import (
	"strings"
	"sync"
	"time"
)

const (
	MaxRoots = 4096
	RootTTL  = 24 * time.Hour
)

// Evidence must come from the trusted event enrichment pipeline; callers must
// not construct the root identity from an unverified child's own claims.
type Evidence struct {
	PID       uint32
	RootPID   uint32
	Comm      string
	RunID     string
	EventType string
}

type rootEntry struct {
	comm     string
	runID    string
	observed time.Time
}

type Store struct {
	mu    sync.RWMutex
	items map[uint32]rootEntry
	clock func() time.Time
}

// NewStore defaults to real time. NewStoreWithClock allows deterministic
// replay and testing without sleeps or exposing mutable internal state.
func NewStore() *Store {
	return NewStoreWithClock(time.Now)
}

func NewStoreWithClock(clock func() time.Time) *Store {
	if clock == nil {
		clock = time.Now
	}
	return &Store{items: make(map[uint32]rootEntry), clock: clock}
}

// Observe records only a direct root event, never a descendant assertion.
// A root exit without run ID removes its entry to mitigate PID recycling.
func (s *Store) Observe(event Evidence) {
	if s == nil || event.PID == 0 || event.RootPID == 0 ||
		event.RootPID != event.PID || strings.TrimSpace(event.Comm) == "" {
		return
	}
	comm := strings.TrimSpace(event.Comm)
	run := strings.TrimSpace(event.RunID)
	now := s.clock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = make(map[uint32]rootEntry)
	}
	if event.EventType == "process_exit" || event.EventType == "exit" {
		if run == "" {
			delete(s.items, event.PID)
		}
		return
	}
	if old, ok := s.items[event.PID]; ok && old.runID != "" && run == "" {
		// Do not drop an established run token on an unversioned event.
		return
	}
	if len(s.items) >= MaxRoots {
		for pid, item := range s.items {
			if now.Sub(item.observed) >= RootTTL {
				delete(s.items, pid)
			}
		}
		if len(s.items) >= MaxRoots {
			// Fail conservatively rather than grow without bounds.
			return
		}
	}
	s.items[event.PID] = rootEntry{comm: comm, runID: run, observed: now}
}

// OwnerComm never invents a root for a generic shell or tool process.
// Both the root PID and run identity must match the observed root entry.
func (s *Store) OwnerComm(event Evidence) string {
	if s == nil || event.RootPID == 0 || event.RootPID == event.PID {
		return ""
	}
	s.mu.RLock()
	entry, found := s.items[event.RootPID]
	s.mu.RUnlock()
	if !found || s.clock().Sub(entry.observed) > RootTTL {
		return ""
	}
	if entry.runID != strings.TrimSpace(event.RunID) {
		return ""
	}
	return entry.comm
}
