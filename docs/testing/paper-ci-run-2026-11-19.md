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
