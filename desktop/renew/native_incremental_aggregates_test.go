package main

import (
	"fmt"
	"math/rand"
	"strconv"
	"testing"
)

func aggregateTestEvent(id string, stamp int64, target string, pid int, network bool, score float64) eventSummary {
	return eventSummary{
		EventID: id, ReceivedAtMS: stamp, Target: target,
		PID: pid, PPID: pid / 2, Comm: "codex-" + strconv.Itoa(pid%13),
		Network: network, Type: "network_connect", RiskScore: score,
		NetBytes: int64(pid%100 - 20), Decision: "ALLOW",
	}
}

func verifyAggregateParity(t *testing.T, a *renewApp, step int) {
	t.Helper()
	wantNet := aggregateNetwork(a.filteredEvents())
	gotNet := a.filteredNetworkRows()
	if !equalNetworkAggregates(gotNet, wantNet) {
		t.Fatalf("step %d network aggregates drifted: rows=%d want=%d", step, len(gotNet), len(wantNet))
	}
	wantProc := aggregateProcesses(a.filteredEvents())
	gotProc := a.filteredProcessAggregateRows()
	if !equalProcessAggregates(gotProc, wantProc) {
		t.Fatalf("step %d process aggregates drifted: rows=%d want=%d", step, len(gotProc), len(wantProc))
	}
}

func TestDeltaAggregatesMatchFullRebuildAcrossRevisionsAndEvictions(t *testing.T) {
	const limit = 1200
	rng := rand.New(rand.NewSource(110126))
	a := newRenewApp("http://127.0.0.1:8080")
	initial := make([]eventSummary, limit)
	for i := range initial {
		initial[i] = aggregateTestEvent(
			strconv.Itoa(2000-i), int64(4000-i),
			"host-"+strconv.Itoa(i%720)+".example", 10000+i%720,
			true, float64(i%101),
		)
	}
	if !a.mergeEventWindow(initial, limit) {
		t.Fatal("initial window merge failed")
	}
	verifyAggregateParity(t, a, 0)
	networkIncremental, processIncremental := 0, 0
	for step := 1; step <= 280; step++ {
		batch := make([]eventSummary, 0, 1+rng.Intn(6))
		for n := 0; n < cap(batch); n++ {
			id := strconv.Itoa(1500 + rng.Intn(1400)) // revisions, old and new IDs
			target := "host-" + strconv.Itoa(rng.Intn(750)) + ".example"
			if rng.Intn(20) == 0 {
				target = " " // remove the network group contribution
			}
			e := aggregateTestEvent(id, int64(rng.Intn(7000)),
				target, 10000+rng.Intn(750), rng.Intn(6) != 0,
				float64(rng.Intn(121)-20))
			e.Type = []string{"network_connect", "file_write", "dns"}[rng.Intn(3)]
			if rng.Intn(9) == 0 {
				e.PID = 0
			}
			if rng.Intn(5) == 0 {
				e.Comm = ""
			}
			if rng.Intn(7) == 0 {
				e.PPID = 0
			}
			batch = append(batch, e)
		}
		beforeNet, beforeProc := a.networkCacheVersion, a.processCacheVersion
		if !a.mergeEventWindow(batch, limit) {
			t.Fatalf("step %d merge reported no update", step)
		}
		if a.networkCacheVersion != beforeNet {
			networkIncremental++
		}
		if a.processCacheVersion != beforeProc {
			processIncremental++
		}
		verifyAggregateParity(t, a, step)
		for _, event := range a.eventMergeWorkspace.aggregateRemoved {
			if event.EventID != "" || event.Target != "" || event.Comm != "" {
				t.Fatalf("step %d retained old event in aggregate scratch", step)
			}
		}
	}
	if networkIncremental < 100 || processIncremental < 100 {
		t.Fatalf("delta repair was not exercised sufficiently: network=%d processes=%d", networkIncremental, processIncremental)
	}
}

