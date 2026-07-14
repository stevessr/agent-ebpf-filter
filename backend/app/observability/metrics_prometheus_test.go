package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"agent-ebpf-filter/pb"
)

func TestPrometheusMetricsExposePersistQueueState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	oldStore := collectorMetricsStore
	oldDeps := deps
	collectorMetricsStore = newCollectorMetricsState()
	deps = Deps{
		Broadcast: make(chan *pb.Event, 1),
		PersistQueueStatus: func() PersistQueueStatus {
			return PersistQueueStatus{
				Active:       true,
				QueueLen:     7,
				QueueCap:     4096,
				Pending:      8,
				FailedTotal:  2,
				DroppedTotal: 3,
			}
		},
		SemanticStateStatus: func() SemanticStateStatus {
			return SemanticStateStatus{
				EntriesByKind:                 map[string]int{"secrets": 3, "agentic_loops": 2},
				Entries:                       5,
				MaxEntries:                    24576,
				ExpiredEvictionsTotal:         11,
				CapacityEvictionsTotal:        12,
				TruncatedStateValuesTotal:     13,
				IgnoredOversizedMetadataTotal: 14,
			}
		},
		ToolBaselineStatus: func() ToolBaselineStatus {
			return ToolBaselineStatus{
				Tools:                     4,
				Samples:                   21,
				MaxTools:                  512,
				MaxSamples:                65536,
				MaxSamplesPerTool:         128,
				ObservationsTotal:         22,
				DriftsTotal:               23,
				ExpiredEvictionsTotal:     24,
				CapacityEvictionsTotal:    25,
				TruncatedStateValuesTotal: 26,
			}
		},
	}
	t.Cleanup(func() {
		collectorMetricsStore = oldStore
		deps = oldDeps
	})

	router := gin.New()
	router.GET("/metrics", HandlePrometheusMetrics)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, body = %s", response.Code, response.Body.String())
	}
	for _, sample := range []string{
		"agent_ebpf_persist_writer_active 1",
		"agent_ebpf_persist_queue_length 7",
		"agent_ebpf_persist_queue_capacity 4096",
		"agent_ebpf_persist_pending 8",
		"agent_ebpf_persist_generation_failed 2",
		"agent_ebpf_persist_generation_dropped 3",
		"agent_ebpf_semantic_state_entries{kind=\"agentic_loops\"} 2",
		"agent_ebpf_semantic_state_entries{kind=\"secrets\"} 3",
		"agent_ebpf_semantic_state_max_entries 24576",
		"agent_ebpf_semantic_state_expired_evictions_total 11",
		"agent_ebpf_semantic_state_capacity_evictions_total 12",
		"agent_ebpf_semantic_state_truncated_values_total 13",
		"agent_ebpf_semantic_state_ignored_metadata_total 14",
		"agent_ebpf_tool_baseline_tools 4",
		"agent_ebpf_tool_baseline_samples 21",
		"agent_ebpf_tool_baseline_max_tools 512",
		"agent_ebpf_tool_baseline_max_samples 65536",
		"agent_ebpf_tool_baseline_observations_total 22",
		"agent_ebpf_tool_baseline_drifts_total 23",
		"agent_ebpf_tool_baseline_expired_evictions_total 24",
		"agent_ebpf_tool_baseline_capacity_evictions_total 25",
		"agent_ebpf_tool_baseline_truncated_values_total 26",
	} {
		if !strings.Contains(response.Body.String(), sample) {
			t.Fatalf("metrics omitted %q:\n%s", sample, response.Body.String())
		}
	}
}
