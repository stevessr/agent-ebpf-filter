package app

import (
	"agent-ebpf-filter/app/recording"
	"bufio"
	"context"
	"errors"
	"log"
	"os"
	"sync"
	"time"
)

const (
	runtimeEventLogQueueSize     = 4096
	runtimeEventLogBufferBytes   = 256 * 1024
	runtimeEventLogFlushBatch    = 128
	runtimeEventLogFlushInterval = 250 * time.Millisecond
	runtimeEventLogFlushRetry    = time.Millisecond
	runtimeEventLogStopTimeout   = 5 * time.Second
	runtimeEventLogRecentTimeout = 10 * time.Second
	runtimeEventLogMaxRecords    = 50000
)

var (
	errRuntimeEventLogQueueFull = errors.New("runtime event log queue is full")
	errRuntimeEventLogStopped   = errors.New("runtime event log writer is not accepting events")
)

type runtimeEventLogStatus struct {
	Active         bool      `json:"active"`
	Stopping       bool      `json:"stopping"`
	QueueLen       int       `json:"queueLen"`
	QueueCap       int       `json:"queueCap"`
	Pending        uint64    `json:"pending"`
	EnqueuedTotal  uint64    `json:"enqueuedTotal"`
	PersistedTotal uint64    `json:"persistedTotal"`
	FailedTotal    uint64    `json:"failedTotal"`
	DroppedTotal   uint64    `json:"droppedTotal"`
	LastFlushedAt  time.Time `json:"lastFlushedAt,omitempty"`
	LastError      string    `json:"lastError,omitempty"`
}

type runtimeEventLogItem struct {
	record CapturedEventRecord
	flush  chan error
}

type runtimeEventLogWriter struct {
	mu             sync.Mutex
	queue          chan runtimeEventLogItem
	stopCh         chan struct{}
	done           chan struct{}
	eventQueueCap  int
	queuedRecords  int
	flushWaiters   int
	accepting      bool
	stopping       bool
	enqueuedTotal  uint64
	persistedTotal uint64
	failedTotal    uint64
	droppedTotal   uint64
	lastFlushedAt  time.Time
	lastError      string
	terminalErr    error
	stopRequested  bool
}

func startRuntimeEventLogWriter(file *os.File) (*runtimeEventLogWriter, error) {
	if file == nil {
		return nil, errors.New("runtime event log file is nil")
	}
	if _, err := file.Stat(); err != nil {
		_ = file.Close()
		return nil, err
	}
	writer := &runtimeEventLogWriter{
		// Keep one control slot outside the advertised event capacity so a
		// flush barrier can make progress while event producers are saturated.
		queue:         make(chan runtimeEventLogItem, runtimeEventLogQueueSize+1),
		stopCh:        make(chan struct{}),
		done:          make(chan struct{}),
		eventQueueCap: runtimeEventLogQueueSize,
		accepting:     true,
	}
	go writer.run(file)
	return writer, nil
}

