package app

import (
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

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
