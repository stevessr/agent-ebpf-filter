# Public data training for Agent eBPF Filter

This experiment replaces the earlier hand-written policy fixtures with public datasets. It performs **full-parameter CPU finetuning** of a 2-layer, hidden-size-128 BERT encoder and a GELU classification head. It is not a 7B LLM/LoRA run. The online eBPF collector, legacy 128-dimensional contract, policy maps and automatic blocking are not changed.

## Four supervised evaluations

| Evaluation | Public input | Target | What the result does not establish |
|---|---|---|---|
| Agent tool-action privacy | PrivacyAlign: user request, memories, preceding tool trajectory and candidate final action | Majority human `leaks` vote, ambiguous/tied votes excluded | Real-user deployment accuracy or full-context reasoning |
| Auxiliary privacy | BeaverTails 30k prompt + candidate response | Existing `privacy_violation` label | Actual file/network data exfiltration |
| Auxiliary vulnerability | CodeXGLUE/Devign C/C++ function | Existing `target` defect label | Vulnerability introduction by a particular Agent, exploit success or CVE correctness |
| Auxiliary harmful request | AgentHarm harmful/benign prompt pairs | Requested harmful behavior vs matched benign task | Whether an Agent complied, caused damage, or crossed an authorization boundary |

PrivacyAlign scenarios are synthetic and **publicly released with human annotations**. They are not the earlier locally generated 4,800 policy fixtures. BeaverTails consists of generated responses with human safety labels. These distinctions matter: public-data training is not the same as training on production eBPF trajectories.

## Reproduce

Use Python 3.12 in an isolated environment. The exact package versions are in `requirements.txt`; install CPU PyTorch from its official wheel index when appropriate.

```sh
python -m pip install --index-url https://download.pytorch.org/whl/cpu "torch==2.14.1+cpu"
python -m pip install -r requirements.txt
python download.py
python prepare.py
python prepare_agent_privacy.py
OPENBLAS_NUM_THREADS=2 OMP_NUM_THREADS=4 python -u train_public.py --stage train > training.log 2>&1
OPENBLAS_NUM_THREADS=2 OMP_NUM_THREADS=4 python -u train_public.py --stage evaluate > evaluation.log 2>&1
OPENBLAS_NUM_THREADS=2 OMP_NUM_THREADS=4 python -u train_agent_privacy.py > agent_privacy_training.log 2>&1
OPENBLAS_NUM_THREADS=2 OMP_NUM_THREADS=2 python -u train_code_mlp.py --stage train > code_mlp_training.log 2>&1
OPENBLAS_NUM_THREADS=2 OMP_NUM_THREADS=2 python -u train_code_mlp.py --stage evaluate > code_mlp_evaluation.log 2>&1
python build_report.py
python verify.py
```

Run commands from this directory. Downloads use immutable Hugging Face revisions in `download.py`. No corpus text is executed. No API key is used, no private trace is uploaded, and no public evaluation service is called. CPU memory limit in the actual run was 8 GiB with an 8-CPU quota; Torch uses four intra-op threads.

## Train/validation/calibration/test isolation

- **BeaverTails:** preserve official 30k test, remove all matching normalized test prompts from official training; hash remaining prompt groups into 75% train, 13% validation, 12% calibration. Responses to one prompt stay together. Exact candidate text is deduplicated.
- **Devign:** preserve official test except exact duplicate code; remove its commit/code overlaps from validation and training; remove validation commit/code overlaps from training. Reserve approximately 14% of remaining training commits for calibration. This is stricter than the published random split, and **scores are not directly comparable to the original leaderboard protocol**.
- **AgentHarm:** re-split the public test corpus into local train/validation/test by `id_original`. Both classes and all four hint/detail variants stay together. This consumes part of the public evaluation corpus for training, so the experiment is **not an official AgentHarm benchmark score**. No dedicated calibration split is available. Only 7 original behaviors remain in local test.
- **PrivacyAlign:** preserve official test scenarios; reject matching train scenarios, then hash context/scenario groups into train/validation/calibration. Both candidate actions share a group. Ignore `unsure` annotations; exclude candidates without a strict majority on leakage. Preference winner is not used as a leakage label. Annotations and model provenance are never model input.

Group bootstrap uses 500 replicates for positive-class F1 intervals. It does not measure seed-to-seed training uncertainty. This run uses one random seed, `20261017`. Near-duplicate paraphrases/code versions across different groups have not been exhaustively eliminated. Pretraining corpus contamination cannot be excluded.

## Optimization and selection

The auxiliary encoder is compared in two predeclared runs:

1. Unweighted BCE, LR 3e-4, 3 epochs.
2. Per-task square-root class-ratio weighted BCE (capped at 5), LR 1e-4, 5 epochs.

