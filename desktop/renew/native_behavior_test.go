package main

import (
	"testing"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
)

func TestProjectBehaviorBucketsAndFiltersEvents(t *testing.T) {
	now := int64(1_800_000_000_000)
	events := []eventSummary{
		{EventID: "file", Type: "openat", PID: 17, PPID: 2, Comm: "agent", ReceivedAtMS: now - 1_000},
		{EventID: "net", Type: "network_connect", PID: 17, Comm: "agent", Network: true, NetBytes: 120, ReceivedAtMS: now - 2_000, RiskScore: 77, Decision: "ALERT"},
		{EventID: "exec", Type: "execve", PID: 22, Comm: "child", ReceivedAtMS: now - 3_000},
		{EventID: "other", Type: "opaque_signal", PID: 17, Comm: "agent", ReceivedAtMS: now - 4_000},
		{EventID: "old", Type: "read", PID: 17, ReceivedAtMS: now - 61_000},
		{EventID: "future", Type: "write", PID: 17, ReceivedAtMS: now + 1},
	}
	got := projectBehavior(events, now, 60_000, 0)
	if len(got.Events) != 4 || got.Lanes != [4]int{1, 1, 1, 1} {
		t.Fatalf("unexpected window projection: events=%d lanes=%v", len(got.Events), got.Lanes)
	}
	if got.Attention != 1 || got.MaxRisk != 77 || got.Bytes != 120 || got.Rate != 0.8 {
		t.Fatalf("unexpected telemetry: %+v", got)
	}
	if got.Events[0].EventID != "file" || got.Histogram[23] < 1 {
		t.Fatalf("newest order or time bucket incorrect: %+v", got)
	}
	focused := projectBehavior(events, now, 60_000, 17)
	if len(focused.Events) != 3 || len(focused.Processes) != 1 || focused.Processes[0].PID != 17 {
		t.Fatalf("PID focus failed: %+v", focused)
	}
}

func TestBehaviorLaneAndAttentionClassification(t *testing.T) {
	cases := []struct {
		event     eventSummary
		lane      behaviorLane
		attention bool
	}{
		{eventSummary{Type: " openat "}, behaviorFile, false},
		{eventSummary{Type: "TLS_PLAINTEXT"}, behaviorNetwork, false},
		{eventSummary{Type: "process_exec"}, behaviorProcess, false},
		{eventSummary{EventType: 35, Type: "unknown"}, behaviorNetwork, false},
		{eventSummary{EventType: 41, Type: "unknown"}, behaviorOther, true},
		{eventSummary{Type: "unknown"}, behaviorOther, false},
		{eventSummary{Type: "unknown", RiskScore: 60}, behaviorOther, true},
		{eventSummary{Type: "unknown", Decision: "deny"}, behaviorOther, true},
		{eventSummary{Type: "semantic_alert"}, behaviorOther, true},
	}
	for _, tc := range cases {
		if lane := behaviorLaneForEvent(tc.event); lane != tc.lane {
			t.Errorf("%q classified as %q, want %q", tc.event.Type, lane, tc.lane)
		}
		if got := behaviorNeedsAttention(tc.event); got != tc.attention {
			t.Errorf("%+v attention=%v, want %v", tc.event, got, tc.attention)
		}
	}
}

func TestBehaviorPauseFreezesPresentationNotEventSource(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	a := newRenewApp("")
	a.behaviorWindowMS = 60_000
	a.events = []eventSummary{{EventID: "before", Type: "openat", PID: 1, ReceivedAtMS: now.UnixMilli()}}
	a.toggleBehaviorPause(now)
	a.events = append(a.events, eventSummary{EventID: "after", Type: "write", PID: 1, ReceivedAtMS: now.UnixMilli()})
	if got := a.behaviorDisplay(now); len(got.Events) != 1 || got.Events[0].EventID != "before" {
		t.Fatalf("paused presentation followed incoming events: %+v", got.Events)
	}
	a.toggleBehaviorPause(now)
	if got := a.behaviorDisplay(now); len(got.Events) != 2 {
		t.Fatalf("resume did not show newly received event: %+v", got.Events)
	}
}

func TestBehaviorPausedFiltersKeepOriginalEvidenceAndTime(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	a := newRenewApp("")
	a.behaviorWindowMS = 60000
	a.events = []eventSummary{{EventID: "before", Type: "openat", PID: 2, ReceivedAtMS: now.UnixMilli()}}
	a.toggleBehaviorPause(now)
	a.events = append(a.events, eventSummary{EventID: "after", Type: "write", PID: 2, ReceivedAtMS: now.Add(time.Second).UnixMilli()})
	a.behaviorPID = 2
	a.behaviorWindowMS = 30000
	a.refreshBehaviorFrozen(now.Add(40 * time.Second))
	got := a.behaviorDisplay(now.Add(40 * time.Second))
	if got.NowMS != now.UnixMilli() || len(got.Events) != 1 || got.Events[0].EventID != "before" {
		t.Fatalf("paused filter advanced source/time: %+v", got)
	}
	a.toggleBehaviorPause(now)
	if a.behaviorPausedEvents != nil {
		t.Fatal("resume retained paused event copy")
	}
}

