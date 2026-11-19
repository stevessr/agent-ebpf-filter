# Three experiments for a reproducible systems-paper evaluation

Status: **instruments and schemas provided; no live measurements recorded here.**
A top-conference claim also requires novelty, competing systems, and
external-validity evidence—not simply three completed tables.

## Reproducibility ledger

Archive complete Git SHA/dirty status, kernel/CPU physical core count, CPU
governor, BTF and cgroup availability, BPF LSM enablement, attach status,
compiler versions, random seeds, exact commands, raw event traces, failures,
and full trial denominators. Record 95% confidence intervals where possible.
A rootless static object check is **not** live kernel-enforcement evidence.

## Experiment A: concurrent scalability (manuscript Section 6.1)

Original command:

~~~bash
BENCH_CONCURRENCY="1 4 8 16 32" BENCH_CYCLES=10 \
  ./scripts/benchmark-concurrent.sh
~~~

**Legacy measurement issue:** the original script starts with attached eBPF
programs and measures both the so-called baseline and the instrumented
phase without detaching programs. Their difference is *same-state drift*,
not measured eBPF overhead. The patched script now marks the evidence
comparison_valid=false and the report refuses to publish false overhead.

For an actual on/off comparison, provide an operator-reviewed executable
BENCH_PHASE_SCRIPT accepting a single argument, either baseline or ebpf.
This script must keep the authenticated backend responsive, detach the
relevant hooks for baseline, and reattach for ebpf. The runner checks the
reported attach state after each phase switch. Use an isolated test host.

~~~bash
BENCH_PHASE_SCRIPT=/path/to/operator-reviewed-phase-switch \
BENCH_CONCURRENCY="1 4 8 16 32" BENCH_CYCLES=10 \
  ./scripts/benchmark-concurrent.sh
~~~

**P95 caveat:** benchmark-syscalls-extended.go reports elapsed batch time
divided by number of iterations. That is a throughput-derived mean,
not a distribution of independent syscall latencies. The program does
not emit per-call P95. If per-call samples are independently measured,
place a CSV named latency_ebpf_c1.csv etc. in the report folder, containing
the latency_us header and values. Document exact operation mix and
sampling overhead instead of treating mixed operation samples as a single
homogeneous syscall.

For a **per-call** P95 sample, a separate Go sampler measures one pread(2)
operation on a pre-opened file for each individual sample. It includes
userspace timing cost; it is NOT an eBPF-only overhead estimate. Run it with
the backend BPF programs actually attached, on the same test host and after
the on/off experiment for each C:

~~~bash
mkdir -p reports/ebpf-concurrent-paper
for C in 1 4 8 16 32; do
  go run scripts/paper-syscall-latency.go \
    --concurrency "$C" --samples 50000 --warmup 1000 \
    --output "reports/ebpf-concurrent-paper/latency_ebpf_c${C}.csv"
done
~~~

Use the same report folder as the verified summary_c*.json files when
generating the paper table. These P95 samples describe **pread latency
with attached hooks**, not all 18 operations and not tail latency of full
Agent workflows. Do not merge their distributions or label them as
end-to-end latency.

Test all C levels on the **same physical host**, not separate ephemeral
GitHub Action matrix runners. Randomize phase/order to check temperature,
cache, and governor effects.

| C | Mean overhead (%) | Per-call P95 (us) | State |
|---:|---:|---:|---|
| 1 | — | — | Awaiting valid control |
| 4 | — | — | Awaiting valid control |
| 8 | — | — | Awaiting valid control |
| 16 | — | — | Awaiting valid control |
| 32 | — | — | Awaiting valid control |

## Experiment B: 2–4 Agent attribution ablation

The actual project uses three kinds of evidence: direct PID/registered
context, trusted lifecycle process ancestry/fork/cgroup, and
wrapper/native-hook semantic context (see technical/agent-identity.md).
Shared executable names are not proof of the owning Agent.

Run the real concurrent workload, which starts distinct registered roots,
each executing git, python, and node. It produces only ground-truth child
PID, root PID, run ID, and process lifetime. It does not invent predictions:

~~~bash
python3 scripts/attribution-concurrent-workload.py \
  --agents 2 3 4 --cycles 10 \
  --backend-url http://127.0.0.1:8080 \
  --output reports/paper-experiments/ground-truth.csv
~~~

Requires a privileged, running backend; git, python3 and Node.js. The
development --dry-run option does not register roots and is NOT real
attribution evidence.

Repeat equivalent workloads under three independently verified
configurations: **PID only** (exact PID registration lookup); **lifecycle
only** (trusted fork/exec ancestry, independently seeded root ID);
**three modalities** (PID plus lifecycle plus wrapper/native semantic
tool context). Do not simply mask fields in a combined trace and call it
an isolated ablation. The repository does not currently expose a verified
one-click feature-gate selector for all three variants, so these still
require explicit implementation/configuration verification.

