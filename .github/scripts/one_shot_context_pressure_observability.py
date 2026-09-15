from pathlib import Path
import re


def replace_once(text: str, old: str, new: str, label: str) -> str:
    if old not in text:
        raise SystemExit(f"missing anchor for {label}: {old[:220]!r}")
    return text.replace(old, new, 1)

# ---------------------------------------------------------------------------
# Kernel: failure-only counters and checked correlation-map stores.
# ---------------------------------------------------------------------------
common_path = Path('backend/ebpf/agent_tracker_common.h')
common = common_path.read_text()

collector_map = '''struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct collector_stats);
} collector_stats SEC(".maps");
'''
pressure = collector_map + '''
// Failure-only accounting for correlation/provenance maps. Successful updates
// pay only the existing map update plus a return-code branch; the per-CPU
// stats lookup happens exclusively on an actual failure.
struct context_pressure_stats {
    u64 exit_full_update_failures;
    u64 exit_compact_update_failures;
    u64 single_path_update_failures;
    u64 pair_path_update_failures;
    u64 socket_fd_update_failures;
    u64 socket_parent_update_failures;
};

_Static_assert(sizeof(struct context_pressure_stats) == 48, "context pressure stats ABI changed");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, u32);
    __type(value, struct context_pressure_stats);
} context_pressure_stats SEC(".maps");

#define CONTEXT_PRESSURE_EXIT_FULL     1
#define CONTEXT_PRESSURE_EXIT_COMPACT  2
#define CONTEXT_PRESSURE_SINGLE_PATH   3
#define CONTEXT_PRESSURE_PAIR_PATH     4
#define CONTEXT_PRESSURE_SOCKET_FD     5
#define CONTEXT_PRESSURE_SOCKET_PARENT 6

static __always_inline void record_context_update_failure(u32 kind) {
    u32 key = 0;
    struct context_pressure_stats *stats = bpf_map_lookup_elem(&context_pressure_stats, &key);
    if (!stats) return;
    if (kind == CONTEXT_PRESSURE_EXIT_FULL) stats->exit_full_update_failures++;
    else if (kind == CONTEXT_PRESSURE_EXIT_COMPACT) stats->exit_compact_update_failures++;
    else if (kind == CONTEXT_PRESSURE_SINGLE_PATH) stats->single_path_update_failures++;
    else if (kind == CONTEXT_PRESSURE_PAIR_PATH) stats->pair_path_update_failures++;
    else if (kind == CONTEXT_PRESSURE_SOCKET_FD) stats->socket_fd_update_failures++;
    else if (kind == CONTEXT_PRESSURE_SOCKET_PARENT) stats->socket_parent_update_failures++;
}
'''
common = replace_once(common, collector_map, pressure, 'context pressure stats map')

old_store = '''static __always_inline void store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);
}

static __always_inline void store_exit_compact_meta(u64 pid_tgid, struct exit_compact_meta *meta) {
    bpf_map_update_elem(&exit_compact_ctx, &pid_tgid, meta, BPF_ANY);
}
'''
new_store = '''static __always_inline int store_exit_meta(u64 pid_tgid, struct exit_meta *meta) {
    long rc = bpf_map_update_elem(&exit_ctx, &pid_tgid, meta, BPF_ANY);
    if (rc < 0) record_context_update_failure(CONTEXT_PRESSURE_EXIT_FULL);
    return rc == 0;
}

static __always_inline int store_exit_compact_meta(u64 pid_tgid, struct exit_compact_meta *meta) {
    long rc = bpf_map_update_elem(&exit_compact_ctx, &pid_tgid, meta, BPF_ANY);
    if (rc < 0) record_context_update_failure(CONTEXT_PRESSURE_EXIT_COMPACT);
    return rc == 0;
}
'''
common = replace_once(common, old_store, new_store, 'checked exit stores')

