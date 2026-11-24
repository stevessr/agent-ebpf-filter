package observability

import (
	"agent-ebpf-filter/pb"
	"strings"
	"time"
)

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
	s.capturedArchivedTotal.Add(1)
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
	if accepted {
		s.broadcastQueuedTotal.Add(1)
		return
	}
	s.broadcastDroppedTotal.Add(1)
	// The drop reason is diagnostic text and changes only on the exceptional
	// path. Keep it behind the mutex without forcing successful enqueues to
	// contend with the map-backed event metrics.
	s.mu.Lock()
	s.broadcastLastDropReason = StringsTrimDefault(reason, "unknown")
	s.mu.Unlock()
}

func (s *collectorMetricsState) RecordBroadcastEnqueue(accepted bool, reason string) {
	s.recordBroadcastEnqueue(accepted, reason)
}

func RecordBroadcastReceived() {
	collectorMetricsStore.recordBroadcastReceived()
}

func (s *collectorMetricsState) recordBroadcastReceived() {
	s.broadcastReceivedTotal.Add(1)
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
	if zeroCopy {
		s.ringbufZeroCopyDecodeTotal.Add(1)
		return
	}
	s.ringbufCopyDecodeTotal.Add(1)
}

func (s *collectorMetricsState) RecordRingbufDecode(zeroCopy bool) {
	s.recordRingbufDecode(zeroCopy)
}

func RecordKernelCaptureTiming(delayNS uint64, clock string) {
	collectorMetricsStore.recordKernelCaptureTiming(delayNS, clock)
}

func (s *collectorMetricsState) recordKernelCaptureTiming(delayNS uint64, clock string) {
	s.kernelCaptureDelaySamples.Add(1)
	s.kernelCaptureDelayLastNs.Store(delayNS)
	for {
		current := s.kernelCaptureDelayMaxNs.Load()
		if delayNS <= current || s.kernelCaptureDelayMaxNs.CompareAndSwap(current, delayNS) {
			break
		}
	}
	if strings.TrimSpace(clock) != "monotonic" {
		s.kernelCaptureClockUnknown.Add(1)
	}
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
		CapturedArchivedTotal:          s.capturedArchivedTotal.Load(),
		CapturedPersistedTotal:         s.capturedPersistedTotal,
		CapturedPersistErrorsTotal:     s.capturedPersistErrorsTotal,
		BroadcastQueuedTotal:           s.broadcastQueuedTotal.Load(),
		BroadcastDroppedTotal:          s.broadcastDroppedTotal.Load(),
		BroadcastLastDropReason:        s.broadcastLastDropReason,
		BroadcastReceivedTotal:         s.broadcastReceivedTotal.Load(),
		BroadcastFlushesTotal:          s.broadcastFlushesTotal,
		BroadcastEventsFlushedTotal:    s.broadcastEventsFlushedTotal,
		BroadcastEnvelopesFlushedTotal: s.broadcastEnvelopesFlushedTotal,
		BroadcastMarshalErrorsTotal:    s.broadcastMarshalErrorsTotal,
		BroadcastWriteErrorsTotal:      s.broadcastWriteErrorsTotal,
		BroadcastLastFlushLatencyNs:    s.broadcastLastFlushLatencyNs,
		RingbufZeroCopyDecodeTotal:     s.ringbufZeroCopyDecodeTotal.Load(),
		RingbufCopyDecodeTotal:         s.ringbufCopyDecodeTotal.Load(),
		KernelCaptureDelaySamples:      s.kernelCaptureDelaySamples.Load(),
		KernelCaptureDelayLastNs:       s.kernelCaptureDelayLastNs.Load(),
		KernelCaptureDelayMaxNs:        s.kernelCaptureDelayMaxNs.Load(),
		KernelCaptureClockUnknown:      s.kernelCaptureClockUnknown.Load(),
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
