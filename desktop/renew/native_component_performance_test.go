package main

import (
	"fmt"
	"testing"
)

func TestStructuredEventFilterSignatureAvoidsAmbiguousKeys(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "  CaSe  "
	a.eventTypeFilter = "read\x00session"
	a.eventSessionFilter = "alpha"
	k1 := a.eventFilterCacheKey()
	if k1.Search != "case" {
		t.Fatalf("search normalization changed: %q", k1.Search)
	}
	a.eventTypeFilter = "read"
	a.eventSessionFilter = "session\x00alpha"
	k2 := a.eventFilterCacheKey()
	if k1 == k2 {
		t.Fatal("different filter fields must not collide even with embedded delimiters")
	}
	a.eventAttentionOnly = true
	k3 := a.eventFilterCacheKey()
	if k2 == k3 {
		t.Fatal("attention toggle must invalidate the cache")
	}
	a.eventMinRiskScore = 27.5
	if a.eventFilterCacheKey() == k3 {
		t.Fatal("risk threshold must invalidate the cache")
	}
}

func TestSummaryOwnershipIndexCachedByEventVersion(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "root", PID: 10, Comm: "codex", RootAgentPID: 10, ReceivedAtMS: 100, AgentRunID: "run-a"},
		{EventID: "child", PID: 20, Comm: "bash", RootAgentPID: 10, ReceivedAtMS: 110, AgentRunID: "run-a"},
	}
	a.eventsVersion = 1
	first := a.summaryOwnershipIndex()
	if got := first.attribution(a.events[1]); !got.IsAgent || got.OwnerComm != "codex" {
		t.Fatalf("initial owner invalid: %+v", got)
	}
	// Reused map identity is a visible signal that no reconstruction occurred.
	first.roots[-123] = agentRootIdentity{comm: "cache-marker"}
	second := a.summaryOwnershipIndex()
	if _, ok := second.roots[-123]; !ok {
		t.Fatal("unchanged snapshot should reuse the same map")
	}
	a.events[0].Comm = "claude"
	a.eventsVersion++
	third := a.summaryOwnershipIndex()
	if _, ok := third.roots[-123]; ok {
		t.Fatal("changed event window must rebuild the index")
	}
	if got := third.attribution(a.events[1]); got.OwnerComm != "claude" {
		t.Fatalf("updated attribution is stale: %+v", got)
	}
	// Explicit direct snapshots without a version also avoid stale reuse.
	a.eventsVersion = 0
	a.events[0].Comm = "codex"
	_ = a.summaryOwnershipIndex()
	a.events[0].Comm = "gemini"
	if got := a.summaryOwnershipIndex().attribution(a.events[1]); got.OwnerComm != "gemini" {
		t.Fatal("unversioned direct snapshots must never be cached")
	}
}

func BenchmarkStructuredEventFilterKey(b *testing.B) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "  Agent Open  "
	a.eventSessionFilter = "run-123"
	a.eventMinRiskScore = 27.5
	b.ReportAllocs()
	var result eventFilterSignature
	for i := 0; i < b.N; i++ {
		result = a.eventFilterCacheKey()
	}
	if result.Search == "" {
		b.Fatal("empty search signature")
	}
}

func BenchmarkSharedSummaryOwnershipIndex(b *testing.B) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = make([]eventSummary, 1200)
	for i := range a.events {
		a.events[i] = eventSummary{
			EventID: fmt.Sprint(i), PID: 1000 + i,
			RootAgentPID: 1000, Comm: "python", ReceivedAtMS: int64(i + 100),
		}
	}
	a.events[0] = eventSummary{EventID: "root", PID: 1000, RootAgentPID: 1000, Comm: "codex", AgentRunID: "run", ReceivedAtMS: 100}
	a.eventsVersion = 1
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		index := a.summaryOwnershipIndex()
		_ = index.attribution(a.events[i%len(a.events)])
	}
}

func TestSearchNormalizationReusedAndInvalidated(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = " MiXeD "
	if got := a.eventFilterCacheKey().Search; got != "mixed" {
		t.Fatalf("unexpected normalized query: %q", got)
	}
	if allocs := testing.AllocsPerRun(100, func() { _ = a.eventFilterCacheKey() }); allocs != 0 {
		t.Fatalf("stable filter signatures should allocate 0 B/op, got %.2f allocs", allocs)
	}
	a.search = "  NEW Query  "
	if got := a.eventFilterCacheKey().Search; got != "new query" {
		t.Fatalf("search edit failed to invalidate normalization: %q", got)
	}
	a.search = ""
	if got := a.eventFilterCacheKey().Search; got != "" {
		t.Fatalf("clearing query left stale normalized text: %q", got)
	}
}
