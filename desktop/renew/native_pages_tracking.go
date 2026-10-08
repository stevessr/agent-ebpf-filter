package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *renewApp) trackingView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "跟踪对象").FontSize(28).Bold()
	ui.Text(c, "决定哪些 Agent 命令和工作目录进入 eBPF 观测范围；标签用于把事件归到可读的 Agent 上下文。").TextColor(t.TextMuted)

	if a.registryErr != "" {
		ui.Text(c, a.registryErr).TextColor(t.TextMuted)
	}
	if !a.runtimeCfg.Runtime.PolicyManagementEnabled {
		ui.Column(c).Padding(16).Gap(10).Radius(12).Background(t.Warning.Alpha(0.055)).Border(1, t.Warning.Alpha(0.38)).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				statusPill(c, "只读", t.Warning)
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Text(c, "策略管理当前关闭").Bold()
					ui.Text(c, "现有跟踪范围仍可查看；新增、禁用和删除需要显式开启高权限策略管理。").TextColor(t.TextMuted)
				})
				if ui.Button(c, "前往采集与能力").Clicked() {
					a.page = "监控"
				}
			})
		})
	}

	card(c, fmt.Sprintf("标签 · %d", len(a.registry.Tags)), func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.TextInput(c, &a.newTag).Placeholder("例如 AI Agent").Label("新标签").Width(260)
			if ui.PrimaryButton(c, "创建标签").Clicked() && a.canMutateRegistry() {
				name := strings.TrimSpace(a.newTag)
				if name != "" {
					a.runRegistryMutation(func(ctx context.Context) error {
						return a.client.addTag(ctx, name)
					}, func() {
						a.newTag = ""
					})
				}
			}
		})
		if len(a.registry.Tags) > 0 {
			ui.Row(c).Gap(6).Wrap().Children(func() {
				for _, tag := range a.registry.Tags {
					ui.Badge(c, tag)
				}
			})
		} else {
			ui.Text(c, "暂无标签；先创建一个标签再添加跟踪对象。").FontSize(11).TextColor(t.TextMuted)
		}
	})

	card(c, "跟踪注册表 · "+registryCounts(a.registry), func() {
		ui.Tabs(c, &a.registryTab, "命令", "精确路径", "路径前缀")
		if len(a.registry.Tags) > 0 {
			if a.trackTag == "" || !containsString(a.registry.Tags, a.trackTag) {
				a.trackTag = a.registry.Tags[0]
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				placeholder := "命令名，例如 codex"
				switch a.registryTab {
				case 1:
					placeholder = "精确路径，例如 /home/me/project"
				case 2:
					placeholder = "路径前缀，例如 /home/me/project/"
				}
				ui.TextInput(c, &a.trackName).Placeholder(placeholder).Label("跟踪对象").Grow(1)
				ui.Select(c, &a.trackTag, a.registry.Tags).Label("标签").Width(180)
				if ui.PrimaryButton(c, "添加").Clicked() && a.canMutateRegistry() {
					a.addTrackedObject()
				}
			})
		}
		switch a.registryTab {
		case 1:
			a.trackedPathsTable(c)
		case 2:
			a.trackedPrefixesTable(c)
		default:
			a.trackedCommsTable(c)
		}
	})

	if a.registryBusy {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c)
			ui.Text(c, "等待后端确认…").TextColor(t.TextMuted)
		})
	}
	if ui.Button(c, "重新读取跟踪配置").Clicked() && !a.registryBusy {
		go a.refreshRegistry(context.Background())
	}
}

func (a *renewApp) canMutateRegistry() bool {
	return a.client != nil &&
		a.registryReady &&
		!a.registryBusy &&
		a.runtimeCfg.Runtime.PolicyManagementEnabled
}

func (a *renewApp) addTrackedObject() {
	value := strings.TrimSpace(a.trackName)
	tag := strings.TrimSpace(a.trackTag)
	if value == "" || tag == "" {
		return
	}
	tab := a.registryTab
	a.runRegistryMutation(func(ctx context.Context) error {
		switch tab {
		case 1:
			return a.client.addPath(ctx, value, tag)
		case 2:
			return a.client.addPrefix(ctx, value, tag)
		default:
			return a.client.addComm(ctx, value, tag)
		}
	}, func() {
		a.trackName = ""
	})
}

