# Public-data deep optimization, 2026-10-18

This study actually fits learned models and invokes the resulting weights. It
extends the [previous frozen-weight failure analysis](../model-opencode-20261018/RESULTS.md).
All candidates remain **shadow-only**. Model quality and syscall enforcement are
separate properties. No attack payload is executed or sent to a destination.

## Training and automatic features

- Privacy: public PrivacyAlign human majority labels, uncertain/tied votes
  excluded; 1,524 train, 253 validation, 236 calibration, 382 consumed regression
  examples. Scenario groups remain disjoint. Compare pretrained BERT-Tiny
  (2 layers, width 128) and **BERT-Mini (4 layers, width 256)**. Each receives six
  full-parameter epochs, unweighted BCE plus opposite-label within-scenario pair
  ranking. Context retrieval is label-free. Context/candidate pools, their
  absolute difference and product feed a two-layer interaction head. Also fit
  12 train-vocabulary TF-IDF/logistic alternatives across two views, two lexical
  encoders and three regularization settings. This does not guarantee a neural
  model will beat the sparse candidates.
- HTTP lexical: 111,181 public ECML/CSIC requests; normalized lexical templates
  stay together across sources. Compare payload, parameters and syntax views;
  word/character TF-IDF with logistic regression, or count/ratio features with
  histogram boosting: 18 fits. Vocabulary fitting uses train only. Model
  selection maximizes the **worse-source** validation recall at empirical 1%
  FPR; source-wise calibration fixes a shared conservative threshold.
- Go feature comparison: call the real `kernel-http-behavior.v2` cleaner on all
  requests. Derive 411 values from its 137 features: log counts, normalized
  counts, hint interactions and explicit presence indicators. A train-only
  ExtraTrees importance selector removes constants/correlated features and
  selects at most 32; three histogram-boosting configurations are fitted.
  This is a separate, frozen follow-up family after the lexical generalization
  failure. The selected lexical model is preserved. Missing read/write/exec,
  provenance and process-history fields remain missing; HTTP corpus labels are
  never promoted to syscall destruction or privacy ground truth.
- Versioned correction: two more lexical fits preserve literal `#` request data
  and retain body spaces. A second Go family fixes field-specific ratio
  denominators and uses the minimum source-conditional importance from two
  train-only forests before three boosting fits. Earlier families and their
  failures are preserved. The new rule-ID holdout uses different CRS files and
  removes requests duplicated in the first audit; it shares the same source,
  and is not described as an independent second attack corpus.

The empirical 1% calibration objective is **not** a statistical guarantee. The
privacy calibration set contains only 99 negatives; even zero observed errors
cannot establish a 1% FPR upper bound. Shared-source synthetic traffic, repeated
templates, label uncertainty and distribution shift remain material limitations.
Sigmoid outputs are ranking scores; threshold calibration here does not make
them validated probabilities of a real-world privacy or attack outcome.

## Evaluation boundaries

Previously viewed PrivacyAlign, ECML and CSIC tests are explicitly **consumed
regression** sets. CSIC test traffic is now development data and is no longer
called an independent external test. Original CSIC normal training traffic is
opened after the HTTP freeze; requests sharing a payload with any development
example are removed. Its remaining samples still share a generator/application;
the audit additionally reports unique-payload and unseen-template subsets.

Two additional public sources are evaluation-only:

