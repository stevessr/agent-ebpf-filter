# Actual trained-model evaluation on OpenCode (2026-10-18)

This study runs the **actual frozen safetensors and Go linear-model weights**. No literal canary rule or policy classifier participates. Models stay advisory: threshold hits are counterfactual intervention candidates, not actual BLOCKs. See [RESULTS.md](RESULTS.md).

Weights are reused from the previous research bundles; they are not re-trained, downloaded from an unverified origin, or duplicated in git. `--assets` must contain `artifacts/selected_models.json`, the two selected safetensors, and `base_model/{config.json,vocab.txt}`. The script verifies the frozen checkpoint/tokenizer/config hashes and uses the original token limits, architecture, calibration and thresholds. Dataset/weight source and usage constraints remain those of the original public-risk experiment (PrivacyAlign, BeaverTails, Devign and AgentHarm); this report does not grant unrestricted production licensing.

```bash
uv venv --system-site-packages /tmp/model-test-env
uv pip install --python /tmp/model-test-env/bin/python 'torch==2.14.1+cpu' --index-url https://download.pytorch.org/whl/cpu
uv pip install --python /tmp/model-test-env/bin/python 'transformers==4.57.6' 'tokenizers==0.22.2' 'safetensors==0.8.0' 'huggingface-hub==0.36.2' 'numpy==2.3.5'
npm install --prefix /tmp/opencode-tools opencode-ai@1.19.3 --no-audit --no-fund
(cd backend && go build -o /tmp/behavior-risk ./cmd/behavior-risk)
/tmp/model-test-env/bin/python scripts/research/model-opencode-20261018/live.py \
  --assets /absolute/public-risk-assets \
  --opencode /tmp/opencode-tools/node_modules/.bin/opencode \
  --http-worker /tmp/behavior-risk --http-model /absolute/http_request_model.json \
  --out /tmp/model-live-results
/tmp/model-test-env/bin/python scripts/research/model-opencode-20261018/audit.py --live /tmp/model-live-results
```

`live.py` creates four new OpenCode/free-model sessions (unit test, bug fix, C review, HTTP test). Its pre-tool plugin sends actual tool arguments and the real generated-fixture task context to a local model service. The service loads both BERT checkpoints once, performs forward inference for each call, and records scores/digests/call IDs. For C reads it separately scores actual file bytes with the code-vulnerability head. Actual benign loopback HTTP requests are received by the fixture server and sent to the Go model as complete observed HTTP input. It does not infer HTTP or syscalls from shell strings.

Model input text excludes evaluation labels; labels are attached only to resulting scores. The supervised fixture has no real private data, and the recorded tool calls were reviewed for their scope. These are candidate-action privacy negatives, not proof of every nested kernel outcome. The tiny C controls are diagnostic examples, not a replacement for public-data evaluation. Original captured C labels are retained; audit corrects c06's overlooked signed division overflow (`INT_MIN / -1`) as risky without changing source bytes, weights or scores. `live_audit.json` and adjudicated scores are authoritative for those controls.

For historical call replay and the original consumed public test sets:

```bash
/tmp/model-test-env/bin/python scripts/research/model-opencode-20261018/replay.py \
  --assets /absolute/public-risk-assets --traces /absolute/previous-opencode-evidence/main \
  --out /tmp/model-replay-results
```

Public replay needs the original prepared `agent_privacy_data/test.jsonl` and `data/test.jsonl`. It is a repeated evaluation of existing test rows, not new independent generalization evidence. Historical OpenCode rows with failed original log checksums or task outcomes are excluded. Input context/candidate construction, row counts and score truncation are explicit. No threshold is selected on these results.

The local scoring endpoint is loopback-only, authenticated by an ephemeral per-study token, and shut down with the harness. This is a lab scoring service, not an installed backend, global OpenCode hook or production policy integration. The fixture tree and raw native/model-score records are in the evidence bundle; no real credentials or raw public corpus are included. `audit.py` checks original live-output hashes, native call-ID coverage and each checkpoint digest. These quality failures are not hidden by a successful inference transport check.
