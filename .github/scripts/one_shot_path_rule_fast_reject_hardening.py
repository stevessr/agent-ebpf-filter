from pathlib import Path

def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:240]!r}")
    return text.replace(old, new, 1)

runtime_path = Path('backend/app/runtime_ebpf.go')
runtime = runtime_path.read_text()
runtime = replace_once(
    runtime,
    '\t"strings"\n',
    '\t"strings"\n\t"sync"\n',
    'sync import',
)

anchor = '''func syncTrackingModeFlags(set *trackerMapSet) error {
\tif set == nil {
\t\treturn errors.New("tracker map set is nil")
\t}
\treturn syncTrackingModeMap(set.TrackingMode, set.TrackedPaths, set.TrackedPrefixes)
}
'''
helpers = anchor + '''
var trackingModeMutationMu sync.Mutex

func forceTrackingModePathAny(mode *ebpf.Map) error {
\tif mode == nil {
\t\treturn errors.New("tracking mode map is nil")
\t}
\tflags := trackingModePathExact | trackingModePathPrefix
\tkey := uint32(0)
\tif err := mode.Update(&key, &flags, ebpf.UpdateAny); err != nil {
\t\treturn fmt.Errorf("force fail-open tracking mode: %w", err)
\t}
\treturn nil
}

func rollbackTrackedSelector(set *trackerMapSet, target *ebpf.Map, key any, hadPrevious bool, previous uint32, syncErr error) error {
\tvar rollbackErr error
\tif hadPrevious {
\t\trollbackErr = target.Put(key, previous)
\t} else if err := target.Delete(key); err != nil && !errors.Is(err, ebpf.ErrKeyNotExist) {
\t\trollbackErr = err
\t}
\t// A sync failure must never leave the kernel in a false-negative state.
\t// If exact reconstruction fails for any reason, force both path classes on;
\t// extra path work is preferable to suppressing a configured rule.
\tfailOpenErr := forceTrackingModePathAny(set.TrackingMode)
\tif rollbackErr != nil || failOpenErr != nil {
\t\treturn errors.Join(
\t\t\tfmt.Errorf("sync path tracking mode: %w", syncErr),
\t\t\tfmt.Errorf("rollback tracked selector: %w", rollbackErr),
\t\t\tfailOpenErr,
\t\t)
\t}
\treturn fmt.Errorf("sync path tracking mode: %w", syncErr)
}

func putTrackedSelector(set *trackerMapSet, target *ebpf.Map, key, value any) error {
\tif set == nil || target == nil {
\t\treturn errors.New("tracked selector maps are incomplete")
\t}
\ttrackingModeMutationMu.Lock()
\tdefer trackingModeMutationMu.Unlock()

\tvar previous uint32
\thadPrevious := false
\tif err := target.Lookup(key, &previous); err == nil {
\t\thadPrevious = true
\t} else if !errors.Is(err, ebpf.ErrKeyNotExist) {
\t\treturn fmt.Errorf("snapshot tracked selector: %w", err)
\t}
\tif err := target.Put(key, value); err != nil {
\t\treturn err
\t}
\tif err := syncTrackingModeFlags(set); err != nil {
\t\treturn rollbackTrackedSelector(set, target, key, hadPrevious, previous, err)
\t}
\treturn nil
}

func deleteTrackedSelector(set *trackerMapSet, target *ebpf.Map, key any) error {
\tif set == nil || target == nil {
\t\treturn errors.New("tracked selector maps are incomplete")
\t}
\ttrackingModeMutationMu.Lock()
\tdefer trackingModeMutationMu.Unlock()

\tvar previous uint32
\tif err := target.Lookup(key, &previous); err != nil {
\t\treturn err
\t}
\tif err := target.Delete(key); err != nil {
\t\treturn err
\t}
\tif err := syncTrackingModeFlags(set); err != nil {
\t\treturn rollbackTrackedSelector(set, target, key, true, previous, err)
\t}
\treturn nil
}
'''
runtime = replace_once(runtime, anchor, helpers, 'transactional tracking-mode helpers')
runtime_path.write_text(runtime)

bridge_path = Path('backend/app/handlersbridge.go')
bridge = bridge_path.read_text()
repls = {
'''func (a *handlerTrackerMapsAdapter) TrackedPathsPut(key, value any) error {
\tif err := a.set.TrackedPaths.Put(key, value); err != nil {
\t\treturn err
\t}
\treturn syncTrackingModeFlags(a.set)
}
''': '''func (a *handlerTrackerMapsAdapter) TrackedPathsPut(key, value any) error {
\treturn putTrackedSelector(a.set, a.set.TrackedPaths, key, value)
}
''',
'''func (a *handlerTrackerMapsAdapter) TrackedPathsDelete(key any) error {
\tif err := a.set.TrackedPaths.Delete(key); err != nil {
\t\treturn err
\t}
\treturn syncTrackingModeFlags(a.set)
}
''': '''func (a *handlerTrackerMapsAdapter) TrackedPathsDelete(key any) error {
\treturn deleteTrackedSelector(a.set, a.set.TrackedPaths, key)
}
''',
'''func (a *handlerTrackerMapsAdapter) TrackedPrefixesPut(key, value any) error {
\tif err := a.set.TrackedPrefixes.Put(key, value); err != nil {
\t\treturn err
\t}
\treturn syncTrackingModeFlags(a.set)
}
''': '''func (a *handlerTrackerMapsAdapter) TrackedPrefixesPut(key, value any) error {
\treturn putTrackedSelector(a.set, a.set.TrackedPrefixes, key, value)
}
''',
'''func (a *handlerTrackerMapsAdapter) TrackedPrefixesDelete(key any) error {
\tif err := a.set.TrackedPrefixes.Delete(key); err != nil {
\t\treturn err
\t}
\treturn syncTrackingModeFlags(a.set)
}
''': '''func (a *handlerTrackerMapsAdapter) TrackedPrefixesDelete(key any) error {
\treturn deleteTrackedSelector(a.set, a.set.TrackedPrefixes, key)
}
'''
}
for old, new in repls.items():
    bridge = replace_once(bridge, old, new, 'transactional handler selector mutation')
bridge_path.write_text(bridge)

mcp_path = Path('backend/app/server_mcp.go')
mcp = mcp_path.read_text()
old = '''\t\t\tif err := trackerMaps.TrackedPaths.Put(k, tid); err != nil {
\t\t\t\treturn nil, nil, fmt.Errorf("failed to add tracked path: %w", err)
\t\t\t}
\t\t\tif err := syncTrackingModeFlags(&trackerMaps); err != nil {
\t\t\t\t_ = trackerMaps.TrackedPaths.Delete(k)
\t\t\t\t_ = syncTrackingModeFlags(&trackerMaps)
\t\t\t\treturn nil, nil, fmt.Errorf("sync path tracking mode: %w", err)
\t\t\t}
'''
new = '''\t\t\tif err := putTrackedSelector(&trackerMaps, trackerMaps.TrackedPaths, k, tid); err != nil {
\t\t\t\treturn nil, nil, fmt.Errorf("failed to add tracked path: %w", err)
\t\t\t}
'''
mcp = replace_once(mcp, old, new, 'MCP transactional tracked path mutation')
mcp_path.write_text(mcp)

doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
addition = '''

Path/prefix registry mutations are serialized with mode publication. The backend
snapshots an existing selector before mutation and rolls it back if mode
publication fails. If rollback or precise re-synchronization cannot be trusted,
the mode map is forced to both path classes enabled, preserving capture
correctness at the cost of extra path work rather than allowing a false reject.
'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
