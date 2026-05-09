package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agent-ebpf-filter/pb"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const eventEnvelopeSchemaVersion = "envelope.v1"

var eventEnvelopeJSONMarshaller = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
}

func normalizeCapturedEventRecord(record CapturedEventRecord) CapturedEventRecord {
	if record.Event == nil && record.Envelope != nil && record.Envelope.GetLegacyEvent() != nil {
		record.Event = cloneProtoEvent(record.Envelope.GetLegacyEvent())
	}
	if record.Envelope == nil {
		record.Envelope = buildEventEnvelope(record)
	} else {
		record.Envelope = normalizeEventEnvelope(record.Envelope, record)
	}
	return record
}

func cloneProtoEvent(event *pb.Event) *pb.Event {
	if event == nil {
		return nil
	}
	cloned, ok := proto.Clone(event).(*pb.Event)
	if ok {
		return cloned
	}
	copy := *event
	return &copy
}

func normalizeEventEnvelope(envelope *pb.EventEnvelope, record CapturedEventRecord) *pb.EventEnvelope {
	if envelope == nil {
		return buildEventEnvelope(record)
	}
	cloned, ok := proto.Clone(envelope).(*pb.EventEnvelope)
	if !ok {
		cloned = envelope
	}
	if cloned.GetLegacyEvent() == nil && record.Event != nil {
		cloned.LegacyEvent = cloneProtoEvent(record.Event)
	}
	if strings.TrimSpace(cloned.GetSchemaVersion()) == "" {
		cloned.SchemaVersion = eventEnvelopeSchemaVersion
	}
	if cloned.GetTimestampNs() == 0 {
		timestamp := record.ReceivedAt.UTC()
		if timestamp.IsZero() {
			timestamp = time.Now().UTC()
		}
		cloned.TimestampNs = uint64(timestamp.UnixNano())
	}
	if strings.TrimSpace(cloned.GetSource()) == "" {
		cloned.Source = determineEventEnvelopeSource(record.Event)
	}
	if strings.TrimSpace(cloned.GetEventId()) == "" {
		cloned.EventId = buildEventEnvelopeID(record, firstNonNilEvent(record.Event, cloned.GetLegacyEvent()))
	}
	return cloned
}

func firstNonNilEvent(candidates ...*pb.Event) *pb.Event {
	for _, candidate := range candidates {
		if candidate != nil {
			return candidate
		}
	}
	return nil
}

func buildEventEnvelope(record CapturedEventRecord) *pb.EventEnvelope {
	event := record.Event
	if event == nil {
		return nil
	}
	event = cloneProtoEvent(event)
	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	envelope := &pb.EventEnvelope{
		SchemaVersion:  eventEnvelopeSchemaVersion,
		TimestampNs:    uint64(timestamp.UnixNano()),
		Source:         determineEventEnvelopeSource(event),
		AgentRunId:     event.GetAgentRunId(),
		TaskId:         event.GetTaskId(),
		ConversationId: event.GetConversationId(),
		TurnId:         event.GetTurnId(),
		ToolCallId:     event.GetToolCallId(),
		ToolName:       event.GetToolName(),
		TraceId:        event.GetTraceId(),
		SpanId:         event.GetSpanId(),
		Pid:            event.GetPid(),
		Tgid:           tgidOrPid(event),
		Ppid:           event.GetPpid(),
		Uid:            event.GetUid(),
		Gid:            event.GetGid(),
		Comm:           event.GetComm(),
		ArgvDigest:     event.GetArgvDigest(),
		Cwd:            event.GetCwd(),
		CgroupId:       event.GetCgroupId(),
		ContainerId:    event.GetContainerId(),
		PolicyDecision: event.GetDecision(),
		RiskScore:      event.GetRiskScore(),
		EventType:      event.GetEventType(),
		LegacyEvent:    event,
	}
	envelope.EventId = buildEventEnvelopeID(record, event)

	switch {
	case event.GetType() == "wrapper_intercept":
		envelope.Payload = &pb.EventEnvelope_WrapperEvent{WrapperEvent: buildWrapperEnvelopePayload(event)}
	case event.GetType() == "native_hook":
		envelope.Payload = &pb.EventEnvelope_HookEvent{HookEvent: buildHookEnvelopePayload(event)}
	case strings.HasPrefix(event.GetType(), "mcp"):
		envelope.Payload = &pb.EventEnvelope_McpEvent{McpEvent: buildMCPEnvelopePayload(event)}
	case buildProcessEnvelopePayload(event) != nil:
		envelope.Payload = &pb.EventEnvelope_ProcessEvent{ProcessEvent: buildProcessEnvelopePayload(event)}
	case buildNetworkEnvelopePayload(event) != nil:
		envelope.Payload = &pb.EventEnvelope_NetworkEvent{NetworkEvent: buildNetworkEnvelopePayload(event)}
	case event.GetType() == "execve":
		envelope.Payload = &pb.EventEnvelope_ExecEvent{ExecEvent: buildExecEnvelopePayload(event)}
	case buildFileEnvelopePayload(event) != nil:
		envelope.Payload = &pb.EventEnvelope_FileEvent{FileEvent: buildFileEnvelopePayload(event)}
	case event.GetType() == "semantic_alert" || strings.TrimSpace(event.GetDecision()) != "":
		envelope.Payload = &pb.EventEnvelope_PolicyEvent{PolicyEvent: buildPolicyEnvelopePayload(event)}
	}

	return envelope
}

