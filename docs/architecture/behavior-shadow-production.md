# Behavior shadow scoring: production qualification, 2026-10-18

Status: **research / shadow-only; production blocking rejected**. This change does not deploy a service, change the existing 128-feature policy path, load an eBPF program, or authorize a model to ALLOW/BLOCK. The full-request model fails source transfer. Kernel privacy leakage and destructive behavior heads are still unqualified.

## Delivered engineering controls

- `kernel-http-behavior.v2`: 137 auditable behavior features, exact HTTP method semantics, parsed MIME types, immutable catalog copy, preallocated output map. v1 experiments remain historical, pinned to their original code revision.
- Archive adapter accepts actual `Event` / `Envelope.legacyEvent` and protobuf JSON names. Oversized lines are drained with bounded reads; duplicate keys, aliases, fractional/overflow integers, invalid UTF-8 and nonfinite JSON are rejected. Missing FD paths, stable process identity and loss information remain unknown.
- Strict runtime JSON validates exact field names, duplicate keys, UTF-8, trailing data and nesting depth. HTTP scoring requires explicit complete target, headers and body; capture loss, truncation, unsupported kinds and missing fields cause abstention.
- `behavior-risk` runs outside the kernel hot path. Its only outputs are shadow scores / abstentions, with model digest. No raw paths, headers or body are emitted by scoring. The offline `--vectors` option is sensitive derived data, not anonymization.
- Model files have a 2 MiB limit, SHA-256 verification against a separately trusted manifest, exactly 32,768 finite bounded weights and `shadow_only=true`. A checksum is integrity verification, not a signature or provenance proof.
- Atomic `Manager.Reload` retains the previous model on validation failure; `Disable` immediately abstains. These are Go APIs. The CLI reload/rollback operation is a service restart, not a live reload endpoint.
- Unix socket parent: absolute, owned by service UID, 0700; socket: 0600; no existing-path overwrite or TCP listener. Maximum 4 simultaneous connections, 4 MiB per connection, 512 KiB JSON line, 10 seconds per connection, 16 KiB scoring text. Over-limit clients are closed; malformed records abstain. Cancellation closes active clients and waits for workers.
- systemd template imposes 128 MiB memory, 96 MiB Go memory target, one CPU quota, 32 tasks and 64 FDs with no capabilities. These are configuration limits; a target-host soak test is still required.

## Dataset, fitting and frozen decision

Public source: https://github.com/msudol/Web-Application-Attack-Datasets at `424c6e1c1951570473d9d6c0227eaaac6fdd6d02`. Its root GPL-3.0 and original ECML/CSIC attribution must be preserved when redistributing source data. This experiment does not establish unrestricted licensing of downstream weights. Raw corpora are not committed here or included in the model bundle.

ECML-PKDD 2007 original train/test files were combined and **repartitioned by request template**; these are not the official benchmark splits. 50,116 complete requests, no exact duplicates; train 29,883, validation 7,699, calibration 5,059, test 7,475. Method + target + body templates group related requests; this heuristic is not proof of semantic independence. CSIC 2010 normal/anomalous test files are external-only for this model: 61,065 requests, no exact/template overlap removed. Earlier parameter experiments used CSIC-derived benign records, so this is current-model source isolation, not a project-wide never-seen corpus.

Encoding: raw and twice-decoded byte 3–5 grams, separate FNV namespaces, log-TF, L2, 32,768 dimensions (`byte-fnv1a-3-5-logtf-l2.v1`). Three actual SGD logistic fits at alpha 1e-5 / 1e-4 / 1e-3; average=True, seed 20261018, max_iter=120. Select by validation recall subject to a one-sided 95% FPR upper bound <=1%, then AP. Selected alpha=1e-5. A separate calibration set fixed threshold **0.23892595620096033** before either held-out source was scored. Sigmoid scores are not calibrated probabilities.

The negative-order-statistic bound assumes exchangeability. Template bootstrap intervals are also reported; neither protects against source shift. After the external failure, no threshold or model was changed to improve that test. Three validation-fit results are in `results.json`.

| Frozen evaluation | Rows | Accuracy | Recall | FPR | FPR upper 95% | Gate |
|---|---:|---:|---:|---:|---:|---|
| ECML held-out templates | 7,475 | 98.876% | 97.819% | 0.683% | 0.900% | Pass for this source only |
| CSIC external source | 61,065 | 41.046% | 100.000% | **100.000%** | **100.000%** | **Reject** |

ECML confusion: TN=5,238, FP=36, FN=48, TP=2,153. Template-bootstrap FPR 95% CI [0.477%,0.933%], recall [96.558%,98.771%]. CSIC: TN=0, FP=36,000, FN=0, TP=25,065; every benign request scored above threshold. Both had zero abstentions. This proves the current full-request representation is not transferable enough for enforcement; the exact causal features require a new pre-registered ablation and a new untouched external source. It is not an accuracy improvement claim against prior parameter-only models, which solve a different task.

Actual Go predictions match Python over all 7,475 internal test records with maximum absolute error `2.9976021664879227e-15`. Model size 655,273 bytes; SHA-256 `b53b70d7f7bdf86ff0d24712ad6c10667f47463daf1abf4725e45c1a8d11a25a`.

## Measured resources and validation scope

Same Xeon Platinum 8573C host, three Go microbench runs; medians:

| Feature extraction | Before | After | Reduction |
|---|---:|---:|---:|
| ns/op | 62,046 | 39,191 | 36.84% |
| bytes/op | 30,095 | 13,129 | 56.37% |
| allocations/op | 229 | 78 | 65.94% |