# Replace single-path stores before defining wrappers, so wrapper internals are untouched.
common = re.sub(r'bpf_map_update_elem\(&exit_single_path_ctx, &(pid_tgid), pd, BPF_ANY\);', r'store_exit_single_path(\1, pd);', common)

tail_path = Path('backend/ebpf/agent_tracker_tail.h')
tail = tail_path.read_text()
tail = re.sub(r'bpf_map_update_elem\(&exit_single_path_ctx, &(ptid), pd, BPF_ANY\);', r'store_exit_single_path(\1, pd);', tail)
tail = re.sub(r'bpf_map_update_elem\(&exit_path_ctx, &(ptid), pd, BPF_ANY\);', r'store_exit_path_pair(\1, pd);', tail)

sys_path = Path('backend/ebpf/agent_tracker_syscalls.h')
sys = sys_path.read_text()
sys = re.sub(r'bpf_map_update_elem\(&exit_path_ctx, &(pid_tgid), pd, BPF_ANY\);', r'store_exit_path_pair(\1, pd);', sys)

path_map_anchor = '''struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 1024);
    __type(key, u64);
    __type(value, struct exit_path_data);
} exit_path_ctx SEC(".maps");
'''
path_helpers = path_map_anchor + '''

static __always_inline int store_exit_single_path(u64 pid_tgid, struct exit_single_path_data *data) {
    long rc = bpf_map_update_elem(&exit_single_path_ctx, &pid_tgid, data, BPF_ANY);
    if (rc < 0) record_context_update_failure(CONTEXT_PRESSURE_SINGLE_PATH);
    return rc == 0;
}

static __always_inline int store_exit_path_pair(u64 pid_tgid, struct exit_path_data *data) {
    long rc = bpf_map_update_elem(&exit_path_ctx, &pid_tgid, data, BPF_ANY);
    if (rc < 0) record_context_update_failure(CONTEXT_PRESSURE_PAIR_PATH);
    return rc == 0;
}
'''
common = replace_once(common, path_map_anchor, path_helpers, 'checked path stores')

# Socket provenance stores: count only actual update failures.
common = common.replace(
    'bpf_map_update_elem(&socket_fds, &child_key, &value, BPF_ANY);',
    'if (bpf_map_update_elem(&socket_fds, &child_key, &value, BPF_ANY) < 0) record_context_update_failure(CONTEXT_PRESSURE_SOCKET_FD);',
)
common = common.replace(
    'bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY);',
    'if (bpf_map_update_elem(&socket_fds, &key, &value, BPF_ANY) < 0) record_context_update_failure(CONTEXT_PRESSURE_SOCKET_FD);',
)
common = common.replace(
    'bpf_map_update_elem(&socket_fds, &new_key, &value, BPF_ANY);',
    'if (bpf_map_update_elem(&socket_fds, &new_key, &value, BPF_ANY) < 0) record_context_update_failure(CONTEXT_PRESSURE_SOCKET_FD);',
)
common = common.replace(
    'bpf_map_update_elem(&socket_fd_parents, &child_pid, &parent_tgid, BPF_ANY);',
    'if (bpf_map_update_elem(&socket_fd_parents, &child_pid, &parent_tgid, BPF_ANY) < 0) record_context_update_failure(CONTEXT_PRESSURE_SOCKET_PARENT);',
)

common_path.write_text(common)
tail_path.write_text(tail)
sys_path.write_text(sys)

