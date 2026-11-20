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

func (a *renewApp) rulesView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Wrapper 规则").FontSize(28).Bold()
	ui.Text(c, "为经 agent-wrapper 执行的命令设置允许、告警、阻断或重写策略；高影响动作会在列表中明确标色。").TextColor(t.TextMuted)
	card(c, "内置默认规则 · 只读", func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			statusPill(c, "ALLOW", t.Success)
			ui.Column(c).Grow(1).Gap(4).Children(func() {
				ui.Text(c, "公开 GitHub 仓库克隆").Bold()
				ui.Text(c, "git clone / gh repo clone · 匿名验证仓库公开后放行；仅提高该克隆链路的循环检测阈值，不关闭安全检查。").FontSize(12).TextColor(t.TextMuted)
				ui.Text(c, "需要经 agent-wrapper 捕获命令；私有仓库、危险 Git 参数和用户自定义 Wrapper 规则不受此默认放行影响。").FontSize(11).TextColor(t.TextMuted)
			})
		})
	})

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
			if ui.PrimaryButton(c, "保存规则").Disabled(!a.rulesReady || a.rulesBusy).Clicked() && a.rulesReady && !a.rulesBusy {
				a.saveRule()
			}
			if a.editingRule != "" && ui.Button(c, "取消编辑").Disabled(a.rulesBusy).Clicked() && !a.rulesBusy {
				a.resetRuleForm()
			}
			if ui.Button(c, "重新读取").Disabled(a.rulesBusy).Clicked() && !a.rulesBusy {
				go a.refreshRules(context.Background())
			}
		})
	})
	if a.rulesBusy {
		renewLoadingRow(c, "正在等待规则操作确认…")
	}

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

func ruleActionPill(c *ui.Context, action string) ui.Element {
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
