package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type destinationSummary struct {
	Endpoint string
	Count    int
	Bytes    uint64
}

type agentSessionSummary struct {
	ID       string
	Events   int
	Alerts   int
	LastSeen int64
	Action   string
}

func (a *nativeApp) view(c *ui.Context) {
	data := a.snapshot()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c, data)
		ui.Divider(c)
		ui.Column(c).Grow(1).FillHeight().Padding(22).Gap(16).Children(func() {
			a.header(c, data)
			switch a.page {
			case "events":
				a.eventsPage(c, data)
			case "processes":
				a.processesPage(c, data)
			case "network":
				a.networkPage(c, data)
			case "monitoring":
				a.monitoringPage(c, data)
			case "rules":
				a.rulesPage(c, data)
			default:
				a.overviewPage(c, data)
			}
		})
	})
}

func (a *nativeApp) sidebar(c *ui.Context, data appData) {
	t := c.Theme()
	ui.Column(c).Width(220).FillHeight().Background(t.Surface).Padding(14).Gap(12).Children(func() {
		ui.Column(c).Padding(8, 10).Gap(2).Children(func() {
			ui.Text(c, "Renew").FontSize(22).Bold()
			ui.Text(c, "Agent eBPF 日常监控").FontSize(12).TextColor(t.TextMuted)
		})
		ui.Divider(c)
		if ui.Sidebar(c, &a.page, func() {
			ui.SidebarSection(c, "监控", nil, func() {
				ui.SidebarItem(c, "overview", nil, "概览")
				ui.SidebarItem(c, "events", nil, "事件").Children(func() {
					if count := attentionCount(data.Events); count > 0 {
						ui.Badge(c, fmt.Sprint(count))
					}
				})
				ui.SidebarItem(c, "processes", nil, "进程")
				ui.SidebarItem(c, "network", nil, "网络")
			})
			ui.SidebarSection(c, "日常管理", nil, func() {
				ui.SidebarItem(c, "monitoring", nil, "监控中心")
				ui.SidebarItem(c, "rules", nil, "Wrapper 规则").Children(func() {
					if len(data.Rules.Items) > 0 {
						ui.Badge(c, fmt.Sprint(len(data.Rules.Items)))
					}
				})
			})
		}).Grow(1).Changed() {
			c.Invalidate()
		}
		ui.Divider(c)
		ui.Text(c, shortBackend(a.backend)).FontSize(11).TextColor(t.TextMuted).Padding(4, 8)
	})
}

func (a *nativeApp) header(c *ui.Context, data appData) {
	t := c.Theme()
	title := map[string]string{
		"overview": "概览", "events": "事件", "processes": "进程", "network": "网络",
		"monitoring": "监控中心", "rules": "Wrapper 规则",
	}[a.page]
	if title == "" {
		title = "Renew"
	}
	ui.Row(c).Gap(10).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(24).Bold()
			status := data.StartupStatus
			if data.EventsConnected || data.SystemConnected {
				status = streamStatus(data)
			}
			ui.Text(c, status).FontSize(12).TextColor(t.TextMuted)
		})
		if ui.Button(c, "刷新").Clicked() {
			a.refresh()
		}
		if ui.Button(c, "打开 Web 工作台").Clicked() {
			go mygo.Shell.OpenExternal(joinRenewURL(a.backend))
		}
	})
	if data.StartupError != "" {
		statusPanel(c, "后端启动失败", data.StartupError, true)
	} else if data.LastStreamError != "" {
		statusPanel(c, "实时连接正在重试", data.LastStreamError, false)
	}
}

