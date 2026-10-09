package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// The detail endpoint returns a legacy Event plus a richer protobuf Envelope.
// Read them together without materialising additional requests or inventing
// values. A field's origin stays visible so analysts can verify its evidence.
type eventDetailField struct {
	Label  string
	Value  string
	Origin string
}

type eventDetailSection struct {
	Title  string
	Fields []eventDetailField
}

type eventDetailViewModel struct {
	Type     string
	Action   string
	Target   string
	Decision string
	Risk     string
	When     string
	WhenLabel string
	EvidenceNote string
	NoTargetExpected bool
	Sections []eventDetailSection
}

type eventDetailLayer struct {
	Path   string
	Values map[string]any
	Keys   []string
}

func newEventDetailLayer(path string, values map[string]any) eventDetailLayer {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return eventDetailLayer{Path: path, Values: values, Keys: keys}
}

func eventDetailKey(key string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '-' || r == ' ' {
			return -1
		}
		return r
	}, strings.ToLower(key))
}

func eventDetailLookup(layers []eventDetailLayer, keys ...string) (string, string, bool) {
	for _, layer := range layers {
		keysInLayer := layer.Keys
		if keysInLayer == nil {
			keysInLayer = newEventDetailLayer(layer.Path, layer.Values).Keys
		}
		// Prefer an exact key before fuzzy aliases; map iteration order
		// must never decide which provenance is displayed to the analyst.
		for _, name := range keys {
			if raw, ok := layer.Values[name]; ok {
				if value := detailScalarText(raw); value != "" {
					return value, layer.Path + "." + name, true
				}
			}
			for _, key := range keysInLayer {
				if eventDetailKey(key) == eventDetailKey(name) {
					if value := detailScalarText(layer.Values[key]); value != "" {
						return value, layer.Path + "." + key, true
					}
				}
			}
		}
	}
	return "", "", false
}

// Choose a semantic field first across all sources; otherwise a lower
// priority legacy field (e.g. dstIp) could hide a typed endpoint with a port.
func eventDetailLookupPreferred(layers []eventDetailLayer, keys ...string) (string, string, bool) {
	for _, key := range keys {
		if value, origin, ok := eventDetailLookup(layers, key); ok {
			return value, origin, true
		}
	}
	return "", "", false
}

func detailScalarText(raw any) string {
	switch v := raw.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case bool, int, int64, uint64, json.Number:
		return fmt.Sprint(v)
	default:
		b, err := json.Marshal(v)
		if err == nil {
			return string(b)
		}
		return ""
	}
}

func eventDetailCategory(detail map[string]any, eventType string) string {
	if typed := eventDetailTypedPayload(detail); typed.Kind != "" {
		return typed.Kind
	}
	name := strings.ToLower(strings.TrimSpace(eventType))
	switch {
	case strings.HasPrefix(name, "file_"):
		return "file"
	case strings.HasPrefix(name, "network_"), strings.HasPrefix(name, "net_"):
		return "network"
	case strings.HasPrefix(name, "process_"):
		return "process"
	}
	// Exact syscall stems, not substring matches ("openai_request" is NOT open).
	for _, item := range []struct {
		kind string
		stems []string
	}{
		{"file", []string{"read", "write", "open", "creat", "unlink", "rename", "chmod", "chown", "mkdir", "rmdir", "truncate", "fsync", "lseek", "stat"}},
		{"network", []string{"connect", "accept", "bind", "listen", "send", "recv", "socket", "dns", "tls", "http", "quic"}},
		{"process", []string{"exec", "fork", "clone", "vfork", "exit", "kill", "wait", "ptrace"}},
	} {
		for _, stem := range item.stems {
			if name == stem || strings.HasPrefix(name, stem+"_") || strings.HasPrefix(name, stem+"at") ||
				strings.HasPrefix(name, stem+"ve") || name == stem+"msg" || name == stem+"to" || name == stem+"from" {
				return item.kind
			}
		}
	}
	return "other"
}

