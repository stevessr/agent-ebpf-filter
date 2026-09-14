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