func (a *nativeApp) overviewPage(c *ui.Context, data appData) {
	filtered := filterEvents(data.Events, "", false, false)
	attention := attentionCount(filtered)
	agents := buildAgentSessions(filtered)

	ui.Row(c).Gap(12).Children(func() {
		metricCard(c, "CPU", fmt.Sprintf("%.1f%%", data.System.CPUPercent), data.SystemConnected)
		metricCard(c, "内存", fmt.Sprintf("%.1f%%", data.System.MemPercent), data.SystemConnected)
		metricCard(c, "需要关注", fmt.Sprint(attention), attention == 0)
		metricCard(c, "Agent 会话", fmt.Sprint(len(agents)), data.EventsConnected)
	})

	ui.Row(c).Grow(1).AlignItems(ui.Stretch).Gap(14).Children(func() {
		ui.Column(c).Grow(2).Gap(10).Children(func() {
			sectionTitle(c, "最近活动", fmt.Sprintf("最近缓存 %d 条紧凑摘要", len(data.Events)))
			recent := filtered
			if len(recent) > 18 {
				recent = recent[:18]
			}
			eventList(c, recent, a.loadDetail)
		})
		ui.Column(c).Grow(1).Gap(14).Children(func() {
			sectionTitle(c, "活动 Agent", "按 run / conversation / root PID 聚合")
			if len(agents) == 0 {
				emptyText(c, "当前摘要窗口内没有 Agent 上下文。")
			} else {
				limit := min(len(agents), 6)
				for _, session := range agents[:limit] {
					sessionCard(c, session)
				}
			}
			sectionTitle(c, "系统", "来自后端 /ws/system")
			ui.Text(c, fmt.Sprintf("内存 %s / %s", formatBytes(data.System.MemUsed), formatBytes(data.System.MemTotal))).FontSize(12)
			ui.Text(c, fmt.Sprintf("网络接收 %s/s · 发送 %s/s", formatBytes(data.System.NetRecv), formatBytes(data.System.NetSent))).FontSize(12)
			ui.Text(c, fmt.Sprintf("进程 %d · 采样 %s", len(data.System.Processes), formatInterval(data.Monitoring.StatsIntervalMS))).FontSize(12)
		})
	})
}

func (a *nativeApp) eventsPage(c *ui.Context, data appData) {
	ui.Row(c).Gap(10).Children(func() {
		ui.TextInput(c, &a.query).Placeholder("搜索命令、路径、目标、决策…").Label("搜索事件").Grow(1)
		ui.Checkbox(c, &a.onlyAgents, "仅 Agent")
		ui.Checkbox(c, &a.onlyAttention, "仅需关注")
	})

	events := filterEvents(data.Events, a.query, a.onlyAgents, a.onlyAttention)
	ui.Row(c).Grow(1).AlignItems(ui.Stretch).Gap(14).Children(func() {
		ui.Column(c).Grow(3).Gap(8).Children(func() {
			sectionTitle(c, "事件摘要", fmt.Sprintf("%d / %d", len(events), len(data.Events)))
			eventList(c, events, a.loadDetail)
		})
		ui.Column(c).Grow(2).MinWidth(300).Gap(8).Children(func() {
			sectionTitle(c, "事件详情", "完整 payload 按需从后端读取")
			if data.SelectedEventID == "" {
				emptyText(c, "点击左侧事件读取完整详情。")
				return
			}
			ui.Text(c, data.SelectedEventID).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
			if data.DetailLoading {
				ui.Text(c, "正在读取…").TextColor(c.Theme().TextMuted)
				return
			}
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Text(c, data.Detail).Font("monospace").FontSize(12).NoWrap().Selectable().Padding(10)
			})
		})
	})
}

