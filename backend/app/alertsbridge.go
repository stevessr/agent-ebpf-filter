package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
)

// ── Semantic alert bridge ─────────────────────────────────────────

const (
	semanticPromptLoopThreshold = events.SemanticPromptLoopThreshold
	semanticAPILoopThreshold    = events.SemanticAPILoopThreshold
	semanticFileIOLoopThreshold = events.SemanticFileIOLoopThreshold
)

func buildSemanticAlerts(event *pb.Event) []*pb.Event {
	events.Deps.SemanticAlertsState = semanticAlertsState
	return events.BuildSemanticAlerts(event)
}

func resetSemanticAlertState() {
	events.ResetSemanticAlertState()
	semanticAlertsState = events.Deps.SemanticAlertsState
}

// semanticAlertsState retains the same variable name for backward compat.
// The underlying type is now events.SemanticAlertState.
var semanticAlertsState = events.NewSemanticAlertState()
