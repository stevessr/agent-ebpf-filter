package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/app/platform"

	"github.com/gin-gonic/gin"
)

const (
	researchProcessingDefaultMaxEvents             = 5000
	researchProcessingDefaultQueueSize             = 2048
	researchProcessingDefaultTimelineBucketSeconds = 60
	researchProcessingDefaultTopK                  = 20
	researchProcessingDefaultRecentSamples         = 25
	researchProcessingDefaultArtifactRetentionDays = 14
	researchProcessingDefaultMaxSessionEvents      = 50000
	researchProcessingDefaultExportFormats         = "jsonl,csv,bundle"
)

type researchProcessingWorkKind string

const (
	researchProcessingWorkEvent researchProcessingWorkKind = "event"
	researchProcessingWorkScan  researchProcessingWorkKind = "scan"
	researchProcessingWorkReset researchProcessingWorkKind = "reset"
)

type researchProcessingWorkItem struct {
	kind     researchProcessingWorkKind
	record   CapturedEventRecord
	records  []CapturedEventRecord
	force    bool
	queuedAt time.Time
}

type researchEventSample struct {
	ID        string  `json:"id"`
	Timestamp int64   `json:"timestamp"`
	Time      string  `json:"time"`
	Source    string  `json:"source"`
	EventType string  `json:"eventType"`
	PID       uint32  `json:"pid,omitempty"`
	PPID      uint32  `json:"ppid,omitempty"`
	Comm      string  `json:"comm,omitempty"`
	TraceID   string  `json:"traceId,omitempty"`
	SpanID    string  `json:"spanId,omitempty"`
	Title     string  `json:"title"`
	Target    string  `json:"target,omitempty"`
	RiskScore float64 `json:"riskScore,omitempty"`
	Decision  string  `json:"decision,omitempty"`
}

type researchCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

type researchTimelineBucket struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Time  string `json:"time"`
	Count int    `json:"count"`
}

type researchProcessSummary struct {
	PID        uint32   `json:"pid"`
	PPID       uint32   `json:"ppid,omitempty"`
	Comm       string   `json:"comm"`
	EventCount int      `json:"eventCount"`
	FirstSeen  int64    `json:"firstSeen,omitempty"`
	LastSeen   int64    `json:"lastSeen,omitempty"`
	Sources    []string `json:"sources,omitempty"`
	EventTypes []string `json:"eventTypes,omitempty"`
	ChildPids  []uint32 `json:"childPids,omitempty"`
}

type researchTraceSummary struct {
	TraceID    string   `json:"traceId"`
	EventCount int      `json:"eventCount"`
	FirstSeen  int64    `json:"firstSeen,omitempty"`
	LastSeen   int64    `json:"lastSeen,omitempty"`
	Sources    []string `json:"sources,omitempty"`
	Comms      []string `json:"comms,omitempty"`
}

type researchProcessingSummary struct {
	Total              int                      `json:"total"`
	EarliestTimestamp  int64                    `json:"earliestTimestamp,omitempty"`
	LatestTimestamp    int64                    `json:"latestTimestamp,omitempty"`
	EarliestTime       string                   `json:"earliestTime,omitempty"`
	LatestTime         string                   `json:"latestTime,omitempty"`
	BySource           []researchCount          `json:"bySource"`
	ByType             []researchCount          `json:"byType"`
	ByComm             []researchCount          `json:"byComm"`
	ByPID              []researchCount          `json:"byPid"`
	ByTrace            []researchCount          `json:"byTrace"`
	Timeline           []researchTimelineBucket `json:"timeline"`
	TopProcesses       []researchProcessSummary `json:"topProcesses"`
	TopTraces          []researchTraceSummary   `json:"topTraces"`
	RecentSamples      []researchEventSample    `json:"recentSamples"`
	GeneratedTimestamp int64                    `json:"generatedTimestamp"`
	GeneratedTime      string                   `json:"generatedTime"`
}

