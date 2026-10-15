package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const lineageGraphLimit = 72

// Radial and waterfall views share the same immutable per-frame graph, so a
// mode change never changes the meaning of an edge or the selected PID.
func (a *renewApp) agentLineageView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Agent → SubAgent · 进程派生").FontSize(28).Bold()
	ui.Text(c, "观察 Agent 执行期间的实时子进程树。黄色节点仅代表“子 Agent 候选”（另一 Agent 程序作为后代运行），不等于已经证实的模型层调用。").TextColor(t.TextMuted)
	if !a.systemConnected || len(a.system.Processes) == 0 {
		card(c, "尚无实时进程关系", func() {
			ui.Text(c, "此页面依赖 /ws/system 的实时 PID / PPID / CreateTime 快照，离线时不会用旧事件拼接虚构进程树。").TextColor(t.TextMuted)
			if a.systemErr != "" {
				ui.Text(c, a.systemErr).FontSize(11).TextColor(t.Warning)
			}
			if ui.Button(c, "查看系统连接").Clicked() {
				a.page = "系统"
			}
		})
		return
	}
	roots := agentLineageRoots(a.system.Processes, a.events)
	if len(roots) == 0 {
		card(c, "未检测到 Agent 主进程", func() {
			ui.Text(c, "当前进程快照中没有识别到受支持的 Agent 可执行文件。先运行或跟踪 Codex、Claude Code 等 Agent，页面会随系统流自动更新。").TextColor(t.TextMuted)
			if ui.Button(c, "查看 Agent 识别").Clicked() {
				a.page = "Agent 识别"
			}
		})
		return
	}
	choices := make([]string, 0, len(roots))
	choicesByPID := make(map[string]int, len(roots))
	for _, p := range roots {
		label := fmt.Sprintf("%s · %s · PID %d", agentLabelForLiveProcess(p, a.events), agentProcessName(p), p.PID)
		choices = append(choices, label)
		choicesByPID[label] = p.PID
	}
	if _, ok := choicesByPID[a.lineageRootChoice]; !ok {
		a.lineageRootChoice = choices[0]
		a.lineageSelectedPID = 0
	}
	rootPID := choicesByPID[a.lineageRootChoice]
	graph := buildAgentLineage(a.system.Processes, a.events, rootPID, lineageGraphLimit, a.system.FetchedAt)
	if len(graph.Nodes) == 0 {
		ui.Text(c, "选中进程已退出，请刷新后重新选择。").TextColor(t.Warning)
		return
	}
	if lineageIndexForPID(graph, a.lineageSelectedPID) < 0 {
		a.lineageSelectedPID = rootPID
	}
	candidates, alerts, events := 0, 0, 0
	for _, node := range graph.Nodes {
		if node.Role == "subagent-candidate" {
			candidates++
		}
		alerts += node.Alerts
		events += node.Events
	}
	ui.Row(c).Wrap().Gap(10).Children(func() {
		statCard(c, "可见进程", strconv.Itoa(len(graph.Nodes)), "含 Agent 主进程")
		statCard(c, "子 Agent 候选", strconv.Itoa(candidates), "根据可执行文件与祖先关系推断")
		statCard(c, "窗口事件", strconv.Itoa(events), "仅当前内存事件摘要")
		statCard(c, "异常事件", strconv.Itoa(alerts), "当前摘要窗口的近似计数")
	})
	card(c, "派生链路视图", func() {
		ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Select(c, &a.lineageRootChoice, choices).Width(350).Label("根 Agent")
			ui.Tabs(c, &a.lineageMode, "放射图", "瀑布图")
		})
		if graph.Truncated {
			ui.Textf(c, "当前视图最多展示 %d 个节点 / 6 层子进程；其余已折叠，避免高频采集导致页面卡顿。", lineageGraphLimit).FontSize(11).TextColor(t.Warning)
		}
		if a.lineageMode == 0 {
			ui.Text(c, "从中心 Agent 向外展示派生层级；连线是当前 PPID 父子关系，点击节点可查看进程与对应事件。").FontSize(12).TextColor(t.TextMuted)
			a.drawLineageRadial(c, graph)
		} else {
			ui.Text(c, "每行是快照中仍存活的进程；条形从已知启动时间延伸至快照时刻，不表示任务调用耗时或实际退出时间。").FontSize(12).TextColor(t.TextMuted)
			a.drawLineageWaterfall(c, graph)
		}
		ui.Row(c).Wrap().Gap(12).AlignItems(ui.Center).Children(func() {
			statusPill(c, "根 Agent", t.Accent)
			statusPill(c, "子 Agent 候选", t.Warning)
			statusPill(c, "普通派生进程", t.TextMuted)
			if !graph.Snapshot.IsZero() {
				ui.Text(c, "快照 "+graph.Snapshot.Format("15:04:05")).FontSize(11).TextColor(t.TextMuted)
			}
		})
	})
	a.agentLineageDetail(c, graph)
	a.agentLineageInsights(c, graph)
	ui.Text(c, "数据边界：可确认的是当前操作系统快照里的 PID/PPID 关系，以及原生 Hook 明确发出的 SubAgent 生命周期信号。二者不能自动拼成一条因果调用链。同进程或远端 SubAgent 的精确调用、耗时和结果，需要 SDK 提供稳定的 agent_id / parent_call_id / span_id 和结束时间。").FontSize(11).TextColor(t.TextMuted)
}

