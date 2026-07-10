package app

import (
	"context"
	"testing"
	"time"
)

func TestRunFlowAggregatorGCStopsWithContext(t *testing.T) {
	aggregator := newFlowAggregator()
	aggregator.RecordConnection("10.0.0.2", "93.184.216.34", 41000, 443, "TCP", "curl", 42, "outgoing", "ESTABLISHED")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runFlowAggregatorGC(ctx, aggregator, time.Millisecond, -time.Nanosecond)
		close(done)
	}()

	deadline := time.Now().Add(time.Second)
	for len(aggregator.Snapshot()) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("flow aggregator GC did not evict stale state")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("flow aggregator GC did not stop after context cancellation")
	}
}
