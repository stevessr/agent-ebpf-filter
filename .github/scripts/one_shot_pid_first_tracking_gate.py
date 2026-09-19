from pathlib import Path

def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:260]!r}")
    return text.replace(old, new, 1)

common_path = Path('backend/ebpf/agent_tracker_common.h')
common = common_path.read_text()

old_helpers = '''static __always_inline u32 get_pid_comm_tag_id(u32 pid, char *comm) {
    u32 *tag = bpf_map_lookup_elem(&agent_pids, &pid);
    if (tag) return *tag;
    tag = bpf_map_lookup_elem(&tracked_comms, comm);
    return tag ? *tag : 0;
}
'''
new_helpers = '''static __always_inline u32 get_pid_tag_id(u32 pid) {
    u32 *tag = bpf_map_lookup_elem(&agent_pids, &pid);
    return tag ? *tag : 0;
}

static __always_inline u32 get_comm_tag_id(char *comm) {
    if (!comm) return 0;
    u32 *tag = bpf_map_lookup_elem(&tracked_comms, comm);
    return tag ? *tag : 0;
}

static __always_inline u32 get_pid_comm_tag_id(u32 pid, char *comm) {
    u32 tag_id = get_pid_tag_id(pid);
    return tag_id ? tag_id : get_comm_tag_id(comm);
}

// Registered Agent PIDs are the dominant hot path, so avoid reading comm on a
// PID hit. Exit-side event construction reads comm independently.
static __always_inline u32 get_enter_tag_id_nopath(u32 pid) {
    u32 tag_id = get_pid_tag_id(pid);
    if (tag_id) return tag_id;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    return get_comm_tag_id(comm);
}

// Same PID-first fast path for path-bearing syscalls. tracking_mode is only
// consulted after both PID and comm miss.
static __always_inline u32 get_enter_tag_id_pre_path(u32 pid, u32 *path_flags) {
    u32 tag_id = get_pid_tag_id(pid);
    if (tag_id) return tag_id;
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    tag_id = get_comm_tag_id(comm);
    if (tag_id) return tag_id;
    if (path_flags) *path_flags = tracking_path_mode();
    return 0;
}
'''
common = replace_once(common, old_helpers, new_helpers, 'PID/comm split helpers')

for fn in ('execve', 'openat', 'mkdirat', 'unlinkat'):
    marker = f'int tracepoint__syscalls__sys_enter_{fn}(struct trace_event_raw_sys_enter *ctx) {{'
    start = common.find(marker)
    if start < 0:
        raise SystemExit(f'missing explicit handler {fn}')
    end = common.find('\n}\n', start)
    if end < 0:
        raise SystemExit(f'missing end of explicit handler {fn}')
    end += 3
    block = common[start:end]
    old = '''    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));

    u32 path_flags = 0;
    u32 tag_id = get_tag_id_pre_path(pid, comm, &path_flags);
'''
    new = '''    u32 path_flags = 0;
    u32 tag_id = get_enter_tag_id_pre_path(pid, &path_flags);
'''
    block = replace_once(block, old, new, f'{fn} lazy comm match')
    common = common[:start] + block + common[end:]

common_path.write_text(common)

tail_path = Path('backend/ebpf/agent_tracker_tail.h')
tail = tail_path.read_text()

old_nopath = '''static __always_inline int sys_enter_common_nopath(u64 ptid, char *comm, u32 nr, u32 extra2, u32 extra3) {
    u32 pid = (u32)(ptid >> 32);
    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;
    struct exit_compact_meta meta = {};
    meta.type = TYPE_GENERIC_SYSCALL;
    meta.tag_id = tag_id;
    meta.extra1 = nr;
    meta.extra2 = extra2;
    meta.extra3 = extra3;
    meta.start_ns = bpf_ktime_get_ns();
    return store_exit_compact_meta(ptid, &meta);
}
'''
new_nopath = '''static __always_inline int sys_enter_common_nopath(u64 ptid, u32 nr, u32 extra2, u32 extra3) {
    u32 tag_id = get_enter_tag_id_nopath((u32)(ptid >> 32));
    if (tag_id == 0) return 0;
    struct exit_compact_meta meta = {};
    meta.type = TYPE_GENERIC_SYSCALL;
    meta.tag_id = tag_id;
    meta.extra1 = nr;
    meta.extra2 = extra2;
    meta.extra3 = extra3;
    meta.start_ns = bpf_ktime_get_ns();
    return store_exit_compact_meta(ptid, &meta);
}
'''
tail = replace_once(tail, old_nopath, new_nopath, 'lazy no-path helper')

old_path_prefix = '''    STORE_PID_TGID(); \\
    char comm[TASK_COMM_LEN]; \\
    bpf_get_current_comm(&comm, sizeof(comm)); \\
    u32 path_flags = 0; \\
    u32 tag_id = get_tag_id_pre_path((u32)(ptid >> 32), comm, &path_flags); \\
'''
new_path_prefix = '''    STORE_PID_TGID(); \\
    u32 path_flags = 0; \\
    u32 tag_id = get_enter_tag_id_pre_path((u32)(ptid >> 32), &path_flags); \\
'''
path_macro_count = tail.count(old_path_prefix)
if path_macro_count != 6:
    raise SystemExit(f'expected 6 generic path macros, found {path_macro_count}')
tail = tail.replace(old_path_prefix, new_path_prefix)

