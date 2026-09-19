from pathlib import Path

def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:300]!r}")
    return text.replace(old, new, 1)

common_path = Path('backend/ebpf/agent_tracker_common.h')
common = common_path.read_text()
old = '''// Classify and copy an HTTP/1 start-line with a single 8-byte probe. The old
// request-then-response path probed non-HTTP buffers twice. This helper keeps
// identical privacy semantics while halving the head probes on the hot path.
static __always_inline u32 capture_http1_start_line(char *dst, const void *user_buf, u32 len, u32 *kind) {
    if (kind) *kind = HTTP1_START_NONE;
    if (!dst || !user_buf || len < 4 || !kind) return 0;
    char head[8] = {};
    u32 head_len = len < sizeof(head) ? len : sizeof(head);
    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return 0;

    int request = looks_like_http1_method(head, head_len);
    int response = !request && looks_like_http1_response(head, head_len);
    if (!request && !response) return 0;
    *kind = request ? HTTP1_START_REQUEST : HTTP1_START_RESPONSE;

    u32 capture_len = len;
    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;
    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) {
        *kind = HTTP1_START_NONE;
        return 0;
    }
#pragma clang loop unroll(disable)
    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {
        if ((u32)i >= capture_len) break;
        char c = dst[i];
        if (c == '\r' || c == '\n' || (request && (c == '?' || c == '#'))) {
            dst[i] = '\0';
            return (u32)i;
        }
    }
    dst[capture_len] = '\0';
    return capture_len;
}
'''
new = '''// Probe only the bounded HTTP/1 signature first. Callers use this as a cheap
// eligibility gate before touching per-CPU path scratch or copying up to 255
// bytes from userspace.
static __always_inline u32 classify_http1_start_line(const void *user_buf, u32 len) {
    if (!user_buf || len < 4) return HTTP1_START_NONE;
    char head[8] = {};
    u32 head_len = len < sizeof(head) ? len : sizeof(head);
    if (bpf_probe_read_user(head, head_len, user_buf) < 0) return HTTP1_START_NONE;
    if (looks_like_http1_method(head, head_len)) return HTTP1_START_REQUEST;
    if (looks_like_http1_response(head, head_len)) return HTTP1_START_RESPONSE;
    return HTTP1_START_NONE;
}

// Copy only after classify_http1_start_line() has already established that the
// payload is an HTTP/1 start-line. Query/fragment data is still stripped before
// the sample can cross into ring-buffer telemetry.
static __always_inline u32 capture_http1_start_line_kind(char *dst, const void *user_buf, u32 len, u32 kind) {
    if (!dst || !user_buf || len < 4 || kind == HTTP1_START_NONE) return 0;
    int request = kind == HTTP1_START_REQUEST;
    u32 capture_len = len;
    if (capture_len > MAX_PATH_LEN - 1) capture_len = MAX_PATH_LEN - 1;
    if (bpf_probe_read_user(dst, capture_len, user_buf) < 0) return 0;
#pragma clang loop unroll(disable)
    for (int i = 0; i < MAX_PATH_LEN - 1; i++) {
        if ((u32)i >= capture_len) break;
        char c = dst[i];
        if (c == '\r' || c == '\n' || (request && (c == '?' || c == '#'))) {
            dst[i] = '\0';
            return (u32)i;
        }
    }
    dst[capture_len] = '\0';
    return capture_len;
}

// Compatibility wrapper for call sites that do not need to separate eligibility
// from copying. Hot paths should classify before acquiring scratch.
static __always_inline u32 capture_http1_start_line(char *dst, const void *user_buf, u32 len, u32 *kind) {
    if (!kind) return 0;
    *kind = classify_http1_start_line(user_buf, len);
    return capture_http1_start_line_kind(dst, user_buf, len, *kind);
}
'''
common = replace_once(common, old, new, 'split HTTP/1 classifier/copy helpers')
common_path.write_text(common)

sys_path = Path('backend/ebpf/agent_tracker_syscalls.h')
sys = sys_path.read_text()

# Finish PID-first conversion for macro-generated correlation-only enter paths.
for label, old, new in (
    ('simple enter macro',
'''    u32 pid = pid_tgid >> 32; \\
    char comm[TASK_COMM_LEN]; \\
    bpf_get_current_comm(&comm, sizeof(comm)); \\
    u32 tag_id = get_tag_id(pid, comm, NULL); \\
''',
'''    u32 pid = pid_tgid >> 32; \\
    u32 tag_id = get_enter_tag_id_nopath(pid); \\
'''),
    ('dup enter macro',
'''    u32 tgid = (u32)(pid_tgid >> 32); \\
    char comm[TASK_COMM_LEN]; \\
    bpf_get_current_comm(&comm, sizeof(comm)); \\
    u32 tag_id = get_tag_id(tgid, comm, NULL); \\
''',
'''    u32 tgid = (u32)(pid_tgid >> 32); \\
    u32 tag_id = get_enter_tag_id_nopath(tgid); \\
'''),
):
    sys = replace_once(sys, old, new, label)