func determineEventEnvelopeSource(event *pb.Event) string {
	if event == nil {
		return "unknown"
	}
	switch event.GetType() {
	case "wrapper_intercept":
		return "wrapper"
	case "native_hook":
		return "native_hook"
	case "semantic_alert":
		return "semantic_alert"
	default:
		if strings.HasPrefix(event.GetType(), "mcp") {
			return "mcp"
		}
		return "ebpf_ringbuf"
	}
}

func buildEventEnvelopeID(record CapturedEventRecord, event *pb.Event) string {
	if event == nil {
		return ""
	}
	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Unix(0, 0).UTC()
	}
	parts := []string{
		strconvFormatInt(timestamp.UnixNano()),
		determineEventEnvelopeSource(event),
		event.GetType(),
		strconvFormatUint32(event.GetPid()),
		strconvFormatUint32(event.GetPpid()),
		event.GetComm(),
		event.GetPath(),
		event.GetNetEndpoint(),
		event.GetTraceId(),
		event.GetToolCallId(),
		event.GetDecision(),
		event.GetExtraInfo(),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "evt_" + hex.EncodeToString(sum[:12])
}

func buildExecEnvelopePayload(event *pb.Event) *pb.ExecEvent {
	return &pb.ExecEvent{
		Path:       event.GetPath(),
		Retval:     event.GetRetval(),
		DurationNs: event.GetDurationNs(),
		ExtraInfo:  event.GetExtraInfo(),
		ArgvDigest: event.GetArgvDigest(),
		Cwd:        event.GetCwd(),
	}
}

func buildFileEnvelopePayload(event *pb.Event) *pb.FileEvent {
	if event == nil {
		return nil
	}
	switch event.GetType() {
	case "openat", "open", "read", "write", "chmod", "chown", "rename", "link", "symlink", "mknod", "mkdir", "unlink", "unlinkat":
		return &pb.FileEvent{
			Operation: event.GetType(),
			Path:      event.GetPath(),
			ExtraPath: event.GetExtraPath(),
			Mode:      event.GetMode(),
			Bytes:     event.GetBytes(),
			UidArg:    event.GetUidArg(),
			GidArg:    event.GetGidArg(),
			Retval:    event.GetRetval(),
			ExtraInfo: event.GetExtraInfo(),
		}
	default:
		return nil
	}
}

func buildNetworkEnvelopePayload(event *pb.Event) *pb.NetworkEvent {
	if event == nil {
		return nil
	}
	switch event.GetType() {
	case "network_connect", "network_bind", "network_sendto", "network_recvfrom", "socket", "accept", "accept4":
		return &pb.NetworkEvent{
			Endpoint:  event.GetNetEndpoint(),
			Direction: event.GetNetDirection(),
			Bytes:     event.GetNetBytes(),
			Family:    event.GetNetFamily(),
			Domain:    event.GetDomain(),
			SockType:  event.GetSockType(),
			Protocol:  event.GetProtocol(),
			Retval:    event.GetRetval(),
			ExtraInfo: event.GetExtraInfo(),
		}
	default:
		return nil
	}
}

