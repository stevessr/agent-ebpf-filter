package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Stats struct {
	Name                string     `json:"name"`
	Started             bool       `json:"started"`
	Closed              bool       `json:"closed"`
	QueueLen            int        `json:"queueLen"`
	QueueCap            int        `json:"queueCap"`
	TrackedTotal        int        `json:"trackedTotal"`
	EnqueuedTotal       uint64     `json:"enqueuedTotal"`
	CompletedTotal      uint64     `json:"completedTotal"`
	FailedTotal         uint64     `json:"failedTotal"`
	CanceledTotal       uint64     `json:"canceledTotal"`
	PanickedTotal       uint64     `json:"panickedTotal"`
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

type Runtime struct {
	mu       sync.RWMutex
	name     string
	queue    chan *Entry
	done     chan struct{}
	started  bool
	closed   bool
	tasks    map[string]*Entry
	maxItems int
	handler  func(*Entry) error

	terminalHead *Entry
	terminalTail *Entry

	enqueuedTotal      uint64
	completedTotal     uint64
	failedTotal        uint64
	canceledTotal      uint64
	panickedTotal      uint64
	rejectedTotal      uint64
	runDurationTotal   time.Duration
	runDurationSamples uint64
	lastQueueLatency   time.Duration
	lastRunDuration    time.Duration
	lastTotalDuration  time.Duration
	lastStartedAt      time.Time
	lastFinishedAt     time.Time
	lastError          string
	lastRejectReason   string
	updatedAt          time.Time
}

// DefaultQueueSize is the default worker-queue capacity.
const DefaultQueueSize = 2048

func New(name string, maxItems int, handler func(*Entry) error) *Runtime {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "backend"
	}
	if maxItems <= 0 {
		maxItems = 1024
	}
	return &Runtime{
		name:      name,
		tasks:     make(map[string]*Entry),
		maxItems:  maxItems,
		handler:   handler,
		updatedAt: time.Now().UTC(),
	}
}

// NewUnstarted builds a runtime with a pre-allocated queue but no consumer
// goroutine. Embedders that drive execution themselves (and tests that need a
// deterministic full queue) use it; Submit still enforces capacity.
func NewUnstarted(name string, maxItems int, handler func(*Entry) error, queueSize int) *Runtime {
	r := New(name, maxItems, handler)
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	r.mu.Lock()
	r.startLocked(queueSize)
	r.mu.Unlock()
	return r
}

func (r *Runtime) Start(queueSize int) {
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	r.mu.Lock()
	if r.started || r.closed {
		r.mu.Unlock()
		return
	}
	queue, done := r.startLocked(queueSize)
	r.mu.Unlock()
	go r.run(queue, done)
}

func (r *Runtime) startLocked(queueSize int) (chan *Entry, chan struct{}) {
	queue := make(chan *Entry, queueSize)
	done := make(chan struct{})
	r.queue = queue
	r.done = done
	r.started = true
	r.updatedAt = time.Now().UTC()
	return queue, done
}

func (r *Runtime) Submit(entry *Entry) error {
	if entry == nil || strings.TrimSpace(entry.id) == "" {
		return errors.New("backend task entry is invalid")
	}
	r.mu.Lock()
	if r.closed {
		r.noteRejectedLocked("runtime_closed", ErrClosed)
		r.mu.Unlock()
		return ErrClosed
	}
	if _, exists := r.tasks[entry.id]; exists {
		r.noteRejectedLocked("duplicate_id", ErrDuplicateID)
		r.mu.Unlock()
		return ErrDuplicateID
	}
	queue := r.queue
	if queue == nil {
		var done chan struct{}
		queue, done = r.startLocked(DefaultQueueSize)
		go r.run(queue, done)
	}
	r.tasks[entry.id] = entry
	select {
	case queue <- entry:
		entry.setQueueLen(len(queue))
		r.enqueuedTotal++
		r.updatedAt = time.Now().UTC()
		// Prune only after the enqueue succeeds. A rejected submission must not
		// discard useful history. pruneLocked only evicts terminal entries, so
		// queued/running work remains addressable even above maxItems.
		r.pruneLocked()
		r.mu.Unlock()
		return nil
	default:
		if current := r.tasks[entry.id]; current == entry {
			delete(r.tasks, entry.id)
		}
		r.noteRejectedLocked("queue_full", ErrQueueFull)
		r.mu.Unlock()
		return ErrQueueFull
	}
}

