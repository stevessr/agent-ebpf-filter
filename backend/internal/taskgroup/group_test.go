package taskgroup

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestWaitUntilAllTasksExit(t *testing.T) {
	var group Group
	const tasks = 12
	var finished atomic.Int32
	for i := 0; i < tasks; i++ {
		group.Go(func() {
			finished.Add(1)
		})
	}
	if err := group.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if finished.Load() != tasks {
		t.Fatalf("finished %d, want %d", finished.Load(), tasks)
	}
}

func TestWaitCancelledDoesNotBreakLaterJoin(t *testing.T) {
	var group Group
	release := make(chan struct{})
	group.Go(func() { <-release })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := group.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wait = %v", err)
	}
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := group.Wait(ctx2); err != nil {
		t.Fatalf("joining after cancel: %v", err)
	}
}

func TestNilGroupAndFunc(t *testing.T) {
	var group *Group
	group.Go(func() { t.Error("unexpected launch") })
	if err := group.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	var valid Group
	valid.Go(nil)
	if err := valid.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}
