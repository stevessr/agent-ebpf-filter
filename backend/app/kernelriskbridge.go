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

func applyKernelRiskDecision(raw *core.BpfEvent, event *pb.Event) {
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