func buildProcessEnvelopePayload(event *pb.Event) *pb.ProcessEvent {
	if event == nil {
		return nil
	}
	payload := &pb.ProcessEvent{
		ParentPid: event.GetPpid(),
		ExtraInfo: event.GetExtraInfo(),
	}
	switch event.GetType() {
	case "process_fork":
		payload.Phase = "fork"
		payload.ChildPid = parseUintField(event.GetExtraInfo(), "child_pid")
	case "clone":
		payload.Phase = "clone"
		payload.ChildPid = parseUintField(event.GetExtraInfo(), "child_pid")
	case "process_exec":
		payload.Phase = "exec"
		payload.OldPid = parseUintField(event.GetExtraInfo(), "old_pid")
	case "process_exit", "exit":
		payload.Phase = "exit"
		payload.ExitStatus = int32(parseUintField(event.GetExtraInfo(), "status"))
	case "wait4":
		payload.Phase = "wait4"
		payload.TargetPid = parseUintField(event.GetExtraInfo(), "target_pid")
	default:
		return nil
	}
	return payload
}

func buildPolicyEnvelopePayload(event *pb.Event) *pb.PolicyEvent {
	if event == nil {
		return nil
	}
	return &pb.PolicyEvent{
		Decision:        event.GetDecision(),
		RiskScore:       event.GetRiskScore(),
		Reason:          event.GetExtraInfo(),
		RelatedPath:     firstNonEmpty(event.GetPath(), event.GetExtraPath()),
		RelatedEndpoint: event.GetNetEndpoint(),
	}
}

func buildWrapperEnvelopePayload(event *pb.Event) *pb.WrapperEvent {
	if event == nil {
		return nil
	}
	commandLine := strings.TrimSpace(event.GetPath())
	parts := splitEnvelopeCommandLine(commandLine)
	args := []string{}
	if len(parts) > 1 {
		args = parts[1:]
	}
	return &pb.WrapperEvent{
		CommandLine: commandLine,
		Args:        args,
		Behavior:    event.GetBehavior(),
		ExtraInfo:   event.GetExtraInfo(),
		ToolName:    event.GetToolName(),
	}
}

func buildHookEnvelopePayload(event *pb.Event) *pb.HookEvent {
	if event == nil {
		return nil
	}
	hookName := event.GetComm()
	toolName := event.GetToolName()
	if before, after, ok := strings.Cut(event.GetComm(), ":"); ok {
		hookName = before
		if strings.TrimSpace(toolName) == "" {
			toolName = after
		}
	}
	return &pb.HookEvent{
		HookName:   strings.TrimSpace(hookName),
		ToolName:   strings.TrimSpace(toolName),
		TargetPath: firstNonEmpty(event.GetPath(), event.GetExtraPath()),
		ExtraInfo:  event.GetExtraInfo(),
	}
}

func buildMCPEnvelopePayload(event *pb.Event) *pb.McpEvent {
	if event == nil {
		return nil
	}
	return &pb.McpEvent{
		ToolName:  event.GetToolName(),
		Endpoint:  firstNonEmpty(event.GetNetEndpoint(), event.GetPath()),
		ExtraInfo: event.GetExtraInfo(),
	}
}

func splitEnvelopeCommandLine(commandLine string) []string {
	if strings.TrimSpace(commandLine) == "" {
		return nil
	}
	return strings.Fields(commandLine)
}

func buildCapturedEventJSONRecords(records []CapturedEventRecord) []map[string]any {
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		record = normalizeCapturedEventRecord(record)
		items = append(items, map[string]any{
			"Event":     record.Event,
			"Timestamp": record.ReceivedAt.UnixMilli(),
			"Envelope":  eventEnvelopeToJSONValue(record.Envelope),
		})
	}
	return items
}

func eventEnvelopeToJSONValue(envelope *pb.EventEnvelope) map[string]any {
	if envelope == nil {
		return nil
	}
	payload, err := eventEnvelopeJSONMarshaller.Marshal(envelope)
	if err != nil {
		return map[string]any{"error": err.Error()}
	}
	decoded := make(map[string]any)
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return map[string]any{"error": err.Error()}
	}
	return decoded
}

func strconvFormatUint32(value uint32) string {
	return fmt.Sprintf("%d", value)
}

func strconvFormatInt(value int64) string {
	return fmt.Sprintf("%d", value)
}

func tgidOrPid(event *pb.Event) uint32 {
	if tgid := event.GetTgid(); tgid != 0 {
		return tgid
	}
	return event.GetPid()
}