func TestBehaviorAttentionControlFiltersSelectableEvidence(t *testing.T) {
	a := newRenewApp("")
	a.starting = false
	a.page = "行为"
	now := time.Now().UnixMilli()
	a.events = []eventSummary{{EventID: "routine", Type: "openat", PID: 42, ReceivedAtMS: now}, {EventID: "alert", Type: "write", PID: 43, RiskScore: 70, ReceivedAtMS: now}}
	a.eventsVersion = 1
	view := ui.NewTester(a.view, 1480, 1180)
	if err := view.Click("仅需关注 · 1"); err != nil {
		t.Fatal(err)
	}
	if !a.behaviorAttentionOnly || view.HasText("文件访问 · openat · 42") {
		t.Fatal("attention filter retained routine selectable event")
	}
	if err := view.Click("仅需关注 · 1"); err != nil {
		t.Fatal(err)
	}
	if a.behaviorAttentionOnly || !view.HasText("文件访问 · openat · 42") {
		t.Fatal("attention reset did not restore routine event")
	}
}

func TestBehaviorHistogramIsBoundedAndEmptySafe(t *testing.T) {
	var empty [behaviorBucketCount]int
	if got := behaviorHistogram(empty); utf8.RuneCountInString(got) != behaviorBucketCount {
		t.Fatalf("empty histogram width=%d", len(got))
	}
	values := empty
	values[3] = 1
	values[20] = 7
	if got := behaviorHistogram(values); utf8.RuneCountInString(got) != behaviorBucketCount {
		t.Fatalf("histogram width=%d", len(got))
	}
}

func TestBehaviorProjectionCacheTracksVersionAndTime(t *testing.T) {
	now := time.UnixMilli(1_800_000_000_000)
	a := newRenewApp("")
	a.behaviorWindowMS = 30_000
	a.events = []eventSummary{{EventID: "first", Type: "openat", PID: 9, ReceivedAtMS: now.UnixMilli()}}
	a.eventsVersion = 1
	if got := a.behaviorDisplay(now); len(got.Events) != 1 {
		t.Fatalf("initial projection missing event: %+v", got)
	}
	a.events = append(a.events, eventSummary{EventID: "second", Type: "write", PID: 9, ReceivedAtMS: now.Add(time.Second).UnixMilli()})
	a.eventsVersion++
	if got := a.behaviorDisplay(now.Add(time.Second)); len(got.Events) != 2 {
		t.Fatalf("projection did not refresh for event version: %+v", got.Events)
	}
	if got := a.behaviorDisplay(now.Add(32 * time.Second)); len(got.Events) != 0 {
		t.Fatalf("projection did not advance with time: %+v", got.Events)
	}
}

func TestBehaviorPageRendersAndUsesNativeControls(t *testing.T) {
	a := newRenewApp("")
	a.starting = false
	a.page = "行为"
	now := time.Now()
	a.events = []eventSummary{{EventID: "native-node", Type: "openat", PID: 42, Comm: "agent", Target: "/workspace/file", ReceivedAtMS: now.UnixMilli()}}
	a.eventsVersion = 1
	view := ui.NewTester(a.view, 1280, 1180)
	view.SetPreferences(ui.Preferences{ReduceMotion: true, HighContrast: true, TextScale: 1})
	for _, label := range []string{"行为轨道", "文件访问", "30 秒", "60 秒", "5 分钟", "暂停显示", "近 5 秒速率"} {
		if !view.HasText(label) {
			t.Errorf("behavior page missing %q", label)
		}
	}
	if err := view.Click("文件访问 · openat · 42"); err != nil {
		t.Fatalf("select behavior event: %v", err)
	}
	if !view.HasText("选中事件 · openat") {
		t.Fatal("clicking an orbit node did not reveal its event summary")
	}
	if err := view.Click("30 秒"); err != nil {
		t.Fatalf("select behavior window: %v", err)
	}
	if a.behaviorWindowMS != 30_000 {
		t.Fatalf("selected window=%d, want 30000", a.behaviorWindowMS)
	}
	if err := view.Click("暂停显示"); err != nil {
		t.Fatalf("pause behavior display: %v", err)
	}
	if !a.behaviorPaused || a.behaviorFrozen == nil {
		t.Fatal("native pause control did not freeze the summary")
	}
}
