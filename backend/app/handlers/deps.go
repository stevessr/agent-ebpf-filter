package handlers

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/observability"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/internal/geoip"
	netcore "agent-ebpf-filter/internal/network"
	"agent-ebpf-filter/pb"
	"context"
	"os/exec"
	"time"

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

type IPScope = netcore.IPScope
type FilePreviewResponse = core.FilePreviewResponse
type VmFaultCounters = observability.VmFaultCounters
type GpuInfo = observability.GpuInfo
type ExportConfig = core.ExportConfig

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
	RuntimeSettings           RuntimeSettingsStore
	RuntimeSettingsReplace   func(s RuntimeSettings) (RuntimeSettings, error)

	// Process context
	ProcessContexts ProcessContextStore

	// AgentSight event store
	AgentSightUploadedEvents interface {
		Clear()
		Recent(limit int) []any
		Add(events ...any)
	}

	// AgentSight data pipeline helpers (wired from app-level functions)
	RecentEventFiltersFromRequest func(c any) any // *gin.Context -> recentEventFilters
	FilterRecentEventRecords      func(records []CapturedEventRecord, filters any) []CapturedEventRecord
	NormalizeCapturedEventRecord func(record CapturedEventRecord) CapturedEventRecord
	EventEnvelopeToJSONValue     func(envelope *pb.EventEnvelope) map[string]any
	EnvelopeEventTypeName        func(envelope *pb.EventEnvelope, event *pb.Event) string
	ParseRecentEventTime         func(raw string) time.Time

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
		Query(query netcore.FlowQuery) netcore.FlowQueryResult
		Get(flowID string) (netcore.NetworkFlowSummary, bool)
	}
	TCPTracker interface {
		Snapshot() []netcore.TCPConnectionState
	}
	DNSCorrelation interface {
		LookupIP(ip string) (string, bool)
		LookupDomain(domain string) (string, bool)
		Snapshot() []netcore.DNSCacheSnapshotEntry
	}
	GeoIPDB interface {
		Lookup(ip string) (geoip.Record, bool)
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

	// Export config post-processing (wired from app-level)
	ApplyRetentionConfig           func(settings RuntimeSettings)
	ApplyRuntimeDomainForwardProxy func(settings RuntimeSettings)
	BuildRuntimeConfigResponse         func() core.RuntimeConfigResponse
	BuildRuntimeConfigResponseFromSettings func(s RuntimeSettings) core.RuntimeConfigResponse
	RotateAccessToken              func(settings RuntimeSettings) RuntimeSettings
	ApplyMLConfigPatch             func(dst *core.MLConfig, patch interface{})

	// ML handler closures — all return gin.H or simple types to avoid type coupling
	MLStatus              func() *pb.MLStatus
	BuildMLStatusJSON     func() []byte
	MLEnabled             func() bool
	MLConfig              func() core.MLConfig
	CurrentMLConfig       func() core.MLConfig
	MLIsRunning           func() bool
	MLLogTotal            func() int
	MLGetLogsResponse     func() gin.H
	MLCancelTraining      func()
	MLGetHistoryResponse  func() gin.H
	MLTrain               func(numTrees, maxDepth, minLeaf int) gin.H
	MLFeedbackResult      func(comm, action string) gin.H
	MLSamplesResponse     func() gin.H
	MLSampleLabelResult   func(index int, label string) gin.H
	MLRemoveSampleResult  func(index int) gin.H
	MLSampleAnomalyResult func(index int, score float64) gin.H
	MLAddSample           func(cmdLine, comm string, args []string, label string) gin.H
	MLClassifyAndEmbed    func(comm string, args []string) (interface{}, []float64)
	MLComputeAnomalyScore func(emb []float64) float64
	MLPredict             func(comm string, args []string) MLPrediction
	MLNetworkAudit        func(comm string, args []string) MLNetworkAuditResult
	MLLLMAssessment       func(comm string, args []string) *MLLlmAssessment
	MLExistingCommands      func() []string
	MLImportResult          func() gin.H
	MLAssessCommandSafety   func(c *gin.Context)
	MLExistingCommandsGetFn func(c *gin.Context)
	MLImportExistingFn      func(c *gin.Context)
	MLTuneResult          func() gin.H
	MLTuneModelsResult    func(models []string) gin.H
	MLLLMScoreResult      func(cmdLine, comm string, args []string) gin.H
	MLLLMBatchScoreResult func(samples []gin.H) gin.H
	MLLlmProductionDatasetPullResult func() gin.H
	MLClassicDatasetsList func() gin.H
	MLClassicDatasetGetResult   func(name string) gin.H
	MLClassicDatasetPreviewResult func(name string) gin.H
	MLDatasetPullResult   func(url string) gin.H
	MLDatasetImportResult func(name string) gin.H
	MLDatasetExportResult func() gin.H
	MLDatasetClear        func()
	MLHealthProcesses     func() gin.H
	MLHealthGenerators    func() gin.H
	MLHealthRegister      func(id string)
	MLHealthUnregister    func(id string)
	MLHealthRun           func() gin.H

	// Hooks config closures
	AvailableHooks               func() []core.HookDef
	IsHookInstalled              func(core.HookDef) bool
	InstallNativeHook            func(core.HookDef) error
	UninstallNativeHook          func(core.HookDef) error
	GetShellConfigPath           func() string
	EnsureKiroManagedAgentExists func() error

	// External API / health handler closures
	BuildFeatureManifest      func(settings core.RuntimeSettings) any
	BootstrapTracepointStatus func() any
	CollectorHealth           func() any

	// Shell session manager (wired to *shell.Manager via adapter)
	ShellSessions interface {
		Subscribe() chan struct{}
		Unsubscribe(ch chan struct{})
		List() []any
		NewSession(req any, deps any) (any, error)
		Delete(id string) error
		SendInput(id string, data []byte) error
		ClearClosed()
	}
	MakeShellDeps func() any
}