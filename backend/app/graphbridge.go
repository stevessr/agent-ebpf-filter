package app

import (
	"time"

	"agent-ebpf-filter/app/events"
	"agent-ebpf-filter/internal/executiongraph"
	"github.com/gin-gonic/gin"
)

// ── Graph execution wrappers (migrated to app/events/) ─────────────────────

type ExecutionGraphNode = executiongraph.Node
type ExecutionGraphEdge = executiongraph.Edge
type ExecutionGraphResponse = executiongraph.Response
type executionGraphFilters = executiongraph.Filters

func handleExecutionGraph(c *gin.Context) {
	events.HandleExecutionGraph(c)
}

func serveExecutionGraphWS(c *gin.Context) {
	events.ServeExecutionGraphWS(c)
}

func buildExecutionGraphFromRequest(c *gin.Context) (executiongraph.Response, error) {
	return events.BuildExecutionGraphFromRequest(c)
}

func executionGraphFiltersFromRequest(c *gin.Context) executiongraph.Filters {
	return events.ExecutionGraphFiltersFromRequest(c)
}

func buildExecutionGraph(records []events.CapturedEventRecord, filters executiongraph.Filters) executiongraph.Response {
	return events.BuildExecutionGraph(records, filters)
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
