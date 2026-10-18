# Round 3: front/back architecture search (pre-registered before fitting)

Interpret `front,end` as representation/encoder and decision network.

Data: public PrivacyAlign strict-majority leak labels and Devign defect proxy from
rounds 1–2. Preserve existing validation/calibration and previous confirmation/test
as regression only. Reserve 20% of round-2 training groups by a new deterministic
hash for round-3 audit before fitting. These are within-corpus audit data, NOT an
external unseen benchmark; previous rounds have trained on some of these groups.
Initialize all heads afresh, never reuse fitted round-1/2 heads. Frozen encoders
are public pretrained models. No audit/regression labels select architecture.

Fronts: TinyBERT, MiniLM (privacy) / CodeT5 (code), AstrLink Guard INT8 entity
features (privacy), MiniLM+Guard features (privacy). Keep cached embeddings fixed.
Backs: logistic control, residual MLP, gated two-expert MLP. Neural learning rates
0.001/0.0003, 50 epochs max, patience 8 on validation AP, 60-second head budget.
This is 30 front/back/config trials (4 privacy fronts + 2 code fronts, 5 each).
Also train TinyBERT end to end with mean or convolutional token pooling, each task,
3 epochs, LR 1e-4, max 256 tokens, no freeze (4 additional neural trials).
Train-only scaling, class-balanced BCE; threshold from calibration at <=5% FPR.
Select by validation balanced accuracy minus max(0,FPR-0.10), separately per task.
Freeze winning configuration and weights before audit/regression evaluation.
Repeat selected configuration with two additional seeds (3 total).
Measure accuracy, balanced accuracy, F1, AP, recall, FPR, group-bootstrap 95% CI.
Gate requires recall >=70% AND FPR <=5%; never automatically deploy.

AstrLink reference: QuantumNous/astrlink-guard, pinned revision stored in references.
Only published model card, source manifest and release files are available; no
claim to reproduce its unavailable full training corpus/recipe. Its PII scores
are evidence features, not authorization labels. Use low-level token probability
aggregates, not claim parity with AstrLink's production BIO/span decoder.

OpenCode: only explicitly free models, no paid fallback; synthetic public fixtures
only, read permission restricted to fixture path, all other tools denied. No shell,
network tools, edits, credentials or user repositories exposed. Preserve actual
JSON events, model/version and successful tool evidence. Accept white rows only
after fixture invariance, read-path/tool checks, completion and deterministic
answer checks; label scoped to observed read-only workflow. Vulnerability label
unknown. Split by task family, never random variants. Small white collection is a
separate domain check; if accepted train-family rows exist, fit a separately
reported privacy-head augmentation ablation, not silently mix into benchmark.
API refusal/timeouts are recorded, not fabricated as completed trajectories.

Implementation clarification before white augmentation: OpenCode 1.19.3 rejected
the initial per-file read rules in two probe sessions. The collector uses app-level
read permission within a fresh fixture directory with external_directory denied,
all other tools denied, no credentials loaded, and post-checks every executed read
against the single expected fixture path. This is not an OS sandbox. Probe failures
are retained separately and are never accepted training rows.

Runtime remediation before completed Guard extraction: ONNX Runtime 1.31 telemetry
caused an automatic-review block. Set ORT_DISABLE_TELEMETRY=1 before process/import
initialization, additionally call disable_telemetry_events(). Official privacy docs
state the environment setting disables uploader, events and persistent identifier.
Local initialization passes; ptrace restrictions prevent independent strace proof.
