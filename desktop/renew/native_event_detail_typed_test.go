package main

import (
	"strings"
	"testing"
)

func TestEventDetailEveryTypedPayloadHasOwnEvidence(t *testing.T) {
	cases := []struct {
		name, key, eventType, label, value, title, target string
		data map[string]any
	}{
		{"TLS", "tlsEvent", "tls_capture", "Host", "api.example.test", "TLS / LLM 请求元数据", "https://api.example.test/messages",
			map[string]any{"host": "api.example.test", "url": "https://api.example.test/messages", "rawAvailable": false, "redactionState": "standard"}},
		{"HTTP", "httpEvent", "http", "HTTP 状态", "403", "HTTP 请求元数据", "https://example.test/v1",
			map[string]any{"url": "https://example.test/v1", "status": float64(403), "method": "POST"}},
		{"Wrapper", "wrapperEvent", "wrapper", "命令行", "python task.py", "Agent Wrapper 调用", "python task.py",
			map[string]any{"commandLine": "python task.py", "args": []any{"task.py"}}},
		{"Hook", "hookEvent", "native_hook", "Hook 名称", "PreToolUse", "Agent Hook", "/tmp/demo.txt",
			map[string]any{"hookName": "PreToolUse", "targetPath": "/tmp/demo.txt"}},
		{"MCP", "mcpEvent", "mcp_tool", "请求 ID", "req_8", "MCP 工具调用", "http://127.0.0.1:9000",
			map[string]any{"endpoint": "http://127.0.0.1:9000", "requestId": "req_8"}},
		{"Policy", "policyEvent", "policy", "原因", "forbidden", "策略裁决", "/etc/example",
			map[string]any{"reason": "forbidden", "relatedPath": "/etc/example", "decision": "BLOCK"}},
		{"SSE", "sseEvent", "sse", "已结束", "false", "SSE 流事件", "message",
			map[string]any{"event": "message", "completed": false}},
		{"stdio", "stdioEvent", "stdio", "文件描述符", "2", "标准输入输出", "stderr",
			map[string]any{"fd": "2", "stream": "stderr"}},
		{"metrics", "systemMetricEvent", "system_metric", "CPU 使用率", "21.3", "进程性能指标", "warning",
			map[string]any{"cpuPercent": float64(21.3), "alert": "warning"}},
		{"OTel", "otelSpanEvent", "otel_span", "输入 Token", "3000", "OTel Span", "model.generate",
			map[string]any{"name": "model.generate", "inputTokens": float64(3000)}},
		{"AgentSight", "agentsightAlertEvent", "agentsight_alert", "关联事件 ID", "evt_other", "AgentSight 风险告警", "evt_other",
			map[string]any{"relatedEventId": "evt_other", "severity": "high"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			detail := map[string]any{
				"Event": map[string]any{"type": tc.eventType, "pid": float64(42)},
				"Envelope": map[string]any{tc.key: tc.data},
			}
			model := eventDetailModel(detail)
			if model.Target != tc.target {
				t.Fatalf("target = %q, expected %q", model.Target, tc.target)
			}
			if !eventDetailHasField(model, tc.title, tc.label, tc.value) {
				t.Errorf("missing typed %s=%q in %+v", tc.label, tc.value, model.Sections)
			}
			for _, section := range model.Sections {
				if section.Title != tc.title {
					continue
				}
				for _, f := range section.Fields {
					if !strings.HasPrefix(f.Origin, "Envelope."+tc.key+".") {
						t.Errorf("incorrect typed provenance: %+v", f)
					}
				}
			}
		})
	}
}

func TestEventDetailWrapperBehaviorClassification(t *testing.T) {
	model := eventDetailModel(map[string]any{
		"Event": map[string]any{"type": "wrapper"},
		"Envelope": map[string]any{
			"wrapperEvent": map[string]any{
				"commandLine": "go test ./...",
				"behavior": map[string]any{"primaryCategory": "DEVELOPMENT", "reasoning": "test run"},
			},
		},
	})
	if !eventDetailHasField(model, "策略与分类", "行为分类", "DEVELOPMENT") ||
		!eventDetailHasField(model, "策略与分类", "分类依据", "test run") {
		t.Fatalf("wrapper behavior provenance missing: %+v", model.Sections)
	}
}

func TestEventDetailMissingFileTargetExplainsUncertainty(t *testing.T) {
	model := eventDetailModel(map[string]any{
		"Event": map[string]any{"type": "write", "comm": "omp", "pid": float64(343846)},
	})
	if model.Target != "目标路径或端点未记录" || model.EvidenceNote == "" {
		t.Fatalf("missing target should be explicitly described: %+v", model)
	}
	if strings.Contains(model.EvidenceNote, "一定") || strings.Contains(model.EvidenceNote, "目标是") {
		t.Fatalf("uncertainty should not overclaim: %s", model.EvidenceNote)
	}
}

func TestEventDetailMonotonicTimeNotShownAsCalendar(t *testing.T) {
	model := eventDetailModel(map[string]any{
		"Event": map[string]any{"type": "write"},
		"Envelope": map[string]any{"timestampNs": "120000000000"},
	})
	if model.When != "" {
		t.Fatalf("monotonic timestamp was interpreted as Unix: %+v", model)
	}
	model = eventDetailModel(map[string]any{
		"Event": map[string]any{"type": "write"},
		"Timestamp": float64(1760000000000),
		"Envelope": map[string]any{"timestampNs": "120000000000"},
	})
	if model.When == "" || model.WhenLabel != "后端记录时间" {
		t.Fatalf("persisted timestamp must be authoritative: %+v", model)
	}
	model = eventDetailModel(map[string]any{
		"Event": map[string]any{"type": "write"},
		"Envelope": map[string]any{"timestampNs": "1760000000123456789"},
	})
	if model.When == "" || model.WhenLabel != "事件时间（Unix ns）" {
		t.Fatalf("valid Unix timestamp should render: %+v", model)
	}
}

func TestRelatedCandidatesAvoidPIDReuseAndRankByEvidence(t *testing.T) {
	selected := eventSummary{
		EventID: "anchor", PID: 42, AgentRunID: "run-a",
		ConversationID: "conversation-a", ReceivedAtMS: 10_000_000,
	}
	in := []eventSummary{
		{EventID: "other-pid", PID: 43, AgentRunID: "run-a", ReceivedAtMS: 10_000_100},
		{EventID: "far-pid-only", PID: 42, ReceivedAtMS: 30_000_000},
		{EventID: "near", PID: 42, ReceivedAtMS: 10_020_000},
		{EventID: "run", PID: 42, AgentRunID: "run-a", ReceivedAtMS: 30_000_000},
		{EventID: "session", PID: 42, ConversationID: "conversation-a", ReceivedAtMS: 30_000_000},
		{EventID: "anchor", PID: 42, ReceivedAtMS: 10_000_000},
	}
	got := eventDetailRelatedCandidates(in, selected, 4)
	if len(got) != 3 || got[0].Event.EventID != "run" ||
		got[1].Event.EventID != "session" || got[2].Event.EventID != "near" {
		t.Fatalf("related ranking falsely includes PID reuse: %+v", got)
	}
	if len(eventDetailRelatedCandidates(in, eventSummary{PID: 42}, 4)) != 0 {
		t.Fatal("without time or run evidence, PID alone must not fabricate correlation")
	}
	if n := len(eventDetailRelatedCandidates(in, selected, 1)); n != 1 {
		t.Fatalf("related result limit: %d", n)
	}
}
