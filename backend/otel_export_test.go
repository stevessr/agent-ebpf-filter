package main

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"agent-ebpf-filter/pb"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type captureSpanExporter struct {
	mu    sync.Mutex
	names []string
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
