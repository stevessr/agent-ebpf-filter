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
	Sections []eventDetailSection
}

type eventDetailLayer struct {
	Path   string
	Values map[string]any
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
		for _, name := range keys {
			normalized := eventDetailKey(name)
			for key, raw := range layer.Values {
				if eventDetailKey(key) != normalized || raw == nil {
					continue
				}
				var value string
				switch v := raw.(type) {
				case string:
					value = strings.TrimSpace(v)
				case bool, float64, float32, int, int64, uint64, json.Number:
					value = fmt.Sprint(v)
				default:
					b, err := json.Marshal(v)
					if err == nil {
						value = string(b)
					}
				}
				if value != "" {
					return value, layer.Path + "." + key, true
				}
			}
		}
	}
	return "", "", false
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
		layers = append(layers, eventDetailLayer{prefix, event})
	}
	envelope, _ := mapValue(detail, "Envelope", "envelope").(map[string]any)
	if envelope != nil {
		// Payload fields take precedence only if the legacy event lacks them.
		for _, name := range []string{
			"fileEvent", "networkEvent", "execEvent", "processEvent",
			"policyEvent", "wrapperEvent", "hookEvent", "mcpEvent",
			"tlsEvent", "httpEvent", "sseEvent", "stdioEvent",
			"systemMetricEvent", "otelSpanEvent", "agentSightAlertEvent",
		} {
			if nested, ok := mapValue(envelope, name).(map[string]any); ok {
				layers = append(layers, eventDetailLayer{"Envelope." + name, nested})
			}
		}
		layers = append(layers, eventDetailLayer{"Envelope", envelope})
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
	eventType := get("type")
	m := eventDetailViewModel{Type: eventType, Action: eventAction(eventSummary{Type: eventType}), Decision: get("decision", "policyDecision")}
	if value, _, ok := eventDetailLookup(layers, "riskScore", "risk_score"); ok {
		m.Risk = value
	}
	if millis, _, ok := eventDetailLookup([]eventDetailLayer{{"$", detail}}, "Timestamp"); ok {
		if n, err := strconv.ParseInt(millis, 10, 64); err == nil && n > 0 {
			m.When = time.UnixMilli(n).Local().Format("2006-01-02 15:04:05.000")
		}
	}
	if m.When == "" {
		if nanos := get("timestampNs"); nanos != "" {
			if n, err := strconv.ParseInt(nanos, 10, 64); err == nil && n > 0 {
				m.When = time.Unix(0, n).Local().Format("2006-01-02 15:04:05.000")
			}
		}
	}

	addSection := func(title string, specs ...struct{ label string; keys []string }) {
		fields := make([]eventDetailField, 0, len(specs))
		for _, spec := range specs {
			appendDetailField(&fields, layers, spec.label, spec.keys...)
		}
		if len(fields) > 0 {
			m.Sections = append(m.Sections, eventDetailSection{title, fields})
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
	_ = addSection

	// Infer the operation family from real event type or protobuf payload.
	category := strings.ToLower(eventType)
	hasPayload := func(key string) bool {
		envelope, _ := mapValue(detail, "Envelope", "envelope").(map[string]any)
		_, ok := mapValue(envelope, key).(map[string]any)
		return ok
	}
	isFile := hasPayload("fileEvent") || strings.Contains(category, "file") ||
		strings.Contains(category, "write") || strings.Contains(category, "read") ||
		strings.Contains(category, "open") || strings.Contains(category, "unlink") ||
		strings.Contains(category, "rename") || strings.Contains(category, "chmod") ||
		strings.Contains(category, "mkdir") || strings.Contains(category, "truncate")
	isNetwork := hasPayload("networkEvent") || strings.Contains(category, "net") ||
		strings.Contains(category, "connect") || strings.Contains(category, "accept") ||
		strings.Contains(category, "send") || strings.Contains(category, "recv") ||
		strings.Contains(category, "dns") || strings.Contains(category, "tls")
	isProcess := hasPayload("processEvent") || hasPayload("execEvent") ||
		strings.Contains(category, "exec") || strings.Contains(category, "fork") ||
		strings.Contains(category, "clone") || strings.Contains(category, "kill")

	switch {
	case isFile:
		m.Target = get("path", "targetPath", "relatedPath", "extraPath")
		section("文件操作", field("操作", "operation", "type"),
			field("目标路径", "path", "targetPath", "relatedPath"),
			field("关联路径", "extraPath"), field("模式 / 权限", "mode"),
			field("读写字节数", "bytes"), field("目标 UID", "uidArg"),
			field("目标 GID", "gidArg"), field("返回值", "retval"),
			field("附加信息", "extraInfo"))
	case isNetwork:
		m.Target = get("netEndpoint", "endpoint", "dstIp", "domain", "dnsName", "sni")
		section("网络行为", field("目标端点", "netEndpoint", "endpoint"),
			field("方向", "netDirection", "direction"),
			field("源地址", "srcIp"), field("源端口", "srcPort"),
			field("目标 IP", "dstIp"), field("目标端口", "dstPort"),
			field("域名", "domain", "dnsName"), field("SNI", "sni"),
			field("HTTP Host", "httpHost"), field("传输协议", "transport", "appProtocol"),
			field("网络族", "netFamily", "family"), field("发送字节", "bytesOut"),
			field("接收字节", "bytesIn"), field("流 ID", "flowId"),
			field("返回值", "retval"), field("附加信息", "extraInfo"))
	case isProcess:
		m.Target = get("path", "commandLine", "targetPid")
		section("进程行为", field("阶段", "phase"), field("执行文件", "path"),
			field("命令行", "commandLine"), field("参数", "args"),
			field("工作目录", "cwd"), field("父进程 PID", "parentPid"),
			field("子进程 PID", "childPid"), field("目标 PID", "targetPid"),
			field("退出状态", "exitStatus"), field("返回值", "retval"),
			field("附加信息", "extraInfo"))
	default:
		m.Target = get("path", "targetPath", "netEndpoint", "domain", "commandLine", "relatedEndpoint")
		section("操作详情", field("路径", "path", "targetPath", "relatedPath"),
			field("目标地址", "netEndpoint", "endpoint", "relatedEndpoint"),
			field("命令行", "commandLine"), field("原因", "reason"),
			field("结果", "retval"), field("附加信息", "extraInfo"))
	}
	section("执行主体", field("进程名称", "comm"),
		field("PID", "pid"), field("PPID", "ppid"), field("TGID", "tgid"),
		field("根 Agent PID", "rootAgentPid"), field("UID", "uid"), field("GID", "gid"),
		field("命令行", "commandLine"), field("工作目录", "cwd"),
		field("参数摘要", "argvDigest"), field("跟踪标签", "tag"),
		field("容器 ID", "containerId"), field("Cgroup ID", "cgroupId"))
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
		m.Target = "此事件没有提供明确的操作对象"
	}
	return m
}

func eventDetailFieldRow(c *ui.Context, field eventDetailField) {
	t := c.Theme()
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Text(c, field.Label).Width(126).Shrink(0).FontSize(11).TextColor(t.TextMuted)
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, field.Value).Font("monospace").FontSize(11)
			ui.Text(c, field.Origin).FontSize(9).TextColor(t.TextMuted)
		})
	})
}

