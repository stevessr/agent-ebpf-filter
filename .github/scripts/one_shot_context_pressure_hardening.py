from pathlib import Path


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:220]!r}")
    return text.replace(old, new, 1)

# Keep compact correlation + companion path state all-or-nothing.
tail_path = Path('backend/ebpf/agent_tracker_tail.h')
tail = tail_path.read_text()
tail = tail.replace(
    '    store_exit_compact_meta(ptid, &meta);\n    return 1;\n',
    '    return store_exit_compact_meta(ptid, &meta);\n',
)
old_single = '    store_exit_single_path(ptid, pd); \\\n    return 0; \\'
new_single = '    if (!store_exit_single_path(ptid, pd)) { bpf_map_delete_elem(&exit_compact_ctx, &ptid); return 0; } \\\n    return 0; \\'
if old_single not in tail:
    raise SystemExit('missing generic single-path companion store')
tail = tail.replace(old_single, new_single)
old_pair = '    store_exit_path_pair(ptid, pd); \\\n    return 0; \\'
new_pair = '    if (!store_exit_path_pair(ptid, pd)) { bpf_map_delete_elem(&exit_compact_ctx, &ptid); return 0; } \\\n    return 0; \\'
if old_pair not in tail:
    raise SystemExit('missing generic pair-path companion store')
tail = tail.replace(old_pair, new_pair)
tail_path.write_text(tail)

common_path = Path('backend/ebpf/agent_tracker_common.h')
common = common_path.read_text()
old_explicit = '''    store_exit_compact_meta(pid_tgid, &meta);
    store_exit_single_path(pid_tgid, pd);
    return 0;
'''
new_explicit = '''    if (!store_exit_compact_meta(pid_tgid, &meta)) return 0;
    if (!store_exit_single_path(pid_tgid, pd)) {
        bpf_map_delete_elem(&exit_compact_ctx, &pid_tgid);
        return 0;
    }
    return 0;
'''
count = common.count(old_explicit)
if count != 4:
    raise SystemExit(f'expected 4 explicit compact/path pairings, found {count}')
common = common.replace(old_explicit, new_explicit)
common_path.write_text(common)

# Dynamic HTTP path state is staged before full exit correlation. If the full
# map is under pressure, immediately remove that path entry instead of leaving
# an orphan until pid_tgid reuse/overwrite.
sys_path = Path('backend/ebpf/agent_tracker_syscalls.h')
sys = sys_path.read_text()
old_full_store = '''    store_exit_meta(pid_tgid, &meta);
    return 0;
'''
new_full_store = '''    if (!store_exit_meta(pid_tgid, &meta)) {
        if (meta.type == TYPE_SOCKET_HTTP) discard_dynamic_exit_path(pid_tgid);
        return 0;
    }
    return 0;
'''
count = sys.count(old_full_store)
if count < 5:
    raise SystemExit(f'expected multiple full correlation stores, found {count}')
sys = sys.replace(old_full_store, new_full_store)
# Full stores inside multiline macros use escaped newlines.
old_macro_store = '    store_exit_meta(pid_tgid, &meta); \\\n    return 0; \\'
new_macro_store = '    if (!store_exit_meta(pid_tgid, &meta)) { if (meta.type == TYPE_SOCKET_HTTP) discard_dynamic_exit_path(pid_tgid); return 0; } \\\n    return 0; \\'
sys = sys.replace(old_macro_store, new_macro_store)
sys_path.write_text(sys)

