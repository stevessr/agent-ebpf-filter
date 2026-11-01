package main

// rotateEventBatchIntoArrivalOrder restores oldest-to-newest ordering after
// the bounded UI batch overwrote its oldest entries. In a full circular
// buffer, overwriteAt identifies the oldest retained event. This matters
// when multiple updates for the same EventID arrive in one frame: the
// event merge deliberately implements last-update-wins based on slice order.
//
// Rotation is in-place and allocation-free. It is also used before growing
// a 512-entry live batch into a larger paused/backpressure buffer.
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
