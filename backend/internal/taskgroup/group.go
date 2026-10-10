// Package taskgroup supervises independent runtime goroutines. The runtime
// must cancel long-lived tasks before calling Wait. A timed-out Wait creates
// no background waiter goroutine, so repeated shutdown checks cannot leak.
package taskgroup

import (
	"context"
	"sync"
)

// Group tracks goroutines started during runtime initialization.
// The zero value is usable. Callers must finish adding tasks before Wait.
type Group struct {
	mu sync.Mutex
	pending int
	done chan struct{}
}

// Go registers and launches a task. Nil groups or functions are no-ops.
func (g *Group) Go(run func()) {
	if g == nil || run == nil {
		return
	}
	g.mu.Lock()
	if g.pending == 0 {
		g.done = make(chan struct{})
	}
	g.pending++
	g.mu.Unlock()
	go func() {
		defer func() {
			g.mu.Lock()
			g.pending--
			if g.pending == 0 {
				close(g.done)
			}
			g.mu.Unlock()
		}()
		run()
	}()
}

// Wait waits for all registered tasks to exit. It does not spawn any helper
// goroutines. If ctx expires, callers may wait again; it does not cancel tasks.
func (g *Group) Wait(ctx context.Context) error {
	if g == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	g.mu.Lock()
	if g.pending == 0 {
		g.mu.Unlock()
		return nil
	}
	done := g.done
	g.mu.Unlock()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
