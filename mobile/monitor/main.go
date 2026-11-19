// Renew Monitor is a small read-only MyGo client for a separately hosted
// Agent eBPF backend. It never starts collectors or modifies server policy.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/stevessr/agent-ebpf-filter/mobile/monitor/remote"
)

const (
	basePollInterval = 6 * time.Second
	maxPollInterval  = 30 * time.Second
)

type fetchResult struct {
	generation     uint64
	initialConnect bool
	origin         string
	snapshot       remote.Snapshot
	err            error
}

type probeResult struct {
	generation uint64
	origin     string
	health     remote.Health
	latency    time.Duration
	err        error
}

type pageResult struct {
	generation uint64
	cursor     string
	page       remote.EventPage
	err        error
}

type detailResult struct {
	generation uint64
	eventID    string
	json       string
	err        error
}

type monitorApp struct {
	win    *mygo.Window
	origin string
	token  string
	client *remote.Client
	page   string
	search string
	filter string // "", "block", "risk", "agent"

	connecting          bool
	connected           bool
	authFailed          bool
	suspended           bool
	loading             bool
	probing             bool
	loadingMore         bool
	consecutiveFailures int
	lastErr             string
	probeMsg            string
	probeOK             bool
	snap                remote.Snapshot
	recentOrigins       []string

	epoch      uint64
	probeEpoch uint64
	lastPoll   time.Time
	results    chan fetchResult
	probes     chan probeResult
	morePages  chan pageResult
	details    chan detailResult
	scroll     ui.ScrollState
	detailID   string
	detailText string
	detailErr  string
	detailBusy bool
}

func newMonitorApp() *monitorApp {
	return &monitorApp{
		page:      "connect",
		origin:    "https://",
		results:   make(chan fetchResult, 8),
		probes:    make(chan probeResult, 4),
		morePages: make(chan pageResult, 4),
		details:   make(chan detailResult, 8),
	}
}

func (a *monitorApp) pollInterval() time.Duration {
	if a.consecutiveFailures <= 0 {
		return basePollInterval
	}
	shift := min(a.consecutiveFailures, 3)
	interval := basePollInterval * time.Duration(1<<shift)
	if interval > maxPollInterval {
		return maxPollInterval
	}
	return interval
}

func (a *monitorApp) recordVerifiedOrigin(origin string) {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return
	}
	a.recentOrigins = slices.DeleteFunc(a.recentOrigins, func(item string) bool {
		return strings.EqualFold(item, origin)
	})
	a.recentOrigins = append([]string{origin}, a.recentOrigins...)
	if len(a.recentOrigins) > 4 {
		a.recentOrigins = a.recentOrigins[:4]
	}
}

func (a *monitorApp) applyOpenURL(rawURL string) {
	orig, tok, err := remote.ParseConnectInput(rawURL, "")
	if err != nil {
		a.lastErr = remote.DescribeError(err, rawURL)
		a.page = "connect"
		return
	}
	a.origin = orig
	a.probeMsg = "已从连接链接导入服务地址"
	a.probeOK = true
	a.lastErr = ""
	if tok != "" {
		a.token = tok
		a.connect()
		return
	}
	a.page = "connect"
}

func (a *monitorApp) connect() {
	normalizedOrigin, normalizedToken, err := remote.ParseConnectInput(a.origin, a.token)
	if err != nil {
		a.lastErr = remote.DescribeError(err, a.origin)
		a.probeMsg = ""
		return
	}
	client, err := remote.NewClient(normalizedOrigin, normalizedToken)
	if err != nil {
		a.lastErr = remote.DescribeError(err, normalizedOrigin)
		a.probeMsg = ""
		return
	}
	a.origin = normalizedOrigin
	a.token = normalizedToken
	a.epoch++
	a.client = client
	a.connecting = true
	a.connected = false
	a.authFailed = false
	a.loading = false
	a.loadingMore = false
	a.consecutiveFailures = 0
	a.lastErr = ""
	a.probeMsg = ""
	a.snap = remote.Snapshot{}
	a.detailID, a.detailText, a.detailErr = "", "", ""
	a.detailBusy = false
	a.lastPoll = time.Time{}
	a.refreshWithMode(true)
}

