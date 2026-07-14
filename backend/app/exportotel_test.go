package app

import (
	"agent-ebpf-filter/pb"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"

	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ---- moved from backend/zz_merged_backend_test.go section exportotel_test.go ----

type captureSpanExporter struct {
	mu    sync.Mutex
	names []string
}

type countingSpanProcessor struct {
	forceFlushes atomic.Uint64
}

func (*countingSpanProcessor) OnStart(context.Context, sdktrace.ReadWriteSpan) {}

func (*countingSpanProcessor) OnEnd(sdktrace.ReadOnlySpan) {}

func (*countingSpanProcessor) Shutdown(context.Context) error { return nil }

func (p *countingSpanProcessor) ForceFlush(context.Context) error {
	p.forceFlushes.Add(1)
	return nil
}

func (c *captureSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, span := range spans {
		c.names = append(c.names, span.Name())
	}
	return nil
}

func (c *captureSpanExporter) Shutdown(context.Context) error {
	return nil
}

func (c *captureSpanExporter) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.names...)
}

func installTestOTelProvider(state *otelExporterState, provider *sdktrace.TracerProvider) {
	state.mu.Lock()
	state.enabled = true
	state.ready = true
	state.endpoint = "http://collector:4318"
	state.serviceName = "agent-ebpf-filter-test"
	state.tp = provider
	state.tracer = provider.Tracer("test")
	state.mu.Unlock()
	state.accepting.Store(true)
}

func makeOTelHierarchyRecord(ts time.Time, runID, taskID, toolCallID string) CapturedEventRecord {
	return normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: ts,
		Event: &pb.Event{
			Pid:        321,
			Type:       "wrapper_intercept",
			Comm:       "codex",
			ToolName:   "shell",
			ToolCallId: toolCallID,
			TaskId:     taskID,
			AgentRunId: runID,
		},
	})
}

func assertOTelActiveIndexesMatch(t *testing.T, state *otelExporterState) {
	t.Helper()
	state.mu.RLock()
	defer state.mu.RUnlock()
	if state.runLRU.Len() != len(state.runSpans) || state.taskLRU.Len() != len(state.taskSpans) || state.toolLRU.Len() != len(state.toolSpans) {
		t.Fatalf("active span LRU/map sizes diverged: runs=%d/%d tasks=%d/%d tools=%d/%d",
			state.runLRU.Len(), len(state.runSpans), state.taskLRU.Len(), len(state.taskSpans), state.toolLRU.Len(), len(state.toolSpans))
	}
	indexedRunTasks := 0
	for _, children := range state.runTasks {
		indexedRunTasks += len(children)
	}
	indexedRunTools := 0
	for _, children := range state.runTools {
		indexedRunTools += len(children)
	}
	indexedTaskTools := 0
	for _, children := range state.taskTools {
		indexedTaskTools += len(children)
	}
	wantRunTasks := 0
	for _, span := range state.taskSpans {
		if span.runKey == "" {
			continue
		}
		wantRunTasks++
		if _, ok := state.runTasks[span.runKey][span.key]; !ok {
			t.Fatalf("task %q is missing from run %q index", span.key, span.runKey)
		}
	}
	wantRunTools, wantTaskTools := 0, 0
	for _, span := range state.toolSpans {
		if span.runKey != "" {
			wantRunTools++
			if _, ok := state.runTools[span.runKey][span.key]; !ok {
				t.Fatalf("tool %q is missing from run %q index", span.key, span.runKey)
			}
		}
		if span.taskKey != "" {
			wantTaskTools++
			if _, ok := state.taskTools[span.taskKey][span.key]; !ok {
				t.Fatalf("tool %q is missing from task %q index", span.key, span.taskKey)
			}
		}
	}
	if indexedRunTasks != wantRunTasks || indexedRunTools != wantRunTools || indexedTaskTools != wantTaskTools {
		t.Fatalf("active child indexes retained stale entries: runTasks=%d/%d runTools=%d/%d taskTools=%d/%d",
			indexedRunTasks, wantRunTasks, indexedRunTools, wantRunTools, indexedTaskTools, wantTaskTools)
	}
}