func (a *nativeApp) processesPage(c *ui.Context, data appData) {
	query := strings.TrimSpace(strings.ToLower(a.query))
	ui.TextInput(c, &a.query).Placeholder("按进程名、命令行、用户或 PID 搜索…").Label("搜索进程")
	processes := make([]processSnapshot, 0, len(data.System.Processes))
	for _, process := range data.System.Processes {
		haystack := strings.ToLower(fmt.Sprintf("%d %d %s %s %s", process.PID, process.PPID, process.Name, process.User, process.Command))
		if query == "" || strings.Contains(haystack, query) {
			processes = append(processes, process)
		}
	}
	sectionTitle(c, "实时进程", fmt.Sprintf("%d 个进程 · CPU 排序", len(processes)))
	ui.List(c, nil, len(processes), func(i int) {
		process := processes[i]
		ui.Row(c).Gap(10).Padding(8, 10).Children(func() {
			ui.Text(c, fmt.Sprintf("%d", process.PID)).Width(70).FontSize(12)
			ui.Column(c).Grow(1).Gap(1).Children(func() {
				name := process.Name
				if name == "" {
					name = "(unknown)"
				}
				ui.Text(c, name).Bold().SingleLine()
				ui.Text(c, firstNonEmpty(process.Command, process.User)).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
			})
			ui.Text(c, fmt.Sprintf("CPU %.1f%%", process.CPU)).Width(92).TextAlign(ui.End).FontSize(12)
			ui.Text(c, fmt.Sprintf("MEM %.1f%%", process.Memory)).Width(92).TextAlign(ui.End).FontSize(12)
		})
	}).Grow(1).Children(func() {
		if len(processes) == 0 {
			emptyText(c, "暂无进程数据。")
		}
	})
}

func (a *nativeApp) networkPage(c *ui.Context, data appData) {
	query := strings.TrimSpace(strings.ToLower(a.query))
	ui.TextInput(c, &a.query).Placeholder("搜索域名、IP 或 endpoint…").Label("搜索网络目标")
	destinations := buildDestinations(data.Events)
	if query != "" {
		filtered := destinations[:0]
		for _, destination := range destinations {
			if strings.Contains(strings.ToLower(destination.Endpoint), query) {
				filtered = append(filtered, destination)
			}
		}
		destinations = filtered
	}
	sectionTitle(c, "网络目标", "基于当前有界事件摘要窗口聚合，不等同于全机流量")
	ui.List(c, nil, len(destinations), func(i int) {
		destination := destinations[i]
		ui.Row(c).Gap(12).Padding(9, 10).Children(func() {
			ui.Text(c, destination.Endpoint).Grow(1).SingleLine()
			ui.Text(c, fmt.Sprintf("%d 次", destination.Count)).Width(70).TextAlign(ui.End).FontSize(12)
			ui.Text(c, formatBytes(destination.Bytes)).Width(90).TextAlign(ui.End).FontSize(12).TextColor(c.Theme().TextMuted)
		})
	}).Grow(1).Children(func() {
		if len(destinations) == 0 {
			emptyText(c, "当前摘要窗口没有网络目标。")
		}
	})
}

