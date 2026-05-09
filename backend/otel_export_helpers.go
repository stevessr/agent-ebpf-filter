package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"agent-ebpf-filter/pb"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func buildOTelTracerProvider(endpoint, serviceName string, headers map[string]string, owner *otelExporterState) (*sdktrace.TracerProvider, oteltrace.Tracer, error) {
	exporter, err := buildOTLPHTTPExporter(endpoint, headers, owner)
	if err != nil {
		return nil, nil, err
	}
	res, err := resource.New(
		context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
			attribute.String("agent.component", "agent-ebpf-filter"),
		),
	)
	if err != nil {
		return nil, nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter,
			sdktrace.WithBatchTimeout(2*time.Second),
			sdktrace.WithMaxExportBatchSize(256),
		),
	)
	return provider, provider.Tracer("agent-ebpf-filter/otel"), nil
}

func buildOTLPHTTPExporter(endpoint string, headers map[string]string, owner *otelExporterState) (sdktrace.SpanExporter, error) {
	opts, err := buildOTLPHTTPOptions(endpoint, headers)
	if err != nil {
		return nil, err
	}
	exporter, err := otlptracehttp.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}
	return &monitoringSpanExporter{inner: exporter, owner: owner}, nil
}

func buildOTLPHTTPOptions(endpoint string, headers map[string]string) ([]otlptracehttp.Option, error) {
	trimmedEndpoint := strings.TrimSpace(endpoint)
	if trimmedEndpoint == "" {
		return nil, fmt.Errorf("empty OTLP endpoint")
	}
	opts := []otlptracehttp.Option{
		otlptracehttp.WithHeaders(cloneStringMap(headers)),
		otlptracehttp.WithTimeout(5 * time.Second),
	}
	if !strings.Contains(trimmedEndpoint, "://") {
		opts = append(opts, otlptracehttp.WithEndpoint(trimmedEndpoint))
		return opts, nil
	}
	parsed, err := url.Parse(trimmedEndpoint)
	if err != nil {
		return nil, err
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("invalid OTLP endpoint %q", endpoint)
	}
	path := strings.TrimSpace(parsed.EscapedPath())
	if path == "" || path == "/" {
		path = "/v1/traces"
	}
	opts = append(opts, otlptracehttp.WithEndpoint(parsed.Host), otlptracehttp.WithURLPath(path))
	switch parsed.Scheme {
	case "http":
		opts = append(opts, otlptracehttp.WithInsecure())
	case "https":
	default:
		return nil, fmt.Errorf("unsupported OTLP scheme %q", parsed.Scheme)
	}
	return opts, nil
}

