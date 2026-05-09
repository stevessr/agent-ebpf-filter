package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	otelExporterQueueSize = 2048
	otelToolIdleTimeout   = 20 * time.Second
	otelTaskIdleTimeout   = 45 * time.Second
	otelRunIdleTimeout    = 90 * time.Second
)

type OTelHealthResponse struct {
	Enabled         bool   `json:"enabled"`
	Ready           bool   `json:"ready"`
	Endpoint        string `json:"endpoint"`
	ServiceName     string `json:"serviceName"`
	QueueLen        int    `json:"queueLen"`
	ActiveRunSpans  int    `json:"activeRunSpans"`
	ActiveTaskSpans int    `json:"activeTaskSpans"`
	ActiveToolSpans int    `json:"activeToolSpans"`
	ExportedSpans   uint64 `json:"exportedSpans"`
	DroppedEvents   uint64 `json:"droppedEvents"`
	LastExportedAt  string `json:"lastExportedAt,omitempty"`
	LastError       string `json:"lastError,omitempty"`
}

type activeOTelSpan struct {
	ctx      context.Context
	span     oteltrace.Span
	key      string
	runKey   string
	taskKey  string
	lastSeen time.Time
}

type monitoringSpanExporter struct {
	inner sdktrace.SpanExporter
	owner *otelExporterState
}

func (m *monitoringSpanExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	if m == nil || m.inner == nil {
		return nil
	}
	err := m.inner.ExportSpans(ctx, spans)
	if err != nil {
		if m.owner != nil {
			m.owner.noteExportFailure(err)
		}
		return err
	}
	if m.owner != nil {
		m.owner.noteExportSuccess(len(spans))
	}
	return nil
}

func (m *monitoringSpanExporter) Shutdown(ctx context.Context) error {
	if m == nil || m.inner == nil {
		return nil
	}
	return m.inner.Shutdown(ctx)
}

type otelExporterState struct {
	mu          sync.RWMutex
	queue       chan CapturedEventRecord
	stopCh      chan struct{}
	enabled     bool
	ready       bool
	endpoint    string
	serviceName string
	headers     map[string]string
	lastError   string

	tp     *sdktrace.TracerProvider
	tracer oteltrace.Tracer

	runSpans  map[string]*activeOTelSpan
	taskSpans map[string]*activeOTelSpan
	toolSpans map[string]*activeOTelSpan

	exportedSpans uint64
	droppedEvents uint64
	lastExportAt  atomic.Int64
}

func newOTelExporterState() *otelExporterState {
	state := &otelExporterState{
		queue:     make(chan CapturedEventRecord, otelExporterQueueSize),
		stopCh:    make(chan struct{}),
		headers:   make(map[string]string),
		runSpans:  make(map[string]*activeOTelSpan),
		taskSpans: make(map[string]*activeOTelSpan),
		toolSpans: make(map[string]*activeOTelSpan),
	}
	go state.run()
	go state.sweepLoop()
	return state
}

var otelExporterStore = newOTelExporterState()

func (s *otelExporterState) Close() {
	if s == nil {
		return
	}
	select {
	case <-s.stopCh:
		return
	default:
		close(s.stopCh)
	}
	s.disable()
}

func (s *otelExporterState) run() {
	for {
		select {
		case <-s.stopCh:
			return
		case record := <-s.queue:
			s.handleRecord(record)
		}
	}
}

func (s *otelExporterState) sweepLoop() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.endIdleSpans(now.UTC())
		}
	}
}

func (s *otelExporterState) noteExportSuccess(count int) {
	if s == nil || count <= 0 {
		return
	}
	atomic.AddUint64(&s.exportedSpans, uint64(count))
	s.lastExportAt.Store(time.Now().UTC().UnixNano())
}

func (s *otelExporterState) noteExportFailure(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	s.lastError = err.Error()
	s.mu.Unlock()
}

func (s *otelExporterState) Snapshot() OTelHealthResponse {
	if s == nil {
		return OTelHealthResponse{}
	}
	s.mu.RLock()
	resp := OTelHealthResponse{
		Enabled:         s.enabled,
		Ready:           s.ready,
		Endpoint:        s.endpoint,
		ServiceName:     s.serviceName,
		QueueLen:        len(s.queue),
		ActiveRunSpans:  len(s.runSpans),
		ActiveTaskSpans: len(s.taskSpans),
		ActiveToolSpans: len(s.toolSpans),
		ExportedSpans:   atomic.LoadUint64(&s.exportedSpans),
		DroppedEvents:   atomic.LoadUint64(&s.droppedEvents),
		LastError:       s.lastError,
	}
	s.mu.RUnlock()
	if lastExport := s.lastExportAt.Load(); lastExport > 0 {
		resp.LastExportedAt = time.Unix(0, lastExport).UTC().Format(time.RFC3339Nano)
	}
	return resp
}