func (a *nativeApp) monitoringPage(c *ui.Context, data appData) {
	state := data.Monitoring
	if state.Error != "" {
		statusPanel(c, "监控配置", state.Error, true)
	} else if state.Notice != "" {
		statusPanel(c, "监控配置", state.Notice, false)
	}
	if !state.Ready {
		if state.Busy {
			emptyText(c, "正在读取 /config/runtime…")
		} else {
			emptyText(c, "监控配置尚未加载。")
		}
		return
	}

	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).Gap(18).Children(func() {
			sectionTitle(c, "监控档位", "一次更新事件组与行为处理；TLS 不随档位自动开启")
			ui.Row(c).Gap(10).Children(func() {
				active := activeMonitoringProfile(state)
				for _, profile := range nativeMonitoringProfiles {
					profile := profile
					ui.Column(c).Grow(1).MinWidth(180).Padding(12).Gap(6).Radius(9).Background(c.Theme().Surface).Border(1, c.Theme().Border).Children(func() {
						ui.Row(c).Gap(6).Children(func() {
							ui.Text(c, profile.Title).Bold().Grow(1)
							if active == profile.Key {
								ui.Badge(c, "当前")
							}
						})
						ui.Text(c, profile.Description).FontSize(11).TextColor(c.Theme().TextMuted)
						if ui.Button(c, "应用").Disabled(state.Busy || active == profile.Key).Clicked() {
							a.applyMonitoringProfile(profile.Key)
						}
					})
				}
			})

			sectionTitle(c, "内核事件组", "只改变 Renew 管理的事件类型，不覆盖其他用户自定义禁用项")
			for _, module := range nativeMonitoringModules {
				module := module
				enabled := moduleEnabled(state.DisabledEventTypes, module)
				monitoringToggleRow(c, module.Title, module.Description, module.Cost, enabled, state.Busy, func() {
					a.setMonitoringModule(module.Key, !enabled)
				})
			}

			sectionTitle(c, "运行时处理", "独立于内核采集；高成本能力需要时再开")
			runtimeToggleRow(c, "循环检测", "识别重复上下文与资源浪费循环。", runtimeNestedEnabled(state.Runtime, "loopDetection"), state.Busy, func(enabled bool) {
				a.setRuntimeMonitoringToggle("loopDetection", enabled)
			})
			runtimeToggleRow(c, "行为信号", "维护轻量行为信号状态与规则。", runtimeNestedEnabled(state.Runtime, "signalProcessing"), state.Busy, func(enabled bool) {
				a.setRuntimeMonitoringToggle("signalProcessing", enabled)
			})
			runtimeToggleRow(c, "研究处理", "维护研究/AgentSight 聚合，日常常驻可关闭。", runtimeNestedEnabled(state.Runtime, "researchProcessing"), state.Busy, func(enabled bool) {
				a.setRuntimeMonitoringToggle("researchProcessing", enabled)
			})
			runtimeToggleRow(c, "本地事件持久化", "完整事件保存在后端事件库，桌面仍只保留有界摘要。", runtimeBool(state.Runtime, "logPersistenceEnabled"), state.Busy, func(enabled bool) {
				a.setRuntimeMonitoringToggle("logPersistenceEnabled", enabled)
			})
			runtimeToggleRow(c, "TLS 明文捕获", "高成本深度诊断能力；不会随轻量/日常/深度档位自动开启。", runtimeBool(state.Runtime, "tlsCaptureEnabled"), state.Busy, func(enabled bool) {
				a.setRuntimeMonitoringToggle("tlsCaptureEnabled", enabled)
			})

			sectionTitle(c, "状态", "")
			ui.Column(c).Padding(12).Gap(5).Radius(9).Background(c.Theme().Surface).Border(1, c.Theme().Border).Children(func() {
				ui.Text(c, "系统采样："+formatInterval(state.StatsIntervalMS)).FontSize(12)
				persistence := "未启用 / 未确认"
				if state.PersistedEventLogAlive {
					persistence = "正常"
				}
				ui.Text(c, "事件存储："+persistence).FontSize(12)
				if state.PersistedEventLogPath != "" {
					ui.Text(c, state.PersistedEventLogPath).Font("monospace").FontSize(11).TextColor(c.Theme().TextMuted).Selectable()
				}
				policy := "关闭"
				if runtimeBool(state.Runtime, "policyManagementEnabled") {
					policy = "开启"
				}
				ui.Text(c, "策略管理 gate："+policy).FontSize(12)
			})
		})
	})
}

func monitoringToggleRow(c *ui.Context, title, description, cost string, enabled, busy bool, toggle func()) {
	t := c.Theme()
	ui.Row(c).Gap(12).Padding(10, 12).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).Gap(7).Children(func() {
				ui.Text(c, title).Bold()
				badge := ui.Badge(c, cost+"开销")
				if cost == "高" {
					badge.Background(t.Warning).TextColor(t.AccentText)
				}
			})
			ui.Text(c, description).FontSize(11).TextColor(t.TextMuted)
		})
		status := "已停用"
		action := "启用"
		if enabled {
			status = "已启用"
			action = "停用"
		}
		ui.Text(c, status).Width(64).FontSize(11).TextColor(t.TextMuted).TextAlign(ui.End)
		if ui.Button(c, action).Width(68).Disabled(busy).Clicked() {
			toggle()
		}
	})
}

