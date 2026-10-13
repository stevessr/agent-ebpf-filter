package main

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/stevessr/agent-ebpf-filter/desktop/renew/magpie"
)

// Agent skills and hooks stay separate from the eBPF kernel module manager.
func (a *renewApp) magpieView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Apache Magpie · 插件安装").FontSize(28).Bold()
	ui.Text(c, "安装 Apache Magpie 的开发 Agent 技能插件，不安装内核模块、不改写 eBPF 规则或本仓库文件。").TextColor(t.TextMuted)
	ui.Text(c, "官方来源：https://github.com/apache/magpie").FontSize(12).TextColor(t.TextMuted)
	card(c, "选择目标及插件", func() {
		ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Select(c, &a.magpieTarget, []string{magpie.Claude, magpie.Codex}).Label("目标 Agent").Width(180)
			ui.Select(c, &a.magpieFamily, magpie.Options).Label("安装范围").Width(210)
		})
		if !magpie.Available(a.magpieTarget) {
			statusPill(c, "未找到 Agent CLI", t.Warning)
			ui.Text(c, "当前 PATH 中不存在该 Agent 命令；请先安装 CLI，并以普通用户运行 Renew。").FontSize(12).TextColor(t.TextMuted)
		}
		ui.Text(c, "基础组件：magpie-setup、magpie-utilities，以及仅 Claude Code 提供的 magpie-agent-guard。首次执行会注册官方 apache/magpie Marketplace。").FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, "仅安装选定系列，不会安装所有技能。安装会联网，并可能让 Agent 在后续会话中加载新 Hook。").FontSize(12).TextColor(t.Warning)
	})
	steps, err := magpie.Plan(a.magpieTarget, a.magpieFamily)
	if err != nil {
		ui.Text(c, err.Error()).TextColor(t.Danger)
		return
	}
	card(c, "执行计划（无 Shell、无提权）", func() {
		for _, step := range steps {
			ui.Text(c, step.Display()).Font("monospace").FontSize(12).TextColor(t.TextMuted)
		}
		if a.magpieBusy {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Spinner(c)
				ui.Text(c, "正在调用本机 Agent CLI…").TextColor(t.TextMuted)
			})
		} else if magpie.Available(a.magpieTarget) {
			if ui.PrimaryButton(c, "准备安装").Clicked() {
				a.magpieConfirm = a.magpieTarget + "|" + a.magpieFamily
				a.magpieErr = ""
				a.magpieResult = ""
			}
		}
	})
	if a.magpieErr != "" {
		ui.Text(c, a.magpieErr).TextColor(t.Danger)
	}
	if a.magpieResult != "" {
		ui.Text(c, a.magpieResult).TextColor(t.Success)
	}
	if a.magpieConfirm != "" && !a.magpieBusy {
		selected := a.magpieTarget + "|" + a.magpieFamily
		if a.magpieConfirm != selected {
			a.magpieConfirm = ""
			return
		}
		ui.Column(c).Padding(16).Gap(10).Radius(12).Border(1, t.Warning.Alpha(0.45)).Children(func() {
			statusPill(c, "确认安装", t.Warning)
			ui.Text(c, "确认安装到 "+a.magpieTarget+"？插件可能执行 Hook。请确保信任 Apache Magpie 官方仓库的代码。")
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "确认执行").Clicked() {
					a.installMagpie(a.magpieTarget, a.magpieFamily)
				}
				if ui.Button(c, "取消").Clicked() {
					a.magpieConfirm = ""
				}
			})
		})
	}
	ui.Text(c, "Codex Marketplace 不提供 magpie-agent-guard；Claude Marketplace 自动更新需在 Claude 插件菜单手动启用。").FontSize(11).TextColor(t.TextMuted)
}

func (a *renewApp) installMagpie(target, family string) {
	if a.magpieBusy || a.magpieConfirm != target+"|"+family {
		return
	}
	if _, err := magpie.Plan(target, family); err != nil {
		a.magpieErr = err.Error()
		return
	}
	a.magpieConfirm = ""
	a.magpieBusy = true
	a.magpieResult = ""
	a.magpieErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
		defer cancel()
		err := magpie.Install(ctx, nil, target, family)
		a.update(func() {
			a.magpieBusy = false
			if err != nil {
				a.magpieErr = err.Error()
				return
			}
			a.magpieResult = "安装命令执行成功。请在 "+target+" 中检查插件列表，并按其提示重新加载插件。"
		})
	}()
}
