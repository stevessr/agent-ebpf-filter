from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing phase2 kernel anchor in {path}: {old[:160]!r}")
    write(path, text.replace(old, new, 1))


# ---------------------------------------------------------------------------
# Kernel ABI + capture flags + socket lineage maps.
# ---------------------------------------------------------------------------
replace_once(
    "backend/ebpf/agent_tracker_common.h",
    "#define SOCKET_CAPTURE_HTTP1_REQUEST_LINE (1U << 0)\n",
    """#define SOCKET_CAPTURE_HTTP1_REQUEST_LINE (1U << 0)\n#define SOCKET_CAPTURE_HTTP1_RESPONSE_LINE (1U << 1)\n#define SOCKET_CAPTURE_INCOMING            (1U << 2)\n#define SOCKET_CAPTURE_OUTGOING            (1U << 3)\n#define SOCKET_CAPTURE_FD_DUPLICATED        (1U << 4)\n#define SOCKET_CAPTURE_FD_INHERITED         (1U << 5)\n#define SOCKET_CAPTURE_ACCEPTED             (1U << 6)\n#define SOCKET_CAPTURE_SCATTER_GATHER       (1U << 7)\n""")

replace_once(
    "backend/ebpf/agent_tracker_common.h",
    "    u64 kernel_reserve_failures_total; // CPU-local cumulative reserve failures snapshot\n};\n",
    """    u64 kernel_reserve_failures_total; // CPU-local cumulative reserve failures snapshot\n    u32 kernel_capture_flags;        // bounded-L7 capture source/direction/provenance bits\n    u32 kernel_capture_reserved;     // append-only ABI padding / future capture metadata\n};\n""")
replace_once(
    "backend/ebpf/agent_tracker_common.h",
    '_Static_assert(sizeof(struct event) == 680, "event ABI size changed; update Go BpfEvent intentionally");',
    '_Static_assert(sizeof(struct event) == 688, "event ABI size changed; update Go BpfEvent intentionally");')

replace_once(
    "backend/ebpf/agent_tracker_common.h",
    "    u64 addr_ptr;\n    u64 start_ns;\n};\n",
    """    u64 addr_ptr;\n    u64 start_ns;\n    u32 capture_flags;\n    u32 capture_reserved;\n};\n""")

replace_once(
    "backend/ebpf/agent_tracker_common.h",
    """struct socket_fd_meta {\n    u32 family;\n    u32 sock_type;\n    u32 protocol;\n    u32 remote_port;\n    char remote_addr[16];\n};\n""",
    """struct socket_fd_meta {\n    u32 family;\n    u32 sock_type;\n    u32 protocol;\n    u32 remote_port;\n    char remote_addr[16];\n    u32 provenance_flags;\n    u32 reserved;\n};\n""")

socket_map = """struct {\n    __uint(type, BPF_MAP_TYPE_LRU_HASH);\n    __uint(max_entries, 16384);\n    __type(key, struct socket_fd_key);\n    __type(value, struct socket_fd_meta);\n} socket_fds SEC(\".maps\");\n"""
replace_once(
    "backend/ebpf/agent_tracker_common.h",
    socket_map,
    socket_map + """\n// Lazy process lineage for inherited descriptor tables. We intentionally keep\n// this LRU map bounded instead of walking/copying all descriptors at fork.\nstruct {\n    __uint(type, BPF_MAP_TYPE_LRU_HASH);\n    __uint(max_entries, 8192);\n    __type(key, u32);   // child TGID\n    __type(value, u32); // parent TGID\n} socket_fd_parents SEC(\".maps\");\n""")

replace_once(
    "backend/ebpf/agent_tracker_common.h",
    "    e->kernel_reserve_failures_total = 0;\n",
    "    e->kernel_reserve_failures_total = 0;\n    e->kernel_capture_flags = 0;\n    e->kernel_capture_reserved = 0;\n")

