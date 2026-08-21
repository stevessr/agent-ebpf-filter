package core

import (
	"strings"
	"time"
)

// ── Shared value types (used by app and research packages) ──────────────────

// ResearchCount is a label/count pair used by dataset summaries.
type ResearchCount struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

// DatasetQualitySummary aggregates import readiness metrics for datasets.
type DatasetQualitySummary struct {
	ImportableCount     int      `json:"importableCount"`
	LabeledCount        int      `json:"labeledCount"`
	UnlabeledCount      int      `json:"unlabeledCount"`
	DuplicateCount      int      `json:"duplicateCount"`
	DominantLabel       string   `json:"dominantLabel,omitempty"`
	DominantLabelRatio  float64  `json:"dominantLabelRatio,omitempty"`
	ClassImbalance      bool     `json:"classImbalance"`
	FeatureOutOfRange   int      `json:"featureOutOfRange"`
	NormalizationStatus string   `json:"normalizationStatus"`
	Warnings            []string `json:"warnings,omitempty"`
}

// LoopDetectionFinding is one detected repetition pattern emitted by the
// loop-detection runtime.
type LoopDetectionFinding struct {
	ID              string    `json:"id"`
	ObservedAt      time.Time `json:"observedAt"`
	FirstSeen       time.Time `json:"firstSeen"`
	LastSeen        time.Time `json:"lastSeen"`
	ContextType     string    `json:"contextType"`
	ContextKey      string    `json:"contextKey"`
	RepeatCount     int       `json:"repeatCount"`
	WindowSeconds   int       `json:"windowSeconds"`
	Fingerprint     string    `json:"fingerprint"`
	Target          string    `json:"target"`
	EventTypes      []string  `json:"eventTypes"`
	Pids            []uint32  `json:"pids"`
	Comms           []string  `json:"comms"`
	Paths           []string  `json:"paths"`
	ToolNames       []string  `json:"toolNames"`
	AgentRunID      string    `json:"agentRunId,omitempty"`
	TaskID          string    `json:"taskId,omitempty"`
	ToolCallID      string    `json:"toolCallId,omitempty"`
	TraceID         string    `json:"traceId,omitempty"`
	RootAgentPID    uint32    `json:"rootAgentPid,omitempty"`
	PID             uint32    `json:"pid,omitempty"`
	Comm            string    `json:"comm,omitempty"`
	Reason          string    `json:"reason"`
	SuggestedAction string    `json:"suggestedAction"`
}

// NormalizeKernelRiskFeedbackSettings clamps kernel risk feedback settings.
func NormalizeKernelRiskFeedbackSettings(settings *KernelRiskFeedbackSettings) {
	if settings == nil {
		return
	}
	if settings.MinRiskScore <= 0 {
		settings.MinRiskScore = 85
	}
	if settings.MinRiskScore > 100 {
		settings.MinRiskScore = 100
	}
	if settings.MaxActionsPerMinute <= 0 {
		settings.MaxActionsPerMinute = 30
	}
	if settings.MaxActionsPerMinute > 600 {
		settings.MaxActionsPerMinute = 600
	}
	if settings.Enabled && !settings.EnforceNetwork && !settings.EnforceFileNames && !settings.EnforceExec {
		settings.EnforceNetwork = true
		settings.EnforceFileNames = true
		settings.EnforceExec = true
	}
}

// WorkerScanBatchSize bounds batch allocations in worker scan paths.
const WorkerScanBatchSize = 256

// UniqueStringsPreserveOrder deduplicates while keeping first-seen order.
func UniqueStringsPreserveOrder(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
