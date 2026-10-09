package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestWorkspaceInspectorResponsive(t *testing.T) {
	tests := []struct {
		width float32
		nav   float32
		open  bool
		page  string
		want  bool
	}{
		{1480, 198, true, "概览", true},
		{1480, 198, true, "网络", true},
		{1480, 198, true, "研判", false},
		{1480, 198, true, "系统", true},
		{1320, 198, true, "事件", true},
		{1313, 198, true, "事件", false},
		{1120, 0, true, "事件", true},
		{1120, 198, true, "事件", false},
		{1250, 150, true, "事件", false},
		{1250, 100, true, "事件", true},
		{1480, 198, false, "事件", false},
		{1480, 198, true, "规则", false},
	}
	for _, tc := range tests {
		if got := showWorkspaceInspector(tc.width, tc.nav, tc.open, tc.page); got != tc.want {
			t.Errorf("showWorkspaceInspector(%v, %v, %v, %q) = %v, want %v", tc.width, tc.nav, tc.open, tc.page, got, tc.want)
		}
	}
}


func TestWorkspaceNavigationDetailedLabels(t *testing.T) {
	for _, tc := range []struct {
		open, detailed bool
		want float32
	}{
		{false, false, 0},
		{false, true, 0},
		{true, false, workspaceNavigationCompactWidth},
		{true, true, workspaceNavigationDetailedWidth},
	} {
		if got := workspaceNavigationWidth(tc.open, tc.detailed); got != tc.want {
			t.Errorf("navigation width for open=%t detailed=%t is %v, want %v", tc.open, tc.detailed, got, tc.want)
		}
	}
	for _, id := range []string{"概览", "研判", "事件", "会话", "网络", "域名", "进程", "Agent 识别", "监控", "eBPF 模块", "规则", "跟踪", "路径权限", "终端", "系统"} {
		short, detailed := workspaceNavigationLabel(id, false), workspaceNavigationLabel(id, true)
		if short == "" || detailed == "" || short == detailed {
			t.Errorf("navigation label for %q should be nonempty and distinct: %q / %q", id, short, detailed)
		}
	}
	if got := workspaceNavigationLabel("未知页面", true); got != "未知页面" {
		t.Fatalf("unknown routes must remain unchanged: %q", got)
	}
	if showWorkspaceInspector(1400, workspaceNavigationDetailedWidth, true, "进程") {
		t.Fatal("detailed navigation must not squeeze in the incident inspector")
	}
	if !showWorkspaceInspector(1480, workspaceNavigationDetailedWidth, true, "进程") {
		t.Fatal("wide windows should retain the incident inspector with detailed navigation")
	}
}

func TestWorkspaceNavigationToggleInteractive(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "进程"
	view := ui.NewTester(a.view, 1480, 860)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})

	assertBreadcrumb := func(minX, maxX float32) {
		t.Helper()
		r, ok := view.Find("工作区")
		if !ok || r.X < minX || r.X > maxX {
			t.Fatalf("main workspace must follow navigation width, breadcrumb=%+v found=%t (wanted X %.0f..%.0f)", r, ok, minX, maxX)
		}
	}
	assertBreadcrumb(54, 100)
	if view.HasText("功能导航") {
		t.Fatal("collapsed navigation must not reserve an invisible gutter")
	}
	if err := view.Click("»"); err != nil {
		t.Fatal(err)
	}
	if !a.navigationOpen || !view.HasText("功能导航") {
		t.Fatal("rail toggle must open the sidebar")
	}
	assertBreadcrumb(248, 290)
	if err := view.Click("详细 ›"); err != nil {
		t.Fatal(err)
	}
	if !a.navigationDetailed || !view.HasText("进程活动与资源监测") || !view.HasText("Agent 识别与捕获监视范围") {
		t.Fatal("detailed mode must expand and show full navigation names")
	}
	assertBreadcrumb(352, 390)
	if _, found := view.Find("事件研判"); !found {
		t.Fatal("wide window should retain right-hand incident inspector")
	}
	view.SetSize(1400, 860)
	assertBreadcrumb(352, 390)
	if _, found := view.Find("事件研判"); found {
		t.Fatal("inspector must disappear when detailed sidebar leaves insufficient center space")
	}
	if err := view.Click("精简 ‹"); err != nil {
		t.Fatal(err)
	}
	if a.navigationDetailed {
		t.Fatal("sidebar should return to compact width")
	}
	assertBreadcrumb(248, 290)
	if _, found := view.Find("事件研判"); !found {
		t.Fatal("compact sidebar must release enough space for inspector")
	}
	if err := view.Click("≡"); err != nil {
		t.Fatal(err)
	}
	if a.navigationOpen || view.HasText("功能导航") {
		t.Fatal("collapsing sidebar must hide all of its content")
	}
	assertBreadcrumb(54, 100)
}

