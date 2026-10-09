package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Inspector always uses the already-retained compact summaries. The pinned
// identity is stable when the live stream reorders rows, and never silently
// changes to another event if the pinned summary ages out.
func (a *renewApp) inspectorEvent() (eventSummary, bool) {
	if a.inspectorPinnedID != "" {
		for _, event := range a.events {
			if event.EventID == a.inspectorPinnedID {
				return event, true
			}
		}
		return eventSummary{}, false
	}

	// A direct choice from the alert queue is an investigation override:
	// it must remain visible even when local table filters exclude it.
	// This does not change the filters or request full event details.
	if a.inspectorSelectedID != "" {
		for _, event := range a.events {
			if event.EventID == a.inspectorSelectedID {
				return event, true
			}
		}
	}
	rows := a.events
	if a.page == "事件" || a.page == "概览" {
		rows = a.filteredEvents()
	}
	if len(rows) == 0 {
		return eventSummary{}, false
	}
	if a.page == "事件" && a.eventSelected >= 0 && a.eventSelected < len(rows) &&
		a.eventSelected < a.eventVisibleLimit {
		return rows[a.eventSelected], true
	}
	for _, event := range rows {
		if risk := eventRisk(event); risk == "高风险" || risk == "需关注" {
			return event, true
		}
	}
	return rows[0], true
}

// A direct click on a queue item must take precedence over any old pin.
func (a *renewApp) selectInspectorEvent(id string) {
	if id == "" {
		return
	}
	a.inspectorPinnedID = ""
	a.inspectorSelectedID = id
	// Only an explicit event-table selection should drive table navigation.
	a.eventSelected = -1
	a.inspectorTab = 0
}

func (a *renewApp) inspectorAlerts(limit int) []eventSummary {
	if limit <= 0 {
		return nil
	}
	out := make([]eventSummary, 0, min(limit, 6))
	for _, event := range a.events {
		if risk := eventRisk(event); risk == "正常" || risk == "未评级" {
			continue
		}
		out = append(out, event)
		if len(out) == limit {
			break
		}
	}
	return out
}

func (a *renewApp) clearEventFilters() {
	a.search = ""
	a.eventTypeFilter = ""
	a.eventSessionFilter = ""
	a.eventDecisionFilter = ""
	a.eventPIDFilter = 0
	a.eventRiskFilter = ""
	a.eventAttentionOnly = false
	a.eventVisibleLimit = 50
	a.eventSelected = -1
	a.inspectorSelectedID = ""
}

func (a *renewApp) focusSummary(id string) {
	a.clearEventFilters()
	a.page = "事件"
	a.inspectorSelectedID = id
	for i, event := range a.events {
		if event.EventID == id {
			a.eventSelected = i
			if a.eventVisibleLimit <= i {
				a.eventVisibleLimit = i + 1
			}
			break
		}
	}
}

// openEventFilter is a read-only navigation action. It never changes backend
// capture scope or privileged enforcement policy.
func (a *renewApp) openEventFilter(event eventSummary, kind string) {
	a.clearEventFilters()
	a.page = "事件"
	switch kind {
	case "pid":
		a.eventPIDFilter = event.PID
	case "type":
		a.eventTypeFilter = event.Type
	case "session":
		a.eventSessionFilter = eventSessionKey(event)
	case "attention":
		a.eventAttentionOnly = true
	case "risk":
		a.eventRiskFilter = eventRisk(event)
	case "decision":
		switch strings.ToUpper(strings.TrimSpace(event.Decision)) {
		case "BLOCK", "DENY":
			a.eventDecisionFilter = "已阻断"
		case "ALERT":
			a.eventDecisionFilter = "告警"
		case "ALLOW":
			a.eventDecisionFilter = "已允许"
		}
	}
}

func summaryClipboardText(e eventSummary) string {
	// The summary is already privacy-redacted upstream; do not fetch or copy
	// full event content as a side-effect of the UI.
	return fmt.Sprintf("Event ID: %s\nTime: %s\nType: %s\nProcess: %s (PID %d)\nTarget: %s\nDecision: %s\nRisk score: %.0f",
		e.EventID, eventTime(e), e.Type, e.Comm, e.PID, eventTarget(e), e.Decision, e.RiskScore)
}

