package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

type monitoringModule struct {
	Key         string
	Title       string
	Description string
	Cost        string
	EventTypes  []int
}

var monitoringModules = []monitoringModule{
	{Key: "process", Title: "进程行为", Description: "程序启动、派生、退出与生命周期，适合长期常开。", Cost: "低", EventTypes: []int{0, 18, 19, 26, 27, 28, 29}},
	{Key: "file-changes", Title: "文件改动", Description: "创建、写入、删除、改名、权限与链接变化。", Cost: "低", EventTypes: []int{3, 4, 10, 12, 13, 14, 15, 16, 17}},
	{Key: "network", Title: "基础联网", Description: "连接、监听、TCP 生命周期与 DNS。", Cost: "低", EventTypes: []int{2, 6, 31, 32, 34}},
	{Key: "file-access", Title: "文件访问", Description: "open/read/ioctl 等高频访问，适合短时排查。", Cost: "高", EventTypes: []int{1, 5, 9, 11}},
	{Key: "network-detail", Title: "网络细节", Description: "send/recv/socket/accept 与 TCP 状态细节。", Cost: "高", EventTypes: []int{7, 8, 20, 21, 22, 33}},
	{Key: "deep-syscall", Title: "深度系统调用", Description: "通用 syscall 轨迹，仅建议专业诊断时开启。", Cost: "高", EventTypes: []int{25}},
}

var monitoringProfiles = map[string][]string{
	"轻量": {"process", "file-changes"},
	"日常": {"process", "file-changes", "network"},
	"深度": {"process", "file-changes", "network", "file-access", "network-detail", "deep-syscall"},
}

func (a *renewApp) monitoringView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "监控中心").FontSize(28).Bold()
	ui.Text(c, "直接修改后端 disabledEventTypes；开关只在后端确认成功后才改变。TLS、研究处理等独立运行时能力不会被这些内核预设偷偷打开。").TextColor(t.TextMuted)

	if a.configErr != "" {
		ui.Text(c, a.configErr).TextColor(t.TextMuted)
	}
	card(c, "采集预设", func() {
		ui.Text(c, "预设只组合下面六组内核事件。").FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for _, name := range []string{"轻量", "日常", "深度"} {
				name := name
				if ui.Button(c, name).Clicked() && a.configReady && !a.configBusy {
					a.applyMonitoringProfile(name)
				}
			}
			if ui.Button(c, "重新读取").Clicked() && !a.configBusy {
				go a.refreshEventTypeConfig(context.Background())
			}
		})
	})

	for _, module := range monitoringModules {
		module := module
		card(c, module.Title, func() {
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					ui.Text(c, module.Description).TextColor(t.TextMuted)
					ui.Text(c, "开销："+module.Cost+" · "+eventTypeSummary(module.EventTypes)).FontSize(11).TextColor(t.TextMuted)
				})
				enabled := a.monitoringModuleEnabled(module)
				next := enabled
				if ui.Switch(c, &next).Label(module.Title).Changed() && a.configReady && !a.configBusy {
					a.setMonitoringModule(module, next)
				}
			})
		})
	}
	if !a.configReady {
		ui.Text(c, "配置尚未成功加载，写操作已禁用。").FontSize(12).TextColor(t.TextMuted)
	} else if a.configBusy {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Spinner(c)
			ui.Text(c, "等待后端确认…").TextColor(t.TextMuted)
		})
	}
}

