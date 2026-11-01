package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// Test that every scope of a live event-filter cache key remains distinct.
// A field separator embedded in user-provided text must never alias another
// filter combination; numerical and boolean predicates are typed.
func TestEventFilterSignatureTracksAllConstraints(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "  CODEX  "
	baseline := a.eventFilterCacheKey()
	if baseline.search != "codex" {
		t.Fatalf("search key was not normalized: %q", baseline.search)
	}
	for _, tc := range []struct {
		name string
		set  func(*renewApp)
	}{
		{"type", func(a *renewApp) { a.eventTypeFilter = "file_write" }},
		{"session", func(a *renewApp) { a.eventSessionFilter = "run-1" }},
		{"decision", func(a *renewApp) { a.eventDecisionFilter = "告警" }},
		{"pid", func(a *renewApp) { a.eventPIDFilter = 19 }},
		{"root-pid", func(a *renewApp) { a.eventRootPIDFilter = 11 }},
		{"target", func(a *renewApp) { a.eventTargetFilter = "/tmp/data" }},
		{"domain-agent", func(a *renewApp) { a.eventDomainAgent = "codex" }},
		{"domain-target", func(a *renewApp) { a.eventDomainTarget = "example.org" }},
		{"domain-kind", func(a *renewApp) { a.eventDomainKind = "dns" }},
		{"risk", func(a *renewApp) { a.eventRiskFilter = "高风险" }},
		{"score", func(a *renewApp) { a.eventMinRiskScore = 40.5 }},
		{"file-edit", func(a *renewApp) { a.eventFileEditsOnly = true }},
		{"delegation", func(a *renewApp) { a.eventDelegatedOnly = true }},
		{"attention", func(a *renewApp) { a.eventAttentionOnly = true }},
		{"management", func(a *renewApp) { a.managementEventFilter = "CCS" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newRenewApp("http://127.0.0.1:8080")
			a.search = "  CODEX  "
			previous := a.eventFilterCacheKey()
			tc.set(a)
			changed := a.eventFilterCacheKey()
			if changed == previous {
				t.Fatal("new filter constraint was lost from the signature")
			}
		})
	}
	if first, second := (eventFilterSignature{search: "a\x00b", eventType: "c"}),
		(eventFilterSignature{search: "a", eventType: "b\x00c"}); first == second {
		t.Fatal("typed signature aliased distinct filter fields containing separators")
	}
}

func TestEventFilterCheapPredicatesPreserveCombinedMatches(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "match", PID: 101, Type: "file_write", Target: "/tmp/report", RiskScore: 85, Decision: "ALERT", Comm: "codex", SearchText: "codex /tmp/report", ReceivedAtMS: 30},
		{EventID: "other-pid", PID: 102, Type: "file_write", Target: "/tmp/report", RiskScore: 85, Decision: "ALERT", Comm: "codex", SearchText: "codex /tmp/report", ReceivedAtMS: 20},
		{EventID: "other-risk", PID: 101, Type: "file_write", Target: "/tmp/report", RiskScore: 1, Decision: "ALLOW", Comm: "codex", SearchText: "codex /tmp/report", ReceivedAtMS: 10},
		{EventID: "other-type", PID: 101, Type: "network_connect", Target: "/tmp/report", RiskScore: 85, Decision: "ALERT", Comm: "codex", SearchText: "codex /tmp/report", ReceivedAtMS: 5},
		{EventID: "other-search", PID: 101, Type: "file_write", Target: "/tmp/report", RiskScore: 85, Decision: "ALERT", Comm: "codex", SearchText: "no match text", ReceivedAtMS: 1},
	}
	a.search = " COdex "
	a.eventPIDFilter = 101
	a.eventTypeFilter = "file_write"
	a.eventTargetFilter = "/tmp/report"
	a.eventMinRiskScore = 80
	a.eventRiskFilter = "高风险"
	a.eventDecisionFilter = "告警"
	a.eventAttentionOnly = true
	found := a.filteredEvents()
	if len(found) != 1 || found[0].EventID != "match" {
		t.Fatalf("combined risk, decision, PID, type, target and search returned unexpected events: %+v", found)
	}
	if next := a.filteredEvents(); &next[0] != &found[0] {
		t.Fatal("unchanged filter signature should return the same cached slice")
	}
	a.search = "missing"
	if got := a.filteredEvents(); len(got) != 0 {
		t.Fatalf("changed search must invalidate the filter cache: %+v", got)
	}
	a.search = "codex"
	a.eventPIDFilter = 102
	if got := a.filteredEvents(); len(got) != 1 || got[0].EventID != "other-pid" {
		t.Fatalf("changed PID must invalidate the filter cache: %+v", got)
	}
}

func BenchmarkRenewEventFilterCompute(b *testing.B) {
	for _, tc := range []struct {
		name       string
		query      string
		pid        int
		oneVersion bool
	}{
		{"hot-cache", "codex", 0, true},
		{"search-on-refresh", "codex", 0, false},
		{"pid-then-search-on-refresh", "codex", 1777, false},
		{"pid-then-uncached-search", "codex", 1777, false},
	} {
		b.Run(tc.name, func(b *testing.B) {
			a := newRenewApp("http://127.0.0.1:8080")
			a.search = tc.query
			a.eventPIDFilter = tc.pid
			a.events = make([]eventSummary, 1200)
			for i := range a.events {
				id := strconv.Itoa(i + 1)
				search := "codex /tmp/workspace " + id
				if strings.Contains(tc.name, "uncached") {
					search = ""
				}
				a.events[i] = eventSummary{
					EventID: id,
					PID: 1000 + i,
					Type: "file_write",
					Comm: "codex",
					Target: "/tmp/workspace",
					SearchText: search,
				}
			}
			a.filteredEvents()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if !tc.oneVersion {
					a.eventsVersion++
				}
				_ = a.filteredEvents()
			}
		})
	}
}

var benchmarkLegacyFilterKey string
var benchmarkTypedFilterKey eventFilterSignature

func BenchmarkRenewFilterSignature(b *testing.B) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "  CODEX  "
	a.eventPIDFilter = 1234
	a.eventRootPIDFilter = 42
	a.eventTargetFilter = "/tmp/workspace/foo"
	a.eventDomainAgent = "codex"
	a.eventDomainTarget = "example.org"
	a.eventDomainKind = "dns"
	a.eventRiskFilter = "高风险"
	a.eventMinRiskScore = 78.5
	a.eventAttentionOnly = true

	b.Run("previous-string", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			q := strings.ToLower(strings.TrimSpace(a.search))
			key := q + "\x00" + a.eventTypeFilter + "\x00" + a.eventSessionFilter + "\x00" + a.eventDecisionFilter + "\x00" +
				fmt.Sprint(a.eventPIDFilter) + "\x00" + fmt.Sprint(a.eventRootPIDFilter) + "\x00" + a.eventTargetFilter + "\x00" +
				a.eventDomainAgent + "\x00" + a.eventDomainTarget + "\x00" + a.eventDomainKind + "\x00" +
				fmt.Sprint(a.eventFileEditsOnly) + "\x00" + fmt.Sprint(a.eventDelegatedOnly) + "\x00" +
				a.eventRiskFilter + "\x00" + fmt.Sprint(a.eventMinRiskScore) + "\x00" + a.managementEventFilter
			if a.eventAttentionOnly {
				key += "\x001"
			}
			benchmarkLegacyFilterKey = key
		}
	})
	b.Run("typed-signature", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			benchmarkTypedFilterKey = a.eventFilterCacheKey()
		}
	})
}
