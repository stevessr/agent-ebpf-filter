package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

type networkAggregate struct {
	Target string
	Events int
	Bytes  int64
	PIDs   map[int]struct{}
	Comms  map[string]struct{}
	LastMS int64
	Risk   float64
}

type processAggregate struct {
	PID    int
	PPID   int
	Comm   string
	Events int
	LastMS int64
	Risk   float64
}

func (a *renewApp) overview(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "现在正常吗？").FontSize(28).Bold()
	ui.Text(c, "原生 Go UI 直接读取 Agent eBPF Filter 后端，不启动 WebView。").TextColor(t.TextMuted)

	_, attention, danger := a.riskCounts()
	ui.Row(c).Gap(12).Wrap().Children(func() {
		statCard(c, "采集状态", map[bool]string{true: "正常", false: "异常"}[a.health.CaptureHealthy], fmt.Sprintf("Ringbuf 丢弃 %d", a.health.RingbufDroppedTotal))
		statCard(c, "最近活动", fmt.Sprint(len(a.events)), "当前紧凑摘要窗口")
		statCard(c, "需关注", fmt.Sprint(attention), "风险分 ≥ 60 / ALERT")
		statCard(c, "高风险", fmt.Sprint(danger), "BLOCK / DENY / 高风险")
	})

	card(c, "最近活动", func() {
		filtered := a.filteredEvents()
		if len(filtered) == 0 {
			ui.Text(c, "暂无匹配活动").TextColor(t.TextMuted)
			return
		}
		limit := min(len(filtered), 12)
		for _, event := range filtered[:limit] {
			a.eventRow(c, event)
		}
		if len(filtered) > limit {
			ui.Textf(c, "还有 %d 条，可在“事件”页查看", len(filtered)-limit).FontSize(11).TextColor(t.TextMuted)
		}
	})

	card(c, "已跟踪进程", func() {
		if len(a.trackedComms) == 0 {
			ui.Text(c, "后端未返回显式跟踪列表").TextColor(t.TextMuted)
			return
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for _, name := range a.trackedComms {
				ui.Badge(c, name)
			}
		})
	})
}

func (a *renewApp) eventsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "事件").FontSize(28).Bold()
	ui.Text(c, "当前仅保留紧凑摘要；完整事件仍由后端持久化。").TextColor(t.TextMuted)
	rows := a.filteredEvents()
	card(c, fmt.Sprintf("活动 · %d", len(rows)), func() {
		if len(rows) == 0 {
			ui.Text(c, "没有匹配事件").TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "时间", Width: 78, Fixed: true},
			{Title: "动作", Width: 150},
			{Title: "进程", Width: 120},
			{Title: "目标", MinWidth: 220},
			{Title: "风险", Width: 86},
			{Title: "分数", Width: 64, Align: ui.End},
		}
		a.eventTable.Key = func(row int) any { return rows[row].EventID }
		ui.Table(c, &a.eventTable, cols, len(rows), func(row, col int) {
			e := rows[row]
			switch col {
			case 0:
				ui.Text(c, eventTime(e)).Font("monospace").SingleLine()
			case 1:
				ui.Text(c, eventAction(e)).SingleLine()
			case 2:
				ui.Text(c, displayOr(e.Comm, "-")).SingleLine()
			case 3:
				ui.Text(c, eventTarget(e)).SingleLine()
			case 4:
				ui.Text(c, eventRisk(e)).SingleLine()
			case 5:
				if e.RiskScore > 0 {
					ui.Textf(c, "%.0f", e.RiskScore)
				} else {
					ui.Text(c, "-")
				}
			}
		}).Height(480).Label("事件摘要")
	})
}