func (s *otelExporterState) ApplySettings(settings RuntimeSettings) {
	if s == nil {
		return
	}
	endpoint := strings.TrimSpace(settings.OtlpEndpoint)
	serviceName := firstNonEmpty(settings.OtlpServiceName, "agent-ebpf-filter")
	headers := cloneStringMap(settings.OtlpHeaders)

	if !settings.OtlpEnabled {
		s.disable()
		return
	}
	if endpoint == "" {
		s.mu.Lock()
		s.enabled = true
		s.ready = false
		s.endpoint = ""
		s.serviceName = serviceName
		s.headers = headers
		s.lastError = "OTLP endpoint is required when export is enabled"
		s.mu.Unlock()
		s.disableProviderOnly()
		return
	}

	provider, tracer, err := buildOTelTracerProvider(endpoint, serviceName, headers, s)
	if err != nil {
		s.mu.Lock()
		s.enabled = true
		s.ready = false
		s.endpoint = endpoint
		s.serviceName = serviceName
		s.headers = headers
		s.lastError = err.Error()
		s.mu.Unlock()
		s.disableProviderOnly()
		return
	}

	s.mu.Lock()
	oldProvider := s.tp
	oldRunSpans := s.runSpans
	oldTaskSpans := s.taskSpans
	oldToolSpans := s.toolSpans

	s.enabled = true
	s.ready = true
	s.endpoint = endpoint
	s.serviceName = serviceName
	s.headers = headers
	s.lastError = ""
	s.tp = provider
	s.tracer = tracer
	s.runSpans = make(map[string]*activeOTelSpan)
	s.taskSpans = make(map[string]*activeOTelSpan)
	s.toolSpans = make(map[string]*activeOTelSpan)
	s.mu.Unlock()

	endSpanMap(oldToolSpans, time.Now().UTC())
	endSpanMap(oldTaskSpans, time.Now().UTC())
	endSpanMap(oldRunSpans, time.Now().UTC())
	if oldProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = oldProvider.ForceFlush(ctx)
		_ = oldProvider.Shutdown(ctx)
		cancel()
	}
}

func (s *otelExporterState) disable() {
	if s == nil {
		return
	}
	s.mu.Lock()
	oldProvider := s.tp
	oldRunSpans := s.runSpans
	oldTaskSpans := s.taskSpans
	oldToolSpans := s.toolSpans

	s.enabled = false
	s.ready = false
	s.endpoint = ""
	s.serviceName = ""
	s.headers = make(map[string]string)
	s.lastError = ""
	s.tp = nil
	s.tracer = nil
	s.runSpans = make(map[string]*activeOTelSpan)
	s.taskSpans = make(map[string]*activeOTelSpan)
	s.toolSpans = make(map[string]*activeOTelSpan)
	s.mu.Unlock()

	endSpanMap(oldToolSpans, time.Now().UTC())
	endSpanMap(oldTaskSpans, time.Now().UTC())
	endSpanMap(oldRunSpans, time.Now().UTC())
	if oldProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = oldProvider.ForceFlush(ctx)
		_ = oldProvider.Shutdown(ctx)
		cancel()
	}
}

func (s *otelExporterState) disableProviderOnly() {
	if s == nil {
		return
	}
	s.mu.Lock()
	oldProvider := s.tp
	oldRunSpans := s.runSpans
	oldTaskSpans := s.taskSpans
	oldToolSpans := s.toolSpans
	s.tp = nil
	s.tracer = nil
	s.runSpans = make(map[string]*activeOTelSpan)
	s.taskSpans = make(map[string]*activeOTelSpan)
	s.toolSpans = make(map[string]*activeOTelSpan)
	s.mu.Unlock()

	endSpanMap(oldToolSpans, time.Now().UTC())
	endSpanMap(oldTaskSpans, time.Now().UTC())
	endSpanMap(oldRunSpans, time.Now().UTC())
	if oldProvider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = oldProvider.Shutdown(ctx)
		cancel()
	}
}

