SEC("tracepoint/syscalls/sys_enter_ioctl")
int tracepoint__syscalls__sys_enter_ioctl(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_IOCTL;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // request

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "Special Resource Interaction (ioctl)", 38);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_ioctl")
int tracepoint__syscalls__sys_exit_ioctl(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: bind (comm-only, network from args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_bind")
int tracepoint__syscalls__sys_enter_bind(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_BIND;
    meta.tag_id = tag_id;
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

SEC("tracepoint/syscalls/sys_exit_bind")
int tracepoint__syscalls__sys_exit_bind(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: sendto (comm-only, network from args[4], len at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_sendto")
int tracepoint__syscalls__sys_enter_sendto(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_SENDTO;
    meta.tag_id = tag_id;
    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);
    meta.extra3 = (u32)ctx->args[2]; // byte count

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket sendto", 14);

        // Capture initial payload bytes for protocol detection (TLS SNI, HTTP, DNS)
        // Only for tracked processes with reasonable payload size
        u32 data_len = (u32)ctx->args[2];
        if (tag_id != 0 && data_len > 0 && data_len <= (MAX_PATH_LEN - 1)) {
            u32 capture_len = data_len & (MAX_PATH_LEN - 1);
            const void *user_buf = (const void *)ctx->args[1];
            bpf_probe_read_user(pd->extra4, capture_len, user_buf);
            pd->extra4[capture_len] = '\0';
        } else {
            __builtin_memset(pd->extra4, 0, MAX_PATH_LEN);
        }

        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_sendto")
int tracepoint__syscalls__sys_exit_sendto(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: recvfrom (comm-only, network from args[4], len at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_recvfrom")
int tracepoint__syscalls__sys_enter_recvfrom(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_RECVFROM;
    meta.tag_id = tag_id;
    meta.extra3 = (u32)ctx->args[2]; // byte count
    meta.addr_ptr = ctx->args[4]; // Store pointer to read at exit

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket recvfrom", 16);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_recvfrom")
int tracepoint__syscalls__sys_exit_recvfrom(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;

    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    // Read the address now that the syscall has completed
    if (meta.addr_ptr && ctx->ret > 0) {
        fill_network_endpoint(e, (void *)meta.addr_ptr, NET_DIR_INCOMING, (u32)ctx->ret);
    }

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: read (no path, fd at args[0], count at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_read")
int tracepoint__syscalls__sys_enter_read(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_READ;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.extra3 = (u32)ctx->args[2]; // count

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_read")
int tracepoint__syscalls__sys_exit_read(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: write (no path, fd at args[0], count at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_write")
int tracepoint__syscalls__sys_enter_write(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_WRITE;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.extra3 = (u32)ctx->args[2]; // count

    store_exit_meta(pid_tgid, &meta);

    // Capture initial payload bytes for protocol detection
    u32 data_len = (u32)ctx->args[2];
    if (data_len > 0 && data_len <= (MAX_PATH_LEN - 1)) {
        u32 zero = 0;
        struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
        if (pd) {
            u32 capture_len = data_len & (MAX_PATH_LEN - 1);
            const void *user_buf = (const void *)ctx->args[1];
            bpf_probe_read_user(pd->extra4, capture_len, user_buf);
            pd->extra4[capture_len] = '\0';
            pd->path[0] = 'w'; pd->path[1] = 'r'; pd->path[2] = 'i'; pd->path[3] = 't';
            pd->path[4] = 'e'; pd->path[5] = '\0';
            bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
        }
    }

    return 0;
}

SEC("tracepoint/syscalls/sys_exit_write")
int tracepoint__syscalls__sys_exit_write(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: open (path at args[0], flags at args[1], mode at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_open")
int tracepoint__syscalls__sys_enter_open(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_OPEN;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // flags
    meta.extra2 = (u32)ctx->args[2]; // mode

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_open")
int tracepoint__syscalls__sys_exit_open(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: chmod (path at args[0], mode at args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_chmod")
int tracepoint__syscalls__sys_enter_chmod(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_CHMOD;
    meta.tag_id = tag_id;
    meta.extra2 = (u32)ctx->args[1]; // mode

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_chmod")
int tracepoint__syscalls__sys_exit_chmod(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: chown (path at args[0], uid at args[1], gid at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_chown")
int tracepoint__syscalls__sys_enter_chown(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_CHOWN;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // uid
    meta.extra2 = (u32)ctx->args[2]; // gid

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_chown")
int tracepoint__syscalls__sys_exit_chown(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: rename (path at args[0]=oldpath, extra4=args[1]=newpath)
// ============================================================
SEC("tracepoint/syscalls/sys_enter_rename")
int tracepoint__syscalls__sys_enter_rename(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *oldpath = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, oldpath);
    const char *newpath = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, newpath);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_RENAME;
    meta.tag_id = tag_id;

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_rename")
int tracepoint__syscalls__sys_exit_rename(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: link (path at args[0]=oldpath/target, extra4=args[1]=newpath)
// ============================================================
SEC("tracepoint/syscalls/sys_enter_link")
int tracepoint__syscalls__sys_enter_link(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *oldpath = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, oldpath);
    const char *newpath = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, newpath);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_LINK;
    meta.tag_id = tag_id;

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_link")
int tracepoint__syscalls__sys_exit_link(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: symlink (path at args[0]=target, extra4=args[1]=linkpath)
// ============================================================
SEC("tracepoint/syscalls/sys_enter_symlink")
int tracepoint__syscalls__sys_enter_symlink(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *target = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, target);
    const char *linkpath = (const char *)ctx->args[1];
    bpf_probe_read_user_str(pd->extra4, MAX_PATH_LEN, linkpath);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_SYMLINK;
    meta.tag_id = tag_id;

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_symlink")
int tracepoint__syscalls__sys_exit_symlink(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: mknod (path at args[0], mode at args[1], dev at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_mknod")
int tracepoint__syscalls__sys_enter_mknod(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (!pd) return 0;
    const char *filename = (const char *)ctx->args[0];
    bpf_probe_read_user_str(pd->path, MAX_PATH_LEN, filename);

    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_MKNOD;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[1]; // mode
    meta.extra2 = (u32)ctx->args[2]; // dev

    store_exit_meta(pid_tgid, &meta);
    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_mknod")
int tracepoint__syscalls__sys_exit_mknod(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: clone (no path, flags at args[0])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_clone")
int tracepoint__syscalls__sys_enter_clone(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_CLONE;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // flags

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_clone")
int tracepoint__syscalls__sys_exit_clone(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    // Auto-track child PID: if the parent is tracked in agent_pids,
    // register the child with the same tag for full process-tree tracing.
    u32 child_pid = (u32)ctx->ret;
    if (child_pid > 0) {
        u32 parent_pid = (u32)(pid_tgid >> 32);
        u32 *tag = bpf_map_lookup_elem(&agent_pids, &parent_pid);
        if (tag) {
            bpf_map_update_elem(&agent_pids, &child_pid, tag, BPF_NOEXIST);
        }
    }

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: exit_group (no path, status at args[0])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_exit_group")
int tracepoint__syscalls__sys_enter_exit_group(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_EXIT;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // status

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_exit_group")
int tracepoint__syscalls__sys_exit_exit_group(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: wait4 (target pid at args[0], options at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_wait4")
int tracepoint__syscalls__sys_enter_wait4(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_WAIT4;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)(s32)ctx->args[0];
    meta.extra2 = (u32)ctx->args[2];
    meta.start_ns = bpf_ktime_get_ns();

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_wait4")
int tracepoint__syscalls__sys_exit_wait4(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (meta.start_ns != 0) {
        u64 now = bpf_ktime_get_ns();
        if (now >= meta.start_ns) {
            e->duration_ns = now - meta.start_ns;
        }
    }
    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: socket (no path, domain at args[0], type at args[1], protocol at args[2])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_socket")
int tracepoint__syscalls__sys_enter_socket(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
    meta.type = TYPE_SOCKET;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // domain
    meta.extra2 = (u32)ctx->args[1]; // type
    meta.extra3 = (u32)ctx->args[2]; // protocol

    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_socket")
int tracepoint__syscalls__sys_exit_socket(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: accept (no path, fd at args[0], network from args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_accept")
int tracepoint__syscalls__sys_enter_accept(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_ACCEPT;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.addr_ptr = ctx->args[1]; // Store pointer to read at exit

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket accept", 14);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_accept")
int tracepoint__syscalls__sys_exit_accept(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;

    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    if (meta.addr_ptr && ctx->ret >= 0) {
        fill_network_endpoint(e, (void *)meta.addr_ptr, NET_DIR_INCOMING, 0);
    }

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: accept4 (no path, fd at args[0], network from args[1])
// ============================================================
SEC("tracepoint/syscalls/sys_enter_accept4")
int tracepoint__syscalls__sys_enter_accept4(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 pid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
    meta.type = TYPE_ACCEPT4;
    meta.tag_id = tag_id;
    meta.extra1 = (u32)ctx->args[0]; // fd
    meta.addr_ptr = ctx->args[1]; // Store pointer to read at exit

    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket accept4", 15);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_accept4")
int tracepoint__syscalls__sys_exit_accept4(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;

    struct event *e = reserve_event();
    if (!e) return 0;

    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    if (meta.addr_ptr && ctx->ret >= 0) {
        fill_network_endpoint(e, (void *)meta.addr_ptr, NET_DIR_INCOMING, 0);
    }

    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }

    submit_event(e);
    return 0;
}

// ============================================================
// Per-syscall handlers — generated via macros for all remaining
// path-carrying and security-relevant Linux syscalls.
// ============================================================

// ── enter/exit helpers shared by macros ──

