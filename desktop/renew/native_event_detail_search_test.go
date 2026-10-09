package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEventDetailSearchCanPageBeyondFirst300(t *testing.T) {
	fields := make([]any, 725)
	for i := range fields {
		fields[i] = map[string]any{"value": fmt.Sprintf("item-%04d", i)}
	}
	detail := map[string]any{"Envelope": map[string]any{"items": fields}}
	first := eventDetailTreePage(detail, "", 0, eventFieldPageSize)
	if len(first.Items) != eventFieldPageSize || !first.HasMore || first.ScanLimited {
		t.Fatalf("first page incorrect: %+v", first)
	}
	far := eventDetailTreePage(detail, "", 600, eventFieldPageSize)
	if len(far.Items) != eventFieldPageSize || !far.HasMore {
		t.Fatalf("field page past old 300 limit unavailable: %+v", far)
	}
	if !strings.Contains(far.Items[0].Label, "[600]") || far.Items[0].Value != "item-0600" {
		t.Fatalf("pagination lost deterministic order: %+v", far.Items[0])
	}
	final := eventDetailTreePage(detail, "", 720, eventFieldPageSize)
	if len(final.Items) != 5 || final.HasMore {
		t.Fatalf("last page incorrect: %+v", final)
	}
	late := eventDetailTreePage(detail, "item-0724", 0, 40)
	if len(late.Items) != 1 || late.Items[0].Value != "item-0724" {
		t.Fatalf("search missed late matching value: %+v", late)
	}
}

func TestEventDetailFieldBrowserLimitsDeepAndHugePayloads(t *testing.T) {
	nested := map[string]any{"value": "deep"}
	for i := 0; i < eventFieldDepthLimit+3; i++ {
		nested = map[string]any{"nested": nested}
	}
	result := eventDetailTreePage(nested, "", 0, 100)
	if !result.ScanLimited || !result.HasMore {
		t.Fatalf("deep nesting should be explicit about skipped data: %+v", result)
	}
	large := make([]any, eventFieldScanLimit+25)
	for i := range large { large[i] = map[string]any{"index": i} }
	limited := eventDetailTreePage(map[string]any{"data": large}, "nonexistent-field-name", 0, eventFieldPageSize)
	if !limited.ScanLimited || limited.Scanned > eventFieldScanLimit+1 {
		t.Fatalf("large payload traversed without bound: %+v", limited)
	}
}

func TestEventDetailGenerationRejectsOldSameIDResponse(t *testing.T) {
	a := &renewApp{eventDetailOpen: true, eventDetailID: "evt_one", eventDetailGeneration: 7}
	if !a.acceptsEventDetailResponse("evt_one", 7) {
		t.Fatal("current detail request was rejected")
	}
	a.releaseEventDetailPayload()
	a.eventDetailOpen = true
	a.eventDetailID = "evt_one"
	if a.acceptsEventDetailResponse("evt_one", 7) {
		t.Fatal("old response was accepted after close/reopen of the same event")
	}
	if !a.acceptsEventDetailResponse("evt_one", 8) {
		t.Fatal("new response was rejected")
	}
	a.eventDetailOpen = false
	if a.acceptsEventDetailResponse("evt_one", 8) {
		t.Fatal("closed modal should reject a late response")
	}
}

func TestEventDetailFieldExpansionPreservesCopy(t *testing.T) {
	full := "/home/user/" + strings.Repeat("非常长的文件路径/", 55)
	key := ""
	tester := ui.NewTester(func(c *ui.Context) {
		eventDetailFieldRowAdaptive(c, eventDetailField{
			Label: "目标路径", Value: full, Origin: "Event.path",
		}, 290, &key)
	}, 320, 300)
	tester.Frame()
	if !tester.HasText("展开") {
		t.Fatal("long field has no expand control")
	}
	if err := tester.Click("展开"); err != nil {
		t.Fatalf("cannot expand full field: %v", err)
	}
	tester.Frame()
	if key != "Event.path" || !tester.HasText("收起") {
		t.Fatalf("field did not expand: %q", key)
	}
	if err := tester.Click("复制"); err != nil {
		t.Fatalf("cannot copy long field: %v", err)
	}
	if got := tester.Clipboard(); got != full {
		t.Fatalf("copied value truncated: %q", got)
	}
}

func TestEnforcementRequiresFreshStateAndExplicitConfirmation(t *testing.T) {
	a := &renewApp{
		eventDetailOpen: true,
		eventDetailID: "evt_policy",
		eventDetail: map[string]any{"Event": map[string]any{
			"type": "connect", "netEndpoint": "192.0.2.10:443",
		}},
	}
	a.runtimeCfg.Runtime.PolicyManagementEnabled = true
	tester := ui.NewTester(func(c *ui.Context) {
		a.eventDetailEnforcement(c)
	}, 780, 400)
	tester.Frame()
	if !tester.HasText("当前没有可验证的新鲜策略状态") &&
		!tester.HasText("当前没有可验证的新鲜策略状态，已禁用修改操作。请重新加载事件详情。") {
		t.Fatal("stale state did not block privileged UI")
	}
	a.eventDetailEnforcementReady = true
	tester.Frame()
	if err := tester.Click("选择 · 阻断 IP 192.0.2.10"); err != nil {
		t.Fatalf("policy selection missing: %v", err)
	}
	tester.Frame()
	if a.eventDetailPendingAction == "" || !tester.HasText("确认修改策略") {
		t.Fatal("policy requires confirmation before execution")
	}
	if err := tester.Click("取消"); err != nil {
		t.Fatalf("cannot cancel policy choice: %v", err)
	}
	if a.eventDetailPendingAction != "" {
		t.Fatal("policy choice was not cancelled")
	}
}
