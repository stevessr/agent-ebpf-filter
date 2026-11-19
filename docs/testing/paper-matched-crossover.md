# Matched-daemon eBPF tracepoint crossover (research only)

## Motivation

The earlier 2026-11-19 *before starting the daemon* / *after starting the
daemon* read(2) comparison cannot identify tracing cost by itself. Changes to
daemon load, OS caching, CPU frequency, environment and run order confound
that estimate. The prior +84–94% relative changes remain **exploratory**.

## Intervention and negative control

[Workflow](../../.github/workflows/paper-matched-crossover.yml) runs on an
**ephemeral GitHub-hosted Linux runner** with a running project backend that
remains alive across every measured block. Only the project's own
`/sys/fs/bpf/agent-ebpf/links/*` syscall tracepoint BPF pins are switched:

- `off`: unpin every project tracepoint link, then verify their **original
  kernel link IDs are absent** from `bpftool -j link show`. Never treat mere
  filesystem absence as proof the eBPF programs detached.
- `on`: execute the project's privileged `--ebpf-bootstrap`, then verify
  the exact tracepoint pin-name set is restored and each **new kernel link
  ID** appears in the kernel's global BPF link table.
- At each phase, check the *same backend HTTP process* is responsive.
- Keep cgroup network enforcement and BPF LSM link roots entirely out of
  scope. Off mode is **not** a security-disable action for production hosts.

The CI-root-only tool enforces explicit `CI=true`, `RUNNER_TEMP`, checked-out
workspace and bpffs mount preconditions. It attempts to restore tracepoints
after interruption and reports failure if kernel restoration cannot be
verified. Do not run this on shared or production hosts.

## Order, denominator and statistics

C=1,4,8,16,32. Each level has 10 pairs. Exactly five pairs run off→on
and five on→off, randomized with a recorded seed; level order is also
randomized. Each phase samples 5,000 individually timed `read(2)` system
calls on a per-worker `/dev/zero` file descriptor, preceded by 200 warmups.

For each pair, calculate:

```text
delta_percent = 100 * (mean_on_us / mean_off_us - 1)
```

Report the mean **paired** percentage over 10 pairs, with a 95% percentile
bootstrap interval resampling the **ten pairs**, not millions of correlated
calls. Separately report full off/on read means and on-state P95 latency.
The analyzer rejects missing or duplicated pairs, zero baseline, unsafe
paths, unexpected tracepoint pin counts, missing kernel verification and
unrestored tracepoints. Its tests use fake data only to test correctness,
not to claim evidence.

## What this measures

This experiment estimates the incremental runtime cost of enabling
project tracing and delivering its events through an always-running
backend, on one CI runner and one synthetic `read(2)` workload.

It is **not** a measurement of isolated eBPF instruction time,
full Agent workflow wall time, arbitrary syscall latency, or
worst-case server slowdown. CPU-frequency drift, kernel pin/rebootstrap
effects, caching and GitHub runner sharing remain relevant. eBPF source
attachment and pinned kernel-state health must appear in the artifact
before any numeric data is promoted to the paper.

Run URL and numeric results belong here **only after successful CI completion**.
