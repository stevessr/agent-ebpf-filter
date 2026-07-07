package app

import (
	"errors"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	backendTaskStatusQueued    = "queued"
	backendTaskStatusRunning   = "running"
	backendTaskStatusSucceeded = "succeeded"
	backendTaskStatusFailed    = "failed"
	backendTaskStatusCanceled  = "canceled"
)

var (
	errBackendTaskQueueFull = errors.New("backend task queue is full")
	errBackendTaskCanceled  = errors.New("backend task canceled")
)

type backendTaskRuntimeEntry struct {
	mu         sync.RWMutex
	id         string
	kind       string
	status     string
	progress   float64
	queuedAt   time.Time
	startedAt  *time.Time
	finishedAt *time.Time
	err        string
	payload    any
	cancel     chan struct{}
	cancelOnce sync.Once
	queueLen   int
}

type backendTaskRuntimeSnapshot struct {
	ID              string     `json:"id"`
	Kind            string     `json:"kind"`
	Status          string     `json:"status"`
	Progress        float64    `json:"progress"`
	QueuedAt        time.Time  `json:"queuedAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	FinishedAt      *time.Time `json:"finishedAt,omitempty"`
	Error           string     `json:"error,omitempty"`
	QueueLen        int        `json:"queueLen,omitempty"`
	QueueLatencyMs  float64    `json:"queueLatencyMs,omitempty"`
	RunDurationMs   float64    `json:"runDurationMs,omitempty"`
	TotalDurationMs float64    `json:"totalDurationMs,omitempty"`
}

type backendTaskRuntimeStats struct {
	Name                string     `json:"name"`
	Started             bool       `json:"started"`
	QueueLen            int        `json:"queueLen"`
	QueueCap            int        `json:"queueCap"`
	TrackedTotal        int        `json:"trackedTotal"`
	EnqueuedTotal       uint64     `json:"enqueuedTotal"`
	CompletedTotal      uint64     `json:"completedTotal"`
	FailedTotal         uint64     `json:"failedTotal"`
	CanceledTotal       uint64     `json:"canceledTotal"`
	RejectedTotal       uint64     `json:"rejectedTotal"`
	LastQueueLatencyMs  float64    `json:"lastQueueLatencyMs,omitempty"`
	LastRunDurationMs   float64    `json:"lastRunDurationMs,omitempty"`
	LastTotalDurationMs float64    `json:"lastTotalDurationMs,omitempty"`
	AvgRunDurationMs    float64    `json:"avgRunDurationMs,omitempty"`
	LastStartedAt       *time.Time `json:"lastStartedAt,omitempty"`
	LastFinishedAt      *time.Time `json:"lastFinishedAt,omitempty"`
	LastError           string     `json:"lastError,omitempty"`
	LastRejectReason    string     `json:"lastRejectReason,omitempty"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

type backendTaskRuntime struct {
	mu       sync.RWMutex
	name     string
	queue    chan *backendTaskRuntimeEntry
	started  bool
	tasks    map[string]*backendTaskRuntimeEntry
	maxItems int
	handler  func(*backendTaskRuntimeEntry) error

	enqueuedTotal     uint64
	completedTotal    uint64
	failedTotal       uint64
	canceledTotal     uint64
	rejectedTotal     uint64
	runDurationTotal  time.Duration
	lastQueueLatency  time.Duration
	lastRunDuration   time.Duration
	lastTotalDuration time.Duration
	lastStartedAt     time.Time
	lastFinishedAt    time.Time
	lastError         string
	lastRejectReason  string
	updatedAt         time.Time
}

func newBackendTaskRuntime(name string, maxItems int, handler func(*backendTaskRuntimeEntry) error) *backendTaskRuntime {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "backend"
	}
	if maxItems <= 0 {
		maxItems = 1024
	}
	return &backendTaskRuntime{
		name:      name,
		tasks:     make(map[string]*backendTaskRuntimeEntry),
		maxItems:  maxItems,
		handler:   handler,
		updatedAt: time.Now().UTC(),
	}
}