old_helpers = """static __always_inline struct socket_fd_meta *lookup_socket_fd(u32 tgid, s32 fd) {\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    return bpf_map_lookup_elem(&socket_fds, &key);\n}\n\nstatic __always_inline void remember_socket_fd(u32 tgid, s32 fd, u32 family, u32 sock_type, u32 protocol) {\n    if (fd < 0) return;\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    struct socket_fd_meta value = {\n        .family = family,\n        .sock_type = sock_type,\n        .protocol = protocol,\n    };\n    bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY);\n}\n\nstatic __always_inline void update_socket_fd_remote(u32 tgid, s32 fd, struct exit_meta *meta) {\n    if (fd < 0 || !meta) return;\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    struct socket_fd_meta value = {};\n    struct socket_fd_meta *existing = bpf_map_lookup_elem(&socket_fds, &key);\n    if (existing) __builtin_memcpy(&value, existing, sizeof(value));\n    value.family = meta->net_family;\n    value.remote_port = meta->net_port;\n    __builtin_memcpy(value.remote_addr, meta->net_addr, sizeof(value.remote_addr));\n    bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY);\n}\n\nstatic __always_inline void forget_socket_fd(u32 tgid, s32 fd) {\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    bpf_map_delete_elem(&socket_fds, &key);\n}\n\nstatic __always_inline void fill_network_meta_from_socket(struct exit_meta *meta, struct socket_fd_meta *socket, u32 bytes) {\n    if (!meta || !socket) return;\n    meta->net_family = socket->family;\n    meta->net_direction = NET_DIR_OUTGOING;\n    meta->net_bytes = bytes;\n    meta->net_port = socket->remote_port;\n    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));\n}\n"""
new_helpers = """static __always_inline struct socket_fd_meta *lookup_socket_fd_direct(u32 tgid, s32 fd) {\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    return bpf_map_lookup_elem(&socket_fds, &key);\n}\n\n// Resolve one or two parent generations lazily. The copied child entry becomes\n// authoritative after first use, keeping the normal hot path to one hash lookup.\nstatic __always_inline struct socket_fd_meta *lookup_socket_fd(u32 tgid, s32 fd) {\n    struct socket_fd_meta *direct = lookup_socket_fd_direct(tgid, fd);\n    if (direct) return direct;\n\n    u32 *parent_ptr = bpf_map_lookup_elem(&socket_fd_parents, &tgid);\n    if (!parent_ptr) return 0;\n    u32 parent = *parent_ptr;\n    struct socket_fd_meta *inherited = lookup_socket_fd_direct(parent, fd);\n\n    if (!inherited) {\n        u32 *grand_ptr = bpf_map_lookup_elem(&socket_fd_parents, &parent);\n        if (grand_ptr) {\n            u32 grand = *grand_ptr;\n            inherited = lookup_socket_fd_direct(grand, fd);\n        }\n    }\n    if (!inherited) return 0;\n\n    struct socket_fd_meta value = {};\n    __builtin_memcpy(&value, inherited, sizeof(value));\n    value.provenance_flags |= SOCKET_CAPTURE_FD_INHERITED;\n    struct socket_fd_key child_key = {.tgid = tgid, .fd = fd};\n    bpf_map_update_elem(&socket_fds, &child_key, &value, BPF_ANY);\n    return bpf_map_lookup_elem(&socket_fds, &child_key);\n}\n\nstatic __always_inline void remember_socket_fd(u32 tgid, s32 fd, u32 family, u32 sock_type, u32 protocol) {\n    if (fd < 0) return;\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    struct socket_fd_meta value = {\n        .family = family,\n        .sock_type = sock_type,\n        .protocol = protocol,\n    };\n    bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY);\n}\n\nstatic __always_inline void duplicate_socket_fd(u32 tgid, s32 oldfd, s32 newfd, u32 flags) {\n    if (newfd < 0 || oldfd == newfd) return;\n    struct socket_fd_meta *source = lookup_socket_fd(tgid, oldfd);\n    struct socket_fd_key new_key = {.tgid = tgid, .fd = newfd};\n    if (!source) {\n        // dup2/dup3 can replace a socket with a non-socket descriptor.\n        bpf_map_delete_elem(&socket_fds, &new_key);\n        return;\n    }\n    struct socket_fd_meta value = {};\n    __builtin_memcpy(&value, source, sizeof(value));\n    value.provenance_flags |= flags | SOCKET_CAPTURE_FD_DUPLICATED;\n    bpf_map_update_elem(&socket_fds, &new_key, &value, BPF_ANY);\n}\n\nstatic __always_inline void update_socket_fd_remote(u32 tgid, s32 fd, struct exit_meta *meta) {\n    if (fd < 0 || !meta) return;\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    struct socket_fd_meta value = {};\n    struct socket_fd_meta *existing = lookup_socket_fd(tgid, fd);\n    if (existing) __builtin_memcpy(&value, existing, sizeof(value));\n    value.family = meta->net_family;\n    value.remote_port = meta->net_port;\n    __builtin_memcpy(value.remote_addr, meta->net_addr, sizeof(value.remote_addr));\n    bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY);\n}\n\nstatic __always_inline void forget_socket_fd(u32 tgid, s32 fd) {\n    struct socket_fd_key key = {.tgid = tgid, .fd = fd};\n    bpf_map_delete_elem(&socket_fds, &key);\n}\n\nstatic __always_inline void fill_network_meta_from_socket_direction(struct exit_meta *meta, struct socket_fd_meta *socket, u32 bytes, u32 direction) {\n    if (!meta || !socket) return;\n    meta->net_family = socket->family;\n    meta->net_direction = direction;\n    meta->net_bytes = bytes;\n    meta->net_port = socket->remote_port;\n    meta->capture_flags |= socket->provenance_flags;\n    __builtin_memcpy(meta->net_addr, socket->remote_addr, sizeof(meta->net_addr));\n}\n\nstatic __always_inline void fill_network_meta_from_socket(struct exit_meta *meta, struct socket_fd_meta *socket, u32 bytes) {\n    fill_network_meta_from_socket_direction(meta, socket, bytes, NET_DIR_OUTGOING);\n}\n"""
replace_once("backend/ebpf/agent_tracker_common.h", old_helpers, new_helpers)

