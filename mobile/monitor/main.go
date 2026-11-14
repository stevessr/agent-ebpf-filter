// Renew Monitor is a small read-only MyGo client for a separately hosted
// Agent eBPF backend. It never starts collectors or modifies server policy.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/stevessr/agent-ebpf-filter/mobile/monitor/remote"
)

type fetchResult struct {
	generation uint64
	snapshot   remote.Snapshot
	err        error
}

type detailResult struct {
	generation uint64
	eventID    string
	json       string
	err        error
}

type monitorApp struct {
	origin  string
	token   string
	client  *remote.Client
	page    string
	search  string
	loading bool
	lastErr string
	snap    remote.Snapshot

	epoch      uint64
	lastPoll   time.Time
	results    chan fetchResult
	details    chan detailResult
	scroll     ui.ScrollState
	detailID   string
	detailText string
	detailErr  string
	detailBusy bool
}

func newMonitorApp() *monitorApp {
	return &monitorApp{
		page: "connect",
		origin: "https://",
		results: make(chan fetchResult, 8),
		details: make(chan detailResult, 8),
	}
}

func (a *monitorApp) connect() {
	client, err := remote.NewClient(a.origin, a.token)
	if err != nil {
		a.lastErr = err.Error()
		return
	}
	a.epoch++
	a.client = client
	a.lastErr = ""
	a.snap = remote.Snapshot{}
	a.detailID, a.detailText, a.detailErr = "", "", ""
	a.lastPoll = time.Time{}
	a.page = "home"
	a.refresh()
}

func (a *monitorApp) disconnect() {
	a.epoch++
	a.client = nil
	a.loading = false
	a.lastErr = ""
	a.snap = remote.Snapshot{}
	a.detailID, a.detailText, a.detailErr = "", "", ""
	a.detailBusy = false
	a.token = "" // clear the credential from the editable field on disconnect
	a.page = "connect"
}

func (a *monitorApp) refresh() {
	if a.client == nil || a.loading {
		return
	}
	a.loading = true
	a.lastPoll = time.Now()
	generation, client := a.epoch, a.client
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()
		snapshot, err := client.FetchSnapshot(ctx, 60)
		select {
		case a.results <- fetchResult{generation: generation, snapshot: snapshot, err: err}:
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
			if r.err != nil {
				a.lastErr = r.err.Error()
			} else {
				a.snap = r.snapshot
				a.lastErr = ""
			}
		case r := <-a.details:
			if r.generation != a.epoch || r.eventID != a.detailID {
				continue
			}
			a.detailBusy = false
			a.detailText = r.json
			if r.err != nil {
				a.detailErr = r.err.Error()
			} else {
				a.detailErr = ""
			}
		default:
			return
		}
	}
}

func (a *monitorApp) view(c *ui.Context) {
	a.receiveResults()
	if a.client != nil && !a.loading && time.Since(a.lastPoll) >= 6*time.Second {
		a.refresh()
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
					ui.Text(c, "明镜 · Mobile").FontSize(23).Bold()
					ui.Text(c, "Agent eBPF / 远程只读监控").FontSize(11).TextColor(theme.TextMuted)
				})
				if a.client != nil {
					if ui.Button(c, "刷新").Disabled(a.loading).Clicked() {
						a.lastPoll = time.Time{}
						a.refresh()
					}
				}
			})
			if a.lastErr != "" {
				ui.Column(c).Padding(10).Radius(10).
					Background(theme.Danger.Alpha(0.10)).Children(func() {
						ui.Text(c, a.lastErr).FontSize(12).TextColor(theme.Danger)
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
					if ui.Button(c, item.label).Grow(1).Clicked() {
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
			ui.Text(c, "在已运行 Agent eBPF 后端的主机上配置 TLS 和 API Token。手机端只读取状态、事件及详情。").
				FontSize(12).TextColor(t.TextMuted)
			ui.TextInput(c, &a.origin).Label("服务地址").Placeholder("https://monitor.example.com:8443")
			ui.TextInput(c, &a.token).Label("API Token（不写入磁盘）").Password().Placeholder("服务端访问令牌")
			ui.Text(c, "仅接受 HTTPS 远程连接。HTTP 只允许本机 loopback；不要将带令牌的 HTTP 端口暴露到局域网。").
				FontSize(11).TextColor(t.TextMuted)
			if ui.PrimaryButton(c, "连接并读取").Clicked() {
				a.connect()
			}
			if a.client != nil {
				ui.Text(c, "当前已建立监控会话").TextColor(t.Success)
				if ui.Button(c, "断开并清除令牌").Clicked() {
					a.disconnect()
				}
			}
		})
		mobileCard(c, func() {
			ui.Text(c, "远程连接说明").Bold()
			ui.Text(c, "1. 启动远程 Agent eBPF 后端并开启认证。")
			ui.Text(c, "2. 通过可信 HTTPS 反向代理或 VPN/TLS 地址访问。")
			ui.Text(c, "3. 输入服务地址与 Token 后连接。")
			ui.Text(c, "连接失败不会修改后端采集、阻断或模型设置。").FontSize(11).TextColor(t.TextMuted)
		})
	})
}

