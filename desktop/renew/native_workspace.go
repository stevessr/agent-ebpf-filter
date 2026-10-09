package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/egoist/mygo"

	"github.com/egoist/mygo/ui"
)

// Keep platform materials opt-in to the supported native compositors. On
// Wayland and X11 we always paint an opaque workspace, including when the
// user has a desktop transparency setting enabled elsewhere.
func windowMaterial(goos string, enabled bool) mygo.Vibrancy {
	if !enabled {
		return mygo.VibrancyNone
	}
	switch goos {
	case "darwin":
		return mygo.VibrancySidebar
	case "windows":
		return mygo.VibrancyMica
	default:
		return mygo.VibrancyNone
	}
}

// The right rail is helpful on large displays, but must never squeeze event
// tables and configuration forms on a laptop-sized window.
func pageHasInspector(page string) bool {
	switch page {
	case "概览", "事件", "会话", "网络", "进程", "系统":
		return true
	default:
		return false
	}
}

// Minimum usable center canvas when the 302-DIP investigator is visible.
// Evaluate the current animated sidebar width, not only its target state,
// to prevent the inspector from overlapping tables during the transition.
func showWorkspaceInspector(windowWidth, navigationWidth float32, expanded bool, page string) bool {
	const activityRailWidth = 54
	const inspectorWidth = 302
	const minimumCenterWidth = 760
	return expanded && pageHasInspector(page) &&
		windowWidth-activityRailWidth-navigationWidth-inspectorWidth >= minimumCenterWidth
}


const (
	workspaceNavigationCompactWidth  float32 = 198
	workspaceNavigationDetailedWidth float32 = 302
)

// Page IDs remain stable; only the optional sidebar's visible names change.
var workspaceNavigationLabels = map[string][2]string{
	"概览":       {"态势总览", "系统态势与运行概览"},
	"研判":       {"风险研判工作台", "风险事件研判工作台"},
	"事件":       {"事件流", "eBPF 实时事件流"},
	"会话":       {"Agent 会话", "Agent 会话与行为关联"},
	"网络":       {"网络外联", "网络连接与外联目标"},
	"进程":       {"进程活动", "进程活动与资源监测"},
	"Agent 识别": {"捕获与监视范围", "Agent 识别与捕获监视范围"},
	"监控":       {"采集设置", "实时采集与监控设置"},
	"eBPF 模块":  {"内核模块", "eBPF 内核模块管理"},
	"规则":       {"Wrapper 规则", "Wrapper 拦截与防护规则"},
	"跟踪":       {"跟踪范围", "进程、命令与路径跟踪"},
	"路径权限":   {"文件访问保护", "文件路径与访问权限"},
	"终端":       {"本地 Shell · 多标签与分屏", "本地 Shell 终端与多窗格"},
	"系统":       {"系统诊断", "采集链路与系统运行诊断"},
}

func workspaceNavigationWidth(open, detailed bool) float32 {
	if !open {
		return 0
	}
	if detailed {
		return workspaceNavigationDetailedWidth
	}
	return workspaceNavigationCompactWidth
}

func workspaceNavigationLabel(page string, detailed bool) string {
	labels, ok := workspaceNavigationLabels[page]
	if !ok {
		return page
	}
	if detailed {
		return labels[1]
	}
	return labels[0]
}

