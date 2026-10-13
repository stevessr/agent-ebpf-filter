package main

import (
	"context"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/stevessr/agent-ebpf-filter/desktop/renew/magpie"
)

// The integration is deliberately independent of Renew's privileged backend.
// Installing third-party Magpie packages is always an explicit user action.
func (a *renewApp) magpieView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Magpie · 插件安装").FontSize(28).Bold()
	ui.Text(c, "yetone/magpie：多 Agent 模型管理与本地 API 网关。这里安装的是 Magpie 的 npm 插件，不是 eBPF 内核插件，也不是 Apache Magpie。").TextColor(t.TextMuted)
	ui.Text(c, magpie.ProjectURL).FontSize(12).TextColor(t.TextMuted)

	path := magpie.Locate()
	card(c, "Magpie 本体", func() {
		ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			if path != "" {
				statusPill(c, "已找到 Magpie CLI", t.Success)
				ui.Text(c, path).Font("monospace").FontSize(12).TextColor(t.TextMuted)
			} else {
				statusPill(c, "尚未安装或不在搜索路径", t.Warning)
				ui.Text(c, "可从官网安装，也可通过 Go 安装命令行版本。").TextColor(t.TextMuted)
			}
		})
		if path == "" {
			ui.Text(c, "官方方式：go install github.com/yetone/magpie@latest（需本机 Go 工具链；GUI 依赖需按上游文档准备）。").FontSize(12).TextColor(t.TextMuted)
			if !magpie.GoAvailable() {
				ui.Text(c, "未检测到 go 命令；请从 https://usemagpie.ai 安装官方构建。").TextColor(t.Warning)
			} else if !a.magpieBusy {
				if ui.Button(c, "准备安装 Magpie").Clicked() {
					a.prepareMagpieAction(magpie.InstallApp, "")
				}
			}
		} else {
			ui.Text(c, "已检测到本机程序。安装插件仅修改 Magpie 用户级插件环境，不改变 Agent eBPF Filter 的策略。").FontSize(12).TextColor(t.TextMuted)
		}
	})

	card(c, "安装 Magpie 插件", func() {
		ui.Text(c, "选择示例包名或输入受信任的 npm 包名。支持 OpenCode 鉴权插件、Pi 插件与 Magpie 网关中间件。").FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if ui.Button(c, "模型映射中间件").Clicked() {
				a.magpieSpec = "@magpie-community/middleware-model-map"
			}
			if ui.Button(c, "Gemini 鉴权").Clicked() {
				a.magpieSpec = "opencode-gemini-auth"
			}
			if ui.Button(c, "Pi Antigravity").Clicked() {
				a.magpieSpec = "pi-antigravity"
			}
		})
		ui.TextInput(c, &a.magpieSpec).Label("npm 包名").Placeholder("@magpie-community/middleware-model-map").Width(425)
		spec := strings.TrimSpace(a.magpieSpec)
		if spec != "" {
			plan, err := magpie.Plan(magpie.AddPlugin, spec)
			if err != nil {
				ui.Text(c, err.Error()).TextColor(t.Warning).FontSize(12)
			} else {
				ui.Text(c, plan.Display()).Font("monospace").FontSize(12).TextColor(t.TextMuted)
			}
		}
		if path != "" && !a.magpieBusy && magpie.ValidPackage(spec) {
			if ui.PrimaryButton(c, "准备安装插件").Clicked() {
				a.prepareMagpieAction(magpie.AddPlugin, spec)
			}
		}
		ui.Text(c, "第三方插件可执行代码、读取或修改模型请求。仅安装可信包；本入口不接受 git URL、脚本路径或 Shell 命令。").FontSize(12).TextColor(t.Warning)
	})

	if path != "" {
		card(c, "插件维护", func() {
			ui.Text(c, "通过官方命令 magpie plugin update 更新已安装插件。更新可能引入新的代码或行为。").FontSize(12).TextColor(t.TextMuted)
			if !a.magpieBusy && ui.Button(c, "准备更新所有插件").Clicked() {
				a.prepareMagpieAction(magpie.UpdatePlugins, "")
			}
		})
	}

	if a.magpieBusy {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c)
			ui.Text(c, "正在执行本机 Magpie/Go 命令…").TextColor(t.TextMuted)
		})
	}
	if a.magpieErr != "" {
		ui.Text(c, a.magpieErr).TextColor(t.Danger)
	}
	if a.magpieNotice != "" {
		ui.Text(c, a.magpieNotice).TextColor(t.Success)
	}
	if a.magpiePendingAction != "" && !a.magpieBusy {
		plan, err := magpie.Plan(a.magpiePendingAction, a.magpiePendingSpec)
		if err == nil {
			ui.Column(c).Padding(16).Gap(10).Radius(12).Border(1, t.Warning.Alpha(0.45)).Children(func() {
				statusPill(c, "安装/更新需确认", t.Warning)
				ui.Text(c, "即将作为当前普通用户执行以下命令：")
				ui.Text(c, plan.Display()).Font("monospace")
				ui.Text(c, "此操作会联网，并可能执行第三方代码；与 eBPF 内核权限完全隔离。").TextColor(t.Warning).FontSize(12)
				ui.Row(c).Gap(8).Children(func() {
					if ui.PrimaryButton(c, "确认执行").Clicked() {
						a.runMagpieAction(a.magpiePendingAction, a.magpiePendingSpec)
					}
					if ui.Button(c, "取消").Clicked() {
						a.magpiePendingAction = ""
						a.magpiePendingSpec = ""
					}
				})
			})
		}
	}
}

func (a *renewApp) prepareMagpieAction(action, spec string) {
	if a.magpieBusy {
		return
	}
	if _, err := magpie.Plan(action, spec); err != nil {
		a.magpieErr = err.Error()
		return
	}
	a.magpiePendingAction, a.magpiePendingSpec = action, spec
	a.magpieErr, a.magpieNotice = "", ""
}

func (a *renewApp) runMagpieAction(action, spec string) {
	if a.magpieBusy || a.magpiePendingAction != action || a.magpiePendingSpec != spec {
		return
	}
	a.magpiePendingAction, a.magpiePendingSpec = "", ""
	a.magpieErr, a.magpieNotice = "", ""
	a.magpieBusy = true
	go func() {
		err := magpie.Execute(context.Background(), nil, action, spec)
		a.update(func() {
			a.magpieBusy = false
			if err != nil {
				a.magpieErr = err.Error()
				return
			}
			switch action {
			case magpie.InstallApp:
				a.magpieNotice = "Go 安装完成。若仍检测不到 magpie，请将 GOBIN 或 GOPATH/bin 加入 PATH，或安装官方桌面发行版。"
			case magpie.AddPlugin:
				a.magpieNotice = "已向 Magpie 提交插件安装命令："+spec+"；可在 Magpie 的 Settings → Plugins 查看状态。"
			case magpie.UpdatePlugins:
				a.magpieNotice = "Magpie 插件更新命令执行成功；请在 Magpie 内确认状态。"
			}
		})
	}()
}