func newBackendTaskRuntimeEntry(id, kind string, payload any) *backendTaskRuntimeEntry {
	now := time.Now().UTC()
	return &backendTaskRuntimeEntry{
		id:       strings.TrimSpace(id),
		kind:     strings.TrimSpace(kind),
		status:   backendTaskStatusQueued,
		queuedAt: now,
		payload:  payload,
		cancel:   make(chan struct{}),
	}
}

func (r *backendTaskRuntime) Start(queueSize int) {
	if r == nil {
		return
	}
	if queueSize <= 0 {
		queueSize = researchProcessingDefaultQueueSize
	}
	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return
	}
	r.queue = make(chan *backendTaskRuntimeEntry, queueSize)
	r.started = true
	r.updatedAt = time.Now().UTC()
	queue := r.queue
	r.mu.Unlock()
	go r.run(queue)
}

func (r *backendTaskRuntime) Submit(entry *backendTaskRuntimeEntry) error {
	if r == nil {
		return errors.New("backend task runtime is unavailable")
	}
	if entry == nil || strings.TrimSpace(entry.id) == "" {
		return errors.New("backend task entry is invalid")
	}
	r.mu.Lock()
	r.pruneLocked()
	queue := r.queue
	if queue == nil {
		queue = make(chan *backendTaskRuntimeEntry, researchProcessingDefaultQueueSize)
		r.queue = queue
		r.started = true
		go r.run(queue)
	}
	r.tasks[entry.id] = entry
	select {
	case queue <- entry:
		entry.setQueueLen(len(queue))
		r.enqueuedTotal++
		r.updatedAt = time.Now().UTC()
		r.mu.Unlock()
		return nil
	default:
		delete(r.tasks, entry.id)
		r.rejectedTotal++
		r.lastRejectReason = "queue_full"
		r.lastError = errBackendTaskQueueFull.Error()
		r.updatedAt = time.Now().UTC()
		r.mu.Unlock()
		return errBackendTaskQueueFull
	}
}

func (r *backendTaskRuntime) Get(id string) (*backendTaskRuntimeEntry, bool) {
	if r == nil {
		return nil, false
	}
	r.mu.RLock()
	entry := r.tasks[strings.TrimSpace(id)]
	r.mu.RUnlock()
	return entry, entry != nil
}

func (r *backendTaskRuntime) Cancel(id string) (*backendTaskRuntimeEntry, bool) {
	entry, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	entry.Cancel()
	return entry, true
}

func (r *backendTaskRuntime) Stats() backendTaskRuntimeStats {
	if r == nil {
		return backendTaskRuntimeStats{}
	}
	r.mu.RLock()
	queueLen := 0
	queueCap := 0
	if r.queue != nil {
		queueLen = len(r.queue)
		queueCap = cap(r.queue)
	}
	stats := backendTaskRuntimeStats{
		Name:                r.name,
		Started:             r.started,
		QueueLen:            queueLen,
		QueueCap:            queueCap,
		TrackedTotal:        len(r.tasks),
		EnqueuedTotal:       r.enqueuedTotal,
		CompletedTotal:      r.completedTotal,
		FailedTotal:         r.failedTotal,
		CanceledTotal:       r.canceledTotal,
		RejectedTotal:       r.rejectedTotal,
		LastQueueLatencyMs:  durationMilliseconds(r.lastQueueLatency),
		LastRunDurationMs:   durationMilliseconds(r.lastRunDuration),
		LastTotalDurationMs: durationMilliseconds(r.lastTotalDuration),
		AvgRunDurationMs:    backendTaskAverageDurationMs(r.runDurationTotal, r.completedTotal),
		LastStartedAt:       ptrTimeIfSet(r.lastStartedAt),
		LastFinishedAt:      ptrTimeIfSet(r.lastFinishedAt),
		LastError:           r.lastError,
		LastRejectReason:    r.lastRejectReason,
		UpdatedAt:           r.updatedAt,
	}
	r.mu.RUnlock()
	return stats
}

