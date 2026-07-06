package app

import (
	"strings"

	"agent-ebpf-filter/pb"
)

func enqueueBroadcastEvent(queue chan<- *pb.Event, event *pb.Event, source string) bool {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "unknown"
	}
	if event == nil {
		collectorMetricsStore.RecordBroadcastEnqueue(false, source+":nil_event")
		return false
	}
	if queue == nil {
		collectorMetricsStore.RecordBroadcastEnqueue(false, source+":queue_unavailable")
		return false
	}
	select {
	case queue <- event:
		collectorMetricsStore.RecordBroadcastEnqueue(true, "")
		return true
	default:
		collectorMetricsStore.RecordBroadcastEnqueue(false, source+":queue_full")
		return false
	}
}
