package observability

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

type bpfCollectorStats struct {
	RingbufEventsTotal        uint64
	RingbufReserveFailedTotal uint64
}

type collectorPIDKey struct {
	PID  uint32
	Comm string
}

type CollectorMetricsSnapshot struct {
	EventsByTypeTotal              map[string]uint64
	EventsByPIDTotal               map[collectorPIDKey]uint64
	AgentSightCountersTotal        map[string]uint64
	PersistAppendLatencyNs         uint64
	CapturedArchivedTotal          uint64
	CapturedPersistedTotal         uint64
	CapturedPersistErrorsTotal     uint64
	BroadcastQueuedTotal           uint64
	BroadcastDroppedTotal          uint64
	BroadcastLastDropReason        string
	BroadcastReceivedTotal         uint64
	BroadcastFlushesTotal          uint64
	BroadcastEventsFlushedTotal    uint64
	BroadcastEnvelopesFlushedTotal uint64
	BroadcastMarshalErrorsTotal    uint64
	BroadcastWriteErrorsTotal      uint64
	BroadcastLastFlushLatencyNs    uint64
	RingbufZeroCopyDecodeTotal     uint64
	RingbufCopyDecodeTotal         uint64
	KernelRiskEvaluationsTotal     uint64
	KernelRiskAlertsTotal          uint64
	KernelRiskBlocksTotal          uint64
	KernelRiskLastEvalLatencyNs    uint64
	KernelRiskFeedbackApplied      uint64
	KernelRiskFeedbackDropped      uint64
	KernelRiskFeedbackLastError    string
}

type CollectorHealthResponse struct {
	CollectorMapAvailable          bool              `json:"collectorMapAvailable"`
	RingbufEventsTotal             uint64            `json:"ringbufEventsTotal"`
	RingbufDroppedTotal            uint64            `json:"ringbufDroppedTotal"`
	RingbufReserveFailedTotal      uint64            `json:"ringbufReserveFailedTotal"`
	RingbufZeroCopyDecodeTotal     uint64            `json:"ringbufZeroCopyDecodeTotal"`
	RingbufCopyDecodeTotal         uint64            `json:"ringbufCopyDecodeTotal"`
	EventsByTypeTotal              map[string]uint64 `json:"eventsByTypeTotal"`
	EventsByPidTotal               map[string]uint64 `json:"eventsByPidTotal,omitempty"`
	AgentSightCountersTotal        map[string]uint64 `json:"agentSightCountersTotal,omitempty"`
	BackendQueueLen                int               `json:"backendQueueLen"`
	WsClients                      int               `json:"wsClients"`
	PersistAppendLatencyNs         uint64            `json:"persistAppendLatencyNs"`
	CapturedArchivedTotal          uint64            `json:"capturedArchivedTotal"`
	CapturedPersistedTotal         uint64            `json:"capturedPersistedTotal"`
	CapturedPersistErrorsTotal     uint64            `json:"capturedPersistErrorsTotal"`
	BroadcastQueuedTotal           uint64            `json:"broadcastQueuedTotal"`
	BroadcastDroppedTotal          uint64            `json:"broadcastDroppedTotal"`
	BroadcastLastDropReason        string            `json:"broadcastLastDropReason,omitempty"`
	BroadcastReceivedTotal         uint64            `json:"broadcastReceivedTotal"`
	BroadcastFlushesTotal          uint64            `json:"broadcastFlushesTotal"`
	BroadcastEventsFlushedTotal    uint64            `json:"broadcastEventsFlushedTotal"`
	BroadcastEnvelopesFlushedTotal uint64            `json:"broadcastEnvelopesFlushedTotal"`
	BroadcastMarshalErrorsTotal    uint64            `json:"broadcastMarshalErrorsTotal"`
	BroadcastWriteErrorsTotal      uint64            `json:"broadcastWriteErrorsTotal"`
	BroadcastLastFlushLatencyNs    uint64            `json:"broadcastLastFlushLatencyNs"`
	KernelRiskEvaluationsTotal     uint64            `json:"kernelRiskEvaluationsTotal"`
	KernelRiskAlertsTotal          uint64            `json:"kernelRiskAlertsTotal"`
	KernelRiskBlocksTotal          uint64            `json:"kernelRiskBlocksTotal"`
	KernelRiskLastEvalLatencyNs    uint64            `json:"kernelRiskLastEvalLatencyNs"`
	KernelRiskFeedbackApplied      uint64            `json:"kernelRiskFeedbackApplied"`
	KernelRiskFeedbackDropped      uint64            `json:"kernelRiskFeedbackDropped"`
	KernelRiskFeedbackLastError    string            `json:"kernelRiskFeedbackLastError,omitempty"`
	CaptureHealthy                 bool              `json:"captureHealthy"`
}

