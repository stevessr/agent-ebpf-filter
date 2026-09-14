from pathlib import Path

P = Path("backend/ebpf/agent_tracker_syscalls.h")
text = P.read_text()


def replace_once(old: str, new: str):
    global text
    if old not in text:
        raise SystemExit(f"missing anchor: {old[:180]!r}")
    text = text.replace(old, new, 1)


old_macros = r'''// Generic enter handler macro for simple syscalls
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
'''
new_macros = r'''// Static syscall labels are compile-time constants. Keep only the small
// exit_meta correlation record across enter/exit and write the label directly
// into the ringbuf event on exit. This avoids a 512-byte exit_path_ctx hash
// update + lookup + delete for every simple syscall and prevents unrelated
// per-CPU scratch bytes from leaking into extra4.
#define DEFINE_SIMPLE_ENTER_HANDLER(name, type_enum) \
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
    return 0; \
}

#define DEFINE_STATIC_EXIT_HANDLER(name, path_str) \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    u64 pid_tgid = bpf_get_current_pid_tgid(); \
    struct exit_meta meta = {}; \
    if (!consume_exit_meta(pid_tgid, &meta)) return 0; \
    struct event *e = reserve_event(); \
    if (!e) return 0; \
    fill_from_exit_meta(e, pid_tgid, &meta); \
    e->retval = ctx->ret; \
    __builtin_memcpy(e->path, path_str, sizeof(path_str) - 1); \
    submit_event(e); \
    return 0; \
}
'''
replace_once(old_macros, new_macros)

pairs = [
    ('DEFINE_SIMPLE_ENTER_HANDLER(ioctl, TYPE_IOCTL, "Special Resource Interaction (ioctl)")\nDEFINE_GENERIC_EXIT_HANDLER(ioctl)',
     'DEFINE_SIMPLE_ENTER_HANDLER(ioctl, TYPE_IOCTL)\nDEFINE_STATIC_EXIT_HANDLER(ioctl, "Special Resource Interaction (ioctl)")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(chmod, TYPE_CHMOD, "chmod")\nDEFINE_GENERIC_EXIT_HANDLER(chmod)',
     'DEFINE_SIMPLE_ENTER_HANDLER(chmod, TYPE_CHMOD)\nDEFINE_STATIC_EXIT_HANDLER(chmod, "chmod")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(chown, TYPE_CHOWN, "chown")\nDEFINE_GENERIC_EXIT_HANDLER(chown)',
     'DEFINE_SIMPLE_ENTER_HANDLER(chown, TYPE_CHOWN)\nDEFINE_STATIC_EXIT_HANDLER(chown, "chown")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(mknod, TYPE_MKNOD, "mknod")\nDEFINE_GENERIC_EXIT_HANDLER(mknod)',
     'DEFINE_SIMPLE_ENTER_HANDLER(mknod, TYPE_MKNOD)\nDEFINE_STATIC_EXIT_HANDLER(mknod, "mknod")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(clone, TYPE_CLONE, "process clone")\nDEFINE_GENERIC_EXIT_HANDLER(clone)',
     'DEFINE_SIMPLE_ENTER_HANDLER(clone, TYPE_CLONE)\nDEFINE_STATIC_EXIT_HANDLER(clone, "process clone")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(wait4, TYPE_WAIT4, "process wait4")\nDEFINE_GENERIC_EXIT_HANDLER(wait4)',
     'DEFINE_SIMPLE_ENTER_HANDLER(wait4, TYPE_WAIT4)\nDEFINE_STATIC_EXIT_HANDLER(wait4, "process wait4")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(exit_group, TYPE_EXIT, "process exit")\nDEFINE_GENERIC_EXIT_HANDLER(exit_group)',
     'DEFINE_SIMPLE_ENTER_HANDLER(exit_group, TYPE_EXIT)\nDEFINE_STATIC_EXIT_HANDLER(exit_group, "process exit")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(open, TYPE_OPEN, "file open")\nDEFINE_GENERIC_EXIT_HANDLER(open)',
     'DEFINE_SIMPLE_ENTER_HANDLER(open, TYPE_OPEN)\nDEFINE_STATIC_EXIT_HANDLER(open, "file open")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(rename, TYPE_RENAME, "file rename")\nDEFINE_GENERIC_EXIT_HANDLER(rename)',
     'DEFINE_SIMPLE_ENTER_HANDLER(rename, TYPE_RENAME)\nDEFINE_STATIC_EXIT_HANDLER(rename, "file rename")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(link, TYPE_LINK, "file link")\nDEFINE_GENERIC_EXIT_HANDLER(link)',
     'DEFINE_SIMPLE_ENTER_HANDLER(link, TYPE_LINK)\nDEFINE_STATIC_EXIT_HANDLER(link, "file link")'),
    ('DEFINE_SIMPLE_ENTER_HANDLER(symlink, TYPE_SYMLINK, "file symlink")\nDEFINE_GENERIC_EXIT_HANDLER(symlink)',
     'DEFINE_SIMPLE_ENTER_HANDLER(symlink, TYPE_SYMLINK)\nDEFINE_STATIC_EXIT_HANDLER(symlink, "file symlink")'),
]
for old, new in pairs:
    replace_once(old, new)

