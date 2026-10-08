package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// One shared palette per window: avoid allocating a new Theme every frame.
// Inspired by the reference image's low-contrast navy editor chrome.
var renewDesktopTheme = func() *ui.Theme {
	t := ui.DarkTheme()
	t.Background = ui.Hex("#0d111b")
	t.Surface = ui.Hex("#151b27")
	t.SurfaceHover = ui.Hex("#202a3b")
	t.SurfacePressed = ui.Hex("#29364d")
	t.Border = ui.Hex("#2b3546")
	t.Text = ui.Hex("#e8edf5")
	t.TextMuted = ui.Hex("#94a1b5")
	t.Accent = ui.Hex("#839bf5")
	t.AccentHover = ui.Hex("#9bafff")
	t.AccentPressed = ui.Hex("#6684e0")
	t.AccentText = ui.Hex("#111827")
	t.Warning = ui.Hex("#eebc66")
	t.Danger = ui.Hex("#ff7a8f")
	t.Success = ui.Hex("#64cfa9")
	t.Selection = ui.RGBA(131, 155, 245, 0.28)
	t.Focus = ui.RGBA(131, 155, 245, 0.55)
	t.Radius = 8
	t.Spacing = 4
	t.FontSize = 13
	return t
}()

// The right rail is helpful on large displays, but must never squeeze event
// tables and configuration forms on a laptop-sized window.
func showWorkspaceInspector(width float32, expanded bool, page string) bool {
	return expanded && width >= 1320 && (page == "概览" || page == "事件")
}

func (a *renewApp) workspaceView(c *ui.Context) {
	c.SetTheme(renewDesktopTheme)
	t := c.Theme()
	width, _ := c.Size()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Background(t.Background).Children(func() {
		a.activityRail(c)
		a.sidebar(c)
		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			a.header(c)
			ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Scroll(c).Grow(1).Padding(20).Gap(16).Children(func() {
						switch {
						case a.starting:
							a.startingView(c)
						case a.lastErr != "" && len(a.events) == 0:
							a.errorView(c)
						default:
							a.workspacePage(c)
						}
					})
				})
				if showWorkspaceInspector(width, a.inspectorOpen, a.page) {
					a.inspector(c)
				}
			})
			// Keep the existing explicit detail API, modal and payload release.
			wasOpen := a.eventDetailOpen
			a.eventDetailModal(c)
			if wasOpen && !a.eventDetailOpen {
				a.releaseEventDetailPayload()
			}
		})
	})
}

func (a *renewApp) workspacePage(c *ui.Context) {
	switch a.page {
	case "事件":
		a.eventsView(c)
	case "会话":
		a.sessionsView(c)
	case "网络":
		a.networkView(c)
	case "进程":
		a.processesView(c)
	case "监控":
		a.monitoringView(c)
	case "eBPF 模块":
		a.ebpfModulesView(c)
	case "规则":
		a.rulesView(c)
	case "跟踪":
		a.trackingView(c)
	case "路径权限":
		a.pathAccessView(c)
	case "系统":
		a.systemView(c)
	default:
		a.overview(c)
	}
}

// Activity-bar shortcuts are a quick route into real monitoring pages.
// The full navigation and privileged configuration gates remain unchanged.
func (a *renewApp) activityRail(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(54).Shrink(0).Background(t.Background).Gap(8).Padding(9, 6).Children(func() {
		ui.Box(c).Size(40, 40).Radius(12).Background(t.Accent.Alpha(0.18)).Center().Children(func() {
			ui.Text(c, "镜").Bold().FontSize(18).TextColor(t.Accent)
		})
		ui.Divider(c)
		for _, item := range []struct{ label, glyph string }{
			{"概览", "⌂"},
			{"事件", "☷"},
			{"会话", "◎"},
			{"网络", "⇄"},
			{"进程", "▣"},
			{"监控", "◈"},
			{"规则", "◇"},
			{"系统", "⚙"},
		} {
			marker := item.glyph
			if a.page == item.label {
				marker = "●"
			}
			if ui.Button(c, marker).Tooltip(item.label).Width(40).Clicked() {
				a.page = item.label
			}
		}
		ui.Spacer(c)
		if ui.Button(c, "☰").Tooltip("跟踪范围").Width(40).Clicked() {
			a.page = "跟踪"
		}
	})
}

