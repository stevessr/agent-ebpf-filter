package app

import (
	"sync"

	"agent-ebpf-filter/app/network"
	"agent-ebpf-filter/app/shell"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"

	"agent-ebpf-filter/app/events"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// AppContext aggregates the application's shared runtime state.
//
// The package-level AppCtx is initialized once in Main and is also attached
// to each request by ContextMiddleware for handlers that need it.
type AppContext struct {
	// ── Subpackage managers ──────────────────────────────────────────
	Network *network.Manager

	// ── Event system ────────────────────────────────────────────────
	Broadcast         chan *pb.Event
	EventClientHub    *protoClientHub
	EnvelopeClientHub *protoClientHub
	Upgrader          websocket.Upgrader

	// ── Runtime config ──────────────────────────────────────────────
	RuntimeSettings      *runtimeState
	CapturedEventArchive *eventArchive

	// ── Shell ───────────────────────────────────────────────────────
	ShellSessions *shell.Manager

	// ── Plugins ─────────────────────────────────────────────────────
	PluginRegistry *pluginStore

	// ── eBPF ────────────────────────────────────────────────────────
	TrackerMaps trackerMapSet

	// ── Feature registry (compile-time feature gates) ───────────────
	FeatureRegistry *FeatureRegistry

	// ── Tags & rules ────────────────────────────────────────────────
	TagsMu               sync.RWMutex
	TagMap               map[uint32]string
	TagNameToID          map[string]uint32
	NextTagID            uint32
	WrapperRulesMu       sync.RWMutex
	WrapperRules         map[string]core.WrapperRule
	DisabledCommsMu      sync.RWMutex
	DisabledComms        map[string]struct{}
	DisabledEventTypesMu sync.RWMutex
	DisabledEventTypes   map[uint32]struct{}

	// ── Flow / network analysis ─────────────────────────────────────
	NetworkFlowAggregator *flowAggregator

	// ── Telemetry ────────────────────────────────────────────────────
	OTelExporterStore     *otelExporterState
	EventRecordingStore   *eventRecordingState
	CollectorMetricsStore *metricsStoreBridge

	// ── Cluster ──────────────────────────────────────────────────────
	ClusterManager *clusterManager

	// ── Process / cgroup tracking ────────────────────────────────────
	TrackedProcessContexts *events.ProcessContextStore
	CgroupAttribution      *cgroupAttributionStore
	ToolBaseline           *toolBaselineStore

	// ── Semantic alerts ──────────────────────────────────────────────
	SemanticAlertsState *events.SemanticAlertState

	// ── Protocol detection ───────────────────────────────────────────
	ProtoCache *protoDetectionCache

	// ── AgentSight ───────────────────────────────────────────────────
	AgentSightUploadedEvents *agentSightEventStore

	// ── Domain forward proxy ─────────────────────────────────────────
	DomainForwardProxyService *domainForwardProxyRuntime
}

// AppCtx is the application's legacy dependency container.
var AppCtx *AppContext

func currentNetworkManager() *network.Manager {
	if AppCtx == nil {
		return nil
	}
	return AppCtx.Network
}

func bindAppNetworkState(appContext *AppContext) {
	if appContext == nil || appContext.Network == nil {
		return
	}
	dnsCorrelation = appContext.Network.DNSCache()
	networkFlowAggregator = newFlowAggregator()
	appContext.NetworkFlowAggregator = networkFlowAggregator
}

// Ctx extracts the AppContext attached to a gin request.
// It falls back to the package-level AppCtx for non-HTTP callers.
func Ctx(c *gin.Context) *AppContext {
	if v, ok := c.Get("appctx"); ok {
		return v.(*AppContext)
	}
	return AppCtx
}

// ContextMiddleware returns a gin middleware that stores the AppContext
// on the request context, making it available via Ctx(c *gin.Context).
func ContextMiddleware(ac *AppContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("appctx", ac)
		c.Next()
	}
}

// newAppContext creates and populates a new AppContext with default values.
// Callers must set RuntimeSettings, CapturedEventArchive, TrackerMaps after creation.
func newAppContext() *AppContext {
	return &AppContext{
		Network:           network.NewManager(),
		EventClientHub:    newProtoClientHub(),
		EnvelopeClientHub: newProtoClientHub(),
		TagMap: map[uint32]string{
			0: "Unknown", 1: "AI Agent", 2: "Git", 3: "Build Tool",
			4: "System Pkg", 5: "Runtime", 6: "System Tool",
			7: "Network Tool", 8: "Security", 9: "Shell",
			10: "Language Pkg", 11: "Container CLI", 12: "Agent CLI",
		},
		TagNameToID: map[string]uint32{
			"AI Agent": 1, "Git": 2, "Build Tool": 3, "System Pkg": 4,
			"Runtime": 5, "System Tool": 6, "Network Tool": 7,
			"Security": 8, "Shell": 9, "Language Pkg": 10,
			"Container CLI": 11, "Agent CLI": 12,
		},
		NextTagID:          13,
		WrapperRules:       make(map[string]core.WrapperRule),
		DisabledComms:      make(map[string]struct{}),
		DisabledEventTypes: make(map[uint32]struct{}),
	}
}