// Avoid legacy substring classification in the visible headline: an event
// named "openai_request" is not evidence that a file was opened.
func eventDetailAction(detail map[string]any, eventType string) string {
	kind := eventDetailCategory(detail, eventType)
	switch kind {
	case "file":
		typ := strings.ToLower(strings.TrimSpace(eventType))
		for _, verb := range []string{"write", "rename", "unlink", "rmdir", "mkdir", "truncate", "chmod", "chown", "creat"} {
			if typ == verb || strings.HasPrefix(typ, verb+"_") || strings.HasPrefix(typ, "file_"+verb) {
				return "修改文件"
			}
		}
		if typ == "read" || strings.HasPrefix(typ, "read") || strings.HasPrefix(typ, "open") {
			return "读取文件"
		}
		return "文件活动"
	case "network":
		return "网络活动"
	case "process":
		return "进程活动"
	case "policy":
		return "策略裁决"
	case "wrapper":
		return "Agent 命令执行"
	case "hook":
		return "Agent Hook 回调"
	case "mcp":
		return "MCP 工具调用"
	case "tls":
		return "TLS / LLM 请求"
	case "http":
		return "HTTP 请求"
	case "sse":
		return "SSE 流事件"
	case "stdio":
		return "标准输入输出"
	case "metric":
		return "进程性能指标"
	case "otel":
		return "OTel Span"
	case "alert":
		return "AgentSight 风险告警"
	default:
		if eventType != "" {
			return eventType
		}
		return "未分类事件"
	}
}

func eventDetailPreview(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…（可复制完整值）"
}

func eventDetailLayers(detail map[string]any) []eventDetailLayer {
	layers := make([]eventDetailLayer, 0, 4)
	if detail == nil {
		return layers
	}
	if event := detailRecord(detail); event != nil {
		prefix := "Event"
		if _, ok := detail["Event"]; !ok {
			if _, ok := detail["event"]; !ok {
				prefix = "$"
			} else {
				prefix = "event"
			}
		}
		layers = append(layers, newEventDetailLayer(prefix, event))
		if behavior, ok := mapValue(event, "behavior").(map[string]any); ok {
			layers = append(layers, newEventDetailLayer(prefix+".behavior", behavior))
		}
	}
	envelope, _ := mapValue(detail, "Envelope", "envelope").(map[string]any)
	if envelope != nil {
		// Payload fields take precedence only if the legacy event lacks them.
		for _, name := range []string{
			"fileEvent", "networkEvent", "execEvent", "processEvent",
			"policyEvent", "wrapperEvent", "hookEvent", "mcpEvent",
			"tlsEvent", "httpEvent", "sseEvent", "stdioEvent",
			"systemMetricEvent", "otelSpanEvent", "agentsightAlertEvent",
		} {
			if nested, ok := mapValue(envelope, name).(map[string]any); ok {
				layers = append(layers, newEventDetailLayer("Envelope." + name, nested))
				if eventDetailKey(name) == "wrapperevent" {
					if behavior, ok := mapValue(nested, "behavior").(map[string]any); ok {
						layers = append(layers, newEventDetailLayer("Envelope."+name+".behavior", behavior))
					}
				}
			}
		}
		layers = append(layers, newEventDetailLayer("Envelope", envelope))
	}
	return layers
}

func appendDetailField(dst *[]eventDetailField, layers []eventDetailLayer, label string, keys ...string) {
	if value, origin, ok := eventDetailLookup(layers, keys...); ok {
		*dst = append(*dst, eventDetailField{label, value, origin})
	}
}