# ---------------------------------------------------------------------------
# Runtime: retain all relevant pinned handles; expose them to observability.
# ---------------------------------------------------------------------------
core_path = Path('backend/core/types.go')
core = core_path.read_text()
old_maps = '''type TrackerMapSet struct {
\tAgentPids       *ebpf.Map
\tTrackedComms    *ebpf.Map
\tTrackedPaths    *ebpf.Map
\tTrackedPrefixes *ebpf.Map
\tEvents          *ebpf.Map
\tCollectorStats  *ebpf.Map
\tSocketFds       *ebpf.Map
\tSocketFdParents *ebpf.Map
}
'''
new_maps = '''type TrackerMapSet struct {
\tAgentPids            *ebpf.Map
\tTrackedComms         *ebpf.Map
\tTrackedPaths         *ebpf.Map
\tTrackedPrefixes      *ebpf.Map
\tEvents               *ebpf.Map
\tCollectorStats       *ebpf.Map
\tContextPressureStats *ebpf.Map
\tExitCtx              *ebpf.Map
\tExitCompactCtx       *ebpf.Map
\tExitSinglePathBuf    *ebpf.Map
\tExitPathBuf          *ebpf.Map
\tExitSinglePathCtx    *ebpf.Map
\tExitPathCtx          *ebpf.Map
\tSocketFds            *ebpf.Map
\tSocketFdParents      *ebpf.Map
}
'''
core = replace_once(core, old_maps, new_maps, 'TrackerMapSet fields')
core_path.write_text(core)

runtime_path = Path('backend/app/runtime_ebpf.go')
runtime = runtime_path.read_text()
runtime = replace_once(
    runtime,
    '"events", "collector_stats", "tracked_comms"',
    '"events", "collector_stats", "context_pressure_stats", "tracked_comms"',
    'mapNames context stats',
)
runtime = replace_once(
    runtime,
    '\t\t"collector_stats": objs.CollectorStats,\n',
    '\t\t"collector_stats": objs.CollectorStats, "context_pressure_stats": objs.ContextPressureStats,\n',
    'pin context pressure stats',
)
old_to = '''\treturn trackerMapSet{
\t\tAgentPids:       maps["agent_pids"],
\t\tEvents:          maps["events"],
\t\tCollectorStats:  maps["collector_stats"],
\t\tTrackedComms:    maps["tracked_comms"],
\t\tTrackedPaths:    maps["tracked_paths"],
\t\tTrackedPrefixes: maps["tracked_prefixes"],
\t\tSocketFds:       maps["socket_fds"],
\t}, nil
'''
new_to = '''\treturn trackerMapSet{
\t\tAgentPids:            maps["agent_pids"],
\t\tEvents:               maps["events"],
\t\tCollectorStats:       maps["collector_stats"],
\t\tContextPressureStats: maps["context_pressure_stats"],
\t\tTrackedComms:         maps["tracked_comms"],
\t\tTrackedPaths:         maps["tracked_paths"],
\t\tTrackedPrefixes:      maps["tracked_prefixes"],
\t\tExitCtx:              maps["exit_ctx"],
\t\tExitCompactCtx:       maps["exit_compact_ctx"],
\t\tExitSinglePathBuf:    maps["exit_single_path_buf"],
\t\tExitPathBuf:          maps["exit_path_buf"],
\t\tExitSinglePathCtx:    maps["exit_single_path_ctx"],
\t\tExitPathCtx:          maps["exit_path_ctx"],
\t\tSocketFds:            maps["socket_fds"],
\t\tSocketFdParents:      maps["socket_fd_parents"],
\t}, nil
'''
runtime = replace_once(runtime, old_to, new_to, 'toTrackerMapSet complete handles')
old_close = '''\tfor _, mp := range []*(*ebpf.Map){&set.AgentPids, &set.Events, &set.CollectorStats, &set.TrackedComms, &set.TrackedPaths, &set.TrackedPrefixes} {
'''
new_close = '''\tfor _, mp := range []*(*ebpf.Map){
\t\t&set.AgentPids, &set.Events, &set.CollectorStats, &set.ContextPressureStats,
\t\t&set.TrackedComms, &set.TrackedPaths, &set.TrackedPrefixes,
\t\t&set.ExitCtx, &set.ExitCompactCtx, &set.ExitSinglePathBuf, &set.ExitPathBuf,
\t\t&set.ExitSinglePathCtx, &set.ExitPathCtx, &set.SocketFds, &set.SocketFdParents,
\t} {
'''
runtime = replace_once(runtime, old_close, new_close, 'close all tracker map handles')
runtime_path.write_text(runtime)

