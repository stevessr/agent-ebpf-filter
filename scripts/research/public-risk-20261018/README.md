# Round 2: new encoders with bounded automatic optimization

This round replaces the generic 2-layer BERT representation with two task-specific pretrained encoders:

- **Privacy:** `sentence-transformers/all-MiniLM-L6-v2`, revision `1110a243fdf4706b3f48f1d95db1a4f5529b4d41`.
- **Code:** `Salesforce/codet5-small` encoder, revision `b1ee9570c289f21b5922b9c768a1ce12957bf968`.
- **Matched control:** the original Google 2-layer BERT **pretrained** checkpoint, not the previously finetuned checkpoint. It is encoded with the same new representation rules and its head is trained on the same new training split.

The pretrained encoders are frozen. Linear and two-hidden-layer neural classification heads are actually trained; no full encoder/LLM finetuning is claimed in this round. Frozen representation caching permits a bounded, auditable CPU experiment search.

## Research translated into this experiment

[autoresearch](https://github.com/karpathy/autoresearch) motivates an immutable evaluation/data component, bounded experiment budget and keep/discard ledger. The original project is GPU pretraining code; we implement the experimental pattern in this classifier pipeline. This is a finite prescribed search, not a claim that an LLM autonomously invents training-code edits. [The HPO comparison](https://arxiv.org/abs/2603.24647) motivates keeping conventional bounded hyperparameter searches as controls.

[AutoProver](https://github.com/Certora/AutoProver) and [AIProver](https://arxiv.org/abs/2610.05367) motivate proposing a candidate, checking explicit conditions, retaining diagnostics and revising. [AutoSciRub](https://arxiv.org/abs/2608.31076) motivates defining executable acceptance criteria before running the research. These are methodological adaptations. No Solidity/Lean prover validates our neural predictions, and the resulting certificate does not formally prove model correctness.

`program.md` fixes the budget, data boundary, selection objective and deployment gate before search.

## Data protocol

Data sources and human annotation mapping are inherited unchanged from [round one](../public-risk-20261017/README.md). PrivacyAlign scenarios remain synthetic with human annotations. CodeXGLUE/Devign remains a code-defect **proxy**, not evidence that an Agent introduced a vulnerability or caused destruction. Harmful request recognition and actual destructive-action outcomes remain different tasks.

This round addresses the failed direct privacy and code tasks. It does not re-label general harmful requests as actual destructive operations.

The old published test has already been inspected. It is reported only as **regression**, not as an untouched independent test. Before any new head training, `prepare_round2.py` reserves about 20% of old training groups as **confirmation**. Those groups are excluded from all round-two fitting and search, including the matched tiny control. This is a within-corpus, within-round holdout, not a new external dataset or a guarantee against pretraining contamination.

Code training is capped at 4,500 functions by deterministic group sampling; validation and calibration are capped at 800 each. Confirmation and regression are not sampled down. Privacy uses all remaining eligible data. Source hashes and group counts are recorded.

## Representation changes

Privacy candidates, user request/memories and preceding tool history are encoded separately. Each unit uses 190-token chunks; up to two candidate chunks, two request/memory chunks and six uniformly distributed history chunks are retained. Retrieval is a softmax-weighted context mean based on candidate/context cosine signals. Feature views compare candidate only, candidate + source + absolute difference + product, and the latter plus retrieved/global history. Retrieval uses no annotations or outcome labels. A separate evidence view adds 16 fixed, label-free lexical measurements (candidate/source/request word, number and email overlap, text length and category-word counts). Exact matches are evidence features, not proof of authorization. Candidate-only, retrieval-only and evidence-augmented views are compared. This augmentation was registered before any head-search trial.

Coverage is bounded and reported. This is not lossless full-context reasoning, and retrieval similarity is not a proof of causality or authorized data flow.

Code uses the first 64 and last 126 tokens if a function exceeds 190 tokens, then obtains normalized mean and max hidden-state pooling. The matched tiny model uses exactly the same rule. This tests encoder differences while retaining a documented truncation limitation.

## Optimization

- Eight fixed configurations per base/task, 16 per task total. Linear regularization/view choices and MLP view/LR choices are listed in `autosearch.py`.
- Each head trial has a 30-second fit/selection timeout; encoder extraction is a separate, reported cost. MLPs have up to 60 epochs, validation AP early stopping and patience seven.
- Thresholds come only from calibration: maximize balanced accuracy with empirical FPR<=10%. Also report a calibration FPR<=5% point.
- Keep/discard objective is validation balanced accuracy minus `0.5*max(0,FPR-0.20)`. No F1-optimal nearly-all-positive operating point is used.
- The earlier round optimized an F1 operating point and used different training counts. Its raw numbers must not be attributed solely to changing the base; the main comparison is the matched tiny control in this round.

- Winners are written and SHA-256 frozen before confirmation/regression evaluation. Two additional head seeds repeat the selected configuration, without reselecting architecture.
- All trials, failures, timings and seed results are preserved. A rerun refuses to overwrite an existing ledger or search after evaluation.
- Deployment acceptance is reported only: confirmation recall>=70% and FPR<=5%. A failed gate remains a failed result. This experiment does not change runtime/kernel policies.

## Reproduction

Start by installing round-one dependencies, downloading its fixed data and running its data preparation. The directory resolution accepts the sibling `public-risk-20261017` or `ROUND1_DIR`.

```sh
python -m pip install --index-url https://download.pytorch.org/whl/cpu 'torch==2.14.1+cpu'
python -m pip install -r requirements.txt
python download_models.py
python prepare_round2.py
python verify.py
python extract.py --task privacy --base new
python extract.py --task privacy --base tiny
python extract.py --task code --base new
python extract.py --task code --base tiny
python augment_evidence.py new
python augment_evidence.py tiny
python verify.py
```

For each task `privacy`/`code` and each base `new`/`tiny`, run:

```sh
python autosearch.py --task privacy --base new --stage search
```

Only after **all four** searches have frozen their winners, run the corresponding four `--stage evaluate` commands. Then run `verify.py`, `export_heads.py` and `report.py`.

The exact commands may be scheduled concurrently within available CPU/RAM. No remote GPU/API spending or private data upload is used. Corpus contents are never executed.

## Artifacts and limitations

`artifacts/*_trials.jsonl` is the keep/discard ledger; `*_frozen.json` is the pre-evaluation selection record; `*_results.json` records confirmation, regression, group bootstrap intervals and three head seeds. `certificate.json` checks file hashes, group isolation, finite embeddings and frozen checkpoint hashes. It does not certify labels or semantic security.

The confirmation distributions are small and from the same public corpora. Repeated validation search can still overfit; multiple seeds do not remove annotation errors, correlated scenarios or unknown pretraining overlap. English-dominant performance does not establish Chinese performance. Three-seed variation reflects the classifier head, not variation from training a new encoder.

Head bundles created by joblib are trusted-local research artifacts; exported numeric JSON heads avoid requiring arbitrary pickle loading for exchange. Encoders are obtained from pinned publishers. Dataset and base-model terms remain applicable; do not infer unrestricted deployment rights from the experiment's source-code license.