type collectorMetricsState struct {
	mu                             sync.RWMutex
	eventsByTypeTotal              map[string]uint64
	eventsByPIDTotal               map[collectorPIDKey]uint64
	agentSightCountersTotal        map[string]uint64
	persistAppendLatencyNs         uint64
	capturedArchivedTotal          uint64
	capturedPersistedTotal         uint64
	capturedPersistErrorsTotal     uint64
	broadcastQueuedTotal           uint64
	broadcastDroppedTotal          uint64
	broadcastLastDropReason        string
	broadcastReceivedTotal         uint64
	broadcastFlushesTotal          uint64
	broadcastEventsFlushedTotal    uint64
	broadcastEnvelopesFlushedTotal uint64
	broadcastMarshalErrorsTotal    uint64
	broadcastWriteErrorsTotal      uint64
	broadcastLastFlushLatencyNs    uint64
	ringbufZeroCopyDecodeTotal     uint64
	ringbufCopyDecodeTotal         uint64
	kernelRiskEvaluationsTotal     uint64
	kernelRiskAlertsTotal          uint64
	kernelRiskBlocksTotal          uint64
	kernelRiskLastEvalLatencyNs    uint64
	kernelRiskFeedbackApplied      uint64
	kernelRiskFeedbackDropped      uint64
	kernelRiskFeedbackLastError    string
}

type CollectorMetricsState = collectorMetricsState

const maxCollectorPIDSeries = 512

func newCollectorMetricsState() *collectorMetricsState {
	return &collectorMetricsState{
		eventsByTypeTotal:       make(map[string]uint64),
		eventsByPIDTotal:        make(map[collectorPIDKey]uint64),
		agentSightCountersTotal: make(map[string]uint64),
	}
}

func NewCollectorMetricsState() *CollectorMetricsState {
	return newCollectorMetricsState()
}

var collectorMetricsStore = newCollectorMetricsState()

func RecordEvent(event *pb.Event) {
	collectorMetricsStore.recordEvent(event)
}

func (s *collectorMetricsState) recordEvent(event *pb.Event) {
	if event == nil {
		return
	}
	typeKey := event.GetType()
	if typeKey == "" {
		typeKey = "unknown"
	}
	pidKey := collectorPIDKey{PID: event.GetPid(), Comm: StringsTrimDefault(event.GetComm(), "unknown")}

	s.mu.Lock()
	s.eventsByTypeTotal[typeKey]++
	if pidKey.PID != 0 {
		if _, ok := s.eventsByPIDTotal[pidKey]; ok || len(s.eventsByPIDTotal) < maxCollectorPIDSeries {
			s.eventsByPIDTotal[pidKey]++
		}
	}
	s.mu.Unlock()
}

