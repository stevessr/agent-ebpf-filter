package app

import (
	"context"
	"sync"
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/core"
	"agent-ebpf-filter/pb"
)

// ── Kernel risk wrappers (migrated to app/events/) ─────────────────────────

type (
	kernelRiskDecision       = events.KernelRiskDecision
	kernelRiskFeedbackAction = events.KernelRiskFeedbackAction
)

type kernelRiskFeedbackState struct {
	mu          sync.Mutex
	seen        map[string]time.Time
	windowStart time.Time
	windowCount int
}

func (s *kernelRiskFeedbackState) Allow(action kernelRiskFeedbackAction, settings KernelRiskFeedbackSettings, now time.Time) bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.seen == nil {
		s.seen = make(map[string]time.Time)
	}
	key := action.Kind + "\x00" + action.Target
	if last, ok := s.seen[key]; ok && now.Sub(last) < 5*time.Minute {
		return false
	}
	if s.windowStart.IsZero() || now.Sub(s.windowStart) >= time.Minute {
		s.windowStart = now
		s.windowCount = 0
	}
	limit := settings.MaxActionsPerMinute
	if limit <= 0 {
		limit = 30
	}
	if s.windowCount >= limit {
		return false
	}
	s.windowCount++
	s.seen[key] = now
	return true
}

const maxKernelAuditDuration = 10 * time.Minute

// annotateKernelAuditTiming preserves raw kernel provenance and derives a
// wall-clock capture time without confusing userspace ingest time with probe
// time. Existing flow-level first/last timestamps remain authoritative.
func annotateKernelAuditTiming(raw *core.BpfEvent, event *pb.Event, observedAt time.Time) {
	if raw == nil || event == nil {
		return
	}
	observation := observeKernelKtime(raw.KernelTimestampNs, observedAt)
	capturedAt := observation.CapturedAt

	event.KernelTimestampNs = raw.KernelTimestampNs
	event.KernelSequence = raw.KernelSequence
	event.KernelCpu = raw.KernelCPU
	event.KernelClock = observation.Clock
	event.IngestTimestampNs = uint64(observation.IngestedAt.UnixNano())
	event.CaptureDelayNs = observation.DelayNS
	event.CaptureTimestampNs = uint64(capturedAt.UnixNano())
	event.AuditFlags = raw.AuditFlags
	event.KernelAuditGeneration = raw.KernelAuditGeneration
	event.KernelDroppedSinceLast = raw.KernelDroppedSinceLast
	event.KernelReserveFailuresTotal = raw.KernelReserveFailuresTotal
	recordKernelSequenceObservation(raw)
	collectorMetricsStore.RecordKernelCaptureTiming(observation.DelayNS, observation.Clock)

	if event.GetLastSeenMs() == 0 {
		event.LastSeenMs = uint64(capturedAt.UnixMilli())
	}
	if event.GetFirstSeenMs() != 0 {
		return
	}

	startedAt := capturedAt
	if event.GetType() == "syscall" && raw.DurationNs > 0 {
		duration := time.Duration(raw.DurationNs)
		if duration > 0 && duration <= maxKernelAuditDuration {
			startedAt = capturedAt.Add(-duration)
		}
	}
	event.FirstSeenMs = uint64(startedAt.UnixMilli())
}

func applyKernelRiskDecision(raw *core.BpfEvent, event *pb.Event) {
	annotateKernelAuditTiming(raw, event, time.Now().UTC())
	events.ApplyKernelRiskDecision(raw, event)
}

func startKernelRiskFeedbackWorker(ctx context.Context) {
	events.StartKernelRiskFeedbackWorker(ctx)
}

func shutdownKernelRiskFeedbackWorker(ctx context.Context) error {
	return events.ShutdownKernelRiskFeedbackWorker(ctx)
}

func kernelRiskFeedbackActions(settings RuntimeSettings, event *pb.Event, decision kernelRiskDecision) []kernelRiskFeedbackAction {
	return events.KernelRiskFeedbackActions(settings, event, decision)
}