func (a *monitorApp) probeConnection() {
	if a.probing || a.connecting {
		return
	}
	normalizedOrigin, normalizedToken, err := remote.ParseConnectInput(a.origin, a.token)
	if err != nil {
		a.lastErr = remote.DescribeError(err, a.origin)
		a.probeMsg = ""
		return
	}
	client, err := remote.NewClient(normalizedOrigin, normalizedToken)
	if err != nil {
		a.lastErr = remote.DescribeError(err, normalizedOrigin)
		a.probeMsg = ""
		return
	}
	a.origin = normalizedOrigin
	a.token = normalizedToken
	a.probeEpoch++
	a.probing = true
	a.lastErr = ""
	a.probeMsg = ""
	generation := a.probeEpoch
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()
		health, latency, probeErr := client.Probe(ctx)
		select {
		case a.probes <- probeResult{
			generation: generation,
			origin:     normalizedOrigin,
			health:     health,
			latency:    latency,
			err:        probeErr,
		}:
		default:
		}
	}()
}

func (a *monitorApp) disconnect() {
	a.epoch++
	a.probeEpoch++
	a.client = nil
	a.connecting = false
	a.connected = false
	a.authFailed = false
	a.loading = false
	a.probing = false
	a.loadingMore = false
	a.consecutiveFailures = 0
	a.lastErr = ""
	a.probeMsg = ""
	a.snap = remote.Snapshot{}
	a.detailID, a.detailText, a.detailErr = "", "", ""
	a.detailBusy = false
	a.token = "" // clear the credential from the editable field on disconnect
	a.page = "connect"
}

func (a *monitorApp) refresh() {
	a.refreshWithMode(!a.connected)
}

func (a *monitorApp) refreshWithMode(initialConnect bool) {
	if a.client == nil || a.loading {
		return
	}
	a.loading = true
	if initialConnect {
		a.connecting = true
	}
	a.lastPoll = time.Now()
	generation, client := a.epoch, a.client
	origin := client.Origin()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()
		snapshot, err := client.FetchSnapshot(ctx, 60)
		select {
		case a.results <- fetchResult{
			generation:     generation,
			initialConnect: initialConnect,
			origin:         origin,
			snapshot:       snapshot,
			err:            err,
		}:
		default:
		}
	}()
}

func (a *monitorApp) loadMoreEvents() {
	cursor := strings.TrimSpace(a.snap.NextCursor)
	if a.client == nil || !a.connected || a.loading || a.loadingMore || cursor == "" {
		return
	}
	a.loadingMore = true
	generation, client := a.epoch, a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		page, err := client.FetchEventsPage(ctx, 60, cursor)
		select {
		case a.morePages <- pageResult{
			generation: generation,
			cursor:     cursor,
			page:       page,
			err:        err,
		}:
		default:
		}
	}()
}

func (a *monitorApp) loadDetail(id string) {
	if id == "" || a.client == nil || a.detailBusy {
		return
	}
	if a.detailID == id {
		a.detailID, a.detailText, a.detailErr = "", "", ""
		return
	}
	a.detailID, a.detailText, a.detailErr = id, "", ""
	a.detailBusy = true
	client, generation := a.client, a.epoch
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer cancel()
		payload, err := client.FetchEventDetail(ctx, id)
		var result string
		if err == nil {
			data, encodeErr := json.MarshalIndent(payload, "", "  ")
			err = encodeErr
			result = string(data)
		}
		select {
		case a.details <- detailResult{generation: generation, eventID: id, json: result, err: err}:
		default:
		}
	}()
}