old_num = '''    u64 ptid = bpf_get_current_pid_tgid(); \\
    char comm[TASK_COMM_LEN]; \\
    bpf_get_current_comm(&comm, sizeof(comm)); \\
    sys_enter_common_nopath(ptid, comm, nr, (u32)ctx->args[arg_idx], 0); \\
'''
new_num = '''    u64 ptid = bpf_get_current_pid_tgid(); \\
    sys_enter_common_nopath(ptid, nr, (u32)ctx->args[arg_idx], 0); \\
'''
tail = replace_once(tail, old_num, new_num, 'SYS_NUM lazy comm')

old_num2 = '''    u64 ptid = bpf_get_current_pid_tgid(); \\
    char comm[TASK_COMM_LEN]; \\
    bpf_get_current_comm(&comm, sizeof(comm)); \\
    sys_enter_common_nopath(ptid, comm, nr, (u32)ctx->args[a_idx], (u32)ctx->args[b_idx]); \\
'''
new_num2 = '''    u64 ptid = bpf_get_current_pid_tgid(); \\
    sys_enter_common_nopath(ptid, nr, (u32)ctx->args[a_idx], (u32)ctx->args[b_idx]); \\
'''
tail = replace_once(tail, old_num2, new_num2, 'SYS_NUM2 lazy comm')
tail_path.write_text(tail)

test_path = Path('backend/app/runtime_tracking_mode_source_test.go')
test = test_path.read_text()
addition = r'''

func assertPIDLookupBeforeCommRead(t *testing.T, block, name string) {
	t.Helper()
	pid := strings.Index(block, "get_pid_tag_id(pid)")
	comm := strings.Index(block, "bpf_get_current_comm")
	if pid < 0 {
		t.Fatalf("%s is missing PID-first lookup", name)
	}
	if comm < 0 {
		t.Fatalf("%s is missing lazy comm fallback", name)
	}
	if pid > comm {
		t.Fatalf("%s reads comm before checking tracked PID", name)
	}
}

func TestPIDFirstEnterTrackingSourceContract(t *testing.T) {
	commonBytes, err := os.ReadFile("../ebpf/agent_tracker_common.h")
	if err != nil {
		t.Fatal(err)
	}
	tailBytes, err := os.ReadFile("../ebpf/agent_tracker_tail.h")
	if err != nil {
		t.Fatal(err)
	}
	common := string(commonBytes)
	tail := string(tailBytes)

	for _, helper := range []string{"get_enter_tag_id_nopath", "get_enter_tag_id_pre_path"} {
		marker := "static __always_inline u32 " + helper
		block := sourceBlock(t, common, marker, "\n}\n")
		assertPIDLookupBeforeCommRead(t, block, helper)
	}

	for _, name := range []string{"execve", "openat", "mkdirat", "unlinkat"} {
		marker := "int tracepoint__syscalls__sys_enter_" + name + "(struct trace_event_raw_sys_enter *ctx) {"
		block := sourceBlock(t, common, marker, "\n}\n")
		if !strings.Contains(block, "get_enter_tag_id_pre_path") {
			t.Fatalf("%s bypasses PID-first path matcher", name)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", name)
		}
	}

	for _, macro := range []string{"SYS_PATH0", "SYS_PATH01", "SYS_PATH1", "SYS_PATH13", "SYS_PATH02", "SYS_PATH4"} {
		marker := "#define " + macro + "(name, nr)"
		start := strings.Index(tail, marker)
		if start < 0 {
			t.Fatalf("missing macro %s", macro)
		}
		rest := tail[start:]
		end := strings.Index(rest, "\n// ── Macro:")
		if end < 0 {
			end = len(rest)
		}
		block := rest[:end]
		if !strings.Contains(block, "get_enter_tag_id_pre_path") {
			t.Fatalf("%s bypasses PID-first path matcher", macro)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", macro)
		}
	}

	for _, macro := range []string{"SYS_NUM", "SYS_NUM2"} {
		marker := "#define " + macro + "(name, nr"
		start := strings.Index(tail, marker)
		if start < 0 {
			t.Fatalf("missing macro %s", macro)
		}
		rest := tail[start:]
		end := strings.Index(rest, "\n// ")
		if end < 0 {
			end = len(rest)
		}
		block := rest[:end]
		if !strings.Contains(block, "sys_enter_common_nopath") {
			t.Fatalf("%s bypasses lazy no-path matcher", macro)
		}
		if strings.Contains(block, "bpf_get_current_comm") {
			t.Fatalf("%s performs an unconditional enter-side comm read", macro)
		}
	}
}
'''
if 'func TestPIDFirstEnterTrackingSourceContract' not in test:
    test += addition
test_path.write_text(test)

doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
doc_addition = '''

### PID-first enter-side selector lookup

Correlation-only syscall enter programs now check the registered Agent PID map
before reading the current command name. For a PID hit, path and scalar syscall
macros skip the enter-side bpf_get_current_comm helper entirely; event
construction on sys_exit still reads and reports the current comm as before.
Only a PID miss pays the comm helper and tracked_comms lookup, and path-bearing
syscalls proceed to tracking_mode/path inspection only after both selectors
miss. Immediate tracepoint emitters such as TCP flow events are intentionally
unchanged because they need comm for the event emitted in that same program.
'''
if doc_addition not in doc:
    doc += doc_addition
doc_path.write_text(doc)
