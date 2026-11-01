package main

import (
	"math/rand"
	"reflect"
	"strconv"
	"testing"
)

func batchIDs(events []eventSummary) []string {
	out := make([]string, len(events))
	for i, event := range events {
		out[i] = event.EventID
	}
	return out
}

func TestBoundedEventBatchKeepsNewestRevisionAfterWrap(t *testing.T) {
	var batch []eventSummary
	at := 0
	updates := []eventSummary{
		{EventID: "first", RiskScore: 1},
		{EventID: "same", RiskScore: 11},
		{EventID: "third", RiskScore: 3},
		{EventID: "fourth", RiskScore: 4},
		// The newest same-ID record overwrites physical slot 0 while
		// an old copy remains in slot 1.
		{EventID: "same", RiskScore: 99},
	}
	for i, e := range updates {
		var dropped bool
		batch, dropped = appendBoundedEventBatch(batch, e, 4, &at)
		if dropped != (i >= 4) {
			t.Fatalf("update %d dropped=%t", i, dropped)
		}
	}
	if at != 1 {
		t.Fatalf("oldest ring slot=%d, want 1", at)
	}
	rotateEventBatchIntoArrivalOrder(batch, at)
	if got, want := batchIDs(batch), []string{"same", "third", "fourth", "same"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("retained arrival order=%v, want %v", got, want)
	}
	merged := mergeEventSummaries(nil, batch, 1200)
	if len(merged) != 3 {
		t.Fatalf("same-ID records were not deduplicated: %v", batchIDs(merged))
	}
	for _, e := range merged {
		if e.EventID == "same" {
			if e.RiskScore != 99 {
				t.Fatalf("old revision unexpectedly won: %+v", e)
			}
			return
		}
	}
	t.Fatal("missing same-ID event")
}

func TestBoundedEventBatchGrowthAndRepeatedOverwrites(t *testing.T) {
	rng := rand.New(rand.NewSource(20261101))
	for trial := 0; trial < 140; trial++ {
		capacity := 3 + rng.Intn(48)
		var batch []eventSummary
		var retained []eventSummary
		at := 0
		for i := 0; i < 250; i++ {
			if i == 110 {
				capacity += 50 // paused mode enlarges an already wrapped batch
			}
			e := eventSummary{
				EventID: strconv.Itoa(rng.Intn(25)),
				RiskScore: float64(i + 1),
			}
			if len(retained) < capacity {
				retained = append(retained, e)
			} else {
				copy(retained, retained[1:])
				retained[len(retained)-1] = e
			}
			var dropped bool
			batch, dropped = appendBoundedEventBatch(batch, e, capacity, &at)
			if dropped && len(batch) != capacity {
				t.Fatalf("trial %d, step %d: overwrote before reaching capacity", trial, i)
			}
			// A full buffer is oldest-first only after normalization.
			// Check on a clone to leave the active overwrite index intact.
			normalized := append([]eventSummary(nil), batch...)
			rotateEventBatchIntoArrivalOrder(normalized, at)
			// After growing, the buffer preserves only its old retained
			// events plus the subsequent append/overwrites.
			if !reflect.DeepEqual(normalized, retained) {
				t.Fatalf("trial %d, step %d: got %v, want %v", trial, i, batchIDs(normalized), batchIDs(retained))
			}
		}
	}
}

func TestEventMergeComputesSearchTextFromLatestRevision(t *testing.T) {
	batch := make([]eventSummary, 0, 512)
	for i := 0; i < 400; i++ {
		batch = append(batch, eventSummary{
			EventID: "revised", ReceivedAtMS: int64(i),
			Target: "/tmp/old-" + strconv.Itoa(i),
			Comm: "codex", PID: i + 100,
		})
	}
	batch = append(batch, eventSummary{
		EventID: "revised", ReceivedAtMS: 700,
		Target: "/tmp/new-final", Comm: "codex", PID: 999,
	})
	batch = append(batch, eventSummary{
		EventID: "unrelated", ReceivedAtMS: 900,
		Target: "/tmp/other", Comm: "codex", PID: 4,
	})
	merged := mergeEventSummaries(nil, batch, 1200)
	if len(merged) != 2 {
		t.Fatalf("expected 2 unique IDs, got %d", len(merged))
	}
	for _, e := range merged {
		if e.SearchText != buildEventSearchText(e) {
			t.Fatalf("search corpus was not computed from retained summary: %+v", e)
		}
		if e.EventID == "revised" && (e.PID != 999 || e.Target != "/tmp/new-final") {
			t.Fatalf("latest same-ID revision was not kept: %+v", e)
		}
	}
}

