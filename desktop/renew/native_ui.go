package main

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type renewApp struct {
	win     *mygo.Window
	backend string
	client  *apiClient

	starting   bool
	connected  bool
	healthReady bool
	lastErr    string
	lastSync   time.Time

	page   string
	search string
	paused bool

	// Local native terminal lifecycle stays outside the privileged backend.
	terminalInitialized bool
	terminalTabs []*renewTerminalTab
	terminalActive int
	terminalTabScroll ui.ScrollState
	terminalNextID int
	terminalFocusRequest int
	terminalError string
	terminalClosing atomic.Bool
	navigationOpen bool
	navigationDetailed bool
	inspectorOpen bool
	materialEnabled bool
	tray *mygo.Tray
	trayAvailable bool
	minimizeToTray bool
	// Mutable per-window theme, refreshed from MyGo's system theme every frame.
	workspaceTheme ui.Theme
	inspectorTab int
	inspectorPinnedID string
	inspectorSelectedID string

	events             []eventSummary
	eventMergeScratch  []eventSummary
	eventsVersion      uint64
	eventUIQueue       chan eventSummary
	eventUIDropped     eventDropCounter
	eventUIPaused      eventPauseFlag
	eventUIBatcherStarted eventBatcherFlag
	eventUICommitPending eventCommitFlag
	filterCacheValid   bool
	filterCacheVersion uint64
	filterCacheKey     string
	filterCacheRows    []eventSummary
	riskCacheValid     bool
	riskCacheVersion   uint64
	riskCacheCounts    [3]int
	networkCacheValid  bool
	networkCacheVersion uint64
	networkCacheKey    string
	networkCacheRows   []networkAggregate
	processCacheValid  bool
	processCacheVersion uint64
	processCacheKey    string
	processCacheRows   []processAggregate
	domainCacheValid   bool
	domainCacheVersion uint64
	domainCacheRows    []agentDomainRow
	optionCacheValid   bool
	optionCacheVersion uint64
	eventTypeOptions   []string
	eventSessionOptions []string
	health             collectorHealth
	trackedComms       []string
	system             systemSnapshot
	systemConnected    bool
	systemErr          string
	eventStreamConnected bool
	eventStreamErr       string
	historyCursor      string
	historyInitialized bool
	historyLoading     bool

	eventTable          ui.ListState
	networkTable        ui.ListState
	networkSelected     int
	processTable        ui.ListState
	domainTable         ui.ListState
	domainSelected      int
	domainSearch        string
	domainAgentFilter   string
	domainRiskOnly      bool
	domainShowIPs       bool
	domainLastFilter    string
	domainSelectedKey   string
	domainLastVersion   uint64
	configProjectPath string
	configLoaded      bool
	configLoading     bool
	configGeneration  uint64
	configInspection  agentConfigInspection
	configTable       ui.ListState
	sessionTable        ui.ListState
	rulesTable          ui.ListState
	commTable           ui.ListState
	pathTable           ui.ListState
	prefixTable         ui.ListState
	eventSelected       int
	processSelected     int
	sessionSelected     int
	ruleSelected        int
	commSelected        int
	pathSelected        int
	prefixSelected      int
	eventPIDFilter      int
	eventRootPIDFilter  int
	eventTargetFilter   string
	eventDomainAgent    string
	eventDomainTarget   string
	eventDomainKind     string
	eventFileEditsOnly  bool
	eventDelegatedOnly  bool
	eventReturnPage     string
	eventRiskFilter     string
	eventTypeFilter     string
	eventSessionFilter  string
	eventDecisionFilter string
	eventAttentionOnly  bool
	eventVisibleLimit   int
	eventDetailOpen     bool
	eventDetailLoading  bool
	eventDetailID       string
	eventDetail         map[string]any
	eventDetailText     string
	eventDetailErr      string
	eventDetailTab      int
	eventProcessTab      int
	eventProcessSelectedPID int
	eventProcessExpanded map[int]bool
	eventDetailFieldSearch string
	eventDetailFieldOffset int
	eventDetailSearchSnapshot string
	eventDetailExpandedField string
	eventDetailGeneration uint64
	eventDetailEnforcementReady bool
	eventDetailPendingAction string
	enforcement         enforcementSnapshot
	enforcementBusy     bool
	enforcementErr      string
	pathAccessLoaded bool
	pathAccessBusy bool
	pathAccessErr string
	pathAccessState lsmSandboxStatus
	pathAccessTarget string
	pathAccessMode string
	pathAccessConfirm bool
	pathAccessConfirmText string
	pathAccessPending fileAccessRule

	configReady        bool
	configBusy         bool
	configErr          string
	disabledEventTypes map[int]bool
	runtimeCfg         runtimeConfigResponse
	runtimeReady       bool

	modulesReady         bool
	modulesBusy          bool
	modulesErr           string
	modulesNotice        string
	modulesPendingUnload string
	modules              []ebpfModule

	agentScopes          agentScopePolicy
	agentScopesReady     bool
	agentScopesBusy      bool
	agentScopesErr       string
	agentScopesNotice    string
	captureScopeMode     string
	monitorScopeMode     string
	captureScopeText     string
	monitorScopeText     string
	agentSearch          string
	agentLastSearch      string
	agentFocusPID        int
	agentReturnPage      string
	pathAccessReturnPage string
	agentTable           ui.ListState
	agentSelected        int

	registryReady bool
	registryBusy  bool
	registryErr   string
	registryTab   int
	registryFilterTag    string
	registryFilterStatus string
	registryLastFilterTag string
	registryLastFilterStatus string
	registry      registrySnapshot
	newTag        string
	trackName     string
	trackTag      string

	ccs                   ccsSnapshot
	astrlink              astrLinkSnapshot
	ccr                   ccrSnapshot
	antigravity           antigravitySnapshot
	managementEventFilter string

	rulesReady    bool
	rulesBusy     bool
	rulesErr      string
	rules         []wrapperRule
	ruleSearch    string
	ruleComm      string
	ruleAction    string
	ruleRegex     string
	ruleReplace   string
	rulePriority  string
	ruleRewrite   string
	editingRule   string
	pendingDelete string
}

