package main

import (
	"strings"
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestAggregateNetworkUsesBoundedSummaryFields(t *testing.T) {
	rows := aggregateNetwork([]eventSummary{
		{PID: 10, Comm: "codex", Type: "NETWORK_CONNECT", Target: "example.com", Network: true, NetBytes: 12, RiskScore: 20, ReceivedAtMS: 10},
		{PID: 11, Comm: "curl", Type: "DNS_QUERY", Target: "example.com", Network: true, NetBytes: 8, RiskScore: 70, ReceivedAtMS: 20},
		{PID: 12, Comm: "bash", Type: "WRITE", Target: "/tmp/x", ReceivedAtMS: 30},
	})
	if len(rows) != 1 {
		t.Fatalf("got %d rows, want 1", len(rows))
	}
	if rows[0].Target != "example.com" || rows[0].Events != 2 || rows[0].Bytes != 20 || rows[0].Risk != 70 {
		t.Fatalf("unexpected row: %+v", rows[0])
	}
	if len(rows[0].PIDs) != 2 {
		t.Fatalf("got %d PIDs, want 2", len(rows[0].PIDs))
	}
}

func TestAggregateProcesses(t *testing.T) {
	rows := aggregateProcesses([]eventSummary{
		{PID: 42, PPID: 1, Comm: "codex", ReceivedAtMS: 10, RiskScore: 10},
		{PID: 42, PPID: 1, Comm: "codex", ReceivedAtMS: 20, RiskScore: 80},
		{PID: 7, Comm: "node", ReceivedAtMS: 5},
	})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].PID != 42 || rows[0].Events != 2 || rows[0].Risk != 80 {
		t.Fatalf("unexpected first row: %+v", rows[0])
	}
}

func TestDisabledSliceIsStable(t *testing.T) {
	got := disabledSlice(map[int]bool{34: true, 2: true, 10: false, 6: true})
	want := []int{2, 6, 34}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestMergeEventSummariesDeduplicatesAndSorts(t *testing.T) {
	got := mergeEventSummaries(
		[]eventSummary{{EventID: "old", ReceivedAtMS: 10}, {EventID: "same", ReceivedAtMS: 20, Comm: "old"}},
		[]eventSummary{{EventID: "new", ReceivedAtMS: 30}, {EventID: "same", ReceivedAtMS: 40, Comm: "fresh"}},
		3,
	)
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].EventID != "same" || got[0].Comm != "fresh" || got[1].EventID != "new" {
		t.Fatalf("unexpected merge order: %#v", got)
	}
}


func TestMergeEventSummariesCachesSearchTextAndMaintainsWindow(t *testing.T) {
	existing := []eventSummary{
		{EventID: "e3", ReceivedAtMS: 30, Comm: "codex"},
		{EventID: "e2", ReceivedAtMS: 20, Comm: "bash"},
		{EventID: "e1", ReceivedAtMS: 10, Comm: "node"},
	}
	got := mergeEventSummaries(existing, []eventSummary{
		{EventID: "e4", ReceivedAtMS: 40, Comm: "curl", Target: "/tmp/a"},
		{EventID: "e2", ReceivedAtMS: 35, Comm: "python", Target: "/tmp/b"},
	}, 4)
	if len(got) != 4 {
		t.Fatalf("len=%d, want 4", len(got))
	}
	want := []string{"e4", "e2", "e3", "e1"}
	for i, id := range want {
		if got[i].EventID != id {
			t.Fatalf("row %d id=%q, want %q; got=%#v", i, got[i].EventID, id, got)
		}
		if got[i].SearchText == "" {
			t.Fatalf("row %d did not cache search text: %#v", i, got[i])
		}
	}
	if !strings.Contains(got[1].SearchText, "python") || strings.Contains(got[1].SearchText, "bash") {
		t.Fatalf("replacement search text is stale: %q", got[1].SearchText)
	}
}

func TestFilteredEventsCacheInvalidatesOnEventVersion(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = mergeEventSummaries(nil, []eventSummary{
		{EventID: "1", ReceivedAtMS: 10, Comm: "codex"},
	}, 1200)
	a.eventsVersion = 1
	first := a.filteredEvents()
	second := a.filteredEvents()
	if len(first) != 1 || len(second) != 1 || &first[0] != &second[0] {
		t.Fatal("filtered event cache was not reused")
	}

	a.events = mergeEventSummaries(a.events, []eventSummary{
		{EventID: "2", ReceivedAtMS: 20, Comm: "bash"},
	}, 1200)
	a.eventsVersion++
	third := a.filteredEvents()
	if len(third) != 2 || third[0].EventID != "2" {
		t.Fatalf("cache did not invalidate after event version change: %#v", third)
	}
}


