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
	} {
		if !strings.Contains(response.Body.String(), sample) {
			t.Fatalf("metrics omitted %q:\n%s", sample, response.Body.String())
		}
	}
}
