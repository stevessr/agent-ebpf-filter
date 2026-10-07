package main

import (
	"reflect"
	"testing"
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