func (s *otelExporterState) Record(record CapturedEventRecord) {
	if s == nil || record.Event == nil {
		return
	}
	record = normalizeCapturedEventRecord(record)
	select {
	case s.queue <- record:
	default:
		atomic.AddUint64(&s.droppedEvents, 1)
	}
}

func (s *otelExporterState) handleRecord(record CapturedEventRecord) {
	record = normalizeCapturedEventRecord(record)
	envelope := record.Envelope
	if envelope == nil {
		return
	}

	timestamp := record.ReceivedAt.UTC()
	if ts := envelope.GetTimestampNs(); ts > 0 {
		timestamp = time.Unix(0, int64(ts)).UTC()
	}
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}

	hierarchy := s.ensureSpanHierarchy(envelope, timestamp)
	attrs := buildOTelAttributes(envelope)
	eventName := otelEventName(envelope)
	if hierarchy.tool != nil {
		hierarchy.tool.span.AddEvent(eventName, oteltrace.WithTimestamp(timestamp), oteltrace.WithAttributes(attrs...))
	} else if hierarchy.task != nil {
		hierarchy.task.span.AddEvent(eventName, oteltrace.WithTimestamp(timestamp), oteltrace.WithAttributes(attrs...))
	} else if hierarchy.run != nil {
		hierarchy.run.span.AddEvent(eventName, oteltrace.WithTimestamp(timestamp), oteltrace.WithAttributes(attrs...))
	}

	if s.shouldCreateChildSpan(envelope) {
		parentCtx := context.Background()
		switch {
		case hierarchy.tool != nil:
			parentCtx = hierarchy.tool.ctx
		case hierarchy.task != nil:
			parentCtx = hierarchy.task.ctx
		case hierarchy.run != nil:
			parentCtx = hierarchy.run.ctx
		}
		s.createChildSpan(parentCtx, eventName, envelope, attrs, timestamp)
	}

	if shouldEndOTelHierarchy(envelope) {
		s.endRelatedSpans(envelope, timestamp)
	}
}

