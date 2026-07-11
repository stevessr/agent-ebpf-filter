package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestLoopDetectionWorkerShutdownAndRestart(t *testing.T) {
	worker := newLoopDetectionWorker()
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

func TestLoopDetectionWorkerFindsRepeatedToolContext(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{LoopDetection: LoopDetectionSettings{
		Enabled:            true,
		WindowSeconds:      60,
		RepeatThreshold:    3,
		MaxContexts:        64,
		QueueSize:          128,
		EmitSemanticAlerts: false,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newLoopDetectionWorker()
	base := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		worker.processRecord(CapturedEventRecord{
			ReceivedAt: base.Add(time.Duration(i) * time.Second),
			Event: &pb.Event{
				Pid:          4242,
				Type:         "http_message",
				Comm:         "codex",
				ToolName:     "llm.call",
				AgentRunId:   "run-1",
				TaskId:       "task-1",
				ToolCallId:   "tool-1",
				TraceId:      "trace-1",
				ExtraInfo:    "prompt_digest=abc123def456 model=qwen",
				RootAgentPid: 4000,
			},
		}, false)
	}

	status := worker.Status()
	if status.FindingsTotal != 1 || len(status.RecentFindings) != 1 {
		t.Fatalf("FindingsTotal=%d len=%d, want one finding", status.FindingsTotal, len(status.RecentFindings))
	}
	finding := status.RecentFindings[0]
	if finding.ContextType != "tool_call" || !strings.Contains(finding.ContextKey, "tool-1") {
		t.Fatalf("finding context = %s %q, want localized tool call", finding.ContextType, finding.ContextKey)
	}
	if finding.Target != "digest:abc123def456" {
		t.Fatalf("finding target = %q, want prompt digest target", finding.Target)
	}
	if finding.RepeatCount != 3 || finding.AgentRunID != "run-1" || finding.TaskID != "task-1" || finding.ToolCallID != "tool-1" {
		t.Fatalf("finding did not preserve repeated context metadata: %+v", finding)
	}
}

func TestLoopDetectionManualScanWorksWhenStreamingDisabled(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{LoopDetection: LoopDetectionSettings{
		Enabled:            false,
		WindowSeconds:      30,
		RepeatThreshold:    2,
		MaxContexts:        16,
		QueueSize:          128,
		EmitSemanticAlerts: false,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newLoopDetectionWorker()
	record := CapturedEventRecord{
		ReceivedAt: time.Now().UTC(),
		Event: &pb.Event{
			Pid:        99,
			Type:       "openat",
			Comm:       "agent",
			Path:       "/tmp/research-loop.txt",
			AgentRunId: "run-disabled",
		},
	}
	worker.processRecord(record, false)
	worker.processRecord(record, false)
	if status := worker.Status(); status.ConsumedTotal != 0 || status.FindingsTotal != 0 {
		t.Fatalf("disabled streaming consumed=%d findings=%d, want no streaming work", status.ConsumedTotal, status.FindingsTotal)
	}

	worker.processRecord(record, true)
	worker.processRecord(record, true)
	status := worker.Status()
	if status.FindingsTotal != 1 {
		t.Fatalf("manual scan findings=%d, want one forced finding", status.FindingsTotal)
	}
}

func TestLoopDetectionSemanticAlertShape(t *testing.T) {
	finding := loopDetectionFinding{
		ContextType:   "task",
		ContextKey:    "task:run-1/task-1",
		RepeatCount:   5,
		WindowSeconds: 30,
		Fingerprint:   "http_message|target=digest:abcdef",
		Target:        "digest:abcdef",
		AgentRunID:    "run-1",
		TaskID:        "task-1",
		PID:           123,
		RootAgentPID:  100,
	}
	alert := loopDetectionSemanticAlert(finding)
	if alert.GetType() != "semantic_alert" || alert.GetEventType() != pb.EventType_SEMANTIC_ALERT || alert.GetComm() != "RESOURCE_WASTING_LOOP" {
		t.Fatalf("unexpected alert shape: %+v", alert)
	}
	if !strings.Contains(alert.GetExtraInfo(), "source=loop_detection_worker") || !strings.Contains(alert.GetExtraInfo(), "repeat_count=5") {
		t.Fatalf("alert extra info missing loop metadata: %q", alert.GetExtraInfo())
	}
	if !shouldIgnoreLoopDetectionEvent(alert) {
		t.Fatalf("semantic alert must not be fed back into the loop detector")
	}
}
