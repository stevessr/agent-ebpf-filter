package main

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The desktop reads local config files only after a deliberate user action.
// It never sends file contents, environment values, or credentials to the
// privileged eBPF backend. All presentation fields are allowlisted.
func (a *renewApp) refreshAgentConfigInspection() {
	if a.configLoading {
		return
	}
	a.configGeneration++
	generation := a.configGeneration
	project := strings.TrimSpace(a.configProjectPath)
	a.configLoading = true
	go func() {
		result := inspectLocalAgentConfigs(project)
		a.update(func() {
			if generation != a.configGeneration {
				return
			}
			a.configInspection = result
			a.configLoaded = true
			a.configLoading = false
		})
	}()
}

func (a *renewApp) agentConfigPanel(c *ui.Context) {
	t := c.Theme()
	card(c, "Claude Code / Codex 配置识别（只读）", func() {
		ui.Text(c, "从当前桌面用户的 Claude settings.json、Codex config.toml 读取模型、Provider 与 API 域名；可指定一个项目目录检查项目配置。不会读取 auth.json 或执行 Hook/MCP 命令；设置文件可能包含凭据，但仅保留白名单安全字段。").FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.configProjectPath).Label("项目绝对路径（可选）").Width(340)
			label := "读取本机配置"
			if a.configLoaded {
				label = "重新读取配置"
			}
			if a.configLoading {
				label = "正在读取配置…"
			}
			if ui.PrimaryButton(c, label).Disabled(a.configLoading).Clicked() {
				a.refreshAgentConfigInspection()
			}
		})
		ui.Text(c, "只显示白名单字段与 URL 的主机名，不展示 URL 路径、参数、Header、API Key 或完整设置。项目配置仅为候选：实际生效配置还受 CLI、环境变量、受信任工作区和受管策略影响。").FontSize(11).TextColor(t.TextMuted)
		if !a.configLoaded {
			if a.configLoading {
				ui.Text(c, "正在分析本地文件…").TextColor(t.TextMuted)
			} else {
				ui.Text(c, "尚未读取本机配置。点击按钮进行一次性只读检查。").TextColor(t.TextMuted)
			}
			return
		}
		for _, note := range a.configInspection.Notes {
			ui.Text(c, note).FontSize(11).TextColor(t.Warning)
		}
		rows := a.configInspection.Candidates
		if len(rows) == 0 {
			return
		}
		ui.Text(c, fmt.Sprintf("已识别 %d 条配置声明。以下“观测”仅来自当前有限事件摘要，不表示完整网络请求或配置生效状态。", len(rows))).FontSize(11).TextColor(t.TextMuted)
		cols := []ui.TableColumn{
			{Title: "Agent", Width: 124, Fixed: true},
			{Title: "配置来源", MinWidth: 156},
			{Title: "Provider", MinWidth: 150},
			{Title: "模型", MinWidth: 132},
			{Title: "配置安全模式", MinWidth: 190},
			{Title: "候选主机", MinWidth: 185},
			{Title: "观测事件", Width: 78, Align: ui.End},
		}
		ownership := buildAgentOwnershipIndex(a.events, nil)
		a.configTable.Key = func(i int) any {
			row := rows[i]
			return row.Agent + "\x00" + row.Scope + "\x00" + row.Provider + "\x00" + row.Source
		}
		ui.Table(c, &a.configTable, cols, len(rows), func(i, col int) {
			item := rows[i]
			switch col {
			case 0:
				ui.Text(c, item.Agent).SingleLine()
			case 1:
				ui.Text(c, item.Scope+" · "+item.Source).SingleLine()
			case 2:
				ui.Text(c, displayOr(item.Provider, "未声明")).SingleLine()
			case 3:
				ui.Text(c, displayOr(item.Model, "未声明")).SingleLine()
			case 4:
				ui.Text(c, displayOr(item.Security, "未声明")).SingleLine()
			case 5:
				ui.Text(c, formatConfigCandidate(item)).SingleLine()
			case 6:
				ui.Text(c, fmt.Sprint(countObservedConfigHostWithIndex(item.Agent, item.Host, a.events, ownership)))
			}
		}).Height(260).Label("本机 Agent 配置候选")
	})
}
