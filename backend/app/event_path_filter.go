package app

import (
	"agent-ebpf-filter/internal/eventnoise"
	"agent-ebpf-filter/pb"
)

// Keep the existing internal helpers and API contracts, but isolate the
// deterministic noise policy from protobuf and mutable runtime state.
const ignoredPathBypassRiskScore = eventnoise.BypassRiskScore

func defaultIgnoredEventPaths() []string {
	return eventnoise.DefaultIgnoredPaths()
}

func routineSystemNoisePaths() []string {
	return eventnoise.RoutineNoisePaths()
}

func normalizeIgnoredEventPaths(values []string) ([]string, error) {
	return eventnoise.NormalizeIgnoredPaths(values)
}

func pathMatchesIgnoredPrefix(path string, ignored []string) bool {
	return eventnoise.MatchesPrefix(path, ignored)
}

func eventPathNoiseEligible(event *pb.Event) bool {
	if event == nil {
		return false
	}
	return eventnoise.ReadOrMetadataEligible(event.GetType(),
		event.GetEventType() == pb.EventType_OPENAT ||
			event.GetEventType() == pb.EventType_OPEN ||
			event.GetEventType() == pb.EventType_READ)
}

func isUsrBinReadWriteNoise(event *pb.Event) bool {
	if event == nil {
		return false
	}
	return eventnoise.UsrBinReadWriteNoise(event.GetType(), event.GetPath(),
		event.GetExtraPath(), event.GetEventType() == pb.EventType_READ ||
			event.GetEventType() == pb.EventType_WRITE)
}

func eventBypassesIgnoredPaths(event *pb.Event) bool {
	if event == nil {
		return false
	}
	return eventnoise.BypassesIgnoredPaths(event.GetDecision(),
		int64(event.GetRiskScore()), event.GetType())
}

func eventNoiseInput(event *pb.Event) eventnoise.Event {
	return eventnoise.Event{
		Type:      event.GetType(),
		Decision:  event.GetDecision(),
		Path:      event.GetPath(),
		ExtraPath: event.GetExtraPath(),
		RiskScore: int64(event.GetRiskScore()),
		KernelOpenRead: event.GetEventType() == pb.EventType_OPENAT ||
			event.GetEventType() == pb.EventType_OPEN ||
			event.GetEventType() == pb.EventType_READ,
		KernelReadWrite: event.GetEventType() == pb.EventType_READ ||
			event.GetEventType() == pb.EventType_WRITE,
	}
}

// shouldIgnoreEventPath runs only after semantic alert evaluation. A high-risk
// event, an alert, or a denied action always reaches the user. An explicit
// empty ignoredPaths list disables all built-in noise suppressions.
func shouldIgnoreEventPath(event *pb.Event) bool {
	if event == nil || eventBypassesIgnoredPaths(event) || runtimeSettingsStore == nil {
		return false
	}
	// The pure matcher reads configured paths under the existing runtime lock;
	// no per-event slice allocation or mutating global policy is introduced.
	runtimeSettingsStore.mu.RLock()
	defer runtimeSettingsStore.mu.RUnlock()
	return eventnoise.ShouldIgnore(eventNoiseInput(event),
		runtimeSettingsStore.settings.IgnoredPaths)
}