func TestInspectorUsesCompactEventsWithoutLoadingDetails(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "ok", Type: "read", RiskScore: 0},
		{EventID: "alert", Type: "openat", Decision: "ALERT", RiskScore: 70},
	}
	a.page = "概览"
	event, ok := a.inspectorEvent()
	if !ok || event.EventID != "alert" {
		t.Fatalf("expected attention event, got %+v, %v", event, ok)
	}
	if a.eventDetailOpen || a.eventDetail != nil {
		t.Fatal("inspector must not materialize full event detail")
	}
	a.page = "事件"
	a.eventSelected = 0
	event, ok = a.inspectorEvent()
	if !ok || event.EventID != "ok" {
		t.Fatalf("expected selected event, got %+v, %v", event, ok)
	}
}

func TestFocusSummaryResetsFiltersAndSelectsMatchingID(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{{EventID: "first"}, {EventID: "second"}}
	a.page = "概览"
	a.search = "no-match"
	a.eventAttentionOnly = true
	a.eventDecisionFilter = "已阻断"
	a.focusSummary("second")
	if a.page != "事件" || a.search != "" || a.eventAttentionOnly || a.eventDecisionFilter != "" {
		t.Fatalf("focusSummary did not open unfiltered event page: %+v", a)
	}
	if a.eventSelected != 1 {
		t.Fatalf("selected row = %d, want 1", a.eventSelected)
	}
}

func TestInspectorPinSurvivesFiltersAndDoesNotFallBackSilently(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "keep", Type: "openat", PID: 42},
		{EventID: "other", Type: "connect", PID: 43, Decision: "ALERT"},
	}
	a.inspectorPinnedID = "keep"
	a.eventTypeFilter = "connect"
	event, ok := a.inspectorEvent()
	if !ok || event.EventID != "keep" {
		t.Fatalf("pinned event should bypass filters: %+v, %v", event, ok)
	}
	a.events = a.events[1:]
	event, ok = a.inspectorEvent()
	if ok || event.EventID != "" {
		t.Fatalf("stale pin should not silently jump to a different event: %+v, %v", event, ok)
	}
	a.inspectorPinnedID = ""
	event, ok = a.inspectorEvent()
	if !ok || event.EventID != "other" {
		t.Fatalf("unpin should resume latest matching events: %+v, %v", event, ok)
	}
}

func TestExactPIDFilterAndFilterReset(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "one", PID: 12, Type: "openat"},
		{EventID: "two", PID: 312, Type: "openat"},
		{EventID: "three", PID: 12, Type: "connect"},
	}
	a.openEventFilter(a.events[0], "pid")
	if a.page != "事件" || a.eventPIDFilter != 12 {
		t.Fatalf("PID filter was not activated")
	}
	got := a.filteredEvents()
	if len(got) != 2 || got[0].PID != 12 || got[1].PID != 12 {
		t.Fatalf("PID filter must be exact, got: %+v", got)
	}
	a.openEventFilter(a.events[0], "type")
	if a.eventPIDFilter != 0 || a.eventTypeFilter != "openat" {
		t.Fatalf("switching filter did not reset previous PID constraint")
	}
	got = a.filteredEvents()
	if len(got) != 2 {
		t.Fatalf("type filter count=%d, want 2", len(got))
	}
	a.clearEventFilters()
	if a.eventTypeFilter != "" || a.eventPIDFilter != 0 || len(a.filteredEvents()) != 3 {
		t.Fatal("reset must clear all event constraints")
	}
}

