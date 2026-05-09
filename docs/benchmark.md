# Benchmark and Attack Replay

The repo now includes an **offline runtime replay suite** for agent-security scenarios.

## Scenario catalog

File:

- `benchmarks/runtime-replay/scenarios.json`

It includes:

- benign flows: `git status`, `npm install`, `pip install`, `pytest`, `cargo build`, PR review read-only scans
- malicious flows: `curl|bash`, secret read, reverse shell, workspace escape, `chmod +x` then exec, suspicious SSH, hidden network egress, lightweight fork storm
- agentic flows: prompt-injection file exfiltration, malicious MCP tool, unexpected browser/network behavior, suspicious remote-devbox action, resource-wasting loop

## How to run

From the repo root:

```bash
rtk make runtime-benchmark
```

Or directly:

```bash
rtk bash -lc 'cd backend && RUNTIME_REPLAY_OUT=../reports/runtime-replay-manual/summary.json go test ./... -run TestRuntimeReplaySuite -count=1 -v'
```

## Output

The helper script writes:

- `reports/runtime-replay-<timestamp>/summary.json`

The summary tracks:

- scenario coverage / pass count
- false positives / false negatives
- p50 / p95 / p99 replay-event latency
- p50 / p95 / p99 wrapper-decision latency
- p50 / p95 / p99 first-alert / block latency
- memory allocation delta during replay
- trace-correlation accuracy for inherited child context

## Live-vs-offline note

This suite is **offline replay**, so:

- ringbuf drop rate is `0` in the replay summary by construction
- kernel collection lag must still be checked from live endpoints such as:
  - `/system/collector-health`
  - `/metrics`

Use the replay suite to catch logic regressions in:

- context inheritance
- semantic alert generation
- wrapper decision path latency
- expected-vs-observed behavior classification
