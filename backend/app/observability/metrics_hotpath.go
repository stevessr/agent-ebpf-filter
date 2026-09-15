package observability

import "sync/atomic"

// appHotPathMetricCounters holds monotonically increasing counters updated by
// the event data plane. Keeping them outside collectorMetricsState avoids
// taking its broad mutex for every ring-buffer sample and successful queue
// handoff. They are folded into the canonical metrics state before snapshots
// and exports so the public metrics schema stays unchanged.
type appHotPathMetricCounters struct {
	broadcastQueued   atomic.Uint64
	broadcastReceived atomic.Uint64
	capturedArchived  atomic.Uint64
	ringbufZeroCopy   atomic.Uint64
	ringbufCopy       atomic.Uint64
}

var hotPathMetrics appHotPathMetricCounters

func RecordHotBroadcastEnqueue(accepted bool, reason string) {
	if !accepted {
		// Drops are exceptional and carry a last-reason string, so keep the
		// existing locked path for exact diagnostic semantics.
		RecordBroadcastEnqueue(false, reason)
		return
	}
	hotPathMetrics.broadcastQueued.Add(1)
}

func RecordHotBroadcastReceived() {
	hotPathMetrics.broadcastReceived.Add(1)
}

func RecordHotCapturedArchive() {
	hotPathMetrics.capturedArchived.Add(1)
}

func RecordHotRingbufDecode(zeroCopy bool) {
	if zeroCopy {
		hotPathMetrics.ringbufZeroCopy.Add(1)
		return
	}
	hotPathMetrics.ringbufCopy.Add(1)
}

// FlushAppHotPathMetrics atomically drains pending data-plane counters into the
// canonical metrics state. Concurrent producers are safe: increments racing a
// Swap remain pending for the next flush rather than being lost.
func FlushAppHotPathMetrics() {
	broadcastQueued := hotPathMetrics.broadcastQueued.Swap(0)
	broadcastReceived := hotPathMetrics.broadcastReceived.Swap(0)
	capturedArchived := hotPathMetrics.capturedArchived.Swap(0)
	ringbufZeroCopy := hotPathMetrics.ringbufZeroCopy.Swap(0)
	ringbufCopy := hotPathMetrics.ringbufCopy.Swap(0)

	if broadcastQueued == 0 && broadcastReceived == 0 && capturedArchived == 0 && ringbufZeroCopy == 0 && ringbufCopy == 0 {
		return
	}

	collectorMetricsStore.mu.Lock()
	collectorMetricsStore.broadcastQueuedTotal += broadcastQueued
	collectorMetricsStore.broadcastReceivedTotal += broadcastReceived
	collectorMetricsStore.capturedArchivedTotal += capturedArchived
	collectorMetricsStore.ringbufZeroCopyDecodeTotal += ringbufZeroCopy
	collectorMetricsStore.ringbufCopyDecodeTotal += ringbufCopy
	collectorMetricsStore.mu.Unlock()
}
