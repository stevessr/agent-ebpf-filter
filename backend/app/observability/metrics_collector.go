package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/pb"
)

type bpfCollectorStats struct {
	RingbufEventsTotal        uint64
	RingbufReserveFailedTotal uint64
	EventSequence             uint64
	PendingDroppedEvents      uint64
	AuditGeneration           uint64
}

type kernelContextPressureStats struct {
	ExitFullUpdateFailures     uint64
	ExitCompactUpdateFailures  uint64
	SinglePathUpdateFailures   uint64
	PairPathUpdateFailures     uint64
	SocketFDUpdateFailures     uint64
	SocketParentUpdateFailures uint64
}

type ContextMapPressure struct {
	Entries              uint32  `json:"entries"`
	Capacity             uint32  `json:"capacity"`
	KeySize              uint32  `json:"keySize"`
	ValueSize            uint32  `json:"valueSize"`
	PayloadBytes         uint64  `json:"payloadBytes"`
	CapacityPayloadBytes uint64  `json:"capacityPayloadBytes"`
	Utilization          float64 `json:"utilization"`
	UpdateFailuresTotal  uint64  `json:"updateFailuresTotal"`
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
	KernelCaptureDelaySamples      uint64
	KernelCaptureDelayLastNs       uint64
	KernelCaptureDelayMaxNs        uint64
	KernelCaptureClockUnknown      uint64
	KernelRiskEvaluationsTotal     uint64
	KernelRiskAlertsTotal          uint64
	KernelRiskBlocksTotal          uint64
	KernelRiskLastEvalLatencyNs    uint64
	KernelRiskFeedbackApplied      uint64
	KernelRiskFeedbackDropped      uint64
	KernelRiskFeedbackLastError    string
}

