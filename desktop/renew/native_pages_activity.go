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
	ui.Text(c, "先回答健康与风险，再进入事件、网络和规则等专业视图。").TextColor(t.TextMuted)

	headline, detail, level := a.overviewHeadline()
	tone := t.Success
	switch level {
	case "danger":
		tone = t.Danger
	case "warning":
		tone = t.Warning
	}
	ui.Column(c).Padding(18).Gap(12).Radius(12).Background(tone.Alpha(0.055)).Border(1, tone.Alpha(0.38)).Children(func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, headline).FontSize(20).Bold()
					statusPill(c, map[string]string{"success": "正常", "warning": "关注", "danger": "异常"}[level], tone)
				})
				ui.Text(c, detail).TextColor(t.TextMuted)
			})
			if level != "success" && a.connected {
				if ui.PrimaryButton(c, "查看相关事件").Clicked() {
					a.eventAttentionOnly = true
					a.eventVisibleLimit = 50
					a.eventSelected = -1
					a.page = "事件"
				}
			}
		})
	})

	_, attention, danger := a.riskCounts()
	ui.Row(c).Gap(12).Wrap().Children(func() {
		statCard(c, "采集状态", map[bool]string{true: "正常", false: "异常"}[a.health.CaptureHealthy], fmt.Sprintf("Ringbuf 丢弃 %d", a.health.RingbufDroppedTotal))
		statCard(c, "最近活动", fmt.Sprint(len(a.events)), "当前紧凑摘要窗口")
		statCard(c, "需关注", fmt.Sprint(attention), "风险分 ≥ 60 / ALERT")
		statCard(c, "高风险", fmt.Sprint(danger), "BLOCK / DENY / 高风险")
		if a.systemConnected {
			statCard(c, "CPU", fmt.Sprintf("%.1f%%", a.system.CPUTotal), fmt.Sprintf("%d 个实时进程", len(a.system.Processes)))
			statCard(c, "内存", fmt.Sprintf("%.1f%%", a.system.MemPercent), fmt.Sprintf("%s / %s", formatBytes(int64(a.system.MemUsed)), formatBytes(int64(a.system.MemTotal))))
		}
	})

	card(c, "最近活动", func() {
		filtered := a.filteredEvents()
		if len(filtered) == 0 {
			ui.Text(c, "暂无匹配活动。若刚启动，等待事件流进入；若有筛选条件，可在顶栏清空搜索。").TextColor(t.TextMuted)
			return
		}
		limit := min(len(filtered), 12)
		for _, event := range filtered[:limit] {
			a.eventRow(c, event)
		}
		if len(filtered) > limit {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Textf(c, "还有 %d 条活动", len(filtered)-limit).FontSize(11).TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, "打开事件页").Clicked() {
					a.page = "事件"
				}
			})
		}
	})

	card(c, "已跟踪进程", func() {
		if len(a.trackedComms) == 0 {
			ui.Text(c, "后端未返回显式跟踪列表；可在“跟踪范围”中添加命令、路径或标签。").TextColor(t.TextMuted)
			return
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for _, name := range a.trackedComms {
				ui.Badge(c, name)
			}
		})
	})
}

func (a *renewApp) overviewHeadline() (headline, detail, level string) {
	if a.starting {
		return "正在建立监控", "Renew 正在连接已有后端，或请求系统授权启动本机后端。", "warning"
	}
	if !a.connected {
		return "后端不可用", "当前无法确认系统是否正常；请检查后端连接或重新启动本机监控。", "danger"
	}
	if !a.health.CaptureHealthy {
		return "采集链路异常", fmt.Sprintf("eBPF 采集健康检查未通过；Ringbuf 累计丢弃 %d。", a.health.RingbufDroppedTotal), "danger"
	}
	_, attention, danger := a.riskCounts()
	if danger > 0 {
		return "发现高风险活动", fmt.Sprintf("当前摘要窗口中有 %d 条高风险事件，建议优先查看阻断、拒绝和高分事件。", danger), "danger"
	}
	if attention > 0 {
		return "有活动需要关注", fmt.Sprintf("当前摘要窗口中有 %d 条需关注事件；采集链路本身运行正常。", attention), "warning"
	}
	if !a.eventStreamConnected {
		return "监控在线，但事件流处于回退模式", "后端可用，Renew 正通过兼容路径同步摘要；实时性可能略低于 Native IPC。", "warning"
	}
	return "当前运行正常", "采集链路与实时事件流在线，当前摘要窗口没有需要关注的活动。", "success"
}

