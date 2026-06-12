//go:build ignore

// Optimized syscall handlers using macros to reduce code duplication
// This file replaces the repetitive handlers in agent_tracker_syscalls.h

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
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN); \
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

DEFINE_SIMPLE_ENTER_HANDLER(socket, TYPE_SOCKET, "socket create")
DEFINE_GENERIC_EXIT_HANDLER(socket)

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
    struct exit_meta meta = {.type = TYPE_SENDTO, .tag_id = tag_id};
    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);
    meta.extra3 = (u32)ctx->args[2];
    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket sendto", 14);
        u32 data_len = (u32)ctx->args[2];
        if (tag_id != 0 && data_len > 0 && data_len <= (MAX_PATH_LEN - 1)) {
            u32 capture_len = data_len & (MAX_PATH_LEN - 1);
            bpf_probe_read_user(pd->extra4, capture_len, (const void *)ctx->args[1]);
            pd->extra4[capture_len] = '\0';
        }
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
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

// Add remaining handlers using macros...
DEFINE_SIMPLE_ENTER_HANDLER(accept, TYPE_ACCEPT, "socket accept")
DEFINE_GENERIC_EXIT_HANDLER(accept)

DEFINE_SIMPLE_ENTER_HANDLER(accept4, TYPE_ACCEPT4, "socket accept4")
DEFINE_GENERIC_EXIT_HANDLER(accept4)

DEFINE_SIMPLE_ENTER_HANDLER(clone, TYPE_CLONE, "process clone")
DEFINE_GENERIC_EXIT_HANDLER(clone)

DEFINE_SIMPLE_ENTER_HANDLER(wait4, TYPE_WAIT4, "process wait4")
DEFINE_GENERIC_EXIT_HANDLER(wait4)

DEFINE_SIMPLE_ENTER_HANDLER(exit_group, TYPE_EXIT, "process exit")
DEFINE_GENERIC_EXIT_HANDLER(exit_group)

DEFINE_SIMPLE_ENTER_HANDLER(read, TYPE_READ, "file read")
DEFINE_GENERIC_EXIT_HANDLER(read)

DEFINE_SIMPLE_ENTER_HANDLER(write, TYPE_WRITE, "file write")
DEFINE_GENERIC_EXIT_HANDLER(write)

DEFINE_SIMPLE_ENTER_HANDLER(open, TYPE_OPEN, "file open")
DEFINE_GENERIC_EXIT_HANDLER(open)

DEFINE_SIMPLE_ENTER_HANDLER(rename, TYPE_RENAME, "file rename")
DEFINE_GENERIC_EXIT_HANDLER(rename)

DEFINE_SIMPLE_ENTER_HANDLER(link, TYPE_LINK, "file link")
DEFINE_GENERIC_EXIT_HANDLER(link)

DEFINE_SIMPLE_ENTER_HANDLER(symlink, TYPE_SYMLINK, "file symlink")
DEFINE_GENERIC_EXIT_HANDLER(symlink)
