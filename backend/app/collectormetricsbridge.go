package app

import (
	"agent-ebpf-filter/app/observability"
	"agent-ebpf-filter/pb"
	"time"

	"github.com/gin-gonic/gin"
)

// ── Type aliases ─────────────────────────────────────────────────────────
//
// These types are defined in app/observability/ and re-exported here so that
// callers in the app package (handlers, MCP, routes, TLS, kernel risk, etc.)
// can reference them without changing import paths.

type CollectorHealthResponse = observability.CollectorHealthResponse

// collectorMetricsSnapshot is used by events__attribution_cgroup.go for
// collector rate-limit hint computation.
type collectorMetricsSnapshot = observability.CollectorMetricsSnapshot
type collectorMetricsState = observability.CollectorMetricsState

func newCollectorMetricsState() *collectorMetricsState {
	return observability.NewCollectorMetricsState()
}

// stringsTrimDefault is used by kernel_risk.go and kernel_risk_feedback.go
// in the app package. It is defined in observability/ but needed here.
func stringsTrimDefault(value, fallback string) string {
	return observability.StringsTrimDefault(value, fallback)
}

// ── collectorMetricsStore bridge ─────────────────────────────────────────
//
// collectorMetricsStore retains the same variable name so all 20+ callers
// in app/ continue to work without modification. Every method delegates to
// the observability subpackage.

type metricsStoreBridge struct{}

func (metricsStoreBridge) RecordEvent(event *pb.Event) {
	observability.RecordEvent(event)
}

func (metricsStoreBridge) RecordAgentSightCounter(name string) {
	observability.RecordAgentSightCounter(name)
}

func (metricsStoreBridge) SetPersistAppendLatency(duration time.Duration) {
	observability.SetPersistAppendLatency(duration)
}

func (metricsStoreBridge) RecordCapturedArchive() {
	observability.RecordCapturedArchive()
}

func (metricsStoreBridge) RecordCapturedPersist(err error, duration time.Duration) {
	observability.RecordCapturedPersist(err, duration)
}

func (metricsStoreBridge) RecordCapturedPersistBatch(persisted, failed uint64, duration time.Duration) {
	observability.RecordCapturedPersistBatch(persisted, failed, duration)
}

func (metricsStoreBridge) RecordBroadcastEnqueue(accepted bool, reason string) {
	observability.RecordBroadcastEnqueue(accepted, reason)
}

func (metricsStoreBridge) RecordBroadcastReceived() {
	observability.RecordBroadcastReceived()
}

func (metricsStoreBridge) RecordBroadcastFlush(events, envelopes, marshalErrors, writeErrors int, duration time.Duration) {
	observability.RecordBroadcastFlush(events, envelopes, marshalErrors, writeErrors, duration)
}

func (metricsStoreBridge) RecordRingbufDecode(zeroCopy bool) {
	observability.RecordRingbufDecode(zeroCopy)
}

func (metricsStoreBridge) RecordKernelRiskDecision(decision string, elapsed time.Duration) {
	observability.RecordKernelRiskDecision(decision, elapsed)
}

func (metricsStoreBridge) RecordKernelRiskFeedback(success bool, err error) {
	observability.RecordKernelRiskFeedback(success, err)
}

func (metricsStoreBridge) Snapshot() CollectorHealthResponse {
	return observability.GetCollectorHealthSnapshot()
}

var collectorMetricsStore metricsStoreBridge

// ── Standalone function bridges ──────────────────────────────────────────

func getGPUMetrics() (map[int32]observability.GpuInfo, []*pb.GPUStatus) {
	return observability.GetGPUMetrics()
}

func readVMFaultCounters() (observability.VmFaultCounters, error) {
	return observability.ReadVMFaultCounters()
}

func getCoreTypes() []pb.CPUInfo_Core_Type {
	return observability.GetCoreTypes()
}

func handlePrometheusMetrics(c *gin.Context) {
	observability.HandlePrometheusMetrics(c)
}

func handleCollectorHealth(c *gin.Context) {
	observability.HandleCollectorHealth(c)
}