func (r *Runtime) noteRejectedLocked(reason string, err error) {
	r.rejectedTotal++
	r.lastRejectReason = reason
	if err != nil {
		r.lastError = err.Error()
	}
	r.updatedAt = time.Now().UTC()
}

func (r *Runtime) Get(id string) (*Entry, bool) {
	r.mu.RLock()
	entry := r.tasks[strings.TrimSpace(id)]
	r.mu.RUnlock()
	return entry, entry != nil
}

func (r *Runtime) Cancel(id string) (*Entry, bool) {
	entry, ok := r.Get(id)
	if !ok {
		return nil, false
	}
	entry.Cancel()
	return entry, true
}

func (r *Runtime) Stats() Stats {
	r.mu.RLock()
	queueLen := 0
	queueCap := 0
	if r.queue != nil {
		queueLen = len(r.queue)
		queueCap = cap(r.queue)
	}
	stats := Stats{
		Name:                r.name,
		Started:             r.started,
		Closed:              r.closed,
		QueueLen:            queueLen,
		QueueCap:            queueCap,
		TrackedTotal:        len(r.tasks),
		EnqueuedTotal:       r.enqueuedTotal,
		CompletedTotal:      r.completedTotal,
		FailedTotal:         r.failedTotal,
		CanceledTotal:       r.canceledTotal,
		PanickedTotal:       r.panickedTotal,
		RejectedTotal:       r.rejectedTotal,
		LastQueueLatencyMs:  durationMS(r.lastQueueLatency),
		LastRunDurationMs:   durationMS(r.lastRunDuration),
		LastTotalDurationMs: durationMS(r.lastTotalDuration),
		AvgRunDurationMs:    backendTaskAverageDurationMs(r.runDurationTotal, r.runDurationSamples),
		LastStartedAt:       ptrTimeIfSet(r.lastStartedAt),
		LastFinishedAt:      ptrTimeIfSet(r.lastFinishedAt),
		LastError:           r.lastError,
		LastRejectReason:    r.lastRejectReason,
		UpdatedAt:           r.updatedAt,
	}
	r.mu.RUnlock()
	return stats
}

func (r *Runtime) run(queue <-chan *Entry, done chan<- struct{}) {
	defer close(done)
	for entry := range queue {
		if entry == nil {
			continue
		}
		if !entry.markRunning() {
			r.completeEntry(entry, StatusCanceled, 1, "", false)
			continue
		}
		err := r.execute(entry)
		if err != nil {
			if errors.Is(err, ErrCanceled) || entry.IsCanceled() {
				r.completeEntry(entry, StatusCanceled, 1, "", false)
				continue
			}
			r.completeEntry(entry, StatusFailed, entry.Progress(), err.Error(), errors.Is(err, ErrHandlerPanic))
			continue
		}
		if entry.IsCanceled() {
			r.completeEntry(entry, StatusCanceled, 1, "", false)
			continue
		}
		r.completeEntry(entry, StatusSucceeded, 1, "", false)
	}
}

// Shutdown rejects new submissions, cancels tracked active tasks, closes the
// queue, and waits for the worker to finish draining canceled entries.
func (r *Runtime) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.started = false
		for _, entry := range r.tasks {
			entry.Cancel()
		}
		if r.queue != nil {
			close(r.queue)
		}
		r.updatedAt = time.Now().UTC()
	}
	done := r.done
	r.mu.Unlock()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("shutdown backend task runtime %q: %w", r.name, ctx.Err())
	}
}