func StringsTrimDefault(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func RecordAgentSightCounter(name string) {
	collectorMetricsStore.recordAgentSightCounter(name)
}

func (s *collectorMetricsState) recordAgentSightCounter(name string) {
	name = StringsTrimDefault(name, "unknown")
	s.mu.Lock()
	s.agentSightCountersTotal[name]++
	s.mu.Unlock()
}

func SetPersistAppendLatency(duration time.Duration) {
	collectorMetricsStore.setPersistAppendLatency(duration)
}

func (s *collectorMetricsState) setPersistAppendLatency(duration time.Duration) {
	s.mu.Lock()
	s.persistAppendLatencyNs = uint64(duration.Nanoseconds())
	s.mu.Unlock()
}

func RecordCapturedArchive() {
	collectorMetricsStore.recordCapturedArchive()
}

func (s *collectorMetricsState) recordCapturedArchive() {
	s.mu.Lock()
	s.capturedArchivedTotal++
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordCapturedArchive() {
	s.recordCapturedArchive()
}

func RecordCapturedPersist(err error, duration time.Duration) {
	collectorMetricsStore.recordCapturedPersist(err, duration)
}

func (s *collectorMetricsState) recordCapturedPersist(err error, duration time.Duration) {
	s.mu.Lock()
	s.persistAppendLatencyNs = uint64(duration.Nanoseconds())
	if err != nil {
		s.capturedPersistErrorsTotal++
	} else {
		s.capturedPersistedTotal++
	}
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordCapturedPersist(err error, duration time.Duration) {
	s.recordCapturedPersist(err, duration)
}

func RecordBroadcastEnqueue(accepted bool, reason string) {
	collectorMetricsStore.recordBroadcastEnqueue(accepted, reason)
}

func (s *collectorMetricsState) recordBroadcastEnqueue(accepted bool, reason string) {
	s.mu.Lock()
	if accepted {
		s.broadcastQueuedTotal++
	} else {
		s.broadcastDroppedTotal++
		s.broadcastLastDropReason = StringsTrimDefault(reason, "unknown")
	}
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordBroadcastEnqueue(accepted bool, reason string) {
	s.recordBroadcastEnqueue(accepted, reason)
}

func RecordBroadcastReceived() {
	collectorMetricsStore.recordBroadcastReceived()
}

func (s *collectorMetricsState) recordBroadcastReceived() {
	s.mu.Lock()
	s.broadcastReceivedTotal++
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordBroadcastReceived() {
	s.recordBroadcastReceived()
}

func RecordBroadcastFlush(events, envelopes, marshalErrors, writeErrors int, duration time.Duration) {
	collectorMetricsStore.recordBroadcastFlush(events, envelopes, marshalErrors, writeErrors, duration)
}

func (s *collectorMetricsState) recordBroadcastFlush(events, envelopes, marshalErrors, writeErrors int, duration time.Duration) {
	if events < 0 {
		events = 0
	}
	if envelopes < 0 {
		envelopes = 0
	}
	if marshalErrors < 0 {
		marshalErrors = 0
	}
	if writeErrors < 0 {
		writeErrors = 0
	}
	s.mu.Lock()
	s.broadcastFlushesTotal++
	s.broadcastEventsFlushedTotal += uint64(events)
	s.broadcastEnvelopesFlushedTotal += uint64(envelopes)
	s.broadcastMarshalErrorsTotal += uint64(marshalErrors)
	s.broadcastWriteErrorsTotal += uint64(writeErrors)
	s.broadcastLastFlushLatencyNs = uint64(duration.Nanoseconds())
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordBroadcastFlush(events, envelopes, marshalErrors, writeErrors int, duration time.Duration) {
	s.recordBroadcastFlush(events, envelopes, marshalErrors, writeErrors, duration)
}

func RecordRingbufDecode(zeroCopy bool) {
	collectorMetricsStore.recordRingbufDecode(zeroCopy)
}

func (s *collectorMetricsState) recordRingbufDecode(zeroCopy bool) {
	s.mu.Lock()
	if zeroCopy {
		s.ringbufZeroCopyDecodeTotal++
	} else {
		s.ringbufCopyDecodeTotal++
	}
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordRingbufDecode(zeroCopy bool) {
	s.recordRingbufDecode(zeroCopy)
}

func RecordKernelRiskDecision(decision string, duration time.Duration) {
	collectorMetricsStore.recordKernelRiskDecision(decision, duration)
}

func (s *collectorMetricsState) recordKernelRiskDecision(decision string, duration time.Duration) {
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

func (s *collectorMetricsState) RecordKernelRiskDecision(decision string, duration time.Duration) {
	s.recordKernelRiskDecision(decision, duration)
}

func RecordKernelRiskFeedback(applied bool, err error) {
	collectorMetricsStore.recordKernelRiskFeedback(applied, err)
}

func (s *collectorMetricsState) recordKernelRiskFeedback(applied bool, err error) {
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

func (s *collectorMetricsState) RecordKernelRiskFeedback(applied bool, err error) {
	s.recordKernelRiskFeedback(applied, err)
}

func (s *collectorMetricsState) rawSnapshot() CollectorMetricsSnapshot {
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
	return CollectorMetricsSnapshot{
		EventsByTypeTotal:              eventsByType,
		EventsByPIDTotal:               eventsByPID,
		AgentSightCountersTotal:        agentSightCounters,
		PersistAppendLatencyNs:         s.persistAppendLatencyNs,
		CapturedArchivedTotal:          s.capturedArchivedTotal,
		CapturedPersistedTotal:         s.capturedPersistedTotal,
		CapturedPersistErrorsTotal:     s.capturedPersistErrorsTotal,
		BroadcastQueuedTotal:           s.broadcastQueuedTotal,
		BroadcastDroppedTotal:          s.broadcastDroppedTotal,
		BroadcastLastDropReason:        s.broadcastLastDropReason,
		BroadcastReceivedTotal:         s.broadcastReceivedTotal,
		BroadcastFlushesTotal:          s.broadcastFlushesTotal,
		BroadcastEventsFlushedTotal:    s.broadcastEventsFlushedTotal,
		BroadcastEnvelopesFlushedTotal: s.broadcastEnvelopesFlushedTotal,
		BroadcastMarshalErrorsTotal:    s.broadcastMarshalErrorsTotal,
		BroadcastWriteErrorsTotal:      s.broadcastWriteErrorsTotal,
		BroadcastLastFlushLatencyNs:    s.broadcastLastFlushLatencyNs,
		RingbufZeroCopyDecodeTotal:     s.ringbufZeroCopyDecodeTotal,
		RingbufCopyDecodeTotal:         s.ringbufCopyDecodeTotal,
		KernelRiskEvaluationsTotal:     s.kernelRiskEvaluationsTotal,
		KernelRiskAlertsTotal:          s.kernelRiskAlertsTotal,
		KernelRiskBlocksTotal:          s.kernelRiskBlocksTotal,
		KernelRiskLastEvalLatencyNs:    s.kernelRiskLastEvalLatencyNs,
		KernelRiskFeedbackApplied:      s.kernelRiskFeedbackApplied,
		KernelRiskFeedbackDropped:      s.kernelRiskFeedbackDropped,
		KernelRiskFeedbackLastError:    s.kernelRiskFeedbackLastError,
	}
}

func (s *collectorMetricsState) Snapshot() CollectorMetricsSnapshot {
	return s.rawSnapshot()
}

func GetCollectorHealthSnapshot() CollectorHealthResponse {
	return collectorMetricsStore.snapshot()
}

func (s *collectorMetricsState) snapshot() CollectorHealthResponse {
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

	legacyWSClients := 0
	if deps.ClientsMu != nil {
		deps.ClientsMu.Lock()
		legacyWSClients = len(deps.Clients)
		deps.ClientsMu.Unlock()
	} else if deps.Clients != nil {
		legacyWSClients = len(deps.Clients)
	}
	envelopeWSClients := 0
	if deps.EnvelopeClientsMu != nil {
		deps.EnvelopeClientsMu.Lock()
		envelopeWSClients = len(deps.EnvelopeClients)
		deps.EnvelopeClientsMu.Unlock()
	} else if deps.EnvelopeClients != nil {
		envelopeWSClients = len(deps.EnvelopeClients)
	}

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
		CollectorMapAvailable:          mapAvailable,
		RingbufEventsTotal:             bpfStats.RingbufEventsTotal,
		RingbufDroppedTotal:            bpfStats.RingbufReserveFailedTotal,
		RingbufReserveFailedTotal:      bpfStats.RingbufReserveFailedTotal,
		RingbufZeroCopyDecodeTotal:     raw.RingbufZeroCopyDecodeTotal,
		RingbufCopyDecodeTotal:         raw.RingbufCopyDecodeTotal,
		EventsByTypeTotal:              eventsByType,
		EventsByPidTotal:               eventsByPID,
		AgentSightCountersTotal:        agentSightCounters,
		BackendQueueLen:                len(deps.Broadcast),
		WsClients:                      legacyWSClients + envelopeWSClients,
		PersistAppendLatencyNs:         raw.PersistAppendLatencyNs,
		CapturedArchivedTotal:          raw.CapturedArchivedTotal,
		CapturedPersistedTotal:         raw.CapturedPersistedTotal,
		CapturedPersistErrorsTotal:     raw.CapturedPersistErrorsTotal,
		BroadcastQueuedTotal:           raw.BroadcastQueuedTotal,
		BroadcastDroppedTotal:          raw.BroadcastDroppedTotal,
		BroadcastLastDropReason:        raw.BroadcastLastDropReason,
		BroadcastReceivedTotal:         raw.BroadcastReceivedTotal,
		BroadcastFlushesTotal:          raw.BroadcastFlushesTotal,
		BroadcastEventsFlushedTotal:    raw.BroadcastEventsFlushedTotal,
		BroadcastEnvelopesFlushedTotal: raw.BroadcastEnvelopesFlushedTotal,
		BroadcastMarshalErrorsTotal:    raw.BroadcastMarshalErrorsTotal,
		BroadcastWriteErrorsTotal:      raw.BroadcastWriteErrorsTotal,
		BroadcastLastFlushLatencyNs:    raw.BroadcastLastFlushLatencyNs,
		KernelRiskEvaluationsTotal:     raw.KernelRiskEvaluationsTotal,
		KernelRiskAlertsTotal:          raw.KernelRiskAlertsTotal,
		KernelRiskBlocksTotal:          raw.KernelRiskBlocksTotal,
		KernelRiskLastEvalLatencyNs:    raw.KernelRiskLastEvalLatencyNs,
		KernelRiskFeedbackApplied:      raw.KernelRiskFeedbackApplied,
		KernelRiskFeedbackDropped:      raw.KernelRiskFeedbackDropped,
		KernelRiskFeedbackLastError:    raw.KernelRiskFeedbackLastError,
		CaptureHealthy:                 !mapAvailable || bpfStats.RingbufReserveFailedTotal == 0,
	}
}

func loadCollectorStatsSnapshot() (bpfCollectorStats, bool) {
	if deps.TrackerMaps == nil {
		return bpfCollectorStats{}, false
	}
	collectorStatsMap := deps.TrackerMaps.GetCollectorStats()
	if collectorStatsMap == nil {
		return bpfCollectorStats{}, false
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return bpfCollectorStats{}, false
	}

	values := make([]bpfCollectorStats, cpuCount)
	key := uint32(0)
	if err := collectorStatsMap.Lookup(&key, &values); err != nil {
		return bpfCollectorStats{}, false
	}

	var total bpfCollectorStats
	for _, value := range values {
		total.RingbufEventsTotal += value.RingbufEventsTotal
		total.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal
	}
	return total, true
}

func HandleCollectorHealth(c *gin.Context) {
	c.JSON(http.StatusOK, GetCollectorHealthSnapshot())
}
