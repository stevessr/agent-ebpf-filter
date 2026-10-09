package main

import (
	"strings"
	"testing"
)

func eventDetailHasField(model eventDetailViewModel, section, label, value string) bool {
	for _, group := range model.Sections {
		if group.Title != section {
			continue
		}
		for _, item := range group.Fields {
			if item.Label == label && item.Value == value {
				return true
			}
		}
	}
	return false
}

func TestEventDetailModelFileWriteEvidence(t *testing.T) {
	payload := map[string]any{
		"Event": map[string]any{
			"type": "write", "comm": "omp", "pid": float64(343846),
			"ppid": float64(343645), "uid": float64(1000),
			"path": "/home/user/project/main.go", "bytes": float64(512),
			"tag": "Agent CLI", "riskScore": float64(20),
		},
		"Timestamp": float64(1760000000000),
		"Envelope": map[string]any{
			"source": "kernel",
			"agentRunId": "run_123",
			"fileEvent": map[string]any{"operation": "write", "mode": "0644"},
		},
	}
	model := eventDetailModel(payload)
	if model.Type != "write" || model.Target != "/home/user/project/main.go" || model.Risk != "20" || model.When == "" {
		t.Fatalf("incorrect file event summary: %+v", model)
	}
	for _, tc := range []struct{ section, label, value string }{
		{"文件操作", "目标路径", "/home/user/project/main.go"},
		{"文件操作", "读写字节数", "512"},
		{"文件操作", "模式 / 权限", "0644"},
		{"执行主体", "进程名称", "omp"},
		{"执行主体", "PID", "343846"},
		{"执行主体", "UID", "1000"},
		{"Agent 与调用链", "运行 ID", "run_123"},
		{"采集与证据", "采集来源", "kernel"},
	} {
		if !eventDetailHasField(model, tc.section, tc.label, tc.value) {
			t.Errorf("missing %s / %s = %q in %+v", tc.section, tc.label, tc.value, model.Sections)
		}
	}
}

func TestEventDetailModelEnvelopeNetworkAndMissingFields(t *testing.T) {
	payload := map[string]any{
		"Event": map[string]any{"type": "connect", "comm": "curl", "pid": float64(42)},
		"Envelope": map[string]any{
			"networkEvent": map[string]any{
				"endpoint": "198.51.100.1:443",
				"transport": "tcp",
				"dstPort": float64(443),
			},
		},
	}
	model := eventDetailModel(payload)
	if model.Target != "198.51.100.1:443" || model.Risk != "" || model.Decision != "" {
		t.Fatalf("network summary should reflect provided data only: %+v", model)
	}
	if !eventDetailHasField(model, "网络行为", "目标端点", "198.51.100.1:443") {
		t.Fatalf("network payload not inspected: %+v", model.Sections)
	}
	if eventDetailHasField(model, "执行主体", "UID", "0") {
		t.Fatal("missing UID must not be fabricated")
	}
}

func TestEventDetailTreeSearchAndBound(t *testing.T) {
	detail := map[string]any{
		"Event": map[string]any{"comm": "omp", "pid": float64(42)},
		"Envelope": map[string]any{
			"fileEvent": map[string]any{"path": "/tmp/test.txt"},
			"sanitizedFields": []any{"commandLine"},
		},
	}
	found := eventDetailTreeItems(detail, "path")
	if len(found) != 1 || found[0].Label != "Envelope.fileEvent.path" ||
		found[0].Value != "/tmp/test.txt" {
		t.Fatalf("unexpected tree path filter: %+v", found)
	}
	if values := eventDetailTreeItems(detail, "commandline"); len(values) != 1 ||
		!strings.Contains(values[0].Label, "sanitizedFields[0]") {
		t.Fatalf("array child was not searchable: %+v", values)
	}
	if values := eventDetailTreeItems(detail, "does-not-exist"); len(values) != 0 {
		t.Fatalf("unexpected search results: %+v", values)
	}
}
