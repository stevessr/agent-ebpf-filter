from pathlib import Path
import re


def read(path):
    return Path(path).read_text(errors="surrogateescape")


def write(path, content):
    Path(path).write_text(content)


def replace_all(path, old, new, minimum=1):
    text = read(path)
    count = text.count(old)
    if count < minimum:
        raise SystemExit(f"expected >= {minimum} replacements in {path}, got {count}: {old[:100]!r}")
    write(path, text.replace(old, new))


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing anchor in {path}: {old[:120]!r}")
    write(path, text.replace(old, new, 1))


common = "backend/ebpf/agent_tracker_common.h"
text = read(common)
start = text.find("#define HTTP1_START_NONE")
end = text.find("// Convenience inline", start)
if start < 0 or end < 0:
    raise SystemExit("unable to locate staged HTTP1 classifier region")
classifier = r'''#define HTTP1_START_NONE 0
#define HTTP1_START_REQUEST 1
#define HTTP1_START_RESPONSE 2

static __always_inline int looks_like_http1_response(const char *head, u32 len) {
    if (!head || len < 8) return 0;
    return head[0] == 'H' && head[1] == 'T' && head[2] == 'T' && head[3] == 'P' &&
           head[4] == '/' && head[5] == '1' && head[6] == '.';
}

// Classify and copy an HTTP/1 start-line with a single 8-byte probe. The old
// request-then-response path probed non-HTTP buffers twice. This helper keeps
// identical privacy semantics while halving the head probes on the hot path.
static __always_inline u32 capture_http1_start_line(char *dst, const void *user_buf, u32 len, u32 *kind) {
    if (kind) *kind = HTTP1_START_NONE;
    if (!dst || !user_buf || len < 4 || !kind) return 0;
    char head[8] = {};
    u32 head_len = len < sizeof(head) ? len : sizeof(head);
    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return 0;

    int request = looks_like_http1_method(head, head_len);
    int response = !request && looks_like_http1_response(head, head_len);
    if (!request && !response) return 0;
    *kind = request ? HTTP1_START_REQUEST : HTTP1_START_RESPONSE;

    u32 capture_len = len;
    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;
    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) {
        *kind = HTTP1_START_NONE;
        return 0;
    }
#pragma clang loop unroll(disable)
    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {
        if ((u32)i >= capture_len) break;
        char c = dst[i];
        if (c == '\r' || c == '\n' || (request && (c == '?' || c == '#'))) {
            dst[i] = '\0';
            return (u32)i;
        }
    }
    dst[capture_len] = '\0';
    return capture_len;
}

'''
write(common, text[:start] + classifier + text[end:])

# exit_ctx needs its own socket_type scratch field. capture_reserved is already
# used by readv to carry iovcnt across enter/exit and must not be overloaded.
replace_once(common,
'''    u32 capture_flags;
    u32 capture_reserved;
};''',
'''    u32 capture_flags;
    u32 capture_reserved;
    u32 socket_type;
    u32 socket_reserved;
};''')
replace_once(common,
'''    meta->capture_flags |= socket->provenance_flags;
    meta->capture_reserved = socket->sock_type;
    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));''',
'''    meta->capture_flags |= socket->provenance_flags;
    meta->socket_type = socket->sock_type;
    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));''')
replace_once(common,
'''    e->kernel_capture_flags = meta->capture_flags;
    e->kernel_capture_reserved = meta->capture_reserved;
    __builtin_memcpy(e->net_addr, meta->net_addr, 16);''',
'''    e->kernel_capture_flags = meta->capture_flags;
    e->kernel_capture_reserved = meta->socket_type;
    __builtin_memcpy(e->net_addr, meta->net_addr, 16);''')

syscalls = "backend/ebpf/agent_tracker_syscalls.h"
# Any explicit sockaddr path that already has socket provenance must carry the
# socket type too; connected paths get it from fill_network_meta_from_socket*().
replace_all(syscalls,
'''        meta.capture_flags |= socket->provenance_flags;
''',
'''        meta.capture_flags |= socket->provenance_flags;
        meta.socket_type = socket->sock_type;
''', minimum=1)

# read(2) incoming pair.
replace_once(syscalls,
'''        captured = capture_http1_request_line(pd->extra4, (const void *)meta.addr_ptr, actual);
        if (captured > 0) {
            meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_INCOMING;
        } else {
            captured = capture_http1_response_line(pd->extra4, (const void *)meta.addr_ptr, actual);
            if (captured > 0) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_INCOMING;
        }''',
'''        u32 start_kind = HTTP1_START_NONE;
        captured = capture_http1_start_line(pd->extra4, (const void *)meta.addr_ptr, actual, &start_kind);
        if (captured > 0) {
            meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
            if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
            else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
        }''')