# Stats-map read failure must not masquerade as zero failures.
metrics_path = Path('backend/app/observability/metrics_collector.go')
metrics = metrics_path.read_text()
old_stats = '''\tvar totalStats kernelContextPressureStats
\tif statsMap := deps.TrackerMaps.GetContextPressureStats(); statsMap != nil {
\t\tcpuCount, err := ebpf.PossibleCPU()
\t\tif err == nil && cpuCount > 0 {
\t\t\tvalues := make([]kernelContextPressureStats, cpuCount)
\t\t\tkey := uint32(0)
\t\t\tif err := statsMap.Lookup(&key, &values); err == nil {
\t\t\t\ttotalStats = aggregateContextPressureStats(values)
\t\t\t}
\t\t}
\t}
\tfailures := contextFailureByMap(totalStats)
\tout := make(map[string]ContextMapPressure, len(maps))
\tavailable := true
'''
new_stats = '''\tvar totalStats kernelContextPressureStats
\tstatsAvailable := false
\tif statsMap := deps.TrackerMaps.GetContextPressureStats(); statsMap != nil {
\t\tcpuCount, err := ebpf.PossibleCPU()
\t\tif err == nil && cpuCount > 0 {
\t\t\tvalues := make([]kernelContextPressureStats, cpuCount)
\t\t\tkey := uint32(0)
\t\t\tif err := statsMap.Lookup(&key, &values); err == nil {
\t\t\t\ttotalStats = aggregateContextPressureStats(values)
\t\t\t\tstatsAvailable = true
\t\t\t}
\t\t}
\t}
\tfailures := contextFailureByMap(totalStats)
\tout := make(map[string]ContextMapPressure, len(maps))
\tavailable := statsAvailable
'''
metrics = replace_once(metrics, old_stats, new_stats, 'stats availability')
old_health = 'CaptureHealthy:                   !mapAvailable || (bpfStats.RingbufReserveFailedTotal == 0 && generationConsistent && raw.AgentSightCountersTotal["kernel_sequence_unexplained_missing_events"] == 0 && raw.AgentSightCountersTotal["kernel_sequence_out_of_order"] == 0 && contextUpdateFailures == 0),'
new_health = 'CaptureHealthy:                   !mapAvailable || (contextMapsAvailable && bpfStats.RingbufReserveFailedTotal == 0 && generationConsistent && raw.AgentSightCountersTotal["kernel_sequence_unexplained_missing_events"] == 0 && raw.AgentSightCountersTotal["kernel_sequence_out_of_order"] == 0 && contextUpdateFailures == 0),'
metrics = replace_once(metrics, old_health, new_health, 'capture health requires context availability')
metrics_path.write_text(metrics)

# Surface whether the pressure snapshot is complete.
prom_path = Path('backend/app/observability/metrics_prometheus.go')
prom = prom_path.read_text()
anchor = '\twritePrometheusHeader(&b, "agent_ebpf_context_map_entries", "gauge", "Current entries in transient eBPF correlation/provenance maps.")\n'
availability = '''\twritePrometheusHeader(&b, "agent_ebpf_context_maps_available", "gauge", "Whether transient eBPF map occupancy and update-failure telemetry are fully readable.")
\tif health.ContextMapsAvailable {
\t\twritePrometheusSample(&b, "agent_ebpf_context_maps_available", nil, 1)
\t} else {
\t\twritePrometheusSample(&b, "agent_ebpf_context_maps_available", nil, 0)
\t}
''' + anchor
prom = replace_once(prom, anchor, availability, 'Prometheus availability gauge')
prom_path.write_text(prom)

# Tests: zero deps must be explicitly unavailable and aggregate mapping remains deterministic.
test_path = Path('backend/app/observability/metrics_collector_test.go')
test = test_path.read_text()
addition = '''

func TestContextMapPressureUnavailableWithoutTrackerMaps(t *testing.T) {
\toldDeps := deps
\tdeps.TrackerMaps = nil
\tt.Cleanup(func() { deps = oldDeps })
\tpressure, available, failures := loadContextMapPressureSnapshot()
\tif available || failures != 0 || len(pressure) != 0 {
\t\tt.Fatalf("unexpected pressure snapshot without tracker maps: available=%v failures=%d pressure=%+v", available, failures, pressure)
\t}
}
'''
if addition not in test:
    test += addition
test_path.write_text(test)

# Docs clarify approximation and all-or-nothing pairing.
doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
addition = '''

Pressure snapshots are deliberately approximate while hot hash/LRU maps mutate;
iteration is bounded by each map's configured capacity. If the per-CPU failure
map cannot be read, the pressure snapshot is reported unavailable rather than
silently treating failures as zero. Path correlation is also all-or-nothing:
if compact/full correlation or its companion path update fails, the sibling
state is not left behind for a later pid/tgid reuse.
'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