func TestOTelExporterBuildsHierarchyAndChildSpans(t *testing.T) {
	state := newOTelExporterState()
	defer state.Close()

	exporter := &captureSpanExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	defer func() {
		_ = provider.Shutdown(context.Background())
	}()

	state.mu.Lock()
	state.enabled = true
	state.ready = true
	state.endpoint = "http://collector:4318"
	state.serviceName = "agent-ebpf-filter-test"
	state.tp = provider
	state.tracer = provider.Tracer("test")
	state.mu.Unlock()

	baseTime := time.Unix(1_700_000_000, 0).UTC()
	execRecord := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: baseTime,
		Event: &pb.Event{
			Pid:          321,
			Ppid:         320,
			Type:         "execve",
			Path:         "/usr/bin/git",
			Comm:         "git",
			ToolName:     "shell",
			ToolCallId:   "tool-1",
			TaskId:       "task-1",
			AgentRunId:   "run-1",
			TraceId:      "trace-1",
			RootAgentPid: 321,
			DurationNs:   12_000,
		},
	})
	state.handleRecord(execRecord)

	exitRecord := normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: baseTime.Add(250 * time.Millisecond),
		Event: &pb.Event{
			Pid:          321,
			Ppid:         320,
			Type:         "process_exit",
			Comm:         "git",
			ToolName:     "shell",
			ToolCallId:   "tool-1",
			TaskId:       "task-1",
			AgentRunId:   "run-1",
			TraceId:      "trace-1",
			RootAgentPid: 321,
			ExtraInfo:    "status=0",
		},
	})
	state.handleRecord(exitRecord)
	state.endIdleSpans(baseTime.Add(2 * time.Minute))
	state.mu.RLock()
	activeMapsEmpty := len(state.runSpans) == 0 && len(state.taskSpans) == 0 && len(state.toolSpans) == 0
	activeLRUsEmpty := state.runLRU.Len() == 0 && state.taskLRU.Len() == 0 && state.toolLRU.Len() == 0
	state.mu.RUnlock()
	if !activeMapsEmpty || !activeLRUsEmpty {
		t.Fatalf("ended hierarchy retained active map/LRU state: mapsEmpty=%v lrusEmpty=%v", activeMapsEmpty, activeLRUsEmpty)
	}
	assertOTelActiveIndexesMatch(t, state)

	names := exporter.Names()
	for _, expected := range []string{"agent.run", "codex.task", "tool.call", "process.exec", "process.exit"} {
		if !slices.Contains(names, expected) {
			t.Fatalf("expected exported spans to include %q, got %v", expected, names)
		}
	}
}