func runtimeToggleRow(c *ui.Context, title, description string, enabled, busy bool, set func(bool)) {
	t := c.Theme()
	ui.Row(c).Gap(12).Padding(10, 12).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, title).Bold()
			ui.Text(c, description).FontSize(11).TextColor(t.TextMuted)
		})
		status := "关闭"
		action := "开启"
		if enabled {
			status = "开启"
			action = "关闭"
		}
		ui.Text(c, status).Width(48).FontSize(11).TextColor(t.TextMuted).TextAlign(ui.End)
		if ui.Button(c, action).Width(68).Disabled(busy).Clicked() {
			set(!enabled)
		}
	})
}

func (a *nativeApp) rulesPage(c *ui.Context, data appData) {
	state := data.Rules
	canManage := data.Monitoring.Ready && runtimeBool(data.Monitoring.Runtime, "policyManagementEnabled")
	if state.Error != "" {
		statusPanel(c, "Wrapper 规则", state.Error, true)
	} else if state.Notice != "" {
		statusPanel(c, "Wrapper 规则", state.Notice, false)
	}
	if !canManage {
		statusPanel(c, "策略管理 gate 未开启", "可以查看规则，但新增、修改与删除会被后端拒绝；请在 Web 工作台启用 policy management。", false)
	}

	ui.Row(c).Grow(1).AlignItems(ui.Stretch).Gap(16).Children(func() {
		ui.Column(c).Grow(2).MinWidth(420).Gap(9).Children(func() {
			sectionTitle(c, "现有规则", fmt.Sprintf("%d 条", len(state.Items)))
			if !state.Ready && state.Busy {
				emptyText(c, "正在读取 /config/rules…")
				return
			}
			ui.List(c, nil, len(state.Items), func(i int) {
				rule := state.Items[i]
				ui.Row(c).Gap(10).Padding(9, 10).Children(func() {
					ui.Column(c).Grow(1).Gap(2).Children(func() {
						ui.Row(c).Gap(7).Children(func() {
							ui.Text(c, rule.Comm).Bold()
							ui.Badge(c, strings.ToUpper(rule.Action))
							ui.Text(c, fmt.Sprintf("P%d", rule.Priority)).FontSize(11).TextColor(c.Theme().TextMuted)
						})
						ui.Text(c, ruleScope(rule)).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
					})
					if ui.Button(c, "编辑").Disabled(state.Busy).Clicked() {
						a.editRule(rule)
					}
					deleteLabel := "删除"
					if a.pendingRuleDelete == rule.Comm {
						deleteLabel = "确认删除"
					}
					if ui.Button(c, deleteLabel).Disabled(state.Busy || !canManage).Clicked() {
						if a.pendingRuleDelete == rule.Comm {
							a.pendingRuleDelete = ""
							a.deleteWrapperRule(rule.Comm)
						} else {
							a.pendingRuleDelete = rule.Comm
						}
					}
				})
			}).Grow(1).Children(func() {
				if state.Ready && len(state.Items) == 0 {
					emptyText(c, "还没有 agent-wrapper 规则。")
				}
			})
		})

		ui.Column(c).Grow(1).MinWidth(310).Gap(10).Padding(12).Radius(9).Background(c.Theme().Surface).Border(1, c.Theme().Border).Children(func() {
			formTitle := "新增规则"
			if a.ruleEditing != "" {
				formTitle = "编辑规则"
			}
			sectionTitle(c, formTitle, "")
			ui.TextInput(c, &a.ruleComm).Placeholder("例如 curl").Label("命令")
			ui.Select(c, &a.ruleAction, []string{"ALLOW", "ALERT", "BLOCK", "REWRITE"}).Label("动作")
			ui.TextInput(c, &a.rulePriority).Placeholder("0").Label("优先级")
			ui.TextInput(c, &a.ruleRegex).Placeholder("可选：参数正则").Label("参数正则")
			if a.ruleAction == "REWRITE" {
				if a.ruleRegex != "" {
					ui.TextInput(c, &a.ruleReplacement).Placeholder("正则替换内容").Label("替换")
				} else {
					ui.TextArea(c, &a.ruleRewrite).Placeholder("[\"echo\",\"hello\"]").Label("固定重写 argv").Height(90)
				}
			}
			ui.Text(c, "Wrapper 规则只约束经 agent-wrapper 执行的命令，不等同于系统级 BPF LSM 策略。").FontSize(11).TextColor(c.Theme().TextMuted)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if ui.Button(c, "清空").Disabled(state.Busy).Clicked() {
					a.resetRuleForm()
				}
				if ui.PrimaryButton(c, "保存").Disabled(state.Busy || !state.Ready || !canManage || strings.TrimSpace(a.ruleComm) == "").Clicked() {
					rule, err := a.ruleFromForm()
					if err != nil {
						a.update(func(data *appData) { data.Rules.Error = err.Error() })
					} else {
						a.saveWrapperRule(rule)
					}
				}
			})
		})
	})
}

