from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"missing phase2 test anchor in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))


path = "backend/app/events/kernel_socket_http_test.go"
replace_once(
    path,
    '''\traw.NetFamily = 2\n\traw.NetDirection = 1\n''',
    '''\traw.NetFamily = 2\n\traw.NetDirection = 1\n\traw.KernelCaptureFlags = kernelCaptureHTTPRequest | kernelCaptureOutgoing\n''')
replace_once(
    path,
    '''event.GetKernelPayloadPrefixLen() != 53 || event.GetKernelCaptureFlags() != 1''',
    '''event.GetKernelPayloadPrefixLen() != 53 || event.GetKernelCaptureFlags() != kernelCaptureHTTPRequest|kernelCaptureOutgoing''')

p = Path(path)
text = p.read_text()
text += r'''

func TestBuildKernelSocketHTTPResponseEvent(t *testing.T) {
	oldGetTagName := Deps.GetTagName
	oldApplyRisk := Deps.ApplyKernelRiskDecision
	Deps.GetTagName = func(uint32) string { return "test" }
	Deps.ApplyKernelRiskDecision = func(*BpfEvent, *pb.Event) {}
	defer func() {
		Deps.GetTagName = oldGetTagName
		Deps.ApplyKernelRiskDecision = oldApplyRisk
	}()

	var raw BpfEvent
	raw.PID = 124
	raw.TGID = 124
	raw.Type = 43
	raw.Extra1 = 11
	raw.Extra2 = 28
	raw.Extra3 = 512
	raw.NetFamily = 2
	raw.NetDirection = 2
	raw.KernelCaptureFlags = kernelCaptureHTTPResponse | kernelCaptureIncoming | kernelCaptureFDInherited
	copy(raw.Comm[:], "agent")
	copy(raw.Path[:], "socket http")
	copy(raw.Extra4[:], "HTTP/1.1 429 Too Many Requests")

	event := BuildKernelEvent(raw)
	if event.GetHttpStatus() != 429 || event.GetKernelSocketFd() != 11 {
		t.Fatalf("response event = %+v", event)
	}
	if event.GetKernelCaptureFlags() != raw.KernelCaptureFlags || event.GetNetDirection() != "incoming" {
		t.Fatalf("response provenance = %+v", event)
	}
}
'''
p.write_text(text)