func (a *renewApp) networkView(c *ui.Context) {
	t := c.Theme()
	rows := aggregateNetwork(a.filteredEvents())
	ui.Text(c, "网络").FontSize(28).Bold()
	ui.Text(c, "按当前有界事件摘要窗口聚合外联目标；事件数和字节数不是完整连接/流量统计。").TextColor(t.TextMuted)
	card(c, fmt.Sprintf("访问目标 · %d", len(rows)), func() {
		if len(rows) == 0 {
			ui.Text(c, "当前摘要窗口没有网络目标").TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "目标", MinWidth: 260, Fixed: true},
			{Title: "事件", Width: 80, Align: ui.End},
			{Title: "摘要字节", Width: 110, Align: ui.End},
			{Title: "PID", Width: 80, Align: ui.End},
			{Title: "进程", Width: 150},
			{Title: "最高风险", Width: 90, Align: ui.End},
		}
		a.networkTable.Key = func(row int) any { return rows[row].Target }
		ui.Table(c, &a.networkTable, cols, len(rows), func(row, col int) {
			r := rows[row]
			switch col {
			case 0:
				ui.Text(c, r.Target).SingleLine()
			case 1:
				ui.Text(c, strconv.Itoa(r.Events))
			case 2:
				ui.Text(c, formatBytes(r.Bytes))
			case 3:
				ui.Text(c, strconv.Itoa(len(r.PIDs)))
			case 4:
				ui.Text(c, joinKeys(r.Comms, 3)).SingleLine()
			case 5:
				ui.Textf(c, "%.0f", r.Risk)
			}
		}).Height(480).Label("网络目标")
	})
}

func (a *renewApp) processesView(c *ui.Context) {
	t := c.Theme()
	rows := aggregateProcesses(a.filteredEvents())
	ui.Text(c, "进程").FontSize(28).Bold()
	ui.Text(c, "当前先展示事件窗口中真实出现过的进程活动；系统 protobuf 快照保持独立协议，不用 JSON 旁路伪造。").TextColor(t.TextMuted)
	card(c, fmt.Sprintf("活动进程 · %d", len(rows)), func() {
		if len(rows) == 0 {
			ui.Text(c, "当前摘要窗口没有进程活动").TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "PID", Width: 86, Fixed: true},
			{Title: "PPID", Width: 86, Align: ui.End},
			{Title: "进程", MinWidth: 200},
			{Title: "事件", Width: 80, Align: ui.End},
			{Title: "最高风险", Width: 90, Align: ui.End},
			{Title: "最近活动", Width: 100},
		}
		a.processTable.Key = func(row int) any { return rows[row].PID }
		ui.Table(c, &a.processTable, cols, len(rows), func(row, col int) {
			r := rows[row]
			switch col {
			case 0:
				ui.Text(c, strconv.Itoa(r.PID)).Font("monospace")
			case 1:
				if r.PPID > 0 {
					ui.Text(c, strconv.Itoa(r.PPID)).Font("monospace")
				} else {
					ui.Text(c, "-")
				}
			case 2:
				ui.Text(c, displayOr(r.Comm, "未知进程")).SingleLine()
			case 3:
				ui.Text(c, strconv.Itoa(r.Events))
			case 4:
				ui.Textf(c, "%.0f", r.Risk)
			case 5:
				ui.Text(c, summaryTime(r.LastMS)).SingleLine()
			}
		}).Height(480).Label("活动进程")
	})
}

func (a *renewApp) systemView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "系统").FontSize(28).Bold()
	card(c, "采集器", func() {
		state := "异常"
		if a.health.CaptureHealthy {
			state = "正常"
		}
		ui.Text(c, state).FontSize(22).Bold()
		ui.Textf(c, "Ringbuf 丢弃总数：%d", a.health.RingbufDroppedTotal).TextColor(t.TextMuted)
		if !a.lastSync.IsZero() {
			ui.Text(c, "最后同步："+a.lastSync.Format("15:04:05")).FontSize(12).TextColor(t.TextMuted)
		}
	})
	card(c, "连接", func() {
		ui.Text(c, a.backend).Font("monospace")
		ui.Text(c, "桌面端为纯 Go/MyGo Native UI；专业工作台仍可单独在浏览器打开。").TextColor(t.TextMuted)
	})
}

