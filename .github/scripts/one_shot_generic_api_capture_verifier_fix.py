from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"missing verifier-fix anchor in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "backend/ebpf/agent_tracker_common.h",
    r'''// Copy only the HTTP/1 request target/request line, never headers/body. Query
// and fragment bytes are removed in-kernel before the sample crosses ringbuf,
// so API keys embedded in URLs never reach userspace through this path.
static __always_inline u32 capture_http1_request_line(char *dst, const void *user_buf, u32 len) {
    if (!dst || !user_buf || len < 4) return 0;
    char head[8] = {};
    u32 head_len = len < sizeof(head) ? len : sizeof(head);
    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return 0;
    if (!looks_like_http1_method(head, head_len)) return 0;

    u32 capture_len = len;
    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;
    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) return 0;

    u32 sanitized_len = capture_len;
    u32 redact_tail = 0;
#pragma clang loop unroll(disable)
    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {
        if ((u32)i >= capture_len) break;
        if (redact_tail) {
            dst[i] = '\0';
            continue;
        }
        char c = dst[i];
        if (c == '?' || c == '#' || c == '\r' || c == '\n') {
            dst[i] = '\0';
            sanitized_len = (u32)i;
            redact_tail = 1;
        }
    }
    dst[capture_len] = '\0';
    return sanitized_len;
}
''',
    r'''// Copy only the HTTP/1 request target/request line, never headers/body. The
// scratch sample is NUL-terminated at query/fragment/line-end. sys_exit then
// uses bpf_probe_read_kernel_str(), so bytes after that delimiter never cross
// into the ringbuf event. This avoids verifier state explosion from clearing a
// dynamic 255-byte tail one byte at a time.
static __always_inline u32 capture_http1_request_line(char *dst, const void *user_buf, u32 len) {
    if (!dst || !user_buf || len < 4) return 0;
    char head[8] = {};
    u32 head_len = len < sizeof(head) ? len : sizeof(head);
    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return 0;
    if (!looks_like_http1_method(head, head_len)) return 0;

    u32 capture_len = len;
    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;
    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) return 0;
#pragma clang loop unroll(disable)
    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {
        if ((u32)i >= capture_len) break;
        char c = dst[i];
        if (c == '?' || c == '#' || c == '\r' || c == '\n') {
            dst[i] = '\0';
            return (u32)i;
        }
    }
    dst[capture_len] = '\0';
    return capture_len;
}
''')

replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    r'''    if (pd) { \
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN); \
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN); \
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid); \
    } \
''',
    r'''    if (pd) { \
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN); \
        if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) { \
            bpf_probe_read_kernel_str(e->extra4, MAX_PATH_LEN, pd->extra4); \
        } else { \
            __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN); \
        } \
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid); \
    } \
''')

replace_once(
    "docs/backend/generic-api-capture.md",
    "Query strings and fragments are truncated and their captured tail bytes are zeroed **inside eBPF before ringbuf submission**, so URL credentials do not cross this kernel/userspace boundary.",
    "Query strings and fragments are NUL-truncated **inside eBPF**; the exit path copies the sanitized prefix into the already-zeroed ringbuf event with `bpf_probe_read_kernel_str`, so bytes beyond the delimiter never cross the kernel/userspace boundary."
)
