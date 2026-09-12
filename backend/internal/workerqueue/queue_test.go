package workerqueue

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func drain(consumed *atomic.Int64) func(ctx context.Context, items <-chan int) {
	return func(ctx context.Context, items <-chan int) {
		for {
			select {
			case <-ctx.Done():
				return
			case <-items:
				consumed.Add(1)
			}
		}
	}
}

func TestQueueStartEnqueueShutdownRestart(t *testing.T) {
	var q Queue[int]
	var consumed atomic.Int64
	if q.TryEnqueue(1) != NotStarted {
		t.Fatal("unstarted queue accepted work")
	}
	ctx, cancel := context.WithCancel(context.Background())
	if !q.Start(ctx, 8, drain(&consumed)) {
		t.Fatal("Start() did not launch a generation")
	}
	if q.Start(ctx, 8, drain(&consumed)) {
		t.Fatal("second Start() launched a duplicate consumer")
	}
	if stats := q.Stats(); !stats.Started || stats.Cap != 8 {
		t.Fatalf("Stats() = %+v", stats)
	}
	if q.TryEnqueue(1) != Accepted {
		t.Fatal("running queue rejected work")
	}
	deadline := time.Now().Add(time.Second)
	for consumed.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if consumed.Load() != 1 {
		t.Fatal("consumer did not receive the item")
	}

	cancel()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := q.Shutdown(waitCtx); err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if stats := q.Stats(); stats.Started || stats.Cap != 0 {
		t.Fatalf("Stats() after shutdown = %+v", stats)
	}
	if q.TryEnqueue(2) != NotStarted {
		t.Fatal("stopped queue accepted work")
	}
	if err := q.Shutdown(waitCtx); err != nil {
		t.Fatalf("idempotent Shutdown() error = %v", err)
	}

	if !q.Start(context.Background(), 4, drain(&consumed)) {
		t.Fatal("restart did not launch a generation")
	}
	if err := q.Shutdown(waitCtx); err != nil {
		t.Fatalf("Shutdown() after restart error = %v", err)
	}
}

func TestQueueFullIsReportedWithoutBlocking(t *testing.T) {
	var q Queue[int]
	block := make(chan struct{})
	q.Start(context.Background(), 1, func(ctx context.Context, items <-chan int) {
		<-block
		for range items {
		}
	})
	t.Cleanup(func() { close(block) })
	if q.TryEnqueue(1) != Accepted {
		t.Fatal("first item rejected")
	}
	started := time.Now()
	if q.TryEnqueue(2) != Full {
		t.Fatal("full queue did not report Full")
	}
	if time.Since(started) > 50*time.Millisecond {
		t.Fatal("TryEnqueue blocked on a full queue")
	}
}

// A Shutdown that times out must leave the generation registered as stopping:
// no new work is accepted, and Start must not launch a second consumer until
// the first has actually exited.
func TestQueueShutdownTimeoutKeepsStoppingGeneration(t *testing.T) {
	var q Queue[int]
	release := make(chan struct{})
	q.Start(context.Background(), 1, func(ctx context.Context, items <-chan int) {
		<-ctx.Done()
		<-release
	})
	expired, cancel := context.WithCancel(context.Background())
	cancel()
	if err := q.Shutdown(expired); !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() error = %v, want context cancellation", err)
	}
	if stats := q.Stats(); !stats.Started || stats.Cap != 0 {
		t.Fatalf("stopping generation stats = %+v, want started with no queue", stats)
	}
	if q.TryEnqueue(1) != NotStarted {
		t.Fatal("stopping generation accepted work")
	}
	if q.Start(context.Background(), 4, func(context.Context, <-chan int) {}) {
		t.Fatal("Start() replaced a generation that is still stopping")
	}

	close(release)
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	if err := q.Shutdown(waitCtx); err != nil {
		t.Fatalf("second Shutdown() error = %v", err)
	}
	if q.Stats().Started {
		t.Fatal("generation still registered after the consumer exited")
	}
	if !q.Start(context.Background(), 4, func(ctx context.Context, _ <-chan int) { <-ctx.Done() }) {
		t.Fatal("Start() refused after the stopping generation finished")
	}
	q.Shutdown(waitCtx)
}

func TestNilQueueIsInert(t *testing.T) {
	var q *Queue[int]
	if q.Start(context.Background(), 1, nil) || q.TryEnqueue(1) != NotStarted || q.Shutdown(context.Background()) != nil || q.Stats().Started {
		t.Fatal("nil queue is not inert")
	}
}
