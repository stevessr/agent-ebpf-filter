# Kernel eBPF Linear Guard — scoped machine-learning enforcement

> Experimental opt-in path. The existing backend's regular cgroup sandbox,
> BPF LSM, kernel-risk feedback, and the separate DKMS `kernel-ml` module
> are unchanged. This feature does **not** automatically deploy models.

## Decision

A high held-out accuracy alone is not sufficient for kernel blocking:
- **Against**: an agent may legitimately connect to uncommon ports; group
  leakage inflates offline accuracy; a single false positive can interrupt
  an entire task; BPF socket hooks lack user intent, payload, TLS semantics,
  and many filesystem signals. An erroneous model update has immediate impact.
- **For**: a tiny fixed-point classifier on stable kernel-extractable fields
  can be bounded, verifier-friendly, very fast, and useful for *narrow* network
  operations. A separate cgroup attachment and explicit destination scope
  limits blast radius.
- **Conclusion**: use a separate cgroup-scoped, shadow-first classifier with
  independent operator-approved ports. Only arm block mode on review of
  evidence, measured false-positive risk, and an exact model digest.

## Present implementation

- `backend/ebpf/kernel_ml_guard.c`: independent `cgroup/connect4`, `cgroup/sendmsg4`,
  `cgroup/connect6`, and `cgroup/sendmsg6` eBPF (the 6 hooks accept
  **only IPv4-mapped IPv6** destinations for model evaluation). Returns 0 to deny only if (1) manually attached
  to a non-root cgroup, (2) model schema valid, (3) external IPv4 candidate,
  (4) destination port explicitly reviewed, (5) integer score reaches
  threshold, and (6) current mode is `block`. Otherwise allows.
- `backend/internal/kernelguard`: bounded model file/feature contract;
  deterministic prediction parity and strict held-out numerical release gate.
- `backend/cmd/kernel-ml-guard`: explicit model/file/port/cgroup CLI. Loads
  a compiled ELF object dynamically, attaches all four hooks, enters shadow
  first, then optionally arms block after model hash + holdout checks.
  On SIGTERM/SIGINT, disarms and closes attachments; nothing is pinned.
- `scripts/train-kernel-guard.py`: offline training/holdout report; no
  privileged kernel access or automatic deployment.
- Current feature vector is a six-element binary vector:
  `[external_ipv4_candidate, port_22, port_4444, port_ge_49152, port_443, port_53]`.
  Only public-candidate IPv4 endpoints are eligible; this conservatively
  excludes common reserved/private/test addresses. The predicate is not
  a complete IANA destination classifier.
- Score is `bias + sum(weights[i] * feature[i])`, with int32 bounds. This
  is a signed-integer linear classifier, **not** a probability or intent model.
  `weights[1..5]` must include a positive port-specific coefficient, and
  the generic external-only baseline must not reach the threshold.
- The operator separately approves at most 64 ports in `kernel_guard_ports`.
  Even an all-positive classifier cannot block arbitrary target ports.
- Per-CPU map reports examined, reviewed, matched and actually blocked. A
  matched event in shadow **is not** a denied syscall.

## Build and safe usage (Linux; cgroup v2, root/CAP_BPF required)

```bash
# E.g. Ubuntu with clang, LLVM and libbpf-dev:
clang -O2 -g -target bpf -D__TARGET_ARCH_x86 \
  -Ibackend/ebpf -c backend/ebpf/kernel_ml_guard.c \
  -o /tmp/kernel_ml_guard.o
cd backend
go test ./internal/kernelguard ./cmd/kernel-ml-guard
go build -o /tmp/kernel-ml-guard ./cmd/kernel-ml-guard
```

Train with a labeled CSV whose columns are `agent_run_id,destination_ip,destination_port,label`;
label is `benign` or `attack` and agent_run_id is a genuine grouping key.
Do not derive the label from current model output or the existing risk score.