func (a *renewApp) rulesView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Wrapper 规则").FontSize(28).Bold()
	ui.Text(c, "规则是后端共享配置，按命令名匹配；仅经 agent-wrapper 执行的命令受此策略约束。").TextColor(t.TextMuted)

	if a.rulesErr != "" {
		ui.Text(c, a.rulesErr).TextColor(t.TextMuted)
	}

	title := "添加规则"
	if a.editingRule != "" {
		title = "编辑规则"
	}
	card(c, title, func() {
		ui.Form(c, func() {
			ui.Field(c, "命令名", func() {
				ui.TextInput(c, &a.ruleComm).Placeholder("例如 curl").Label("命令名")
			})
			ui.Field(c, "动作", func() {
				ui.Select(c, &a.ruleAction, []string{"ALLOW", "BLOCK", "ALERT", "REWRITE"}).Label("动作")
			})
			ui.Field(c, "优先级", func() {
				ui.TextInput(c, &a.rulePriority).Placeholder("0").Label("优先级")
			})
			ui.Field(c, "正则（可选）", func() {
				ui.TextInput(c, &a.ruleRegex).Placeholder("匹配命令参数").Label("正则")
			})
			if a.ruleAction == "REWRITE" && strings.TrimSpace(a.ruleRegex) != "" {
				ui.Field(c, "替换", func() {
					ui.TextInput(c, &a.ruleReplace).Placeholder("替换文本").Label("替换")
				})
			}
			if a.ruleAction == "REWRITE" && strings.TrimSpace(a.ruleRegex) == "" {
				ui.Field(c, "重写命令", func() {
					ui.TextInput(c, &a.ruleRewrite).Placeholder("[\"echo\",\"hello\"]").Label("JSON 字符串数组")
				})
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			if ui.PrimaryButton(c, "保存规则").Clicked() && a.rulesReady && !a.rulesBusy {
				a.saveRule()
			}
			if a.editingRule != "" && ui.Button(c, "取消编辑").Clicked() && !a.rulesBusy {
				a.resetRuleForm()
			}
			if ui.Button(c, "重新读取").Clicked() && !a.rulesBusy {
				go a.refreshRules(context.Background())
			}
		})
	})

	rows := a.filteredRules()
	card(c, fmt.Sprintf("已配置规则 · %d", len(rows)), func() {
		ui.SearchField(c, &a.ruleSearch).Label("搜索规则").Width(320)
		if len(rows) == 0 {
			ui.Text(c, "还没有匹配的 agent-wrapper 规则").TextColor(t.TextMuted)
			return
		}
		a.rulesTable.Key = func(row int) any { return rows[row].Comm }
		cols := []ui.TableColumn{
			{Title: "命令", Width: 150, Fixed: true},
			{Title: "动作", Width: 100},
			{Title: "优先级", Width: 80, Align: ui.End},
			{Title: "匹配 / 重写", MinWidth: 280},
		}
		table := ui.Table(c, &a.rulesTable, cols, len(rows), func(row, col int) {
			r := rows[row]
			switch col {
			case 0:
				ui.Text(c, r.Comm).SingleLine()
			case 1:
				ui.Text(c, r.Action).SingleLine()
			case 2:
				ui.Text(c, strconv.Itoa(r.Priority))
			case 3:
				ui.Text(c, ruleDescription(r)).SingleLine()
			}
		}).Height(320).Label("Wrapper 规则")
		if table.Submitted() && a.ruleSelected >= 0 && a.ruleSelected < len(rows) {
			a.editRule(rows[a.ruleSelected])
		}
		if a.ruleSelected >= 0 && a.ruleSelected < len(rows) {
			selected := rows[a.ruleSelected]
			ui.Row(c).Gap(8).Children(func() {
				if ui.Button(c, "编辑所选").Clicked() && !a.rulesBusy {
					a.editRule(selected)
				}
				if ui.Button(c, "删除所选").Clicked() && !a.rulesBusy {
					a.pendingDelete = selected.Comm
				}
			})
		}
	})

	if a.pendingDelete != "" {
		card(c, "确认删除", func() {
			ui.Text(c, "确定删除 "+a.pendingDelete+" 的规则？此操作影响所有 Agent 工具的对应命令。")
			ui.Row(c).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "确认删除").Clicked() && !a.rulesBusy {
					a.deleteRule(a.pendingDelete)
				}
				if ui.Button(c, "取消").Clicked() && !a.rulesBusy {
					a.pendingDelete = ""
				}
			})
		})
	}
}