func buildOTelAttributes(envelope *pb.EventEnvelope) []attribute.KeyValue {
	if envelope == nil {
		return nil
	}
	attrs := []attribute.KeyValue{
		attribute.String("agent.schema_version", envelope.GetSchemaVersion()),
		attribute.String("agent.event_id", envelope.GetEventId()),
		attribute.String("agent.source", envelope.GetSource()),
		attribute.String("agent.event_type", envelope.GetEventType().String()),
	}
	appendStringAttr := func(key, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			attrs = append(attrs, attribute.String(key, trimmed))
		}
	}
	appendIntAttr := func(key string, value uint64) {
		if value != 0 {
			attrs = append(attrs, attribute.Int64(key, int64(value)))
		}
	}
	appendStringAttr("agent.run_id", envelope.GetAgentRunId())
	appendStringAttr("agent.task_id", envelope.GetTaskId())
	appendStringAttr("agent.conversation_id", envelope.GetConversationId())
	appendStringAttr("agent.turn_id", envelope.GetTurnId())
	appendStringAttr("agent.tool_call_id", envelope.GetToolCallId())
	appendStringAttr("agent.tool_name", envelope.GetToolName())
	appendStringAttr("agent.trace_id", envelope.GetTraceId())
	appendStringAttr("agent.span_id", envelope.GetSpanId())
	appendStringAttr("agent.cwd", envelope.GetCwd())
	appendStringAttr("agent.container_id", envelope.GetContainerId())
	appendStringAttr("agent.argv_digest", envelope.GetArgvDigest())
	appendStringAttr("agent.policy_decision", envelope.GetPolicyDecision())
	appendIntAttr("process.pid", uint64(envelope.GetPid()))
	appendIntAttr("process.parent_pid", uint64(envelope.GetPpid()))
	appendIntAttr("process.tgid", uint64(envelope.GetTgid()))
	appendIntAttr("process.uid", uint64(envelope.GetUid()))
	appendIntAttr("process.gid", uint64(envelope.GetGid()))
	appendIntAttr("process.cgroup_id", envelope.GetCgroupId())
	appendStringAttr("process.command", envelope.GetComm())
	if envelope.GetRiskScore() > 0 {
		attrs = append(attrs, attribute.Float64("agent.risk_score", envelope.GetRiskScore()))
	}
	if legacy := envelope.GetLegacyEvent(); legacy != nil {
		if legacy.GetRetval() != 0 {
			attrs = append(attrs, attribute.Int64("process.retval", legacy.GetRetval()))
		}
		if legacy.GetDurationNs() > 0 {
			attrs = append(attrs, attribute.Int64("process.duration_ns", int64(legacy.GetDurationNs())))
		}
	}
	switch payload := envelope.GetPayload().(type) {
	case *pb.EventEnvelope_ExecEvent:
		appendStringAttr("process.executable.path", payload.ExecEvent.GetPath())
	case *pb.EventEnvelope_FileEvent:
		appendStringAttr("file.operation", payload.FileEvent.GetOperation())
		appendStringAttr("file.path", payload.FileEvent.GetPath())
		appendStringAttr("file.extra_path", payload.FileEvent.GetExtraPath())
		if payload.FileEvent.GetBytes() > 0 {
			attrs = append(attrs, attribute.Int64("file.bytes", int64(payload.FileEvent.GetBytes())))
		}
	case *pb.EventEnvelope_NetworkEvent:
		appendStringAttr("network.endpoint", payload.NetworkEvent.GetEndpoint())
		appendStringAttr("network.direction", payload.NetworkEvent.GetDirection())
		appendStringAttr("network.domain", payload.NetworkEvent.GetDomain())
		appendStringAttr("network.family", payload.NetworkEvent.GetFamily())
		appendStringAttr("network.sock_type", payload.NetworkEvent.GetSockType())
		if payload.NetworkEvent.GetBytes() > 0 {
			attrs = append(attrs, attribute.Int64("network.bytes", int64(payload.NetworkEvent.GetBytes())))
		}
	case *pb.EventEnvelope_ProcessEvent:
		appendStringAttr("process.phase", payload.ProcessEvent.GetPhase())
		appendIntAttr("process.child_pid", uint64(payload.ProcessEvent.GetChildPid()))
		appendIntAttr("process.target_pid", uint64(payload.ProcessEvent.GetTargetPid()))
		if payload.ProcessEvent.GetExitStatus() != 0 {
			attrs = append(attrs, attribute.Int("process.exit_status", int(payload.ProcessEvent.GetExitStatus())))
		}
	case *pb.EventEnvelope_PolicyEvent:
		appendStringAttr("policy.reason", payload.PolicyEvent.GetReason())
		appendStringAttr("policy.related_path", payload.PolicyEvent.GetRelatedPath())
		appendStringAttr("policy.related_endpoint", payload.PolicyEvent.GetRelatedEndpoint())
	case *pb.EventEnvelope_WrapperEvent:
		appendStringAttr("wrapper.command_line", payload.WrapperEvent.GetCommandLine())
	case *pb.EventEnvelope_HookEvent:
		appendStringAttr("hook.name", payload.HookEvent.GetHookName())
		appendStringAttr("hook.target_path", payload.HookEvent.GetTargetPath())
	case *pb.EventEnvelope_McpEvent:
		appendStringAttr("mcp.tool_name", payload.McpEvent.GetToolName())
		appendStringAttr("mcp.server_name", payload.McpEvent.GetServerName())
		appendStringAttr("mcp.endpoint", payload.McpEvent.GetEndpoint())
		appendStringAttr("mcp.request_id", payload.McpEvent.GetRequestId())
	}
	return attrs
}

