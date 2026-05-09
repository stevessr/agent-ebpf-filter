package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

type bpfCollectorStats struct {
	RingbufEventsTotal        uint64
	RingbufReserveFailedTotal uint64
}

type collectorPIDKey struct {
	PID  uint32
	Comm string
}

type collectorMetricsSnapshot struct {
	EventsByTypeTotal      map[string]uint64
	EventsByPIDTotal       map[collectorPIDKey]uint64
	PersistAppendLatencyNs uint64
}

type CollectorHealthResponse struct {
	CollectorMapAvailable     bool              `json:"collectorMapAvailable"`
	RingbufEventsTotal        uint64            `json:"ringbufEventsTotal"`
	RingbufDroppedTotal       uint64            `json:"ringbufDroppedTotal"`
	RingbufReserveFailedTotal uint64            `json:"ringbufReserveFailedTotal"`
	EventsByTypeTotal         map[string]uint64 `json:"eventsByTypeTotal"`
	EventsByPidTotal          map[string]uint64 `json:"eventsByPidTotal,omitempty"`
	BackendQueueLen           int               `json:"backendQueueLen"`
	WsClients                 int               `json:"wsClients"`
	PersistAppendLatencyNs    uint64            `json:"persistAppendLatencyNs"`
	CaptureHealthy            bool              `json:"captureHealthy"`
}

type collectorMetricsState struct {
	mu                     sync.RWMutex
	eventsByTypeTotal      map[string]uint64
	eventsByPIDTotal       map[collectorPIDKey]uint64
	persistAppendLatencyNs uint64
}

const maxCollectorPIDSeries = 512

func newCollectorMetricsState() *collectorMetricsState {
	return &collectorMetricsState{
		eventsByTypeTotal: make(map[string]uint64),
		eventsByPIDTotal:  make(map[collectorPIDKey]uint64),
	}
}

var collectorMetricsStore = newCollectorMetricsState()

func (s *collectorMetricsState) RecordEvent(event *pb.Event) {
	if event == nil {
		return
	}
	typeKey := event.GetType()
	if typeKey == "" {
		typeKey = "unknown"
	}
	pidKey := collectorPIDKey{PID: event.GetPid(), Comm: stringsTrimDefault(event.GetComm(), "unknown")}

	s.mu.Lock()
	s.eventsByTypeTotal[typeKey]++
	if pidKey.PID != 0 {
		if _, ok := s.eventsByPIDTotal[pidKey]; ok || len(s.eventsByPIDTotal) < maxCollectorPIDSeries {
			s.eventsByPIDTotal[pidKey]++
		}
	}
	s.mu.Unlock()
}

func stringsTrimDefault(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func (s *collectorMetricsState) SetPersistAppendLatency(duration time.Duration) {
	s.mu.Lock()
	s.persistAppendLatencyNs = uint64(duration.Nanoseconds())
	s.mu.Unlock()
}

func (s *collectorMetricsState) rawSnapshot() collectorMetricsSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	eventsByType := make(map[string]uint64, len(s.eventsByTypeTotal))
	for key, value := range s.eventsByTypeTotal {
		eventsByType[key] = value
	}
	eventsByPID := make(map[collectorPIDKey]uint64, len(s.eventsByPIDTotal))
	for key, value := range s.eventsByPIDTotal {
		eventsByPID[key] = value
	}
	return collectorMetricsSnapshot{
		EventsByTypeTotal:      eventsByType,
		EventsByPIDTotal:       eventsByPID,
		PersistAppendLatencyNs: s.persistAppendLatencyNs,
	}
}

func (s *collectorMetricsState) Snapshot() CollectorHealthResponse {
	bpfStats, mapAvailable := loadCollectorStatsSnapshot()
	raw := s.rawSnapshot()

	eventsByType := make(map[string]uint64, len(raw.EventsByTypeTotal))
	typeKeys := make([]string, 0, len(raw.EventsByTypeTotal))
	for key := range raw.EventsByTypeTotal {
		typeKeys = append(typeKeys, key)
	}
	sort.Strings(typeKeys)
	for _, key := range typeKeys {
		eventsByType[key] = raw.EventsByTypeTotal[key]
	}
	eventsByPID := make(map[string]uint64, len(raw.EventsByPIDTotal))
	pidKeys := make([]collectorPIDKey, 0, len(raw.EventsByPIDTotal))
	for key := range raw.EventsByPIDTotal {
		pidKeys = append(pidKeys, key)
	}
	sort.Slice(pidKeys, func(i, j int) bool {
		if pidKeys[i].PID == pidKeys[j].PID {
			return pidKeys[i].Comm < pidKeys[j].Comm
		}
		return pidKeys[i].PID < pidKeys[j].PID
	})
	for _, key := range pidKeys {
		eventsByPID[fmt.Sprintf("%d:%s", key.PID, key.Comm)] = raw.EventsByPIDTotal[key]
	}

	clientsMu.Lock()
	legacyWSClients := len(clients)
	clientsMu.Unlock()
	envelopeClientsMu.Lock()
	envelopeWSClients := len(envelopeClients)
	envelopeClientsMu.Unlock()

	return CollectorHealthResponse{
		CollectorMapAvailable:     mapAvailable,
		RingbufEventsTotal:        bpfStats.RingbufEventsTotal,
		RingbufDroppedTotal:       bpfStats.RingbufReserveFailedTotal,
		RingbufReserveFailedTotal: bpfStats.RingbufReserveFailedTotal,
		EventsByTypeTotal:         eventsByType,
		EventsByPidTotal:          eventsByPID,
		BackendQueueLen:           len(broadcast),
		WsClients:                 legacyWSClients + envelopeWSClients,
		PersistAppendLatencyNs:    raw.PersistAppendLatencyNs,
		CaptureHealthy:            !mapAvailable || bpfStats.RingbufReserveFailedTotal == 0,
	}
}

func loadCollectorStatsSnapshot() (bpfCollectorStats, bool) {
	if trackerMaps.CollectorStats == nil {
		return bpfCollectorStats{}, false
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return bpfCollectorStats{}, false
	}

	values := make([]bpfCollectorStats, cpuCount)
	key := uint32(0)
	if err := trackerMaps.CollectorStats.Lookup(&key, &values); err != nil {
		return bpfCollectorStats{}, false
	}

	var total bpfCollectorStats
	for _, value := range values {
		total.RingbufEventsTotal += value.RingbufEventsTotal
		total.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal
	}
	return total, true
}

func handleCollectorHealth(c *gin.Context) {
	c.JSON(http.StatusOK, collectorMetricsStore.Snapshot())
}