func (a *renewApp) workspaceView(c *ui.Context) {
	// MyGo's frame theme already tracks light/dark system appearance and changes.
	// Copy its current colors before applying the Renew palette so that OS
	// accents, high contrast, font scaling, and native theme switches survive.
	styleRenewWorkspaceTheme(&a.workspaceTheme, c.Theme(), c.Preferences().HighContrast)
	c.SetTheme(&a.workspaceTheme)
	t := c.Theme()
	width, _ := c.Size()
	vibrant := c.Vibrancy()
	if vibrant {
		c.Root().Background(ui.Transparent)
	}
	shell := ui.Row(c).Fill().AlignItems(ui.Stretch)
	if !vibrant {
		shell.Background(t.Background)
	}
	// MyGo 0.3 animations honor the OS reduced-motion setting.
	// Animate actual widths, not a percentage, so resizing and switching
	// label density use the same inspector layout calculation.
	navTarget := workspaceNavigationWidth(a.navigationOpen, a.navigationDetailed)
	navWidth := shell.Animate("renew-navigation", navTarget, 180*time.Millisecond)
	shell.Children(func() {
		a.activityRail(c)
		if navWidth > 0 {
			ui.Row(c).Width(navWidth).Shrink(0).AlignItems(ui.Stretch).Clip().Children(func() {
				a.sidebar(c)
			})
		}
		ui.Column(c).Key("workspace-content").Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			a.header(c, navWidth)
			if a.page == "终端" {
				// Real PTY bounds without a parent Scroll; also usable offline.
				a.terminalView(c)
			} else {
				ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Scroll(c).Grow(1).Padding(20).Gap(16).Children(func() {
						switch {
						case a.starting:
							a.startingView(c)
						case a.lastErr != "" && len(a.events) == 0:
							a.errorView(c)
						default:
							ui.Column(c).Key("page-"+a.page).Gap(16).Transition(ui.ElementTransition{
							Duration: 160 * time.Millisecond,
							Enter: &ui.Motion{Y: 8},
						}).Children(func() { a.workspacePage(c) })
						}
					})
				})
				if showWorkspaceInspector(width, navWidth, a.inspectorOpen, a.page) {
					a.inspector(c)
				}
				})
			}
			a.workspaceFooter(c)
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
	case "研判":
		a.inspectorStandalone(c)
	case "会话":
		a.sessionsView(c)
	case "网络":
		a.networkView(c)
	case "进程":
		a.processesView(c)
	case "Agent 识别":
		a.agentRecognitionView(c)
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
	rail := ui.Column(c).Width(54).Shrink(0).Gap(8).Padding(9, 6)
	if !c.Vibrancy() {
		rail.Background(t.Background)
	}
	rail.Children(func() {
		ui.Box(c).Size(40, 40).Radius(12).Background(t.Accent.Alpha(0.18)).Center().Children(func() {
			ui.Icon(c, renewMirrorSVG).Size(27, 27).TextColor(t.Accent).Label(desktopBrandName)
		})
		ui.Divider(c)
		for _, item := range []struct{ label, glyph string }{
			{"概览", "⌂"},
			{"事件", "☷"},
			{"研判", "◇"},
			{"会话", "◎"},
			{"网络", "⇄"},
			{"进程", "▣"},
			{"Agent 识别", "◉"},
			{"监控", "◈"},
			{"规则", "▤"},
			{"系统", "⚙"},
			{"终端", "⌘"},
		} {
			marker := item.glyph
			if a.page == item.label {
				marker = "●"
			}
			if ui.Button(c, marker).Tooltip(workspaceNavigationLabel(item.label, true)).Width(40).Clicked() {
				a.page = item.label
			}
		}
		ui.Spacer(c)
		navLabel := "≡"
		navTip := "收起导航"
		if !a.navigationOpen {
			navLabel, navTip = "»", "展开导航"
		}
		if ui.Button(c, navLabel).Tooltip(navTip).Width(40).Clicked() {
			a.navigationOpen = !a.navigationOpen
		}
	})
}

