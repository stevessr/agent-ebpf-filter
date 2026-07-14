package app

import (
	"agent-ebpf-filter/app/platform"
	"container/list"
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// ---- moved from backend/zz_merged_backend.go section export_otel.go ----

type OTelHealthResponse struct {
	Enabled          bool   `json:"enabled"`
	Ready            bool   `json:"ready"`
	Endpoint         string `json:"endpoint"`
	ServiceName      string `json:"serviceName"`
	QueueLen         int    `json:"queueLen"`
	QueueCap         int    `json:"queueCap"`
	EnqueuedEvents   uint64 `json:"enqueuedEvents"`
	ProcessedEvents  uint64 `json:"processedEvents"`
	ActiveRunSpans   int    `json:"activeRunSpans"`
	ActiveTaskSpans  int    `json:"activeTaskSpans"`
	ActiveToolSpans  int    `json:"activeToolSpans"`
	MaxRunSpans      int    `json:"maxRunSpans"`
	MaxTaskSpans     int    `json:"maxTaskSpans"`
	MaxToolSpans     int    `json:"maxToolSpans"`
	EvictedRunSpans  uint64 `json:"evictedRunSpans"`
	EvictedTaskSpans uint64 `json:"evictedTaskSpans"`
	EvictedToolSpans uint64 `json:"evictedToolSpans"`
	ExportedSpans    uint64 `json:"exportedSpans"`
	DroppedEvents    uint64 `json:"droppedEvents"`
	LastExportedAt   string `json:"lastExportedAt,omitempty"`
	LastError        string `json:"lastError,omitempty"`
}

type activeOTelSpan struct {
	ctx      context.Context
	span     oteltrace.Span
	key      string
	runKey   string
	taskKey  string
	lastSeen time.Time
	lruEntry *list.Element
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
	lifecycleMu sync.Mutex
	enqueueMu   sync.RWMutex
	processMu   sync.Mutex
	mu          sync.RWMutex
	queue       chan CapturedEventRecord
	stopCh      chan struct{}
	closeOnce   sync.Once
	workers     sync.WaitGroup
	closed      atomic.Bool
	accepting   atomic.Bool
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
	runLRU    *list.List
	taskLRU   *list.List
	toolLRU   *list.List
	runTasks  map[string]map[string]struct{}
	runTools  map[string]map[string]struct{}
	taskTools map[string]map[string]struct{}

	maxRunSpans      int
	maxTaskSpans     int
	maxToolSpans     int
	evictedRunSpans  uint64
	evictedTaskSpans uint64
	evictedToolSpans uint64

	enqueuedEvents  atomic.Uint64
	processedEvents atomic.Uint64
	exportedSpans   atomic.Uint64
	droppedEvents   atomic.Uint64
	lastExportAt    atomic.Int64
}

func newOTelExporterState() *otelExporterState {
	state := &otelExporterState{
		queue:        make(chan CapturedEventRecord, otelExporterQueueSize),
		stopCh:       make(chan struct{}),
		headers:      make(map[string]string),
		maxRunSpans:  otelMaxActiveRunSpans,
		maxTaskSpans: otelMaxActiveTaskSpans,
		maxToolSpans: otelMaxActiveToolSpans,
	}
	state.resetActiveSpansLocked()
	state.workers.Add(2)
	go func() {
		defer state.workers.Done()
		state.run()
	}()
	go func() {
		defer state.workers.Done()
		state.sweepLoop()
	}()
	return state
}

var otelExporterStore = newOTelExporterState()

func (s *otelExporterState) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.closed.Store(true)
		s.accepting.Store(false)
		s.lifecycleMu.Lock()
		defer s.lifecycleMu.Unlock()
		// Record holds a read lock through its non-blocking enqueue. Taking the
		// write lock here linearizes shutdown after every already accepted event,
		// so run can drain the queue without a late producer racing its empty check.
		s.enqueueMu.Lock()
		close(s.stopCh)
		s.enqueueMu.Unlock()
		s.workers.Wait()
		s.disable()
	})
}

func (s *otelExporterState) run() {
	for {
		select {
		case <-s.stopCh:
			for {
				select {
				case record := <-s.queue:
					s.processQueuedRecord(record)
				default:
					return
				}
			}
		case record := <-s.queue:
			s.processQueuedRecord(record)
		}
	}
}