func (a *monitorApp) overviewView(c *ui.Context) {
	t := c.Theme()
	if a.client == nil {
		mobileCard(c, func() {
			ui.Text(c, "尚未连接").FontSize(18).Bold()
			ui.Text(c, "切换至「连接」，输入后端 HTTPS 地址和 Token。").TextColor(t.TextMuted)
		})
		return
	}
	health := a.snap.Health
	ui.Column(c).Gap(12).Children(func() {
		mobileCard(c, func() {
			ui.Text(c, "采集器状态").FontSize(18).Bold()
			if a.snap.CheckedAt.IsZero() {
				ui.Text(c, "正在读取…").TextColor(t.TextMuted)
			} else if health.CaptureHealthy {
				ui.Text(c, "采集正常").FontSize(24).Bold().TextColor(t.Success)
			} else {
				ui.Text(c, "采集器需检查").FontSize(24).Bold().TextColor(t.Warning)
			}
			ui.Text(c, "最后更新："+formatChecked(a.snap.CheckedAt)).FontSize(12).TextColor(t.TextMuted)
		})
		mobileCard(c, func() {
			ui.Text(c, "健康指标").FontSize(17).Bold()
			metricRow(c, "Ringbuf 丢弃累计", fmt.Sprint(health.RingbufDroppedTotal))
			metricRow(c, "后端队列长度", fmt.Sprint(health.BackendQueueLen))
			metricRow(c, "持久化队列", fmt.Sprintf("%d / %d", health.PersistQueueLen, health.PersistQueueCap))
			metricRow(c, "待持久化事件", fmt.Sprint(health.PersistPending))
		})
		blocked, uncertain := 0, 0
		for _, event := range a.snap.Events {
			if strings.EqualFold(event.Decision, "BLOCK") { blocked++ }
			if event.RiskScore >= 0.7 { uncertain++ }
		}
		mobileCard(c, func() {
			ui.Text(c, "最近活动（最多 60 条）").FontSize(17).Bold()
			metricRow(c, "最近事件", fmt.Sprint(len(a.snap.Events)))
			metricRow(c, "阻断决策", fmt.Sprint(blocked))
			metricRow(c, "高风险评分 ≥ 0.70", fmt.Sprint(uncertain))
			if ui.Button(c, "查看事件").Clicked() {
				a.page = "events"
				a.scroll.Y = 0
			}
		})
		ui.Text(c, "统计基于当前快照，不能代表全量历史事件。").FontSize(11).TextColor(t.TextMuted)
	})
}

func metricRow(c *ui.Context, name, value string) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Text(c, name).Grow(1).TextColor(t.TextMuted)
		ui.Text(c, value).Bold()
	})
}

func (a *monitorApp) eventsView(c *ui.Context) {
	t := c.Theme()
	if a.client == nil {
		mobileCard(c, func(){ ui.Text(c, "请先连接远程后端。") })
		return
	}
	ui.Column(c).Gap(10).Children(func() {
		ui.Text(c, "最近事件").FontSize(20).Bold()
		ui.TextInput(c, &a.search).Placeholder("搜索进程、目标、Agent、决策")
		shown := 0
		search := strings.ToLower(strings.TrimSpace(a.search))
		for _, event := range a.snap.Events {
			if search != "" && !strings.Contains(strings.ToLower(strings.Join([]string{
				event.Comm, event.Tag, event.Target, event.Domain, event.Decision, event.AgentRunID,
			}, " ")), search) {
				continue
			}
			shown++
			// A stable ID is required for details; events without IDs remain viewable.
			id := event.EventID
			mobileCard(c, func() {
				ui.Row(c).Gap(8).Children(func() {
					ui.Text(c, firstNonEmpty(event.Comm, event.Type, "未知进程")).Bold().Grow(1)
					color := t.TextMuted
					if strings.EqualFold(event.Decision, "BLOCK") { color = t.Danger }
					ui.Text(c, firstNonEmpty(event.Decision, "—")).Bold().TextColor(color)
				})
				ui.Text(c, fmt.Sprintf("PID %d · %s · 风险 %.2f", event.PID, firstNonEmpty(event.Tag, "未归属"), event.RiskScore)).
					FontSize(11).TextColor(t.TextMuted)
				if target := firstNonEmpty(event.Target, event.Domain, event.NetEndpoint); target != "" {
					ui.Text(c, target).FontSize(12)
				}
				ui.Text(c, formatEventTime(event.ReceivedAt)).FontSize(11).TextColor(t.TextMuted)
				if id != "" {
					label := "详情"
					if id == a.detailID { label = "收起详情" }
					if ui.Button(c, label).Clicked() { a.loadDetail(id) }
					if id == a.detailID {
						if a.detailBusy { ui.Text(c, "正在读取详情…").TextColor(t.TextMuted) }
						if a.detailErr != "" { ui.Text(c, a.detailErr).TextColor(t.Danger) }
						if a.detailText != "" { ui.Text(c, a.detailText).Font("monospace").FontSize(11) }
					}
				}
			})
		}
		if shown == 0 {
			ui.Text(c, "暂无匹配事件。").TextColor(t.TextMuted)
		}
		ui.Text(c, fmt.Sprintf("当前快照展示 %d / %d 条事件", shown, len(a.snap.Events))).
			FontSize(11).TextColor(t.TextMuted)
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" { return v }
	}
	return ""
}

func formatChecked(t time.Time) string {
	if t.IsZero() { return "尚未同步" }
	return t.Local().Format("15:04:05")
}

func formatEventTime(ms int64) string {
	if ms <= 0 { return "时间未知" }
	return time.UnixMilli(ms).Local().Format("01-02 15:04:05")
}

func main() {
	app := newMonitorApp()
	mygo.App.SetName("Renew Monitor")
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title: "Renew Monitor",
			Width: 400,
			Height: 760,
			Content: ui.View(app.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
