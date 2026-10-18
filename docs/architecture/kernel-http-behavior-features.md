# Kernel and HTTP behavior feature cleaning (offline v2)

This implements the behavior-cleaning slice of #112, alongside the context
bootstrap in draft #114. It is a separate offline package and CLI. It does not
change the legacy 128-dimensional extractor, attach probes, enable TLS capture,
or feed automatic blocking. No runtime accuracy claim is made.

## Evidence and input contract

`backend/internal/behaviorfeatures` accepts exactly one of `syscall`, `http`, or
`parameter`. `source` records a provenance claim, not authenticated evidence.
Callers must obtain syscall data from the collector and HTTP plaintext from an
explicitly enabled plaintext capture source. An IP, DNS name or SNI alone is not
an HTTP request. A wrapper command, LLM explanation, OpenCode read tool call,
source-code defect label or public HTTP parameter is not a kernel event.

`Value.present=false` is unknown; it is not a safe/negative observation. This
applies to syscall returns, actual I/O, unresolved paths, truncation, directions,
duplicate headers and historical attribution. Each output contains only a kind,
schema version and named numeric values/masks. Batch output additionally maps to
the original input index. Raw IDs, scope keys, file paths, hosts, query values,
headers and bodies are excluded. Aggregate features are not differential privacy.

| Channel | Extracted behavior | Preserved uncertainty |
|---|---|---|
| Syscall | read/write/exec/open/delete/rename/permission, success/errno, requested and actual bytes | syscall family does not prove a file FD; unknown return is not success |
| Paths | lexical depth/traversal, relative/absolute, sensitive/temp/executable hints, rename target | known untruncated path required; no symlink, mount or inode resolution |
| Open | access intent, create/truncate/append, O_PATH/O_TMPFILE | mode bits are separate; O_PATH is not read access; generic Linux flag ABI |
| HTTP | method, request-target boundaries, duplicate query keys, authority mismatch, destination IP properties | host mismatch is a hint; private IP is not malicious; no DNS resolution or allowlist decision |
| Headers | counts, duplicate Content-Length and CL+TE hints | duplicate claims require a list that preserves duplicates; a map cannot prove absence |
| Target/body | separate bounded lexical statistics, entropy, encoding/traversal and SQL/XSS/command/URL hints | hints are inputs, never labels or proof of exploitation; no full WAF/parser equivalence |
| History | prior events, successful sensitive-path reads, successful writes, outgoing requests over 1/30/300 s | counts cover this batch only, not complete host history; temporal relation is not dataflow |

The ordered `Catalog()` is the independent `kernel-http-behavior.v2` contract.
No `risk_score`, `decision`, source identity, dataset label, attack category or
run identifier becomes a training feature. The canonical CLI rejects unknown
JSON fields, duplicate keys, excessive nesting and trailing JSON values.

## Path and syscall semantics

- File read/write features need `fd_path` supplied by a verified collector-side
  FD resolution. The current archive adapter does not invent this from `path`.
- `actual_bytes=max(return,0)` only applies to supported read/write calls;
  legacy `Event.bytes` is requested count. Return absence remains unknown.
- `path_truncated` is optional: absent or true prevents lexical path categories.
  This avoids asserting a complete path from an unqualified capture prefix.
- A relative `*at` path needs the matching `dirfd_path`, or `dirfd=-100`
  (`AT_FDCWD`) with known cwd. Legacy collapsed `open`/`openat` stays unresolved.
  Relative renameat target resolution is intentionally unavailable until its
  separate target-dirfd context is supplied by a future contract.
- Linux paths are case-sensitive; backslashes are ordinary characters. NUL
  prevents path classification. Lexical cleanup does not resolve filesystem
  objects. Absolute path hints are neither authorization nor exfiltration proof.
- Coverage is limited to the syscall families in `extractSys`; mmap, splice,
  sendfile, io_uring, descriptor inheritance and socket FD classification are
  not implemented. Missing families must not silently count as covered.

## Website requests

Parse request-target boundaries before decoding. Decoding is bounded to two
layers and preserves error/residual-encoding flags. Encoded `?`/`#` never creates
a new query/fragment boundary. Query keys use form decoding; URI paths do not
turn `+` into a space. Body form decoding requires the content type; unknown
body types receive bounded lexical inspection, not invented JSON fields.

