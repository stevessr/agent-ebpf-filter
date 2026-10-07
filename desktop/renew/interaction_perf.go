package main

import (
	"context"
	"sync/atomic"
	"time"
)

const (
	eventUIFlushInterval = 16 * time.Millisecond
	eventUIBatchMax       = 96
	eventUIQueueSize      = 2048
)

func (a *renewApp) startEventUIBatcher(ctx context.Context) {
	if !a.eventUIBatcherStarted.CompareAndSwap(false, true) {
		return
	}
	if a.eventUIQueue == nil {
		a.eventUIQueue = make(chan eventSummary, eventUIQueueSize)
	}
	go func() {
		defer a.eventUIBatcherStarted.Store(false)
		ticker := time.NewTicker(eventUIFlushInterval)
		defer ticker.Stop()

		batch := make([]eventSummary, 0, eventUIBatchMax)
		flush := func() {
			if len(batch) == 0 {
				return
			}
			if a.eventUIPaused.Load() {
				if len(batch) > 1200 {
					dropped := len(batch) - 1200
					copy(batch, batch[dropped:])
					batch = batch[:1200]
					a.eventUIDropped.Add(uint64(dropped))
				}
				return
			}
			pending := append([]eventSummary(nil), batch...)
			batch = batch[:0]
			a.update(func() {
				a.events = mergeEventSummaries(a.events, pending, 1200)
				a.eventsVersion++
				a.lastSync = time.Now()
			})
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case e := <-a.eventUIQueue:
				batch = append(batch, e)
				if len(batch) >= eventUIBatchMax {
					flush()
				}
			case <-ticker.C:
				flush()
			}
		}
	}()
}

func (a *renewApp) queueEventSummary(e eventSummary) {
	if a.eventUIQueue == nil {
		return
	}
	select {
	case a.eventUIQueue <- e:
	default:
		// Never let rendering backpressure stall the native IPC reader. Drop
		// the oldest queued summary and keep the most recent activity visible.
		select {
		case <-a.eventUIQueue:
		default:
		}
		select {
		case a.eventUIQueue <- e:
		default:
		}
		a.eventUIDropped.Add(1)
	}
}

func (a *renewApp) queueEventSummaries(events []eventSummary) {
	for _, event := range events {
		a.queueEventSummary(event)
	}
}

func (a *renewApp) eventUIDroppedCount() uint64 {
	return a.eventUIDropped.Load()
}

type eventDropCounter = atomic.Uint64
type eventPauseFlag = atomic.Bool
type eventBatcherFlag = atomic.Bool
