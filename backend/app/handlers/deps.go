package handlers

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/observability"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/pb"
	"context"
	"os/exec"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
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
type FilePreviewResponse = core.FilePreviewResponse
type VmFaultCounters = observability.VmFaultCounters
type GpuInfo = observability.GpuInfo

// WrapperRule is a rule for the agent-wrapper (mirrors core.WrapperRule).
type WrapperRule struct {
	Comm         string   `json:"comm"`
	Action       string   `json:"action"`
	RewrittenCmd []string `json:"rewritten_cmd,omitempty"`
	Regex        string   `json:"regex,omitempty"`
	Replacement  string   `json:"replacement,omitempty"`
	Priority     int      `json:"priority,omitempty"`
}

// CameraStream is a live camera feed broadcast stream (struct-of-funcs bridge).
type CameraStream struct {
	SubscribeFn func() CameraSubscriber
}

// Subscribe delegates to the injected SubscribeFn.
func (s *CameraStream) Subscribe() CameraSubscriber {
	if s.SubscribeFn != nil {
		return s.SubscribeFn()
	}
	return nil
}

// CameraSubscriber receives frames from a CameraStream.
type CameraSubscriber interface {
	NextFrame(ctx context.Context) ([]byte, error)
	Unsubscribe()
}

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
	ConfigTagNames      func() []string
	IsCommDisabled      func(comm string) bool
	AddDisabledComm     func(comm string)
	RemoveDisabledComm  func(comm string)
	DeleteDisabledComm  func(comm string)
	DisabledEventTypes       func() []uint32
	AddDisabledEventType     func(et uint32)
	RemoveDisabledEventType  func(et uint32)
	ConfigRules              func() []*pb.WrapperRule
	UpsertConfigRule         func(comm, action, rewrittenCmd, regex, replacement string, priority int32)
	DeleteConfigRule         func(comm string)

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

	// Camera stream access (hardware handlers)
	GetCameraStream func(devName string) *CameraStream

	// Write proto or JSON response (uses concrete gin.Context)
	WriteProtoOrJSON func(c *gin.Context, code int, msg proto.Message, jsonData interface{})

	// System / platform helper closures
	GetRealHomeDir            func() string
	ResolveWrapperPath         func() string
	DropPrivileges             func(cmd *exec.Cmd)
	ConfigureCommandForRealUser func(cmd *exec.Cmd)
	OriginalInvokerIDs         func() (uid, gid uint32, ok bool)

	// File preview
	BuildFilePreview func(path string) (any, error)

	// Stats / observability closures (system stats WS)
	GetGPUMetrics       func() (map[int32]GpuInfo, []*pb.GPUStatus)
	ReadVMFaultCounters func() (VmFaultCounters, error)
	VMFaultCountersZero func() VmFaultCounters
	GetCoreTypes        func() []pb.CPUInfo_Core_Type
	GetZramStats        func() (used, total uint64)
	BroadcastCh         chan<- *pb.Event
	EventSchemaVersion  string
	SendTLSBridge       func(bridge chan<- *pb.Event, event *pb.Event)
}