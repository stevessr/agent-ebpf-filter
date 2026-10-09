# Runtime security frameworks: integration review (October 2026)

This note compares **verified upstream documentation** to the existing Agent eBPF Filter implementation. It describes patterns adopted by our project, not copied code or a claim of API compatibility.

## Selected frameworks

| Framework / upstream reference | Relevant design | Implemented here | Not implemented |
|---|---|---|---|
| [Falco: rule fields](https://falco.org/docs/reference/rules/rule-fields/) and [rule exceptions](https://falco.org/docs/concepts/rules/exceptions/) | Clear rule identity, severity, event source, and **scoped** actor+target exceptions rather than broad binary-name exclusions | A fixed semantic rule ID plus evidence-strength-based risk floor; path-keyed bounded cooldown on repeatedly reported interleaved mutations | Falco YAML rule loader, exception authoring UI, Falco priority/source semantics |
| [Falco: dropped events / throttling](https://falco.org/docs/concepts/event-sources/kernel/dropped-events/) | Telemetry loss damages stateful conclusions; a flood of observations should not produce a flood of reports | One alert per verified path/cooldown period; existing kernel dropped-event counts retained in record | Event-loss-sensitive cancellation of correlation, global token-bucket output limiting |
| [Tetragon: selectors](https://tetragon.io/docs/concepts/tracing-policy/selectors/) and [tracing policy API](https://tetragon.io/docs/reference/tracing-policy/) | Match on syscall return values, process/parent identity, namespace and other verified fields | The **userspace** semantic detector rejects failed filesystem mutations; write requires positive observed byte result, metadata syscalls allow 0 | Tetragon CRDs, in-kernel per-hook return selectors, namespace inode proof |
| [Tetragon: process lifecycle](https://tetragon.io/docs/use-cases/process-lifecycle/) | Process start/exit and parent/identity information are first-class incident evidence | Keep recorded historical PID distinct from live process lookup; only compare compatible Agent identity classes | A globally unique exec_id equivalent or complete process provenance DAG |
| [Tracee: event model](https://aquasecurity.github.io/tracee/dev/docs/events/) and [detectors](https://aquasecurity.github.io/tracee/dev/docs/events/custom/overview/) | Separate raw syscall observations from derived detection events and preserve event context | Emit a `semantic_alert` with originating event metadata, underlying path evidence and explicit reason | Tracee's EventDetector API, policy YAML and detector plugin interoperability |

## Implementation: outcome-aware, bounded file modification correlation

The detector in `backend/app/events/alerts_semantic.go` consumes events only when:

1. The event describes a supported mutating operation and the syscall's outcome is compatible with success. A failed syscall (`retval < 0`) has not modified the resource. For `write`, even `retval == 0` is insufficient to claim any bytes were written. Because some adapter events omit results, this deliberately **favors missing a potential correlation over inventing a positive mutation**.
2. The path is not a synthetic FD description, socket/pipe or redacted placeholder; relative paths must have known absolute CWD. We still cannot guarantee two equal pathnames identify an identical inode without mount namespace/inode information.
3. The two observations have comparable, distinct Agent identities. A process PID alone is not an Agent, and root PID vs run ID cannot be compared as distinct identities.
4. The bounded state key includes a known container ID when available, and keeps unscoped events separate. Thus a same-named path in container B neither correlates with nor overwrites container A's path history. A shared bind mount is possible, but the event schema cannot prove it; this is deliberately conservative.
5. Two distinct writers are observed within a 15-second window, and this **scoped path** has not emitted a correlation alert in the preceding 15 seconds. The evidence retains the latest writer and last alert time in the existing capped map; this does not add an unbounded queue.

The emitted event remains named `MULTI_AGENT_FILE_CONTENTION` for compatibility with saved records, filters and dashboards. **Semantics:** *interleaved modifications to a reported pathname*; it is *not* evidence of simultaneous writes, inode identity, a kernel data race, or malicious intent. Therefore the correlation alone contributes a heuristic **70/100 risk floor**, rather than 96/100. The original event's risk score is not lowered.

## Why not embed another framework?

The project already has eBPF probes, protobuf events, process/Agent attribution and a Renew desktop client. Embedding Falco/Tetragon/Tracee would duplicate collection and introduce another process, policy lifecycle and deployment dependency. It is safer to implement narrowly scoped invariants and regressions first. This change does not claim external rule/config compatibility.

## Operational noise and replay hardening (incremental improvement)

- Pseudo-files and device endpoints such as `/dev/null`, `/dev/pts/*`, `/proc/*`, `/sys/*`, and `/dev/fd/*` are not suitable evidence of two Agents modifying the same **regular file**. Only the cross-Agent file-contention predicate excludes these targets; secret-access and other safety rules still receive their original events. We deliberately preserve ordinary files under `/dev/shm`.
- Out-of-order observations on a scoped path must not overwrite newer evidence or synthesize a temporal correlation. The correlation state now ignores timestamps older than the stored observation. This is mainly relevant for synthetic replay, asynchronous producers and clock skew; the production detector uses ingestion time.
- Three monotonic counters are exposed through the existing semantic state status JSON: `fileCorrelationAlertsTotal`, `fileCorrelationDedupedTotal`, and `fileCorrelationLateTotal`. All updates happen under the existing state lock and use no unbounded retention. They measure decision volume, not false-positive rate, which requires ground-truth labels.

## Regression and follow-up

The new `TestSemanticFileContentionRequiresSuccessfulSyscallResult` and `TestSemanticFileContentionCooldownAndContainerIsolation` tests cover successful/failing syscall outcomes, null-byte writes, known container boundaries, deduplication and cooldown expiry. CI invokes Go tests through the repository's bpf-ts smoke workflow.

Next candidates for an independently reviewed PR: a schema-level **process exec ID with start time**, inode/mount namespace evidence for cross-container paths, **scoped, explicit actor+path exceptions** (never blanket process-name suppression), a machine-readable finding/evidence schema, and a shadow replay suite reporting precision, false-alerts per 10k normal events, and detection latency. Any exception management must retain auditing and rollback.