type CollectorHealthResponse struct {
	CollectorMapAvailable            bool                          `json:"collectorMapAvailable"`
	RingbufEventsTotal               uint64                        `json:"ringbufEventsTotal"`
	RingbufDroppedTotal              uint64                        `json:"ringbufDroppedTotal"`
	RingbufReserveFailedTotal        uint64                        `json:"ringbufReserveFailedTotal"`
	RingbufZeroCopyDecodeTotal       uint64                        `json:"ringbufZeroCopyDecodeTotal"`
	RingbufCopyDecodeTotal           uint64                        `json:"ringbufCopyDecodeTotal"`
	KernelSequencedEventsTotal       uint64                        `json:"kernelSequencedEventsTotal"`
	KernelSequenceAttemptsTotal      uint64                        `json:"kernelSequenceAttemptsTotal"`
	KernelPendingDroppedEvents       uint64                        `json:"kernelPendingDroppedEvents"`
	KernelAuditGeneration            uint64                        `json:"kernelAuditGeneration"`
	KernelAuditGenerationConsistent  bool                          `json:"kernelAuditGenerationConsistent"`
	KernelReportedReserveGapEvents   uint64                        `json:"kernelReportedReserveGapEventsTotal"`
	KernelReportedReserveDropped     uint64                        `json:"kernelReportedReserveDroppedEventsTotal"`
	KernelSequenceUnexplainedMissing uint64                        `json:"kernelSequenceUnexplainedMissingEventsTotal"`
	KernelCaptureDelaySamples        uint64                        `json:"kernelCaptureDelaySamples"`
	KernelCaptureDelayLastNs         uint64                        `json:"kernelCaptureDelayLastNs"`
	KernelCaptureDelayMaxNs          uint64                        `json:"kernelCaptureDelayMaxNs"`
	KernelCaptureClockUnknown        uint64                        `json:"kernelCaptureClockUnknownTotal"`
	ContextMapsAvailable             bool                          `json:"contextMapsAvailable"`
	ContextMapPressure               map[string]ContextMapPressure `json:"contextMapPressure,omitempty"`
	ContextMapUpdateFailuresTotal    uint64                        `json:"contextMapUpdateFailuresTotal"`
	EventsByTypeTotal                map[string]uint64             `json:"eventsByTypeTotal"`
	EventsByPidTotal                 map[string]uint64             `json:"eventsByPidTotal,omitempty"`
	AgentSightCountersTotal          map[string]uint64             `json:"agentSightCountersTotal,omitempty"`
	SemanticStateEntriesByKind       map[string]int                `json:"semanticStateEntriesByKind"`
	SemanticStateEntries             int                           `json:"semanticStateEntries"`
	SemanticStateMaxEntries          int                           `json:"semanticStateMaxEntries"`
	SemanticStateExpiredEvictions    uint64                        `json:"semanticStateExpiredEvictionsTotal"`
	SemanticStateCapacityEvictions   uint64                        `json:"semanticStateCapacityEvictionsTotal"`
	SemanticStateTruncatedValues     uint64                        `json:"semanticStateTruncatedValuesTotal"`
	SemanticStateIgnoredMetadata     uint64                        `json:"semanticStateIgnoredOversizedMetadataTotal"`
	SemanticStateLastSweepAt         string                        `json:"semanticStateLastSweepAt,omitempty"`
	ToolBaselineTools                int                           `json:"toolBaselineTools"`
	ToolBaselineSamples              int                           `json:"toolBaselineSamples"`
	ToolBaselineMaxTools             int                           `json:"toolBaselineMaxTools"`
	ToolBaselineMaxSamples           int                           `json:"toolBaselineMaxSamples"`
	ToolBaselineMaxSamplesPerTool    int                           `json:"toolBaselineMaxSamplesPerTool"`
	ToolBaselineObservations         uint64                        `json:"toolBaselineObservationsTotal"`
	ToolBaselineDrifts               uint64                        `json:"toolBaselineDriftsTotal"`
	ToolBaselineExpiredEvictions     uint64                        `json:"toolBaselineExpiredEvictionsTotal"`
	ToolBaselineCapacityEvictions    uint64                        `json:"toolBaselineCapacityEvictionsTotal"`
	ToolBaselineTruncatedValues      uint64                        `json:"toolBaselineTruncatedValuesTotal"`
	ToolBaselineLastSweepAt          string                        `json:"toolBaselineLastSweepAt,omitempty"`
	BackendQueueLen                  int                           `json:"backendQueueLen"`
	WsClients                        int                           `json:"wsClients"`
	PersistAppendLatencyNs           uint64                        `json:"persistAppendLatencyNs"`
	CapturedArchivedTotal            uint64                        `json:"capturedArchivedTotal"`
	CapturedPersistedTotal           uint64                        `json:"capturedPersistedTotal"`
	CapturedPersistErrorsTotal       uint64                        `json:"capturedPersistErrorsTotal"`
	PersistWriterActive              bool                          `json:"persistWriterActive"`
	PersistWriterStopping            bool                          `json:"persistWriterStopping"`
	PersistQueueLen                  int                           `json:"persistQueueLen"`
	PersistQueueCap                  int                           `json:"persistQueueCap"`
	PersistPending                   uint64                        `json:"persistPending"`
	PersistGenerationEnqueued        uint64                        `json:"persistGenerationEnqueued"`
	PersistGenerationPersisted       uint64                        `json:"persistGenerationPersisted"`
	PersistGenerationFailed          uint64                        `json:"persistGenerationFailed"`
	PersistGenerationDropped         uint64                        `json:"persistGenerationDropped"`
	PersistWriterLastFlushedAt       string                        `json:"persistWriterLastFlushedAt,omitempty"`
	PersistWriterLastError           string                        `json:"persistWriterLastError,omitempty"`
	BroadcastQueuedTotal             uint64                        `json:"broadcastQueuedTotal"`
	BroadcastDroppedTotal            uint64                        `json:"broadcastDroppedTotal"`
	BroadcastLastDropReason          string                        `json:"broadcastLastDropReason,omitempty"`
	BroadcastReceivedTotal           uint64                        `json:"broadcastReceivedTotal"`
	BroadcastFlushesTotal            uint64                        `json:"broadcastFlushesTotal"`
	BroadcastEventsFlushedTotal      uint64                        `json:"broadcastEventsFlushedTotal"`
	BroadcastEnvelopesFlushedTotal   uint64                        `json:"broadcastEnvelopesFlushedTotal"`
	BroadcastMarshalErrorsTotal      uint64                        `json:"broadcastMarshalErrorsTotal"`
	BroadcastWriteErrorsTotal        uint64                        `json:"broadcastWriteErrorsTotal"`
	BroadcastLastFlushLatencyNs      uint64                        `json:"broadcastLastFlushLatencyNs"`
	KernelRiskEvaluationsTotal       uint64                        `json:"kernelRiskEvaluationsTotal"`
	KernelRiskAlertsTotal            uint64                        `json:"kernelRiskAlertsTotal"`
	KernelRiskBlocksTotal            uint64                        `json:"kernelRiskBlocksTotal"`
	KernelRiskLastEvalLatencyNs      uint64                        `json:"kernelRiskLastEvalLatencyNs"`
	KernelRiskFeedbackApplied        uint64                        `json:"kernelRiskFeedbackApplied"`
	KernelRiskFeedbackDropped        uint64                        `json:"kernelRiskFeedbackDropped"`
	KernelRiskFeedbackLastError      string                        `json:"kernelRiskFeedbackLastError,omitempty"`
	CaptureHealthy                   bool                          `json:"captureHealthy"`
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
	kernelCaptureDelaySamples      uint64
	kernelCaptureDelayLastNs       uint64
	kernelCaptureDelayMaxNs        uint64
	kernelCaptureClockUnknown      uint64
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
	collectorMetricsStore.recordAgentSightCounterN(name, 1)
}

