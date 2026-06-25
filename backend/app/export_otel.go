package app

import (
	"agent-ebpf-filter/app/platform"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ---- moved from backend/zz_merged_backend.go section export_otel.go ----

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
	serviceName := platform.FirstNonEmpty(settings.OtlpServiceName, "agent-ebpf-filter")
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