func buildHierarchyAttributes(envelope *pb.EventEnvelope, level string) []attribute.KeyValue {
	attrs := []attribute.KeyValue{
		attribute.String("agent.hierarchy_level", level),
	}
	switch level {
	case "run":
		if value := firstNonEmpty(envelope.GetAgentRunId(), otelRunKey(envelope)); value != "" {
			attrs = append(attrs, attribute.String("agent.run_id", value))
		}
	case "task":
		if value := firstNonEmpty(envelope.GetTaskId(), otelTaskKey(envelope, otelRunKey(envelope))); value != "" {
			attrs = append(attrs, attribute.String("agent.task_id", value))
		}
	case "tool":
		if value := firstNonEmpty(envelope.GetToolCallId(), otelToolKey(envelope, otelTaskKey(envelope, otelRunKey(envelope)), otelRunKey(envelope))); value != "" {
			attrs = append(attrs, attribute.String("agent.tool_call_id", value))
		}
		if toolName := strings.TrimSpace(envelope.GetToolName()); toolName != "" {
			attrs = append(attrs, attribute.String("agent.tool_name", toolName))
		}
	}
	if traceID := strings.TrimSpace(envelope.GetTraceId()); traceID != "" {
		attrs = append(attrs, attribute.String("agent.trace_id", traceID))
	}
	return attrs
}

func otelEventName(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return "event.unknown"
	}
	switch envelope.GetPayload().(type) {
	case *pb.EventEnvelope_ExecEvent:
		return "process.exec"
	case *pb.EventEnvelope_FileEvent:
		file := envelope.GetFileEvent()
		operation := strings.TrimSpace(file.GetOperation())
		if operation == "" {
			operation = "unknown"
		}
		return "file." + operation
	case *pb.EventEnvelope_NetworkEvent:
		network := envelope.GetNetworkEvent()
		direction := strings.TrimSpace(network.GetDirection())
		switch {
		case strings.EqualFold(direction, "connect"), strings.Contains(strings.ToLower(network.GetEndpoint()), ":"):
			return "network.connect"
		case strings.EqualFold(direction, "send"):
			return "network.send"
		case strings.EqualFold(direction, "recv"):
			return "network.recv"
		default:
			return "network.flow"
		}
	case *pb.EventEnvelope_ProcessEvent:
		phase := strings.TrimSpace(envelope.GetProcessEvent().GetPhase())
		if phase == "" {
			phase = "unknown"
		}
		return "process." + phase
	case *pb.EventEnvelope_PolicyEvent:
		return "policy.decision"
	case *pb.EventEnvelope_WrapperEvent:
		return "tool.call"
	case *pb.EventEnvelope_HookEvent:
		return "hook.callback"
	case *pb.EventEnvelope_McpEvent:
		return "mcp.call"
	default:
		return "event." + strings.ToLower(strings.TrimSpace(envelope.GetSource()))
	}
}

func otelToolSpanName(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return "tool.call"
	}
	if _, ok := envelope.GetPayload().(*pb.EventEnvelope_McpEvent); ok {
		return "mcp.call"
	}
	toolName := strings.ToLower(strings.TrimSpace(envelope.GetToolName()))
	switch {
	case strings.Contains(toolName, "llm"), strings.Contains(toolName, "chat"), strings.Contains(toolName, "completion"):
		return "llm.call"
	case strings.Contains(toolName, "review"), strings.Contains(strings.ToLower(strings.TrimSpace(envelope.GetTaskId())), "review"):
		return "pr.review"
	default:
		return "tool.call"
	}
}

func shouldMarkSpanError(envelope *pb.EventEnvelope) bool {
	if envelope == nil {
		return false
	}
	if decision := strings.ToUpper(strings.TrimSpace(envelope.GetPolicyDecision())); decision == "BLOCK" || decision == "ALERT" {
		return true
	}
	if legacy := envelope.GetLegacyEvent(); legacy != nil && legacy.GetRetval() < 0 {
		return true
	}
	return false
}

func otelStatusMessage(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return ""
	}
	if decision := strings.TrimSpace(envelope.GetPolicyDecision()); decision != "" {
		return "policy decision: " + decision
	}
	if legacy := envelope.GetLegacyEvent(); legacy != nil && legacy.GetRetval() < 0 {
		return fmt.Sprintf("retval=%d", legacy.GetRetval())
	}
	return "runtime alert"
}

func shouldEndOTelHierarchy(envelope *pb.EventEnvelope) bool {
	if envelope == nil {
		return false
	}
	process := envelope.GetProcessEvent()
	if process == nil {
		return false
	}
	return strings.EqualFold(process.GetPhase(), "exit")
}

