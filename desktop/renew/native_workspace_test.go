package main

import (
	"strings"
	"testing"
)

func TestWorkspaceInspectorResponsive(t *testing.T) {
	tests := []struct {
		width float32
		open  bool
		page  string
		want  bool
	}{
		{1480, true, "概览", true},
		{1480, true, "网络", true},
		{1480, true, "系统", true},
		{1320, true, "事件", true},
		{1319, true, "事件", false},
		{1480, false, "事件", false},
		{1480, true, "规则", false},
	}
	for _, tc := range tests {
		if got := showWorkspaceInspector(tc.width, tc.open, tc.page); got != tc.want {
			t.Errorf("showWorkspaceInspector(%v, %v, %q) = %v, want %v", tc.width, tc.open, tc.page, got, tc.want)
		}
	}
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