# ---------------------------------------------------------------------------
# Observability dependency adapter.
# ---------------------------------------------------------------------------
deps_path = Path('backend/app/observability/deps.go')
deps_text = deps_path.read_text()
deps_text = replace_once(
    deps_text,
    '''type TrackerMapSet interface {
\tGetCollectorStats() *ebpf.Map
}
''',
    '''type TrackerMapSet interface {
\tGetCollectorStats() *ebpf.Map
\tGetContextPressureStats() *ebpf.Map
\tGetContextMaps() map[string]*ebpf.Map
}
''',
    'observability tracker map interface',
)
deps_path.write_text(deps_text)

bridge_path = Path('backend/app/observabilitybridge.go')
bridge = bridge_path.read_text()
bridge_anchor = '''func (observabilityTrackerMapSet) GetCollectorStats() *ebpf.Map {
\treturn trackerMaps.CollectorStats
}
'''
bridge_new = bridge_anchor + '''
func (observabilityTrackerMapSet) GetContextPressureStats() *ebpf.Map {
\treturn trackerMaps.ContextPressureStats
}

func (observabilityTrackerMapSet) GetContextMaps() map[string]*ebpf.Map {
\treturn map[string]*ebpf.Map{
\t\t"exit_ctx":             trackerMaps.ExitCtx,
\t\t"exit_compact_ctx":     trackerMaps.ExitCompactCtx,
\t\t"exit_single_path_ctx": trackerMaps.ExitSinglePathCtx,
\t\t"exit_path_ctx":        trackerMaps.ExitPathCtx,
\t\t"socket_fds":           trackerMaps.SocketFds,
\t\t"socket_fd_parents":    trackerMaps.SocketFdParents,
\t}
}
'''
bridge = replace_once(bridge, bridge_anchor, bridge_new, 'observability context maps adapter')
bridge_path.write_text(bridge)

# ---------------------------------------------------------------------------
# Collector health: on-demand occupancy + failure aggregation.
# ---------------------------------------------------------------------------
metrics_path = Path('backend/app/observability/metrics_collector.go')
metrics = metrics_path.read_text()
insert_after_stats = '''type bpfCollectorStats struct {
\tRingbufEventsTotal        uint64
\tRingbufReserveFailedTotal uint64
\tEventSequence             uint64
\tPendingDroppedEvents      uint64
\tAuditGeneration           uint64
}
'''
pressure_types = insert_after_stats + '''

type kernelContextPressureStats struct {
\tExitFullUpdateFailures     uint64
\tExitCompactUpdateFailures  uint64
\tSinglePathUpdateFailures   uint64
\tPairPathUpdateFailures     uint64
\tSocketFDUpdateFailures     uint64
\tSocketParentUpdateFailures uint64
}

type ContextMapPressure struct {
\tEntries              uint32  `json:"entries"`
\tCapacity             uint32  `json:"capacity"`
\tKeySize              uint32  `json:"keySize"`
\tValueSize            uint32  `json:"valueSize"`
\tPayloadBytes         uint64  `json:"payloadBytes"`
\tCapacityPayloadBytes uint64  `json:"capacityPayloadBytes"`
\tUtilization          float64 `json:"utilization"`
\tUpdateFailuresTotal  uint64  `json:"updateFailuresTotal"`
}
'''
metrics = replace_once(metrics, insert_after_stats, pressure_types, 'context pressure health types')

