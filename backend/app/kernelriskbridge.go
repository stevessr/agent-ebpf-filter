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

type kernelRiskDecision = events.KernelRiskDecision
type kernelRiskFeedbackAction = events.KernelRiskFeedbackAction

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

// annotateKernelAuditTiming records the userspace observation window for an
// eBPF ring-buffer event using fields that are already part of the event schema.
//
// This deliberately does NOT claim LastSeenMs is a raw kernel timestamp. It is
// the time the decoded sample reached the backend. For syscall records whose
// eBPF enter/exit correlation produced DurationNs, FirstSeenMs is a best-effort
// start estimate. Flow-level network events may already carry authoritative
// first/last timestamps from the flow aggregator; those are never overwritten.
func annotateKernelAuditTiming(raw *core.BpfEvent, event *pb.Event, observedAt time.Time) {
	if raw == nil || event == nil {
		return
	}
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	} else {
		observedAt = observedAt.UTC()
	}
	observedMS := uint64(observedAt.UnixMilli())
	if event.GetLastSeenMs() == 0 {
		event.LastSeenMs = observedMS
	}
	if event.GetFirstSeenMs() != 0 {
		return
	}

	startedAt := observedAt
	// TYPE_TCP_STATE_CHANGE historically stores old/new TCP state in the raw
	// DurationNs slot. Restrict duration-based estimation to actual syscall-like
	// records so that packed metadata can never become a bogus multi-year span.
	if event.GetType() != "tcp_state_change" && raw.DurationNs > 0 {
		duration := time.Duration(raw.DurationNs)
		if duration > 0 && duration <= maxKernelAuditDuration {
			startedAt = observedAt.Add(-duration)
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
