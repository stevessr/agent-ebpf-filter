package main

import (
	"fmt"
	"github.com/egoist/mygo/ui"
	"reflect"
)

// Harness updates run in ordinary user terminals, never in the privileged backend.
func (a *renewApp) beginHarnessUpdateScan() {
	if a.harnessUpdateBusy {
		return
	}
	a.harnessUpdateBusy = true
	a.harnessUpdateConfirm = ""
	go func() {
		plans := scanHarnessUpdatePlans()
		if a.terminalClosing.Load() {
			return
		}
		a.update(func() {
			a.harnessUpdatePlans = plans
			a.harnessUpdateScanned = true
			a.harnessUpdateBusy = false
			a.harnessUpdateError = ""
			chosen := false
			for _, plan := range plans {
				if plan.ID == a.harnessUpdateSelected {
					chosen = true
					break
				}
			}
			if !chosen {
				a.harnessUpdateSelected = ""
				for _, plan := range plans {
					if len(plan.Command) > 0 {
						a.harnessUpdateSelected = plan.ID
						break
					}
				}
				if a.harnessUpdateSelected == "" && len(plans) > 0 {
					a.harnessUpdateSelected = plans[0].ID
				}
			}
		})
	}()
}
func (a *renewApp) selectedHarnessUpdatePlan() (harnessUpdatePlan, bool) {
	for _, plan := range a.harnessUpdatePlans {
		if plan.ID == a.harnessUpdateSelected {
			return plan, true
		}
	}
	return harnessUpdatePlan{}, false
}
func harnessCommandForID(id string) string {
	if id == "deepseek" {
		return "dsh"
	}
	for _, spec := range codingHarnessCatalog {
		if spec.ID == id {
			return spec.Command
		}
	}
	return ""
}
func reinspectHarnessUpdatePlan(plan harnessUpdatePlan, p harnessProbe) harnessUpdatePlan {
	if plan.ID == "claude-desktop" || plan.ID == "chatgpt-desktop" {
		for _, candidate := range scanDesktopAppUpdates(p) {
			if candidate.ID == plan.ID {
				return candidate
			}
		}
		return harnessUpdatePlan{ID: plan.ID, Source: "missing"}
	}
	path, _ := p.lookup(harnessCommandForID(plan.ID))
	return inspectHarnessInstallation(plan.ID, plan.Label, path, p)
}
func (a *renewApp) startHarnessUpdate(plan harnessUpdatePlan) {
	if a.harnessUpdateBusy || len(plan.Command) == 0 {
		return
	}
	a.harnessUpdateConfirm = ""
	a.harnessUpdateBusy = true
	a.harnessUpdateError = ""
	go func() {
		// Reinspect active ownership immediately before launching the updater.
		probe := systemHarnessProbe()
		latest := reinspectHarnessUpdatePlan(plan, probe)
		if a.terminalClosing.Load() {
			return
		}
		a.update(func() {
			a.harnessUpdateBusy = false
			if latest.Path != plan.Path || latest.Source != plan.Source || latest.Package != plan.Package || latest.Evidence != plan.Evidence || !reflect.DeepEqual(latest.Command, plan.Command) {
				a.harnessUpdateError = "安装来源已变化，请重新检测后确认。"
				a.harnessUpdateScanned = false
				return
			}
			pane, err := a.newTerminalPaneCommand(terminalHomeDir(), latest.Command)
			if err != nil {
				a.harnessUpdateError = fmt.Sprintf("无法打开更新终端：%v", err)
				return
			}
			a.appendTerminalTab(pane, "更新 "+plan.Label)
			a.harnessUpdateError = "已打开更新终端；请检查执行日志与最终退出状态。"
		})
	}()
}
func (a *renewApp) harnessUpdaterView(c *ui.Context) {
	t := c.Theme()
	if !a.harnessUpdateScanned && !a.harnessUpdateBusy {
		a.beginHarnessUpdateScan()
	}
	ui.Column(c).MinWidth(0).Padding(8, 12).Gap(5).Radius(10).Border(1, t.Border).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(8).MinWidth(0).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Harness 更新管理").FontSize(12).Bold()
			if a.harnessUpdateBusy {
				statusPill(c, "检测中", t.Warning)
				renewLoadingIndicator(c, "正在检查本地 Harness 更新来源")
			} else if a.harnessUpdateScanned {
				statusPill(c, "本机来源检测", t.Success)
			}
			ui.Spacer(c)
			if !a.harnessUpdateBusy && ui.Button(c, "重新检测").Tooltip("只读检查本机可执行文件、包数据库与安装目录").Clicked() {
				a.beginHarnessUpdateScan()
			}
		})
		if !a.harnessUpdateScanned {
			ui.Text(c, "正在检测 PATH、软件包归属与官方安装目录…").FontSize(11).TextColor(t.TextMuted)
			return
		}
		plan, ok := a.selectedHarnessUpdatePlan()
		if !ok {
			return
		}
		ui.Row(c).Gap(8).MinWidth(0).AlignItems(ui.Center).Children(func() {
			ui.Button(c, plan.Label+" ▾").Tooltip("选择要更新的 Harness").Menu(func(menu *ui.Menu) {
				for _, entry := range a.harnessUpdatePlans {
					label := entry.Label + " · " + entry.Source
					if menu.Item(label).Checked(entry.ID == a.harnessUpdateSelected).Chosen() {
						a.harnessUpdateSelected = entry.ID
						a.harnessUpdateConfirm = ""
						a.harnessUpdateError = ""
					}
				}
			})
			ui.Text(c, "来源: "+plan.Source).FontSize(11).Bold()
			if plan.Package != "" {
				ui.Text(c, "包: "+plan.Package).FontSize(11).TextColor(t.TextMuted).SingleLine()
			}
			ui.Spacer(c)
			if len(plan.Command) > 0 && !a.harnessUpdateBusy && a.harnessUpdateConfirm != plan.ID {
				if ui.Button(c, "预览并准备更新").Clicked() {
					a.harnessUpdateConfirm = plan.ID
				}
			}
		})
		if plan.Path != "" {
			ui.Text(c, "入口: "+plan.Path).FontSize(10).TextColor(t.TextMuted).SingleLine()
		}
		if plan.Evidence != "" {
			ui.Text(c, plan.Evidence).FontSize(10).TextColor(t.TextMuted)
		}
		if len(plan.Command) > 0 {
			ui.Text(c, "执行预览: "+updatePreview(plan.Command)).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
		if plan.Advisory != "" {
			ui.Text(c, plan.Advisory).FontSize(11).TextColor(t.TextMuted)
		}
		if a.harnessUpdateConfirm == plan.ID && !a.harnessUpdateBusy && len(plan.Command) > 0 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				statusPill(c, "需要确认", t.Warning)
				ui.Text(c, "在本地普通用户终端执行指定包更新；如需系统授权，由安装器自行提示。").FontSize(11)
				if ui.PrimaryButton(c, "确认执行更新").Clicked() {
					a.startHarnessUpdate(plan)
				}
				if ui.Button(c, "取消").Clicked() {
					a.harnessUpdateConfirm = ""
				}
			})
		}
		if a.harnessUpdateError != "" {
			ui.Text(c, a.harnessUpdateError).FontSize(11).TextColor(t.Warning)
		}
	})
}
