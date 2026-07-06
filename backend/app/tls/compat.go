package tls

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Compatibility shims for legacy app-package tests that now live under
// app/tls after the TLS capture refactor.
type processContext = ProcessContext

type tlsProcessContextStore struct {
	mu    sync.RWMutex
	items map[uint32]ProcessContext
}

func newTLSProcessContextStore() *tlsProcessContextStore {
	return &tlsProcessContextStore{items: make(map[uint32]ProcessContext)}
}

func (s *tlsProcessContextStore) Set(pid uint32, ctx ProcessContext) {
	if s == nil || pid == 0 {
		return
	}
	s.mu.Lock()
	s.items[pid] = ctx
	s.mu.Unlock()
}

func (s *tlsProcessContextStore) Get(pid uint32) (ProcessContext, bool) {
	if s == nil || pid == 0 {
		return ProcessContext{}, false
	}
	s.mu.RLock()
	ctx, ok := s.items[pid]
	s.mu.RUnlock()
	return ctx, ok
}

func (s *tlsProcessContextStore) Delete(pid uint32) {
	if s == nil || pid == 0 {
		return
	}
	s.mu.Lock()
	delete(s.items, pid)
	s.mu.Unlock()
}

var trackedProcessContexts = newTLSProcessContextStore()

type noopTLSCollectorMetrics struct{}

func (noopTLSCollectorMetrics) RecordAgentSightCounter(string)      {}
func (noopTLSCollectorMetrics) RecordBroadcastEnqueue(bool, string) {}

func init() {
	deps.TrackedProcessContexts = trackedProcessContexts
	deps.CollectorMetrics = noopTLSCollectorMetrics{}
	deps.Upgrader = &websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
}

func registerTLSCaptureRoutes(router gin.IRouter, runtime tlsCaptureRuntime, store *TLSCaptureStore, rules *TLSCaptureRuleStore) {
	RegisterTLSCaptureRoutes(router, runtime, store, rules)
}
