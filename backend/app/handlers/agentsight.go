package handlers

import (
	"sync"

	"agent-ebpf-filter/internal/boundedring"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/tls"
)

// ---- moved from app/handlers_agentsight.go ----

// ── Types ───────────────────────────────────────────────────────────

type AgentSightExportEvent struct {
	ID        string         `json:"id,omitempty"`
	Timestamp int64          `json:"timestamp"`
	Source    string         `json:"source"`
	PID       uint32         `json:"pid"`
	PPID      uint32         `json:"ppid,omitempty"`
	Comm      string         `json:"comm"`
	TraceID   string         `json:"trace_id,omitempty"`
	SpanID    string         `json:"span_id,omitempty"`
	Data      map[string]any `json:"data"`
}

type AgentSightEventQuery struct {
	Limit      int
	Filters    any // recentEventFilters from ws_api.go
	Search     string
	IncludeTLS bool
	Sources    []string
	EventTypes []string
	PIDs       []uint32
	RunnerID   string
}

type AgentSightRunnerStatus struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Type           string         `json:"type"`
	Enabled        bool           `json:"enabled"`
	Running        bool           `json:"running"`
	State          string         `json:"state"`
	EventCount     int            `json:"event_count"`
	LastEventTs    int64          `json:"last_event_timestamp,omitempty"`
	LastEventISO   string         `json:"last_event_time,omitempty"`
	SupportedStart bool           `json:"supported_start"`
	SupportedStop  bool           `json:"supported_stop"`
	Details        map[string]any `json:"details,omitempty"`
}

type AgentSightEventsStats struct {
	Total              int            `json:"total"`
	Limit              int            `json:"limit"`
	Runner             string         `json:"runner,omitempty"`
	BySource           map[string]int `json:"by_source"`
	ByType             map[string]int `json:"by_type"`
	ByRunner           map[string]int `json:"by_runner"`
	ByComm             map[string]int `json:"by_comm"`
	EarliestTimestamp  int64          `json:"earliest_timestamp,omitempty"`
	LatestTimestamp    int64          `json:"latest_timestamp,omitempty"`
	EarliestTime       string         `json:"earliest_time,omitempty"`
	LatestTime         string         `json:"latest_time,omitempty"`
	GeneratedTimestamp int64          `json:"generated_timestamp"`
	GeneratedTime      string         `json:"generated_time"`
}

var errAgentSightUploadEventLimit = errors.New("AgentSight upload event limit exceeded")

type boundedAgentSightIDSet struct {
	capacity int
	entries  map[string]struct{}
	order    []string
	next     int
}

func newBoundedAgentSightIDSet(capacity int) *boundedAgentSightIDSet {
	if capacity < 1 {
		capacity = 1
	}
	return &boundedAgentSightIDSet{
		capacity: capacity,
		entries:  make(map[string]struct{}, capacity),
		order:    make([]string, 0, capacity),
	}
}

func (set *boundedAgentSightIDSet) Add(id string) bool {
	if _, exists := set.entries[id]; exists {
		return false
	}
	if len(set.order) < set.capacity {
		set.order = append(set.order, id)
		set.entries[id] = struct{}{}
		return true
	}

	evicted := set.order[set.next]
	delete(set.entries, evicted)
	set.order[set.next] = id
	set.entries[id] = struct{}{}
	set.next = (set.next + 1) % set.capacity
	return true
}

func (set *boundedAgentSightIDSet) Len() int {
	return len(set.entries)
}

func (set *boundedAgentSightIDSet) Contains(id string) bool {
	_, ok := set.entries[id]
	return ok
}

func agentSightStreamDedupeCapacity(limit int) int {
	if limit <= 0 {
		limit = agentSightDefaultLimit
	}
	capacity := limit * 2
	maxCapacity := agentSightMaxLimit * 2
	if capacity > maxCapacity {
		return maxCapacity
	}
	return capacity
}

// ── Handler functions ───────────────────────────────────────────────

func HandleAgentSightEvents(tlsStore *tls.TLSCaptureStore, forceJSONL bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		events, source, err := collectAgentSightEvents(c, tlsStore)
		if err != nil {
			if agentSightRequestCanceled(c, err) {
				return
			}
			c.JSON(agentSightErrorStatus(err), gin.H{"error": err.Error()})
			return
		}
		writeAgentSightEvents(c, events, source, forceJSONL)
	}
}

func HandleAgentSightEventsQuery(tlsStore *tls.TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		query, err := agentSightQueryFromJSONRequest(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		events, source, err := collectAgentSightEventsForQuery(c.Request.Context(), query, tlsStore)
		if err != nil {
			if agentSightRequestCanceled(c, err) {
				return
			}
			c.JSON(agentSightErrorStatus(err), gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"source": source,
			"events": events,
			"stats":  buildAgentSightEventsStats(events, query.Limit, query.RunnerID),
		})
	}
}