health_anchor = '''\tKernelCaptureClockUnknown        uint64            `json:"kernelCaptureClockUnknownTotal"`
\tEventsByTypeTotal                map[string]uint64 `json:"eventsByTypeTotal"`
'''
health_new = '''\tKernelCaptureClockUnknown        uint64                        `json:"kernelCaptureClockUnknownTotal"`
\tContextMapsAvailable             bool                          `json:"contextMapsAvailable"`
\tContextMapPressure               map[string]ContextMapPressure `json:"contextMapPressure,omitempty"`
\tContextMapUpdateFailuresTotal    uint64                        `json:"contextMapUpdateFailuresTotal"`
\tEventsByTypeTotal                map[string]uint64             `json:"eventsByTypeTotal"`
'''
metrics = replace_once(metrics, health_anchor, health_new, 'health context fields')

snapshot_anchor = '''func (s *collectorMetricsState) snapshot() CollectorHealthResponse {
\tbpfStats, mapAvailable, generationConsistent := loadCollectorStatsSnapshot()
\traw := s.rawSnapshot()
'''
snapshot_new = '''func (s *collectorMetricsState) snapshot() CollectorHealthResponse {
\tbpfStats, mapAvailable, generationConsistent := loadCollectorStatsSnapshot()
\tcontextPressure, contextMapsAvailable, contextUpdateFailures := loadContextMapPressureSnapshot()
\traw := s.rawSnapshot()
'''
metrics = replace_once(metrics, snapshot_anchor, snapshot_new, 'snapshot context pressure load')

return_anchor = '''\t\tKernelCaptureClockUnknown:        raw.KernelCaptureClockUnknown,
\t\tEventsByTypeTotal:                eventsByType,
'''
return_new = '''\t\tKernelCaptureClockUnknown:        raw.KernelCaptureClockUnknown,
\t\tContextMapsAvailable:             contextMapsAvailable,
\t\tContextMapPressure:               contextPressure,
\t\tContextMapUpdateFailuresTotal:    contextUpdateFailures,
\t\tEventsByTypeTotal:                eventsByType,
'''
metrics = replace_once(metrics, return_anchor, return_new, 'snapshot return context pressure')
metrics = replace_once(
    metrics,
    'raw.AgentSightCountersTotal["kernel_sequence_out_of_order"] == 0),',
    'raw.AgentSightCountersTotal["kernel_sequence_out_of_order"] == 0 && contextUpdateFailures == 0),',
    'capture healthy context failures',
)