func TestAggregateDeltaRestoresOldExtremaAndDistinctMembers(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	initial := make([]eventSummary, 0, 100)
	for i := 0; i < 100; i++ {
		initial = append(initial, aggregateTestEvent(
			strconv.Itoa(i+1), int64(i+1), "host-"+strconv.Itoa(i)+".example",
			10000+i, true, float64(i%50)))
	}
	// Populate one group with multiple contributors. Removing the max risk,
	// latest timestamp, and one distinct PID/Comm must restore older values.
	initial = append(initial,
		aggregateTestEvent("hot-old", 900, "hot.example", 101, true, 40),
		aggregateTestEvent("hot-new", 1000, "hot.example", 102, true, 99),
	)
	a.mergeEventWindow(initial, 1000)
	a.filteredNetworkRows()
	a.filteredProcessAggregateRows()
	if !a.mergeEventWindow([]eventSummary{
		aggregateTestEvent("hot-new", 1100, "other.example", 103, true, 10),
	}, 1000) {
		t.Fatal("revision did not change event window")
	}
	verifyAggregateParity(t, a, 1)
	var hot *networkAggregate
	for i := range a.networkCacheRows {
		if a.networkCacheRows[i].Target == "hot.example" {
			hot = &a.networkCacheRows[i]
			break
		}
	}
	if hot == nil || hot.Events != 1 || hot.Risk != 40 || hot.LastMS != 900 {
		t.Fatalf("failed to restore previous maxima: %+v", hot)
	}
	if _, exists := hot.PIDs[102]; exists {
		t.Fatal("stale PID retained after event moved to another target")
	}
	if !a.mergeEventWindow([]eventSummary{
		aggregateTestEvent("hot-old", 1200, " ", 0, false, 0),
	}, 1000) {
		t.Fatal("second revision did not change event window")
	}
	verifyAggregateParity(t, a, 2)
	for _, row := range a.filteredNetworkRows() {
		if row.Target == "hot.example" {
			t.Fatal("empty network group survived after its last event moved")
		}
	}
}

func TestAggregateDeltaFallbackForSmallGroupsAndFilteredQueries(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.mergeEventWindow([]eventSummary{
		aggregateTestEvent("1", 10, "one.example", 101, true, 30),
		aggregateTestEvent("2", 20, "two.example", 102, true, 70),
	}, 1200)
	a.filteredNetworkRows()
	a.filteredProcessAggregateRows()
	oldNetwork, oldProcess := a.networkCacheVersion, a.processCacheVersion
	a.mergeEventWindow([]eventSummary{aggregateTestEvent("3", 30, "third.example", 103, true, 90)}, 1200)
	if a.networkCacheVersion != oldNetwork || a.processCacheVersion != oldProcess {
		t.Fatal("small tables should defer to cheap original full aggregation")
	}
	verifyAggregateParity(t, a, 1)

	a.eventPIDFilter = 101
	a.filteredNetworkRows()
	a.filteredProcessAggregateRows()
	oldNetwork, oldProcess = a.networkCacheVersion, a.processCacheVersion
	a.mergeEventWindow([]eventSummary{aggregateTestEvent("4", 40, "fourth.example", 101, true, 10)}, 1200)
	if a.networkCacheVersion != oldNetwork || a.processCacheVersion != oldProcess {
		t.Fatal("filtered aggregate caches must not be updated from unfiltered event deltas")
	}
	verifyAggregateParity(t, a, 2)
}

var incrementalNetworkSink []networkAggregate
var incrementalProcessSink []processAggregate

func BenchmarkRenewAggregateDeltaViews(b *testing.B) {
	for _, tc := range []struct {
		name string
		incremental bool
	}{
		{"full-aggregate-rescan", false},
		{"touched-group-recompute", true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			a := newRenewApp("http://127.0.0.1:8080")
			initial := make([]eventSummary, 1200)
			for i := range initial {
				initial[i] = aggregateTestEvent(
					strconv.Itoa(1200-i), int64(1200-i),
					fmt.Sprintf("svc-%d.example", i), 10000+i,
					true, float64(i%101))
			}
			a.mergeEventWindow(initial, 1200)
			a.filteredNetworkRows()
			a.filteredProcessAggregateRows()
			batch := make([]eventSummary, 4)
			for j := range batch {
				batch[j] = aggregateTestEvent(
					strconv.Itoa(2000+j), int64(2000+j),
					fmt.Sprintf("new-svc-%d.example", j), 20000+j,
					true, float64(j*20))
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !tc.incremental {
					// Only disable aggregate caches: event merge and risk
					// paths remain identical between benchmark variants.
					a.networkCacheValid = false
					a.processCacheValid = false
				}
				a.mergeEventWindow(batch, 1200)
				incrementalNetworkSink = a.filteredNetworkRows()
				incrementalProcessSink = a.filteredProcessAggregateRows()
			}
		})
	}
}
