package app

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestResearchProcessingWorkerShutdownTimeoutKeepsGeneration(t *testing.T) {
	worker := newResearchProcessingWorker()
	oldDone := make(chan struct{})
	worker.mu.Lock()
	worker.started = true
	worker.queue = make(chan researchProcessingWorkItem, 1)
	worker.cancel = func() {}
	worker.done = oldDone
	worker.mu.Unlock()

	shutdownCtx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := worker.Shutdown(shutdownCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() error = %v, want context cancellation", err)
	}
	worker.mu.RLock()
	started, queue, done := worker.started, worker.queue, worker.done
	worker.mu.RUnlock()
	if !started || queue != nil || done != oldDone {
		t.Fatalf("timed-out shutdown state = started:%v queue:%v done:%p, want active generation with nil queue and done %p", started, queue, done, oldDone)
	}

	worker.Start(context.Background(), 4)
	worker.mu.RLock()
	started, queue, done = worker.started, worker.queue, worker.done
	worker.mu.RUnlock()
	if !started || queue != nil || done != oldDone {
		t.Fatalf("Start() replaced a stopping generation: started:%v queue:%v done:%p", started, queue, done)
	}
}

func TestResearchEventRingPreservesOrderAcrossWrapAndResize(t *testing.T) {
	var ring researchEventRing
	samples := func(ids ...string) []researchEventSample {
		out := make([]researchEventSample, len(ids))
		for index, id := range ids {
			out[index] = researchEventSample{ID: id}
		}
		return out
	}
	ids := func(items []researchEventSample) []string {
		out := make([]string, len(items))
		for index := range items {
			out[index] = items[index].ID
		}
		return out
	}
	wantIDs := func(want ...string) {
		t.Helper()
		got := ids(ring.Snapshot())
		if len(got) != len(want) {
			t.Fatalf("ring IDs = %v, want %v", got, want)
		}
		for index := range want {
			if got[index] != want[index] {
				t.Fatalf("ring IDs = %v, want %v", got, want)
			}
		}
	}

	if evicted := ring.Append(samples("1", "2", "3"), 3); evicted != 0 {
		t.Fatalf("initial append evicted %d samples", evicted)
	}
	wantIDs("1", "2", "3")
	if evicted := ring.Append(samples("4"), 3); evicted != 1 {
		t.Fatalf("wrapped append evicted %d samples, want 1", evicted)
	}
	wantIDs("2", "3", "4")
	if evicted := ring.Append(samples("5", "6"), 3); evicted != 2 {
		t.Fatalf("batch append evicted %d samples, want 2", evicted)
	}
	wantIDs("4", "5", "6")
	if evicted := ring.Append(samples("7"), 2); evicted != 2 {
		t.Fatalf("shrink append evicted %d samples, want 2", evicted)
	}
	wantIDs("6", "7")
	if evicted := ring.Append(samples("8"), 4); evicted != 0 {
		t.Fatalf("grow append evicted %d samples", evicted)
	}
	wantIDs("6", "7", "8")
	if cap(ring.items) > ring.limit {
		t.Fatalf("ring capacity = %d, limit %d", cap(ring.items), ring.limit)
	}

	ring.Reset()
	wantIDs()
	if ring.start != 0 || ring.limit != 4 {
		t.Fatalf("reset ring state = start:%d limit:%d", ring.start, ring.limit)
	}
	if evicted := ring.Append(samples("9", "10", "11", "12", "13"), 3); evicted != 2 {
		t.Fatalf("large batch evicted %d samples, want 2", evicted)
	}
	wantIDs("11", "12", "13")
}