func (a *renewApp) monitoringModuleEnabled(module monitoringModule) bool {
	if !a.configReady {
		return false
	}
	for _, eventType := range module.EventTypes {
		if a.disabledEventTypes[eventType] {
			return false
		}
	}
	return true
}

func (a *renewApp) setMonitoringModule(module monitoringModule, enabled bool) {
	next := cloneDisabled(a.disabledEventTypes)
	for _, eventType := range module.EventTypes {
		if enabled {
			delete(next, eventType)
		} else {
			next[eventType] = true
		}
	}
	a.writeEventTypes(next)
}

func (a *renewApp) applyMonitoringProfile(name string) {
	keys := monitoringProfiles[name]
	enabled := make(map[string]bool, len(keys))
	for _, key := range keys {
		enabled[key] = true
	}
	next := cloneDisabled(a.disabledEventTypes)
	for _, module := range monitoringModules {
		for _, eventType := range module.EventTypes {
			if enabled[module.Key] {
				delete(next, eventType)
			} else {
				next[eventType] = true
			}
		}
	}
	a.writeEventTypes(next)
}

func (a *renewApp) writeEventTypes(next map[int]bool) {
	if a.client == nil || a.configBusy {
		return
	}
	a.configBusy = true
	a.configErr = ""
	values := disabledSlice(next)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cfg, err := a.client.putEventTypeConfig(ctx, values)
		a.update(func() {
			a.configBusy = false
			if err != nil {
				a.configErr = err.Error()
				return
			}
			a.configReady = true
			a.disabledEventTypes = disabledSet(cfg.DisabledEventTypes)
		})
	}()
}

func (a *renewApp) refreshEventTypeConfig(parent context.Context) {
	if a.client == nil {
		return
	}
	a.update(func() {
		a.configBusy = true
		a.configErr = ""
	})
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	cfg, err := a.client.eventTypeConfig(ctx)
	a.update(func() {
		a.configBusy = false
		if err != nil {
			a.configErr = err.Error()
			return
		}
		a.configReady = true
		a.disabledEventTypes = disabledSet(cfg.DisabledEventTypes)
	})
}

func (a *renewApp) refreshRules(parent context.Context) {
	if a.client == nil {
		return
	}
	a.update(func() {
		a.rulesBusy = true
		a.rulesErr = ""
	})
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	rules, err := a.client.rules(ctx)
	a.update(func() {
		a.rulesBusy = false
		if err != nil {
			a.rulesErr = err.Error()
			return
		}
		a.rulesReady = true
		a.rules = rules
		if a.ruleSelected >= len(a.filteredRules()) {
			a.ruleSelected = -1
		}
	})
}

func (a *renewApp) saveRule() {
	comm := strings.TrimSpace(a.ruleComm)
	if comm == "" {
		a.rulesErr = "命令名不能为空"
		return
	}
	priority, err := strconv.Atoi(strings.TrimSpace(a.rulePriority))
	if err != nil {
		a.rulesErr = "优先级必须是整数"
		return
	}
	rule := wrapperRule{
		Comm:        comm,
		Action:      strings.ToUpper(strings.TrimSpace(a.ruleAction)),
		Regex:       a.ruleRegex,
		Replacement: a.ruleReplace,
		Priority:    priority,
	}
	if rule.Action == "REWRITE" && strings.TrimSpace(rule.Regex) == "" {
		if err := json.Unmarshal([]byte(a.ruleRewrite), &rule.RewrittenCmd); err != nil || len(rule.RewrittenCmd) == 0 {
			a.rulesErr = "重写命令需要非空 JSON 字符串数组，例如 [\"echo\",\"hello\"]"
			return
		}
	}
	oldComm := a.editingRule
	a.rulesBusy = true
	a.rulesErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.client.saveRule(ctx, rule); err != nil {
			a.update(func() {
				a.rulesBusy = false
				a.rulesErr = err.Error()
			})
			return
		}
		if oldComm != "" && oldComm != rule.Comm {
			if err := a.client.deleteRule(ctx, oldComm); err != nil {
				a.update(func() {
					a.rulesBusy = false
					a.rulesErr = "新规则已保存，但删除旧规则失败：" + err.Error()
				})
				return
			}
		}
		rules, loadErr := a.client.rules(ctx)
		a.update(func() {
			a.rulesBusy = false
			if loadErr != nil {
				a.rulesErr = loadErr.Error()
				return
			}
			a.rules = rules
			a.rulesReady = true
			a.rulesErr = ""
			a.resetRuleForm()
		})
	}()
}