func (a *monitorApp) receiveResults() {
	for {
		select {
		case r := <-a.results:
			if r.generation != a.epoch {
				continue
			}
			a.loading = false
			a.connecting = false
			if r.err != nil {
				a.consecutiveFailures++
				a.authFailed = remote.IsAuthError(r.err)
				a.lastErr = remote.DescribeError(r.err, r.origin)
				if !a.connected {
					// Initial handshake failed: do not retain an unverified session
					// or keep polling in the background.
					a.client = nil
					a.page = "connect"
				}
			} else {
				wasConnected := a.connected
				a.connected = true
				a.authFailed = false
				a.consecutiveFailures = 0
				a.snap = r.snapshot
				a.lastErr = ""
				a.probeMsg = ""
				a.recordVerifiedOrigin(r.origin)
				if r.initialConnect && !wasConnected {
					a.page = "home"
					a.scroll.Y = 0
				}
			}
		case r := <-a.probes:
			if r.generation != a.probeEpoch {
				continue
			}
			a.probing = false
			if r.err != nil {
				a.probeOK = false
				a.probeMsg = ""
				a.lastErr = remote.DescribeError(r.err, r.origin)
			} else {
				a.probeOK = true
				a.lastErr = ""
				a.recordVerifiedOrigin(r.origin)
				state := "采集正常"
				if !r.health.CaptureHealthy {
					state = "采集器需检查"
				}
				a.probeMsg = fmt.Sprintf("连接验证通过 · 延迟 %d ms · %s", max(r.latency.Milliseconds(), 1), state)
			}
		case r := <-a.morePages:
			if r.generation != a.epoch || r.cursor != a.snap.NextCursor {
				continue
			}
			a.loadingMore = false
			if r.err != nil {
				a.lastErr = remote.DescribeError(r.err, a.currentOrigin())
			} else {
				a.snap.Events = remote.MergeEventPages(a.snap.Events, r.page.Events, 240)
				a.snap.NextCursor = r.page.NextCursor
				a.snap.HasMore = r.page.HasMore || r.page.NextCursor != ""
				a.lastErr = ""
			}
		case r := <-a.details:
			if r.generation != a.epoch || r.eventID != a.detailID {
				continue
			}
			a.detailBusy = false
			a.detailText = r.json
			if r.err != nil {
				a.detailErr = remote.DescribeError(r.err, a.currentOrigin())
			} else {
				a.detailErr = ""
			}
		default:
			return
		}
	}
}

func (a *monitorApp) currentOrigin() string {
	if a.client != nil {
		return a.client.Origin()
	}
	return a.origin
}

func (a *monitorApp) connectionStatusBadge(t *ui.Theme) (string, ui.Color) {
	switch {
	case a.connecting:
		return "正在验证连接…", t.Warning
	case a.client == nil || !a.connected:
		return "未连接", t.TextMuted
	case a.authFailed:
		return "认证已失效", t.Danger
	case a.consecutiveFailures > 0:
		return fmt.Sprintf("网络重试中 (%d)", a.consecutiveFailures), t.Warning
	default:
		if ms := a.snap.Latency.Milliseconds(); ms > 0 {
			return fmt.Sprintf("已连接 · %d ms", ms), t.Success
		}
		return "已连接", t.Success
	}
}