func (s *otelExporterState) ensureSpanHierarchy(envelope *pb.EventEnvelope, ts time.Time) otelSpanHierarchy {
	var hierarchy otelSpanHierarchy
	if s == nil || envelope == nil {
		return hierarchy
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.ready || s.tracer == nil {
		return hierarchy
	}

	runKey := otelRunKey(envelope)
	taskKey := otelTaskKey(envelope, runKey)
	toolKey := otelToolKey(envelope, taskKey, runKey)

	if runKey != "" {
		if span := s.runSpans[runKey]; span != nil {
			span.lastSeen = ts
			hierarchy.run = span
		} else {
			ctx, span := s.tracer.Start(
				context.Background(),
				"agent.run",
				oteltrace.WithTimestamp(ts),
				oteltrace.WithAttributes(buildHierarchyAttributes(envelope, "run")...),
			)
			active := &activeOTelSpan{ctx: ctx, span: span, key: runKey, runKey: runKey, lastSeen: ts}
			s.runSpans[runKey] = active
			hierarchy.run = active
		}
	}

	if taskKey != "" {
		if span := s.taskSpans[taskKey]; span != nil {
			span.lastSeen = ts
			hierarchy.task = span
		} else {
			parentCtx := context.Background()
			if hierarchy.run != nil {
				parentCtx = hierarchy.run.ctx
			}
			ctx, span := s.tracer.Start(
				parentCtx,
				"codex.task",
				oteltrace.WithTimestamp(ts),
				oteltrace.WithAttributes(buildHierarchyAttributes(envelope, "task")...),
			)
			active := &activeOTelSpan{ctx: ctx, span: span, key: taskKey, runKey: runKey, taskKey: taskKey, lastSeen: ts}
			s.taskSpans[taskKey] = active
			hierarchy.task = active
		}
	}

	if toolKey != "" {
		if span := s.toolSpans[toolKey]; span != nil {
			span.lastSeen = ts
			hierarchy.tool = span
		} else {
			parentCtx := context.Background()
			if hierarchy.task != nil {
				parentCtx = hierarchy.task.ctx
			} else if hierarchy.run != nil {
				parentCtx = hierarchy.run.ctx
			}
			ctx, span := s.tracer.Start(
				parentCtx,
				otelToolSpanName(envelope),
				oteltrace.WithTimestamp(ts),
				oteltrace.WithAttributes(buildHierarchyAttributes(envelope, "tool")...),
			)
			active := &activeOTelSpan{ctx: ctx, span: span, key: toolKey, runKey: runKey, taskKey: taskKey, lastSeen: ts}
			s.toolSpans[toolKey] = active
			hierarchy.tool = active
		}
	}

	if hierarchy.task == nil && hierarchy.tool != nil {
		hierarchy.task = s.taskSpans[hierarchy.tool.taskKey]
	}
	if hierarchy.run == nil {
		switch {
		case hierarchy.tool != nil:
			hierarchy.run = s.runSpans[hierarchy.tool.runKey]
		case hierarchy.task != nil:
			hierarchy.run = s.runSpans[hierarchy.task.runKey]
		}
	}
	return hierarchy
}

type otelSpanHierarchy struct {
	run  *activeOTelSpan
	task *activeOTelSpan
	tool *activeOTelSpan
}

func (s *otelExporterState) shouldCreateChildSpan(envelope *pb.EventEnvelope) bool {
	if envelope == nil {
		return false
	}
	switch envelope.GetPayload().(type) {
	case *pb.EventEnvelope_ExecEvent, *pb.EventEnvelope_NetworkEvent, *pb.EventEnvelope_ProcessEvent, *pb.EventEnvelope_McpEvent, *pb.EventEnvelope_WrapperEvent, *pb.EventEnvelope_HookEvent:
		return true
	case *pb.EventEnvelope_FileEvent:
		return strings.HasPrefix(otelEventName(envelope), "file.")
	case *pb.EventEnvelope_PolicyEvent:
		return strings.TrimSpace(envelope.GetPolicyDecision()) != "" || envelope.GetRiskScore() > 0
	default:
		return false
	}
}

func (s *otelExporterState) createChildSpan(parentCtx context.Context, spanName string, envelope *pb.EventEnvelope, attrs []attribute.KeyValue, ts time.Time) {
	if s == nil || envelope == nil {
		return
	}
	s.mu.RLock()
	tracer := s.tracer
	ready := s.ready
	s.mu.RUnlock()
	if !ready || tracer == nil {
		return
	}
	endTs := ts.Add(time.Nanosecond)
	if legacy := envelope.GetLegacyEvent(); legacy != nil && legacy.GetDurationNs() > 0 {
		endTs = ts.Add(time.Duration(legacy.GetDurationNs()))
	}
	_, span := tracer.Start(parentCtx, spanName, oteltrace.WithTimestamp(ts), oteltrace.WithAttributes(attrs...))
	if shouldMarkSpanError(envelope) {
		span.SetStatus(codes.Error, otelStatusMessage(envelope))
	}
	span.End(oteltrace.WithTimestamp(endTs))
}

func (s *otelExporterState) endIdleSpans(now time.Time) {
	if s == nil {
		return
	}
	s.mu.Lock()
	toolSpans := collectIdleSpans(s.toolSpans, now, otelToolIdleTimeout)
	for _, span := range toolSpans {
		delete(s.toolSpans, span.key)
	}
	taskSpans := collectIdleTaskSpans(s.taskSpans, s.toolSpans, now, otelTaskIdleTimeout)
	for _, span := range taskSpans {
		delete(s.taskSpans, span.key)
	}
	runSpans := collectIdleRunSpans(s.runSpans, s.taskSpans, s.toolSpans, now, otelRunIdleTimeout)
	for _, span := range runSpans {
		delete(s.runSpans, span.key)
	}
	tp := s.tp
	s.mu.Unlock()

	endSpanSlice(toolSpans, now)
	endSpanSlice(taskSpans, now)
	endSpanSlice(runSpans, now)
	if tp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = tp.ForceFlush(ctx)
		cancel()
	}
}

