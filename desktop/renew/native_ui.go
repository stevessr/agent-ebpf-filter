package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type renewApp struct {
	win     *mygo.Window
	backend string
	client  *apiClient

	starting bool
	connected bool
	lastErr string
	lastSync time.Time

	page   string
	search string
	paused bool

	events       []eventSummary
	health       collectorHealth
	trackedComms []string
}

func newRenewApp(backend string) *renewApp {
	return &renewApp{
		backend:  backend,
		starting: true,
		page:     "概览",
	}
}

func (a *renewApp) runPolling(ctx context.Context) {
	a.refresh(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !a.paused {
				a.refresh(ctx)
			}
		}
	}
}

func (a *renewApp) refresh(parent context.Context) {
	if a.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	snapshot, err := a.client.snapshot(ctx, 320)
	if err != nil {
		a.update(func() {
			a.connected = false
			a.lastErr = err.Error()
		})
		return
	}
	a.update(func() {
		a.connected = true
		a.lastErr = ""
		a.events = snapshot.Events
		a.health = snapshot.Health
		a.trackedComms = snapshot.TrackedComms
		a.lastSync = snapshot.FetchedAt
	})
}

func (a *renewApp) view(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			a.header(c)
			ui.Scroll(c).Grow(1).Padding(24).Gap(18).Children(func() {
				if a.starting {
					a.startingView(c)
					return
				}
				if a.lastErr != "" && len(a.events) == 0 {
					a.errorView(c)
					return
				}
				switch a.page {
				case "事件":
					a.eventsView(c)
				case "系统":
					a.systemView(c)
				default:
					a.overview(c)
				}
			})
		})
	})
}

func (a *renewApp) sidebar(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Width(214).Shrink(0).Padding(18, 14).Gap(14).Background(t.Surface).Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(34, 34).Radius(10).Background(t.Accent).Center().Children(func() {
				ui.Text(c, "R").Bold().TextColor(t.Background)
			})
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, "Renew").FontSize(16).Bold()
				ui.Text(c, "Agent 日常监控").FontSize(11).TextColor(t.TextMuted)
			})
		})
		ui.Column(c).Gap(6).Children(func() {
			for _, page := range []string{"概览", "事件", "系统"} {
				button := ui.Button(c, page).Width(184)
				if page == a.page {
					button = ui.PrimaryButton(c, page).Width(184)
				}
				if button.Clicked() {
					a.page = page
				}
			}
		})
		ui.Box(c).Grow(1)
		ui.Column(c).Gap(4).Children(func() {
			status := "未连接"
			if a.starting {
				status = "正在启动"
			} else if a.connected {
				status = "后端已连接"
			}
			ui.Text(c, status).FontSize(12).Bold()
			ui.Text(c, a.backend).FontSize(10).TextColor(t.TextMuted).MaxLines(2)
		})
	})
}

func (a *renewApp) header(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Height(58).Padding(10, 20).Gap(12).AlignItems(ui.Center).Background(t.Surface).Children(func() {
		ui.Text(c, a.page).FontSize(18).Bold()
		ui.Box(c).Grow(1)
		ui.SearchField(c, &a.search).Label("搜索事件").Width(280)
		label := "暂停"
		if a.paused {
			label = "继续"
		}
		if ui.Button(c, label).Clicked() {
			a.paused = !a.paused
			if !a.paused {
				go a.refresh(context.Background())
			}
		}
		if ui.Button(c, "刷新").Clicked() {
			go a.refresh(context.Background())
		}
	})
}

func (a *renewApp) startingView(c *ui.Context) {
	t := c.Theme()
	card(c, "正在启动 Agent eBPF Filter", func() {
		ui.Text(c, "Renew 正在复用现有后端，或通过系统授权启动随包后端。桌面 UI 本身保持普通用户权限。").TextColor(t.TextMuted)
		ui.Spinner(c)
	})
}

func (a *renewApp) errorView(c *ui.Context) {
	t := c.Theme()
	card(c, "后端未就绪", func() {
		ui.Text(c, a.lastErr).TextColor(t.TextMuted)
		ui.Text(c, "可通过 AGENT_BACKEND_URL / --backend 指向其他实例；远程受保护实例可通过 AGENT_API_TOKEN 提供 token。").FontSize(12).TextColor(t.TextMuted)
		if ui.PrimaryButton(c, "重试").Clicked() {
			go a.bootstrap(context.Background())
		}
	})
}