func eventDetailSectionCard(c *ui.Context, section eventDetailSection, width float32) {
	card(c, section.Title, func() {
		for _, field := range section.Fields {
			eventDetailFieldRow(c, field)
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
		ui.Text(c, model.Target).Font("monospace").FontSize(12)
		if model.When != "" {
			ui.Text(c, "发生时间  "+model.When).FontSize(11).TextColor(t.TextMuted)
		}
		ui.Text(c, "此处展示后端实际记录的字段；空缺信息不会推测补全。").FontSize(10).TextColor(t.TextMuted)
	})
	if width >= 760 {
		leftWidth := (width - 46) / 2
		ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
			for col := 0; col < 2; col++ {
				ui.Column(c).Width(leftWidth).Gap(12).Children(func() {
					for index, section := range model.Sections {
						if index%2 == col {
							eventDetailSectionCard(c, section, leftWidth)
						}
					}
				})
			}
		})
	} else {
		for _, section := range model.Sections {
			eventDetailSectionCard(c, section, width-12)
		}
	}
	a.eventDetailRelated(c, detail)
}

func (a *renewApp) eventDetailRelated(c *ui.Context, detail map[string]any) {
	layers := eventDetailLayers(detail)
	pidText, _, ok := eventDetailLookup(layers, "pid")
	if !ok {
		return
	}
	pid, err := strconv.Atoi(pidText)
	if err != nil || pid <= 0 {
		return
	}
	related := make([]eventSummary, 0, 4)
	for _, item := range a.events {
		if item.EventID != "" && item.EventID != a.eventDetailID && item.PID == pid {
			related = append(related, item)
			if len(related) == 4 {
				break
			}
		}
	}
	if len(related) == 0 {
		return
	}
	card(c, "同进程事件 · 已缓存摘要", func() {
		for _, item := range related {
			item := item
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, summaryTime(item.ReceivedAtMS)+"  "+displayOr(item.Type, "event")).FontSize(11).Grow(1)
				ui.Text(c, eventTarget(item)).MaxLines(1).FontSize(10).TextColor(c.Theme().TextMuted).Grow(1)
				if ui.Button(c, "查看").Clicked() {
					a.openEventDetail(item.EventID)
				}
			})
		}
	})
}

func eventDetailTreeItems(detail map[string]any, query string) []eventDetailField {
	var out []eventDetailField
	query = strings.ToLower(strings.TrimSpace(query))
	var walk func(string, any, int)
	walk = func(path string, value any, depth int) {
		if depth > 7 || len(out) >= 300 {
			return
		}
		switch node := value.(type) {
		case map[string]any:
			keys := make([]string, 0, len(node))
			for key := range node {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				child := path + "." + key
				if path == "$" {
					child = key
				}
				walk(child, node[key], depth+1)
			}
		case []any:
			for index, item := range node {
				walk(fmt.Sprintf("%s[%d]", path, index), item, depth+1)
				if len(out) >= 300 {
					return
				}
			}
		default:
			if value == nil {
				return
			}
			text := fmt.Sprint(value)
			if query == "" || strings.Contains(strings.ToLower(path), query) || strings.Contains(strings.ToLower(text), query) {
				out = append(out, eventDetailField{Label: path, Value: text})
			}
		}
	}
	walk("$", detail, 0)
	return out
}
