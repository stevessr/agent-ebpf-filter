# Generic API capture pipeline

The capture path is intentionally split into two layers.

## Kernel provenance layer

The main tracker keeps an LRU `socket_fds` map keyed by `(tgid, fd)`. `socket()` seeds the map, a successful `connect()` updates the remote endpoint, and `close()` deletes the descriptor. This lets the `write()` tracepoint distinguish a confirmed socket from an ordinary file descriptor without trying to infer descriptor type from payload bytes.

For confirmed sockets, the eBPF program performs a bounded HTTP/1 method check and copies **only the request line** (maximum 255 bytes) into the event scratch buffer. Headers and bodies are deliberately not copied, so Authorization/Cookie material is outside this kernel capture path. Query strings and fragments are NUL-truncated **inside eBPF**; the exit path copies the sanitized prefix into the already-zeroed ringbuf event with `bpf_probe_read_kernel_str`, so bytes beyond the delimiter never cross the kernel/userspace boundary. A matching event is emitted as `SOCKET_HTTP` with the fd, captured prefix length, requested byte count, endpoint metadata, and the same kernel audit generation/CPU sequence/loss provenance as every other tracker event.

`sendto()` uses the same request-line helper and falls back to the fd-provenance endpoint for connected sockets without an explicit destination address. Non-HTTP socket writes remain ordinary `write`/`sendto` events and do not carry payload prefixes.

Userspace still normalizes the target a second time before it enters `pb.Event`, providing defense in depth against query/fragment persistence.

## Protocol/API fingerprint layer

`backend/app/captureprofile` is a protocol-neutral immutable-snapshot matcher. Inputs are metadata only: capture source, protocol, method, host, path, header names and content type. Profiles are JSON-compatible data with host suffix/substring, path prefix/substring, method, protocol, required-header and content-type constraints.

The same matcher is used for:

- OpenSSL/BoringSSL plaintext capture;
- Go `crypto/tls` plaintext capture;
- GnuTLS/NSS/rustls plaintext capture;
- HTTP/1 parser output;
- HTTP/2 HPACK-decoded `:authority` / `:path` metadata;
- kernel plaintext `SOCKET_HTTP` request-line events;
- future proxy, QUIC decryption, language-runtime and SDK-specific capture sources.

Built-in profiles cover OpenAI, OpenAI-compatible endpoints, Anthropic, Anthropic-compatible endpoints, Google Gemini, Azure OpenAI, AWS Bedrock, OpenRouter, Mistral, Cohere and Ollama. The matcher publishes rule snapshots atomically through `Registry.Replace`, so a runtime configuration endpoint/file watcher can replace profiles without locks on the capture hot path.

Provider-specific host matches score above path-only compatibility rules. Consequently `/v1/responses` observed without a host is labeled `openai-compatible`, while the same path with `api.openai.com` is labeled `openai`.

## Privacy and trust boundaries

This feature is metadata-first, not a blanket packet dumper. Kernel capture does not copy headers or bodies. TLS capture keeps the existing bounded/opt-in plaintext policy and existing header/body redaction. API fingerprinting never needs credentials or prompt contents.

Kernel `SOCKET_HTTP` events inherit the tracker audit tuple (`kernel_audit_generation`, per-CPU sequence, capture timestamp, reserve-loss counters), and the persisted deterministic protobuf hash chain therefore binds the capture provenance and normalized API metadata together.

## Known next steps

The socket-fd map currently follows `socket`, `connect`, `write`, `sendto`, and `close`. It is cleared on tracker generation rotation so a recycled `(tgid, fd)` cannot inherit provenance across backend downtime. Descriptor duplication (`dup*`), inherited sockets across fork, scatter/gather `writev/sendmsg`, and decrypted HTTP/3/QUIC are separate follow-ups. TLS/library capture already remains the authoritative source for encrypted HTTP request paths.


## Phase 2: descriptor lineage and bidirectional metadata

The kernel socket identity layer now follows descriptor topology rather than only the original `socket()` return value:

- `dup`, `dup2`, and `dup3` copy or invalidate socket provenance as descriptors are replaced;
- `sched_process_fork` records a bounded lazy parent lineage, allowing a child to materialize inherited socket metadata on first use without iterating descriptor tables in eBPF;
- `accept` and `accept4` mark returned sockets as accepted and preserve listener family/type/protocol plus peer endpoint when the sockaddr is available;
- `read` and `write` recognize both HTTP/1 request lines and response status lines on confirmed plaintext sockets;
- raw `kernel_capture_flags` distinguish request/response, incoming/outgoing, duplicated/inherited/accepted descriptor provenance, and future scatter/gather capture.

The lineage resolver intentionally follows at most two parent generations before materializing a child entry. This keeps verifier complexity and map lookups bounded. Long-lived descendants normally materialize entries during their first socket operation, making subsequent lookups direct.

## Runtime profile overlays

Set `AGENT_EBPF_API_PROFILES=/path/to/profiles.json` to layer custom rules over the built-ins. The file remains a JSON array of profile objects. Custom profiles with an existing `id` replace that built-in ID; new IDs are appended. The watcher hashes file contents every two seconds and publishes a fully validated immutable snapshot atomically. Invalid updates are rejected while the last known-good rules remain active.