func (s *otelExporterState) processQueuedRecord(record CapturedEventRecord) {
	s.handleRecord(record)
	s.processedEvents.Add(1)
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
	s.exportedSpans.Add(uint64(count))
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
		Enabled:          s.enabled,
		Ready:            s.ready,
		Endpoint:         s.endpoint,
		ServiceName:      s.serviceName,
		QueueLen:         len(s.queue),
		QueueCap:         cap(s.queue),
		EnqueuedEvents:   s.enqueuedEvents.Load(),
		ProcessedEvents:  s.processedEvents.Load(),
		ActiveRunSpans:   len(s.runSpans),
		ActiveTaskSpans:  len(s.taskSpans),
		ActiveToolSpans:  len(s.toolSpans),
		MaxRunSpans:      s.maxRunSpans,
		MaxTaskSpans:     s.maxTaskSpans,
		MaxToolSpans:     s.maxToolSpans,
		EvictedRunSpans:  s.evictedRunSpans,
		EvictedTaskSpans: s.evictedTaskSpans,
		EvictedToolSpans: s.evictedToolSpans,
		ExportedSpans:    s.exportedSpans.Load(),
		DroppedEvents:    s.droppedEvents.Load(),
		LastError:        s.lastError,
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
	s.lifecycleMu.Lock()
	defer s.lifecycleMu.Unlock()
	if s.closed.Load() {
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
		s.accepting.Store(false)
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
		s.accepting.Store(false)
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

	s.accepting.Store(false)
	s.enqueueMu.Lock()
	s.processMu.Lock()
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
	s.resetActiveSpansLocked()
	s.mu.Unlock()
	s.processMu.Unlock()
	s.enqueueMu.Unlock()
	if !s.closed.Load() {
		s.accepting.Store(true)
	}

	endSpanMap(oldToolSpans, time.Now().UTC())
	endSpanMap(oldTaskSpans, time.Now().UTC())
	endSpanMap(oldRunSpans, time.Now().UTC())
	s.shutdownProvider(oldProvider)
}

func (s *otelExporterState) disable() {
	if s == nil {
		return
	}
	s.accepting.Store(false)
	s.enqueueMu.Lock()
	s.processMu.Lock()
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
	s.resetActiveSpansLocked()
	s.mu.Unlock()
	s.processMu.Unlock()
	s.enqueueMu.Unlock()

	endSpanMap(oldToolSpans, time.Now().UTC())
	endSpanMap(oldTaskSpans, time.Now().UTC())
	endSpanMap(oldRunSpans, time.Now().UTC())
	s.shutdownProvider(oldProvider)
}

func (s *otelExporterState) disableProviderOnly() {
	if s == nil {
		return
	}
	s.accepting.Store(false)
	s.enqueueMu.Lock()
	s.processMu.Lock()
	s.mu.Lock()
	oldProvider := s.tp
	oldRunSpans := s.runSpans
	oldTaskSpans := s.taskSpans
	oldToolSpans := s.toolSpans
	s.tp = nil
	s.tracer = nil
	s.resetActiveSpansLocked()
	s.mu.Unlock()
	s.processMu.Unlock()
	s.enqueueMu.Unlock()

	endSpanMap(oldToolSpans, time.Now().UTC())
	endSpanMap(oldTaskSpans, time.Now().UTC())
	endSpanMap(oldRunSpans, time.Now().UTC())
	s.shutdownProvider(oldProvider)
}

func (s *otelExporterState) shutdownProvider(provider *sdktrace.TracerProvider) {
	if provider == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := provider.Shutdown(ctx); err != nil {
		s.noteExportFailure(err)
	}
}

func (s *otelExporterState) Record(record CapturedEventRecord) {
	if s == nil || record.Event == nil || s.closed.Load() || !s.accepting.Load() {
		return
	}
	s.enqueueMu.RLock()
	defer s.enqueueMu.RUnlock()
	if s.closed.Load() || !s.accepting.Load() {
		return
	}
	select {
	case s.queue <- record:
		s.enqueuedEvents.Add(1)
	default:
		s.droppedEvents.Add(1)
	}
}

func (s *otelExporterState) handleRecord(record CapturedEventRecord) {
	if s == nil {
		return
	}
	s.processMu.Lock()
	defer s.processMu.Unlock()
	if record.Envelope == nil {
		record = normalizeCapturedEventRecord(record)
	}
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
