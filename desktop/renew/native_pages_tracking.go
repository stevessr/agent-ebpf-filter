package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *renewApp) trackingView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "跟踪对象").FontSize(28).Bold()
	ui.Text(c, "决定哪些 Agent 命令和工作目录进入 eBPF 观测范围；标签用于把事件归到可读的 Agent 上下文。").TextColor(t.TextMuted)
	if ui.Button(c, "配置敏感文件读写权限").Clicked() { a.page = "路径权限" }

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
		ui.Tabs(c, &a.registryTab, "命令", "文件夹内文件", "文件", "文件夹及其子文件")
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			tags := append([]string{"全部标签"}, a.registry.Tags...)
			if a.registryFilterTag == "" { a.registryFilterTag = "全部标签" }
			if a.registryFilterStatus == "" { a.registryFilterStatus = "全部状态" }
			ui.Select(c, &a.registryFilterTag, tags).Label("按标签筛选").Width(180)
			ui.Select(c, &a.registryFilterStatus, []string{"全部状态", "跟踪中", "已禁用"}).Label("按状态筛选").Width(150)
		})
		// A changed filter must never leave a stale selected index pointing
		// at another rule that can be deleted or disabled by mistake.
		if a.registryLastFilterTag != a.registryFilterTag ||
			a.registryLastFilterStatus != a.registryFilterStatus {
			a.commSelected, a.pathSelected, a.prefixSelected = -1, -1, -1
			a.registryLastFilterTag = a.registryFilterTag
			a.registryLastFilterStatus = a.registryFilterStatus
		}
		if a.registryTab == 1 {
			ui.Text(c, "文件夹内文件：批量登记当前直接子文件，不包含子文件夹；新建文件需要重新登记。").FontSize(11).TextColor(t.TextMuted)
		}
		if a.registryTab == 3 {
			ui.Text(c, "文件夹及其子文件：按路径前缀递归跟踪，包括未来新增的文件和子目录。").FontSize(11).TextColor(t.TextMuted)
		}
		if len(a.registry.Tags) > 0 {
			if a.trackTag == "" || !containsString(a.registry.Tags, a.trackTag) {
				a.trackTag = a.registry.Tags[0]
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				placeholder := "命令名，例如 codex"
				switch a.registryTab {
				case 1:
					placeholder = "文件夹路径，例如 /home/me/project"
				case 2:
					placeholder = "文件绝对路径，例如 /home/me/project/a.txt"
				case 3:
					placeholder = "递归目录，例如 /home/me/project"
				}
				ui.TextInput(c, &a.trackName).Placeholder(placeholder).Label("跟踪对象").Grow(1)
				if a.registryTab != 0 {
					pick := "选择文件夹…"
					if a.registryTab == 2 { pick = "选择文件…" }
					if ui.Button(c, pick).Clicked() { a.pickTrackingPath() }
				}
				ui.Select(c, &a.trackTag, a.registry.Tags).Label("标签").Width(180)
				if ui.PrimaryButton(c, "添加").Clicked() && a.canMutateRegistry() {
					a.addTrackedObject()
				}
			})
		}
		switch a.registryTab {
		case 1, 2:
			a.trackedPathsTable(c)
		case 3:
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

// pickTrackingPath opens the OS-native file/folder chooser outside the UI
// render callback. Selection never mutates privileged tracking rules.
func (a *renewApp) pickTrackingPath() {
	tab := a.registryTab
	if tab == 0 { return }
	go func() {
		directory := tab != 2
		title := "选择要跟踪的文件"
		if directory { title = "选择要跟踪的文件夹" }
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: a.win, Title: title, Directory: directory,
			DefaultPath: a.trackName,
			ShowHiddenFiles: true,
		})
		a.update(func() {
			if err != nil { a.registryErr = err.Error(); return }
			if len(paths) > 0 { a.trackName = paths[0]; a.registryErr = "" }
		})
	}()
}

// listDirectFiles snapshots only regular immediate children. This maps the
// non-recursive UI mode to the existing exact-path backend contract; future
// files must be explicitly registered again.
func listDirectFiles(dir string) ([]string, error) {
	if !filepath.IsAbs(dir) { return nil, fmt.Errorf("请选择绝对文件夹路径") }
	items, err := os.ReadDir(dir)
	if err != nil { return nil, err }
	const maxFiles = 512
	paths := make([]string, 0, min(len(items), maxFiles))
	for _, item := range items {
		if !item.Type().IsRegular() { continue }
		paths = append(paths, filepath.Join(dir, item.Name()))
		if len(paths) > maxFiles {
			return nil, fmt.Errorf("文件夹内文件超过 %d 个，请缩小范围", maxFiles)
		}
	}
	if len(paths) == 0 { return nil, fmt.Errorf("文件夹中没有可登记的普通文件") }
	return paths, nil
}

