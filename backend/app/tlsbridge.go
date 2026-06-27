package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/tls"
)

// ── tlsProcessContextAdapter bridges events.ProcessContext → tls.ProcessContext ──

type tlsProcessContextAdapter struct {
	store *events.ProcessContextStore
}

func (a *tlsProcessContextAdapter) Get(pid uint32) (tls.ProcessContext, bool) {
	ctx, ok := a.store.Get(pid)
	if !ok {
		return tls.ProcessContext{}, false
	}
	return tls.ProcessContext{
		RootAgentPid:   ctx.RootAgentPid,
		AgentRunID:     ctx.AgentRunID,
		TaskID:         ctx.TaskID,
		ConversationID: ctx.ConversationID,
		TurnID:         ctx.TurnID,
		ToolCallID:     ctx.ToolCallID,
		ToolName:       ctx.ToolName,
		TraceID:        ctx.TraceID,
		SpanID:         ctx.SpanID,
		Decision:       ctx.Decision,
		ContainerID:    ctx.ContainerID,
		ArgvDigest:     ctx.ArgvDigest,
		Cwd:            ctx.Cwd,
		RiskScore:      ctx.RiskScore,
	}, true
}

// ── Init TLS subpackage ──────────────────────────────────────────────────────

// initTLS is called during app initialization to inject global dependencies
// into the tls subpackage before TLS capture goroutines start.
func initTLS() {
	tls.Init(tls.Deps{
		Broadcast:              broadcast,
		TrackedProcessContexts:  &tlsProcessContextAdapter{store: trackedProcessContexts},
		CollectorMetrics:        &collectorMetricsStore,
		Upgrader:                &upgrader,
	})
}