Profiles can additionally restrict `sources` and `directions`, for example `kernel_socket_prefix` + `incoming` for a local webhook/API server. This lets the same engine classify client APIs, reverse proxies, local gateways, and self-hosted OpenAI/Anthropic-compatible endpoints without adding provider-specific eBPF code.

## gRPC

HTTP/2 plaintext events with an `application/grpc` content type are normalized to `grpc`. The canonical `/package.Service/Method` route is split into service and method without decoding protobuf bodies. If no explicit vendor profile matches, the event still receives a generic `grpc:<service>` profile, service as the product, method as the operation, and a moderate metadata-only confidence score.


## Scatter/gather syscall coverage

The plaintext kernel sampler also covers native 64-bit `writev`, `readv`, `sendmsg`, and `recvmsg`. To keep both verifier cost and privacy exposure bounded, only the first iovec is inspected and only enough bytes for an HTTP/1 start-line candidate are copied into the per-CPU scratch buffer. The body, subsequent iovecs, authorization headers, cookies, and arbitrary message control data are not captured.

`kernel_capture_flags` includes `SCATTER_GATHER`, so downstream analysis can distinguish this best-effort prefix from a contiguous `read`/`write` sample. `sendmsg` uses `msg_name` when present and otherwise reuses connected-fd provenance; `recvmsg` can refresh the peer endpoint after return. compat32 user tasks are intentionally not decoded as 64-bit `iovec/msghdr`; they retain ordinary syscall telemetry rather than risking pointer-layout misinterpretation.


## Socket-view visual configuration

The Network Flow workspace now owns the capture-profile control plane. Open a
flow/socket detail and choose **Create capture profile from this socket**, or use
the **Capture Profiles** tab directly.

The editor writes one canonical overlay file:

- `AGENT_EBPF_API_PROFILES` when explicitly configured;
- otherwise `${runtime settings dir}/api-capture-profiles.json`.

The file is atomically replaced, validated before publication, and consumed by
the same hot-reload watcher used for external/ConfigMap updates. Built-in
profiles remain the base layer; custom profiles override built-ins with the same
ID.

Authenticated endpoints:

- `GET /network/capture-profiles` — custom/effective profile state and supported
  source/protocol/direction selectors;
- `PUT /network/capture-profiles` — atomically replace the custom overlay;
- `POST /network/capture-profiles/preview` — evaluate one draft against up to
  500 protocol-neutral observations using the production matcher.

The flow preview is intentionally host/method/protocol oriented because the
aggregated socket table does not persist request paths. Path selectors are still
evaluated against real L7 capture events. This keeps the visual configuration
honest instead of fabricating path visibility that the flow aggregator does not
have.


## Indexed matcher and socket-scope selectors

The profile registry compiles immutable rules into dispatch buckets keyed by the
high-frequency exact selectors `source`, `protocol`, `direction`, `method`, and
`transport`. A lookup probes at most 32 exact/wildcard bucket combinations and
then runs host/path/header/CIDR checks only for those candidates. Profile reload
still uses atomic snapshot publication, so capture readers take no mutex.

Profiles may additionally constrain `transports`, `families`, `remote_ports`,
`remote_cidrs`, and `processes`. Kernel socket-prefix events feed family,
endpoint port/IP, process name, and the socket type carried in the existing
append-only ABI word; TLS plaintext events feed process and TCP transport. A
selector whose metadata is unavailable fails closed for that profile rather
than silently matching a broader scope.

HTTP/1 socket start-line classification now performs one bounded 8-byte userspace
probe to distinguish request/response/non-HTTP before copying a start-line. This
replaces the previous request probe followed by a second response probe on the
same syscall while preserving the same query/fragment truncation boundary.

## Hot-path rejection

Kernel HTTP/1 metadata probing is restricted to known `SOCK_STREAM` descriptors.
The socket type is masked with `SOCK_TYPE_MASK`, so `SOCK_NONBLOCK`/`SOCK_CLOEXEC`
flags do not accidentally disable capture. Unknown descriptor provenance keeps the
conservative fallback where the syscall itself still provides a socket endpoint.
This avoids user-memory L7 probes on known UDP/raw sockets without changing the
userspace profile matcher or privacy boundary.


### Dynamic exit-context fast path

Outgoing `write`, `writev`, `sendmsg`, and `sendto` no longer round-trip static
labels through `exit_path_ctx`. The per-thread hash context is populated only
after the bounded HTTP/1 classifier recognizes a real start-line; ordinary TLS,
binary, file, UDP, and non-HTTP stream writes keep their static label entirely
in the exit program. Failed ring-buffer reservations explicitly discard any
dynamic HTTP context, preventing stale per-thread entries.

### Static syscall-label fast path

