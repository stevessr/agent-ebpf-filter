from pathlib import Path


def read(path):
    return Path(path).read_text()


def write(path, content):
    Path(path).write_text(content)


def replace_once(path, old, new):
    text = read(path)
    if old not in text:
        raise SystemExit(f"missing hardening anchor in {path}: {old[:120]!r}")
    write(path, text.replace(old, new, 1))


replace_once(
    "backend/ebpf/agent_tracker_common.h",
    r'''// Copy only the HTTP/1 request line, never headers/body. This keeps the kernel
// sampler useful for request-path discovery while avoiding Authorization/Cookie
// material. Query strings are removed in userspace before persistence.
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
        if (dst[i] == '\r' || dst[i] == '\n') {
            dst[i] = '\0';
            return (u32)i;
        }
    }
    dst[capture_len] = '\0';
    return capture_len;
}
''',
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
''')

# Do not inspect payloads for untracked processes. This also avoids retaining
# exit_path scratch entries that have no matching userspace event.
replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    '''    u32 tag_id = get_tag_id(pid, comm, NULL);\n    struct exit_meta meta = {.type = TYPE_SENDTO, .tag_id = tag_id};\n    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);\n    meta.extra1 = (u32)ctx->args[0];\n    meta.extra3 = (u32)ctx->args[2];\n''',
    '''    u32 tag_id = get_tag_id(pid, comm, NULL);\n    if (tag_id == 0) return 0;\n    struct exit_meta meta = {.type = TYPE_SENDTO, .tag_id = tag_id};\n    fill_network_meta(&meta, (const void *)ctx->args[4], NET_DIR_OUTGOING, (u32)ctx->args[2]);\n    meta.extra1 = (u32)ctx->args[0];\n    meta.extra3 = (u32)ctx->args[2];\n    if (meta.net_family == 0) {\n        struct socket_fd_meta *socket = lookup_socket_fd(pid, (s32)ctx->args[0]);\n        if (socket) fill_network_meta_from_socket(&meta, socket, (u32)ctx->args[2]);\n    }\n''')

# close() is provenance maintenance, not a new semantic audit event. Avoid a
# fake architecture-specific syscall number and only delete the fd identity on
# a successful close.
replace_once(
    "backend/ebpf/agent_tracker_syscalls.h",
    r'''SEC("tracepoint/syscalls/sys_enter_close")
