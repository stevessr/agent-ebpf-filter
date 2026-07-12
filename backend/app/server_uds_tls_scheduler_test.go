package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWrapperTLSAttachSchedulerBoundsAndDeduplicates(t *testing.T) {
	started := make(chan wrapperTLSAttachRequest, 4)
	release := make(chan struct{})
	scheduler := newWrapperTLSAttachScheduler(context.Background(), 1, func(_ context.Context, req wrapperTLSAttachRequest) {
		started <- req
		<-release
	})

	first := wrapperTLSAttachRequest{PID: 101, Comm: "codex"}
	if err := scheduler.Submit(first); err != nil {
		t.Fatalf("Submit(first) error = %v", err)
	}
	select {
	case got := <-started:
		if got.PID != first.PID {
			t.Fatalf("started PID = %d", got.PID)
		}
	case <-time.After(time.Second):
		t.Fatal("first attach did not start")
	}
	if err := scheduler.Submit(first); !errors.Is(err, errWrapperTLSAttachDuplicate) {
		t.Fatalf("Submit(duplicate) error = %v", err)
	}
	if err := scheduler.Submit(wrapperTLSAttachRequest{PID: 102}); err != nil {
		t.Fatalf("Submit(queued) error = %v", err)
	}
	if err := scheduler.Submit(wrapperTLSAttachRequest{PID: 103}); !errors.Is(err, errWrapperTLSAttachQueueFull) {
		t.Fatalf("Submit(full) error = %v", err)
	}
	close(release)
	scheduler.Stop()
	if err := scheduler.Submit(wrapperTLSAttachRequest{PID: 104}); !errors.Is(err, errWrapperTLSAttachSchedulerClosed) {
		t.Fatalf("Submit(after stop) error = %v", err)
	}
}

func TestWrapperTLSAttachSchedulerStopCancelsWorker(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	scheduler := newWrapperTLSAttachScheduler(context.Background(), 1, func(ctx context.Context, _ wrapperTLSAttachRequest) {
		once.Do(func() { close(started) })
		<-ctx.Done()
	})
	if err := scheduler.Submit(wrapperTLSAttachRequest{PID: 201}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("attach worker did not start")
	}
	done := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler Stop did not cancel worker")
	}
	scheduler.Stop()
}

func TestWrapperTLSAttachSchedulerRecoversWorkerPanic(t *testing.T) {
	completed := make(chan uint32, 1)
	scheduler := newWrapperTLSAttachScheduler(context.Background(), 2, func(_ context.Context, req wrapperTLSAttachRequest) {
		if req.PID == 301 {
			panic("boom")
		}
		completed <- req.PID
	})
	defer scheduler.Stop()
	if err := scheduler.Submit(wrapperTLSAttachRequest{PID: 301}); err != nil {
		t.Fatalf("Submit(panic) error = %v", err)
	}
	if err := scheduler.Submit(wrapperTLSAttachRequest{PID: 302}); err != nil {
		t.Fatalf("Submit(after panic) error = %v", err)
	}
	select {
	case pid := <-completed:
		if pid != 302 {
			t.Fatalf("completed PID = %d", pid)
		}
	case <-time.After(time.Second):
		t.Fatal("worker did not continue after panic")
	}
}
