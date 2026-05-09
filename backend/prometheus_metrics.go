package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
)

func handlePrometheusMetrics(c *gin.Context) {
	health := collectorMetricsStore.Snapshot()
	raw := collectorMetricsStore.rawSnapshot()

	var b strings.Builder
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_events_total", "counter", "Total events successfully submitted to the eBPF ring buffer.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_events_total", nil, float64(health.RingbufEventsTotal))
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_dropped_total", "counter", "Total events dropped before ring buffer submission.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_dropped_total", nil, float64(health.RingbufDroppedTotal))
	writePrometheusHeader(&b, "agent_ebpf_ringbuf_reserve_failed_total", "counter", "Total ring buffer reserve failures.")
	writePrometheusSample(&b, "agent_ebpf_ringbuf_reserve_failed_total", nil, float64(health.RingbufReserveFailedTotal))
	writePrometheusHeader(&b, "agent_ebpf_backend_queue_len", "gauge", "Current backend event queue length.")
	writePrometheusSample(&b, "agent_ebpf_backend_queue_len", nil, float64(health.BackendQueueLen))
	writePrometheusHeader(&b, "agent_ebpf_ws_clients", "gauge", "Current number of event WebSocket clients across legacy and envelope streams.")
	writePrometheusSample(&b, "agent_ebpf_ws_clients", nil, float64(health.WsClients))
	writePrometheusHeader(&b, "agent_ebpf_persist_append_latency_seconds", "gauge", "Latest persisted event log append latency in seconds.")
	writePrometheusSample(&b, "agent_ebpf_persist_append_latency_seconds", nil, float64(health.PersistAppendLatencyNs)/1e9)
	writePrometheusHeader(&b, "agent_ebpf_capture_healthy", "gauge", "Whether capture currently reports no ring buffer drops.")
	if health.CaptureHealthy {
		writePrometheusSample(&b, "agent_ebpf_capture_healthy", nil, 1)
	} else {
		writePrometheusSample(&b, "agent_ebpf_capture_healthy", nil, 0)
	}

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
