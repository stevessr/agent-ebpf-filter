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
