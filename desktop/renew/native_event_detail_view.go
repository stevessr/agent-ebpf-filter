package main

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// Compact cards use a vertical label/value stack; otherwise a 126-DIP label
// and clipboard button leave no readable value space on small windows.
func eventDetailFieldRowAdaptive(c *ui.Context, field eventDetailField, width float32, expandedKey *string) {
	t := c.Theme()
	expanded := expandedKey != nil && *expandedKey == field.Origin
	visible := eventDetailPreview(field.Value, 280)
	if expanded {
		visible = field.Value
	}
	showValue := func() {
		if expanded {
			ui.Text(c, field.Value).Font("monospace").FontSize(11)
		} else {
			ui.Text(c, visible).Font("monospace").FontSize(11).MaxLines(3)
		}
		ui.Text(c, field.Origin).FontSize(9).TextColor(t.TextMuted)
	}
	showActions := func() {
		if len([]rune(field.Value)) > 280 && expandedKey != nil {
			label := "展开"
			if expanded {
				label = "收起"
			}
			if ui.Button(c, label).Clicked() {
				if expanded {
					*expandedKey = ""
				} else {
					*expandedKey = field.Origin
				}
			}
		}
		if ui.Button(c, "复制").Tooltip("复制" + field.Label + "的完整值").Clicked() {
			c.WriteClipboard(field.Value)
			c.Toast("已复制 " + field.Label)
		}
	}
	if width < 440 {
		ui.Column(c).Gap(4).Padding(0, 0, 5, 0).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, field.Label).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0)
				showActions()
			})
			showValue()
		})
		return
	}
	ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Text(c, field.Label).Width(112).Shrink(0).FontSize(11).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(showValue)
		showActions()
	})
}

// Existing small component tests retain the basic simple row entry point.
func eventDetailFieldRow(c *ui.Context, field eventDetailField) {
	eventDetailFieldRowAdaptive(c, field, 520, nil)
}

func eventDetailSectionCard(c *ui.Context, section eventDetailSection, width float32, expandedKey *string) {
	card(c, section.Title, func() {
		for _, field := range section.Fields {
			eventDetailFieldRowAdaptive(c, field, width, expandedKey)
		}
	}).Width(width)
}

// Show one continuous investigation, instead of a pair of large mostly-empty
// cards. Layout width is driven by the viewport, not a fixed 760 DIP.
func (a *renewApp) richEventDetail(c *ui.Context, detail map[string]any, width float32) {
	t := c.Theme()
	model := eventDetailModel(detail)
	card(c, "事件概况", func() {
		ui.Row(c).Gap(7).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, model.Action).FontSize(17).Bold()
			if model.Type != "" {
				ui.Badge(c, model.Type)
			}
			if model.Decision != "" {
				switch strings.ToUpper(model.Decision) {
				case "BLOCK", "DENY":
					statusPill(c, model.Decision, t.Danger)
				case "ALERT":
					statusPill(c, model.Decision, t.Warning)
				default:
					ui.Badge(c, model.Decision)
				}
			}
			if model.Risk != "" {
				score, err := strconv.ParseFloat(model.Risk, 64)
				if err == nil {
					riskPill(c, eventRisk(eventSummary{RiskScore: score, Decision: model.Decision}))
				}
				ui.Badge(c, "风险分 "+model.Risk)
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, eventDetailPreview(model.Target, 520)).Font("monospace").FontSize(12).Grow(1).MinWidth(0).MaxLines(3)
			// A caveat must not hide a valid target's copy action; only a
			// missing or non-applicable target should disable it.
			if model.Target != "" && model.Target != "目标路径或端点未记录" &&
				!model.NoTargetExpected && ui.Button(c, "复制目标").Clicked() {
				c.WriteClipboard(model.Target)
				c.Toast("目标已复制")
			}
		})
		if model.EvidenceNote != "" {
			tone := t.Warning
			if model.NoTargetExpected {
				tone = t.TextMuted
			}
			ui.Text(c, model.EvidenceNote).FontSize(11).TextColor(tone)
		}
		if model.When != "" {
			ui.Text(c, model.WhenLabel+"  "+model.When).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Text(c, "字段来源与脱敏状态以记录为准；没有采集到的内容不进行推断。").FontSize(10).TextColor(t.TextMuted)
	})
	if width >= 760 {
		leftWidth := (width - 46) / 2
		ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
			for col := 0; col < 2; col++ {
				ui.Column(c).Width(leftWidth).Gap(12).Children(func() {
					for index, section := range model.Sections {
						if index%2 == col {
							eventDetailSectionCard(c, section, leftWidth, &a.eventDetailExpandedField)
						}
					}
				})
			}
		})
	} else {
		for _, section := range model.Sections {
			eventDetailSectionCard(c, section, width-12, &a.eventDetailExpandedField)
		}
	}
	a.eventDetailRelated(c, detail)
}