# write(2) outgoing pair.
replace_once(syscalls,
'''            u32 captured = capture_http1_request_line(pd->extra4, (const void *)ctx->args[1], requested);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_OUTGOING;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                captured = capture_http1_response_line(pd->extra4, (const void *)ctx->args[1], requested);
                if (captured > 0) {
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                    meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_OUTGOING;
                    __builtin_memcpy(pd->path, "socket http", 12);
                } else {
                    __builtin_memcpy(pd->path, "socket write", 13);
                }
            }''',
'''            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], requested, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                __builtin_memcpy(pd->path, "socket write", 13);
            }''')

# writev(2) outgoing pair.
replace_once(syscalls,
'''            u32 captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, first_len);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_OUTGOING;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                captured = capture_http1_response_line(pd->extra4, (const void *)iov.base, first_len);
                if (captured > 0) {
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                    meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_OUTGOING;
                    __builtin_memcpy(pd->path, "socket http", 12);
                } else {
                    __builtin_memcpy(pd->path, "socket writev", 14);
                }
            }''',
'''            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                __builtin_memcpy(pd->path, "socket writev", 14);
            }''')

# readv(2) incoming pair.
replace_once(syscalls,
'''            captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, available);
            if (captured > 0) {
                meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_INCOMING;
            } else {
                captured = capture_http1_response_line(pd->extra4, (const void *)iov.base, available);
                if (captured > 0) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_INCOMING;
            }''',
'''            u32 start_kind = HTTP1_START_NONE;
            captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, available, &start_kind);
            if (captured > 0) {
                meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
            }''')

# sendmsg(2) outgoing pair.
replace_once(syscalls,
'''            u32 captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, first_len);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_OUTGOING;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                captured = capture_http1_response_line(pd->extra4, (const void *)iov.base, first_len);
                if (captured > 0) {
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                    meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_OUTGOING;
                    __builtin_memcpy(pd->path, "socket http", 12);
                }
            }''',
'''            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                __builtin_memcpy(pd->path, "socket http", 12);
            }''')

# recvmsg(2) incoming pair.
replace_once(syscalls,
'''                captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, available);
                if (captured > 0) {
                    meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_INCOMING;
                } else {
                    captured = capture_http1_response_line(pd->extra4, (const void *)iov.base, available);
                    if (captured > 0) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_INCOMING;
                }''',
'''                u32 start_kind = HTTP1_START_NONE;
                captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, available, &start_kind);
                if (captured > 0) {
                    meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
                    if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                    else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                }''')

# Ensure the staged sendto replacement (already fused by the first script) also
# propagates socket type in the explicit sockaddr branch through replace_all.
remaining_request = read(syscalls).count("capture_http1_request_line(")
remaining_response = read(syscalls).count("capture_http1_response_line(")
if remaining_request or remaining_response:
    raise SystemExit(f"unfused HTTP1 parser calls remain: request={remaining_request} response={remaining_response}")

# Add a same-build linear baseline benchmark to quantify the dispatch win.
bench = "backend/app/captureprofile/profile_bench_test.go"
text = read(bench)
if "BenchmarkRegistryMatchLinear512" not in text:
    text += r'''

func linearMatchForBenchmark(registry *Registry, observation Observation) (Match, bool) {
    snapshot := registry.snapshot.Load()
    observation = normalizeObservation(observation)
    best := Match{}
    matched := false
    for _, profile := range snapshot.profiles {
        candidate, ok := matchProfile(profile, observation)
        if !ok {
            continue
        }
        if !matched || candidate.Score > best.Score || (candidate.Score == best.Score && candidate.ProfileID < best.ProfileID) {
            best = candidate
            matched = true
        }
    }
    return best, matched
}

func BenchmarkRegistryMatchLinear512(b *testing.B) {
    registry := benchmarkRegistry(512)
    observation := Observation{Source: "kernel_socket_prefix", Protocol: "http1", Direction: "outgoing", Method: "POST", Transport: "tcp", Host: "target.example.test", Path: "/v1/jobs/42"}
    b.ReportAllocs()
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        if match, ok := linearMatchForBenchmark(registry, observation); !ok || match.ProfileID != "target" {
            b.Fatal("target profile did not match")
        }
    }
}
'''
    write(bench, text)
