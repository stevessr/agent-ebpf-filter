package app

import (
	"agent-ebpf-filter/app/runtime"
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
const (
	PluginKindEBPF    = types.PluginKindEBPF
	PluginKindWebhook = types.PluginKindWebhook
	PluginKindCommand = types.PluginKindCommand
)

const (
	PluginAttachTracepoint = types.PluginAttachTracepoint
	PluginAttachKprobe     = types.PluginAttachKprobe
	PluginAttachKretprobe  = types.PluginAttachKretprobe
	PluginAttachLSM        = types.PluginAttachLSM
	PluginAttachNone       = types.PluginAttachNone
)
