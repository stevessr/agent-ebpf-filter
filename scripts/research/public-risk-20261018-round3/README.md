# Agent risk model research, round 3

Research only; does not change runtime enforcement. See `RESULTS.md` for actual
metrics and `program.md` for the predeclared experiment/selection rules.

## What changes

- Four privacy frontends: Google TinyBERT, all-MiniLM-L6-v2, AstrLink Guard INT8
  token evidence, MiniLM plus Guard; two code frontends: TinyBERT and CodeT5-small.
- Linear, residual and gated-expert decision networks. Full-parameter TinyBERT
  mean/CNN pooling alternatives. Four end-to-end models and 30 head-search trials.
- Local-only Guard evidence; full-process ONNX telemetry opt-out before import.
- OpenCode 1.19.3 safe read-only synthetic fixture trajectories using explicitly
  free GLM-5.3-Flash and DeepSeek-V4.1-Flash. Data labels require observed tool
  completion, expected path, fixture invariance, correct answer and reported $0.

## Reproduce

Keep sibling `public-risk-20261017` and `public-risk-20261018` research directories
from the parent commit. Re-run their preparation/extraction scripts or set
`ROUND1_DIR` and `ROUND2_DIR` to their prepared directories. Round-2 frozen
embedding caches are needed; round-2 fitted heads are not used.

```bash
python -m pip install -r requirements.txt
python prepare.py
python download_guard.py
ORT_DISABLE_TELEMETRY=1 python guard_features.py
python train.py privacy heads
python train.py privacy e2e
python train.py code heads
python train.py code e2e
python train.py privacy freeze
python train.py code freeze
python replicate_controls.py privacy
python replicate_controls.py code
python test_protocol.py
python train.py privacy evaluate
python train.py code evaluate
```

The published run scheduled each task's e2e/head stages in different orders for
CPU utilization. Selection compares the same fixed configurations; preserve the
original append-only ledgers. Never run two search stages for the SAME task at
once. Seeds are 20261019, 20261020, 20261021.

For white data, install `opencode-ai@1.19.3` into a separate local prefix and set
`OPENCODE_BINARY` to its binary. Run `python collect_white.py`. Free availability
is checked by the CLI model list and observed returned cost; no paid fallback.
The collector removes provider credentials from the child environment, uses
separate XDG directories, forbids external-directory access and all non-read tools.
It does not claim OS isolation. Accepted rows cover observed synthetic reads only.

After collection and primary model freeze, run the separately reported ablation:

```bash
ORT_DISABLE_TELEMETRY=1 python white_features.py
python white_ablation.py train
python white_ablation.py evaluate
python export_models.py
python report.py
python verify.py
```

The neural head time budget is checked at epoch boundaries (soft 60 seconds);
linear controls use max_iter=1200. No test scores govern stopping. The augmentation
uses 15 accepted rows at weight 1, with architecture fixed from the main search.

## Data and interpretation

PrivacyAlign is public synthetic agent-context data with human candidate leak
annotations (Apache-2.0). Devign is code-defect proxy data under the CodeXGLUE
C-UDA terms and upstream project licenses. Preserve round-1 source revisions,
licenses and filters. Defect prediction is not proof of exploitable vulnerability
or destructive agent behavior. The new white traces have privacy/destruction
labels 0 only for these observed read-only tasks; code-vulnerability labels unknown.

Round-3 audit is a new group split within previously used public training data,
not an independent external benchmark. Previous confirmation and old test are
regression only. Architecture is chosen on validation; FPR threshold is calibrated
separately. A <=5% calibration FPR does not guarantee <=5% population FPR.
Actual harmful/destructive-action outcome detection still needs matched positive
and negative execution traces. No production gate or enforcement is enabled.

## AstrLink reference and limits

Inspected Calcium-Ion/AstrLink at `989ff6cea8bc6483fa1c9980ece949e288a4c7b8`
and QuantumNous/astrlink-guard at
`56988fbffdd37d4b0f477066457ddab8b197b3dc` (v0.1.2 weights).
The public model card describes a 12-layer multilingual MiniLM token classifier,
fictional assistant-request/code/log samples and public Nemotron-PII,
nym-pii-multilingual-data and CredData sources. Training examples/recipe are not
distributed there; this work does not claim to replicate them or reuse their
in-house evaluation scores as our measurements.

The inspiration is hard-negative handling (log dates, versions, IDs), multiple
input domains, contextual entity evidence and explicit false-positive reporting.
Our Guard frontend uses token category probability aggregates, not their complete
BIO/offset/URL cleanup decoder. It is a new evidence frontend evaluated on our
leak task, not a replication of AstrLink's NER benchmark. PII presence alone does
not determine authorization or leakage. Three 384-token windows per text can
omit content; coverage is recorded in extraction metadata.

Sources:
- https://huggingface.co/QuantumNous/astrlink-guard
- https://github.com/Calcium-Ion/AstrLink
- https://huggingface.co/datasets/nvidia/Nemotron-PII
- https://huggingface.co/datasets/Wismut/nym-pii-multilingual-data
- https://github.com/Samsung/CredData
- https://opencode.ai/docs/permissions/
- https://opencode.ai/docs/cli/
- https://opencode.ai/v2/docs/console/models/
- https://github.com/microsoft/onnxruntime/blob/main/docs/Privacy.md

Source/report/manifests/ledgers are in Git. Public raw corpora, downloaded base
weights, intermediate features and executable pickle objects are excluded.
Released trained checkpoints use safetensors plus JSON metadata.
