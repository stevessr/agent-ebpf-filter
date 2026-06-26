package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
)

// ── Kernel risk wrappers (migrated to app/events/) ─────────────────────────

func applyKernelRiskDecision(raw *core.BpfEvent, event *pb.Event) {
	events.ApplyKernelRiskDecision(raw, event)
}

func startKernelRiskFeedbackWorker() {
	events.StartKernelRiskFeedbackWorker()
}