func (a *renewApp) sidebar(c *ui.Context) {
	t := c.Theme()
	_, attention, danger := a.riskCounts()
	width := workspaceNavigationWidth(true, a.navigationDetailed)
	label := func(page string) string { return workspaceNavigationLabel(page, a.navigationDetailed) }
	side := ui.Column(c).Width(width).Shrink(0).Border(1, t.Border)
	if !c.Vibrancy() {
		side.Background(t.Surface)
	}
	side.Children(func() {
		ui.Row(c).Padding(9, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "功能导航").FontSize(12).Bold().Grow(1)
			switchLabel, tip := "详细 ›", "展开侧边栏，显示完整的功能名称"
			if a.navigationDetailed {
				switchLabel, tip = "精简 ‹", "收窄侧边栏，显示精简的功能名称"
			}
			if ui.Button(c, switchLabel).Tooltip(tip).Clicked() {
				a.navigationDetailed = !a.navigationDetailed
			}
		})
		ui.Divider(c)
		menu := ui.Sidebar(c, &a.page, func() {
			ui.SidebarSection(c, "监控工作台", nil, func() {
				ui.SidebarItem(c, "概览", nil, label("概览"))
				ui.SidebarItem(c, "研判", nil, label("研判"))
				events := ui.SidebarItem(c, "事件", nil, label("事件"))
				switch {
				case danger > 0:
					events.Children(func() { statusPill(c, fmt.Sprint(danger), t.Danger) })
				case attention > 0:
					events.Children(func() { statusPill(c, fmt.Sprint(attention), t.Warning) })
				}
				ui.SidebarItem(c, "会话", nil, label("会话"))
				ui.SidebarItem(c, "网络", nil, label("网络"))
				ui.SidebarItem(c, "进程", nil, label("进程"))
				ui.SidebarItem(c, "Agent 识别", nil, label("Agent 识别"))
			})
			ui.SidebarSection(c, "防护与管理", nil, func() {
				ui.SidebarItem(c, "监控", nil, label("监控"))
				ui.SidebarItem(c, "eBPF 模块", nil, label("eBPF 模块"))
				ui.SidebarItem(c, "规则", nil, label("规则"))
				ui.SidebarItem(c, "跟踪", nil, label("跟踪"))
				ui.SidebarItem(c, "路径权限", nil, label("路径权限"))
			})
			ui.SidebarSection(c, "工具", nil, func() {
				ui.SidebarItem(c, "终端", nil, label("终端"))
			})
			ui.SidebarSection(c, "运行诊断", nil, func() {
				system := ui.SidebarItem(c, "系统", nil, label("系统"))
				_, level := a.collectorStatus()
				if level == "danger" {
					system.Children(func() { statusPill(c, "异常", t.Danger) })
				} else if level == "warning" {
					system.Children(func() { statusPill(c, "同步", t.Warning) })
				}
			})
		}).Grow(1).Width(width - 2)
		if c.Vibrancy() {
			menu.Background(ui.Transparent)
		}

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

func (a *renewApp) header(c *ui.Context, navigationWidth float32) {
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
			// The native toolbar moves actions to its overflow menu on narrow
			// windows, retaining keyboard navigation and accessibility labels.
			ui.Toolbar(c, func() {
				if ui.Button(c, map[bool]string{true: "继续", false: "暂停"}[a.paused]).
					Tooltip("暂停或恢复桌面事件合并").Clicked() {
					a.paused = !a.paused
					a.eventUIPaused.Store(a.paused)
					if !a.paused {
						go a.refresh(context.Background())
					}
				}
				if ui.Button(c, "刷新").Tooltip("重新读取当前页面的数据").Clicked() {
					a.refreshActiveView()
				}
				if windowMaterial(runtime.GOOS, true) != mygo.VibrancyNone {
					caption := "纯色模式"
					if !a.materialEnabled {
						caption = "系统材质"
					}
					if ui.Button(c, caption).Tooltip("切换系统材质与不透明背景").Clicked() {
						a.materialEnabled = !a.materialEnabled
						if a.win != nil {
							a.win.SetVibrancy(windowMaterial(runtime.GOOS, a.materialEnabled))
						}
					}
				}
			}).MinWidth(0).Label("监控操作")
		})
		ui.Divider(c)
		ui.Row(c).MinHeight(38).Padding(6, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "工作区").FontSize(11).TextColor(t.TextMuted)
			ui.Text(c, "›").TextColor(t.TextMuted)
			ui.Badge(c, a.page).Background(t.Accent.Alpha(0.16)).TextColor(t.Accent)
			if a.page == "概览" || a.page == "事件" || a.page == "网络" || a.page == "进程" {
				if a.eventPIDFilter > 0 {
					statusPill(c, fmt.Sprintf("PID %d", a.eventPIDFilter), t.Accent)
				}
				if a.eventRiskFilter != "" {
					statusPill(c, a.eventRiskFilter, workspaceStatusTone(t, map[string]string{"高风险": "danger", "需关注": "warning"}[a.eventRiskFilter]))
				}
				if a.hasEventConstraints() {
					if ui.Button(c, "重置事件筛选").Tooltip("清除 PID、类型、会话、决策与风险约束").Clicked() {
						a.clearEventFilters()
					}
				}
			}
			ui.Spacer(c)
			if pageHasInspector(a.page) {
				width, _ := c.Size()
				if showWorkspaceInspector(width, navigationWidth, true, a.page) {
					label := "打开研判栏"
					if a.inspectorOpen {
						label = "收起研判栏"
					}
					if ui.Button(c, label).Tooltip("宽窗口显示右侧事件上下文").Clicked() {
						a.inspectorOpen = !a.inspectorOpen
					}
				} else if ui.Button(c, "打开研判页").Tooltip("在当前宽度以独立页面浏览研判信息").Clicked() {
					a.page = "研判"
				}
			}
		})
	})
}


