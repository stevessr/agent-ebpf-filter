from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing phase2b anchor in {path}: {old[:180]!r}")
    write(path, text.replace(old, new, 1))


path = "backend/ebpf/agent_tracker_syscalls.h"
replace_once(
    path,
    '''// This file replaces the repetitive handlers in agent_tracker_syscalls.h\n\n// Generic enter handler macro for simple syscalls\n''',
    r'''// This file replaces the repetitive handlers in agent_tracker_syscalls.h

// 64-bit userspace scatter/gather ABI. We deliberately inspect only the first
// iovec for bounded HTTP start-line metadata; payload/header bodies are never
// walked. Native x86_64/arm64 are covered. compat32 tasks fall back to the
// ordinary syscall event instead of decoding a mismatched pointer layout.
struct capture_iovec64 {
    u64 base;
    u64 len;
};

struct capture_msghdr64 {
    u64 msg_name;
    u32 msg_namelen;
    u32 pad0;
    u64 msg_iov;
    u64 msg_iovlen;
    u64 msg_control;
    u64 msg_controllen;
    u32 msg_flags;
    u32 pad1;
};

static __always_inline int capture_first_iovec(const void *iov_ptr, u64 iovcnt, struct capture_iovec64 *iov) {
    if (!iov_ptr || !iov || iovcnt == 0) return 0;
    if (bpf_probe_read_user(iov, sizeof(*iov), iov_ptr) < 0) return 0;
    if (iov->base == 0 || iov->len == 0) return 0;
    return 1;
}

static __always_inline int capture_first_msghdr_iovec(const void *msg_ptr, struct capture_msghdr64 *msg, struct capture_iovec64 *iov) {
    if (!msg_ptr || !msg || !iov) return 0;
    if (bpf_probe_read_user(msg, sizeof(*msg), msg_ptr) < 0) return 0;
    return capture_first_iovec((const void *)msg->msg_iov, msg->msg_iovlen, iov);
}

static __always_inline u32 capture_iov_len(u64 length) {
    return length > 0xffffffffULL ? 0xffffffffU : (u32)length;
}

// Generic enter handler macro for simple syscalls
''')

