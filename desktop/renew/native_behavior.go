package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

const (
	behaviorWindowDefault    = int64(60_000)
	behaviorRenderLimit      = behaviorOrbitMarksPerLane
	behaviorProcessMenuLimit = 64
	behaviorBucketCount      = 24
)

type behaviorLane string

const (
	behaviorFile    behaviorLane = "file"
	behaviorNetwork behaviorLane = "network"
	behaviorProcess behaviorLane = "process"
	behaviorOther   behaviorLane = "other"
)

type behaviorSnapshot struct {
	NowMS     int64
	WindowMS  int64
	PID       int
	Events    []eventSummary
	Lanes     [4]int
	Processes []processAggregate
	Histogram [behaviorBucketCount]int
	Rate      float64
	Attention int
	MaxRisk   float64
	HasRisk   bool
	Bytes     int64
}

func behaviorLaneForEvent(e eventSummary) behaviorLane {
	t := strings.ToLower(strings.TrimSpace(e.Type))
	switch t {
	case "open", "openat", "openat2", "read", "write", "mkdir", "mkdirat", "unlink", "unlinkat", "ioctl", "chmod", "chown", "rename", "renameat2", "linkat", "symlinkat", "truncate":
		return behaviorFile
	case "execve", "execveat", "clone", "fork", "vfork", "exit", "exit_group", "process_fork", "process_exec", "process_exit", "wait4":
		return behaviorProcess
	}
	// Keep numeric protobuf classifications in sync with tracker_common.proto.
	switch e.EventType {
	case 1, 3, 4, 5, 9, 10, 11, 12, 13, 14, 15, 16, 17:
		return behaviorFile
	case 0:
		if t == "" {
			return behaviorProcess
		}
	case 18, 19, 26, 27, 28, 29:
		return behaviorProcess
	case 2, 6, 7, 8, 20, 21, 22, 31, 32, 33, 34, 35, 36, 37, 43:
		return behaviorNetwork
	}
	if e.Network || strings.Contains(t, "network") || strings.Contains(t, "connect") || strings.Contains(t, "tcp") || strings.Contains(t, "dns") || strings.Contains(t, "socket") || strings.Contains(t, "http") || strings.Contains(t, "tls") || strings.Contains(t, "sendto") || strings.Contains(t, "recvfrom") {
		return behaviorNetwork
	}
	return behaviorOther
}

func behaviorNeedsAttention(e eventSummary) bool {
	decision := strings.ToUpper(strings.TrimSpace(e.Decision))
	return e.RiskScore >= 60 || decision == "ALERT" || decision == "BLOCK" || decision == "DENY" ||
		e.EventType == 30 || e.EventType == 41 ||
		strings.EqualFold(strings.TrimSpace(e.Type), "semantic_alert") || strings.EqualFold(strings.TrimSpace(e.Type), "agentsight_alert")
}

// projectBehavior bounds all derived data to the caller's already-bounded event window.
func projectBehavior(events []eventSummary, nowMS, windowMS int64, pid int) behaviorSnapshot {
	if windowMS <= 0 {
		windowMS = behaviorWindowDefault
	}
	out := behaviorSnapshot{NowMS: nowMS, WindowMS: windowMS, PID: pid}
	start := nowMS - windowMS
	for _, e := range events {
		if e.ReceivedAtMS < start || e.ReceivedAtMS > nowMS || e.ReceivedAtMS <= 0 || (pid > 0 && e.PID != pid) {
			continue
		}
		out.Events = append(out.Events, e)
	}
	sort.Slice(out.Events, func(i, j int) bool { return out.Events[i].ReceivedAtMS > out.Events[j].ReceivedAtMS })
	byLane := map[behaviorLane]int{}
	byPID := make(map[int]*processAggregate)
	for _, e := range out.Events {
		lane := behaviorLaneForEvent(e)
		byLane[lane]++
		proc := byPID[e.PID]
		if proc == nil && e.PID > 0 {
			proc = &processAggregate{PID: e.PID, PPID: e.PPID, Comm: e.Comm}
			byPID[e.PID] = proc
		}
		if proc != nil {
			proc.Events++
			if e.ReceivedAtMS > proc.LastMS {
				proc.LastMS = e.ReceivedAtMS
			}
			if e.RiskScore > proc.Risk {
				proc.Risk = e.RiskScore
			}
		}
		if behaviorNeedsAttention(e) {
			out.Attention++
		}
		if e.RiskScore > out.MaxRisk {
			out.MaxRisk, out.HasRisk = e.RiskScore, true
		}
		if e.NetBytes > 0 {
			out.Bytes += e.NetBytes
		}
		bucket := int((e.ReceivedAtMS - start) * behaviorBucketCount / windowMS)
		if bucket < 0 {
			bucket = 0
		}
		if bucket >= behaviorBucketCount {
			bucket = behaviorBucketCount - 1
		}
		out.Histogram[bucket]++
		if e.ReceivedAtMS >= nowMS-5_000 {
			out.Rate++
		}
	}
	out.Lanes = [4]int{byLane[behaviorFile], byLane[behaviorNetwork], byLane[behaviorProcess], byLane[behaviorOther]}
	if windowMS < 5_000 && windowMS > 0 {
		out.Rate /= float64(windowMS) / 1000
	} else {
		out.Rate /= 5
	}
	for _, p := range byPID {
		out.Processes = append(out.Processes, *p)
	}
	sort.Slice(out.Processes, func(i, j int) bool {
		if out.Processes[i].Events != out.Processes[j].Events {
			return out.Processes[i].Events > out.Processes[j].Events
		}
		return out.Processes[i].PID < out.Processes[j].PID
	})
	return out
}