func (a *nativeApp) editRule(rule wrapperRule) {
	a.ruleEditing = rule.Comm
	a.ruleComm = rule.Comm
	a.ruleAction = strings.ToUpper(rule.Action)
	a.ruleRegex = rule.Regex
	a.ruleReplacement = rule.Replacement
	a.rulePriority = strconv.Itoa(rule.Priority)
	payload, _ := json.Marshal(rule.RewrittenCmd)
	a.ruleRewrite = string(payload)
	a.pendingRuleDelete = ""
}

func (a *nativeApp) resetRuleForm() {
	a.ruleEditing = ""
	a.ruleComm = ""
	a.ruleAction = "ALERT"
	a.ruleRegex = ""
	a.ruleReplacement = ""
	a.rulePriority = "0"
	a.ruleRewrite = "[]"
	a.pendingRuleDelete = ""
}

func (a *nativeApp) ruleFromForm() (wrapperRule, error) {
	comm := strings.TrimSpace(a.ruleComm)
	if comm == "" {
		return wrapperRule{}, fmt.Errorf("命令名不能为空")
	}
	priority := 0
	if raw := strings.TrimSpace(a.rulePriority); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return wrapperRule{}, fmt.Errorf("优先级必须是整数")
		}
		priority = value
	}
	action := strings.ToUpper(strings.TrimSpace(a.ruleAction))
	switch action {
	case "ALLOW", "ALERT", "BLOCK", "REWRITE":
	default:
		return wrapperRule{}, fmt.Errorf("不支持的规则动作：%s", action)
	}
	rule := wrapperRule{
		Comm: comm, Action: action, Regex: strings.TrimSpace(a.ruleRegex),
		Replacement: a.ruleReplacement, Priority: priority,
	}
	if action == "REWRITE" && rule.Regex == "" {
		args, err := parseRewriteArgs(a.ruleRewrite)
		if err != nil {
			return wrapperRule{}, err
		}
		rule.RewrittenCmd = args
	}
	return rule, nil
}

func ruleScope(rule wrapperRule) string {
	if rule.Regex != "" {
		if rule.Replacement != "" {
			return rule.Regex + " → " + rule.Replacement
		}
		return "regex: " + rule.Regex
	}
	if len(rule.RewrittenCmd) > 0 {
		return strings.Join(rule.RewrittenCmd, " ")
	}
	return "全部参数"
}