type researchProcessingStatus struct {
	Enabled               bool                       `json:"enabled"`
	Settings              ResearchProcessingSettings `json:"settings"`
	QueueLen              int                        `json:"queueLen"`
	QueueCap              int                        `json:"queueCap"`
	EnqueuedTotal         uint64                     `json:"enqueuedTotal,omitempty"`
	ConsumedTotal         uint64                     `json:"consumedTotal"`
	DroppedTotal          uint64                     `json:"droppedTotal"`
	BufferedTotal         int                        `json:"bufferedTotal"`
	BufferEvictedTotal    uint64                     `json:"bufferEvictedTotal"`
	LastError             string                     `json:"lastError,omitempty"`
	LastDropReason        string                     `json:"lastDropReason,omitempty"`
	LastEnqueuedAt        *time.Time                 `json:"lastEnqueuedAt,omitempty"`
	LastProcessedAt       *time.Time                 `json:"lastProcessedAt,omitempty"`
	LastDroppedAt         *time.Time                 `json:"lastDroppedAt,omitempty"`
	LastSummaryAt         *time.Time                 `json:"lastSummaryAt,omitempty"`
	PendingSummary        bool                       `json:"pendingSummary"`
	SummaryRebuilds       uint64                     `json:"summaryRebuilds,omitempty"`
	LastSummaryDurationMs float64                    `json:"lastSummaryDurationMs,omitempty"`
	LastWorkLatencyMs     float64                    `json:"lastWorkLatencyMs,omitempty"`
	ThroughputPerSecond   float64                    `json:"throughputPerSecond,omitempty"`
	UpdatedAt             time.Time                  `json:"updatedAt"`
	Summary               researchProcessingSummary  `json:"summary"`
}

type researchProcessingTaskRequest struct {
	Action string `json:"action"`
	Limit  int    `json:"limit"`
}

type researchProcessingTaskResponse struct {
	Status   string `json:"status"`
	Action   string `json:"action"`
	Records  int    `json:"records,omitempty"`
	QueueLen int    `json:"queueLen"`
}

type researchProcessingWorker struct {
	lifecycleMu         sync.Mutex
	mu                  sync.RWMutex
	queue               chan researchProcessingWorkItem
	cancel              context.CancelFunc
	done                chan struct{}
	started             bool
	startedAt           time.Time
	events              researchEventRing
	eventsVersion       uint64
	summary             researchProcessingSummary
	summaryDirty        bool
	summarySettings     researchProcessingSummarySettings
	summaryRebuilds     uint64
	lastSummaryDuration time.Duration
	lastSummaryAt       time.Time
	enqueuedTotal       uint64
	consumedTotal       uint64
	droppedTotal        uint64
	bufferEvictedTotal  uint64
	lastError           string
	lastDropReason      string
	lastEnqueuedAt      time.Time
	lastProcessedAt     time.Time
	lastDroppedAt       time.Time
	lastWorkLatency     time.Duration
	updatedAt           time.Time
}

var researchProcessingWorkerStore = newResearchProcessingWorker()

type researchProcessingSummarySettings struct {
	TimelineBucketSeconds int
	TopK                  int
	RecentSamples         int
}

// researchEventRing retains samples in insertion order while
// overwriting the oldest slot in O(1) once full. It is guarded by the owning
// researchProcessingWorker.mu.
type researchEventRing struct {
	items []researchEventSample
	start int
	limit int
}

func (r *researchEventRing) Len() int {
	if r == nil {
		return 0
	}
	return len(r.items)
}

func (r *researchEventRing) Reset() {
	if r == nil {
		return
	}
	clear(r.items)
	r.items = r.items[:0]
	r.start = 0
}

func (r *researchEventRing) Append(samples []researchEventSample, limit int) int {
	if r == nil || len(samples) == 0 {
		return 0
	}
	if limit <= 0 {
		limit = researchProcessingDefaultMaxEvents
	}
	evicted := r.resize(limit)
	if overflow := len(r.items) + len(samples) - limit; overflow > 0 {
		evicted += overflow
	}
	if len(samples) >= limit {
		if cap(r.items) < limit {
			r.items = make([]researchEventSample, limit)
		} else {
			clear(r.items)
			r.items = r.items[:limit]
		}
		copy(r.items, samples[len(samples)-limit:])
		r.start = 0
		return evicted
	}
	needed := len(r.items) + len(samples)
	if needed > limit {
		needed = limit
	}
	r.grow(needed, limit)
	for _, sample := range samples {
		if len(r.items) < limit {
			r.items = append(r.items, sample)
			continue
		}
		r.items[r.start] = sample
		r.start++
		if r.start == limit {
			r.start = 0
		}
	}
	return evicted
}

