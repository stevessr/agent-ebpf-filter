from pathlib import Path

P = Path("backend/ebpf/agent_tracker_syscalls.h")
text = P.read_text()


def replace_once(old, new):
    global text
    if old not in text:
        raise SystemExit(f"missing anchor: {old[:180]!r}")
    text = text.replace(old, new, 1)


# Dynamic exit-path helpers: outgoing capture now persists context only after a
# real HTTP/1 start-line hit. Static labels are written directly on sys_exit.
anchor = '''static __always_inline u32 capture_iov_len(u64 length) {
    return length > 0xffffffffULL ? 0xffffffffU : (u32)length;
}
'''
insert = anchor + '''
static __always_inline void discard_dynamic_exit_path(u64 pid_tgid) {
    bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
}

static __always_inline void fill_dynamic_http_exit(struct event *e, u64 pid_tgid) {
    __builtin_memcpy(e->path, "socket http", 12);
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        bpf_probe_read_kernel_str(e->extra4, MAX_PATH_LEN, pd->extra4);
    }
    bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
}
'''
replace_once(anchor, insert)

# sendto: per-CPU scratch only when a probe is eligible; hash context only when
# the probe actually recognized HTTP.
old = '''    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket sendto", 14);
        u32 data_len = (u32)ctx->args[2];
        u32 start_kind = HTTP1_START_NONE;
        u32 captured = (!socket || socket_http1_capture_eligible(socket)) ? capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], data_len, &start_kind) : 0;
        if (captured > 0) {
            meta.type = TYPE_SOCKET_HTTP;
            meta.extra2 = captured;
            meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
            if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
            else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
            __builtin_memcpy(pd->path, "socket http", 12);
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}
DEFINE_GENERIC_EXIT_HANDLER(sendto)
'''
new = '''    if (!socket || socket_http1_capture_eligible(socket)) {
        u32 zero = 0;
        struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
        if (pd) {
            u32 data_len = (u32)ctx->args[2];
            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], data_len, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
            }
        }
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_sendto")
int tracepoint__syscalls__sys_exit_sendto(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    struct event *e = reserve_event();
    if (!e) {
        if (meta.type == TYPE_SOCKET_HTTP) discard_dynamic_exit_path(pid_tgid);
        return 0;
    }
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) {
        fill_dynamic_http_exit(e, pid_tgid);
    } else {
        __builtin_memcpy(e->path, "socket sendto", 14);
    }
    submit_event(e);
    return 0;
}
'''
replace_once(old, new)

# write: do not touch exit_path_buf/ctx for files, known non-stream sockets, or
# ordinary encrypted/binary stream writes.
old = '''    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        if (socket) {
            u32 start_kind = HTTP1_START_NONE;
            u32 captured = socket_http1_capture_eligible(socket) ? capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], requested, &start_kind) : 0;
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                __builtin_memcpy(pd->path, "socket write", 13);
            }
        } else {
            __builtin_memcpy(pd->path, "file write", 11);
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}
DEFINE_GENERIC_EXIT_HANDLER(write)
'''
new = '''    if (socket_http1_capture_eligible(socket)) {
        u32 zero = 0;
        struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
        if (pd) {
            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)ctx->args[1], requested, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
            }
        }
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_write")
int tracepoint__syscalls__sys_exit_write(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    struct event *e = reserve_event();
    if (!e) {
        if (meta.type == TYPE_SOCKET_HTTP) discard_dynamic_exit_path(pid_tgid);
        return 0;
    }
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) {
        fill_dynamic_http_exit(e, pid_tgid);
    } else if (meta.socket_type != 0) {
        __builtin_memcpy(e->path, "socket write", 13);
    } else {
        __builtin_memcpy(e->path, "file write", 11);
    }
    submit_event(e);
    return 0;
}
'''
replace_once(old, new)

# writev enter: persist exit_path_ctx only for a recognized dynamic start-line.
old = '''    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        if (socket && have_iov) {
            u32 start_kind = HTTP1_START_NONE;
            u32 captured = socket_http1_capture_eligible(socket) ? capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind) : 0;
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                __builtin_memcpy(pd->path, "socket http", 12);
            } else {
                __builtin_memcpy(pd->path, "socket writev", 14);
            }
        } else if (socket) {
            __builtin_memcpy(pd->path, "socket writev", 14);
        } else {
            __builtin_memcpy(pd->path, "file writev", 12);
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
'''
new = '''    if (socket_http1_capture_eligible(socket) && have_iov) {
        u32 zero = 0;
        struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
        if (pd) {
            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
            }
        }
    }
'''
replace_once(old, new)

old = '''    struct event *e = reserve_event();
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
'''
new = '''    struct event *e = reserve_event();
    if (!e) {
        if (meta.type == TYPE_SOCKET_HTTP) discard_dynamic_exit_path(pid_tgid);
        return 0;
    }
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) {
        fill_dynamic_http_exit(e, pid_tgid);
    } else if (meta.socket_type != 0) {
        __builtin_memcpy(e->path, "socket writev", 14);
    } else {
        __builtin_memcpy(e->path, "file writev", 12);
    }
    submit_event(e);
'''
replace_once(old, new)

# sendmsg: same dynamic-only persistence strategy.
old = '''    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket sendmsg", 15);
        if (socket && have_iov && socket_http1_capture_eligible(socket)) {
            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                __builtin_memcpy(pd->path, "socket http", 12);
            }
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
'''
new = '''    if (socket_http1_capture_eligible(socket) && have_iov) {
        u32 zero = 0;
        struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
        if (pd) {
            u32 start_kind = HTTP1_START_NONE;
            u32 captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, first_len, &start_kind);
            if (captured > 0) {
                meta.type = TYPE_SOCKET_HTTP;
                meta.extra2 = captured;
                meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
            }
        }
    }
'''
replace_once(old, new)

old = '''    struct event *e = reserve_event();
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
'''
new = '''    struct event *e = reserve_event();
    if (!e) {
        if (meta.type == TYPE_SOCKET_HTTP) discard_dynamic_exit_path(pid_tgid);
        return 0;
    }
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) {
        fill_dynamic_http_exit(e, pid_tgid);
    } else {
        __builtin_memcpy(e->path, "socket sendmsg", 15);
    }
    submit_event(e);
'''
replace_once(old, new)

# recvmsg was the one incoming scatter/gather path that still probed known
# non-stream sockets. Match read/readv's stream-only rule.
replace_once(
    '''    if (ctx->ret > 0 && meta.extra2 == 1 && meta.addr_ptr != 0) {''',
    '''    if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && meta.addr_ptr != 0) {'''
)

P.write_text(text)

D = Path("docs/backend/generic-api-capture.md")
doc = D.read_text()
addition = '''\n### Dynamic exit-context fast path\n\nOutgoing `write`, `writev`, `sendmsg`, and `sendto` no longer round-trip static\nlabels through `exit_path_ctx`. The per-thread hash context is populated only\nafter the bounded HTTP/1 classifier recognizes a real start-line; ordinary TLS,\nbinary, file, UDP, and non-HTTP stream writes keep their static label entirely\nin the exit program. Failed ring-buffer reservations explicitly discard any\ndynamic HTTP context, preventing stale per-thread entries.\n'''
if addition.strip() not in doc:
    marker = "## Hot-path rejection"
    pos = doc.find(marker)
    if pos >= 0:
        next_section = doc.find("\n## ", pos + len(marker))
        if next_section >= 0:
            doc = doc[:next_section] + addition + doc[next_section:]
        else:
            doc += addition
    else:
        doc += addition
D.write_text(doc)