// inspectorStandalone keeps incident triage accessible on smaller displays.
// It uses the exact same real data and actions as the wide-screen rail.
func (a *renewApp) inspectorStandalone(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "风险研判工作台").FontSize(24).Bold()
	ui.Text(c, "固定、筛选和查看真实事件；完整记录仍按需从后端读取。").TextColor(t.TextMuted)
	a.inspector(c)
}

func (a *renewApp) inspector(c *ui.Context) {
	t := c.Theme()
	_, attention, danger := a.riskCounts()
	width := float32(302)
	if a.page == "研判" {
		width = 560
	}
	panel := ui.Column(c).Width(width).Shrink(0).Background(t.Surface).Border(1, t.Border)
	if a.page == "研判" {
		panel.Height(690)
	}
	panel.Children(func() {
		ui.Row(c).Padding(14, 13).Gap(7).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).Gap(3).Children(func() {
				ui.Text(c, "事件研判").FontSize(14).Bold()
				ui.Text(c, "本地摘要 · 按需加载原始详情").FontSize(10).TextColor(t.TextMuted)
			})
			closeTitle := "关闭侧栏"
			if a.page == "研判" {
				closeTitle = "返回事件列表"
			}
			if ui.Button(c, "×").Tooltip(closeTitle).Clicked() {
				if a.page == "研判" {
					a.page = "事件"
				} else {
					a.inspectorOpen = false
				}
			}
		})
		ui.Tabs(c, &a.inspectorTab, "事件研判", "运行诊断")
		ui.Divider(c)
		if a.inspectorTab == 1 {
			a.inspectorDiagnostics(c)
		} else {
			ui.Scroll(c).Grow(1).Padding(12).Gap(12).Children(func() {
				if !a.localMonitor { ui.Row(c).Gap(8).Children(func() {
					ui.Column(c).Grow(1).Padding(12).Gap(4).Radius(9).Background(t.Background).Children(func() {
						ui.Text(c, "需关注").FontSize(11).TextColor(t.TextMuted)
						ui.Text(c, strconv.Itoa(attention)).FontSize(22).Bold().TextColor(t.Warning)
						if ui.Button(c, "筛选").Tooltip("仅展示需关注等级").Clicked() {
							a.clearEventFilters()
							a.eventRiskFilter = "需关注"
							a.page = "事件"
						}
					})
					ui.Column(c).Grow(1).Padding(12).Gap(4).Radius(9).Background(t.Background).Children(func() {
						ui.Text(c, "高风险").FontSize(11).TextColor(t.TextMuted)
						ui.Text(c, strconv.Itoa(danger)).FontSize(22).Bold().TextColor(t.Danger)
						if ui.Button(c, "筛选").Tooltip("仅展示高风险等级").Clicked() {
							a.clearEventFilters()
							a.eventRiskFilter = "高风险"
							a.page = "事件"
						}
					})
				})
				ui.Text(c, "统计来自当前最多 1200 条摘要，并非历史总量").FontSize(10).TextColor(t.TextMuted)
                if ui.Button(c, "查看所有需关注事件").Clicked() {
                    a.clearEventFilters()
                    a.eventAttentionOnly = true
                    a.page = "事件"
                }
                } else {
                    ui.Text(c, fmt.Sprintf("Windows 本机采样 · %d 条未评级状态观察",len(a.events))).FontSize(12).TextColor(t.Warning)
                    ui.Text(c, "观察到的连接与进程变化不是威胁判定；不进行风险评分或拦截。").FontSize(11).TextColor(t.TextMuted)
                }
				ui.Divider(c)
				ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
					ui.Text(c, "事件上下文").Bold().FontSize(12).Grow(1)
					if a.inspectorPinnedID != "" {
						statusPill(c, "已固定", t.Accent)
					}
				})
				event, ok := a.inspectorEvent()
				if !ok {
					if a.inspectorPinnedID != "" {
						ui.Text(c, "固定的事件已离开内存摘要窗口；解除固定后可继续自动跟随。").FontSize(11).TextColor(t.Warning)
						if ui.Button(c, "解除固定").Clicked() {
							a.inspectorPinnedID = ""
						}
					} else {
						ui.Text(c, "当前没有匹配的事件；检查筛选条件或等待采集。").FontSize(11).TextColor(t.TextMuted)
					}
				} else {
					a.inspectorEventCard(c, event)
				}
				ui.Divider(c)
				if a.localMonitor { ui.Text(c, "采样记录不会作为风险告警").FontSize(12).TextColor(t.TextMuted) } else { ui.Text(c, "最近需关注事件").FontSize(12).Bold() }
				alerts := a.inspectorAlerts(5)
				if len(alerts) == 0 {
					ui.Text(c, "当前摘要窗口暂无待关注事件。").FontSize(11).TextColor(t.TextMuted)
				}
				for _, alert := range alerts {
					item := alert
					ui.Column(c).Padding(8).Gap(4).Radius(8).Background(t.Background).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							riskPill(c, eventRisk(item))
							ui.Text(c, eventTime(item)).FontSize(10).TextColor(t.TextMuted)
						})
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							if label := eventHarnessLabel(item); label != "未识别" {
								drawHarnessIcon(c, label)
							}
							ui.Text(c, eventAction(item)+" · "+displayOr(item.Comm, "未知")).FontSize(11).MaxLines(2)
						})
						if ui.Button(c, "查看此事件").Clicked() {
							a.selectInspectorEvent(item.EventID)
						}
					})
				}
			})
		}
		ui.Divider(c)
		ui.Row(c).Padding(11).Gap(8).Wrap().Children(func() {
			if ui.Button(c, "事件列表").Clicked() { a.page = "事件" }
			if ui.Button(c, "系统诊断").Clicked() { a.page = "系统" }
		})
	})
}

