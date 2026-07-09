package app

import (
	"agent-ebpf-filter/app/network"
	"agent-ebpf-filter/app/runtime"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/app/types"
)

// Type aliases — these types are defined canonically in the types subpackage.
// During the incremental refactoring, the app package re-exports them so
// existing code within this package continues to compile unchanged.
type FeatureID = types.FeatureID
type FeatureDangerLevel = types.FeatureDangerLevel
type PluginKind = types.PluginKind
type PluginAttachKind = types.PluginAttachKind
type PluginManifest = types.PluginManifest

// Type aliases to the runtime subpackage — moved during refactoring.
type TracepointBootstrapStatus = runtime.TracepointBootstrapStatus

// Constant re-exports — these mirror the values in the types subpackage.
// The aliased types above make them fully interchangeable.
// (PluginKind and PluginAttachKind values moved to constant.go)

// ── TLS subpackage type aliases (exported types only) ───────────────────────
type TLSCaptureStore = tls.TLSCaptureStore
type TLSCaptureRuleStore = tls.TLSCaptureRuleStore
type TLSCaptureRule = tls.TLSCaptureRule
type TLSCaptureStats = tls.TLSCaptureStats
type tlsCaptureBroadcaster = tls.TLSBroadcaster
type TLSCaptureController = tls.TLSCaptureController
type TLSProbeManager = tls.TLSProbeManager
type FragmentAssembler = tls.FragmentAssembler
type TLSHTTPStreamAssembler = tls.TLSHTTPStreamAssembler
type TLSPlaintextEvent = tls.TLSPlaintextEvent
type TLSBuiltinExecutableAttachStatus = tls.TLSBuiltinExecutableAttachStatus
type TLSLibraryStatus = tls.TLSLibraryStatus
type completedTLSFragment = tls.CompletedTLSFragment
type tlsProbeTarget = tls.ProbeTarget
type tlsAgentLoopState = tls.AgentLoopState
type RustlsOffsets = tls.RustlsOffsets
type AgentSightTLSAnalyzer = tls.AgentSightTLSAnalyzer
type AgentSightTLSFilter = tls.AgentSightTLSFilter
type AgentSightAnalyzer = tls.AgentSightAnalyzer
type AgentSightHTTPAnalyzer = tls.AgentSightHTTPAnalyzer
type AgentSightHTTPFilter = tls.AgentSightHTTPFilter
type codexCaptureSink = tls.CodexCaptureSink

// ── Network subpackage type and function aliases ─────────────────────────
type NetworkAuditResult = network.NetworkAuditResult
type NetworkAuditFinding = network.NetworkAuditFinding
type NetworkAuditFlags = network.NetworkAuditFlags

func AuditNetworkBehavior(comm, cmdline string) NetworkAuditResult {
	return network.AuditNetworkBehavior(comm, cmdline)
}