func eventDetailModel(detail map[string]any) eventDetailViewModel {
	layers := eventDetailLayers(detail)
	get := func(keys ...string) string {
		value, _, _ := eventDetailLookup(layers, keys...)
		return value
	}
	prefer := func(keys ...string) string {
		value, _, _ := eventDetailLookupPreferred(layers, keys...)
		return value
	}
	eventType := get("type")
	semanticAlert := eventType == "semantic_alert"
	m := eventDetailViewModel{Type: eventType, Action: eventDetailAction(detail, eventType), Decision: get("decision", "policyDecision")}
	if semanticAlert {
		m.Action = "语义风险告警"
	}
	if value, _, ok := eventDetailLookup(layers, "riskScore", "risk_score"); ok {
		m.Risk = value
	}
	if millis, _, ok := eventDetailLookup([]eventDetailLayer{newEventDetailLayer("$", detail)}, "Timestamp"); ok {
		if n, err := strconv.ParseInt(millis, 10, 64); err == nil && n > 0 {
			m.When = time.UnixMilli(n).Local().Format("2006-01-02 15:04:05.000")
			m.WhenLabel = "后端记录时间"
		}
	}
	if m.When == "" {
		if nanos := get("timestampNs"); nanos != "" {
			if n, err := strconv.ParseInt(nanos, 10, 64); err == nil && isPlausibleUnixNanoseconds(n) {
				m.When = time.Unix(0, n).Local().Format("2006-01-02 15:04:05.000")
				m.WhenLabel = "事件时间（Unix ns）"
			}
		}
	}

	// A typed field specification keeps the presentation deterministic and
	// makes missing keys truly absent, rather than displaying bogus "--" rows.
	type spec struct{ label string; keys []string }
	field := func(label string, keys ...string) spec { return spec{label, keys} }
	section := func(title string, specs ...spec) {
		fields := make([]eventDetailField, 0, len(specs))
		for _, item := range specs {
			appendDetailField(&fields, layers, item.label, item.keys...)
		}
		if len(fields) > 0 {
			m.Sections = append(m.Sections, eventDetailSection{title, fields})
		}
	}

	// Typed protobuf payload is authoritative; syscall names are a fallback.
	category := eventDetailCategory(detail, eventType)
	switch category {
	case "file":
		m.Target = prefer("path", "targetPath", "relatedPath", "extraPath")
		section("文件操作", field("操作", "operation", "type"),
			field("目标路径", "path", "targetPath", "relatedPath"),
			field("关联路径", "extraPath"), field("模式 / 权限", "mode"),
			field("读写字节数", "bytes"), field("目标 UID", "uidArg"),
			field("目标 GID", "gidArg"), field("返回值", "retval"),
			field("附加信息", "extraInfo"))
	case "network":
		m.Target = prefer("netEndpoint", "endpoint", "dstIp", "domain", "dnsName", "sni")
		section("网络行为", field("目标端点", "netEndpoint", "endpoint"),
			field("方向", "netDirection", "direction"),
			field("源地址", "srcIp"), field("源端口", "srcPort"),
			field("目标 IP", "dstIp"), field("目标端口", "dstPort"),
			field("域名", "domain", "dnsName"), field("SNI", "sni"),
			field("HTTP Host", "httpHost"), field("传输协议", "transport", "appProtocol"),
			field("网络族", "netFamily", "family"), field("发送字节", "bytesOut"),
			field("接收字节", "bytesIn"), field("流 ID", "flowId"),
			field("返回值", "retval"), field("附加信息", "extraInfo"))
	case "process":
		m.Target = prefer("path", "commandLine", "targetPid")
		section("进程行为", field("阶段", "phase"), field("执行文件", "path"),
			field("命令行", "commandLine"), field("参数", "args"),
			field("工作目录", "cwd"), field("父进程 PID", "parentPid"),
			field("子进程 PID", "childPid"), field("旧 PID", "oldPid"),
			field("目标 PID", "targetPid"), field("退出状态", "exitStatus"), field("返回值", "retval"),
			field("附加信息", "extraInfo"))
	default:
		if semanticAlert {
			m.Target = prefer("path", "targetPath", "netEndpoint", "domain")
			// Historical versions could emit the synthetic label "file write" as
			// a contention target. Preserve it in raw evidence, not as a file.
			if get("comm") == "MULTI_AGENT_FILE_CONTENTION" && !strings.HasPrefix(m.Target, "/") {
				m.Target = ""
				m.EvidenceNote = "该合成语义告警没有可核实的文件路径，可能是旧版本基于操作描述生成的误报；请核对原始事件证据。"
				section("语义告警", field("规则编号", "comm"), field("原始事件信息", "extraInfo"))
			} else {
				section("语义告警", field("规则编号", "comm"), field("关联目标", "path", "targetPath"),
					field("原始事件信息", "extraInfo"))
			}
		} else if eventDetailTypedPayload(detail).Kind == "" {
			// A typed payload gets its own domain view below. A generic card here
			// would mistake unrelated legacy fields for part of that payload.
			m.Target = prefer("path", "targetPath", "netEndpoint", "domain", "commandLine", "relatedEndpoint")
			section("操作详情", field("路径", "path", "targetPath", "relatedPath"),
				field("目标地址", "netEndpoint", "endpoint", "relatedEndpoint"),
				field("命令行", "commandLine"), field("原因", "reason"),
				field("结果", "retval"), field("附加信息", "extraInfo"))
		}
	}
	// Domain cards use only fields from the corresponding protobuf oneof.
	// Legacy fields remain visible in the common sections, with their origin.
	if typed := eventDetailTypedPayload(detail); typed.Kind != "" {
		if !semanticAlert && typed.Kind != "file" && typed.Kind != "network" && typed.Kind != "process" {
			if target := eventTypedTarget(typed); target != "" {
				m.Target = target
			}
		}
		if domain := eventDetailTypedSection(typed); len(domain.Fields) > 0 {
			m.Sections = append(m.Sections, domain)
		}
	}
	// Typed alert metadata must not re-introduce a synthetic or ambiguous
	// file target after the legacy evidence check above.
	if semanticAlert && get("comm") == "MULTI_AGENT_FILE_CONTENTION" &&
		!strings.HasPrefix(m.Target, "/") {
		m.Target = ""
		if m.EvidenceNote == "" {
			m.EvidenceNote = "该合成语义告警没有可核实的文件路径，可能是旧版本误报；请核对原始事件证据。"
		}
	}

	section("策略与分类", field("策略决定", "decision", "policyDecision"),
		field("风险评分", "riskScore"), field("判定原因", "reason"),
		field("关联策略路径", "relatedPath"), field("关联端点", "relatedEndpoint"),
		field("行为分类", "primaryCategory"), field("分类置信度", "confidence"),
		field("分类依据", "reasoning"))
	if semanticAlert {
		fields := make([]eventDetailField, 0, 12)
		if comm := semanticAlertSourceComm(get("extraInfo")); comm != "" {
			fields = append(fields, eventDetailField{
				Label: "源进程名称", Value: comm, Origin: "Event.extraInfo (comm)",
			})
		}
		for _, item := range []spec{
			field("来源 PID（事件时）", "pid"), field("来源 PPID（事件时）", "ppid"),
			field("来源 TGID（事件时）", "tgid"), field("根 Agent PID", "rootAgentPid"),
			field("UID", "uid"), field("GID", "gid"), field("工作目录", "cwd"),
			field("跟踪标签", "tag"), field("容器 ID", "containerId"), field("Cgroup ID", "cgroupId"),
		} {
			appendDetailField(&fields, layers, item.label, item.keys...)
		}
		if len(fields) > 0 {
			m.Sections = append(m.Sections, eventDetailSection{Title: "来源事件进程", Fields: fields})
		}
	} else {
		section("执行主体", field("进程名称", "comm"),
			field("PID", "pid"), field("PPID", "ppid"), field("TGID", "tgid"),
			field("根 Agent PID", "rootAgentPid"), field("UID", "uid"), field("GID", "gid"),
			field("命令行", "commandLine"), field("工作目录", "cwd"),
			field("参数摘要", "argvDigest"), field("跟踪标签", "tag"),
			field("容器 ID", "containerId"), field("Cgroup ID", "cgroupId"))
	}
	section("Agent 与调用链", field("运行 ID", "agentRunId"),
		field("任务 ID", "taskId"), field("会话 ID", "conversationId"),
		field("Turn ID", "turnId"), field("工具调用 ID", "toolCallId"),
		field("工具名称", "toolName"), field("Trace ID", "traceId"),
		field("Span ID", "spanId"))
	section("采集与证据", field("采集来源", "captureSource", "source"),
		field("事件类型编号", "eventType"), field("持续时间 (ns)", "durationNs"),
		field("内核时间戳 (ns)", "kernelTimestampNs"), field("内核序号", "kernelSequence"),
		field("内核 CPU", "kernelCpu"), field("内核时钟", "kernelClock"),
		field("采集延迟 (ns)", "captureDelayNs"), field("采集时间戳 (ns)", "captureTimestampNs"),
		field("脱敏等级", "redactionLevel"), field("已脱敏字段", "sanitizedFields"),
		field("审计标志", "auditFlags"), field("丢弃计数", "kernelDroppedSinceLast"))
	if m.Target == "" {
		typedKind := eventDetailTypedPayload(detail).Kind
		switch typedKind {
		case "metric", "otel", "sse", "stdio":
			m.Target = "无文件或网络操作对象"
			m.NoTargetExpected = true
			m.EvidenceNote = "此类事件主要描述遥测指标或数据流，不要求存在文件路径或网络端点。"
		default:
			m.Target = "目标路径或端点未记录"
			if m.EvidenceNote == "" {
				m.EvidenceNote = "当前记录没有可用的操作目标，不能反推出具体文件或地址。"
			}
			if category == "file" && (eventType == "write" || eventType == "read") {
				m.EvidenceNote += " Linux write/read 系统调用使用文件描述符而非文件名；只有存在文件描述符到路径的有效关联，才能定位实际文件。"
			}
			if redaction := get("redactionLevel"); redaction != "" {
				m.EvidenceNote += " 记录的脱敏等级：" + redaction
			}
		}
	}
	return m
}

