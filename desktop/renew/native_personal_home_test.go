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
