# 2026-11-19 paper experiment evidence ledger

This file records **observed** CI facts, including limitations. Do not use a
code path's intended behavior as a measured performance or denial rate.

## Previous attachment smoke: successful (different CI runner)

[GitHub Actions run 44482390652](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44482390652)
reported `compiledCount=210`, `attachedCount=202`, `skippedCount=8`
while performing its authenticated bootstrap/attachment check.

Its one-cycle concurrency comparison reported -1.36% at C=1, but that is
**not an actual eBPF overhead measurement**, since both phases have BPF
attached. It is retained as a smoke diagnostic only.

## New live CI run and capability probe

[GitHub Actions run 44485145318](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44485145318)
was triggered on commit `e16cfb32fed030b6f95eaabbedbe17acef1dd2c1`.

Its independent host-capability job completed with:

- Host kernel: Linux 7.0.0-1017-azure
- CPU: Intel Xeon 6973P-C, 4 logical CPU threads
- Kernel BTF: readable
- cgroup v2 filesystem: mounted, root not writable from runner account
- bpffs: mounted
- LSM list: lockdown, capability, landlock, yama, apparmor, ima, evm
- **BPF LSM absent**, so this environment cannot yield valid BPF LSM
  end-to-end denial evidence.
  
The live data collection **runs in a separate job and hence potentially a
different ephemeral runner** from the capability probe. The live job must
archive its own host manifest and its on/off measurement status; do not
assume the two job hosts are identical.

The new CI job collects 10 x 20,000 individually timed read(2) calls for
each of C=1, 4, 8, 16, 32 before backend startup and, if verified attached,
after startup. Values are exploratory because they are ordered and
the backend daemon is absent during the off control. It also attempts
2–4 registered Agent roots with same-name git/python/node workflows and
conditionally runs actual errno probes *only if* BPF LSM and cgroup
are available.

**Artifact validity rules**: Attribute accuracy needs isolated PID-only /
lifecycle-only / tri-modal predictions and ground truth; PID timestamps
alone prove no accuracy. A successful smoke check cannot be called
100% policy denial unless controls, actual errno and recovery are
recorded for every trial. A per-read P95 is not a whole-workflow P95.

## Completed live experiment run: read(2) and same-name Agent workloads

The full experimental job in [run 44485145318](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44485145318)
completed successfully. Original evidence is the
`paper-live-experiments-ci-host` artifact (artifact ID `14510517460`).

This specific **experiment job** ran on AMD EPYC 7763 (4 logical CPUs),
Linux `7.0.0-1017-azure`, Go 1.27.3. This is distinct from the independent
capability job's Intel Xeon runner; the two hosts must not be conflated.

| C | read off mean (µs) | read with attached BPF mean (µs) | attached P95 (µs) | observed change (%) |
|---:|---:|---:|---:|---:|
| 1 | 0.4789 | 0.9028 | 1.1820 | +88.51 |
| 4 | 0.6216 | 1.1476 | 1.1520 | +84.61 |
| 8 | 0.6140 | 1.1322 | 1.1420 | +84.40 |
| 16 | 0.6205 | 1.1803 | 1.1520 | +90.20 |
| 32 | 0.6070 | 1.1787 | 1.1720 | +94.20 |

Each cell uses ten repetitions of 20,000 timed read(2) calls per state,
200,000 samples per C and state, 2 million total. **Difference is not
causal eBPF-only overhead**: off samples ran before the backend started,
on samples after; there was no randomized crossover or matched backend
CPU load. C=8/16/32 oversubscribe the runner's 4 logical CPUs.

The backend reported `compiledCount=210`, `attachedCount=202`,
`skippedCount=8`, status `partial`; the listed unavailable points
include `syscalls/sys_enter_stat`, `syscalls/sys_enter_lstat`,
`syscalls/sys_enter_umount2`, related exits, and `tcp/tcp_connect`
and `tcp/tcp_close`.

The Agent workload produced 270 registered tool process records across
2, 3, and 4 Agent roots, with exactly 90 git, 90 python, 90 node.
**All 90 command-specific batch windows overlapped across all Agents
in that batch**, verified by process start/end timestamps.
This validates concurrency, *not attribution accuracy*. The three
isolated inference methods were not run and all 3-way attribution
accuracy / false / unknown rates remain unmeasured.

