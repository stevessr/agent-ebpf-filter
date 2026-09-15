# Attack-Impact AutoML

This document describes the host-security risk objectives used by Agent eBPF Filter's model selection and risk scoring.

## Design goals

The ML layer is optimized for security outcomes rather than plain classification accuracy. A model that achieves high aggregate accuracy but allows destructive, privilege, persistence, or exfiltration behavior must rank below a model that catches those attacks with slightly lower benign accuracy.

The design borrows public architectural ideas from modern edge-security scoring systems: deterministic high-confidence detections and machine learning are layered, a global score is accompanied by vector-specific scores, and policy consumes the resulting scores instead of treating the ML classifier as the only source of truth. The implementation here is host/eBPF-specific and does not reproduce any vendor-private model.

## Runtime attack vectors

Runtime risk is represented as four independent 0-100 vectors. Higher means more dangerous.

| Vector | Examples |
| --- | --- |
| `intrusion` | privilege escalation, credential access, reverse shell, scanning, firewall bypass |
| `destruction` | file deletion/wiping, process killing, destructive writes, destructive firewall changes |
| `exfiltration` | credential/file upload, DNS tunnel, explicit transfer/exfiltration patterns |
| `persistence` | cron/autostart/systemd enablement, authorized keys, C2/reverse-shell persistence |

The global host risk is derived from the strongest vector, a bounded general ML/anomaly/LLM prior, and smaller contributions from correlated secondary vectors. It is deliberately not a simple sum.

The external compatibility score remains `0..100`, where larger values mean higher risk:

- `0..19`: `CLEAN`
- `20..39`: `LIKELY_CLEAN`
- `40..59`: `SUSPICIOUS`
- `60..79`: `LIKELY_ATTACK`
- `80..100`: `ATTACK`

Deterministic evidence such as reverse-shell, data-exfiltration, DNS-tunnel, and firewall-manipulation findings can strongly raise a relevant vector. Anomaly evidence cannot invent an attack vector by itself; it only strengthens identified vectors or acts as a global prior.

## AutoML security metrics

Cross-model AutoML records conventional accuracy metrics and the following attack-impact metrics:

| Metric | Meaning |
| --- | --- |
| `attackRecall` | recall over samples whose ground truth requires BLOCK, ALERT, or REWRITE |
| `highImpactRecall` | recall for destructive/credential/privilege/persistence samples; requires BLOCK or ALERT for full credit |
| `intrusionRecall` | recall on intrusion-vector samples |
| `destructionRecall` | recall on destructive-vector samples |
| `exfiltrationRecall` | recall on exfiltration-vector samples |
| `persistenceRecall` | recall on persistence-vector samples |
| `catastrophicMissRate` | fraction of high-impact samples predicted directly as ALLOW |
| `riskWeightedRecall` | attack recall weighted by expected consequence |
| `benignFalsePositiveRate` | fraction of benign samples receiving BLOCK, ALERT, or REWRITE |
| `securityUtility` | normalized asymmetric utility that penalizes harmful misses more than ordinary false positives |
| `threatVectorCoverage` | fraction of the four attack vectors represented in the validation slice |

### Asymmetric cost policy

A missed destructive/credential attack costs substantially more than a benign false positive. Benign actions also have different disruption costs:

- false BLOCK: highest benign cost
- false ALERT: lower cost
- false REWRITE: lower again, but not free

For high-impact attacks, a prediction of ALLOW is counted as a catastrophic miss. REWRITE may be useful but does not earn full high-impact recall, because a destructive operation that is merely rewritten still requires careful review.

## AutoML objective

Cross-model selection defaults to `securityUtility` when no explicit metric is provided. Callers may explicitly choose:

- `securityUtility`
- `riskWeightedRecall`
- `highImpactRecall`
- `attackRecall`
- `intrusionRecall`
- `destructionRecall`
- `exfiltrationRecall`
- `persistenceRecall`
- legacy objectives such as balanced accuracy or throughput

Sparse validation slices fall back to broader metrics when a requested attack vector has no samples. This avoids selecting a model on a meaningless zero score.

## Policy boundary

These scores are evidence for policy. High-confidence deterministic enforcement remains independent and can override ML output. The risk model is intended to improve detection of evasive or previously unseen behavior and to choose safer model families/features, not to weaken existing BPF LSM, command-policy, or network-audit enforcement.

## Current limitation

The cross-model selector is security-first. When optional per-model parameter-grid tuning is enabled, the inner grid currently uses balanced accuracy for security-only outer objectives, then the resulting trained candidate is re-evaluated with the full attack-impact metrics. A future refinement can make every parameter-grid cell carry the same attack-impact metrics and optimize `securityUtility` directly.
