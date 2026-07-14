package events

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/app/wsstream"
	"agent-ebpf-filter/internal/executiongraph"

	"github.com/gin-gonic/gin"
)

// ---- moved from backend/zz_merged_backend.go section graph_execution.go ----

type ExecutionGraphNode = executiongraph.Node
type ExecutionGraphEdge = executiongraph.Edge
type ExecutionGraphResponse = executiongraph.Response
type executionGraphFilters = executiongraph.Filters

func HandleExecutionGraph(c *gin.Context) {
	graph, err := BuildExecutionGraphFromRequest(c)
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

func ServeExecutionGraphWS(c *gin.Context) {
	conn, err := Deps.Upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(wsstream.ControlReadLimit)

	interval := ParseExecutionGraphInterval(c.Query("interval"))
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
		graph, err := BuildExecutionGraphFromRequest(c)
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

func BuildExecutionGraphFromRequest(c *gin.Context) (ExecutionGraphResponse, error) {
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
		if Deps.ReadCapturedEventsContext != nil {
			records, err = Deps.ReadCapturedEventsContext(ctx, replayPath, limit)
		} else {
			records, err = Deps.ReadCapturedEvents(replayPath, limit)
		}
		source = "replay_file"
	} else {
		records, source, err = Deps.RuntimeSettingsRecentEvents(limit)
	}
	if err != nil {
		return ExecutionGraphResponse{}, err
	}
	if err := ctx.Err(); err != nil {
		return ExecutionGraphResponse{}, err
	}

	filters := ExecutionGraphFiltersFromRequest(c)
	graph, err := BuildExecutionGraphContext(ctx, records, filters)
	if err != nil {
		return ExecutionGraphResponse{}, err
	}
	graph.Source = source
	return graph, nil
}

func BuildExecutionGraph(records []CapturedEventRecord, filters executionGraphFilters) ExecutionGraphResponse {
	graph, _ := BuildExecutionGraphContext(context.Background(), records, filters)
	return graph
}

func BuildExecutionGraphContext(ctx context.Context, records []CapturedEventRecord, filters executionGraphFilters) (ExecutionGraphResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	internalRecords := make([]executiongraph.Record, 0, len(records))
	for index, record := range records {
		if index%128 == 0 {
			if err := ctx.Err(); err != nil {
				return ExecutionGraphResponse{}, err
			}
		}
		internalRecords = append(internalRecords, executiongraph.Record{
			Event:      record.Event,
			ReceivedAt: record.ReceivedAt,
		})
	}
	return executiongraph.BuildContext(ctx, internalRecords, filters)
}

func ExecutionGraphFiltersFromRequest(c *gin.Context) executionGraphFilters {
	filters := executionGraphFilters{
		AgentRunID:  strings.TrimSpace(c.Query("agent_run_id")),
		ToolCallID:  strings.TrimSpace(c.Query("tool_call_id")),
		TraceID:     strings.TrimSpace(c.Query("trace_id")),
		Path:        strings.TrimSpace(c.Query("path")),
		Domain:      strings.TrimSpace(c.Query("domain")),
		Comm:        strings.TrimSpace(c.Query("comm")),
		ToolName:    strings.TrimSpace(c.Query("tool_name")),
		Decision:    strings.TrimSpace(c.Query("decision")),
		ProcessTree: ParseExecutionGraphBool(c.Query("process_tree")),
	}
	if rawPID := strings.TrimSpace(c.Query("pid")); rawPID != "" {
		if parsed, err := strconv.ParseUint(rawPID, 10, 32); err == nil {
			pid := uint32(parsed)
			filters.PID = &pid
		}
	}
	if rawRisk := strings.TrimSpace(c.Query("risk_min")); rawRisk != "" {
		if parsed, err := strconv.ParseFloat(rawRisk, 64); err == nil {
			filters.RiskMin = parsed
		}
	}
	if parsed, ok := ParseExecutionGraphTime(c.Query("since")); ok {
		filters.Since = &parsed
	}
	if parsed, ok := ParseExecutionGraphTime(c.Query("until")); ok {
		filters.Until = &parsed
	}
	return filters
}

func ParseExecutionGraphBool(raw string) bool {
	return executiongraph.ParseBool(raw)
}

func ParseExecutionGraphInterval(raw string) time.Duration {
	return executiongraph.ParseInterval(raw)
}

func ParseExecutionGraphTime(raw string) (time.Time, bool) {
	return executiongraph.ParseTime(raw)
}
