package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func agentSightQueryFor(t *testing.T, method, target, body string) AgentSightEventQuery {
	t.Helper()
	gin.SetMode(gin.TestMode)
	var query AgentSightEventQuery
	var parseErr error
	router := gin.New()
	handle := func(c *gin.Context) {
		if method == http.MethodGet {
			query = agentSightQueryFromRequest(c)
			return
		}
		query, parseErr = agentSightQueryFromJSONRequest(c)
	}
	router.GET("/events", handle)
	router.POST("/events", handle)
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	router.ServeHTTP(httptest.NewRecorder(), req)
	if parseErr != nil {
		t.Fatalf("query parse error: %v", parseErr)
	}
	return query
}

// Query-parameter filters used to be dropped on the AgentSight path because
// the filter value was carried as an untyped struct that the matcher could
// not read; both transports must now narrow the result set.
func TestAgentSightQueryFiltersApplyFromQueryParamsAndBody(t *testing.T) {
	events := []AgentSightExportEvent{
		{Timestamp: 1_000, Source: "ebpf_ringbuf", PID: 42, Comm: "node", TraceID: "t1", Data: map[string]any{"type": "openat"}},
		{Timestamp: 2_000, Source: "wrapper", PID: 43, Comm: "bash", TraceID: "t2", Data: map[string]any{"type": "wrapper_intercept"}},
	}
	matches := func(query AgentSightEventQuery) []uint32 {
		var pids []uint32
		for _, event := range events {
			if agentSightExportEventMatches(event, query) {
				pids = append(pids, event.PID)
			}
		}
		return pids
	}

	get := agentSightQueryFor(t, http.MethodGet, "/events?pid=42&comm=NOD&type=openat&trace_id=t1", "")
	if got := matches(get); len(got) != 1 || got[0] != 42 {
		t.Fatalf("GET filters matched %v, want [42]", got)
	}
	if none := agentSightQueryFor(t, http.MethodGet, "/events?comm=python", ""); len(matches(none)) != 0 {
		t.Fatal("GET comm filter did not exclude non-matching events")
	}

	post := agentSightQueryFor(t, http.MethodPost, "/events?pid=42", `{"comm":"bash","pid":43,"since":1500}`)
	if post.Filters.PID != 43 || post.Filters.Comm != "bash" || post.Filters.Since.IsZero() {
		t.Fatalf("body filters not applied: %+v", post.Filters)
	}
	if got := matches(post); len(got) != 1 || got[0] != 43 {
		t.Fatalf("POST filters matched %v, want [43]", got)
	}

	// Body fields that are absent keep the query-parameter value.
	merged := agentSightQueryFor(t, http.MethodPost, "/events?trace_id=t2", `{"source":"wrapper"}`)
	if merged.Filters.TraceID != "t2" || merged.Filters.Source != "wrapper" {
		t.Fatalf("query/body merge = %+v", merged.Filters)
	}
	if until := agentSightQueryFor(t, http.MethodGet, "/events?until="+time.UnixMilli(1_500).UTC().Format(time.RFC3339Nano), ""); len(matches(until)) != 1 {
		t.Fatalf("until filter matched %v, want one event", matches(until))
	}
}