int tracepoint__syscalls__sys_enter_close(struct trace_event_raw_sys_enter *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    u32 tgid = pid_tgid >> 32;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    u32 tag_id = get_tag_id(tgid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_meta meta = {.type = TYPE_GENERIC_SYSCALL, .tag_id = tag_id, .extra1 = 3, .extra2 = (u32)ctx->args[0]};
    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_close")
int tracepoint__syscalls__sys_exit_close(struct trace_event_raw_sys_exit *ctx) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return 0;
    if (ctx->ret == 0) forget_socket_fd((u32)(pid_tgid >> 32), (s32)meta.extra2);
    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    submit_event(e);
    return 0;
}
''',
    r'''SEC("tracepoint/syscalls/sys_enter_close")
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
''')

# A pinned fd map must never bridge tracker generations: PID/fd pairs can be
# recycled while the backend is down. Clear it before attaching a compatible
# reload and let new socket/connect activity repopulate authoritative entries.
replace_once(
    "backend/app/runtime_ebpf.go",
    '''\t\t\tgeneration, err := rotateKernelAuditGeneration(objs.CollectorStats)\n\t\t\tif err != nil {\n\t\t\t\tplatform.CloseMapHandles(replacements)\n\t\t\t\treturn nil, err\n\t\t\t}\n\t\t\tif err := pinLinks(&objs); err != nil {\n''',
    '''\t\t\tgeneration, err := rotateKernelAuditGeneration(objs.CollectorStats)\n\t\t\tif err != nil {\n\t\t\t\tplatform.CloseMapHandles(replacements)\n\t\t\t\treturn nil, err\n\t\t\t}\n\t\t\tif err := clearSocketFDProvenance(objs.SocketFds); err != nil {\n\t\t\t\tplatform.CloseMapHandles(replacements)\n\t\t\t\treturn nil, err\n\t\t\t}\n\t\t\tif err := pinLinks(&objs); err != nil {\n''')

replace_once(
    "backend/app/runtime_ebpf.go",
    '''func pinMaps(objs *bpf.AgentTrackerObjects) error {\n''',
    r'''func clearSocketFDProvenance(socketFds *ebpf.Map) error {
	if socketFds == nil {
		return errors.New("socket_fds map is nil")
	}
	iter := socketFds.Iterate()
	keys := make([]bpf.AgentTrackerSocketFdKey, 0, 64)
	var key bpf.AgentTrackerSocketFdKey
	var value bpf.AgentTrackerSocketFdMeta
	for iter.Next(&key, &value) {
		keys = append(keys, key)
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("iterate socket_fds before generation rotation: %w", err)
	}
	for i := range keys {
		if err := socketFds.Delete(&keys[i]); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
			return fmt.Errorf("clear socket_fds provenance: %w", err)
		}
	}
	return nil
}

func pinMaps(objs *bpf.AgentTrackerObjects) error {
''')

replace_once(
    "backend/app/tls/api_fingerprint.go",
    '''\tif event.Vendor == "" || strings.HasSuffix(match.Vendor, "-compatible") == false {\n\t\t// A host-specific fingerprint is stronger than the historical substring\n\t\t// vendor inference. Path-only compatible profiles only fill an empty value.\n\t\tif event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {\n\t\t\tevent.Vendor = match.Vendor\n\t\t}\n\t}\n''',
    '''\t// A host-specific fingerprint is stronger than the historical substring\n\t// vendor inference. Path-only compatible profiles only fill an empty value.\n\tif event.Vendor == "" || !strings.HasSuffix(match.Vendor, "-compatible") {\n\t\tevent.Vendor = match.Vendor\n\t}\n''')

replace_once(
    "docs/backend/generic-api-capture.md",
    '''For confirmed sockets, the eBPF program performs a bounded HTTP/1 method check and copies **only the request line** (maximum 255 bytes) into the event scratch buffer. Headers and bodies are deliberately not copied, so Authorization/Cookie material is outside this kernel capture path. A matching event is emitted as `SOCKET_HTTP` with the fd, captured prefix length, requested byte count, endpoint metadata, and the same kernel audit generation/CPU sequence/loss provenance as every other tracker event.\n\n`sendto()` uses the same request-line helper. Non-HTTP socket writes remain ordinary `write`/`sendto` events and do not carry payload prefixes.\n\nThe userspace normalization boundary immediately strips query and fragment data before the request line enters `pb.Event`, preventing API keys embedded in query strings from being persisted in `extra_path`.\n''',
    '''For confirmed sockets, the eBPF program performs a bounded HTTP/1 method check and copies **only the request line** (maximum 255 bytes) into the event scratch buffer. Headers and bodies are deliberately not copied, so Authorization/Cookie material is outside this kernel capture path. Query strings and fragments are truncated and their captured tail bytes are zeroed **inside eBPF before ringbuf submission**, so URL credentials do not cross this kernel/userspace boundary. A matching event is emitted as `SOCKET_HTTP` with the fd, captured prefix length, requested byte count, endpoint metadata, and the same kernel audit generation/CPU sequence/loss provenance as every other tracker event.\n\n`sendto()` uses the same request-line helper and falls back to the fd-provenance endpoint for connected sockets without an explicit destination address. Non-HTTP socket writes remain ordinary `write`/`sendto` events and do not carry payload prefixes.\n\nUserspace still normalizes the target a second time before it enters `pb.Event`, providing defense in depth against query/fragment persistence.\n''')

replace_once(
    "docs/backend/generic-api-capture.md",
    '''The socket-fd map currently follows `socket`, `connect`, `write`, `sendto`, and `close`. Descriptor duplication (`dup*`), inherited sockets across fork, scatter/gather `writev/sendmsg`, and decrypted HTTP/3/QUIC are separate follow-ups. TLS/library capture already remains the authoritative source for encrypted HTTP request paths.\n''',
    '''The socket-fd map currently follows `socket`, `connect`, `write`, `sendto`, and `close`. It is cleared on tracker generation rotation so a recycled `(tgid, fd)` cannot inherit provenance across backend downtime. Descriptor duplication (`dup*`), inherited sockets across fork, scatter/gather `writev/sendmsg`, and decrypted HTTP/3/QUIC are separate follow-ups. TLS/library capture already remains the authoritative source for encrypted HTTP request paths.\n''')