func TestTakeOldestEventUIBatchPreservesBacklogAndIndependentStorage(t *testing.T) {
	var batch []eventSummary
	var stream []eventSummary
	at := 0
	// Force a wrapped 1200-event backlog, including repeat IDs in the
	// portion crossing the circular-buffer boundary.
	for i := 0; i < 1500; i++ {
		e := eventSummary{
			EventID: strconv.Itoa(i % 1150),
			RiskScore: float64(i),
			Target: "/tmp/event-" + strconv.Itoa(i),
		}
		stream = append(stream, e)
		batch, _ = appendBoundedEventBatch(batch, e, 1200, &at)
	}
	want := stream[len(stream)-1200:]
	first, pending := takeOldestEventUIBatch(batch, at, 512, nil)
	if len(first) != 512 || len(pending) != 688 {
		t.Fatalf("split size=%d+%d, want 512+688", len(first), len(pending))
	}
	if !reflect.DeepEqual(first, want[:512]) || !reflect.DeepEqual(pending, want[512:]) {
		t.Fatal("first recovery split failed to restore chronology")
	}
	firstSnapshot := append([]eventSummary(nil), first...)
	// Continue receiving live events while the previous 512 summaries are
	// owned by the (potentially asynchronous) UI commit callback.
	for i := 1500; i < 1510; i++ {
		e := eventSummary{EventID: strconv.Itoa(i), RiskScore: float64(i)}
		pending, _ = appendBoundedEventBatch(pending, e, 1200, new(int))
		want = append(want, e)
	}
	if !reflect.DeepEqual(first, firstSnapshot) {
		t.Fatal("new arrivals overwrote the in-flight UI commit buffer")
	}
	second, pending := takeOldestEventUIBatch(pending, 0, 512, nil)
	if !reflect.DeepEqual(second, want[512:1024]) || len(pending) != 186 {
		t.Fatal("second recovery chunk lost order or events")
	}
	third, tail := takeOldestEventUIBatch(pending, 0, 512, nil)
	if tail != nil || !reflect.DeepEqual(third, want[1024:]) {
		t.Fatal("third recovery chunk lost residual events")
	}
	got := append(append(append([]eventSummary(nil), first...), second...), third...)
	if !reflect.DeepEqual(got, want) {
		t.Fatal("recovered stream differs from newest retained chronological stream")
	}
}

func TestTakeOldestEventUIBatchFastPathAndReferenceClearing(t *testing.T) {
	batch := []eventSummary{
		{EventID: "old", Target: "/tmp/old"},
		{EventID: "middle", Target: "/tmp/middle"},
		{EventID: "new", Target: "/tmp/new"},
	}
	ready, rest := takeOldestEventUIBatch(batch, 0, 512, nil)
	if rest != nil || &ready[0] != &batch[0] {
		t.Fatal("small batches must use the existing zero-copy commit path")
	}
	big := make([]eventSummary, 5, 8)
	for i := range big {
		big[i] = eventSummary{EventID: strconv.Itoa(i), Target: "target-" + strconv.Itoa(i)}
	}
	first, rest := takeOldestEventUIBatch(big, 0, 2, nil)
	if len(first) != 2 || len(rest) != 3 {
		t.Fatalf("split sizes %d, %d", len(first), len(rest))
	}
	if big[3].Target != "" || big[4].Target != "" {
		t.Fatal("compacted backing buffer retains references to transferred rows")
	}
}

func TestProductionResumeCapUsesTwoOrderedCommits(t *testing.T) {
	full := make([]eventSummary, eventUIPauseCapacity)
	for i := range full {
		full[i] = eventSummary{
			EventID: strconv.Itoa(i),
			Target: "/tmp/event-" + strconv.Itoa(i),
		}
	}
	scratch := make([]eventSummary, 0, eventUIResumeCommitCapacity)
	first, rest := takeOldestEventUIBatch(full, 0, eventUIResumeCommitCapacity, scratch)
	if len(first) != 640 || len(rest) != 560 {
		t.Fatalf("actual resume chunking must be 640+560, got %d+%d", len(first), len(rest))
	}
	if &first[0] != &scratch[:cap(scratch)][0] {
		t.Fatal("resume fast path did not use the recycled commit buffer")
	}
	for i, event := range first {
		if event.EventID != strconv.Itoa(i) {
			t.Fatalf("first chunk out of order at %d: %q", i, event.EventID)
		}
	}
	second, tail := takeOldestEventUIBatch(rest, 0, eventUIResumeCommitCapacity, nil)
	if tail != nil || len(second) != 560 {
		t.Fatalf("second commit should finish backlog, left=%d", len(tail))
	}
	for i, event := range second {
		if event.EventID != strconv.Itoa(i+640) {
			t.Fatalf("second chunk out of order at %d: %q", i, event.EventID)
		}
	}
}