func (a *renewApp) behaviorDisplay(now time.Time) behaviorSnapshot {
	if a.behaviorPaused && a.behaviorFrozen != nil {
		return *a.behaviorFrozen
	}
	nowMS := now.UnixMilli()
	if !a.behaviorCacheValid || a.behaviorCacheVersion != a.eventsVersion ||
		a.behaviorCacheSecond != nowMS/1000 || a.behaviorCacheWindow != a.behaviorWindowMS || a.behaviorCachePID != a.behaviorPID {
		a.behaviorCache = projectBehavior(a.events, nowMS, a.behaviorWindowMS, a.behaviorPID)
		a.behaviorCacheVersion = a.eventsVersion
		a.behaviorCacheSecond = nowMS / 1000
		a.behaviorCacheWindow = a.behaviorWindowMS
		a.behaviorCachePID = a.behaviorPID
		a.behaviorCacheValid = true
	}
	return a.behaviorCache
}

func (a *renewApp) refreshBehaviorFrozen(now time.Time) {
	if !a.behaviorPaused {
		return
	}
	frozenNow := now.UnixMilli()
	if a.behaviorFrozen != nil {
		frozenNow = a.behaviorFrozen.NowMS
	}
	snapshot := projectBehavior(a.behaviorPausedEvents, frozenNow, a.behaviorWindowMS, a.behaviorPID)
	a.behaviorFrozen = &snapshot
}

func (a *renewApp) toggleBehaviorPause(now time.Time) {
	if !a.behaviorPaused {
		a.behaviorPausedEvents = append([]eventSummary(nil), a.events...)
		snapshot := projectBehavior(a.behaviorPausedEvents, now.UnixMilli(), a.behaviorWindowMS, a.behaviorPID)
		a.behaviorFrozen = &snapshot
		a.behaviorPaused = true
		return
	}
	a.behaviorPaused = false
	a.behaviorFrozen = nil
	a.behaviorPausedEvents = nil
}

