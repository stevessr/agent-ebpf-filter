static __always_inline int sys_enter_common_path(u32 pid, char *comm, char *path, u32 nr, u32 extra2, u32 extra3) {
    u32 tag_id = get_tag_id(pid, comm, path);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {};
    meta.type = TYPE_GENERIC_SYSCALL;
    meta.tag_id = tag_id;
    meta.extra1 = nr;
    meta.extra2 = extra2;
    meta.extra3 = extra3;
    meta.start_ns = bpf_ktime_get_ns();
    u64 ptid = bpf_get_current_pid_tgid();
    store_exit_meta(ptid, &meta);
    return 1;
}

static __always_inline int sys_enter_common_nopath(u32 pid, char *comm, u32 nr, u32 extra2, u32 extra3) {
    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {};
    meta.type = TYPE_GENERIC_SYSCALL;
    meta.tag_id = tag_id;
    meta.extra1 = nr;
    meta.extra2 = extra2;
    meta.extra3 = extra3;
    meta.start_ns = bpf_ktime_get_ns();
    u64 ptid = bpf_get_current_pid_tgid();
    store_exit_meta(ptid, &meta);
    return 1;
}

static __always_inline void sys_exit_common(struct trace_event_raw_sys_exit *ctx, int has_path) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return;
    struct event *e = reserve_event();
    if (!e) return;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (meta.start_ns != 0) {
        u64 now = bpf_ktime_get_ns();
        if (now >= meta.start_ns) {
            e->duration_ns = now - meta.start_ns;
        }
    }
    if (has_path) {
        struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
        if (pd) {
            __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
            __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
            bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
        }
    }
    submit_event(e);
}

// ── Helper: store pid_tgid as lvalue ──
#define STORE_PID_TGID() u64 ptid = bpf_get_current_pid_tgid()

// ── Macro: path at args[0], single path ──
#define SYS_PATH0(name, nr) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    STORE_PID_TGID(); u32 pid = (u32)(ptid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 zero = 0; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero); \
    if (!pd) return 0; \
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, (const char *)ctx->args[0]); \
    if (!sys_enter_common_path(pid, comm, pd->path, nr, 0, 0)) return 0; \
    bpf_map_update_elem(&exit_path_ctx, &ptid, pd, BPF_ANY); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 1); \
    return 0; \
}

// ── Macro: path at args[0], dual-path (args[0]=primary, args[1]=secondary) ──
#define SYS_PATH01(name, nr) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    STORE_PID_TGID(); u32 pid = (u32)(ptid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 zero = 0; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero); \
    if (!pd) return 0; \
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, (const char *)ctx->args[0]); \
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, (const char *)ctx->args[1]); \
    if (!sys_enter_common_path(pid, comm, pd->path, nr, 0, 0)) return 0; \
    bpf_map_update_elem(&exit_path_ctx, &ptid, pd, BPF_ANY); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 1); \
    return 0; \
}

// ── Macro: path at args[1] (fd-relative), single path ──
#define SYS_PATH1(name, nr) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    STORE_PID_TGID(); u32 pid = (u32)(ptid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 zero = 0; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero); \
    if (!pd) return 0; \
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, (const char *)ctx->args[1]); \
    if (!sys_enter_common_path(pid, comm, pd->path, nr, 0, 0)) return 0; \
    bpf_map_update_elem(&exit_path_ctx, &ptid, pd, BPF_ANY); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 1); \
    return 0; \
}

// ── Macro: dual path at args[1]+args[3] ──
#define SYS_PATH13(name, nr) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    STORE_PID_TGID(); u32 pid = (u32)(ptid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 zero = 0; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero); \
    if (!pd) return 0; \
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, (const char *)ctx->args[1]); \
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, (const char *)ctx->args[3]); \
    if (!sys_enter_common_path(pid, comm, pd->path, nr, 0, 0)) return 0; \
    bpf_map_update_elem(&exit_path_ctx, &ptid, pd, BPF_ANY); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 1); \
    return 0; \
}

// ── Macro: symlinkat — target=args[0], linkpath=args[2] ──
#define SYS_PATH02(name, nr) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    STORE_PID_TGID(); u32 pid = (u32)(ptid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 zero = 0; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero); \
    if (!pd) return 0; \
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, (const char *)ctx->args[0]); \
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, (const char *)ctx->args[2]); \
    if (!sys_enter_common_path(pid, comm, pd->path, nr, 0, 0)) return 0; \
    bpf_map_update_elem(&exit_path_ctx, &ptid, pd, BPF_ANY); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 1); \
    return 0; \
}

// ── Macro: fanotify_mark — path at args[4] ──
#define SYS_PATH4(name, nr) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    STORE_PID_TGID(); u32 pid = (u32)(ptid >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    u32 zero = 0; \
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero); \
    if (!pd) return 0; \
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, (const char *)ctx->args[4]); \
    if (!sys_enter_common_path(pid, comm, pd->path, nr, 0, 0)) return 0; \
    bpf_map_update_elem(&exit_path_ctx, &ptid, pd, BPF_ANY); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 1); \
    return 0; \
}

