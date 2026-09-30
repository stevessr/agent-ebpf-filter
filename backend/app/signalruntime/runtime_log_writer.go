package signalruntime

import (
	"agent-ebpf-filter/app/tasks"
	"agent-ebpf-filter/internal/workerqueue"
	"context"
	"sync"
	"time"
)

const signalProgramLogWriterDefaultQueueSize = 2048

type signalProgramLogWriterStatus struct {
	Running         bool       `json:"running"`
	Accepting       bool       `json:"accepting"`
	QueueLen        int        `json:"queueLen"`
	QueueCap        int        `json:"queueCap"`
	EnqueuedTotal   uint64     `json:"enqueuedTotal"`
	CompletedTotal  uint64     `json:"completedTotal"`
	PersistedTotal  uint64     `json:"persistedTotal"`
	FailedTotal     uint64     `json:"failedTotal"`
	DroppedTotal    uint64     `json:"droppedTotal"`
	LastError       string     `json:"lastError,omitempty"`
	LastEnqueuedAt  *time.Time `json:"lastEnqueuedAt,omitempty"`
	LastCompletedAt *time.Time `json:"lastCompletedAt,omitempty"`
	LastDroppedAt   *time.Time `json:"lastDroppedAt,omitempty"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type signalProgramLogWriter struct {
	queue workerqueue.Queue[signalProgramLogWorkItem]
	mu    sync.RWMutex

	enqueuedTotal   uint64
	completedTotal  uint64
	persistedTotal  uint64
	failedTotal     uint64
	droppedTotal    uint64
	lastError       string
	lastEnqueuedAt  time.Time
	lastCompletedAt time.Time
	lastDroppedAt   time.Time
	updatedAt       time.Time
}

var signalProgramLogWriterStore = newSignalProgramLogWriter()

func newSignalProgramLogWriter() *signalProgramLogWriter {
	return &signalProgramLogWriter{updatedAt: time.Now().UTC()}
}

func startSignalProgramLogWriter(ctx context.Context) {
	settings := SnapshotSettingsHook().SignalProcessing
	NormalizeSettings(&settings)
	signalProgramLogWriterStore.Start(ctx, settings.QueueSize)
}

func (w *signalProgramLogWriter) Start(ctx context.Context, queueSize int) {
	if w == nil {
		return
	}
	queueSize = tasks.NormalizeQueueSize(queueSize, signalProgramLogWriterDefaultQueueSize)
	if w.queue.Start(ctx, queueSize, func(ctx context.Context, items <-chan signalProgramLogWorkItem) {
		w.run(ctx, items)
		w.touch()
	}) {
		w.touch()
	}
}

func (w *signalProgramLogWriter) touch() {
	w.mu.Lock()
	w.updatedAt = time.Now().UTC()
	w.mu.Unlock()
}

func (w *signalProgramLogWriter) run(ctx context.Context, queue <-chan signalProgramLogWorkItem) {
	for {
		if ctx.Err() != nil {
			w.stopAccepting(queue)
			w.drain(queue)
			return
		}
		select {
		case <-ctx.Done():
			w.stopAccepting(queue)
			w.drain(queue)
			return
		case item := <-queue:
			w.process(item)
		}
	}
}

// stopAccepting closes the door before draining so no item can be accepted
// after the drain has finished.
func (w *signalProgramLogWriter) stopAccepting(queue <-chan signalProgramLogWorkItem) {
	if w == nil {
		return
	}
	w.queue.StopAccepting(queue)
	w.touch()
}

func (w *signalProgramLogWriter) drain(queue <-chan signalProgramLogWorkItem) {
	for {
		select {
		case item := <-queue:
			w.process(item)
		default:
			return
		}
	}
}

func (w *signalProgramLogWriter) process(item signalProgramLogWorkItem) {
	persisted, failed, lastError := persistSignalProgramLogWork(item)
	now := time.Now().UTC()
	w.mu.Lock()
	w.completedTotal++
	w.persistedTotal += uint64(persisted)
	w.failedTotal += uint64(failed)
	if lastError != "" {
		w.lastError = lastError
	}
	w.lastCompletedAt = now
	w.updatedAt = now
	w.mu.Unlock()
}

// Enqueue returns whether the item was accepted and whether a writer generation
// is active. Callers may use synchronous compatibility only when active is
// false; an active but full/stopping writer intentionally drops instead of
// reintroducing disk I/O on the event ingestion path.
func (w *signalProgramLogWriter) Enqueue(item signalProgramLogWorkItem) (accepted, active bool) {
	if w == nil {
		return false, false
	}
	if !w.queue.Stats().Started {
		return false, false
	}
	now := time.Now().UTC()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.queue.TryEnqueue(item) == workerqueue.Accepted {
		w.enqueuedTotal++
		w.lastEnqueuedAt = now
		w.updatedAt = now
		return true, true
	}
	w.droppedTotal++
	w.lastError = "signal program log writer queue is full or stopping"
	w.lastDroppedAt = now
	w.updatedAt = now
	return false, true
}

func (w *signalProgramLogWriter) Shutdown(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if w.queue.Stats().Started {
		w.touch()
	}
	return w.queue.Shutdown(ctx)
}

func (w *signalProgramLogWriter) Status() signalProgramLogWriterStatus {
	if w == nil {
		return signalProgramLogWriterStatus{}
	}
	queueStats := w.queue.Stats()
	w.mu.RLock()
	defer w.mu.RUnlock()
	status := signalProgramLogWriterStatus{
		Running:        queueStats.Started,
		Accepting:      queueStats.Cap > 0,
		QueueLen:       queueStats.Len,
		QueueCap:       queueStats.Cap,
		EnqueuedTotal:  w.enqueuedTotal,
		CompletedTotal: w.completedTotal,
		PersistedTotal: w.persistedTotal,
		FailedTotal:    w.failedTotal,
		DroppedTotal:   w.droppedTotal,
		LastError:      w.lastError,
		UpdatedAt:      w.updatedAt,
	}
	status.LastEnqueuedAt = signalProgramLogTimePointer(w.lastEnqueuedAt)
	status.LastCompletedAt = signalProgramLogTimePointer(w.lastCompletedAt)
	status.LastDroppedAt = signalProgramLogTimePointer(w.lastDroppedAt)
	return status
}

func signalProgramLogTimePointer(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	value = value.UTC()
	return &value
}

// LogWriter returns the shared signal program-log writer.
func LogWriter() *signalProgramLogWriter { return signalProgramLogWriterStore }

// StartProgramLogWriter launches the shared program-log writer loop.
func StartProgramLogWriter(ctx context.Context) { startSignalProgramLogWriter(ctx) }
