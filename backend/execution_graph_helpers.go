package main

import (
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/internal/executiongraph"
	"github.com/gin-gonic/gin"
)

func executionGraphFiltersFromRequest(c *gin.Context) executionGraphFilters {
	filters := executionGraphFilters{
		AgentRunID:  strings.TrimSpace(c.Query("agent_run_id")),
		ToolCallID:  strings.TrimSpace(c.Query("tool_call_id")),
		TraceID:     strings.TrimSpace(c.Query("trace_id")),
		Path:        strings.TrimSpace(c.Query("path")),
		Domain:      strings.TrimSpace(c.Query("domain")),
		Comm:        strings.TrimSpace(c.Query("comm")),
		ToolName:    strings.TrimSpace(c.Query("tool_name")),
		Decision:    strings.TrimSpace(c.Query("decision")),
		ProcessTree: parseExecutionGraphBool(c.Query("process_tree")),
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
	if parsed, ok := parseExecutionGraphTime(c.Query("since")); ok {
		filters.Since = &parsed
	}
	if parsed, ok := parseExecutionGraphTime(c.Query("until")); ok {
		filters.Until = &parsed
	}
	return filters
}

func parseExecutionGraphBool(raw string) bool {
	return executiongraph.ParseBool(raw)
}

func parseExecutionGraphInterval(raw string) time.Duration {
	return executiongraph.ParseInterval(raw)
}

func parseExecutionGraphTime(raw string) (time.Time, bool) {
	return executiongraph.ParseTime(raw)
}