# The accept macro has the same selector prologue as dup; replace its remaining
# occurrence after the dup macro was converted.
sys = replace_once(
    sys,
'''    u32 tgid = (u32)(pid_tgid >> 32); \\
    char comm[TASK_COMM_LEN]; \\
    bpf_get_current_comm(&comm, sizeof(comm)); \\
    u32 tag_id = get_tag_id(tgid, comm, NULL); \\
''',
'''    u32 tgid = (u32)(pid_tgid >> 32); \\
    u32 tag_id = get_enter_tag_id_nopath(tgid); \\
''',
    'accept enter macro',
)

# Outgoing contiguous payloads: classify before per-CPU scratch lookup.
old = '''    if (!socket || socket_http1_capture_eligible(socket)) {
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
                store_exit_path_pair(pid_tgid, pd);
            }
        }
    }
'''
new = '''    if (!socket || socket_http1_capture_eligible(socket)) {
        u32 data_len = (u32)ctx->args[2];
        u32 start_kind = classify_http1_start_line((const void *)ctx->args[1], data_len);
        if (start_kind != HTTP1_START_NONE) {
            u32 zero = 0;
            struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
            if (pd) {
                u32 captured = capture_http1_start_line_kind(pd->extra4, (const void *)ctx->args[1], data_len, start_kind);
                if (captured > 0) {
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                    meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                    if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                    else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                    store_exit_path_pair(pid_tgid, pd);
                }
            }
        }
    }
'''
sys = replace_once(sys, old, new, 'sendto classify-before-scratch')

old = '''    if (socket_http1_capture_eligible(socket)) {
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
                store_exit_path_pair(pid_tgid, pd);
            }
        }
    }
'''
new = '''    if (socket_http1_capture_eligible(socket)) {
        u32 start_kind = classify_http1_start_line((const void *)ctx->args[1], requested);
        if (start_kind != HTTP1_START_NONE) {
            u32 zero = 0;
            struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
            if (pd) {
                u32 captured = capture_http1_start_line_kind(pd->extra4, (const void *)ctx->args[1], requested, start_kind);
                if (captured > 0) {
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                    meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                    if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                    else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                    store_exit_path_pair(pid_tgid, pd);
                }
            }
        }
    }
'''
sys = replace_once(sys, old, new, 'write classify-before-scratch')

old = '''    if (socket_http1_capture_eligible(socket) && have_iov) {
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
                store_exit_path_pair(pid_tgid, pd);
            }
        }
    }
'''
new = '''    if (socket_http1_capture_eligible(socket) && have_iov) {
        u32 start_kind = classify_http1_start_line((const void *)iov.base, first_len);
        if (start_kind != HTTP1_START_NONE) {
            u32 zero = 0;
            struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
            if (pd) {
                u32 captured = capture_http1_start_line_kind(pd->extra4, (const void *)iov.base, first_len, start_kind);
                if (captured > 0) {
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                    meta.capture_flags |= SOCKET_CAPTURE_OUTGOING;
                    if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                    else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                    store_exit_path_pair(pid_tgid, pd);
                }
            }
        }
    }
'''
# writev and sendmsg share this exact capture body, so require two conversions.
if sys.count(old) != 2:
    raise SystemExit(f'expected two scatter/gather outgoing capture blocks, found {sys.count(old)}')
sys = sys.replace(old, new, 2)

# Incoming read: only touch scratch after socket/return eligibility and protocol signature.
old = '''    u32 zero = 0;
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
'''
new = '''    struct exit_path_data *pd = 0;
    u32 captured = 0;
    if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && meta.addr_ptr != 0) {
        u32 actual = (u32)ctx->ret;
        u32 start_kind = classify_http1_start_line((const void *)meta.addr_ptr, actual);
        if (start_kind != HTTP1_START_NONE) {
            u32 zero = 0;
            pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
            if (pd) {
                captured = capture_http1_start_line_kind(pd->extra4, (const void *)meta.addr_ptr, actual, start_kind);
                if (captured > 0) {
                    meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
                    if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                    else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                    meta.type = TYPE_SOCKET_HTTP;
                    meta.extra2 = captured;
                }
            }
        }
    }
'''
sys = replace_once(sys, old, new, 'read classify-before-scratch')

old = '''    u32 zero = 0;
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
'''
new = '''    struct exit_path_data *pd = 0;
    u32 captured = 0;
    if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && meta.addr_ptr != 0 && meta.capture_reserved > 0) {
        struct capture_iovec64 iov = {};
        if (capture_first_iovec((const void *)meta.addr_ptr, meta.capture_reserved, &iov)) {
            u32 available = capture_iov_len(iov.len);
            if ((u64)ctx->ret < available) available = (u32)ctx->ret;
            u32 start_kind = classify_http1_start_line((const void *)iov.base, available);
            if (start_kind != HTTP1_START_NONE) {
                u32 zero = 0;
                pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
                if (pd) {
                    captured = capture_http1_start_line_kind(pd->extra4, (const void *)iov.base, available, start_kind);
                    if (captured > 0) {
                        meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
                        if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                        else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                        meta.type = TYPE_SOCKET_HTTP;
                        meta.extra2 = captured;
                    }
                }
            }
        }
    }
'''
sys = replace_once(sys, old, new, 'readv classify-before-scratch')