func newRenewApp(backend string) *renewApp {
	a := &renewApp{
		backend:            backend,
		starting:           true,
		page:               "概览",
		navigationOpen:       false,
		inspectorOpen:      true,
		materialEnabled:    true,
		eventSelected:      -1,
		networkSelected:    -1,
		processSelected:    -1,
		domainSelected:     -1,
		domainShowIPs:      true,
		sessionSelected:    -1,
		ruleSelected:       -1,
		commSelected:       -1,
		pathSelected:       -1,
		prefixSelected:     -1,
		eventVisibleLimit:  50,
		agentSelected:      -1,
		captureScopeMode:   "黑名单",
		monitorScopeMode:   "黑名单",
		disabledEventTypes: make(map[int]bool),
		ruleAction:         "ALERT",
		rulePriority:       "0",
		ruleRewrite:        "[]",
		pathAccessMode: "阻止写入",
	}
	a.eventTable.Selected = &a.eventSelected
	a.networkTable.Selected = &a.networkSelected
	a.processTable.Selected = &a.processSelected
	a.domainTable.Selected = &a.domainSelected
	a.sessionTable.Selected = &a.sessionSelected
	a.rulesTable.Selected = &a.ruleSelected
	a.commTable.Selected = &a.commSelected
	a.agentTable.Selected = &a.agentSelected
	a.pathTable.Selected = &a.pathSelected
	a.prefixTable.Selected = &a.prefixSelected
	return a
}

func (a *renewApp) runPolling(ctx context.Context, session *backendSession) {
	a.startEventUIBatcher(ctx)
	go a.pollCCS(ctx)
	go a.pollAstrLink(ctx)
	go a.pollCCR(ctx)
	go a.pollAntigravity(ctx)
	if session != nil && session.reader != nil && session.nativeIPCVersion >= nativeIPCVersion {
		go a.runNativeIPC(ctx, session)
	} else {
		go a.runSystemStats(ctx)
		go a.runEventSummaryStream(ctx)
	}
	a.refresh(ctx)
	a.refreshConfiguration(ctx)
	go a.refreshAgentScopes(ctx)
	go a.refreshEBPFModules(ctx)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !a.eventUIPaused.Load() {
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
	fullSnapshot := !a.historyInitialized || !a.eventStreamConnected
	var snapshot apiSnapshot
	var err error
	if fullSnapshot {
		snapshot, err = a.client.snapshot(ctx, 320)
	} else {
		snapshot, err = a.client.statusSnapshot(ctx)
	}
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
		if fullSnapshot {
			a.mergeEventWindow(snapshot.Events, 1200)
			if !a.historyInitialized {
				a.historyCursor = snapshot.NextCursor
				a.historyInitialized = true
			}
		}
		a.health = snapshot.Health
		a.healthReady = true
		a.trackedComms = snapshot.TrackedComms
		a.lastSync = snapshot.FetchedAt
	})
}

