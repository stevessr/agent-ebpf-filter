package app

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"strings"
	"sync"
	"time"
)

const (
	toolBaselineMaxTools          = 512
	toolBaselineMaxSamplesPerTool = 128
	toolBaselineMinObservations   = 16
	toolBaselineTTL               = 24 * time.Hour
	toolBaselineEvictionInterval  = time.Minute
	toolBaselineMaxNameRunes      = 128
	toolBaselineMaxEventRunes     = 64
	toolBaselineDigestHexRunes    = 16
	toolBaselineDigestSuffixRunes = 1 + toolBaselineDigestHexRunes
)

type toolBaselineBehaviorKey struct {
	Comm      string
	EventType string
}

type toolBaselineSample struct {
	Count          uint64
	LastSeen       time.Time
	recencyElement *list.Element
}

type toolBaselineTool struct {
	samples          map[toolBaselineBehaviorKey]*toolBaselineSample
	recency          list.List
	recencyElement   *list.Element
	observationCount uint64
}

type toolBaselineStatus struct {
	Tools                     int
	Samples                   int
	MaxTools                  int
	MaxSamples                int
	MaxSamplesPerTool         int
	ObservationsTotal         uint64
	DriftsTotal               uint64
	ExpiredEvictionsTotal     uint64
	CapacityEvictionsTotal    uint64
	TruncatedStateValuesTotal uint64
	LastSweepAt               time.Time
}

type toolBaselineStore struct {
	mu                        sync.Mutex
	tools                     map[string]*toolBaselineTool
	recency                   list.List
	sampleCount               int
	observationsTotal         uint64
	driftsTotal               uint64
	expiredEvictionsTotal     uint64
	capacityEvictionsTotal    uint64
	truncatedStateValuesTotal uint64
	lastObservedAt            time.Time
	lastSweepAt               time.Time
}

func newToolBaselineStore() *toolBaselineStore {
	return &toolBaselineStore{tools: make(map[string]*toolBaselineTool)}
}

// Observe atomically checks the prior baseline and then records the current
// behavior. This ordering lets a new behavior alert once without allowing the
// event being evaluated to make itself part of the baseline first.
func (s *toolBaselineStore) Observe(toolName, comm, eventType string) (string, bool) {
	return s.observeAt(toolName, comm, eventType, time.Now().UTC())
}