old = '''    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    u32 captured = 0;
    if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && meta.addr_ptr != 0) {
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
'''
new = '''    struct exit_path_data *pd = 0;
    u32 captured = 0;
    if (ctx->ret > 0 && meta.extra2 == 1 && (meta.socket_type & SOCK_TYPE_MASK) == SOCK_STREAM && meta.addr_ptr != 0) {
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
            u32 available = capture_iov_len(iov.len);
            if ((u64)ctx->ret < available) available = (u32)ctx->ret;
            u32 start_kind = classify_http1_start_line((const void *)iov.base, available);
            if (start_kind != HTTP1_START_NONE) {
                u32 zero = 0;
                pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
                if (pd) {
                    captured = capture_http1_start_line_kind(pd->extra4, (const void *)iov.base, available, start_kind);
                    if (captured > 0) {
                        meta.capture_flags |= SOCKET_CAPTURE_INCOMING;
                        if (start_kind == HTTP1_START_REQUEST) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_REQUEST_LINE;
                        else if (start_kind == HTTP1_START_RESPONSE) meta.capture_flags |= SOCKET_CAPTURE_HTTP1_RESPONSE_LINE;
                        meta.type = TYPE_SOCKET_HTTP;
                        meta.extra2 = captured;
                    }
                }
            }
        }
    }
'''
sys = replace_once(sys, old, new, 'recvmsg classify-before-scratch')

sys_path.write_text(sys)

test_path = Path('backend/app/runtime_tracking_mode_source_test.go')
test = test_path.read_text()
addition = r'''

func TestL7ScratchLookupAfterProtocolGateSourceContract(t *testing.T) {
	commonBytes, err := os.ReadFile("../ebpf/agent_tracker_common.h")
	if err != nil {
		t.Fatal(err)
	}
	syscallBytes, err := os.ReadFile("../ebpf/agent_tracker_syscalls.h")
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBytes)
	syscalls := string(syscallBytes)

	if !strings.Contains(common, "static __always_inline u32 classify_http1_start_line") ||
		!strings.Contains(common, "static __always_inline u32 capture_http1_start_line_kind") {
		t.Fatal("HTTP/1 classifier/copy split is missing")
	}

	assertGate := func(name, marker string) {
		t.Helper()
		block := sourceBlock(t, syscalls, marker, "\n}\n")
		classify := strings.Index(block, "classify_http1_start_line")
		scratch := strings.Index(block, "bpf_map_lookup_elem(&exit_path_buf")
		if classify < 0 || scratch < 0 {
			t.Fatalf("%s is missing classifier or scratch lookup", name)
		}
		if classify > scratch {
			t.Fatalf("%s acquires L7 scratch before protocol classification", name)
		}
	}

	for _, name := range []string{"sendto", "write", "writev", "sendmsg"} {
		assertGate(name, "int tracepoint__syscalls__sys_enter_"+name+"(struct trace_event_raw_sys_enter *ctx) {")
	}
	for _, name := range []string{"read", "readv", "recvmsg"} {
		assertGate(name, "int tracepoint__syscalls__sys_exit_"+name+"(struct trace_event_raw_sys_exit *ctx) {")
	}
}

func TestRemainingMacroEnterPathsArePIDFirst(t *testing.T) {
	syscallBytes, err := os.ReadFile("../ebpf/agent_tracker_syscalls.h")
	if err != nil {
		t.Fatal(err)
	}
	src := string(syscallBytes)
	for _, macro := range []string{"DEFINE_SIMPLE_ENTER_HANDLER", "DEFINE_DUP_HANDLER", "DEFINE_ACCEPT_HANDLER"} {
		start := strings.Index(src, "#define "+macro)
		if start < 0 {
			t.Fatalf("missing macro %s", macro)
		}
		rest := src[start:]
		end := strings.Index(rest, "\n\n")
		if end < 0 {
			end = len(rest)
		}
		block := rest[:end]
		if !strings.Contains(block, "get_enter_tag_id_nopath") {
			t.Fatalf("%s bypasses PID-first selector", macro)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s still performs unconditional enter-side comm read", macro)
		}
	}
}
'''
if 'func TestL7ScratchLookupAfterProtocolGateSourceContract' not in test:
    test += addition
test_path.write_text(test)

doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
addition = '''

### L7 sampling fast gates

HTTP/1 sampling now separates an 8-byte protocol classifier from the bounded
start-line copy. Socket read/write/send paths classify the user buffer before
acquiring the per-CPU path scratch map, so ordinary file reads, non-stream
socket traffic, and non-HTTP stream traffic avoid the scratch lookup and the
up-to-255-byte copy entirely. Only a positive HTTP/1 request/response signature
enters the metadata-copy path. Query/fragment stripping and all existing privacy
boundaries are unchanged.

The remaining macro-generated simple, dup, and accept enter handlers also use
the same PID-first selector helper, eliminating their unconditional comm helper
for registered Agent PIDs.
'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
