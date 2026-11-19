# 2026-11-19 CI measured paper inputs

These compact tables were transcribed from original, inspectable GitHub Actions CSV
artifacts, not synthesized from assumed kernel behavior.

- read_observed_summary.csv: 10 × 20,000 per-call read(2) samples per concurrency and
  state, **not causal eBPF-only overhead**, on a 4-CPU AMD EPYC 7763 runner.
- agent_workload_overlap.csv: actual PID/timestamp overlap for registered Agent root
  workloads, **not the three-mode attribution accuracy ablation**.
- cgroup_network_summary.csv: cgroup-only network denial observed, actual errno=1
  after policy activation and successful before/after controls. Times are localhost
  HTTP API round-trip latencies, not kernel map update latency.

Raw evidence:
- [CI run 44485145318](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44485145318)
- [CI run 44486257386](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44486257386)

The kernel did not have a working BPF LSM enforcer. File FD / mmap blocks were not
measured. A subsequent real 3-mode study DID complete: independent PID-only,
lifecycle-only and combined backend instances on the same CI runner,
with real event-ID/PID/time truth. Details are in
`attribution_three_mode_measured.csv` and
`attribution_by_command_measured.csv` ([run 44492325326](https://github.com/stevessr/agent-ebpf-filter/actions/runs/44492325326)).

PID-only: 0/2530 correct (2530 unattributed).
Lifecycle-only: 2592/2597 correct (99.81%, 5 unknown).
Combined: 2559/2575 correct (99.38%, 16 unknown).
All modes: 0 observed wrong-Agent event assignments,
270/270 subprocesses with at least one captured event.
**Combined did not outperform lifecycle; the workload did not generate
actual Native Hook semantic traces.** These observations cannot prove
that a third semantic modality improves attribution accuracy.
The new run is a distinct commit/host workload; do not combine its
denominators with the earlier 90/270 sensor-coverage experiment.
