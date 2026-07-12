package network

import (
	"fmt"
	"testing"
	"time"
)

func TestTCPStateTrackerHardCapEvictsOldest(t *testing.T) {
	tracker := NewTCPStateTracker()
	tracker.maxEntries = 4
	base := time.Now().UTC().Add(-time.Hour)

	for i := 0; i < 4; i++ {
		tracker.RecordConnect("127.0.0.1", "198.51.100.1", uint32(1000+i), 443, 1, "test")
		key := tracker.ConnKey("127.0.0.1", "198.51.100.1", uint32(1000+i), 443)
		tracker.connections[key].LastUpdate = base.Add(time.Duration(i) * time.Minute)
	}
	oldest := tracker.ConnKey("127.0.0.1", "198.51.100.1", 1000, 443)
	tracker.RecordConnect("127.0.0.1", "198.51.100.2", 2000, 443, 2, "test")

	if got := len(tracker.Snapshot()); got != 4 {
		t.Fatalf("expected hard cap of 4 entries, got %d", got)
	}
	if _, exists := tracker.connections[oldest]; exists {
		t.Fatalf("expected oldest entry %q to be evicted", oldest)
	}
}

func TestTCPStateTrackerEvictsStaleNonTerminalConnections(t *testing.T) {
	tracker := NewTCPStateTracker()
	now := time.Now().UTC()

	for i, state := range []TCPState{TCPStateClosed, TCPStateSynSent, TCPStateEstablished} {
		tracker.RecordConnect("127.0.0.1", "203.0.113.1", uint32(3000+i), 443, 1, fmt.Sprintf("test-%d", i))
		key := tracker.ConnKey("127.0.0.1", "203.0.113.1", uint32(3000+i), 443)
		tracker.connections[key].State = state
		tracker.connections[key].LastUpdate = now.Add(-20 * time.Minute)
	}
	recentKey := tracker.ConnKey("127.0.0.1", "203.0.113.2", 4000, 443)
	tracker.RecordConnect("127.0.0.1", "203.0.113.2", 4000, 443, 1, "recent")

	tracker.EvictStale(time.Minute, 10*time.Minute)

	if got := len(tracker.Snapshot()); got != 1 {
		t.Fatalf("expected only recent connection to remain, got %d", got)
	}
	if _, exists := tracker.connections[recentKey]; !exists {
		t.Fatal("recent non-terminal connection was evicted")
	}
}
