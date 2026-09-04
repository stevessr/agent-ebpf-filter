package app

import (
	"sync"
	"sync/atomic"
)

const disabledEventTypeFastBits = 64

// filterPublishingRWMutex preserves the existing sync.RWMutex-shaped control
// plane API while publishing a fresh immutable read snapshot before a writer
// releases the lock. Event ingestion never needs to take the read lock.
type filterPublishingRWMutex struct {
	sync.RWMutex
	publish func()
}

func (m *filterPublishingRWMutex) Unlock() {
	if m.publish != nil {
		m.publish()
	}
	m.RWMutex.Unlock()
}

type disabledCommSnapshot struct {
	values map[string]struct{}
}

type disabledEventTypeSnapshot struct {
	mask     uint64
	overflow map[uint32]struct{}
}

var (
	disabledCommSnapshotStore      atomic.Pointer[disabledCommSnapshot]
	disabledEventTypeSnapshotStore atomic.Pointer[disabledEventTypeSnapshot]
)

func init() {
	// The canonical maps are empty today, but publishing once during package
	// initialization keeps snapshot state correct if future defaults become
	// non-empty without going through a mutation helper.
	disabledCommsMu.publish = publishDisabledCommSnapshotLocked
	disabledEventTypesMu.publish = publishDisabledEventTypeSnapshotLocked

	disabledCommsMu.Lock()
	disabledCommsMu.Unlock()
	disabledEventTypesMu.Lock()
	disabledEventTypesMu.Unlock()
}

// publishDisabledCommSnapshotLocked is called while disabledCommsMu is write
// locked. The published map is immutable for the rest of its lifetime.
func publishDisabledCommSnapshotLocked() {
	var values map[string]struct{}
	if len(disabledComms) != 0 {
		values = make(map[string]struct{}, len(disabledComms))
		for comm := range disabledComms {
			values[comm] = struct{}{}
		}
	}
	disabledCommSnapshotStore.Store(&disabledCommSnapshot{values: values})
}

func lookupDisabledCommSnapshot(comm string) bool {
	snapshot := disabledCommSnapshotStore.Load()
	if snapshot == nil {
		return false
	}
	_, disabled := snapshot.values[comm]
	return disabled
}

// publishDisabledEventTypeSnapshotLocked builds one coherent snapshot. Event
// types below 64 use a bit mask; larger values retain exact configuration
// semantics through the immutable overflow map.
func publishDisabledEventTypeSnapshotLocked() {
	var mask uint64
	var overflow map[uint32]struct{}
	for eventType := range disabledEventTypes {
		if eventType < disabledEventTypeFastBits {
			mask |= uint64(1) << eventType
			continue
		}
		if overflow == nil {
			overflow = make(map[uint32]struct{})
		}
		overflow[eventType] = struct{}{}
	}
	disabledEventTypeSnapshotStore.Store(&disabledEventTypeSnapshot{
		mask:     mask,
		overflow: overflow,
	})
}

func isEventTypeDisabled(eventType uint32) bool {
	snapshot := disabledEventTypeSnapshotStore.Load()
	if snapshot == nil {
		return false
	}
	if eventType < disabledEventTypeFastBits {
		return snapshot.mask&(uint64(1)<<eventType) != 0
	}
	_, disabled := snapshot.overflow[eventType]
	return disabled
}
