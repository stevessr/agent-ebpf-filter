package main

import (
	"math"
	"reflect"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func TestAggregateNetworkUsesBoundedSummaryFields(t *testing.T) {
	rows := aggregateNetwork([]eventSummary{
		{PID: 10, Comm: "codex", Type: "NETWORK_CONNECT", Domain: "example.com", NetBytes: 12, RiskScore: 20, ReceivedAtMS: 10},
		{PID: 11, Comm: "curl", Type: "DNS_QUERY", Domain: "example.com", NetBytes: 8, RiskScore: 70, ReceivedAtMS: 20},
		{PID: 12, Comm: "bash", Type: "WRITE", Path: "/tmp/x", ReceivedAtMS: 30},
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