func (r *researchEventRing) Snapshot() []researchEventSample {
	if r == nil || len(r.items) == 0 {
		return nil
	}
	out := make([]researchEventSample, len(r.items))
	if r.start == 0 {
		copy(out, r.items)
		return out
	}
	copied := copy(out, r.items[r.start:])
	copy(out[copied:], r.items[:r.start])
	return out
}

func (r *researchEventRing) resize(limit int) int {
	if r.limit == limit {
		return 0
	}
	ordered := r.Snapshot()
	evicted := 0
	if len(ordered) > limit {
		evicted = len(ordered) - limit
		ordered = ordered[len(ordered)-limit:]
	}
	r.items = nil
	if len(ordered) > 0 {
		capacity := len(ordered)
		if capacity < 16 {
			capacity = min(16, limit)
		}
		r.items = make([]researchEventSample, len(ordered), capacity)
		copy(r.items, ordered)
	}
	r.start = 0
	r.limit = limit
	return evicted
}

func (r *researchEventRing) grow(needed, limit int) {
	if needed <= cap(r.items) {
		return
	}
	capacity := cap(r.items) * 2
	if capacity < 16 {
		capacity = 16
	}
	if capacity < needed {
		capacity = needed
	}
	if capacity > limit {
		capacity = limit
	}
	items := make([]researchEventSample, len(r.items), capacity)
	copy(items, r.items)
	r.items = items
}

func newResearchProcessingWorker() *researchProcessingWorker {
	now := time.Now().UTC()
	return &researchProcessingWorker{
		summary:   researchProcessingSummary{},
		startedAt: now,
		updatedAt: now,
	}
}

func startResearchProcessingWorker(ctx context.Context) {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	researchProcessingWorkerStore.Start(ctx, settings.QueueSize)
}

func (w *researchProcessingWorker) Start(ctx context.Context, queueSize int) {
	if w == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	queueSize = normalizeBackendWorkerQueueSize(queueSize, researchProcessingDefaultQueueSize)
	w.lifecycleMu.Lock()
	defer w.lifecycleMu.Unlock()
	w.mu.Lock()
	if w.started {
		w.mu.Unlock()
		return
	}
	workerCtx, cancel := context.WithCancel(ctx)
	w.queue = make(chan researchProcessingWorkItem, queueSize)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.started = true
	now := time.Now().UTC()
	if w.startedAt.IsZero() {
		w.startedAt = now
	}
	w.updatedAt = now
	queue := w.queue
	done := w.done
	w.mu.Unlock()
	go func() {
		w.run(workerCtx, queue)
		w.lifecycleMu.Lock()
		w.mu.Lock()
		if w.done == done {
			w.queue = nil
			w.cancel = nil
			w.done = nil
			w.started = false
			w.updatedAt = time.Now().UTC()
		}
		w.mu.Unlock()
		close(done)
		w.lifecycleMu.Unlock()
	}()
}

func (w *researchProcessingWorker) run(ctx context.Context, queue <-chan researchProcessingWorkItem) {
	for {
		var item researchProcessingWorkItem
		select {
		case <-ctx.Done():
			return
		case item = <-queue:
		}
		if ctx.Err() != nil {
			return
		}
		switch item.kind {
		case researchProcessingWorkReset:
			w.resetNow()
		case researchProcessingWorkScan:
			w.resetNow()
			w.processRecordsContext(ctx, item.records, item.force, item.queuedAt)
		case researchProcessingWorkEvent:
			w.processRecordsContext(ctx, []CapturedEventRecord{item.record}, item.force, item.queuedAt)
		}
	}
}

func (w *researchProcessingWorker) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.lifecycleMu.Lock()
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		w.lifecycleMu.Unlock()
		return nil
	}
	cancel := w.cancel
	done := w.done
	w.queue = nil
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
	w.lifecycleMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func queueResearchProcessingRecord(record CapturedEventRecord) {
	if record.Event == nil {
		return
	}
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if !settings.Enabled {
		researchProcessingWorkerStore.noteDrop("disabled", "research processing is disabled")
		return
	}
	researchProcessingWorkerStore.EnqueueEvent(record)
}

func (w *researchProcessingWorker) EnqueueEvent(record CapturedEventRecord) bool {
	return w.enqueue(researchProcessingWorkItem{kind: researchProcessingWorkEvent, record: record})
}

func (w *researchProcessingWorker) EnqueueScan(records []CapturedEventRecord) bool {
	return w.enqueue(researchProcessingWorkItem{kind: researchProcessingWorkScan, records: records, force: true})
}