request_helper_end = """    dst[capture_len] = '\\0';\n    return capture_len;\n}\n\n// Convenience inline for sys_exit handlers"""
response_helpers = """    dst[capture_len] = '\\0';\n    return capture_len;\n}\n\nstatic __always_inline int looks_like_http1_response(const char *head, u32 len) {\n    if (!head || len < 8) return 0;\n    return head[0] == 'H' && head[1] == 'T' && head[2] == 'T' && head[3] == 'P' &&\n           head[4] == '/' && head[5] == '1' && head[6] == '.';\n}\n\nstatic __always_inline u32 capture_http1_response_line(char *dst, const void *user_buf, u32 len) {\n    if (!dst || !user_buf || len < 8) return 0;\n    char head[8] = {};\n    u32 head_len = len < sizeof(head) ? len : sizeof(head);\n    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return 0;\n    if (!looks_like_http1_response(head, head_len)) return 0;\n    u32 capture_len = len;\n    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;\n    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) return 0;\n#pragma clang loop unroll(disable)\n    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {\n        if ((u32)i >= capture_len) break;\n        char c = dst[i];\n        if (c == '\\r' || c == '\\n') {\n            dst[i] = '\\0';\n            return (u32)i;\n        }\n    }\n    dst[capture_len] = '\\0';\n    return capture_len;\n}\n\n// Convenience inline for sys_exit handlers"""
replace_once("backend/ebpf/agent_tracker_common.h", request_helper_end, response_helpers)

replace_once(
    "backend/ebpf/agent_tracker_common.h",
    "    e->net_port = meta->net_port;\n    __builtin_memcpy(e->net_addr, meta->net_addr, 16);\n",
    "    e->net_port = meta->net_port;\n    e->kernel_capture_flags = meta->capture_flags;\n    __builtin_memcpy(e->net_addr, meta->net_addr, 16);\n")

replace_once(
    "backend/ebpf/agent_tracker_common.h",
    "    bpf_map_update_elem(&agent_pids, &child_pid, tag, BPF_ANY);\n\n    struct event *e = reserve_event();\n",
    """    bpf_map_update_elem(&agent_pids, &child_pid, tag, BPF_ANY);\n    u32 parent_tgid = (u32)(bpf_get_current_pid_tgid() >> 32);\n    if (parent_tgid != 0 && child_pid != parent_tgid) {\n        bpf_map_update_elem(&socket_fd_parents, &child_pid, &parent_tgid, BPF_ANY);\n    }\n\n    struct event *e = reserve_event();\n""")