func (s *otelExporterState) endRelatedSpans(envelope *pb.EventEnvelope, ts time.Time) {
	if s == nil || envelope == nil {
		return
	}
	runKey := otelRunKey(envelope)
	taskKey := otelTaskKey(envelope, runKey)
	toolKey := otelToolKey(envelope, taskKey, runKey)

	s.mu.Lock()
	var spans []*activeOTelSpan
	if toolKey != "" {
		if span := s.toolSpans[toolKey]; span != nil {
			spans = append(spans, span)
			delete(s.toolSpans, toolKey)
		}
	}
	if taskKey != "" && !hasActiveToolForTask(s.toolSpans, taskKey) {
		if span := s.taskSpans[taskKey]; span != nil {
			spans = append(spans, span)
			delete(s.taskSpans, taskKey)
		}
	}
	if runKey != "" && !hasActiveTaskForRun(s.taskSpans, runKey) && !hasActiveToolForRun(s.toolSpans, runKey) {
		if span := s.runSpans[runKey]; span != nil {
			spans = append(spans, span)
			delete(s.runSpans, runKey)
		}
	}
	tp := s.tp
	s.mu.Unlock()

	endSpanSlice(spans, ts)
	if tp != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = tp.ForceFlush(ctx)
		cancel()
	}
}

func buildOTelTracerProvider(endpoint, serviceName string, headers map[string]string, owner *otelExporterState) (*sdktrace.TracerProvider, oteltrace.Tracer, error) {
	exporter, err := buildOTLPHTTPExporter(endpoint, headers, owner)
	if err != nil {
		return nil, nil, err
	}
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("agent.component", "agent-ebpf-filter"),
		),
	)
	if err != nil {
		return nil, nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(2*time.Second),
			sdktrace.WithMaxExportBatchSize(256),
		),
	)
	return provider, provider.Tracer("agent-ebpf-filter/otel"), nil
}

func buildOTLPHTTPExporter(endpoint string, headers map[string]string, owner *otelExporterState) (sdktrace.SpanExporter, error) {
	opts, err := buildOTLPHTTPOptions(endpoint, headers)
	if err != nil {
		return nil, err
	}
	exporter, err := otlptracehttp.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &monitoringSpanExporter{inner: exporter, owner: owner}, nil
}

func buildOTLPHTTPOptions(endpoint string, headers map[string]string) ([]otlptracehttp.Option, error) {
	trimmedEndpoint := strings.TrimSpace(endpoint)
	if trimmedEndpoint == "" {
		return nil, fmt.Errorf("empty OTLP endpoint")
	}
	opts := []otlptracehttp.Option{
		otlptracehttp.WithHeaders(cloneStringMap(headers)),
		otlptracehttp.WithTimeout(5 * time.Second),
	}
	if !strings.Contains(trimmedEndpoint, "://") {
		opts = append(opts, otlptracehttp.WithEndpoint(trimmedEndpoint))
		return opts, nil
	}
	parsed, err := url.Parse(trimmedEndpoint)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid OTLP endpoint %q", endpoint)
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" || path == "/" {
		path = "/v1/traces"
	}
	opts = append(opts, otlptracehttp.WithEndpoint(parsed.Host), otlptracehttp.WithURLPath(path))
	switch parsed.Scheme {
	case "http":
		opts = append(opts, otlptracehttp.WithInsecure())
	case "https":
	default:
		return nil, fmt.Errorf("unsupported OTLP scheme %q", parsed.Scheme)
	}
	return opts, nil
}