// Semantic alerts are synthetic events: Event.comm holds the rule code,
// while the originating process comm is recorded in structured extraInfo text.
func semanticAlertSourceComm(extraInfo string) string {
	beforeReason, _, ok := strings.Cut(extraInfo, " reason=")
	if !ok {
		return ""
	}
	index := strings.Index(beforeReason, " comm=")
	if index < 0 {
		return ""
	}
	return strings.TrimSpace(beforeReason[index+len(" comm="):])
}

// Compact cards use a vertical label/value stack; otherwise a 126-DIP label
// and clipboard button leave no readable value space on small windows.
func eventDetailFieldRowAdaptive(c *ui.Context, field eventDetailField, width float32, expandedKey *string) {
	t := c.Theme()
	expanded := expandedKey != nil && *expandedKey == field.Origin
	visible := eventDetailPreview(field.Value, 280)
	if expanded {
		visible = field.Value
	}
	showValue := func() {
		if expanded {
			ui.Text(c, field.Value).Font("monospace").FontSize(11)
		} else {
			ui.Text(c, visible).Font("monospace").FontSize(11).MaxLines(3)
		}
		ui.Text(c, field.Origin).FontSize(9).TextColor(t.TextMuted)
	}
	showActions := func() {
		if len([]rune(field.Value)) > 280 && expandedKey != nil {
			label := "展开"
			if expanded { label = "收起" }
			if ui.Button(c, label).Clicked() {
				if expanded { *expandedKey = "" } else { *expandedKey = field.Origin }
			}
		}
		if ui.Button(c, "复制").Tooltip("复制"+field.Label+"的完整值").Clicked() {
			c.WriteClipboard(field.Value)
			c.Toast("已复制 "+field.Label)
		}
	}
	if width < 440 {
		ui.Column(c).Gap(4).Padding(0, 0, 5, 0).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, field.Label).FontSize(11).TextColor(t.TextMuted).Grow(1).MinWidth(0)
				showActions()
			})
			showValue()
		})
		return
	}
	ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
		ui.Text(c, field.Label).Width(112).Shrink(0).FontSize(11).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(showValue)
		showActions()
	})
}

