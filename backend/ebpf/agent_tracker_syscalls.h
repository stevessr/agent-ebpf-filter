//go:build ignore

// Optimized syscall handlers using macros to reduce code duplication
// This file replaces the repetitive handlers in agent_tracker_syscalls.h

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
#define DEFINE_SIMPLE_ENTER_HANDLER(name, type_enum, path_str) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    u64 pid_tgid = bpf_get_current_pid_tgid(); \
    u32 pid = pid_tgid >> 32; \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 tag_id = get_tag_id(pid, comm, NULL); \
    if (tag_id == 0) return 0; \
    struct exit_meta meta = {.type = type_enum, .tag_id = tag_id}; \
    store_exit_meta(pid_tgid, &meta); \
    u32 zero = 0; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero); \
    if (pd) { \
        __builtin_memcpy(pd->path, path_str, sizeof(path_str)-1); \
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY); \
    } \
    return 0; \
}

// Generic exit handler macro - shared by all syscalls
#define DEFINE_GENERIC_EXIT_HANDLER(name) \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    u64 pid_tgid = bpf_get_current_pid_tgid(); \
    struct exit_meta meta = {}; \
    if (!consume_exit_meta(pid_tgid, &meta)) return 0; \
    struct event *e = reserve_event(); \
    if (!e) return 0; \
    fill_from_exit_meta(e, pid_tgid, &meta); \
    e->retval = ctx->ret; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid); \
    if (pd) { \
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN); \
        if (meta.type == TYPE_SOCKET_HTTP && meta.extra2 > 0) { \
            bpf_probe_read_kernel_str(e->extra4, MAX_PATH_LEN, pd->extra4); \
        } else { \
            __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN); \
        } \
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid); \
    } \
    submit_event(e); \
    return 0; \
}

// Define all simple handlers with one line each
DEFINE_SIMPLE_ENTER_HANDLER(ioctl, TYPE_IOCTL, "Special Resource Interaction (ioctl)")
DEFINE_GENERIC_EXIT_HANDLER(ioctl)

DEFINE_SIMPLE_ENTER_HANDLER(chmod, TYPE_CHMOD, "chmod")
DEFINE_GENERIC_EXIT_HANDLER(chmod)

DEFINE_SIMPLE_ENTER_HANDLER(chown, TYPE_CHOWN, "chown")
DEFINE_GENERIC_EXIT_HANDLER(chown)

DEFINE_SIMPLE_ENTER_HANDLER(mknod, TYPE_MKNOD, "mknod")
DEFINE_GENERIC_EXIT_HANDLER(mknod)

SEC("tracepoint/syscalls/sys_enter_socket")
int tracepoint__syscalls__sys_enter_socket(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {
        .type = TYPE_SOCKET,
        .tag_id = tag_id,
        .extra1 = (u32)ctx->args[0],
        .extra2 = (u32)ctx->args[1],
        .extra3 = (u32)ctx->args[2],
    };
    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket create", 14);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")
int tracepoint__syscalls__sys_exit_socket(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    if (ctx->ret >= 0) {
        remember_socket_fd((u32)(pid_tgid >> 32), (s32)ctx->ret, meta.extra1, meta.extra2, (u32)meta.extra3);
    }
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }
    submit_event(e);
    return 0;
}

