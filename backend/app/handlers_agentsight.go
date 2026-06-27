package app

import (
	"agent-ebpf-filter/app/handlers"
	"sync"

	"github.com/gin-gonic/gin"
)

// ---- AgentSight route registration and event store (kept in app/) ----
// All handler functions moved to app/handlers/agentsight.go.
// Bridge functions in handlersbridge.go delegate to them.

const (
	agentSightDefaultLimit = 500
	agentSightMaxLimit     = 5000
)

// Compatibility aliases kept in app/ for the store, adapters, and legacy tests.
type agentSightExportEvent = handlers.AgentSightExportEvent
type agentSightRunnerStatus = handlers.AgentSightRunnerStatus
type agentSightEventsStats = handlers.AgentSightEventsStats

var agentSightUploadedEvents = newAgentSightEventStore(2000)

// ── Event store ──────────────────────────────────────────────────────

type agentSightEventStore struct {
	mu     sync.RWMutex
	events []agentSightExportEvent
	max    int
}

func newAgentSightEventStore(max int) *agentSightEventStore {
	if max <= 0 {
		max = 1000
	}
	return &agentSightEventStore{max: max}
}

func (s *agentSightEventStore) Add(events ...agentSightExportEvent) {
	if s == nil || len(events) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, events...)
	if len(s.events) > s.max {
		copy(s.events, s.events[len(s.events)-s.max:])
		s.events = s.events[:s.max]
	}
}

func (s *agentSightEventStore) Recent(limit int) []agentSightExportEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || limit > len(s.events) {
		limit = len(s.events)
	}
	if limit == 0 {
		return nil
	}
	out := make([]agentSightExportEvent, limit)
	copy(out, s.events[len(s.events)-limit:])
	return out
}

func (s *agentSightEventStore) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = nil
}

// ── Route registration ──────────────────────────────────────────────

func registerAgentSightRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	syncHandlerDeps()
	router.GET("/agentsight/runners", handleAgentSightRunners(tlsStore))
	router.GET("/agentsight/events", handleAgentSightEvents(tlsStore, false))
	router.POST("/agentsight/events", handleAgentSightEventsUpload)
	router.GET("/agentsight/events.jsonl", handleAgentSightEvents(tlsStore, true))
	router.GET("/agentsight/events/stats", handleAgentSightEventsStats(tlsStore, ""))
	router.GET("/agentsight/events/runners/:id/stats", handleAgentSightRunnerStats(tlsStore))
	router.POST("/agentsight/events/query", handleAgentSightEventsQuery(tlsStore))
	router.GET("/agentsight/events/stream", handleAgentSightEventsStream(tlsStore))
	router.GET("/agentsight/stream/merged", handleAgentSightEventsStream(tlsStore))
	router.GET("/agentsight/stream/runner/:id", handleAgentSightRunnerStream(tlsStore))
}

func registerAgentSightCompatibilityRoutes(router gin.IRouter, tlsStore *TLSCaptureStore) {
	syncHandlerDeps()
	router.GET("/runners", handleAgentSightRunners(tlsStore))
	router.GET("/events", handleAgentSightEvents(tlsStore, true))
	router.POST("/events", handleAgentSightEventsUpload)
	router.GET("/events/stats", handleAgentSightEventsStats(tlsStore, ""))
	router.GET("/events/runners/:id/stats", handleAgentSightRunnerStats(tlsStore))
	router.POST("/events/query", handleAgentSightEventsQuery(tlsStore))
	router.GET("/events/stream", handleAgentSightEventsStream(tlsStore))
	router.GET("/stream/merged", handleAgentSightEventsStream(tlsStore))
	router.GET("/stream/runner/:id", handleAgentSightRunnerStream(tlsStore))
}
