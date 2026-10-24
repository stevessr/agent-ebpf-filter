package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// The desktop intentionally consumes the existing, authenticated and redacted
// AgentSight API. No second eBPF collector or plaintext interception is needed.
type agentSightUsageSession struct {
	ID                 string `json:"id"`
	EventCount         int    `json:"event_count"`
	UsageEvents        int    `json:"usage_events"`
	InputTokens        uint64 `json:"input_tokens"`
	OutputTokens       uint64 `json:"output_tokens"`
	CacheReadTokens    uint64 `json:"cache_read_tokens"`
	CacheCreationTokens uint64 `json:"cache_creation_tokens"`
	LastEventTimestamp int64  `json:"last_event_timestamp"`
}

type agentSightUsageModel struct {
	Model               string `json:"model"`
	UsageEvents         int    `json:"usage_events"`
	InputTokens         uint64 `json:"input_tokens"`
	OutputTokens        uint64 `json:"output_tokens"`
	CacheReadTokens     uint64 `json:"cache_read_tokens"`
	CacheCreationTokens uint64 `json:"cache_creation_tokens"`
}

type agentSightUsageSummary struct {
	EventCount          int                      `json:"event_count"`
	UnattributedEvents  int                      `json:"unattributed_events"`
	UsageEvents         int                      `json:"usage_events"`
	InputTokens         uint64                   `json:"input_tokens"`
	OutputTokens        uint64                   `json:"output_tokens"`
	CacheReadTokens     uint64                   `json:"cache_read_tokens"`
	CacheCreationTokens uint64                   `json:"cache_creation_tokens"`
	Sessions            []agentSightUsageSession `json:"sessions"`
	Models              []agentSightUsageModel   `json:"models"`
	Coverage            string                   `json:"coverage"`
}

type agentSightUsageResponse struct {
	Source  string                 `json:"source"`
	Limit   int                    `json:"limit"`
	Summary agentSightUsageSummary `json:"summary"`
}

const agentSightDesktopEventLimit = 2000

func (c *apiClient) agentSightUsage(ctx context.Context) (agentSightUsageResponse, error) {
	var response agentSightUsageResponse
	err := c.getJSON(ctx, "/api/v1/agentsight/report/usage?limit="+strconv.Itoa(agentSightDesktopEventLimit), &response)
	return response, err
}

// Trigger on first view, periodically while visible, and on explicit refresh.
// The timestamp also rate-limits failed requests (including 404 or 401).
func (a *renewApp) requestAgentSightUsage(force bool) {
	if a.client == nil || a.agentSightLoading {
		return
	}
	if !force && !a.agentSightChecked.IsZero() && time.Since(a.agentSightChecked) < 15*time.Second {
		return
	}
	a.agentSightLoading = true
	a.agentSightChecked = time.Now()
	client := a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		report, err := client.agentSightUsage(ctx)
		a.update(func() {
			a.agentSightLoading = false
			if err != nil {
				a.agentSightError = err.Error()
				// Keep the previous successful snapshot visible but clearly
				// marked as stale if subsequent refreshes fail.
				return
			}
			a.agentSightReport = report
			a.agentSightReady = true
			a.agentSightError = ""
			a.agentSightSynced = time.Now()
			if a.agentSightSessionSelected >= len(report.Summary.Sessions) {
				a.agentSightSessionSelected = -1
			}
			if a.agentSightModelSelected >= len(report.Summary.Models) {
				a.agentSightModelSelected = -1
			}
		})
	}()
}

func agentSightModelLabel(model string) string {
	if strings.TrimSpace(model) == "" || model == "unknown" {
		return "模型未知"
	}
	return model
}

func agentSightSessionLabel(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "未知会话"
	}
	// IDs may be long; do not lose their source distinction.
	if len(id) > 72 {
		return id[:44] + "…" + id[len(id)-18:]
	}
	return id
}

// Only native run IDs can be mapped to the already-implemented event filter.
// A trace-only or imported session is not proof of the same run identity.
func agentSightEventSessionKey(id string) (string, bool) {
	id = strings.TrimSpace(id)
	if id == "" || strings.HasPrefix(id, "trace:") {
		return "", false
	}
	return "run:" + id, true
}

