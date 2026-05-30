package executiongraph

import (
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
)

func matchesExecutionGraphFilters(record Record, event *pb.Event, filters Filters, pidTree map[uint32]struct{}) bool {
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

func buildExecutionGraphPIDTree(records []Record, filters Filters) map[uint32]struct{} {
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

func matchesExecutionGraphNonPIDFilters(record Record, event *pb.Event, filters Filters) bool {
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

func ParseBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "t", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func ParseInterval(raw string) time.Duration {
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

func ParseTime(raw string) (time.Time, bool) {
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
