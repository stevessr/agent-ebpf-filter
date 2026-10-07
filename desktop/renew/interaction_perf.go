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

func (a *renewApp) mergeEventWindow(incoming []eventSummary, limit int) bool {
	old := a.events
	next := mergeEventSummariesInto(a.eventMergeScratch, old, incoming, limit)
	if len(old) == len(next) {
		if len(old) == 0 || &old[0] == &next[0] {
			return false
		}
	}
	a.events = next
	if cap(old) > 0 {
		a.eventMergeScratch = old[:0]
	} else {
		a.eventMergeScratch = nil
	}
	a.eventsVersion++
	return true
}

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

		batchPool := make(chan []eventSummary, 2)
		nextBatch := func() []eventSummary {
			select {
			case buf := <-batchPool:
				return buf[:0]
			default:
				return make([]eventSummary, 0, eventUIBatchMax)
			}
		}
		recycleBatch := func(buf []eventSummary) {
			for i := range buf {
				buf[i] = eventSummary{}
			}
			select {
			case batchPool <- buf[:0]:
			default:
			}
		}

		batch := nextBatch()
		pausedWrite := 0
		flush := func() {
			if len(batch) == 0 || a.eventUIPaused.Load() {
				return
			}
			pending := batch
			batch = nextBatch()
			pausedWrite = 0
			a.update(func() {
				if a.mergeEventWindow(pending, 1200) {
					a.lastSync = time.Now()
				}
				recycleBatch(pending)
			})
		}

		for {
			select {
			case <-ctx.Done():
				flush()
				return
			case e := <-a.eventUIQueue:
				if a.eventUIPaused.Load() {
					if len(batch) < 1200 {
						batch = append(batch, e)
					} else {
						batch[pausedWrite] = e
						pausedWrite++
						if pausedWrite == len(batch) {
							pausedWrite = 0
						}
						a.eventUIDropped.Add(1)
					}
					continue
				}
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
