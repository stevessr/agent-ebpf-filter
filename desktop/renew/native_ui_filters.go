package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

func (a *renewApp) collectorStatus() (label, level string) {
	switch {
	case a.starting:
		return "同步中", "warning"
	case !a.connected:
		return "离线", "danger"
	case !a.healthReady:
		return "同步中", "warning"
	case !a.health.CaptureHealthy:
		return "异常", "danger"
	case a.syncFreshnessAt(time.Now()) == "unknown":
		return "等待同步", "warning"
	case a.syncFreshnessAt(time.Now()) == "stale":
		return "状态过时", "warning"
	case a.syncFreshnessAt(time.Now()) == "clock-skew":
		return "时钟异常", "warning"
	default:
		return "正常", "success"
	}
}

func (a *renewApp) pipelineStatus() (label, level string) {
	switch {
	case a.starting:
		return "启动中", "warning"
	case !a.connected:
		return "后端离线", "danger"
	case !a.healthReady:
		return "正在同步", "warning"
	case !a.health.CaptureHealthy:
		return "采集异常", "danger"
	case a.syncFreshnessAt(time.Now()) == "unknown":
		return "等待采集同步", "warning"
	case a.syncFreshnessAt(time.Now()) == "stale":
		return "采集状态过时", "warning"
	case a.syncFreshnessAt(time.Now()) == "clock-skew":
		return "同步时间异常", "warning"
	case !a.eventStreamConnected:
		return "事件流回退", "warning"
	case !a.systemConnected:
		return "系统流重连", "warning"
	default:
		return "全链路实时", "success"
	}
}

func pageSubtitle(page string) string {
	switch page {
	case "研判":
		return "事件研判、上下文筛选与运行诊断"
	case "事件":
		return "实时事件、风险筛选与按需详情"
	case "行为":
		return "文件、网络、进程与其他信号的实时轨迹"
	case "会话":
		return "按 Agent 上下文聚合运行会话"
	case "AgentSight":
		return "已观测模型调用、Token 用量与归因证据"
	case "派生":
		return "Agent 调用关系与实时派生进程（放射图 / 瀑布图）"
	case "网络":
		return "外联目标与摘要窗口聚合"
	case "域名":
		return "按 Agent 关联域名、IP 与安全事件"
	case "进程":
		return "实时进程与 Agent 活动"
	case "Agent 识别":
		return "进程识别、捕获名单与监视名单"
	case "监控":
		return "采集范围、运行时能力与开销"
	case "eBPF 模块":
		return "独立 eBPF 插件的手动加载与卸载"
	case "规则":
		return "agent-wrapper 策略与重写"
	case "跟踪":
		return "命令、路径与标签范围"
	case "路径权限":
		return "按完整路径精确限制读取和写入"
	case "系统":
		return "采集器、系统流与队列诊断"
	case "CCS":
		return "CC-Switch、AstrLink、CCR、Antigravity 与 eBPF 事件联动"
	case "终端":
		return "普通用户 Shell · Ghostty VT · 多标签与分屏"
	case "文件":
		return "不经特权后端的本地文件浏览与管理"
	case "编辑器":
		return "MyGo 原生 UTF-8 代码编辑、语法高亮与保存"
	default:
		return "健康、风险与最近活动"
	}
}

func statusPill(c *ui.Context, text string, tone ui.Color) ui.Element {
	return ui.Badge(c, text).Background(tone.Alpha(0.13)).TextColor(tone)
}

func riskTextColor(t *ui.Theme, risk string) ui.Color {
	switch risk {
	case "高风险":
		return t.Danger
	case "需关注":
		return t.Warning
	default:
		return t.TextMuted
	}
}

func riskPill(c *ui.Context, risk string) ui.Element {
	t := c.Theme()
	switch risk {
	case "高风险":
		return statusPill(c, risk, t.Danger)
	case "需关注":
		return statusPill(c, risk, t.Warning)
	default:
		return statusPill(c, risk, t.Success)
	}
}

func pageUsesEventSearch(page string) bool {
	switch page {
	case "概览", "事件", "会话", "网络", "进程":
		return true
	default:
		return false
	}
}

// A comparable signature avoids rebuilding a concatenated string (and
// formatting numbers with fmt.Sprint) for every native animation frame.
// Strings in the signature are immutable filter values, not event payloads.
type eventFilterSignature struct {
	search, eventType, session, decision, target string
	domainAgent, domainTarget, domainKind        string
	risk, management                             string
	pid, rootPID                                 int
	minRisk                                      float64
	fileEditsOnly, delegatedOnly, attentionOnly  bool
}