// The signals below do not manufacture OS nodes or infer a duration from
// events that lack a stable SubAgent invocation identifier.
func (a *renewApp) agentLineageInsights(c *ui.Context, graph agentLineage) {
	t := c.Theme()
	ui.Row(c).Gap(10).Wrap().Children(func() {
		card(c, "重点进程 · 快捷定位", func() {
			shown := 0
			for _, node := range graph.Nodes {
				if node.Role != "subagent-candidate" && node.Alerts == 0 {
					continue
				}
				if shown == 12 {
					ui.Text(c, "仅列出前 12 项，完整节点可在图中选择。").FontSize(11).TextColor(t.TextMuted)
					break
				}
				shown++
				label := fmt.Sprintf("%s · PID %d", agentProcessName(node.Process), node.Process.PID)
				if node.Alerts > 0 {
					label += fmt.Sprintf(" · %d 条异常", node.Alerts)
				}
				if ui.Button(c, label).Clicked() {
					a.lineageSelectedPID = node.Process.PID
				}
			}
			if shown == 0 {
				ui.Text(c, "当前可见进程中暂无 Agent 候选或异常。").TextColor(t.TextMuted)
			}
		})
	})
	signals := collectAgentLineageSignals(graph, a.events, 24)
	card(c, fmt.Sprintf("SubAgent 原生 Hook · 最近 %d 条", len(signals)), func() {
		ui.Text(c, "这一栏只展示实际观察到的 SubagentStart、SubagentStop、agentSpawn 等原生 Hook 信号；它们可能发生在同一 PID 内，但没有稳定的 agent_id 时不推断父调用或任务时长。").FontSize(11).TextColor(t.TextMuted)
		if len(signals) == 0 {
			ui.Text(c, "当前事件窗口尚无可关联的 SubAgent 生命周期 Hook。可以在监控设置中开启对应 CLI 的原生 Hook 采集。").TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "时间", Width: 92, Fixed: true},
			{Title: "Hook", MinWidth: 140},
			{Title: "Agent", MinWidth: 130},
			{Title: "PID", Width: 90},
			{Title: "生命周期", Width: 104},
		}
		ui.Table(c, &a.lineageSignalTable, cols, len(signals), func(row, col int) {
			sig := signals[row]
			switch col {
			case 0:
				ui.Text(c, time.UnixMilli(sig.WhenMS).Format("15:04:05")).Font("monospace")
			case 1:
				ui.Text(c, sig.Name)
			case 2:
				ui.Text(c, displayOr(sig.Provider, "未识别"))
			case 3:
				ui.Textf(c, "%d", sig.PID).Font("monospace")
			case 4:
				if sig.Kind == "start" {
					statusPill(c, "已观察到开始", t.Accent)
				} else {
					statusPill(c, "已观察到结束", t.TextMuted)
				}
			}
		}).Height(min(float32(430), 58+float32(len(signals))*32)).Label("原生 SubAgent 生命周期 Hook 信号")
	})
}

func radialLineagePoints(graph agentLineage, width, height float32) []ui.Point {
	points := make([]ui.Point, len(graph.Nodes))
	if len(points) == 0 {
		return points
	}
	children := make([][]int, len(points))
	maxDepth := 0
	for i, node := range graph.Nodes {
		if node.Parent >= 0 && node.Parent < i {
			children[node.Parent] = append(children[node.Parent], i)
		}
		if node.Depth > maxDepth {
			maxDepth = node.Depth
		}
	}
	leaves := make([]int, len(points))
	var count func(int) int
	count = func(i int) int {
		if len(children[i]) == 0 {
			leaves[i] = 1
			return 1
		}
		for _, j := range children[i] {
			leaves[i] += count(j)
		}
		return leaves[i]
	}
	count(0)
	maxRadius := float64(min(width*0.42, height*0.41))
	ring := maxRadius / float64(max(1, maxDepth))
	points[0] = ui.Point{X: width / 2, Y: height / 2}
	var place func(i int, start, end float64)
	place = func(i int, start, end float64) {
		if i != 0 {
			angle := (start+end)*math.Pi - math.Pi/2
			r := ring * float64(graph.Nodes[i].Depth)
			points[i] = ui.Point{
				X: width/2 + float32(math.Cos(angle)*r),
				Y: height/2 + float32(math.Sin(angle)*r),
			}
		}
		cursor := start
		for _, j := range children[i] {
			span := (end - start) * float64(leaves[j]) / float64(leaves[i])
			place(j, cursor, cursor+span)
			cursor += span
		}
	}
	place(0, 0, 2)
	return points
}