func (a *renewApp) refreshActiveView() {
	// Keep the continuously streamed metrics subscription intact: opening a
	// second system WebSocket here would duplicate the collector workload.
	go a.refresh(context.Background())
	switch a.page {
	case "Agent 识别":
		go a.refreshAgentScopes(context.Background())
		go a.refreshRegistry(context.Background())
	case "监控":
		go a.refreshConfiguration(context.Background())
	case "规则":
		go a.refreshRules(context.Background())
	case "跟踪":
		go a.refreshRegistry(context.Background())
	case "eBPF 模块":
		go a.refreshEBPFModules(context.Background())
	case "路径权限":
		go a.refreshPathAccess()
		go a.refreshConfiguration(context.Background())
	}
}

func (a *renewApp) hasEventConstraints() bool {
	return a.eventPIDFilter > 0 || a.eventRiskFilter != "" ||
		a.eventTypeFilter != "" || a.eventSessionFilter != "" ||
		a.eventDecisionFilter != "" || a.eventAttentionOnly
}

func (a *renewApp) workspaceFooter(c *ui.Context) {
	t := c.Theme()
	_, attention, danger := a.riskCounts()
	ui.Row(c).MinHeight(29).Padding(5, 12).Gap(12).AlignItems(ui.Center).Background(t.Surface).Children(func() {
		switch {
		case a.starting:
			statusPill(c, "启动中", t.Warning)
		case !a.connected:
			statusPill(c, "后端离线", t.Danger)
		case a.paused:
			statusPill(c, "界面已暂停", t.Warning)
		case a.eventStreamConnected:
			statusPill(c, "事件流实时", t.Success)
		default:
			statusPill(c, "事件流回退", t.Warning)
		}
		ui.Text(c, fmt.Sprintf("摘要 %d", len(a.events))).FontSize(10).Font("monospace").TextColor(t.TextMuted)
		if danger > 0 {
			if ui.Button(c, fmt.Sprintf("高风险 %d", danger)).Tooltip("查看高风险事件").Clicked() {
				a.clearEventFilters()
				a.eventRiskFilter = "高风险"
				a.page = "事件"
			}
		} else if attention > 0 {
			if ui.Button(c, fmt.Sprintf("需关注 %d", attention)).Tooltip("查看需要关注的事件").Clicked() {
				a.clearEventFilters()
				a.eventRiskFilter = "需关注"
				a.page = "事件"
			}
		}
		ui.Spacer(c)
		if !a.lastSync.IsZero() {
			ui.Text(c, "同步 "+a.lastSync.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted)
		}
		if ui.Button(c, fmt.Sprintf("丢弃 %d", a.health.RingbufDroppedTotal)).Tooltip("打开采集链路诊断").Clicked() {
			a.page = "系统"
		}
	})
}
