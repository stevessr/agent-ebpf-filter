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
	if page := renewPageTransition(); page.Enter == nil || page.Exit == nil ||
		page.Duration > 240*time.Millisecond {
		t.Fatalf("page navigation should keep a bounded enter/exit transition: %+v", page)
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
