package main

import (
	"sort"
	"strconv"
	"strings"
	"time"
)

// Proto EventEnvelope uses a oneof. Interpret the actual typed payload in its
// own namespace rather than conflating e.g. TLS "status" with a process status.
type eventTypedPayload struct {
	Kind   string
	Origin string
	Data   map[string]any
}

func eventDetailTypedPayload(detail map[string]any) eventTypedPayload {
	envelope, _ := mapValue(detail, "Envelope", "envelope").(map[string]any)
	for _, entry := range []struct{ key, kind string }{
		{"fileEvent", "file"}, {"networkEvent", "network"},
		{"execEvent", "process"}, {"processEvent", "process"},
		{"policyEvent", "policy"}, {"wrapperEvent", "wrapper"},
		{"hookEvent", "hook"}, {"mcpEvent", "mcp"},
		{"tlsEvent", "tls"}, {"httpEvent", "http"},
		{"sseEvent", "sse"}, {"stdioEvent", "stdio"},
		{"systemMetricEvent", "metric"}, {"otelSpanEvent", "otel"},
		{"agentsightAlertEvent", "alert"},
	} {
		if raw := mapValue(envelope, entry.key); raw != nil {
			if values, ok := raw.(map[string]any); ok {
				return eventTypedPayload{Kind: entry.kind, Origin: "Envelope." + entry.key, Data: values}
			}
		}
	}
	return eventTypedPayload{}
}

func eventTypedField(payload eventTypedPayload, label string, keys ...string) (eventDetailField, bool) {
	if payload.Data == nil {
		return eventDetailField{}, false
	}
	value, origin, ok := eventDetailLookup(newTypedLayers(payload), keys...)
	if !ok {
		return eventDetailField{}, false
	}
	return eventDetailField{Label: label, Value: value, Origin: origin}, true
}

func newTypedLayers(payload eventTypedPayload) []eventDetailLayer {
	return []eventDetailLayer{newEventDetailLayer(payload.Origin, payload.Data)}
}

type eventDetailFieldSpec struct {
	Label string
	Keys  []string
}

func typedFields(payload eventTypedPayload, specs ...eventDetailFieldSpec) []eventDetailField {
	out := make([]eventDetailField, 0, len(specs))
	for _, spec := range specs {
		if field, ok := eventTypedField(payload, spec.Label, spec.Keys...); ok {
			out = append(out, field)
		}
	}
	return out
}

func eventTypedTarget(payload eventTypedPayload) string {
	var fields []string
	switch payload.Kind {
	case "file", "process":
		fields = []string{"path", "targetPid"}
	case "network":
		fields = []string{"endpoint", "dstIp", "domain", "dnsName", "sni"}
	case "http", "tls":
		fields = []string{"url", "host", "requestPath"}
	case "mcp":
		fields = []string{"endpoint", "serverName", "toolName"}
	case "hook":
		fields = []string{"targetPath", "toolName"}
	case "wrapper":
		fields = []string{"commandLine", "toolName"}
	case "policy":
		fields = []string{"relatedPath", "relatedEndpoint"}
	case "otel":
		fields = []string{"name", "model"}
	case "sse":
		fields = []string{"event"}
	case "stdio":
		fields = []string{"stream", "fd"}
	case "metric":
		fields = []string{"alert", "processState"}
	case "alert":
		fields = []string{"relatedEventId", "reason"}
	}
	value, _, _ := eventDetailLookupPreferred(newTypedLayers(payload), fields...)
	return value
}

