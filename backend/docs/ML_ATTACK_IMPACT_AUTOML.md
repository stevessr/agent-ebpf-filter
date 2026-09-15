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

Generic `NETWORK` activity is intentionally vector-neutral. Network context becomes intrusion, exfiltration, or persistence only when concrete evidence such as reverse-shell, explicit upload, DNS tunneling, suspicious targets, scanning, or firewall manipulation is present.

## AutoML security metrics

Both cross-model selection and every parameter-grid cell record conventional accuracy metrics and the following attack-impact metrics:

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

The evaluation policy is explicit and versioned (`attack-impact-v2`). Current normalized costs/credits are:

| Decision | Meaning | Value |
| --- | --- | ---: |
| false `BLOCK` on benign | highest benign disruption cost | 2.50 |
| false `ALERT` on benign | lower interruption cost | 1.25 |
| false `REWRITE` on benign | semantic-change cost | 0.50 |
| `BLOCK` on attack | full containment credit | 1.00 |
| `ALERT` on attack | strong detection, weaker containment | 0.88 |
| `REWRITE` on attack | partial mitigation credit | 0.55 |
| catastrophic high-impact `ALLOW` | extra multiplicative security-utility penalty | 0.65 |

For high-impact attacks, a prediction of ALLOW is counted as a catastrophic miss. REWRITE may be useful but does not earn full high-impact recall, because a destructive operation that is merely rewritten still requires careful review.

## Confidence-aware ranking

Raw percentages are retained for display, but AutoML does **not** rank binomial security rates purely by the observed percentage.

Recall objectives use a 95% Wilson lower confidence bound. Error-rate objectives (`catastrophicMissRate` and `benignFalsePositiveRate`) use the complementary 95% Wilson upper bound. This prevents tiny slices from looking artificially perfect.

For example, an observed `1/1 = 100%` destruction recall has a much weaker conservative ranking score than `95/100 = 95%`. Logs therefore expose three values together:

- observed metric,
- conservative ranking score,
- validation support count.

`securityUtility` and `riskWeightedRecall` are weighted continuous objectives rather than Bernoulli rates, so their ranking values are not transformed by Wilson bounds.

## AutoML objective

Cross-model selection and parameter-grid tuning default to `securityUtility` when no explicit metric is provided. Callers may explicitly choose:

- `securityUtility`
- `riskWeightedRecall`
- `highImpactRecall`
- `attackRecall`
- `intrusionRecall`
- `destructionRecall`
- `exfiltrationRecall`
- `persistenceRecall`
- `catastrophicMissRate`
- `benignFalsePositiveRate`
- legacy objectives such as balanced accuracy or throughput

If a requested attack-vector objective has no validation support, that candidate/cell is marked non-comparable for the objective. The selector does not silently substitute generic accuracy, because that would make the experiment answer a different question from the one requested.

## Family × feature security profile

Cross-model runs also aggregate candidates by `model family × feature class`. The summary is support-weighted rather than a simple mean:

- attack recall is weighted by attack samples,
- each I/D/E/P recall is weighted by samples for that vector,
- catastrophic miss rate is weighted by high-impact samples,
- benign false-positive rate is weighted by benign samples,
- security utility is weighted by all scored validation samples.

This prevents a family with a single perfect vector example from overpowering a family evaluated on a substantially larger slice. The training log prints each group's security utility, global attack recall, high-impact recall, I/D/E/P recalls, catastrophic miss rate, and strongest member model.

## Parameter-grid behavior

The inner parameter grid is now security-first as well. Every cell computes `AttackImpactMetrics`, receives the same requested security objective as the outer cross-model run, and can become best only when that objective has validation coverage. Optional per-model parameter tuning therefore no longer optimizes balanced accuracy first and re-ranks only at the outer layer.

## Policy boundary

These scores are evidence for policy. High-confidence deterministic enforcement remains independent and can override ML output. The risk model is intended to improve detection of evasive or previously unseen behavior and to choose safer model families/features, not to weaken existing BPF LSM, command-policy, or network-audit enforcement.

## Validation

`.github/workflows/ml-security.yml` generates the required protobuf/eBPF bindings, runs the full backend `app/ml` test suite and attack-risk app tests, and runs frontend typecheck/build for ML-related changes. This is separate from the repository-integrity workflow so a green gitlink check cannot be mistaken for ML validation.
