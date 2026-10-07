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
	ui.Text(c, "先选择日常使用强度，再按需要打开高开销或敏感能力；所有修改都以后端确认结果为准。").TextColor(t.TextMuted)

	if a.configErr != "" {
		ui.Text(c, a.configErr).TextColor(t.TextMuted)
	}
	card(c, "采集强度", func() {
		ui.Text(c, "“日常”适合作为长期常开默认值；“深度”会启用高频访问与 syscall 轨迹，建议仅排障时使用。").FontSize(12).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
			for _, name := range []string{"轻量", "日常", "深度"} {
				name := name
				active := a.monitoringProfileActive(name)
				var button *ui.Element
				if active {
					button = ui.PrimaryButton(c, name+" · 当前")
				} else {
					button = ui.Button(c, name)
				}
				if button.Clicked() && a.configReady && !a.configBusy && !active {
					a.applyMonitoringProfile(name)
				}
			}
			ui.Spacer(c)
			if ui.Button(c, "重新读取").Clicked() && !a.configBusy {
				go a.refreshEventTypeConfig(context.Background())
			}
		})
	})

	for _, module := range monitoringModules {
		module := module
		card(c, module.Title, func() {
			ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(5).Children(func() {
					ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
						ui.Text(c, module.Description).TextColor(t.TextMuted).Grow(1)
						if module.Cost == "高" {
							statusPill(c, "高开销", t.Warning)
						} else {
							statusPill(c, "低开销", t.Success)
						}
					})
					ui.Text(c, eventTypeSummary(module.EventTypes)).Font("monospace").FontSize(10).TextColor(t.TextMuted)
				})
				enabled := a.monitoringModuleEnabled(module)
				next := enabled
				if ui.Switch(c, &next).Label(module.Title).Changed() && a.configReady && !a.configBusy {
					a.setMonitoringModule(module, next)
				}
			})
		})
	}

	card(c, "行为分析与运行时", func() {
		ui.Text(c, "这些开关会实际启停对应后端路径；更改以后端返回的运行时配置为准。").FontSize(11).TextColor(t.TextMuted)
		for _, item := range []struct {
			Key, Title, Description, Badge string
		}{
			{"loopDetection", "行为循环检测", "识别重复执行、反复读写与资源浪费循环。", ""},
			{"signalProcessing", "信号处理", "启用信号规则、TTL 衰减与选中程序日志处理。", ""},
			{"researchProcessing", "研究处理", "维护研究视图、时间线与会话派生数据；深度排查时开启。", "诊断"},
			{"tlsCapture", "TLS 明文捕获", "高敏感、高开销能力；仅在明确需要时开启。", "敏感"},
			{"persistence", "本地事件持久化", "将完整事件保留在后端日志/事件库，桌面仍只按需读取。", ""},
			{"policyManagement", "策略管理", "允许本机 UI 下发跟踪配置与 cgroup/BPF LSM 阻断动作。", "高权限"},
		} {
			item := item
			ui.Row(c).Padding(7, 0).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(3).Children(func() {
					ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
						ui.Text(c, item.Title).Bold()
						switch item.Badge {
						case "敏感":
							statusPill(c, item.Badge, t.Danger)
						case "高权限":
							statusPill(c, item.Badge, t.Warning)
						case "诊断":
							statusPill(c, item.Badge, t.Warning)
						}
					})
					ui.Text(c, item.Description).FontSize(11).TextColor(t.TextMuted)
				})
				enabled := a.runtimeToggleEnabled(item.Key)
				next := enabled
				if ui.Switch(c, &next).Label(item.Title).Changed() && a.runtimeReady && !a.configBusy {
					a.setRuntimeToggle(item.Key, next)
				}
			})
		}
		if a.runtimeCfg.PersistedEventLogPath != "" {
			ui.Text(c, "事件库："+a.runtimeCfg.PersistedEventLogPath).Font("monospace").FontSize(10).TextColor(t.TextMuted)
		}
	})
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
	ui.Text(c, "为经 agent-wrapper 执行的命令设置允许、告警、阻断或重写策略；高影响动作会在列表中明确标色。").TextColor(t.TextMuted)

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
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Select(c, &a.ruleAction, []string{"ALLOW", "BLOCK", "ALERT", "REWRITE"}).Label("动作")
					ruleActionPill(c, a.ruleAction)
				})
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
				ruleActionPill(c, r.Action)
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
		ui.Column(c).Padding(16).Gap(10).Radius(12).Background(t.Danger.Alpha(0.05)).Border(1, t.Danger.Alpha(0.38)).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				statusPill(c, "删除规则", t.Danger)
				ui.Text(c, "确定删除 "+a.pendingDelete+"？此操作影响所有 Agent 工具的对应命令。").Grow(1)
			})
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