func (w *researchProcessingWorker) EnqueueReset() bool {
	return w.enqueue(researchProcessingWorkItem{kind: researchProcessingWorkReset})
}

func (w *researchProcessingWorker) enqueue(item researchProcessingWorkItem) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	queue := w.queue
	if queue == nil {
		w.mu.RUnlock()
		w.noteDrop("worker_not_started", "research processing worker is not started")
		return false
	}
	if item.queuedAt.IsZero() {
		item.queuedAt = time.Now().UTC()
	}
	accepted := false
	select {
	case queue <- item:
		accepted = true
	default:
	}
	w.mu.RUnlock()
	if accepted {
		w.noteEnqueued(item.queuedAt)
		return true
	}
	w.noteDrop("queue_full", "research processing queue is full")
	return false
}

func (w *researchProcessingWorker) noteEnqueued(queuedAt time.Time) {
	if w == nil {
		return
	}
	if queuedAt.IsZero() {
		queuedAt = time.Now().UTC()
	}
	w.mu.Lock()
	w.enqueuedTotal++
	w.lastEnqueuedAt = queuedAt.UTC()
	w.updatedAt = queuedAt.UTC()
	w.mu.Unlock()
}

func (w *researchProcessingWorker) noteDrop(reason, message string) {
	if w == nil {
		return
	}
	now := time.Now().UTC()
	w.mu.Lock()
	w.droppedTotal++
	w.lastDropReason = reason
	w.lastError = message
	w.lastDroppedAt = now
	w.updatedAt = now
	w.mu.Unlock()
}

func (w *researchProcessingWorker) resetNow() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.events.Reset()
	w.eventsVersion++
	w.summary = researchProcessingSummary{}
	w.summaryDirty = false
	w.summarySettings = researchProcessingSummarySettings{}
	w.lastError = ""
	w.lastDropReason = ""
	now := time.Now().UTC()
	w.lastSummaryAt = time.Time{}
	w.updatedAt = now
	w.mu.Unlock()
}

func (w *researchProcessingWorker) processRecord(record CapturedEventRecord, force bool) {
	w.processRecords([]CapturedEventRecord{record}, force, time.Time{})
}

func (w *researchProcessingWorker) processRecords(records []CapturedEventRecord, force bool, queuedAt time.Time) {
	w.processRecordsContext(context.Background(), records, force, queuedAt)
}

