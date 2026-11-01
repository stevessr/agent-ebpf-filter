package main

import (
	"math/rand"
	"reflect"
	"strconv"
	"testing"
)

// Reference caches are deliberately cold after each merge. This compares the
// incremental path against the real, full native filter and aggregation code.
func equalNetworkAggregates(a, b []networkAggregate) bool {
	if len(a) != len(b) {
		return false
	}
	byKey := make(map[string]networkAggregate, len(b))
	for _, row := range b {
		byKey[row.Target] = row
	}
	for _, row := range a {
		if !reflect.DeepEqual(row, byKey[row.Target]) {
			return false
		}
	}
	return true
}

func equalProcessAggregates(a, b []processAggregate) bool {
	if len(a) != len(b) {
		return false
	}
	byKey := make(map[int]processAggregate, len(b))
	for _, row := range b {
		byKey[row.PID] = row
	}
	for _, row := range a {
		if !reflect.DeepEqual(row, byKey[row.PID]) {
			return false
		}
	}
	return true
}

func TestSelectiveIncrementalFilterMatchesFullScanAndAggregates(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*renewApp)
	}{
		{"pid-and-search", func(a *renewApp) {
			a.eventPIDFilter = 1017
			a.search = "codex"
		}},
		{"risk-and-decision", func(a *renewApp) {
			a.eventMinRiskScore = 90
			a.eventRiskFilter = "高风险"
			a.eventDecisionFilter = "已阻断"
		}},
		{"type-and-session", func(a *renewApp) {
			a.eventTypeFilter = "network_connect"
			a.eventSessionFilter = "run-1"
		}},
		{"root-and-target", func(a *renewApp) {
			a.eventRootPIDFilter = 1005
			a.eventTargetFilter = "example.org"
		}},
		{"no-match", func(a *renewApp) { a.search = "not-in-events-12345" }},
		{"unfiltered", func(a *renewApp) {}},
		{"owner-domain-fallback", func(a *renewApp) {
			a.eventDomainAgent = "codex"
			a.eventDomainTarget = "example.org"
			a.eventDomainKind = "dns"
		}},
	}
	for caseIndex, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newRenewApp("http://127.0.0.1:8080")
			ref := newRenewApp("http://127.0.0.1:8080")
			tc.setup(a)
			tc.setup(ref)
			rng := rand.New(rand.NewSource(27291 + int64(caseIndex)))
			initial := make([]eventSummary, 1200)
			for i := range initial {
				initial[i] = eventSummary{
					EventID: strconv.Itoa(9000-i), PID: 1000 + i%50,
					RootAgentPID: 1000 + i%50, ReceivedAtMS: int64(9000-i),
					RiskScore: float64(i%101), Decision: []string{"ALLOW", "ALERT", "DENY"}[i%3],
					Type: []string{"file_write", "network_connect"}[i%2],
					Target: []string{"/tmp/record", "example.org"}[i%2],
					AgentRunID: "run-" + strconv.Itoa(i%5),
					Comm: "codex", Tag: "codex", Network: i%2 == 1,
					NetBytes: int64(i + 1),
				}
			}
			a.mergeEventWindow(initial, 1200)
			ref.events = a.events
			_ = a.filteredEvents()
			// Populate network and process caches as the real pages do.
			a.filteredNetworkRows()
			a.filteredProcessAggregateRows()
			for iteration := 0; iteration < 150; iteration++ {
				var batch []eventSummary
				for i := 0; i < 1+rng.Intn(8); i++ {
					id := rng.Intn(2400)
					batch = append(batch, eventSummary{
						EventID: strconv.Itoa(id + 7000),
						PID: 1000 + rng.Intn(50),
						RootAgentPID: 1000 + rng.Intn(50),
						ReceivedAtMS: int64(rng.Intn(12000)),
						RiskScore: float64(rng.Intn(101)),
						Decision: []string{"ALLOW", "ALERT", "DENY"}[rng.Intn(3)],
						Type: []string{"file_write", "network_connect"}[rng.Intn(2)],
						Target: []string{"example.org", "/tmp/record"}[rng.Intn(2)],
						AgentRunID: "run-" + strconv.Itoa(rng.Intn(5)),
						Comm: "codex", Tag: "codex",
						Network: rng.Intn(2) == 1,
						NetBytes: int64(rng.Intn(2048)),
					})
				}
				a.mergeEventWindow(batch, 1200)
				ref.events = a.events
				ref.eventsVersion++
				ref.filterCacheValid = false
				ref.networkCacheValid = false
				ref.processCacheValid = false
				if tc.name == "owner-domain-fallback" && a.filterCacheVersion == a.eventsVersion {
					t.Fatalf("owner/domain filter must not receive an incremental cache version at step %d", iteration)
				}
				got, want := a.filteredEvents(), ref.filteredEvents()
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("step %d incremental events differ from full scan: got=%v want=%v", iteration, idsForIncrementalTest(got), idsForIncrementalTest(want))
				}
				if gotN, wantN := a.filteredNetworkRows(), aggregateNetwork(want); !equalNetworkAggregates(gotN, wantN) {
					t.Fatalf("step %d network aggregates differ from full scan: got=%+v want=%+v", iteration, gotN, wantN)
				}
				if gotP, wantP := a.filteredProcessAggregateRows(), aggregateProcesses(want); !equalProcessAggregates(gotP, wantP) {
					t.Fatalf("step %d process aggregates differ from full scan: got=%+v want=%+v", iteration, gotP, wantP)
				}
				if a.filterCacheVersion != a.eventsVersion {
					t.Fatalf("step %d active filter cache is not current", iteration)
				}
				if len(a.eventMergeWorkspace.filterAdded) != 0 || len(a.eventMergeWorkspace.filterRemoved) != 0 {
					t.Fatalf("step %d change scratch retained event references", iteration)
				}
			}
		})
	}
}

