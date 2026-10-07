package main

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type renewApp struct {
	win     *mygo.Window
	backend string
	client  *apiClient

	starting  bool
	connected bool
	lastErr   string
	lastSync  time.Time

	page   string
	search string
	paused bool

	events             []eventSummary
	eventsVersion      uint64
	eventUIQueue       chan eventSummary
	eventUIDropped     eventDropCounter
	eventUIPaused      eventPauseFlag
	eventUIBatcherStarted eventBatcherFlag
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
	processTable        ui.ListState
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
	enforcement         enforcementSnapshot
	enforcementBusy     bool
	enforcementErr      string

	configReady        bool
	configBusy         bool
	configErr          string
	disabledEventTypes map[int]bool
	runtimeCfg         runtimeConfigResponse
	runtimeReady       bool

	registryReady bool
	registryBusy  bool
	registryErr   string
	registryTab   int
	registry      registrySnapshot
	newTag        string
	trackName     string
	trackTag      string

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
		eventSelected:      -1,
		processSelected:    -1,
		sessionSelected:    -1,
		ruleSelected:       -1,
		commSelected:       -1,
		pathSelected:       -1,
		prefixSelected:     -1,
		eventVisibleLimit:  50,
		disabledEventTypes: make(map[int]bool),
		ruleAction:         "ALERT",
		rulePriority:       "0",
		ruleRewrite:        "[]",
	}
	a.eventTable.Selected = &a.eventSelected
	a.processTable.Selected = &a.processSelected
	a.sessionTable.Selected = &a.sessionSelected
	a.rulesTable.Selected = &a.ruleSelected
	a.commTable.Selected = &a.commSelected
	a.pathTable.Selected = &a.pathSelected
	a.prefixTable.Selected = &a.prefixSelected
	return a
}

func (a *renewApp) runPolling(ctx context.Context, session *backendSession) {
	a.startEventUIBatcher(ctx)
	if session != nil && session.reader != nil && session.nativeIPCVersion >= nativeIPCVersion {
		go a.runNativeIPC(ctx, session)
	} else {
		go a.runSystemStats(ctx)
		go a.runEventSummaryStream(ctx)
	}
	a.refresh(ctx)
	a.refreshConfiguration(ctx)
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
			a.events = mergeEventSummaries(a.events, snapshot.Events, 1200)
			a.eventsVersion++
			if !a.historyInitialized {
				a.historyCursor = snapshot.NextCursor
				a.historyInitialized = true
			}
		}
		a.health = snapshot.Health
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
				case "会话":
					a.sessionsView(c)
				case "网络":
					a.networkView(c)
				case "进程":
					a.processesView(c)
				case "监控":
					a.monitoringView(c)
				case "规则":
					a.rulesView(c)
				case "跟踪":
					a.trackingView(c)
				case "系统":
					a.systemView(c)
				default:
					a.overview(c)
				}
			})
			a.eventDetailModal(c)
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
			for _, page := range []string{"概览", "事件", "会话", "网络", "进程", "监控", "规则", "跟踪", "系统"} {
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
		if pageUsesEventSearch(a.page) {
			ui.SearchField(c, &a.search).Label("搜索当前摘要").Width(280)
		}
		label := "暂停"
		if a.paused {
			label = "继续"
		}
		if ui.Button(c, label).Clicked() {
			a.paused = !a.paused
			a.eventUIPaused.Store(a.paused)
			if !a.paused {
				go a.refresh(context.Background())
			}
		}
		if ui.Button(c, "刷新").Clicked() {
			go a.refresh(context.Background())
			if a.page == "监控" || a.page == "规则" || a.page == "跟踪" {
				go a.refreshConfiguration(context.Background())
			}
		}
	})
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

func (a *renewApp) eventFilterCacheKey() string {
	q := strings.ToLower(strings.TrimSpace(a.search))
	key := q + "\x00" + a.eventTypeFilter + "\x00" + a.eventSessionFilter + "\x00" + a.eventDecisionFilter
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

	out := make([]eventSummary, 0, min(len(a.events), 256))
	for _, event := range a.events {
		if q != "" && !strings.Contains(eventSearchText(event), q) {
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