func ruleActionPill(c *ui.Context, action string) *ui.Element {
	t := c.Theme()
	switch strings.ToUpper(strings.TrimSpace(action)) {
	case "BLOCK":
		return statusPill(c, "BLOCK", t.Danger)
	case "ALERT":
		return statusPill(c, "ALERT", t.Warning)
	case "ALLOW":
		return statusPill(c, "ALLOW", t.Success)
	case "REWRITE":
		return statusPill(c, "REWRITE", t.Accent)
	default:
		return ui.Badge(c, displayOr(action, "未知"))
	}
}

func (a *renewApp) monitoringProfileActive(name string) bool {
	keys, ok := monitoringProfiles[name]
	if !ok || !a.configReady {
		return false
	}
	enabled := make(map[string]bool, len(keys))
	for _, key := range keys {
		enabled[key] = true
	}
	for _, module := range monitoringModules {
		if a.monitoringModuleEnabled(module) != enabled[module.Key] {
			return false
		}
	}
	return true
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
	loop, signal, research := false, false, false
	switch name {
	case "日常":
		loop, signal = true, true
	case "深度":
		loop, signal, research = true, true, true
	}
	if a.client == nil || a.configBusy {
		return
	}
	a.configBusy = true
	a.configErr = ""
	values := disabledSlice(next)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		cfg, err := a.client.applyMonitoringProfile(ctx, values, loop, signal, research)
		a.update(func() {
			a.configBusy = false
			if err != nil {
				a.configErr = err.Error()
				return
			}
			a.applyRuntimeConfig(cfg)
		})
	}()
}

func (a *renewApp) writeEventTypes(next map[int]bool) {
	if a.client == nil || a.configBusy {
		return
	}
	a.configBusy = true
	a.configErr = ""
	values := disabledSlice(next)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		cfg, err := a.client.putRuntimeConfig(ctx, map[string]any{"disabledEventTypes": values})
		a.update(func() {
			a.configBusy = false
			if err != nil {
				a.configErr = err.Error()
				return
			}
			a.applyRuntimeConfig(cfg)
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
	ctx, cancel := context.WithTimeout(parent, 6*time.Second)
	defer cancel()
	cfg, err := a.client.runtimeConfig(ctx)
	a.update(func() {
		a.configBusy = false
		if err != nil {
			a.configErr = err.Error()
			return
		}
		a.applyRuntimeConfig(cfg)
	})
}

func (a *renewApp) applyRuntimeConfig(cfg runtimeConfigResponse) {
	a.runtimeCfg = cfg
	a.runtimeReady = true
	a.configReady = true
	a.configErr = ""
	a.disabledEventTypes = disabledSet(cfg.Runtime.DisabledEventTypes)
}

func (a *renewApp) runtimeToggleEnabled(key string) bool {
	switch key {
	case "loopDetection":
		return a.runtimeCfg.Runtime.LoopDetection.Enabled
	case "signalProcessing":
		return a.runtimeCfg.Runtime.SignalProcessing.Enabled
	case "researchProcessing":
		return a.runtimeCfg.Runtime.ResearchProcessing.Enabled
	case "tlsCapture":
		return a.runtimeCfg.Runtime.TLSCaptureEnabled
	case "persistence":
		return a.runtimeCfg.Runtime.LogPersistenceEnabled
	case "policyManagement":
		return a.runtimeCfg.Runtime.PolicyManagementEnabled
	default:
		return false
	}
}

func (a *renewApp) setRuntimeToggle(key string, enabled bool) {
	if a.client == nil || a.configBusy || !a.runtimeReady {
		return
	}
	a.configBusy = true
	a.configErr = ""
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		cfg, err := a.client.runtimePatchWithNestedToggle(ctx, key, enabled)
		a.update(func() {
			a.configBusy = false
			if err != nil {
				a.configErr = err.Error()
				return
			}
			a.applyRuntimeConfig(cfg)
		})
	}()
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
