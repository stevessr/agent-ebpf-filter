from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:180]!r}")
    return text.replace(old, new, 1)

common_path = Path('backend/ebpf/agent_tracker_common.h')
common = common_path.read_text()

exit_ctx_anchor = '''struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 10240);
    __type(key, u64);
    __type(value, struct exit_meta);
} exit_ctx SEC(".maps");
'''
compact_def = exit_ctx_anchor + '''
// Most filesystem/process/descriptor correlation needs only scalar metadata.
// Keep it out of the network-capable 88-byte exit_meta map to reduce hash-map
// value bandwidth on common syscalls without weakening enter/exit correlation.
struct exit_compact_meta {
    u32 type;
    u32 tag_id;
    u32 extra1;
    u32 extra2;
    u64 extra3;
    u64 start_ns;
};

_Static_assert(sizeof(struct exit_compact_meta) == 32, "compact exit context ABI changed");

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 8192);
    __type(key, u64);
    __type(value, struct exit_compact_meta);
} exit_compact_ctx SEC(".maps");
'''
common = replace_once(common, exit_ctx_anchor, compact_def, 'compact exit map')

store_anchor = '''static __always_inline void store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);
}
'''
store_new = store_anchor + '''
static __always_inline void store_exit_compact_meta(u64 pid_tgid, struct exit_compact_meta *meta) {
    bpf_map_update_elem(&exit_compact_ctx, &pid_tgid, meta, BPF_ANY);
}
'''
common = replace_once(common, store_anchor, store_new, 'compact store helper')

consume_anchor = '''// Convenience inline for sys_exit handlers that only need pid_tgid correlation
// Returns 0 if no context was found (not a tracked syscall)
static __always_inline u32 consume_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
'''
compact_helpers = '''// Compact correlation for generic filesystem/process/fd syscalls. These events
// intentionally carry no network endpoint or capture-provenance fields.
static __always_inline u32 consume_exit_compact_meta(u64 pid_tgid, struct exit_compact_meta *meta) {
    struct exit_compact_meta *m = bpf_map_lookup_elem(&exit_compact_ctx, &pid_tgid);
    if (!m) return 0;
    __builtin_memcpy(meta, m, sizeof(*meta));
    bpf_map_delete_elem(&exit_compact_ctx, &pid_tgid);
    return meta->tag_id;
}

static __always_inline void fill_from_exit_compact_meta(struct event *e, u64 pid_tgid, struct exit_compact_meta *meta) {
    char comm[TASK_COMM_LEN];
    bpf_get_current_comm(&comm, sizeof(comm));
    fill_base_info(e, (u32)(pid_tgid >> 32), meta->tag_id, comm);
    e->type = meta->type;
    e->retval = 0;
    e->duration_ns = 0;
    e->extra1 = meta->extra1;
    e->extra2 = meta->extra2;
    e->extra3 = meta->extra3;
}

''' + consume_anchor
common = replace_once(common, consume_anchor, compact_helpers, 'compact consume/fill helpers')

for fn in ('execve', 'openat', 'mkdirat', 'unlinkat'):
    enter_marker = f'int tracepoint__syscalls__sys_enter_{fn}(struct trace_event_raw_sys_enter *ctx) {{'
    start = common.find(enter_marker)
    if start < 0: raise SystemExit(f'missing {fn} enter')
    end = common.find('\n}\n', start) + 3
    block = common[start:end]
    block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_compact_meta meta = {};', f'{fn} enter meta')
    block = replace_once(block, 'store_exit_meta(pid_tgid, &meta);', 'store_exit_compact_meta(pid_tgid, &meta);', f'{fn} enter store')
    common = common[:start] + block + common[end:]

    exit_marker = f'int tracepoint__syscalls__sys_exit_{fn}(struct trace_event_raw_sys_exit *ctx) {{'
    start = common.find(exit_marker)
    if start < 0: raise SystemExit(f'missing {fn} exit')
    end = common.find('\n}\n', start) + 3
    block = common[start:end]
    block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_compact_meta meta = {};', f'{fn} exit meta')
    block = replace_once(block, 'consume_exit_meta(pid_tgid, &meta)', 'consume_exit_compact_meta(pid_tgid, &meta)', f'{fn} exit consume')
    block = replace_once(block, 'fill_from_exit_meta(e, pid_tgid, &meta);', 'fill_from_exit_compact_meta(e, pid_tgid, &meta);', f'{fn} exit fill')
    common = common[:start] + block + common[end:]

