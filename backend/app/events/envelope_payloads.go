package events

import (
	"agent-ebpf-filter/app/platform"
	"agent-ebpf-filter/pb"
	"strings"
)

// setEnvelopePayload picks the typed payload for event. The candidates are
// tried in priority order and each is built at most once.
func setEnvelopePayload(envelope *pb.EventEnvelope, event *pb.Event) {
	switch eventType := event.GetType(); {
	case eventType == "wrapper_intercept":
		envelope.Payload = &pb.EventEnvelope_WrapperEvent{WrapperEvent: buildWrapperEnvelopePayload(event)}
		return
	case eventType == "native_hook":
		envelope.Payload = &pb.EventEnvelope_HookEvent{HookEvent: buildHookEnvelopePayload(event)}
		return
	case strings.HasPrefix(eventType, "mcp"):
		envelope.Payload = &pb.EventEnvelope_McpEvent{McpEvent: buildMCPEnvelopePayload(event)}
		return
	}
	if payload := buildProcessEnvelopePayload(event); payload != nil {
		envelope.Payload = &pb.EventEnvelope_ProcessEvent{ProcessEvent: payload}
		return
	}
	if payload := buildTLSEnvelopePayload(event); payload != nil {
		envelope.Payload = &pb.EventEnvelope_TlsEvent{TlsEvent: payload}
		return
	}
	if payload := buildOTelSpanEnvelopePayload(event); payload != nil {
		envelope.Payload = &pb.EventEnvelope_OtelSpanEvent{OtelSpanEvent: payload}
		return
	}
	if payload := buildStdioEnvelopePayload(event); payload != nil {
		envelope.Payload = &pb.EventEnvelope_StdioEvent{StdioEvent: payload}
		return
	}
	if payload := buildSystemMetricEnvelopePayload(event); payload != nil {
		envelope.Payload = &pb.EventEnvelope_SystemMetricEvent{SystemMetricEvent: payload}
		return
	}
	if payload := buildNetworkEnvelopePayload(event); payload != nil {
		envelope.Payload = &pb.EventEnvelope_NetworkEvent{NetworkEvent: payload}
		return
	}
	if event.GetType() == "execve" {
		envelope.Payload = &pb.EventEnvelope_ExecEvent{ExecEvent: buildExecEnvelopePayload(event)}
		return
	}
	if payload := buildFileEnvelopePayload(event); payload != nil {
		envelope.Payload = &pb.EventEnvelope_FileEvent{FileEvent: payload}
		return
	}
	if event.GetType() == "semantic_alert" || strings.TrimSpace(event.GetDecision()) != "" {
		envelope.Payload = &pb.EventEnvelope_PolicyEvent{PolicyEvent: buildPolicyEnvelopePayload(event)}
	}
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
	// Decide before allocating: most events are not process lifecycle events.
	var phase string
	switch event.GetType() {
	case "process_fork":
		phase = "fork"
	case "clone":
		phase = "clone"
	case "process_exec":
		phase = "exec"
	case "process_exit", "exit":
		phase = "exit"
	case "wait4":
		phase = "wait4"
	default:
		return nil
	}
	payload := &pb.ProcessEvent{
		Phase:     phase,
		ParentPid: event.GetPpid(),
		ExtraInfo: event.GetExtraInfo(),
	}
	switch phase {
	case "fork", "clone":
		payload.ChildPid = platform.ParseUintField(event.GetExtraInfo(), "child_pid")
	case "exec":
		payload.OldPid = platform.ParseUintField(event.GetExtraInfo(), "old_pid")
	case "exit":
		payload.ExitStatus = int32(platform.ParseUintField(event.GetExtraInfo(), "status"))
	case "wait4":
		payload.TargetPid = platform.ParseUintField(event.GetExtraInfo(), "target_pid")
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