func TestMergeEventWindowAlternatesBackingBuffers(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	if !a.mergeEventWindow([]eventSummary{{EventID: "1", ReceivedAtMS: 10}}, 1200) {
		t.Fatal("initial merge reported no change")
	}
	first := &a.events[0]

	if !a.mergeEventWindow([]eventSummary{{EventID: "2", ReceivedAtMS: 20}}, 1200) {
		t.Fatal("second merge reported no change")
	}
	second := &a.events[0]
	if first == second {
		t.Fatal("second merge unexpectedly overwrote the active event window")
	}

	if !a.mergeEventWindow([]eventSummary{{EventID: "3", ReceivedAtMS: 30}}, 1200) {
		t.Fatal("third merge reported no change")
	}
	if &a.events[0] != first {
		t.Fatal("third merge did not reuse the first backing buffer")
	}
}

func TestEventDecisionFilters(t *testing.T) {
	if !matchesEventDecision(eventSummary{Decision: "deny"}, "已阻断") {
		t.Fatal("deny must match blocked")
	}
	if !matchesEventDecision(eventSummary{Decision: "ALERT"}, "告警") {
		t.Fatal("alert must match alert filter")
	}
	if matchesEventDecision(eventSummary{Decision: "ALLOW"}, "已阻断") {
		t.Fatal("allow must not match blocked")
	}
}

func TestEnforcementTargetsRejectHostnameAndAcceptLiteralIP(t *testing.T) {
	dns := enforcementTargets(map[string]any{
		"event": map[string]any{"netEndpoint": "example.com:443", "path": "relative"},
	})
	if dns.IP != "" || dns.Port != 0 || dns.ExecPath != "" {
		t.Fatalf("hostname must not become enforcement target: %+v", dns)
	}

	ip := enforcementTargets(map[string]any{
		"event": map[string]any{"netEndpoint": "192.0.2.10:443", "path": "/usr/bin/curl"},
	})
	if ip.IP != "192.0.2.10" || ip.Port != 443 || ip.ExecPath != "/usr/bin/curl" {
		t.Fatalf("unexpected literal-IP target: %+v", ip)
	}
}

func TestDecodeSystemStatsProtobuf(t *testing.T) {
	var process []byte
	process = protowire.AppendTag(process, 1, protowire.VarintType)
	process = protowire.AppendVarint(process, 42)
	process = protowire.AppendTag(process, 2, protowire.VarintType)
	process = protowire.AppendVarint(process, 1)
	process = protowire.AppendTag(process, 3, protowire.BytesType)
	process = protowire.AppendString(process, "codex")
	process = protowire.AppendTag(process, 4, protowire.Fixed64Type)
	process = protowire.AppendFixed64(process, math.Float64bits(12.5))
	process = protowire.AppendTag(process, 5, protowire.Fixed32Type)
	process = protowire.AppendFixed32(process, math.Float32bits(3.25))
	process = protowire.AppendTag(process, 6, protowire.BytesType)
	process = protowire.AppendString(process, "steve")
	process = protowire.AppendTag(process, 10, protowire.BytesType)
	process = protowire.AppendString(process, "codex exec")

	var cpu []byte
	cpu = protowire.AppendTag(cpu, 1, protowire.Fixed64Type)
	cpu = protowire.AppendFixed64(cpu, math.Float64bits(37.5))

	var memory []byte
	memory = protowire.AppendTag(memory, 1, protowire.VarintType)
	memory = protowire.AppendVarint(memory, 1000)
	memory = protowire.AppendTag(memory, 2, protowire.VarintType)
	memory = protowire.AppendVarint(memory, 500)
	memory = protowire.AppendTag(memory, 3, protowire.Fixed32Type)
	memory = protowire.AppendFixed32(memory, math.Float32bits(50))

	var ioInfo []byte
	for field, value := range map[protowire.Number]uint64{1: 10, 2: 20, 3: 30, 4: 40} {
		ioInfo = protowire.AppendTag(ioInfo, field, protowire.VarintType)
		ioInfo = protowire.AppendVarint(ioInfo, value)
	}

	var data []byte
	for _, field := range []struct {
		n protowire.Number
		b []byte
	}{{1, process}, {3, cpu}, {4, memory}, {5, ioInfo}} {
		data = protowire.AppendTag(data, field.n, protowire.BytesType)
		data = protowire.AppendBytes(data, field.b)
	}

	got, err := decodeSystemStats(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Processes) != 1 || got.Processes[0].PID != 42 || got.Processes[0].Name != "codex" {
		t.Fatalf("process decode: %+v", got.Processes)
	}
	if got.CPUTotal != 37.5 || got.MemTotal != 1000 || got.MemUsed != 500 || got.NetSent != 40 {
		t.Fatalf("stats decode: %+v", got)
	}
}

func TestSystemWebSocketURL(t *testing.T) {
	got, err := systemWebSocketURL("https://host.example/base")
	if err != nil {
		t.Fatal(err)
	}
	if got != "wss://host.example/base/ws/system" {
		t.Fatalf("got %q", got)
	}
}