anchor = '''DEFINE_GENERIC_EXIT_HANDLER(write)\n\nDEFINE_SIMPLE_ENTER_HANDLER(open, TYPE_OPEN, "file open")\n'''
scatter = r'''DEFINE_GENERIC_EXIT_HANDLER(write)

// writev/readv: first-iovec bounded metadata capture. The real byte count from
// sys_exit is preserved in extra3/net_bytes, while capture_flags records that
// the L7 prefix originated from scatter/gather I/O.
SEC("tracepoint/syscalls/sys_enter_writev")
int tracepoint__syscalls__sys_enter_writev(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;

    s32 fd = (s32)ctx->args[0];
    struct capture_iovec64 iov = {};
    int have_iov = capture_first_iovec((const void *)ctx->args[1], ctx->args[2], &iov);
    u32 first_len = have_iov ? capture_iov_len(iov.len) : 0;
    struct exit_meta meta = {.type = TYPE_WRITE, .tag_id = tag_id, .extra1 = (u32)fd, .extra3 = first_len};
    struct socket_fd_meta *socket = lookup_socket_fd(tgid, fd);
    if (socket) {
        fill_network_meta_from_socket(&meta, socket, first_len);
        meta.capture_flags |= SOCKET_CAPTURE_SCATTER_GATHER;
    }

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        if (socket && have_iov) {
            u32 captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, first_len);
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
            }
        } else if (socket) {
            __builtin_memcpy(pd->path, "socket writev", 14);
        } else {
            __builtin_memcpy(pd->path, "file writev", 12);
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_writev")
int tracepoint__syscalls__sys_exit_writev(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        meta.net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) {
            bpf_probe_read_kernel_str(e->extra4, MAX_PATH_LEN, pd->extra4);
        }
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }
    submit_event(e);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_readv")
int tracepoint__syscalls__sys_enter_readv(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    s32 fd = (s32)ctx->args[0];
    struct exit_meta meta = {.type = TYPE_READ, .tag_id = tag_id, .extra1 = (u32)fd, .addr_ptr = ctx->args[1], .capture_reserved = (u32)ctx->args[2]};
    struct socket_fd_meta *socket = lookup_socket_fd(tgid, fd);
    if (socket) {
        meta.extra2 = 1;
        fill_network_meta_from_socket_direction(&meta, socket, 0, NET_DIR_INCOMING);
        meta.capture_flags |= SOCKET_CAPTURE_SCATTER_GATHER;
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_readv")
int tracepoint__syscalls__sys_exit_readv(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    u32 captured = 0;
    if (ctx->ret > 0 && meta.extra2 == 1 && pd && meta.addr_ptr != 0 && meta.capture_reserved > 0) {
        struct capture_iovec64 iov = {};
        if (capture_first_iovec((const void *)meta.addr_ptr, meta.capture_reserved, &iov)) {
            u32 available = capture_iov_len(iov.len);
            if ((u64)ctx->ret < available) available = (u32)ctx->ret;
            captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, available);
            if (captured > 0) {
                meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_INCOMING;
            } else {
                captured = capture_http1_response_line(pd->extra4, (const void *)iov.base, available);
                if (captured > 0) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_INCOMING;
            }
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
            }
        }
    }
    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        meta.net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (pd && captured > 0) {
        __builtin_memcpy(e->path, "socket http", 12);
        bpf_probe_read_kernel_str(e->extra4, MAX_PATH_LEN, pd->extra4);
    } else if (meta.extra2 == 1) {
        __builtin_memcpy(e->path, "socket readv", 13);
    } else {
        __builtin_memcpy(e->path, "file readv", 11);
    }
    submit_event(e);
    return 0;
}

// sendmsg/recvmsg: native 64-bit msghdr only, first iovec only. This catches
// runtimes which coalesce protocol metadata through scatter/gather without
// copying arbitrary headers or bodies into kernel telemetry.
SEC("tracepoint/syscalls/sys_enter_sendmsg")
int tracepoint__syscalls__sys_enter_sendmsg(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    s32 fd = (s32)ctx->args[0];
    struct capture_msghdr64 msg = {};
    struct capture_iovec64 iov = {};
    int have_iov = capture_first_msghdr_iovec((const void *)ctx->args[1], &msg, &iov);
    u32 first_len = have_iov ? capture_iov_len(iov.len) : 0;
    struct exit_meta meta = {.type = TYPE_SENDTO, .tag_id = tag_id, .extra1 = (u32)fd, .extra3 = first_len};
    if (msg.msg_name != 0 && msg.msg_namelen != 0) {
        fill_network_meta(&meta, (const void *)msg.msg_name, NET_DIR_OUTGOING, first_len);
    }
    struct socket_fd_meta *socket = lookup_socket_fd(tgid, fd);
    if (meta.net_family == 0 && socket) {
        fill_network_meta_from_socket(&meta, socket, first_len);
    } else if (socket) {
        meta.capture_flags |= socket->provenance_flags;
    }
    if (socket) meta.capture_flags |= SOCKET_CAPTURE_SCATTER_GATHER;

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket sendmsg", 15);
        if (socket && have_iov) {
            u32 captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, first_len);
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
            }
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_sendmsg")
int tracepoint__syscalls__sys_exit_sendmsg(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        meta.net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) bpf_probe_read_kernel_str(e->extra4, MAX_PATH_LEN, pd->extra4);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }
    submit_event(e);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_recvmsg")
int tracepoint__syscalls__sys_enter_recvmsg(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    s32 fd = (s32)ctx->args[0];
    struct exit_meta meta = {.type = TYPE_RECVFROM, .tag_id = tag_id, .extra1 = (u32)fd, .extra2 = 0, .addr_ptr = ctx->args[1]};
    struct socket_fd_meta *socket = lookup_socket_fd(tgid, fd);
    if (socket) {
        meta.extra2 = 1;
        fill_network_meta_from_socket_direction(&meta, socket, 0, NET_DIR_INCOMING);
        meta.capture_flags |= SOCKET_CAPTURE_SCATTER_GATHER;
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_recvmsg")
int tracepoint__syscalls__sys_exit_recvmsg(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    u32 captured = 0;
    if (ctx->ret > 0 && meta.extra2 == 1 && meta.addr_ptr != 0) {
        struct capture_msghdr64 msg = {};
        struct capture_iovec64 iov = {};
        if (capture_first_msghdr_iovec((const void *)meta.addr_ptr, &msg, &iov)) {
            if (msg.msg_name != 0 && msg.msg_namelen != 0) {
                struct exit_meta peer = {};
                fill_network_meta(&peer, (const void *)msg.msg_name, NET_DIR_INCOMING, (u32)ctx->ret);
                if (peer.net_family != 0) {
                    meta.net_family = peer.net_family;
                    meta.net_port = peer.net_port;
                    __builtin_memcpy(meta.net_addr, peer.net_addr, sizeof(meta.net_addr));
                    update_socket_fd_remote(tgid, (s32)meta.extra1, &peer);
                }
            }
            if (pd) {
                u32 available = capture_iov_len(iov.len);
                if ((u64)ctx->ret < available) available = (u32)ctx->ret;
                captured = capture_http1_request_line(pd->extra4, (const void *)iov.base, available);
                if (captured > 0) {
                    meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_INCOMING;
                } else {
                    captured = capture_http1_response_line(pd->extra4, (const void *)iov.base, available);
                    if (captured > 0) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_INCOMING;
                }
                if (captured > 0) {
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                }
            }
        }
    }
    if (ctx->ret > 0) {
        meta.extra3 = (u32)ctx->ret;
        meta.net_bytes = (u32)ctx->ret;
    }
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (pd && captured > 0) {
        __builtin_memcpy(e->path, "socket http", 12);
        bpf_probe_read_kernel_str(e->extra4, MAX_PATH_LEN, pd->extra4);
    } else {
        __builtin_memcpy(e->path, "socket recvmsg", 15);
    }
    submit_event(e);
    return 0;
}

DEFINE_SIMPLE_ENTER_HANDLER(open, TYPE_OPEN, "file open")
'''
replace_once(path, anchor, scatter)