common_path.write_text(common)

tail_path = Path('backend/ebpf/agent_tracker_tail.h')
tail = tail_path.read_text()
for fn in ('sys_enter_common_path', 'sys_enter_common_nopath'):
    marker = f'static __always_inline int {fn}('
    start = tail.find(marker)
    if start < 0: raise SystemExit(f'missing {fn}')
    end = tail.find('\n}\n', start) + 3
    block = tail[start:end]
    block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_compact_meta meta = {};', f'{fn} meta')
    block = replace_once(block, 'store_exit_meta(ptid, &meta);', 'store_exit_compact_meta(ptid, &meta);', f'{fn} store')
    tail = tail[:start] + block + tail[end:]

marker = 'static __always_inline void sys_exit_common('
start = tail.find(marker)
if start < 0: raise SystemExit('missing sys_exit_common')
end = tail.find('\n}\n', start) + 3
block = tail[start:end]
block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_compact_meta meta = {};', 'sys_exit_common meta')
block = replace_once(block, 'consume_exit_meta(pid_tgid, &meta)', 'consume_exit_compact_meta(pid_tgid, &meta)', 'sys_exit_common consume')
block = replace_once(block, 'fill_from_exit_meta(e, pid_tgid, &meta);', 'fill_from_exit_compact_meta(e, pid_tgid, &meta);', 'sys_exit_common fill')
tail = tail[:start] + block + tail[end:]
tail_path.write_text(tail)

sys_path = Path('backend/ebpf/agent_tracker_syscalls.h')
sys = sys_path.read_text()
old_simple_enter = '''    struct exit_meta meta = {.type = type_enum, .tag_id = tag_id}; \\
    store_exit_meta(pid_tgid, &meta); \\
'''
new_simple_enter = '''    struct exit_compact_meta meta = {.type = type_enum, .tag_id = tag_id}; \\
    store_exit_compact_meta(pid_tgid, &meta); \\
'''
sys = replace_once(sys, old_simple_enter, new_simple_enter, 'simple enter macro')

static_macro_anchor = '''#define DEFINE_STATIC_EXIT_HANDLER(name, path_str) \\
SEC("tracepoint/syscalls/sys_exit_" #name) \\
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \\
    u64 pid_tgid = bpf_get_current_pid_tgid(); \\
    struct exit_meta meta = {}; \\
    if (!consume_exit_meta(pid_tgid, &meta)) return 0; \\
    struct event *e = reserve_event(); \\
    if (!e) return 0; \\
    fill_from_exit_meta(e, pid_tgid, &meta); \\
    e->retval = ctx->ret; \\
    __builtin_memcpy(e->path, path_str, sizeof(path_str) - 1); \\
    submit_event(e); \\
    return 0; \\
}
'''
compact_static_macro = static_macro_anchor + '''
#define DEFINE_COMPACT_STATIC_EXIT_HANDLER(name, path_str) \\
SEC("tracepoint/syscalls/sys_exit_" #name) \\
int tracepoint__syscalls__sys_exit_##name(struct trace_event_raw_sys_exit *ctx) { \\
    u64 pid_tgid = bpf_get_current_pid_tgid(); \\
    struct exit_compact_meta meta = {}; \\
    if (!consume_exit_compact_meta(pid_tgid, &meta)) return 0; \\
    struct event *e = reserve_event(); \\
    if (!e) return 0; \\
    fill_from_exit_compact_meta(e, pid_tgid, &meta); \\
    e->retval = ctx->ret; \\
    __builtin_memcpy(e->path, path_str, sizeof(path_str) - 1); \\
    submit_event(e); \\
    return 0; \\
}
'''
sys = replace_once(sys, static_macro_anchor, compact_static_macro, 'compact static exit macro')

