package main

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Workspace routes in the narrow-screen native menu are the same stable
// page IDs the ordinary sidebar uses. Both include privileged settings
// rather than silently hiding them on smaller displays.
var workspaceNavigationMenuSections = []struct {
	title string
	pages []string
}{
	{"监控工作台", []string{"概览", "行为", "研判", "事件", "会话", "AgentSight", "派生", "网络", "域名", "进程", "Agent 识别"}},
	{"防护与管理", []string{"监控", "eBPF 模块", "规则", "跟踪", "路径权限"}},
	{"工具", []string{"终端", "文件", "编辑器", "CCS"}},
	{"运行诊断", []string{"系统"}},
}

func (a *renewApp) compactNavigationMenu(m *ui.Menu) {
	for _, section := range workspaceNavigationMenuSections {
		section := section
		m.Submenu(section.title, func(items *ui.Menu) {
			for _, page := range section.pages {
				if items.Item(workspaceNavigationLabel(page, false)).Checked(a.page == page).Chosen() {
					a.page = page
				}
			}
		})
	}
}

// Activity-bar shortcuts are a quick route into real monitoring pages.
// The full navigation and privileged configuration gates remain unchanged.
func (a *renewApp) activityRail(c *ui.Context, windowWidth float32) {
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
		for _, page := range workspaceRailPages {
			active := a.page == page
			name := workspaceNavigationLabel(page, true)
			button := ui.ButtonBase(c.Key("rail-" + page)).
				Width(40).Height(40).Padding(0).Radius(11).
				AlignItems(ui.Center).Justify(ui.Center).
				Label(name).Tooltip(name).
				Transition(renewRailColorTransition())
			switch {
			case active:
				button.Background(t.Accent.Alpha(0.17)).TextColor(t.Accent)
				if c.Preferences().HighContrast {
					button.Border(2, t.Accent)
				} else {
					button.Border(1, t.Accent.Alpha(0.28))
				}
			case button.Pressed():
				button.Background(t.SurfacePressed).TextColor(t.Text)
			case button.Hovered():
				button.Background(t.SurfaceHover).TextColor(t.Text)
			default:
				button.TextColor(t.TextMuted)
			}
			button.Children(func() {
				ui.Icon(c, workspaceNavigationIcon(page)).Size(20, 20)
			})
			if button.Clicked() {
				a.page = page
			}
		}
		ui.Spacer(c)
		if workspaceUsesNavigationMenu(windowWidth) {
			// A native menu is keyboard/screen-reader accessible and avoids
			// laying a sidebar on top of full-width event tables in niri.
			ui.Button(c, "≡").Label("打开全部页面导航").Tooltip("打开页面导航（窄窗口模式）").
				Width(40).Menu(a.compactNavigationMenu)
		} else {
			navLabel := "≡"
			navTip := "收起导航"
			if !a.navigationOpen {
				navLabel, navTip = "»", "展开导航"
			}
			if ui.Button(c, navLabel).Tooltip(navTip).Width(40).Clicked() {
				a.navigationOpen = !a.navigationOpen
			}
		}
	})
}