# Regression surface: mark the existing kernel-event test as scatter/gather too,
# proving raw flags pass through normalization without being guessed in Go.
path = "backend/app/events/kernel_socket_http_test.go"
replace_once(
    path,
    "raw.KernelCaptureFlags = kernelCaptureHTTPRequest | kernelCaptureOutgoing\n",
    "raw.KernelCaptureFlags = kernelCaptureHTTPRequest | kernelCaptureOutgoing | kernelCaptureScatterGather\n")
replace_once(
    path,
    "event.GetKernelCaptureFlags() != kernelCaptureHTTPRequest|kernelCaptureOutgoing",
    "event.GetKernelCaptureFlags() != kernelCaptureHTTPRequest|kernelCaptureOutgoing|kernelCaptureScatterGather")

path = "docs/backend/generic-api-capture.md"
text = read(path)
text += r'''

## Scatter/gather syscall coverage

The plaintext kernel sampler also covers native 64-bit `writev`, `readv`, `sendmsg`, and `recvmsg`. To keep both verifier cost and privacy exposure bounded, only the first iovec is inspected and only enough bytes for an HTTP/1 start-line candidate are copied into the per-CPU scratch buffer. The body, subsequent iovecs, authorization headers, cookies, and arbitrary message control data are not captured.

`kernel_capture_flags` includes `SCATTER_GATHER`, so downstream analysis can distinguish this best-effort prefix from a contiguous `read`/`write` sample. `sendmsg` uses `msg_name` when present and otherwise reuses connected-fd provenance; `recvmsg` can refresh the peer endpoint after return. compat32 user tasks are intentionally not decoded as 64-bit `iovec/msghdr`; they retain ordinary syscall telemetry rather than risking pointer-layout misinterpretation.
'''
write(path, text)