func (a *renewApp) eventsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "事件").FontSize(28).Bold()
	ui.Text(c, "紧凑摘要支持本地筛选与后端历史分页；完整事件只在打开详情时按 ID 读取。").TextColor(t.TextMuted)

	eventTypes, eventSessions := a.eventFilterOptions()
	ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Select(c, &a.eventTypeFilter, eventTypes).Label("事件类型").Width(170)
		ui.Select(c, &a.eventSessionFilter, eventSessions).Label("会话").Width(210)
		ui.Select(c, &a.eventDecisionFilter, []string{"", "已阻断", "告警", "已允许"}).Label("决策").Width(130)
		ui.Checkbox(c, &a.eventAttentionOnly, "只看待关注")
		if ui.Button(c, "清除筛选").Clicked() {
			a.search = ""
			a.eventTypeFilter = ""
			a.eventSessionFilter = ""
			a.eventDecisionFilter = ""
			a.eventAttentionOnly = false
			a.eventVisibleLimit = 50
			a.eventSelected = -1
		}
	})

	rows := a.filteredEvents()
	limit := a.eventVisibleLimit
	if limit <= 0 {
		limit = 50
	}
	if limit > len(rows) {
		limit = len(rows)
	}
	visible := rows[:limit]

	card(c, fmt.Sprintf("活动 · %d / 已加载 %d", len(rows), len(a.events)), func() {
		if len(visible) == 0 {
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
		a.eventTable.Key = func(row int) any { return visible[row].EventID }
		ui.Table(c, &a.eventTable, cols, len(visible), func(row, col int) {
			e := visible[row]
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
				risk := eventRisk(e)
				ui.Text(c, risk).TextColor(riskTextColor(t, risk)).SingleLine()
			case 5:
				if e.RiskScore > 0 {
					ui.Textf(c, "%.0f", e.RiskScore)
				} else {
					ui.Text(c, "-")
				}
			}
		}).Height(440).Label("事件摘要")
		if a.eventSelected >= 0 && a.eventSelected < len(visible) {
			selected := visible[a.eventSelected]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, selected.EventID).Font("monospace").FontSize(10).TextColor(t.TextMuted).Grow(1)
				if ui.PrimaryButton(c, "详细").Clicked() {
					a.openEventDetail(selected.EventID)
				}
			})
		}
	})

	ui.Row(c).Gap(10).Wrap().Children(func() {
		if len(rows) > limit && ui.Button(c, "展开更多（+50）").Clicked() {
			a.eventVisibleLimit += 50
		}
		if strings.TrimSpace(a.historyCursor) != "" {
			label := "加载更早记录"
			if a.historyLoading {
				label = "正在读取…"
			}
			if ui.Button(c, label).Clicked() && !a.historyLoading {
				a.loadOlderEvents()
			}
		}
	})
	if strings.TrimSpace(a.historyCursor) == "" && a.historyInitialized {
		ui.Text(c, "已到当前后端历史窗口的末尾。").FontSize(10).TextColor(t.TextMuted)
	}
}
func (a *renewApp) networkView(c *ui.Context) {
	t := c.Theme()
	rows := a.filteredNetworkRows()
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
	ui.Text(c, "进程").FontSize(28).Bold()

	if a.systemConnected && len(a.system.Processes) > 0 {
		rows := a.filteredSystemProcesses()
		ui.Text(c, "来自 /ws/system 的 protobuf 实时进程快照，按 CPU 使用率排序；搜索同时匹配 PID、用户与命令行。").TextColor(t.TextMuted)
		card(c, fmt.Sprintf("实时进程 · %d", len(rows)), func() {
			if len(rows) == 0 {
				ui.Text(c, "当前搜索没有匹配进程").TextColor(t.TextMuted)
				return
			}
			cols := []ui.TableColumn{
				{Title: "PID", Width: 82, Fixed: true},
				{Title: "PPID", Width: 82, Align: ui.End},
				{Title: "进程", MinWidth: 170},
				{Title: "CPU", Width: 80, Align: ui.End},
				{Title: "内存", Width: 80, Align: ui.End},
				{Title: "用户", Width: 120},
			}
			a.processTable.Key = func(row int) any { return rows[row].PID }
			table := ui.Table(c, &a.processTable, cols, len(rows), func(row, col int) {
				p := rows[row]
				switch col {
				case 0:
					ui.Text(c, strconv.Itoa(p.PID)).Font("monospace")
				case 1:
					ui.Text(c, strconv.Itoa(p.PPID)).Font("monospace")
				case 2:
					ui.Text(c, displayOr(p.Name, "未知进程")).SingleLine()
				case 3:
					ui.Textf(c, "%.1f%%", p.CPU)
				case 4:
					ui.Textf(c, "%.1f%%", p.MemPercent)
				case 5:
					ui.Text(c, displayOr(p.User, "-")).SingleLine()
				}
			}).Height(410).Label("实时进程")
			if table.Submitted() && a.processSelected >= 0 && a.processSelected < len(rows) {
				// Keep the selection visible below; submit is an accessibility-
				// friendly equivalent of opening the row detail.
			}
			if a.processSelected >= 0 && a.processSelected < len(rows) {
				p := rows[a.processSelected]
				ui.Column(c).Gap(5).Children(func() {
					ui.Textf(c, "%s · PID %d / PPID %d", displayOr(p.Name, "未知进程"), p.PID, p.PPID).Bold()
					ui.Text(c, displayOr(p.Cmdline, "后端未提供命令行")).Font("monospace").FontSize(10).TextColor(t.TextMuted).MaxLines(4)
				})
			}
		})
		return
	}

	rows := a.filteredProcessAggregateRows()
	ui.Text(c, "系统 protobuf 流当前不可用，降级展示已加载事件窗口中真实出现过的进程活动。").TextColor(t.TextMuted)
	if a.systemErr != "" {
		ui.Text(c, a.systemErr).FontSize(10).TextColor(t.TextMuted)
	}
	card(c, fmt.Sprintf("事件活动进程 · %d", len(rows)), func() {
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

	ui.Row(c).Gap(12).Wrap().Children(func() {
		state := "异常"
		if a.health.CaptureHealthy {
			state = "正常"
		}
		statCard(c, "采集器", state, fmt.Sprintf("Ringbuf 丢弃 %d", a.health.RingbufDroppedTotal))
		if a.systemConnected {
			statCard(c, "CPU", fmt.Sprintf("%.1f%%", a.system.CPUTotal), fmt.Sprintf("%d 个进程", len(a.system.Processes)))
			statCard(c, "内存", fmt.Sprintf("%.1f%%", a.system.MemPercent), fmt.Sprintf("%s / %s", formatBytes(int64(a.system.MemUsed)), formatBytes(int64(a.system.MemTotal))))
			statCard(c, "系统流", "实时", "protobuf /ws/system")
		} else {
			statCard(c, "系统流", "重连中", "protobuf /ws/system")
		}
	})

	card(c, "I/O 快照", func() {
		if !a.systemConnected {
			ui.Text(c, "等待系统 protobuf 流…").TextColor(t.TextMuted)
			if a.systemErr != "" {
				ui.Text(c, a.systemErr).FontSize(10).TextColor(t.TextMuted)
			}
			return
		}
		ui.Row(c).Gap(20).Wrap().Children(func() {
			ui.Text(c, "磁盘读 "+formatBytes(int64(a.system.DiskRead))).Font("monospace")
			ui.Text(c, "磁盘写 "+formatBytes(int64(a.system.DiskWrite))).Font("monospace")
			ui.Text(c, "网络收 "+formatBytes(int64(a.system.NetRecv))).Font("monospace")
			ui.Text(c, "网络发 "+formatBytes(int64(a.system.NetSent))).Font("monospace")
		})
		if !a.system.FetchedAt.IsZero() {
			ui.Text(c, "系统快照："+a.system.FetchedAt.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted)
		}
	})

	card(c, "后端与队列", func() {
		ui.Text(c, a.backend).Font("monospace")
		ui.Textf(c, "后端队列：%d", a.health.BackendQueueLen).TextColor(t.TextMuted)
		ui.Textf(c, "持久化队列：%d / %d · pending %d", a.health.PersistQueueLen, a.health.PersistQueueCap, a.health.PersistPending).TextColor(t.TextMuted)
		ui.Textf(c, "桌面事件合并队列：%d / %d · 丢弃 %d", len(a.eventUIQueue), eventUIQueueSize, a.eventUIDroppedCount()).TextColor(t.TextMuted)
		if !a.lastSync.IsZero() {
			ui.Text(c, "摘要同步："+a.lastSync.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted)
		}
		ui.Text(c, "桌面端为纯 Go/MyGo Native UI；系统实时数据直接解码后端 protobuf。").FontSize(11).TextColor(t.TextMuted)
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
					riskPill(c, risk)
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
		target := strings.TrimSpace(e.Target)
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
	if e.Network {
		return true
	}
	t := strings.ToLower(e.Type)
	return strings.Contains(t, "network") || strings.Contains(t, "connect") || strings.Contains(t, "tcp") || strings.Contains(t, "dns") || strings.Contains(t, "socket")
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
