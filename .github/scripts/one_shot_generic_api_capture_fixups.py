from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def function_segment(text, function_name, end_marker):
    start_marker = f"int {function_name}("
    start = text.find(start_marker)
    if start < 0:
        raise SystemExit(f"missing function {function_name}")
    end = text.find(end_marker, start)
    if end < 0:
        raise SystemExit(f"missing end marker for {function_name}: {end_marker!r}")
    return start, end, text[start:end]


# The staging script uses a deliberately broad anchor for the connect-exit
# reserve block. Remove that inserted snippet from whichever earlier syscall it
# matched, then insert it into the connect exit function explicitly. Keeping
# this update before reserve_event() means fd provenance survives audit-ring
# pressure: capture telemetry loss must not corrupt the socket identity map.
path = "backend/ebpf/agent_tracker_common.h"
text = read(path)
remote_update = """    if (ctx->ret == 0) {\n        update_socket_fd_remote((u32)(pid_tgid >> 32), (s32)meta.extra1, &meta);\n    }\n\n"""
count = text.count(remote_update)
if count != 1:
    raise SystemExit(f"expected exactly one staged remote update, got {count}")
text = text.replace(remote_update, "", 1)
start, end, segment = function_segment(
    text,
    "tracepoint__syscalls__sys_exit_connect",
    "// ============================================================\n// sys_enter / sys_exit: mkdirat",
)
anchor = "    if (!consume_exit_meta(pid_tgid, &meta)) return 0;\n\n"
if anchor not in segment:
    raise SystemExit("missing connect consume anchor")
segment = segment.replace(anchor, anchor + remote_update, 1)
text = text[:start] + segment + text[end:]
write(path, text)


# sendto may discover the HTTP request-line after the initial metadata setup.
# Persist exit_meta only after that classification so sys_exit observes the
# final event type and captured-prefix length.
path = "backend/ebpf/agent_tracker_syscalls.h"
text = read(path)
start, end, segment = function_segment(
    text,
    "tracepoint__syscalls__sys_enter_sendto",
    "DEFINE_GENERIC_EXIT_HANDLER(sendto)",
)
store = "    store_exit_meta(pid_tgid, &meta);\n"
if segment.count(store) != 1:
    raise SystemExit(f"sendto store count = {segment.count(store)}, want 1")
segment = segment.replace(store, "", 1)
return_anchor = "    return 0;\n}\n"
if return_anchor not in segment:
    raise SystemExit("missing sendto return anchor")
segment = segment.replace(return_anchor, store + return_anchor, 1)
text = text[:start] + segment + text[end:]
write(path, text)


# events is normally wired by the parent app package. This package-level unit
# test runs events in isolation, so inject only the two callbacks reached by a
# socket_http event and keep dst_port=0 to avoid exercising the flow subsystem.
write("backend/app/events/kernel_socket_http_test.go", r'''package events

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
''')
