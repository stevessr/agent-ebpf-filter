from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:180]!r}")
    return text.replace(old, new, 1)

# ---------------------------------------------------------------------------
# BPF common maps + explicit legacy handlers
# ---------------------------------------------------------------------------
p = Path("backend/ebpf/agent_tracker_common.h")
s = p.read_text()

old = '''// Exit path context map: stores path data for sys_exit (split due to 512-byte stack limit)
struct exit_path_data {
    char path[MAX_PATH_LEN];
    char extra4[MAX_PATH_LEN];
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 2048);
    __type(key, u64);
    __type(value, struct exit_path_data);
} exit_path_ctx SEC(".maps");
'''
new = '''// Exit path contexts are split by payload shape. Most path syscalls carry one
// 256-byte path, while dual-path syscalls and dynamic HTTP metadata need the
// full 512-byte pair. Keeping them separate halves hash-map value traffic for
// the common single-path case without inflating exit_meta for every syscall.
struct exit_single_path_data {
    char path[MAX_PATH_LEN];
};

struct exit_path_data {
    char path[MAX_PATH_LEN];
    char extra4[MAX_PATH_LEN];
};

_Static_assert(sizeof(struct exit_single_path_data) == MAX_PATH_LEN, "single path context ABI changed");
_Static_assert(sizeof(struct exit_path_data) == MAX_PATH_LEN * 2, "pair path context ABI changed");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 2048);
    __type(key, u64);
    __type(value, struct exit_single_path_data);
} exit_single_path_ctx SEC(".maps");

// Dual paths and recognized HTTP start-lines are lower-volume than ordinary
// single-path filesystem syscalls, so a smaller pool preserves the previous
// aggregate preallocated value budget (roughly 1 MiB across both maps).
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u64);
    __type(value, struct exit_path_data);
} exit_path_ctx SEC(".maps");
'''
s = replace_once(s, old, new, "split path map definitions")

# connect has a fixed label and should not use either dynamic path map. Also
# reject untracked connects before touching exit_ctx.
old = '''    u32 tag_id = get_tag_id(pid, comm, NULL);

    struct exit_meta meta = {};
'''
new = '''    u32 tag_id = get_tag_id(pid, comm, NULL);
    if (tag_id == 0) return 0;

    struct exit_meta meta = {};
'''
s = replace_once(s, old, new, "connect early tag rejection")

old = '''    store_exit_meta(pid_tgid, &meta);

    u32 zero = 0;
    struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);
    if (pd) {
        __builtin_memcpy(pd->path, "socket connect", 15);
        bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);
    }
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_connect")'''
new = '''    store_exit_meta(pid_tgid, &meta);
    return 0;
}

SEC("tracepoint/syscalls/sys_exit_connect")'''
s = replace_once(s, old, new, "connect static enter label removal")

old = '''    struct event *e = reserve_event();
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
// sys_enter / sys_exit: mkdirat'''
new = '''    struct event *e = reserve_event();
    if (!e) return 0;
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    __builtin_memcpy(e->path, "socket connect", 15);

    submit_event(e);
    return 0;
}

// ============================================================
// sys_enter / sys_exit: mkdirat'''
s = replace_once(s, old, new, "connect static exit label")

# The remaining explicit exit_path_ctx updates in this file are single-path
# execve/openat/mkdirat/unlinkat handlers. Store only the 256-byte primary path.
old_update = '    bpf_map_update_elem(&exit_path_ctx, &pid_tgid, pd, BPF_ANY);'
count = s.count(old_update)
if count != 4:
    raise SystemExit(f"expected 4 explicit single-path updates after connect rewrite, found {count}")
s = s.replace(old_update, '    bpf_map_update_elem(&exit_single_path_ctx, &pid_tgid, pd->path, BPF_ANY);')

old_exit = '''    struct event *e = reserve_event();
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
'''
new_exit = '''    struct event *e = reserve_event();
    if (!e) {
        bpf_map_delete_elem(&exit_single_path_ctx, &pid_tgid);
        return 0;
    }
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;

    struct exit_single_path_data *pd = bpf_map_lookup_elem(&exit_single_path_ctx, &pid_tgid);
    if (pd) {
        __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
        bpf_map_delete_elem(&exit_single_path_ctx, &pid_tgid);
    }

    submit_event(e);
'''
count = s.count(old_exit)
if count != 4:
    raise SystemExit(f"expected 4 explicit single-path exits, found {count}")
s = s.replace(old_exit, new_exit)
p.write_text(s)

