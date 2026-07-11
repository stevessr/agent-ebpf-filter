package handlers

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/app/tls"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
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
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		events, source, err := collectAgentSightEventsForQuery(query, tlsStore)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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

		emitSnapshot := func() {
			events, _, err := collectAgentSightEventsForQuery(query, tlsStore)
			if err != nil {
				c.SSEvent("error", gin.H{"error": err.Error()})
				return
			}
			for _, event := range events {
				key := event.ID
				if key == "" {
					key = agentSightStableID("event", event.Timestamp, event.Source, event.PID, event.Comm, event.Data)
				}
				if !seen.Add(key) {
					continue
				}
				c.SSEvent("event", event)
			}
		}

		c.Stream(func(_ io.Writer) bool {
			emitSnapshot()
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
		events, _, err := collectAgentSightEventsForQuery(AgentSightEventQuery{Limit: agentSightMaxLimit, IncludeTLS: true}, tlsStore)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
		events, _, err := collectAgentSightEventsForQuery(query, tlsStore)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, buildAgentSightEventsStats(events, query.Limit, query.RunnerID))
	}
}

func HandleAgentSightRunnerStats(tlsStore *tls.TLSCaptureStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		runnerID := strings.TrimSpace(c.Param("id"))
		HandleAgentSightEventsStats(tlsStore, runnerID)(c)
	}
}

// ── Data collection ─────────────────────────────────────────────────
