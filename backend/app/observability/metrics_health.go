package observability

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/cilium/ebpf"
	"github.com/gin-gonic/gin"
)

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
		"exit_io_ctx":          stats.ExitIOUpdateFailures,
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
		total.ExitIOUpdateFailures += value.ExitIOUpdateFailures
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
