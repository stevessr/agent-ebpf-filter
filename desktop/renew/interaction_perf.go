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
	// Only apply the delta when the risk cache describes the old window.
	// First-time loads and external test fixtures still use the full scan.
	updateRisk := a.riskCacheValid && a.riskCacheVersion == a.eventsVersion
	currentFilter := a.eventFilterCacheKey()
	warmFilter := a.filterCacheValid && a.filterCacheVersion == a.eventsVersion &&
		a.filterCacheKey == currentFilter
	// Root-agent attribution can legitimately change for untouched events
	// when new root evidence arrives. Only row-local filters are incremental.
	aliasFilter := warmFilter && currentFilter.isUnfiltered()
	updateFilter := warmFilter && !aliasFilter &&
		currentFilter.domainTarget == "" && len(a.filterCacheRows) <= 128
	// Only the unfiltered aggregate view is independent of historical
	// selection semantics. Filtered aggregates remain lazily recomputed from
	// the canonical selected rows, never from unrelated event deltas.
	unfiltered := currentFilter.isUnfiltered()
	warmNetwork := unfiltered && a.networkCacheValid &&
		a.networkCacheVersion == a.eventsVersion && a.networkCacheKey == currentFilter &&
		len(a.networkCacheRows) >= incrementalAggregateMinGroups
	warmProcesses := unfiltered && a.processCacheValid &&
		a.processCacheVersion == a.eventsVersion && a.processCacheKey == currentFilter &&
		len(a.processCacheRows) >= incrementalAggregateMinGroups
	trackAggregates := warmNetwork || warmProcesses
	next := mergeEventSummariesIntoWorkspace(a.eventMergeScratch, old, incoming, limit,
		&a.eventMergeWorkspace, updateRisk, updateFilter, trackAggregates)
	defer func() {
		clear(a.eventMergeWorkspace.filterAdded)
		a.eventMergeWorkspace.filterAdded = a.eventMergeWorkspace.filterAdded[:0]
		clear(a.eventMergeWorkspace.filterRemoved)
		clear(a.eventMergeWorkspace.aggregateRemoved)
		a.eventMergeWorkspace.aggregateRemoved = a.eventMergeWorkspace.aggregateRemoved[:0]
	}()
	if len(old) == len(next) {
		if len(old) == 0 || &old[0] == &next[0] {
			return false
		}
	}
	a.events = next
	// A pure limit-only truncation may return a slice of the active buffer.
	// Never recycle that buffer as scratch; the next batch could overwrite
	// live rows before the merge has finished reading them.
	if cap(old) > 0 && (len(old) == 0 || len(next) == 0 || &old[0] != &next[0]) {
		a.eventMergeScratch = old[:0]
	} else {
		a.eventMergeScratch = nil
	}
	a.eventsVersion++
	if aliasFilter {
		a.filterCacheRows = a.events
		a.filterCacheVersion = a.eventsVersion
	} else if updateFilter {
		a.applyIncrementalFilterRows(&a.eventMergeWorkspace)
	}
	if warmNetwork {
		if rows, ok := incrementalNetworkAggregateRows(a.networkCacheRows, a.events,
			a.eventMergeWorkspace.aggregateRemoved, a.eventMergeWorkspace.filterAdded); ok {
			a.networkCacheRows = rows
			a.networkCacheVersion = a.eventsVersion
		}
	}
	if warmProcesses {
		if rows, ok := incrementalProcessAggregateRows(a.processCacheRows, a.events,
			a.eventMergeWorkspace.aggregateRemoved, a.eventMergeWorkspace.filterAdded); ok {
			a.processCacheRows = rows
			a.processCacheVersion = a.eventsVersion
		}
	}
	if updateRisk {
		for bucket, delta := range a.eventMergeWorkspace.riskDelta {
			a.riskCacheCounts[bucket] += delta
		}
		a.riskCacheVersion = a.eventsVersion
	}
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
				// Paused mode can grow a previously wrapped live batch.
				// Normalize its retained arrival order before appending.
				if overwriteAt != 0 {
					rotateEventBatchIntoArrivalOrder(batch, overwriteAt)
					overwriteAt = 0
				}
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
			// The storage is circular after overflow. Restore chronological
			// order before deduplication so the newest same-ID revision wins.
			rotateEventBatchIntoArrivalOrder(batch, overwriteAt)
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
