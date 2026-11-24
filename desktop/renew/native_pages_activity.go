package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/stevessr/agent-ebpf-filter/desktop/renew/design"
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
	headline, detail, level := a.overviewHeadline()
	tone := workspaceStatusTone(t, level)
	_, attention, danger := a.riskCounts()
	// Only the low-frequency collector status changes the surface tint.
	// High contrast keeps its original surface, border and text contrast.
	statusSurface := t.Surface
	if !c.Preferences().HighContrast {
		statusSurface = t.Surface.Mix(tone, 0.08)
	}

	// The default desktop landing view answers "am I being monitored, and
	// what should I do?" instead of showing a second investigator dashboard.
	ui.Column(c).Key("renew-overview-health").Padding(18).Gap(12).Radius(12).
		Background(statusSurface).Border(1, t.Border).
		Transition(renewStatusColorTransition()).Children(func() {
		ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(42, 42).Radius(12).Background(t.Accent.Alpha(0.17)).Center().Children(func() {
				ui.Icon(c, renewMirrorSVG).Size(29, 29).TextColor(t.Accent).Label(desktopBrandName)
			})
			ui.Column(c).Grow(1).MinWidth(210).Gap(3).Children(func() {
				ui.Text(c, "我的设备").FontSize(22).Bold()
				ui.Text(c, "监控状态与需要处理的活动").FontSize(12).TextColor(t.TextMuted)
			})
			status, statusLevel := a.pipelineStatus()
			statusPill(c, status, workspaceStatusTone(t, statusLevel))
		})
		ui.Divider(c)
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, headline).FontSize(19).Bold().TextColor(tone)
			statusPill(c, map[string]string{"success": "正常", "warning": "关注", "danger": "异常"}[level], tone)
		})
		ui.Text(c, detail).TextColor(t.TextMuted)
		ui.Text(c, a.overviewSnapshotHint()).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			if actionLabel, actionPage, attentionOnly := a.overviewAction(); actionLabel != "" {
				if ui.PrimaryButton(c, actionLabel).Clicked() {
					if attentionOnly {
						a.clearEventFilters()
						a.eventAttentionOnly = true
					}
					a.page = actionPage
				}
			} else if !a.connected && !a.starting {
				if ui.PrimaryButton(c, "重试连接").Clicked() {
					a.retryBackendConnection()
				}
			}
			if ui.Button(c, "查看全部活动").Clicked() {
				a.clearEventFilters()
				a.page = "事件"
			}
		})
	})

	design.MetricStrip(c, overviewMetricSpecs(len(a.events), attention, danger))
	ui.Text(c, "数量只统计当前摘要窗口，不代表设备所有历史活动；断线或采集异常时不能据此判断设备安全。").
		FontSize(11).TextColor(t.TextMuted)

	ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		if design.Action(c, "查看联网行为", design.ActionNeutral).Tooltip("查看 Agent 关联的域名及 IP 连接").Clicked() {
			a.page = "域名"
		}
		if design.Action(c, "调整监控范围", design.ActionNeutral).Tooltip("选择日常或深度采集方案，不会直接修改拦截策略").Clicked() {
			a.page = "监控"
		}
		if design.Action(c, "查看访问权限", design.ActionNeutral).Tooltip("查看或编辑路径访问规则，策略生效以服务端反馈为准").Clicked() {
			a.page = "路径权限"
		}
	})

	groups, hidden := a.personalAlertGroups()
	if len(groups) > 0 || len(a.personalAlertMuteOrder) > 0 {
		// Reveal on first attention only; a stable key prevents live summary
		// updates from animating the entire incident group repeatedly.
		ui.Column(c).Key("renew-priority-alerts").Transition(renewRevealTransition()).Children(func() {
			card(c, "优先核查", func() {
				ui.Text(c, "按风险等级展示相似活动，数量只来自已加载摘要。高风险始终显示；收起关注项只影响本次桌面提醒，不会放行或修改策略。").
					FontSize(11).TextColor(t.TextMuted)
				if len(a.personalAlertMuteOrder) > 0 {
					ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
						ui.Textf(c, "本次已收起 %d 类关注提示（当前窗口 %d 条），记录仍可在事件页查看。",
							len(a.personalAlertMuteOrder), hidden).FontSize(11).TextColor(t.TextMuted).Grow(1)
						if ui.Button(c, "恢复全部提示").Clicked() {
							a.restorePersonalAlerts()
						}
					})
				}
				if len(groups) == 0 {
					ui.Text(c, "当前未显示需要核查的提醒。已收起的关注事件仍保留在事件记录中。").
						FontSize(12).TextColor(t.TextMuted)
				}
				for _, group := range groups[:min(len(groups), 3)] {
					group := group
					ui.Column(c).Gap(5).Children(func() {
						a.eventRow(c, group.Event)
						ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
							if group.Count > 1 {
								ui.Badge(c, fmt.Sprintf("同类活动 %d 条", group.Count))
							}
							if group.Risk == "需关注" {
								if ui.Button(c, "本次收起同类提示").
									Tooltip("只在本次运行中收起相同进程、目标、动作和风险等级的关注提示；不影响原始记录、规则或高风险事件").Clicked() {
									a.mutePersonalAlert(group)
								}
							}
						})
					})
				}
				if len(groups) > 3 {
					ui.Textf(c, "另有 %d 类待关注活动，可在事件页查看。", len(groups)-3).
						FontSize(11).TextColor(t.TextMuted)
				}
				if ui.Button(c, "查看所有需关注活动").Clicked() {
					a.clearEventFilters()
					a.eventAttentionOnly = true
					a.page = "事件"
				}
			})
		})
	}

	card(c, "最近活动", func() {
		// This summary must not silently inherit filters from the Events
		// page, otherwise a clean home could be mistaken for no activity.
		if len(a.events) == 0 {
			design.EmptyState(c, "暂无活动摘要", "可能是刚启动、未匹配到采集范围，或事件流暂不可用；这不是安全证明。")
		} else {
			for _, event := range a.overviewRecentEvents() {
				a.eventRow(c, event)
			}
		}
		if ui.Button(c, "打开事件记录").Tooltip("清除旧筛选后查看事件详情与证据").Clicked() {
			a.clearEventFilters()
			a.page = "事件"
		}
	})
	design.NoticeCard(c, design.Notice{
		Title: "证据与处置",
		Message: "风险分数和 ALERT 表示需要核查，并不等于已拦截；请在事件详情核对决策、执行结果和证据。",
		Tone: design.Warning,
	})
}

