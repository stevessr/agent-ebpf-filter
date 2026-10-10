package main

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPersonalOverviewSnapshotHintStates(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*renewApp)
		want  string
	}{
		{"startup", func(a *renewApp) {}, "正在建立连接"},
		{"offline overrides pause", func(a *renewApp) {
			a.starting = false
			a.paused = true
		}, "连接已中断"},
		{"first sync", func(a *renewApp) {
			a.starting = false
			a.connected = true
		}, "正在确认采集器"},
		{"pause only affects display", func(a *renewApp) {
			a.starting = false
			a.connected = true
			a.healthReady = true
			a.health.CaptureHealthy = true
			a.paused = true
		}, "不是后台采集"},
		{"capture unhealthy", func(a *renewApp) {
			a.starting = false
			a.connected = true
			a.healthReady = true
			a.paused = true // A broken collector must override the pause hint.
		}, "可能漏记活动"},
		{"stream fallback", func(a *renewApp) {
			a.starting = false
			a.connected = true
			a.healthReady = true
			a.health.CaptureHealthy = true
		}, "回退同步"},
		{"synced view", func(a *renewApp) {
			a.starting = false
			a.connected = true
			a.healthReady = true
			a.health.CaptureHealthy = true
			a.eventStreamConnected = true
			a.lastSync = time.Date(2026, 10, 10, 9, 7, 6, 0, time.Local)
		}, "09:07:06"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newRenewApp("http://127.0.0.1:8080")
			tc.setup(a)
			if got := a.overviewSnapshotHint(); !strings.Contains(got, tc.want) {
				t.Fatalf("overviewSnapshotHint() = %q, want substring %q", got, tc.want)
			}
		})
	}
}

func TestPersonalOverviewRecentEventsIgnoreInvestigationFilters(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "intentionally-unmatched"
	a.eventRiskFilter = "高风险"
	a.eventAttentionOnly = true
	a.eventTargetFilter = "/never-matches"
	for i := 0; i < 8; i++ {
		a.events = append(a.events, eventSummary{EventID: fmt.Sprintf("evt-%d", i)})
	}
	got := a.overviewRecentEvents()
	if len(got) != 5 || got[0].EventID != "evt-0" || got[4].EventID != "evt-4" {
		t.Fatalf("recent events should be unfiltered and capped at five, got %#v", got)
	}
	if a.search != "intentionally-unmatched" || a.eventRiskFilter != "高风险" {
		t.Fatal("home summary must not mutate investigator filters")
	}
	a.events = nil
	if got := a.overviewRecentEvents(); len(got) != 0 {
		t.Fatalf("empty summary should remain empty, got %#v", got)
	}
}

func TestPersonalOverviewPriorityEventsShowOlderRisk(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.search = "hides-everything-on-events-page"
	a.eventAttentionOnly = false
	// The first five new events are normal. An older high-risk event must
	// remain visible on the homepage instead of being pushed out of sight.
	for i := 0; i < 6; i++ {
		a.events = append(a.events, eventSummary{EventID: fmt.Sprintf("normal-%d", i)})
	}
	a.events = append(a.events,
		eventSummary{EventID: "attention-latest", RiskScore: 65},
		eventSummary{EventID: "danger-older", RiskScore: 95},
		eventSummary{EventID: "danger-oldest", RiskScore: 90},
	)
	got := a.overviewPriorityEvents()
	if len(got) != 3 {
		t.Fatalf("priority length = %d; want three", len(got))
	}
	want := []string{"danger-older", "danger-oldest", "attention-latest"}
	for i, event := range got {
		if event.EventID != want[i] {
			t.Fatalf("priority[%d] = %q; want %q", i, event.EventID, want[i])
		}
	}
	if a.search != "hides-everything-on-events-page" {
		t.Fatal("personal alerts must not alter investigator filters")
	}

	a.events = []eventSummary{{EventID: "normal"}}
	if got := a.overviewPriorityEvents(); len(got) != 0 {
		t.Fatalf("normal activity is not a priority alert: %#v", got)
	}
}

func TestPausedEventDisplayStillRefreshesCollectorStatus(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	if !a.shouldFetchEventWindow() {
		t.Fatal("initial snapshot should include event summaries")
	}
	a.historyInitialized = true
	a.eventStreamConnected = true
	if a.shouldFetchEventWindow() {
		t.Fatal("active event stream should only fetch status")
	}
	a.eventStreamConnected = false
	if !a.shouldFetchEventWindow() {
		t.Fatal("fallback needs event window")
	}
	a.eventUIPaused.Store(true)
	if a.shouldFetchEventWindow() {
		t.Fatal("pausing the event display should fetch status only")
	}
	a.eventUIPaused.Store(false)
	if !a.shouldFetchEventWindow() {
		t.Fatal("resume should fetch summaries if stream is offline")
	}
}