// ── Macro: comm-only with numeric extra2 at args[N] ──
#define SYS_NUM(name, nr, arg_idx) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    u32 pid = (u32)(bpf_get_current_pid_tgid() >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    sys_enter_common_nopath(pid, comm, nr, (u32)ctx->args[arg_idx], 0); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 0); \
    return 0; \
}

// ── Macro: comm-only with extra2=args[a], extra3=args[b] ──
#define SYS_NUM2(name, nr, a_idx, b_idx) \
SEC("tracepoint/syscalls/sys_enter_" #name) \
int tracepoint__syscalls__sys_enter_##name(struct trace_event_raw_sys_enter *ctx) { \
    u32 pid = (u32)(bpf_get_current_pid_tgid() >> 32); \
    char comm[TASK_COMM_LEN]; \
    bpf_get_current_comm(&comm, sizeof(comm)); \
    sys_enter_common_nopath(pid, comm, nr, (u32)ctx->args[a_idx], (u32)ctx->args[b_idx]); \
    return 0; \
} \
SEC("tracepoint/syscalls/sys_exit_" #name) \
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \
    sys_exit_common(ctx, 0); \
    return 0; \
}

// ═══════════════════════════════════════════════════════════
// Instantiate all syscall handlers via macros
// ═══════════════════════════════════════════════════════════

// ── Path at args[0] ──
SYS_PATH0(stat,        4)
SYS_PATH0(lstat,       6)
SYS_PATH0(access,      21)
SYS_PATH0(truncate,    76)
SYS_PATH0(chdir,       80)
SYS_PATH0(mkdir,       83)
SYS_PATH0(rmdir,       84)
SYS_PATH0(creat,       85)
SYS_PATH0(unlink,      87)
SYS_PATH0(readlink,    89)
SYS_PATH0(chroot,      161)
SYS_PATH0(umount2,     166)
SYS_PATH0(swapon,      167)
SYS_PATH0(swapoff,     168)
SYS_PATH0(sethostname, 170)
SYS_PATH0(setdomainname, 171)
SYS_PATH0(setxattr,    188)
SYS_PATH0(lsetxattr,   189)
SYS_PATH0(getxattr,    191)
SYS_PATH0(lgetxattr,   192)
SYS_PATH0(listxattr,   194)
SYS_PATH0(llistxattr,  195)
SYS_PATH0(removexattr, 197)
SYS_PATH0(lremovexattr, 198)
SYS_PATH0(fsopen,      430)
SYS_PATH0(memfd_create, 319)
SYS_PATH0(execveat,    322)

// ── Dual path at args[0]+args[1] ──
SYS_PATH01(pivot_root, 155)
SYS_PATH01(mount,      165)

// ── Path at args[1] (fd-relative) ──
SYS_PATH1(mknodat,     259)
SYS_PATH1(fchownat,    260)
SYS_PATH1(futimesat,   261)
SYS_PATH1(newfstatat,  262)
SYS_PATH1(readlinkat,  267)
SYS_PATH1(fchmodat,    268)
SYS_PATH1(faccessat,   269)
SYS_PATH1(utimensat,   280)
SYS_PATH1(name_to_handle_at, 303)
SYS_PATH1(openat2,     437)
SYS_PATH1(faccessat2,  439)
SYS_PATH1(inotify_add_watch, 254)
SYS_PATH1(open_tree,   428)

// ── Dual path at args[1]+args[3] ──
SYS_PATH13(renameat,   264)
SYS_PATH13(linkat,     265)
SYS_PATH13(renameat2,  316)
SYS_PATH13(move_mount, 429)

// ── Special: symlinkat target=args[0], linkpath=args[2] ──
SYS_PATH02(symlinkat,  266)

// ── Special: fanotify_mark path at args[4] ──
SYS_PATH4(fanotify_mark, 301)

// ── Security-relevant comm-only ──
SYS_NUM(kill,          62,  1)  // sig
SYS_NUM(tkill,         200, 0)  // sig
SYS_NUM2(tgkill,       234, 1, 2) // tgid, sig
SYS_NUM(ptrace,        101, 0)  // request
SYS_NUM(prctl,         157, 0)  // option
SYS_NUM(syslog,        103, 0)  // type
SYS_NUM(capget,        125, 0)  // header
SYS_NUM(capset,        126, 0)  // header
SYS_NUM(iopl,          172, 0)  // level
SYS_NUM(ioperm,        173, 0)  // from
SYS_NUM(init_module,    175, 1) // len
SYS_NUM(unshare,       272, 0)  // flags
SYS_NUM(setns,         308, 1)  // nstype
SYS_NUM(process_vm_readv,  310, 0) // pid
SYS_NUM(process_vm_writev, 311, 0) // pid
SYS_NUM(kcmp,          312, 2)  // type
SYS_NUM2(seccomp,      317, 0, 1) // operation, flags
SYS_NUM2(kexec_load,   246, 0, 1) // entry, nr_segments
SYS_NUM(kexec_file_load, 320, 0) // kernel_fd
SYS_NUM(bpf,           321, 0)  // cmd
SYS_NUM(request_key,   249, 0)  // type
SYS_NUM(keyctl,        250, 0)  // option

char _license[] SEC("license") = "Dual MIT/GPL";
