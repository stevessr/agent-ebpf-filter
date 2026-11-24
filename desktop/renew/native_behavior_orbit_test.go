package main

import (
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestBehaviorOrbitAngleUsesSnapshotTimeWindow(t *testing.T) {
	const now, window = int64(200_000), int64(60_000)
	for _, tc := range []struct {
		name string
		time int64
		want float64
	}{
		{name: "newest", time: now, want: -80},
		{name: "oldest", time: now - window, want: 220},
		{name: "quarter old", time: now - window/4, want: -5},
		{name: "clamp future", time: now + window, want: -80},
		{name: "clamp stale", time: now - 2*window, want: 220},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := behaviorOrbitAngle(now, window, tc.time); math.Abs(got-tc.want) > 1e-9 {
				t.Fatalf("angle=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestBehaviorOrbitMarksAreBoundedPerLaneAndKeepCountsSeparate(t *testing.T) {
	snapshot := behaviorSnapshot{NowMS: 100_000, WindowMS: 60_000}
	for i := 0; i < 31; i++ {
		snapshot.Events = append(snapshot.Events, eventSummary{
			EventID: string(rune('a' + i)), Type: "openat", ReceivedAtMS: snapshot.NowMS - int64(i*1000),
		})
	}
	snapshot.Lanes[0] = len(snapshot.Events)
	marks := behaviorOrbitMarks(snapshot, 100, 100, [4]float32{40, 60, 80, 100})
	if len(marks) != behaviorOrbitMarksPerLane {
		t.Fatalf("rendered %d marks, want cap %d", len(marks), behaviorOrbitMarksPerLane)
	}
	if snapshot.Lanes[0] != 31 {
		t.Fatalf("source lane count changed to %d", snapshot.Lanes[0])
	}
	if marks[0].event.EventID != snapshot.Events[0].EventID || marks[len(marks)-1].event.EventID != snapshot.Events[behaviorOrbitMarksPerLane-1].EventID {
		t.Fatal("marks did not preserve newest-first snapshot order")
	}
	if marks[0].angle != -80 || marks[len(marks)-1].angle <= marks[0].angle {
		t.Fatalf("unexpected time ordering: first=%v last=%v", marks[0].angle, marks[len(marks)-1].angle)
	}
}

func TestBehaviorOrbitCanvasHasHeightAndSelectsPaintedNode(t *testing.T) {
	a := newRenewApp("")
	snapshot := behaviorSnapshot{NowMS: 100000, WindowMS: 60000, Events: []eventSummary{{EventID: "painted", Type: "openat", ReceivedAtMS: 100000}}, Lanes: [4]int{1, 0, 0, 0}}
	view := ui.NewTester(func(c *ui.Context) { ui.Column(c).Fill().Children(func() { a.drawBehaviorOrbit(c, snapshot) }) }, 1000, 460)
	outer := float32(167)
	x, y := behaviorOrbitPoint(331.2, 196, outer*(1-3*.18)-2.5, -80)
	view.ClickAt(x, y)
	if a.behaviorSelectedID != "painted" {
		t.Fatalf("painted node click selected %q; canvas may be collapsed or hit geometry mismatched", a.behaviorSelectedID)
	}
}

func TestBehaviorOrbitAttentionUsesWarningShape(t *testing.T) {
	snapshot := behaviorSnapshot{NowMS: 1000, WindowMS: 1000, Events: []eventSummary{
		{EventID: "ordinary", Type: "openat", ReceivedAtMS: 900},
		{EventID: "watch", Type: "connect", Decision: "ALERT", ReceivedAtMS: 800},
	}}
	marks := behaviorOrbitMarks(snapshot, 0, 0, [4]float32{20, 40, 60, 80})
	if len(marks) != 2 || marks[0].attention || !marks[1].attention {
		t.Fatalf("attention marks = %#v", marks)
	}
}
