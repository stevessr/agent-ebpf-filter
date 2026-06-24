package app

import (
	"agent-ebpf-filter/pb"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section metrics_collector.go ----

type bpfCollectorStats struct {
	RingbufEventsTotal        uint64
	RingbufReserveFailedTotal uint64
}

type collectorPIDKey struct {
	PID  uint32
	Comm string
}

type collectorMetricsSnapshot struct {
	EventsByTypeTotal           map[string]uint64
	EventsByPIDTotal            map[collectorPIDKey]uint64
	AgentSightCountersTotal     map[string]uint64
	PersistAppendLatencyNs      uint64
	RingbufZeroCopyDecodeTotal  uint64
	RingbufCopyDecodeTotal      uint64
	KernelRiskEvaluationsTotal  uint64
	KernelRiskAlertsTotal       uint64
	KernelRiskBlocksTotal       uint64
	KernelRiskLastEvalLatencyNs uint64
	KernelRiskFeedbackApplied   uint64
	KernelRiskFeedbackDropped   uint64
	KernelRiskFeedbackLastError string
}

type CollectorHealthResponse struct {
	CollectorMapAvailable       bool              `json:"collectorMapAvailable"`
	RingbufEventsTotal          uint64            `json:"ringbufEventsTotal"`
	RingbufDroppedTotal         uint64            `json:"ringbufDroppedTotal"`
	RingbufReserveFailedTotal   uint64            `json:"ringbufReserveFailedTotal"`
	RingbufZeroCopyDecodeTotal  uint64            `json:"ringbufZeroCopyDecodeTotal"`
	RingbufCopyDecodeTotal      uint64            `json:"ringbufCopyDecodeTotal"`
	EventsByTypeTotal           map[string]uint64 `json:"eventsByTypeTotal"`
	EventsByPidTotal            map[string]uint64 `json:"eventsByPidTotal,omitempty"`
	AgentSightCountersTotal     map[string]uint64 `json:"agentSightCountersTotal,omitempty"`
	BackendQueueLen             int               `json:"backendQueueLen"`
	WsClients                   int               `json:"wsClients"`
	PersistAppendLatencyNs      uint64            `json:"persistAppendLatencyNs"`
	KernelRiskEvaluationsTotal  uint64            `json:"kernelRiskEvaluationsTotal"`
	KernelRiskAlertsTotal       uint64            `json:"kernelRiskAlertsTotal"`
	KernelRiskBlocksTotal       uint64            `json:"kernelRiskBlocksTotal"`
	KernelRiskLastEvalLatencyNs uint64            `json:"kernelRiskLastEvalLatencyNs"`
	KernelRiskFeedbackApplied   uint64            `json:"kernelRiskFeedbackApplied"`
	KernelRiskFeedbackDropped   uint64            `json:"kernelRiskFeedbackDropped"`
	KernelRiskFeedbackLastError string            `json:"kernelRiskFeedbackLastError,omitempty"`
	CaptureHealthy              bool              `json:"captureHealthy"`
}

type collectorMetricsState struct {
	mu                          sync.RWMutex
	eventsByTypeTotal           map[string]uint64
	eventsByPIDTotal            map[collectorPIDKey]uint64
	agentSightCountersTotal     map[string]uint64
	persistAppendLatencyNs      uint64
	ringbufZeroCopyDecodeTotal  uint64
	ringbufCopyDecodeTotal      uint64
	kernelRiskEvaluationsTotal  uint64
	kernelRiskAlertsTotal       uint64
	kernelRiskBlocksTotal       uint64
	kernelRiskLastEvalLatencyNs uint64
	kernelRiskFeedbackApplied   uint64
	kernelRiskFeedbackDropped   uint64
	kernelRiskFeedbackLastError string
}

const maxCollectorPIDSeries = 512

func newCollectorMetricsState() *collectorMetricsState {
	return &collectorMetricsState{
		eventsByTypeTotal:       make(map[string]uint64),
		eventsByPIDTotal:        make(map[collectorPIDKey]uint64),
		agentSightCountersTotal: make(map[string]uint64),
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

func (s *collectorMetricsState) RecordAgentSightCounter(name string) {
	name = stringsTrimDefault(name, "unknown")
	s.mu.Lock()
	s.agentSightCountersTotal[name]++
	s.mu.Unlock()
}

func (s *collectorMetricsState) SetPersistAppendLatency(duration time.Duration) {
	s.mu.Lock()
	s.persistAppendLatencyNs = uint64(duration.Nanoseconds())
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordRingbufDecode(zeroCopy bool) {
	s.mu.Lock()
	if zeroCopy {
		s.ringbufZeroCopyDecodeTotal++
	} else {
		s.ringbufCopyDecodeTotal++
	}
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordKernelRiskDecision(decision string, duration time.Duration) {
	s.mu.Lock()
	s.kernelRiskEvaluationsTotal++
	s.kernelRiskLastEvalLatencyNs = uint64(duration.Nanoseconds())
	switch strings.ToUpper(strings.TrimSpace(decision)) {
	case "ALERT", "OBSERVE":
		s.kernelRiskAlertsTotal++
	case "BLOCK":
		s.kernelRiskBlocksTotal++
	}
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordKernelRiskFeedback(applied bool, err error) {
	s.mu.Lock()
	if applied {
		s.kernelRiskFeedbackApplied++
		s.kernelRiskFeedbackLastError = ""
	} else {
		s.kernelRiskFeedbackDropped++
		if err != nil {
			s.kernelRiskFeedbackLastError = err.Error()
		}
	}
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
	agentSightCounters := make(map[string]uint64, len(s.agentSightCountersTotal))
	for key, value := range s.agentSightCountersTotal {
		agentSightCounters[key] = value
	}
	return collectorMetricsSnapshot{
		EventsByTypeTotal:           eventsByType,
		EventsByPIDTotal:            eventsByPID,
		AgentSightCountersTotal:     agentSightCounters,
		PersistAppendLatencyNs:      s.persistAppendLatencyNs,
		RingbufZeroCopyDecodeTotal:  s.ringbufZeroCopyDecodeTotal,
		RingbufCopyDecodeTotal:      s.ringbufCopyDecodeTotal,
		KernelRiskEvaluationsTotal:  s.kernelRiskEvaluationsTotal,
		KernelRiskAlertsTotal:       s.kernelRiskAlertsTotal,
		KernelRiskBlocksTotal:       s.kernelRiskBlocksTotal,
		KernelRiskLastEvalLatencyNs: s.kernelRiskLastEvalLatencyNs,
		KernelRiskFeedbackApplied:   s.kernelRiskFeedbackApplied,
		KernelRiskFeedbackDropped:   s.kernelRiskFeedbackDropped,
		KernelRiskFeedbackLastError: s.kernelRiskFeedbackLastError,
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

	agentSightCounters := make(map[string]uint64, len(raw.AgentSightCountersTotal))
	agentSightKeys := make([]string, 0, len(raw.AgentSightCountersTotal))
	for key := range raw.AgentSightCountersTotal {
		agentSightKeys = append(agentSightKeys, key)
	}
	sort.Strings(agentSightKeys)
	for _, key := range agentSightKeys {
		agentSightCounters[key] = raw.AgentSightCountersTotal[key]
	}

	return CollectorHealthResponse{
		CollectorMapAvailable:       mapAvailable,
		RingbufEventsTotal:          bpfStats.RingbufEventsTotal,
		RingbufDroppedTotal:         bpfStats.RingbufReserveFailedTotal,
		RingbufReserveFailedTotal:   bpfStats.RingbufReserveFailedTotal,
		RingbufZeroCopyDecodeTotal:  raw.RingbufZeroCopyDecodeTotal,
		RingbufCopyDecodeTotal:      raw.RingbufCopyDecodeTotal,
		EventsByTypeTotal:           eventsByType,
		EventsByPidTotal:            eventsByPID,
		AgentSightCountersTotal:     agentSightCounters,
		BackendQueueLen:             len(broadcast),
		WsClients:                   legacyWSClients + envelopeWSClients,
		PersistAppendLatencyNs:      raw.PersistAppendLatencyNs,
		KernelRiskEvaluationsTotal:  raw.KernelRiskEvaluationsTotal,
		KernelRiskAlertsTotal:       raw.KernelRiskAlertsTotal,
		KernelRiskBlocksTotal:       raw.KernelRiskBlocksTotal,
		KernelRiskLastEvalLatencyNs: raw.KernelRiskLastEvalLatencyNs,
		KernelRiskFeedbackApplied:   raw.KernelRiskFeedbackApplied,
		KernelRiskFeedbackDropped:   raw.KernelRiskFeedbackDropped,
		KernelRiskFeedbackLastError: raw.KernelRiskFeedbackLastError,
		CaptureHealthy:              !mapAvailable || bpfStats.RingbufReserveFailedTotal == 0,
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
