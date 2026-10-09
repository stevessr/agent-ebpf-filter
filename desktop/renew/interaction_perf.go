package main

import (
	"context"
	"sync/atomic"
	"time"
)

const (
	eventUIFlushInterval  = 16 * time.Millisecond
	eventUIFrameCapacity  = 512
	eventUIPauseCapacity  = 1200
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
	// Preserve the selected event identity instead of leaving the table
	// pointing at a different row when newer summaries arrive at the top.
	// Only run this O(window) lookup while an Events table row is selected.
	if a.page == "事件" && a.eventSelected >= 0 && a.inspectorSelectedID != "" {
		a.eventSelected = -1
		for i, event := range a.filteredEvents() {
			if event.EventID == a.inspectorSelectedID {
				a.eventSelected = i
				break
			}
		}
	}
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
				return make([]eventSummary, 0, eventUIFrameCapacity)
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
		overwriteAt := 0
		appendLatest := func(e eventSummary, capacity int) {
			if len(batch) < capacity {
				batch = append(batch, e)
				return
			}
			batch[overwriteAt] = e
			overwriteAt++
			if overwriteAt == len(batch) {
				overwriteAt = 0
			}
			a.eventUIDropped.Add(1)
		}
		flush := func() {
			if len(batch) == 0 || a.eventUIPaused.Load() {
				return
			}
			if !a.eventUICommitPending.CompareAndSwap(false, true) {
				return
			}
			pending := batch
			batch = nextBatch()
			overwriteAt = 0
			a.update(func() {
				defer a.eventUICommitPending.Store(false)
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
				capacity := eventUIFrameCapacity
				if a.eventUIPaused.Load() || len(batch) > eventUIFrameCapacity {
					capacity = eventUIPauseCapacity
				}
				appendLatest(e, capacity)
			case <-ticker.C:
				// The ticker is the only normal flush point. Event floods can
				// fill/overwrite the bounded frame batch, but never schedule
				// multiple Window.Update callbacks inside one frame interval.
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
type eventCommitFlag = atomic.Bool