Join kernel events to ground truth by PID, process start timestamp and
run boundary. Reject PID reuse ambiguity; do not infer Agent from comm.
Create attribution.csv with the columns:

~~~csv
event_id,agent_count,command,truth_agent,pid_only,lifecycle_hook_only,trimodal
~~~

Each row is one uniquely labeled event. Use an actual Agent ID or the
unknown label ⊥. Report the mutually exclusive fractions:
correct / N, assigned-to-wrong-Agent / N, unknown / N. Also stratify by
command to expose same-name cross-Agent contamination.

| Agents | PID-only: correct/false/⊥ | Lifecycle: correct/false/⊥ | Combined: correct/false/⊥ |
|---:|---|---|---|
| 2 | — / — / — | — / — / — | — / — / — |
| 3 | — / — / — | — / — / — | — / — / — |
| 4 | — / — / — | — / — / — | — / — / — |

## Experiment C: cgroup + 14-hook BPF LSM enforcement

Use a privileged, isolated host with an active BPF LSM and writable
cgroup v2. Preflight, check the compiled objects, then run the smoke test:

~~~bash
make os-enforcement-preflight
make os-enforcement-check
make os-enforcement-smoke-start
# Or: make os-enforcement-smoke against a running privileged backend
~~~

The smoke-start target cleans up the backend process it launches on exit.
**For repeated trials, keep a privileged backend running in a separate
terminal**, then enable policy-management on that isolated test backend,
e.g. with an already authorized local API:

~~~bash
sudo -E env DISABLE_AUTH=true ./backend/agent-ebpf-filter
# In a second shell (isolated test host only):
curl -fsS -X PUT http://127.0.0.1:8080/config/runtime \
  -H 'Content-Type: application/json' \
  -d '{"policyManagementEnabled":true}'
~~~

If authentication is enabled, supply the legitimate API token instead of
DISABLE_AUTH; do not enable policy management on an exposed production host.

The smoke suite covers TCP, UDP, IPv4-mapped IPv6, existing FD
read/write, and mmap/mprotect among other cases. A passing smoke test
is not a deterministic-rate measurement: some probe branches currently
treat arbitrary OSError as blocked and do not preserve errno.

For every *repeated* trial record all four conditions:
(1) identical call succeeds with no rule, (2) it is denied with the rule,
(3) the syscall reports EPERM (1) or EACCES (13), and
(4) it succeeds again after rule revocation. Other errors such as
ECONNREFUSED or ENOENT are **not** policy denials. Time policy apply and
revoke API requests with a monotonic clock (microseconds). These are
API round-trip times, **not** independently proven BPF map propagation
times. Report skipped scenarios separately and preserve all failed trials.

Run the repeated-trial recorder against that privileged backend. Enable the
backend's policy-management gate in the isolated host before issuing policy
writes. The recorder requires live attached cgroup and BPF LSM status;
each row records the actual Python syscall errno, not merely an exception.
A sidecar manifest records the kernel, timestamp and requested scenarios.

~~~bash
python3 scripts/paper-enforcement-trials.py \
  --backend-url http://127.0.0.1:8080 --trials 100 \
  --output reports/paper-experiments/enforcement.csv
~~~

The recorder deliberately uses local loopback endpoints and unique temporary
file names, then removes each rule after each trial. Always inspect the
resulting manifest, failed trials and skipped IPv4-mapped IPv6 cases.

CSV schema:

~~~csv
case,control_ok,denied,errno,restored,apply_us,revoke_us
~~~

Allowed cases: tcp_connect, udp_connect, udp_sendto,
ipv4_mapped_ipv6, existing_fd_read, existing_fd_write, mmap.
Repeat trials (e.g. 100 per case); never print 100% without a real N.
Existence of fourteen compiled LSM sections is not evidence that all
fourteen are successfully attached.

| Path | Verified deny rate | Rule apply P50/P95 (us) | Revoke P50/P95 (us) |
|---|---:|---:|---:|
| TCP connect | — | — | — |
| UDP connect | — | — | — |
| UDP sendto | — | — | — |
| IPv4-mapped IPv6 | — | — | — |
| Existing FD read | — | — | — |
| Existing FD write | — | — | — |
| Existing FD mmap | — | — | — |

## Generate paper-ready Markdown (no invented data)

~~~bash
python3 scripts/paper-experiment-tables.py \
  --concurrency-dir reports/ebpf-concurrent-YYYYMMDD-HHMMSS \
  --attribution-csv reports/paper-experiments/attribution.csv \
  --enforcement-csv reports/paper-experiments/enforcement.csv \
  --output reports/paper-experiments/tables.md

python3 scripts/tests/test-paper-experiment-tables.py -v
python3 scripts/tests/test-paper-enforcement.py -v
~~~

Missing files remain clearly unmeasured. Invalid evidence fails loudly.
Maintain raw files with the paper artifact; do not copy illustrative
performance figures from existing docs/testing/concurrent-benchmark.md.