func (a *renewApp) sidebar(c *ui.Context) {
	t := c.Theme()
	_, attention, danger := a.riskCounts()
	ui.Column(c).Width(198).Shrink(0).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).Padding(16, 14, 13, 14).Gap(4).Children(func() {
			ui.Text(c, desktopBrandName).FontSize(17).Bold()
			ui.Text(c, "AGENT eBPF  /  WORKSPACE").Font("monospace").FontSize(10).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Sidebar(c, &a.page, func() {
			ui.SidebarSection(c, "监控工作台", nil, func() {
				ui.SidebarItem(c, "概览", nil, "态势总览")
				events := ui.SidebarItem(c, "事件", nil, "事件流")
				switch {
				case danger > 0:
					events.Children(func() { statusPill(c, fmt.Sprint(danger), t.Danger) })
				case attention > 0:
					events.Children(func() { statusPill(c, fmt.Sprint(attention), t.Warning) })
				}
				ui.SidebarItem(c, "会话", nil, "Agent 会话")
				ui.SidebarItem(c, "网络", nil, "网络外联")
				ui.SidebarItem(c, "进程", nil, "进程活动")
			})
			ui.SidebarSection(c, "防护与管理", nil, func() {
				ui.SidebarItem(c, "监控", nil, "采集设置")
				ui.SidebarItem(c, "eBPF 模块", nil, "内核模块")
				ui.SidebarItem(c, "规则", nil, "Wrapper 规则")
				ui.SidebarItem(c, "跟踪", nil, "跟踪范围")
				ui.SidebarItem(c, "路径权限", nil, "文件访问保护")
			})
			ui.SidebarSection(c, "运行诊断", nil, func() {
				system := ui.SidebarItem(c, "系统", nil, "系统诊断")
				_, level := a.collectorStatus()
				if level == "danger" {
					system.Children(func() { statusPill(c, "异常", t.Danger) })
				} else if level == "warning" {
					system.Children(func() { statusPill(c, "同步", t.Warning) })
				}
			})
		}).Grow(1).Width(196)
		ui.Divider(c)
		ui.Column(c).Padding(12, 14).Gap(8).Children(func() {
			label, level := a.pipelineStatus()
			tone := workspaceStatusTone(t, level)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				statusPill(c, label, tone)
				if a.paused {
					statusPill(c, "界面暂停", t.Warning)
				}
			})
			ui.Text(c, a.backend).Font("monospace").FontSize(10).TextColor(t.TextMuted).MaxLines(2)
		})
	})
}

func workspaceStatusTone(t *ui.Theme, level string) ui.Color {
	switch level {
	case "danger":
		return t.Danger
	case "warning":
		return t.Warning
	default:
		return t.Success
	}
}

func (a *renewApp) header(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Background(t.Surface).Children(func() {
		ui.Row(c).MinHeight(61).Padding(10, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(3).MinWidth(132).Grow(1).Children(func() {
				ui.Text(c, a.page).FontSize(18).Bold()
				ui.Text(c, pageSubtitle(a.page)).FontSize(10).SingleLine().TextColor(t.TextMuted)
			})
			if pageUsesEventSearch(a.page) {
				ui.SearchField(c, &a.search).Label("搜索事件、进程或目标").Width(215)
			}
			label, level := a.pipelineStatus()
			statusPill(c, label, workspaceStatusTone(t, level))
			if ui.Button(c, map[bool]string{true: "继续", false: "暂停"}[a.paused]).
				Tooltip("暂停或恢复桌面事件合并").Clicked() {
				a.paused = !a.paused
				a.eventUIPaused.Store(a.paused)
				if !a.paused {
					go a.refresh(context.Background())
				}
			}
			if ui.Button(c, "刷新").Tooltip("同步最新状态").Clicked() {
				go a.refresh(context.Background())
				if a.page == "监控" || a.page == "规则" || a.page == "跟踪" {
					go a.refreshConfiguration(context.Background())
				}
			}
		})
		ui.Divider(c)
		ui.Row(c).MinHeight(38).Padding(6, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "工作区").FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "›").TextColor(t.TextMuted)
			ui.Badge(c, a.page).Background(t.Accent.Alpha(0.16)).TextColor(t.Accent)
			ui.Spacer(c)
			if a.page == "概览" || a.page == "事件" {
				_, _ = c.Size()
				label := "打开研判栏"
				if a.inspectorOpen {
					label = "收起研判栏"
				}
				if ui.Button(c, label).Tooltip("宽窗口显示右侧事件上下文").Clicked() {
					a.inspectorOpen = !a.inspectorOpen
				}
			}
		})
	})
}

// inspectorEvent prefers the selected event on the Events page. It never
// requests full payloads, so simply opening the inspector costs no API calls.
func (a *renewApp) inspectorEvent() (eventSummary, bool) {
	rows := a.events
	if a.page == "事件" || a.page == "概览" {
		rows = a.filteredEvents()
	}
	if len(rows) == 0 {
		return eventSummary{}, false
	}
	if a.page == "事件" && a.eventSelected >= 0 && a.eventSelected < len(rows) &&
		a.eventSelected < a.eventVisibleLimit {
		return rows[a.eventSelected], true
	}
	for _, event := range rows {
		if eventRisk(event) != "正常" {
			return event, true
		}
	}
	return rows[0], true
}

