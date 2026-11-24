package observability

import (
	"strings"
	"sync"
	"sync/atomic"
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
	ExitIOUpdateFailures       uint64
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
	capturedArchivedTotal          atomic.Uint64
	capturedPersistedTotal         uint64
	capturedPersistErrorsTotal     uint64
	broadcastQueuedTotal           atomic.Uint64
	broadcastDroppedTotal          atomic.Uint64
	broadcastLastDropReason        string
	broadcastReceivedTotal         atomic.Uint64
	broadcastFlushesTotal          uint64
	broadcastEventsFlushedTotal    uint64
	broadcastEnvelopesFlushedTotal uint64
	broadcastMarshalErrorsTotal    uint64
	broadcastWriteErrorsTotal      uint64
	broadcastLastFlushLatencyNs    uint64
	ringbufZeroCopyDecodeTotal     atomic.Uint64
	ringbufCopyDecodeTotal         atomic.Uint64
	kernelCaptureDelaySamples      atomic.Uint64
	kernelCaptureDelayLastNs       atomic.Uint64
	kernelCaptureDelayMaxNs        atomic.Uint64
	kernelCaptureClockUnknown      atomic.Uint64
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

func StringsTrimDefault(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}
