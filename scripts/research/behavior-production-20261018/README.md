# Production qualification of behavior scoring (2026-10-18)

**Rejected for production enforcement.** This is a reproducible public-data experiment plus a bounded shadow worker. See [full report and pilot/rollback instructions](../../../docs/architecture/behavior-shadow-production.md). Scores do not authorize behavior.

The prior 137-feature behavioral extractor remains an auditable structural view. This experiment adds a bounded lexical HTTP-request model; it does not silently replace kernel privacy/destructive heads with request labels. No external website is contacted by training/evaluation. The kernel fixture uses only dummy files and loopback.

## Reproduce

Python dependencies are pinned by `../behavior-features-20261018/requirements.txt`. Go version follows `backend/go.mod`. Provision a research host with >8 GiB memory: the complete training matrix has ~350M nonzeros. The online worker is independently bounded; research training has not been converted to out-of-core fitting.

```bash
python3 -m pip install -r scripts/research/behavior-features-20261018/requirements.txt
git clone https://github.com/msudol/Web-Application-Attack-Datasets.git /tmp/request-data
git -C /tmp/request-data checkout 424c6e1c1951570473d9d6c0227eaaac6fdd6d02
(cd backend && go build -trimpath -o /tmp/behavior-risk ./cmd/behavior-risk)
python3 scripts/research/behavior-production-20261018/train_production.py --data /tmp/request-data --worker /tmp/behavior-risk --out /tmp/behavior-study
# A separate invocation is deliberate: the training process releases its matrices first.
python3 scripts/research/behavior-production-20261018/evaluate_frozen.py --data /tmp/request-data --worker /tmp/behavior-risk --out /tmp/behavior-study
python3 -m unittest discover -s scripts/research/behavior-production-20261018 -p test_production.py -v
```

Use a **new output directory** for a new experiment. Do not overwrite a frozen study or tune after seeing external/test results. `evaluate_frozen.py` checks both the model and protocol hashes. For this original run only, `--training-log` can recover trial metrics from its original stdout because it predated structured `trials.json` output. The original fit completed before the evaluation memory fix; the evaluator did not refit or recalibrate. Sorted L2 accumulation was later added for byte-stable vectors; frozen prediction replay verifies no threshold-decision change. Refit numeric bytes may differ with that roundoff fix, dependency/platform versions or solver behavior; the original model is identified by its frozen SHA, not by a claim of universal bitwise fitting reproducibility.

Files committed here: protocol, freeze, measured results, runtime benchmark, input/calibration tests, benign fixture and parser. Large/sensitive derived representations, raw corpora and Python pickles are not committed. The separately delivered model bundle contains inert JSON weights, split hashes/indices, predictions and manifests; no attack executable or raw dataset. The `private_splits.json` name means an evaluation audit file, not user-private source data.

## Interpretation

- Internal ECML template holdout: FPR 0.683%, recall 97.819%.
- External CSIC: FPR **100%**, recall 100%; all 36,000 benign examples fail. Deployment gate fails.
- Public data are historical generated requests, not representative production traffic. Prior parameter studies shared CSIC ancestry; the current fitted model trains only on ECML.
- Independent privacy leakage and destructive-behavior accuracy is **not measured**. No real eBPF pipeline or production host is validated by a strace-only benign fixture.
- Model checksum: `b53b70d7f7bdf86ff0d24712ad6c10667f47463daf1abf4725e45c1a8d11a25a`.

Preserve source dataset attribution and the curator's GPL-3.0 terms when obtaining/redistributing data. Dataset availability does not itself establish downstream weight distribution permissions.