func eventList(c *ui.Context, events []eventSummary, onSelect func(string)) {
	ui.List(c, nil, len(events), func(i int) {
		event := events[i]
		label := describeEvent(event)
		if ui.Button(c, "").FillWidth().Children(func() {
			ui.Row(c).FillWidth().Gap(10).Padding(3, 1).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, label).Bold().SingleLine()
					meta := strings.TrimSpace(strings.Join(nonEmpty(event.Comm, event.Path, event.NetEndpoint), " · "))
					if meta != "" {
						ui.Text(c, meta).FontSize(11).TextColor(c.Theme().TextMuted).SingleLine()
					}
				})
				if isAttentionEvent(event) {
					ui.Badge(c, attentionLabel(event)).Background(c.Theme().Danger).TextColor(c.Theme().AccentText)
				}
				ui.Text(c, relativeTime(event.ReceivedAtMS)).FontSize(11).TextColor(c.Theme().TextMuted).Width(72).TextAlign(ui.End)
			})
		}).Clicked() {
			onSelect(eventIdentity(event))
		}
	}).Grow(1).Children(func() {
		if len(events) == 0 {
			emptyText(c, "没有匹配的事件。")
		}
	})
}

func metricCard(c *ui.Context, label, value string, healthy bool) {
	t := c.Theme()
	card := ui.Column(c).Grow(1).Padding(14).Gap(5).Radius(10).Background(t.Surface).Border(1, t.Border)
	card.Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, value).FontSize(24).Bold().Grow(1)
			if healthy {
				ui.Badge(c, "正常")
			}
		})
	})
}

func statusPanel(c *ui.Context, title, message string, danger bool) {
	t := c.Theme()
	box := ui.Column(c).Padding(10, 12).Gap(3).Radius(8).Background(t.Surface).Border(1, t.Border)
	if danger {
		box.Border(1, t.Danger)
	}
	box.Children(func() {
		ui.Text(c, title).Bold().FontSize(12)
		ui.Text(c, message).FontSize(11).TextColor(t.TextMuted)
	})
}

func sectionTitle(c *ui.Context, title, subtitle string) {
	ui.Row(c).Gap(10).Children(func() {
		ui.Text(c, title).Bold().Grow(1)
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(11).TextColor(c.Theme().TextMuted)
		}
	})
}

func sessionCard(c *ui.Context, session agentSessionSummary) {
	t := c.Theme()
	ui.Column(c).Padding(10).Gap(3).Radius(8).Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, session.ID).Bold().SingleLine()
		ui.Text(c, session.Action).FontSize(11).TextColor(t.TextMuted).SingleLine()
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, fmt.Sprintf("%d 事件", session.Events)).FontSize(11)
			if session.Alerts > 0 {
				ui.Badge(c, fmt.Sprintf("%d 关注", session.Alerts)).Background(t.Danger).TextColor(t.AccentText)
			}
		})
	})
}

func emptyText(c *ui.Context, text string) {
	ui.Text(c, text).TextColor(c.Theme().TextMuted).Padding(12)
}

func filterEvents(events []eventSummary, query string, onlyAgents, onlyAttention bool) []eventSummary {
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]eventSummary, 0, len(events))
	for _, event := range events {
		if onlyAgents && !isAgentEvent(event) {
			continue
		}
		if onlyAttention && !isAttentionEvent(event) {
			continue
		}
		if query != "" {
			haystack := strings.ToLower(strings.Join([]string{
				event.Tag, event.Comm, event.Type, event.Path, event.ExtraPath,
				event.NetEndpoint, event.Domain, event.ToolName, event.Decision,
			}, " "))
			if !strings.Contains(haystack, query) {
				continue
			}
		}
		result = append(result, event)
	}
	return result
}

func attentionCount(events []eventSummary) int {
	count := 0
	for _, event := range events {
		if isAttentionEvent(event) {
			count++
		}
	}
	return count
}

func buildDestinations(events []eventSummary) []destinationSummary {
	index := make(map[string]*destinationSummary)
	for _, event := range events {
		endpoint := firstNonEmpty(event.Domain, event.NetEndpoint)
		endpoint = strings.TrimSpace(endpoint)
		if endpoint == "" {
			continue
		}
		row := index[endpoint]
		if row == nil {
			row = &destinationSummary{Endpoint: endpoint}
			index[endpoint] = row
		}
		row.Count++
		row.Bytes += event.NetBytes
	}
	result := make([]destinationSummary, 0, len(index))
	for _, row := range index {
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Count == result[j].Count {
			return result[i].Bytes > result[j].Bytes
		}
		return result[i].Count > result[j].Count
	})
	return result
}

