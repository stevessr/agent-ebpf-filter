# Magpie × AstrLink Guard: Local Privacy Audit Plugin

A **separate, optional integration** for [Agent eBPF Filter](../../README.md) and
[yetone/magpie](https://github.com/yetone/magpie). It uses Magpie's **asynchronous
request hook** for model-backed pre-egress scanning and a **synchronous gateway
middleware** for cheap deterministic fallback. A Python loopback-only sidecar runs
[QuantumNous/astrlink-guard](https://huggingface.co/QuantumNous/astrlink-guard)
locally. No weights are vendored into Agent eBPF Filter.

**This is additive to Magpie's built-in privacy masking, not a substitute.**
Magpie already supports AstrLink Guard as an in-process CPU model. Use its
Settings → Privacy masking with model detection enabled for fail-closed privacy
protection, then optionally run this plugin for local accelerator experiments,
extra regex coverage and aggregate audit. Magpie's async hooks are *fail-open*:
if an async hook times out, is disabled or fails to run, its modification is
rolled back. Even synchronous middleware is fail-open on internal errors. The
built-in `redact` plugin is the authority for mandatory outbound masking.

## 1. Install the local model (explicitly, once)

```sh
cd integrations/magpie-astrlink-guard
python3 -m venv .venv
. .venv/bin/activate
# For NVIDIA, install the CUDA-enabled wheel in place of the CPU wheel:
pip install -r requirements-cuda.txt
# For CPU-only systems instead: pip install -r requirements-cpu.txt
python - <<'PY'
from huggingface_hub import snapshot_download
snapshot_download(
    repo_id="QuantumNous/astrlink-guard",
    revision="v0.1.3",
    local_dir="models/astrlink-guard",
    allow_patterns=["int8/*", "fp16/*", "LICENSE", "SHA256SUMS"],
)
PY
```

The model release is Apache-2.0. To avoid downloading FP16 weights on a
CPU-only machine, remove `fp16/*` from `allow_patterns`. For full-code accuracy
comparison, add `fp32/*` and a compatible provider mapping in service.py.
Model files stay local and are **not** committed to the project repository.

## 2. Start the local scanner

```sh
# Choose a long random token; use exactly the same value in Magpie's options.
export ASTRLINK_GUARD_TOKEN="$(python -c 'import secrets; print(secrets.token_urlsafe(32))')"
python service.py --model-dir models/astrlink-guard --backend auto
# GPU-only diagnosis: --backend cuda --strict-provider
# Check the selected backend: curl http://127.0.0.1:8765/health
```

Backends: `auto`, `cpu`, `cuda`, `tensorrt`, `rocm`, `directml`, `coreml`,
`openvino`, `webgpu`, `vulkan`. **They are conditional on a working ONNX Runtime
Execution Provider and the matching model.** `auto` probes installed providers,
tries FP16 GPU backends, then uses INT8 CPU as a fallback. Explicit backends
also fall back to CPU by default; `--strict-provider` instead refuses startup.

**Vulkan:** normal ONNX Runtime wheels do not include
`VulkanExecutionProvider`. `--backend vulkan --strict-provider` works only when
a separately supplied compatible ONNX Runtime build really advertises it and
runs the model. The `webgpu` option requires a WebGPU EP and does **not** prove
that its implementation uses Vulkan. Do not describe this as verified Vulkan
acceleration without testing on the target GPU.

For CoreML or DirectML, use an appropriate ONNX Runtime build for that OS.
CUDA requires a compatible NVIDIA driver, CUDA/cuDNN, and the GPU ORT wheel.
ONNX may place unsupported operators on the CPU; check provider-level traces
before quoting GPU-only speedups. The FP16 model uses fixed 512-token windows
and stride overlap of 128; the CPU model uses dynamic INT8 shapes.

## 3. Install as a Magpie plugin

```sh
magpie plugin add ./integrations/magpie-astrlink-guard
magpie plugin options agent-ebpf-magpie-astrlink-guard '{
  "serverUrl":"http://127.0.0.1:8765",
  "authToken":"<same token as ASTRLINK_GUARD_TOKEN>",
  "action":"redact",
  "gatewayTimeoutMs":5000,
  "labels":["private_person","private_address","phone","email","account","payment_card","common_secret"]
}'
```

Run `magpie plugin add` from the repository root (or use an absolute path),
then ensure the **Gateway middleware** built-in plugin is enabled as well. The
async hook and synchronous middleware are packaged together. If editing
source-only hooks, reload their plugin configuration to restart the Bun host.

Action `redact` (default) masks matching input strings with `[REDACTED:label]`;
action `audit` only gathers aggregate detection statistics and never changes
the request. The plugin deliberately does **not** promise model-level `block`,
since Magpie's async hook API cannot reject on inference failure. For mandatory
block decisions use Magpie's built-in Privacy policy or a true local proxy with
fail-closed request ownership.

The hook inspects text in Chat Completions, Responses, Anthropic Messages and
Gemini content containers. It skips model name, tool declarations, tool-choice,
image bytes, signatures and binary payloads. It does not intercept arbitrary
TLS traffic, HTTPS on the host, MCP channels outside the gateway, or future
conversation turns when the client bypasses Magpie.

The plugin checks the selected endpoint is loopback-only and never transmits
raw text to a remote scanner. Use the sidecar `ASTRLINK_GUARD_TOKEN` to prevent
other local programs from making unauthenticated calls. The token is stored in
Magpie's local plugin settings; protect those settings like local secrets.

## 4. Audit and verify

```sh
curl -H "Authorization: Bearer $ASTRLINK_GUARD_TOKEN" \
  http://127.0.0.1:8765/stats
node --test integrations/magpie-astrlink-guard/tests/*.test.mjs
python3 -m unittest discover -s integrations/magpie-astrlink-guard/tests -p 'test_*.py'
```

`/stats` exposes only request count, scanned-text count, detection counts per
label, mean latency and chosen backend, in memory. It never stores request
bodies, matched values or tokens. For realistic speed measurement compare
warm p50/p95 under repeated representative Chinese, English and code requests;
measure CPU INT8 vs CUDA FP16 independently (GPU launch overhead can make
short inputs slower). Assess false negatives and false positives before
changing production masking policy.

Limitations to test on real Magpie: async hook mutations are best-effort and
may be ignored on timeout; built-in redaction may also mask inputs before they
reach the upstream provider. Tokenization, FP16 numeric compatibility, and
provider execution are model/runtime dependent. Local unit tests do not count
as a completed CUDA or Vulkan inference benchmark.

## 5. Real inference smoke and benchmark

The GitHub workflow includes a model-enabled **CPU smoke test** that downloads
the pinned 0.1.3 INT8 model and classifies one synthetic email. This is distinct
from offline unit tests. For local warm latency measurements:

```sh
python scripts/benchmark.py --model-dir models/astrlink-guard --backend cpu --runs 50
python scripts/benchmark.py --model-dir models/astrlink-guard --backend cuda --strict-provider --runs 50
```

The script reports backend, provider, warm p50/p95 and mean inference latency.
These are model-only microbenchmarks, not end-to-end Magpie gateway latency.
The GPU tests require a real CUDA installation; Vulkan requires a compatible
third-party EP as documented above. No made-up latency figures are included.
