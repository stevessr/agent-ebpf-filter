package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
)

const (
	behaviorOrbitHeight       = float32(400)
	behaviorOrbitMarksPerLane = 24
	behaviorOrbitHitRadius    = float32(11)
)

type behaviorOrbitMark struct {
	event     eventSummary
	lane      behaviorLane
	laneIndex int
	x, y      float32
	angle     float64
	attention bool
}

type behaviorOrbitLane struct {
	key   behaviorLane
	name  string
	index int
}

var behaviorOrbitLanes = [...]behaviorOrbitLane{
	{key: behaviorFile, name: "文件访问", index: 0},
	{key: behaviorNetwork, name: "网络通信", index: 1},
	{key: behaviorProcess, name: "进程执行", index: 2},
	{key: behaviorOther, name: "其他信号", index: 3},
}

// behaviorOrbitAngle maps the newest event to -80° and the oldest to 220°.
func behaviorOrbitAngle(nowMS, windowMS, receivedAtMS int64) float64 {
	if windowMS <= 0 {
		windowMS = behaviorWindowDefault
	}
	age := float64(nowMS-receivedAtMS) / float64(windowMS)
	age = math.Max(0, math.Min(1, age))
	return -80 + age*300
}

func behaviorOrbitPoint(cx, cy, radius float32, angle float64) (float32, float32) {
	radians := angle * math.Pi / 180
	return cx + radius*float32(math.Cos(radians)), cy + radius*float32(math.Sin(radians))
}

// behaviorOrbitMarks caps each lane independently and preserves snapshot order.
func behaviorOrbitMarks(snapshot behaviorSnapshot, cx, cy float32, radii [4]float32) []behaviorOrbitMark {
	marks := make([]behaviorOrbitMark, 0, min(len(snapshot.Events), 4*behaviorOrbitMarksPerLane))
	shown := [4]int{}
	for _, event := range snapshot.Events {
		lane := behaviorLaneForEvent(event)
		laneIndex := behaviorOrbitLaneIndex(lane)
		if laneIndex < 0 || shown[laneIndex] >= behaviorOrbitMarksPerLane {
			continue
		}
		angle := behaviorOrbitAngle(snapshot.NowMS, snapshot.WindowMS, event.ReceivedAtMS)
		// Small deterministic offsets keep neighboring points legible without
		// changing the event's time angle or lane identity.
		radius := radii[laneIndex] + float32((shown[laneIndex]%3)-1)*2.5
		x, y := behaviorOrbitPoint(cx, cy, radius, angle)
		marks = append(marks, behaviorOrbitMark{
			event: event, lane: lane, laneIndex: laneIndex, x: x, y: y,
			angle: angle, attention: behaviorNeedsAttention(event),
		})
		shown[laneIndex]++
	}
	return marks
}

// Resolve the actual release position; hit-test callbacks may belong to an earlier frame.
func behaviorOrbitPick(snapshot behaviorSnapshot, x, y, width, height float32) string {
	summaryX := width * .69
	if width < 300 {
		summaryX = width * .66
	}
	cx, cy := min(width*.34, summaryX*.48), height*.49
	outer := max(float32(48), min((height-66)/2, cx-15))
	gap := outer * .18
	radii := [4]float32{outer - 3*gap, outer - 2*gap, outer - gap, outer}
	closest, id := behaviorOrbitHitRadius*behaviorOrbitHitRadius, ""
	for _, mark := range behaviorOrbitMarks(snapshot, cx, cy, radii) {
		dx, dy := x-mark.x, y-mark.y
		distance := dx*dx + dy*dy
		if distance <= closest {
			closest, id = distance, mark.event.EventID
		}
	}
	return id
}

func behaviorOrbitLaneIndex(lane behaviorLane) int {
	switch lane {
	case behaviorFile:
		return 0
	case behaviorNetwork:
		return 1
	case behaviorProcess:
		return 2
	case behaviorOther:
		return 3
	default:
		return -1
	}
}