func (a *renewApp) sidebar(c *ui.Context, detailed bool) {
	t := c.Theme()
	_, attention, danger := a.riskCounts()
	width := workspaceNavigationWidth(true, detailed)
	label := func(page string) string { return workspaceNavigationLabel(page, detailed) }
	// Preserve MyGo's Sidebar tree semantics and keyboard arrow navigation.
	// A chosen row stays accent-highlighted even when focus moves to content.
	var selectedItem ui.Element
	navItem := func(page string) ui.Element {
		item := ui.SidebarItem(c, page, workspaceNavigationIcon(page), label(page))
		item.Transition(renewRailColorTransition())
		if a.page == page {
			selectedItem = item
		}
		return item
	}
	side := ui.Column(c).Width(width).Shrink(0).Border(1, t.Border)
	if !c.Vibrancy() {
		side.Background(t.Surface)
	}
	side.Children(func() {
		ui.Row(c).Padding(9, 10).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "功能导航").FontSize(12).Bold().Grow(1)
			switchLabel, tip := "详细 ›", "在宽窗口显示完整的功能名称"
			switch {
			case a.navigationDetailed && !detailed:
				switchLabel, tip = "自动精简", "当前窗口宽度不足，详细导航将在加宽后自动恢复；点击取消详细偏好"
			case a.navigationDetailed:
				switchLabel, tip = "精简 ‹", "收窄侧边栏，显示精简的功能名称"
			}
			if ui.Button(c, switchLabel).Tooltip(tip).Clicked() {
				a.navigationDetailed = !a.navigationDetailed
			}
		})
		ui.Divider(c)
		menu := ui.Sidebar(c, &a.page, func() {
			ui.SidebarSection(c, "监控工作台", nil, func() {
				navItem("概览")
				navItem("行为")
				navItem("研判")
				events := navItem("事件")
				switch {
				case danger > 0:
					events.Children(func() { statusPill(c, fmt.Sprint(danger), t.Danger) })
				case attention > 0:
					events.Children(func() { statusPill(c, fmt.Sprint(attention), t.Warning) })
				}
				navItem("会话")
				navItem("AgentSight")
				navItem("派生")
				navItem("网络")
				navItem("域名")
				navItem("进程")
				navItem("Agent 识别")
			})
			ui.SidebarSection(c, "防护与管理", nil, func() {
				navItem("监控")
				navItem("eBPF 模块")
				navItem("规则")
				navItem("跟踪")
				navItem("路径权限")
			})
			ui.SidebarSection(c, "工具", nil, func() {
				navItem("终端")
				navItem("文件")
				navItem("编辑器")
				navItem("CCS")
			})
			ui.SidebarSection(c, "运行诊断", nil, func() {
				system := navItem("系统")
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
		if selectedItem.Valid() && !menu.Focused() {
			selectedItem.Background(t.Accent.Alpha(0.15)).TextColor(t.Accent)
			if c.Preferences().HighContrast {
				selectedItem.Border(2, t.Accent)
			}
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
	width, _ := c.Size()
	compact := workspaceCompactHeader(width, navigationWidth)
	ui.Column(c).Background(t.Surface).Children(func() {
		ui.Row(c).MinHeight(61).Padding(10, 16).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(3).MinWidth(132).Grow(1).Children(func() {
				ui.Text(c, a.page).FontSize(18).Bold()
				ui.Text(c, pageSubtitle(a.page)).FontSize(10).SingleLine().TextColor(t.TextMuted)
			})
			if pageUsesEventSearch(a.page) && !compact {
				ui.SearchField(c, &a.search).Label("搜索事件、进程或目标").Width(215)
			}
			label, level := a.pipelineStatus()
			statusPill(c, label, workspaceStatusTone(t, level))
			if a.paused && !workspaceTightHeader(width, navigationWidth) {
				statusPill(c, "事件展示已暂停", t.Warning)
			}
			if !workspaceTightHeader(width, navigationWidth) {
				a.headerActions(c)
			}
		})
		if workspaceTightHeader(width, navigationWidth) {
			ui.Row(c).Padding(5, 12).Gap(8).AlignItems(ui.Center).Children(func() {
				if a.paused {
					statusPill(c, "展示暂停", t.Warning)
				}
				a.headerActions(c)
			})
		}
		if compact && pageUsesEventSearch(a.page) {
			ui.Row(c).Padding(5, 16).Children(func() {
				ui.SearchField(c, &a.search).Label("搜索事件、进程或目标").Grow(1).MinWidth(0)
			})
		}
		ui.Divider(c)
		ui.Row(c).MinHeight(38).Padding(6, 16).Gap(8).AlignItems(ui.Center).Children(func() {
			if !workspaceTightHeader(width, navigationWidth) {
				ui.Text(c, "工作区").FontSize(11).TextColor(t.TextMuted)
				ui.Text(c, "›").TextColor(t.TextMuted)
			}
			ui.Badge(c, a.page).Background(t.Accent.Alpha(0.16)).TextColor(t.Accent)
			if a.page == "概览" || a.page == "事件" || a.page == "网络" || a.page == "进程" {
				if n := a.eventConstraintCount(); n > 0 {
					statusPill(c, fmt.Sprintf("筛选 %d 项", n), t.Accent)
				}
				if !compact && a.eventPIDFilter > 0 {
					statusPill(c, fmt.Sprintf("PID %d", a.eventPIDFilter), t.Accent)
				}
				if !compact && a.eventRiskFilter != "" {
					statusPill(c, a.eventRiskFilter, workspaceStatusTone(t, map[string]string{"高风险": "danger", "需关注": "warning"}[a.eventRiskFilter]))
				}
				if a.hasEventConstraints() {
					if ui.Button(c, "重置事件筛选").Tooltip("清除搜索、PID、Agent、目标、会话、决策与风险约束").Clicked() {
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

// Header actions are laid out as one native toolbar, in the first row on
// desktop and on a separate full-width row on narrow tiled windows. MyGo
// handles overflow and keyboard navigation in both placements.
func (a *renewApp) headerActions(c *ui.Context) {
	// The native toolbar moves actions to its overflow menu on narrow
	// windows, retaining keyboard navigation and accessibility labels.
	ui.Toolbar(c, func() {
		// The icon rail intentionally exposes only the most-used pages.
		// A native category menu keeps every route one click away even
		// when the full sidebar is collapsed.
		ui.Button(c, "前往页面").Tooltip("按类别快速切换监控、管理与诊断页面").Menu(a.compactNavigationMenu)
		if a.detachedParent == nil && detachableMonitoringPage(a.page) {
			if ui.Button(c, "分离本页").Tooltip("在独立原生窗口打开此监控页面；筛选和选中状态各自独立").Clicked() {
				a.openDetachedMonitoringPage(a.page)
			}
		}
		if ui.Button(c, map[bool]string{true: "恢复事件", false: "暂停事件"}[a.paused]).
			Tooltip("只暂停 Renew 桌面事件列表的更新，不会停止后台采集和策略执行").Clicked() {
			a.paused = !a.paused
			a.eventUIPaused.Store(a.paused)
			if !a.paused {
				go a.refresh(context.Background())
			}
		}
		if ui.Button(c, "刷新").Tooltip("重新读取当前页面的数据").Clicked() {
			a.refreshActiveView()
		}
		if a.trayAvailable {
			label := "关闭后退出"
			if a.minimizeToTray {
				label = "关闭后驻留托盘"
			}
			if ui.Button(c, label).Tooltip("切换关闭窗口时退出或驻留托盘。驻留期间继续监控；从托盘明确退出才结束。").Clicked() {
				a.minimizeToTray = !a.minimizeToTray
			}
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
}

func (a *renewApp) refreshActiveView() {
	if a.page == "文件" {
		a.browseLocalDirectory(a.files.dir)
		return
	}
	if a.page == "编辑器" {
		return
	}
	// Keep the continuously streamed metrics subscription intact: opening a
	// second system WebSocket here would duplicate the collector workload.
	go a.refresh(context.Background())
	switch a.page {
	case "AgentSight":
		a.agentSightChecked = time.Time{}
		a.requestAgentSightUsage(true)
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
	case "CCS":
		go a.refreshCCS(context.Background())
		go a.refreshAstrLink(context.Background())
		go a.refreshCCR(context.Background())
		go a.refreshAntigravity(context.Background())
	case "路径权限":
		go a.refreshPathAccess()
		go a.refreshConfiguration(context.Background())
	}
}

func (a *renewApp) eventConstraintCount() int {
	count := 0
	for _, enabled := range [...]bool{
		a.search != "", a.eventPIDFilter > 0, a.eventRootPIDFilter > 0,
		a.eventTargetFilter != "", a.eventRiskFilter != "", a.eventMinRiskScore > 0,
		a.eventTypeFilter != "", a.eventSessionFilter != "",
		a.eventDecisionFilter != "", a.eventAttentionOnly,
		a.eventFileEditsOnly, a.eventDelegatedOnly,
		a.eventDomainAgent != "", a.eventDomainTarget != "", a.eventDomainKind != "",
	} {
		if enabled {
			count++
		}
	}
	return count
}

func (a *renewApp) hasEventConstraints() bool {
	return a.eventConstraintCount() > 0
}

// Offline data is still useful for inspection. Only the empty Overview gets
// a full-screen connection error; all other pages retain their controls and
// any already-captured evidence.
func (a *renewApp) connectionBanner(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Wrap().Gap(10).Padding(12).Background(t.Warning.Alpha(0.08)).Border(1, t.Warning.Alpha(0.35)).Radius(8).Children(func() {
		ui.Column(c).Grow(1).MinWidth(230).Gap(3).Children(func() {
			ui.Text(c, "监控连接已中断").TextColor(t.Warning).Bold()
			ui.Text(c, "可查看已有记录，但无法确认新活动；请勿将无新告警视作安全。").FontSize(11).TextColor(t.TextMuted)
		})
		if ui.Button(c, "重试连接").Tooltip("重新读取已有后端；仅初始启动失败时重新启动内置后端").Clicked() {
			a.retryBackendConnection()
		}
		if a.page != "系统" && ui.Button(c, "系统诊断").Tooltip("查看采集器与数据传输状态").Clicked() {
			a.page = "系统"
		}
	})
}

// Keep failures visible next to the data they may affect. This is a read-only
// status surface: visiting diagnostics must never alter collection or policy.
func (a *renewApp) captureWarning(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Wrap().Gap(10).Padding(12).AlignItems(ui.Center).
		Background(t.Danger.Alpha(0.08)).Border(1, t.Danger.Alpha(0.4)).Radius(8).Children(func() {
		ui.Column(c).Grow(1).MinWidth(230).Gap(3).Children(func() {
			ui.Text(c, "采集链路异常，摘要可能不完整").Bold().TextColor(t.Danger)
			ui.Text(c, fmt.Sprintf("eBPF 健康检查未通过 · Ringbuf 累计丢弃 %d；请核对系统诊断，勿以空白事件判断安全。",
				a.health.RingbufDroppedTotal)).FontSize(11).TextColor(t.TextMuted)
		})
		if ui.Button(c, "查看采集诊断").Tooltip("只读查看采集器健康、事件丢弃及传输状态").Clicked() {
			a.page = "系统"
		}
	})
}

// The footer never reports a live event stream when the collector itself
// has failed. Pause is a presentation state and must not mask an unhealthy
// capture pipeline.
func (a *renewApp) workspaceFooterStatus() (string, string) {
	label, level := a.pipelineStatus()
	if level == "danger" || a.starting {
		return label, level
	}
	if a.paused && level == "success" {
		return "事件展示暂停", "warning"
	}
	return label, level
}

func (a *renewApp) workspaceFooter(c *ui.Context) {
	t := c.Theme()
	_, attention, danger := a.riskCounts()
	width, _ := c.Size()
	ui.Row(c).MinHeight(29).Padding(5, 12).Gap(10).AlignItems(ui.Center).Background(t.Surface).Children(func() {
		label, level := a.workspaceFooterStatus()
		statusPill(c, label, workspaceStatusTone(t, level))
		if width >= 700 {
			ui.Text(c, fmt.Sprintf("摘要 %d", len(a.events))).FontSize(11).Font("monospace").TextColor(t.TextMuted)
		}
		if danger > 0 {
			if ui.Button(c, fmt.Sprintf("高风险 %d", danger)).Tooltip("查看高风险事件").Clicked() {
				a.clearEventFilters()
				a.eventRiskFilter = "高风险"
				a.page = "事件"
			}
		}
		if attention > 0 {
			if ui.Button(c, fmt.Sprintf("需关注 %d", attention)).Tooltip("查看需关注事件").Clicked() {
				a.clearEventFilters()
				a.eventRiskFilter = "需关注"
				a.page = "事件"
			}
		}
		ui.Spacer(c)
		if width >= 1024 && !a.lastSync.IsZero() {
			ui.Text(c, "同步 "+a.lastSync.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted)
		}
		// Nonzero packet/event loss must stay visible even in very narrow
		// windows; zero-valued diagnostics can be reached from System.
		if width >= 700 || a.health.RingbufDroppedTotal > 0 {
			if ui.Button(c, fmt.Sprintf("丢弃 %d", a.health.RingbufDroppedTotal)).Tooltip("打开采集链路诊断").Clicked() {
				a.page = "系统"
			}
		}
	})
}