Both use AdamW, weight decay 0.01, gradient clipping at 1, batch size 64, max 192 tokens, three task-specific logits and masked task loss. The small AgentHarm training subset is repeated eight times for optimizer exposure; these repetitions are **not additional independent data**. Checkpoints are selected by mean validation average precision, never test accuracy.

For PrivacyAlign compare the pretrained encoder against transfer from the weighted auxiliary encoder. Both undergo full finetuning for 5 epochs at LR 1e-4, batch 32, selected by validation AP; transfer additionally uses class-ratio BCE weighting. This is a combined optimization, not a clean causal ablation of transfer alone. A frozen auxiliary checkpoint and per-task TF-IDF/logistic models are controls.

PrivacyAlign packs up to 190 context tokens and 126 candidate tokens, retaining both beginning and end of each segment in a 320-token sequence. This preserves candidate visibility but discards much of long histories. Truncation counts are reported, and conclusions must not imply full-trajectory reasoning. The auxiliary model uses standard leading truncation at 192 tokens, which can truncate long functions and responses.

Calibration is sigmoid/Platt logistic regression on a separate calibration split. Decision thresholds maximize positive F1 on validation, with test untouched. This operating point can have a large FPR; it is **not a safe default for blocking**. Metrics include accuracy, balanced accuracy, positive F1, macro F1, precision, recall, FPR, AP, ROC-AUC and Brier. Both selected-threshold and 0.5-threshold results are retained.

Additional predeclared candidate-only TF-IDF and validation-selected convex score blends compare whether context helps and whether combining neural/lexical scores helps. Blend weight is selected from {0, .25, .5, .75, 1} on validation AP, including pure-model endpoints; test labels do not select the blend. The frozen auxiliary transfer control receives calibration/threshold fitting, so it is not a strictly zero-shot calibrated classifier.

TF-IDF vocabulary and IDF fit on train only. `C ∈ {0.25, 1, 4}` is selected by validation AP. A deep model is not assumed to beat a simpler baseline. Results and failures are reported rather than hidden.

## Sources and usage constraints

| Source | Revision | License/card |
|---|---|---|
| [PrivacyAlign](https://huggingface.co/datasets/ServiceNow/PrivacyAlign) | `edaf507a2d1019b03b592b92a60d113b7eb46425` | Apache-2.0; synthetic scenarios, human privacy annotations |
| [BeaverTails](https://huggingface.co/datasets/PKU-Alignment/BeaverTails) | `8401fe609d288129cc684a9b3be6a93e41cfe678` | CC BY-NC 4.0; safety classifier research, not dialogue generation |
| [CodeXGLUE defect detection](https://huggingface.co/datasets/google/code_x_glue_cc_defect_detection) | `69bd48c03223c2104342acd9a807caf61ac3efb8` | Dataset card: C-UDA; upstream code retains applicable source terms |
| [AgentHarm](https://huggingface.co/datasets/ai-safety-institute/AgentHarm) | `e23b3fe60a0da9037314b88e5ee3a0c054970dad` | MIT with additional safety/security-only use clause |
| [Google small BERT](https://huggingface.co/google/bert_uncased_L-2_H-128_A-2) | `30b0a37ccaaa32f332884b96992754e246e48c5f` | Apache-2.0 |

The experiment is for defensive safety research. Shared-encoder and transferred weights have mixed-source training history including noncommercial data; do not label all resulting weights as unrestricted Apache-2.0. Public source data are downloaded from their publishers instead of copied into the git repository. Agent-SafetyBench was inspected but not used as a binary outcome dataset; R-Judge was not used because no explicit dataset license was established.

## Outputs

`artifacts/*history.json`, `training_config.json`, `results.json`, `agent_privacy_results.json`, test prediction JSONL, data split manifests and the readable `RESULTS.md` provide the audit trail. Model weights use `safetensors`. Baseline `joblib` files must only be loaded from this trusted experiment; `joblib` is a pickle-based format and is not a safe interchange format for arbitrary downloads.

The experiment should remain offline/shadow-only. It does not implement uncertainty abstention, trusted authorization interpretation or a calibrated probability of a production incident. Chinese and multilingual behavior remain untested. The next deployment-relevant dataset is independently labelled event-cutoff Agent/eBPF replay data, not more random fixture expansions.

## Additional targeted code architecture

After observing weak transformer **validation** performance on code, a separate TF-IDF → train-only TruncatedSVD(256) → StandardScaler → 256/128/64/1 GELU MLP is trained. LR {1e-3, 3e-4}, at most 30 epochs, patience 5 and best validation AP choose the run/checkpoint. This is an adaptive validation-driven architecture experiment, not a predeclared clean ablation and not test-driven tuning. The representation is fit solely on training functions. Its evaluation uses the same held-out commits and calibration/threshold procedure. The SVD/TF-IDF preprocessing must accompany MLP weights for inference.
