from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:240]!r}")
    return text.replace(old, new, 1)

# ---------------------------------------------------------------------------
# Kernel tracking mode + split PID/comm vs path matching.
# ---------------------------------------------------------------------------
common_path = Path('backend/ebpf/agent_tracker_common.h')
common = common_path.read_text()

prefix_map = '''struct {
    __uint(type, BPF_MAP_TYPE_LPM_TRIE);
    __uint(max_entries, 256);
    __uint(map_flags, BPF_F_NO_PREALLOC);
    __type(key, struct lpm_key);
    __type(value, u32);
} tracked_prefixes SEC(".maps");
'''
mode_map = prefix_map + '''

#define TRACKING_MODE_PATH_EXACT  (1U << 0)
#define TRACKING_MODE_PATH_PREFIX (1U << 1)
#define TRACKING_MODE_PATH_ANY    (TRACKING_MODE_PATH_EXACT | TRACKING_MODE_PATH_PREFIX)

// Userspace maintains these bits from the actual tracked path maps. The array
// lookup is performed only after PID/comm misses on path-bearing syscalls. If
// lookup ever fails, fail open to full path matching so optimization can never
// suppress configured path tracking.
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, u32);
} tracking_mode SEC(".maps");

static __always_inline u32 tracking_path_mode(void) {
    u32 key = 0;
    u32 *flags = bpf_map_lookup_elem(&tracking_mode, &key);
    return flags ? *flags : TRACKING_MODE_PATH_ANY;
}
'''
common = replace_once(common, prefix_map, mode_map, 'tracking mode map')

old_get = '''static __always_inline u32 get_tag_id(u32 pid, char *comm, char *path) {
    u32 *tag = bpf_map_lookup_elem(&agent_pids, &pid);
    if (tag) return *tag;
    tag = bpf_map_lookup_elem(&tracked_comms, comm);
    if (tag) return *tag;
    if (path) {
        tag = bpf_map_lookup_elem(&tracked_paths, path);
        if (tag) return *tag;

        // LPM trie prefix match
        u32 path_len = 0;
        #pragma unroll
        for (path_len = 0; path_len < LPM_PATH_LEN; path_len++) {
            if (path[path_len] == '\\0') break;
        }
        if (path_len > 0) {
            struct lpm_key lpmk = {};
            lpmk.prefix_len = path_len * 8;
            __builtin_memcpy(lpmk.data, path, LPM_PATH_LEN);
            tag = bpf_map_lookup_elem(&tracked_prefixes, &lpmk);
            if (tag) return *tag;
        }
    }
    return 0;
}
'''
new_get = '''static __always_inline u32 get_pid_comm_tag_id(u32 pid, char *comm) {
    u32 *tag = bpf_map_lookup_elem(&agent_pids, &pid);
    if (tag) return *tag;
    tag = bpf_map_lookup_elem(&tracked_comms, comm);
    return tag ? *tag : 0;
}

static __always_inline u32 get_path_tag_id(char *path, u32 path_flags) {
    if (!path) return 0;
    u32 *tag = 0;
    if (path_flags & TRACKING_MODE_PATH_EXACT) {
        tag = bpf_map_lookup_elem(&tracked_paths, path);
        if (tag) return *tag;
    }
    if (!(path_flags & TRACKING_MODE_PATH_PREFIX)) return 0;

    u32 path_len = 0;
#pragma unroll
    for (path_len = 0; path_len < LPM_PATH_LEN; path_len++) {
        if (path[path_len] == '\\0') break;
    }
    if (path_len == 0) return 0;

    struct lpm_key lpmk = {};
    lpmk.prefix_len = path_len * 8;
    __builtin_memcpy(lpmk.data, path, LPM_PATH_LEN);
    tag = bpf_map_lookup_elem(&tracked_prefixes, &lpmk);
    return tag ? *tag : 0;
}

// Resolve the cheap PID/comm selectors before any userspace path read. The
// caller only reads a path after a miss when path rules actually exist.
static __always_inline u32 get_tag_id_pre_path(u32 pid, char *comm, u32 *path_flags) {
    u32 tag_id = get_pid_comm_tag_id(pid, comm);
    if (tag_id) return tag_id;
    if (path_flags) *path_flags = tracking_path_mode();
    return 0;
}

static __always_inline u32 get_tag_id(u32 pid, char *comm, char *path) {
    u32 tag_id = get_pid_comm_tag_id(pid, comm);
    if (tag_id || !path) return tag_id;
    return get_path_tag_id(path, tracking_path_mode());
}
'''
common = replace_once(common, old_get, new_get, 'split tag resolution')

