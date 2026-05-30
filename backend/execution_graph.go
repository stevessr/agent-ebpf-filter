package main

import (
	"net/http"
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

func handleExecutionGraph(c *gin.Context) {
	graph, err := buildExecutionGraphFromRequest(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, graph)
}

func serveExecutionGraphWS(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	defer conn.Close()

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
			_ = conn.WriteJSON(gin.H{"error": err.Error()})
			return false
		}
		if err := conn.WriteJSON(graph); err != nil {
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
		case <-ticker.C:
			if !writeGraph() {
				return
			}
		}
	}
}

func buildExecutionGraphFromRequest(c *gin.Context) (ExecutionGraphResponse, error) {
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
		records, err = readCapturedEventsFile(replayPath, limit)
		source = "replay_file"
	} else {
		records, source, err = runtimeSettingsStore.RecentEvents(limit)
	}
	if err != nil {
		return ExecutionGraphResponse{}, err
	}

	filters := executionGraphFiltersFromRequest(c)
	graph := buildExecutionGraph(records, filters)
	graph.Source = source
	return graph, nil
}

func buildExecutionGraph(records []CapturedEventRecord, filters executionGraphFilters) ExecutionGraphResponse {
	internalRecords := make([]executiongraph.Record, 0, len(records))
	for _, record := range records {
		internalRecords = append(internalRecords, executiongraph.Record{
			Event:      record.Event,
			ReceivedAt: record.ReceivedAt,
		})
	}
	return executiongraph.Build(internalRecords, filters)
}

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