func buildOTelAttributes(envelope *pb.EventEnvelope) []attribute.KeyValue {
	if envelope == nil {
		return nil
	}
	attrs := []attribute.KeyValue{
		attribute.String("agent.schema_version", envelope.GetSchemaVersion()),
		attribute.String("agent.event_id", envelope.GetEventId()),
		attribute.String("agent.source", envelope.GetSource()),
		attribute.String("agent.event_type", envelope.GetEventType().String()),
	}
	appendStringAttr := func(key, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			attrs = append(attrs, attribute.String(key, trimmed))
		}
	}
	appendIntAttr := func(key string, value uint64) {
		if value != 0 {
			attrs = append(attrs, attribute.Int64(key, int64(value)))
		}
	}
	appendStringAttr("agent.run_id", envelope.GetAgentRunId())
	appendStringAttr("agent.task_id", envelope.GetTaskId())
	appendStringAttr("agent.conversation_id", envelope.GetConversationId())
	appendStringAttr("agent.turn_id", envelope.GetTurnId())
	appendStringAttr("agent.tool_call_id", envelope.GetToolCallId())
	appendStringAttr("agent.tool_name", envelope.GetToolName())
	appendStringAttr("agent.trace_id", envelope.GetTraceId())
	appendStringAttr("agent.span_id", envelope.GetSpanId())
	appendStringAttr("agent.cwd", envelope.GetCwd())
	appendStringAttr("agent.container_id", envelope.GetContainerId())
	appendStringAttr("agent.argv_digest", envelope.GetArgvDigest())
	appendStringAttr("agent.policy_decision", envelope.GetPolicyDecision())
	appendIntAttr("process.pid", uint64(envelope.GetPid()))
	appendIntAttr("process.parent_pid", uint64(envelope.GetPpid()))
	appendIntAttr("process.tgid", uint64(envelope.GetTgid()))
	appendIntAttr("process.uid", uint64(envelope.GetUid()))
	appendIntAttr("process.gid", uint64(envelope.GetGid()))
	appendIntAttr("process.cgroup_id", envelope.GetCgroupId())
	appendStringAttr("process.command", envelope.GetComm())
	if envelope.GetRiskScore() > 0 {
		attrs = append(attrs, attribute.Float64("agent.risk_score", envelope.GetRiskScore()))
	}
	if legacy := envelope.GetLegacyEvent(); legacy != nil {
		if legacy.GetRetval() != 0 {
			attrs = append(attrs, attribute.Int64("process.retval", legacy.GetRetval()))
		}
		if legacy.GetDurationNs() > 0 {
			attrs = append(attrs, attribute.Int64("process.duration_ns", int64(legacy.GetDurationNs())))
		}
	}
	switch payload := envelope.GetPayload().(type) {
	case *pb.EventEnvelope_ExecEvent:
		appendStringAttr("process.executable.path", payload.ExecEvent.GetPath())
	case *pb.EventEnvelope_FileEvent:
		appendStringAttr("file.operation", payload.FileEvent.GetOperation())
		appendStringAttr("file.path", payload.FileEvent.GetPath())
		appendStringAttr("file.extra_path", payload.FileEvent.GetExtraPath())
		if payload.FileEvent.GetBytes() > 0 {
			attrs = append(attrs, attribute.Int64("file.bytes", int64(payload.FileEvent.GetBytes())))
		}
	case *pb.EventEnvelope_NetworkEvent:
		appendStringAttr("network.endpoint", payload.NetworkEvent.GetEndpoint())
		appendStringAttr("network.direction", payload.NetworkEvent.GetDirection())
		appendStringAttr("network.domain", payload.NetworkEvent.GetDomain())
		appendStringAttr("network.family", payload.NetworkEvent.GetFamily())
		appendStringAttr("network.sock_type", payload.NetworkEvent.GetSockType())
		if payload.NetworkEvent.GetBytes() > 0 {
			attrs = append(attrs, attribute.Int64("network.bytes", int64(payload.NetworkEvent.GetBytes())))
		}
	case *pb.EventEnvelope_ProcessEvent:
		appendStringAttr("process.phase", payload.ProcessEvent.GetPhase())
		appendIntAttr("process.child_pid", uint64(payload.ProcessEvent.GetChildPid()))
		appendIntAttr("process.target_pid", uint64(payload.ProcessEvent.GetTargetPid()))
		if payload.ProcessEvent.GetExitStatus() != 0 {
			attrs = append(attrs, attribute.Int("process.exit_status", int(payload.ProcessEvent.GetExitStatus())))
		}
	case *pb.EventEnvelope_PolicyEvent:
		appendStringAttr("policy.reason", payload.PolicyEvent.GetReason())
		appendStringAttr("policy.related_path", payload.PolicyEvent.GetRelatedPath())
		appendStringAttr("policy.related_endpoint", payload.PolicyEvent.GetRelatedEndpoint())
	case *pb.EventEnvelope_WrapperEvent:
		appendStringAttr("wrapper.command_line", payload.WrapperEvent.GetCommandLine())
	case *pb.EventEnvelope_HookEvent:
		appendStringAttr("hook.name", payload.HookEvent.GetHookName())
		appendStringAttr("hook.target_path", payload.HookEvent.GetTargetPath())
	case *pb.EventEnvelope_McpEvent:
		appendStringAttr("mcp.tool_name", payload.McpEvent.GetToolName())
		appendStringAttr("mcp.server_name", payload.McpEvent.GetServerName())
		appendStringAttr("mcp.endpoint", payload.McpEvent.GetEndpoint())
		appendStringAttr("mcp.request_id", payload.McpEvent.GetRequestId())
	}
	return attrs
}

