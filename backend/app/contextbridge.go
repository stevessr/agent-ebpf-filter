package app

import (
	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/pb"
)

// ── Context event bridge functions ─────────────────────────────────

type processContext = events.ProcessContext
type registerPayload = events.RegisterPayload
type processContextStore = events.ProcessContextStore

func syncProcessContextDeps() {
	events.Deps.ProcessContexts = trackedProcessContexts
}

func enrichEventContext(event *pb.Event) *pb.Event {
	syncProcessContextDeps()
	return events.EnrichEventContext(event)
}

func applyBestEffortProcessContextToEvent(event *pb.Event) {
	syncProcessContextDeps()
	events.ApplyBestEffortProcessContextToEvent(event)
}

func buildArgvDigest(parts ...string) string {
	return events.BuildArgvDigest(parts...)
}

func buildArgvDigestFromCommand(comm string, args []string) string {
	return events.BuildArgvDigestFromCommand(comm, args)
}

func buildProcessContextFromRegister(req events.RegisterPayload) events.ProcessContext {
	return events.BuildProcessContextFromRegister(events.RegisterPayload(req))
}

func buildProcessContextFromWrapperRequest(req *pb.WrapperRequest, decision string, riskScore float64) events.ProcessContext {
	return events.BuildProcessContextFromWrapperRequest(req, decision, riskScore)
}

func buildProcessContextFromHookPayload(payload map[string]interface{}, toolName, path string) (uint32, events.ProcessContext) {
	return events.BuildProcessContextFromHookPayload(payload, toolName, path)
}

func newProcessContextStore() *processContextStore {
	return events.NewProcessContextStore()
}

// trackedProcessContexts retains the same variable name for backward compat.
// The underlying type is now events.ProcessContextStore.
var trackedProcessContexts = events.NewProcessContextStore()
