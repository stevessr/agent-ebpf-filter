package executiongraph

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Node struct {
	ID        string            `json:"id"`
	Kind      string            `json:"kind"`
	Label     string            `json:"label"`
	Subtitle  string            `json:"subtitle,omitempty"`
	PID       uint32            `json:"pid,omitempty"`
	RiskScore float64           `json:"riskScore,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Label  string `json:"label,omitempty"`
}

type Response struct {
	EventCount          int            `json:"eventCount"`
	Source              string         `json:"source"`
	NodeCounts          map[string]int `json:"nodeCounts,omitempty"`
	EdgeCounts          map[string]int `json:"edgeCounts,omitempty"`
	Nodes               []Node         `json:"nodes"`
	Edges               []Edge         `json:"edges"`
	Truncated           bool           `json:"truncated"`
	OmittedEventCount   int            `json:"omittedEventCount"`
	OmittedNodeCount    int            `json:"omittedNodeCount"`
	OmittedEdgeCount    int            `json:"omittedEdgeCount"`
	TruncatedFieldCount int            `json:"truncatedFieldCount"`
}

type Filters struct {
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
	Node Node
	Kind string
}

func Build(records []Record, filters Filters) Response {
	graph, _ := BuildContext(context.Background(), records, filters)
	return graph
}

func BuildContext(ctx context.Context, records []Record, filters Filters) (Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	builder := newGraphBuilder()
	if len(records) > graphMaxInputRecords {
		builder.omittedEvents = len(records) - graphMaxInputRecords
		records = records[len(records)-graphMaxInputRecords:]
	}
	filters = prepareExecutionGraphFilters(filters)
	matchedEvents := 0
	pidTree, err := buildExecutionGraphPIDTreeContext(ctx, records, filters)
	if err != nil {
		return Response{}, err
	}

	for index, record := range records {
		if index%64 == 0 {
			if err := ctx.Err(); err != nil {
				return Response{}, err
			}
		}
		event := record.Event
		if !matchesExecutionGraphFilters(record, event, filters, pidTree) {
			continue
		}
		matchedEvents++

		processNode := builder.addNode(buildProcessGraphNode(event))
		if event.GetPpid() > 0 && event.GetPpid() != event.GetPid() {
			parentID := processNodeID(event.GetPpid())
			parentNode := builder.addNode(Node{
				ID:       parentID,
				Kind:     "process",
				Label:    fmt.Sprintf("pid %d", event.GetPpid()),
				Subtitle: "parent process",
				PID:      event.GetPpid(),
				Metadata: map[string]string{
					"pid": strconv.FormatUint(uint64(event.GetPpid()), 10),
				},
			})
			builder.addEdge(Edge{ID: parentNode.ID + "->" + processNode.ID + ":parent_process", Source: parentNode.ID, Target: processNode.ID, Kind: "parent_process", Label: "parent process"})
		}

		activityNode := builder.addNode(buildExecutionGraphActivityNode(record, event, index))
		processToActivityKind := graphActivityEdgeKind(event)
		builder.addEdge(Edge{
			ID:     processNode.ID + "->" + activityNode.ID + ":" + processToActivityKind,
			Source: processNode.ID,
			Target: activityNode.ID,
			Kind:   processToActivityKind,
			Label:  processToActivityKind,
		})

		if event.GetAgentRunId() != "" {
			runNode := builder.addNode(Node{
				ID:        graphEntityNodeID("run", event.GetAgentRunId()),
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
			builder.addEdge(Edge{ID: runNode.ID + "->" + processNode.ID + ":contains", Source: runNode.ID, Target: processNode.ID, Kind: "contains", Label: "contains"})
			if event.GetToolCallId() != "" {
				toolNode := builder.addNode(Node{
					ID:        graphEntityNodeID("tool", event.GetToolCallId()),
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
				builder.addEdge(Edge{ID: runNode.ID + "->" + toolNode.ID + ":contains", Source: runNode.ID, Target: toolNode.ID, Kind: "contains", Label: "contains"})
				builder.addEdge(Edge{ID: toolNode.ID + "->" + processNode.ID + ":owns", Source: toolNode.ID, Target: processNode.ID, Kind: "owns", Label: "owns"})
			}
		}

		if event.GetDecision() != "" {
			decisionNode, decisionEdgeKind := buildExecutionDecisionNode(record, event, index)
			decisionNode = builder.addNode(decisionNode)
			builder.addEdge(Edge{
				ID:     activityNode.ID + "->" + decisionNode.ID + ":" + decisionEdgeKind,
				Source: activityNode.ID,
				Target: decisionNode.ID,
				Kind:   decisionEdgeKind,
				Label:  decisionEdgeKind,
			})
		}

		switch event.GetType() {
		case "process_exec":
			if oldPID, ok := extractGraphPID(event.GetExtraInfo(), "old_pid"); ok && oldPID != event.GetPid() {
				oldNode := builder.addNode(Node{
					ID:       processNodeID(oldPID),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", oldPID),
					Subtitle: "pre-exec pid",
					PID:      oldPID,
					Metadata: map[string]string{"pid": strconv.FormatUint(uint64(oldPID), 10)},
				})
				builder.addEdge(Edge{ID: oldNode.ID + "->" + processNode.ID + ":exec_chain", Source: oldNode.ID, Target: processNode.ID, Kind: "exec_chain", Label: "exec"})
			}
		case "process_fork", "clone":
			if childPID, ok := extractGraphPID(event.GetExtraInfo(), "child_pid"); ok {
				childNode := builder.addNode(Node{
					ID:       processNodeID(childPID),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", childPID),
					Subtitle: "child process",
					PID:      childPID,
					Metadata: map[string]string{"pid": strconv.FormatUint(uint64(childPID), 10)},
				})
				builder.addEdge(Edge{ID: processNode.ID + "->" + childNode.ID + ":child_process", Source: processNode.ID, Target: childNode.ID, Kind: "child_process", Label: "child process"})
				builder.addEdge(Edge{ID: activityNode.ID + "->" + childNode.ID + ":spawned", Source: activityNode.ID, Target: childNode.ID, Kind: "spawned", Label: "spawned"})
			}
		case "wait4":
			if targetPID, ok := extractGraphPID(event.GetExtraInfo(), "target_pid"); ok {
				targetNode := builder.addNode(Node{
					ID:       processNodeID(targetPID),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", targetPID),
					Subtitle: "wait target",
					PID:      targetPID,
					Metadata: map[string]string{"pid": strconv.FormatUint(uint64(targetPID), 10)},
				})
				builder.addEdge(Edge{ID: activityNode.ID + "->" + targetNode.ID + ":waited", Source: activityNode.ID, Target: targetNode.ID, Kind: "waited", Label: "waited"})
			}
		case "process_exit", "exit":
			exitID := processNode.ID + ":exit:" + strconv.FormatInt(record.ReceivedAt.UnixNano(), 10)
			status := strings.TrimSpace(event.GetExtraInfo())
			if status == "" {
				status = "exit status"
			}
			exitNode := builder.addNode(Node{
				ID:       exitID,
				Kind:     "exit_status",
				Label:    status,
				Metadata: map[string]string{"status": status},
			})
			builder.addEdge(Edge{ID: activityNode.ID + "->" + exitNode.ID + ":exited", Source: activityNode.ID, Target: exitNode.ID, Kind: "exited", Label: "exited"})
		case "semantic_alert":
			alertID := processNode.ID + ":alert:" + sanitizeGraphIDParts(event.GetComm(), event.GetPath(), event.GetExtraInfo())
			alertNode := builder.addNode(Node{
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
			builder.addEdge(Edge{ID: activityNode.ID + "->" + alertNode.ID + ":alerted", Source: activityNode.ID, Target: alertNode.ID, Kind: "alerted", Label: "alerted"})
		}

		for _, relation := range graphFileRelations(event) {
			relation.Node = builder.addNode(relation.Node)
			builder.addEdge(Edge{ID: activityNode.ID + "->" + relation.Node.ID + ":" + relation.Kind, Source: activityNode.ID, Target: relation.Node.ID, Kind: relation.Kind, Label: relation.Kind})
		}
		for _, relation := range graphNetworkRelations(event) {
			relation.Node = builder.addNode(relation.Node)
			builder.addEdge(Edge{ID: activityNode.ID + "->" + relation.Node.ID + ":" + relation.Kind, Source: activityNode.ID, Target: relation.Node.ID, Kind: relation.Kind, Label: relation.Kind})
		}
	}

	if filters.PID != nil && filters.ProcessTree {
		builder.addNode(Node{
			ID:       processNodeID(*filters.PID),
			Kind:     "process",
			Label:    fmt.Sprintf("pid %d", *filters.PID),
			Subtitle: "monitored process",
			PID:      *filters.PID,
			Metadata: map[string]string{
				"pid":       strconv.FormatUint(uint64(*filters.PID), 10),
				"monitored": "true",
			},
		})
	}
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}
	return builder.response(ctx, matchedEvents)
}