for name in ('ioctl','chmod','chown','mknod','clone','wait4','exit_group','open','rename','link','symlink'):
    old = f'DEFINE_STATIC_EXIT_HANDLER({name},'
    new = f'DEFINE_COMPACT_STATIC_EXIT_HANDLER({name},'
    sys = replace_once(sys, old, new, f'{name} compact static exit')

# socket() carries only scalar fd-creation metadata.
for fn in ('socket', 'close'):
    for direction in ('enter', 'exit'):
        sig = f'int tracepoint__syscalls__sys_{direction}_{fn}('
        start = sys.find(sig)
        if start < 0: raise SystemExit(f'missing {fn} {direction}')
        end = sys.find('\n}\n', start) + 3
        block = sys[start:end]
        if direction == 'enter':
            block = replace_once(block, 'struct exit_meta meta = {', 'struct exit_compact_meta meta = {', f'{fn} enter meta')
            block = replace_once(block, 'store_exit_meta(pid_tgid, &meta);', 'store_exit_compact_meta(pid_tgid, &meta);', f'{fn} enter store')
        else:
            block = replace_once(block, 'struct exit_meta meta = {};', 'struct exit_compact_meta meta = {};', f'{fn} exit meta')
            block = replace_once(block, 'consume_exit_meta(pid_tgid, &meta)', 'consume_exit_compact_meta(pid_tgid, &meta)', f'{fn} exit consume')
            if fn == 'socket':
                block = replace_once(block, 'fill_from_exit_meta(e, pid_tgid, &meta);', 'fill_from_exit_compact_meta(e, pid_tgid, &meta);', 'socket exit fill')
        sys = sys[:start] + block + sys[end:]

# dup/dup2/dup3 lineage needs only oldfd + tag/type.
marker = '#define DEFINE_DUP_HANDLER(name)'
start = sys.find(marker)
if start < 0: raise SystemExit('missing dup macro')
end = sys.find('\nDEFINE_DUP_HANDLER(dup)', start)
block = sys[start:end]
block = block.replace('struct exit_meta meta = {.type = TYPE_SOCKET, .tag_id = tag_id, .extra1 = (u32)ctx->args[0]};', 'struct exit_compact_meta meta = {.type = TYPE_SOCKET, .tag_id = tag_id, .extra1 = (u32)ctx->args[0]};')
block = block.replace('store_exit_meta(pid_tgid, &meta);', 'store_exit_compact_meta(pid_tgid, &meta);')
block = block.replace('struct exit_meta meta = {};', 'struct exit_compact_meta meta = {};')
block = block.replace('consume_exit_meta(pid_tgid, &meta)', 'consume_exit_compact_meta(pid_tgid, &meta)')
if 'exit_meta meta' in block or 'store_exit_meta' in block or 'consume_exit_meta' in block:
    raise SystemExit('dup macro still uses full exit context')
sys = sys[:start] + block + sys[end:]

sys_path.write_text(sys)

runtime_path = Path('backend/app/runtime_ebpf.go')
runtime = runtime_path.read_text()
runtime = replace_once(runtime, '"tracked_prefixes", "exit_ctx", "exit_single_path_buf",', '"tracked_prefixes", "exit_ctx", "exit_compact_ctx", "exit_single_path_buf",', 'runtime mapNames compact')
runtime = replace_once(runtime, '"tracked_prefixes": objs.TrackedPrefixes, "exit_ctx": objs.ExitCtx,', '"tracked_prefixes": objs.TrackedPrefixes, "exit_ctx": objs.ExitCtx, "exit_compact_ctx": objs.ExitCompactCtx,', 'runtime pin compact')
runtime_path.write_text(runtime)

doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
addition = '''\n### Compact enter/exit correlation\n\nGeneric filesystem/process syscalls and scalar descriptor lifecycle operations\nnow correlate through a 32-byte `exit_compact_ctx` value instead of the\nnetwork-capable 88-byte `exit_meta`. Socket creation, close, and dup lineage are\nalso eligible because they only carry scalar fd metadata. Network endpoint,\naccepted-peer, payload-pointer, and L7 capture paths continue to use the full\ncontext. This cuts hash-map value bandwidth by about 64% for the compact class\nwithout changing event ABI or enter/exit semantics.\n'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