// Existing small component tests retain the basic simple row entry point.
func eventDetailFieldRow(c *ui.Context, field eventDetailField) {
	eventDetailFieldRowAdaptive(c, field, 520, nil)
}

func eventDetailSectionCard(c *ui.Context, section eventDetailSection, width float32, expandedKey *string) {
	card(c, section.Title, func() {
		for _, field := range section.Fields {
			eventDetailFieldRowAdaptive(c, field, width, expandedKey)
		}
	}).Width(width)
}

// Show one continuous investigation, instead of a pair of large mostly-empty
// cards. Layout width is driven by the viewport, not a fixed 760 DIP.
func (a *renewApp) richEventDetail(c *ui.Context, detail map[string]any, width float32) {
	t := c.Theme()
	model := eventDetailModel(detail)
	card(c, "事件概况", func() {
		ui.Row(c).Gap(7).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Text(c, model.Action).FontSize(17).Bold()
			if model.Type != "" {
				ui.Badge(c, model.Type)
			}
			if model.Decision != "" {
				switch strings.ToUpper(model.Decision) {
				case "BLOCK", "DENY":
					statusPill(c, model.Decision, t.Danger)
				case "ALERT":
					statusPill(c, model.Decision, t.Warning)
				default:
					ui.Badge(c, model.Decision)
				}
			}
			if model.Risk != "" {
				score, err := strconv.ParseFloat(model.Risk, 64)
				if err == nil {
					riskPill(c, eventRisk(eventSummary{RiskScore: score, Decision: model.Decision}))
				}
				ui.Badge(c, "风险分 " + model.Risk)
			}
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, eventDetailPreview(model.Target, 520)).Font("monospace").FontSize(12).Grow(1).MinWidth(0).MaxLines(3)
			if model.EvidenceNote == "" && ui.Button(c, "复制目标").Clicked() {
				c.WriteClipboard(model.Target)
				c.Toast("目标已复制")
			}
		})
		if model.EvidenceNote != "" {
			tone := t.Warning
			if model.NoTargetExpected {
				tone = t.TextMuted
			}
			ui.Text(c, model.EvidenceNote).FontSize(11).TextColor(tone)
		}
		if model.When != "" {
			ui.Text(c, model.WhenLabel+"  "+model.When).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Text(c, "字段来源与脱敏状态以记录为准；没有采集到的内容不进行推断。").FontSize(10).TextColor(t.TextMuted)
	})
	if width >= 760 {
		leftWidth := (width - 46) / 2
		ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
			for col := 0; col < 2; col++ {
				ui.Column(c).Width(leftWidth).Gap(12).Children(func() {
					for index, section := range model.Sections {
						if index%2 == col {
							eventDetailSectionCard(c, section, leftWidth, &a.eventDetailExpandedField)
						}
					}
				})
			}
		})
	} else {
		for _, section := range model.Sections {
			eventDetailSectionCard(c, section, width-12, &a.eventDetailExpandedField)
		}
	}
	a.eventDetailRelated(c, detail)
}