# socket(): the label is fixed; do not allocate/copy a 512-byte path context.
replace_once(r'''    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket create", 14);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")''', r'''    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")''')
replace_once(r'''    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }
    submit_event(e);
    return 0;
}

// Network syscalls with metadata''', r'''    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    __builtin_memcpy(e->path, "socket create", 14);
    submit_event(e);
    return 0;
}

// Network syscalls with metadata''')

# bind(): endpoint metadata stays in exit_meta; its label is static.
replace_once(r'''    fill_network_meta(&meta, (const void *)ctx->args[1], NET_DIR_LISTEN, 0);
    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket bind", 12);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}
DEFINE_GENERIC_EXIT_HANDLER(bind)''', r'''    fill_network_meta(&meta, (const void *)ctx->args[1], NET_DIR_LISTEN, 0);
    store_exit_meta(pid_tgid, &meta);
    return 0;
}
DEFINE_STATIC_EXIT_HANDLER(bind, "socket bind")''')

# recvfrom(): same static-label fast path.
replace_once(r'''    struct exit_meta meta = {.type = TYPE_RECVFROM, .tag_id = tag_id, .extra3 = (u32)ctx->args[2], .addr_ptr = ctx->args[4]};
    store_exit_meta(pid_tgid, &meta);
    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket recvfrom", 16);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}
DEFINE_GENERIC_EXIT_HANDLER(recvfrom)''', r'''    struct exit_meta meta = {.type = TYPE_RECVFROM, .tag_id = tag_id, .extra3 = (u32)ctx->args[2], .addr_ptr = ctx->args[4]};
    store_exit_meta(pid_tgid, &meta);
    return 0;
}
DEFINE_STATIC_EXIT_HANDLER(recvfrom, "socket recvfrom")''')

P.write_text(text)

D = Path("docs/backend/generic-api-capture.md")
doc = D.read_text()
addition = '''\n### Static syscall-label fast path\n\nFixed syscall labels no longer use `exit_path_buf`/`exit_path_ctx` as a transport\nbetween enter and exit programs. Simple operations such as `ioctl`, permission\nchanges, process wait/clone labels, basic file-operation labels, `socket()`,\n`bind()`, and `recvfrom()` keep only the compact `exit_meta` correlation record;\ntheir constant label is copied directly into the ring-buffer event on sys_exit.\nThis removes a 512-byte hash-map update plus lookup/delete from each event and\nalso avoids copying unrelated per-CPU scratch `extra4` bytes into static events.\nDynamic file paths and recognized HTTP start-lines continue to use the existing\nbounded context maps.\n'''
if addition not in doc:
    doc += addition
D.write_text(doc)