func (a *renewApp) behaviorView(c *ui.Context) {
	t := c.Theme()
	now := time.Now()
	snapshot := a.behaviorDisplay(now)
	ui.Text(c, "行为可视化").FontSize(22).Bold()
	ui.Text(c, "使用本桌面现有事件流；只显示已接收的有界摘要，不代表完整主机审计。").FontSize(11).TextColor(t.TextMuted)
	ui.Row(c).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		status, level := a.pipelineStatus()
		statusPill(c, status, workspaceStatusTone(t, level))
		if a.behaviorPaused {
			statusPill(c, "仅展示已暂停 · 采集继续", t.Warning)
		}
		ui.Spacer(c)
		for _, window := range []int64{30_000, 60_000, 300_000} {
			label := map[int64]string{30_000: "30 秒", 60_000: "60 秒", 300_000: "5 分钟"}[window]
			button := ui.Button(c, label)
			if snapshot.WindowMS == window {
				button.Background(t.Accent.Alpha(0.16)).TextColor(t.Accent)
			}
			if button.Clicked() {
				a.behaviorWindowMS = window
				a.refreshBehaviorFrozen(now)
			}
		}
		pauseLabel := "暂停显示"
		if a.behaviorPaused {
			pauseLabel = "继续显示"
		}
		if ui.Button(c, pauseLabel).Tooltip("只冻结此页摘要；后台事件流与策略继续工作").Clicked() {
			a.toggleBehaviorPause(now)
		}
	})
	ui.Row(c).Gap(8).Wrap().Children(func() {
		ui.Text(c, "进程聚焦").TextColor(t.TextMuted)
		if ui.Button(c, map[bool]string{true: fmt.Sprintf("PID %d", a.behaviorPID), false: "全部进程"}[a.behaviorPID > 0]).Menu(func(m *ui.Menu) {
			if m.Item("全部进程").Checked(a.behaviorPID == 0).Chosen() {
				a.behaviorPID = 0
				a.refreshBehaviorFrozen(now)
			}
			source, sourceNow := a.events, now.UnixMilli()
			if a.behaviorPaused && a.behaviorFrozen != nil {
				source, sourceNow = a.behaviorPausedEvents, a.behaviorFrozen.NowMS
			}
			processes := projectBehavior(source, sourceNow, snapshot.WindowMS, 0).Processes
			if len(processes) > behaviorProcessMenuLimit {
				processes = processes[:behaviorProcessMenuLimit]
			}
			for _, p := range processes {
				p := p
				label := fmt.Sprintf("%s · PID %d · %d 条", displayOr(p.Comm, "进程"), p.PID, p.Events)
				if m.Item(label).Checked(a.behaviorPID == p.PID).Chosen() {
					a.behaviorPID = p.PID
					a.refreshBehaviorFrozen(now)
				}
			}
		}).Clicked() {
		}
		for i, lane := range []behaviorLane{behaviorFile, behaviorNetwork, behaviorProcess, behaviorOther} {
			label := []string{"文件", "网络", "进程", "其他"}[i]
			button := ui.Button(c, fmt.Sprintf("%s %d", label, snapshot.Lanes[i]))
			if a.behaviorLane == lane {
				button.Background(t.Accent.Alpha(0.16)).TextColor(t.Accent)
			}
			if button.Clicked() {
				if a.behaviorLane == lane {
					a.behaviorLane = ""
				} else {
					a.behaviorLane = lane
				}
			}
		}
	})
	attentionToggle := ui.Button(c, fmt.Sprintf("仅需关注 · %d", snapshot.Attention))
	if a.behaviorAttentionOnly {
		attentionToggle.Background(t.Warning.Alpha(0.16)).TextColor(t.Warning)
	}
	if attentionToggle.Clicked() {
		a.behaviorAttentionOnly = !a.behaviorAttentionOnly
		a.behaviorSelectedID = ""
	}
	attentionColor := t.Success
	if snapshot.Attention > 0 {
		attentionColor = t.Warning
	}
	if snapshot.HasRisk && snapshot.MaxRisk >= 80 {
		attentionColor = t.Danger
	}
	ui.Row(c).Gap(8).Wrap().Children(func() {
		behaviorMetric(c, "窗口事件", fmt.Sprint(len(snapshot.Events)))
		behaviorMetric(c, "近 5 秒速率", fmt.Sprintf("%.1f / 秒", snapshot.Rate))
		behaviorMetric(c, "网络字节", formatBytes(snapshot.Bytes))
		behaviorMetric(c, "需关注证据", fmt.Sprintf("%d 条", snapshot.Attention))
		if snapshot.HasRisk {
			statusPill(c, fmt.Sprintf("最高风险 %.0f", snapshot.MaxRisk), attentionColor)
		} else {
			statusPill(c, "未记录正风险分数", t.TextMuted)
		}
	})
	card(c, "行为轨道", func() {
		orbitSnapshot := snapshot
		if a.behaviorLane != "" || a.behaviorAttentionOnly {
			orbitSnapshot.Events = make([]eventSummary, 0, min(len(snapshot.Events), behaviorOrbitMarksPerLane))
			orbitSnapshot.Lanes = [4]int{}
			for _, event := range snapshot.Events {
				if (a.behaviorLane != "" && behaviorLaneForEvent(event) != a.behaviorLane) || (a.behaviorAttentionOnly && !behaviorNeedsAttention(event)) {
					continue
				}
				orbitSnapshot.Events = append(orbitSnapshot.Events, event)
				if index := behaviorOrbitLaneIndex(behaviorLaneForEvent(event)); index >= 0 {
					orbitSnapshot.Lanes[index]++
				}
			}
		}
		ui.Text(c, "四类同心轨道按时间定位事件；点选可聚焦节点，再打开完整事件证据。").FontSize(11).TextColor(t.TextMuted)
		a.drawBehaviorOrbit(c, orbitSnapshot)
		ui.Text(c, "键盘可选事件").FontSize(11).TextColor(t.TextMuted)
		ui.Row(c).Gap(6).Wrap().Children(func() {
			shown := [4]int{}
			for _, event := range snapshot.Events {
				lane := behaviorLaneForEvent(event)
				index := behaviorOrbitLaneIndex(lane)
				if index < 0 || (a.behaviorLane != "" && a.behaviorLane != lane) || (a.behaviorAttentionOnly && !behaviorNeedsAttention(event)) || shown[index] >= 4 {
					continue
				}
				shown[index]++
				laneName := []string{"文件访问", "网络通信", "进程执行", "其他信号"}[index]
				label := fmt.Sprintf("%s · %s · %d", laneName, event.Type, event.PID)
				if behaviorNeedsAttention(event) {
					label = "◆ " + label
				}
				event := event
				button := ui.Button(c, label).Tooltip(fmt.Sprintf("%s · %s", eventTime(event), displayOr(eventTarget(event), "未提供目标")))
				if behaviorNeedsAttention(event) {
					button.TextColor(t.Warning)
				}
				if button.Clicked() {
					a.behaviorSelectedID = event.EventID
				}
			}
		})
	})
	selected := behaviorSelectedEvent(snapshot.Events, a.behaviorSelectedID)
	if selected != nil {
		card(c, "选中事件 · "+selected.Type, func() {
			ui.Row(c).Gap(8).Wrap().Children(func() {
				ui.Text(c, fmt.Sprintf("%s · %s · PID %d", eventTime(*selected), displayOr(selected.Comm, "进程"), selected.PID)).Font("monospace")
				risk := eventRisk(*selected)
				riskPill(c, risk)
			})
			ui.Text(c, "目标："+displayOr(eventTarget(*selected), "未提供")).Font("monospace").TextColor(t.TextMuted)
			if selected.Decision != "" {
				ui.Text(c, "策略记录："+selected.Decision+"（这不单独证明内核已阻断）").FontSize(11).TextColor(t.Warning)
			}
			if ui.PrimaryButton(c, "打开完整事件证据").Clicked() && selected.EventID != "" {
				a.openEventDetail(selected.EventID)
			}
		})
	}
	if len(snapshot.Events) == 0 {
		ui.Text(c, "当前时间窗没有匹配摘要。空窗口不表示设备安全；请检查采集状态、进程聚焦和所选时间范围。").TextColor(t.TextMuted)
	}
	ui.Text(c, fmt.Sprintf("本页使用最多 1200 条内存摘要；每条轨道最多显示 %d 个节点。暂停仅冻结本页画面。", behaviorRenderLimit)).FontSize(11).TextColor(t.TextMuted)
}

func behaviorMetric(c *ui.Context, label, value string) {
	ui.Column(c).Gap(2).Padding(10, 12).Border(1, c.Theme().Border).Radius(8).Children(func() {
		ui.Text(c, label).FontSize(11).TextColor(c.Theme().TextMuted)
		ui.Text(c, value).FontSize(15).Bold()
	})
}
func behaviorSelectedEvent(events []eventSummary, id string) *eventSummary {
	for i := range events {
		if events[i].EventID == id && id != "" {
			return &events[i]
		}
	}
	return nil
}
func behaviorHistogram(values [behaviorBucketCount]int) string {
	bars := []rune("▁▂▃▄▅▆▇█")
	maxValue := 0
	for _, v := range values {
		if v > maxValue {
			maxValue = v
		}
	}
	if maxValue == 0 {
		return strings.Repeat("·", len(values))
	}
	var b strings.Builder
	for _, v := range values {
		idx := v * 7 / maxValue
		if idx < 0 {
			idx = 0
		}
		b.WriteRune(bars[idx])
	}
	return b.String()
}
