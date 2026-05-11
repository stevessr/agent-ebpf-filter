package main

import (
	"sort"
	"sync"
)

type TLSCaptureStore struct {
	mu        sync.RWMutex
	events    []TLSPlaintextEvent
	max       int
	libraries map[string]TLSLibraryStatus
}

func NewTLSCaptureStore(max int) *TLSCaptureStore {
	if max <= 0 {
		max = 1000
	}
	return &TLSCaptureStore{
		max:       max,
		libraries: make(map[string]TLSLibraryStatus),
	}
}

func (s *TLSCaptureStore) Add(event TLSPlaintextEvent) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events = append(s.events, event)
	if len(s.events) > s.max {
		copy(s.events, s.events[len(s.events)-s.max:])
		s.events = s.events[:s.max]
	}
}

func (s *TLSCaptureStore) Recent(limit int) []TLSPlaintextEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	if limit == 0 {
		return []TLSPlaintextEvent{}
	}
	out := make([]TLSPlaintextEvent, limit)
	copy(out, s.events[len(s.events)-limit:])
	return out
}

func (s *TLSCaptureStore) SetLibraryStatus(status TLSLibraryStatus) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	key := status.Name + "\x00" + status.Path
	if status.Path == "" {
		key = status.Name
	}
	s.libraries[key] = status
}

func (s *TLSCaptureStore) LibraryStatuses() []TLSLibraryStatus {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]TLSLibraryStatus, 0, len(s.libraries))
	for _, status := range s.libraries {
		out = append(out, status)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].Path < out[j].Path
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func (s *TLSCaptureStore) Count() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.events)
}
