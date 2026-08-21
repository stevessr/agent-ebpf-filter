package research

// Runtime hooks wired by the app package at startup. The research package
// must not import package app (app registers these routes), so live state
// flows in through these function variables.

import (
	"context"

	"agent-ebpf-filter/app/ml"
	"agent-ebpf-filter/internal/behavior"

	"agent-ebpf-filter/app/handlers"
	"agent-ebpf-filter/core"
)

var (
	// SnapshotRuntimeSettingsHook returns the live runtime settings.
	SnapshotRuntimeSettingsHook = func() core.RuntimeSettings { return core.RuntimeSettings{} }

	// RecentCapturedEventsHook mirrors runtimeState.RecentEvents.
	RecentCapturedEventsHook = func(limit int) ([]CapturedEventRecord, string, error) {
		return nil, "", nil
	}

	// RecentLoopFindingsHook exposes the loop-detection worker's recent findings.
	RecentLoopFindingsHook = func() []core.LoopDetectionFinding { return nil }

	// EmbedderHook returns the shared instruction embedder.
	EmbedderHook = func() *behavior.InstructionEmbedder { return behavior.DefaultEmbedder() }

	// FeatureExtractorHook returns the shared feature extractor. The default
	// zero-feature sink keeps unwired callers safe until app bootstrap runs.
	FeatureExtractorHook = func() TrainingFeatureSink { return noopTrainingFeatureSink{} }

	// RecordSampleSideEffectsHook mirrors app.recordCommandSampleSideEffects.
	RecordSampleSideEffectsHook = func(sample ml.TrainingSample) {}

	// SampleLabelNameHook mirrors app.sampleLabelName via ml.ActionLabel.
	SampleLabelNameHook = func(label int32) string {
		if label < 0 {
			return "-"
		}
		if name, ok := ml.ActionLabel[label]; ok {
			return name
		}
		return "-"
	}
)

// AgentSightUploadedEvents is the shared uploaded-event store; the app
// assigns it during bootstrap. Nil-safe: store methods tolerate nil receivers.
var AgentSightUploadedEvents *handlers.AgentSightEventStore

// AssessCommandSafetyHook runs the command-safety engine and returns a
// gin.H-shaped map; wired by app at bootstrap.
var AssessCommandSafetyHook = func(ctx context.Context, comm string, args []string, user string, pid uint32, includeLLM bool) map[string]any {
	return map[string]any{}
}

// TrainingFeatureSink is the feature-extraction surface used by research
// training; implemented by the app FeatureExtractor.
type TrainingFeatureSink interface {
	Extract(comm string, args []string, user string, pid uint32) [core.FeatureDim]float64
	AddHistory(comm, category, action string, anomalyScore float64, pid uint32, user string, argsLen int, argsCount int)
}

func snapshotRuntimeSettings() core.RuntimeSettings { return SnapshotRuntimeSettingsHook() }

func recentCapturedEvents(limit int) ([]CapturedEventRecord, string, error) {
	return RecentCapturedEventsHook(limit)
}

func recentLoopFindings() []core.LoopDetectionFinding { return RecentLoopFindingsHook() }

// noopTrainingFeatureSink produces zero-valued feature vectors; it exists so
// an unwired FeatureExtractorHook never dereferences nil.
type noopTrainingFeatureSink struct{}

func (noopTrainingFeatureSink) Extract(comm string, args []string, user string, pid uint32) [FeatureDim]float64 {
	return [FeatureDim]float64{}
}

func (noopTrainingFeatureSink) AddHistory(comm, category, action string, anomalyScore float64, pid uint32, user string, argsLen int, argsCount int) {
}
