package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
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

func TestEventDetailCategoryDoesNotGuessFromSubstring(t *testing.T) {
	for _, tc := range []struct{ eventType, want string }{
		{"openat", "file"}, {"file_write", "file"}, {"connect", "network"},
		{"sendmsg", "network"}, {"execve", "process"}, {"clone", "process"},
		{"openai_request", "other"}, {"readiness_probe", "other"},
	} {
		if got := eventDetailCategory(nil, tc.eventType); got != tc.want {
			t.Errorf("%q classified as %q instead of %q", tc.eventType, got, tc.want)
		}
	}
	got := eventDetailCategory(map[string]any{
		"Envelope": map[string]any{"networkEvent": map[string]any{"endpoint": "192.0.2.1:443"}},
	}, "open")
	if got != "network" {
		t.Fatalf("explicit typed evidence must outrank ambiguous event type: %q", got)
	}
}

func TestEventDetailPolicyExplanationAndProvenance(t *testing.T) {
	model := eventDetailModel(map[string]any{
		"Event": map[string]any{
			"type": "write",
			"behavior": map[string]any{
				"primary_category": "FILE_WRITE", "confidence": "high",
				"reasoning": "detected file modification",
			},
		},
		"Envelope": map[string]any{
			"policyDecision": "ALERT",
			"policyEvent": map[string]any{
				"reason": "sensitive target", "relatedPath": "/tmp/example",
			},
		},
	})
	for _, tc := range []struct{ label, value string }{
		{"策略决定", "ALERT"},
		{"判定原因", "sensitive target"},
		{"行为分类", "FILE_WRITE"},
		{"分类置信度", "high"},
		{"分类依据", "detected file modification"},
	} {
		if !eventDetailHasField(model, "策略与分类", tc.label, tc.value) {
			t.Errorf("missing policy evidence: %s = %q", tc.label, tc.value)
		}
	}
	for _, group := range model.Sections {
		if group.Title == "策略与分类" {
			for _, field := range group.Fields {
				if field.Origin == "" {
					t.Fatalf("policy field %q has no source", field.Label)
				}
			}
		}
	}
}

func TestDetailPreviewDoesNotSplitUnicode(t *testing.T) {
	if got := eventDetailPreview("命令行测试", 3); got != "命令行…（可复制完整值）" {
		t.Fatalf("unicode preview = %q", got)
	}
	if got := eventDetailPreview("short", 50); got != "short" {
		t.Fatalf("short preview = %q", got)
	}
}

func TestEventDetailFieldRowCopiesCompleteValue(t *testing.T) {
	const full = "/home/user/" + "很长的路径/"
	tester := ui.NewTester(func(c *ui.Context) {
		eventDetailFieldRow(c, eventDetailField{
			Label: "目标路径", Value: full, Origin: "Event.path",
		})
	}, 520, 170)
	tester.Frame()
	if err := tester.Click("复制"); err != nil {
		t.Fatalf("copy button not reachable: %v", err)
	}
	if got := tester.Clipboard(); got != full {
		t.Fatalf("copy returned %q, want %q", got, full)
	}
}

func TestEventDetailTargetPrefersCompleteEndpointAcrossLayers(t *testing.T) {
	detail := map[string]any{
		"Event": map[string]any{"type": "connect", "dstIp": "203.0.113.5"},
		"Envelope": map[string]any{
			"networkEvent": map[string]any{"endpoint": "203.0.113.5:443"},
		},
	}
	if model := eventDetailModel(detail); model.Target != "203.0.113.5:443" {
		t.Fatalf("lost typed port information in target: %q", model.Target)
	}
	target := enforcementTargets(detail)
	if target.IP != "203.0.113.5" || target.Port != 443 {
		t.Fatalf("typed endpoint should retain port: %+v", target)
	}
}

func TestEventDetailFieldLookupPrefersExactKey(t *testing.T) {
	layers := []eventDetailLayer{{
		Path: "Event",
		Values: map[string]any{
			"riskScore": float64(20), "risk_score": float64(40),
		},
	}}
	got, origin, ok := eventDetailLookup(layers, "riskScore")
	if !ok || got != "20" || origin != "Event.riskScore" {
		t.Fatalf("ambiguous aliases resolved nondeterministically: %q, %q, %v", got, origin, ok)
	}
}

func TestEventDetailVisualLayoutAcrossThemeAndWindowSizes(t *testing.T) {
	detail := map[string]any{
		"Event": map[string]any{
			"type": "write", "pid": float64(19), "comm": "agent",
			"path": "/home/user/project/config.txt",
		},
	}
	for _, dims := range [][2]int{{1120, 820}, {510, 680}} {
		a := &renewApp{eventDetailID: "test-evidence"}
		tester := ui.NewTester(func(c *ui.Context) {
			width, _ := c.Size()
			ui.Scroll(c).Fill().Children(func() {
				a.richEventDetail(c, detail, width-24)
			})
		}, dims[0], dims[1])
		tester.Frame()
		for _, label := range []string{"事件概况", "文件操作", "执行主体"} {
			if !tester.HasText(label) {
				t.Errorf("%dx%d missing %q", dims[0], dims[1], label)
			}
		}
		tester.SetDark(true)
		tester.Frame()
		if !tester.HasText("事件概况") {
			t.Errorf("%dx%d missing summary in dark theme", dims[0], dims[1])
		}
	}
}