# ---------------------------------------------------------------------------
# Socket syscalls: accepted fds, dup lineage, bidirectional HTTP start lines.
# ---------------------------------------------------------------------------
replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    """// Add remaining handlers using macros...\nDEFINE_SIMPLE_ENTER_HANDLER(accept, TYPE_ACCEPT, \"socket accept\")\nDEFINE_GENERIC_EXIT_HANDLER(accept)\n\nDEFINE_SIMPLE_ENTER_HANDLER(accept4, TYPE_ACCEPT4, \"socket accept4\")\nDEFINE_GENERIC_EXIT_HANDLER(accept4)\n\nDEFINE_SIMPLE_ENTER_HANDLER(clone, TYPE_CLONE, \"process clone\")\n""",
    r'''// Descriptor lineage maintenance. These handlers intentionally do not emit
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
''')

replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    """DEFINE_SIMPLE_ENTER_HANDLER(read, TYPE_READ, \"file read\")\nDEFINE_GENERIC_EXIT_HANDLER(read)\n\nSEC(\"tracepoint/syscalls/sys_enter_write\")\n""",
    r'''SEC("tracepoint/syscalls/sys_enter_read")
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
    if (ctx->ret > 0 && meta.extra2 == 1 && pd && meta.addr_ptr != 0) {
        u32 actual = (u32)ctx->ret;
        captured = capture_http1_request_line(pd->extra4, (const void *)meta.addr_ptr, actual);
        if (captured > 0) {
            meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_INCOMING;
        } else {
            captured = capture_http1_response_line(pd->extra4, (const void *)meta.addr_ptr, actual);
            if (captured > 0) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_INCOMING;
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
''')

# write(): request OR response start-line and capture provenance flags.
replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    """            u32 captured = capture_http1_request_line(pd->extra4, (const void *)ctx->args[1], requested);\n            if (captured > 0) {\n                meta.type = TYPE_SOCKET_HTTP;\n                meta.extra2 = captured;\n                __builtin_memcpy(pd->path, \"socket http\", 12);\n            } else {\n                __builtin_memcpy(pd->path, \"socket write\", 13);\n            }\n""",
    """            u32 captured = capture_http1_request_line(pd->extra4, (const void *)ctx->args[1], requested);\n            if (captured > 0) {\n                meta.type = TYPE_SOCKET_HTTP;\n                meta.extra2 = captured;\n                meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_OUTGOING;\n                __builtin_memcpy(pd->path, \"socket http\", 12);\n            } else {\n                captured = capture_http1_response_line(pd->extra4, (const void *)ctx->args[1], requested);\n                if (captured > 0) {\n                    meta.type = TYPE_SOCKET_HTTP;\n                    meta.extra2 = captured;\n                    meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_OUTGOING;\n                    __builtin_memcpy(pd->path, \"socket http\", 12);\n                } else {\n                    __builtin_memcpy(pd->path, \"socket write\", 13);\n                }\n            }\n""")

# sendto(): preserve socket provenance even with explicit sockaddr and classify response lines too.
replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    """    if (meta.net_family == 0) {\n        struct socket_fd_meta *socket = lookup_socket_fd(pid, (s32)ctx->args[0]);\n        if (socket) fill_network_meta_from_socket(&meta, socket, (u32)ctx->args[2]);\n    }\n""",
    """    struct socket_fd_meta *socket = lookup_socket_fd(pid, (s32)ctx->args[0]);\n    if (meta.net_family == 0) {\n        if (socket) fill_network_meta_from_socket(&meta, socket, (u32)ctx->args[2]);\n    } else if (socket) {\n        meta.capture_flags |= socket->provenance_flags;\n    }\n""")
replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    """        u32 captured = capture_http1_request_line(pd->extra4, (const void *)ctx->args[1], data_len);\n        if (captured > 0) {\n            meta.type = TYPE_SOCKET_HTTP;\n            meta.extra2 = captured;\n            __builtin_memcpy(pd->path, \"socket http\", 12);\n        }\n""",
    """        u32 captured = capture_http1_request_line(pd->extra4, (const void *)ctx->args[1], data_len);\n        if (captured > 0) {\n            meta.type = TYPE_SOCKET_HTTP;\n            meta.extra2 = captured;\n            meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE | SOCKET_CAPTURE_OUTGOING;\n            __builtin_memcpy(pd->path, \"socket http\", 12);\n        } else {\n            captured = capture_http1_response_line(pd->extra4, (const void *)ctx->args[1], data_len);\n            if (captured > 0) {\n                meta.type = TYPE_SOCKET_HTTP;\n                meta.extra2 = captured;\n                meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE | SOCKET_CAPTURE_OUTGOING;\n                __builtin_memcpy(pd->path, \"socket http\", 12);\n            }\n        }\n""")