func agentSightUsageCoverageText(report agentSightUsageSummary) string {
	if report.EventCount == 0 {
		return "当前窗口内没有可用事件，不能判断 Token 消耗。"
	}
	if report.UsageEvents == 0 {
		return fmt.Sprintf("已读取 %d 条事件，但未发现明确的模型 Token 用量字段；Token 使用量未知。", report.EventCount)
	}
	return fmt.Sprintf("已读取 %d 条事件，其中 %d 条包含用量证据；%d 条未能归因至具体会话。缺失记录不计为零消耗。",
		report.EventCount, report.UsageEvents, report.UnattributedEvents)
}

func (a *renewApp) agentSightView(c *ui.Context) {
	t := c.Theme()
	a.requestAgentSightUsage(false)
	ui.Row(c).Gap(12).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(245).Gap(4).Children(func() {
			ui.Text(c, "AgentSight · 行为洞察").FontSize(27).Bold()
			ui.Text(c, "会话归因、模型 Token、缓存用量与 Agent 轨迹调查").TextColor(t.TextMuted)
		})
		if a.agentSightLoading {
			ui.Badge(c, "正在同步")
		} else if a.agentSightError != "" {
			statusPill(c, "同步异常", t.Warning)
		} else if a.agentSightReady {
			statusPill(c, "已同步", t.Success)
		}
		if ui.Button(c, "刷新报告").Tooltip("重新查询已有的 AgentSight 事件元数据，不会新增采集或改变拦截策略").Clicked() {
			a.requestAgentSightUsage(true)
		}
	})
	if a.agentSightError != "" {
		card(c, "用量接口异常", func() {
			ui.Text(c, a.agentSightError).TextColor(t.Warning)
			ui.Text(c, "确认后端启用了 AgentSight 模块，且 Desktop 使用的访问令牌具有读取权限。").FontSize(12).TextColor(t.TextMuted)
			if a.agentSightReady {
				ui.Text(c, "下方为上次成功同步的数据，不代表当前状态。").FontSize(12).TextColor(t.TextMuted)
			}
		})
	}
	if !a.agentSightReady {
		if a.agentSightError == "" {
			ui.Text(c, "正在读取已保留的 AgentSight 事件与模型调用用量…").TextColor(t.TextMuted)
		}
		return
	}
	summary := a.agentSightReport.Summary
	ui.Text(c, agentSightUsageCoverageText(summary)).FontSize(12).TextColor(t.TextMuted)
	if !a.agentSightSynced.IsZero() {
		ui.Text(c, "最近同步 "+a.agentSightSynced.Format("15:04:05")+" · 最多 "+strconv.Itoa(a.agentSightReport.Limit)+" 条事件 · 只读统计").
			FontSize(11).TextColor(t.TextMuted)
	}
	ui.Row(c).Gap(12).Wrap().Children(func() {
		statCard(c, "输入 Token", strconv.FormatUint(summary.InputTokens, 10), "仅统计有用量证据的调用")
		statCard(c, "输出 Token", strconv.FormatUint(summary.OutputTokens, 10), "缺失用量不记为零")
		statCard(c, "缓存读取", strconv.FormatUint(summary.CacheReadTokens, 10), "已明确采集的计数")
		statCard(c, "缓存创建", strconv.FormatUint(summary.CacheCreationTokens, 10), "已明确采集的计数")
	})
	ui.Row(c).Gap(10).Wrap().Children(func() {
		ui.Badge(c, fmt.Sprintf("已归因会话 %d", len(summary.Sessions)))
		ui.Badge(c, fmt.Sprintf("模型 %d", len(summary.Models)))
		ui.Badge(c, fmt.Sprintf("用量事件 %d / %d", summary.UsageEvents, summary.EventCount))
		if ui.Button(c, "查看会话活动").Tooltip("跳转原有的 Agent 进程与行为会话页").Clicked() {
			a.page = "会话"
		}
		if ui.Button(c, "事件时间线").Tooltip("打开原有事件页查看时间、决策和追踪证据").Clicked() {
			a.clearEventFilters()
			a.page = "事件"
		}
	})
	card(c, fmt.Sprintf("模型用量 · %d", len(summary.Models)), func() {
		if len(summary.Models) == 0 {
			ui.Text(c, "当前没有带明确 Token 用量的模型调用；网络连接不能推断出实际调用次数或 Token 数。").
				TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "模型", MinWidth: 190, Fixed: true},
			{Title: "用量事件", Width: 100, Align: ui.End},
			{Title: "输入", Width: 100, Align: ui.End},
			{Title: "输出", Width: 100, Align: ui.End},
			{Title: "缓存读取", Width: 110, Align: ui.End},
			{Title: "缓存创建", Width: 110, Align: ui.End},
		}
		a.agentSightModelTable.Key = func(row int) any { return summary.Models[row].Model }
		ui.Table(c, &a.agentSightModelTable, cols, len(summary.Models), func(row, col int) {
			model := summary.Models[row]
			switch col {
			case 0:
				ui.Text(c, agentSightModelLabel(model.Model)).SingleLine()
			case 1:
				ui.Text(c, strconv.Itoa(model.UsageEvents))
			case 2:
				ui.Text(c, strconv.FormatUint(model.InputTokens, 10))
			case 3:
				ui.Text(c, strconv.FormatUint(model.OutputTokens, 10))
			case 4:
				ui.Text(c, strconv.FormatUint(model.CacheReadTokens, 10))
			case 5:
				ui.Text(c, strconv.FormatUint(model.CacheCreationTokens, 10))
			}
		}).Height(210).Label("AgentSight 模型 Token 用量")
	})
	card(c, fmt.Sprintf("会话用量与关联 · %d", len(summary.Sessions)), func() {
		if len(summary.Sessions) == 0 {
			ui.Text(c, "暂无带有运行或追踪标识的事件；普通 shell 事件不会自动被认为属于某个 Agent 会话。").
				TextColor(t.TextMuted)
			return
		}
		cols := []ui.TableColumn{
			{Title: "会话 ID", MinWidth: 205, Fixed: true},
			{Title: "事件", Width: 75, Align: ui.End},
			{Title: "用量证据", Width: 90, Align: ui.End},
			{Title: "输入", Width: 95, Align: ui.End},
			{Title: "输出", Width: 95, Align: ui.End},
			{Title: "最后时间", Width: 105},
		}
		a.agentSightSessionTable.Key = func(row int) any { return summary.Sessions[row].ID }
		ui.Table(c, &a.agentSightSessionTable, cols, len(summary.Sessions), func(row, col int) {
			session := summary.Sessions[row]
			switch col {
			case 0:
				ui.Text(c, agentSightSessionLabel(session.ID)).Tooltip(session.ID).SingleLine()
			case 1:
				ui.Text(c, strconv.Itoa(session.EventCount))
			case 2:
				ui.Text(c, strconv.Itoa(session.UsageEvents))
			case 3:
				ui.Text(c, strconv.FormatUint(session.InputTokens, 10))
			case 4:
				ui.Text(c, strconv.FormatUint(session.OutputTokens, 10))
			case 5:
				ui.Text(c, summaryTime(session.LastEventTimestamp)).SingleLine()
			}
		}).Height(310).Label("AgentSight Agent 会话报告")
		if a.agentSightSessionSelected >= 0 && a.agentSightSessionSelected < len(summary.Sessions) {
			selected := summary.Sessions[a.agentSightSessionSelected]
			ui.Row(c).Gap(10).Wrap().AlignItems(ui.Center).Children(func() {
				ui.Text(c, "选中 "+agentSightSessionLabel(selected.ID)).Grow(1).SingleLine()
				if key, ok := agentSightEventSessionKey(selected.ID); ok {
					if ui.PrimaryButton(c, "调查本会话事件").Tooltip("只跳转已有运行 ID 证据，不推断跨进程关联").Clicked() {
						a.navigateSessionEvents(key, false, false)
					}
				} else {
					ui.Text(c, "仅有 trace 标识，无法可靠映射到原生会话筛选").FontSize(11).TextColor(t.TextMuted)
				}
			})
		}
	})
	card(c, "采集与隐私", func() {
		ui.Text(c, "报告仅使用后端现有的脱敏事件元数据，不读取 prompt 正文或 TLS 明文，也不自动执行阻断。").
			FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, "同一模型可能产生多个事件；用量事件数不等于 API 调用数。未采集完整用量时不能据此计算总费用。").
			FontSize(12).TextColor(t.TextMuted)
	})
}
