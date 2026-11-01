package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestRenewMotionPresetsStayShortAndNonBouncy(t *testing.T) {
	nav := renewNavigationMotion()
	if nav.Duration != 0 || nav.Spring.Bounce != 0 ||
		nav.Spring.Duration < 200*time.Millisecond ||
		nav.Spring.Duration > 350*time.Millisecond {
		t.Fatalf("sidebar spring must settle quickly without overshooting: %+v", nav)
	}
	if rail := renewRailColorTransition(); !rail.Colors ||
		rail.Duration > 200*time.Millisecond || rail.Enter != nil || rail.Exit != nil {
		t.Fatalf("navigation hover must animate color, not layout: %+v", rail)
	}
	if page := renewPageTransition(); page.Enter == nil || page.Exit != nil ||
		page.Duration > 200*time.Millisecond {
		t.Fatalf("security pages must enter quickly without retaining ghost event evidence: %+v", page)
	}
	if tab := renewInspectorTabTransition(); tab.Enter == nil || tab.Exit != nil ||
		tab.Duration > 200*time.Millisecond {
		t.Fatalf("health tabs must reveal without lingering stale diagnostics: %+v", tab)
	}
	if status := renewStatusColorTransition(); !status.Colors ||
		status.Enter != nil || status.Exit != nil ||
		status.Duration > 250*time.Millisecond {
		t.Fatalf("collector status must change color without changing layout: %+v", status)
	}
	if reveal := renewRevealTransition(); reveal.Exit != nil ||
		reveal.Enter == nil || reveal.Enter.Collapse ||
		reveal.Duration > 200*time.Millisecond {
		t.Fatalf("low-frequency alerts must not move or collapse the rest of the layout: %+v", reveal)
	}
}

func TestRenewInspectorTabsRemainUsableWithReducedMotion(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.starting = false
	a.page = "事件"
	view := ui.NewTester(a.view, 1480, 860)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, HighContrast: true, TextScale: 1})
	if err := view.Click("运行诊断"); err != nil {
		t.Fatal(err)
	}
	if a.inspectorTab != 1 || !view.HasText("采集链路") {
		t.Fatal("native inspector diagnostics tab must remain interactive")
	}
	a.events = []eventSummary{{EventID: "incoming", Decision: "ALERT", RiskScore: 70}}
	a.eventsVersion++
	view.Frame()
	if a.inspectorTab != 1 || !view.HasText("采集链路") {
		t.Fatal("live event arrival must not steal focus or change inspector tabs")
	}
	a.inspectorTab = 0
	view.Frame()
	if !view.HasText("事件上下文") {
		t.Fatal("event inspection panel should return after switching tabs")
	}
	if a.eventDetailOpen {
		t.Fatal("tab animation must not fetch full incident detail")
	}
}

func TestRenewStatusAnimationDoesNotMaskCollectorFailure(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.starting = false
	a.connected = true
	a.healthReady = true
	a.health.CaptureHealthy = true
	a.eventStreamConnected = true
	a.systemConnected = true
	view := ui.NewTester(a.view, 1480, 860)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, HighContrast: true, TextScale: 1})
	if !view.HasText("当前运行正常") {
		t.Fatal("healthy collector should be described by its actual health snapshot")
	}
	a.health.CaptureHealthy = false
	view.Frame()
	if !view.HasText("采集链路异常") || view.HasText("当前运行正常") {
		t.Fatal("status color transition must never delay or falsify the health label")
	}
}

func TestWorkspaceMotionLayoutKeepsControlsStableDuringSidebarSpring(t *testing.T) {
	// At 1250 DIP an inspector fits only with less than 134 DIP of sidebar.
	// Compute its visibility against the future occupied width from the
	// first animation frame rather than letting it briefly flicker.
	for _, tc := range []struct {
		name           string
		animated, goal float32
		layout         float32
		inspector      bool
		compactHeader  bool
	}{
		{"expand-before-first-frame", 0, 198, 198, false, true},
		{"expand-midway", 86, 198, 198, false, true},
		{"expand-final", 198, 198, 198, false, true},
		{"collapse-start", 198, 0, 198, false, true},
		{"collapse-midway", 100, 0, 100, true, false},
		{"collapse-final", 0, 0, 0, true, false},
		{"reverse-while-opening", 86, 0, 86, true, false},
		{"reverse-while-closing", 100, 198, 198, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			layout := workspaceMotionLayoutWidth(tc.animated, tc.goal)
			if layout != tc.layout {
				t.Fatalf("stable nav layout = %.0f, want %.0f", layout, tc.layout)
			}
			if show := showWorkspaceInspector(1250, layout, true, "事件"); show != tc.inspector {
				t.Fatalf("inspector visibility = %t, want %t", show, tc.inspector)
			}
			if compact := workspaceCompactHeader(1100, layout); compact != tc.compactHeader {
				t.Fatalf("header must not oscillate while the spring moves, compact=%t want=%t", compact, tc.compactHeader)
			}
		})
	}
}

func TestWorkspaceMotionReleasesSidebarImmediatelyInNarrowWindow(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "事件"
	a.starting = false
	a.navigationOpen = true
	view := ui.NewTester(a.view, 1480, 860)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if !view.HasText("功能导航") {
		t.Fatal("full-width desktop should show the sidebar")
	}
	view.SetSize(760, 720)
	if view.HasText("功能导航") {
		t.Fatal("compact window must jump directly to rail-only navigation")
	}
	if !a.navigationOpen {
		t.Fatal("compact fallback must not discard the user's navigation preference")
	}
	view.SetSize(1480, 860)
	if !view.HasText("功能导航") {
		t.Fatal("restoring desktop width should restore the original navigation preference")
	}
}

func TestWorkspaceDenseTablesSkipRepeatedSidebarReflow(t *testing.T) {
	for _, tc := range []struct {
		page    string
		events  int
		instant bool
	}{
		{"事件", 0, false},
		{"事件", 255, false},
		{"事件", 256, true},
		{"事件", 1200, true},
		{"网络", 256, true},
		{"进程", 256, true},
		{"概览", 1200, false},
		{"系统", 1200, false},
		{"规则", 1200, false},
	} {
		if got := workspaceSnapNavigation(tc.page, tc.events); got != tc.instant {
			t.Errorf("snap navigation page %q, events %d = %t, want %t",
				tc.page, tc.events, got, tc.instant)
		}
	}
	// Native user intent remains visible even though the expensive width
	// transition may be skipped for an active, dense events table.
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "事件"
	a.starting = false
	a.events = make([]eventSummary, 1200)
	view := ui.NewTester(a.view, 1480, 860)
	if err := view.Click("»"); err != nil {
		t.Fatal(err)
	}
	if !a.navigationOpen || !view.HasText("功能导航") {
		t.Fatal("optimized navigation must still reveal all native sidebar routes")
	}
	if err := view.Click("≡"); err != nil {
		t.Fatal(err)
	}
	if a.navigationOpen || view.HasText("功能导航") {
		t.Fatal("optimized navigation must still collapse without reserving a gutter")
	}
}
