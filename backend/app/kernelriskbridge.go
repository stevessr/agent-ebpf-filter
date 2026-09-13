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
// the time the decoded sample reached the backend. For generic syscall records
// the tracker records enter/exit DurationNs, allowing FirstSeenMs to be a
// best-effort start estimate. Flow-level network events may already carry
// authoritative first/last timestamps; those are never overwritten.
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
	// Only TYPE_GENERIC_SYSCALL has duration_ns measured from bpf_ktime_get_ns
	// enter→exit correlation today. Other event types are intentionally ignored:
	// notably tcp_state_change historically packs old/new TCP states into the
	// same raw DurationNs slot.
	if event.GetType() == "syscall" && raw.DurationNs > 0 {
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
