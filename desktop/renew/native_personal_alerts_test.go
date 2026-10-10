package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

func personalAlertFixture(id, target, decision string, risk float64, ms int64) eventSummary {
	return eventSummary{
		EventID: id, Type: "network_connect", Comm: "test-agent",
		Target: target, Decision: decision, RiskScore: risk,
		ReceivedAtMS: ms,
	}
}

func TestPersonalAlertGroupsMergeExactSimilarEvents(t *testing.T) {
	events := []eventSummary{
		personalAlertFixture("old-alert", "one.example", "ALERT", 65, 10),
		personalAlertFixture("high", "one.example", "BLOCK", 90, 30),
		personalAlertFixture("new-alert", "one.example", "ALERT", 70, 20),
		personalAlertFixture("other", "two.example", "ALERT", 65, 40),
		personalAlertFixture("ordinary", "three.example", "ALLOW", 10, 50),
	}
	groups, hidden := groupPersonalAlerts(events, nil)
	if hidden != 0 || len(groups) != 3 {
		t.Fatalf("got %d groups / %d hidden, want 3 / 0", len(groups), hidden)
	}
	if groups[0].Risk != "高风险" || groups[0].Event.EventID != "high" {
		t.Fatalf("high risk must come first: %#v", groups)
	}
	if groups[1].Event.EventID != "other" || groups[2].Count != 2 ||
		groups[2].Event.EventID != "new-alert" {
		t.Fatalf("newest representative or similarity grouping incorrect: %#v", groups)
	}
	// Field boundaries and targets are part of the signature, not substring comparisons.
	if personalAlertSignature(events[0]) == personalAlertSignature(events[3]) ||
		personalAlertSignature(events[0]) == personalAlertSignature(events[1]) {
		t.Fatal("different target or severity must not collide into one group")
	}
}

func TestPersonalAlertAcknowledgementsNeverHideHighRisk(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	attention := personalAlertFixture("attn", "api.example", "ALERT", 65, 1)
	high := personalAlertFixture("block", "api.example", "BLOCK", 92, 2)
	a.events = []eventSummary{attention, high}
	a.eventsVersion = 1
	groups, hidden := a.personalAlertGroups()
	if len(groups) != 2 || hidden != 0 {
		t.Fatalf("initial groups = %#v, hidden=%d", groups, hidden)
	}
	a.mutePersonalAlert(groups[0]) // A high-risk action may not be silenced.
	if len(a.personalAlertMuted) != 0 {
		t.Fatal("high-risk group must not be acknowledgeable")
	}
	a.mutePersonalAlert(groups[1])
	got, hidden := a.personalAlertGroups()
	if len(got) != 1 || got[0].Risk != "高风险" || hidden != 1 {
		t.Fatalf("must keep high-risk visible, got %#v hidden=%d", got, hidden)
	}
	// Even a later risk escalation of an acknowledged kind stays actionable.
	a.events = append(a.events, personalAlertFixture("escalated", "api.example", "ALERT", 99, 3))
	a.eventsVersion++
	got, hidden = a.personalAlertGroups()
	if len(got) != 2 || hidden != 1 {
		t.Fatalf("escalated risk must remain visible, got %#v hidden=%d", got, hidden)
	}
	// Acknowledged records are never deleted or rewritten.
	if len(a.events) != 3 {
		t.Fatal("acknowledgement modified event evidence")
	}
	a.restorePersonalAlerts()
	got, hidden = a.personalAlertGroups()
	if hidden != 0 || len(got) != 3 || len(a.personalAlertMuteOrder) != 0 {
		t.Fatalf("restoring must reveal all groups; got %#v hidden=%d", got, hidden)
	}
}

func TestPersonalAlertMuteListBoundedAndFIFO(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	for i := 0; i < personalAlertMuteLimit+1; i++ {
		event := personalAlertFixture(fmt.Sprintf("id-%d", i), fmt.Sprintf("host-%d.example", i), "ALERT", 65, int64(i))
		a.mutePersonalAlert(personalAlertGroup{Risk: "需关注", Key: personalAlertSignature(event)})
	}
	if len(a.personalAlertMuteOrder) != personalAlertMuteLimit ||
		len(a.personalAlertMuted) != personalAlertMuteLimit {
		t.Fatalf("acknowledgement memory must be bounded, got %d", len(a.personalAlertMuted))
	}
	first := personalAlertSignature(personalAlertFixture("first", "host-0.example", "ALERT", 65, 0))
	last := personalAlertSignature(personalAlertFixture("last", fmt.Sprintf("host-%d.example", personalAlertMuteLimit), "ALERT", 65, 0))
	if _, ok := a.personalAlertMuted[first]; ok {
		t.Fatal("the oldest acknowledgement should be evicted")
	}
	if _, ok := a.personalAlertMuted[last]; !ok {
		t.Fatal("newest acknowledgement should remain")
	}
	a.restorePersonalAlerts()
	if len(a.personalAlertMuteOrder) != 0 || len(a.personalAlertMuted) != 0 {
		t.Fatal("restore must remove all temporary acknowledgements")
	}
}

func TestPersonalAlertOverviewReadableWhenMuted(t *testing.T) {
	a := newRenewApp("http://127.0.0.1:8080")
	a.starting = false
	a.connected = true
	a.healthReady = true
	a.health.CaptureHealthy = true
	a.eventStreamConnected = true
	a.systemConnected = true
	e := personalAlertFixture("attention", "api.example", "ALERT", 65, 10)
	a.events = []eventSummary{e}
	a.eventsVersion = 1
	view := ui.NewTester(a.view, 1100, 760)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	if !view.HasText("优先核查") {
		t.Fatal("personal overview must show actionable attention alerts")
	}
	a.mutePersonalAlert(personalAlertGroup{Risk: "需关注", Key: personalAlertSignature(e)})
	view.Frame()
	if !view.HasText("恢复全部提示") || !view.HasText("最近活动") {
		t.Fatal("muted home must keep restoring and full activity access visible")
	}
}