func (a *monitorApp) view(c *ui.Context) {
	a.receiveResults()
	if a.client != nil && a.connected && !a.authFailed && !a.suspended && !a.loading &&
		time.Since(a.lastPoll) >= a.pollInterval() {
		a.refreshWithMode(false)
	}
	// Schedules a frame without a busy animation loop. The network goroutines
	// only send results through channels; all state changes happen here.
	c.After(time.Second)
	top, _, bottom, _ := c.SafeArea()
	theme := c.Theme()
	ui.Column(c).Fill().Background(theme.Background).
		Padding(top+8, 14, bottom+8, 14).Gap(12).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Column(c).Grow(1).Gap(2).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
					ui.Text(c, "明镜 · Mobile").FontSize(22).Bold()
					badgeText, badgeColor := a.connectionStatusBadge(theme)
					ui.Badge(c, badgeText).Background(badgeColor.Alpha(0.14)).TextColor(badgeColor)
				})
				subtitle := "Agent eBPF / 远程只读监控"
				if a.client != nil && a.connected {
					subtitle = a.client.Origin()
				}
				ui.Text(c, subtitle).FontSize(11).TextColor(theme.TextMuted)
			})
			if a.client != nil && a.connected {
				label := "刷新"
				if a.loading {
					label = "同步中…"
				}
				if ui.Button(c, label).Disabled(a.loading).Clicked() {
					a.authFailed = false
					a.lastPoll = time.Time{}
					a.refreshWithMode(false)
				}
			}
		})
		if a.lastErr != "" {
			ui.Column(c).Padding(10).Radius(10).Gap(6).
				Background(theme.Danger.Alpha(0.10)).Border(1, theme.Danger.Alpha(0.30)).Children(func() {
				ui.Text(c, a.lastErr).FontSize(12).TextColor(theme.Danger)
				if a.authFailed && a.page != "connect" {
					if ui.Button(c, "前往更新 Token").Clicked() {
						a.page = "connect"
						a.scroll.Y = 0
					}
				}
			})
		}
		ui.Scroll(c).Fill().TrackScroll(&a.scroll).Children(func() {
			switch a.page {
			case "events":
				a.eventsView(c)
			case "connect":
				a.connectView(c)
			default:
				a.overviewView(c)
			}
		})
		ui.Row(c).Gap(8).Children(func() {
			for _, item := range []struct{ id, label string }{
				{"home", "概览"}, {"events", "事件"}, {"connect", "连接"},
			} {
				item := item
				var btn ui.Element
				if a.page == item.id {
					btn = ui.PrimaryButton(c, item.label).Grow(1)
				} else {
					btn = ui.Button(c, item.label).Grow(1)
				}
				if btn.Clicked() {
					a.page = item.id
					a.scroll.Y = 0
				}
			}
		})
	})
}

func mobileCard(c *ui.Context, content func()) {
	t := c.Theme()
	ui.Column(c).Padding(14).Gap(10).Radius(14).
		Background(t.Surface).Border(1, t.Border).Children(content)
}

func (a *monitorApp) connectView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(14).Children(func() {
		mobileCard(c, func() {
			ui.Text(c, "连接远程监控服务").FontSize(18).Bold()
			ui.Text(c, "在已运行 Agent eBPF 后端的主机上配置 TLS 和 API Token。支持输入域名端口（自动补全 HTTPS）或 agentebpf://connect 连接链接。").
				FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.origin).Label("服务地址或连接串").Placeholder("https://monitor.example.com:8443")
			ui.Row(c).Gap(6).Wrap().Children(func() {
				if ui.Button(c, "HTTPS 模板").Clicked() {
					a.origin = "https://"
					a.lastErr = ""
				}
				if ui.Button(c, "USB 调试 (127.0.0.1:8080)").Clicked() {
					a.origin = "http://127.0.0.1:8080"
					a.lastErr = ""
				}
				for _, recent := range a.recentOrigins {
					recent := recent
					if ui.Button(c, recent).Clicked() {
						a.origin = recent
						a.lastErr = ""
					}
				}
			})
			ui.TextInput(c, &a.token).Label("API Token（仅驻留内存，不写盘）").Password().Placeholder("服务端访问令牌")
			ui.Text(c, "仅接受 HTTPS 远程连接；HTTP 仅限本机回环（配合 adb reverse tcp:8080 tcp:8080 调试）。").
				FontSize(11).TextColor(t.TextMuted)
			if a.probeMsg != "" {
				tone := t.Success
				if !a.probeOK {
					tone = t.Warning
				}
				ui.Text(c, a.probeMsg).FontSize(12).TextColor(tone)
			}
			ui.Row(c).Gap(8).Wrap().Children(func() {
				connectLabel := "连接并读取"
				if a.connecting {
					connectLabel = "正在连接…"
				}
				if ui.PrimaryButton(c, connectLabel).Disabled(a.connecting || a.probing).Clicked() {
					a.connect()
				}
				probeLabel := "测试连接"
				if a.probing {
					probeLabel = "正在测试…"
				}
				if ui.Button(c, probeLabel).Disabled(a.connecting || a.probing).Clicked() {
					a.probeConnection()
				}
			})
			if a.client != nil && a.connected {
				ui.Column(c).Padding(10).Radius(10).Gap(6).
					Background(t.Success.Alpha(0.08)).Border(1, t.Success.Alpha(0.28)).Children(func() {
					ui.Text(c, "当前已建立监控会话："+a.client.Origin()).FontSize(12).Bold().TextColor(t.Success)
					ui.Text(c, fmt.Sprintf("最后同步：%s · 延迟 %d ms · 轮询间隔 %s",
						formatChecked(a.snap.CheckedAt),
						max(a.snap.Latency.Milliseconds(), 1),
						a.pollInterval().Round(time.Second))).FontSize(11).TextColor(t.TextMuted)
					if ui.Button(c, "断开并清除令牌").Clicked() {
						a.disconnect()
					}
				})
			}
		})
		mobileCard(c, func() {
			ui.Text(c, "远程与 USB 连接指南").Bold()
			ui.Text(c, "1. 远程连接：在 Linux 主机启动带认证的 Agent eBPF 后端，并通过可信 HTTPS 反向代理或 VPN/TLS 暴露。")
			ui.Text(c, "2. USB 调试：手机通过数据线连接电脑后执行 adb reverse tcp:8080 tcp:8080，在上方选择「USB 调试 (127.0.0.1:8080)」。")
			ui.Text(c, "3. 快速链接：支持通过 agentebpf://connect?origin=https://host:port 唤醒并自动填充服务地址。")
			ui.Text(c, "手机端仅执行只读 GET 查询，连接或断开绝不修改后端采集、阻断或模型策略。").FontSize(11).TextColor(t.TextMuted)
		})
	})
}

