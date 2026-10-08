package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *renewApp) ebpfModulesView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "eBPF 模块").FontSize(28).Bold()
	ui.Text(c, "按需加载或卸载已注册的自定义 eBPF 程序。操作会真正挂载或关闭对应内核 eBPF 链接，而不是只过滤事件。").TextColor(t.TextMuted)
	ui.Text(c, "内置核心采集器不在此列表中；其事件组可在「监控」调整。手动操作仅影响当前后端运行期，不改变模块的开机启用设置。").FontSize(11).TextColor(t.TextMuted)
	if a.runtimeReady && !a.runtimeCfg.Runtime.PolicyManagementEnabled {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			statusPill(c, "策略管理未启用", t.Warning)
			ui.Text(c, "后端要求启用策略管理后才能加载或卸载 eBPF 插件。").FontSize(12).TextColor(t.TextMuted)
			if ui.Button(c, "前往监控设置").Clicked() {
				a.page = "监控"
			}
		})
	}

	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		if ui.Button(c, "刷新模块状态").Clicked() && !a.modulesBusy {
			go a.refreshEBPFModules(context.Background())
		}
		if a.modulesBusy {
			ui.Spinner(c)
			ui.Text(c, "等待后端确认…").TextColor(t.TextMuted)
		}
		if a.modulesReady {
			loaded := 0
			for _, module := range a.modules {
				if module.Loaded {
					loaded++
				}
			}
			ui.Spacer(c)
			statusPill(c, fmt.Sprintf("已挂载 %d / %d", loaded, len(a.modules)), t.Accent)
		}
	})
	if a.modulesErr != "" {
		ui.Text(c, a.modulesErr).TextColor(t.Danger)
	}
	if a.modulesNotice != "" {
		ui.Text(c, a.modulesNotice).TextColor(t.Success)
	}
	if !a.modulesReady {
		ui.Text(c, "模块列表尚未从后端读取；请检查连接或点击刷新。").TextColor(t.TextMuted)
		return
	}
	if len(a.modules) == 0 {
		card(c, "没有已注册的 eBPF 模块", func() {
			ui.Text(c, "可先在 Web 工作台的插件管理器中创建、编译并注册 eBPF 程序，然后在此按需加载和卸载。").TextColor(t.TextMuted)
		})
		return
	}

	for _, module := range a.modules {
		module := module
		name := strings.TrimSpace(module.Name)
		if name == "" {
			name = module.ID
		}
		card(c, name, func() {
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(5).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
						ui.Text(c, module.ID).Font("monospace").FontSize(11).TextColor(t.TextMuted)
						switch {
						case module.Loaded:
							statusPill(c, "已挂载", t.Success)
						case module.LoadError != "":
							statusPill(c, "加载失败", t.Danger)
						default:
							statusPill(c, "未挂载", t.TextMuted)
						}
						if module.Enabled {
							statusPill(c, "自动启用", t.Accent)
						} else {
							statusPill(c, "手动模式", t.TextMuted)
						}
					})
					if module.Description != "" {
						ui.Text(c, module.Description).FontSize(12).TextColor(t.TextMuted)
					}
					target := module.AttachKind
					if module.AttachTarget != "" {
						target += " · " + module.AttachTarget
					}
					if module.ProgramName != "" {
						target += " · " + module.ProgramName
					}
					if target != "" {
						ui.Text(c, target).Font("monospace").FontSize(11).TextColor(t.TextMuted)
					}
					if module.LoadError != "" {
						ui.Text(c, "后端错误：" + module.LoadError).FontSize(11).TextColor(t.Danger)
					}
				})
				if module.Loaded {
					if ui.Button(c, "卸载").Clicked() && !a.modulesBusy {
						a.modulesPendingUnload = module.ID
						a.modulesNotice = ""
					}
				} else {
					if ui.PrimaryButton(c, "加载").Clicked() && !a.modulesBusy {
						a.setEBPFModuleLoaded(module.ID, true)
					}
				}
			})
		})
	}

	if a.modulesPendingUnload != "" {
		id := a.modulesPendingUnload
		ui.Column(c).Padding(16).Gap(10).Radius(12).Background(t.Danger.Alpha(0.05)).Border(1, t.Danger.Alpha(0.38)).Children(func() {
			statusPill(c, "确认卸载", t.Danger)
			ui.Text(c, "确认卸载 eBPF 模块 "+id+"？这会解除该模块的内核挂载，其对应监控能力将立即停止；模块配置与其他 eBPF 程序不会被删除。")
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "确认卸载").Clicked() && !a.modulesBusy {
					a.setEBPFModuleLoaded(id, false)
				}
				if ui.Button(c, "取消").Clicked() && !a.modulesBusy {
					a.modulesPendingUnload = ""
				}
			})
		})
	}
}

func (a *renewApp) refreshEBPFModules(parent context.Context) {
	if a.client == nil {
		return
	}
	a.update(func() {
		a.modulesBusy = true
		a.modulesErr = ""
		a.modulesNotice = ""
	})
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	modules, err := a.client.ebpfModules(ctx)
	a.update(func() {
		a.modulesBusy = false
		if err != nil {
			a.modulesErr = err.Error()
			return
		}
		a.modules = modules
		a.modulesReady = true
		a.modulesPendingUnload = ""
	})
}

func findEBPFModule(modules []ebpfModule, id string) (ebpfModule, bool) {
	for _, module := range modules {
		if module.ID == id {
			return module, true
		}
	}
	return ebpfModule{}, false
}

func (a *renewApp) setEBPFModuleLoaded(id string, load bool) {
	if a.client == nil || !a.modulesReady || a.modulesBusy {
		return
	}
	module, found := findEBPFModule(a.modules, id)
	if !found || module.Loaded == load {
		return
	}
	a.modulesBusy = true
	a.modulesPendingUnload = ""
	a.modulesErr = ""
	a.modulesNotice = ""

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, err := a.client.setEBPFModuleLoaded(ctx, id, load)
		var latest []ebpfModule
		if err == nil {
			latest, err = a.client.ebpfModules(ctx)
			if err != nil {
				err = fmt.Errorf("操作接口已返回成功，但重新读取模块状态失败：%w", err)
			} else if confirmed, found := findEBPFModule(latest, id); !found || confirmed.Loaded != load {
				err = fmt.Errorf("后端未确认模块 %s 的最终挂载状态", id)
			}
		}
		a.update(func() {
			a.modulesBusy = false
			if err != nil {
				a.modulesErr = err.Error()
				return
			}
			a.modules = latest
			if load {
				a.modulesNotice = "eBPF 模块 " + id + " 已加载"
			} else {
				a.modulesNotice = "eBPF 模块 " + id + " 已卸载"
			}
		})
	}()
}