func buildHierarchyAttributes(envelope *pb.EventEnvelope, level string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("agent.hierarchy_level", level),
	}
	switch level {
	case "run":
		if value := firstNonEmpty(envelope.GetAgentRunId(), otelRunKey(envelope)); value != "" {
			attrs = append(attrs, attribute.String("agent.run_id", value))
		}
	case "task":
		if value := firstNonEmpty(envelope.GetTaskId(), otelTaskKey(envelope, otelRunKey(envelope))); value != "" {
			attrs = append(attrs, attribute.String("agent.task_id", value))
		}
	case "tool":
		if value := firstNonEmpty(envelope.GetToolCallId(), otelToolKey(envelope, otelTaskKey(envelope, otelRunKey(envelope)), otelRunKey(envelope))); value != "" {
			attrs = append(attrs, attribute.String("agent.tool_call_id", value))
		}
		if toolName := strings.TrimSpace(envelope.GetToolName()); toolName != "" {
			attrs = append(attrs, attribute.String("agent.tool_name", toolName))
		}
	}
	if traceID := strings.TrimSpace(envelope.GetTraceId()); traceID != "" {
		attrs = append(attrs, attribute.String("agent.trace_id", traceID))
	}
	return attrs
}

func otelEventName(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return "event.unknown"
	}
	switch envelope.GetPayload().(type) {
	case *pb.EventEnvelope_ExecEvent:
		return "process.exec"
	case *pb.EventEnvelope_FileEvent:
		file := envelope.GetFileEvent()
		operation := strings.TrimSpace(file.GetOperation())
		if operation == "" {
			operation = "unknown"
		}
		return "file." + operation
	case *pb.EventEnvelope_NetworkEvent:
		network := envelope.GetNetworkEvent()
		direction := strings.TrimSpace(network.GetDirection())
		switch {
		case strings.EqualFold(direction, "connect"), strings.Contains(strings.ToLower(network.GetEndpoint()), ":"):
			return "network.connect"
		case strings.EqualFold(direction, "send"):
			return "network.send"
		case strings.EqualFold(direction, "recv"):
			return "network.recv"
		default:
			return "network.flow"
		}
	case *pb.EventEnvelope_ProcessEvent:
		phase := strings.TrimSpace(envelope.GetProcessEvent().GetPhase())
		if phase == "" {
			phase = "unknown"
		}
		return "process." + phase
	case *pb.EventEnvelope_PolicyEvent:
		return "policy.decision"
	case *pb.EventEnvelope_WrapperEvent:
		return "tool.call"
	case *pb.EventEnvelope_HookEvent:
		return "hook.callback"
	case *pb.EventEnvelope_McpEvent:
		return "mcp.call"
	default:
		return "event." + strings.ToLower(strings.TrimSpace(envelope.GetSource()))
	}
}

func otelToolSpanName(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return "tool.call"
	}
	if _, ok := envelope.GetPayload().(*pb.EventEnvelope_McpEvent); ok {
		return "mcp.call"
	}
	toolName := strings.ToLower(strings.TrimSpace(envelope.GetToolName()))
	switch {
	case strings.Contains(toolName, "llm"), strings.Contains(toolName, "chat"), strings.Contains(toolName, "completion"):
		return "llm.call"
	case strings.Contains(toolName, "review"), strings.Contains(strings.ToLower(strings.TrimSpace(envelope.GetTaskId())), "review"):
		return "pr.review"
	default:
		return "tool.call"
	}
}

func shouldMarkSpanError(envelope *pb.EventEnvelope) bool {
	if envelope == nil {
		return false
	}
	if decision := strings.ToUpper(strings.TrimSpace(envelope.GetPolicyDecision())); decision == "BLOCK" || decision == "ALERT" {
		return true
	}
	if legacy := envelope.GetLegacyEvent(); legacy != nil && legacy.GetRetval() < 0 {
		return true
	}
	return false
}

func otelStatusMessage(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return ""
	}
	if decision := strings.TrimSpace(envelope.GetPolicyDecision()); decision != "" {
		return "policy decision: " + decision
	}
	if legacy := envelope.GetLegacyEvent(); legacy != nil && legacy.GetRetval() < 0 {
		return fmt.Sprintf("retval=%d", legacy.GetRetval())
	}
	return "runtime alert"
}

func shouldEndOTelHierarchy(envelope *pb.EventEnvelope) bool {
	if envelope == nil {
		return false
	}
	process := envelope.GetProcessEvent()
	if process == nil {
		return false
	}
	return strings.EqualFold(process.GetPhase(), "exit")
}

