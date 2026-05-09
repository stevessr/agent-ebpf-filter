package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
	"github.com/gin-gonic/gin"
)

type ExecutionGraphNode struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Label     string            `json:"label"`
	Subtitle  string            `json:"subtitle,omitempty"`
	PID       uint32            `json:"pid,omitempty"`
	RiskScore float64           `json:"riskScore,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type ExecutionGraphEdge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Label  string `json:"label,omitempty"`
}

type ExecutionGraphResponse struct {
	EventCount int                  `json:"eventCount"`
	Source     string               `json:"source"`
	NodeCounts map[string]int       `json:"nodeCounts,omitempty"`
	EdgeCounts map[string]int       `json:"edgeCounts,omitempty"`
	Nodes      []ExecutionGraphNode `json:"nodes"`
	Edges      []ExecutionGraphEdge `json:"edges"`
}

type executionGraphFilters struct {
	AgentRunID  string
	ToolCallID  string
	TraceID     string
	Path        string
	Domain      string
	Comm        string
	ToolName    string
	Decision    string
	PID         *uint32
	ProcessTree bool
	RiskMin     float64
	Since       *time.Time
	Until       *time.Time
}

type graphRelation struct {
	Node ExecutionGraphNode
	Kind string
}

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

	records, source, err := runtimeSettingsStore.RecentEvents(limit)
	if err != nil {
		return ExecutionGraphResponse{}, err
	}

	filters := executionGraphFiltersFromRequest(c)
	graph := buildExecutionGraph(records, filters)
	graph.Source = source
	return graph, nil
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

func buildExecutionGraph(records []CapturedEventRecord, filters executionGraphFilters) ExecutionGraphResponse {
	nodes := make(map[string]ExecutionGraphNode)
	edges := make(map[string]ExecutionGraphEdge)
	matchedEvents := 0
	pidTree := buildExecutionGraphPIDTree(records, filters)

	addNode := func(node ExecutionGraphNode) {
		if node.ID == "" {
			return
		}
		if existing, ok := nodes[node.ID]; ok {
			if node.RiskScore > existing.RiskScore {
				existing.RiskScore = node.RiskScore
			}
			if existing.Subtitle == "" && node.Subtitle != "" {
				existing.Subtitle = node.Subtitle
			}
			if (existing.Label == "" || isGenericProcessLabel(existing)) && node.Label != "" && !isGenericProcessLabel(node) {
				existing.Label = node.Label
			}
			if existing.PID == 0 && node.PID != 0 {
				existing.PID = node.PID
			}
			if len(node.Metadata) > 0 {
				if existing.Metadata == nil {
					existing.Metadata = make(map[string]string, len(node.Metadata))
				}
				for key, value := range node.Metadata {
					if strings.TrimSpace(value) == "" {
						continue
					}
					if _, exists := existing.Metadata[key]; !exists {
						existing.Metadata[key] = value
					}
				}
			}
			nodes[node.ID] = existing
			return
		}
		nodes[node.ID] = node
	}
	addEdge := func(edge ExecutionGraphEdge) {
		if edge.ID == "" || edge.Source == "" || edge.Target == "" {
			return
		}
		edges[edge.ID] = edge
	}

	for index, record := range records {
		event := record.Event
		if !matchesExecutionGraphFilters(record, event, filters, pidTree) {
			continue
		}
		matchedEvents++

		processNode := buildProcessGraphNode(event)
		addNode(processNode)
		if event.GetPpid() > 0 && event.GetPpid() != event.GetPid() {
			parentID := processNodeID(event.GetPpid())
			addNode(ExecutionGraphNode{
				ID:       parentID,
				Kind:     "process",
				Label:    fmt.Sprintf("pid %d", event.GetPpid()),
				Subtitle: "parent process",
				PID:      event.GetPpid(),
				Metadata: map[string]string{
					"pid": strconv.FormatUint(uint64(event.GetPpid()), 10),
				},
			})
			addEdge(ExecutionGraphEdge{ID: parentID + "->" + processNode.ID + ":parent_process", Source: parentID, Target: processNode.ID, Kind: "parent_process", Label: "parent process"})
		}

		activityNode := buildExecutionGraphActivityNode(record, event, index)
		addNode(activityNode)
		processToActivityKind := graphActivityEdgeKind(event)
		addEdge(ExecutionGraphEdge{
			ID:     processNode.ID + "->" + activityNode.ID + ":" + processToActivityKind,
			Source: processNode.ID,
			Target: activityNode.ID,
			Kind:   processToActivityKind,
			Label:  processToActivityKind,
		})

		if event.GetAgentRunId() != "" {
			runID := "run:" + event.GetAgentRunId()
			addNode(ExecutionGraphNode{
				ID:        runID,
				Kind:      "agent_run",
				Label:     event.GetAgentRunId(),
				Subtitle:  buildRunSubtitle(event),
				RiskScore: event.GetRiskScore(),
				Metadata: map[string]string{
					"agentRunId":     event.GetAgentRunId(),
					"conversationId": event.GetConversationId(),
					"turnId":         event.GetTurnId(),
					"traceId":        event.GetTraceId(),
				},
			})
			addEdge(ExecutionGraphEdge{ID: runID + "->" + processNode.ID + ":contains", Source: runID, Target: processNode.ID, Kind: "contains", Label: "contains"})
			if event.GetToolCallId() != "" {
				toolID := "tool:" + event.GetToolCallId()
				addNode(ExecutionGraphNode{
					ID:        toolID,
					Kind:      "tool_call",
					Label:     event.GetToolCallId(),
					Subtitle:  event.GetToolName(),
					RiskScore: event.GetRiskScore(),
					Metadata: map[string]string{
						"toolCallId": event.GetToolCallId(),
						"toolName":   event.GetToolName(),
						"traceId":    event.GetTraceId(),
						"agentRunId": event.GetAgentRunId(),
					},
				})
				addEdge(ExecutionGraphEdge{ID: runID + "->" + toolID + ":contains", Source: runID, Target: toolID, Kind: "contains", Label: "contains"})
				addEdge(ExecutionGraphEdge{ID: toolID + "->" + processNode.ID + ":owns", Source: toolID, Target: processNode.ID, Kind: "owns", Label: "owns"})
			}
		}

		if event.GetDecision() != "" {
			decisionNode, decisionEdgeKind := buildExecutionDecisionNode(record, event, index)
			addNode(decisionNode)
			addEdge(ExecutionGraphEdge{
				ID:     activityNode.ID + "->" + decisionNode.ID + ":" + decisionEdgeKind,
				Source: activityNode.ID,
				Target: decisionNode.ID,
				Kind:   decisionEdgeKind,
				Label:  decisionEdgeKind,
			})
		}

		switch event.GetType() {
		case "process_exec":
			if oldPID, ok := extractGraphInt(event.GetExtraInfo(), "old_pid"); ok && oldPID > 0 && uint32(oldPID) != event.GetPid() {
				oldNode := ExecutionGraphNode{
					ID:       processNodeID(uint32(oldPID)),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", oldPID),
					Subtitle: "pre-exec pid",
					PID:      uint32(oldPID),
					Metadata: map[string]string{"pid": strconv.Itoa(oldPID)},
				}
				addNode(oldNode)
				addEdge(ExecutionGraphEdge{ID: oldNode.ID + "->" + processNode.ID + ":exec_chain", Source: oldNode.ID, Target: processNode.ID, Kind: "exec_chain", Label: "exec"})
			}
		case "process_fork", "clone":
			if childPID, ok := extractGraphInt(event.GetExtraInfo(), "child_pid"); ok && childPID > 0 {
				childNode := ExecutionGraphNode{
					ID:       processNodeID(uint32(childPID)),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", childPID),
					Subtitle: "child process",
					PID:      uint32(childPID),
					Metadata: map[string]string{"pid": strconv.Itoa(childPID)},
				}
				addNode(childNode)
				addEdge(ExecutionGraphEdge{ID: processNode.ID + "->" + childNode.ID + ":child_process", Source: processNode.ID, Target: childNode.ID, Kind: "child_process", Label: "child process"})
				addEdge(ExecutionGraphEdge{ID: activityNode.ID + "->" + childNode.ID + ":spawned", Source: activityNode.ID, Target: childNode.ID, Kind: "spawned", Label: "spawned"})
			}
		case "wait4":
			if targetPID, ok := extractGraphInt(event.GetExtraInfo(), "target_pid"); ok && targetPID > 0 {
				targetNode := ExecutionGraphNode{
					ID:       processNodeID(uint32(targetPID)),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", targetPID),
					Subtitle: "wait target",
					PID:      uint32(targetPID),
					Metadata: map[string]string{"pid": strconv.Itoa(targetPID)},
				}
				addNode(targetNode)
				addEdge(ExecutionGraphEdge{ID: activityNode.ID + "->" + targetNode.ID + ":waited", Source: activityNode.ID, Target: targetNode.ID, Kind: "waited", Label: "waited"})
			}
		case "process_exit", "exit":
			exitID := processNode.ID + ":exit:" + strconv.FormatInt(record.ReceivedAt.UnixNano(), 10)
			status := strings.TrimSpace(event.GetExtraInfo())
			if status == "" {
				status = "exit status"
			}
			addNode(ExecutionGraphNode{
				ID:       exitID,
				Kind:     "exit_status",
				Label:    status,
				Metadata: map[string]string{"status": status},
			})
			addEdge(ExecutionGraphEdge{ID: activityNode.ID + "->" + exitID + ":exited", Source: activityNode.ID, Target: exitID, Kind: "exited", Label: "exited"})
		case "semantic_alert":
			alertID := processNode.ID + ":alert:" + sanitizeGraphID(event.GetComm()+":"+event.GetPath()+":"+event.GetExtraInfo())
			addNode(ExecutionGraphNode{
				ID:        alertID,
				Kind:      "policy_alert",
				Label:     event.GetComm(),
				Subtitle:  event.GetExtraInfo(),
				RiskScore: event.GetRiskScore(),
				Metadata: map[string]string{
					"decision": event.GetDecision(),
					"path":     event.GetPath(),
				},
			})
			addEdge(ExecutionGraphEdge{ID: activityNode.ID + "->" + alertID + ":alerted", Source: activityNode.ID, Target: alertID, Kind: "alerted", Label: "alerted"})
		}

		for _, relation := range graphFileRelations(event) {
			addNode(relation.Node)
			addEdge(ExecutionGraphEdge{ID: activityNode.ID + "->" + relation.Node.ID + ":" + relation.Kind, Source: activityNode.ID, Target: relation.Node.ID, Kind: relation.Kind, Label: relation.Kind})
		}
		for _, relation := range graphNetworkRelations(event) {
			addNode(relation.Node)
			addEdge(ExecutionGraphEdge{ID: activityNode.ID + "->" + relation.Node.ID + ":" + relation.Kind, Source: activityNode.ID, Target: relation.Node.ID, Kind: relation.Kind, Label: relation.Kind})
		}
	}

	nodeList := make([]ExecutionGraphNode, 0, len(nodes))
	nodeCounts := make(map[string]int)
	for _, node := range nodes {
		nodeList = append(nodeList, node)
		nodeCounts[node.Kind]++
	}
	sort.Slice(nodeList, func(i, j int) bool {
		if nodeList[i].Kind == nodeList[j].Kind {
			return nodeList[i].Label < nodeList[j].Label
		}
		return nodeList[i].Kind < nodeList[j].Kind
	})

	edgeList := make([]ExecutionGraphEdge, 0, len(edges))
	edgeCounts := make(map[string]int)
	for _, edge := range edges {
		edgeList = append(edgeList, edge)
		edgeCounts[edge.Kind]++
	}
	sort.Slice(edgeList, func(i, j int) bool { return edgeList[i].ID < edgeList[j].ID })

	return ExecutionGraphResponse{
		EventCount: matchedEvents,
		NodeCounts: nodeCounts,
		EdgeCounts: edgeCounts,
		Nodes:      nodeList,
		Edges:      edgeList,
	}
}

func matchesExecutionGraphFilters(record CapturedEventRecord, event *pb.Event, filters executionGraphFilters, pidTree map[uint32]struct{}) bool {
	if event == nil {
		return false
	}
	if filters.AgentRunID != "" && event.GetAgentRunId() != filters.AgentRunID {
		return false
	}
	if filters.ToolCallID != "" && event.GetToolCallId() != filters.ToolCallID {
		return false
	}
	if filters.TraceID != "" && event.GetTraceId() != filters.TraceID {
		return false
	}
	if filters.ToolName != "" && !strings.Contains(strings.ToLower(event.GetToolName()), strings.ToLower(filters.ToolName)) {
		return false
	}
	if filters.Decision != "" && !strings.EqualFold(event.GetDecision(), filters.Decision) {
		return false
	}
	if filters.Comm != "" && !strings.Contains(strings.ToLower(event.GetComm()), strings.ToLower(filters.Comm)) {
		return false
	}
	if filters.PID != nil {
		if filters.ProcessTree {
			if !eventMatchesExecutionGraphPIDTree(event, pidTree) {
				return false
			}
		} else if event.GetPid() != *filters.PID && event.GetPpid() != *filters.PID {
			return false
		}
	}
	if filters.RiskMin > 0 && event.GetRiskScore() < filters.RiskMin {
		return false
	}
	if filters.Since != nil && record.ReceivedAt.Before(*filters.Since) {
		return false
	}
	if filters.Until != nil && record.ReceivedAt.After(*filters.Until) {
		return false
	}
	if filters.Path != "" {
		needle := strings.ToLower(filters.Path)
		if !strings.Contains(strings.ToLower(event.GetPath()), needle) && !strings.Contains(strings.ToLower(event.GetExtraPath()), needle) {
			return false
		}
	}
	if filters.Domain != "" {
		needle := strings.ToLower(filters.Domain)
		if !strings.Contains(strings.ToLower(event.GetDomain()), needle) && !strings.Contains(strings.ToLower(event.GetNetEndpoint()), needle) {
			return false
		}
	}
	return true
}

func buildExecutionGraphPIDTree(records []CapturedEventRecord, filters executionGraphFilters) map[uint32]struct{} {
	if filters.PID == nil || !filters.ProcessTree {
		return nil
	}
	seed := *filters.PID
	tree := map[uint32]struct{}{seed: {}}
	changed := true
	for changed {
		changed = false
		for _, record := range records {
			event := record.Event
			if event == nil || !matchesExecutionGraphNonPIDFilters(record, event, filters) {
				continue
			}
			pid := event.GetPid()
			ppid := event.GetPpid()
			if _, ok := tree[ppid]; ok && pid != 0 {
				if _, exists := tree[pid]; !exists {
					tree[pid] = struct{}{}
					changed = true
				}
			}
			if childPID, ok := extractGraphInt(event.GetExtraInfo(), "child_pid"); ok && childPID > 0 {
				if _, ok := tree[pid]; ok {
					child := uint32(childPID)
					if _, exists := tree[child]; !exists {
						tree[child] = struct{}{}
						changed = true
					}
				}
			}
		}
	}
	return tree
}

func matchesExecutionGraphNonPIDFilters(record CapturedEventRecord, event *pb.Event, filters executionGraphFilters) bool {
	filters.PID = nil
	filters.ProcessTree = false
	return matchesExecutionGraphFilters(record, event, filters, nil)
}

func eventMatchesExecutionGraphPIDTree(event *pb.Event, pidTree map[uint32]struct{}) bool {
	if len(pidTree) == 0 {
		return false
	}
	if _, ok := pidTree[event.GetPid()]; ok {
		return true
	}
	if _, ok := pidTree[event.GetPpid()]; ok {
		return true
	}
	if childPID, ok := extractGraphInt(event.GetExtraInfo(), "child_pid"); ok && childPID > 0 {
		_, ok := pidTree[uint32(childPID)]
		return ok
	}
	return false
}

func parseExecutionGraphBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func parseExecutionGraphInterval(raw string) time.Duration {
	value := strings.TrimSpace(raw)
	if value == "" {
		return 1500 * time.Millisecond
	}
	if millis, err := strconv.Atoi(value); err == nil {
		d := time.Duration(millis) * time.Millisecond
		if d < 500*time.Millisecond {
			return 500 * time.Millisecond
		}
		if d > 30*time.Second {
			return 30 * time.Second
		}
		return d
	}
	if d, err := time.ParseDuration(value); err == nil {
		if d < 500*time.Millisecond {
			return 500 * time.Millisecond
		}
		if d > 30*time.Second {
			return 30 * time.Second
		}
		return d
	}
	return 1500 * time.Millisecond
}

func parseExecutionGraphTime(raw string) (time.Time, bool) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, false
	}
	if unixMillis, err := strconv.ParseInt(value, 10, 64); err == nil {
		switch len(value) {
		case 10:
			return time.Unix(unixMillis, 0).UTC(), true
		case 13:
			return time.UnixMilli(unixMillis).UTC(), true
		case 16:
			return time.UnixMicro(unixMillis).UTC(), true
		case 19:
			return time.Unix(0, unixMillis).UTC(), true
		}
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func buildProcessGraphNode(event *pb.Event) ExecutionGraphNode {
	label := strings.TrimSpace(event.GetComm())
	if label == "" {
		label = fmt.Sprintf("pid %d", event.GetPid())
	}
	return ExecutionGraphNode{
		ID:        processNodeID(event.GetPid()),
		Kind:      "process",
		Label:     label,
		Subtitle:  "pid=" + strconv.FormatUint(uint64(event.GetPid()), 10),
		PID:       event.GetPid(),
		RiskScore: event.GetRiskScore(),
		Metadata: map[string]string{
			"pid":          strconv.FormatUint(uint64(event.GetPid()), 10),
			"ppid":         strconv.FormatUint(uint64(event.GetPpid()), 10),
			"uid":          strconv.FormatUint(uint64(event.GetUid()), 10),
			"gid":          strconv.FormatUint(uint64(event.GetGid()), 10),
			"comm":         event.GetComm(),
			"type":         event.GetType(),
			"path":         event.GetPath(),
			"decision":     event.GetDecision(),
			"agentRunId":   event.GetAgentRunId(),
			"toolCallId":   event.GetToolCallId(),
			"toolName":     event.GetToolName(),
			"traceId":      event.GetTraceId(),
			"spanId":       event.GetSpanId(),
			"rootAgentPid": strconv.FormatUint(uint64(event.GetRootAgentPid()), 10),
			"cgroupId":     strconv.FormatUint(event.GetCgroupId(), 10),
			"containerId":  event.GetContainerId(),
			"argvDigest":   event.GetArgvDigest(),
		},
	}
}

func buildExecutionGraphActivityNode(record CapturedEventRecord, event *pb.Event, index int) ExecutionGraphNode {
	id := fmt.Sprintf("evt:%d:%d:%s", record.ReceivedAt.UnixNano(), index, sanitizeGraphID(event.GetType()))
	label := strings.TrimSpace(event.GetType())
	if label == "" {
		label = "event"
	}
	return ExecutionGraphNode{
		ID:        id,
		Kind:      graphEventNodeKind(event),
		Label:     label,
		Subtitle:  buildGraphEventSubtitle(event),
		RiskScore: event.GetRiskScore(),
		Metadata: map[string]string{
			"type":          event.GetType(),
			"receivedAt":    record.ReceivedAt.UTC().Format(time.RFC3339Nano),
			"path":          event.GetPath(),
			"extraPath":     event.GetExtraPath(),
			"netEndpoint":   event.GetNetEndpoint(),
			"netDirection":  event.GetNetDirection(),
			"domain":        event.GetDomain(),
			"decision":      event.GetDecision(),
			"extraInfo":     event.GetExtraInfo(),
			"agentRunId":    event.GetAgentRunId(),
			"toolCallId":    event.GetToolCallId(),
			"toolName":      event.GetToolName(),
			"traceId":       event.GetTraceId(),
			"spanId":        event.GetSpanId(),
			"riskScore":     strconv.FormatFloat(event.GetRiskScore(), 'f', 2, 64),
			"durationNs":    strconv.FormatUint(event.GetDurationNs(), 10),
			"schemaVersion": event.GetSchemaVersion(),
		},
	}
}

func buildExecutionDecisionNode(record CapturedEventRecord, event *pb.Event, index int) (ExecutionGraphNode, string) {
	decisionKind := graphDecisionEdgeKind(event.GetDecision())
	id := fmt.Sprintf("decision:%d:%d:%s", record.ReceivedAt.UnixNano(), index, sanitizeGraphID(event.GetDecision()))
	return ExecutionGraphNode{
		ID:        id,
		Kind:      "policy_decision",
		Label:     strings.ToUpper(strings.TrimSpace(event.GetDecision())),
		Subtitle:  strings.TrimSpace(event.GetToolName()),
		RiskScore: event.GetRiskScore(),
		Metadata: map[string]string{
			"decision":   strings.ToUpper(strings.TrimSpace(event.GetDecision())),
			"toolName":   event.GetToolName(),
			"toolCallId": event.GetToolCallId(),
			"agentRunId": event.GetAgentRunId(),
			"traceId":    event.GetTraceId(),
			"extraInfo":  event.GetExtraInfo(),
		},
	}, decisionKind
}

func buildRunSubtitle(event *pb.Event) string {
	parts := make([]string, 0, 2)
	if conversationID := strings.TrimSpace(event.GetConversationId()); conversationID != "" {
		parts = append(parts, conversationID)
	}
	if turnID := strings.TrimSpace(event.GetTurnId()); turnID != "" {
		parts = append(parts, "turn="+turnID)
	}
	return strings.Join(parts, " • ")
}

func buildGraphEventSubtitle(event *pb.Event) string {
	for _, candidate := range []string{
		strings.TrimSpace(event.GetPath()),
		strings.TrimSpace(event.GetNetEndpoint()),
		strings.TrimSpace(event.GetDecision()),
		strings.TrimSpace(event.GetExtraInfo()),
	} {
		if candidate != "" {
			return candidate
		}
	}
	if event.GetDurationNs() > 0 {
		return fmt.Sprintf("%d ns", event.GetDurationNs())
	}
	return ""
}

func graphEventNodeKind(event *pb.Event) string {
	switch event.GetType() {
	case "wrapper_intercept":
		return "wrapper_event"
	case "native_hook":
		return "hook_event"
	case "semantic_alert":
		return "policy_alert"
	default:
		return "syscall"
	}
}

func graphActivityEdgeKind(event *pb.Event) string {
	switch event.GetType() {
	case "process_fork", "clone":
		return "spawned"
	case "execve", "process_exec":
		return "execed"
	case "wait4":
		return "waited"
	case "process_exit", "exit":
		return "exited"
	case "semantic_alert":
		return "alerted"
	case "wrapper_intercept", "native_hook":
		return "reviewed"
	default:
		return "observed"
	}
}

func graphDecisionEdgeKind(decision string) string {
	switch strings.ToUpper(strings.TrimSpace(decision)) {
	case "BLOCK":
		return "blocked"
	case "REWRITE":
		return "rewritten"
	case "ALERT":
		return "alerted"
	case "ALLOW":
		return "allowed"
	default:
		return "decided"
	}
}

func processNodeID(pid uint32) string {
	return "proc:" + strconv.FormatUint(uint64(pid), 10)
}

func isGenericProcessLabel(node ExecutionGraphNode) bool {
	return node.Kind == "process" && strings.HasPrefix(strings.TrimSpace(node.Label), "pid ")
}

func graphFileRelations(event *pb.Event) []graphRelation {
	relations := make([]graphRelation, 0, 2)
	appendPath := func(path, kind string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		relations = append(relations, graphRelation{
			Node: ExecutionGraphNode{
				ID:       "file:" + path,
				Kind:     "file",
				Label:    path,
				Metadata: map[string]string{"path": path},
			},
			Kind: kind,
		})
	}

	switch event.GetType() {
	case "execve":
		appendPath(event.GetPath(), "execed")
	case "openat", "open":
		appendPath(event.GetPath(), "opened")
	case "read":
		appendPath(event.GetPath(), "read")
	case "write", "chmod", "chown", "mkdir", "mknod", "link", "symlink":
		appendPath(event.GetPath(), "wrote")
	case "rename":
		appendPath(event.GetPath(), "wrote")
		if extraPath, ok := extractGraphString(event.GetExtraInfo(), "newpath"); ok {
			appendPath(extraPath, "rewritten")
		}
	case "unlink", "unlinkat":
		appendPath(event.GetPath(), "deleted")
	}
	return relations
}

func graphNetworkRelations(event *pb.Event) []graphRelation {
	endpoint := strings.TrimSpace(event.GetNetEndpoint())
	if endpoint == "" {
		endpoint = strings.TrimSpace(event.GetPath())
	}
	if endpoint == "" {
		return nil
	}

	var edgeKind string
	switch event.GetType() {
	case "network_connect", "network_bind", "socket", "accept", "accept4":
		edgeKind = "connected"
	case "network_sendto":
		edgeKind = "wrote"
	case "network_recvfrom":
		edgeKind = "read"
	default:
		return nil
	}

	metadata := map[string]string{
		"endpoint": endpoint,
		"domain":   event.GetDomain(),
		"family":   event.GetNetFamily(),
	}
	return []graphRelation{{
		Node: ExecutionGraphNode{
			ID:       "net:" + endpoint,
			Kind:     "network",
			Label:    endpoint,
			Subtitle: event.GetDomain(),
			Metadata: metadata,
		},
		Kind: edgeKind,
	}}
}

func extractGraphInt(extraInfo, key string) (int, bool) {
	pattern := key + "="
	for _, field := range strings.Fields(extraInfo) {
		if !strings.HasPrefix(field, pattern) {
			continue
		}
		value := strings.TrimPrefix(field, pattern)
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func extractGraphString(extraInfo, key string) (string, bool) {
	pattern := key + "="
	for _, field := range strings.Fields(extraInfo) {
		if !strings.HasPrefix(field, pattern) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(field, pattern))
		if value != "" {
			return value, true
		}
	}
	return "", false
}

func sanitizeGraphID(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(
		"/", "_",
		" ", "_",
		":", "_",
		"|", "_",
		"\\", "_",
		"=", "_",
		"?", "_",
		"&", "_",
	)
	return replacer.Replace(value)
}
