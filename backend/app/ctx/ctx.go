// Package ctx provides the application's dependency injection container.
//
// Usage:
//
//	// In main.go:
//	appCtx := ctx.New(network.NewManager(), sandbox.NewManager())
//	r.Use(ctx.Middleware(appCtx))
//
//	// In any handler:
//	func handleFoo(c *gin.Context) {
//	    appCtx := ctx.From(c)
//	    appCtx.Network.DoSomething()
//	}
//
// The package-level singleton AppCtx is kept for backward compatibility
// during the incremental migration. New code should use ctx.From(c).
package ctx

import (
	"sync"
	"time"

	"agent-ebpf-filter/app/network"
	"agent-ebpf-filter/app/sandbox"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ── Interfaces for app-private types (using any to avoid import cycles) ──

// RuntimeSettingsStore provides access to runtime configuration.
type RuntimeSettingsStore interface {
	LoadOrCreate() (core.RuntimeSettings, error)
	Snapshot() core.RuntimeSettings
	ExpectedToken() string
	HookSecret(id string) string
	Replace(settings core.RuntimeSettings) (core.RuntimeSettings, error)
	UpdateLogging(enabled bool, path string) (core.RuntimeSettings, error)
	RotateAccessToken() (core.RuntimeSettings, error)
	RecentEvents(limit int) ([]core.CapturedEventRecord, string, error)
	AppendEvent(record core.CapturedEventRecord) error
	TruncateEventLog() error
}

// EventArchive provides bounded thread-safe event storage.
type EventArchive interface {
	Add(record core.CapturedEventRecord)
	Snapshot(limit int) []core.CapturedEventRecord
	Clear()
	SetMax(n int)
	EvictOlderThan(threshold time.Time)
	Count() int
}

// ── Ctx: the DI container ─────────────────────────────────────────────────

// Ctx aggregates all formerly-global application state.
//
// Create once in Main(), attach to gin context via Middleware(),
// and retrieve in handlers via From(c).
type Ctx struct {
	// Subpackage managers
	Network *network.Manager
	Sandbox *sandbox.Manager

	// Event system
	Broadcast         chan *pb.Event
	Clients           map[*websocket.Conn]bool
	ClientsMu         sync.Mutex
	EnvelopeClients   map[*websocket.Conn]bool
	EnvelopeClientsMu sync.Mutex
	Upgrader          websocket.Upgrader

	// Runtime config
	RuntimeSettings      RuntimeSettingsStore
	CapturedEventArchive EventArchive

	// Shell (interface{} because concrete types live in app package)
	ShellSessions any

	// ML engine
	MLEngine         any
	MLEnabled        bool
	MLConfig         any
	MLModelLoaded    bool
	CurrentModelType any

	// Plugins
	PluginRegistry any

	// eBPF tracker maps
	TrackerMaps any

	// Feature registry
	FeatureRegistry any

	// Tags & rules
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

	// Flow / network analysis
	NetworkFlowAggregator any

	// Telemetry
	OTelExporterStore     any
	EventRecordingStore   any
	CollectorMetricsStore any

	// Cluster
	ClusterManager any

	// Process / cgroup tracking
	TrackedProcessContexts any
	CgroupAttribution      any
	ToolBaseline           any

	// Semantic alerts
	SemanticAlertsState any

	// ML training / prediction
	GlobalTrainingStore   any
	GlobalTrainer         any
	GlobalAutoTuneState   any
	GlobalPredictionCache any

	// Protocol detection
	ProtoCache any

	// AgentSight
	AgentSightUploadedEvents any

	// Domain forward proxy
	DomainForwardProxyService any
}

// New creates a Ctx with default initialisation of subpackage managers.
func New() *Ctx {
	return &Ctx{
		Network: network.NewManager(),
		Sandbox: sandbox.NewManager(),
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

// ── Middleware ────────────────────────────────────────────────────────────

// Middleware returns a gin middleware that stores the Ctx on the request
// context, making it available via From(c).
func Middleware(cx *Ctx) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set("appctx", cx)
		c.Next()
	}
}

// From extracts the Ctx from a gin request context.
// Panics if the middleware was not installed.
func From(c *gin.Context) *Ctx {
	if v, ok := c.Get("appctx"); ok {
		return v.(*Ctx)
	}
	// Fallback — the singleton must be set.
	if defaultCtx != nil {
		return defaultCtx
	}
	return nil
}

// ── Singleton (deprecated) ────────────────────────────────────────────────

// Default is the package-level singleton.
// Deprecated: use From(c) in handlers instead.
var defaultCtx *Ctx

// SetDefault sets the package-level singleton.
// Deprecated: use Middleware instead.
func SetDefault(cx *Ctx) {
	defaultCtx = cx
}

// GetDefault returns the package-level singleton.
// Deprecated: use From(c) instead.
func GetDefault() *Ctx {
	return defaultCtx
}