func lineageTone(t *ui.Theme, n agentLineageNode) ui.Color {
	if n.Alerts > 0 {
		return t.Danger
	}
	switch n.Role {
	case "agent":
		return t.Accent
	case "subagent-candidate":
		return t.Warning
	default:
		return t.TextMuted
	}
}

func (a *renewApp) drawLineageRadial(c *ui.Context, graph agentLineage) {
	t := c.Theme()
	canvas := ui.Box(c).FillWidth().Height(540).Radius(12).Border(1, t.Border).
		Background(t.Background).Focusable().Label("Agent 放射派生图，点击节点查看详情")
	last := canvas.LayoutBounds()
	if x, y, over := canvas.PointerPosition(); over && canvas.Clicked() && last.W > 0 {
		points := radialLineagePoints(graph, last.W, last.H)
		hit := -1
		best := float32(22 * 22)
		for i, pos := range points {
			dx, dy := pos.X-x, pos.Y-y
			d2 := dx*dx + dy*dy
			if d2 <= best {
				best, hit = d2, i
			}
		}
		if hit >= 0 {
			a.lineageSelectedPID = graph.Nodes[hit].Process.PID
		}
	}
	canvas.Draw(func(p *ui.Painter, r ui.Rect) {
		points := radialLineagePoints(graph, r.W, r.H)
		for _, f := range []float32{0.25, 0.50, 0.75, 1} {
			radius := min(r.W*0.42, r.H*0.41) * f
			p.StrokePath((&ui.Path{}).Circle(r.X+r.W/2, r.Y+r.H/2, radius), 1, t.Border.Alpha(0.7))
		}
		for i, node := range graph.Nodes {
			if node.Parent < 0 || node.Parent >= i {
				continue
			}
			from, to := points[node.Parent], points[i]
			edge := (&ui.Path{}).MoveTo(r.X+from.X, r.Y+from.Y).
				LineTo(r.X+to.X, r.Y+to.Y)
			p.StrokePath(edge, 1.8, lineageTone(t, node).Alpha(0.55))
		}
		for i, node := range graph.Nodes {
			pos := points[i]
			x, y := r.X+pos.X, r.Y+pos.Y
			tone := lineageTone(t, node)
			radius := float32(7)
			if node.Parent < 0 {
				radius = 13
			} else if node.Role == "subagent-candidate" {
				radius = 10
			}
			if a.lineageSelectedPID == node.Process.PID {
				p.FillPath((&ui.Path{}).Circle(x, y, radius+6), tone.Alpha(0.18))
				p.StrokePath((&ui.Path{}).Circle(x, y, radius+4), 2, tone)
			}
			p.FillPath((&ui.Path{}).Circle(x, y, radius), tone)
			if node.Alerts > 0 {
				p.StrokePath((&ui.Path{}).Circle(x, y, radius+2), 2.5, t.Danger)
			}
			if i == 0 || node.Role == "subagent-candidate" || len(graph.Nodes) <= 26 {
				label := agentProcessName(node.Process)
				if len([]rune(label)) > 16 {
					label = string([]rune(label)[:15]) + "…"
				}
				label = fmt.Sprintf("%s · %d", label, node.Process.PID)
				p.Text(x+radius+4, y-8, label, 11, t.Text)
			}
		}
	})
}

func waterfallBarGeometry(graph agentLineage, plotWidth float32) []float32 {
	starts := make([]float32, len(graph.Nodes))
	lo, hi := lineageVisibleSpan(graph)
	if hi <= lo {
		return starts
	}
	for i, n := range graph.Nodes {
		if n.Process.CreateTime <= 0 {
			starts[i] = -1
			continue
		}
		fraction := float64(n.Process.CreateTime-lo) / float64(hi-lo)
		starts[i] = float32(math.Max(0, math.Min(1, fraction))) * plotWidth
	}
	return starts
}

