package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type agentScopeList struct {
	Mode    string   `json:"mode"`
	Entries []string `json:"entries"`
}

type agentScopePolicy struct {
	Capture agentScopeList `json:"capture"`
	Monitor agentScopeList `json:"monitor"`
}

type agentRecognitionRow struct {
	Comm     string
	Tag      string
	Label    string
	PID      int
	Events   int
	LastSeen int64
	Source   string
	Disabled bool
	CPU float64
	MemPercent float64
}

func agentScopeModeLabel(mode string) string {
	if mode == "whitelist" { return "白名单" }
	return "黑名单"
}

func agentScopeModeValue(label string) string {
	if label == "白名单" { return "whitelist" }
	return "blacklist"
}

func agentScopeNames(text string) []string {
	values := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n'
	})
	out := make([]string, 0, len(values))
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		name := strings.ToLower(strings.TrimSpace(value))
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func scopeMatches(list agentScopeList, comm, tag string) bool {
	found := false
	for _, entry := range list.Entries {
		if strings.EqualFold(entry, strings.TrimSpace(comm)) ||
			strings.EqualFold(entry, strings.TrimSpace(tag)) {
			found = true
			break
		}
	}
	if list.Mode == "whitelist" { return found }
	return !found
}

func agentRecognitionKey(comm string, pid int) string {
	name := strings.ToLower(strings.TrimSpace(comm))
	if pid <= 0 {
		return name + ":registered"
	}
	return name + ":" + strconv.Itoa(pid)
}

// Three independent sources make whitelist recovery possible even with no
// captured events: known live processes, recent event context and saved comms.
// Rows are keyed by (comm, PID) rather than collapsing concurrent sessions.
func aggregateAgentRecognition(events []eventSummary, registry registrySnapshot) []agentRecognitionRow {
	return aggregateAgentRecognitionWithProcesses(events, registry, nil)
}

func aggregateAgentRecognitionWithProcesses(events []eventSummary, registry registrySnapshot, processes []systemProcess) []agentRecognitionRow {
	tracked := make(map[string]trackedComm, len(registry.Comms))
	for _, comm := range registry.Comms {
		if name := strings.ToLower(strings.TrimSpace(comm.Comm)); name != "" {
			tracked[name] = comm
		}
	}
	byKey := make(map[string]*agentRecognitionRow)
	seen := make(map[string]bool)

	for _, p := range processes {
		comm := strings.TrimSpace(p.Name)
		name := strings.ToLower(comm)
		if name == "" || p.PID <= 0 { continue }
		saved, registered := tracked[name]
		if !registered && harnessLabelFor(comm) == "未识别" {
			continue // Never identify a generic process solely from a substring.
		}
		key := agentRecognitionKey(comm, p.PID)
		byKey[key] = &agentRecognitionRow{
			Comm: comm, Tag: saved.Tag, PID: p.PID,
			Label: harnessLabelFor(saved.Tag, comm), Source: "实时进程",
			Disabled: saved.Disabled, CPU: p.CPU, MemPercent: p.MemPercent,
		}
		seen[name] = true
	}

	for _, event := range events {
		comm := strings.TrimSpace(event.Comm)
		name := strings.ToLower(comm)
		if name == "" { continue }
		saved, registered := tracked[name]
		if !registered && !isAgentSummary(event) && harnessLabelFor(event.Tag, comm) == "未识别" {
			continue
		}
		key := agentRecognitionKey(comm, event.PID)
		row := byKey[key]
		if row == nil {
			row = &agentRecognitionRow{
				Comm: comm, PID: event.PID, Tag: saved.Tag,
				Source: "事件识别", Disabled: saved.Disabled,
			}
			byKey[key] = row
		}
		row.Events++
		if event.ReceivedAtMS >= row.LastSeen {
			row.LastSeen = event.ReceivedAtMS
			if strings.TrimSpace(event.Tag) != "" { row.Tag = event.Tag }
		}
		row.Label = harnessLabelFor(row.Tag, row.Comm)
		seen[name] = true
	}

	for name, saved := range tracked {
		if seen[name] { continue }
		key := agentRecognitionKey(saved.Comm, 0)
		byKey[key] = &agentRecognitionRow{
			Comm: saved.Comm, Tag: saved.Tag,
			Label: harnessLabelFor(saved.Tag, saved.Comm),
			Source: "跟踪注册表", Disabled: saved.Disabled,
		}
	}

	out := make([]agentRecognitionRow, 0, len(byKey))
	for _, row := range byKey { out = append(out, *row) }
	sort.Slice(out, func(i, j int) bool {
		if out[i].PID > 0 && out[j].PID == 0 { return true }
		if out[j].PID > 0 && out[i].PID == 0 { return false }
		if out[i].LastSeen != out[j].LastSeen { return out[i].LastSeen > out[j].LastSeen }
		if out[i].Comm != out[j].Comm { return out[i].Comm < out[j].Comm }
		return out[i].PID < out[j].PID
	})
	return out
}

