package main

import (
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
// Evaluate the larger of the animated and target sidebar widths: when the
// user opens the sidebar, reserve space before the animation starts; when
// closing it, release space only when the animated width actually shrinks.
func showWorkspaceInspector(windowWidth, navigationWidth float32, expanded bool, page string) bool {
	const activityRailWidth = 54
	const inspectorWidth = 302
	const minimumCenterWidth = 760
	return expanded && pageHasInspector(page) &&
		windowWidth-activityRailWidth-navigationWidth-inspectorWidth >= minimumCenterWidth
}

// Layout-sensitive controls must not switch between compact and expanded
// arrangements on each intermediate spring frame. Max(current, target) is
// conservative in both directions and prevents the inspector from briefly
// occupying space that the opening sidebar is about to need.
func workspaceMotionLayoutWidth(current, target float32) float32 {
	return max(current, target)
}

const (
	workspaceNavigationCompactWidth  float32 = 198
	workspaceNavigationDetailedWidth float32 = 302
)

// Page IDs remain stable; only the optional sidebar's visible names change.
var workspaceNavigationLabels = map[string][2]string{
	"概览":         {"态势总览", "系统态势与运行概览"},
	"行为":         {"行为轨道", "行为轨道与风险证据"},
	"研判":         {"风险研判工作台", "风险事件研判工作台"},
	"事件":         {"事件流", "eBPF 实时事件流"},
	"会话":         {"Agent 会话", "Agent 会话与行为关联"},
	"AgentSight": {"AgentSight 洞察", "会话、模型 Token 与轨迹证据"},
	"派生":         {"Agent 派生", "Agent → SubAgent 调用与进程派生"},
	"网络":         {"网络外联", "网络连接与外联目标"},
	"域名":         {"Agent 域名监控", "Agent 域名与外联目的地监控"},
	"进程":         {"进程活动", "进程活动与资源监测"},
	"Agent 识别":   {"捕获与监视范围", "Agent 识别与捕获监视范围"},
	"监控":         {"采集设置", "实时采集与监控设置"},
	"eBPF 模块":    {"内核模块", "eBPF 内核模块管理"},
	"规则":         {"Wrapper 规则", "Wrapper 拦截与防护规则"},
	"跟踪":         {"跟踪范围", "进程、命令与路径跟踪"},
	"路径权限":       {"文件访问保护", "文件路径与访问权限"},
	"终端":         {"本地 Shell · 多标签与分屏", "本地 Shell 终端与多窗格"},
	"文件":         {"文件管理", "本机文件管理与目录浏览"},
	"编辑器":        {"原生编辑器", "MyGo 原生代码编辑器"},
	"CCS":        {"管理软件联动", "CC-Switch / AstrLink / CCR / Antigravity 本地视图"},
	"系统":         {"系统诊断", "采集链路与系统运行诊断"},
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

// Keep at least a laptop-sized center canvas even if the user prefers full
// navigation labels. The preference survives resizing and is restored when
// there is enough room; the icon rail remains available at every size.
const (
	workspaceActivityRailWidth      float32 = 54
	workspaceDetailedMinWindowWidth float32 = 1240
	workspaceHeaderMinContentWidth  float32 = 860
	workspaceNarrowNavigationWidth  float32 = 900
	workspaceTightHeaderWidth       float32 = 660
)

func workspaceResponsiveNavigationWidth(windowWidth float32, open, detailed bool) (float32, bool) {
	// On compact laptop/tiling layouts the expanded sidebar would reduce
	// the live event table below its usable width. Use the native rail menu
	// instead, without changing the user's preferred sidebar state.
	if windowWidth < workspaceNarrowNavigationWidth {
		return 0, false
	}
	effectiveDetailed := detailed && windowWidth >= workspaceDetailedMinWindowWidth
	return workspaceNavigationWidth(open, effectiveDetailed), effectiveDetailed
}

func workspaceUsesNavigationMenu(windowWidth float32) bool {
	return windowWidth < workspaceNarrowNavigationWidth
}

func workspaceTightHeader(windowWidth, navigationWidth float32) bool {
	return windowWidth-workspaceActivityRailWidth-navigationWidth < workspaceTightHeaderWidth
}

func workspaceCompactHeader(windowWidth, navigationWidth float32) bool {
	return windowWidth-workspaceActivityRailWidth-navigationWidth < workspaceHeaderMinContentWidth
}

func workspaceShowsBackendError(page string, starting bool, lastErr string, eventCount int) bool {
	return !starting && page == "概览" && lastErr != "" && eventCount == 0
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
	// MyGo v0.7.3 animations honor the OS reduced-motion setting.
	// Below the compact threshold there is no room to animate a reserved
	// sidebar: release it immediately rather than squeezing live event tables.
	navTarget, navDetailed := workspaceResponsiveNavigationWidth(width, a.navigationOpen, a.navigationDetailed)
	var navWidth float32
	if workspaceUsesNavigationMenu(width) {
		navWidth = shell.Jump("renew-navigation", 0)
	} else {
		navWidth = shell.AnimateTo("renew-navigation", navTarget, renewNavigationMotion())
	}
	// Only the painted sidebar width animates. Reflow-sensitive controls
	// make one conservative decision per target change, not one per frame.
	layoutNavWidth := workspaceMotionLayoutWidth(navWidth, navTarget)
	shell.Children(func() {
		a.activityRail(c, width)
		if navWidth > 0 {
			ui.Row(c).Width(navWidth).Shrink(0).AlignItems(ui.Stretch).Clip().Children(func() {
				a.sidebar(c, navDetailed)
			})
		}
		ui.Column(c).Key("workspace-content").Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			a.header(c, layoutNavWidth)
			if a.page == "终端" {
				// Real PTY bounds without a parent Scroll; also usable offline.
				a.terminalView(c)
			} else if a.page == "文件" {
				a.localFileManagerView(c)
			} else if a.page == "编辑器" {
				a.localEditorView(c)
			} else {
				ui.Row(c).Grow(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() {
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						pagePadding := float32(20)
						if width < 1200 {
							pagePadding = 12
						}
						// Keep each page's scroll state isolated from the next page.
						// Switching from a deep event history to Overview must not
						// jump the landing card out of view.
						ui.Scroll(c.Key("renew-page-scroll-" + a.page)).
							Grow(1).Padding(pagePadding).Gap(16).Children(func() {
							switch {
							case a.starting:
								a.startingView(c)
							case workspaceShowsBackendError(a.page, a.starting, a.lastErr, len(a.events)):
								a.errorView(c)
							default:
								if !a.connected && a.lastErr != "" {
									a.connectionBanner(c)
								}
								// A populated event table is not proof that collection is healthy.
								// Make capture failures visible on every monitoring page.
								if a.connected && a.healthReady && !a.health.CaptureHealthy && a.page != "概览" && a.page != "系统" {
									a.captureWarning(c)
								}
								ui.Column(c).Key("page-" + a.page).Gap(16).Transition(renewPageTransition()).Children(func() { a.workspacePage(c) })
							}
						})
					})
					if showWorkspaceInspector(width, layoutNavWidth, a.inspectorOpen, a.page) {
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
	case "行为":
		a.behaviorView(c)
	case "研判":
		a.inspectorStandalone(c)
	case "会话":
		a.sessionsView(c)
	case "AgentSight":
		a.agentSightView(c)
	case "派生":
		a.agentLineageView(c)
	case "网络":
		a.networkView(c)
	case "域名":
		a.agentDomainsView(c)
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
	case "文件":
		a.localFileManagerView(c)
	case "编辑器":
		a.localEditorView(c)
	case "系统":
		a.systemView(c)
	case "CCS":
		a.integrationsView(c)
	default:
		a.overview(c)
	}
}
