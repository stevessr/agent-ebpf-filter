package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/app/recording"
	"agent-ebpf-filter/app/wsstream"
	"agent-ebpf-filter/internal/executiongraph"

	"github.com/gin-gonic/gin"
)

type ExecutionGraphNode = executiongraph.Node
type ExecutionGraphEdge = executiongraph.Edge
type ExecutionGraphResponse = executiongraph.Response
type executionGraphFilters = executiongraph.Filters

// handleExecutionGraph serves GET /events/graph.
func handleExecutionGraph(c *gin.Context) {
	graph, err := buildExecutionGraphFromRequest(c)
	if err != nil {
		if c.Request.Context().Err() != nil || errors.Is(err, context.Canceled) {
			return
		}
		status := http.StatusInternalServerError
		if errors.Is(err, context.DeadlineExceeded) {
			status = http.StatusServiceUnavailable
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, graph)
}

// serveExecutionGraphWS pushes a rebuilt graph on every interval tick.
func serveExecutionGraphWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsstream.ControlReadLimit)

	interval := parseExecutionGraphInterval(c.Query("interval"))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	writeGraph := func() bool {
		graph, err := buildExecutionGraphFromRequest(c)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return false
			}
			_ = wsstream.WriteJSON(conn, gin.H{"error": err.Error()})
			return false
		}
		if err := wsstream.WriteJSON(conn, graph); err != nil {
			return false
		}
		return true
	}

	if !writeGraph() {
		return
	}
	for {
		select {
		case <-done:
			return
		case <-c.Request.Context().Done():
			return
		case <-ticker.C:
			if !writeGraph() {
				return
			}
		}
	}
}

// buildExecutionGraphFromRequest loads the requested event window (recent
// memory/log records, or a replay file) and builds the graph over it.
func buildExecutionGraphFromRequest(c *gin.Context) (executiongraph.Response, error) {
	ctx := c.Request.Context()
	limit := 200
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 2000 {
			limit = parsed
		}
	}

	source := "memory"
	var records []CapturedEventRecord
	var err error
	if replayPath := strings.TrimSpace(c.Query("replay_path")); replayPath != "" {
		records, err = recording.ReadCapturedEventsFileContext(ctx, replayPath, limit)
		source = "replay_file"
	} else {
		records, source, err = runtimeSettingsStore.RecentEventsContext(ctx, limit)
	}
	if err != nil {
		return executiongraph.Response{}, err
	}
	if err := ctx.Err(); err != nil {
		return executiongraph.Response{}, err
	}

	graph, err := buildExecutionGraphContext(ctx, records, executionGraphFiltersFromRequest(c))
	if err != nil {
		return executiongraph.Response{}, err
	}
	graph.Source = source
	return graph, nil
}

func executionGraphFiltersFromRequest(c *gin.Context) executiongraph.Filters {
	return events.ExecutionGraphFiltersFromRequest(c)
}

func buildExecutionGraph(records []events.CapturedEventRecord, filters executiongraph.Filters) executiongraph.Response {
	return events.BuildExecutionGraph(records, filters)
}

func buildExecutionGraphContext(ctx context.Context, records []events.CapturedEventRecord, filters executiongraph.Filters) (executiongraph.Response, error) {
	return events.BuildExecutionGraphContext(ctx, records, filters)
}

func parseExecutionGraphBool(raw string) bool {
	return events.ParseExecutionGraphBool(raw)
}

func parseExecutionGraphInterval(raw string) time.Duration {
	return events.ParseExecutionGraphInterval(raw)
}

func parseExecutionGraphTime(raw string) (time.Time, bool) {
	return events.ParseExecutionGraphTime(raw)
}