// drawBehaviorOrbit paints the time-window evidence with native theme colors.
// It reads the snapshot and selection only; it does not mutate app or snapshot state.
func (a *renewApp) drawBehaviorOrbit(c *ui.Context, snapshot behaviorSnapshot) {
	theme := c.Theme()
	textSize := max(float32(11), theme.FontSize)
	if textSize == 0 {
		textSize = 13
	}
	highContrast := c.Preferences().HighContrast

	ui.Box(c).FillWidth().Height(behaviorOrbitHeight).Role(ui.RoleGroup).Label("行为轨道：文件访问、网络通信、进程执行、其他信号；事件点表示时间位置，告警使用菱形标记").Draw(func(p *ui.Painter, r ui.Rect) {
		width, height := r.W, r.H
		if width <= 0 || height <= 0 {
			return
		}
		p.Transform(ui.Translation(r.X, r.Y), 0, 0, func() {

			// Give the four lane summary a fixed readable column; the orbit uses
			// the remaining width and shrinks naturally with its container.
			summaryX := width * 0.69
			if width < 300 {
				summaryX = width * 0.66
			}
			cx := min(width*0.34, summaryX*0.48)
			cy := height * 0.49
			outer := min((height-66)/2, cx-15)
			outer = max(float32(48), outer)
			gap := outer * 0.18
			radii := [4]float32{outer - 3*gap, outer - 2*gap, outer - gap, outer}
			lineWidth := float32(1)
			if highContrast {
				lineWidth = 1.7
			}

			// Cardinal guides and a 60-step time scale, with a labeled quarter,
			// half and three-quarter window. Angle follows the Vue reference.
			var guides ui.Path
			guides.Circle(cx, cy, outer+8)
			guides.MoveTo(cx-outer-12, cy).LineTo(cx+outer+12, cy)
			guides.MoveTo(cx, cy-outer-12).LineTo(cx, cy+outer+12)
			p.StrokePath(&guides, lineWidth, theme.Border)
			for tick := 0; tick <= 60; tick++ {
				angle := -80 + float64(tick)*5
				inner := outer + 1
				if tick%5 == 0 {
					inner = outer - 4
				}
				x1, y1 := behaviorOrbitPoint(cx, cy, inner, angle)
				x2, y2 := behaviorOrbitPoint(cx, cy, outer+4, angle)
				var mark ui.Path
				mark.MoveTo(x1, y1).LineTo(x2, y2)
				color := theme.Border
				thickness := float32(0.7)
				if tick%5 == 0 {
					color, thickness = theme.TextMuted, lineWidth
				}
				p.StrokePath(&mark, thickness, color)
			}
			labelSize := max(float32(11), textSize*0.85)
			p.Text(cx+8, max(float32(2), cy-outer-23), "现在 / Now", labelSize, theme.TextMuted)
			windowMS := snapshot.WindowMS
			if windowMS <= 0 {
				windowMS = behaviorWindowDefault
			}
			quarter := windowMS / 4000
			p.Text(cx+outer+8, cy+3, fmt.Sprintf("−%ds", quarter), labelSize, theme.TextMuted)
			p.Text(cx-18, min(height-labelSize-2, cy+outer+13), fmt.Sprintf("−%ds", windowMS/2000), labelSize, theme.TextMuted)
			p.Text(max(float32(1), cx-outer-39), cy-outer*0.25, fmt.Sprintf("−%ds", windowMS*3/4000), labelSize, theme.TextMuted)

			// Four concentric lanes. High contrast keeps stronger solid boundaries.
			for i, radius := range radii {
				var track ui.Path
				track.Circle(cx, cy, radius)
				track.Circle(cx, cy, max(float32(1), radius-2))
				color := theme.Border
				if highContrast {
					color = theme.Text
				}
				p.StrokePath(&track, lineWidth, color)
				p.Text(cx-radius+4, cy-4, fmt.Sprintf("%02d", i+1), labelSize, theme.TextMuted)
			}

			// Connect each ring to its lane summary with a static native cubic path.
			rowTop := float32(57)
			rowStep := (height - 94) / 3
			for i := range behaviorOrbitLanes {
				anchorX, anchorY := behaviorOrbitPoint(cx, cy, radii[i], -24+float64(i*14))
				targetX := summaryX - 7
				targetY := rowTop + float32(i)*rowStep + textSize*0.65
				var link ui.Path
				link.MoveTo(anchorX, anchorY).CubeTo(anchorX+(targetX-anchorX)*0.35, anchorY, targetX-(targetX-anchorX)*0.25, targetY, targetX, targetY)
				p.StrokePath(&link, lineWidth, theme.Border)
				p.Fill(ui.Rect{X: targetX - 2, Y: targetY - 2, W: 4, H: 4}, theme.TextMuted, 2)
				lane := behaviorOrbitLanes[i]
				p.Text(summaryX, rowTop+float32(i)*rowStep, lane.name, textSize, theme.Text)
				p.Text(summaryX, rowTop+float32(i)*rowStep+textSize*1.25, fmt.Sprintf("%d", snapshot.Lanes[i]), textSize*1.15, theme.Accent)
			}

			// The center is an explicit observation window, not a safety verdict.
			coreRadius := max(float32(20), radii[0]*0.78)
			var core ui.Path
			core.Circle(cx, cy, coreRadius)
			p.FillPath(&core, theme.Surface)
			var coreBorder ui.Path
			coreBorder.Circle(cx, cy, coreRadius)
			p.StrokePath(&coreBorder, lineWidth, theme.Border)
			countColor := theme.Text
			countLabel := fmt.Sprintf("%d", len(snapshot.Events))
			if len(snapshot.Events) == 0 {
				countColor = theme.Warning
			}
			centerText := func(label string, y, size float32, color ui.Color) {
				w, _ := p.MeasureText(0, ui.Span{Text: label, Size: size})
				p.Text(cx-w/2, y, label, size, color)
			}
			centerText(countLabel, cy-textSize*2, max(float32(26), textSize*1.8), countColor)
			centerText("窗口内事件", cy+textSize*0.65, labelSize, theme.TextMuted)
			centerText(fmt.Sprintf("%ds", windowMS/1000), cy+textSize*2, labelSize, theme.Accent)
			if len(snapshot.Events) == 0 {
				p.Text(cx-outer*0.54, cy+outer*0.47, "空窗口 ≠ 安全", labelSize, theme.Warning)
			}

			marks := behaviorOrbitMarks(snapshot, cx, cy, radii)
			for _, mark := range marks {
				dotColor := theme.Accent
				if mark.attention {
					dotColor = theme.Warning
				}
				if mark.event.EventID == a.behaviorSelectedID && mark.event.EventID != "" {
					var selection ui.Path
					selection.Circle(mark.x, mark.y, 7)
					p.StrokePath(&selection, 1.5, theme.Text)
				}
				if mark.attention {
					size := float32(4.2)
					var diamond ui.Path
					diamond.MoveTo(mark.x, mark.y-size).LineTo(mark.x+size, mark.y).LineTo(mark.x, mark.y+size).LineTo(mark.x-size, mark.y).Close()
					p.FillPath(&diamond, dotColor)
				} else {
					var dot ui.Path
					dot.Circle(mark.x, mark.y, 3.1)
					p.FillPath(&dot, dotColor)
				}
			}

			// Always retain collector and pipeline degradation in this visualization.
			status, level := a.pipelineStatus()
			statusColor := theme.Success
			if level == "warning" {
				statusColor = theme.Warning
			} else if level == "danger" {
				statusColor = theme.Danger
			}
			p.Text(4, height-textSize-4, "采集 / 管道："+status, textSize, statusColor)
			if a.health.RingbufDroppedTotal > 0 || a.eventUIDroppedCount() > 0 {
				drops := a.health.RingbufDroppedTotal + int64(a.eventUIDroppedCount())
				p.Text(width*0.52, height-textSize-4, fmt.Sprintf("丢弃 %d", drops), textSize, theme.Warning)
			}
		})
	}).Children(func() {
		region := ui.Box(c).Absolute().Fill().Role(ui.RoleGroup).Label("行为轨道事件节点；可用下方事件按钮键盘选择")
		region.HitTest(func(x, y float32, r ui.Rect) bool {
			return behaviorOrbitPick(snapshot, x-r.X, y-r.Y, r.W, r.H) != ""
		})
		region.OnClick(func() {
			x, y, over := region.PointerPosition()
			r := region.Bounds()
			if over {
				if id := behaviorOrbitPick(snapshot, x, y, r.W, r.H); id != "" {
					a.behaviorSelectedID = id
				}
			}
		})
	})
}