# Explicit single-path syscall handlers: pre-resolve before scratch/probe.
for fn, arg_idx in [('execve',0), ('openat',1), ('mkdirat',1), ('unlinkat',1)]:
    marker = f'int tracepoint__syscalls__sys_enter_{fn}(struct trace_event_raw_sys_enter *ctx) {{'
    start = common.find(marker)
    if start < 0: raise SystemExit(f'missing {fn} enter')
    end = common.find('\n}\n', start) + 3
    block = common[start:end]
    scratch_anchor = '''    u32 zero = 0;
    struct exit_single_path_data *pd = bpf_map_lookup_elem(&exit_single_path_buf, &zero);
'''
    pre = '''    u32 path_flags = 0;
    u32 tag_id = get_tag_id_pre_path(pid, comm, &path_flags);
    if (tag_id == 0 && !(path_flags & TRACKING_MODE_PATH_ANY)) return 0;

''' + scratch_anchor
    block = replace_once(block, scratch_anchor, pre, f'{fn} pre-path reject')
    old_tag = '''    u32 tag_id = get_tag_id(pid, comm, pd->path);
    if (tag_id == 0) return 0;
'''
    new_tag = '''    if (tag_id == 0) tag_id = get_path_tag_id(pd->path, path_flags);
    if (tag_id == 0) return 0;
'''
    block = replace_once(block, old_tag, new_tag, f'{fn} post-probe path resolve')
    common = common[:start] + block + common[end:]
common_path.write_text(common)

