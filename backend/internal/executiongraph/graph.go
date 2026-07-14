package executiongraph

import (
	"context"
	"fmt"
	"sort"
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
	EventCount int            `json:"eventCount"`
	Source     string         `json:"source"`
	NodeCounts map[string]int `json:"nodeCounts,omitempty"`
	EdgeCounts map[string]int `json:"edgeCounts,omitempty"`
	Nodes      []Node         `json:"nodes"`
	Edges      []Edge         `json:"edges"`
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
	nodes := make(map[string]Node)
	edges := make(map[string]Edge)
	matchedEvents := 0
	pidTree, err := buildExecutionGraphPIDTreeContext(ctx, records, filters)
	if err != nil {
		return Response{}, err
	}

	addNode := func(node Node) {
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
	addEdge := func(edge Edge) {
		if edge.ID == "" || edge.Source == "" || edge.Target == "" {
			return
		}
		edges[edge.ID] = edge
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

		processNode := buildProcessGraphNode(event)
		addNode(processNode)
		if event.GetPpid() > 0 && event.GetPpid() != event.GetPid() {
			parentID := processNodeID(event.GetPpid())
			addNode(Node{
				ID:       parentID,
				Kind:     "process",
				Label:    fmt.Sprintf("pid %d", event.GetPpid()),
				Subtitle: "parent process",
				PID:      event.GetPpid(),
				Metadata: map[string]string{
					"pid": strconv.FormatUint(uint64(event.GetPpid()), 10),
				},
			})
			addEdge(Edge{ID: parentID + "->" + processNode.ID + ":parent_process", Source: parentID, Target: processNode.ID, Kind: "parent_process", Label: "parent process"})
		}

		activityNode := buildExecutionGraphActivityNode(record, event, index)
		addNode(activityNode)
		processToActivityKind := graphActivityEdgeKind(event)
		addEdge(Edge{
			ID:     processNode.ID + "->" + activityNode.ID + ":" + processToActivityKind,
			Source: processNode.ID,
			Target: activityNode.ID,
			Kind:   processToActivityKind,
			Label:  processToActivityKind,
		})

		if event.GetAgentRunId() != "" {
			runID := "run:" + event.GetAgentRunId()
			addNode(Node{
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
			addEdge(Edge{ID: runID + "->" + processNode.ID + ":contains", Source: runID, Target: processNode.ID, Kind: "contains", Label: "contains"})
			if event.GetToolCallId() != "" {
				toolID := "tool:" + event.GetToolCallId()
				addNode(Node{
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
				addEdge(Edge{ID: runID + "->" + toolID + ":contains", Source: runID, Target: toolID, Kind: "contains", Label: "contains"})
				addEdge(Edge{ID: toolID + "->" + processNode.ID + ":owns", Source: toolID, Target: processNode.ID, Kind: "owns", Label: "owns"})
			}
		}

		if event.GetDecision() != "" {
			decisionNode, decisionEdgeKind := buildExecutionDecisionNode(record, event, index)
			addNode(decisionNode)
			addEdge(Edge{
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
				oldNode := Node{
					ID:       processNodeID(uint32(oldPID)),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", oldPID),
					Subtitle: "pre-exec pid",
					PID:      uint32(oldPID),
					Metadata: map[string]string{"pid": strconv.Itoa(oldPID)},
				}
				addNode(oldNode)
				addEdge(Edge{ID: oldNode.ID + "->" + processNode.ID + ":exec_chain", Source: oldNode.ID, Target: processNode.ID, Kind: "exec_chain", Label: "exec"})
			}
		case "process_fork", "clone":
			if childPID, ok := extractGraphInt(event.GetExtraInfo(), "child_pid"); ok && childPID > 0 {
				childNode := Node{
					ID:       processNodeID(uint32(childPID)),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", childPID),
					Subtitle: "child process",
					PID:      uint32(childPID),
					Metadata: map[string]string{"pid": strconv.Itoa(childPID)},
				}
				addNode(childNode)
				addEdge(Edge{ID: processNode.ID + "->" + childNode.ID + ":child_process", Source: processNode.ID, Target: childNode.ID, Kind: "child_process", Label: "child process"})
				addEdge(Edge{ID: activityNode.ID + "->" + childNode.ID + ":spawned", Source: activityNode.ID, Target: childNode.ID, Kind: "spawned", Label: "spawned"})
			}
		case "wait4":
			if targetPID, ok := extractGraphInt(event.GetExtraInfo(), "target_pid"); ok && targetPID > 0 {
				targetNode := Node{
					ID:       processNodeID(uint32(targetPID)),
					Kind:     "process",
					Label:    fmt.Sprintf("pid %d", targetPID),
					Subtitle: "wait target",
					PID:      uint32(targetPID),
					Metadata: map[string]string{"pid": strconv.Itoa(targetPID)},
				}
				addNode(targetNode)
				addEdge(Edge{ID: activityNode.ID + "->" + targetNode.ID + ":waited", Source: activityNode.ID, Target: targetNode.ID, Kind: "waited", Label: "waited"})
			}
		case "process_exit", "exit":
			exitID := processNode.ID + ":exit:" + strconv.FormatInt(record.ReceivedAt.UnixNano(), 10)
			status := strings.TrimSpace(event.GetExtraInfo())
			if status == "" {
				status = "exit status"
			}
			addNode(Node{
				ID:       exitID,
				Kind:     "exit_status",
				Label:    status,
				Metadata: map[string]string{"status": status},
			})
			addEdge(Edge{ID: activityNode.ID + "->" + exitID + ":exited", Source: activityNode.ID, Target: exitID, Kind: "exited", Label: "exited"})
		case "semantic_alert":
			alertID := processNode.ID + ":alert:" + sanitizeGraphID(event.GetComm()+":"+event.GetPath()+":"+event.GetExtraInfo())
			addNode(Node{
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
			addEdge(Edge{ID: activityNode.ID + "->" + alertID + ":alerted", Source: activityNode.ID, Target: alertID, Kind: "alerted", Label: "alerted"})
		}

		for _, relation := range graphFileRelations(event) {
			addNode(relation.Node)
			addEdge(Edge{ID: activityNode.ID + "->" + relation.Node.ID + ":" + relation.Kind, Source: activityNode.ID, Target: relation.Node.ID, Kind: relation.Kind, Label: relation.Kind})
		}
		for _, relation := range graphNetworkRelations(event) {
			addNode(relation.Node)
			addEdge(Edge{ID: activityNode.ID + "->" + relation.Node.ID + ":" + relation.Kind, Source: activityNode.ID, Target: relation.Node.ID, Kind: relation.Kind, Label: relation.Kind})
		}
	}

	if filters.PID != nil && filters.ProcessTree {
		addNode(Node{
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

	nodeList := make([]Node, 0, len(nodes))
	nodeCounts := make(map[string]int)
	nodeIndex := 0
	for _, node := range nodes {
		if nodeIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return Response{}, err
			}
		}
		nodeList = append(nodeList, node)
		nodeCounts[node.Kind]++
		nodeIndex++
	}
	sort.Slice(nodeList, func(i, j int) bool {
		if nodeList[i].Kind == nodeList[j].Kind {
			return nodeList[i].Label < nodeList[j].Label
		}
		return nodeList[i].Kind < nodeList[j].Kind
	})

	edgeList := make([]Edge, 0, len(edges))
	edgeCounts := make(map[string]int)
	edgeIndex := 0
	for _, edge := range edges {
		if edgeIndex%256 == 0 {
			if err := ctx.Err(); err != nil {
				return Response{}, err
			}
		}
		edgeList = append(edgeList, edge)
		edgeCounts[edge.Kind]++
		edgeIndex++
	}
	sort.Slice(edgeList, func(i, j int) bool { return edgeList[i].ID < edgeList[j].ID })
	if err := ctx.Err(); err != nil {
		return Response{}, err
	}

	return Response{
		EventCount: matchedEvents,
		NodeCounts: nodeCounts,
		EdgeCounts: edgeCounts,
		Nodes:      nodeList,
		Edges:      edgeList,
	}, nil
}
