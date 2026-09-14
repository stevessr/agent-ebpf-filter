package events

import (
	"testing"

	"agent-ebpf-filter/pb"
)

func TestBuildKernelSocketHTTPEventSanitizesAndClassifiesPath(t *testing.T) {
	oldGetTagName := Deps.GetTagName
	oldApplyRisk := Deps.ApplyKernelRiskDecision
	Deps.GetTagName = func(uint32) string { return "test" }
	Deps.ApplyKernelRiskDecision = func(*BpfEvent, *pb.Event) {}
	defer func() {
		Deps.GetTagName = oldGetTagName
		Deps.ApplyKernelRiskDecision = oldApplyRisk
	}()

	var raw BpfEvent
	raw.PID = 123
	raw.TGID = 123
	raw.Type = 43
	raw.Extra1 = 9
	raw.Extra2 = 53
	raw.Extra3 = 4096
	raw.NetFamily = 2
	raw.NetDirection = 1
	copy(raw.Comm[:], "agent")
	copy(raw.Path[:], "socket http")
	copy(raw.Extra4[:], "POST /v1/responses?api_key=top-secret HTTP/1.1")

	event := BuildKernelEvent(raw)
	if event.GetType() != "socket_http" || event.GetCaptureSource() != "kernel_socket_prefix" || event.GetAppProtocol() != "http1" {
		t.Fatalf("unexpected event: %+v", event)
	}
	if event.GetHttpMethod() != "POST" || event.GetHttpPath() != "/v1/responses" {
		t.Fatalf("method/path = %q %q", event.GetHttpMethod(), event.GetHttpPath())
	}
	if event.GetExtraPath() != "POST /v1/responses" {
		t.Fatalf("raw query leaked through ExtraPath: %q", event.GetExtraPath())
	}
	if event.GetApiProfile() != "openai-compatible.responses" || event.GetApiVendor() != "openai-compatible" {
		t.Fatalf("profile = %q vendor=%q", event.GetApiProfile(), event.GetApiVendor())
	}
	if event.GetKernelSocketFd() != 9 || event.GetKernelPayloadPrefixLen() != 53 || event.GetKernelCaptureFlags() != 1 {
		t.Fatalf("kernel capture metadata missing: %+v", event)
	}
}