func (w *runtimeEventLogWriter) Enqueue(record CapturedEventRecord) (bool, error) {
	if w == nil {
		return false, errRuntimeEventLogStopped
	}
	if record.Event == nil {
		return false, errors.New("runtime event log record has no event")
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.accepting {
		err := w.terminalErr
		if err == nil {
			err = errRuntimeEventLogStopped
		}
		w.droppedTotal++
		w.lastError = err.Error()
		return false, err
	}
	eventCapacity := w.eventCapacityLocked()
	queueLimit := cap(w.queue)
	if w.flushWaiters > 0 && queueLimit > 0 {
		queueLimit--
	}
	if w.queuedRecords >= eventCapacity || len(w.queue) >= queueLimit {
		w.droppedTotal++
		w.lastError = errRuntimeEventLogQueueFull.Error()
		return false, errRuntimeEventLogQueueFull
	}
	select {
	case w.queue <- runtimeEventLogItem{record: record}:
		w.enqueuedTotal++
		w.queuedRecords++
		return true, nil
	default:
		w.droppedTotal++
		w.lastError = errRuntimeEventLogQueueFull.Error()
		return false, errRuntimeEventLogQueueFull
	}
}

func (w *runtimeEventLogWriter) FlushContext(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ack := make(chan error, 1)
	done := w.done
	retry := time.NewTicker(runtimeEventLogFlushRetry)
	defer retry.Stop()
	registered := false
	defer func() {
		if !registered {
			return
		}
		w.mu.Lock()
		w.flushWaiters--
		w.mu.Unlock()
	}()
	queued := false
	for !queued {
		w.mu.Lock()
		if !w.accepting {
			if registered {
				w.flushWaiters--
				registered = false
			}
			w.mu.Unlock()
			select {
			case <-done:
				return w.TerminalError()
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if !registered {
			w.flushWaiters++
			registered = true
		}
		select {
		case w.queue <- runtimeEventLogItem{flush: ack}:
			w.flushWaiters--
			registered = false
			queued = true
			w.mu.Unlock()
		default:
			w.mu.Unlock()
		}
		if queued {
			break
		}

		// Never wait for queue capacity while holding mu: a terminal writer
		// needs the same lock to stop accepting and drain queued barriers.
		select {
		case <-done:
			return w.TerminalError()
		case <-ctx.Done():
			return ctx.Err()
		case <-retry.C:
		}
	}

	select {
	case err := <-ack:
		return err
	case <-w.done:
		return w.TerminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *runtimeEventLogWriter) StopContext(ctx context.Context) error {
	if w == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	w.mu.Lock()
	if !w.stopRequested {
		w.accepting = false
		w.stopping = true
		w.stopRequested = true
		close(w.stopCh)
	}
	done := w.done
	w.mu.Unlock()

	select {
	case <-done:
		return w.TerminalError()
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *runtimeEventLogWriter) Status() runtimeEventLogStatus {
	if w == nil {
		return runtimeEventLogStatus{}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	pending := uint64(0)
	completed := w.persistedTotal + w.failedTotal
	if w.enqueuedTotal > completed {
		pending = w.enqueuedTotal - completed
	}
	return runtimeEventLogStatus{
		Active:         w.accepting,
		Stopping:       w.stopping,
		QueueLen:       w.queuedRecords,
		QueueCap:       w.eventCapacityLocked(),
		Pending:        pending,
		EnqueuedTotal:  w.enqueuedTotal,
		PersistedTotal: w.persistedTotal,
		FailedTotal:    w.failedTotal,
		DroppedTotal:   w.droppedTotal,
		LastFlushedAt:  w.lastFlushedAt,
		LastError:      w.lastError,
	}
}

func (w *runtimeEventLogWriter) TerminalError() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.terminalErr
}

func (w *runtimeEventLogWriter) run(file *os.File) {
	buffered := bufio.NewWriterSize(file, runtimeEventLogBufferBytes)
	ticker := time.NewTicker(runtimeEventLogFlushInterval)
	defer ticker.Stop()
	pending := 0
	var terminalErr error

	flushPending := func() error {
		if pending == 0 {
			return nil
		}
		count := pending
		pending = 0
		startedAt := time.Now()
		if err := buffered.Flush(); err != nil {
			w.noteFailed(uint64(count), err, time.Since(startedAt))
			return err
		}
		w.notePersisted(uint64(count), time.Since(startedAt))
		return nil
	}

	processRecord := func(record CapturedEventRecord) error {
		startedAt := time.Now()
		payload, err := recording.MarshalRecord(record)
		if err != nil {
			w.noteFailed(1, err, time.Since(startedAt))
			return nil
		}
		if _, err := buffered.Write(payload); err != nil {
			failed := pending + 1
			pending = 0
			w.noteFailed(uint64(failed), err, time.Since(startedAt))
			return err
		}
		if err := buffered.WriteByte('\n'); err != nil {
			failed := pending + 1
			pending = 0
			w.noteFailed(uint64(failed), err, time.Since(startedAt))
			return err
		}
		pending++
		if pending >= runtimeEventLogFlushBatch || buffered.Buffered() >= runtimeEventLogBufferBytes/2 {
			return flushPending()
		}
		return nil
	}

	processItem := func(item runtimeEventLogItem) error {
		if item.flush != nil {
			err := flushPending()
			item.flush <- err
			return err
		}
		return processRecord(item.record)
	}

	drain := func(reason error) {
		for {
			select {
			case item := <-w.queue:
				w.noteDequeued(item)
				if reason != nil {
					if item.flush != nil {
						item.flush <- reason
					} else {
						w.noteFailed(1, reason, 0)
					}
					continue
				}
				if err := processItem(item); err != nil {
					terminalErr = errors.Join(terminalErr, err)
					reason = err
					w.stopAccepting(err)
				}
			default:
				return
			}
		}
	}

	defer func() {
		if err := flushPending(); err != nil {
			terminalErr = errors.Join(terminalErr, err)
		}
		if err := file.Close(); err != nil {
			terminalErr = errors.Join(terminalErr, err)
		}
		w.finish(terminalErr)
		if terminalErr != nil {
			log.Printf("[WARN] runtime event log writer stopped after failure: %v", terminalErr)
		}
	}()

	for {
		select {
		case <-w.stopCh:
			drain(nil)
			return
		default:
		}
		select {
		case <-w.stopCh:
			drain(nil)
			return
		case <-ticker.C:
			if err := flushPending(); err != nil {
				terminalErr = errors.Join(terminalErr, err)
				w.stopAccepting(err)
				drain(err)
				return
			}
		case item := <-w.queue:
			w.noteDequeued(item)
			if err := processItem(item); err != nil {
				terminalErr = errors.Join(terminalErr, err)
				w.stopAccepting(err)
				drain(err)
				return
			}
		}
	}
}

func (w *runtimeEventLogWriter) eventCapacityLocked() int {
	if w.eventQueueCap > 0 {
		return w.eventQueueCap
	}
	return cap(w.queue)
}

func (w *runtimeEventLogWriter) noteDequeued(item runtimeEventLogItem) {
	if item.flush != nil {
		return
	}
	w.mu.Lock()
	if w.queuedRecords > 0 {
		w.queuedRecords--
	}
	w.mu.Unlock()
}

func (w *runtimeEventLogWriter) stopAccepting(err error) {
	w.mu.Lock()
	if w.accepting {
		w.accepting = false
		w.stopping = true
	}
	if err != nil {
		w.lastError = err.Error()
	}
	w.mu.Unlock()
}

func (w *runtimeEventLogWriter) notePersisted(count uint64, duration time.Duration) {
	if count == 0 {
		return
	}
	w.mu.Lock()
	w.persistedTotal += count
	w.lastFlushedAt = time.Now().UTC()
	w.mu.Unlock()
	collectorMetricsStore.RecordCapturedPersistBatch(count, 0, duration)
}

func (w *runtimeEventLogWriter) noteFailed(count uint64, err error, duration time.Duration) {
	if count == 0 && err == nil {
		return
	}
	w.mu.Lock()
	w.failedTotal += count
	if err != nil {
		w.lastError = err.Error()
	}
	w.mu.Unlock()
	collectorMetricsStore.RecordCapturedPersistBatch(0, count, duration)
}

func (w *runtimeEventLogWriter) finish(err error) {
	w.mu.Lock()
	w.accepting = false
	w.stopping = false
	w.terminalErr = err
	if err != nil {
		w.lastError = err.Error()
	}
	done := w.done
	w.mu.Unlock()
	close(done)
}

func runtimeEventLogStopContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), runtimeEventLogStopTimeout)
}
