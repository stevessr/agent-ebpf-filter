package main

// rotateEventBatchIntoArrivalOrder restores oldest-to-newest ordering after
// the bounded UI batch overwrote its oldest entries. In a full circular
// buffer, overwriteAt identifies the oldest retained event. This matters
// when multiple updates for the same EventID arrive in one frame: the
// event merge deliberately implements last-update-wins based on slice order.
//
// Rotation is in-place and allocation-free. It is also used before growing
// a 512-entry live batch into a larger paused/backpressure buffer.
// appendBoundedEventBatch retains the most recent capacity records without
// allocating when the batch is full. dropped is true only on overwrite.
// Its pointer is local to the batching goroutine; the renderer only receives
// the normalized immutable batch after a scheduled UI update.
func appendBoundedEventBatch(batch []eventSummary, event eventSummary, capacity int, overwriteAt *int) ([]eventSummary, bool) {
	if len(batch) < capacity {
		if *overwriteAt != 0 {
			rotateEventBatchIntoArrivalOrder(batch, *overwriteAt)
			*overwriteAt = 0
		}
		return append(batch, event), false
	}
	if len(batch) == 0 {
		return batch, true
	}
	batch[*overwriteAt] = event
	*overwriteAt = (*overwriteAt + 1) % len(batch)
	return batch, true
}

// takeOldestEventUIBatch bounds the CPU burst after a long pause or UI
// backpressure. It commits at most maxCommit oldest retained events, keeping
// the rest in arrival order for subsequent timer ticks. The caller supplies
// an independent recycled buffer for split commits: ready must not share
// storage with remaining because the UI callback reads ready asynchronously.
//
// A normal <= maxCommit batch keeps the zero-copy fast path.
func takeOldestEventUIBatch(batch []eventSummary, overwriteAt, maxCommit int, recycled []eventSummary) (ready, remaining []eventSummary) {
	rotateEventBatchIntoArrivalOrder(batch, overwriteAt)
	if maxCommit <= 0 || len(batch) <= maxCommit {
		return batch, nil
	}
	if cap(recycled) < maxCommit {
		recycled = make([]eventSummary, 0, maxCommit)
	}
	ready = append(recycled[:0], batch[:maxCommit]...)
	rest := len(batch) - maxCommit
	copy(batch, batch[maxCommit:])
	// Tail slots would otherwise retain payload strings after compaction.
	clear(batch[rest:])
	return ready, batch[:rest]
}

func rotateEventBatchIntoArrivalOrder(events []eventSummary, overwriteAt int) {
	if len(events) < 2 || overwriteAt <= 0 || overwriteAt >= len(events) {
		return
	}
	reverseEventBatch(events[:overwriteAt])
	reverseEventBatch(events[overwriteAt:])
	reverseEventBatch(events)
}

func reverseEventBatch(events []eventSummary) {
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
}
