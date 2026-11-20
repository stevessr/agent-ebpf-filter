package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type renewApp struct {
	win     *mygo.Window
	backend string
	client  *apiClient

	starting    bool
	connected   bool
	healthReady bool
	lastErr     string
	lastSync    time.Time

	page                  string
	lineageMode           int // 0 radial, 1 waterfall
	behaviorWindowMS      int64
	behaviorPID           int
	behaviorLane          behaviorLane
	behaviorAttentionOnly bool
	behaviorPausedEvents  []eventSummary
	behaviorPaused        bool
	behaviorFrozen        *behaviorSnapshot
	behaviorSelectedID    string
	behaviorCacheValid    bool
	behaviorCacheVersion  uint64
	behaviorCacheSecond   int64
	behaviorCacheWindow   int64
	behaviorCachePID      int
	behaviorCache         behaviorSnapshot
	lineageRootChoice     string
	lineageSelectedPID    int
	lineageSignalTable    ui.ListState
	lineageSignalSelected int
	files                 localFileWorkspace
	search                string
	paused                bool

	// Local native terminal lifecycle stays outside the privileged backend.
	terminalInitialized  bool
	terminalTabs         []*renewTerminalTab
	terminalActive       int
	terminalTabScroll    ui.ScrollState
	terminalNextID       int
	terminalFocusRequest int
	terminalError        string
	// Coding CLI detection is cached until explicitly refreshed.
	codingHarnessPaths map[string]string
	// Installer discovery is read-only and decoupled from the privileged backend.
	harnessUpdatePlans    []harnessUpdatePlan
	harnessUpdateScanned  bool
	harnessUpdateBusy     bool
	harnessUpdateSelected string
	harnessUpdateConfirm  string
	harnessUpdateError    string
	// DeepSeek runs as an unprivileged terminal pane; only its loopback UI is embedded.
	deepseekPane    *renewTerminalPane
	deepseekWebView *mygo.WebView
	deepseekPort    int
	deepseekStatus  string
	deepseekVisible bool
	terminalClosing atomic.Bool
	// Floating windows share this process and never start another collector.
	detachedParent    *renewApp
	detachedTerminals map[int]*renewApp
	detachedPages     map[string]*renewApp
	detachedPageID    string
	// True only when the main window was hidden to keep detached windows alive.
	hiddenForDetached  bool
	navigationOpen     bool
	navigationDetailed bool
	inspectorOpen      bool
	materialEnabled    bool
	tray               *mygo.Tray
	trayAvailable      bool
	minimizeToTray     bool
	// Mutable per-window theme, refreshed from MyGo's system theme every frame.
	workspaceTheme      ui.Theme
	inspectorTab        int
	inspectorPinnedID   string
	inspectorSelectedID string

	events              []eventSummary
	eventMergeScratch   []eventSummary
	eventMergeWorkspace eventMergeWorkspace
	eventsVersion       uint64
	eventUIQueue        chan eventSummary
	// Serializing producers makes the bounded drop-oldest sequence atomic
	// relative to other enqueues, and keeps exact drop accounting.
	eventUIQueueMu         sync.Mutex
	eventUIDropped         eventDropCounter
	eventUIQueueDropped    eventDropCounter
	eventUIBatchDropped    eventDropCounter
	eventUIBatchBuffered   atomic.Int64
	eventUIBatchPeak       atomic.Int64
	eventUICommits         atomic.Uint64
	eventUICommitted       atomic.Uint64
	eventUILastCommit      atomic.Int64
	eventUIBusyTicks       atomic.Uint64
	eventUIPaused          eventPauseFlag
	eventUIBatcherStarted  eventBatcherFlag
	eventUICommitPending   eventCommitFlag
	filterCacheValid       bool
	filterCacheVersion     uint64
	filterCacheKey         eventFilterSignature
	filterSearchOriginal   string
	filterSearchNormalized string
	filterCacheRows        []eventSummary
	filterMergeScratch     []eventSummary
	riskCacheValid         bool
	riskCacheVersion       uint64
	riskCacheCounts        [3]int
	// The ownership index was previously rebuilt by each event row,
	// leading to O(visible rows * event window) CPU work and allocations.
	// Cache only the event-derived index; live-process attribution must
	// still use a fresh, independently verified process snapshot.
	ownershipCacheValid   bool
	ownershipCacheVersion uint64
	ownershipCacheCount   int
	ownershipCacheFirstID string
	ownershipCacheLastID  string
	ownershipCache        agentOwnershipIndex
	// Local, session-scoped alert triage. Never changes backend policy.
	personalAlertMuted              map[[32]byte]struct{}
	personalAlertMuteOrder          [][32]byte
	personalAlertMuteVersion        uint64
	personalAlertCacheReady         bool
	personalAlertCacheEventsVersion uint64
	personalAlertCacheMuteVersion   uint64
	personalAlertCacheRows          []personalAlertGroup
	personalAlertCacheHidden        int
	networkCacheValid               bool
	networkCacheVersion             uint64
	networkCacheKey                 eventFilterSignature
	networkCacheRows                []networkAggregate
	processCacheValid               bool
	processCacheVersion             uint64
	processCacheKey                 eventFilterSignature
	processCacheRows                []processAggregate
	domainCacheValid                bool
	domainCacheVersion              uint64
	domainCacheRows                 []agentDomainRow
	optionCacheValid                bool
	optionCacheVersion              uint64
	eventTypeOptions                []string
	eventSessionOptions             []string
	health                          collectorHealth
	trackedComms                    []string
	system                          systemSnapshot
	systemConnected                 bool
	systemErr                       string
	eventStreamConnected            bool
	eventStreamErr                  string
	historyCursor                   string
	historyInitialized              bool
	historyLoading                  bool

	eventTable                  ui.ListState
	networkTable                ui.ListState
	networkSelected             int
	processTable                ui.ListState
	domainTable                 ui.ListState
	domainSelected              int
	domainSearch                string
	domainAgentFilter           string
	domainRiskOnly              bool
	domainShowIPs               bool
	domainLastFilter            string
	domainSelectedKey           string
	domainLastVersion           uint64
	configProjectPath           string
	configLoaded                bool
	configLoading               bool
	configGeneration            uint64
	configInspection            agentConfigInspection
	configTable                 ui.ListState
	sessionTable                ui.ListState
	agentSightModelTable        ui.ListState
	agentSightSessionTable      ui.ListState
	agentSightModelSelected     int
	agentSightSessionSelected   int
	agentSightReport            agentSightUsageResponse
	agentSightReady             bool
	agentSightLoading           bool
	agentSightError             string
	agentSightChecked           time.Time
	agentSightSynced            time.Time
	rulesTable                  ui.ListState
	commTable                   ui.ListState
	pathTable                   ui.ListState
	prefixTable                 ui.ListState
	eventSelected               int
	processSelected             int
	sessionSelected             int
	ruleSelected                int
	commSelected                int
	pathSelected                int
	prefixSelected              int
	eventPIDFilter              int
	eventRootPIDFilter          int
	eventTargetFilter           string
	eventDomainAgent            string
	eventDomainTarget           string
	eventDomainKind             string
	eventFileEditsOnly          bool
	eventDelegatedOnly          bool
	eventReturnPage             string
	eventRiskFilter             string
	eventMinRiskScore           float64
	eventTypeFilter             string
	eventSessionFilter          string
	eventDecisionFilter         string
	eventAttentionOnly          bool
	eventVisibleLimit           int
	eventDetailOpen             bool
	eventDetailLoading          bool
	eventDetailID               string
	eventDetail                 map[string]any
	eventDetailText             string
	eventDetailErr              string
	eventDetailTab              int
	eventProcessTab             int
	eventProcessSelectedPID     int
	eventProcessExpanded        map[int]bool
	eventDetailFieldSearch      string
	eventDetailFieldOffset      int
	eventDetailSearchSnapshot   string
	eventDetailExpandedField    string
	eventDetailGeneration       uint64
	eventDetailEnforcementReady bool
	eventDetailPendingAction    string
	enforcement                 enforcementSnapshot
	enforcementBusy             bool
	enforcementErr              string
	pathAccessLoaded            bool
	pathAccessBusy              bool
	pathAccessErr               string
	pathAccessState             lsmSandboxStatus
	pathAccessTarget            string
	pathAccessMode              string
	pathAccessConfirm           bool
	pathAccessConfirmText       string
	pathAccessPending           fileAccessRule

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

	registryReady            bool
	registryBusy             bool
	registryErr              string
	registryTab              int
	registryFilterTag        string
	registryFilterStatus     string
	registryLastFilterTag    string
	registryLastFilterStatus string
	registry                 registrySnapshot
	newTag                   string
	trackName                string
	trackTag                 string

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
		backend: backend,
		// The channel pointer never changes after construction. Both the
		// native IPC reader and diagnostics can safely access it.
		eventUIQueue:              make(chan eventSummary, eventUIQueueSize),
		starting:                  true,
		page:                      "概览",
		files:                     localFileWorkspace{active: -1},
		navigationOpen:            false,
		inspectorOpen:             true,
		materialEnabled:           true,
		eventSelected:             -1,
		networkSelected:           -1,
		processSelected:           -1,
		domainSelected:            -1,
		domainShowIPs:             true,
		sessionSelected:           -1,
		agentSightModelSelected:   -1,
		agentSightSessionSelected: -1,
		ruleSelected:              -1,
		commSelected:              -1,
		pathSelected:              -1,
		prefixSelected:            -1,
		eventVisibleLimit:         50,
		agentSelected:             -1,
		captureScopeMode:          "黑名单",
		monitorScopeMode:          "黑名单",
		disabledEventTypes:        make(map[int]bool),
		ruleAction:                "ALERT",
		rulePriority:              "0",
		ruleRewrite:               "[]",
		pathAccessMode:            "阻止写入",
	}
	a.eventTable.Selected = &a.eventSelected
	a.networkTable.Selected = &a.networkSelected
	a.processTable.Selected = &a.processSelected
	a.domainTable.Selected = &a.domainSelected
	a.sessionTable.Selected = &a.sessionSelected
	a.agentSightModelTable.Selected = &a.agentSightModelSelected
	a.agentSightSessionTable.Selected = &a.agentSightSessionSelected
	a.lineageSignalTable.Selected = &a.lineageSignalSelected
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
			// Pausing local event presentation must not hide a disconnected
			// or unhealthy collector. Refresh status without event payloads.
			a.refresh(ctx)
		}
	}
}