func TestBuildOTLPHTTPOptionsSupportsExplicitPath(t *testing.T) {
	opts, err := buildOTLPHTTPOptions("https://collector.example.com/custom/traces", map[string]string{
		"Authorization": "Bearer token",
	})
	if err != nil {
		t.Fatalf("buildOTLPHTTPOptions() error = %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("expected options to be returned")
	}
}

func TestBoundedOTelSpanLimitsCapUnlimitedEnvironment(t *testing.T) {
	for _, key := range []string{
		"OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT",
		"OTEL_SPAN_ATTRIBUTE_COUNT_LIMIT",
		"OTEL_SPAN_EVENT_COUNT_LIMIT",
		"OTEL_EVENT_ATTRIBUTE_COUNT_LIMIT",
		"OTEL_SPAN_LINK_COUNT_LIMIT",
		"OTEL_LINK_ATTRIBUTE_COUNT_LIMIT",
	} {
		t.Setenv(key, "-1")
	}
	limits := boundedOTelSpanLimits()
	if limits.AttributeValueLengthLimit != otelMaxAttributeLength ||
		limits.AttributeCountLimit != otelMaxSpanAttributes ||
		limits.EventCountLimit != otelMaxSpanEvents ||
		limits.AttributePerEventCountLimit != otelMaxEventAttributes ||
		limits.LinkCountLimit != otelMaxSpanLinks ||
		limits.AttributePerLinkCountLimit != otelMaxLinkAttributes {
		t.Fatalf("unlimited SDK span limits were not bounded: %#v", limits)
	}

	t.Setenv("OTEL_SPAN_ATTRIBUTE_VALUE_LENGTH_LIMIT", "64")
	t.Setenv("OTEL_SPAN_EVENT_COUNT_LIMIT", "16")
	limits = boundedOTelSpanLimits()
	if limits.AttributeValueLengthLimit != 64 || limits.EventCountLimit != 16 {
		t.Fatalf("stricter operator limits were not preserved: %#v", limits)
	}
}

func TestOTelAttributeAndEventNamesAreExplicitlyBounded(t *testing.T) {
	longValue := strings.Repeat("界", otelMaxAttributeLength+100) + "\xff"
	envelope := &pb.EventEnvelope{
		SchemaVersion:  longValue,
		EventId:        longValue,
		Source:         longValue,
		AgentRunId:     longValue,
		TaskId:         longValue,
		ToolCallId:     longValue,
		ToolName:       longValue,
		TraceId:        longValue,
		PolicyDecision: longValue,
		Payload: &pb.EventEnvelope_FileEvent{FileEvent: &pb.FileEvent{
			Operation: longValue,
			Path:      longValue,
		}},
	}
	attrs := append(buildOTelAttributes(envelope), buildHierarchyAttributes(envelope, "tool")...)
	for _, attr := range attrs {
		if attr.Value.Type() != attribute.STRING {
			continue
		}
		value := attr.Value.AsString()
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > otelMaxAttributeLength {
			t.Fatalf("attribute %q was not safely bounded: bytes=%d runes=%d", attr.Key, len(value), utf8.RuneCountInString(value))
		}
	}
	name := otelEventName(envelope)
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > otelMaxNameLength {
		t.Fatalf("event name was not safely bounded: bytes=%d runes=%d", len(name), utf8.RuneCountInString(name))
	}
	status := otelStatusMessage(envelope)
	if !utf8.ValidString(status) || utf8.RuneCountInString(status) > otelMaxAttributeLength {
		t.Fatalf("status message was not safely bounded: bytes=%d runes=%d", len(status), utf8.RuneCountInString(status))
	}
}

func TestStableOTelKeyPreservesExistingEncodingAndBoundsComponents(t *testing.T) {
	parts := []string{" run:alpha ", "", "task:beta", " trace:gamma"}
	legacyInput := strings.Join([]string{"run:alpha", "task:beta", "trace:gamma"}, "\x00")
	sum := sha256.Sum256([]byte(legacyInput))
	want := "otel_" + hex.EncodeToString(sum[:10])
	if got := stableOTelKey(parts...); got != want {
		t.Fatalf("stableOTelKey() = %q, want compatibility key %q", got, want)
	}
	commonPrefix := strings.Repeat("x", otelMaxAttributeLength+1)
	first := boundedOTelKeyComponent(commonPrefix + "a")
	second := boundedOTelKeyComponent(commonPrefix + "b")
	if first == second || len(first) > 80 || len(second) > 80 {
		t.Fatalf("oversized key components were not compactly distinguished: first=%q second=%q", first, second)
	}
}

func TestOTelExporterCloseIsConcurrentAndRejectsLateWork(t *testing.T) {
	state := newOTelExporterState()

	const closers = 16
	var wg sync.WaitGroup
	wg.Add(closers)
	for range closers {
		go func() {
			defer wg.Done()
			state.Close()
		}()
	}
	wg.Wait()

	if !state.closed.Load() {
		t.Fatal("exporter was not marked closed")
	}
	select {
	case <-state.stopCh:
	default:
		t.Fatal("exporter stop channel is still open")
	}
	workersDone := make(chan struct{})
	go func() {
		state.workers.Wait()
		close(workersDone)
	}()
	select {
	case <-workersDone:
	case <-time.After(time.Second):
		t.Fatal("exporter workers did not stop before Close returned")
	}

	state.Record(CapturedEventRecord{Event: &pb.Event{Type: "execve"}})
	if queued := len(state.queue); queued != 0 {
		t.Fatalf("post-close queue length = %d, want 0", queued)
	}
	state.ApplySettings(RuntimeSettings{
		OtlpEnabled:     true,
		OtlpEndpoint:    "http://collector:4318",
		OtlpServiceName: "late-config",
	})
	if snapshot := state.Snapshot(); snapshot.Enabled || snapshot.Ready {
		t.Fatalf("closed exporter accepted settings: %#v", snapshot)
	}
}

func TestOTelExporterDisabledRecordPathDoesNotQueueOrAllocate(t *testing.T) {
	state := newOTelExporterState()
	defer state.Close()
	record := CapturedEventRecord{Event: &pb.Event{
		Pid:        321,
		Type:       "wrapper_intercept",
		AgentRunId: "disabled-run",
	}}

	allocations := testing.AllocsPerRun(1000, func() {
		state.Record(record)
	})
	if allocations != 0 {
		t.Fatalf("disabled Record() allocations = %.2f, want 0", allocations)
	}
	snapshot := state.Snapshot()
	if snapshot.QueueLen != 0 || snapshot.EnqueuedEvents != 0 || snapshot.ProcessedEvents != 0 || snapshot.DroppedEvents != 0 {
		t.Fatalf("disabled exporter accepted or processed work: %#v", snapshot)
	}
}

func TestOTelSpanCapacityEvictsLeastRecentlyUsedHierarchy(t *testing.T) {
	state := newOTelExporterState()
	exporter := &captureSpanExporter{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	installTestOTelProvider(state, provider)
	defer state.Close()

	state.mu.Lock()
	state.maxRunSpans = 2
	state.maxTaskSpans = 8
	state.maxToolSpans = 8
	state.mu.Unlock()

	baseTime := time.Unix(1_700_000_000, 0).UTC()
	recordA := makeOTelHierarchyRecord(baseTime, "run-a", "task-a", "tool-a")
	recordB := makeOTelHierarchyRecord(baseTime.Add(time.Second), "run-b", "task-b", "tool-b")
	recordC := makeOTelHierarchyRecord(baseTime.Add(3*time.Second), "run-c", "task-c", "tool-c")
	state.handleRecord(recordA)
	state.handleRecord(recordB)
	// Refresh A after B so the capacity reclaim must choose B.
	refreshedA := makeOTelHierarchyRecord(baseTime.Add(2*time.Second), "run-a", "task-a", "tool-a")
	state.handleRecord(refreshedA)
	state.handleRecord(recordC)

	runA := otelRunKey(recordA.Envelope)
	runB := otelRunKey(recordB.Envelope)
	runC := otelRunKey(recordC.Envelope)
	state.mu.RLock()
	_, hasA := state.runSpans[runA]
	_, hasB := state.runSpans[runB]
	_, hasC := state.runSpans[runC]
	runLRULen := state.runLRU.Len()
	taskLRULen := state.taskLRU.Len()
	toolLRULen := state.toolLRU.Len()
	state.mu.RUnlock()
	if !hasA || hasB || !hasC {
		t.Fatalf("unexpected active runs after LRU reclaim: A=%v B=%v C=%v", hasA, hasB, hasC)
	}

	snapshot := state.Snapshot()
	if snapshot.ActiveRunSpans != 2 || snapshot.ActiveTaskSpans != 2 || snapshot.ActiveToolSpans != 2 {
		t.Fatalf("hierarchy capacity was not enforced: %#v", snapshot)
	}
	if snapshot.EvictedRunSpans != 1 || snapshot.EvictedTaskSpans != 1 || snapshot.EvictedToolSpans != 1 {
		t.Fatalf("hierarchy eviction metrics mismatch: %#v", snapshot)
	}
	if runLRULen != snapshot.ActiveRunSpans || taskLRULen != snapshot.ActiveTaskSpans || toolLRULen != snapshot.ActiveToolSpans {
		t.Fatalf("LRU/map sizes diverged: run=%d task=%d tool=%d snapshot=%#v", runLRULen, taskLRULen, toolLRULen, snapshot)
	}
	assertOTelActiveIndexesMatch(t, state)
	for _, name := range []string{"agent.run", "codex.task", "tool.call"} {
		if !slices.Contains(exporter.Names(), name) {
			t.Fatalf("capacity reclaim did not end %q span: %v", name, exporter.Names())
		}
	}
}

func TestOTelTaskAndToolCapacitiesRemainHardBounded(t *testing.T) {
	tests := []struct {
		name             string
		maxTasks         int
		maxTools         int
		wantTasks        int
		wantTools        int
		wantEvictedTasks uint64
		wantEvictedTools uint64
		sharedTask       bool
	}{
		{
			name:             "task reclaim cascades to child tool",
			maxTasks:         2,
			maxTools:         8,
			wantTasks:        2,
			wantTools:        2,
			wantEvictedTasks: 4,
			wantEvictedTools: 4,
		},
		{
			name:             "tool reclaim preserves parents",
			maxTasks:         8,
			maxTools:         2,
			wantTasks:        1,
			wantTools:        2,
			wantEvictedTools: 4,
			sharedTask:       true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newOTelExporterState()
			provider := sdktrace.NewTracerProvider()
			installTestOTelProvider(state, provider)
			defer state.Close()
			state.mu.Lock()
			state.maxRunSpans = 8
			state.maxTaskSpans = test.maxTasks
			state.maxToolSpans = test.maxTools
			state.mu.Unlock()

			baseTime := time.Unix(1_700_000_000, 0).UTC()
			for index := 0; index < 6; index++ {
				taskID := fmt.Sprintf("task-%d", index)
				if test.sharedTask {
					taskID = "shared-task"
				}
				state.handleRecord(makeOTelHierarchyRecord(
					baseTime.Add(time.Duration(index)*time.Second),
					"shared-run",
					taskID,
					fmt.Sprintf("tool-%d", index),
				))
			}

			snapshot := state.Snapshot()
			if snapshot.ActiveRunSpans != 1 || snapshot.ActiveTaskSpans != test.wantTasks || snapshot.ActiveToolSpans != test.wantTools {
				t.Fatalf("active hierarchy exceeded cap: %#v", snapshot)
			}
			if snapshot.EvictedTaskSpans != test.wantEvictedTasks || snapshot.EvictedToolSpans != test.wantEvictedTools {
				t.Fatalf("capacity eviction metrics mismatch: %#v", snapshot)
			}
			assertOTelActiveIndexesMatch(t, state)
		})
	}
}

func TestOTelExporterCloseDrainsEveryAcceptedConcurrentRecord(t *testing.T) {
	state := newOTelExporterState()
	provider := sdktrace.NewTracerProvider()
	installTestOTelProvider(state, provider)
	record := makeOTelHierarchyRecord(time.Now().UTC(), "run-close", "", "")

	start := make(chan struct{})
	var producers sync.WaitGroup
	for producer := 0; producer < 8; producer++ {
		producers.Add(1)
		go func() {
			defer producers.Done()
			<-start
			for index := 0; index < 1000; index++ {
				state.Record(record)
			}
		}()
	}
	close(start)
	deadline := time.Now().Add(time.Second)
	for state.Snapshot().EnqueuedEvents == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state.Close()
	producers.Wait()

	snapshot := state.Snapshot()
	if snapshot.QueueLen != 0 {
		t.Fatalf("queue retained records after close: %#v", snapshot)
	}
	if snapshot.ProcessedEvents != snapshot.EnqueuedEvents {
		t.Fatalf("accepted records were not drained: %#v", snapshot)
	}
	if !state.closed.Load() || state.accepting.Load() {
		t.Fatalf("closed exporter still accepts events: closed=%v accepting=%v", state.closed.Load(), state.accepting.Load())
	}
}

func TestOTelEventHandlingDoesNotForceFlushPerHierarchy(t *testing.T) {
	state := newOTelExporterState()
	processor := &countingSpanProcessor{}
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(processor))
	installTestOTelProvider(state, provider)
	defer state.Close()

	baseTime := time.Unix(1_700_000_000, 0).UTC()
	state.handleRecord(normalizeCapturedEventRecord(CapturedEventRecord{
		ReceivedAt: baseTime,
		Event: &pb.Event{
			Pid:        321,
			Type:       "process_exit",
			AgentRunId: "run-exit",
		},
	}))
	state.handleRecord(makeOTelHierarchyRecord(baseTime.Add(time.Second), "run-idle", "", ""))
	state.endIdleSpans(baseTime.Add(2 * time.Minute))

	if got := processor.forceFlushes.Load(); got != 0 {
		t.Fatalf("event/idle processing forced synchronous exporter flush %d times", got)
	}
}