func (a *monitorApp) overviewView(c *ui.Context) {
	t := c.Theme()
	if a.client == nil || !a.connected {
		mobileCard(c, func() {
			ui.Text(c, "尚未连接").FontSize(18).Bold()
			ui.Text(c, "切换至「连接」，输入后端 HTTPS 地址和 Token 并完成连接验证。").TextColor(t.TextMuted)
			if ui.PrimaryButton(c, "前往连接设置").Clicked() {
				a.page = "connect"
				a.scroll.Y = 0
			}
		})
		return
	}
	health := a.snap.Health
	ui.Column(c).Gap(12).Children(func() {
		mobileCard(c, func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, "采集器状态").FontSize(18).Bold().Grow(1)
				if a.consecutiveFailures > 0 {
					ui.Badge(c, "展示上次快照").Background(t.Warning.Alpha(0.15)).TextColor(t.Warning)
				}
			})
			if a.snap.CheckedAt.IsZero() {
				ui.Text(c, "正在读取…").TextColor(t.TextMuted)
			} else if health.CaptureHealthy {
				ui.Text(c, "采集正常").FontSize(24).Bold().TextColor(t.Success)
			} else {
				ui.Text(c, "采集器需检查").FontSize(24).Bold().TextColor(t.Warning)
			}
			ui.Text(c, fmt.Sprintf("服务：%s · 最后更新：%s · 延迟 %d ms",
				a.client.Origin(),
				formatChecked(a.snap.CheckedAt),
				max(a.snap.Latency.Milliseconds(), 1))).FontSize(12).TextColor(t.TextMuted)
		})
		mobileCard(c, func() {
			ui.Text(c, "健康与内核指标").FontSize(17).Bold()
			metricRow(c, "Ringbuf 累计事件", fmt.Sprint(health.RingbufEventsTotal))
			metricRow(c, "Ringbuf 丢弃累计", fmt.Sprint(health.RingbufDroppedTotal))
			metricRow(c, "后端队列长度", fmt.Sprint(health.BackendQueueLen))
			metricRow(c, "持久化队列", fmt.Sprintf("%d / %d", health.PersistQueueLen, health.PersistQueueCap))
			metricRow(c, "待持久化 / 已持久化", fmt.Sprintf("%d / %d", health.PersistPending, health.CapturedPersistedTotal))
			if health.KernelRiskAlertsTotal > 0 || health.KernelRiskBlocksTotal > 0 {
				metricRow(c, "内核风险告警 / 阻断", fmt.Sprintf("%d / %d", health.KernelRiskAlertsTotal, health.KernelRiskBlocksTotal))
			}
		})
		blocked, uncertain, agentEvents := 0, 0, 0
		for _, event := range a.snap.Events {
			if strings.EqualFold(event.Decision, "BLOCK") {
				blocked++
			}
			if event.RiskScore >= 0.7 {
				uncertain++
			}
			if event.HasAgentContext || (strings.TrimSpace(event.Tag) != "" && !strings.EqualFold(event.Tag, "Unknown")) {
				agentEvents++
			}
		}
		mobileCard(c, func() {
			ui.Text(c, fmt.Sprintf("最近活动（已加载 %d 条）", len(a.snap.Events))).FontSize(17).Bold()
			metricRow(c, "Agent 归属事件", fmt.Sprint(agentEvents))
			metricRow(c, "阻断决策", fmt.Sprint(blocked))
			metricRow(c, "高风险评分 ≥ 0.70", fmt.Sprint(uncertain))
			ui.Row(c).Gap(8).Wrap().Children(func() {
				if ui.Button(c, "查看全部事件").Clicked() {
					a.filter = ""
					a.page = "events"
					a.scroll.Y = 0
				}
				if blocked > 0 {
					if ui.Button(c, "仅看阻断").Clicked() {
						a.filter = "block"
						a.page = "events"
						a.scroll.Y = 0
					}
				}
				if uncertain > 0 {
					if ui.Button(c, "仅看高风险").Clicked() {
						a.filter = "risk"
						a.page = "events"
						a.scroll.Y = 0
					}
				}
			})
		})
		ui.Text(c, "统计基于当前已加载快照，不能代表全量历史事件。").FontSize(11).TextColor(t.TextMuted)
	})
}