func RecordAgentSightCounterN(name string, delta uint64) {
	collectorMetricsStore.recordAgentSightCounterN(name, delta)
}

func (s *collectorMetricsState) recordAgentSightCounter(name string) {
	s.recordAgentSightCounterN(name, 1)
}

func (s *collectorMetricsState) recordAgentSightCounterN(name string, delta uint64) {
	if delta == 0 {
		return
	}
	name = StringsTrimDefault(name, "unknown")
	s.mu.Lock()
	s.agentSightCountersTotal[name] += delta
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
	if err != nil {
		s.recordCapturedPersistBatch(0, 1, duration)
		return
	}
	s.recordCapturedPersistBatch(1, 0, duration)
}

func RecordCapturedPersistBatch(persisted, failed uint64, duration time.Duration) {
	collectorMetricsStore.recordCapturedPersistBatch(persisted, failed, duration)
}

func (s *collectorMetricsState) recordCapturedPersistBatch(persisted, failed uint64, duration time.Duration) {
	if persisted == 0 && failed == 0 {
		return
	}
	s.mu.Lock()
	s.persistAppendLatencyNs = uint64(duration.Nanoseconds())
	s.capturedPersistedTotal += persisted
	s.capturedPersistErrorsTotal += failed
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordCapturedPersist(err error, duration time.Duration) {
	s.recordCapturedPersist(err, duration)
}

func (s *collectorMetricsState) RecordCapturedPersistBatch(persisted, failed uint64, duration time.Duration) {
	s.recordCapturedPersistBatch(persisted, failed, duration)
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

func RecordKernelCaptureTiming(delayNS uint64, clock string) {
	collectorMetricsStore.recordKernelCaptureTiming(delayNS, clock)
}

func (s *collectorMetricsState) recordKernelCaptureTiming(delayNS uint64, clock string) {
	s.mu.Lock()
	s.kernelCaptureDelaySamples++
	s.kernelCaptureDelayLastNs = delayNS
	if delayNS > s.kernelCaptureDelayMaxNs {
		s.kernelCaptureDelayMaxNs = delayNS
	}
	if strings.TrimSpace(clock) != "monotonic" {
		s.kernelCaptureClockUnknown++
	}
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordKernelCaptureTiming(delayNS uint64, clock string) {
	s.recordKernelCaptureTiming(delayNS, clock)
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
		KernelCaptureDelaySamples:      s.kernelCaptureDelaySamples,
		KernelCaptureDelayLastNs:       s.kernelCaptureDelayLastNs,
		KernelCaptureDelayMaxNs:        s.kernelCaptureDelayMaxNs,
		KernelCaptureClockUnknown:      s.kernelCaptureClockUnknown,
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
	bpfStats, mapAvailable, generationConsistent := loadCollectorStatsSnapshot()
	contextPressure, contextMapsAvailable, contextUpdateFailures := loadContextMapPressureSnapshot()
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
	if deps.LegacyWSClientCount != nil {
		legacyWSClients = deps.LegacyWSClientCount()
	}
	envelopeWSClients := 0
	if deps.EnvelopeWSClientCount != nil {
		envelopeWSClients = deps.EnvelopeWSClientCount()
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
	persistQueue := PersistQueueStatus{}
	if deps.PersistQueueStatus != nil {
		persistQueue = deps.PersistQueueStatus()
	}
	semanticState := SemanticStateStatus{EntriesByKind: map[string]int{}}
	if deps.SemanticStateStatus != nil {
		semanticState = deps.SemanticStateStatus()
	}
	if semanticState.EntriesByKind == nil {
		semanticState.EntriesByKind = map[string]int{}
	}
	toolBaseline := ToolBaselineStatus{}
	if deps.ToolBaselineStatus != nil {
		toolBaseline = deps.ToolBaselineStatus()
	}

	return CollectorHealthResponse{
		CollectorMapAvailable:            mapAvailable,
		RingbufEventsTotal:               bpfStats.RingbufEventsTotal,
		RingbufDroppedTotal:              bpfStats.RingbufReserveFailedTotal,
		RingbufReserveFailedTotal:        bpfStats.RingbufReserveFailedTotal,
		RingbufZeroCopyDecodeTotal:       raw.RingbufZeroCopyDecodeTotal,
		RingbufCopyDecodeTotal:           raw.RingbufCopyDecodeTotal,
		KernelSequencedEventsTotal:       bpfStats.RingbufEventsTotal,
		KernelSequenceAttemptsTotal:      bpfStats.EventSequence,
		KernelPendingDroppedEvents:       bpfStats.PendingDroppedEvents,
		KernelAuditGeneration:            bpfStats.AuditGeneration,
		KernelAuditGenerationConsistent:  generationConsistent,
		KernelReportedReserveGapEvents:   raw.AgentSightCountersTotal["kernel_reported_reserve_gap_events"],
		KernelReportedReserveDropped:     raw.AgentSightCountersTotal["kernel_reported_reserve_dropped_events"],
		KernelSequenceUnexplainedMissing: raw.AgentSightCountersTotal["kernel_sequence_unexplained_missing_events"],
		KernelCaptureDelaySamples:        raw.KernelCaptureDelaySamples,
		KernelCaptureDelayLastNs:         raw.KernelCaptureDelayLastNs,
		KernelCaptureDelayMaxNs:          raw.KernelCaptureDelayMaxNs,
		KernelCaptureClockUnknown:        raw.KernelCaptureClockUnknown,
		ContextMapsAvailable:             contextMapsAvailable,
		ContextMapPressure:               contextPressure,
		ContextMapUpdateFailuresTotal:    contextUpdateFailures,
		EventsByTypeTotal:                eventsByType,
		EventsByPidTotal:                 eventsByPID,
		AgentSightCountersTotal:          agentSightCounters,
		SemanticStateEntriesByKind:       semanticState.EntriesByKind,
		SemanticStateEntries:             semanticState.Entries,
		SemanticStateMaxEntries:          semanticState.MaxEntries,
		SemanticStateExpiredEvictions:    semanticState.ExpiredEvictionsTotal,
		SemanticStateCapacityEvictions:   semanticState.CapacityEvictionsTotal,
		SemanticStateTruncatedValues:     semanticState.TruncatedStateValuesTotal,
		SemanticStateIgnoredMetadata:     semanticState.IgnoredOversizedMetadataTotal,
		SemanticStateLastSweepAt:         semanticState.LastSweepAt,
		ToolBaselineTools:                toolBaseline.Tools,
		ToolBaselineSamples:              toolBaseline.Samples,
		ToolBaselineMaxTools:             toolBaseline.MaxTools,
		ToolBaselineMaxSamples:           toolBaseline.MaxSamples,
		ToolBaselineMaxSamplesPerTool:    toolBaseline.MaxSamplesPerTool,
		ToolBaselineObservations:         toolBaseline.ObservationsTotal,
		ToolBaselineDrifts:               toolBaseline.DriftsTotal,
		ToolBaselineExpiredEvictions:     toolBaseline.ExpiredEvictionsTotal,
		ToolBaselineCapacityEvictions:    toolBaseline.CapacityEvictionsTotal,
		ToolBaselineTruncatedValues:      toolBaseline.TruncatedStateValuesTotal,
		ToolBaselineLastSweepAt:          toolBaseline.LastSweepAt,
		BackendQueueLen:                  len(deps.Broadcast),
		WsClients:                        legacyWSClients + envelopeWSClients,
		PersistAppendLatencyNs:           raw.PersistAppendLatencyNs,
		CapturedArchivedTotal:            raw.CapturedArchivedTotal,
		CapturedPersistedTotal:           raw.CapturedPersistedTotal,
		CapturedPersistErrorsTotal:       raw.CapturedPersistErrorsTotal,
		PersistWriterActive:              persistQueue.Active,
		PersistWriterStopping:            persistQueue.Stopping,
		PersistQueueLen:                  persistQueue.QueueLen,
		PersistQueueCap:                  persistQueue.QueueCap,
		PersistPending:                   persistQueue.Pending,
		PersistGenerationEnqueued:        persistQueue.EnqueuedTotal,
		PersistGenerationPersisted:       persistQueue.PersistedTotal,
		PersistGenerationFailed:          persistQueue.FailedTotal,
		PersistGenerationDropped:         persistQueue.DroppedTotal,
		PersistWriterLastFlushedAt:       persistQueue.LastFlushedAt,
		PersistWriterLastError:           persistQueue.LastError,
		BroadcastQueuedTotal:             raw.BroadcastQueuedTotal,
		BroadcastDroppedTotal:            raw.BroadcastDroppedTotal,
		BroadcastLastDropReason:          raw.BroadcastLastDropReason,
		BroadcastReceivedTotal:           raw.BroadcastReceivedTotal,
		BroadcastFlushesTotal:            raw.BroadcastFlushesTotal,
		BroadcastEventsFlushedTotal:      raw.BroadcastEventsFlushedTotal,
		BroadcastEnvelopesFlushedTotal:   raw.BroadcastEnvelopesFlushedTotal,
		BroadcastMarshalErrorsTotal:      raw.BroadcastMarshalErrorsTotal,
		BroadcastWriteErrorsTotal:        raw.BroadcastWriteErrorsTotal,
		BroadcastLastFlushLatencyNs:      raw.BroadcastLastFlushLatencyNs,
		KernelRiskEvaluationsTotal:       raw.KernelRiskEvaluationsTotal,
		KernelRiskAlertsTotal:            raw.KernelRiskAlertsTotal,
		KernelRiskBlocksTotal:            raw.KernelRiskBlocksTotal,
		KernelRiskLastEvalLatencyNs:      raw.KernelRiskLastEvalLatencyNs,
		KernelRiskFeedbackApplied:        raw.KernelRiskFeedbackApplied,
		KernelRiskFeedbackDropped:        raw.KernelRiskFeedbackDropped,
		KernelRiskFeedbackLastError:      raw.KernelRiskFeedbackLastError,
		CaptureHealthy:                   !mapAvailable || (contextMapsAvailable && bpfStats.RingbufReserveFailedTotal == 0 && generationConsistent && raw.AgentSightCountersTotal["kernel_sequence_unexplained_missing_events"] == 0 && raw.AgentSightCountersTotal["kernel_sequence_out_of_order"] == 0 && contextUpdateFailures == 0),
	}
}

func contextFailureByMap(stats kernelContextPressureStats) map[string]uint64 {
	return map[string]uint64{
		"exit_ctx":             stats.ExitFullUpdateFailures,
		"exit_compact_ctx":     stats.ExitCompactUpdateFailures,
		"exit_single_path_ctx": stats.SinglePathUpdateFailures,
		"exit_path_ctx":        stats.PairPathUpdateFailures,
		"socket_fds":           stats.SocketFDUpdateFailures,
		"socket_fd_parents":    stats.SocketParentUpdateFailures,
	}
}

func aggregateContextPressureStats(values []kernelContextPressureStats) kernelContextPressureStats {
	var total kernelContextPressureStats
	for _, value := range values {
		total.ExitFullUpdateFailures += value.ExitFullUpdateFailures
		total.ExitCompactUpdateFailures += value.ExitCompactUpdateFailures
		total.SinglePathUpdateFailures += value.SinglePathUpdateFailures
		total.PairPathUpdateFailures += value.PairPathUpdateFailures
		total.SocketFDUpdateFailures += value.SocketFDUpdateFailures
		total.SocketParentUpdateFailures += value.SocketParentUpdateFailures
	}
	return total
}

func countBPFMapEntries(m *ebpf.Map) (uint32, error) {
	if m == nil {
		return 0, nil
	}
	var count uint32
	var key []byte
	for {
		next, err := m.NextKeyBytes(key)
		if err != nil {
			return count, err
		}
		if next == nil {
			return count, nil
		}
		count++
		if count >= m.MaxEntries() {
			// Concurrent deletion can make hash iteration revisit keys. Clamp at
			// capacity so health polling cannot spin indefinitely on a hot map.
			return count, nil
		}
		key = next
	}
}

func buildContextMapPressure(m *ebpf.Map, failures uint64) (ContextMapPressure, error) {
	if m == nil {
		return ContextMapPressure{UpdateFailuresTotal: failures}, nil
	}
	entries, err := countBPFMapEntries(m)
	if err != nil {
		return ContextMapPressure{}, err
	}
	capacity := m.MaxEntries()
	keySize := m.KeySize()
	valueSize := m.ValueSize()
	entryPayload := uint64(keySize) + uint64(valueSize)
	pressure := ContextMapPressure{
		Entries:              entries,
		Capacity:             capacity,
		KeySize:              keySize,
		ValueSize:            valueSize,
		PayloadBytes:         uint64(entries) * entryPayload,
		CapacityPayloadBytes: uint64(capacity) * entryPayload,
		UpdateFailuresTotal:  failures,
	}
	if capacity != 0 {
		pressure.Utilization = float64(entries) / float64(capacity)
	}
	return pressure, nil
}

func loadContextMapPressureSnapshot() (map[string]ContextMapPressure, bool, uint64) {
	if deps.TrackerMaps == nil {
		return map[string]ContextMapPressure{}, false, 0
	}
	maps := deps.TrackerMaps.GetContextMaps()
	if len(maps) == 0 {
		return map[string]ContextMapPressure{}, false, 0
	}

	var totalStats kernelContextPressureStats
	statsAvailable := false
	if statsMap := deps.TrackerMaps.GetContextPressureStats(); statsMap != nil {
		cpuCount, err := ebpf.PossibleCPU()
		if err == nil && cpuCount > 0 {
			values := make([]kernelContextPressureStats, cpuCount)
			key := uint32(0)
			if err := statsMap.Lookup(&key, &values); err == nil {
				totalStats = aggregateContextPressureStats(values)
				statsAvailable = true
			}
		}
	}
	failures := contextFailureByMap(totalStats)
	out := make(map[string]ContextMapPressure, len(maps))
	available := statsAvailable
	var totalFailures uint64
	for name, m := range maps {
		if m == nil {
			available = false
			continue
		}
		pressure, err := buildContextMapPressure(m, failures[name])
		if err != nil {
			available = false
			continue
		}
		out[name] = pressure
		totalFailures += pressure.UpdateFailuresTotal
	}
	return out, available, totalFailures
}

func loadCollectorStatsSnapshot() (bpfCollectorStats, bool, bool) {
	if deps.TrackerMaps == nil {
		return bpfCollectorStats{}, false, true
	}
	collectorStatsMap := deps.TrackerMaps.GetCollectorStats()
	if collectorStatsMap == nil {
		return bpfCollectorStats{}, false, true
	}

	cpuCount, err := ebpf.PossibleCPU()
	if err != nil || cpuCount <= 0 {
		return bpfCollectorStats{}, false, true
	}

	values := make([]bpfCollectorStats, cpuCount)
	key := uint32(0)
	if err := collectorStatsMap.Lookup(&key, &values); err != nil {
		return bpfCollectorStats{}, false, true
	}

	var total bpfCollectorStats
	generationConsistent := true
	for _, value := range values {
		total.RingbufEventsTotal += value.RingbufEventsTotal
		total.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal
		total.EventSequence += value.EventSequence
		total.PendingDroppedEvents += value.PendingDroppedEvents
		if value.AuditGeneration == 0 {
			generationConsistent = false
			continue
		}
		if total.AuditGeneration == 0 {
			total.AuditGeneration = value.AuditGeneration
		} else if total.AuditGeneration != value.AuditGeneration {
			generationConsistent = false
		}
	}
	return total, true, generationConsistent
}

func HandleCollectorHealth(c *gin.Context) {
	c.JSON(http.StatusOK, GetCollectorHealthSnapshot())
}