func eventDetailTypedSection(payload eventTypedPayload) eventDetailSection {
	// An explicit typed section prevents data from another payload or a legacy
	// top-level field with a similarly named key from being presented as evidence.
	spec := func(label string, keys ...string) eventDetailFieldSpec {
		return eventDetailFieldSpec{Label: label, Keys: keys}
	}
	var title string
	var specs []eventDetailFieldSpec
	switch payload.Kind {
	case "file":
		title = "文件元数据"
		specs = []eventDetailFieldSpec{
			spec("操作", "operation"), spec("路径", "path"), spec("关联路径", "extraPath"),
			spec("模式", "mode"), spec("字节数", "bytes"), spec("目标 UID", "uidArg"),
			spec("目标 GID", "gidArg"), spec("返回值", "retval"), spec("备注", "extraInfo"),
		}
	case "network":
		title = "网络扩展"
		specs = []eventDetailFieldSpec{
			spec("服务", "serviceName"), spec("协议", "appProtocol"), spec("DNS", "dnsName"),
			spec("SNI", "sni"), spec("ALPN", "tlsAlpn"), spec("QUIC 状态", "quicState"),
			spec("网络接口", "interfaceName"), spec("入站包数", "packetsIn"),
			spec("出站包数", "packetsOut"), spec("首次出现 (ms)", "firstSeenMs"),
			spec("最近出现 (ms)", "lastSeenMs"), spec("过期等级", "staleLevel"),
			spec("历史流", "historic"), spec("地理国家", "geoCountry"),
			spec("ASN", "geoAsn"), spec("IP 范围", "ipScope"),
		}
	case "process":
		title = "进程扩展"
		specs = []eventDetailFieldSpec{
			spec("阶段", "phase"), spec("父 PID", "parentPid"), spec("子 PID", "childPid"),
			spec("目标 PID", "targetPid"), spec("旧 PID", "oldPid"),
			spec("退出码", "exitStatus"), spec("额外信息", "extraInfo"),
		}
	case "policy":
		title = "策略裁决"
		specs = []eventDetailFieldSpec{
			spec("决策", "decision"), spec("风险评分", "riskScore"),
			spec("原因", "reason"), spec("关联路径", "relatedPath"),
			spec("关联端点", "relatedEndpoint"),
		}
	case "wrapper":
		title = "Agent Wrapper 调用"
		specs = []eventDetailFieldSpec{
			spec("命令行", "commandLine"), spec("参数", "args"),
			spec("工具名称", "toolName"), spec("附加信息", "extraInfo"),
		}
	case "hook":
		title = "Agent Hook"
		specs = []eventDetailFieldSpec{
			spec("Hook 名称", "hookName"), spec("工具名称", "toolName"),
			spec("目标路径", "targetPath"), spec("附加信息", "extraInfo"),
		}
	case "mcp":
		title = "MCP 工具调用"
		specs = []eventDetailFieldSpec{
			spec("工具名称", "toolName"), spec("服务器", "serverName"),
			spec("端点", "endpoint"), spec("请求 ID", "requestId"),
			spec("附加信息", "extraInfo"),
		}
	case "tls":
		title = "TLS / LLM 请求元数据"
		specs = []eventDetailFieldSpec{
			spec("方向", "direction"), spec("TLS 库", "library"),
			spec("Host", "host"), spec("方法", "method"), spec("URL", "url"),
			spec("请求路径", "requestPath"), spec("HTTP 状态", "status"),
			spec("内容类型", "contentType"), spec("主体大小", "bodySize"),
			spec("已截断", "truncated"), spec("脱敏状态", "redactionState"),
			spec("原始内容可用", "rawAvailable"),
			spec("消息角色", "messageRole"), spec("提示摘要", "promptDigest"),
			spec("提示长度", "promptLen"), spec("厂商", "vendor"),
			spec("捕获来源", "captureSource"), spec("API 档案", "apiProfile"),
			spec("产品", "apiProduct"), spec("API 操作", "apiOperation"),
			spec("API 置信度", "apiConfidence"),
		}
	case "http":
		title = "HTTP 请求元数据"
		specs = []eventDetailFieldSpec{
			spec("方向", "direction"), spec("方法", "method"), spec("URL", "url"),
			spec("Host", "host"), spec("HTTP 状态", "status"),
			spec("内容类型", "contentType"), spec("主体大小", "bodySize"),
			spec("已截断", "truncated"), spec("脱敏状态", "redactionState"),
		}
	case "sse":
		title = "SSE 流事件"
		specs = []eventDetailFieldSpec{
			spec("事件", "event"), spec("数据摘要", "dataDigest"),
			spec("数据大小", "dataSize"), spec("已结束", "completed"),
			spec("脱敏状态", "redactionState"),
		}
	case "stdio":
		title = "标准输入输出"
		specs = []eventDetailFieldSpec{
			spec("文件描述符", "fd"), spec("流", "stream"),
			spec("大小", "size"), spec("已截断", "truncated"),
			spec("二进制", "binary"), spec("脱敏状态", "redactionState"),
		}
	case "metric":
		title = "进程性能指标"
		specs = []eventDetailFieldSpec{
			spec("CPU 使用率", "cpuPercent"), spec("内存字节", "memoryBytes"),
			spec("线程数", "threads"), spec("子进程数", "children"),
			spec("文件描述符数", "fdCount"), spec("进程状态", "processState"),
			spec("告警", "alert"),
		}
	case "otel":
		title = "OTel Span"
		specs = []eventDetailFieldSpec{
			spec("名称", "name"), spec("种类", "kind"), spec("状态", "status"),
			spec("Provider", "provider"), spec("模型", "model"),
			spec("延迟 (ms)", "latencyMs"), spec("输入 Token", "inputTokens"),
			spec("输出 Token", "outputTokens"), spec("错误", "error"),
		}
	case "alert":
		title = "AgentSight 风险告警"
		specs = []eventDetailFieldSpec{
			spec("类别", "category"), spec("严重性", "severity"),
			spec("原因", "reason"), spec("关联事件 ID", "relatedEventId"),
			spec("脱敏状态", "redactionState"),
		}
	}
	return eventDetailSection{Title: title, Fields: typedFields(payload, specs...)}
}