func (a *renewApp) drawLineageWaterfall(c *ui.Context, graph agentLineage) {
	t := c.Theme()
	const rowHeight float32 = 29
	const left float32 = 208
	height := 82 + rowHeight*float32(len(graph.Nodes))
	canvas := ui.Box(c).FillWidth().Height(height).Radius(12).Border(1, t.Border).
		Background(t.Background).Focusable().Label("Agent 进程派生时间瀑布图，点击行查看详情")
	if x, y, over := canvas.PointerPosition(); over && canvas.Clicked() {
		_ = x
		row := int((y - 62) / rowHeight)
		if y >= 62 && row >= 0 && row < len(graph.Nodes) {
			a.lineageSelectedPID = graph.Nodes[row].Process.PID
		}
	}
	canvas.Draw(func(p *ui.Painter, r ui.Rect) {
		plotWidth := max(float32(40), r.W-left-18)
		starts := waterfallBarGeometry(graph, plotWidth)
		lo, hi := lineageVisibleSpan(graph)
		for i := 0; i <= 4; i++ {
			x := r.X + left + plotWidth*float32(i)/4
			p.StrokePath((&ui.Path{}).MoveTo(x, r.Y+50).LineTo(x, r.Y+r.H-14), 1, t.Border.Alpha(0.5))
			at := time.Unix(lo+(hi-lo)*int64(i)/4, 0)
			p.Text(x-20, r.Y+19, at.Format("15:04:05"), 10, t.TextMuted)
		}
		p.Text(r.X+14, r.Y+19, "PID / 进程（缩进表示父子层级）", 11, t.TextMuted)
		for i, node := range graph.Nodes {
			y := r.Y+62+rowHeight*float32(i)
			tone := lineageTone(t, node)
			if a.lineageSelectedPID == node.Process.PID {
				p.Fill(ui.Rect{X: r.X+5, Y: y-3, W: r.W-10, H: rowHeight-2}, t.Accent.Alpha(0.10), 5)
			}
			label := fmt.Sprintf("%d · %s", node.Process.PID, agentProcessName(node.Process))
			runes := []rune(label)
			if len(runes) > 23 {
				label = string(runes[:22]) + "…"
			}
			indent := float32(min(node.Depth, 5))*12
			p.Text(r.X+12+indent, y+3, label, 11, t.Text)
			if starts[i] < 0 {
				p.Text(r.X+left+8, y+3, "启动时间未知", 10, t.TextMuted)
				continue
			}
			start := r.X+left+starts[i]
			if start > r.X+left+plotWidth-3 {
				start = r.X+left+plotWidth-3
			}
			p.Fill(ui.Rect{X: start, Y: y+3, W: max(float32(3), r.X+left+plotWidth-start), H: 15}, tone.Alpha(0.75), 4)
			p.FillPath((&ui.Path{}).Circle(start, y+10, 4), tone)
			if node.Alerts > 0 {
				p.Fill(ui.Rect{X: start, Y: y+3, W: 3, H: 15}, t.Danger, 1)
			}
		}
	})
}

func (a *renewApp) agentLineageDetail(c *ui.Context, graph agentLineage) {
	t := c.Theme()
	index := lineageIndexForPID(graph, a.lineageSelectedPID)
	if index < 0 {
		return
	}
	node := graph.Nodes[index]
	p := node.Process
	card(c, fmt.Sprintf("当前选中 · %s / PID %d", agentProcessName(p), p.PID), func() {
		role := "普通派生进程"
		switch node.Role {
		case "agent":
			role = "根 Agent"
		case "subagent-candidate":
			role = "子 Agent 候选 · 仅进程层推断"
		}
		ui.Row(c).Wrap().Gap(8).AlignItems(ui.Center).Children(func() {
			statusPill(c, role, lineageTone(t, node))
			ui.Textf(c, "PID %d  ·  PPID %d  ·  层级 %d", p.PID, p.PPID, node.Depth).Font("monospace")
			ui.Textf(c, "事件 %d  ·  告警 %d", node.Events, node.Alerts).TextColor(t.TextMuted)
		})
		ui.Text(c, strings.TrimSpace(p.Cmdline)).Font("monospace").FontSize(11).TextColor(t.TextMuted).MaxLines(3)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if p.CreateTime > 0 {
				ui.Text(c, "启动 "+time.Unix(p.CreateTime, 0).Format("2006-01-02 15:04:05")).FontSize(11).TextColor(t.TextMuted)
			} else {
				ui.Text(c, "启动时间未知").FontSize(11).TextColor(t.TextMuted)
			}
			ui.Textf(c, "CPU %.1f%%  ·  内存 %.1f%%", p.CPU, p.MemPercent).FontSize(11).TextColor(t.TextMuted)
		})
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if ui.Button(c, "查看此 PID 的事件").Clicked() {
				a.navigatePIDEvents(p.PID)
			}
			if p.PPID > 0 && ui.Button(c, "父进程事件").Clicked() {
				a.navigatePIDEvents(p.PPID)
			}
			if node.Role == "subagent-candidate" && ui.Button(c, "以此 Agent 为根查看").Clicked() {
				a.lineageRootChoice = fmt.Sprintf("%s · %s · PID %d", agentLabelForLiveProcess(p, a.events), agentProcessName(p), p.PID)
				a.lineageSelectedPID = p.PID
			}
		})
	})
}