func (a *renewApp) refreshConfiguration(parent context.Context) {
	if a.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()

	runtimeCfg, runtimeErr := a.client.runtimeConfig(ctx)
	rules, rulesErr := a.client.rules(ctx)
	registry, registryErr := a.client.registry(ctx)
	a.update(func() {
		if runtimeErr != nil {
			a.configReady = false
			a.runtimeReady = false
			a.configErr = runtimeErr.Error()
		} else {
			a.runtimeCfg = runtimeCfg
			a.runtimeReady = true
			a.configReady = true
			a.configErr = ""
			a.disabledEventTypes = disabledSet(runtimeCfg.Runtime.DisabledEventTypes)
		}
		if rulesErr != nil {
			a.rulesReady = false
			a.rulesErr = rulesErr.Error()
		} else {
			a.rulesReady = true
			a.rulesErr = ""
			a.rules = rules
			if a.ruleSelected >= len(a.filteredRules()) {
				a.ruleSelected = -1
			}
		}
		if registryErr != nil {
			a.registryReady = false
			a.registryErr = registryErr.Error()
		} else {
			a.registryReady = true
			a.registryErr = ""
			a.registry = registry
		}
	})
}

// view delegates the desktop shell to its own file so monitoring pages stay independent.
func (a *renewApp) view(c *ui.Context) {
	a.workspaceView(c)
}

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
	case "会话":
		return "按 Agent 上下文聚合运行会话"
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

func (a *renewApp) startingView(c *ui.Context) {
	t := c.Theme()
	card(c, "明镜高悬 · 正在连接监控后端", func() {
		ui.Text(c, "正在复用现有后端，或通过系统授权启动内置后端。桌面界面始终以普通用户权限运行。").TextColor(t.TextMuted)
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

func (a *renewApp) eventFilterCacheKey() string {
	q := strings.ToLower(strings.TrimSpace(a.search))
	key := q + "\x00" + a.eventTypeFilter + "\x00" + a.eventSessionFilter + "\x00" + a.eventDecisionFilter + "\x00" + fmt.Sprint(a.eventPIDFilter) + "\x00" + fmt.Sprint(a.eventRootPIDFilter) + "\x00" + a.eventTargetFilter + "\x00" + a.eventDomainAgent + "\x00" + a.eventDomainTarget + "\x00" + a.eventDomainKind + "\x00" + fmt.Sprint(a.eventFileEditsOnly) + "\x00" + fmt.Sprint(a.eventDelegatedOnly) + "\x00" + a.eventRiskFilter + "\x00" + a.managementEventFilter
	if a.eventAttentionOnly {
		key += "\x001"
	}
	return key
}

func (a *renewApp) filteredEvents() []eventSummary {
	q := strings.ToLower(strings.TrimSpace(a.search))
	key := a.eventFilterCacheKey()
	if a.filterCacheValid && a.filterCacheVersion == a.eventsVersion && a.filterCacheKey == key {
		return a.filterCacheRows
	}
	if q == "" && a.eventPIDFilter == 0 && a.eventRootPIDFilter == 0 && a.eventTargetFilter == "" && a.eventDomainTarget == "" && !a.eventFileEditsOnly && !a.eventDelegatedOnly && a.eventRiskFilter == "" && a.eventTypeFilter == "" && a.eventSessionFilter == "" && a.eventDecisionFilter == "" && a.managementEventFilter == "" && !a.eventAttentionOnly {
		a.filterCacheValid = true
		a.filterCacheVersion = a.eventsVersion
		a.filterCacheKey = key
		a.filterCacheRows = a.events
		return a.events
	}

	out := make([]eventSummary, 0, min(len(a.events), 256))
	var domainOwners agentOwnershipIndex
	if a.eventDomainTarget != "" {
		domainOwners = buildAgentOwnershipIndex(a.events, nil)
	}
	for _, event := range a.events {
		if a.eventDomainTarget != "" && !matchesAgentDomainEvent(event, domainOwners, a.eventDomainAgent, a.eventDomainTarget, a.eventDomainKind) {
			continue
		}
		if q != "" && !strings.Contains(eventSearchText(event), q) {
			continue
		}
		if a.managementEventFilter != "" && managementAppForEvent(event) != a.managementEventFilter {
			continue
		}
		if a.eventRiskFilter != "" && eventRisk(event) != a.eventRiskFilter {
			continue
		}
		if a.eventPIDFilter > 0 && event.PID != a.eventPIDFilter {
			continue
		}
		if !matchesEventDrilldown(event, a.eventRootPIDFilter, a.eventTargetFilter, a.eventFileEditsOnly, a.eventDelegatedOnly) {
			continue
		}
		if a.eventTypeFilter != "" && event.Type != a.eventTypeFilter {
			continue
		}
		if a.eventSessionFilter != "" && eventSessionKey(event) != a.eventSessionFilter {
			continue
		}
		if !matchesEventDecision(event, a.eventDecisionFilter) {
			continue
		}
		if a.eventAttentionOnly && eventRisk(event) == "正常" {
			continue
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
