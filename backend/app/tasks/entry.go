package tasks

import (
	"errors"
	"strings"
	"sync"
	"time"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

var (
	ErrQueueFull    = errors.New("backend task queue is full")
	ErrDuplicateID  = errors.New("backend task id already exists")
	ErrClosed       = errors.New("backend task runtime is closed")
	ErrCanceled     = errors.New("backend task canceled")
	ErrHandlerPanic = errors.New("backend task handler panicked")
)

type Entry struct {
	mu         sync.RWMutex
	id         string
	kind       string
	status     string
	progress   float64
	queuedAt   time.Time
	startedAt  *time.Time
	finishedAt *time.Time
	accounted  bool
	// terminalNext is owned by Runtime.mu after accounted is set.
	terminalNext *Entry
	err          string
	payload      any
	cancel       chan struct{}
	cancelOnce   sync.Once
	queueLen     int
}

type Snapshot struct {
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

func ptrTimeIfSet(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	cp := t
	return &cp
}

func durationMS(d time.Duration) float64 {
	if d < 0 {
		d = 0
	}
	return float64(d.Nanoseconds()) / float64(time.Millisecond)
}

func NewEntry(id, kind string, payload any) *Entry {
	now := time.Now().UTC()
	return &Entry{
		id:       strings.TrimSpace(id),
		kind:     strings.TrimSpace(kind),
		status:   StatusQueued,
		queuedAt: now,
		payload:  payload,
		cancel:   make(chan struct{}),
	}
}

func (entry *Entry) Snapshot() Snapshot {
	if entry == nil {
		return Snapshot{}
	}
	entry.mu.RLock()
	out := entry.snapshotLocked()
	entry.mu.RUnlock()
	setBackendTaskSnapshotDurations(&out)
	return out
}

func (entry *Entry) snapshotLocked() Snapshot {
	return Snapshot{
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
}

func setBackendTaskSnapshotDurations(snapshot *Snapshot) {
	if snapshot == nil {
		return
	}
	queueLatency, runDuration, totalDuration := backendTaskSnapshotDurations(*snapshot)
	snapshot.QueueLatencyMs = durationMS(queueLatency)
	snapshot.RunDurationMs = durationMS(runDuration)
	snapshot.TotalDurationMs = durationMS(totalDuration)
}

func (entry *Entry) Payload() any {
	if entry == nil {
		return nil
	}
	entry.mu.RLock()
	payload := entry.payload
	entry.mu.RUnlock()
	return payload
}

func (entry *Entry) Cancel() {
	if entry == nil {
		return
	}
	now := time.Now().UTC()
	entry.mu.Lock()
	if backendTaskStatusTerminal(entry.status) {
		entry.mu.Unlock()
		return
	}
	entry.cancelOnce.Do(func() { close(entry.cancel) })
	if entry.status == StatusQueued {
		entry.status = StatusCanceled
		entry.progress = 1
		entry.finishedAt = &now
	}
	entry.mu.Unlock()
}

func (entry *Entry) IsCanceled() bool {
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

func (entry *Entry) markRunning() bool {
	now := time.Now().UTC()
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.status == StatusCanceled || entry.IsCanceled() {
		return false
	}
	entry.status = StatusRunning
	entry.progress = max(entry.progress, 0.01)
	entry.startedAt = &now
	return true
}

func (entry *Entry) SetProgress(progress float64) {
	if entry == nil {
		return
	}
	progress = clampBackendTaskProgress(progress)
	entry.mu.Lock()
	if entry.status == StatusRunning && progress > entry.progress {
		entry.progress = progress
	}
	entry.mu.Unlock()
}

func (entry *Entry) Progress() float64 {
	if entry == nil {
		return 0
	}
	entry.mu.RLock()
	progress := entry.progress
	entry.mu.RUnlock()
	return progress
}

func (entry *Entry) finish(status string, progress float64, message string) {
	_ = entry.finishAt(status, progress, message, time.Now().UTC())
}

func (entry *Entry) finishAt(status string, progress float64, message string, finishedAt time.Time) string {
	progress = clampBackendTaskProgress(progress)
	entry.mu.Lock()
	status = entry.finishLocked(status, progress, message, finishedAt)
	entry.mu.Unlock()
	return status
}

// finishForRuntime publishes the terminal state and claims its metrics/history
// accounting under the same entry lock. This makes duplicate or concurrent
// completion attempts harmless without adding a second runtime-side index.
func (entry *Entry) finishForRuntime(status string, progress float64, message string, finishedAt time.Time) (Snapshot, bool) {
	progress = clampBackendTaskProgress(progress)
	entry.mu.Lock()
	if entry.accounted {
		entry.mu.Unlock()
		return Snapshot{}, false
	}
	entry.finishLocked(status, progress, message, finishedAt)
	entry.accounted = true
	snapshot := entry.snapshotLocked()
	entry.mu.Unlock()
	setBackendTaskSnapshotDurations(&snapshot)
	return snapshot, true
}

func (entry *Entry) finishLocked(status string, progress float64, message string, finishedAt time.Time) string {
	if backendTaskStatusTerminal(entry.status) && entry.finishedAt != nil {
		return entry.status
	}
	if status != StatusCanceled && entry.IsCanceled() {
		status = StatusCanceled
		progress = 1
		message = ""
	}
	entry.status = status
	entry.progress = progress
	entry.finishedAt = &finishedAt
	entry.err = message
	return status
}

func clampBackendTaskProgress(progress float64) float64 {
	if progress < 0 {
		return 0
	}
	if progress > 1 {
		return 1
	}
	return progress
}

func (entry *Entry) setQueueLen(queueLen int) {
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

func backendTaskSnapshotDurations(snapshot Snapshot) (time.Duration, time.Duration, time.Duration) {
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
	return durationMS(total / time.Duration(count))
}

// ID returns the entry identifier.
func (e *Entry) ID() string { return e.id }

// MarkRunning transitions a queued entry into the running state; it reports
// false when the entry was canceled first.
func (e *Entry) MarkRunning() bool { return e.markRunning() }

// CancelChan exposes the entry's cancellation channel for external
// coordination (closed on Cancel or completion).
func (e *Entry) CancelChan() <-chan struct{} { return e.cancel }