func (a *renewApp) eventFilterCacheKey() eventFilterSignature {
	// Case folding/search normalization is unnecessary on every frame while
	// the user has not changed the input. Avoid another temporary string
	// during sidebar animations and steady event-table redraws.
	if a.search != a.filterSearchOriginal {
		a.filterSearchOriginal = a.search
		a.filterSearchNormalized = strings.ToLower(strings.TrimSpace(a.search))
	}
	return eventFilterSignature{
		search:        a.filterSearchNormalized,
		eventType:     a.eventTypeFilter,
		session:       a.eventSessionFilter,
		decision:      a.eventDecisionFilter,
		target:        a.eventTargetFilter,
		domainAgent:   a.eventDomainAgent,
		domainTarget:  a.eventDomainTarget,
		domainKind:    a.eventDomainKind,
		risk:          a.eventRiskFilter,
		management:    a.managementEventFilter,
		pid:           a.eventPIDFilter,
		rootPID:       a.eventRootPIDFilter,
		minRisk:       a.eventMinRiskScore,
		fileEditsOnly: a.eventFileEditsOnly,
		delegatedOnly: a.eventDelegatedOnly,
		attentionOnly: a.eventAttentionOnly,
	}
}

// matchesEventFilter is shared by full-window scans and incremental
// additions. Keep the ordering of cheap predicates ahead of full-text
// construction, and never infer an Agent ownership from live PID reuse.
func matchesEventFilter(event eventSummary, key eventFilterSignature, domainOwners agentOwnershipIndex) bool {
	if key.pid > 0 && event.PID != key.pid {
		return false
	}
	if key.minRisk > 0 && event.RiskScore < key.minRisk {
		return false
	}
	if key.eventType != "" && event.Type != key.eventType {
		return false
	}
	if !matchesEventDrilldown(event, key.rootPID, key.target, key.fileEditsOnly, key.delegatedOnly) {
		return false
	}
	if key.session != "" && eventSessionKey(event) != key.session {
		return false
	}
	if !matchesEventDecision(event, key.decision) {
		return false
	}
	if key.risk != "" || key.attentionOnly {
		risk := eventRisk(event)
		if key.risk != "" && risk != key.risk {
			return false
		}
		if key.attentionOnly && risk == "正常" {
			return false
		}
	}
	if key.management != "" && managementAppForEvent(event) != key.management {
		return false
	}
	if key.domainTarget != "" && !matchesAgentDomainEvent(event, domainOwners, key.domainAgent, key.domainTarget, key.domainKind) {
		return false
	}
	return key.search == "" || strings.Contains(eventSearchText(event), key.search)
}

// Empty means the filter directly aliases the original event window. Do not
// reallocate or copy those rows during merge: just advance the cache version.
func (key eventFilterSignature) isUnfiltered() bool {
	return key.search == "" && key.eventType == "" && key.session == "" &&
		key.decision == "" && key.pid == 0 && key.rootPID == 0 &&
		key.target == "" && key.domainTarget == "" && !key.fileEditsOnly &&
		!key.delegatedOnly && key.risk == "" && key.minRisk <= 0 &&
		key.management == "" && !key.attentionOnly
}

// applyIncrementalFilterRows merges already-ordered matching additions with
// existing matching rows, excluding all revised or evicted EventIDs. Neither
// side needs a full-window predicate rescan. Only used for selective filters;
// large/broad caches continue to rebuild lazily for predictable memory use.
func (a *renewApp) applyIncrementalFilterRows(workspace *eventMergeWorkspace) {
	key := a.filterCacheKey
	if key.isUnfiltered() {
		a.filterCacheRows = a.events
		a.filterCacheVersion = a.eventsVersion
		return
	}
	if len(workspace.filterAdded) == 0 && len(workspace.filterRemoved) == 0 {
		a.filterCacheVersion = a.eventsVersion
		return
	}
	oldRows := a.filterCacheRows
	added := workspace.filterAdded
	// Reuse the merge's addition buffer for the matching subset; write only
	// behind the source cursor so range iteration is never overwritten.
	matching := added[:0]
	for _, event := range added {
		if matchesEventFilter(event, key, agentOwnershipIndex{}) {
			matching = append(matching, event)
		}
	}
	// Filtering only new/changed events and copying at most 128 prior
	// matches is cheaper than checking all 1200 event summaries each batch.
	capacity := len(oldRows) + len(matching)
	var output []eventSummary
	if cap(a.filterMergeScratch) >= capacity {
		output = a.filterMergeScratch[:0]
	} else if capacity != 0 {
		output = make([]eventSummary, 0, capacity)
	}
	i, j := 0, 0
	for i < len(oldRows) || j < len(matching) {
		if i < len(oldRows) {
			if _, removed := workspace.filterRemoved[oldRows[i].EventID]; removed {
				i++
				continue
			}
		}
		if j >= len(matching) {
			if i < len(oldRows) {
				output = append(output, oldRows[i])
				i++
			}
		} else if i >= len(oldRows) || eventSummaryBefore(matching[j], oldRows[i]) {
			output = append(output, matching[j])
			j++
		} else {
			output = append(output, oldRows[i])
			i++
		}
	}
	a.filterCacheRows = output
	// The recycled result buffer must not pin strings from evicted events.
	// Both buffers are owned by a filtered view (unfiltered aliases bypass it).
	if cap(oldRows) > 0 {
		clear(oldRows[:cap(oldRows)])
	}
	a.filterMergeScratch = oldRows[:0]
	a.filterCacheVersion = a.eventsVersion
}

