package app

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/internal/workerqueue"
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const (
	loopDetectionDefaultWindowSeconds   = 30
	loopDetectionDefaultRepeatThreshold = 5
	loopDetectionDefaultMaxContexts     = 512
	loopDetectionDefaultQueueSize       = 2048
	loopDetectionFindingLimit           = 50
	loopDetectionWindowGCMaxInterval    = 30 * time.Second
	loopDetectionWindowMetadataLimit    = 12
	loopDetectionContextComponentBytes  = 128
	loopDetectionMetadataValueBytes     = 160
	loopDetectionTargetBytes            = 240
	loopDetectionExtraInfoScanBytes     = 4096
)

type loopDetectionStatus struct {
	Enabled        bool                   `json:"enabled"`
	Settings       LoopDetectionSettings  `json:"settings"`
	QueueLen       int                    `json:"queueLen"`
	QueueCap       int                    `json:"queueCap"`
	ConsumedTotal  uint64                 `json:"consumedTotal"`
	FindingsTotal  uint64                 `json:"findingsTotal"`
	DroppedTotal   uint64                 `json:"droppedTotal"`
	WindowCount    int                    `json:"windowCount"`
	WindowGCRuns   uint64                 `json:"windowGCRunsTotal"`
	WindowEvicted  uint64                 `json:"windowEvictedTotal"`
	RecentFindings []loopDetectionFinding `json:"recentFindings"`
	LastError      string                 `json:"lastError,omitempty"`
	UpdatedAt      time.Time              `json:"updatedAt"`
}

type loopDetectionTaskRequest struct {
	Action string `json:"action"`
	Limit  int    `json:"limit"`
}

type loopDetectionTaskResponse struct {
	Status   string `json:"status"`
	Action   string `json:"action"`
	Records  int    `json:"records,omitempty"`
	QueueLen int    `json:"queueLen"`
}

type loopDetectionWorkKind string

const (
	loopDetectionWorkEvent loopDetectionWorkKind = "event"
	loopDetectionWorkScan  loopDetectionWorkKind = "scan"
	loopDetectionWorkReset loopDetectionWorkKind = "reset"
)

type loopDetectionWorkItem struct {
	kind    loopDetectionWorkKind
	record  CapturedEventRecord
	records []CapturedEventRecord
	force   bool
}

type loopDetectionWindowKey struct {
	contextKey  string
	fingerprint string
}

type loopDetectionWindow struct {
	FirstSeen     time.Time
	LastSeen      time.Time
	Count         int
	Alerted       bool
	ContextType   string
	ContextKey    string
	Fingerprint   string
	Target        string
	EventTypes    map[string]struct{}
	Pids          map[uint32]struct{}
	Comms         map[string]struct{}
	Paths         map[string]struct{}
	ToolNames     map[string]struct{}
	AgentRunID    string
	TaskID        string
	ToolCallID    string
	TraceID       string
	RootAgentPID  uint32
	PID           uint32
	Comm          string
	WindowSeconds int

	key     loopDetectionWindowKey
	lruPrev *loopDetectionWindow
	lruNext *loopDetectionWindow
}

type loopDetectionWorker struct {
	queue         workerqueue.Queue[loopDetectionWorkItem]
	mu            sync.RWMutex
	windows       map[loopDetectionWindowKey]*loopDetectionWindow
	windowLRUHead *loopDetectionWindow
	windowLRUTail *loopDetectionWindow
	lastWindowGC  time.Time
	windowGCRuns  uint64
	windowEvicted uint64
	findings      []loopDetectionFinding
	consumedTotal uint64
	findingsTotal uint64
	droppedTotal  uint64
	lastError     string
	updatedAt     time.Time
}

var loopDetectionWorkerStore = newLoopDetectionWorker()

func newLoopDetectionWorker() *loopDetectionWorker {
	return &loopDetectionWorker{
		windows:   make(map[loopDetectionWindowKey]*loopDetectionWindow),
		findings:  make([]loopDetectionFinding, 0, loopDetectionFindingLimit),
		updatedAt: time.Now().UTC(),
	}
}

func startLoopDetectionWorker(ctx context.Context) {
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	loopDetectionWorkerStore.Start(ctx, settings.QueueSize)
}

func (w *loopDetectionWorker) Start(ctx context.Context, queueSize int) {
	queueSize = normalizeBackendWorkerQueueSize(queueSize, loopDetectionDefaultQueueSize)
	if w.queue.Start(ctx, queueSize, func(ctx context.Context, items <-chan loopDetectionWorkItem) {
		w.run(ctx, items)
		w.touch()
	}) {
		w.touch()
	}
}