# ---------------------------------------------------------------------------
# Generic syscall tail: route single and dual paths to different maps and make
# reserve failures clean up whichever context was staged.
# ---------------------------------------------------------------------------
p = Path("backend/ebpf/agent_tracker_tail.h")
s = p.read_text()
old = '''static __always_inline void sys_exit_common(struct trace_event_raw_sys_exit *ctx, int has_path) {
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
'''
new = '''#define EXIT_PATH_NONE   0
#define EXIT_PATH_SINGLE 1
#define EXIT_PATH_PAIR   2

static __always_inline void discard_sys_exit_path(u64 pid_tgid, int path_mode) {
    if (path_mode == EXIT_PATH_SINGLE) {
        bpf_map_delete_elem(&exit_single_path_ctx, &pid_tgid);
    } else if (path_mode == EXIT_PATH_PAIR) {
        bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
    }
}

static __always_inline void sys_exit_common(struct trace_event_raw_sys_exit *ctx, int path_mode) {
    u64 pid_tgid = bpf_get_current_pid_tgid();
    struct exit_meta meta = {};
    if (!consume_exit_meta(pid_tgid, &meta)) return;
    struct event *e = reserve_event();
    if (!e) {
        // The enter-side path has no future consumer once exit_meta is consumed.
        // Always clear it on ring-buffer pressure instead of leaving a stale
        // per-thread entry until the next syscall happens to overwrite it.
        discard_sys_exit_path(pid_tgid, path_mode);
        return;
    }
    fill_from_exit_meta(e, pid_tgid, &meta);
    e->retval = ctx->ret;
    if (meta.start_ns != 0) {
        u64 now = bpf_ktime_get_ns();
        if (now >= meta.start_ns) {
            e->duration_ns = now - meta.start_ns;
        }
    }
    if (path_mode == EXIT_PATH_SINGLE) {
        struct exit_single_path_data *pd = bpf_map_lookup_elem(&exit_single_path_ctx, &pid_tgid);
        if (pd) {
            __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
            bpf_map_delete_elem(&exit_single_path_ctx, &pid_tgid);
        }
    } else if (path_mode == EXIT_PATH_PAIR) {
        struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_ctx, &pid_tgid);
        if (pd) {
            __builtin_memcpy(e->path, pd->path, MAX_PATH_LEN);
            __builtin_memcpy(e->extra4, pd->extra4, MAX_PATH_LEN);
            bpf_map_delete_elem(&exit_path_ctx, &pid_tgid);
        }
    }
    submit_event(e);
}
'''
s = replace_once(s, old, new, "path-mode sys_exit_common")

# Modify each macro segment without touching dual-path storage by accident.
def patch_macro(text: str, name: str, next_marker: str, mode: str, single: bool) -> str:
    start = text.index(f'#define {name}')
    end = text.index(next_marker, start)
    segment = text[start:end]
    if single:
        old_map = 'bpf_map_update_elem(&exit_path_ctx, &ptid, pd, BPF_ANY);'
        if old_map not in segment:
            raise SystemExit(f"{name}: missing path update")
        segment = segment.replace(old_map, 'bpf_map_update_elem(&exit_single_path_ctx, &ptid, pd->path, BPF_ANY);', 1)
    old_call = 'sys_exit_common(ctx, 1);'
    if old_call not in segment:
        raise SystemExit(f"{name}: missing exit call")
    segment = segment.replace(old_call, f'sys_exit_common(ctx, {mode});', 1)
    return text[:start] + segment + text[end:]

s = patch_macro(s, 'SYS_PATH0(name, nr)', '// ── Macro: path at args[0], dual-path', 'EXIT_PATH_SINGLE', True)
s = patch_macro(s, 'SYS_PATH01(name, nr)', '// ── Macro: path at args[1] (fd-relative), single path', 'EXIT_PATH_PAIR', False)
s = patch_macro(s, 'SYS_PATH1(name, nr)', '// ── Macro: dual path at args[1]+args[3]', 'EXIT_PATH_SINGLE', True)
s = patch_macro(s, 'SYS_PATH13(name, nr)', '// ── Macro: symlinkat', 'EXIT_PATH_PAIR', False)
s = patch_macro(s, 'SYS_PATH02(name, nr)', '// ── Macro: fanotify_mark', 'EXIT_PATH_PAIR', False)
s = patch_macro(s, 'SYS_PATH4(name, nr)', '// ── Macro: comm-only with numeric extra2', 'EXIT_PATH_SINGLE', True)
s = s.replace('sys_exit_common(ctx, 0);', 'sys_exit_common(ctx, EXIT_PATH_NONE);')
p.write_text(s)

# ---------------------------------------------------------------------------
# Runtime pin/load list for the new map.
# ---------------------------------------------------------------------------
p = Path("backend/app/runtime_ebpf.go")
s = p.read_text()
s = replace_once(
    s,
    '"exit_ctx", "exit_path_buf", "exit_path_ctx", "socket_fds"',
    '"exit_ctx", "exit_path_buf", "exit_single_path_ctx", "exit_path_ctx", "socket_fds"',
    "runtime mapNames",
)
s = replace_once(
    s,
    '"exit_path_buf": objs.ExitPathBuf, "exit_path_ctx": objs.ExitPathCtx,\n\t\t"socket_fds":',
    '"exit_path_buf": objs.ExitPathBuf, "exit_single_path_ctx": objs.ExitSinglePathCtx,\n\t\t"exit_path_ctx": objs.ExitPathCtx, "socket_fds":',
    "runtime pinMaps",
)
p.write_text(s)

# ---------------------------------------------------------------------------
# Documentation
# ---------------------------------------------------------------------------
p = Path("docs/backend/generic-api-capture.md")
s = p.read_text()
addition = '''\n### Split dynamic path contexts\n\nDynamic path correlation is now shape-aware. Single-path filesystem syscalls\nuse a dedicated 256-byte `exit_single_path_ctx`, while dual-path operations and\nrecognized HTTP metadata retain the 512-byte `exit_path_ctx`. The pair map is\nreduced to 1024 entries while the single-path map has 2048 entries, keeping the\ncombined preallocated value budget approximately equal to the previous 2048 x\n512-byte map but halving update/lookup bandwidth for the dominant single-path\ncase. `connect()` no longer stages its fixed label in either map, and untracked\nconnects return before `exit_ctx` correlation. Both path maps are explicitly\ncleaned when ring-buffer reservation fails after exit metadata is consumed.\n'''
if addition not in s:
    s += addition
p.write_text(s)
