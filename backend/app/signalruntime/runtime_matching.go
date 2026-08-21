package signalruntime

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"agent-ebpf-filter/pb"
)

func shouldIgnoreSignalProcessingEvent(event *pb.Event) bool {
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

func signalRuleMatches(rule SignalRule, event *pb.Event) bool {
	if event == nil {
		return false
	}
	kind := strings.ToLower(strings.TrimSpace(rule.Kind))
	if kind == "" {
		kind = signalKindCustom
	}
	if kind != signalKindCustom && !signalKindDefaultMatches(kind, event) {
		return false
	}
	if len(rule.Conditions) == 0 {
		return kind != signalKindCustom
	}
	for _, condition := range rule.Conditions {
		if !signalConditionMatches(condition, event) {
			return false
		}
	}
	return true
}

func signalKindDefaultMatches(kind string, event *pb.Event) bool {
	if event == nil {
		return false
	}
	eventType := strings.ToLower(signalEventType(event))
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case signalKindPathAccess:
		return strings.TrimSpace(event.GetPath()) != "" ||
			strings.TrimSpace(event.GetExtraPath()) != "" ||
			strings.Contains(eventType, "open") ||
			strings.Contains(eventType, "read") ||
			strings.Contains(eventType, "write")
	case signalKindChildProcess:
		return event.GetEventType() == pb.EventType_EXECVE ||
			event.GetEventType() == pb.EventType_SCHED_PROCESS_EXEC ||
			event.GetEventType() == pb.EventType_SCHED_PROCESS_FORK ||
			event.GetEventType() == pb.EventType_CLONE ||
			strings.Contains(eventType, "exec") ||
			strings.Contains(eventType, "fork") ||
			strings.Contains(eventType, "clone")
	case signalKindRepeatedRead:
		return (event.GetEventType() == pb.EventType_READ ||
			event.GetEventType() == pb.EventType_OPEN ||
			event.GetEventType() == pb.EventType_OPENAT ||
			strings.Contains(eventType, "read") ||
			strings.Contains(eventType, "open")) &&
			(strings.TrimSpace(event.GetPath()) != "" || strings.TrimSpace(event.GetExtraPath()) != "")
	default:
		return false
	}
}

func signalConditionMatches(condition SignalCondition, event *pb.Event) bool {
	values := signalFieldValues(condition.Field, event)
	operator := normalizeSignalConditionOperator(condition.Operator, condition.Value)
	needle := strings.TrimSpace(condition.Value)
	switch operator {
	case "exists", "any":
		for _, value := range values {
			if strings.TrimSpace(value) != "" {
				return true
			}
		}
		return false
	case "regex":
		if needle == "" {
			return false
		}
		re, err := regexp.Compile(needle)
		if err != nil {
			return false
		}
		for _, value := range values {
			if re.MatchString(value) {
				return true
			}
		}
		return false
	}
	if needle == "" {
		return false
	}
	needleLower := strings.ToLower(needle)
	for _, value := range values {
		value = strings.TrimSpace(value)
		valueLower := strings.ToLower(value)
		switch operator {
		case "equals":
			if valueLower == needleLower {
				return true
			}
		case "not_equals":
			if valueLower != "" && valueLower != needleLower {
				return true
			}
		case "prefix":
			if strings.HasPrefix(valueLower, needleLower) {
				return true
			}
		case "suffix":
			if strings.HasSuffix(valueLower, needleLower) {
				return true
			}
		case "not_contains":
			if valueLower != "" && !strings.Contains(valueLower, needleLower) {
				return true
			}
		default:
			if strings.Contains(valueLower, needleLower) {
				return true
			}
		}
	}
	return false
}

func signalFieldValues(field string, event *pb.Event) []string {
	if event == nil {
		return nil
	}
	switch normalizeSignalFieldName(field) {
	case "path":
		return nonEmptySignalValues(event.GetPath(), event.GetExtraPath())
	case "extrapath":
		return nonEmptySignalValues(event.GetExtraPath())
	case "comm", "program":
		return nonEmptySignalValues(event.GetComm())
	case "type", "eventtype":
		return nonEmptySignalValues(event.GetType(), event.GetEventType().String(), strconv.Itoa(int(event.GetEventType())))
	case "childcommand", "command", "cmdline":
		return nonEmptySignalValues(event.GetPath(), event.GetExtraInfo(), event.GetArgvDigest(), event.GetComm())
	case "readkey":
		return nonEmptySignalValues(event.GetPath(), event.GetExtraPath(), event.GetArgvDigest())
	case "target":
		return nonEmptySignalValues(signalStableTarget(event))
	case "extrainfo":
		return nonEmptySignalValues(event.GetExtraInfo())
	case "cwd":
		return nonEmptySignalValues(event.GetCwd())
	case "tool", "toolname":
		return nonEmptySignalValues(event.GetToolName())
	case "netendpoint", "endpoint":
		return nonEmptySignalValues(event.GetNetEndpoint(), event.GetDstIp(), event.GetDnsName(), event.GetSni(), event.GetHttpHost(), event.GetDomain())
	case "decision":
		return nonEmptySignalValues(event.GetDecision())
	default:
		return nonEmptySignalValues(event.GetPath(), event.GetExtraPath(), event.GetExtraInfo(), event.GetComm(), event.GetToolName(), signalEventType(event))
	}
}

