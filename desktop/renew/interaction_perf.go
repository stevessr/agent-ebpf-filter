package main

import (
	"context"
	"sync/atomic"
	"time"
)

const (
	eventUIFlushInterval  = 16 * time.Millisecond
	eventUIFrameCapacity  = 512
	// In a paused backlog, 640 balances P99 peak latency with recovery CPU
	// and drains a 1200-event window in two UI commits.
	eventUIResumeCommitCapacity = 640
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
				return make([]eventSummary, 0, eventUIResumeCommitCapacity)
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
			var dropped bool
			batch, dropped = appendBoundedEventBatch(batch, e, capacity, &overwriteAt)
			if dropped {
				a.eventUIDropped.Add(1)
				a.eventUIBatchDropped.Add(1)
			}
			a.updateEventUIBatchPressure(len(batch))
		}

		flush := func(final bool) {
			if len(batch) == 0 || a.eventUIPaused.Load() {
				return
			}
			if !a.eventUICommitPending.CompareAndSwap(false, true) {
				a.eventUIBusyTicks.Add(1)
				return
			}
			// Keep normal <=512-record commits zero-copy. On resume, split
			// a full pause backlog at 640 records (1200 -> 640+560),
			// preserving oldest-first event order and requiring two ticks.
			var pending []eventSummary
			if len(batch) > eventUIResumeCommitCapacity && !final {
				pending, batch = takeOldestEventUIBatch(batch, overwriteAt,
					eventUIResumeCommitCapacity, nextBatch())
			} else {
				rotateEventBatchIntoArrivalOrder(batch, overwriteAt)
				pending = batch
				batch = nextBatch()
			}
			overwriteAt = 0
			a.eventUIBatchBuffered.Store(int64(len(batch)))
			a.update(func() {
				defer a.eventUICommitPending.Store(false)
				if a.mergeEventWindow(pending, 1200) {
					a.lastSync = time.Now()
				}
				a.eventUICommits.Add(1)
				a.eventUICommitted.Add(uint64(len(pending)))
				a.eventUILastCommit.Store(int64(len(pending)))
				recycleBatch(pending)
			})
		}

		for {
			select {
			case <-ctx.Done():
				// No following timer tick is guaranteed after shutdown;
				// commit the full final retained batch if one can be queued.
				flush(true)
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
				flush(false)
			}
		}
	}()
}

// queueEventSummary never blocks the native IPC reader on a full UI queue.
// The producer mutex makes discard-oldest + enqueue linearizable among all
// producers; without this a competing producer can steal the freed slot,
// causing the incoming event to be discarded without being counted.
func (a *renewApp) queueEventSummary(e eventSummary) {
	a.eventUIQueueMu.Lock()
	defer a.eventUIQueueMu.Unlock()
	queue := a.eventUIQueue
	if queue == nil {
		return
	}
	select {
	case queue <- e:
		return
	default:
	}
	// The queue was full at the first attempt. A consumer may have
	// drained it in the meantime; only count a real discarded record.
	select {
	case <-queue:
		a.eventUIDropped.Add(1)
		a.eventUIQueueDropped.Add(1)
	default:
	}
	select {
	case queue <- e:
		return
	default:
		// Defensive fallback: if external code ever sends directly to the
		// channel, account for this undelivered event as well.
		a.eventUIDropped.Add(1)
		a.eventUIQueueDropped.Add(1)
	}
}

// Read-only native diagnostic counters, safe across the IPC/GUI goroutines.
func (a *renewApp) updateEventUIBatchPressure(n int) {
	current := int64(n)
	a.eventUIBatchBuffered.Store(current)
	for peak := a.eventUIBatchPeak.Load(); current > peak; peak = a.eventUIBatchPeak.Load() {
		if a.eventUIBatchPeak.CompareAndSwap(peak, current) {
			break
		}
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