func (r *backendTaskRuntime) run(queue <-chan *backendTaskRuntimeEntry) {
	for entry := range queue {
		if entry == nil {
			continue
		}
		if !entry.markRunning() {
			entry.finish(backendTaskStatusCanceled, 1, "")
			r.noteFinished(entry, backendTaskStatusCanceled, "")
			continue
		}
		err := error(nil)
		if r.handler != nil {
			err = r.handler(entry)
		}
		if err != nil {
			if errors.Is(err, errBackendTaskCanceled) || entry.IsCanceled() {
				entry.finish(backendTaskStatusCanceled, 1, "")
				r.noteFinished(entry, backendTaskStatusCanceled, "")
				continue
			}
			entry.finish(backendTaskStatusFailed, entry.Progress(), err.Error())
			r.noteFinished(entry, backendTaskStatusFailed, err.Error())
			continue
		}
		if entry.IsCanceled() {
			entry.finish(backendTaskStatusCanceled, 1, "")
			r.noteFinished(entry, backendTaskStatusCanceled, "")
			continue
		}
		entry.finish(backendTaskStatusSucceeded, 1, "")
		r.noteFinished(entry, backendTaskStatusSucceeded, "")
	}
}

func (r *backendTaskRuntime) noteFinished(entry *backendTaskRuntimeEntry, status, message string) {
	if r == nil {
		return
	}
	snapshot := entry.Snapshot()
	queueLatency, runDuration, totalDuration := backendTaskSnapshotDurations(snapshot)
	r.mu.Lock()
	r.completedTotal++
	r.runDurationTotal += runDuration
	r.lastQueueLatency = queueLatency
	r.lastRunDuration = runDuration
	r.lastTotalDuration = totalDuration
	if snapshot.StartedAt != nil {
		r.lastStartedAt = *snapshot.StartedAt
	}
	if snapshot.FinishedAt != nil {
		r.lastFinishedAt = *snapshot.FinishedAt
	}
	switch status {
	case backendTaskStatusFailed:
		r.failedTotal++
	case backendTaskStatusCanceled:
		r.canceledTotal++
	}
	r.lastError = message
	r.updatedAt = time.Now().UTC()
	r.mu.Unlock()
}

func (r *backendTaskRuntime) pruneLocked() {
	if r == nil || len(r.tasks) <= r.maxItems {
		return
	}
	type item struct {
		id string
		ts time.Time
	}
	items := make([]item, 0, len(r.tasks))
	for id, entry := range r.tasks {
		snapshot := entry.Snapshot()
		ts := snapshot.QueuedAt
		if snapshot.FinishedAt != nil {
			ts = *snapshot.FinishedAt
		}
		items = append(items, item{id: id, ts: ts})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ts.Before(items[j].ts) })
	for len(r.tasks) > r.maxItems && len(items) > 0 {
		delete(r.tasks, items[0].id)
		items = items[1:]
	}
}

func (entry *backendTaskRuntimeEntry) Snapshot() backendTaskRuntimeSnapshot {
	if entry == nil {
		return backendTaskRuntimeSnapshot{}
	}
	entry.mu.RLock()
	out := backendTaskRuntimeSnapshot{
		ID:         entry.id,
		Kind:       entry.kind,
		Status:     entry.status,
		Progress:   entry.progress,
		QueuedAt:   entry.queuedAt,
		StartedAt:  cloneTimePtr(entry.startedAt),
		FinishedAt: cloneTimePtr(entry.finishedAt),
		Error:      entry.err,
		QueueLen:   entry.queueLen,
	}
	entry.mu.RUnlock()
	queueLatency, runDuration, totalDuration := backendTaskSnapshotDurations(out)
	out.QueueLatencyMs = durationMilliseconds(queueLatency)
	out.RunDurationMs = durationMilliseconds(runDuration)
	out.TotalDurationMs = durationMilliseconds(totalDuration)
	return out
}

