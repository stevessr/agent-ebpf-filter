// Package taskgroup supervises independent runtime goroutines. The runtime
// is responsible for cancelling long-lived tasks before calling Wait.
// No task launched by this group is detached from its wait lifecycle.
package taskgroup

import (
	"context"
	"sync"
)

// Group tracks goroutines started during runtime initialization.
// The zero value is usable. Callers must finish adding tasks before Wait.
type Group struct {
	wg sync.WaitGroup
}

// Go registers and launches a task. Nil groups or functions are no-ops.
func (g *Group) Go(run func()) {
	if g == nil || run == nil {
		return
	}
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		run()
	}()
}

// Wait waits for the group to exit, bounded by the supplied context.
// A timed-out Wait does not cancel tasks or allow unsafe reuse of the group.
func (g *Group) Wait(ctx context.Context) error {
	if g == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