func (a *renewApp) eventRow(c *ui.Context, e eventSummary) {
	t := c.Theme()
	ui.Row(c).Padding(9, 0).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, eventTime(e)).Width(66).Font("monospace").FontSize(11).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, eventAction(e)).Bold()
				if e.Comm != "" {
					ui.Badge(c, e.Comm)
				}
				risk := eventRisk(e)
				if risk != "正常" {
					ui.Badge(c, risk)
				}
			})
			ui.Text(c, eventTarget(e)).FontSize(12).TextColor(t.TextMuted).MaxLines(1)
		})
		if e.RiskScore > 0 {
			ui.Textf(c, "%.0f", e.RiskScore).Width(36).Font("monospace").TextColor(t.TextMuted)
		}
	})
}

func aggregateNetwork(events []eventSummary) []networkAggregate {
	byTarget := make(map[string]*networkAggregate)
	for _, e := range events {
		target := strings.TrimSpace(e.Domain)
		if target == "" {
			target = strings.TrimSpace(e.NetEndpoint)
		}
		if target == "" || !isNetworkEvent(e) {
			continue
		}
		row := byTarget[target]
		if row == nil {
			row = &networkAggregate{Target: target, PIDs: make(map[int]struct{}), Comms: make(map[string]struct{})}
			byTarget[target] = row
		}
		row.Events++
		if e.NetBytes > 0 {
			row.Bytes += e.NetBytes
		}
		if e.ReceivedAtMS > row.LastMS {
			row.LastMS = e.ReceivedAtMS
		}
		if e.RiskScore > row.Risk {
			row.Risk = e.RiskScore
		}
		if e.PID > 0 {
			row.PIDs[e.PID] = struct{}{}
		}
		if strings.TrimSpace(e.Comm) != "" {
			row.Comms[e.Comm] = struct{}{}
		}
	}
	rows := make([]networkAggregate, 0, len(byTarget))
	for _, row := range byTarget {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Events != rows[j].Events {
			return rows[i].Events > rows[j].Events
		}
		return rows[i].LastMS > rows[j].LastMS
	})
	return rows
}

func aggregateProcesses(events []eventSummary) []processAggregate {
	byPID := make(map[int]*processAggregate)
	for _, e := range events {
		if e.PID <= 0 {
			continue
		}
		row := byPID[e.PID]
		if row == nil {
			row = &processAggregate{PID: e.PID}
			byPID[e.PID] = row
		}
		row.Events++
		if e.ReceivedAtMS > row.LastMS {
			row.LastMS = e.ReceivedAtMS
		}
		if e.RiskScore > row.Risk {
			row.Risk = e.RiskScore
		}
		if e.PPID > 0 {
			row.PPID = e.PPID
		}
		if strings.TrimSpace(e.Comm) != "" {
			row.Comm = e.Comm
		}
	}
	rows := make([]processAggregate, 0, len(byPID))
	for _, row := range byPID {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LastMS != rows[j].LastMS {
			return rows[i].LastMS > rows[j].LastMS
		}
		return rows[i].Events > rows[j].Events
	})
	return rows
}

func isNetworkEvent(e eventSummary) bool {
	if strings.TrimSpace(e.NetEndpoint) != "" || strings.TrimSpace(e.Domain) != "" {
		return true
	}
	t := strings.ToLower(e.Type)
	return strings.Contains(t, "network") || strings.Contains(t, "tcp") || strings.Contains(t, "dns") || strings.Contains(t, "socket")
}

func formatBytes(v int64) string {
	if v < 1024 {
		return fmt.Sprintf("%d B", v)
	}
	if v < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(v)/1024)
	}
	if v < 1024*1024*1024 {
		return fmt.Sprintf("%.1f MiB", float64(v)/(1024*1024))
	}
	return fmt.Sprintf("%.1f GiB", float64(v)/(1024*1024*1024))
}

func joinKeys(values map[string]struct{}, limit int) string {
	keys := make([]string, 0, len(values))
	for value := range values {
		keys = append(keys, value)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		return strings.Join(keys[:limit], ", ") + fmt.Sprintf(" +%d", len(keys)-limit)
	}
	return strings.Join(keys, ", ")
}

func displayOr(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func summaryTime(ms int64) string {
	if ms <= 0 {
		return "--:--:--"
	}
	return eventTime(eventSummary{ReceivedAtMS: ms})
}