// This refers to a nearby cached observation, never proof that one event
// caused the next. PID reuse means a time window or an explicit run/session
// association is required before showing a related row.
type eventDetailRelatedItem struct {
	Event       eventSummary
	Relation    string
	DistanceMS  int64
}

func eventDetailRelatedCandidates(events []eventSummary, selected eventSummary, limit int) []eventDetailRelatedItem {
	if selected.PID <= 0 || limit <= 0 {
		return nil
	}
	out := make([]eventDetailRelatedItem, 0, min(limit, 8))
	for _, e := range events {
		if e.EventID == "" || e.EventID == selected.EventID || e.PID != selected.PID {
			continue
		}
		var distance int64
		if e.ReceivedAtMS != 0 && selected.ReceivedAtMS != 0 {
			distance = e.ReceivedAtMS - selected.ReceivedAtMS
			if distance < 0 {
				distance = -distance
			}
		}
		relation := ""
		switch {
		case selected.AgentRunID != "" && e.AgentRunID == selected.AgentRunID:
			relation = "同 Agent Run"
		case selected.ConversationID != "" && e.ConversationID == selected.ConversationID:
			relation = "同会话"
		case e.ReceivedAtMS != 0 && selected.ReceivedAtMS != 0 && distance <= (2*time.Minute).Milliseconds():
			relation = "同 PID · 两分钟内"
		default:
			// No evidence of either a bounded time window or a stable run.
			continue
		}
		out = append(out, eventDetailRelatedItem{Event: e, Relation: relation, DistanceMS: distance})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Relation != out[j].Relation {
			// Explicitly correlated Agent run/session precedes a PID-only row.
			rank := func(name string) int {
				switch name {
				case "同 Agent Run": return 0
				case "同会话": return 1
				default: return 2
				}
			}
			return rank(out[i].Relation) < rank(out[j].Relation)
		}
		if out[i].DistanceMS != out[j].DistanceMS {
			return out[i].DistanceMS < out[j].DistanceMS
		}
		return out[i].Event.EventID < out[j].Event.EventID
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// A monotonic timestamp is a duration since boot, not Unix nanoseconds.
// Never render it as a 1970 calendar date. The top-level persisted Timestamp
// (milliseconds) is authoritative whenever present.
func isPlausibleUnixNanoseconds(value int64) bool {
	return value >= time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano() &&
		value <= time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC).UnixNano()
}

// Protobuf JSON emits unsigned 64-bit values as decimal strings; preserve the
// full integer when deriving process ids and calendar times.
func eventDetailInt64(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil
}