func (a *renewApp) refreshAgentScopes(parent context.Context) {
	if a.client == nil || a.agentScopesBusy { return }
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	cfg, err := a.client.agentScopePolicy(ctx)
	a.update(func() {
		if err != nil {
			a.agentScopesReady = false
			a.agentScopesErr = err.Error()
			return
		}
		a.agentScopes = cfg
		a.captureScopeMode = agentScopeModeLabel(cfg.Capture.Mode)
		a.monitorScopeMode = agentScopeModeLabel(cfg.Monitor.Mode)
		a.captureScopeText = strings.Join(cfg.Capture.Entries, ", ")
		a.monitorScopeText = strings.Join(cfg.Monitor.Entries, ", ")
		a.agentScopesReady = true
		a.agentScopesErr = ""
	})
}

func (a *renewApp) saveAgentScope(kind string) {
	if !a.agentScopesReady || a.agentScopesBusy || a.client == nil ||
		!a.runtimeCfg.Runtime.PolicyManagementEnabled { return }
	// Only commit the chosen section; unsaved edits on the other section are
	// not silently applied. The server atomically persists both lists.
	next := a.agentScopes
	switch kind {
	case "capture":
		next.Capture = agentScopeList{
			Mode: agentScopeModeValue(a.captureScopeMode), Entries: agentScopeNames(a.captureScopeText),
		}
	case "monitor":
		next.Monitor = agentScopeList{
			Mode: agentScopeModeValue(a.monitorScopeMode), Entries: agentScopeNames(a.monitorScopeText),
		}
	default:
		return
	}
	a.agentScopesBusy = true
	a.agentScopesErr = ""
	a.agentScopesNotice = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		saved, err := a.client.putAgentScopePolicy(ctx, next)
		a.update(func() {
			a.agentScopesBusy = false
			if err != nil {
				a.agentScopesErr = err.Error()
				return
			}
			a.agentScopes = saved
			if kind == "capture" {
				a.captureScopeMode = agentScopeModeLabel(saved.Capture.Mode)
				a.captureScopeText = strings.Join(saved.Capture.Entries, ", ")
			} else {
				a.monitorScopeMode = agentScopeModeLabel(saved.Monitor.Mode)
				a.monitorScopeText = strings.Join(saved.Monitor.Entries, ", ")
			}
			a.agentScopesNotice = "名单已由后端确认并持久化"
		})
	}()
}

func appendScopeEntry(text, name string) string {
	name = strings.TrimSpace(name)
	if name == "" { return text }
	for _, entry := range agentScopeNames(text) {
		if strings.EqualFold(entry, name) { return text }
	}
	if strings.TrimSpace(text) == "" { return name }
	return strings.TrimSpace(text) + ", " + name
}

