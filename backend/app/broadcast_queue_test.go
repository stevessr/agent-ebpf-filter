package app

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestEnqueueBroadcastEventMetrics(t *testing.T) {
	queue := make(chan *pb.Event, 1)
	before := collectorMetricsStore.Snapshot()

	if !enqueueBroadcastEvent(queue, &pb.Event{Type: "unit"}, "unit") {
		t.Fatal("first enqueue should be accepted")
	}
	if enqueueBroadcastEvent(queue, &pb.Event{Type: "unit"}, "unit") {
		t.Fatal("second enqueue should be dropped on full queue")
	}

	after := collectorMetricsStore.Snapshot()
	if after.BroadcastQueuedTotal != before.BroadcastQueuedTotal+1 {
		t.Fatalf("queued counter mismatch before=%+v after=%+v", before, after)
	}
	if after.BroadcastDroppedTotal != before.BroadcastDroppedTotal+1 || after.BroadcastLastDropReason != "unit:queue_full" {
		t.Fatalf("drop counter mismatch before=%+v after=%+v", before, after)
	}
}