func idsForIncrementalTest(events []eventSummary) []string {
	ids := make([]string, len(events))
	for i, e := range events {
		ids[i] = e.EventID
	}
	return ids
}

func TestIncrementalFilterInvalidatesOnUserChangeAndOldDomainEvidence(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "codex"
	a.mergeEventWindow([]eventSummary{
		{EventID: "1", PID: 101, Comm: "codex", ReceivedAtMS: 10},
	}, 1200)
	if got := a.filteredEvents(); len(got) != 1 {
		t.Fatalf("initial search: %+v", got)
	}
	a.search = "other"
	a.mergeEventWindow([]eventSummary{{EventID: "2", PID: 102, Comm: "codex", ReceivedAtMS: 20}}, 1200)
	if got := a.filteredEvents(); len(got) != 0 {
		t.Fatalf("edited query reused stale cached rows: %+v", got)
	}
	a.search = "codex"
	a.eventPIDFilter = 102
	if got := a.filteredEvents(); len(got) != 1 || got[0].EventID != "2" {
		t.Fatalf("PID change reused stale cache: %+v", got)
	}
	a.eventDomainTarget = "example.org"
	a.eventDomainAgent = "codex"
	a.eventDomainKind = "dns"
	_ = a.filteredEvents()
	oldVersion := a.filterCacheVersion
	a.mergeEventWindow([]eventSummary{{EventID: "3", PID: 103, Comm: "codex", ReceivedAtMS: 30}}, 1200)
	if a.filterCacheVersion != oldVersion {
		t.Fatal("owner-domain evidence must force a full rescan")
	}
}