func (s *toolBaselineStore) observeAt(toolName, comm, eventType string, now time.Time) (string, bool) {
	if s == nil {
		return "", false
	}
	var toolNameTruncated, commTruncated, eventTypeTruncated bool
	toolName, toolNameTruncated = boundToolBaselineValue(toolName, toolBaselineMaxNameRunes)
	comm, commTruncated = boundToolBaselineValue(comm, toolBaselineMaxNameRunes)
	eventType, eventTypeTruncated = boundToolBaselineValue(eventType, toolBaselineMaxEventRunes)
	if toolName == "" || comm == "" || eventType == "" {
		return "", false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	key := toolBaselineBehaviorKey{Comm: comm, EventType: eventType}
	s.mu.Lock()
	s.ensureInitializedLocked()
	if now.Before(s.lastObservedAt) {
		now = s.lastObservedAt
	} else {
		s.lastObservedAt = now
	}
	s.observationsTotal++
	s.noteTruncationsLocked(toolNameTruncated, commTruncated, eventTypeTruncated)
	s.evictExpiredToolLocked(toolName, now)

	tool, exists := s.tools[toolName]
	knownBehavior := false
	if exists {
		_, knownBehavior = tool.samples[key]
	}
	drift := exists && !knownBehavior && len(tool.samples) >= 3 && tool.observationCount >= toolBaselineMinObservations

	if !exists {
		if len(s.tools) >= toolBaselineMaxTools {
			s.evictOldestToolLocked()
		}
		tool = &toolBaselineTool{samples: make(map[toolBaselineBehaviorKey]*toolBaselineSample)}
		tool.recencyElement = s.recency.PushFront(toolName)
		s.tools[toolName] = tool
	} else {
		s.recency.MoveToFront(tool.recencyElement)
	}

	if sample, ok := tool.samples[key]; ok {
		sample.Count++
		tool.observationCount++
		sample.LastSeen = now
		tool.recency.MoveToFront(sample.recencyElement)
	} else {
		if len(tool.samples) >= toolBaselineMaxSamplesPerTool {
			s.evictOldestSampleLocked(tool)
		}
		sample := &toolBaselineSample{
			Count:    1,
			LastSeen: now,
		}
		sample.recencyElement = tool.recency.PushFront(key)
		tool.samples[key] = sample
		s.sampleCount++
		tool.observationCount++
	}
	if drift {
		s.driftsTotal++
	}
	s.mu.Unlock()

	if !drift {
		return "", false
	}
	return fmt.Sprintf("tool %q baseline drift: unexpected behavior %s/%s", toolName, comm, eventType), true
}

func (s *toolBaselineStore) EvictExpired(now time.Time) toolBaselineStatus {
	if s == nil {
		return emptyToolBaselineStatus()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInitializedLocked()
	for toolName := range s.tools {
		s.evictExpiredToolLocked(toolName, now)
	}
	s.lastSweepAt = now
	return s.statusLocked()
}

func (s *toolBaselineStore) Status() toolBaselineStatus {
	if s == nil {
		return emptyToolBaselineStatus()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureInitializedLocked()
	return s.statusLocked()
}

func (s *toolBaselineStore) statusLocked() toolBaselineStatus {
	return toolBaselineStatus{
		Tools:                     len(s.tools),
		Samples:                   s.sampleCount,
		MaxTools:                  toolBaselineMaxTools,
		MaxSamples:                toolBaselineMaxTools * toolBaselineMaxSamplesPerTool,
		MaxSamplesPerTool:         toolBaselineMaxSamplesPerTool,
		ObservationsTotal:         s.observationsTotal,
		DriftsTotal:               s.driftsTotal,
		ExpiredEvictionsTotal:     s.expiredEvictionsTotal,
		CapacityEvictionsTotal:    s.capacityEvictionsTotal,
		TruncatedStateValuesTotal: s.truncatedStateValuesTotal,
		LastSweepAt:               s.lastSweepAt,
	}
}

func emptyToolBaselineStatus() toolBaselineStatus {
	return toolBaselineStatus{
		MaxTools:          toolBaselineMaxTools,
		MaxSamples:        toolBaselineMaxTools * toolBaselineMaxSamplesPerTool,
		MaxSamplesPerTool: toolBaselineMaxSamplesPerTool,
	}
}

func (s *toolBaselineStore) ensureInitializedLocked() {
	if s.tools == nil {
		s.tools = make(map[string]*toolBaselineTool)
	}
}

func (s *toolBaselineStore) evictExpiredToolLocked(toolName string, now time.Time) {
	tool, ok := s.tools[toolName]
	if !ok {
		return
	}
	cutoff := now.Add(-toolBaselineTTL)
	for {
		oldest := tool.recency.Back()
		if oldest == nil {
			break
		}
		key, keyOK := oldest.Value.(toolBaselineBehaviorKey)
		if !keyOK {
			tool.recency.Remove(oldest)
			continue
		}
		sample := tool.samples[key]
		if sample != nil && !sample.LastSeen.Before(cutoff) {
			break
		}
		delete(tool.samples, key)
		tool.recency.Remove(oldest)
		s.sampleCount--
		if sample != nil && tool.observationCount >= sample.Count {
			tool.observationCount -= sample.Count
		} else if sample != nil {
			tool.observationCount = 0
		}
		s.expiredEvictionsTotal++
	}
	if len(tool.samples) == 0 {
		s.removeToolLocked(toolName, tool)
	}
}

func (s *toolBaselineStore) evictOldestToolLocked() {
	oldest := s.recency.Back()
	if oldest == nil {
		return
	}
	toolName, _ := oldest.Value.(string)
	tool := s.tools[toolName]
	evictedSamples := s.removeToolLocked(toolName, tool)
	s.capacityEvictionsTotal += uint64(evictedSamples)
}

func (s *toolBaselineStore) evictOldestSampleLocked(tool *toolBaselineTool) {
	if tool == nil {
		return
	}
	oldest := tool.recency.Back()
	if oldest == nil {
		return
	}
	key, ok := oldest.Value.(toolBaselineBehaviorKey)
	if !ok {
		return
	}
	sample := tool.samples[key]
	delete(tool.samples, key)
	tool.recency.Remove(oldest)
	s.sampleCount--
	if sample != nil && tool.observationCount >= sample.Count {
		tool.observationCount -= sample.Count
	} else if sample != nil {
		tool.observationCount = 0
	}
	s.capacityEvictionsTotal++
}

func (s *toolBaselineStore) removeToolLocked(toolName string, tool *toolBaselineTool) int {
	if tool == nil {
		return 0
	}
	evictedSamples := len(tool.samples)
	delete(s.tools, toolName)
	if tool.recencyElement != nil {
		s.recency.Remove(tool.recencyElement)
	}
	s.sampleCount -= evictedSamples
	return evictedSamples
}

func (s *toolBaselineStore) noteTruncationsLocked(values ...bool) {
	for _, truncated := range values {
		if truncated {
			s.truncatedStateValuesTotal++
		}
	}
}

func boundToolBaselineValue(value string, maxRunes int) (string, bool) {
	originalLength := len(value)
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return "", value != ""
	}
	prefixRuneLimit := maxRunes - toolBaselineDigestSuffixRunes
	if prefixRuneLimit < 0 {
		prefixRuneLimit = 0
	}
	prefixEnd := 0
	runeCount := 0
	truncated := false
	for index := range value {
		if runeCount == prefixRuneLimit {
			prefixEnd = index
		}
		if runeCount == maxRunes {
			truncated = true
			break
		}
		runeCount++
	}
	if !truncated {
		if len(value) != originalLength {
			value = strings.Clone(value)
		}
		return value, false
	}

	digest := digestToolBaselineValue(value)
	var encoded [toolBaselineDigestHexRunes]byte
	hex.Encode(encoded[:], digest[:toolBaselineDigestHexRunes/2])
	if maxRunes <= toolBaselineDigestHexRunes {
		return string(encoded[:maxRunes]), true
	}
	return value[:prefixEnd] + "~" + string(encoded[:]), true
}

func digestToolBaselineValue(value string) [sha256.Size]byte {
	digest := sha256.New()
	writeToolBaselineDigestString(digest, value)
	var sum [sha256.Size]byte
	digest.Sum(sum[:0])
	return sum
}

func writeToolBaselineDigestString(digest hash.Hash, value string) {
	var buffer [4096]byte
	for offset := 0; offset < len(value); {
		count := copy(buffer[:], value[offset:])
		_, _ = digest.Write(buffer[:count])
		offset += count
	}
}

var toolBaseline = newToolBaselineStore()