func metricRow(c *ui.Context, name, value string) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, name).Grow(1).TextColor(t.TextMuted)
		ui.Text(c, value).Bold()
	})
}

func matchesMobileEvent(event remote.Event, filter, search string) bool {
	switch filter {
	case "block":
		if !strings.EqualFold(event.Decision, "BLOCK") {
			return false
		}
	case "risk":
		if event.RiskScore < 0.7 {
			return false
		}
	case "agent":
		if !event.HasAgentContext && (strings.TrimSpace(event.Tag) == "" || strings.EqualFold(event.Tag, "Unknown")) {
			return false
		}
	}
	if search == "" {
		return true
	}
	return strings.Contains(strings.ToLower(strings.Join([]string{
		event.Comm, event.Type, event.Tag, event.Target, event.Domain,
		event.NetEndpoint, event.Decision, event.AgentRunID, event.ToolName,
		fmt.Sprint(event.PID),
	}, " ")), search)
}

func (a *monitorApp) eventsView(c *ui.Context) {
	t := c.Theme()
	if a.client == nil || !a.connected {
		mobileCard(c, func() {
			ui.Text(c, "请先连接远程后端。")
			if ui.Button(c, "前往连接").Clicked() {
				a.page = "connect"
				a.scroll.Y = 0
			}
		})
		return
	}
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, "最近事件").FontSize(20).Bold()
		ui.TextInput(c, &a.search).Placeholder("搜索进程、PID、目标、Agent、工具、决策")
		ui.Row(c).Gap(6).Wrap().Children(func() {
			for _, item := range []struct{ id, label string }{
				{"", "全部"},
				{"block", "阻断"},
				{"risk", "高风险 ≥0.70"},
				{"agent", "Agent 活动"},
			} {
				item := item
				var btn ui.Element
				if a.filter == item.id {
					btn = ui.PrimaryButton(c, item.label)
				} else {
					btn = ui.Button(c, item.label)
				}
				if btn.Clicked() {
					a.filter = item.id
				}
			}
		})
		shown := 0
		search := strings.ToLower(strings.TrimSpace(a.search))
		for _, event := range a.snap.Events {
			if !matchesMobileEvent(event, a.filter, search) {
				continue
			}
			shown++
			// A stable ID is required for details; events without IDs remain viewable.
			id := event.EventID
			mobileCard(c, func() {
				ui.Row(c).Gap(8).Children(func() {
					title := firstNonEmpty(event.Comm, event.Type, "未知进程")
					if event.Type != "" && event.Comm != "" && !strings.EqualFold(event.Comm, event.Type) {
						title = event.Comm + " · " + event.Type
					}
					ui.Text(c, title).Bold().Grow(1)
					color := t.TextMuted
					if strings.EqualFold(event.Decision, "BLOCK") {
						color = t.Danger
					} else if event.RiskScore >= 0.7 {
						color = t.Warning
					}
					ui.Text(c, firstNonEmpty(event.Decision, "—")).Bold().TextColor(color)
				})
				meta := fmt.Sprintf("PID %d · %s · 风险 %.2f", event.PID, firstNonEmpty(event.Tag, "未归属"), event.RiskScore)
				if event.ToolName != "" {
					meta += " · " + event.ToolName
				}
				ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted)
				if target := firstNonEmpty(event.Target, event.Domain, event.NetEndpoint); target != "" {
					ui.Text(c, target).FontSize(12)
				}
				ui.Text(c, formatEventTime(event.ReceivedAt)).FontSize(11).TextColor(t.TextMuted)
				if id != "" {
					label := "详情"
					if id == a.detailID {
						label = "收起详情"
					}
					if ui.Button(c, label).Clicked() {
						a.loadDetail(id)
					}
					if id == a.detailID {
						if a.detailBusy {
							ui.Text(c, "正在读取详情…").TextColor(t.TextMuted)
						}
						if a.detailErr != "" {
							ui.Text(c, a.detailErr).TextColor(t.Danger)
						}
						if a.detailText != "" {
							ui.Text(c, a.detailText).Font("monospace").FontSize(11)
						}
					}
				}
			})
		}
		if shown == 0 {
			ui.Text(c, "暂无匹配事件。").TextColor(t.TextMuted)
		}
		if a.snap.NextCursor != "" && len(a.snap.Events) < 240 {
			moreLabel := "加载更早事件"
			if a.loadingMore {
				moreLabel = "正在加载更早事件…"
			}
			if ui.Button(c, moreLabel).Disabled(a.loadingMore || a.loading).Clicked() {
				a.loadMoreEvents()
			}
		}
		ui.Text(c, fmt.Sprintf("当前快照展示 %d / %d 条事件", shown, len(a.snap.Events))).
			FontSize(11).TextColor(t.TextMuted)
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func formatChecked(t time.Time) string {
	if t.IsZero() {
		return "尚未同步"
	}
	return t.Local().Format("15:04:05")
}

func formatEventTime(ms int64) string {
	if ms <= 0 {
		return "时间未知"
	}
	return time.UnixMilli(ms).Local().Format("01-02 15:04:05")
}

func main() {
	app := newMonitorApp()
	mygo.App.SetName("Renew Monitor")
	mygo.App.OnOpenURL(func(rawURL string) {
		app.applyOpenURL(rawURL)
		if app.win != nil {
			app.win.Update(func() {})
		}
	})
	mygo.Power.OnSuspend(func() {
		app.suspended = true
	})
	mygo.Power.OnResume(func() {
		app.suspended = false
		if app.win != nil {
			app.win.Update(func() {})
		}
	})
	mygo.App.WhenReady(func() {
		app.win = mygo.NewWindow(mygo.WindowOptions{
			Title:   "Renew Monitor",
			Width:   400,
			Height:  760,
			Content: ui.View(app.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
