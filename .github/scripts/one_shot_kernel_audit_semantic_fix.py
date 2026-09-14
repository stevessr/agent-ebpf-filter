from pathlib import Path


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    if text.count(old) != 1:
        raise SystemExit(f"{path}: expected one match, got {text.count(old)}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "backend/app/runtime_ebpf.go",
    '''\tfor i := range values {
\t\t// Keep cumulative successful/reserve-failure counters across a compatible
\t\t// program reload, but start sequence continuity in an explicit new epoch.
\t\tvalues[i].EventSequence = 0
\t\tvalues[i].PendingDroppedEvents = 0
\t\tvalues[i].AuditGeneration = generation
\t}
''',
    '''\tfor i := range values {
\t\t// Keep cumulative successful/reserve-failure/attempt counters across a
\t\t// compatible reload. The generation itself is the continuity epoch, so
\t\t// the CPU-local attempt sequence can remain monotonic across reloads.
\t\tvalues[i].PendingDroppedEvents = 0
\t\tvalues[i].AuditGeneration = generation
\t}
''',
)

replace_once(
    "backend/app/runtime_ebpf.go",
    '''\tcpuCount, err := ebpf.PossibleCPU()
\tif err != nil || cpuCount <= 0 {
\t\treturn 0, fmt.Errorf("discover possible CPUs for audit generation: %w", err)
\t}
''',
    '''\tcpuCount, err := ebpf.PossibleCPU()
\tif err != nil {
\t\treturn 0, fmt.Errorf("discover possible CPUs for audit generation: %w", err)
\t}
\tif cpuCount <= 0 {
\t\treturn 0, errors.New("discover possible CPUs for audit generation: no CPUs reported")
\t}
''',
)

replace_once(
    "backend/app/runtime_ebpf_audit_test.go",
    '''\tfor i, value := range values {
\t\tif value.EventSequence != 0 || value.PendingDroppedEvents != 0 || value.AuditGeneration != 99 {
\t\t\tt.Fatalf("slot %d continuity state = %+v", i, value)
\t\t}
\t}
\tif values[0].RingbufEventsTotal != 10 || values[0].RingbufReserveFailedTotal != 3 || values[1].RingbufEventsTotal != 20 || values[1].RingbufReserveFailedTotal != 4 {
\t\tt.Fatalf("cumulative counters were not preserved: %+v", values)
\t}
''',
    '''\tfor i, value := range values {
\t\tif value.PendingDroppedEvents != 0 || value.AuditGeneration != 99 {
\t\t\tt.Fatalf("slot %d generation state = %+v", i, value)
\t\t}
\t}
\tif values[0].RingbufEventsTotal != 10 || values[0].RingbufReserveFailedTotal != 3 || values[0].EventSequence != 11 || values[1].RingbufEventsTotal != 20 || values[1].RingbufReserveFailedTotal != 4 || values[1].EventSequence != 22 {
\t\tt.Fatalf("cumulative counters were not preserved: %+v", values)
\t}
''',
)

replace_once(
    "backend/app/observability/metrics_collector.go",
    '''\tvar total bpfCollectorStats
\tgenerationConsistent := true
\tfor _, value := range values {
\t\ttotal.RingbufEventsTotal += value.RingbufEventsTotal
\t\ttotal.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal
\t\ttotal.EventSequence += value.EventSequence
\t\ttotal.PendingDroppedEvents += value.PendingDroppedEvents
\t\tif value.AuditGeneration != 0 {
\t\t\tif total.AuditGeneration == 0 {
\t\t\t\ttotal.AuditGeneration = value.AuditGeneration
\t\t\t} else if total.AuditGeneration != value.AuditGeneration {
\t\t\t\tgenerationConsistent = false
\t\t\t}
\t\t}
\t}
''',
    '''\tvar total bpfCollectorStats
\tgenerationConsistent := true
\tfor _, value := range values {
\t\ttotal.RingbufEventsTotal += value.RingbufEventsTotal
\t\ttotal.RingbufReserveFailedTotal += value.RingbufReserveFailedTotal
\t\ttotal.EventSequence += value.EventSequence
\t\ttotal.PendingDroppedEvents += value.PendingDroppedEvents
\t\tif value.AuditGeneration == 0 {
\t\t\tgenerationConsistent = false
\t\t\tcontinue
\t\t}
\t\tif total.AuditGeneration == 0 {
\t\t\ttotal.AuditGeneration = value.AuditGeneration
\t\t} else if total.AuditGeneration != value.AuditGeneration {
\t\t\tgenerationConsistent = false
\t\t}
\t}
''',
)