func buildAgentSessions(events []eventSummary) []agentSessionSummary {
	index := make(map[string]*agentSessionSummary)
	for _, event := range events {
		if !isAgentEvent(event) {
			continue
		}
		id := firstNonEmpty(event.AgentRunID, event.ConversationID)
		if id == "" && event.RootAgentPID != 0 {
			id = fmt.Sprintf("PID %d", event.RootAgentPID)
		}
		if id == "" {
			id = fmt.Sprintf("PID %d", event.PID)
		}
		row := index[id]
		if row == nil {
			row = &agentSessionSummary{ID: id}
			index[id] = row
		}
		row.Events++
		if isAttentionEvent(event) {
			row.Alerts++
		}
		if event.ReceivedAtMS >= row.LastSeen {
			row.LastSeen = event.ReceivedAtMS
			row.Action = describeEvent(event)
		}
	}
	result := make([]agentSessionSummary, 0, len(index))
	for _, row := range index {
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeen > result[j].LastSeen })
	return result
}

func describeEvent(event eventSummary) string {
	t := strings.ToLower(event.Type)
	switch {
	case strings.Contains(t, "exec"):
		return "启动进程 · " + firstNonEmpty(event.Path, event.Comm, event.Type)
	case strings.Contains(t, "connect") || strings.Contains(t, "send") || strings.Contains(t, "recv") || event.NetEndpoint != "":
		return "访问网络 · " + firstNonEmpty(event.Domain, event.NetEndpoint, event.Comm)
	case strings.Contains(t, "open") || strings.Contains(t, "read"):
		return "读取文件 · " + firstNonEmpty(event.Path, event.Comm, event.Type)
	case strings.Contains(t, "write") || strings.Contains(t, "rename") || strings.Contains(t, "unlink"):
		return "修改文件 · " + firstNonEmpty(event.Path, event.Comm, event.Type)
	case event.ToolName != "":
		return "调用工具 · " + event.ToolName
	default:
		return firstNonEmpty(event.Type, event.Tag, "系统活动")
	}
}

func attentionLabel(event eventSummary) string {
	decision := strings.ToUpper(strings.TrimSpace(event.Decision))
	if strings.Contains(decision, "BLOCK") || strings.Contains(decision, "DENY") {
		return "已阻断"
	}
	if event.RiskScore >= 60 {
		return fmt.Sprintf("风险 %.0f", event.RiskScore)
	}
	return "关注"
}

func relativeTime(ms int64) string {
	if ms <= 0 {
		return ""
	}
	d := time.Since(time.UnixMilli(ms))
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 时", int(d.Hours()))
	default:
		return time.UnixMilli(ms).Format("01-02")
	}
}

func streamStatus(data appData) string {
	parts := make([]string, 0, 2)
	if data.EventsConnected {
		parts = append(parts, "事件流在线")
	} else {
		parts = append(parts, "事件流离线")
	}
	if data.SystemConnected {
		parts = append(parts, "系统指标在线")
	} else {
		parts = append(parts, "系统指标离线")
	}
	return strings.Join(parts, " · ")
}

func shortBackend(backend string) string {
	backend = strings.TrimPrefix(backend, "http://")
	backend = strings.TrimPrefix(backend, "https://")
	return backend
}

func formatInterval(ms int) string {
	switch ms {
	case 2_000:
		return "2 秒"
	case 5_000:
		return "5 秒"
	case 10_000:
		return "10 秒"
	case 30_000:
		return "30 秒"
	default:
		return fmt.Sprintf("%.1f 秒", float64(ms)/1000)
	}
}

func formatBytes(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit && exp < 4; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func nonEmpty(values ...string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
