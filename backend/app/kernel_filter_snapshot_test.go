package app

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func replaceDisabledFiltersForTest(t testing.TB, comms map[string]struct{}, eventTypes map[uint32]struct{}) {
	t.Helper()

	disabledCommsMu.Lock()
	previousComms := disabledComms
	disabledComms = comms
	disabledCommsMu.Unlock()

	disabledEventTypesMu.Lock()
	previousEventTypes := disabledEventTypes
	disabledEventTypes = eventTypes
	disabledEventTypesMu.Unlock()

	t.Cleanup(func() {
		disabledCommsMu.Lock()
		disabledComms = previousComms
		disabledCommsMu.Unlock()

		disabledEventTypesMu.Lock()
		disabledEventTypes = previousEventTypes
		disabledEventTypesMu.Unlock()
	})
}

func TestDisabledCommSnapshotPublishesOnWriteUnlock(t *testing.T) {
	replaceDisabledFiltersForTest(t, map[string]struct{}{}, map[uint32]struct{}{})

	disabledCommsMu.Lock()
	disabledComms["bash"] = struct{}{}
	disabledCommsMu.Unlock()
	if !lookupDisabledCommSnapshot("bash") {
		t.Fatal("published snapshot does not contain added comm")
	}

	old := disabledCommSnapshotStore.Load()
	disabledCommsMu.Lock()
	disabledComms["curl"] = struct{}{}
	disabledCommsMu.Unlock()
	if _, exists := old.values["curl"]; exists {
		t.Fatal("old comm snapshot was mutated in place")
	}

	disabledCommsMu.Lock()
	delete(disabledComms, "bash")
	disabledCommsMu.Unlock()
	if lookupDisabledCommSnapshot("bash") {
		t.Fatal("removed comm remained disabled")
	}
}

func TestDisabledEventTypeSnapshotMaskBoundaryAndOverflow(t *testing.T) {
	replaceDisabledFiltersForTest(t, map[string]struct{}{}, map[uint32]struct{}{})

	disabledEventTypesMu.Lock()
	disabledEventTypes[63] = struct{}{}
	disabledEventTypes[64] = struct{}{}
	disabledEventTypes[100] = struct{}{}
	disabledEventTypesMu.Unlock()

	for _, eventType := range []uint32{63, 64, 100} {
		if !isEventTypeDisabled(eventType) {
			t.Fatalf("event type %d should be disabled", eventType)
		}
	}
	for _, eventType := range []uint32{62, 65, 101} {
		if isEventTypeDisabled(eventType) {
			t.Fatalf("event type %d unexpectedly disabled", eventType)
		}
	}

	old := disabledEventTypeSnapshotStore.Load()
	disabledEventTypesMu.Lock()
	disabledEventTypes[101] = struct{}{}
	disabledEventTypesMu.Unlock()
	if _, exists := old.overflow[101]; exists {
		t.Fatal("old event-type snapshot was mutated in place")
	}
}

func TestDisabledFilterSnapshotsConcurrentReadersAndWriters(t *testing.T) {
	replaceDisabledFiltersForTest(t,
		map[string]struct{}{"stable": {}},
		map[uint32]struct{}{1: {}},
	)

	var failures atomic.Int64
	stop := make(chan struct{})
	var readers sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if !lookupDisabledCommSnapshot("stable") || !isEventTypeDisabled(1) {
						failures.Add(1)
					}
				}
			}
		}()
	}

	for iteration := 0; iteration < 5000; iteration++ {
		comm := fmt.Sprintf("temp-%d", iteration%64)
		disabledCommsMu.Lock()
		disabledComms[comm] = struct{}{}
		disabledCommsMu.Unlock()
		disabledCommsMu.Lock()
		delete(disabledComms, comm)
		disabledCommsMu.Unlock()

		eventType := uint32(100 + iteration%64)
		disabledEventTypesMu.Lock()
		disabledEventTypes[eventType] = struct{}{}
		disabledEventTypesMu.Unlock()
		disabledEventTypesMu.Lock()
		delete(disabledEventTypes, eventType)
		disabledEventTypesMu.Unlock()
	}
	close(stop)
	readers.Wait()

	if got := failures.Load(); got != 0 {
		t.Fatalf("stable filter entries disappeared %d times", got)
	}
}

func BenchmarkKernelEventTypeFilterSnapshot(b *testing.B) {
	replaceDisabledFiltersForTest(b,
		map[string]struct{}{},
		map[uint32]struct{}{34: {}, 100: {}},
	)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		_ = isEventTypeDisabled(34)
	}
}

func BenchmarkKernelEventTypeFilterLockedBaseline(b *testing.B) {
	replaceDisabledFiltersForTest(b,
		map[string]struct{}{},
		map[uint32]struct{}{34: {}, 100: {}},
	)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		disabledEventTypesMu.RLock()
		_, _ = disabledEventTypes[34]
		disabledEventTypesMu.RUnlock()
	}
}