func TestResearchProcessingRingSummaryKeepsNewestEventsInOrder(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{
		Enabled:               true,
		MaxEvents:             100,
		TimelineBucketSeconds: 60,
		TopK:                  10,
		RecentSamples:         5,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newResearchProcessingWorker()
	base := time.Date(2026, 7, 14, 11, 0, 0, 0, time.UTC)
	records := make([]CapturedEventRecord, 150)
	for index := range records {
		records[index] = CapturedEventRecord{
			ReceivedAt: base.Add(time.Duration(index) * time.Second),
			Event:      &pb.Event{Pid: uint32(index + 1), Type: "openat", Comm: "ring", Path: "/tmp/" + strconv.Itoa(index)},
		}
	}
	worker.processRecords(records, false, time.Time{})
	status := worker.Status()
	if status.ConsumedTotal != 150 || status.BufferedTotal != 100 || status.BufferEvictedTotal != 50 || status.Summary.Total != 100 {
		t.Fatalf("ring summary totals = %+v", status)
	}
	if status.Summary.EarliestTimestamp != base.Add(50*time.Second).UnixMilli() || status.Summary.LatestTimestamp != base.Add(149*time.Second).UnixMilli() {
		t.Fatalf("ring summary time range = %d..%d", status.Summary.EarliestTimestamp, status.Summary.LatestTimestamp)
	}
	if len(status.Summary.RecentSamples) != 5 {
		t.Fatalf("recent sample count = %d, want 5", len(status.Summary.RecentSamples))
	}
	for index, sample := range status.Summary.RecentSamples {
		want := base.Add(time.Duration(145+index) * time.Second).UnixMilli()
		if sample.Timestamp != want {
			t.Fatalf("recent sample %d timestamp = %d, want %d", index, sample.Timestamp, want)
		}
	}
}

func TestResearchProcessingCanceledScanDoesNotMutateState(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{
		Enabled:   true,
		MaxEvents: 300,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newResearchProcessingWorker()
	records := make([]CapturedEventRecord, backendWorkerScanBatchSize*2+1)
	for index := range records {
		records[index] = CapturedEventRecord{
			ReceivedAt: time.Unix(1700000000+int64(index), 0),
			Event:      &pb.Event{Pid: uint32(index + 1), Type: "openat", Comm: "scan", Path: "/tmp/item"},
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.processRecordsContext(ctx, records, true, time.Now().UTC())
	if status := worker.Status(); status.ConsumedTotal != 0 || status.BufferedTotal != 0 {
		t.Fatalf("canceled scan mutated state: %+v", status)
	}

	worker.processRecords(records, true, time.Time{})
	status := worker.Status()
	if status.ConsumedTotal != uint64(len(records)) || status.BufferedTotal != 300 {
		t.Fatalf("batched scan totals = consumed:%d buffered:%d, want %d/300", status.ConsumedTotal, status.BufferedTotal, len(records))
	}
}

func TestResearchProcessingWorkerShutdownAndRestart(t *testing.T) {
	worker := newResearchProcessingWorker()
	ctx, cancel := context.WithCancel(context.Background())
	worker.Start(ctx, 8)
	cancel()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := worker.Shutdown(waitCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	worker.mu.RLock()
	started, queue := worker.started, worker.queue
	worker.mu.RUnlock()
	if started || queue != nil {
		t.Fatalf("worker after shutdown = started:%v queue:%v", started, queue)
	}
	if worker.EnqueueReset() {
		t.Fatal("stopped worker accepted new work")
	}

	restartCtx, restartCancel := context.WithCancel(context.Background())
	worker.Start(restartCtx, 4)
	restartCancel()
	if err := worker.Shutdown(waitCtx); err != nil {
		t.Fatalf("shutdown after restart error = %v", err)
	}
}

func TestResearchProcessingWorkerBuildsFrontendLikeSummary(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{
		Enabled:               true,
		MaxEvents:             100,
		QueueSize:             128,
		TimelineBucketSeconds: 60,
		TopK:                  10,
		RecentSamples:         5,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newResearchProcessingWorker()
	base := time.Date(2026, 7, 3, 9, 0, 0, 0, time.UTC)
	records := []CapturedEventRecord{
		{ReceivedAt: base, Event: &pb.Event{Pid: 100, Ppid: 1, Type: "execve", EventType: pb.EventType_EXECVE, Comm: "agent", TraceId: "trace-a", Path: "/usr/bin/agent"}},
		{ReceivedAt: base.Add(10 * time.Second), Event: &pb.Event{Pid: 101, Ppid: 100, Type: "openat", EventType: pb.EventType_OPENAT, Comm: "agent-child", TraceId: "trace-a", Path: "/tmp/a.txt"}},
		{ReceivedAt: base.Add(70 * time.Second), Event: &pb.Event{Pid: 101, Ppid: 100, Type: "network_connect", EventType: pb.EventType_NETWORK_CONNECT, Comm: "agent-child", TraceId: "trace-a", NetEndpoint: "127.0.0.1:8080"}},
	}
	for _, record := range records {
		worker.processRecord(record, false)
	}

	status := worker.Status()
	if status.ConsumedTotal != 3 || status.BufferedTotal != 3 || status.Summary.Total != 3 {
		t.Fatalf("summary totals mismatch: %+v", status)
	}
	if len(status.Summary.Timeline) != 2 {
		t.Fatalf("timeline buckets = %d, want 2: %+v", len(status.Summary.Timeline), status.Summary.Timeline)
	}
	if len(status.Summary.TopProcesses) == 0 || status.Summary.TopProcesses[0].PID != 101 || status.Summary.TopProcesses[0].EventCount != 2 {
		t.Fatalf("top process summary mismatch: %+v", status.Summary.TopProcesses)
	}
	if len(status.Summary.TopTraces) != 1 || status.Summary.TopTraces[0].TraceID != "trace-a" || status.Summary.TopTraces[0].EventCount != 3 {
		t.Fatalf("trace summary mismatch: %+v", status.Summary.TopTraces)
	}
	if len(status.Summary.RecentSamples) != 3 || status.Summary.RecentSamples[2].Target != "127.0.0.1:8080" {
		t.Fatalf("recent samples mismatch: %+v", status.Summary.RecentSamples)
	}
}

func TestResearchProcessingManualScanWorksWhenDisabled(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{
		Enabled:               false,
		MaxEvents:             100,
		QueueSize:             128,
		TimelineBucketSeconds: 60,
		TopK:                  10,
		RecentSamples:         5,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newResearchProcessingWorker()
	record := CapturedEventRecord{ReceivedAt: time.Now().UTC(), Event: &pb.Event{Pid: 200, Type: "openat", Comm: "research", Path: "/tmp/research.txt"}}
	worker.processRecord(record, false)
	if status := worker.Status(); status.ConsumedTotal != 0 || status.Summary.Total != 0 {
		t.Fatalf("disabled streaming should not process records: %+v", status)
	}
	worker.processRecord(record, true)
	if status := worker.Status(); status.ConsumedTotal != 1 || status.Summary.Total != 1 || len(status.Summary.ByComm) != 1 {
		t.Fatalf("forced manual scan should process records: %+v", status)
	}
}

func TestResearchProcessingWorkerRebuildsSummaryLazily(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{
		Enabled:               true,
		MaxEvents:             100,
		QueueSize:             128,
		TimelineBucketSeconds: 60,
		TopK:                  10,
		RecentSamples:         5,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newResearchProcessingWorker()
	base := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		worker.processRecord(CapturedEventRecord{
			ReceivedAt: base.Add(time.Duration(i) * time.Second),
			Event:      &pb.Event{Pid: uint32(300 + i), Type: "openat", Comm: "research", Path: "/tmp/research.txt"},
		}, false)
	}
	if !worker.summaryDirty || worker.summaryRebuilds != 0 {
		t.Fatalf("summary should be dirty and unrebuilt before status: dirty=%v rebuilds=%d", worker.summaryDirty, worker.summaryRebuilds)
	}

	status := worker.Status()
	if status.Summary.Total != 10 || status.SummaryRebuilds != 1 || status.LastSummaryAt == nil || status.LastProcessedAt == nil {
		t.Fatalf("lazy summary status mismatch: %+v", status)
	}
	if status.PendingSummary {
		t.Fatalf("status should refresh pending summary: %+v", status)
	}
	if status.ThroughputPerSecond <= 0 {
		t.Fatalf("expected throughput metric: %+v", status)
	}

	again := worker.Status()
	if again.SummaryRebuilds != status.SummaryRebuilds {
		t.Fatalf("clean status should not rebuild summary: first=%d again=%d", status.SummaryRebuilds, again.SummaryRebuilds)
	}
}

func TestResearchProcessingWorkerDropReasons(t *testing.T) {
	record := CapturedEventRecord{ReceivedAt: time.Now().UTC(), Event: &pb.Event{Pid: 400, Type: "openat", Comm: "research", Path: "/tmp/research.txt"}}
	worker := newResearchProcessingWorker()
	if worker.EnqueueEvent(record) {
		t.Fatal("enqueue should fail before worker starts")
	}
	status := worker.Status()
	if status.LastDropReason != "worker_not_started" || status.DroppedTotal != 1 {
		t.Fatalf("worker_not_started drop mismatch: %+v", status)
	}

	worker.queue = make(chan researchProcessingWorkItem, 1)
	worker.started = true
	if !worker.EnqueueEvent(record) {
		t.Fatal("first enqueue should fill the manual queue")
	}
	if worker.EnqueueEvent(record) {
		t.Fatal("second enqueue should fail on full queue")
	}
	status = worker.Status()
	if status.LastDropReason != "queue_full" || status.DroppedTotal != 2 || status.EnqueuedTotal != 1 || status.LastEnqueuedAt == nil || status.LastDroppedAt == nil {
		t.Fatalf("queue_full drop mismatch: %+v", status)
	}

	oldStore := runtimeSettingsStore
	oldWorker := researchProcessingWorkerStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{ResearchProcessing: ResearchProcessingSettings{
		Enabled:               false,
		MaxEvents:             100,
		QueueSize:             128,
		TimelineBucketSeconds: 60,
		TopK:                  10,
		RecentSamples:         5,
	}}}
	researchProcessingWorkerStore = newResearchProcessingWorker()
	t.Cleanup(func() {
		runtimeSettingsStore = oldStore
		researchProcessingWorkerStore = oldWorker
	})
	queueResearchProcessingRecord(record)
	status = researchProcessingWorkerStore.Status()
	if status.LastDropReason != "disabled" || status.DroppedTotal != 1 {
		t.Fatalf("disabled drop mismatch: %+v", status)
	}
}

func BenchmarkResearchEventRingSteadyStateAppend(b *testing.B) {
	for _, limit := range []int{100, researchProcessingDefaultMaxEvents} {
		b.Run(strconv.Itoa(limit), func(b *testing.B) {
			var ring researchEventRing
			warm := make([]researchEventSample, limit)
			for index := range warm {
				warm[index].Timestamp = int64(index)
			}
			ring.Append(warm, limit)
			sample := []researchEventSample{{Timestamp: int64(limit)}}

			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				sample[0].Timestamp = int64(iteration)
				ring.Append(sample, limit)
			}
		})
	}
}