- [PiSAs](https://huggingface.co/datasets/ServiceNow/PiSAs), revision
  `8a1ba30ba210d4a430bbd25cf4a3500d0e22c5b9`, Apache-2.0. First five lexical
  scenarios per each of nine domains, selected using names only. The transfer
  diagnostic adapts each annotated attribute to a JSON message action. Labels
  describe **task appropriateness**, not original agent execution outcomes.
  Only task text and artifact author/content enter the context. Appropriateness
  enums, visibility lists, private archetypes and oracle utility are excluded
  from inputs. These are not the original PiSAs benchmark scores.
- [OWASP CRS](https://github.com/coreruleset/coreruleset), v4.29.0 commit
  `ab3ccd5fcd691424ba3f320d4040c61417270193`, Apache-2.0. The offline challenge
  includes only explicitly expected target-rule detections in supported
  single-stage scalar requests. A negative test for one rule is **not** labelled
  globally benign. Unavailable predeclared rule files and exclusions are counted.
  No CRS engine, web application or exploit is run. This measures learned
  threshold hits on expectation-positive strings, not actual interception.

Four new real OpenCode sessions use `opencode/deepseek-v4.1-flash-free` after
model/threshold freeze. Each before-tool hook calls the frozen privacy model;
the loopback server separately invokes both learned HTTP families on requests
it actually receives. Task completion, native call IDs, log hashes and model
digests are reconciled. Ordinary requests and legitimate code-documentation
  uploads are reported separately. Four HTTP families are scored on the same
  sixteen received requests; repeated model scoring never multiplies the
  actual-request denominator. All are disposable fixtures without real
secrets. OpenCode transport health is not model accuracy; threshold hits never
become production BLOCK decisions.

## Reproduce

Use Python 3.12 and the pinned CPU requirements in a virtual environment. The
observed environment is recorded in `provenance.json`. For the prepared weight
bundle, use this PR's source code and unpack the bundle into `round`; rebuild
the cleaner only with a matching digest or explicitly create a new experiment.
Joblib artifacts are trusted local research outputs, not an untrusted upload API.

From the repository root (Go 1.27.1):

```bash
python3 -m venv /tmp/risk-env
/tmp/risk-env/bin/pip install -r scripts/research/deep-optimization-20261018/requirements.txt
export PYTHONPATH="$PWD/scripts/research/deep-optimization-20261018"
export OMP_NUM_THREADS=2 OPENBLAS_NUM_THREADS=2
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/download.py --assets /tmp/risk-assets --data /tmp/risk-public-http --root /tmp/risk-round --evaluation
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/prepare_public.py --assets /tmp/risk-assets
(cd backend && go build -o /tmp/risk-round/behavior-features ./cmd/behavior-features)
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/privacy.py --assets /tmp/risk-assets --mini /tmp/risk-round/base_mini --out /tmp/risk-round/privacy
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/http_model.py --data /tmp/risk-public-http --out /tmp/risk-round/http
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/structured.py --data /tmp/risk-public-http --extractor /tmp/risk-round/behavior-features --out /tmp/risk-round/http_structured
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/challenge.py --root /tmp/risk-round --data /tmp/risk-public-http
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/corrected.py --root /tmp/risk-round --data /tmp/risk-public-http
/tmp/risk-env/bin/python scripts/research/deep-optimization-20261018/challenge.py --root /tmp/risk-round --data /tmp/risk-public-http --corpus crs_v2 --families http http_structured http_v2 http_structured_v3
BEHAVIOR_EXTRACTOR=/tmp/risk-round/behavior-features /tmp/risk-env/bin/python -m unittest discover -s scripts/research/deep-optimization-20261018 -p 'test_*.py' -v
```

For `evaluate.py`, the threshold-only legacy baseline additionally requires the
previous round's two digest-pinned checkpoints and `selected_models.json` in
the original public assets directory. Those are not silently substituted with
new weights. `live.py` additionally takes `--root`, `--assets` and an absolute
`--opencode` path to OpenCode 1.19.3; isolate its XDG state as the harness does.
`audit.py` takes `--root`, `--assets` and `--data` and must pass before publishing
results. Training can take tens of minutes on CPU; no GPU or paid API is used.

PrivacyAlign and the Google miniature bases carry Apache-2.0. The HTTP mirror's
curator declares GPL-3.0; retain original ECML/CSIC attribution and verify the
underlying dataset permissions for any production redistribution. Public access
alone is not a production model licence. See [results](RESULTS.md), including
the failed gates and model hashes. No current backend enforcement model is
replaced by this research path.