func (a *renewApp) focusSummary(id string) {
	a.page = "事件"
	a.search = ""
	a.eventTypeFilter = ""
	a.eventSessionFilter = ""
	a.eventDecisionFilter = ""
	a.eventAttentionOnly = false
	a.eventSelected = -1
	for i, event := range a.events {
		if event.EventID == id {
			a.eventSelected = i
			if a.eventVisibleLimit <= i {
				a.eventVisibleLimit = i + 1
			}
			break
		}
	}
}

func (a *renewApp) inspector(c *ui.Context) {
	t := c.Theme()
	_, attention, danger := a.riskCounts()
	ui.Column(c).Width(302).Shrink(0).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Row(c).Padding(15, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, "事件研判").FontSize(14).Bold()
				ui.Text(c, "基于已采集摘要 · 非 AI 推断").FontSize(10).TextColor(t.TextMuted)
			})
			if ui.Button(c, "×").Tooltip("关闭侧栏").Clicked() {
				a.inspectorOpen = false
			}
		})
		ui.Divider(c)
		ui.Scroll(c).Grow(1).Padding(14).Gap(13).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Column(c).Grow(1).Padding(12).Gap(5).Radius(9).Background(t.Background).Children(func() {
					ui.Text(c, "需关注").FontSize(11).TextColor(t.TextMuted)
					ui.Text(c, fmt.Sprint(attention)).FontSize(22).Bold().TextColor(t.Warning)
				})
				ui.Column(c).Grow(1).Padding(12).Gap(5).Radius(9).Background(t.Background).Children(func() {
					ui.Text(c, "高风险").FontSize(11).TextColor(t.TextMuted)
					ui.Text(c, fmt.Sprint(danger)).FontSize(22).Bold().TextColor(t.Danger)
				})
			})
			ui.Text(c, "统计范围：当前桌面事件摘要窗口").FontSize(10).TextColor(t.TextMuted)
			ui.Divider(c)
			ui.Text(c, "当前事件").Bold().FontSize(12)
			selected, ok := a.inspectorEvent()
			if !ok {
				ui.Text(c, "暂无与当前搜索匹配的事件。").TextColor(t.TextMuted).FontSize(12)
			} else {
				ui.Column(c).Padding(12).Gap(9).Radius(10).Background(t.Background).Border(1, t.Border).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
						riskPill(c, eventRisk(selected))
						ui.Text(c, eventTime(selected)).FontSize(10).Font("monospace").TextColor(t.TextMuted)
					})
					ui.Text(c, eventAction(selected)).FontSize(14).Bold()
					ui.Text(c, displayOr(selected.Comm, "未知进程")+"  ·  PID "+fmt.Sprint(selected.PID)).FontSize(11).TextColor(t.TextMuted)
					target := strings.TrimSpace(eventTarget(selected))
					if target != "" {
						ui.Text(c, target).Font("monospace").FontSize(11).MaxLines(3)
					}
					if selected.Decision != "" {
						ui.Text(c, "决策  "+selected.Decision).FontSize(11).TextColor(t.TextMuted)
					}
					if selected.EventID != "" {
						if ui.PrimaryButton(c, "按需加载完整详情").Clicked() {
							a.openEventDetail(selected.EventID)
						}
						if ui.Button(c, "定位到事件表").Clicked() {
							a.focusSummary(selected.EventID)
						}
					}
				})
			}
			ui.Divider(c)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "运行状态").FontSize(12).Bold().Grow(1)
				collector, level := a.collectorStatus()
				statusPill(c, collector, workspaceStatusTone(t, level))
			})
			ui.Column(c).Padding(12).Gap(8).Radius(10).Background(t.Background).Children(func() {
				label, level := a.pipelineStatus()
				ui.Text(c, label).TextColor(workspaceStatusTone(t, level)).Bold().FontSize(12)
				if a.lastSync.IsZero() {
					ui.Text(c, "等待后端首次同步").FontSize(11).TextColor(t.TextMuted)
				} else {
					ui.Text(c, "最近同步  "+a.lastSync.Format("15:04:05")).FontSize(11).TextColor(t.TextMuted)
				}
				ui.Text(c, fmt.Sprintf("Ringbuf 丢弃  %d", a.health.RingbufDroppedTotal)).Font("monospace").FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, fmt.Sprintf("后端队列  %d", a.health.BackendQueueLen)).Font("monospace").FontSize(11).TextColor(t.TextMuted)
			})
			ui.Text(c, "提醒：未发现高风险不代表所有行为都已被采集。").FontSize(11).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		ui.Row(c).Padding(12).Gap(8).Children(func() {
			if ui.Button(c, "所有事件").Clicked() { a.page = "事件" }
			if ui.Button(c, "采集设置").Clicked() { a.page = "监控" }
		})
	})
}