Actual frozen Go worker, sequential stdio JSON round-trip, synthetic benign 1,825-byte request, 100 warmup + 1,000 measured: P50 0.294 ms, P95 0.679 ms, P99 1.261 ms, peak child RSS 11,776 KiB. This is not a concurrent socket SLO, adversarial maximum-size benchmark, long soak, or target-host cgroup validation.

Training resource incident is retained: the initial Python-list sparse cache exhausted the 8 GiB environment before fitting. Typed-array caching completed all three fits. Evaluation was stopped before completion when retaining two full sparse matrices approached the same limit. The identical frozen model was evaluated with bounded Go scoring and 128-record parity batches. The final recipe writes frozen artifacts and exits; evaluation runs separately after memory is released. Training itself still retains ~350M nonzeros and requires a larger-memory research host; the tiny inference model does not imply tiny training memory.

Local race/contract tests: 44 named tests/subtests passed; 2 native Unix socket tests were skipped because this environment forbids sockets. CI must execute both with no skips. CI also exercises bounded fuzzing, adapter/calibration tests and actual benign `strace` read/write/openat/execve observations plus a loopback HTTP request. The first CI trace audit failed on annotated `AT_FDCWD</cwd>` syntax; its parser and regression test were corrected. The corrected CI run [39093408256](https://github.com/stevessr/agent-ebpf-filter/actions/runs/39093408256) passed every behavior-contract step at commit `0a03474638e752f57bd8de65c117591da80af835`: real socket tests, race/vet, fuzzing, 13 Python tests and the benign trace. Trace counts: execve=2, openat=4, write=3, read=2, plus one loopback HTTP observation; all 12 records cleaned. The CI parser is deliberately fixture-specific, not a general production trace ingester.

A successful benign kernel fixture proves ingestion mechanics only. It does not validate the eBPF collector, TLS decryption, stable process-to-request joins, attack labels, exploit detection or prevention of destruction/exfiltration.

## Opt-in shadow pilot and rollback

Use only after review of the failed model gate; this is a pilot template, not an instruction to enable blocking. The user-facing authorization policy remains outside this service.

1. Build from reviewed source: `cd backend && go build -trimpath -o behavior-risk ./cmd/behavior-risk`.
2. Verify the model digest against an independently reviewed manifest, then install binary root-owned at `/usr/local/bin/behavior-risk` (0755), model at `/etc/agent-ebpf/http_request_model.json` (root-owned 0644), directory root-owned 0755. Set `/etc/agent-ebpf/behavior-shadow.env` root-owned 0600 with `BEHAVIOR_MODEL_SHA256=<reviewed digest>`. Keep prior binary/model/env as a versioned rollback set.
3. Inspect `deploy/systemd/agent-behavior-shadow.service`, install it under `/etc/systemd/system/`, run `systemctl daemon-reload` and start only the opt-in shadow service. DynamicUser owns `/run/agent-behavior` (0700); a root collector can access its 0600 socket. A non-root collector needs an explicitly designed identity integration; do not relax socket permissions to make it work.
4. Send schema-v2 JSONL inputs, at most four concurrent connections; reconnect within the ten-second lifetime and handle EOF/timeouts/abstentions. Use a bounded asynchronous queue in any future collector integration and record drops. **No such backend queue/wiring is delivered in this PR.** Never interpret missing score as benign or call a blocking decision from this worker.
5. Observe counter-only diagnostics, client latency, drop/abstention rates, source mix and human-reviewed false positives. Unknown provenance is not repaired by a client-supplied `source` string. Do not retain raw credentials or request bodies merely to obtain metrics.
6. Roll back by stopping the shadow service; the established policy path continues independently. To restore a prior reviewed model, stop service, restore the prior binary/model/digest set, verify checksum, restart. A failed new model load exits without serving. Library users can use `Manager.Disable()` or verified atomic reload directly.

Not performed here: installing/enabling the service on a production host, modifying production policy, running attacks against websites, or creating destructive/privacy-leaking training examples on real systems.

## Gates still required before production enforcement

The subsequent [public-data deep optimization](../../scripts/research/deep-optimization-20261018/RESULTS.md)
completed 43 actual fits, two six-epoch BERT adaptations and new frozen-model
OpenCode sessions. It improves privacy ranking metrics, but low-FPR privacy
recall, PiSAs transfer recall and real benign HTTP false positives still fail.
Versioned request normalization and field-ratio corrections retain their failed
results. Those Python research models are separate artifacts and do not replace
the Go worker or change the production enforcement qualification below.

- Multi-source generalization and measured production benign FPR, separated by site/tenant/time, with attack-family holdout and an untouched external source. The tested CSIC source is now consumed as evaluation evidence.
- Separately labelled privacy leakage and destructive syscall heads; authorization and provenance context; safe lab replay with established outcomes. HTTP attack/anomaly labels cannot substitute for either head.
- Real eBPF→archive→v2 feature→decision trace validation on supported kernels, loss counters, FD/path resolution and stable identity; TLS visibility and HTTP request framing completeness.
- Bounded backend integration, target-host concurrency/maximum-size/soak/cgroup tests, operator metrics, rollout and tested incident rollback.

References: scikit-learn threshold tuning https://scikit-learn.org/stable/modules/classification_threshold.html ; calibration https://scikit-learn.org/stable/modules/calibration.html ; regression test design https://github.com/coreruleset/go-ftw . These informed evaluation discipline; they were not used as evidence that this model passed those projects' tests.

Final code validation: [CI 39093620547](https://github.com/stevessr/agent-ebpf-filter/actions/runs/39093620547) passed all behavior-contract steps at `5082bc3541069912104f644789a03970bb602eb1`. Sorted L2 accumulation removes map-order roundoff nondeterminism; replay of all 7,475 internal and 61,065 external records changed no threshold decisions (max score change 1.7763568394002505e-15), with the original weights/threshold frozen. Regression determinism test passed 100 repetitions.