func (a *renewApp) inspectorEventCard(c *ui.Context, event eventSummary) {
	t := c.Theme()
	ui.Column(c).Padding(12).Gap(8).Radius(10).Background(t.Background).Border(1, t.Border).Children(func() {
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			riskPill(c, eventRisk(event))
			ui.Text(c, eventTime(event)).Font("monospace").FontSize(10).TextColor(t.TextMuted)
			ui.Spacer(c)
			label := "固定"
			if a.inspectorPinnedID == event.EventID {
				label = "解除固定"
			}
			if ui.Button(c, label).Tooltip("固定此事件，不随实时事件更新而跳转").Clicked() {
				if a.inspectorPinnedID == event.EventID {
					a.inspectorPinnedID = ""
				} else {
					a.inspectorPinnedID = event.EventID
				}
			}
		})
		if a.inspectorPinnedID == "" && a.inspectorSelectedID != "" {
			if ui.Button(c, "恢复跟随最新风险").Tooltip("清除手动选择，自动查看最新待关注事件").Clicked() {
				a.inspectorSelectedID = ""
				a.eventSelected = -1
			}
		}
		ui.Text(c, eventAction(event)).FontSize(14).Bold()
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if label := eventHarnessLabel(event); label != "未识别" {
				drawHarnessIcon(c, label)
			}
			ui.Text(c, displayOr(event.Comm, "未知进程")+" · PID "+strconv.Itoa(event.PID)).FontSize(11).TextColor(t.TextMuted)
		})
		if target := strings.TrimSpace(eventTarget(event)); target != "" && target != "-" {
			ui.Text(c, target).Font("monospace").FontSize(11).MaxLines(4)
		}
		if event.Decision != "" {
			ui.Text(c, "策略决策: "+event.Decision).FontSize(11).TextColor(t.TextMuted)
		}
		if event.HasAgentContext || event.AgentRunID != "" || event.ConversationID != "" {
			ui.Text(c, eventSessionKey(event)).FontSize(11).TextColor(t.TextMuted).MaxLines(2)
		}
		if event.EventID != "" {
			if ui.PrimaryButton(c, "查看完整事件详情").Clicked() {
				a.openEventDetail(event.EventID)
			}
			ui.Row(c).Gap(6).Wrap().Children(func() {
				if ui.Button(c, "复制摘要").Clicked() {
					c.WriteClipboard(summaryClipboardText(event))
				}
				if ui.Button(c, "定位").Clicked() {
					a.focusSummary(event.EventID)
				}
			})
		}
		ui.Divider(c)
		ui.Text(c, "关联检索").FontSize(11).Bold()
		ui.Row(c).Wrap().Gap(6).Children(func() {
			if event.PID > 0 && ui.Button(c, "同 PID").Clicked() {
				a.openEventFilter(event, "pid")
			}
			if event.Type != "" && ui.Button(c, "同类型").Clicked() {
				a.openEventFilter(event, "type")
			}
			if isAgentSummary(event) && ui.Button(c, "同会话").Clicked() {
				a.openEventFilter(event, "session")
			}
		})
	})
}

