package research

import (
	"agent-ebpf-filter/app/tasks"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/workerqueue"
	"context"
	"sync"
	"time"
)

type researchProcessingWorker struct {
	queue               workerqueue.Queue[researchProcessingWorkItem]
	mu                  sync.RWMutex
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

var ProcessingWorker = newResearchProcessingWorker()

func newResearchProcessingWorker() *researchProcessingWorker {
	now := time.Now().UTC()
	return &researchProcessingWorker{
		summary:   researchProcessingSummary{},
		startedAt: now,
		updatedAt: now,
	}
}

func startResearchProcessingWorker(ctx context.Context) {
	settings := snapshotRuntimeSettings().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	ProcessingWorker.Start(ctx, settings.QueueSize)
}

func (w *researchProcessingWorker) Start(ctx context.Context, queueSize int) {
	queueSize = tasks.NormalizeQueueSize(queueSize, researchProcessingDefaultQueueSize)
	started := w.queue.Start(ctx, queueSize, func(ctx context.Context, items <-chan researchProcessingWorkItem) {
		w.run(ctx, items)
		w.touch()
	})
	if !started {
		return
	}
	now := time.Now().UTC()
	w.mu.Lock()
	if w.startedAt.IsZero() {
		w.startedAt = now
	}
	w.updatedAt = now
	w.mu.Unlock()
}

func (w *researchProcessingWorker) touch() {
	w.mu.Lock()
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
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
	if w.queue.Stats().Started {
		w.touch()
	}
	return w.queue.Shutdown(ctx)
}

func queueResearchProcessingRecord(record CapturedEventRecord) {
	if record.Event == nil {
		return
	}
	if !ResearchSettingsHook().Enabled {
		ProcessingWorker.noteDrop("disabled", "research processing is disabled")
		return
	}
	ProcessingWorker.EnqueueEvent(record)
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
	if item.queuedAt.IsZero() {
		item.queuedAt = time.Now().UTC()
	}
	switch w.queue.TryEnqueue(item) {
	case workerqueue.Accepted:
		w.noteEnqueued(item.queuedAt)
		return true
	case workerqueue.NotStarted:
		w.noteDrop("worker_not_started", "research processing worker is not started")
	default:
		w.noteDrop("queue_full", "research processing queue is full")
	}
	return false
}

func (w *researchProcessingWorker) noteEnqueued(queuedAt time.Time) {
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
	if len(records) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	settings := snapshotRuntimeSettings().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	if !force && !settings.Enabled {
		w.noteDrop("disabled", "research processing is disabled")
		return
	}
	maxEvents := settings.MaxEvents
	if maxEvents <= 0 {
		maxEvents = researchProcessingDefaultMaxEvents
	}
	for offset := 0; offset < len(records); offset += core.WorkerScanBatchSize {
		if ctx.Err() != nil {
			return
		}
		end := min(offset+core.WorkerScanBatchSize, len(records))
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

func (w *researchProcessingWorker) Status() ResearchProcessingStatus {
	settings := snapshotRuntimeSettings().ResearchProcessing
	normalizeResearchProcessingSettings(&settings)
	w.refreshSummaryIfNeeded(settings)
	queueStats := w.queue.Stats()
	queueLen, queueCap := queueStats.Len, queueStats.Cap
	w.mu.RLock()
	lastEnqueuedAt := ptrTimeIfSet(w.lastEnqueuedAt)
	lastProcessedAt := ptrTimeIfSet(w.lastProcessedAt)
	lastDroppedAt := ptrTimeIfSet(w.lastDroppedAt)
	lastSummaryAt := ptrTimeIfSet(w.lastSummaryAt)
	status := ResearchProcessingStatus{
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

// StartProcessingWorker launches the background research-processing worker.
func StartProcessingWorker(ctx context.Context) { startResearchProcessingWorker(ctx) }

// ProcessingWorker exposes the shared worker for lifecycle management.
// NewProcessingWorker builds an isolated worker instance (tests).
func NewProcessingWorker() *researchProcessingWorker { return newResearchProcessingWorker() }

// Defaults exported for the app settings normalizer.
const (
	ArtifactRetentionDaysDefault = researchProcessingDefaultArtifactRetentionDays
	MaxSessionEventsDefault      = researchProcessingDefaultMaxSessionEvents
	ExportFormatsDefault         = researchProcessingDefaultExportFormats
)

// QueueProcessingRecord enqueues a captured event into the research
// processing pipeline from the app worker path.
func QueueProcessingRecord(record CapturedEventRecord) { queueResearchProcessingRecord(record) }