```bash
python3 scripts/train-kernel-guard.py --input path/to/labeled.csv \
  --model /tmp/guard.json --holdout /tmp/guard-holdout.json

# Verify the group-held-out report WITHOUT root privileges or eBPF access:
cd backend
go run ./cmd/kernel-ml-guard --validate-only --model /tmp/guard.json \
  --holdout /tmp/guard-holdout.json --validation-csv ../path/to/labeled.csv
cd ..

# Start *shadow*, with a dedicated non-root cgroup containing test Agent tasks.
sudo /tmp/kernel-ml-guard --object /tmp/kernel_ml_guard.o \
  --model /tmp/guard.json --cgroup /sys/fs/cgroup/agent-test \
  --ports 4444,22 --mode shadow
```

When a separately reviewed production-like dataset, kernel smoke tests,
rollbacks and cgroup scope are acceptable, *block* requires additionally:
`--mode block --holdout /tmp/guard-holdout.json --approve-sha256 <exact SHA256
reported by the training utility>`. It is a deliberate per-launch decision;
never give the monitored Agent permission to invoke this privileged loader.

The privileged launcher independently replays the deterministic holdout split
from the labeled CSV using its own Go feature extractor and requires all
confusion-matrix totals **and benign/attack independent-run support** to match
the versioned `kernel-guard-holdout-v2` report before arming. The SHA-256
of the **exact CSV bytes independently replayed** must also match the report.
The evaluator reads a bounded snapshot once, avoiding hash/replay races. Previous
`v1` reports are not accepted by block mode. The trainer prints point
estimates and Wilson bounds; only the strictly validated counters enter the
published JSON contract. This catches
accidental report/model mismatches; it does **not** prove labels or CSV
provenance.

**Critical caveat**: the holdout JSON is not signed and is not external
attestation. The numerical gate is not proof that labels are honest or
that future traffic matches training data. The operator needs an
independently held dataset and manual authorization. The CLI intentionally
does not attach anything on a dry build or test.

## Failure modes and gaps

- Default mode is shadow; absent/invalid model, missing reviewed port,
  private or reserved IPv4 candidate => no model block.
- IPv4-mapped IPv6 destinations are normalized to the same IPv4 classifier;
  **native IPv6 remains unclassified**, and existing cgroup policies should
  independently enforce IPv4/native IPv6. No guarantee for previously established
  TCP connections, unix sockets, file access or processes in other cgroups.
- Real BPF verifier acceptance and effective network blocking require a
  privileged Linux integration smoke test on target kernel; Go unit tests
  alone are not enough.
- If the loader crashes abruptly while running, process-held links should
  close on process exit; operator should still inspect `bpftool cgroup show`.
- Scope is cgroup and descendant processes; service owners should avoid
  placing privileged or unrelated tasks inside that cgroup.
- The v2 holdout release gate requires >=1,000 benign samples, >=100 attack
  samples, >=10 distinct benign Agent runs **and** >=10 distinct attack
  Agent runs. It requires a **one-sided Wilson 95% FPR upper bound <=0.1%**
  and **one-sided Wilson 95% attack-recall lower bound >=95%**.
  These are conservative numerical release criteria rather than established
  production accuracy. Repeated events within a run are correlated: row-level
  Wilson confidence bounds do **not** replace independent-run and temporal
  holdouts or operator-reviewed labels. Shadow must remain the default until
  the deployment workload has been evaluated.

## Model safety and evaluation roadmap

1. Preserve single-source parity of C/Go/Python feature extraction with
   cross-language test fixtures, bounded C complexity and BPF load checks.
2. Collect legitimate enterprise/developer network flows and curated attacks
   from unrelated runs, codebases and dates; split by Agent run **before**
   model selection; validate on temporally held-out traffic.
3. Add a separate adjudicator for false positives and negative controls;
   include DNS, CDNs, remote admin, IPv4-mapped IPv6 equivalence, existing sockets,
   high ports, NAT, namespaces and simulated poisoned data.
4. Report confusion matrix, per-run FPR, high-impact recall, PR-AUC,
   classifier mismatch count, p95/p99 kernel program runtime, map memory,
   policy generation, and decision-to-deny confirmation. Use shadow-first
   rollout with human review and an auditable rollback procedure.
5. Keep multi-Agent intent and history-based models in user space: the
   six observed network features do not measure malicious intent.

References: [Linux BPF verifier](https://docs.kernel.org/bpf/verifier.html),
[LSM BPF](https://docs.kernel.org/bpf/prog_lsm.html). This guard uses
**cgroup** return values, not LSM return conventions.