Methods such as CONNECT/asterisk-form and malformed targets can remain unknown
under `net/url.ParseRequestURI`; payload hints still do not prove a request was
accepted or an attack succeeded. This is not a full HTTP request-smuggling,
SSRF, SQLi or XSS validator. IP properties do not replace destination policy,
DNS-rebinding protection or post-resolution checks.

## Attribution, ordering and bounds

History requires all of host ID, boot ID, run ID, PID namespace, TGID,
process-start time, clock and event time. Only equal scopes/clocks join. Missing
identity keeps history absent. Sorting uses supplied event time, never wall
clock now. Equal timestamps are not treated as prior events. Windows include
the lower bound and exclude the current timestamp.

Only `(scope, source, event ID)` deduplicates; conflicting duplicate contents
error. Cross-source records are retained. Incomplete-scope records are not
deduplicated. There is no arbitrary cross-source causal merge or raw-path hash.

Limits: 64 KiB per text field, 256 headers / 64 KiB aggregate headers, 512 KiB
per CLI line, JSON depth 32, 20,000 events and 16 MiB serialized input per batch.
The streaming mode has no cross-record state. This is an offline extractor;
benchmark figures are not production hot-path SLOs.

## Running

```sh
cd backend
go build -o /tmp/behavior-features ./cmd/behavior-features
go test -race ./internal/behaviorfeatures ./cmd/behavior-features
go vet ./internal/behaviorfeatures ./cmd/behavior-features
/tmp/behavior-features -catalog
/tmp/behavior-features < canonical.jsonl > cleaned.jsonl
/tmp/behavior-features -batch < canonical.jsonl > batch.json
```

The canonical structs in `types.go` define snake_case fields. Example, explicitly
synthetic contract input (not collected behavior or training ground truth):

```json
{"source":"fixture","syscall":{"name":"read","fd_path":"/etc/shadow","path_truncated":false,"return":12,"requested_bytes":4096}}
```

For existing archive/TLS streams:

```sh
python3 scripts/research/behavior-features-20261018/adapt_capture.py --format archive < archive.jsonl | /tmp/behavior-features > cleaned.jsonl
python3 scripts/research/behavior-features-20261018/adapt_capture.py --format tls < tls.jsonl | /tmp/behavior-features > cleaned-tls.jsonl
```

Adapter diagnostics on stderr count unsupported/invalid records without echoing
content. The adapter intentionally leaves process-start/host/boot identity
missing: current archived fields are insufficient for safe correlation. A TLS
`write` direction alone is not an outgoing client request; a server also writes
responses. IP/SNI-only connections are counted as unsupported for this contract.
`proto3` defaults omitted by the archive serializer cannot recover field presence.

## Public-data feature engineering and promotion

See `scripts/research/behavior-features-20261018/README.md` and `results.json`.
Train-only variance filtering, mutual information and correlation pruning select
from structural, log-count, normalized-count and hint-interaction features.
All candidate models use the same template-group split. Validation selects
models, calibration selects a threshold, test only reports. Initial test results
were inspected before the MI discrete-mask correction and fixed-0.5 diagnostics;
the follow-up is explicitly reused-test confirmation. It must not be promoted
as a fresh blind test or a real kernel/privacy/attack-outcome evaluation.

## Remaining collector work

To train kernel-level decisions, collect actual consented traces with stable
host/boot/process identity, explicit return/truncation presence, dirfd and FD
lifecycle/inode evidence, request direction and capture-loss accounting. Attach
independent outcome labels (authorization, verified file effect, destination and
actual leakage), with run/task/harness/source-isolated holdouts. Do not derive
labels from these heuristics or from the current model's decisions.

References: [Linux open/openat](https://man7.org/linux/man-pages/man2/openat.2.html),
[OWASP CRS](https://owasp.org/projects/crs),
[OWASP SSRF guidance](https://cheatsheetseries.owasp.org/cheatsheets/Server_Side_Request_Forgery_Prevention_Cheat_Sheet.html),
[original HttpParamsDataset](https://github.com/Morzeux/HttpParamsDataset).

Version 2 adds exact-key/UTF-8 validation and corrects case-sensitive HTTP method / media type parsing. Version 1 research artifacts remain tied to commit 2073296ea6a02c6f36846bb12ba354ac946df961; do not mix feature schema versions. See the production readiness report for current evidence.