load_anchor = '''func loadCollectorStatsSnapshot() (bpfCollectorStats, bool, bool) {
'''
context_loader = '''func contextFailureByMap(stats kernelContextPressureStats) map[string]uint64 {
\treturn map[string]uint64{
\t\t"exit_ctx":             stats.ExitFullUpdateFailures,
\t\t"exit_compact_ctx":     stats.ExitCompactUpdateFailures,
\t\t"exit_single_path_ctx": stats.SinglePathUpdateFailures,
\t\t"exit_path_ctx":        stats.PairPathUpdateFailures,
\t\t"socket_fds":           stats.SocketFDUpdateFailures,
\t\t"socket_fd_parents":    stats.SocketParentUpdateFailures,
\t}
}

func aggregateContextPressureStats(values []kernelContextPressureStats) kernelContextPressureStats {
\tvar total kernelContextPressureStats
\tfor _, value := range values {
\t\ttotal.ExitFullUpdateFailures += value.ExitFullUpdateFailures
\t\ttotal.ExitCompactUpdateFailures += value.ExitCompactUpdateFailures
\t\ttotal.SinglePathUpdateFailures += value.SinglePathUpdateFailures
\t\ttotal.PairPathUpdateFailures += value.PairPathUpdateFailures
\t\ttotal.SocketFDUpdateFailures += value.SocketFDUpdateFailures
\t\ttotal.SocketParentUpdateFailures += value.SocketParentUpdateFailures
\t}
\treturn total
}

func countBPFMapEntries(m *ebpf.Map) (uint32, error) {
\tif m == nil {
\t\treturn 0, nil
\t}
\tvar count uint32
\tvar key []byte
\tfor {
\t\tnext, err := m.NextKeyBytes(key)
\t\tif err != nil {
\t\t\treturn count, err
\t\t}
\t\tif next == nil {
\t\t\treturn count, nil
\t\t}
\t\tcount++
\t\tif count >= m.MaxEntries() {
\t\t\t// Concurrent deletion can make hash iteration revisit keys. Clamp at
\t\t\t// capacity so health polling cannot spin indefinitely on a hot map.
\t\t\treturn count, nil
\t\t}
\t\tkey = next
\t}
}

func buildContextMapPressure(m *ebpf.Map, failures uint64) (ContextMapPressure, error) {
\tif m == nil {
\t\treturn ContextMapPressure{UpdateFailuresTotal: failures}, nil
\t}
\tentries, err := countBPFMapEntries(m)
\tif err != nil {
\t\treturn ContextMapPressure{}, err
\t}
\tcapacity := m.MaxEntries()
\tkeySize := m.KeySize()
\tvalueSize := m.ValueSize()
\tentryPayload := uint64(keySize) + uint64(valueSize)
\tpressure := ContextMapPressure{
\t\tEntries:              entries,
\t\tCapacity:             capacity,
\t\tKeySize:              keySize,
\t\tValueSize:            valueSize,
\t\tPayloadBytes:         uint64(entries) * entryPayload,
\t\tCapacityPayloadBytes: uint64(capacity) * entryPayload,
\t\tUpdateFailuresTotal:  failures,
\t}
\tif capacity != 0 {
\t\tpressure.Utilization = float64(entries) / float64(capacity)
\t}
\treturn pressure, nil
}

func loadContextMapPressureSnapshot() (map[string]ContextMapPressure, bool, uint64) {
\tif deps.TrackerMaps == nil {
\t\treturn map[string]ContextMapPressure{}, false, 0
\t}
\tmaps := deps.TrackerMaps.GetContextMaps()
\tif len(maps) == 0 {
\t\treturn map[string]ContextMapPressure{}, false, 0
\t}

\tvar totalStats kernelContextPressureStats
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
\tvar totalFailures uint64
\tfor name, m := range maps {
\t\tif m == nil {
\t\t\tavailable = false
\t\t\tcontinue
\t\t}
\t\tpressure, err := buildContextMapPressure(m, failures[name])
\t\tif err != nil {
\t\t\tavailable = false
\t\t\tcontinue
\t\t}
\t\tout[name] = pressure
\t\ttotalFailures += pressure.UpdateFailuresTotal
\t}
\treturn out, available, totalFailures
}

''' + load_anchor
metrics = replace_once(metrics, load_anchor, context_loader, 'context pressure loader')
metrics_path.write_text(metrics)