func (a *renewApp) deleteRule(comm string) {
	a.rulesBusy = true
	a.rulesErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.client.deleteRule(ctx, comm); err != nil {
			a.update(func() {
				a.rulesBusy = false
				a.rulesErr = err.Error()
			})
			return
		}
		rules, loadErr := a.client.rules(ctx)
		a.update(func() {
			a.rulesBusy = false
			a.pendingDelete = ""
			a.ruleSelected = -1
			if loadErr != nil {
				a.rulesErr = loadErr.Error()
				return
			}
			a.rules = rules
			a.rulesReady = true
			a.rulesErr = ""
			if a.editingRule == comm {
				a.resetRuleForm()
			}
		})
	}()
}

func (a *renewApp) editRule(rule wrapperRule) {
	a.editingRule = rule.Comm
	a.ruleComm = rule.Comm
	a.ruleAction = rule.Action
	a.ruleRegex = rule.Regex
	a.ruleReplace = rule.Replacement
	a.rulePriority = strconv.Itoa(rule.Priority)
	data, _ := json.Marshal(rule.RewrittenCmd)
	a.ruleRewrite = string(data)
	if a.ruleRewrite == "null" {
		a.ruleRewrite = "[]"
	}
}

func (a *renewApp) resetRuleForm() {
	a.editingRule = ""
	a.ruleComm = ""
	a.ruleAction = "ALERT"
	a.ruleRegex = ""
	a.ruleReplace = ""
	a.rulePriority = "0"
	a.ruleRewrite = "[]"
}

func (a *renewApp) filteredRules() []wrapperRule {
	q := strings.ToLower(strings.TrimSpace(a.ruleSearch))
	out := make([]wrapperRule, 0, len(a.rules))
	for _, rule := range a.rules {
		haystack := strings.ToLower(strings.Join([]string{
			rule.Comm, rule.Action, rule.Regex, rule.Replacement, strings.Join(rule.RewrittenCmd, " "),
		}, " "))
		if q == "" || strings.Contains(haystack, q) {
			out = append(out, rule)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].Comm < out[j].Comm
	})
	return out
}

func ruleDescription(rule wrapperRule) string {
	if rule.Regex != "" {
		if rule.Replacement != "" {
			return rule.Regex + " → " + rule.Replacement
		}
		return rule.Regex
	}
	if len(rule.RewrittenCmd) > 0 {
		return strings.Join(rule.RewrittenCmd, " ")
	}
	return "全部参数"
}

func disabledSet(values []int) map[int]bool {
	out := make(map[int]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func cloneDisabled(values map[int]bool) map[int]bool {
	out := make(map[int]bool, len(values))
	for value, disabled := range values {
		if disabled {
			out[value] = true
		}
	}
	return out
}

func disabledSlice(values map[int]bool) []int {
	out := make([]int, 0, len(values))
	for value, disabled := range values {
		if disabled {
			out = append(out, value)
		}
	}
	sort.Ints(out)
	return out
}

func eventTypeSummary(values []int) string {
	parts := make([]string, len(values))
	for i, value := range values {
		parts[i] = strconv.Itoa(value)
	}
	return "event types " + strings.Join(parts, ", ")
}