Cgroup sandbox was **available and attached** with 4 links.
BPF LSM was **not available/attached**: attempting
`lsm_enforce_file_mprotect` returned
`load program: invalid argument: helper call is not allowed in probe`.
Consequently the LSM file/mmap denial cells stay unmeasured.

A separate **cgroup-only network deny** workflow
[run 44486257386](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44486257386)
was launched to exercise TCP/UDP/IPv4-mapped IPv6 paths without requiring
the unavailable LSM. Its final per-case error-code evidence must be
read from its own artifact, not assumed from a successful build.

## Completed 100-trial-per-case cgroup network policy enforcement

[GitHub Actions run 44486257386](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44486257386)
completed successfully. Raw artifact: `paper-cgroup-network-denial-100trials`
(artifact ID `14510942702`). Live status recorded
`available=true`, `attached=true`, `linkCount=4` for cgroup
`connect4`, `connect6`, `sendmsg4`, and `sendmsg6`.

| Probe | Full control+denial+recovery | Rule apply API P50/P95 (µs) | Rule revoke API P50/P95 (µs) |
|---|---:|---:|---:|
| TCP connect | 100/100, errno=1 | 312.6 / 384.0 | 264.1 / 298.8 |
| UDP connect | 100/100, errno=1 | 250.7 / 318.4 | 241.0 / 304.9 |
| UDP sendto | 100/100, errno=1 | 240.7 / 305.8 | 253.8 / 318.2 |
| IPv4-mapped IPv6 | 100/100, errno=1 | 307.9 / 357.7 | 260.3 / 309.1 |

All 400 recorded trials had `control_ok=true`, `denied=true`,
`errno=1 (EPERM)`, and `restored=true`, checked from original CSV,
with no trials missing in these four declared cases.
These data substantiate 100% **observed** blocking in 100 trials for each
cgroup network path on the test runner, not a universal 100% guarantee.

Times are end-to-end localhost policy HTTP API round trips, not direct
kernel BPF map mutation latencies or enforcement propagation times.
The 14 BPF LSM hooks could not be attached and no valid read/write/mmap
block-rate or policy latency has been measured for them.

Permanent compact summary CSVs are under
`benchmarks/paper-2026-11-19/`. Full sample-level evidence remains in
the Actions artifacts while retained.

## Real isolated three-mode attribution study (completed)

[GitHub Actions run 44492325326](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44492325326),
artifact `paper-attribution-three-mode-raw` (artifact ID `14514285575`).
The tested **code SHA** is
`1254f815b76bae1f1c7026c64f36f623288cd7e6`, not the later
documentation/CI branch HEAD. Host kernel:
Linux `7.0.0-1017-azure`, x86_64. Each mode was a *fresh real backend
process* with independently verified attached kernel tracing programs
and 30 cohorts of actual competing registered Agent roots.

| Concurrent Agents | Mode | Event N | Correct | Wrong Agent | Unattributed (⊥) | Process coverage |
|---:|---|---:|---:|---:|---:|---:|
| 2 | PID-only | 529 | 0 | 0 | 529 | 60/60 |
| 3 | PID-only | 919 | 0 | 0 | 919 | 90/90 |
| 4 | PID-only | 1082 | 0 | 0 | 1082 | 120/120 |
| 2 | Lifecycle-only | 584 | 584 | 0 | 0 | 60/60 |
| 3 | Lifecycle-only | 815 | 815 | 0 | 0 | 90/90 |
| 4 | Lifecycle-only | 1198 | 1193 | 0 | 5 | 120/120 |
| 2 | Combined | 598 | 598 | 0 | 0 | 60/60 |
| 3 | Combined | 835 | 828 | 0 | 7 | 90/90 |
| 4 | Combined | 1142 | 1133 | 0 | 9 | 120/120 |

Aggregate **event-level**:
- PID-only: 0/2530 correct, 0/2530 falsely attributed,
  2530/2530 unattributed.
- Lifecycle-only: 2592/2597 correctly attributed (99.81%);
  zero wrong-Agent, 5/2597 unattributed.
- Combined: 2559/2575 correctly attributed (99.38%);
  zero wrong-Agent, 16/2575 unattributed.
- **Process-level coverage** is 270/270 in all three independent runs.
  At least one correct kernel event occurred for each of the 270/270
  tool-processes in lifecycle and combined modes; PID-only has 0/270.
- Event mix includes `execve`, `process_exit`, `process_fork`,
  `process_exec` and `write`. Some independent process-exit events
  in the later two modes remained unattributed (all in `node`).
