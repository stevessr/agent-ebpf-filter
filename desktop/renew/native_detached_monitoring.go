package main

import (
    "runtime"

    "github.com/egoist/mygo"
    "github.com/egoist/mygo/ui"
)

var detachableMonitoringPages = map[string]string{
    "概览": "overview",
    "研判": "inspector",
    "事件": "events",
    "会话": "sessions",
    "网络": "network",
    "域名": "domains",
    "进程": "processes",
    "系统": "system",
}

func detachableMonitoringPage(page string) bool {
    _, ok := detachableMonitoringPages[page]
    return ok
}

// copyMonitoringSnapshot transfers compact, already-redacted UI data.
// Floating pages keep their own search, selection, tabs and view caches.
func (a *renewApp) copyMonitoringSnapshot(child *renewApp) {
    if child == nil {
        return
    }
    child.client = a.client
    child.localWindowsMonitoring = a.localWindowsMonitoring
    child.starting = a.starting
    child.paused = a.paused
    child.connected = a.connected
    child.healthReady = a.healthReady
    child.lastErr = a.lastErr
    child.lastSync = a.lastSync
    child.health = a.health
    child.trackedComms = a.trackedComms
    child.eventStreamConnected = a.eventStreamConnected
    child.eventStreamErr = a.eventStreamErr
    child.system = a.system
    child.systemConnected = a.systemConnected
    child.systemErr = a.systemErr
    child.historyInitialized = a.historyInitialized
    child.historyCursor = a.historyCursor
    if child.eventsVersion != a.eventsVersion {
        child.events = append(child.events[:0], a.events...)
        child.eventsVersion = a.eventsVersion
        child.filterCacheValid = false
        child.riskCacheValid = false
        child.networkCacheValid = false
        child.processCacheValid = false
        child.domainCacheValid = false
        child.optionCacheValid = false
    }
}

// openDetachedMonitoringPage reuses the main collector and IPC. It does not
// start any new WebSocket, backend service, or elevated process.
func (a *renewApp) openDetachedMonitoringPage(page string) {
    key, ok := detachableMonitoringPages[page]
    if !ok || a.detachedParent != nil || a.terminalClosing.Load() {
        return
    }
    if existing := a.detachedPages[page]; existing != nil && existing.win != nil {
        existing.win.Restore()
        existing.win.Show()
        existing.win.Focus()
        return
    }
    child := newRenewApp(a.backend)
    child.detachedParent = a
    child.detachedPageID = page
    child.page = page
    child.inspectorTab = a.inspectorTab
    child.inspectorPinnedID = a.inspectorPinnedID
    child.inspectorSelectedID = a.inspectorSelectedID
    a.copyMonitoringSnapshot(child)

    win := mygo.NewWindow(mygo.WindowOptions{
        Title: desktopBrandName + " · " + page,
        Width: 1080,
        Height: 760,
        MinWidth: 640,
        MinHeight: 430,
        StateKey: "monitor-" + key,
        Vibrancy: windowMaterial(runtime.GOOS, a.materialEnabled),
        Content: ui.View(child.detachedMonitoringView),
    })
    child.win = win
    if a.detachedPages == nil {
        a.detachedPages = make(map[string]*renewApp)
    }
    a.detachedPages[page] = child
    win.OnClosed(func() {
        child.releaseEventDetailPayload()
        delete(a.detachedPages, page)
        child.win = nil
        a.restoreMainAfterLastDetached()
    })
    win.Show()
    win.Focus()
}

// redirectDetachedNavigation keeps a detached window dedicated to one page.
// Internal links into other monitoring or policy pages open in the owner
// workspace; a detached observation window never becomes a policy editor.
func (a *renewApp) redirectDetachedNavigation() string {
	if a.detachedParent == nil || a.detachedPageID == "" || a.page == a.detachedPageID {
		return ""
	}
	target := a.page
	a.page = a.detachedPageID
	return target
}

func (a *renewApp) openDetachedDestinationInMain(target string) {
	if a.detachedParent == nil || target == "" || a.detachedParent.win == nil {
		return
	}
	parent := a.detachedParent
	parent.hiddenForDetached = false
	parent.win.Update(func() {
		parent.page = target
	})
	parent.win.Restore()
	parent.win.Show()
	parent.win.Focus()
}

func (a *renewApp) detachedMonitoringView(c *ui.Context) {
    if target := a.redirectDetachedNavigation(); target != "" {
        a.openDetachedDestinationInMain(target)
    }
    styleRenewWorkspaceTheme(&a.workspaceTheme, c.Theme(), c.Preferences().HighContrast)
    c.SetTheme(&a.workspaceTheme)
    theme := c.Theme()
    ui.Column(c).Fill().MinWidth(0).MinHeight(0).Background(theme.Background).Children(func() {
        ui.Row(c).MinHeight(54).Padding(10, 16).Gap(10).Wrap().AlignItems(ui.Center).Background(theme.Surface).Children(func() {
            ui.Text(c, a.page).FontSize(17).Bold()
            status, level := a.pipelineStatus()
            statusPill(c, status, workspaceStatusTone(theme, level))
            if a.paused {
                statusPill(c, "事件展示暂停", theme.Warning)
            }
            ui.Spacer(c)
            if ui.Button(c, "系统诊断").Tooltip("在主工作台检查采集状态，避免独立窗口重复建立连接").Clicked() {
                a.openDetachedDestinationInMain("系统")
            }
            if ui.Button(c, "返回主窗口").Tooltip("在主工作台打开本页，关闭独立窗口").Clicked() {
                parent := a.detachedParent
                if parent != nil && parent.win != nil {
                    parent.hiddenForDetached = false
                    parent.win.Update(func() { parent.page = a.page })
                    parent.win.Restore()
                    parent.win.Show()
                    parent.win.Focus()
                }
                if a.win != nil {
                    a.win.Close()
                }
            }
        })
        ui.Divider(c)
        ui.Scroll(c).Grow(1).MinHeight(0).MinWidth(0).Padding(16).Gap(14).Children(func() {
            if a.starting || !a.connected || !a.healthReady || !a.health.CaptureHealthy || !a.eventStreamConnected || a.paused {
                ui.Row(c).Wrap().Padding(10).Gap(8).Radius(8).
                    Background(theme.Warning.Alpha(0.08)).Border(1, theme.Warning.Alpha(0.32)).
                    Children(func() {
                        ui.Text(c, a.overviewSnapshotHint()).FontSize(12).TextColor(theme.Warning)
                    })
            }
            a.workspacePage(c)
            // A link inside the observation page may request another route.
            // Redirect it after the current frame rather than rendering
            // privileged controls into a floating monitoring window.
            if target := a.redirectDetachedNavigation(); target != "" {
                a.openDetachedDestinationInMain(target)
            }
        })
        wasOpen := a.eventDetailOpen
        a.eventDetailModal(c)
        if wasOpen && !a.eventDetailOpen {
            a.releaseEventDetailPayload()
        }
    })
}
