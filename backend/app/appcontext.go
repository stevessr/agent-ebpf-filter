package app

import (
	"sync"

	"agent-ebpf-filter/app/network"
	"agent-ebpf-filter/app/sandbox"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"

	"github.com/gorilla/websocket"
)

// AppContext aggregates the most-widely-used global state into a single struct.
//
// This is an incremental step toward full dependency injection. A single
// package-level var AppCtx holds the reference, set once in Main().
// Future phases will:
//  1. Eliminate var AppCtx by passing *AppContext explicitly to every handler
//  2. Extract the struct into its own package with interface abstractions
//
// Until then, existing code reads globals through AppCtx.FieldName instead
// of its own var declaration.
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

	// ── Flow analysis ───────────────────────────────────────────────
	NetworkFlowAggregator *flowAggregator
}

// AppCtx is the application's dependency-injection container.
// Set once in Main(); read by handler and helper code.
// TODO(future): eliminate this singleton by passing *AppContext explicitly.
var AppCtx *AppContext