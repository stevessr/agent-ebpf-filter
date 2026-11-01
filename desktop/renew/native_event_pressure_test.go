package main

import (
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
)

func TestRenewEventUIQueueConcurrentProducersAccountForEveryInput(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	const producers, each = 8, 3000
	var wg sync.WaitGroup
	for producer := 0; producer < producers; producer++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := 0; n < each; n++ {
				a.queueEventSummary(eventSummary{
					EventID: strconv.Itoa(id) + "-" + strconv.Itoa(n),
					PID: id*each + n,
				})
			}
		}(producer)
	}
	wg.Wait()
	wantTotal := uint64(producers * each)
	if got := a.eventUIQueueDropped.Load(); got != wantTotal-uint64(eventUIQueueSize) {
		t.Fatalf("queue overflow count=%d, want %d", got, wantTotal-uint64(eventUIQueueSize))
	}
	if total := a.eventUIDroppedCount(); total != a.eventUIQueueDropped.Load() {
		t.Fatalf("total drops %d differ from queue-only drops %d", total, a.eventUIQueueDropped.Load())
	}
	if len(a.eventUIQueue) != eventUIQueueSize {
		t.Fatalf("queue length=%d, want %d", len(a.eventUIQueue), eventUIQueueSize)
	}
	seen := make(map[string]struct{}, eventUIQueueSize)
	for len(a.eventUIQueue) > 0 {
		e := <-a.eventUIQueue
		if _, duplicate := seen[e.EventID]; duplicate {
			t.Fatalf("duplicate retained event %q", e.EventID)
		}
		seen[e.EventID] = struct{}{}
	}
	if uint64(len(seen))+a.eventUIQueueDropped.Load() != wantTotal {
		t.Fatalf("record conservation failed: retained=%d dropped=%d input=%d",
			len(seen), a.eventUIQueueDropped.Load(), wantTotal)
	}
}

func TestRenewEventUIQueueConcurrentReaderConservesRecords(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	const producers, each = 6, 1500
	done := make(chan struct{})
	readerStopped := make(chan struct{})
	var received atomic.Int64
	go func() {
		defer close(readerStopped)
		for {
			select {
			case <-done:
				return
			case <-a.eventUIQueue:
				received.Add(1)
			}
		}
	}()
	var wg sync.WaitGroup
	for id := 0; id < producers; id++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for n := 0; n < each; n++ {
				a.queueEventSummary(eventSummary{
					EventID: strconv.Itoa(id) + ":" + strconv.Itoa(n),
				})
			}
		}(id)
	}
	wg.Wait()
	close(done)
	<-readerStopped
	retained := len(a.eventUIQueue)
	accepted := uint64(received.Load()) + uint64(retained)
	dropped := a.eventUIQueueDropped.Load()
	if accepted+dropped != uint64(producers*each) {
		t.Fatalf("concurrent reader conservation: received=%d buffered=%d dropped=%d input=%d",
			received.Load(), retained, dropped, producers*each)
	}
	if a.eventUIDroppedCount() != dropped {
		t.Fatalf("double-counted or missed dropped queue summaries: total=%d queue=%d",
			a.eventUIDroppedCount(), dropped)
	}
}

func TestRenewEventUIBatchPressureAndDropReasons(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	batch := make([]eventSummary, 0, eventUIFrameCapacity)
	overwriteAt := 0
	for i := 0; i < eventUIFrameCapacity+100; i++ {
		batch = a.appendEventToUIBatch(batch, eventSummary{
			EventID: strconv.Itoa(i),
			ReceivedAtMS: int64(i),
		}, eventUIFrameCapacity, &overwriteAt)
	}
	if a.eventUIBatchDropped.Load() != 100 || a.eventUIDroppedCount() != 100 {
		t.Fatalf("batch drop accounting: batch=%d total=%d",
			a.eventUIBatchDropped.Load(), a.eventUIDroppedCount())
	}
	if a.eventUIBatchBuffered.Load() != eventUIFrameCapacity ||
		a.eventUIBatchPeak.Load() != eventUIFrameCapacity {
		t.Fatalf("pressure gauges: buffered=%d peak=%d",
			a.eventUIBatchBuffered.Load(), a.eventUIBatchPeak.Load())
	}
	rotateEventBatchIntoArrivalOrder(batch, overwriteAt)
	for i, e := range batch {
		want := strconv.Itoa(i + 100)
		if e.EventID != want {
			t.Fatalf("retained batch[%d]=%s, want %s", i, e.EventID, want)
		}
	}
	a.updateEventUIBatchPressure(0)
	if a.eventUIBatchBuffered.Load() != 0 ||
		a.eventUIBatchPeak.Load() != eventUIFrameCapacity {
		t.Fatal("resetting buffered events erased high-water observation")
	}
	for i := 0; i < eventUIQueueSize+3; i++ {
		a.queueEventSummary(eventSummary{EventID: strconv.Itoa(i)})
	}
	if a.eventUIQueueDropped.Load() != 3 || a.eventUIDroppedCount() != 103 {
		t.Fatalf("queue and batch drop reasons don't add up: queue=%d batch=%d total=%d",
			a.eventUIQueueDropped.Load(), a.eventUIBatchDropped.Load(), a.eventUIDroppedCount())
	}
}

func TestRenewEventUIBatchPeakConcurrentCompareAndSwap(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	var wg sync.WaitGroup
	for n := int64(1); n <= 1500; n++ {
		wg.Add(1)
		go func(n int64) {
			defer wg.Done()
			a.updateEventUIBatchPressure(int(n))
		}(n)
	}
	wg.Wait()
	if peak := a.eventUIBatchPeak.Load(); peak != 1500 {
		t.Fatalf("high-water counter lost an update, got %d", peak)
	}
}

func BenchmarkRenewEventUIQueueOverflow(b *testing.B) {
	a := newRenewApp("http://127.0.0.1:8080")
	for i := 0; i < eventUIQueueSize; i++ {
		a.queueEventSummary(eventSummary{EventID: "warm"})
	}
	e := eventSummary{EventID: "latest", PID: 1234}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.queueEventSummary(e)
	}
	b.StopTimer()
	if a.eventUIQueueDropped.Load() != uint64(b.N) {
		b.Fatalf("lost drop accounting: %d, want %d", a.eventUIQueueDropped.Load(), b.N)
	}
}

func BenchmarkRenewEventUIBatchOverflow(b *testing.B) {
	a := newRenewApp("http://127.0.0.1:8080")
	batch := make([]eventSummary, 0, eventUIFrameCapacity)
	at := 0
	for i := 0; i < eventUIFrameCapacity; i++ {
		batch = a.appendEventToUIBatch(batch, eventSummary{EventID: "warm"}, eventUIFrameCapacity, &at)
	}
	e := eventSummary{EventID: "latest", PID: 1234}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batch = a.appendEventToUIBatch(batch, e, eventUIFrameCapacity, &at)
	}
	b.StopTimer()
	if a.eventUIBatchDropped.Load() != uint64(b.N) {
		b.Fatalf("lost batch accounting: %d, want %d", a.eventUIBatchDropped.Load(), b.N)
	}
}