func (w *researchProcessingWorker) processRecordsContext(ctx context.Context, records []CapturedEventRecord, force bool, queuedAt time.Time) {
	if w == nil || len(records) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if !force && !settings.Enabled {
		w.noteDrop("disabled", "research processing is disabled")
		return
	}
	maxEvents := settings.MaxEvents
	if maxEvents <= 0 {
		maxEvents = researchProcessingDefaultMaxEvents
	}
	for offset := 0; offset < len(records); offset += backendWorkerScanBatchSize {
		if ctx.Err() != nil {
			return
		}
		end := min(offset+backendWorkerScanBatchSize, len(records))
		samples := make([]researchEventSample, 0, end-offset)
		for _, record := range records[offset:end] {
			if record.Event == nil {
				continue
			}
			sample, ok := researchEventSampleFromRecord(record)
			if ok {
				samples = append(samples, sample)
			}
		}
		if len(samples) == 0 {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		now := time.Now().UTC()
		w.mu.Lock()
		w.consumedTotal += uint64(len(samples))
		w.bufferEvictedTotal += uint64(w.events.Append(samples, maxEvents))
		w.eventsVersion++
		w.summaryDirty = true
		w.lastProcessedAt = now
		if !queuedAt.IsZero() {
			if latency := now.Sub(queuedAt.UTC()); latency >= 0 {
				w.lastWorkLatency = latency
			}
		}
		w.updatedAt = now
		w.mu.Unlock()
	}
}

func (w *researchProcessingWorker) Status() researchProcessingStatus {
	settings := runtimeSettingsStore.Snapshot().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if w == nil {
		return researchProcessingStatus{Enabled: settings.Enabled, Settings: settings, UpdatedAt: time.Now().UTC()}
	}
	w.refreshSummaryIfNeeded(settings)
	w.mu.RLock()
	queueLen := 0
	queueCap := 0
	if w.queue != nil {
		queueLen = len(w.queue)
		queueCap = cap(w.queue)
	}
	lastEnqueuedAt := ptrTimeIfSet(w.lastEnqueuedAt)
	lastProcessedAt := ptrTimeIfSet(w.lastProcessedAt)
	lastDroppedAt := ptrTimeIfSet(w.lastDroppedAt)
	lastSummaryAt := ptrTimeIfSet(w.lastSummaryAt)
	status := researchProcessingStatus{
		Enabled:               settings.Enabled,
		Settings:              settings,
		QueueLen:              queueLen,
		QueueCap:              queueCap,
		EnqueuedTotal:         w.enqueuedTotal,
		ConsumedTotal:         w.consumedTotal,
		DroppedTotal:          w.droppedTotal,
		BufferedTotal:         w.events.Len(),
		BufferEvictedTotal:    w.bufferEvictedTotal,
		LastError:             w.lastError,
		LastDropReason:        w.lastDropReason,
		LastEnqueuedAt:        lastEnqueuedAt,
		LastProcessedAt:       lastProcessedAt,
		LastDroppedAt:         lastDroppedAt,
		LastSummaryAt:         lastSummaryAt,
		PendingSummary:        w.summaryDirty,
		SummaryRebuilds:       w.summaryRebuilds,
		LastSummaryDurationMs: durationMilliseconds(w.lastSummaryDuration),
		LastWorkLatencyMs:     durationMilliseconds(w.lastWorkLatency),
		ThroughputPerSecond:   throughputPerSecond(w.consumedTotal, w.startedAt, time.Now().UTC()),
		UpdatedAt:             w.updatedAt,
		Summary:               cloneResearchProcessingSummary(w.summary),
	}
	w.mu.RUnlock()
	return status
}

func (w *researchProcessingWorker) refreshSummaryIfNeeded(settings ResearchProcessingSettings) {
	if w == nil {
		return
	}
	key := researchProcessingSummarySettingsFrom(settings)
	for attempt := 0; attempt < 2; attempt++ {
		w.mu.RLock()
		needsRefresh := w.summaryDirty || w.summarySettings != key
		if !needsRefresh {
			w.mu.RUnlock()
			return
		}
		events := w.events.Snapshot()
		version := w.eventsVersion
		w.mu.RUnlock()

		started := time.Now()
		summary := buildResearchProcessingSummary(events, settings)
		duration := time.Since(started)
		now := time.Now().UTC()

		w.mu.Lock()
		if w.eventsVersion == version {
			w.summary = summary
			w.summarySettings = key
			w.summaryDirty = false
			w.summaryRebuilds++
			w.lastSummaryDuration = duration
			w.lastSummaryAt = now
			w.updatedAt = now
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
	}

	w.mu.Lock()
	if w.summaryDirty || w.summarySettings != key {
		started := time.Now()
		w.summary = buildResearchProcessingSummary(w.events.Snapshot(), settings)
		w.summarySettings = key
		w.summaryDirty = false
		w.summaryRebuilds++
		w.lastSummaryDuration = time.Since(started)
		w.lastSummaryAt = time.Now().UTC()
		w.updatedAt = w.lastSummaryAt
	}
	w.mu.Unlock()
}

func researchProcessingSummarySettingsFrom(settings ResearchProcessingSettings) researchProcessingSummarySettings {
	normalizeResearchProcessingSettings(&settings)
	return researchProcessingSummarySettings{
		TimelineBucketSeconds: settings.TimelineBucketSeconds,
		TopK:                  settings.TopK,
		RecentSamples:         settings.RecentSamples,
	}
}

func ptrTimeIfSet(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func durationMilliseconds(d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	return float64(d.Microseconds()) / 1000
}

func throughputPerSecond(total uint64, startedAt, now time.Time) float64 {
	if total == 0 || startedAt.IsZero() || now.IsZero() || !now.After(startedAt) {
		return 0
	}
	return float64(total) / now.Sub(startedAt).Seconds()
}

func researchEventSampleFromRecord(record CapturedEventRecord) (researchEventSample, bool) {
	record = normalizeCapturedEventRecord(record)
	event := record.Event
	if event == nil {
		return researchEventSample{}, false
	}
	envelope := record.Envelope
	observedAt := record.ReceivedAt.UTC()
	if observedAt.IsZero() && envelope != nil && envelope.GetTimestampNs() > 0 {
		observedAt = time.Unix(0, int64(envelope.GetTimestampNs())).UTC()
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	source := determineEventEnvelopeSource(event)
	if envelope != nil && strings.TrimSpace(envelope.GetSource()) != "" {
		source = envelope.GetSource()
	}
	if strings.TrimSpace(source) == "" {
		source = "unknown"
	}
	eventType := envelopeEventTypeName(envelope, event)
	if eventType == "" {
		eventType = platform.FirstNonEmpty(event.GetType(), event.GetEventType().String())
	}
	pid := event.GetPid()
	ppid := event.GetPpid()
	comm := strings.TrimSpace(event.GetComm())
	traceID := strings.TrimSpace(event.GetTraceId())
	spanID := strings.TrimSpace(event.GetSpanId())
	if envelope != nil {
		if pid == 0 {
			pid = envelope.GetPid()
		}
		if ppid == 0 {
			ppid = envelope.GetPpid()
		}
		comm = platform.FirstNonEmpty(comm, envelope.GetComm())
		traceID = platform.FirstNonEmpty(traceID, envelope.GetTraceId())
		spanID = platform.FirstNonEmpty(spanID, envelope.GetSpanId())
	}
	target := platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath(), event.GetNetEndpoint(), event.GetDnsName(), event.GetSni(), event.GetDomain(), event.GetServiceName())
	titleParts := []string{source, eventType}
	if comm != "" {
		titleParts = append(titleParts, comm)
	}
	if target != "" {
		titleParts = append(titleParts, target)
	}
	id := ""
	if envelope != nil {
		id = strings.TrimSpace(envelope.GetEventId())
	}
	if id == "" {
		id = fmt.Sprintf("research-%d-%s-%d-%s", observedAt.UnixMilli(), sanitizeResearchIDPart(source), pid, sanitizeResearchIDPart(eventType))
	}
	return researchEventSample{
		ID:        id,
		Timestamp: observedAt.UnixMilli(),
		Time:      observedAt.Format(time.RFC3339Nano),
		Source:    strings.ToLower(strings.TrimSpace(source)),
		EventType: strings.TrimSpace(eventType),
		PID:       pid,
		PPID:      ppid,
		Comm:      comm,
		TraceID:   traceID,
		SpanID:    spanID,
		Title:     strings.Join(nonEmptyResearchParts(titleParts...), " · "),
		Target:    target,
		RiskScore: event.GetRiskScore(),
		Decision:  strings.TrimSpace(event.GetDecision()),
	}, true
}

func buildResearchProcessingSummary(events []researchEventSample, settings ResearchProcessingSettings) researchProcessingSummary {
	normalizeResearchProcessingSettings(&settings)
	now := time.Now().UTC()
	summary := researchProcessingSummary{
		Total:              len(events),
		BySource:           []researchCount{},
		ByType:             []researchCount{},
		ByComm:             []researchCount{},
		ByPID:              []researchCount{},
		ByTrace:            []researchCount{},
		Timeline:           []researchTimelineBucket{},
		TopProcesses:       []researchProcessSummary{},
		TopTraces:          []researchTraceSummary{},
		RecentSamples:      []researchEventSample{},
		GeneratedTimestamp: now.UnixMilli(),
		GeneratedTime:      now.Format(time.RFC3339Nano),
	}
	if len(events) == 0 {
		return summary
	}
	bySource := map[string]int{}
	byType := map[string]int{}
	byComm := map[string]int{}
	byPID := map[string]int{}
	byTrace := map[string]int{}
	processes := map[uint32]*mutableResearchProcess{}
	traces := map[string]*mutableResearchTrace{}
	timeline := map[int64]int{}
	bucketMs := int64(settings.TimelineBucketSeconds) * 1000
	if bucketMs <= 0 {
		bucketMs = int64(researchProcessingDefaultTimelineBucketSeconds) * 1000
	}
	for _, event := range events {
		if summary.EarliestTimestamp == 0 || event.Timestamp < summary.EarliestTimestamp {
			summary.EarliestTimestamp = event.Timestamp
		}
		if event.Timestamp > summary.LatestTimestamp {
			summary.LatestTimestamp = event.Timestamp
		}
		incrementResearchCount(bySource, event.Source)
		incrementResearchCount(byType, event.EventType)
		incrementResearchCount(byComm, event.Comm)
		if event.PID != 0 {
			incrementResearchCount(byPID, strconv.FormatUint(uint64(event.PID), 10)+":"+event.Comm)
			proc := processes[event.PID]
			if proc == nil {
				proc = &mutableResearchProcess{summary: researchProcessSummary{PID: event.PID, PPID: event.PPID, Comm: platform.FirstNonEmpty(event.Comm, "unknown")}, sources: map[string]struct{}{}, eventTypes: map[string]struct{}{}, childPids: map[uint32]struct{}{}}
				processes[event.PID] = proc
			}
			if proc.summary.PPID == 0 {
				proc.summary.PPID = event.PPID
			}
			proc.summary.EventCount++
			if proc.summary.FirstSeen == 0 || event.Timestamp < proc.summary.FirstSeen {
				proc.summary.FirstSeen = event.Timestamp
			}
			if event.Timestamp > proc.summary.LastSeen {
				proc.summary.LastSeen = event.Timestamp
			}
			if event.Source != "" {
				proc.sources[event.Source] = struct{}{}
			}
			if event.EventType != "" {
				proc.eventTypes[event.EventType] = struct{}{}
			}
		}
		if event.TraceID != "" {
			incrementResearchCount(byTrace, event.TraceID)
			trace := traces[event.TraceID]
			if trace == nil {
				trace = &mutableResearchTrace{summary: researchTraceSummary{TraceID: event.TraceID}, sources: map[string]struct{}{}, comms: map[string]struct{}{}}
				traces[event.TraceID] = trace
			}
			trace.summary.EventCount++
			if trace.summary.FirstSeen == 0 || event.Timestamp < trace.summary.FirstSeen {
				trace.summary.FirstSeen = event.Timestamp
			}
			if event.Timestamp > trace.summary.LastSeen {
				trace.summary.LastSeen = event.Timestamp
			}
			if event.Source != "" {
				trace.sources[event.Source] = struct{}{}
			}
			if event.Comm != "" {
				trace.comms[event.Comm] = struct{}{}
			}
		}
		bucket := event.Timestamp / bucketMs * bucketMs
		timeline[bucket]++
	}
	for _, proc := range processes {
		if proc.summary.PPID != 0 {
			if parent := processes[proc.summary.PPID]; parent != nil {
				parent.childPids[proc.summary.PID] = struct{}{}
			}
		}
	}
	summary.BySource = topResearchCounts(bySource, settings.TopK)
	summary.ByType = topResearchCounts(byType, settings.TopK)
	summary.ByComm = topResearchCounts(byComm, settings.TopK)
	summary.ByPID = topResearchCounts(byPID, settings.TopK)
	summary.ByTrace = topResearchCounts(byTrace, settings.TopK)
	summary.TopProcesses = topResearchProcesses(processes, settings.TopK)
	summary.TopTraces = topResearchTraces(traces, settings.TopK)
	summary.Timeline = researchTimelineBuckets(timeline, bucketMs)
	summary.RecentSamples = recentResearchSamples(events, settings.RecentSamples)
	if summary.EarliestTimestamp > 0 {
		summary.EarliestTime = time.UnixMilli(summary.EarliestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	if summary.LatestTimestamp > 0 {
		summary.LatestTime = time.UnixMilli(summary.LatestTimestamp).UTC().Format(time.RFC3339Nano)
	}
	return summary
}

type mutableResearchProcess struct {
	summary    researchProcessSummary
	sources    map[string]struct{}
	eventTypes map[string]struct{}
	childPids  map[uint32]struct{}
}

type mutableResearchTrace struct {
	summary researchTraceSummary
	sources map[string]struct{}
	comms   map[string]struct{}
}

func incrementResearchCount(counts map[string]int, key string) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	counts[key]++
}

func topResearchCounts(counts map[string]int, limit int) []researchCount {
	items := make([]researchCount, 0, len(counts))
	for key, count := range counts {
		items = append(items, researchCount{Key: key, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Key < items[j].Key
		}
		return items[i].Count > items[j].Count
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func topResearchProcesses(processes map[uint32]*mutableResearchProcess, limit int) []researchProcessSummary {
	items := make([]researchProcessSummary, 0, len(processes))
	for _, proc := range processes {
		item := proc.summary
		item.Sources = sortedResearchStrings(proc.sources, 12)
		item.EventTypes = sortedResearchStrings(proc.eventTypes, 12)
		item.ChildPids = sortedResearchUint32s(proc.childPids, 24)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EventCount == items[j].EventCount {
			return items[i].PID < items[j].PID
		}
		return items[i].EventCount > items[j].EventCount
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func topResearchTraces(traces map[string]*mutableResearchTrace, limit int) []researchTraceSummary {
	items := make([]researchTraceSummary, 0, len(traces))
	for _, trace := range traces {
		item := trace.summary
		item.Sources = sortedResearchStrings(trace.sources, 12)
		item.Comms = sortedResearchStrings(trace.comms, 12)
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].EventCount == items[j].EventCount {
			return items[i].TraceID < items[j].TraceID
		}
		return items[i].EventCount > items[j].EventCount
	})
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func researchTimelineBuckets(counts map[int64]int, bucketMs int64) []researchTimelineBucket {
	starts := make([]int64, 0, len(counts))
	for start := range counts {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	items := make([]researchTimelineBucket, 0, len(starts))
	for _, start := range starts {
		items = append(items, researchTimelineBucket{Start: start, End: start + bucketMs, Time: time.UnixMilli(start).UTC().Format(time.RFC3339Nano), Count: counts[start]})
	}
	return items
}

func recentResearchSamples(events []researchEventSample, limit int) []researchEventSample {
	if limit <= 0 {
		limit = researchProcessingDefaultRecentSamples
	}
	if len(events) <= limit {
		out := make([]researchEventSample, len(events))
		copy(out, events)
		return out
	}
	out := make([]researchEventSample, limit)
	copy(out, events[len(events)-limit:])
	return out
}

func sortedResearchStrings(set map[string]struct{}, limit int) []string {
	items := make([]string, 0, len(set))
	for value := range set {
		if strings.TrimSpace(value) != "" {
			items = append(items, value)
		}
	}
	sort.Strings(items)
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func sortedResearchUint32s(set map[uint32]struct{}, limit int) []uint32 {
	items := make([]uint32, 0, len(set))
	for value := range set {
		if value != 0 {
			items = append(items, value)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i] < items[j] })
	if limit > 0 && len(items) > limit {
		return items[:limit]
	}
	return items
}

func cloneResearchProcessingSummary(summary researchProcessingSummary) researchProcessingSummary {
	cloned := summary
	cloned.BySource = append([]researchCount(nil), summary.BySource...)
	cloned.ByType = append([]researchCount(nil), summary.ByType...)
	cloned.ByComm = append([]researchCount(nil), summary.ByComm...)
	cloned.ByPID = append([]researchCount(nil), summary.ByPID...)
	cloned.ByTrace = append([]researchCount(nil), summary.ByTrace...)
	cloned.Timeline = append([]researchTimelineBucket(nil), summary.Timeline...)
	cloned.TopProcesses = append([]researchProcessSummary(nil), summary.TopProcesses...)
	cloned.TopTraces = append([]researchTraceSummary(nil), summary.TopTraces...)
	cloned.RecentSamples = append([]researchEventSample(nil), summary.RecentSamples...)
	return cloned
}

func sanitizeResearchIDPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func nonEmptyResearchParts(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func handleResearchProcessingStatus(c *gin.Context) {
	c.JSON(200, researchProcessingWorkerStore.Status())
}

func handleResearchProcessingTask(c *gin.Context) {
	var req researchProcessingTaskRequest
	if status, err := bindResearchJSON(c, &req); err != nil {
		c.JSON(status, gin.H{"error": "invalid research processing task"})
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action == "" {
		action = "scan_recent"
	}
	switch action {
	case "scan_recent", "scan":
		limit := req.Limit
		if limit <= 0 {
			limit = 1000
		}
		if limit > 50000 {
			limit = 50000
		}
		records, _, err := runtimeSettingsStore.RecentEvents(limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if !researchProcessingWorkerStore.EnqueueScan(records) {
			c.JSON(503, gin.H{"error": "research processing worker queue is full or not started"})
			return
		}
		status := researchProcessingWorkerStore.Status()
		c.JSON(202, researchProcessingTaskResponse{Status: "queued", Action: "scan_recent", Records: len(records), QueueLen: status.QueueLen})
	case "reset":
		if !researchProcessingWorkerStore.EnqueueReset() {
			researchProcessingWorkerStore.resetNow()
		}
		status := researchProcessingWorkerStore.Status()
		c.JSON(202, researchProcessingTaskResponse{Status: "queued", Action: "reset", QueueLen: status.QueueLen})
	default:
		c.JSON(400, gin.H{"error": "unsupported research processing task", "supported": []string{"scan_recent", "reset"}})
	}
}