func (a *renewApp) filteredEvents() []eventSummary {
	key := a.eventFilterCacheKey()
	if a.filterCacheValid && a.filterCacheVersion == a.eventsVersion && a.filterCacheKey == key {
		return a.filterCacheRows
	}
	if key.isUnfiltered() {
		a.filterCacheValid = true
		a.filterCacheVersion = a.eventsVersion
		a.filterCacheKey = key
		a.filterCacheRows = a.events
		return a.events
	}

	// A precise filter (PID, root, target or session) commonly returns one
	// row, not 256; avoid allocating a large buffer before the first match.
	// On the same filter after an event batch, use the previous match count
	// to avoid repeatedly growing and copying hundreds of compact structs.
	hint := min(len(a.events), 256)
	if key.pid > 0 || key.rootPID > 0 || key.target != "" ||
		key.domainTarget != "" || key.session != "" {
		hint = min(len(a.events), 16)
	}
	if a.filterCacheValid && a.filterCacheKey == key {
		if previous := len(a.filterCacheRows); previous > 0 {
			hint = min(len(a.events), max(hint, previous))
		} else {
			hint = min(len(a.events), 16)
		}
	}
	var out []eventSummary
	var domainOwners agentOwnershipIndex
	if a.eventDomainTarget != "" {
		domainOwners = a.eventOwnershipIndex()
	}
	for _, event := range a.events {
		if !matchesEventFilter(event, key, domainOwners) {
			continue
		}
		if out == nil {
			out = make([]eventSummary, 0, hint)
		}
		out = append(out, event)
	}
	a.filterCacheValid = true
	a.filterCacheVersion = a.eventsVersion
	a.filterCacheKey = key
	a.filterCacheRows = out
	return out
}

func (a *renewApp) riskCounts() (normal, attention, danger int) {
	if a.riskCacheValid && a.riskCacheVersion == a.eventsVersion {
		return a.riskCacheCounts[0], a.riskCacheCounts[1], a.riskCacheCounts[2]
	}
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
	a.riskCacheValid = true
	a.riskCacheVersion = a.eventsVersion
	a.riskCacheCounts = [3]int{normal, attention, danger}
	return
}

func (a *renewApp) filteredNetworkRows() []networkAggregate {
	key := a.eventFilterCacheKey()
	if a.networkCacheValid && a.networkCacheVersion == a.eventsVersion && a.networkCacheKey == key {
		return a.networkCacheRows
	}
	rows := aggregateNetwork(a.filteredEvents())
	a.networkCacheValid = true
	a.networkCacheVersion = a.eventsVersion
	a.networkCacheKey = key
	a.networkCacheRows = rows
	return rows
}

func (a *renewApp) filteredProcessAggregateRows() []processAggregate {
	key := a.eventFilterCacheKey()
	if a.processCacheValid && a.processCacheVersion == a.eventsVersion && a.processCacheKey == key {
		return a.processCacheRows
	}
	rows := aggregateProcesses(a.filteredEvents())
	a.processCacheValid = true
	a.processCacheVersion = a.eventsVersion
	a.processCacheKey = key
	a.processCacheRows = rows
	return rows
}

func (a *renewApp) eventFilterOptions() ([]string, []string) {
	if a.optionCacheValid && a.optionCacheVersion == a.eventsVersion {
		return a.eventTypeOptions, a.eventSessionOptions
	}
	a.eventTypeOptions = uniqueEventTypes(a.events)
	a.eventSessionOptions = uniqueEventSessions(a.events)
	a.optionCacheValid = true
	a.optionCacheVersion = a.eventsVersion
	return a.eventTypeOptions, a.eventSessionOptions
}

func card(c *ui.Context, title string, body func()) ui.Element {
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
