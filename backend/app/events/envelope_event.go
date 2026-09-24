package events

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const eventEnvelopeSchemaVersion = "envelope.v1"

var EnvelopeJSONMarshaller = protojson.MarshalOptions{
	UseProtoNames:   true,
	EmitUnpopulated: false,
}

func NormalizeCapturedEventRecord(record CapturedEventRecord) CapturedEventRecord {
	if record.Event == nil && record.Envelope != nil && record.Envelope.GetLegacyEvent() != nil {
		record.Event = CloneProtoEvent(record.Envelope.GetLegacyEvent())
	}
	if record.Envelope == nil {
		record.Envelope = buildEventEnvelope(record)
	} else {
		record.Envelope = normalizeEventEnvelope(record.Envelope, record)
	}
	return record
}

func CloneProtoEvent(event *pb.Event) *pb.Event {
	if event == nil {
		return nil
	}
	cloned, ok := proto.Clone(event).(*pb.Event)
	if ok {
		return cloned
	}
	return event
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
		cloned.LegacyEvent = CloneProtoEvent(record.Event)
	}
	if strings.TrimSpace(cloned.GetSchemaVersion()) == "" {
		cloned.SchemaVersion = eventEnvelopeSchemaVersion
	}
	legacy := firstNonNilEvent(record.Event, cloned.GetLegacyEvent())
	if cloned.GetTimestampNs() == 0 {
		if legacy != nil && legacy.GetCaptureTimestampNs() != 0 {
			cloned.TimestampNs = legacy.GetCaptureTimestampNs()
		} else {
			timestamp := record.ReceivedAt.UTC()
			if timestamp.IsZero() {
				timestamp = time.Now().UTC()
			}
			cloned.TimestampNs = uint64(timestamp.UnixNano())
		}
	}
	if legacy != nil {
		if cloned.GetKernelTimestampNs() == 0 {
			cloned.KernelTimestampNs = legacy.GetKernelTimestampNs()
		}
		if cloned.GetKernelSequence() == 0 {
			cloned.KernelSequence = legacy.GetKernelSequence()
		}
		if cloned.GetKernelCpu() == 0 {
			cloned.KernelCpu = legacy.GetKernelCpu()
		}
		if strings.TrimSpace(cloned.GetKernelClock()) == "" {
			cloned.KernelClock = legacy.GetKernelClock()
		}
		if cloned.GetIngestTimestampNs() == 0 {
			cloned.IngestTimestampNs = legacy.GetIngestTimestampNs()
		}
		if cloned.GetCaptureDelayNs() == 0 {
			cloned.CaptureDelayNs = legacy.GetCaptureDelayNs()
		}
		if cloned.GetCaptureTimestampNs() == 0 {
			cloned.CaptureTimestampNs = legacy.GetCaptureTimestampNs()
		}
		if cloned.GetAuditFlags() == 0 {
			cloned.AuditFlags = legacy.GetAuditFlags()
		}
		if cloned.GetKernelAuditGeneration() == 0 {
			cloned.KernelAuditGeneration = legacy.GetKernelAuditGeneration()
		}
		if cloned.GetKernelDroppedSinceLast() == 0 {
			cloned.KernelDroppedSinceLast = legacy.GetKernelDroppedSinceLast()
		}
		if cloned.GetKernelReserveFailuresTotal() == 0 {
			cloned.KernelReserveFailuresTotal = legacy.GetKernelReserveFailuresTotal()
		}
	}
	if strings.TrimSpace(cloned.GetSource()) == "" {
		cloned.Source = DetermineEnvelopeSource(record.Event)
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

// buildEventEnvelope derives the envelope for record.Event. The envelope's
// LegacyEvent shares record.Event rather than cloning it: a captured record
// already owns a private copy of the event, both views are only ever
// serialised together, and redaction (redactCapturedEventRecord) is applied
// to the shared object exactly once.
func buildEventEnvelope(record CapturedEventRecord) *pb.EventEnvelope {
	event := record.Event
	if event == nil {
		return nil
	}
	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	timestampNS := uint64(timestamp.UnixNano())
	if event.GetCaptureTimestampNs() != 0 {
		timestampNS = event.GetCaptureTimestampNs()
	}
	envelope := &pb.EventEnvelope{
		SchemaVersion:              eventEnvelopeSchemaVersion,
		TimestampNs:                timestampNS,
		Source:                     DetermineEnvelopeSource(event),
		AgentRunId:                 event.GetAgentRunId(),
		TaskId:                     event.GetTaskId(),
		ConversationId:             event.GetConversationId(),
		TurnId:                     event.GetTurnId(),
		ToolCallId:                 event.GetToolCallId(),
		ToolName:                   event.GetToolName(),
		TraceId:                    event.GetTraceId(),
		SpanId:                     event.GetSpanId(),
		Pid:                        event.GetPid(),
		Tgid:                       tgidOrPid(event),
		Ppid:                       event.GetPpid(),
		Uid:                        event.GetUid(),
		Gid:                        event.GetGid(),
		Comm:                       event.GetComm(),
		ArgvDigest:                 event.GetArgvDigest(),
		Cwd:                        event.GetCwd(),
		CgroupId:                   event.GetCgroupId(),
		ContainerId:                event.GetContainerId(),
		PolicyDecision:             event.GetDecision(),
		RiskScore:                  event.GetRiskScore(),
		EventType:                  event.GetEventType(),
		KernelTimestampNs:          event.GetKernelTimestampNs(),
		KernelSequence:             event.GetKernelSequence(),
		KernelCpu:                  event.GetKernelCpu(),
		KernelClock:                event.GetKernelClock(),
		IngestTimestampNs:          event.GetIngestTimestampNs(),
		CaptureDelayNs:             event.GetCaptureDelayNs(),
		CaptureTimestampNs:         event.GetCaptureTimestampNs(),
		AuditFlags:                 event.GetAuditFlags(),
		KernelAuditGeneration:      event.GetKernelAuditGeneration(),
		KernelDroppedSinceLast:     event.GetKernelDroppedSinceLast(),
		KernelReserveFailuresTotal: event.GetKernelReserveFailuresTotal(),
		LegacyEvent:                event,
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
	case buildTLSEnvelopePayload(event) != nil:
		envelope.Payload = &pb.EventEnvelope_TlsEvent{TlsEvent: buildTLSEnvelopePayload(event)}
	case buildOTelSpanEnvelopePayload(event) != nil:
		envelope.Payload = &pb.EventEnvelope_OtelSpanEvent{OtelSpanEvent: buildOTelSpanEnvelopePayload(event)}
	case buildStdioEnvelopePayload(event) != nil:
		envelope.Payload = &pb.EventEnvelope_StdioEvent{StdioEvent: buildStdioEnvelopePayload(event)}
	case buildSystemMetricEnvelopePayload(event) != nil:
		envelope.Payload = &pb.EventEnvelope_SystemMetricEvent{SystemMetricEvent: buildSystemMetricEnvelopePayload(event)}
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

func DetermineEnvelopeSource(event *pb.Event) string {
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

	// Kernel provenance is stable across persistence/replay and across
	// userspace scheduling delays. Prefer it whenever an explicit generation
	// and CPU-local sequence are available.
	if event.GetKernelAuditGeneration() != 0 && event.GetKernelSequence() != 0 {
		parts := []string{
			"kernel-v2",
			fmt.Sprintf("%d", event.GetKernelAuditGeneration()),
			strconvFormatUint32(event.GetKernelCpu()),
			fmt.Sprintf("%d", event.GetKernelSequence()),
			fmt.Sprintf("%d", event.GetKernelTimestampNs()),
			event.GetType(),
			strconvFormatUint32(event.GetPid()),
			strconvFormatUint32(tgidOrPid(event)),
		}
		sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
		return "evt_" + hex.EncodeToString(sum[:12])
	}

	timestamp := record.ReceivedAt.UTC()
	if timestamp.IsZero() {
		timestamp = time.Unix(0, 0).UTC()
	}
	// The hashed material is the NUL-joined field list; it is assembled in a
	// stack scratch buffer so typical events hash without allocating.
	var scratch [512]byte
	buf := strconv.AppendInt(scratch[:0], timestamp.UnixNano(), 10)
	buf = append(buf, 0)
	buf = append(buf, DetermineEnvelopeSource(event)...)
	buf = append(buf, 0)
	buf = append(buf, event.GetType()...)
	buf = append(buf, 0)
	buf = strconv.AppendUint(buf, uint64(event.GetPid()), 10)
	buf = append(buf, 0)
	buf = strconv.AppendUint(buf, uint64(event.GetPpid()), 10)
	for _, part := range [...]string{
		event.GetComm(),
		event.GetPath(),
		event.GetNetEndpoint(),
		event.GetTraceId(),
		event.GetToolCallId(),
		event.GetDecision(),
		event.GetExtraInfo(),
	} {
		buf = append(buf, 0)
		buf = append(buf, part...)
	}
	sum := sha256.Sum256(buf)
	var id [4 + 24]byte
	copy(id[:], "evt_")
	hex.Encode(id[4:], sum[:12])
	return string(id[:])
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
	case "network_connect", "network_bind", "network_sendto", "network_recvfrom", "socket", "accept", "accept4", "tcp_connect", "tcp_close", "tcp_state_change", "dns_query":
		return &pb.NetworkEvent{
			Endpoint:       event.GetNetEndpoint(),
			Direction:      event.GetNetDirection(),
			Bytes:          event.GetNetBytes(),
			Family:         event.GetNetFamily(),
			Domain:         event.GetDomain(),
			SockType:       event.GetSockType(),
			Protocol:       event.GetProtocol(),
			Retval:         event.GetRetval(),
			ExtraInfo:      event.GetExtraInfo(),
			FlowId:         event.GetFlowId(),
			SrcIp:          event.GetSrcIp(),
			SrcPort:        event.GetSrcPort(),
			DstIp:          event.GetDstIp(),
			DstPort:        event.GetDstPort(),
			Transport:      event.GetTransport(),
			AppProtocol:    event.GetAppProtocol(),
			ServiceName:    event.GetServiceName(),
			DnsName:        event.GetDnsName(),
			Sni:            event.GetSni(),
			HttpHost:       event.GetHttpHost(),
			TlsAlpn:        event.GetTlsAlpn(),
			QuicState:      event.GetQuicState(),
			InterfaceName:  event.GetInterfaceName(),
			BytesIn:        event.GetBytesIn(),
			BytesOut:       event.GetBytesOut(),
			PacketsIn:      event.GetPacketsIn(),
			PacketsOut:     event.GetPacketsOut(),
			FirstSeenMs:    event.GetFirstSeenMs(),
			LastSeenMs:     event.GetLastSeenMs(),
			StaleLevel:     event.GetStaleLevel(),
			Historic:       event.GetHistoric(),
			GeoCountry:     event.GetGeoCountry(),
			GeoCountryCode: event.GetGeoCountryCode(),
			GeoAsn:         event.GetGeoAsn(),
			IpScope:        event.GetIpScope(),
		}
	default:
		return nil
	}
}

func buildOTelSpanEnvelopePayload(event *pb.Event) *pb.OtelSpanEvent {
	if event == nil || event.GetType() != "otel_span" {
		return nil
	}
	vendor := platform.FirstNonEmpty(event.GetServiceName(), platform.ParseStringField(event.GetExtraInfo(), "provider"))
	status := "ok"
	if event.GetDecision() == "ALERT" {
		status = "error"
	}
	return &pb.OtelSpanEvent{
		Name:     platform.FirstNonEmpty(event.GetToolName(), vendor, "genai.request"),
		Kind:     "client",
		Status:   status,
		Provider: vendor,
		Model:    platform.ParseStringField(event.GetExtraInfo(), "model"),
		Error:    platform.ParseStringField(event.GetExtraInfo(), "error"),
	}
}

func buildTLSEnvelopePayload(event *pb.Event) *pb.TLSEvent {
	if event == nil || event.GetType() != "tls_plaintext" {
		return nil
	}
	status := platform.ParseUintField(event.GetExtraInfo(), "status")
	return &pb.TLSEvent{
		Direction:      event.GetNetDirection(),
		Library:        platform.ParseStringField(event.GetExtraInfo(), "lib"),
		Host:           platform.FirstNonEmpty(event.GetHttpHost(), event.GetNetEndpoint()),
		Method:         event.GetPath(),
		Url:            event.GetExtraPath(),
		Status:         status,
		BodySize:       uint64(platform.ParseUintField(event.GetExtraInfo(), "body_size")),
		RedactionState: "sanitized",
		RawAvailable:   false,
		MessageRole:    platform.ParseStringField(event.GetExtraInfo(), "role"),
		PromptDigest:   platform.ParseStringField(event.GetExtraInfo(), "prompt_digest"),
		PromptLen:      uint64(platform.ParseUintField(event.GetExtraInfo(), "prompt_len")),
		Vendor:         platform.FirstNonEmpty(event.GetServiceName(), platform.ParseStringField(event.GetExtraInfo(), "vendor")),
		CaptureSource:  event.GetCaptureSource(),
		AppProtocol:    event.GetAppProtocol(),
		RequestPath:    event.GetHttpPath(),
		ApiProfile:     event.GetApiProfile(),
		ApiProduct:     event.GetApiProduct(),
		ApiOperation:   event.GetApiOperation(),
		ApiConfidence:  event.GetApiConfidence(),
	}
}

func buildStdioEnvelopePayload(event *pb.Event) *pb.StdioEvent {
	if event == nil || event.GetType() != "stdio" {
		return nil
	}
	stream := platform.FirstNonEmpty(platform.ParseStringField(event.GetExtraInfo(), "stream"), event.GetPath())
	return &pb.StdioEvent{
		Fd:             platform.ParseStringField(event.GetExtraInfo(), "fd"),
		Stream:         stream,
		Size:           event.GetBytes(),
		Truncated:      false,
		Binary:         false,
		RedactionState: "metadata_only",
	}
}

func buildSystemMetricEnvelopePayload(event *pb.Event) *pb.SystemMetricEvent {
	if event == nil || event.GetType() != "system_metric" {
		return nil
	}
	return &pb.SystemMetricEvent{
		CpuPercent:   platform.ParseFloatField(event.GetExtraInfo(), "cpu_percent"),
		MemoryBytes:  event.GetBytes(),
		ProcessState: platform.ParseStringField(event.GetExtraInfo(), "state"),
		Alert:        platform.ParseStringField(event.GetExtraInfo(), "alert"),
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
		payload.ChildPid = platform.ParseUintField(event.GetExtraInfo(), "child_pid")
	case "clone":
		payload.Phase = "clone"
		payload.ChildPid = platform.ParseUintField(event.GetExtraInfo(), "child_pid")
	case "process_exec":
		payload.Phase = "exec"
		payload.OldPid = platform.ParseUintField(event.GetExtraInfo(), "old_pid")
	case "process_exit", "exit":
		payload.Phase = "exit"
		payload.ExitStatus = int32(platform.ParseUintField(event.GetExtraInfo(), "status"))
	case "wait4":
		payload.Phase = "wait4"
		payload.TargetPid = platform.ParseUintField(event.GetExtraInfo(), "target_pid")
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
		RelatedPath:     platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath()),
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
		TargetPath: platform.FirstNonEmpty(event.GetPath(), event.GetExtraPath()),
		ExtraInfo:  event.GetExtraInfo(),
	}
}

func buildMCPEnvelopePayload(event *pb.Event) *pb.McpEvent {
	if event == nil {
		return nil
	}
	return &pb.McpEvent{
		ToolName:  event.GetToolName(),
		Endpoint:  platform.FirstNonEmpty(event.GetNetEndpoint(), event.GetPath()),
		ExtraInfo: event.GetExtraInfo(),
	}
}

func splitEnvelopeCommandLine(commandLine string) []string {
	if strings.TrimSpace(commandLine) == "" {
		return nil
	}
	return strings.Fields(commandLine)
}

func BuildCapturedEventJSONRecords(records []CapturedEventRecord) []map[string]any {
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		record = NormalizeCapturedEventRecord(record)
		items = append(items, map[string]any{
			"Event":     record.Event,
			"Timestamp": record.ReceivedAt.UnixMilli(),
			"Envelope":  EnvelopeToJSONValue(record.Envelope),
		})
	}
	return items
}

func EnvelopeToJSONValue(envelope *pb.EventEnvelope) map[string]any {
	if envelope == nil {
		return nil
	}
	payload, err := EnvelopeJSONMarshaller.Marshal(envelope)
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
	return strconv.FormatUint(uint64(value), 10)
}

func strconvFormatInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

func tgidOrPid(event *pb.Event) uint32 {
	if tgid := event.GetTgid(); tgid != 0 {
		return tgid
	}
	return event.GetPid()
}