func (a *renewApp) overviewHeadline() (headline, detail, level string) {
	if a.starting {
		return "正在建立监控", "明镜高悬正在连接已有后端，或请求系统授权启动本机后端。", "warning"
	}
	if !a.connected {
		return "后端不可用", "当前无法确认系统是否正常；请检查后端连接或重新启动本机监控。", "danger"
	}
	if !a.healthReady {
		return "正在同步运行状态", "后端已经连接，正在等待第一份采集器健康状态与事件摘要快照。", "warning"
	}
	if !a.health.CaptureHealthy {
		return "采集链路异常", fmt.Sprintf("eBPF 采集健康检查未通过；Ringbuf 累计丢弃 %d。", a.health.RingbufDroppedTotal), "danger"
	}
	_, attention, danger := a.riskCounts()
	if danger > 0 {
		return "发现高风险活动", fmt.Sprintf("当前摘要窗口中有 %d 条高风险事件，建议优先查看阻断、拒绝和高分事件。", danger), "danger"
	}
	if attention > 0 {
		if a.syncFreshnessAt(time.Now()) == "stale" || a.syncFreshnessAt(time.Now()) == "clock-skew" {
			return "有活动需要关注", fmt.Sprintf("当前摘要窗口中有 %d 条需关注事件，但采集同步状态异常；请核查时间和连接。", attention), "warning"
		}
		return "有活动需要关注", fmt.Sprintf("当前摘要窗口中有 %d 条需关注事件；采集链路本身运行正常。", attention), "warning"
	}
	if a.syncFreshnessAt(time.Now()) == "unknown" {
		return "等待采集同步", "采集器报告可用，但尚未收到带时间戳的有效同步快照；暂无法确认监控实时性。", "warning"
	}
	if a.syncFreshnessAt(time.Now()) == "stale" {
		return "采集证据已过时", "最近两分钟没有新的健康同步，无法仅凭连接状态确认监控实时性；建议查看系统诊断。", "warning"
	}
	if a.syncFreshnessAt(time.Now()) == "clock-skew" {
		return "采集时间异常", "同步记录晚于本机时钟；请检查时钟与系统诊断，暂勿将摘要视为完整实时证据。", "warning"
	}
	if !a.eventStreamConnected {
		return "监控在线，但事件流处于回退模式", "后端可用，当前正通过兼容路径同步摘要；实时性可能略低于 Native IPC。", "warning"
	}
	if !a.systemConnected {
		return "监控在线，但系统流正在重连", "事件采集仍在工作；CPU、内存和实时进程信息暂时不可用或正在恢复。", "warning"
	}
	return "当前运行正常", "采集链路、事件流和系统流在线，当前摘要窗口没有需要关注的活动。", "success"
}

