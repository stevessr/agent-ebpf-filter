package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
)

// ── Semantic alert bridge ─────────────────────────────────────────

func buildSemanticAlerts(event *pb.Event) []*pb.Event {
	return events.BuildSemanticAlerts(event)
}

func resetSemanticAlertState() {
	events.ResetSemanticAlertState()
}

// semanticAlertsState retains the same variable name for backward compat.
// The underlying type is now events.SemanticAlertState.
var semanticAlertsState = events.NewSemanticAlertState()