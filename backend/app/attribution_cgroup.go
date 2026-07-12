package app

import (
	"agent-ebpf-filter/app/events"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

// ---- moved from backend/zz_merged_backend.go section attribution_cgroup.go ----

// cgroupAttribution maps cgroup IDs to agent execution context.
// This enables attribution of all child processes and flows within a cgroup
// to the originating agent run and tool call.
type cgroupAttributionEntry struct {
	CgroupID     uint64
	AgentRunID   string
	TaskID       string
	ToolCallID   string
	RootAgentPID uint32
	CreatedAt    time.Time
}

type cgroupAttributionStore struct {
	mu    sync.RWMutex
	items map[uint64]cgroupAttributionEntry
}

func newCgroupAttributionStore() *cgroupAttributionStore {
	return &cgroupAttributionStore{
		items: make(map[uint64]cgroupAttributionEntry),
	}
}

func (s *cgroupAttributionStore) Set(cgroupID uint64, entry cgroupAttributionEntry) {
	if s == nil || cgroupID == 0 {
		return
	}
	entry.CgroupID = cgroupID
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	s.items[cgroupID] = entry
	s.mu.Unlock()
}

func (s *cgroupAttributionStore) Get(cgroupID uint64) (cgroupAttributionEntry, bool) {
	if s == nil || cgroupID == 0 {
		return cgroupAttributionEntry{}, false
	}
	s.mu.RLock()
	entry, ok := s.items[cgroupID]
	s.mu.RUnlock()
	return entry, ok
}

func (s *cgroupAttributionStore) EvictOlderThan(maxAge time.Duration) {
	if s == nil {
		return
	}
	cutoff := time.Now().UTC().Add(-maxAge)
	s.mu.Lock()
	for id, entry := range s.items {
		if entry.CreatedAt.Before(cutoff) {
			delete(s.items, id)
		}
	}
	s.mu.Unlock()
}

var cgroupAttribution = newCgroupAttributionStore()

// attachCgroupToAgent associates a cgroup ID with the agent context from a register payload.
func attachCgroupToAgent(cgroupID uint64, payload events.RegisterPayload) {
	if cgroupID == 0 {
		return
	}
	cgroupAttribution.Set(cgroupID, cgroupAttributionEntry{
		AgentRunID:   payload.AgentRunID,
		TaskID:       payload.TaskID,
		ToolCallID:   payload.ToolCallID,
		RootAgentPID: payload.PID,
	})
}

// enrichEventWithCgroupContext adds agent context from cgroup attribution to an event.
func enrichEventWithCgroupContext(cgroupID uint64) (agentRunID, taskID, toolCallID string) {
	entry, ok := cgroupAttribution.Get(cgroupID)
	if !ok {
		return "", "", ""
	}
	return entry.AgentRunID, entry.TaskID, entry.ToolCallID
}

// ── Per-tool baseline and drift detection ────────────────────────────

type toolBaselineSample struct {
	ToolName  string
	Comm      string
	EventType string
	Path      string
	Count     int
	LastSeen  time.Time
}

type toolBaselineStore struct {
	mu           sync.RWMutex
	samples      map[string]map[string]*toolBaselineSample // toolName -> (comm+eventType)
	lastEviction time.Time
}

const (
	toolBaselineMaxTools          = 512
	toolBaselineMaxSamplesPerTool = 128
	toolBaselineTTL               = 24 * time.Hour
	toolBaselineEvictionInterval  = time.Minute
	toolBaselineMaxNameRunes      = 128
	toolBaselineMaxEventRunes     = 64
	toolBaselineMaxPathRunes      = 512
)

func newToolBaselineStore() *toolBaselineStore {
	return &toolBaselineStore{
		samples: make(map[string]map[string]*toolBaselineSample),
	}
}

func (s *toolBaselineStore) Record(toolName, comm, eventType, path string) {
	if s == nil {
		return
	}
	toolName = truncateBaselineValue(toolName, toolBaselineMaxNameRunes)
	comm = truncateBaselineValue(comm, toolBaselineMaxNameRunes)
	eventType = truncateBaselineValue(eventType, toolBaselineMaxEventRunes)
	path = truncateBaselineValue(path, toolBaselineMaxPathRunes)
	if toolName == "" || comm == "" || eventType == "" {
		return
	}
	now := time.Now().UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastEviction.IsZero() || now.Sub(s.lastEviction) >= toolBaselineEvictionInterval {
		s.evictExpiredLocked(now)
		s.lastEviction = now
	}

	toolSamples, ok := s.samples[toolName]
	if !ok {
		if len(s.samples) >= toolBaselineMaxTools {
			s.evictOldestToolLocked()
		}
		toolSamples = make(map[string]*toolBaselineSample)
		s.samples[toolName] = toolSamples
	}

	key := comm + ":" + eventType
	sample, ok := toolSamples[key]
	if !ok {
		if len(toolSamples) >= toolBaselineMaxSamplesPerTool {
			evictOldestToolSampleLocked(toolSamples)
		}
		sample = &toolBaselineSample{
			ToolName:  toolName,
			Comm:      comm,
			EventType: eventType,
			Path:      path,
			Count:     1,
			LastSeen:  now,
		}
		toolSamples[key] = sample
	} else {
		sample.Count++
		sample.LastSeen = now
	}
}

func truncateBaselineValue(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxRunes {
		value = string(runes[:maxRunes])
	}
	return value
}

func (s *toolBaselineStore) evictExpiredLocked(now time.Time) {
	for toolName := range s.samples {
		s.evictExpiredToolLocked(toolName, now)
	}
}

func (s *toolBaselineStore) evictExpiredToolLocked(toolName string, now time.Time) {
	toolSamples, ok := s.samples[toolName]
	if !ok {
		return
	}
	cutoff := now.Add(-toolBaselineTTL)
	for key, sample := range toolSamples {
		if sample == nil || sample.LastSeen.Before(cutoff) {
			delete(toolSamples, key)
		}
	}
	if len(toolSamples) == 0 {
		delete(s.samples, toolName)
	}
}

func (s *toolBaselineStore) evictOldestToolLocked() {
	var oldestTool string
	var oldestSeen time.Time
	for toolName, toolSamples := range s.samples {
		latestSeen := time.Time{}
		for _, sample := range toolSamples {
			if sample != nil && sample.LastSeen.After(latestSeen) {
				latestSeen = sample.LastSeen
			}
		}
		if oldestTool == "" || latestSeen.Before(oldestSeen) {
			oldestTool = toolName
			oldestSeen = latestSeen
		}
	}
	if oldestTool != "" {
		delete(s.samples, oldestTool)
	}
}

func evictOldestToolSampleLocked(samples map[string]*toolBaselineSample) {
	var oldestKey string
	var oldestSeen time.Time
	for key, sample := range samples {
		if sample == nil || oldestKey == "" || sample.LastSeen.Before(oldestSeen) {
			oldestKey = key
			if sample != nil {
				oldestSeen = sample.LastSeen
			}
		}
	}
	if oldestKey != "" {
		delete(samples, oldestKey)
	}
}

// detectDrift checks if a current behavior deviates from the tool's baseline.
// Returns a drift reason if the behavior is anomalous for this tool.
func (s *toolBaselineStore) detectDrift(toolName, comm, eventType string) (string, bool) {
	if s == nil {
		return "", false
	}
	toolName = truncateBaselineValue(toolName, toolBaselineMaxNameRunes)
	comm = truncateBaselineValue(comm, toolBaselineMaxNameRunes)
	eventType = truncateBaselineValue(eventType, toolBaselineMaxEventRunes)
	if toolName == "" || comm == "" || eventType == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evictExpiredToolLocked(toolName, time.Now().UTC())

	toolSamples, ok := s.samples[toolName]
	if !ok {
		// No baseline yet - this is a new tool, not drift
		return "", false
	}

	key := comm + ":" + eventType
	_, ok = toolSamples[key]
	if ok {
		// This behavior is in the baseline - no drift
		return "", false
	}

	// This behavior is NOT in the baseline for this tool
	// Check if we have enough samples to have a meaningful baseline
	if len(toolSamples) < 3 {
		return "", false
	}

	return fmt.Sprintf("tool %q baseline drift: unexpected behavior %s/%s",
		toolName, comm, eventType), true
}

var toolBaseline = newToolBaselineStore()

// ── Sampling and rate limiting hints ─────────────────────────────────

type collectorRateLimitHint struct {
	PID       uint32
	Comm      string
	EventType string
	RatePerS  float64
	Suggested string // "sample", "throttle", "drop"
}

type collectorRateLimitState struct {
	mu        sync.RWMutex
	lastCheck time.Time
	hints     []collectorRateLimitHint
}

var collectorRateLimits = &collectorRateLimitState{}

func (s *collectorRateLimitState) computeHints(metrics collectorMetricsSnapshot) []collectorRateLimitHint {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if now.Sub(s.lastCheck) < 5*time.Second {
		return s.hints
	}
	s.lastCheck = now

	hints := make([]collectorRateLimitHint, 0)

	// Find high-volume PIDs (>1000 events/sec equivalent)
	totalEvents := uint64(0)
	for _, count := range metrics.EventsByTypeTotal {
		totalEvents += count
	}

	for pidKey, count := range metrics.EventsByPIDTotal {
		// Simple threshold: if a single PID contributes >30% of total events
		if totalEvents > 0 && float64(count)/float64(totalEvents) > 0.30 {
			hints = append(hints, collectorRateLimitHint{
				PID:       pidKey.PID,
				Comm:      pidKey.Comm,
				RatePerS:  float64(count) / 5.0, // rough estimate
				Suggested: "throttle",
			})
		}
	}

	s.hints = hints
	return hints
}

func runCgroupAttributionGC(ctx context.Context, store *cgroupAttributionStore, interval, maxAge time.Duration) {
	if ctx == nil || store == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			store.EvictOlderThan(maxAge)
		}
	}
}
