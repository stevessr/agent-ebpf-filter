package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

func HandlePrometheusMetrics(c *gin.Context) {
	health := collectorMetricsStore.snapshot()
	raw := collectorMetricsStore.rawSnapshot()

	var b strings.Builder
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_events_total", "counter", "Total events successfully submitted to the eBPF ring buffer.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_events_total", nil, float64(health.RingbufEventsTotal))
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_dropped_total", "counter", "Total events dropped before ring buffer submission.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_dropped_total", nil, float64(health.RingbufDroppedTotal))
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_reserve_failed_total", "counter", "Total ring buffer reserve failures.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_reserve_failed_total", nil, float64(health.RingbufReserveFailedTotal))
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_zero_copy_decode_total", "counter", "Total eBPF ring buffer samples decoded through the zero-copy mmap-backed path.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_zero_copy_decode_total", nil, float64(health.RingbufZeroCopyDecodeTotal))
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_copy_decode_total", "counter", "Total eBPF ring buffer samples decoded through the endian/alignment-safe copy fallback path.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_copy_decode_total", nil, float64(health.RingbufCopyDecodeTotal))
	writePrometheusHeader(&b, "agent_ebpf_backend_queue_len", "gauge", "Current backend event queue length.")
	writePrometheusSample(&b, "agent_ebpf_backend_queue_len", nil, float64(health.BackendQueueLen))
	writePrometheusHeader(&b, "agent_ebpf_ws_clients", "gauge", "Current number of event WebSocket clients across legacy and envelope streams.")
	writePrometheusSample(&b, "agent_ebpf_ws_clients", nil, float64(health.WsClients))
	writePrometheusHeader(&b, "agent_ebpf_persist_append_latency_seconds", "gauge", "Latest persisted event log append latency in seconds.")
	writePrometheusSample(&b, "agent_ebpf_persist_append_latency_seconds", nil, float64(health.PersistAppendLatencyNs)/1e9)
	writePrometheusHeader(&b, "agent_ebpf_captured_archived_total", "counter", "Total captured events appended to the in-memory archive.")
	writePrometheusSample(&b, "agent_ebpf_captured_archived_total", nil, float64(health.CapturedArchivedTotal))
	writePrometheusHeader(&b, "agent_ebpf_captured_persisted_total", "counter", "Total captured events successfully appended to the persistent event log.")
	writePrometheusSample(&b, "agent_ebpf_captured_persisted_total", nil, float64(health.CapturedPersistedTotal))
	writePrometheusHeader(&b, "agent_ebpf_captured_persist_errors_total", "counter", "Total captured event persistence failures.")
	writePrometheusSample(&b, "agent_ebpf_captured_persist_errors_total", nil, float64(health.CapturedPersistErrorsTotal))
	writePrometheusHeader(&b, "agent_ebpf_persist_writer_active", "gauge", "Whether the asynchronous persisted-event writer is accepting records.")
	if health.PersistWriterActive {
		writePrometheusSample(&b, "agent_ebpf_persist_writer_active", nil, 1)
	} else {
		writePrometheusSample(&b, "agent_ebpf_persist_writer_active", nil, 0)
	}
	writePrometheusHeader(&b, "agent_ebpf_persist_queue_length", "gauge", "Current persisted-event writer queue length.")
	writePrometheusSample(&b, "agent_ebpf_persist_queue_length", nil, float64(health.PersistQueueLen))
	writePrometheusHeader(&b, "agent_ebpf_persist_queue_capacity", "gauge", "Configured persisted-event writer queue capacity.")
	writePrometheusSample(&b, "agent_ebpf_persist_queue_capacity", nil, float64(health.PersistQueueCap))
	writePrometheusHeader(&b, "agent_ebpf_persist_pending", "gauge", "Accepted persisted-event records not yet completed by the writer.")
	writePrometheusSample(&b, "agent_ebpf_persist_pending", nil, float64(health.PersistPending))
	writePrometheusHeader(&b, "agent_ebpf_persist_generation_failed", "gauge", "Persisted-event records failed in the current writer generation.")
	writePrometheusSample(&b, "agent_ebpf_persist_generation_failed", nil, float64(health.PersistGenerationFailed))
	writePrometheusHeader(&b, "agent_ebpf_persist_generation_dropped", "gauge", "Persisted-event records rejected in the current writer generation.")
	writePrometheusSample(&b, "agent_ebpf_persist_generation_dropped", nil, float64(health.PersistGenerationDropped))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_queued_total", "counter", "Total events accepted into the backend broadcast queue.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_queued_total", nil, float64(health.BroadcastQueuedTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_dropped_total", "counter", "Total events dropped before entering the backend broadcast queue.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_dropped_total", nil, float64(health.BroadcastDroppedTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_received_total", "counter", "Total events received by the backend WebSocket broadcaster.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_received_total", nil, float64(health.BroadcastReceivedTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_flushes_total", "counter", "Total non-empty WebSocket broadcast batch flushes.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_flushes_total", nil, float64(health.BroadcastFlushesTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_events_flushed_total", "counter", "Total legacy event messages included in WebSocket batch flushes.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_events_flushed_total", nil, float64(health.BroadcastEventsFlushedTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_envelopes_flushed_total", "counter", "Total event envelopes included in WebSocket batch flushes.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_envelopes_flushed_total", nil, float64(health.BroadcastEnvelopesFlushedTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_marshal_errors_total", "counter", "Total WebSocket broadcast protobuf marshal errors.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_marshal_errors_total", nil, float64(health.BroadcastMarshalErrorsTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_write_errors_total", "counter", "Total WebSocket broadcast write failures.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_write_errors_total", nil, float64(health.BroadcastWriteErrorsTotal))
	writePrometheusHeader(&b, "agent_ebpf_broadcast_last_flush_latency_seconds", "gauge", "Latest non-empty WebSocket broadcast batch flush latency in seconds.")
	writePrometheusSample(&b, "agent_ebpf_broadcast_last_flush_latency_seconds", nil, float64(health.BroadcastLastFlushLatencyNs)/1e9)
	writePrometheusHeader(&b, "agent_ebpf_capture_healthy", "gauge", "Whether capture currently reports no ring buffer drops.")
	if health.CaptureHealthy {
		writePrometheusSample(&b, "agent_ebpf_capture_healthy", nil, 1)
	} else {
		writePrometheusSample(&b, "agent_ebpf_capture_healthy", nil, 0)
	}
	writePrometheusHeader(&b, "agent_ebpf_semantic_state_entries", "gauge", "Current bounded semantic-correlation state entries by kind.")
	semanticKinds := make([]string, 0, len(health.SemanticStateEntriesByKind))
	for kind := range health.SemanticStateEntriesByKind {
		semanticKinds = append(semanticKinds, kind)
	}
	sort.Strings(semanticKinds)
	for _, kind := range semanticKinds {
		writePrometheusSample(&b, "agent_ebpf_semantic_state_entries", map[string]string{"kind": kind}, float64(health.SemanticStateEntriesByKind[kind]))
	}
	writePrometheusHeader(&b, "agent_ebpf_semantic_state_max_entries", "gauge", "Combined capacity of bounded semantic-correlation state.")
	writePrometheusSample(&b, "agent_ebpf_semantic_state_max_entries", nil, float64(health.SemanticStateMaxEntries))
	writePrometheusHeader(&b, "agent_ebpf_semantic_state_expired_evictions_total", "counter", "Semantic-correlation entries evicted after their TTL.")
	writePrometheusSample(&b, "agent_ebpf_semantic_state_expired_evictions_total", nil, float64(health.SemanticStateExpiredEvictions))
	writePrometheusHeader(&b, "agent_ebpf_semantic_state_capacity_evictions_total", "counter", "Semantic-correlation entries evicted to enforce capacity.")
	writePrometheusSample(&b, "agent_ebpf_semantic_state_capacity_evictions_total", nil, float64(health.SemanticStateCapacityEvictions))
	writePrometheusHeader(&b, "agent_ebpf_semantic_state_truncated_values_total", "counter", "Oversized semantic-correlation keys or values replaced with bounded stable identifiers.")
	writePrometheusSample(&b, "agent_ebpf_semantic_state_truncated_values_total", nil, float64(health.SemanticStateTruncatedValues))
	writePrometheusHeader(&b, "agent_ebpf_semantic_state_ignored_metadata_total", "counter", "Oversized semantic metadata fields ignored before correlation.")
	writePrometheusSample(&b, "agent_ebpf_semantic_state_ignored_metadata_total", nil, float64(health.SemanticStateIgnoredMetadata))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_tools", "gauge", "Current tools retained in the bounded behavior baseline.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_tools", nil, float64(health.ToolBaselineTools))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_samples", "gauge", "Current behavior samples retained in the bounded tool baseline.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_samples", nil, float64(health.ToolBaselineSamples))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_max_tools", "gauge", "Maximum tools retained in the behavior baseline.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_max_tools", nil, float64(health.ToolBaselineMaxTools))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_max_samples", "gauge", "Maximum behavior samples retained across all tools.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_max_samples", nil, float64(health.ToolBaselineMaxSamples))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_observations_total", "counter", "Accepted tool behavior observations checked and recorded atomically.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_observations_total", nil, float64(health.ToolBaselineObservations))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_drifts_total", "counter", "Previously unseen behaviors detected against mature tool baselines.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_drifts_total", nil, float64(health.ToolBaselineDrifts))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_expired_evictions_total", "counter", "Tool behavior samples evicted after their TTL.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_expired_evictions_total", nil, float64(health.ToolBaselineExpiredEvictions))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_capacity_evictions_total", "counter", "Tool behavior samples evicted to enforce capacity.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_capacity_evictions_total", nil, float64(health.ToolBaselineCapacityEvictions))
	writePrometheusHeader(&b, "agent_ebpf_tool_baseline_truncated_values_total", "counter", "Oversized tool baseline values replaced with bounded stable identifiers.")
	writePrometheusSample(&b, "agent_ebpf_tool_baseline_truncated_values_total", nil, float64(health.ToolBaselineTruncatedValues))
	writePrometheusHeader(&b, "agent_ebpf_kernel_risk_evaluations_total", "counter", "Total low-latency kernel event risk evaluations run before event broadcast.")
	writePrometheusSample(&b, "agent_ebpf_kernel_risk_evaluations_total", nil, float64(health.KernelRiskEvaluationsTotal))
	writePrometheusHeader(&b, "agent_ebpf_kernel_risk_alerts_total", "counter", "Total kernel event risk evaluations that produced ALERT or OBSERVE decisions.")
	writePrometheusSample(&b, "agent_ebpf_kernel_risk_alerts_total", nil, float64(health.KernelRiskAlertsTotal))
	writePrometheusHeader(&b, "agent_ebpf_kernel_risk_blocks_total", "counter", "Total kernel event risk evaluations that produced BLOCK decisions.")
	writePrometheusSample(&b, "agent_ebpf_kernel_risk_blocks_total", nil, float64(health.KernelRiskBlocksTotal))
	writePrometheusHeader(&b, "agent_ebpf_kernel_risk_last_eval_latency_seconds", "gauge", "Latest kernel event risk evaluation latency in seconds.")
	writePrometheusSample(&b, "agent_ebpf_kernel_risk_last_eval_latency_seconds", nil, float64(health.KernelRiskLastEvalLatencyNs)/1e9)
	writePrometheusHeader(&b, "agent_ebpf_kernel_risk_feedback_applied_total", "counter", "Total user-space risk decisions fed back into kernel-enforced cgroup or LSM policy maps.")
	writePrometheusSample(&b, "agent_ebpf_kernel_risk_feedback_applied_total", nil, float64(health.KernelRiskFeedbackApplied))
	writePrometheusHeader(&b, "agent_ebpf_kernel_risk_feedback_dropped_total", "counter", "Total kernel-risk feedback actions skipped, rate-limited, queued out, or failed.")
	writePrometheusSample(&b, "agent_ebpf_kernel_risk_feedback_dropped_total", nil, float64(health.KernelRiskFeedbackDropped))

	typeKeys := make([]string, 0, len(raw.EventsByTypeTotal))
	for key := range raw.EventsByTypeTotal {
		typeKeys = append(typeKeys, key)
	}
	sort.Strings(typeKeys)
	writePrometheusHeader(&b, "agent_ebpf_events_by_type_total", "counter", "Captured events grouped by event type.")
	for _, key := range typeKeys {
		writePrometheusSample(&b, "agent_ebpf_events_by_type_total", map[string]string{"type": key}, float64(raw.EventsByTypeTotal[key]))
	}

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
	writePrometheusHeader(&b, "agent_ebpf_events_by_pid_total", "counter", "Captured events grouped by pid and command.")
	for _, key := range pidKeys {
		writePrometheusSample(&b, "agent_ebpf_events_by_pid_total", map[string]string{"pid": fmt.Sprintf("%d", key.PID), "comm": key.Comm}, float64(raw.EventsByPIDTotal[key]))
	}

	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(b.String()))
}

func writePrometheusHeader(builder *strings.Builder, name, metricType, help string) {
	fmt.Fprintf(builder, "# HELP %s %s\n", name, help)
	fmt.Fprintf(builder, "# TYPE %s %s\n", name, metricType)
}

func writePrometheusSample(builder *strings.Builder, name string, labels map[string]string, value float64) {
	fmt.Fprintf(builder, "%s%s %v\n", name, formatPrometheusLabels(labels), value)
}

func formatPrometheusLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", key, escapePrometheusLabelValue(labels[key])))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func escapePrometheusLabelValue(value string) string {
	replacer := strings.NewReplacer(
		"\\", "\\\\",
		"\n", "\\n",
		"\"", "\\\"",
	)
	return replacer.Replace(value)
}
