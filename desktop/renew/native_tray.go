package main

import (
	_ "embed"

	"github.com/egoist/mygo"
)

//go:embed resources/icon.png
var renewTrayIconPNG []byte

// The tray is deliberately optional. If Wayland's status notifier is missing,
// closing the only window must retain the ordinary exit behavior.
func (a *renewApp) setupTray(window *mygo.Window) error {
	open := func(page string) {
		window.Update(func() {
			a.page = page
		})
		window.Restore()
		window.Show()
		window.Focus()
	}
	domainMenuLabel, domainMenuPage := "Agent 域名监控", "域名"
	riskMenuLabel := "风险事件"
	exitMenuLabel := "退出（停止本程序启动的采集器）"
	if a.localMonitor {
		domainMenuLabel, domainMenuPage = "本机采样状态", "系统"
		riskMenuLabel = "采样事件"
		exitMenuLabel = "退出（停止本机采样）"
	}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon: renewTrayIconPNG,
		ToolTip: desktopBrandName + " · Agent 安全监控",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开安全概览", Click: func(*mygo.MenuItem, *mygo.Window) { open("概览") }},
			{Label: domainMenuLabel, Click: func(*mygo.MenuItem, *mygo.Window) { open(domainMenuPage) }},
			{Label: riskMenuLabel, Click: func(*mygo.MenuItem, *mygo.Window) {
				window.Update(func() {
					a.clearEventFilters()
					if !a.localMonitor { a.eventAttentionOnly = true }
					a.page = "事件"
				})
				window.Restore()
				window.Show()
				window.Focus()
			}},
			mygo.Separator(),
			{Label: exitMenuLabel, Click: func(*mygo.MenuItem, *mygo.Window) {
				a.minimizeToTray = false
				mygo.App.Quit()
			}},
		}),
	})
	if err != nil {
		return err
	}
	a.tray = tray
	a.trayAvailable = true
	window.OnClose(func(e *mygo.CloseEvent) {
		if a.minimizeToTray {
			e.PreventDefault()
			window.Hide()
		}
	})
	return nil
}