func (w *loopDetectionWorker) touch() {
	w.mu.Lock()
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *loopDetectionWorker) run(ctx context.Context, queue <-chan loopDetectionWorkItem) {
	for {
		var item loopDetectionWorkItem
		select {
		case <-ctx.Done():
			return
		case item = <-queue:
		}
		if ctx.Err() != nil {
			return
		}
		switch item.kind {
		case loopDetectionWorkReset:
			w.resetNow()
		case loopDetectionWorkScan:
			w.processScan(ctx, item.records, item.force)
		case loopDetectionWorkEvent:
			w.processRecord(item.record, item.force)
		}
	}
}

func (w *loopDetectionWorker) Shutdown(ctx context.Context) error {
	if w.queue.Stats().Started {
		w.touch()
	}
	return w.queue.Shutdown(ctx)
}

func queueLoopDetectionRecord(record CapturedEventRecord) {
	if record.Event == nil || shouldIgnoreLoopDetectionEvent(record.Event) {
		return
	}
	// Only the enable flag gates ingestion; the worker normalises the rest of
	// the settings when it applies them.
	if !runtimeSettingsStore.LoopDetectionSettings().Enabled {
		return
	}
	loopDetectionWorkerStore.EnqueueEvent(record)
}

func (w *loopDetectionWorker) EnqueueEvent(record CapturedEventRecord) bool {
	return w.enqueue(loopDetectionWorkItem{kind: loopDetectionWorkEvent, record: record})
}

func (w *loopDetectionWorker) EnqueueScan(records []CapturedEventRecord) bool {
	return w.enqueue(loopDetectionWorkItem{kind: loopDetectionWorkScan, records: records, force: true})
}

func (w *loopDetectionWorker) EnqueueReset() bool {
	return w.enqueue(loopDetectionWorkItem{kind: loopDetectionWorkReset})
}

func (w *loopDetectionWorker) enqueue(item loopDetectionWorkItem) bool {
	switch w.queue.TryEnqueue(item) {
	case workerqueue.Accepted:
		return true
	case workerqueue.NotStarted:
		w.noteDrop("loop detection worker is not started")
	default:
		w.noteDrop("loop detection queue is full")
	}
	return false
}

func (w *loopDetectionWorker) resetNow() {
	w.mu.Lock()
	w.windows = make(map[loopDetectionWindowKey]*loopDetectionWindow)
	w.windowLRUHead = nil
	w.windowLRUTail = nil
	w.lastWindowGC = time.Time{}
	w.findings = w.findings[:0]
	w.lastError = ""
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *loopDetectionWorker) noteDrop(message string) {
	w.mu.Lock()
	w.droppedTotal++
	w.lastError = message
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *loopDetectionWorker) processRecord(record CapturedEventRecord, force bool) {
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	w.processRecordWithSettings(record, force, settings)
}

