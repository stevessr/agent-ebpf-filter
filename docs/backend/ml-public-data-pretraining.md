# Reviewed external-corpus training for the command decision model

This is an **offline, candidate-only** extension to the existing random-forest
command classifier. It does not deploy an artifact, alter cgroup/BPF LSM policy,
or prove eBPF syscall accuracy. The default scheduled baseline is unchanged:
pinned `rivoluzione-informatica/bash-datasets` `taglio4_safe.jsonl` and
`taglio4_unsafe.jsonl`.

## Why broader data, not just more trees?

The existing grouped holdout separates identical command signatures, but the
public command corpus is still one distribution. Its reported ~97% validation
accuracy is **not** cross-host, cross-agent, cross-run or time-forward evidence.
The prior linear-model holdout correction (PR #146) also shows why a training
score must not masquerade as held-out accuracy.

### Candidate external data sources (reviewed 2026-11-03)

| Source | Best use | Ground-truth / compatibility warning |
| --- | --- | --- |
| [CLI Command Risk, Hugging Face v1.5](https://huggingface.co/datasets/yannelli/cli-command-risk) | Near-domain Bash/zsh/sh/dash/fish command pretraining, hard benign near misses and destructive operations | **136,497 rows, 1,258 template families, 28,296 distinct commands** at the v1.5 dataset card, not 136,497 independent examples. CC BY 4.0. Risk levels and tags are model-generated; no human review. Its `data_exfiltration_pattern` category has **0** examples. Do **not** bulk-map `high`/`critical` to `BLOCK` or `safe` to runtime `ALLOW`; curate and review each chosen action in context. Honor family-based splits. The author's separate website has displayed different row counts for different revisions: pin a dataset revision and checksum. |
| [AIT Log Data Set v2.1](https://zenodo.org/records/19483937) | Linux audit/syslog/host-event evaluation with realistic benign background and attacks in eight emulated enterprise testbeds | Whole collection ~137 GB; no-pcap variants reduce size. Its attack-related event ground truth is **not** a command authorization decision. Decode event sequences, align host/session/time and annotate policy actions before import. Verify distribution and licensing per downloaded record. |
| [CAM-LDS v2](https://zenodo.org/records/18861762) | Cross-technique attack context and MITRE ATT&CK-based generalization (81 techniques across 13 tactics) | Attack-generated logs may include innocent background activity. Attack-window membership **must never** automatically label every process command as BLOCK. |
| [LID-DS 2021](https://github.com/LID-DS/LID-DS) | Linux syscall-sequence detection and comparing kernel-context features | Dataset structure differs from the current 128-dim command extractor. Build an event/sequence adapter instead of textifying syscall records as commands. Verify the downloaded dataset's usage terms independently of the loader repository's GPL license. |
| [AgentDojo](https://github.com/sequrity-ai/agentdojo-benchmark) | Indirect prompt-injection and Agent tool-use policy evaluation | This is an agent task/defense benchmark, **not** labeled command-line data. Use it for a separate semantic/rule head and measure task utility and attacker success, not as direct RF command training rows. |

**Suggested order:** review small, diverse samples of CLI Command Risk first;
collect controlled benign commands and genuinely labeled agent sessions next;
use AIT/CAM/LID-DS for separate event-level out-of-distribution checks. A model
that only learns source-specific syntactic templates will not generalize to
exfiltration and multi-step attacks.

## Mandatory reviewed JSONL schema

Each line is a JSON object with `command` (or `commandLine`), `label`,
`label_source`, `scenario_id`, and `run_id`.

```json
{"command":"stat ./build.log","label":"ALLOW","label_source":"human_reviewed","scenario_id":"benign-build","run_id":"build-001"}
```

Allowed labels for the current candidate are only `ALLOW` and `BLOCK`
(`safe` and `unsafe` are accepted as input synonyms). Provenance is limited
to `human_reviewed` or `policy_decision`. Use a human or separately
validated policy to assign per-command actions. Do **not** mark an attack
window's background commands BLOCK; do **not** treat a model-generated risk
severity as an authorization decision.

The curation tool rejects missing provenance, conflicting action labels for
the same command, missing scenario/run IDs, oversized or malformed records,
and insufficient independent split support. It re-derives runtime features
rather than trusting supplied vectors.

## Reproducible candidate workflow

From the repository root, prepare one or more already-reviewed JSONL files.

```sh
python3 scripts/ml-curate-corpus.py \
  --source shell=corpora/reviewed-shell.jsonl \
  --license shell=CC-BY-4.0 \
  --source agent=corpora/reviewed-agent.jsonl \
  --license agent=INTERNAL-REVIEWED \
  --group-by both \
  --output-dir artifacts/curated-ml

python3 -m unittest discover -s scripts -p 'test_ml_curate_corpus.py' -v
sha256sum artifacts/curated-ml/train.jsonl artifacts/curated-ml/test.jsonl
```

Inspect `artifacts/curated-ml/manifest.json` for actual input hashes,
per-action support, removed duplicate commands, and held-out group counts.
Store generated data/artifacts outside Git and verify each source license.
The example license identifiers describe provenance, **not** permission to
redistribute an internal corpus.

From `backend/` (using Go 1.27.3 and generated eBPF bindings):

```sh
go run ./cmd/ml-train \
  --dataset-file ../artifacts/curated-ml/train.jsonl \
  --dataset-sha256 '<TRAIN_SHA256_FROM_MANIFEST>' \
  --dataset-license 'REVIEWED-MIXED' \
  --seed 20261108 \
  --external-test-file ../artifacts/curated-ml/test.jsonl \
  --external-test-sha256 '<TEST_SHA256_FROM_MANIFEST>' \
  --output artifacts/reviewed-decision-model.bin
```

`--dataset-license` is a provenance string. For mixed sources the per-source
licenses and hashes are in the curation manifest; retain it alongside the
model manifest, and do not claim the combined corpus has a unified license.

Candidate training uses a nonzero, documented PRNG seed by default; pass
`--seed` to repeat or compare experiments under identical random sampling.
The chosen seed is stored in `hyperparameters.seed` in the manifest. This
does not itself establish generalization; repeat evaluation over multiple
fixed seeds and independent scenario/time holdouts before selecting a model.

Offline command corpora do not contain trustworthy per-command wall-clock
context. The candidate trainer and external evaluator therefore neutralize
feature dimensions 114–118 (current hour, weekday, cyclic hour, weekend)
after extraction. The deployed runtime extractor still supplies its real
context; constant offline clock features cannot be selected as tree splits.
This avoids training a time-of-day shortcut from the CI runner's clock.

The CLI writes the candidate binary and
`artifacts/reviewed-decision-model.bin.manifest.json`. It reports **internal
grouped holdout** results and (when explicitly provided) an **independent
external test** block containing accuracy, balanced accuracy, macro-F1, BLOCK
precision/recall, benign false-positive rate and confusion matrix.
The external evaluator reloads the candidate, validates both file hashes,
rejects repeated commands within the test and exact train/test command
overlap, **requires nonempty scenario_id/run_id on every row, and rejects
shared scenario or run IDs between train and test**. These checks also apply
when the files were prepared without the curation script. It never trains on
external test rows.

In addition to observed external BLOCK recall and benign FPR, the manifest
reports `blockRecallLower95` and `benignFPRUpper95`, two-sided 95% Wilson
**row-level** uncertainty bounds. Multiple commands from the same scenario
are correlated: these bounds may still be overly optimistic for a new
scenario and must not be interpreted as independent deployment guarantees.
The manifest additionally reports `independentGroups`, `blockGroups`,
`benignGroups`, `groupBlockRecall`, `groupBlockRecallLower95`,
`groupBenignFalsePositiveRate`, and `groupBenignFPRUpper95`. Scenario
and run IDs form transitive connected components, so a run linking two scenarios
does not make two independent observations. Under these **strict component
metrics**, a group containing BLOCK actions only succeeds if **all** its BLOCK
actions are correctly blocked; a group containing benign ALLOW actions has a
false-positive event if **any** ALLOW action is rejected. Mixed-action groups
contribute to both denominators. Wilson bounds are calculated on distinct
components instead of repeated rows; neither the IDs nor this statistical
interval prove real host or time independence. The definitions intentionally
differ from row-level recall and false-positive rates.

A deployment decision additionally needs many distinct, independently
reviewed ALLOW and BLOCK scenarios, not merely many command rows.

### Required interpretation

1. Group by **both** (the default): scenario IDs and run IDs form a
   transitive graph across all source files. Every connected component stays
   entirely inside one partition, so neither a shared run nor a shared
   scenario can leak between train/validation/test. This may substantially
   reduce the number of *independent* groups. When it cannot yield sufficient
   ALLOW/BLOCK support, obtain more separately reviewed scenarios and runs
   rather than weakening the split. The explicit `--group-by scenario` or
   `--group-by run` options isolate only that identifier and should be used
   only when the omitted identifier is not a meaningful leakage key.
2. Keep the external **test** set sealed. Use the separate `validation.jsonl`
   to select hyperparameters; the CLI's internal holdout is only a preliminary
   diagnostic, not a substitute for cross-scenario selection.
3. Stratify/collect enough independently labeled **BLOCK** and **ALLOW** cases
   to estimate both recall and benign FPR with confidence intervals. A perfect
   result on one BLOCK sample is not evidence of production readiness.
4. Report source-specific, temporal and environment-shift results. Compare
   against the pinned baseline at the same false-positive budget. Count
   dangerous BLOCK→ALLOW misses separately; a BLOCK→ALERT prediction is
   not equivalent to actual blocking.
5. Measure latency and policy impact separately before any runtime promotion.
   Neither the script nor CI enables automatic enforcement.

The initial implementation intentionally excludes automated data downloads
from AIT/CAM/LID-DS and automated mapping of their event labels to actions.
Those require dataset-specific, audited conversion code and a reviewed ground
truth contract. CI validates the curation code; real-model accuracy still needs
an actual pinned corpus and complete test run.
