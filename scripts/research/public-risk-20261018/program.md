# Round 2: bounded automatic risk-model experiments

Goal: replace generic 2-layer BERT with semantic MiniLM and code-pretrained CodeT5 encoders, then test whether candidate/context representations and trained heads improve risk discrimination.

## Before execution

- Pin public data and model revisions. Keep the old test as a previously viewed regression set.
- Reserve confirmation groups from the old training portion BEFORE new-base/head training. These are held out for this round; they are not a newly collected external benchmark.
- No confirmation/regression labels may select architecture, weights, representation, threshold or stopping time.
- Freeze public encoders in the feature-cache stage. Train linear and deep classification heads; do not describe this stage as full backbone finetuning.
- Use CPU only. Maximum head-trial budget: 30 seconds excluding feature extraction, at most 16 configurations/task; separate encoder extraction cost from head search cost.

## Executable rubric / certificates

1. All group pairs across train/validation/calibration/confirmation/regression are disjoint.
2. Input features depend only on code or candidate + preceding context. Labels, annotations, IDs, risk/decision outputs never enter the encoder.
3. All transforms that learn from labels or data distribution are fit on train only.
4. Pick thresholds on calibration only: maximize balanced accuracy subject to empirical calibration FPR<=10%. Also retain 0.5 and a calibration FPR<=5% point.
5. Keep candidates by validation balanced accuracy minus 0.5 * max(0, validation FPR-20%). Retain all failed/non-improving trials in an append-only results log.
6. Logistic regularization and MLP architecture/LR/dropout are bounded; neural checkpoints may use validation AP for early stopping. This finite search is an implementation inspired by autoresearch, not a claim to run its GPU pretraining code or an LLM-driven unrestricted code search.
7. Freeze winners and their hashes before scoring confirmation/regression. Evaluate selected models for three head-training seeds. Report grouping uncertainty and seed variation.
8. Deployment gate (report only): confirmation recall>=70% and FPR<=5%. Failure must be preserved. No model automatically changes kernel policies.

Verification means executable experimental checks, not formal proof that a neural classifier is correct. AutoProver/AIProver inform the propose/check/diagnose/revise structure; no Certora/Lean prover is run on these predictions.

Sources:
- https://github.com/karpathy/autoresearch
- https://arxiv.org/abs/2603.24647
- https://github.com/Certora/AutoProver
- https://arxiv.org/abs/2610.05367
- https://arxiv.org/abs/2608.31076

Pre-search representation amendment (before any head trial): privacy includes an optional 16-dimensional observable source/candidate lexical relation block. It uses no labels. Keep the eight-per-base trial budget by comparing candidate/retrieval/evidence linear views and context/evidence MLP views.
