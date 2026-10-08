package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
)

type agentSessionSummary struct {
	Key        string
	Label      string
	Events     int
	Alerts     int
	LastSeen   int64
	LastAction string
}

var harnessLabels = map[string]string{
	"codex": "Codex", "codex-code-mode": "Codex", "codex-code-mode-host": "Codex",
	"claude": "Claude Code", "claude code": "Claude Code", "claude-code": "Claude Code",
	"gemini": "Gemini CLI", "gemini cli": "Gemini CLI", "gemini-cli": "Gemini CLI",
	"dsh": "DeepSeek Harness", "deepseek-harness": "DeepSeek Harness", "deepseek harness": "DeepSeek Harness",
	"copilot": "GitHub Copilot", "github copilot": "GitHub Copilot",
	"cursor": "Cursor", "cursor-agent": "Cursor",
	"opencode": "OpenCode",
	"pi": "Pi", "pi-coding-agent": "Pi",
	"omp": "Oh My Pi", "oh my pi": "Oh My Pi", "oh-my-pi": "Oh My Pi",
	"kiro": "Kiro CLI", "kiro-cli": "Kiro CLI", "kiro cli": "Kiro CLI",
	"augment": "Augment", "auggie": "Augment",
	"agy": "Antigravity CLI", "antigravity": "Antigravity CLI", "antigravity cli": "Antigravity CLI",
	"zcode": "ZCode", "zcode.appimage": "ZCode",
	"mcode": "MiniMax Code", "minimax code": "MiniMax Code",
}

func eventHarnessLabel(event eventSummary) string {
	return harnessLabelFor(event.Tag, event.Comm)
}

func isAgentSummary(event eventSummary) bool {
	return event.HasAgentContext ||
		event.AgentRunID != "" ||
		event.ConversationID != "" ||
		event.RootAgentPID > 0 ||
		(strings.TrimSpace(event.Tag) != "" && !strings.EqualFold(event.Tag, "Unknown"))
}

func eventSessionKey(event eventSummary) string {
	contextID := strings.Join(nonEmptyStrings(event.AgentRunID, event.ConversationID), ":")
	root := event.RootAgentPID
	if root <= 0 {
		root = event.PID
	}
	harness := eventHarnessLabel(event)
	if contextID == "" {
		return harness + " · PID " + strconv.Itoa(root)
	}
	if harness == "未识别" {
		return harness + " · " + contextID + " · PID " + strconv.Itoa(root)
	}
	return harness + " · " + contextID
}

func nonEmptyStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}

func aggregateAgentSessions(events []eventSummary) []agentSessionSummary {
	byKey := make(map[string]*agentSessionSummary)
	for _, event := range events {
		if !isAgentSummary(event) {
			continue
		}
		key := eventSessionKey(event)
		session := byKey[key]
		if session == nil {
			session = &agentSessionSummary{Key: key, Label: key}
			byKey[key] = session
		}
		session.Events++
		if eventRisk(event) != "正常" || event.Type == "semantic_alert" || event.Type == "agentsight_alert" {
			session.Alerts++
		}
		if event.ReceivedAtMS >= session.LastSeen {
			session.LastSeen = event.ReceivedAtMS
			action := eventAction(event)
			target := eventTarget(event)
			if target != "-" {
				action += " · " + target
			}
			session.LastAction = action
		}
	}
	out := make([]agentSessionSummary, 0, len(byKey))
	for _, session := range byKey {
		out = append(out, *session)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeen > out[j].LastSeen })
	return out
}

func (a *renewApp) sessionsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "Agent 会话").FontSize(28).Bold()
	ui.Text(c, "把事件按 Agent 运行上下文归并，优先发现哪一个会话正在产生需要关注的行为。").TextColor(t.TextMuted)

	rows := aggregateAgentSessions(a.events)
	q := strings.ToLower(strings.TrimSpace(a.search))
	if q != "" {
		filtered := rows[:0]
		for _, row := range rows {
			if strings.Contains(strings.ToLower(row.Label+" "+row.LastAction), q) {
				filtered = append(filtered, row)
			}
		}
		rows = filtered
	}

	totalAlerts := 0
	for _, row := range rows {
		totalAlerts += row.Alerts
	}
	ui.Row(c).Gap(12).Wrap().Children(func() {
		statCard(c, "活动会话", strconv.Itoa(len(rows)), "当前摘要窗口")
		statCard(c, "会话告警", strconv.Itoa(totalAlerts), "需关注或高风险活动")
		stream := "回退同步"
		if a.eventStreamConnected {
			stream = "实时"
		}
		statCard(c, "事件流", stream, fmt.Sprintf("内存摘要 %d / 1200", len(a.events)))
	})

	card(c, fmt.Sprintf("活动会话 · %d", len(rows)), func() {
		if len(rows) == 0 {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "暂无可归并的 Agent 会话；确认目标命令已经加入跟踪范围。").TextColor(t.TextMuted).Grow(1)
				if ui.Button(c, "打开跟踪范围").Clicked() {
					a.page = "跟踪"
				}
			})
			return
		}
		cols := []ui.TableColumn{
			{Title: "会话", MinWidth: 280, Fixed: true},
			{Title: "动作", Width: 80, Align: ui.End},
			{Title: "告警", Width: 80, Align: ui.End},
			{Title: "最近动作", MinWidth: 260},
			{Title: "最后活动", Width: 100},
		}
		a.sessionTable.Key = func(row int) any { return rows[row].Key }
		ui.Table(c, &a.sessionTable, cols, len(rows), func(row, col int) {
			session := rows[row]
			switch col {
			case 0:
				harnessIdentity(c, session.Label, strings.SplitN(session.Label, " · ", 2)[0])
			case 1:
				ui.Text(c, strconv.Itoa(session.Events))
			case 2:
				if session.Alerts > 0 {
					statusPill(c, strconv.Itoa(session.Alerts), t.Warning)
				} else {
					ui.Text(c, "0").TextColor(t.TextMuted)
				}
			case 3:
				ui.Text(c, displayOr(session.LastAction, "-")).SingleLine()
			case 4:
				ui.Text(c, summaryTime(session.LastSeen)).SingleLine()
			}
		}).Height(430).Label("Agent 会话")

		if a.sessionSelected >= 0 && a.sessionSelected < len(rows) {
			selected := rows[a.sessionSelected]
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				harnessIdentity(c, selected.Label, strings.SplitN(selected.Label, " · ", 2)[0])
				if ui.PrimaryButton(c, "查看此会话事件").Clicked() {
					a.clearEventFilters()
					a.eventSessionFilter = selected.Key
					a.page = "事件"
				}
			})
		}
	})

	card(c, "数据来源", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if a.eventStreamConnected {
				statusPill(c, "实时事件流", t.Success)
			} else {
				statusPill(c, "兼容回退", t.Warning)
			}
			ui.Text(c, fmt.Sprintf("内存中保留 %d 条紧凑摘要，最多 1200 条。", len(a.events))).TextColor(t.TextMuted)
		})
		if a.eventStreamErr != "" && !a.eventStreamConnected {
			ui.Text(c, a.eventStreamErr).FontSize(10).TextColor(t.TextMuted)
		}
	})
}