Fixed syscall labels no longer use `exit_path_buf`/`exit_path_ctx` as a transport
between enter and exit programs. Simple operations such as `ioctl`, permission
changes, process wait/clone labels, basic file-operation labels, `socket()`,
`bind()`, and `recvfrom()` keep only the compact `exit_meta` correlation record;
their constant label is copied directly into the ring-buffer event on sys_exit.
This removes a 512-byte hash-map update plus lookup/delete from each event and
also avoids copying unrelated per-CPU scratch `extra4` bytes into static events.
Dynamic file paths and recognized HTTP start-lines continue to use the existing
bounded context maps.

### Split dynamic path contexts

Dynamic path correlation is now shape-aware. Single-path filesystem syscalls
use a dedicated 256-byte `exit_single_path_ctx`, while dual-path operations and
recognized HTTP metadata retain the 512-byte `exit_path_ctx`. The pair map is
reduced to 1024 entries while the single-path map has 2048 entries, keeping the
combined preallocated value budget approximately equal to the previous 2048 x
512-byte map but halving update/lookup bandwidth for the dominant single-path
case. `connect()` no longer stages its fixed label in either map, and untracked
connects return before `exit_ctx` correlation. Both path maps are explicitly
cleaned when ring-buffer reservation fails after exit metadata is consumed.

### Single-path scratch fast path

The dynamic filesystem path fast path now uses a dedicated 256-byte per-CPU
`exit_single_path_buf` before publishing into `exit_single_path_ctx`. Dual-path
operations and recognized HTTP start-lines continue to use the 512-byte
`exit_path_buf`. This keeps enter-time path snapshots stable while reducing the
cache footprint touched by common one-path syscalls; it deliberately avoids
re-reading user pointers on sys_exit, which could observe mutated or unmapped
path memory.

### Compact enter/exit correlation

Generic filesystem/process syscalls and scalar descriptor lifecycle operations
now correlate through a 32-byte `exit_compact_ctx` value instead of the
network-capable 88-byte `exit_meta`. Socket creation, close, and dup lineage are
also eligible because they only carry scalar fd metadata. Network endpoint,
accepted-peer, payload-pointer, and L7 capture paths continue to use the full
context. This cuts hash-map value bandwidth by about 64% for the compact class
without changing event ABI or enter/exit semantics.

### Untracked network fast reject

`bind()` and `recvfrom()` now return immediately after a zero `get_tag_id()`
result, matching the rest of the network capture handlers. Untracked processes
therefore avoid enter/exit correlation map traffic and can no longer create
tag-zero network events through these two tracepoints.

### Correlation-map pressure observability

Transient enter/exit and socket-provenance maps now expose on-demand pressure
through collector health and Prometheus. The backend enumerates pinned maps only
when health/metrics are queried, so syscall hot paths do not pay an occupancy
counter update. Kernel-side per-CPU `context_pressure_stats` increments only when
a map update actually fails. Metrics include current entries, configured
capacity, key/value payload budget, utilization ratio, and cumulative update
failures for `exit_ctx`, `exit_compact_ctx`, single/pair path contexts,
`socket_fds`, and `socket_fd_parents`. Any observed update failure marks capture
health unhealthy because enter/exit or provenance correlation may be incomplete.


Pressure snapshots are deliberately approximate while hot hash/LRU maps mutate;
iteration is bounded by each map's configured capacity. If the per-CPU failure
map cannot be read, the pressure snapshot is reported unavailable rather than
silently treating failures as zero. Path correlation is also all-or-nothing:
if compact/full correlation or its companion path update fails, the sibling
state is not left behind for a later pid/tgid reuse.

### Path-rule fast reject

Path-bearing syscall tracepoints now split cheap PID/comm matching from expensive
user-path inspection. A tiny pinned `tracking_mode` ARRAY records whether exact
or prefix path rules currently exist. After PID/comm misses, if neither class is
configured the program returns before per-CPU path scratch lookup,
`bpf_probe_read_user_str`, exact-path hash lookup, or LPM prefix matching.
Tracked PID/comm events still snapshot and report their path, and configured
path rules retain exact/prefix semantics. Mode lookup failure is fail-open to
full path matching.

The backend derives mode bits from the real tracked path maps, synchronizes them
before tracepoint links are attached, and refreshes them after official path
configuration mutations. Fresh bootstrap also performs best-effort backup of
legacy tracked maps before replacing an older pin layout, restores rules before
attach, then publishes the matching mode bits, preventing both config loss and
reload-time false rejects when a new required map is introduced.


Path/prefix registry mutations are serialized with mode publication. The backend
snapshots an existing selector before mutation and rolls it back if mode
publication fails. If rollback or precise re-synchronization cannot be trusted,
the mode map is forced to both path classes enabled, preserving capture
correctness at the cost of extra path work rather than allowing a false reject.


### PID-first enter-side selector lookup

Correlation-only syscall enter programs now check the registered Agent PID map
before reading the current command name. For a PID hit, path and scalar syscall
macros skip the enter-side bpf_get_current_comm helper entirely; event
construction on sys_exit still reads and reports the current comm as before.
Only a PID miss pays the comm helper and tracked_comms lookup, and path-bearing
syscalls proceed to tracking_mode/path inspection only after both selectors
miss. Immediate tracepoint emitters such as TCP flow events are intentionally
unchanged because they need comm for the event emitted in that same program.
