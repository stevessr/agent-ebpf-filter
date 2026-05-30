package executiongraph

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
)

func buildProcessGraphNode(event *pb.Event) Node {
	label := strings.TrimSpace(event.GetComm())
	if label == "" {
		label = fmt.Sprintf("pid %d", event.GetPid())
	}
	return Node{
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

func buildExecutionGraphActivityNode(record Record, event *pb.Event, index int) Node {
	id := fmt.Sprintf("evt:%d:%d:%s", record.ReceivedAt.UnixNano(), index, sanitizeGraphID(event.GetType()))
	label := strings.TrimSpace(event.GetType())
	if label == "" {
		label = "event"
	}
	return Node{
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

func buildExecutionDecisionNode(record Record, event *pb.Event, index int) (Node, string) {
	decisionKind := graphDecisionEdgeKind(event.GetDecision())
	id := fmt.Sprintf("decision:%d:%d:%s", record.ReceivedAt.UnixNano(), index, sanitizeGraphID(event.GetDecision()))
	return Node{
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

func isGenericProcessLabel(node Node) bool {
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
			Node: Node{
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
		Node: Node{
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