func (r *Runtime) execute(entry *Entry) (err error) {
	if r.handler == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = newBackendTaskPanicError(recovered)
		}
	}()
	return r.handler(entry)
}

func newBackendTaskPanicError(recovered any) error {
	return fmt.Errorf("%w: %v", ErrHandlerPanic, recovered)
}

func (r *Runtime) completeEntry(entry *Entry, status string, progress float64, message string, panicked bool) {
	if entry == nil {
		return
	}
	snapshot, accounted := entry.finishForRuntime(status, progress, message, time.Now().UTC())
	if !accounted {
		return
	}
	r.noteFinished(entry, snapshot, panicked && snapshot.Status == StatusFailed)
}

func (r *Runtime) noteFinished(entry *Entry, snapshot Snapshot, panicked bool) {
	queueLatency, runDuration, totalDuration := backendTaskSnapshotDurations(snapshot)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.completedTotal++
	if snapshot.StartedAt != nil {
		r.runDurationTotal += runDuration
		r.runDurationSamples++
	}
	r.lastQueueLatency = queueLatency
	r.lastRunDuration = runDuration
	r.lastTotalDuration = totalDuration
	if snapshot.StartedAt != nil {
		r.lastStartedAt = *snapshot.StartedAt
	}
	if snapshot.FinishedAt != nil {
		r.lastFinishedAt = *snapshot.FinishedAt
	}
	switch snapshot.Status {
	case StatusFailed:
		r.failedTotal++
	case StatusCanceled:
		r.canceledTotal++
	}
	if panicked {
		r.panickedTotal++
	}
	r.lastError = snapshot.Error
	r.updatedAt = time.Now().UTC()
	r.trackTerminalLocked(entry)
	r.pruneLocked()
}

func (r *Runtime) pruneLocked() {
	for len(r.tasks) > r.maxItems {
		entry := r.terminalHead
		if entry == nil {
			return
		}
		r.terminalHead = entry.terminalNext
		entry.terminalNext = nil
		if r.terminalHead == nil {
			r.terminalTail = nil
		}
		if current := r.tasks[entry.id]; current == entry {
			delete(r.tasks, entry.id)
		}
	}
}

func (r *Runtime) trackTerminalLocked(entry *Entry) {
	if entry.id == "" {
		return
	}
	if current := r.tasks[entry.id]; current != entry {
		return
	}
	if r.terminalTail == nil {
		r.terminalHead = entry
	} else {
		r.terminalTail.terminalNext = entry
	}
	r.terminalTail = entry
}

func backendTaskStatusTerminal(status string) bool {
	switch status {
	case StatusSucceeded, StatusFailed, StatusCanceled:
		return true
	default:
		return false
	}
}

// ScanBatchSize bounds temporary allocations and lock hold times while
// workers process large manual replay requests.
const ScanBatchSize = 256

// MaxQueueSize is the hard ceiling for worker queues.
const MaxQueueSize = 65536

// NormalizeQueueSize clamps a requested queue size against the default.
func NormalizeQueueSize(queueSize, defaultSize int) int {
	if defaultSize <= 0 {
		defaultSize = 1
	}
	if queueSize <= 0 {
		queueSize = defaultSize
	}
	if queueSize > MaxQueueSize {
		queueSize = MaxQueueSize
	}
	return queueSize
}

func NewPanicError(recovered any) error {
	return fmt.Errorf("%w: %v", ErrHandlerPanic, recovered)
}

// Track registers an entry with the runtime bookkeeping without enqueueing
// it; used by embedders that manage their own execution loop.
func (r *Runtime) Track(entry *Entry) {
	if entry == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tasks[entry.id] = entry
}

// CompleteEntry finalizes an entry that was executed outside the runtime
// worker goroutine.
func (r *Runtime) CompleteEntry(entry *Entry, status string, progress float64, message string, panicked bool) {
	r.completeEntry(entry, status, progress, message, panicked)
}
