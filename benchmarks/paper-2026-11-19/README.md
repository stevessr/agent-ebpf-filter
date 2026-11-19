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
measured. No combined PID-only/lifecycle-only/trimodal attribution scores exist yet.