func (entry *backendTaskRuntimeEntry) Payload() any {
	if entry == nil {
		return nil
	}
	entry.mu.RLock()
	payload := entry.payload
	entry.mu.RUnlock()
	return payload
}

func (entry *backendTaskRuntimeEntry) Cancel() {
	if entry == nil {
		return
	}
	entry.cancelOnce.Do(func() { close(entry.cancel) })
	now := time.Now().UTC()
	entry.mu.Lock()
	if entry.status == backendTaskStatusQueued {
		entry.status = backendTaskStatusCanceled
		entry.progress = 1
		entry.finishedAt = &now
	}
	entry.mu.Unlock()
}

func (entry *backendTaskRuntimeEntry) IsCanceled() bool {
	if entry == nil || entry.cancel == nil {
		return false
	}
	select {
	case <-entry.cancel:
		return true
	default:
		return false
	}
}

func (entry *backendTaskRuntimeEntry) markRunning() bool {
	if entry == nil {
		return false
	}
	now := time.Now().UTC()
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.status == backendTaskStatusCanceled || entry.IsCanceled() {
		return false
	}
	entry.status = backendTaskStatusRunning
	entry.progress = maxFloat(entry.progress, 0.01)
	entry.startedAt = &now
	return true
}

func (entry *backendTaskRuntimeEntry) SetProgress(progress float64) {
	if entry == nil {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	entry.mu.Lock()
	if entry.status == backendTaskStatusRunning && progress > entry.progress {
		entry.progress = progress
	}
	entry.mu.Unlock()
}

func (entry *backendTaskRuntimeEntry) Progress() float64 {
	if entry == nil {
		return 0
	}
	entry.mu.RLock()
	progress := entry.progress
	entry.mu.RUnlock()
	return progress
}

func (entry *backendTaskRuntimeEntry) finish(status string, progress float64, message string) {
	if entry == nil {
		return
	}
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	now := time.Now().UTC()
	entry.mu.Lock()
	if entry.status == backendTaskStatusCanceled && status == backendTaskStatusSucceeded {
		status = backendTaskStatusCanceled
	}
	entry.status = status
	entry.progress = progress
	entry.finishedAt = &now
	entry.err = message
	entry.mu.Unlock()
}

func (entry *backendTaskRuntimeEntry) setQueueLen(queueLen int) {
	if entry == nil {
		return
	}
	entry.mu.Lock()
	entry.queueLen = queueLen
	entry.mu.Unlock()
}

func cloneTimePtr(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	cloned := *value
	return &cloned
}

func backendTaskSnapshotDurations(snapshot backendTaskRuntimeSnapshot) (time.Duration, time.Duration, time.Duration) {
	var queueLatency time.Duration
	var runDuration time.Duration
	var totalDuration time.Duration
	if snapshot.StartedAt != nil && !snapshot.QueuedAt.IsZero() && snapshot.StartedAt.After(snapshot.QueuedAt) {
		queueLatency = snapshot.StartedAt.Sub(snapshot.QueuedAt)
	}
	if snapshot.StartedAt != nil && snapshot.FinishedAt != nil && snapshot.FinishedAt.After(*snapshot.StartedAt) {
		runDuration = snapshot.FinishedAt.Sub(*snapshot.StartedAt)
	}
	if snapshot.FinishedAt != nil && !snapshot.QueuedAt.IsZero() && snapshot.FinishedAt.After(snapshot.QueuedAt) {
		totalDuration = snapshot.FinishedAt.Sub(snapshot.QueuedAt)
	}
	return queueLatency, runDuration, totalDuration
}

func backendTaskAverageDurationMs(total time.Duration, count uint64) float64 {
	if total <= 0 || count == 0 {
		return 0
	}
	return durationMilliseconds(total / time.Duration(count))
}