func (a *renewApp) overview(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "现在正常吗？").FontSize(28).Bold()
	ui.Text(c, "原生 Go UI 直接读取 Agent eBPF Filter 后端，不启动 WebView。").TextColor(t.TextMuted)

	normal, attention, danger := a.riskCounts()
	ui.Row(c).Gap(12).Wrap().Children(func() {
		statCard(c, "采集状态", map[bool]string{true: "正常", false: "异常"}[a.health.CaptureHealthy], fmt.Sprintf("Ringbuf 丢弃 %d", a.health.RingbufDroppedTotal))
		statCard(c, "最近活动", fmt.Sprint(len(a.events)), "当前紧凑摘要窗口")
		statCard(c, "需关注", fmt.Sprint(attention), "风险分 ≥ 60 / ALERT")
		statCard(c, "高风险", fmt.Sprint(danger), "BLOCK / DENY / 高风险")
		_ = normal
	})

	card(c, "最近活动", func() {
		filtered := a.filteredEvents()
		if len(filtered) == 0 {
			ui.Text(c, "暂无匹配活动").TextColor(t.TextMuted)
			return
		}
		limit := len(filtered)
		if limit > 12 {
			limit = 12
		}
		for _, event := range filtered[:limit] {
			a.eventRow(c, event)
		}
		if len(filtered) > limit {
			ui.Textf(c, "还有 %d 条，可在“事件”页查看", len(filtered)-limit).FontSize(11).TextColor(t.TextMuted)
		}
	})

	card(c, "已跟踪进程", func() {
		if len(a.trackedComms) == 0 {
			ui.Text(c, "后端未返回显式跟踪列表").TextColor(t.TextMuted)
			return
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			for _, name := range a.trackedComms {
				ui.Badge(c, name)
			}
		})
	})
}

func (a *renewApp) eventsView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "事件").FontSize(28).Bold()
	ui.Text(c, "当前仅保留紧凑摘要；完整事件仍由后端持久化。").TextColor(t.TextMuted)
	filtered := a.filteredEvents()
	card(c, fmt.Sprintf("活动 · %d", len(filtered)), func() {
		if len(filtered) == 0 {
			ui.Text(c, "没有匹配事件").TextColor(t.TextMuted)
			return
		}
		for _, event := range filtered {
			a.eventRow(c, event)
		}
	})
}

func (a *renewApp) systemView(c *ui.Context) {
	t := c.Theme()
	ui.Text(c, "系统").FontSize(28).Bold()
	card(c, "采集器", func() {
		state := "异常"
		if a.health.CaptureHealthy {
			state = "正常"
		}
		ui.Text(c, state).FontSize(22).Bold()
		ui.Textf(c, "Ringbuf 丢弃总数：%d", a.health.RingbufDroppedTotal).TextColor(t.TextMuted)
		if !a.lastSync.IsZero() {
			ui.Text(c, "最后同步："+a.lastSync.Format("15:04:05")).FontSize(12).TextColor(t.TextMuted)
		}
	})
	card(c, "连接", func() {
		ui.Text(c, a.backend).Font("monospace")
		ui.Text(c, "桌面端为纯 Go/MyGo Native UI；专业工作台仍可单独在浏览器打开。").TextColor(t.TextMuted)
	})
}

func (a *renewApp) eventRow(c *ui.Context, e eventSummary) {
	t := c.Theme()
	ui.Row(c).Padding(9, 0).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, eventTime(e)).Width(66).Font("monospace").FontSize(11).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, eventAction(e)).Bold()
				if e.Comm != "" {
					ui.Badge(c, e.Comm)
				}
				risk := eventRisk(e)
				if risk != "正常" {
					ui.Badge(c, risk)
				}
			})
			ui.Text(c, eventTarget(e)).FontSize(12).TextColor(t.TextMuted).MaxLines(1)
		})
		if e.RiskScore > 0 {
			ui.Textf(c, "%.0f", e.RiskScore).Width(36).Font("monospace").TextColor(t.TextMuted)
		}
	})
}

func (a *renewApp) filteredEvents() []eventSummary {
	q := strings.ToLower(strings.TrimSpace(a.search))
	if q == "" {
		return a.events
	}
	out := make([]eventSummary, 0, len(a.events))
	for _, event := range a.events {
		if strings.Contains(eventSearchText(event), q) {
			out = append(out, event)
		}
	}
	return out
}

func (a *renewApp) riskCounts() (normal, attention, danger int) {
	for _, event := range a.events {
		switch eventRisk(event) {
		case "高风险":
			danger++
		case "需关注":
			attention++
		default:
			normal++
		}
	}
	return
}

func card(c *ui.Context, title string, body func()) *ui.Element {
	t := c.Theme()
	return ui.Column(c).Padding(18).Gap(12).Radius(12).Background(t.Surface).Border(1, t.Border).Children(func() {
		if title != "" {
			ui.Text(c, title).FontSize(15).Bold()
		}
		body()
	})
}

func statCard(c *ui.Context, label, value, detail string) {
	t := c.Theme()
	card(c, label, func() {
		ui.Text(c, value).FontSize(26).Bold()
		ui.Text(c, detail).FontSize(11).TextColor(t.TextMuted)
	}).Width(220)
}
