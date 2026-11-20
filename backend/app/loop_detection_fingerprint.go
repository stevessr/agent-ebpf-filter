package app

import (
	"agent-ebpf-filter/pb"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

func shouldIgnoreLoopDetectionEvent(event *pb.Event) bool {
	if event == nil {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(event.GetType()), "semantic_alert") {
		return true
	}
	if event.GetEventType() == pb.EventType_SEMANTIC_ALERT {
		return true
	}
	return false
}

func loopDetectionContext(event *pb.Event) (string, string) {
	if event == nil {
		return "", ""
	}
	runID := boundLoopDetectionString(event.GetAgentRunId(), loopDetectionContextComponentBytes)
	taskID := boundLoopDetectionString(event.GetTaskId(), loopDetectionContextComponentBytes)
	toolCallID := boundLoopDetectionString(event.GetToolCallId(), loopDetectionContextComponentBytes)
	traceID := boundLoopDetectionString(event.GetTraceId(), loopDetectionContextComponentBytes)
	if toolCallID != "" {
		return "tool_call", buildLoopDetectionContextKey("tool_call:", runID, taskID, toolCallID)
	}
	if taskID != "" {
		return "task", buildLoopDetectionContextKey("task:", runID, taskID)
	}
	if runID != "" {
		return "agent_run", "agent_run:" + runID
	}
	if traceID != "" {
		return "trace", "trace:" + traceID
	}
	if root := event.GetRootAgentPid(); root != 0 {
		return "root_agent_pid", fmt.Sprintf("root_agent_pid:%d", root)
	}
	if tgid := event.GetTgid(); tgid != 0 {
		return "tgid", fmt.Sprintf("tgid:%d", tgid)
	}
	if pid := event.GetPid(); pid != 0 {
		return "pid", fmt.Sprintf("pid:%d", pid)
	}
	if comm := boundLoopDetectionString(event.GetComm(), loopDetectionMetadataValueBytes); comm != "" {
		return "comm", "comm:" + comm
	}
	return "", ""
}

func buildLoopDetectionContextKey(prefix string, values ...string) string {
	size := len(prefix)
	count := 0
	for _, value := range values {
		if value != "" {
			size += len(value)
			if count > 0 {
				size++
			}
			count++
		}
	}
	var out strings.Builder
	out.Grow(size)
	out.WriteString(prefix)
	wroteValue := false
	for _, value := range values {
		if value == "" {
			continue
		}
		if wroteValue {
			out.WriteByte('/')
		}
		out.WriteString(value)
		wroteValue = true
	}
	return out.String()
}

func loopDetectionFingerprint(event *pb.Event) (fingerprint string, target string) {
	if event == nil {
		return "", ""
	}
	eventType := normalizedLoopEventType(event)
	target = loopDetectionStableTarget(event)
	toolName := strings.ToLower(boundLoopDetectionString(event.GetToolName(), loopDetectionMetadataValueBytes))
	if eventType == "" && target == "" {
		return "", ""
	}
	size := len(eventType)
	if toolName != "" {
		size += len("|tool=") + len(toolName)
	}
	if target != "" {
		size += len("|target=") + len(target)
	}
	var out strings.Builder
	out.Grow(size)
	out.WriteString(eventType)
	if toolName != "" {
		out.WriteString("|tool=")
		out.WriteString(toolName)
	}
	if target != "" {
		out.WriteString("|target=")
		out.WriteString(target)
	}
	return out.String(), target
}

func normalizedLoopEventType(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if value := boundLoopDetectionString(event.GetType(), loopDetectionMetadataValueBytes); value != "" {
		return strings.ToLower(value)
	}
	if value := boundLoopDetectionString(event.GetEventType().String(), loopDetectionMetadataValueBytes); value != "" {
		return strings.ToLower(value)
	}
	return ""
}

func loopDetectionStableTarget(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if digest := extractLoopDetectionDigest(event.GetExtraInfo()); digest != "" {
		return "digest:" + digest
	}
	candidates := []string{
		event.GetPath(),
		event.GetExtraPath(),
		event.GetNetEndpoint(),
		event.GetDnsName(),
		event.GetSni(),
		event.GetDomain(),
		event.GetServiceName(),
		event.GetArgvDigest(),
		event.GetToolName(),
		event.GetComm(),
	}
	for _, candidate := range candidates {
		if normalized := normalizeLoopDetectionTarget(candidate); normalized != "" {
			return normalized
		}
	}
	return ""
}

func extractLoopDetectionDigest(extraInfo string) string {
	extraInfo = strings.TrimSpace(extraInfo)
	if extraInfo == "" {
		return ""
	}
	if len(extraInfo) > loopDetectionExtraInfoScanBytes {
		extraInfo = extraInfo[:loopDetectionExtraInfoScanBytes]
	}
	lower := strings.ToLower(extraInfo)
	keys := []string{"prompt_digest", "promptdigest", "context_digest", "request_digest", "response_digest", "argv_digest", "digest"}
	for _, key := range keys {
		idx := strings.Index(lower, key)
		if idx < 0 {
			continue
		}
		rest := lower[idx+len(key):]
		rest = strings.TrimLeft(rest, " :=\t\n\r\"'")
		if rest == "" {
			continue
		}
		end := len(rest)
		for i, r := range rest {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
				end = i
				break
			}
		}
		value := strings.Trim(rest[:end], " _-")
		if len(value) >= 6 {
			return boundLoopDetectionString(value, loopDetectionContextComponentBytes)
		}
	}
	return ""
}

