package main

import (
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// referenceWindow is deliberately independent of the production two-way
// merge. It models the final ID-keyed window with a full map + sort.
func referenceWindow(old, incoming []eventSummary, limit int) []eventSummary {
	byID := make(map[string]eventSummary, len(old)+len(incoming))
	for _, e := range old {
		if id := strings.TrimSpace(e.EventID); id != "" {
			byID[id] = e
		}
	}
	for _, e := range incoming {
		if id := strings.TrimSpace(e.EventID); id != "" {
			e.EventID = id
			byID[id] = e
		}
	}
	out := make([]eventSummary, 0, len(byID))
	for _, e := range byID {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return eventSummaryBefore(out[i], out[j]) })
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func expectedRiskCounts(events []eventSummary) [3]int {
	var counts [3]int
	for _, e := range events {
		counts[eventRiskBucket(e)]++
	}
	return counts
}

func TestIncrementalRiskCountsAgainstIndependentWindow(t *testing.T) {
	const limit = 64
	rng := rand.New(rand.NewSource(81094))
	a := newRenewApp("http://127.0.0.1:8080")
	var reference []eventSummary
	// A warm risk cache must be updated in-place by every genuine merge,
	// including duplicate-ID revisions and window eviction.
	if got := expectedRiskCounts(a.events); got != [3]int{} {
		t.Fatalf("initial risk counts: %v", got)
	}
	a.riskCounts()
	for iteration := 0; iteration < 350; iteration++ {
		batch := make([]eventSummary, 0, 1+rng.Intn(9))
		for k, n := 0, 1+rng.Intn(9); k < n; k++ {
			identity := rng.Intn(130)
			stamp := int64(rng.Intn(1500))
			decision := []string{"ALLOW", "ALERT", "BLOCK", "DENY", ""}[rng.Intn(5)]
			batch = append(batch, eventSummary{
				EventID: strconv.Itoa(identity), PID: 1000 + identity,
				ReceivedAtMS: stamp, RiskScore: float64(rng.Intn(101)),
				Decision: decision, Type: "file_write", Comm: "codex",
			})
		}
		reference = referenceWindow(reference, batch, limit)
		if !a.mergeEventWindow(batch, limit) {
			t.Fatalf("iteration %d with valid IDs should produce a new window", iteration)
		}
		actualIDs := make([]string, len(a.events))
		referenceIDs := make([]string, len(reference))
		for i := range a.events {
			actualIDs[i] = a.events[i].EventID
			referenceIDs[i] = reference[i].EventID
			if a.events[i].RiskScore != reference[i].RiskScore ||
				a.events[i].ReceivedAtMS != reference[i].ReceivedAtMS ||
				a.events[i].Decision != reference[i].Decision {
				t.Fatalf("iteration %d event %d differed: got=%+v, want=%+v", iteration, i, a.events[i], reference[i])
			}
		}
		if !reflect.DeepEqual(actualIDs, referenceIDs) {
			t.Fatalf("iteration %d mismatched window IDs: got %v, want %v", iteration, actualIDs, referenceIDs)
		}
		n, att, high := a.riskCounts()
		want := expectedRiskCounts(reference)
		if got := [3]int{n, att, high}; got != want {
			t.Fatalf("iteration %d risk cache drift: got %v, want %v", iteration, got, want)
		}
		if !a.riskCacheValid || a.riskCacheVersion != a.eventsVersion {
			t.Fatalf("iteration %d risk cache was not advanced with the merge", iteration)
		}
		if len(a.eventMergeWorkspace.replacements) != 0 {
			t.Fatalf("iteration %d retained stale replacement IDs", iteration)
		}
		for _, e := range a.eventMergeWorkspace.fresh {
			if e.EventID != "" || e.Target != "" {
				t.Fatalf("iteration %d retained batch data in sort scratch", iteration)
			}
		}
	}
}

func TestIncrementalRiskCacheFallsBackOnColdCacheAndInvalidBatch(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{{EventID: "old", RiskScore: 90, ReceivedAtMS: 1}}
	// Cold cache needs the exact full scan on first access.
	if !a.mergeEventWindow([]eventSummary{{EventID: "new", RiskScore: 65, ReceivedAtMS: 2}}, 64) {
		t.Fatal("new event did not merge")
	}
	if a.riskCacheValid {
		t.Fatal("cold risk cache must not be marked valid based on a delta")
	}
	if normal, att, high := a.riskCounts(); normal != 0 || att != 1 || high != 1 {
		t.Fatalf("cold risk cache mismatch: %d, %d, %d", normal, att, high)
	}
	ver := a.eventsVersion
	if a.mergeEventWindow([]eventSummary{{EventID: "   ", RiskScore: 100}}, 64) {
		t.Fatal("invalid-only event batch unexpectedly changed the window")
	}
	if ver != a.eventsVersion {
		t.Fatal("invalid-only batch advanced the event version")
	}
	if n, att, high := a.riskCounts(); n != 0 || att != 1 || high != 1 {
		t.Fatalf("invalid batch changed risk counts: %d, %d, %d", n, att, high)
	}
}

func TestTruncatedEventWindowDoesNotRecycleActiveBackingArray(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "3", ReceivedAtMS: 3, RiskScore: 90},
		{EventID: "2", ReceivedAtMS: 2, RiskScore: 60},
		{EventID: "1", ReceivedAtMS: 1, RiskScore: 0},
	}
	a.riskCounts()
	if !a.mergeEventWindow(nil, 2) || len(a.events) != 2 {
		t.Fatalf("limit-only trim did not reduce the window: %+v", a.events)
	}
	if cap(a.eventMergeScratch) != 0 {
		t.Fatal("active backing array was incorrectly exposed as merge scratch")
	}
	if n, att, high := a.riskCounts(); n != 0 || att != 1 || high != 1 {
		t.Fatalf("limit-only trim kept evicted event in cache: %d %d %d", n, att, high)
	}
	if !a.mergeEventWindow([]eventSummary{{EventID: "4", ReceivedAtMS: 4, RiskScore: 0}}, 2) {
		t.Fatal("merge after trim failed")
	}
	if a.events[0].EventID != "4" || a.events[1].EventID != "3" {
		t.Fatalf("active window was overwritten by scratch: %+v", a.events)
	}
	if n, att, high := a.riskCounts(); n != 1 || att != 0 || high != 1 {
		t.Fatalf("merge after trim produced stale risk counts: %d %d %d", n, att, high)
	}
}