// Model the current event independently from the UI, preserving JSON
// timestamps and preferring persisted evidence over live summary fallbacks.
func eventDetailSelectedSummary(detail map[string]any, eventID string, retained []eventSummary) eventSummary {
	layers := eventDetailLayers(detail)
	read := func(key string) string {
		v, _, _ := eventDetailLookup(layers, key)
		return v
	}
	selected := eventSummary{
		EventID:        eventID,
		AgentRunID:     read("agentRunId"),
		ConversationID: read("conversationId"),
		Type:           read("type"),
		Comm:           read("comm"),
		Tag:            read("tag"),
	}
	if pid, err := strconv.Atoi(read("pid")); err == nil && pid > 0 {
		selected.PID = pid
	}
	if pid, err := strconv.Atoi(read("ppid")); err == nil && pid > 0 {
		selected.PPID = pid
	}
	if pid, err := strconv.Atoi(read("rootAgentPid")); err == nil && pid > 0 {
		selected.RootAgentPID = pid
	}
	if target := eventDetailModel(detail).Target; usableEventTarget(target) {
		selected.Target = target
	}
	selected.Network = isNetworkEvent(selected)
	// fmt.Sprint of a JSON float64 can yield exponent notation, which is
	// invalid for a decimal timestamp. Our scalar lookup preserves decimals.
	if text, _, exists := eventDetailLookup(
		[]eventDetailLayer{newEventDetailLayer("$", detail)}, "Timestamp",
	); exists {
		if ms, ok := eventDetailInt64(text); ok && ms > 0 {
			selected.ReceivedAtMS = ms
		}
	}
	for _, item := range retained {
		if item.EventID != selected.EventID {
			continue
		}
		if selected.PID == 0 { selected.PID = item.PID }
		if selected.PPID == 0 { selected.PPID = item.PPID }
		if selected.RootAgentPID == 0 { selected.RootAgentPID = item.RootAgentPID }
		if selected.Type == "" { selected.Type = item.Type }
		if selected.Comm == "" { selected.Comm = item.Comm }
		if selected.Tag == "" { selected.Tag = item.Tag }
		if selected.Target == "" { selected.Target = item.Target }
		if selected.ReceivedAtMS == 0 { selected.ReceivedAtMS = item.ReceivedAtMS }
		if selected.AgentRunID == "" { selected.AgentRunID = item.AgentRunID }
		if selected.ConversationID == "" { selected.ConversationID = item.ConversationID }
		selected.Network = selected.Network || item.Network
		break
	}
	return selected
}

func (a *renewApp) eventDetailRelated(c *ui.Context, detail map[string]any) {
	selected := eventDetailSelectedSummary(detail, a.eventDetailID, a.events)
	if selected.PID <= 0 && selected.AgentRunID == "" && selected.ConversationID == "" {
		return
	}
	related := eventDetailRelatedCandidates(a.events, selected, 4)
	if len(related) == 0 {
		return
	}
	card(c, "邻近事件 · 当前缓存", func() {
		ui.Text(c, "优先使用 Agent Run/会话标识关联跨进程活动；仅在两分钟窗口内匹配无冲突的同 PID 记录。这些事件不代表因果关系。").FontSize(10).TextColor(c.Theme().TextMuted)
		for _, entry := range related {
			entry := entry
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Badge(c, entry.Relation)
				ui.Text(c, summaryTime(entry.Event.ReceivedAtMS)+"  "+displayOr(entry.Event.Type, "event")).FontSize(11)
				ui.Text(c, eventTarget(entry.Event)).MaxLines(1).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0)
				if ui.Button(c, "查看").Clicked() {
					a.openEventDetail(entry.Event.EventID)
				}
			})
		}
	})
}