func TestSelectedEventIDTracksRealtimeInsertions(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.starting = false
	a.page = "事件"
	a.mergeEventWindow([]eventSummary{
		{EventID: "older", ReceivedAtMS: 100},
		{EventID: "selected", ReceivedAtMS: 200},
	}, 1200)
	a.eventSelected = 0
	a.inspectorSelectedID = "selected"
	a.mergeEventWindow([]eventSummary{{EventID: "new", ReceivedAtMS: 300}}, 1200)
	if a.eventSelected != 1 || a.events[a.eventSelected].EventID != "selected" {
		t.Fatalf("selection drifted to a different event: at %d; events: %+v", a.eventSelected, a.events)
	}
}

func TestInspectorAlertsAndClipboardSummaryAreBounded(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "a", Decision: "ALERT"},
		{EventID: "b", Decision: "BLOCK"},
		{EventID: "c", Type: "read"},
	}
	if got := a.inspectorAlerts(1); len(got) != 1 || got[0].EventID != "a" {
		t.Fatalf("alerts should respect bound and risk classification: %+v", got)
	}
	if got := a.inspectorAlerts(0); len(got) != 0 {
		t.Fatalf("zero limit should yield no alerts: %+v", got)
	}
	copyText := summaryClipboardText(a.events[1])
	if !strings.Contains(copyText, "Event ID: b") || !strings.Contains(copyText, "Decision: BLOCK") {
		t.Fatalf("summary copy does not contain expected compact fields: %q", copyText)
	}
}

func TestRiskSeverityFilterMatchesClassification(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.events = []eventSummary{
		{EventID: "high", RiskScore: 90},
		{EventID: "warn", RiskScore: 65},
		{EventID: "ok", RiskScore: 10},
	}
	a.openEventFilter(a.events[0], "risk")
	if a.eventRiskFilter != "高风险" || len(a.filteredEvents()) != 1 || a.filteredEvents()[0].EventID != "high" {
		t.Fatalf("high risk filter was not applied: %q, %+v", a.eventRiskFilter, a.filteredEvents())
	}
	a.eventRiskFilter = "需关注"
	if got := a.filteredEvents(); len(got) != 1 || got[0].EventID != "warn" {
		t.Fatalf("warning risk filter mismatch: %+v", got)
	}
	a.clearEventFilters()
	if a.eventRiskFilter != "" || len(a.filteredEvents()) != 3 {
		t.Fatal("clearing filters must restore all severity levels")
	}
}

func TestWorkspaceCompactNavigationDoesNotLeaveBlankGutter(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	view := ui.NewTester(a.view, 1480, 860)
	// The test checks layout states, not a wall-clock-dependent animation.
	view.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})

	breadcrumb, ok := view.Find("工作区")
	if !ok || breadcrumb.X < 54 || breadcrumb.X > 100 {
		t.Fatalf("compact navigation should place the page directly after the 54-DIP rail, breadcrumb=%+v found=%v", breadcrumb, ok)
	}
	if view.HasText("监控工作台") {
		t.Fatal("the full navigation should not reserve room when collapsed")
	}
	if !view.HasText("事件研判") {
		t.Fatal("the compact layout should retain the incident-inspector on a wide window")
	}

	// The explicit expand control still provides the full navigation.
	a.navigationOpen = true
	view.Frame()
	if !view.HasText("监控工作台") || !view.HasText("文件访问保护") {
		t.Fatal("expanded navigation must show the full sidebar contents")
	}
	navHeader, found := view.Find("监控工作台")
	if !found || navHeader.X < 54 || navHeader.X > 240 || navHeader.Y < 0 || navHeader.Y > 200 {
		t.Fatalf("expanded sidebar content must begin alongside the activity rail, header=%+v found=%v", navHeader, found)
	}
	breadcrumb, ok = view.Find("工作区")
	if !ok || breadcrumb.X < 245 {
		t.Fatalf("expanded navigation should occupy its own width, breadcrumb=%+v found=%v", breadcrumb, ok)
	}

	// Resizing and collapsing the navigation returns all of its width to the center.
	view.SetSize(1100, 760)
	a.navigationOpen = false
	view.Frame()
	breadcrumb, ok = view.Find("工作区")
	if !ok || breadcrumb.X < 54 || breadcrumb.X > 100 {
		t.Fatalf("collapsed navigation should release its width after resize, breadcrumb=%+v found=%v", breadcrumb, ok)
	}
}

