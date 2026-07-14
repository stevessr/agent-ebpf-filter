package app

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"agent-ebpf-filter/pb"
)

func TestLoopDetectionWorkerShutdownTimeoutKeepsGeneration(t *testing.T) {
	worker := newLoopDetectionWorker()
	oldDone := make(chan struct{})
	worker.mu.Lock()
	worker.started = true
	worker.queue = make(chan loopDetectionWorkItem, 1)
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

func TestLoopDetectionCanceledScanDoesNotMutateState(t *testing.T) {
	oldStore := runtimeSettingsStore
	runtimeSettingsStore = &runtimeState{settings: RuntimeSettings{LoopDetection: LoopDetectionSettings{
		Enabled:         true,
		WindowSeconds:   60,
		RepeatThreshold: 2,
		MaxContexts:     128,
	}}}
	t.Cleanup(func() { runtimeSettingsStore = oldStore })

	worker := newLoopDetectionWorker()
	records := []CapturedEventRecord{{
		ReceivedAt: time.Now().UTC(),
		Event:      &pb.Event{Pid: 1, Type: "openat", Comm: "scan", Path: "/tmp/item", AgentRunId: "run"},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	worker.processScan(ctx, records, true)
	if status := worker.Status(); status.ConsumedTotal != 0 || status.WindowCount != 0 {
		t.Fatalf("canceled scan mutated state: %+v", status)
	}
}

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

func TestLoopDetectionWindowLRUEvictsLeastRecentlyUsed(t *testing.T) {
	worker := newLoopDetectionWorker()
	settings := LoopDetectionSettings{Enabled: true, WindowSeconds: 60, RepeatThreshold: 1000, MaxContexts: 3}
	base := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	process := func(taskID string, offset time.Duration) *pb.Event {
		event := &pb.Event{Type: "openat", TaskId: taskID, Path: "/tmp/shared"}
		worker.processRecordWithSettings(CapturedEventRecord{ReceivedAt: base.Add(offset), Event: event}, true, settings)
		return event
	}

	a := process("a", 0)
	process("b", time.Second)
	process("c", 2*time.Second)
	process("a", 3*time.Second)
	d := process("d", 4*time.Second)

	keyFor := func(event *pb.Event) loopDetectionWindowKey {
		_, contextKey := loopDetectionContext(event)
		fingerprint, _ := loopDetectionFingerprint(event)
		return loopDetectionWindowKey{contextKey: contextKey, fingerprint: fingerprint}
	}
	if len(worker.windows) != 3 {
		t.Fatalf("window count = %d, want 3", len(worker.windows))
	}
	if _, ok := worker.windows[keyFor(a)]; !ok {
		t.Fatal("recently touched window was evicted")
	}
	if _, ok := worker.windows[loopDetectionWindowKey{contextKey: "task:b", fingerprint: keyFor(a).fingerprint}]; ok {
		t.Fatal("least recently used window was retained")
	}
	if _, ok := worker.windows[keyFor(d)]; !ok {
		t.Fatal("newest window is missing")
	}
	if worker.windowLRUHead == nil || worker.windowLRUHead.ContextKey != "task:c" || worker.windowLRUTail == nil || worker.windowLRUTail.ContextKey != "task:d" {
		t.Fatalf("unexpected LRU endpoints: head=%v tail=%v", worker.windowLRUHead, worker.windowLRUTail)
	}
	worker.resetNow()
	if len(worker.windows) != 0 || worker.windowLRUHead != nil || worker.windowLRUTail != nil || !worker.lastWindowGC.IsZero() {
		t.Fatal("reset did not clear loop detection window indexes")
	}
}

func TestLoopDetectionWindowGCIsPeriodic(t *testing.T) {
	worker := newLoopDetectionWorker()
	settings := LoopDetectionSettings{Enabled: true, WindowSeconds: 1, RepeatThreshold: 1000, MaxContexts: 16}
	base := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	process := func(taskID string, at time.Time) {
		worker.processRecordWithSettings(CapturedEventRecord{
			ReceivedAt: at,
			Event:      &pb.Event{Type: "openat", TaskId: taskID, Path: "/tmp/gc"},
		}, true, settings)
	}
	process("old-a", base)
	process("old-b", base.Add(100*time.Millisecond))
	if worker.windowGCRuns != 1 {
		t.Fatalf("GC runs before interval = %d, want 1", worker.windowGCRuns)
	}
	process("fresh", base.Add(3*time.Second))

	if worker.windowGCRuns != 2 {
		t.Fatalf("GC runs after interval = %d, want 2", worker.windowGCRuns)
	}
	if len(worker.windows) != 1 || worker.windowLRUHead == nil || worker.windowLRUHead.ContextKey != "task:fresh" || worker.windowLRUTail != worker.windowLRUHead {
		t.Fatalf("expired window GC state: windows=%d head=%v tail=%v", len(worker.windows), worker.windowLRUHead, worker.windowLRUTail)
	}
	if worker.windowEvicted != 2 {
		t.Fatalf("evicted windows = %d, want 2", worker.windowEvicted)
	}
	status := worker.Status()
	if status.WindowGCRuns != 2 || status.WindowEvicted != 2 {
		t.Fatalf("GC status counters = %+v", status)
	}
}

func TestLoopDetectionWindowMetadataIsBounded(t *testing.T) {
	worker := newLoopDetectionWorker()
	settings := LoopDetectionSettings{Enabled: true, WindowSeconds: 60, RepeatThreshold: 1000, MaxContexts: 16}
	base := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	longTaskID := strings.Repeat("任务", 256)
	for index := 0; index < 100; index++ {
		worker.processRecordWithSettings(CapturedEventRecord{
			ReceivedAt: base,
			Event: &pb.Event{
				Pid:       uint32(index + 1),
				Type:      "openat",
				TaskId:    longTaskID,
				Comm:      strings.Repeat("c", 200) + strconv.Itoa(index),
				Path:      "/tmp/fixed",
				ExtraPath: "/tmp/extra-" + strconv.Itoa(index),
			},
		}, true, settings)
	}

	if len(worker.windows) != 1 {
		t.Fatalf("window count = %d, want 1", len(worker.windows))
	}
	win := worker.windowLRUHead
	if win == nil {
		t.Fatal("missing retained window")
	}
	for name, size := range map[string]int{
		"eventTypes": len(win.EventTypes),
		"pids":       len(win.Pids),
		"comms":      len(win.Comms),
		"paths":      len(win.Paths),
		"toolNames":  len(win.ToolNames),
	} {
		if size > loopDetectionWindowMetadataLimit {
			t.Fatalf("%s size = %d, limit %d", name, size, loopDetectionWindowMetadataLimit)
		}
	}
	if len(win.TaskID) > loopDetectionContextComponentBytes || len(win.Comm) > loopDetectionMetadataValueBytes || len(win.Target) > loopDetectionTargetBytes {
		t.Fatalf("retained values are not bounded: task=%d comm=%d target=%d", len(win.TaskID), len(win.Comm), len(win.Target))
	}
	if !strings.Contains(win.TaskID, "~sha256:") {
		t.Fatalf("long task id lacks stable digest suffix: %q", win.TaskID)
	}
}

func BenchmarkLoopDetectionSteadyStateWindowHit(b *testing.B) {
	for _, maxContexts := range []int{64, loopDetectionDefaultMaxContexts} {
		b.Run(strconv.Itoa(maxContexts), func(b *testing.B) {
			worker := newLoopDetectionWorker()
			settings := LoopDetectionSettings{Enabled: true, WindowSeconds: 60, RepeatThreshold: 1 << 30, MaxContexts: maxContexts}
			base := time.Unix(1, 0).UTC()
			for index := 0; index < maxContexts; index++ {
				worker.processRecordWithSettings(CapturedEventRecord{
					ReceivedAt: base,
					Event:      &pb.Event{Type: "openat", TaskId: "warm-" + strconv.Itoa(index), Path: "/tmp/hot"},
				}, true, settings)
			}
			hot := CapturedEventRecord{ReceivedAt: base, Event: &pb.Event{Type: "openat", TaskId: "warm-0", Path: "/tmp/hot"}}

			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				worker.processRecordWithSettings(hot, true, settings)
			}
		})
	}
}
