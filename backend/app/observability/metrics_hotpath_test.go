package observability

import (
	"runtime"
	"sync"
	"testing"
)

func resetHotPathMetricsForTest() {
	hotPathMetrics.broadcastQueued.Store(0)
	hotPathMetrics.broadcastReceived.Store(0)
	hotPathMetrics.capturedArchived.Store(0)
	hotPathMetrics.ringbufZeroCopy.Store(0)
	hotPathMetrics.ringbufCopy.Store(0)
}

func TestFlushAppHotPathMetrics(t *testing.T) {
	oldStore := collectorMetricsStore
	collectorMetricsStore = newCollectorMetricsState()
	resetHotPathMetricsForTest()
	t.Cleanup(func() {
		resetHotPathMetricsForTest()
		collectorMetricsStore = oldStore
	})

	RecordHotBroadcastEnqueue(true, "")
	RecordHotBroadcastEnqueue(true, "")
	RecordHotBroadcastReceived()
	RecordHotCapturedArchive()
	RecordHotRingbufDecode(true)
	RecordHotRingbufDecode(false)
	FlushAppHotPathMetrics()

	snapshot := collectorMetricsStore.rawSnapshot()
	if snapshot.BroadcastQueuedTotal != 2 {
		t.Fatalf("broadcast queued = %d, want 2", snapshot.BroadcastQueuedTotal)
	}
	if snapshot.BroadcastReceivedTotal != 1 {
		t.Fatalf("broadcast received = %d, want 1", snapshot.BroadcastReceivedTotal)
	}
	if snapshot.CapturedArchivedTotal != 1 {
		t.Fatalf("captured archived = %d, want 1", snapshot.CapturedArchivedTotal)
	}
	if snapshot.RingbufZeroCopyDecodeTotal != 1 || snapshot.RingbufCopyDecodeTotal != 1 {
		t.Fatalf("ringbuf decode counters = %d/%d, want 1/1", snapshot.RingbufZeroCopyDecodeTotal, snapshot.RingbufCopyDecodeTotal)
	}

	FlushAppHotPathMetrics()
	second := collectorMetricsStore.rawSnapshot()
	if second.BroadcastQueuedTotal != snapshot.BroadcastQueuedTotal || second.RingbufZeroCopyDecodeTotal != snapshot.RingbufZeroCopyDecodeTotal {
		t.Fatalf("empty second flush changed counters: before=%+v after=%+v", snapshot, second)
	}
}

func TestFlushAppHotPathMetricsConcurrentProducers(t *testing.T) {
	oldStore := collectorMetricsStore
	collectorMetricsStore = newCollectorMetricsState()
	resetHotPathMetricsForTest()
	t.Cleanup(func() {
		resetHotPathMetricsForTest()
		collectorMetricsStore = oldStore
	})

	const producers = 8
	const perProducer = 10000
	var producerWG sync.WaitGroup
	producerWG.Add(producers)
	stopFlusher := make(chan struct{})
	flusherDone := make(chan struct{})
	go func() {
		defer close(flusherDone)
		for {
			select {
			case <-stopFlusher:
				return
			default:
				FlushAppHotPathMetrics()
				runtime.Gosched()
			}
		}
	}()

	for producer := 0; producer < producers; producer++ {
		go func() {
			defer producerWG.Done()
			for iteration := 0; iteration < perProducer; iteration++ {
				RecordHotBroadcastEnqueue(true, "")
				RecordHotBroadcastReceived()
				RecordHotCapturedArchive()
				RecordHotRingbufDecode(true)
			}
		}()
	}
	producerWG.Wait()
	close(stopFlusher)
	<-flusherDone
	FlushAppHotPathMetrics()

	want := uint64(producers * perProducer)
	snapshot := collectorMetricsStore.rawSnapshot()
	if snapshot.BroadcastQueuedTotal != want || snapshot.BroadcastReceivedTotal != want || snapshot.CapturedArchivedTotal != want || snapshot.RingbufZeroCopyDecodeTotal != want {
		t.Fatalf("concurrent flush lost counters: queued=%d received=%d archived=%d zeroCopy=%d want=%d",
			snapshot.BroadcastQueuedTotal,
			snapshot.BroadcastReceivedTotal,
			snapshot.CapturedArchivedTotal,
			snapshot.RingbufZeroCopyDecodeTotal,
			want,
		)
	}
}

func TestRecordHotBroadcastDropKeepsImmediateReason(t *testing.T) {
	oldStore := collectorMetricsStore
	collectorMetricsStore = newCollectorMetricsState()
	resetHotPathMetricsForTest()
	t.Cleanup(func() {
		resetHotPathMetricsForTest()
		collectorMetricsStore = oldStore
	})

	RecordHotBroadcastEnqueue(false, "kernel_event_reader:queue_full")
	snapshot := collectorMetricsStore.rawSnapshot()
	if snapshot.BroadcastDroppedTotal != 1 || snapshot.BroadcastLastDropReason != "kernel_event_reader:queue_full" {
		t.Fatalf("drop metrics = %d/%q", snapshot.BroadcastDroppedTotal, snapshot.BroadcastLastDropReason)
	}
}

func BenchmarkCollectorHotPathMetrics(b *testing.B) {
	oldStore := collectorMetricsStore
	collectorMetricsStore = newCollectorMetricsState()
	resetHotPathMetricsForTest()
	b.Cleanup(func() {
		resetHotPathMetricsForTest()
		collectorMetricsStore = oldStore
	})

	b.Run("atomic_ingress_pair", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			RecordHotRingbufDecode(true)
			RecordHotBroadcastEnqueue(true, "")
		}
	})

	b.Run("locked_ingress_pair_baseline", func(b *testing.B) {
		b.ReportAllocs()
		for iteration := 0; iteration < b.N; iteration++ {
			RecordRingbufDecode(true)
			RecordBroadcastEnqueue(true, "")
		}
	})
}