func (a *renewApp) addTrackedObject() {
	value := strings.TrimSpace(a.trackName)
	tag := strings.TrimSpace(a.trackTag)
	if value == "" || tag == "" { return }
	tab := a.registryTab
	a.runRegistryMutation(func(ctx context.Context) error {
		if tab > 0 && !filepath.IsAbs(value) {
			return fmt.Errorf("文件或目录必须使用绝对路径")
		}
		switch tab {
		case 1:
			files, err := listDirectFiles(value)
			if err != nil { return err }
			for _, path := range files {
				if err := a.client.addPath(ctx, path, tag); err != nil {
					return fmt.Errorf("登记 %s 失败：%w（之前已登记的条目保留）", path, err)
				}
			}
			return nil
		case 2:
			info, err := os.Stat(value)
			if err != nil { return err }
			if !info.Mode().IsRegular() { return fmt.Errorf("请选择普通文件，而不是文件夹") }
			return a.client.addPath(ctx, filepath.Clean(value), tag)
		case 3:
			info, err := os.Stat(value)
			if err != nil { return err }
			if !info.IsDir() { return fmt.Errorf("请选择文件夹以递归跟踪") }
			return a.client.addPrefix(ctx, filepath.Clean(value), tag)
		default:
			return a.client.addComm(ctx, value, tag)
		}
	}, func() { a.trackName = "" })
}

func filterTrackedComms(items []trackedComm, tag, status string) []trackedComm {
	out := make([]trackedComm, 0, len(items))
	for _, item := range items {
		if tag != "" && tag != "全部标签" && item.Tag != tag { continue }
		if status == "跟踪中" && item.Disabled { continue }
		if status == "已禁用" && !item.Disabled { continue }
		out = append(out, item)
	}
	return out
}

func filterTrackedPaths(items []trackedPath, tag, status string) []trackedPath {
	out := make([]trackedPath, 0, len(items))
	if status == "已禁用" { return out } // only command rules support disable
	for _, item := range items {
		if tag == "" || tag == "全部标签" || item.Tag == tag { out = append(out, item) }
	}
	return out
}

func filterTrackedPrefixes(items []trackedPrefix, tag, status string) []trackedPrefix {
	out := make([]trackedPrefix, 0, len(items))
	if status == "已禁用" { return out }
	for _, item := range items {
		if tag == "" || tag == "全部标签" || item.Tag == tag { out = append(out, item) }
	}
	return out
}

func (a *renewApp) trackedCommsTable(c *ui.Context) {
	t := c.Theme()
	rows := filterTrackedComms(a.registry.Comms, a.registryFilterTag, a.registryFilterStatus)
	if len(rows) == 0 {
		ui.Text(c, "当前筛选条件下没有命令").TextColor(t.TextMuted)
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
	rows := filterTrackedPaths(a.registry.Paths, a.registryFilterTag, a.registryFilterStatus)
	if len(rows) == 0 {
		ui.Text(c, "当前筛选条件下没有文件规则").TextColor(t.TextMuted)
		return
	}
	a.pathTable.Key = func(row int) any { return rows[row].Path }
	cols := []ui.TableColumn{
		{Title: "文件路径", MinWidth: 340, Fixed: true},
		{Title: "标签", Width: 180},
		{Title: "状态", Width: 96},
	}
	ui.Table(c, &a.pathTable, cols, len(rows), func(row, col int) {
		item := rows[row]
		switch col {
		case 0:
			ui.Text(c, item.Path).Font("monospace").SingleLine()
		case 1:
			ui.Text(c, displayOr(item.Tag, "-")).SingleLine()
		case 2:
			statusPill(c, "跟踪中", t.Success)
		}
	}).Height(300).Label("文件跟踪")
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
	rows := filterTrackedPrefixes(a.registry.Prefixes, a.registryFilterTag, a.registryFilterStatus)
	if len(rows) == 0 {
		ui.Text(c, "当前筛选条件下没有递归目录规则").TextColor(t.TextMuted)
		return
	}
	a.prefixTable.Key = func(row int) any { return rows[row].Prefix }
	cols := []ui.TableColumn{
		{Title: "文件夹（含子文件）", MinWidth: 340, Fixed: true},
		{Title: "标签", Width: 180},
		{Title: "状态", Width: 96},
	}
	ui.Table(c, &a.prefixTable, cols, len(rows), func(row, col int) {
		item := rows[row]
		switch col {
		case 0:
			ui.Text(c, item.Prefix).Font("monospace").SingleLine()
		case 1:
			ui.Text(c, displayOr(item.Tag, "-")).SingleLine()
		case 2:
			statusPill(c, "跟踪中", t.Success)
		}
	}).Height(300).Label("递归文件夹跟踪")
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
	return fmt.Sprintf("%d 命令 · %d 文件 · %d 递归目录", len(registry.Comms), len(registry.Paths), len(registry.Prefixes))
}