// Network syscalls with metadata (keep custom handlers)
SEC("tracepoint/syscalls/sys_enter_bind")
int tracepoint__syscalls__sys_enter_bind(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(pid, comm, NULL);
    struct exit_meta meta = {.type = TYPE_BIND, .tag_id = tag_id};
    fill_network_meta(&meta, (const void *)ctx->args[1], NET_DIR_LISTEN, 0);
    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket bind", 12);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}
DEFINE_GENERIC_EXIT_HANDLER(bind)

// Sendto with payload capture
SEC("tracepoint/syscalls/sys_enter_sendto")
int tracepoint__syscalls__sys_enter_sendto(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {.type = TYPE_SENDTO, .tag_id = tag_id};
    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);
    meta.extra1 = (u32)ctx->args[0];
    meta.extra3 = (u32)ctx->args[2];
    struct socket_fd_meta *socket = lookup_socket_fd(pid, (s32)ctx->args[0]);
    if (meta.net_family == 0) {
        if (socket) fill_network_meta_from_socket(&meta, socket, (u32)ctx->args[2]);
    } else if (socket) {
        meta.capture_flags |= socket->provenance_flags;
        meta.socket_type = socket->sock_type;
    }
    u32 zero = 0;
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

SEC("tracepoint/syscalls/sys_enter_recvfrom")
int tracepoint__syscalls__sys_enter_recvfrom(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(pid, comm, NULL);
    struct exit_meta meta = {.type = TYPE_RECVFROM, .tag_id = tag_id, .extra3 = (u32)ctx->args[2], .addr_ptr = ctx->args[4]};
    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket recvfrom", 16);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}
DEFINE_GENERIC_EXIT_HANDLER(recvfrom)

SEC("tracepoint/syscalls/sys_enter_close")
int tracepoint__syscalls__sys_enter_close(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {.type = TYPE_SOCKET, .tag_id = tag_id, .extra2 = (u32)ctx->args[0]};
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_close")
int tracepoint__syscalls__sys_exit_close(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    if (ctx->ret == 0) forget_socket_fd((u32)(pid_tgid >> 32), (s32)meta.extra2);
    return 0;
}

// Descriptor lineage maintenance. These handlers intentionally do not emit
// extra semantic events; their job is to keep later socket/L7 events accurate.
#define DEFINE_DUP_HANDLER(name) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    u64 pid_tgid = bpf_get_current_pid_tgid(); \
    u32 tgid = (u32)(pid_tgid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 tag_id = get_tag_id(tgid, comm, NULL); \
    if (tag_id == 0) return 0; \
    struct exit_meta meta = {.type = TYPE_SOCKET, .tag_id = tag_id, .extra1 = (u32)ctx->args[0]}; \
    store_exit_meta(pid_tgid, &meta); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    u64 pid_tgid = bpf_get_current_pid_tgid(); \
    struct exit_meta meta = {}; \
    if (!consume_exit_meta(pid_tgid, &meta)) return 0; \
    if (ctx->ret >= 0) duplicate_socket_fd((u32)(pid_tgid >> 32), (s32)meta.extra1, (s32)ctx->ret, SOCKET_CAPTURE_FD_DUPLICATED); \
    return 0; \
}

DEFINE_DUP_HANDLER(dup)
DEFINE_DUP_HANDLER(dup2)
DEFINE_DUP_HANDLER(dup3)

#define DEFINE_ACCEPT_HANDLER(name, type_enum) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    u64 pid_tgid = bpf_get_current_pid_tgid(); \
    u32 tgid = (u32)(pid_tgid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 tag_id = get_tag_id(tgid, comm, NULL); \
    if (tag_id == 0) return 0; \
    struct exit_meta meta = {.type = type_enum, .tag_id = tag_id, .extra1 = (u32)ctx->args[0], .addr_ptr = ctx->args[1]}; \
    store_exit_meta(pid_tgid, &meta); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    u64 pid_tgid = bpf_get_current_pid_tgid(); \
    u32 tgid = (u32)(pid_tgid >> 32); \
    struct exit_meta meta = {}; \
    if (!consume_exit_meta(pid_tgid, &meta)) return 0; \
    if (ctx->ret >= 0) { \
        duplicate_socket_fd(tgid, (s32)meta.extra1, (s32)ctx->ret, SOCKET_CAPTURE_ACCEPTED); \
        struct socket_fd_meta *accepted = lookup_socket_fd(tgid, (s32)ctx->ret); \
        if (accepted) fill_network_meta_from_socket_direction(&meta, accepted, 0, NET_DIR_INCOMING); \
        if (meta.addr_ptr != 0) { \
            struct exit_meta peer = {}; \
            fill_network_meta(&peer, (const void *)meta.addr_ptr, NET_DIR_INCOMING, 0); \
            if (peer.net_family != 0) { \
                update_socket_fd_remote(tgid, (s32)ctx->ret, &peer); \
                meta.net_family = peer.net_family; \
                meta.net_port = peer.net_port; \
                __builtin_memcpy(meta.net_addr, peer.net_addr, sizeof(meta.net_addr)); \
            } \
        } \
        meta.extra2 = (u32)ctx->ret; \
        meta.capture_flags |= SOCKET_CAPTURE_ACCEPTED; \
    } \
    struct event *e = reserve_event(); \
    if (!e) return 0; \
    fill_from_exit_meta(e, pid_tgid, &meta); \
    e->retval = ctx->ret; \
    submit_event(e); \
    return 0; \
}

DEFINE_ACCEPT_HANDLER(accept, TYPE_ACCEPT)
DEFINE_ACCEPT_HANDLER(accept4, TYPE_ACCEPT4)

DEFINE_SIMPLE_ENTER_HANDLER(clone, TYPE_CLONE, "process clone")
DEFINE_GENERIC_EXIT_HANDLER(clone)

DEFINE_SIMPLE_ENTER_HANDLER(wait4, TYPE_WAIT4, "process wait4")
DEFINE_GENERIC_EXIT_HANDLER(wait4)

DEFINE_SIMPLE_ENTER_HANDLER(exit_group, TYPE_EXIT, "process exit")
DEFINE_GENERIC_EXIT_HANDLER(exit_group)

SEC("tracepoint/syscalls/sys_enter_read")
int tracepoint__syscalls__sys_enter_read(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = (u32)(pid_tgid >> 32);
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    s32 fd = (s32)ctx->args[0];
    struct exit_meta meta = {.type = TYPE_READ, .tag_id = tag_id, .extra1 = (u32)fd, .extra3 = (u32)ctx->args[2], .addr_ptr = ctx->args[1]};
    struct socket_fd_meta *socket = lookup_socket_fd(tgid, fd);
    if (socket) {
        meta.extra2 = 1; // socket marker until a start-line length replaces it
        fill_network_meta_from_socket_direction(&meta, socket, (u32)ctx->args[2], NET_DIR_INCOMING);
    }
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_read")
int tracepoint__syscalls__sys_exit_read(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    u32 captured = 0;
    if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && pd && meta.addr_ptr != 0) {
        u32 actual = (u32)ctx->ret;
        u32 start_kind = HTTP1_START_NONE;
        captured = capture_http1_start_line(pd->extra4, (const void *)meta.addr_ptr, actual, &start_kind);
        if (captured > 0) {
            meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
            if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
            else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
        }
        if (captured > 0) {
            meta.type = TYPE_SOCKET_HTTP;
            meta.extra2 = captured;
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
        __builtin_memcpy(e->path, "socket read", 12);
    } else {
        __builtin_memcpy(e->path, "file read", 10);
    }
    submit_event(e);
    return 0;
}

SEC("tracepoint/syscalls/sys_enter_write")
int tracepoint__syscalls__sys_enter_write(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;

    s32 fd = (s32)ctx->args[0];
    u32 requested = (u32)ctx->args[2];
    struct socket_fd_meta *socket = lookup_socket_fd(tgid, fd);
    struct exit_meta meta = {.type = TYPE_WRITE, .tag_id = tag_id, .extra1 = (u32)fd, .extra3 = requested};
    if (socket) fill_network_meta_from_socket(&meta, socket, requested);

    u32 zero = 0;
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
    if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && pd && meta.addr_ptr != 0 && meta.capture_reserved > 0) {
        struct capture_iovec64 iov = {};
        if (capture_first_iovec((const void *)meta.addr_ptr, meta.capture_reserved, &iov)) {
            u32 available = capture_iov_len(iov.len);
            if ((u64)ctx->ret < available) available = (u32)ctx->ret;
            u32 start_kind = HTTP1_START_NONE;
            captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, available, &start_kind);
            if (captured > 0) {
                meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
                if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
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
        meta.socket_type = socket->sock_type;
    }
    if (socket) meta.capture_flags |= SOCKET_CAPTURE_SCATTER_GATHER;

    u32 zero = 0;
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
                u32 start_kind = HTTP1_START_NONE;
                captured = capture_http1_start_line(pd->extra4, (const void *)iov.base, available, &start_kind);
                if (captured > 0) {
                    meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
                    if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                    else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
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
DEFINE_GENERIC_EXIT_HANDLER(open)

DEFINE_SIMPLE_ENTER_HANDLER(rename, TYPE_RENAME, "file rename")
DEFINE_GENERIC_EXIT_HANDLER(rename)

DEFINE_SIMPLE_ENTER_HANDLER(link, TYPE_LINK, "file link")
DEFINE_GENERIC_EXIT_HANDLER(link)

DEFINE_SIMPLE_ENTER_HANDLER(symlink, TYPE_SYMLINK, "file symlink")
DEFINE_GENERIC_EXIT_HANDLER(symlink)