func normalizeSignalFieldName(field string) string {
	field = strings.ToLower(strings.TrimSpace(field))
	replacer := strings.NewReplacer("_", "", "-", "", ".", "", " ", "")
	return replacer.Replace(field)
}

func nonEmptySignalValues(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func signalStateKey(rule SignalRule, event *pb.Event) (string, string) {
	target := signalStableTarget(event)
	switch strings.ToLower(strings.TrimSpace(rule.Kind)) {
	case signalKindRepeatedRead:
		target = firstNonEmptySignalValue(event.GetPath(), event.GetExtraPath(), event.GetArgvDigest(), target)
	case signalKindChildProcess:
		target = firstNonEmptySignalValue(event.GetPath(), extractSignalCommand(event.GetExtraInfo()), event.GetArgvDigest(), event.GetComm(), target)
	case signalKindPathAccess:
		target = firstNonEmptySignalValue(event.GetPath(), event.GetExtraPath(), target)
	default:
		target = firstNonEmptySignalValue(target, event.GetPath(), event.GetExtraPath(), event.GetComm(), signalEventType(event))
	}
	if target == "" {
		return "", ""
	}
	context := signalContextKey(event)
	if context == "" {
		context = "global"
	}
	return strings.Join([]string{rule.ID, strings.ToLower(strings.TrimSpace(rule.Kind)), context, target}, "\x00"), target
}

func signalContextKey(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if toolCallID := strings.TrimSpace(event.GetToolCallId()); toolCallID != "" {
		return "tool_call:" + strings.Join(nonEmptySignalValues(event.GetAgentRunId(), event.GetTaskId(), toolCallID), "/")
	}
	if taskID := strings.TrimSpace(event.GetTaskId()); taskID != "" {
		return "task:" + strings.Join(nonEmptySignalValues(event.GetAgentRunId(), taskID), "/")
	}
	if runID := strings.TrimSpace(event.GetAgentRunId()); runID != "" {
		return "agent_run:" + runID
	}
	if root := event.GetRootAgentPid(); root != 0 {
		return fmt.Sprintf("root:%d", root)
	}
	if tgid := event.GetTgid(); tgid != 0 {
		return fmt.Sprintf("tgid:%d", tgid)
	}
	if pid := event.GetPid(); pid != 0 {
		return fmt.Sprintf("pid:%d", pid)
	}
	if comm := strings.TrimSpace(event.GetComm()); comm != "" {
		return "comm:" + comm
	}
	return ""
}

func signalStableTarget(event *pb.Event) string {
	if event == nil {
		return ""
	}
	target := firstNonEmptySignalValue(
		event.GetPath(),
		event.GetExtraPath(),
		event.GetNetEndpoint(),
		event.GetDnsName(),
		event.GetSni(),
		event.GetHttpHost(),
		event.GetDomain(),
		event.GetArgvDigest(),
		event.GetToolName(),
		event.GetComm(),
	)
	if strings.HasPrefix(target, "/") {
		target = filepath.Clean(target)
	}
	if len(target) > 240 {
		target = target[:240]
	}
	return target
}

func firstNonEmptySignalValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func signalEventType(event *pb.Event) string {
	if event == nil {
		return ""
	}
	if value := strings.TrimSpace(event.GetType()); value != "" {
		return value
	}
	return strings.TrimSpace(event.GetEventType().String())
}

func extractSignalCommand(extraInfo string) string {
	extraInfo = strings.TrimSpace(extraInfo)
	if extraInfo == "" {
		return ""
	}
	lower := strings.ToLower(extraInfo)
	for _, key := range []string{"cmdline", "command", "argv", "exec"} {
		idx := strings.Index(lower, key)
		if idx < 0 {
			continue
		}
		rest := strings.TrimLeft(extraInfo[idx+len(key):], " :=\t\n\r\"'")
		if rest == "" {
			continue
		}
		if len(rest) > 240 {
			rest = rest[:240]
		}
		return strings.TrimSpace(rest)
	}
	return extraInfo
}

func signalStateID(key string) string {
	sum := sha1.Sum([]byte(key))
	return "sig_" + hex.EncodeToString(sum[:8])
}

func recordEnvelopeID(record CapturedEventRecord) string {
	if record.Envelope != nil {
		return strings.TrimSpace(record.Envelope.GetEventId())
	}
	return ""
}
