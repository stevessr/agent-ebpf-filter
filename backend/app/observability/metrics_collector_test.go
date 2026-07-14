package observability

import (
	"errors"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestCollectorPipelineMetricsSnapshot(t *testing.T) {
	oldStore := collectorMetricsStore
	oldDeps := deps
	broadcast := make(chan *pb.Event, 4)
	collectorMetricsStore = newCollectorMetricsState()
	deps = Deps{
		Broadcast:             broadcast,
		LegacyWSClientCount:   func() int { return 2 },
		EnvelopeWSClientCount: func() int { return 3 },
		PersistQueueStatus: func() PersistQueueStatus {
			return PersistQueueStatus{Active: true, QueueLen: 7, QueueCap: 64, Pending: 9, EnqueuedTotal: 20, PersistedTotal: 10, FailedTotal: 1, DroppedTotal: 2, LastFlushedAt: "2026-07-14T12:00:00Z", LastError: "queue full"}
		},
	}
	t.Cleanup(func() {
		collectorMetricsStore = oldStore
		deps = oldDeps
	})

	RecordCapturedArchive()
	RecordCapturedPersist(nil, 2*time.Millisecond)
	RecordCapturedPersist(errors.New("disk full"), 3*time.Millisecond)
	RecordBroadcastEnqueue(true, "")
	RecordBroadcastEnqueue(false, "kernel_event_reader:queue_full")
	RecordBroadcastReceived()
	RecordBroadcastFlush(2, 3, 1, 4, 5*time.Millisecond)

	health := GetCollectorHealthSnapshot()
	if health.CapturedArchivedTotal != 1 || health.CapturedPersistedTotal != 1 || health.CapturedPersistErrorsTotal != 1 {
		t.Fatalf("captured pipeline counters mismatch: %+v", health)
	}
	if health.BroadcastReceivedTotal != 1 || health.BroadcastFlushesTotal != 1 || health.BroadcastEventsFlushedTotal != 2 || health.BroadcastEnvelopesFlushedTotal != 3 {
		t.Fatalf("broadcast counters mismatch: %+v", health)
	}
	if health.BroadcastQueuedTotal != 1 || health.BroadcastDroppedTotal != 1 || health.BroadcastLastDropReason != "kernel_event_reader:queue_full" {
		t.Fatalf("broadcast enqueue counters mismatch: %+v", health)
	}
	if health.BroadcastMarshalErrorsTotal != 1 || health.BroadcastWriteErrorsTotal != 4 || health.BroadcastLastFlushLatencyNs != uint64((5*time.Millisecond).Nanoseconds()) {
		t.Fatalf("broadcast error/latency counters mismatch: %+v", health)
	}
	if health.PersistAppendLatencyNs != uint64((3 * time.Millisecond).Nanoseconds()) {
		t.Fatalf("persist latency mismatch: %+v", health)
	}
	if !health.PersistWriterActive || health.PersistQueueLen != 7 || health.PersistQueueCap != 64 || health.PersistPending != 9 || health.PersistGenerationEnqueued != 20 || health.PersistGenerationPersisted != 10 || health.PersistGenerationFailed != 1 || health.PersistGenerationDropped != 2 || health.PersistWriterLastFlushedAt != "2026-07-14T12:00:00Z" || health.PersistWriterLastError != "queue full" {
		t.Fatalf("persist queue status mismatch: %+v", health)
	}
	if health.WsClients != 5 {
		t.Fatalf("websocket client count = %d, want 5", health.WsClients)
	}
}

func TestCollectorPersistBatchMetrics(t *testing.T) {
	state := newCollectorMetricsState()
	state.RecordCapturedPersistBatch(5, 2, 4*time.Millisecond)
	snapshot := state.Snapshot()
	if snapshot.CapturedPersistedTotal != 5 || snapshot.CapturedPersistErrorsTotal != 2 || snapshot.PersistAppendLatencyNs != uint64((4*time.Millisecond).Nanoseconds()) {
		t.Fatalf("persist batch metrics mismatch: %+v", snapshot)
	}
}
