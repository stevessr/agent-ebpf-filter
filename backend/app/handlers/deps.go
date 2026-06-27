package handlers

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/network"

	"github.com/cilium/ebpf"
	"github.com/gorilla/websocket"
)

// ── Type re-exports ────────────────────────────────────────────────

type CapturedEventRecord = core.CapturedEventRecord
type RuntimeSettings = core.RuntimeSettings
type ProcessContext = events.ProcessContext
type RegisterPayload = events.RegisterPayload
type TLSCaptureStore = tls.TLSCaptureStore
type TLSCaptureController = tls.TLSCaptureController
type tlsCaptureBroadcaster = tls.TLSBroadcaster
type TLSCaptureRuleStore = tls.TLSCaptureRuleStore

type IPScope = network.IPScope

// ── Narrow interfaces ──────────────────────────────────────────────

// TrackerMaps provides access to eBPF tracker maps for handlers that
// manage process registration, comm tracking, path tracking, etc.
type TrackerMaps interface {
	AgentPidsPut(key, value any) error
	AgentPidsDelete(key any) error
	TrackedCommsIterate() *ebpf.MapIterator
	TrackedCommsPut(key, value any) error
	TrackedCommsDelete(key any) error
	TrackedPathsIterate() *ebpf.MapIterator
	TrackedPathsPut(key, value any) error
	TrackedPathsDelete(key any) error
	TrackedPrefixesIterate() *ebpf.MapIterator
	TrackedPrefixesPut(key, value any) error
	TrackedPrefixesDelete(key any) error
	CollectorStats() *ebpf.Map
}

// RuntimeSettingsStore provides runtime configuration access for handlers.
type RuntimeSettingsStore interface {
	Snapshot() RuntimeSettings
	RecentEvents(limit int) ([]CapturedEventRecord, string, error)
	TruncateEventLog() error
}

// ProcessContextStore provides process context management for handlers.
type ProcessContextStore interface {
	Set(pid uint32, ctx ProcessContext)
	Delete(pid uint32)
}

// Deps holds all dependencies injected by the app package at init time.
// Every field must be set before any handler processing begins.
var Deps struct {
	// eBPF tracker maps
	TrackerMaps TrackerMaps

	// Runtime settings
	RuntimeSettings RuntimeSettingsStore

	// Process context
	ProcessContexts ProcessContextStore

	// AgentSight event store
	AgentSightUploadedEvents interface{ Clear() }

	// Plugin handler closures
	PluginList         func() []any
	PluginGet          func(id string) (any, bool)
	PluginUpsert       func(manifest any) error
	PluginDelete       func(id string) error
	PluginSetEnabled   func(id string, enabled bool) (any, error)
	PluginValidateID   func(id string) error
	PluginSource       func(id string) (string, bool)
	PluginUnloadEBPF   func(id string)
	CompileUserBPF     func(id, source string) (objPath string, log []byte, err error)
	BPFTemplates       func() []any

	// Tags and rules (config handlers)
	GetTagID        func(name string) uint32
	GetTagName      func(id uint32) string
	SetWrapperRule  func(comm string, rule any)
	DeleteWrapperRule func(comm string)

	// WebSocket upgrader
	Upgrader *websocket.Upgrader

	// Event archive / data handlers
	EventArchiveClear          func()
	AgentSightEventsClear      func()
	RuntimeSettingsTruncateLog func() error

	// Benchmark handlers
	RunBenchmark        func() (run any, stats any)
	GetBenchmarkResults func() any

	// Network flow / TCP / DNS / bandwidth (enrichment handlers)
	NetworkFlowAggregator interface {
		Query(query any) any
		Get(flowID string) (any, bool)
	}
	TCPTracker interface {
		Snapshot() []any
	}
	DNSCorrelation interface {
		LookupIP(ip string) (string, bool)
		LookupDomain(domain string) (string, bool)
		Snapshot() any
	}
	GeoIPDB interface {
		Lookup(ip string) (any, bool)
	}

	// Analyze endpoint helper
	AnalyzeEndpoint func(endpoint string) (IPScope, string, string, float64)

	// Write proto or JSON
	WriteProtoOrJSON func(c interface{ SetJSON(int, any) }, status int, proto, fallback any)
}