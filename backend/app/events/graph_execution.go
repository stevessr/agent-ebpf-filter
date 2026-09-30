package events

import (
	"context"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/internal/executiongraph"

	"github.com/gin-gonic/gin"
)

type ExecutionGraphNode = executiongraph.Node
type ExecutionGraphEdge = executiongraph.Edge
type ExecutionGraphResponse = executiongraph.Response
type executionGraphFilters = executiongraph.Filters

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