func otelRunKey(envelope *pb.EventEnvelope) string {
	if envelope == nil {
		return ""
	}
	parts := []string{}
	if runID := strings.TrimSpace(envelope.GetAgentRunId()); runID != "" {
		parts = append(parts, "run:"+runID)
	}
	if rootPID := envelope.GetPid(); envelope.GetLegacyEvent() != nil && envelope.GetLegacyEvent().GetRootAgentPid() != 0 {
		rootPID = envelope.GetLegacyEvent().GetRootAgentPid()
		if rootPID != 0 {
			parts = append(parts, fmt.Sprintf("root:%d", rootPID))
		}
	}
	if traceID := strings.TrimSpace(envelope.GetTraceId()); traceID != "" {
		parts = append(parts, "trace:"+traceID)
	}
	if len(parts) == 0 {
		if envelope.GetPid() == 0 {
			return ""
		}
		parts = append(parts, fmt.Sprintf("pid:%d", envelope.GetPid()))
	}
	return stableOTelKey(parts...)
}

func otelTaskKey(envelope *pb.EventEnvelope, runKey string) string {
	if envelope == nil {
		return ""
	}
	parts := []string{runKey}
	if taskID := strings.TrimSpace(envelope.GetTaskId()); taskID != "" {
		parts = append(parts, "task:"+taskID)
	} else if conversationID := strings.TrimSpace(envelope.GetConversationId()); conversationID != "" || strings.TrimSpace(envelope.GetTurnId()) != "" {
		parts = append(parts, "conversation:"+conversationID, "turn:"+strings.TrimSpace(envelope.GetTurnId()))
	} else {
		return ""
	}
	return stableOTelKey(parts...)
}

func otelToolKey(envelope *pb.EventEnvelope, taskKey, runKey string) string {
	if envelope == nil {
		return ""
	}
	parts := []string{runKey, taskKey}
	if toolCallID := strings.TrimSpace(envelope.GetToolCallId()); toolCallID != "" {
		parts = append(parts, "tool_call:"+toolCallID)
	} else if toolName := strings.TrimSpace(envelope.GetToolName()); toolName != "" {
		parts = append(parts, "tool_name:"+toolName)
		if envelope.GetPid() != 0 {
			parts = append(parts, fmt.Sprintf("pid:%d", envelope.GetPid()))
		}
	} else {
		return ""
	}
	return stableOTelKey(parts...)
}

func stableOTelKey(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(filtered, "\x00")))
	return "otel_" + hex.EncodeToString(sum[:10])
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if trimmedKey := strings.TrimSpace(key); trimmedKey != "" {
			out[trimmedKey] = strings.TrimSpace(input[key])
		}
	}
	return out
}

func collectIdleSpans(spans map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(spans) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range spans {
		if span == nil || now.Sub(span.lastSeen) < idleFor {
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectIdleTaskSpans(tasks, tools map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range tasks {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasActiveToolForTask(tools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectIdleRunSpans(runs, tasks, tools map[string]*activeOTelSpan, now time.Time, idleFor time.Duration) []*activeOTelSpan {
	if len(runs) == 0 {
		return nil
	}
	out := make([]*activeOTelSpan, 0)
	for _, span := range runs {
		if span == nil || now.Sub(span.lastSeen) < idleFor || hasActiveTaskForRun(tasks, span.key) || hasActiveToolForRun(tools, span.key) {
			continue
		}
		out = append(out, span)
	}
	return out
}

func hasActiveToolForTask(tools map[string]*activeOTelSpan, taskKey string) bool {
	for _, span := range tools {
		if span != nil && span.taskKey == taskKey {
			return true
		}
	}
	return false
}

func hasActiveTaskForRun(tasks map[string]*activeOTelSpan, runKey string) bool {
	for _, span := range tasks {
		if span != nil && span.runKey == runKey {
			return true
		}
	}
	return false
}

func hasActiveToolForRun(tools map[string]*activeOTelSpan, runKey string) bool {
	for _, span := range tools {
		if span != nil && span.runKey == runKey {
			return true
		}
	}
	return false
}

func endSpanMap(spans map[string]*activeOTelSpan, ts time.Time) {
	if len(spans) == 0 {
		return
	}
	list := make([]*activeOTelSpan, 0, len(spans))
	for _, span := range spans {
		if span != nil {
			list = append(list, span)
		}
	}
	endSpanSlice(list, ts)
}

func endSpanSlice(spans []*activeOTelSpan, ts time.Time) {
	for _, span := range spans {
		if span == nil || span.span == nil {
			continue
		}
		span.span.End(oteltrace.WithTimestamp(ts))
	}
}

func handleOTelHealth(c *gin.Context) {
	c.JSON(http.StatusOK, otelExporterStore.Snapshot())
}