func HandleAgentSightEventsStream(tlsStore *tls.TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		query := agentSightQueryFromRequest(c)
		seen := newBoundedAgentSightIDSet(agentSightStreamDedupeCapacity(query.Limit))
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()

		emitSnapshot := func() bool {
			events, _, err := collectAgentSightEventsForQuery(c.Request.Context(), query, tlsStore)
			if err != nil {
				if agentSightRequestCanceled(c, err) {
					return false
				}
				c.SSEvent("error", gin.H{"error": err.Error()})
				return true
			}
			for index, event := range events {
				if index%128 == 0 && c.Request.Context().Err() != nil {
					return false
				}
				key := event.ID
				if key == "" {
					key = agentSightStableID("event", event.Timestamp, event.Source, event.PID, event.Comm, event.Data)
				}
				if !seen.Add(key) {
					continue
				}
				c.SSEvent("event", event)
			}
			return true
		}

		c.Stream(func(_ io.Writer) bool {
			if !emitSnapshot() {
				return false
			}
			select {
			case <-c.Request.Context().Done():
				return false
			case <-ticker.C:
				return true
			}
		})
	}
}

func HandleAgentSightRunnerStream(tlsStore *tls.TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.URL.RawQuery = appendQueryValue(c.Request.URL.RawQuery, "runner", c.Param("id"))
		HandleAgentSightEventsStream(tlsStore)(c)
	}
}

func HandleAgentSightEventsUpload(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, AgentSightUploadMaxBytes))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":     "AgentSight upload is too large",
				"byteLimit": AgentSightUploadMaxBytes,
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	events, err := parseAgentSightUploadPayload(body)
	if err != nil {
		if errors.Is(err, errAgentSightUploadEventLimit) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error":       errAgentSightUploadEventLimit.Error(),
				"recordLimit": AgentSightUploadMaxEvents,
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	batch := make([]any, len(events))
	for index := range events {
		batch[index] = events[index]
	}
	Deps.AgentSightUploadedEvents.Add(batch...)
	c.JSON(http.StatusOK, gin.H{"imported": len(events)})
}

func HandleAgentSightRunners(tlsStore *tls.TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		events, _, err := collectAgentSightEventsForQuery(c.Request.Context(), AgentSightEventQuery{Limit: agentSightMaxLimit, IncludeTLS: true}, tlsStore)
		if err != nil {
			if agentSightRequestCanceled(c, err) {
				return
			}
			c.JSON(agentSightErrorStatus(err), gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"runners": buildAgentSightRunners(events, tlsStore),
		})
	}
}

func HandleAgentSightEventsStats(tlsStore *tls.TLSCaptureStore, runnerID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		query := agentSightQueryFromRequest(c)
		query.Limit = agentSightMaxLimit
		query.RunnerID = platform.FirstNonEmpty(runnerID, query.RunnerID)
		events, _, err := collectAgentSightEventsForQuery(c.Request.Context(), query, tlsStore)
		if err != nil {
			if agentSightRequestCanceled(c, err) {
				return
			}
			c.JSON(agentSightErrorStatus(err), gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildAgentSightEventsStats(events, query.Limit, query.RunnerID))
	}
}

func agentSightRequestCanceled(c *gin.Context, err error) bool {
	return c.Request.Context().Err() != nil || errors.Is(err, context.Canceled)
}

func agentSightErrorStatus(err error) int {
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

func HandleAgentSightRunnerStats(tlsStore *tls.TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		runnerID := strings.TrimSpace(c.Param("id"))
		HandleAgentSightEventsStats(tlsStore, runnerID)(c)
	}
}

// ── Data collection ─────────────────────────────────────────────────

// ── Uploaded-event store (moved from app bridge) ────────────────────

type AgentSightEventStore struct {
	mu     sync.RWMutex
	events *boundedring.Ring[AgentSightExportEvent]
	max    int
}

// Cap reports the backing ring capacity.
func (s *AgentSightEventStore) Cap() int {
	if s == nil {
		return 0
	}
	return s.events.Cap()
}

func NewAgentSightEventStore(max int) *AgentSightEventStore {
	if max <= 0 {
		max = 1000
	}
	return &AgentSightEventStore{
		events: boundedring.New[AgentSightExportEvent](max),
		max:    max,
	}
}

func (s *AgentSightEventStore) Add(events ...AgentSightExportEvent) {
	if s == nil || len(events) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.AddBatch(events)
}

func (s *AgentSightEventStore) Recent(limit int) []AgentSightExportEvent {
	if s == nil {
		return nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.events.Recent(limit)
}

func (s *AgentSightEventStore) Clear() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events.Clear()
}
