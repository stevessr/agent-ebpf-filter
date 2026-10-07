package main

import "testing"

func TestMergeEventSummariesDeduplicatesAndBounds(t *testing.T) {
	existing := []eventSummary{{EventID: "a", ReceivedAtMS: 1}, {EventID: "b", ReceivedAtMS: 2}}
	incoming := []eventSummary{{EventID: "a", ReceivedAtMS: 4, Comm: "new"}, {EventID: "c", ReceivedAtMS: 3}}
	got := mergeEventSummaries(existing, incoming, 2)
	if len(got) != 2 || got[0].EventID != "a" || got[0].Comm != "new" || got[1].EventID != "c" {
		t.Fatalf("unexpected merge: %#v", got)
	}
}

func TestAttentionAndAgentClassification(t *testing.T) {
	if !isAttentionEvent(eventSummary{Decision: "deny"}) {
		t.Fatal("deny should require attention")
	}
	if !isAttentionEvent(eventSummary{RiskScore: 80}) {
		t.Fatal("high risk should require attention")
	}
	if isAttentionEvent(eventSummary{RiskScore: 10}) {
		t.Fatal("low-risk ordinary event should not require attention")
	}
	if !isAgentEvent(eventSummary{AgentRunID: "run-1"}) {
		t.Fatal("agent run should be classified as an agent event")
	}
}

func TestBuildDestinationsPrefersDomain(t *testing.T) {
	got := buildDestinations([]eventSummary{
		{Domain: "example.com", NetEndpoint: "1.2.3.4:443", NetBytes: 10},
		{Domain: "example.com", NetBytes: 20},
		{NetEndpoint: "8.8.8.8:53", NetBytes: 5},
	})
	if len(got) != 2 || got[0].Endpoint != "example.com" || got[0].Count != 2 || got[0].Bytes != 30 {
		t.Fatalf("unexpected destinations: %#v", got)
	}
}