func TestNativeWorkspaceNavigationStates(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	if a.navigationOpen || !a.inspectorOpen {
		t.Fatal("new workspaces should keep the redundant navigation closed but expose the inspector")
	}
	if !pageHasInspector("网络") || pageHasInspector("规则") || pageHasInspector("研判") {
		t.Fatal("inspector should be available on observation views but not duplicate the standalone page")
	}
	if a.networkTable.Selected != &a.networkSelected {
		t.Fatal("network table should allow target selection and drilldown")
	}
	if got := pageSubtitle("研判"); got == "" {
		t.Fatal("standalone inspector should have its own metadata")
	}
}

func TestWorkspaceFilterIndicatorTracksVisibleConstraints(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	if a.hasEventConstraints() {
		t.Fatal("fresh workspace must not appear filtered")
	}
	a.eventRiskFilter = "高风险"
	if !a.hasEventConstraints() {
		t.Fatal("risk filter must be visible in workspace")
	}
	a.clearEventFilters()
	a.eventPIDFilter = 505
	if !a.hasEventConstraints() {
		t.Fatal("PID filter must be visible in workspace")
	}
	a.clearEventFilters()
	if a.hasEventConstraints() {
		t.Fatal("filter indicator must clear with the constraints")
	}
}

func TestWindowMaterialFallsBackOnLinuxAndWhenDisabled(t *testing.T) {
	if got := windowMaterial("linux", true); got != "" {
		t.Fatalf("Linux must stay opaque on X11/Wayland, got %q", got)
	}
	if got := windowMaterial("windows", false); got != "" {
		t.Fatalf("disabled material must be opaque, got %q", got)
	}
	if got := windowMaterial("darwin", false); got != "" {
		t.Fatalf("disabled macOS material must be opaque, got %q", got)
	}
	if got := windowMaterial("windows", true); got != "mica" {
		t.Fatalf("Windows must use Mica, got %q", got)
	}
	if got := windowMaterial("darwin", true); got != "sidebar" {
		t.Fatalf("macOS must use sidebar material, got %q", got)
	}
}

func TestInspectorQueueSelectionOverridesPinnedEvent(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.inspectorPinnedID = "old"
	a.inspectorSelectedID = "old"
	a.inspectorTab = 1
	a.eventSelected = 0
	a.events = []eventSummary{{EventID: "old"}, {EventID: "alert", Decision: "ALERT"}}
	a.selectInspectorEvent("alert")
	if a.inspectorPinnedID != "" || a.inspectorSelectedID != "alert" {
		t.Fatalf("queue selection must replace old pin: pinned=%q selected=%q", a.inspectorPinnedID, a.inspectorSelectedID)
	}
	if a.inspectorTab != 0 || a.eventSelected != -1 {
		t.Fatalf("queue selection must switch to incident tab and clear table index")
	}
	event, ok := a.inspectorEvent()
	if !ok || event.EventID != "alert" {
		t.Fatalf("queue selection was not visible: %+v %v", event, ok)
	}
	a.selectInspectorEvent("")
	if a.inspectorSelectedID != "alert" {
		t.Fatal("empty event identity should not disturb selection")
	}
}

func TestInspectorQueueChoiceBypassesLocalEventFilters(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.page = "事件"
	a.events = []eventSummary{
		{EventID: "normal", Type: "read", RiskScore: 0},
		{EventID: "critical", Type: "connect", RiskScore: 95},
	}
	a.eventTypeFilter = "read"
	a.inspectorPinnedID = "normal"
	a.selectInspectorEvent("critical")
	if len(a.filteredEvents()) != 1 || a.filteredEvents()[0].EventID != "normal" {
		t.Fatal("queue navigation must not silently clear the event table's filters")
	}
	if event, ok := a.inspectorEvent(); !ok || event.EventID != "critical" {
		t.Fatalf("queue selection was hidden by active event filter: %+v %v", event, ok)
	}
	a.focusSummary("critical")
	if a.eventTypeFilter != "" || a.eventSelected != 1 {
		t.Fatal("explicit locate must clear filters and select the target row")
	}
}
