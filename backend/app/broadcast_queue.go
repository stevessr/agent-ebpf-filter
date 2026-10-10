package app

import (
	"agent-ebpf-filter/internal/eventqueue"
	"agent-ebpf-filter/pb"
)

// enqueueBroadcastEvent hands the event to the broadcaster, which then owns
// the mutable object. The admission engine does not depend on protobuf or
// collector metrics; this adapter preserves all existing reason labels.
func enqueueBroadcastEvent(queue chan<- *pb.Event, event *pb.Event, source string) bool {
	result := eventqueue.Offer(queue, event)
	if result == eventqueue.Accepted {
		collectorMetricsStore.RecordBroadcastEnqueue(true, "")
		return true
	}
	collectorMetricsStore.RecordBroadcastEnqueue(false, eventqueue.DropReason(source, result))
	return false
}
