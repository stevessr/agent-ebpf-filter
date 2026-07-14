package executiongraph

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

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
	if filters.ToolName != "" && !containsExecutionGraphFilter(event.GetToolName(), filters.ToolName) {
		return false
	}
	if filters.Decision != "" && !strings.EqualFold(event.GetDecision(), filters.Decision) {
		return false
	}
	if filters.Comm != "" && !containsExecutionGraphFilter(event.GetComm(), filters.Comm) {
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
		if !containsExecutionGraphFilter(event.GetPath(), filters.Path) && !containsExecutionGraphFilter(event.GetExtraPath(), filters.Path) {
			return false
		}
	}
	if filters.Domain != "" {
		if !containsExecutionGraphFilter(event.GetDomain(), filters.Domain) && !containsExecutionGraphFilter(event.GetNetEndpoint(), filters.Domain) {
			return false
		}
	}
	return true
}

func prepareExecutionGraphFilters(filters Filters) Filters {
	filters.ToolName = strings.ToLower(filters.ToolName)
	filters.Comm = strings.ToLower(filters.Comm)
	filters.Path = strings.ToLower(filters.Path)
	filters.Domain = strings.ToLower(filters.Domain)
	return filters
}

func containsExecutionGraphFilter(value, lowercaseNeedle string) bool {
	if lowercaseNeedle == "" {
		return true
	}
	if isASCIIExecutionGraphText(value) && isASCIIExecutionGraphText(lowercaseNeedle) {
		if len(value) < len(lowercaseNeedle) {
			return false
		}
		return containsASCIIFold(value, lowercaseNeedle)
	}
	return strings.Contains(strings.ToLower(value), lowercaseNeedle)
}

func isASCIIExecutionGraphText(value string) bool {
	for index := 0; index < len(value); index++ {
		if value[index] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func containsASCIIFold(value, lowercaseNeedle string) bool {
	needleBytes := len(lowercaseNeedle)
	if needleBytes == 0 {
		return true
	}
	if len(value) < needleBytes {
		return false
	}
	const hashBase uint64 = 16777619
	var needleHash uint64
	var windowHash uint64
	power := uint64(1)
	for index := 0; index < needleBytes; index++ {
		needleHash = needleHash*hashBase + uint64(lowercaseNeedle[index])
		windowHash = windowHash*hashBase + uint64(lowerASCIIByte(value[index]))
		if index+1 < needleBytes {
			power *= hashBase
		}
	}
	if needleHash == windowHash && equalASCIIFold(value[:needleBytes], lowercaseNeedle) {
		return true
	}
	for index := needleBytes; index < len(value); index++ {
		outgoing := uint64(lowerASCIIByte(value[index-needleBytes]))
		incoming := uint64(lowerASCIIByte(value[index]))
		windowHash = (windowHash-outgoing*power)*hashBase + incoming
		if needleHash == windowHash && equalASCIIFold(value[index-needleBytes+1:index+1], lowercaseNeedle) {
			return true
		}
	}
	return false
}

func equalASCIIFold(value, lowercaseNeedle string) bool {
	for index := range lowercaseNeedle {
		if lowerASCIIByte(value[index]) != lowercaseNeedle[index] {
			return false
		}
	}
	return true
}

func lowerASCIIByte(value byte) byte {
	if value >= 'A' && value <= 'Z' {
		return value + ('a' - 'A')
	}
	return value
}

func buildExecutionGraphPIDTree(records []Record, filters Filters) map[uint32]struct{} {
	tree, _ := buildExecutionGraphPIDTreeContext(context.Background(), records, filters)
	return tree
}

func buildExecutionGraphPIDTreeContext(ctx context.Context, records []Record, filters Filters) (map[uint32]struct{}, error) {
	if filters.PID == nil || !filters.ProcessTree {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	seed := *filters.PID
	tree := map[uint32]struct{}{seed: {}}
	children := make(map[uint32][]uint32)
	for index, record := range records {
		if index%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		event := record.Event
		if event == nil || !matchesExecutionGraphNonPIDFilters(record, event, filters) {
			continue
		}
		pid := event.GetPid()
		if pid != 0 {
			children[event.GetPpid()] = append(children[event.GetPpid()], pid)
		}
		if childPID, ok := extractGraphPID(event.GetExtraInfo(), "child_pid"); ok {
			children[pid] = append(children[pid], childPID)
		}
	}
	queue := []uint32{seed}
	for index := 0; index < len(queue); index++ {
		if index%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		for _, child := range children[queue[index]] {
			if _, exists := tree[child]; exists {
				continue
			}
			tree[child] = struct{}{}
			queue = append(queue, child)
		}
	}
	return tree, ctx.Err()
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
	if childPID, ok := extractGraphPID(event.GetExtraInfo(), "child_pid"); ok {
		_, ok := pidTree[childPID]
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
	if millis, err := strconv.ParseInt(value, 10, 64); err == nil {
		if millis < 500 {
			return 500 * time.Millisecond
		}
		if millis > 30000 {
			return 30 * time.Second
		}
		return time.Duration(millis) * time.Millisecond
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
