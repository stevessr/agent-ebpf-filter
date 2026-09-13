// Package workerqueue implements the restartable single-consumer work queue
// shared by the backend's background workers (loop detection, research and
// signal processing).
//
// A Queue owns one consumer goroutine per generation. Start launches a
// generation; Shutdown cancels it and waits for it to exit. If Shutdown gives
// up before the consumer has exited, the generation stays "stopping": it no
// longer accepts work, and a later Start is a no-op until it has actually
// finished, so two consumers never run at once.
package workerqueue

import (
	"context"
	"sync"
)

// EnqueueResult reports why TryEnqueue did or did not accept an item.
type EnqueueResult uint8

const (
	Accepted EnqueueResult = iota
	// NotStarted: no generation is accepting work.
	NotStarted
	// Full: the bounded queue had no room.
	Full
)

// Stats is a point-in-time view of the queue.
type Stats struct {
	// Started is true while a generation exists, including one that is still
	// draining after a timed-out Shutdown.
	Started bool
	Len     int
	Cap     int
}

// Queue is a bounded single-consumer queue with a restartable lifecycle.
// The zero value is ready to use.
type Queue[T any] struct {
	// lifecycleMu serialises Start/Shutdown against the consumer's exit.
	lifecycleMu sync.Mutex
	mu          sync.RWMutex
	queue       chan T
	cancel      context.CancelFunc
	done        chan struct{}
	started     bool
}

// Start launches a consumer generation running run until ctx is cancelled or
// Shutdown is called. It reports whether a new generation was started; it is
// false when one already exists (running or still stopping).
func (q *Queue[T]) Start(ctx context.Context, size int, run func(ctx context.Context, items <-chan T)) bool {
	if q == nil || run == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if size <= 0 {
		size = 1
	}
	q.lifecycleMu.Lock()
	defer q.lifecycleMu.Unlock()
	q.mu.Lock()
	if q.started {
		q.mu.Unlock()
		return false
	}
	workerCtx, cancel := context.WithCancel(ctx)
	queue := make(chan T, size)
	done := make(chan struct{})
	q.queue, q.cancel, q.done, q.started = queue, cancel, done, true
	q.mu.Unlock()

	go func() {
		run(workerCtx, queue)
		q.lifecycleMu.Lock()
		q.mu.Lock()
		if q.done == done {
			q.queue, q.cancel, q.done, q.started = nil, nil, nil, false
		}
		q.mu.Unlock()
		close(done)
		q.lifecycleMu.Unlock()
	}()
	return true
}

// Shutdown stops accepting work, cancels the consumer and waits for it to
// exit or for ctx to expire. On expiry the generation remains registered as
// stopping; a later Shutdown or the consumer's own exit completes it.
func (q *Queue[T]) Shutdown(ctx context.Context) error {
	if q == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	q.lifecycleMu.Lock()
	q.mu.Lock()
	if !q.started {
		q.mu.Unlock()
		q.lifecycleMu.Unlock()
		return nil
	}
	cancel, done := q.cancel, q.done
	q.queue = nil
	q.mu.Unlock()
	q.lifecycleMu.Unlock()
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

// StopAccepting detaches the queue channel from the current generation so
// TryEnqueue reports NotStarted while the consumer drains what it already
// holds. A consumer that wants to drain on cancellation calls this first so
// nothing can be enqueued after the drain finishes. It is a no-op unless
// items is the channel of the live generation.
func (q *Queue[T]) StopAccepting(items <-chan T) {
	if q == nil {
		return
	}
	q.mu.Lock()
	if q.queue != nil && (<-chan T)(q.queue) == items {
		q.queue = nil
	}
	q.mu.Unlock()
}

// TryEnqueue offers item to the running generation without blocking.
func (q *Queue[T]) TryEnqueue(item T) EnqueueResult {
	if q == nil {
		return NotStarted
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	if q.queue == nil {
		return NotStarted
	}
	select {
	case q.queue <- item:
		return Accepted
	default:
		return Full
	}
}

// Stats returns the current lifecycle state and queue occupancy.
func (q *Queue[T]) Stats() Stats {
	if q == nil {
		return Stats{}
	}
	q.mu.RLock()
	defer q.mu.RUnlock()
	stats := Stats{Started: q.started}
	if q.queue != nil {
		stats.Len, stats.Cap = len(q.queue), cap(q.queue)
	}
	return stats
}
