package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestProcessReferenceFromEventEvidence(t *testing.T) {
	detail := map[string]any{
		"Timestamp": float64(1791504653492),
		"Event": map[string]any{
			"pid": float64(391385), "ppid": float64(315919),
			"comm": "MULTI_AGENT_FILE_CONTENTION",
		},
	}
	ref := processReferenceFromEvent(detail)
	if ref.PID != 391385 || ref.PPID != 315919 || ref.Comm != "MULTI_AGENT_FILE_CONTENTION" ||
		ref.Occurred.UnixMilli() != 1791504653492 {
		t.Fatalf("lost recorded process evidence: %+v", ref)
	}
	// Kernel monotonic timestamps cannot establish an absolute PID lifetime.
	unverified := processReferenceFromEvent(map[string]any{
		"Event": map[string]any{"pid": float64(99), "timestampNs": "12345"},
	})
	if !unverified.Occurred.IsZero() {
		t.Fatalf("monotonic timestamp interpreted as calendar time: %+v", unverified)
	}
}

func TestProcessAtEventRejectsReusedPID(t *testing.T) {
	at := time.Date(2026, 10, 9, 12, 10, 53, 0, time.UTC)
	ref := eventProcessReference{PID: 391385, PPID: 315919, Occurred: at}
	later := systemProcess{PID: 391385, Name: "unrelated", CreateTime: at.Add(time.Minute).Unix()}
	if _, ok, note := processAtEvent(ref, []systemProcess{later}); ok || !strings.Contains(note, "PID 复用") {
		t.Fatalf("PID reuse must not be attributed to a historical event: %v %q", ok, note)
	}
	original := systemProcess{PID: 391385, Name: "agent", CreateTime: at.Add(-time.Minute).Unix()}
	if p, ok, _ := processAtEvent(ref, []systemProcess{original}); !ok || p.Name != "agent" {
		t.Fatal("running process with older start time should be available")
	}
	if _, ok, _ := processAtEvent(ref, nil); ok {
		t.Fatal("missing process must not be invented")
	}
	if _, ok, note := processAtEvent(eventProcessReference{PID: 0}, []systemProcess{original}); ok ||
		!strings.Contains(note, "有效 PID") {
		t.Fatal("missing PID must be surfaced")
	}
}

func TestFocusedProcessTreeRetainsEventPIDWithManySiblings(t *testing.T) {
	processes := []systemProcess{{PID: 1, Name: "init"}}
	for i := 2; i < 240; i++ {
		processes = append(processes, systemProcess{PID: i, PPID: 1, Name: "sibling"})
	}
	processes = append(processes, systemProcess{PID: 500, PPID: 238, Name: "target-child"})
	rows, truncated := buildEventProcessTreeRows(processes, 238, nil, 20)
	if !truncated {
		t.Fatal("tree should advertise the display cap")
	}
	targetIndex := -1
	for i, row := range rows {
		if row.Process.PID == 238 {
			targetIndex = i
			if !row.OnPath || row.Depth != 1 {
				t.Fatalf("target not shown in ancestor path: %+v", row)
			}
			break
		}
	}
	if targetIndex == -1 || targetIndex > 2 {
		t.Fatalf("large sibling list hid event PID: %+v", rows)
	}
	if rows[0].Process.PID != 1 {
		t.Fatalf("missing ancestor: %+v", rows[0])
	}
	// Explicit collapsing must override the default expanded ancestor path.
	collapsed, _ := buildEventProcessTreeRows(processes, 238, map[int]bool{1: false}, 20)
	if len(collapsed) != 1 || collapsed[0].Expanded {
		t.Fatalf("explicitly collapsed parent still expanded: %+v", collapsed)
	}
}

func TestProcessTreeCycleAndUnknownParent(t *testing.T) {
	processes := []systemProcess{
		{PID: 7, PPID: 8, Name: "one"},
		{PID: 8, PPID: 7, Name: "two"},
		{PID: 9, PPID: 42, Name: "orphan"},
	}
	rows, truncated := buildEventProcessTreeRows(processes, 7, nil, 120)
	if truncated || len(rows) != 2 {
		t.Fatalf("cycle should render each process only once: %+v, truncated=%v", rows, truncated)
	}
	rows, truncated = buildEventProcessTreeRows(processes, 9, nil, 120)
	if truncated || len(rows) != 1 || rows[0].Depth != 0 {
		t.Fatalf("missing parent should not be fabricated: %+v", rows)
	}
}

func TestEventProcessInvestigationMissingSnapshot(t *testing.T) {
	a := &renewApp{eventProcessTab: 0}
	detail := map[string]any{
		"Event": map[string]any{
			"pid": float64(391385), "ppid": float64(315919), "comm": "agent", "uid": float64(1000),
		},
	}
	tester := ui.NewTester(func(c *ui.Context) {
		a.eventProcessInvestigation(c, detail, 420)
	}, 840, 560)
	tester.Frame()
	for _, label := range []string{"事件时的进程证据", "PID 391385", "PPID 315919", "实时系统数据暂不可用"} {
		if !tester.HasText(label) {
			t.Fatalf("missing event-only fallback label %q", label)
		}
	}
}
