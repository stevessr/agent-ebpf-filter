from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:160]!r}")
    return text.replace(old, new, 1)

common_path = Path("backend/ebpf/agent_tracker_common.h")
common = common_path.read_text()

old_buf = '''// Per-CPU buffer for exit_path_data (avoids 512-byte stack allocation)
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct exit_path_data);
} exit_path_buf SEC(".maps");
'''
new_buf = '''// Per-CPU scratch follows the same payload split as the exit correlation maps.
// Ordinary filesystem path capture touches only 256 bytes; dual paths and
// recognized HTTP start-lines retain the 512-byte pair scratch.
struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct exit_single_path_data);
} exit_single_path_buf SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct exit_path_data);
} exit_path_buf SEC(".maps");
'''
common = replace_once(common, old_buf, new_buf, "per-cpu path scratch split")

for fn in ["execve", "openat", "mkdirat", "unlinkat"]:
    marker = f'int tracepoint__syscalls__sys_enter_{fn}(struct trace_event_raw_sys_enter *ctx) {{'
    start = common.find(marker)
    if start < 0:
        raise SystemExit(f"missing {fn} enter handler")
    end = common.find('\n}\n', start)
    if end < 0:
        raise SystemExit(f"unterminated {fn} enter handler")
    end += 3
    block = common[start:end]
    old_lookup = 'struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);'
    if old_lookup not in block:
        raise SystemExit(f"{fn} no longer uses pair scratch as expected")
    block = block.replace(
        old_lookup,
        'struct exit_single_path_data *pd = bpf_map_lookup_elem(&exit_single_path_buf, &zero);',
        1,
    )
    old_update = 'bpf_map_update_elem(&exit_single_path_ctx, &pid_tgid, pd->path, BPF_ANY);'
    if old_update not in block:
        raise SystemExit(f"{fn} single-path update anchor missing")
    block = block.replace(
        old_update,
        'bpf_map_update_elem(&exit_single_path_ctx, &pid_tgid, pd, BPF_ANY);',
        1,
    )
    common = common[:start] + block + common[end:]

common_path.write_text(common)

tail_path = Path("backend/ebpf/agent_tracker_tail.h")
tail = tail_path.read_text()
for macro in ["SYS_PATH0", "SYS_PATH1", "SYS_PATH4"]:
    marker = f'#define {macro}(name, nr)'
    start = tail.find(marker)
    if start < 0:
        raise SystemExit(f"missing {macro}")
    next_macro = tail.find('\n// ── Macro:', start + len(marker))
    if next_macro < 0:
        next_macro = len(tail)
    block = tail[start:next_macro]
    old_lookup = 'struct exit_path_data *pd = bpf_map_lookup_elem(&exit_path_buf, &zero);'
    if old_lookup not in block:
        raise SystemExit(f"{macro} pair scratch anchor missing")
    block = block.replace(
        old_lookup,
        'struct exit_single_path_data *pd = bpf_map_lookup_elem(&exit_single_path_buf, &zero);',
        1,
    )
    old_update = 'bpf_map_update_elem(&exit_single_path_ctx, &ptid, pd->path, BPF_ANY);'
    if old_update not in block:
        raise SystemExit(f"{macro} update anchor missing")
    block = block.replace(
        old_update,
        'bpf_map_update_elem(&exit_single_path_ctx, &ptid, pd, BPF_ANY);',
        1,
    )
    tail = tail[:start] + block + tail[next_macro:]

tail_path.write_text(tail)

runtime_path = Path("backend/app/runtime_ebpf.go")
runtime = runtime_path.read_text()
runtime = replace_once(
    runtime,
    '"exit_ctx", "exit_path_buf", "exit_single_path_ctx", "exit_path_ctx",',
    '"exit_ctx", "exit_single_path_buf", "exit_path_buf", "exit_single_path_ctx", "exit_path_ctx",',
    "runtime mapNames",
)
runtime = replace_once(
    runtime,
    '"exit_path_buf": objs.ExitPathBuf, "exit_single_path_ctx": objs.ExitSinglePathCtx,',
    '"exit_single_path_buf": objs.ExitSinglePathBuf, "exit_path_buf": objs.ExitPathBuf, "exit_single_path_ctx": objs.ExitSinglePathCtx,',
    "runtime pinMaps",
)
runtime_path.write_text(runtime)

doc_path = Path("docs/backend/generic-api-capture.md")
doc = doc_path.read_text()
addition = '''\n### Single-path scratch fast path\n\nThe dynamic filesystem path fast path now uses a dedicated 256-byte per-CPU\n`exit_single_path_buf` before publishing into `exit_single_path_ctx`. Dual-path\noperations and recognized HTTP start-lines continue to use the 512-byte\n`exit_path_buf`. This keeps enter-time path snapshots stable while reducing the\ncache footprint touched by common one-path syscalls; it deliberately avoids\nre-reading user pointers on sys_exit, which could observe mutated or unmapped\npath memory.\n'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