func (a *renewApp) agentScopeEditor(c *ui.Context, kind, heading, help string) {
	t := c.Theme()
	list := a.agentScopes.Capture
	mode := &a.captureScopeMode
	text := &a.captureScopeText
	if kind == "monitor" {
		list = a.agentScopes.Monitor
		mode = &a.monitorScopeMode
		text = &a.monitorScopeText
	}
	card(c, heading, func() {
		ui.Text(c, help).FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Select(c, mode, []string{"黑名单", "白名单"}).Label(heading+"模式").Width(145)
			if *mode == "白名单" && len(agentScopeNames(*text)) == 0 {
				statusPill(c, "空白名单：不包含任何 Agent", t.Warning)
			} else {
				statusPill(c, fmt.Sprintf("已生效 %d 项", len(list.Entries)), t.Accent)
			}
		})
		ui.TextInput(c, text).Placeholder("命令或 Agent 标签，多个值用逗号分隔，例如 codex, claude").Label(heading+"名单").Grow(1)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, "保存"+heading).Clicked() && !a.agentScopesBusy {
				a.saveAgentScope(kind)
			}
			if ui.Button(c, "放弃修改").Clicked() && !a.agentScopesBusy {
				*mode = agentScopeModeLabel(list.Mode)
				*text = strings.Join(list.Entries, ", ")
			}
			ui.Text(c, "仅完整匹配 comm 或标签，不支持通配符。").FontSize(10).TextColor(t.TextMuted)
		})
	})
}