// Diagnostics always reflects backend-reported health, never a synthetic
// "healthy" status inferred from a lack of events.
func (a *renewApp) inspectorDiagnostics(c *ui.Context) {
	t := c.Theme()
	ui.Scroll(c).Grow(1).Padding(12).Gap(12).Children(func() {
		label, level := a.pipelineStatus()
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "采集链路").FontSize(12).Bold().Grow(1)
			statusPill(c, label, workspaceStatusTone(t, level))
		})
		card(c, "采集状态", func() {
			if !a.healthReady {
				ui.Text(c, "尚未完成首次健康检查").TextColor(t.Warning)
			} else {
				if a.localMonitor {
					ui.Text(c, fmt.Sprintf("Windows 采样摘要溢出 %d", a.health.RingbufDroppedTotal)).Font("monospace").FontSize(11)
					ui.Text(c, "2 秒进程 / TCP 状态快照（实验性，只读；非内核审计）").FontSize(11).TextColor(t.Warning)
				} else {
					ui.Text(c, fmt.Sprintf("Ringbuf 丢弃 %d", a.health.RingbufDroppedTotal)).Font("monospace").FontSize(11)
					ui.Text(c, fmt.Sprintf("后端队列 %d", a.health.BackendQueueLen)).Font("monospace").FontSize(11)
					ui.Text(c, fmt.Sprintf("持久化队列 %d / %d", a.health.PersistQueueLen, a.health.PersistQueueCap)).Font("monospace").FontSize(11)
				}
			}
			if !a.lastSync.IsZero() {
				ui.Text(c, "最近同步 "+a.lastSync.Format("15:04:05")).FontSize(10).TextColor(t.TextMuted)
			}
		})
		card(c, "传输状态", func() {
			if a.eventStreamConnected {
				if a.localMonitor { statusPill(c, "本机事件采样", t.Warning) } else { statusPill(c, "事件流已连接", t.Success) }
			} else {
				statusPill(c, "事件流回退/重连中", t.Warning)
				if a.eventStreamErr != "" {
					ui.Text(c, a.eventStreamErr).FontSize(10).MaxLines(4).TextColor(t.TextMuted)
				}
			}
			if a.systemConnected {
				if a.localMonitor { statusPill(c, "系统指标采样", t.Warning) } else { statusPill(c, "系统流已连接", t.Success) }
			} else {
				statusPill(c, "系统流未连接", t.Warning)
				if a.systemErr != "" {
					ui.Text(c, a.systemErr).FontSize(10).MaxLines(4).TextColor(t.TextMuted)
				}
			}
		})
		if a.systemConnected {
			card(c, "系统负载", func() {
				ui.Text(c, fmt.Sprintf("CPU  %.1f%%", a.system.CPUTotal)).Bold()
				ui.Text(c, fmt.Sprintf("内存  %.1f%%", a.system.MemPercent)).Bold()
				ui.Text(c, fmt.Sprintf("实时进程  %d", len(a.system.Processes))).FontSize(11).TextColor(t.TextMuted)
			})
		}
		ui.Text(c, "没有告警不代表所有活动都被采集。").FontSize(11).TextColor(t.TextMuted)
		if !a.localMonitor { ui.Row(c).Gap(7).Wrap().Children(func() {
			if ui.Button(c, "采集设置").Clicked() { a.page = "监控" }
			if ui.Button(c, "eBPF 模块").Clicked() { a.page = "eBPF 模块" }
		}) }
	})
}