var mergedRiskSink [3]int
var mergedEventSink []eventSummary

func BenchmarkRenewIncrementalEventMerge(b *testing.B) {
	initial := make([]eventSummary, 1200)
	for i := range initial {
		initial[i] = eventSummary{
			EventID: strconv.Itoa(1200 - i), ReceivedAtMS: int64(1200 - i),
			PID: i + 300, Comm: "codex", Type: "file_write", RiskScore: float64(i % 101),
			SearchText: "codex /tmp/workspace",
		}
	}
	batch := make([]eventSummary, 16)
	for i := range batch {
		batch[i] = eventSummary{EventID: strconv.Itoa(10000 + i), ReceivedAtMS: int64(10000 + i), Comm: "codex", RiskScore: float64(i * 7), Type: "file_write"}
	}

	b.Run("previous-merge-then-risk-scan", func(b *testing.B) {
		current := append([]eventSummary(nil), initial...)
		scratch := make([]eventSummary, 0, 1200)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			next := mergeEventSummariesInto(scratch, current, batch, 1200)
			var counts [3]int
			for _, event := range next {
				counts[eventRiskBucket(event)]++
			}
			scratch, current = current[:0], next
			mergedRiskSink = counts
		}
		mergedEventSink = current
	})
	b.Run("scratch-reuse-and-risk-delta", func(b *testing.B) {
		a := newRenewApp("http://127.0.0.1:8080")
		a.events = append([]eventSummary(nil), initial...)
		a.riskCounts()
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			a.mergeEventWindow(batch, 1200)
		}
		mergedEventSink = a.events
		mergedRiskSink = a.riskCacheCounts
	})
}