func (a *renewApp) overviewAction() (label, page string, attentionOnly bool) {
	if a.starting || !a.connected || !a.healthReady {
		return "", "", false
	}
	if !a.health.CaptureHealthy {
		return "打开系统诊断", "系统", false
	}
	_, attention, danger := a.riskCounts()
	if danger > 0 || attention > 0 {
		return "查看相关事件", "事件", true
	}
	if a.syncFreshnessAt(time.Now()) != "fresh" || !a.eventStreamConnected || !a.systemConnected {
		return "打开系统诊断", "系统", false
	}
	return "", "", false
}

// eventDisplaySliderMax grows in whole steps to follow explicit “show more”
// and locate-by-ID actions without silently capping those existing workflows.
func eventDisplaySliderMax(visibleLimit int) float64 {
	if visibleLimit <= 500 {
		return 500
	}
	return float64(((visibleLimit-1)/50 + 1) * 50)
}

func (a *renewApp) eventsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "事件").FontSize(28).Bold()
	ui.Text(c, "紧凑摘要支持本地筛选与后端历史分页；完整事件只在打开详情时按 ID 读取。").TextColor(t.TextMuted)
	if a.eventReturnPage != "" && a.eventReturnPage != "事件" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.Button(c, "← 返回"+a.eventReturnPage).Clicked() {
				a.page = a.eventReturnPage
				a.eventReturnPage = ""
			}
		})
	}

	eventTypes, eventSessions := a.eventFilterOptions()
	ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Select(c, &a.eventTypeFilter, eventTypes).Label("事件类型").Width(170)
		ui.Select(c, &a.eventSessionFilter, eventSessions).Label("会话").Width(210)
		ui.Select(c, &a.eventDecisionFilter, []string{"", "已阻断", "告警", "已允许"}).Label("决策").Width(130)
		ui.Select(c, &a.eventRiskFilter, []string{"", "高风险", "需关注", "正常"}).Label("风险等级").Width(130)
		ui.Checkbox(c, &a.eventAttentionOnly, "只看待关注")
		if ui.Button(c, "清除筛选").Clicked() {
			a.clearEventFilters()
		}
	})
	// View-only filtering: the slider never changes backend capture or enforcement.
	ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Textf(c, "最低风险分 %.1f（0 = 全部）", a.eventMinRiskScore).FontSize(11).TextColor(t.TextMuted)
		if ui.Slider(c.Key("minimum-risk-score"), &a.eventMinRiskScore, 0, 100).
			Label("最低风险分").Width(220).Changed() {
			// Filtering changes row indices; never leave a stale selection.
			a.eventSelected = -1
			a.inspectorSelectedID = ""
		}
	})

	if a.managementEventFilter != "" {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			statusPill(c, "管理器 · "+ccsAppLabel(a.managementEventFilter), t.Accent)
			if ui.Button(c, "移除 CCS 筛选").Clicked() {
				a.managementEventFilter = ""
			}
		})
	}

	if a.eventPIDFilter > 0 || a.eventRootPIDFilter > 0 ||
		a.eventTargetFilter != "" || a.eventDomainTarget != "" || a.eventFileEditsOnly || a.eventDelegatedOnly {
		ui.Row(c).Gap(6).Wrap().AlignItems(ui.Center).Children(func() {
			if a.eventPIDFilter > 0 {
				statusPill(c, fmt.Sprintf("执行 PID %d", a.eventPIDFilter), t.Accent)
			}
			if a.eventRootPIDFilter > 0 {
				statusPill(c, fmt.Sprintf("根 Agent PID %d（含后代）", a.eventRootPIDFilter), t.Accent)
			}
			if a.eventTargetFilter != "" {
				statusPill(c, "精确目标："+a.eventTargetFilter, t.Accent)
			}
			if a.eventDomainTarget != "" {
				statusPill(c, "Agent 域名："+a.eventDomainAgent+" · "+a.eventDomainTarget, t.Accent)
			}
			if a.eventFileEditsOnly {
				statusPill(c, "仅文件修改", t.Accent)
			}
			if a.eventDelegatedOnly {
				statusPill(c, "仅委托编辑", t.Accent)
			}
			if ui.Button(c, "清除关联条件").Clicked() {
				a.eventPIDFilter = 0
				a.eventRootPIDFilter = 0
				a.eventTargetFilter = ""
				a.eventDomainAgent, a.eventDomainTarget, a.eventDomainKind = "", "", ""
				a.eventFileEditsOnly = false
				a.eventDelegatedOnly = false
				a.eventSelected = -1
				a.inspectorSelectedID = ""
			}
		})
	}

	owners := a.eventOwnershipIndex()
	rows := a.filteredEvents()
	// Keep the existing +50 pagination for deep inspection; this native
	// stepped slider offers a quick adjustment for daily monitoring.
	ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
		displayCount := float64(max(a.eventVisibleLimit, 50))
		ui.Textf(c, "列表展示上限 %d 条", a.eventVisibleLimit).FontSize(11).TextColor(t.TextMuted)
		if ui.StepSlider(c.Key("event-display-limit"), &displayCount, 50, eventDisplaySliderMax(a.eventVisibleLimit), 50).
			Label("事件展示条数").Width(220).Changed() {
			a.eventVisibleLimit = int(displayCount)
			if a.eventSelected >= a.eventVisibleLimit {
				a.eventSelected = -1
				a.inspectorSelectedID = ""
			}
		}
	})
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
			{Title: "归属 / 执行进程", Width: 195},
			{Title: "目标", MinWidth: 220},
			{Title: "风险", Width: 86},
			{Title: "分数", Width: 64, Align: ui.End},
		}
		a.eventTable.Key = func(row int) any { return visible[row].EventID }
		table := ui.Table(c, &a.eventTable, cols, len(visible), func(row, col int) {
			e := visible[row]
			switch col {
			case 0:
				ui.Text(c, eventTime(e)).Font("monospace").SingleLine()
			case 1:
				ui.Text(c, eventAction(e)).SingleLine()
			case 2:
				attribution := owners.attribution(e)
				if attribution.Indirect {
					ui.Column(c).Gap(2).MinWidth(0).Children(func() {
						harnessIdentity(c, attribution.OwnerLabel, attribution.OwnerTag, attribution.OwnerComm)
						ui.Text(c, attributionExecutorLabel(attribution)).FontSize(10).TextColor(t.TextMuted).SingleLine()
					})
				} else {
					harnessIdentity(c, displayOr(e.Comm, "-"), e.Tag, e.Comm)
				}
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
		if table.Changed() && a.eventSelected >= 0 && a.eventSelected < len(visible) {
			a.inspectorSelectedID = visible[a.eventSelected].EventID
		}
		if table.Submitted() && a.eventSelected >= 0 && a.eventSelected < len(visible) {
			a.openEventDetail(visible[a.eventSelected].EventID)
		}
		if a.eventSelected >= 0 && a.eventSelected < len(visible) {
			selected := visible[a.eventSelected]
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Text(c, selected.EventID).Font("monospace").FontSize(10).TextColor(t.TextMuted).Grow(1)
				if ui.PrimaryButton(c, "详细").Clicked() {
					a.openEventDetail(selected.EventID)
				}
				if ui.Button(c, "固定到研判栏").Clicked() {
					a.inspectorPinnedID = selected.EventID
					a.inspectorOpen = true
					a.inspectorTab = 0
					width, _ := c.Size()
					if width < 1320 {
						a.page = "研判"
					}
				}
			})
			a.eventQuickLinks(c, selected, false)
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
			if ui.Button(c, label).Disabled(a.historyLoading).Clicked() && !a.historyLoading {
				a.loadOlderEvents()
			}
			if a.historyLoading {
				renewLoadingIndicator(c, "正在读取更早的事件记录")
			}
		}
	})
	if strings.TrimSpace(a.historyCursor) == "" && a.historyInitialized {
		ui.Text(c, "已到当前后端历史窗口的末尾。").FontSize(10).TextColor(t.TextMuted)
	}
}

func (a *renewApp) eventRow(c *ui.Context, e eventSummary) {
	t := c.Theme()
	owner := a.eventOwnershipIndex().attribution(e)
	ui.Row(c).Padding(9, 0).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, eventTime(e)).Width(66).Font("monospace").FontSize(11).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, eventAction(e)).Bold()
				if owner.Indirect {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						if label := harnessLabelFor(owner.OwnerTag, owner.OwnerComm); label != "未识别" {
							drawHarnessIcon(c, label)
						}
						ui.Badge(c, owner.OwnerLabel)
						ui.Text(c, attributionExecutorLabel(owner)).FontSize(10).TextColor(t.TextMuted)
					})
				} else if e.Comm != "" {
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						if label := eventHarnessLabel(e); label != "未识别" {
							drawHarnessIcon(c, label)
						}
						ui.Badge(c, e.Comm)
					})
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
		if e.EventID != "" && ui.Button(c, "定位").Tooltip("在事件表中选中这条摘要").Clicked() {
			a.focusSummary(e.EventID)
		}
	})
}