# ---------------------------------------------------------------------------
# Go ABI + pinned map lifecycle.
# ---------------------------------------------------------------------------
replace_once(
    "backend/core/types.go",
    "\tKernelReserveFailuresTotal             uint64\n}\n",
    "\tKernelReserveFailuresTotal             uint64\n\tKernelCaptureFlags                      uint32\n\t_                                       [4]byte // append-only capture ABI alignment\n}\n")
replace_once(
    "backend/core/types.go",
    "\tSocketFds       *ebpf.Map\n",
    "\tSocketFds       *ebpf.Map\n\tSocketFdParents *ebpf.Map\n")
replace_once(
    "backend/core/types_abi_test.go",
    "if got := unsafe.Sizeof(event); got != 680 {\n\t\tt.Fatalf(\"BpfEvent size = %d, want 680\", got)\n",
    "if got := unsafe.Sizeof(event); got != 688 {\n\t\tt.Fatalf(\"BpfEvent size = %d, want 688\", got)\n")

replace_once(
    "backend/app/runtime_ebpf.go",
    'var mapNames = []string{"agent_pids", "events", "collector_stats", "tracked_comms", "tracked_paths", "tracked_prefixes", "exit_ctx", "exit_path_buf", "exit_path_ctx", "socket_fds"}',
    'var mapNames = []string{"agent_pids", "events", "collector_stats", "tracked_comms", "tracked_paths", "tracked_prefixes", "exit_ctx", "exit_path_buf", "exit_path_ctx", "socket_fds", "socket_fd_parents"}')
replace_once(
    "backend/app/runtime_ebpf.go",
    "\t\t\tif err := clearSocketFDProvenance(objs.SocketFds); err != nil {\n",
    "\t\t\tif err := clearSocketFDProvenance(objs.SocketFds, objs.SocketFdParents); err != nil {\n")
replace_once(
    "backend/app/runtime_ebpf.go",
    "func clearSocketFDProvenance(socketFds *ebpf.Map) error {\n",
    "func clearSocketFDProvenance(socketFds *ebpf.Map, parentMap ...*ebpf.Map) error {\n")
replace_once(
    "backend/app/runtime_ebpf.go",
    "\treturn nil\n}\n\nfunc pinMaps(objs *bpf.AgentTrackerObjects) error {\n",
    """\tif len(parentMap) > 0 && parentMap[0] != nil {\n\t\tparents := parentMap[0]\n\t\titer := parents.Iterate()\n\t\tkeys := make([]uint32, 0, 64)\n\t\tvar child, parent uint32\n\t\tfor iter.Next(&child, &parent) {\n\t\t\tkeys = append(keys, child)\n\t\t}\n\t\tif err := iter.Err(); err != nil {\n\t\t\treturn fmt.Errorf(\"iterate socket_fd_parents before generation rotation: %w\", err)\n\t\t}\n\t\tfor _, child := range keys {\n\t\t\tif err := parents.Delete(&child); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {\n\t\t\t\treturn fmt.Errorf(\"clear socket_fd_parents provenance: %w\", err)\n\t\t\t}\n\t\t}\n\t}\n\treturn nil\n}\n\nfunc pinMaps(objs *bpf.AgentTrackerObjects) error {\n""")
replace_once(
    "backend/app/runtime_ebpf.go",
    '"socket_fds": objs.SocketFds,\n',
    '"socket_fds": objs.SocketFds, "socket_fd_parents": objs.SocketFdParents,\n')

# Proto surface: HTTP status + normalized gRPC route metadata.
replace_once(
    "proto/tracker_events.proto",
    "  int32 kernel_socket_fd = 92;\n}",
    "  int32 kernel_socket_fd = 92;\n  uint32 http_status = 93;\n  string grpc_service = 94;\n  string grpc_method = 95;\n}")
