package main

import "testing"

func TestWorkspaceInspectorResponsive(t *testing.T) {
	tests := []struct {
		width float32
		open  bool
		page  string
		want  bool
	}{
		{1480, true, "概览", true},
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