// Model the current event independently from the UI, preserving JSON
// timestamps and preferring persisted evidence over live summary fallbacks.
func eventDetailSelectedSummary(detail map[string]any, eventID string, retained []eventSummary) eventSummary {
	layers := eventDetailLayers(detail)
	read := func(key string) string {
		v, _, _ := eventDetailLookup(layers, key)
		return v
	}
	selected := eventSummary{
		EventID:        eventID,
		AgentRunID:     read("agentRunId"),
		ConversationID: read("conversationId"),
		Type:           read("type"),
		Comm:           read("comm"),
		Tag:            read("tag"),
	}
	if pid, err := strconv.Atoi(read("pid")); err == nil && pid > 0 {
		selected.PID = pid
	}
	if pid, err := strconv.Atoi(read("ppid")); err == nil && pid > 0 {
		selected.PPID = pid
	}
	if pid, err := strconv.Atoi(read("rootAgentPid")); err == nil && pid > 0 {
		selected.RootAgentPID = pid
	}
	if target := eventDetailModel(detail).Target; usableEventTarget(target) {
		selected.Target = target
	}
	selected.Network = isNetworkEvent(selected)
	// fmt.Sprint of a JSON float64 can yield exponent notation, which is
	// invalid for a decimal timestamp. Our scalar lookup preserves decimals.
	if text, _, exists := eventDetailLookup(
		[]eventDetailLayer{newEventDetailLayer("$", detail)}, "Timestamp",
	); exists {
		if ms, ok := eventDetailInt64(text); ok && ms > 0 {
			selected.ReceivedAtMS = ms
		}
	}
	for _, item := range retained {
		if item.EventID != selected.EventID {
			continue
		}
		if selected.PID == 0 {
			selected.PID = item.PID
		}
		if selected.PPID == 0 {
			selected.PPID = item.PPID
		}
		if selected.RootAgentPID == 0 {
			selected.RootAgentPID = item.RootAgentPID
		}
		if selected.Type == "" {
			selected.Type = item.Type
		}
		if selected.Comm == "" {
			selected.Comm = item.Comm
		}
		if selected.Tag == "" {
			selected.Tag = item.Tag
		}
		if selected.Target == "" {
			selected.Target = item.Target
		}
		if selected.ReceivedAtMS == 0 {
			selected.ReceivedAtMS = item.ReceivedAtMS
		}
		if selected.AgentRunID == "" {
			selected.AgentRunID = item.AgentRunID
		}
		if selected.ConversationID == "" {
			selected.ConversationID = item.ConversationID
		}
		selected.Network = selected.Network || item.Network
		break
	}
	return selected
}

func (a *renewApp) eventDetailRelated(c *ui.Context, detail map[string]any) {
	selected := eventDetailSelectedSummary(detail, a.eventDetailID, a.events)
	if selected.PID <= 0 && selected.AgentRunID == "" && selected.ConversationID == "" {
		return
	}
	related := eventDetailRelatedCandidates(a.events, selected, 4)
	if len(related) == 0 {
		return
	}
	card(c, "邻近事件 · 当前缓存", func() {
		ui.Text(c, "优先使用 Agent Run/会话标识关联跨进程活动；仅在两分钟窗口内匹配无冲突的同 PID 记录。这些事件不代表因果关系。").FontSize(10).TextColor(c.Theme().TextMuted)
		for _, entry := range related {
			entry := entry
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Badge(c, entry.Relation)
				ui.Text(c, summaryTime(entry.Event.ReceivedAtMS)+"  "+displayOr(entry.Event.Type, "event")).FontSize(11)
				ui.Text(c, eventTarget(entry.Event)).MaxLines(1).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0)
				if ui.Button(c, "查看").Clicked() {
					a.openEventDetail(entry.Event.EventID)
				}
			})
		}
	})
}