func normalizeLoopDetectionTarget(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) > loopDetectionTargetBytes {
		return boundLoopDetectionString(value, loopDetectionTargetBytes)
	}
	if strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		value = strings.Join(strings.Fields(value), " ")
	}
	if strings.HasPrefix(value, "/") {
		value = filepath.Clean(value)
	}
	if strings.Contains(value, "://") || strings.Contains(value, ".") || strings.Contains(value, ":") {
		value = strings.ToLower(value)
	}
	return boundLoopDetectionString(value, loopDetectionTargetBytes)
}

func boundLoopDetectionString(value string, maxBytes int) string {
	value = strings.TrimSpace(value)
	if value == "" || maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	suffix := "~sha256:" + hex.EncodeToString(sum[:8])
	if len(suffix) >= maxBytes {
		return suffix[:maxBytes]
	}
	prefixBytes := maxBytes - len(suffix)
	for prefixBytes > 0 && !utf8.ValidString(value[:prefixBytes]) {
		prefixBytes--
	}
	return value[:prefixBytes] + suffix
}

func cloneBoundedLoopDetectionString(value string, maxBytes int) string {
	return strings.Clone(boundLoopDetectionString(value, maxBytes))
}

func addLoopDetectionString(values map[string]struct{}, value string) map[string]struct{} {
	if value == "" {
		return values
	}
	if _, exists := values[value]; exists {
		return values
	}
	if len(values) >= loopDetectionWindowMetadataLimit {
		return values
	}
	if values == nil {
		values = make(map[string]struct{}, loopDetectionWindowMetadataLimit)
	}
	values[strings.Clone(value)] = struct{}{}
	return values
}

func addBoundedLoopPID(values map[uint32]struct{}, value uint32) map[uint32]struct{} {
	if value == 0 {
		return values
	}
	if _, exists := values[value]; exists {
		return values
	}
	if len(values) >= loopDetectionWindowMetadataLimit {
		return values
	}
	if values == nil {
		values = make(map[uint32]struct{}, loopDetectionWindowMetadataLimit)
	}
	values[value] = struct{}{}
	return values
}

func sortedStringSet(set map[string]struct{}, limit int) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		if strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	sort.Strings(values)
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}

func sortedUint32Set(set map[uint32]struct{}, limit int) []uint32 {
	values := make([]uint32, 0, len(set))
	for value := range set {
		if value != 0 {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if limit > 0 && len(values) > limit {
		return values[:limit]
	}
	return values
}