func (a *renewApp) refresh(parent context.Context) {
	if a.client == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	fullSnapshot := a.shouldFetchEventWindow()
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

func (a *renewApp) startingView(c *ui.Context) {
	t := c.Theme()
	card(c, "明镜高悬 · 正在连接监控后端", func() {
		ui.Text(c, "正在复用现有后端，或通过系统授权启动内置后端。桌面界面始终以普通用户权限运行。").TextColor(t.TextMuted)
		renewLoadingIndicator(c, "正在连接监控后端")
	})
}

// Retry a failed initial launch, or refresh an existing connection without
// starting duplicate event streams and backend polling loops.
func (a *renewApp) retryBackendConnection() {
	if a.client != nil {
		a.refreshActiveView()
		return
	}
	if a.starting {
		return
	}
	a.starting = true
	a.lastErr = ""
	go a.bootstrap(context.Background())
}

func (a *renewApp) errorView(c *ui.Context) {
	t := c.Theme()
	card(c, "监控服务尚未连接", func() {
		ui.Text(c, "目前无法确认新的设备活动是否正在被记录。您可以重新连接，或打开系统诊断了解原因。").
			TextColor(t.TextMuted)
		if a.lastErr != "" {
			ui.Text(c, "连接详情："+a.lastErr).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Row(c).Gap(8).Wrap().Children(func() {
			if ui.PrimaryButton(c, "重新连接").Clicked() {
				a.retryBackendConnection()
			}
			if ui.Button(c, "打开系统诊断").Clicked() {
				a.page = "系统"
			}
		})
		ui.Text(c, "高级用法：可以通过 --backend 或 AGENT_BACKEND_URL 连接已有监控服务。").
			FontSize(11).TextColor(t.TextMuted)
	})
}