func (w *loopDetectionWorker) processScan(ctx context.Context, records []CapturedEventRecord, force bool) {
	if len(records) == 0 {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	if !force && !settings.Enabled {
		return
	}
	for _, record := range records {
		if ctx.Err() != nil {
			return
		}
		w.processRecordWithSettings(record, force, settings)
	}
}

func (w *loopDetectionWorker) processRecordWithSettings(record CapturedEventRecord, force bool, settings LoopDetectionSettings) {
	if record.Event == nil || shouldIgnoreLoopDetectionEvent(record.Event) {
		return
	}
	if !force && !settings.Enabled {
		return
	}

	event := record.Event
	observedAt := record.ReceivedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	contextType, contextKey := loopDetectionContext(event)
	fingerprint, target := loopDetectionFingerprint(event)
	if contextKey == "" || fingerprint == "" {
		return
	}
	verifiedClone := isVerifiedPublicCloneProcess(event)
	if verifiedClone {
		// Avoid letting a normal command in the same Agent run inherit
		// a clone's 8x threshold or contribute to its repeat window.
		contextKey += "|public_clone_pid=" + strconv.FormatUint(uint64(event.GetPid()), 10)
	}
	windowSeconds := settings.WindowSeconds
	if windowSeconds <= 0 {
		windowSeconds = loopDetectionDefaultWindowSeconds
	}
	windowDuration := time.Duration(windowSeconds) * time.Second
	threshold := settings.RepeatThreshold
	if threshold <= 0 {
		threshold = loopDetectionDefaultRepeatThreshold
	}
	// A validated public clone legitimately repeats object/file operations.
	// Raise only its loop heuristic threshold; keep detection active and
	// leave all other security alert categories untouched.
	if verifiedClone && threshold <= 1_000_000 {
		threshold *= 8
	}
	windowKey := loopDetectionWindowKey{contextKey: contextKey, fingerprint: fingerprint}

	var finding *loopDetectionFinding
	w.mu.Lock()
	w.consumedTotal++
	win := w.windows[windowKey]
	if win == nil || win.FirstSeen.IsZero() || observedAt.Sub(win.FirstSeen) > windowDuration {
		if win != nil {
			w.removeLoopDetectionWindowLocked(win)
		}
		win = newLoopDetectionWindow(contextType, contextKey, fingerprint, target, event, observedAt, windowSeconds)
		win.key = windowKey
		w.windows[windowKey] = win
		w.appendLoopDetectionWindowLocked(win)
	} else {
		win.Count++
		win.LastSeen = observedAt
		win.WindowSeconds = windowSeconds
		mergeLoopDetectionWindow(win, event, target)
		w.touchLoopDetectionWindowLocked(win)
	}
	w.maybeEvictExpiredLoopDetectionWindowsLocked(observedAt, windowDuration)
	w.enforceLoopDetectionWindowCapacityLocked(settings.MaxContexts)
	if win.Count >= threshold && !win.Alerted {
		built := buildLoopDetectionFinding(win, observedAt)
		w.appendFindingLocked(built)
		win.Alerted = true
		finding = &built
	}
	w.updatedAt = observedAt
	w.mu.Unlock()

	if finding != nil && settings.EmitSemanticAlerts {
		w.emitSemanticAlert(*finding)
	}
}

func (w *loopDetectionWorker) emitSemanticAlert(finding loopDetectionFinding) {
	alert := loopDetectionSemanticAlert(finding)
	if !enqueueBroadcastEvent(broadcast, alert, "loop_detection_alert") {
		w.noteDrop("broadcast queue is full while emitting loop detection alert")
	}
}

func (w *loopDetectionWorker) Status() loopDetectionStatus {
	settings := runtimeSettingsStore.Snapshot().LoopDetection
	normalizeLoopDetectionSettings(&settings)
	queueStats := w.queue.Stats()
	queueLen, queueCap := queueStats.Len, queueStats.Cap
	w.mu.RLock()
	findings := make([]loopDetectionFinding, len(w.findings))
	copy(findings, w.findings)
	status := loopDetectionStatus{
		Enabled:        settings.Enabled,
		Settings:       settings,
		QueueLen:       queueLen,
		QueueCap:       queueCap,
		ConsumedTotal:  w.consumedTotal,
		FindingsTotal:  w.findingsTotal,
		DroppedTotal:   w.droppedTotal,
		WindowCount:    len(w.windows),
		WindowGCRuns:   w.windowGCRuns,
		WindowEvicted:  w.windowEvicted,
		RecentFindings: findings,
		LastError:      w.lastError,
		UpdatedAt:      w.updatedAt,
	}
	w.mu.RUnlock()
	return status
}

func handleLoopDetectionStatus(c *gin.Context) {
	c.JSON(200, loopDetectionWorkerStore.Status())
}

func handleLoopDetectionTask(c *gin.Context) {
	var req loopDetectionTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": "invalid loop detection task"})
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
			limit = 500
		}
		if limit > 10000 {
			limit = 10000
		}
		records, _, err := runtimeSettingsStore.RecentEvents(limit)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if !loopDetectionWorkerStore.EnqueueScan(records) {
			c.JSON(503, gin.H{"error": "loop detection worker queue is full or not started"})
			return
		}
		status := loopDetectionWorkerStore.Status()
		c.JSON(202, loopDetectionTaskResponse{Status: "queued", Action: "scan_recent", Records: len(records), QueueLen: status.QueueLen})
	case "reset":
		if !loopDetectionWorkerStore.EnqueueReset() {
			loopDetectionWorkerStore.resetNow()
		}
		status := loopDetectionWorkerStore.Status()
		c.JSON(202, loopDetectionTaskResponse{Status: "queued", Action: "reset", QueueLen: status.QueueLen})
	default:
		c.JSON(400, gin.H{"error": "unsupported loop detection task", "supported": []string{"scan_recent", "reset"}})
	}
}

// loopDetectionEnvSummary is used by the UI copy and tests to document the
// environment variables that override persisted loop settings on startup.
func loopDetectionEnvSummary() []string {
	return []string{
		"AGENT_RUNTIME_LOOP_DETECTION_ENABLED",
		"AGENT_RUNTIME_LOOP_DETECTION_WINDOW_SECONDS",
		"AGENT_RUNTIME_LOOP_DETECTION_REPEAT_THRESHOLD",
		"AGENT_RUNTIME_LOOP_DETECTION_MAX_CONTEXTS",
		"AGENT_RUNTIME_LOOP_DETECTION_QUEUE_SIZE",
		"AGENT_RUNTIME_LOOP_DETECTION_EMIT_ALERTS",
		platform.RuntimeSettingsPath(),
	}
}