func otelRunKey(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return ""
	}
	parts := []string{}
	if runID := strings.TrimSpace(envelope.GetAgentRunId()); runID != "" {
		parts = append(parts, "run:"+runID)
	}
	if rootPID := envelope.GetPid(); envelope.GetLegacyEvent() != nil && envelope.GetLegacyEvent().GetRootAgentPid() != 0 {
		rootPID = envelope.GetLegacyEvent().GetRootAgentPid()
		if rootPID != 0 {
			parts = append(parts, fmt.Sprintf("root:%d", rootPID))
		}
	}
	if traceID := strings.TrimSpace(envelope.GetTraceId()); traceID != "" {
		parts = append(parts, "trace:"+traceID)
	}
	if len(parts) == 0 {
		if envelope.GetPid() == 0 {
			return ""
		}
		parts = append(parts, fmt.Sprintf("pid:%d", envelope.GetPid()))
	}
	return stableOTelKey(parts...)
}

func otelTaskKey(envelope *pb.EventEnvelope, runKey string) string {
	if envelope == nil {
		return ""
	}
	parts := []string{runKey}
	if taskID := strings.TrimSpace(envelope.GetTaskId()); taskID != "" {
		parts = append(parts, "task:"+taskID)
	} else if conversationID := strings.TrimSpace(envelope.GetConversationId()); conversationID != "" || strings.TrimSpace(envelope.GetTurnId()) != "" {
		parts = append(parts, "conversation:"+conversationID, "turn:"+strings.TrimSpace(envelope.GetTurnId()))
	} else {
		return ""
	}
	return stableOTelKey(parts...)
}

func otelToolKey(envelope *pb.EventEnvelope, taskKey, runKey string) string {
	if envelope == nil {
		return ""
	}
	parts := []string{runKey, taskKey}
	if toolCallID := strings.TrimSpace(envelope.GetToolCallId()); toolCallID != "" {
		parts = append(parts, "tool_call:"+toolCallID)
	} else if toolName := strings.TrimSpace(envelope.GetToolName()); toolName != "" {
		parts = append(parts, "tool_name:"+toolName)
		if envelope.GetPid() != 0 {
			parts = append(parts, fmt.Sprintf("pid:%d", envelope.GetPid()))
		}
	} else {
		return ""
	}
	return stableOTelKey(parts...)
}

func stableOTelKey(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(filtered, "\x00")))
	return "otel_" + hex.EncodeToString(sum[:10])
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if trimmedKey := strings.TrimSpace(key); trimmedKey != "" {
			out[trimmedKey] = strings.TrimSpace(input[key])
		}
	}
	return out
}

func collectIdleSpans(spans map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(spans) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range spans {
		if span == nil || now.Sub(span.lastSeen) < idleFor {
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectIdleTaskSpans(tasks, tools map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range tasks {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasActiveToolForTask(tools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectIdleRunSpans(runs, tasks, tools map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(runs) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range runs {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasActiveTaskForRun(tasks, span.key) || hasActiveToolForRun(tools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
}

func hasActiveToolForTask(tools map[string]*activeOTelSpan, taskKey string) bool {
	for _, span := range tools {
		if span != nil && span.taskKey == taskKey {
			return true
		}
	}
	return false
}

func hasActiveTaskForRun(tasks map[string]*activeOTelSpan, runKey string) bool {
	for _, span := range tasks {
		if span != nil && span.runKey == runKey {
			return true
		}
	}
	return false
}

func hasActiveToolForRun(tools map[string]*activeOTelSpan, runKey string) bool {
	for _, span := range tools {
		if span != nil && span.runKey == runKey {
			return true
		}
	}
	return false
}

func endSpanMap(spans map[string]*activeOTelSpan, ts time.Time) {
	if len(spans) == 0 {
		return
	}
	list := make([]*activeOTelSpan, 0, len(spans))
	for _, span := range spans {
		if span != nil {
			list = append(list, span)
		}
	}
	endSpanSlice(list, ts)
}

func endSpanSlice(spans []*activeOTelSpan, ts time.Time) {
	for _, span := range spans {
		if span == nil || span.span == nil {
			continue
		}
		span.span.End(oteltrace.WithTimestamp(ts))
	}
}

func handleOTelHealth(c *gin.Context) {
	c.JSON(http.StatusOK, otelExporterStore.Snapshot())
}