# Generic path macros use the same two-stage resolution.
tail_path = Path('backend/ebpf/agent_tracker_tail.h')
tail = tail_path.read_text()
old_common_path = '''static __always_inline int sys_enter_common_path(u64 ptid, char *comm, char *path, u32 nr, u32 extra2, u32 extra3) {
    u32 pid = (u32)(ptid >> 32);
    u32 tag_id = get_tag_id(pid, comm, path);
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
new_common_path = '''static __always_inline int sys_enter_common_resolved(u64 ptid, u32 tag_id, u32 nr, u32 extra2, u32 extra3) {
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

static __always_inline int sys_enter_common_path(u64 ptid, char *comm, char *path, u32 nr, u32 extra2, u32 extra3) {
    return sys_enter_common_resolved(ptid, get_tag_id((u32)(ptid >> 32), comm, path), nr, extra2, extra3);
}
'''
tail = replace_once(tail, old_common_path, new_common_path, 'resolved generic enter helper')

# Rewrite each path macro block structurally.
for macro in ['SYS_PATH0','SYS_PATH01','SYS_PATH1','SYS_PATH13','SYS_PATH02','SYS_PATH4']:
    marker = f'#define {macro}(name, nr)'
    start = tail.find(marker)
    if start < 0: raise SystemExit(f'missing {macro}')
    next_marker = tail.find('\n// ── Macro:', start + len(marker))
    if next_marker < 0: next_marker = len(tail)
    block = tail[start:next_marker]
    scratch_token = '    u32 zero = 0; \\\n'
    if scratch_token not in block: raise SystemExit(f'{macro} scratch anchor missing')
    pre = '''    u32 path_flags = 0; \\
    u32 tag_id = get_tag_id_pre_path((u32)(ptid >> 32), comm, &path_flags); \\
    if (tag_id == 0 && !(path_flags & TRACKING_MODE_PATH_ANY)) return 0; \\
''' + scratch_token
    block = block.replace(scratch_token, pre, 1)
    old_call = '    if (!sys_enter_common_path(ptid, comm, pd->path, nr, 0, 0)) return 0; \\\n'
    if old_call not in block: raise SystemExit(f'{macro} common path call missing')
    new_call = '''    if (tag_id == 0) tag_id = get_path_tag_id(pd->path, path_flags); \\
    if (!sys_enter_common_resolved(ptid, tag_id, nr, 0, 0)) return 0; \\
'''
    block = block.replace(old_call, new_call, 1)
    tail = tail[:start] + block + tail[next_marker:]
tail_path.write_text(tail)

# ---------------------------------------------------------------------------
# Runtime: pinned mode map, safe old-map migration, and sync helpers.
# ---------------------------------------------------------------------------
core_path = Path('backend/core/types.go')
core = core_path.read_text()
core = replace_once(core, '\tTrackedPrefixes      *ebpf.Map\n', '\tTrackedPrefixes      *ebpf.Map\n\tTrackingMode         *ebpf.Map\n', 'TrackerMapSet tracking mode')
core_path.write_text(core)

runtime_path = Path('backend/app/runtime_ebpf.go')
runtime = runtime_path.read_text()
runtime = replace_once(runtime, '"tracked_comms", "tracked_paths", "tracked_prefixes",', '"tracked_comms", "tracked_paths", "tracked_prefixes", "tracking_mode",', 'mapNames tracking mode')
runtime = replace_once(runtime, '"tracked_prefixes": objs.TrackedPrefixes, "exit_ctx": objs.ExitCtx,', '"tracked_prefixes": objs.TrackedPrefixes, "tracking_mode": objs.TrackingMode, "exit_ctx": objs.ExitCtx,', 'pin tracking mode')
runtime = replace_once(runtime, '\t\tTrackedPrefixes:      maps["tracked_prefixes"],\n', '\t\tTrackedPrefixes:      maps["tracked_prefixes"],\n\t\tTrackingMode:         maps["tracking_mode"],\n', 'toTrackerMapSet tracking mode')
runtime = replace_once(runtime, '&set.TrackedComms, &set.TrackedPaths, &set.TrackedPrefixes,', '&set.TrackedComms, &set.TrackedPaths, &set.TrackedPrefixes, &set.TrackingMode,', 'close tracking mode')

# Insert mode helpers before bootstrap mode section.
mode_anchor = '// ── mode detection ────────────────────────────────────────────────────────────\n'
mode_helpers = '''const (
\ttrackingModePathExact uint32 = 1 << iota
\ttrackingModePathPrefix
)

func trackingModeFlags(hasExact, hasPrefix bool) uint32 {
\tvar flags uint32
\tif hasExact {
\t\tflags |= trackingModePathExact
\t}
\tif hasPrefix {
\t\tflags |= trackingModePathPrefix
\t}
\treturn flags
}

func mapHasEntries(m *ebpf.Map) (bool, error) {
\tif m == nil {
\t\treturn false, errors.New("map is nil")
\t}
\tkey, err := m.NextKeyBytes(nil)
\tif err != nil {
\t\tif errors.Is(err, ebpf.ErrKeyNotExist) {
\t\t\treturn false, nil
\t\t}
\t\treturn false, err
\t}
\treturn len(key) != 0, nil
}

func syncTrackingModeMap(mode, paths, prefixes *ebpf.Map) error {
\tif mode == nil || paths == nil || prefixes == nil {
\t\treturn errors.New("tracking mode maps are incomplete")
\t}
\thasExact, err := mapHasEntries(paths)
\tif err != nil {
\t\treturn fmt.Errorf("inspect exact path tracking map: %w", err)
\t}
\thasPrefix, err := mapHasEntries(prefixes)
\tif err != nil {
\t\treturn fmt.Errorf("inspect prefix path tracking map: %w", err)
\t}
\tflags := trackingModeFlags(hasExact, hasPrefix)
\tkey := uint32(0)
\tif err := mode.Update(&key, &flags, ebpf.UpdateAny); err != nil {
\t\treturn fmt.Errorf("update tracking mode flags: %w", err)
\t}
\treturn nil
}

func syncTrackingModeFlags(set *trackerMapSet) error {
\tif set == nil {
\t\treturn errors.New("tracker map set is nil")
\t}
\treturn syncTrackingModeMap(set.TrackingMode, set.TrackedPaths, set.TrackedPrefixes)
}

''' + mode_anchor
runtime = replace_once(runtime, mode_anchor, mode_helpers, 'tracking mode helpers')

# Replace doBootstrap with a migration-safe variant.
start = runtime.index('func doBootstrap() (map[string]*ebpf.Map, error) {')
end = runtime.index('\nfunc newKernelAuditGeneration()', start)
old_bootstrap = runtime[start:end]
new_bootstrap = '''func doBootstrap() (map[string]*ebpf.Map, error) {
\tvar backup *trackedDataBackup
\tif replacements, err := loadPinnedMapHandles(); err == nil {
\t\tvar objs bpf.AgentTrackerObjects
\t\tif err := bpf.LoadAgentTrackerObjects(&objs, &ebpf.CollectionOptions{MapReplacements: replacements}); err == nil {
\t\t\tdefer objs.Close()
\t\t\t_ = os.RemoveAll(ebpfPinLinksDir)
\t\t\t_ = os.MkdirAll(ebpfPinLinksDir, 0755)
\t\t\tgeneration, err := rotateKernelAuditGeneration(objs.CollectorStats)
\t\t\tif err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\tif err := clearSocketFDProvenance(objs.SocketFds, objs.SocketFdParents); err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\tif err := syncTrackingModeMap(objs.TrackingMode, objs.TrackedPaths, objs.TrackedPrefixes); err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\tif err := pinLinks(&objs); err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\tif err := ensurePinnedMapPermissions(); err != nil {
\t\t\t\tplatform.CloseMapHandles(replacements)
\t\t\t\treturn nil, err
\t\t\t}
\t\t\tlog.Printf("[INFO] kernel audit generation rotated: %016x", generation)
\t\t\treturn replacements, nil
\t\t}
\t\tbackup = extractTrackedData(replacements)
\t\tplatform.CloseMapHandles(replacements)
\t} else {
\t\t// Adding a required pinned map makes a previous installation fail the
\t\t// all-maps load. Preserve the three user-owned tracking maps before the
\t\t// fresh bootstrap removes the old pin tree.
\t\tbackup = extractTrackedDataBestEffort()
\t}

\t_ = os.RemoveAll(ebpfPinRoot)
\tfor _, d := range []string{ebpfPinMapsDir, ebpfPinLinksDir} {
\t\t_ = os.MkdirAll(d, 0755)
\t}
\tvar objs bpf.AgentTrackerObjects
\tif err := bpf.LoadAgentTrackerObjects(&objs, nil); err != nil {
\t\treturn nil, fmt.Errorf("load eBPF objects: %w", err)
\t}
\tdefer objs.Close()
\tgeneration, err := rotateKernelAuditGeneration(objs.CollectorStats)
\tif err != nil {
\t\treturn nil, err
\t}
\tif err := pinMaps(&objs); err != nil {
\t\treturn nil, err
\t}
\tif err := restoreTrackedDataToMaps(backup, objs.TrackedComms, objs.TrackedPaths, objs.TrackedPrefixes); err != nil {
\t\treturn nil, err
\t}
\tif err := syncTrackingModeMap(objs.TrackingMode, objs.TrackedPaths, objs.TrackedPrefixes); err != nil {
\t\treturn nil, err
\t}
\t// Attach only after restored rules and mode flags agree. This avoids a
\t// reload window where path rules exist but the pre-path gate says none do.
\tif err := pinLinks(&objs); err != nil {
\t\treturn nil, err
\t}
\tif err := ensurePinnedMapPermissions(); err != nil {
\t\treturn nil, err
\t}
\tlog.Printf("[INFO] kernel audit generation initialized: %016x", generation)
\treturn loadPinnedMapHandles()
}
'''
runtime = runtime[:start] + new_bootstrap + runtime[end:]

# Service-mode safety: recompute after handles are loaded too.
old_loaded = '''\tloaded, err := toTrackerMapSet(maps)
\tif err != nil {
\t\tplatform.CloseMapHandles(maps)
\t\treturn err
\t}

\tcloseTrackerMapSet(&trackerMaps)
'''
new_loaded = '''\tloaded, err := toTrackerMapSet(maps)
\tif err != nil {
\t\tplatform.CloseMapHandles(maps)
\t\treturn err
\t}
\tif err := syncTrackingModeFlags(&loaded); err != nil {
\t\tcloseTrackerMapSet(&loaded)
\t\treturn fmt.Errorf("sync path tracking mode: %w", err)
\t}

\tcloseTrackerMapSet(&trackerMaps)
'''
runtime = replace_once(runtime, old_loaded, new_loaded, 'service mode tracking sync')

# Replace legacy restore function with reusable direct restore + best-effort old-map backup.
restore_start = runtime.index('func restoreTrackedData(backup *trackedDataBackup) {')
restore_end = runtime.index('\n// ── shared helpers', restore_start)
new_restore = '''func restoreTrackedDataToMaps(backup *trackedDataBackup, comms, paths, prefixes *ebpf.Map) error {
\tif backup == nil {
\t\treturn nil
\t}
\tif comms == nil || paths == nil || prefixes == nil {
\t\treturn errors.New("tracked map restore target is incomplete")
\t}
\tfor k, v := range backup.Comms {
\t\tif err := comms.Put(k, v); err != nil {
\t\t\treturn fmt.Errorf("restore tracked comm: %w", err)
\t\t}
\t}
\tfor k, v := range backup.Paths {
\t\tif err := paths.Put(k, v); err != nil {
\t\t\treturn fmt.Errorf("restore tracked path: %w", err)
\t\t}
\t}
\tfor _, entry := range backup.Prefixes {
\t\tk := struct {
\t\t\tPrefixLen uint32
\t\t\tData      [64]byte
\t\t}{PrefixLen: entry.PrefixLen, Data: entry.Data}
\t\tif err := prefixes.Put(k, entry.Value); err != nil {
\t\t\treturn fmt.Errorf("restore tracked prefix: %w", err)
\t\t}
\t}
\treturn nil
}

func extractTrackedDataBestEffort() *trackedDataBackup {
\tmaps := make(map[string]*ebpf.Map, 3)
\tfor _, name := range []string{"tracked_comms", "tracked_paths", "tracked_prefixes"} {
\t\tm, err := ebpf.LoadPinnedMap(filepath.Join(ebpfPinMapsDir, name), nil)
\t\tif err == nil {
\t\t\tmaps[name] = m
\t\t}
\t}
\tif len(maps) == 0 {
\t\treturn nil
\t}
\tdefer platform.CloseMapHandles(maps)
\treturn extractTrackedData(maps)
}
'''
runtime = runtime[:restore_start] + new_restore + runtime[restore_end:]
runtime_path.write_text(runtime)

# ---------------------------------------------------------------------------
# Mutation adapters and direct MCP path mutation keep mode current.
# ---------------------------------------------------------------------------
bridge_path = Path('backend/app/handlersbridge.go')
bridge = bridge_path.read_text()
for method, field in [('TrackedPathsPut','TrackedPaths'), ('TrackedPathsDelete','TrackedPaths'), ('TrackedPrefixesPut','TrackedPrefixes'), ('TrackedPrefixesDelete','TrackedPrefixes')]:
    verb = 'Put' if method.endswith('Put') else 'Delete'
    args = 'key, value' if verb == 'Put' else 'key'
    old = f'''func (a *handlerTrackerMapsAdapter) {method}(key''' + (', value any' if verb == 'Put' else ' any') + f''') error {{
\treturn a.set.{field}.{verb}({args})
}}
'''
    new = f'''func (a *handlerTrackerMapsAdapter) {method}(key''' + (', value any' if verb == 'Put' else ' any') + f''') error {{
\tif err := a.set.{field}.{verb}({args}); err != nil {{
\t\treturn err
\t}}
\treturn syncTrackingModeFlags(a.set)
}}
'''
    bridge = replace_once(bridge, old, new, method)
bridge_path.write_text(bridge)

mcp_path = Path('backend/app/server_mcp.go')
mcp = mcp_path.read_text()
old_mcp_path = '''\t\t\tif err := trackerMaps.TrackedPaths.Put(k, tid); err != nil {
\t\t\t\treturn nil, nil, fmt.Errorf("failed to add tracked path: %w", err)
\t\t\t}
\t\t\treturn nil, map[string]any{"success": true, "path": args.Path, "tag": args.Tag}, nil
'''
new_mcp_path = '''\t\t\tif err := trackerMaps.TrackedPaths.Put(k, tid); err != nil {
\t\t\t\treturn nil, nil, fmt.Errorf("failed to add tracked path: %w", err)
\t\t\t}
\t\t\tif err := syncTrackingModeFlags(&trackerMaps); err != nil {
\t\t\t\t_ = trackerMaps.TrackedPaths.Delete(k)
\t\t\t\t_ = syncTrackingModeFlags(&trackerMaps)
\t\t\t\treturn nil, nil, fmt.Errorf("sync path tracking mode: %w", err)
\t\t\t}
\t\t\treturn nil, map[string]any{"success": true, "path": args.Path, "tag": args.Tag}, nil
'''
mcp = replace_once(mcp, old_mcp_path, new_mcp_path, 'MCP path mode sync')
mcp_path.write_text(mcp)

# ---------------------------------------------------------------------------
# Pure unit test for flags + docs.
# ---------------------------------------------------------------------------
test_path = Path('backend/app/runtime_tracking_mode_test.go')
test_path.write_text('''package app

import "testing"

func TestTrackingModeFlags(t *testing.T) {
\ttests := []struct {
\t\texact, prefix bool
\t\twant          uint32
\t}{
\t\t{false, false, 0},
\t\t{true, false, trackingModePathExact},
\t\t{false, true, trackingModePathPrefix},
\t\t{true, true, trackingModePathExact | trackingModePathPrefix},
\t}
\tfor _, tt := range tests {
\t\tif got := trackingModeFlags(tt.exact, tt.prefix); got != tt.want {
\t\t\tt.Fatalf("trackingModeFlags(%v,%v)=%d want %d", tt.exact, tt.prefix, got, tt.want)
\t\t}
\t}
}
''')

doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
addition = '''
### Path-rule fast reject

Path-bearing syscall tracepoints now split cheap PID/comm matching from expensive
user-path inspection. A tiny pinned `tracking_mode` ARRAY records whether exact
or prefix path rules currently exist. After PID/comm misses, if neither class is
configured the program returns before per-CPU path scratch lookup,
`bpf_probe_read_user_str`, exact-path hash lookup, or LPM prefix matching.
Tracked PID/comm events still snapshot and report their path, and configured
path rules retain exact/prefix semantics. Mode lookup failure is fail-open to
full path matching.

The backend derives mode bits from the real tracked path maps, synchronizes them
before tracepoint links are attached, and refreshes them after official path
configuration mutations. Fresh bootstrap also performs best-effort backup of
legacy tracked maps before replacing an older pin layout, restores rules before
attach, then publishes the matching mode bits, preventing both config loss and
reload-time false rejects when a new required map is introduced.
'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
