package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

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
		if a.networkSelected >= 0 && a.networkSelected < len(rows) {
			target := rows[a.networkSelected].Target
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, target).FontSize(11).Font("monospace").TextColor(t.TextMuted).Grow(1).MaxLines(2)
				if ui.PrimaryButton(c, "查看该目标事件").Clicked() {
					a.navigateTargetEvents(target)
				}
			})
		}
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
					harnessIdentity(c, displayOr(p.Name, "未知进程"), p.Name)
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
				ui.Column(c).Gap(6).Children(func() {
					ui.Textf(c, "%s · PID %d / PPID %d", displayOr(p.Name, "未知进程"), p.PID, p.PPID).Bold()
					ui.Text(c, displayOr(p.Cmdline, "后端未提供命令行")).Font("monospace").FontSize(10).TextColor(t.TextMuted).MaxLines(4)
					ui.Row(c).Wrap().Gap(6).Children(func() {
						if agentLabelForLiveProcess(p, a.events) != "" && ui.Button(c, "查看派生关系").Clicked() {
							a.lineageRootChoice = fmt.Sprintf("%s · %s · PID %d", agentLabelForLiveProcess(p, a.events), agentProcessName(p), p.PID)
							a.lineageSelectedPID = p.PID
							a.page = "派生"
						}
						if ui.Button(c, "此 PID 事件").Clicked() {
							a.navigatePIDEvents(p.PID)
						}
						if p.PPID > 0 && ui.Button(c, "父 PID 事件").Clicked() {
							a.navigatePIDEvents(p.PPID)
						}
					})
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
				harnessIdentity(c, displayOr(r.Comm, "未知进程"), r.Comm)
			case 3:
				ui.Text(c, strconv.Itoa(r.Events))
			case 4:
				ui.Textf(c, "%.0f", r.Risk)
			case 5:
				ui.Text(c, summaryTime(r.LastMS)).SingleLine()
			}
		}).Height(480).Label("活动进程")
		if a.processSelected >= 0 && a.processSelected < len(rows) {
			pid := rows[a.processSelected].PID
			ui.Row(c).Gap(6).Wrap().Children(func() {
				if ui.Button(c, fmt.Sprintf("PID %d 事件", pid)).Clicked() {
					a.navigatePIDEvents(pid)
				}
				if rows[a.processSelected].PPID > 0 &&
					ui.Button(c, "父 PID 事件").Clicked() {
					a.navigatePIDEvents(rows[a.processSelected].PPID)
				}
			})
		}
	})
}
func (a *renewApp) systemView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "系统").FontSize(28).Bold()

	_, attention, danger := a.riskCounts()
	if !a.connected || (a.healthReady && !a.health.CaptureHealthy) ||
		!a.eventStreamConnected || !a.systemConnected || danger > 0 || attention > 0 {
		card(c, "异常与快速处置", func() {
			if !a.connected {
				statusPill(c, "后端离线", t.Danger)
				ui.Text(c, "后端未连接，当前无法确认采集状态。").TextColor(t.TextMuted)
			} else if a.healthReady && !a.health.CaptureHealthy {
				statusPill(c, "采集异常", t.Danger)
				ui.Text(c, "采集器报告异常，可检查监控模块与数据采集配置。").TextColor(t.TextMuted)
			} else if !a.eventStreamConnected || !a.systemConnected {
				statusPill(c, "连接降级", t.Warning)
				ui.Text(c, "事件流或系统流正在恢复；可以立即重新同步。").TextColor(t.TextMuted)
			}
			if danger > 0 || attention > 0 {
				ui.Textf(c, "当前事件窗口：%d 条高风险、%d 条需关注", danger, attention)
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if ui.PrimaryButton(c, "定位异常事件").Clicked() {
					a.eventAttentionOnly = true
					a.eventTypeFilter, a.eventSessionFilter, a.eventDecisionFilter, a.search = "", "", "", ""
					a.page = "事件"
				}
				if ui.Button(c, "采集设置").Clicked() {
					a.page = "监控"
				}
				if ui.Button(c, "跟踪范围").Clicked() {
					a.page = "跟踪"
				}
				if ui.Button(c, "立即重连").Clicked() {
					go a.refresh(context.Background())
					go a.refreshConfiguration(context.Background())
				}
			})
		})
	}

	ui.Row(c).Gap(12).Wrap().Children(func() {
		collectorLabel, _ := a.collectorStatus()
		statCard(c, "采集器", collectorLabel, fmt.Sprintf("Ringbuf 丢弃 %d", a.health.RingbufDroppedTotal))
		if a.systemConnected {
			statCard(c, "CPU", fmt.Sprintf("%.1f%%", a.system.CPUTotal), fmt.Sprintf("%d 个进程", len(a.system.Processes)))
			statCard(c, "内存", fmt.Sprintf("%.1f%%", a.system.MemPercent), fmt.Sprintf("%s / %s", formatBytes(int64(a.system.MemUsed)), formatBytes(int64(a.system.MemTotal))))
			statCard(c, "系统流", "实时", "protobuf / Native IPC")
		} else if a.connected {
			statCard(c, "系统流", "重连中", "Native IPC / WebSocket")
		} else {
			statCard(c, "系统流", "不可用", "后端离线")
		}
	})

	card(c, "I/O 快照", func() {
		if !a.systemConnected {
			if a.connected {
				ui.Text(c, "等待系统 protobuf 流…").TextColor(t.TextMuted)
			} else {
				ui.Text(c, "后端离线，系统快照暂不可用。").TextColor(t.TextMuted)
			}
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
		ui.Textf(c, "桌面事件通道：%d / %d · 本地批次 %d / %d (峰值 %d)",
			len(a.eventUIQueue), eventUIQueueSize, a.eventUIBatchBuffered.Load(),
			eventUIPauseCapacity, a.eventUIBatchPeak.Load()).TextColor(t.TextMuted)
		ui.Textf(c, "桌面摘要丢弃：通道溢出 %d · 批次淘汰 %d · 合计 %d",
			a.eventUIQueueDropped.Load(), a.eventUIBatchDropped.Load(),
			a.eventUIDroppedCount()).TextColor(t.TextMuted)
		ui.Textf(c, "桌面提交：%d 批 / %d 条 · 最近 %d 条 · 界面繁忙延后 %d 次",
			a.eventUICommits.Load(), a.eventUICommitted.Load(),
			a.eventUILastCommit.Load(), a.eventUIBusyTicks.Load()).TextColor(t.TextMuted)
		if !a.lastSync.IsZero() {
			ui.Text(c, "摘要同步："+a.lastSync.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted)
		}
		ui.Text(c, "桌面端为纯 Go/MyGo Native UI；系统实时数据直接解码后端 protobuf。").FontSize(11).TextColor(t.TextMuted)
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
