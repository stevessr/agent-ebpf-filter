package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/egoist/mygo"

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

func showWorkspaceInspector(width float32, expanded bool, page string) bool {
	return expanded && width >= 1320 && pageHasInspector(page)
}

func (a *renewApp) workspaceView(c *ui.Context) {
	c.SetTheme(renewDesktopTheme)
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
	navTarget := float32(0)
	if a.navigationOpen {
		navTarget = 1
	}
	navProgress := shell.Animate("renew-navigation", navTarget, 180*time.Millisecond)
	shell.Children(func() {
		a.activityRail(c)
		if navProgress > 0 {
			ui.Row(c).Width(198 * navProgress).Shrink(0).Clip().Children(func() {
				a.sidebar(c)
			})
		}
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
							ui.Column(c).Key("page-"+a.page).Gap(16).Transition(ui.ElementTransition{
							Duration: 160 * time.Millisecond,
							Enter: &ui.Motion{Y: 8},
						}).Children(func() { a.workspacePage(c) })
						}
					})
				})
				inspectorWidth := width
				if !a.navigationOpen {
					inspectorWidth += 198
				}
				if showWorkspaceInspector(inspectorWidth, a.inspectorOpen, a.page) {
					a.inspector(c)
				}
			})
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
			ui.Text(c, "镜").Bold().FontSize(18).TextColor(t.Accent)
		})
		ui.Divider(c)
		for _, item := range []struct{ label, glyph string }{
			{"概览", "⌂"},
			{"事件", "☷"},
			{"研判", "◇"},
			{"会话", "◎"},
			{"网络", "⇄"},
			{"进程", "▣"},
			{"监控", "◈"},
			{"规则", "▤"},
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
	side := ui.Column(c).Width(198).Shrink(0).Border(1, t.Border)
	if !c.Vibrancy() {
		side.Background(t.Surface)
	}
	side.Children(func() {
		ui.Column(c).Padding(16, 14, 13, 14).Gap(4).Children(func() {
			ui.Text(c, desktopBrandName).FontSize(17).Bold()
			ui.Text(c, "AGENT eBPF  /  WORKSPACE").Font("monospace").FontSize(10).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		menu := ui.Sidebar(c, &a.page, func() {
			ui.SidebarSection(c, "监控工作台", nil, func() {
				ui.SidebarItem(c, "概览", nil, "态势总览")
				ui.SidebarItem(c, "研判", nil, "风险研判工作台")
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
		if c.Vibrancy() {
			menu.Background(ui.Transparent)
		}
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
			if c.Vibrancy() {
				ui.Text(c, "原生系统材质").FontSize(10).TextColor(t.TextMuted)
			}
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
				availableWidth := width
				if !a.navigationOpen {
					availableWidth += 198
				}
				if availableWidth >= 1320 {
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
