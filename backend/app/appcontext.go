package app

import (
	"sync"

	"agent-ebpf-filter/app/network"
	"agent-ebpf-filter/app/sandbox"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// AppContext aggregates the most-widely-used global state into a single struct.
//
// This is an incremental step toward full dependency injection. A single
// package-level var AppCtx holds the reference, set once in Main().
// Future phases will eliminate var AppCtx by passing *AppContext explicitly.
type AppContext struct {
	// ── Subpackage managers ──────────────────────────────────────────
	Network *network.Manager
	Sandbox *sandbox.Manager

	// ── Event system ────────────────────────────────────────────────
	Broadcast        chan *pb.Event
	Clients          map[*websocket.Conn]bool
	ClientsMu        sync.Mutex
	EnvelopeClients  map[*websocket.Conn]bool
	EnvelopeClientsMu sync.Mutex
	Upgrader         websocket.Upgrader

	// ── Runtime config ──────────────────────────────────────────────
	RuntimeSettings      *runtimeState
	CapturedEventArchive *eventArchive

	// ── Shell ───────────────────────────────────────────────────────
	ShellSessions *shellSessionManager

	// ── ML engine ───────────────────────────────────────────────────
	MLEngine         Model
	MLEnabled        bool
	MLConfig         MLConfig
	MLModelLoaded    bool
	CurrentModelType ModelType

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
	CollectorMetricsStore *collectorMetricsState

	// ── Cluster ──────────────────────────────────────────────────────
	ClusterManager *clusterManager

	// ── Process / cgroup tracking ────────────────────────────────────
	TrackedProcessContexts *processContextStore
	CgroupAttribution      *cgroupAttributionStore
	ToolBaseline           *toolBaselineStore

	// ── Semantic alerts ──────────────────────────────────────────────
	SemanticAlertsState *semanticAlertState

	// ── ML training / prediction (stub interface) ────────────────────
	GlobalTrainingStore interface{}
	GlobalTrainer       interface{}
	GlobalAutoTuneState interface{}
	GlobalPredictionCache interface{}

	// ── Protocol detection ───────────────────────────────────────────
	ProtoCache *protoDetectionCache

	// ── AgentSight ───────────────────────────────────────────────────
	AgentSightUploadedEvents *agentSightEventStore

	// ── Domain forward proxy ─────────────────────────────────────────
	DomainForwardProxyService *domainForwardProxyRuntime
}

// AppCtx is the application's dependency-injection container.
// Deprecated: use ctx.From(c) in handlers instead.
var AppCtx *AppContext

// Ctx extracts the AppContext from a gin request context.
// Deprecated: this is a thin wrapper for backward compat.
// New code should import "agent-ebpf-filter/app/ctx" and use ctx.From(c).
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
		Network:  network.NewManager(),
		Sandbox:  sandbox.NewManager(),
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
		NextTagID:         13,
		WrapperRules:      make(map[string]core.WrapperRule),
		DisabledComms:     make(map[string]struct{}),
		DisabledEventTypes: make(map[uint32]struct{}),
	}
}