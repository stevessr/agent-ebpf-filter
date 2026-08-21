// Package research — research workbench: session store, processing
// runtime, security evaluation, exports and training readiness views.
//
// Bridge file: type aliases so moved files keep their original identifiers,
// plus the settings normalizer whose constants live in this package.

package research

import (
	"agent-ebpf-filter/app/handlers"
	"agent-ebpf-filter/app/tasks"
	"agent-ebpf-filter/app/tls"
	"agent-ebpf-filter/core"
)

type (
	CapturedEventRecord        = core.CapturedEventRecord
	ResearchProcessingSettings = core.ResearchProcessingSettings
	TLSCaptureStore            = tls.TLSCaptureStore
	DatasetQualitySummary      = core.DatasetQualitySummary
	researchCount              = core.ResearchCount
	loopDetectionFinding       = core.LoopDetectionFinding
)

type (
	TLSPlaintextEvent     = tls.TLSPlaintextEvent
	agentSightExportEvent = handlers.AgentSightExportEvent
	benchmarkCase         = core.BenchmarkCase
)

const FeatureDim = core.FeatureDim

type (
	backendTaskRuntime      = tasks.Runtime
	backendTaskRuntimeEntry = tasks.Entry
	backendTaskRuntimeStats = tasks.Stats
)

var (
	newUnstartedTaskRuntime    = tasks.NewUnstarted
	newBackendTaskRuntime      = tasks.New
	newBackendTaskRuntimeEntry = tasks.NewEntry
	newBackendTaskPanicError   = tasks.NewPanicError
	errBackendTaskCanceled     = tasks.ErrCanceled
	errBackendTaskQueueFull    = tasks.ErrQueueFull
)

// NormalizeResearchProcessingSettings clamps user-supplied research
// processing settings into documented bounds.
func NormalizeResearchProcessingSettings(s *ResearchProcessingSettings) {
	normalizeResearchProcessingSettings(s)
}

// normalizeResearchProcessingSettings clamps settings; constants live here.
func normalizeResearchProcessingSettings(settings *ResearchProcessingSettings) {
	if settings == nil {
		return
	}
	if settings.MaxEvents <= 0 {
		settings.MaxEvents = 5000
	}
	if settings.MaxEvents < 100 {
		settings.MaxEvents = 100
	}
	if settings.MaxEvents > 100000 {
		settings.MaxEvents = 100000
	}
	if settings.QueueSize <= 0 {
		settings.QueueSize = 2048
	}
	if settings.QueueSize < 128 {
		settings.QueueSize = 128
	}
	if settings.QueueSize > 65536 {
		settings.QueueSize = 65536
	}
	if settings.TimelineBucketSeconds <= 0 {
		settings.TimelineBucketSeconds = 60
	}
	if settings.TimelineBucketSeconds > 86400 {
		settings.TimelineBucketSeconds = 86400
	}
	if settings.TopK <= 0 {
		settings.TopK = 20
	}
	if settings.TopK > 200 {
		settings.TopK = 200
	}
	if settings.RecentSamples <= 0 {
		settings.RecentSamples = 25
	}
	if settings.RecentSamples > 500 {
		settings.RecentSamples = 500
	}
	if settings.ArtifactRetentionDays <= 0 {
		settings.ArtifactRetentionDays = researchProcessingDefaultArtifactRetentionDays
	}
	if settings.ArtifactRetentionDays > 3650 {
		settings.ArtifactRetentionDays = 3650
	}
	if settings.MaxSessionEvents <= 0 {
		settings.MaxSessionEvents = researchProcessingDefaultMaxSessionEvents
	}
	if settings.MaxSessionEvents < 100 {
		settings.MaxSessionEvents = 100
	}
	if settings.MaxSessionEvents > 100000 {
		settings.MaxSessionEvents = 100000
	}
	settings.ExportFormats = normalizeResearchExportFormats(settings.ExportFormats)
}
