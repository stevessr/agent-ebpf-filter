package main

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"agent-ebpf-filter/pb"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
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