# Prometheus rendering.
prom_path = Path('backend/app/observability/metrics_prometheus.go')
prom = prom_path.read_text()
prom_anchor = '''\twritePrometheusHeader(&b, "agent_ebpf_backend_queue_len", "gauge", "Current backend event queue length.")
'''
prom_block = '''\twritePrometheusHeader(&b, "agent_ebpf_context_map_entries", "gauge", "Current entries in transient eBPF correlation/provenance maps.")
\twritePrometheusHeader(&b, "agent_ebpf_context_map_capacity", "gauge", "Maximum entries configured for transient eBPF correlation/provenance maps.")
\twritePrometheusHeader(&b, "agent_ebpf_context_map_utilization_ratio", "gauge", "Current transient eBPF map entry utilization ratio.")
\twritePrometheusHeader(&b, "agent_ebpf_context_map_payload_bytes", "gauge", "Current key+value payload bytes represented by transient eBPF map entries; excludes kernel hash overhead.")
\twritePrometheusHeader(&b, "agent_ebpf_context_map_capacity_payload_bytes", "gauge", "Configured key+value payload budget for transient eBPF maps; excludes kernel hash overhead.")
\twritePrometheusHeader(&b, "agent_ebpf_context_map_update_failures_total", "counter", "Kernel correlation/provenance map update failures, recorded only on the failure path.")
\tcontextMapNames := make([]string, 0, len(health.ContextMapPressure))
\tfor name := range health.ContextMapPressure {
\t\tcontextMapNames = append(contextMapNames, name)
\t}
\tsort.Strings(contextMapNames)
\tfor _, name := range contextMapNames {
\t\tpressure := health.ContextMapPressure[name]
\t\tlabels := map[string]string{"map": name}
\t\twritePrometheusSample(&b, "agent_ebpf_context_map_entries", labels, float64(pressure.Entries))
\t\twritePrometheusSample(&b, "agent_ebpf_context_map_capacity", labels, float64(pressure.Capacity))
\t\twritePrometheusSample(&b, "agent_ebpf_context_map_utilization_ratio", labels, pressure.Utilization)
\t\twritePrometheusSample(&b, "agent_ebpf_context_map_payload_bytes", labels, float64(pressure.PayloadBytes))
\t\twritePrometheusSample(&b, "agent_ebpf_context_map_capacity_payload_bytes", labels, float64(pressure.CapacityPayloadBytes))
\t\twritePrometheusSample(&b, "agent_ebpf_context_map_update_failures_total", labels, float64(pressure.UpdateFailuresTotal))
\t}
''' + prom_anchor
prom = replace_once(prom, prom_anchor, prom_block, 'Prometheus context pressure metrics')
prom_path.write_text(prom)

# Unit tests for pure aggregation/math; no privileged eBPF map creation required.
test_path = Path('backend/app/observability/metrics_collector_test.go')
test = test_path.read_text()
test += '''

func TestAggregateContextPressureStats(t *testing.T) {
\tgot := aggregateContextPressureStats([]kernelContextPressureStats{
\t\t{ExitFullUpdateFailures: 1, ExitCompactUpdateFailures: 2, SinglePathUpdateFailures: 3, PairPathUpdateFailures: 4, SocketFDUpdateFailures: 5, SocketParentUpdateFailures: 6},
\t\t{ExitFullUpdateFailures: 10, ExitCompactUpdateFailures: 20, SinglePathUpdateFailures: 30, PairPathUpdateFailures: 40, SocketFDUpdateFailures: 50, SocketParentUpdateFailures: 60},
\t})
\tif got.ExitFullUpdateFailures != 11 || got.ExitCompactUpdateFailures != 22 || got.SinglePathUpdateFailures != 33 || got.PairPathUpdateFailures != 44 || got.SocketFDUpdateFailures != 55 || got.SocketParentUpdateFailures != 66 {
\t\tt.Fatalf("unexpected context pressure aggregate: %+v", got)
\t}
\tfailures := contextFailureByMap(got)
\tif failures["exit_ctx"] != 11 || failures["exit_compact_ctx"] != 22 || failures["exit_single_path_ctx"] != 33 || failures["exit_path_ctx"] != 44 || failures["socket_fds"] != 55 || failures["socket_fd_parents"] != 66 {
\t\tt.Fatalf("unexpected context failure map: %+v", failures)
\t}
}
'''
test_path.write_text(test)

# Documentation.
doc_path = Path('docs/backend/generic-api-capture.md')
doc = doc_path.read_text()
addition = '''
### Correlation-map pressure observability

Transient enter/exit and socket-provenance maps now expose on-demand pressure
through collector health and Prometheus. The backend enumerates pinned maps only
when health/metrics are queried, so syscall hot paths do not pay an occupancy
counter update. Kernel-side per-CPU `context_pressure_stats` increments only when
a map update actually fails. Metrics include current entries, configured
capacity, key/value payload budget, utilization ratio, and cumulative update
failures for `exit_ctx`, `exit_compact_ctx`, single/pair path contexts,
`socket_fds`, and `socket_fd_parents`. Any observed update failure marks capture
health unhealthy because enter/exit or provenance correlation may be incomplete.
'''
if addition not in doc:
    doc += addition
doc_path.write_text(doc)