- Events have unique backend IDs and were joined to ground-truth
  subprocess PID + process lifetime + matching observed PPID, allowing
  privacy-redacted command names only with independent PID/time evidence.

**Research interpretation and threats:**
1. Combined did NOT outperform lifecycle-only (99.38% vs 99.81%
   event-correct). This is an independent-run, unequal-denominator,
   nonrandomized comparison, not statistical proof of inferiority.
2. The real git/python/node workload seeds Agent roots through /register
   but does not generate genuine Native Hook semantic tool spans. The
   contribution of a distinct third semantic modality is *not yet tested*,
   so a "tri-modal improvement" claim would be unsupported.
3. Per-process coverage is not the same as per-syscall capture completeness.
   API snapshots impose a 1000-event page limit; repeated snapshots are
   deduplicated by event ID, and captured events are not a census of every
   syscall.
4. Zero observed wrong-Agent assignments in these trials is not a proof
   of zero false attribution in all real Agent workflows; particularly
   adversarial prompt injection, PID churn and cgroup migration require
   separate testing.

Additional analysis tables:
`benchmarks/paper-2026-11-19/attribution_three_mode_measured.csv`
and `attribution_by_command_measured.csv`.

**Prior failed run:** first isolated study #44490645786 completed PID-only
but could not verify eBPF attachments on the next backend process because
persistent pinned links were reused without bootstrapping new in-process
health. The research-specific startup now refreshes kernel attachment,
and experiment #44492325326 finished all three modes successfully.

## New matched-daemon, kernel-link verified, randomized read(2) crossover

[CI #44512199846](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44512199846)
completed successfully. Raw artifact
`paper-matched-crossover-real-kernel-links`, ID `14523563051`,
tested commit `14cbc06ecf7877917202aff0549225745009f9be`,
kernel `7.0.0-1017-azure`.

**Actual control validation:** the same backend process stayed alive
across all 100 phases. `bpftool -j -f link show` allowed the harness to
verify removal of the 202 original project tracepoint kernel link IDs,
and restoration of 202 new attached kernel link IDs in `on` phases.
The artifact confirms `restored_tracepoint_links=true` and every
phase `actual_kernel_link_ids_verified=true`. Cgroup and BPF LSM
enforcement hook pin directories were never modified.

Each C=1,4,8,16,32 has ten paired cycles, with **five off→on and five
on→off**, random balanced order using seed 20261119. Each of the two
phases has 10×5,000 individually timed read(2) samples per C,
**50,000 per phase per C = 500,000 samples total**. The 95% Bootstrap
CI resamples the ten *pair-level* percentage changes, not individual
syscalls. All values below are from raw CSV with complete samples.

| C | Off read mean (µs) | On read mean (µs) | On P95 (µs) | Mean paired increase | 95% paired bootstrap CI |
|---:|---:|---:|---:|---:|---|
| 1 | 0.2286 | 0.3628 | 0.4650 | +58.82% | [+51.38%, +65.79%] |
| 4 | 0.2757 | 0.4379 | 0.4950 | +59.00% | [+48.38%, +68.81%] |
| 8 | 0.2578 | 0.4430 | 0.4870 | +72.98% | [+62.25%, +85.42%] |
| 16 | 0.2865 | 0.4698 | 0.5170 | +64.22% | [+53.77%, +74.23%] |
| 32 | 0.2724 | 0.4598 | 0.5100 | +69.15% | [+57.63%, +78.29%] |

This is a **material methodological improvement** over the old
off-before-daemon/on-after-daemon data above: the running daemon and
measured machine are matched, and order is balanced. The result is an
estimate of monitoring **instrumentation + collector** overhead for a
tiny `read(2)` operation; it is not the BPF instruction-only cost, and
it does not directly quantify a complete Agent workflow or arbitrary
syscalls. CPU scaling at C=8/16/32 is still oversubscribed on this CI
runner, with potential frequency noise and repeated link pin overhead.
The earlier 2-million-call snapshot remains archived as exploratory,
not replaced or retroactively reinterpreted.

Permanent compact inputs:
`benchmarks/paper-2026-11-19/read_matched_crossover_measured.csv`.
[Protocol](paper-matched-crossover.md).

**First CI attempt #44511234015 failed** to query one pinned perf-event
link by pathname; no performance claim was made from that invalid run.
The harness was corrected to verify via `bpftool -j -f link show`
global IDs and pinned paths; follow-up #44512199846 finished all
phases and restored instrumentation.