func (a *renewApp) trackedCommsTable(c *ui.Context) {
	t := c.Theme()
	rows := a.registry.Comms
	if len(rows) == 0 {
		ui.Text(c, "暂无跟踪命令").TextColor(t.TextMuted)
		return
	}
	a.commTable.Key = func(row int) any { return rows[row].Comm }
	cols := []ui.TableColumn{
		{Title: "命令", MinWidth: 220, Fixed: true},
		{Title: "标签", Width: 180},
		{Title: "状态", Width: 100},
	}
	ui.Table(c, &a.commTable, cols, len(rows), func(row, col int) {
		item := rows[row]
		switch col {
		case 0:
			ui.Text(c, item.Comm).Font("monospace").SingleLine()
		case 1:
			ui.Text(c, displayOr(item.Tag, "-")).SingleLine()
		case 2:
			if item.Disabled {
				statusPill(c, "已禁用", t.Warning)
			} else {
				statusPill(c, "跟踪中", t.Success)
			}
		}
	}).Height(300).Label("跟踪命令")
	if a.commSelected >= 0 && a.commSelected < len(rows) {
		item := rows[a.commSelected]
		ui.Row(c).Gap(8).Children(func() {
			label := "禁用所选"
			if item.Disabled {
				label = "重新启用"
			}
			if ui.Button(c, label).Clicked() && a.canMutateRegistry() {
				disabled := !item.Disabled
				a.runRegistryMutation(func(ctx context.Context) error {
					return a.client.setCommDisabled(ctx, item.Comm, disabled)
				}, nil)
			}
			if ui.Button(c, "删除所选").Clicked() && a.canMutateRegistry() {
				a.runRegistryMutation(func(ctx context.Context) error {
					return a.client.deleteComm(ctx, item.Comm)
				}, func() { a.commSelected = -1 })
			}
		})
	}
}

func (a *renewApp) trackedPathsTable(c *ui.Context) {
	t := c.Theme()
	rows := a.registry.Paths
	if len(rows) == 0 {
		ui.Text(c, "暂无精确路径").TextColor(t.TextMuted)
		return
	}
	a.pathTable.Key = func(row int) any { return rows[row].Path }
	cols := []ui.TableColumn{
		{Title: "精确路径", MinWidth: 340, Fixed: true},
		{Title: "标签", Width: 180},
	}
	ui.Table(c, &a.pathTable, cols, len(rows), func(row, col int) {
		item := rows[row]
		if col == 0 {
			ui.Text(c, item.Path).Font("monospace").SingleLine()
		} else {
			ui.Text(c, displayOr(item.Tag, "-")).SingleLine()
		}
	}).Height(300).Label("精确跟踪路径")
	if a.pathSelected >= 0 && a.pathSelected < len(rows) {
		item := rows[a.pathSelected]
		if ui.Button(c, "删除所选路径").Clicked() && a.canMutateRegistry() {
			a.runRegistryMutation(func(ctx context.Context) error {
				return a.client.deletePath(ctx, item.Path)
			}, func() { a.pathSelected = -1 })
		}
	}
}

func (a *renewApp) trackedPrefixesTable(c *ui.Context) {
	t := c.Theme()
	rows := a.registry.Prefixes
	if len(rows) == 0 {
		ui.Text(c, "暂无路径前缀").TextColor(t.TextMuted)
		return
	}
	a.prefixTable.Key = func(row int) any { return rows[row].Prefix }
	cols := []ui.TableColumn{
		{Title: "路径前缀", MinWidth: 340, Fixed: true},
		{Title: "标签", Width: 180},
	}
	ui.Table(c, &a.prefixTable, cols, len(rows), func(row, col int) {
		item := rows[row]
		if col == 0 {
			ui.Text(c, item.Prefix).Font("monospace").SingleLine()
		} else {
			ui.Text(c, displayOr(item.Tag, "-")).SingleLine()
		}
	}).Height(300).Label("路径前缀")
	if a.prefixSelected >= 0 && a.prefixSelected < len(rows) {
		item := rows[a.prefixSelected]
		if ui.Button(c, "删除所选前缀").Clicked() && a.canMutateRegistry() {
			a.runRegistryMutation(func(ctx context.Context) error {
				return a.client.deletePrefix(ctx, item.Prefix)
			}, func() { a.prefixSelected = -1 })
		}
	}
}

func (a *renewApp) runRegistryMutation(action func(context.Context) error, onSuccess func()) {
	if !a.canMutateRegistry() {
		return
	}
	a.registryBusy = true
	a.registryErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		err := action(ctx)
		if err != nil {
			a.update(func() {
				a.registryBusy = false
				a.registryErr = err.Error()
			})
			return
		}
		registry, loadErr := a.client.registry(ctx)
		a.update(func() {
			a.registryBusy = false
			if loadErr != nil {
				a.registryErr = loadErr.Error()
				return
			}
			a.registry = registry
			a.registryReady = true
			a.registryErr = ""
			if onSuccess != nil {
				onSuccess()
			}
		})
	}()
}

func (a *renewApp) refreshRegistry(parent context.Context) {
	if a.client == nil || a.registryBusy {
		return
	}
	a.update(func() {
		a.registryBusy = true
		a.registryErr = ""
	})
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	registry, err := a.client.registry(ctx)
	a.update(func() {
		a.registryBusy = false
		if err != nil {
			a.registryReady = false
			a.registryErr = err.Error()
			return
		}
		a.registry = registry
		a.registryReady = true
		if a.trackTag == "" && len(registry.Tags) > 0 {
			a.trackTag = registry.Tags[0]
		}
	})
}

func registryCounts(registry registrySnapshot) string {
	return fmt.Sprintf("%d 命令 · %d 精确路径 · %d 前缀", len(registry.Comms), len(registry.Paths), len(registry.Prefixes))
}
