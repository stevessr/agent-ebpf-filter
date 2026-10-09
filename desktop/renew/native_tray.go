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
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon: renewTrayIconPNG,
		ToolTip: desktopBrandName + " · Agent 安全监控",
		Menu: mygo.NewMenu([]*mygo.MenuItem{
			{Label: "打开安全概览", Click: func(*mygo.MenuItem, *mygo.Window) { open("概览") }},
			{Label: "Agent 域名监控", Click: func(*mygo.MenuItem, *mygo.Window) { open("域名") }},
			{Label: "风险事件", Click: func(*mygo.MenuItem, *mygo.Window) {
				window.Update(func() {
					a.clearEventFilters()
					a.eventAttentionOnly = true
					a.page = "事件"
				})
				window.Restore()
				window.Show()
				window.Focus()
			}},
			mygo.Separator(),
			{Label: "退出（停止本程序启动的采集器）", Click: func(*mygo.MenuItem, *mygo.Window) {
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