func TestSelectiveCacheHandlesLimitOnlyTrimAndExplicitRevisions(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.eventPIDFilter = 101
	a.events = []eventSummary{
		{EventID: "5", PID: 101, ReceivedAtMS: 5, RiskScore: 10},
		{EventID: "4", PID: 102, ReceivedAtMS: 4, RiskScore: 20},
		{EventID: "3", PID: 101, ReceivedAtMS: 3, RiskScore: 30},
		{EventID: "2", PID: 101, ReceivedAtMS: 2, RiskScore: 40},
		{EventID: "1", PID: 102, ReceivedAtMS: 1, RiskScore: 50},
	}
	if ids := idsForIncrementalTest(a.filteredEvents()); !reflect.DeepEqual(ids, []string{"5", "3", "2"}) {
		t.Fatalf("initial filtered slice: %v", ids)
	}
	if !a.mergeEventWindow(nil, 3) {
		t.Fatal("limit-only truncation should change the window")
	}
	if ids := idsForIncrementalTest(a.filteredEvents()); !reflect.DeepEqual(ids, []string{"5", "3"}) {
		t.Fatalf("trim retained evicted EventID: %v", ids)
	}
	// A revision to the middle match moves it to the top, and an amended
	// PID causes it to leave the filtered view.
	if !a.mergeEventWindow([]eventSummary{{EventID: "3", PID: 101, ReceivedAtMS: 100, RiskScore: 90}}, 3) {
		t.Fatal("same-ID revision did not merge")
	}
	if ids := idsForIncrementalTest(a.filteredEvents()); !reflect.DeepEqual(ids, []string{"3", "5"}) {
		t.Fatalf("revision reordered incorrectly: %v", ids)
	}
	if !a.mergeEventWindow([]eventSummary{{EventID: "3", PID: 999, ReceivedAtMS: 101}}, 3) {
		t.Fatal("revised event with different PID did not merge")
	}
	if ids := idsForIncrementalTest(a.filteredEvents()); !reflect.DeepEqual(ids, []string{"5"}) {
		t.Fatalf("revision kept a no-longer-matching event: %v", ids)
	}
}

func TestBroadFilterStillUsesSafeFullScanFallback(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "codex"
	initial := make([]eventSummary, 300)
	for i := range initial {
		initial[i] = eventSummary{EventID: strconv.Itoa(300-i), PID: 100+i, Comm: "codex", ReceivedAtMS: int64(300-i)}
	}
	a.mergeEventWindow(initial, 1200)
	if rows := a.filteredEvents(); len(rows) != 300 {
		t.Fatalf("broad search should match all initial records: %d", len(rows))
	}
	previousVersion := a.filterCacheVersion
	a.mergeEventWindow([]eventSummary{{EventID: "next", PID: 101, Comm: "codex", ReceivedAtMS: 1000}}, 1200)
	if a.filterCacheVersion != previousVersion {
		t.Fatal("broad filter should not eagerly update a large cached slice")
	}
	if rows := a.filteredEvents(); len(rows) != 301 || rows[0].EventID != "next" {
		t.Fatalf("full-scan fallback returned stale events: %d, first=%s", len(rows), rows[0].EventID)
	}
}

func BenchmarkRenewSelectiveFilterAfterMerge(b *testing.B) {
	for _, tc := range []struct {
		name        string
		incremental bool
	}{
		{"full-window-recompute", false},
		{"incremental-filter", true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			a := newRenewApp("http://127.0.0.1:8080")
			a.eventPIDFilter = 1017
			a.search = "codex"
			initial := make([]eventSummary, 1200)
			for i := range initial {
				initial[i] = eventSummary{
					EventID: strconv.Itoa(9000 - i),
					PID: 1000 + i%50,
					Comm: "codex", Type: "file_write",
					RiskScore: 20,
					Target: "/tmp/workspace",
					ReceivedAtMS: int64(9000 - i),
				}
			}
			batch := make([]eventSummary, 16)
			for i := range batch {
				batch[i] = eventSummary{
					EventID: strconv.Itoa(12000 + i),
					PID: 1000 + i,
					Comm: "codex", Type: "file_write",
					RiskScore: 20,
					Target: "/tmp/workspace",
					ReceivedAtMS: int64(13000 + i),
				}
			}
			a.mergeEventWindow(initial, 1200)
			_ = a.filteredEvents()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !tc.incremental {
					a.filterCacheValid = false
				}
				a.mergeEventWindow(batch, 1200)
				benchmarkFilteredRows = a.filteredEvents()
				benchmarkNetworkRows = a.filteredNetworkRows()
				benchmarkProcessRows = a.filteredProcessAggregateRows()
			}
		})
	}
}

var benchmarkFilteredRows []eventSummary
var benchmarkNetworkRows []networkAggregate
var benchmarkProcessRows []processAggregate