func (a *renewApp) agentRecognitionView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Agent 识别与范围").FontSize(28).Bold()
	ui.Text(c, "识别运行中的 Agent 命令和标签；独立控制事件捕获与后续分析监视。名单不会阻止 Agent 执行，也不卸载内核探针。").TextColor(t.TextMuted)

	if a.agentScopesErr != "" { ui.Text(c, a.agentScopesErr).TextColor(t.Danger) }
	if a.agentScopesNotice != "" { statusPill(c, a.agentScopesNotice, t.Success) }
	if !a.runtimeCfg.Runtime.PolicyManagementEnabled {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			statusPill(c, "只读", t.Warning)
			ui.Text(c, "需要先开启「监控 → 策略管理」才能修改 Agent 范围。").TextColor(t.TextMuted)
			if ui.Button(c, "前往监控").Clicked() { a.page = "监控" }
		})
	}
	if !a.agentScopesReady {
		ui.Text(c, "名单未加载；保存已禁用。").TextColor(t.Warning)
		if ui.Button(c, "重新读取名单").Clicked() { go a.refreshAgentScopes(context.Background()) }
		return
	}
	ui.Row(c).Gap(12).Wrap().Children(func() {
		statCard(c, "捕获策略", agentScopeModeLabel(a.agentScopes.Capture.Mode), fmt.Sprintf("%d 条匹配规则", len(a.agentScopes.Capture.Entries)))
		statCard(c, "监视策略", agentScopeModeLabel(a.agentScopes.Monitor.Mode), fmt.Sprintf("%d 条匹配规则", len(a.agentScopes.Monitor.Entries)))
	})
	ui.Text(c, "捕获：控制事件是否进入用户态归档、持久化及事件流；监视：控制语义告警、循环/信号/研究分析。捕获先于监视，未捕获的事件无法监视。").FontSize(12).TextColor(t.TextMuted)
	a.agentScopeEditor(c, "capture", "捕获范围", "黑名单排除匹配对象，白名单只捕获匹配对象；空白名单停止接纳事件。")
	a.agentScopeEditor(c, "monitor", "监视范围", "黑名单跳过匹配对象的高级分析，白名单只分析匹配对象；即使不监视，已捕获事件仍可浏览。")
	if a.agentScopesBusy {
		ui.Row(c).Gap(8).Children(func() {
			ui.Spinner(c)
			ui.Text(c, "等待后端确认名单…").TextColor(t.TextMuted)
		})
	}
	card(c, "Agent 识别 · 实时进程、近期事件与跟踪登记", func() {
		ui.SearchField(c, &a.agentSearch).Label("搜索 Agent、标签或 PID")
		rows := aggregateAgentRecognitionWithProcesses(a.events, a.registry, a.system.Processes)
		if query := strings.ToLower(strings.TrimSpace(a.agentSearch)); query != "" {
			filtered := make([]agentRecognitionRow, 0, len(rows))
			for _, row := range rows {
				if strings.Contains(strings.ToLower(row.Comm+" "+row.Tag+" "+row.Label+" "+strconv.Itoa(row.PID)), query) {
					filtered = append(filtered, row)
				}
			}
			rows = filtered
		}
		if len(rows) == 0 {
			ui.Text(c, "当前没有已识别的 Agent；可先在「跟踪」登记命令，或等待 Agent 事件。").TextColor(t.TextMuted)
			if ui.Button(c, "打开跟踪").Clicked() { a.page = "跟踪" }
			return
		}
		cols := []ui.TableColumn{
			{Title: "Agent", MinWidth: 170, Fixed: true},
			{Title: "命令 / 标签", MinWidth: 175},
			{Title: "PID", Width: 75},
			{Title: "事件", Width: 66, Align: ui.End},
			{Title: "捕获", Width: 80},
			{Title: "监视", Width: 80},
			{Title: "来源", Width: 98},
		}
		a.agentTable.Key = func(index int) any { return agentRecognitionKey(rows[index].Comm, rows[index].PID) }
		ui.Table(c, &a.agentTable, cols, len(rows), func(index, column int) {
			row := rows[index]
			switch column {
			case 0:
				visible := row.Label
				if visible == "未识别" { visible = row.Comm }
				harnessIdentity(c, visible, row.Tag, row.Comm)
			case 1:
				ui.Text(c, row.Comm+" · "+displayOr(row.Tag, "-")).SingleLine()
			case 2:
				if row.PID > 0 { ui.Text(c, strconv.Itoa(row.PID)) } else { ui.Text(c, "-") }
			case 3:
				ui.Text(c, strconv.Itoa(row.Events))
			case 4:
				if scopeMatches(a.agentScopes.Capture, row.Comm, row.Tag) {
					statusPill(c, "捕获", t.Success)
				} else { statusPill(c, "排除", t.TextMuted) }
			case 5:
				if !scopeMatches(a.agentScopes.Capture, row.Comm, row.Tag) {
					statusPill(c, "未捕获", t.TextMuted)
				} else if scopeMatches(a.agentScopes.Monitor, row.Comm, row.Tag) {
					statusPill(c, "监视", t.Success)
				} else { statusPill(c, "跳过", t.TextMuted) }
			case 6:
				ui.Text(c, row.Source).FontSize(10).TextColor(t.TextMuted).SingleLine()
			}
		}).Height(380).Label("Agent 识别列表")
		if a.agentSelected >= 0 && a.agentSelected < len(rows) {
			selected := rows[a.agentSelected]
			ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Text(c, selected.Comm+" · "+selected.Label).Bold()
				if selected.PID > 0 {
					ui.Textf(c, "PID %d", selected.PID).Font("monospace")
				}
				if selected.Source == "实时进程" {
					ui.Textf(c, "CPU %.1f%% · 内存 %.1f%%", selected.CPU, selected.MemPercent).TextColor(t.TextMuted)
				}
				if selected.Disabled { statusPill(c, "已在跟踪注册表禁用", t.Warning) }
				if selected.LastSeen > 0 {
					ui.Text(c, "最近事件 "+summaryTime(selected.LastSeen)).TextColor(t.TextMuted)
				}
				if ui.Button(c, "填入捕获名单").Clicked() {
					a.captureScopeText = appendScopeEntry(a.captureScopeText, selected.Comm)
				}
				if ui.Button(c, "填入监视名单").Clicked() {
					a.monitorScopeText = appendScopeEntry(a.monitorScopeText, selected.Comm)
				}
				if selected.PID > 0 && ui.Button(c, "查看事件").Clicked() {
					a.clearEventFilters()
					a.eventPIDFilter = selected.PID
					a.page = "事件"
				}
			})
		}
		ui.Text(c, "识别结果来自实时系统进程快照、最多 1200 条事件摘要及已登记命令；仅展示可识别或已登记的 Agent，非全部进程。名单更改不追溯删除历史。").FontSize(10).TextColor(t.TextMuted)
	})
	if ui.Button(c, "重新读取识别与范围").Clicked() && !a.agentScopesBusy {
		go a.refreshAgentScopes(context.Background())
		go a.refreshRegistry(context.Background())
	}
}
