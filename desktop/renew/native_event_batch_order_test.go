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
		var stream []eventSummary
		at := 0
		for i := 0; i < 250; i++ {
			if i == 110 {
				capacity += 50 // paused mode enlarges an already wrapped batch
			}
			e := eventSummary{
				EventID: strconv.Itoa(rng.Intn(25)),
				RiskScore: float64(i + 1),
			}
			stream = append(stream, e)
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
			retained := min(capacity, len(stream))
			want := stream[len(stream)-retained:]
			if !reflect.DeepEqual(normalized, want) {
				t.Fatalf("trial %d, step %d: got %v, want %v", trial, i, batchIDs(normalized), batchIDs(want))
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
