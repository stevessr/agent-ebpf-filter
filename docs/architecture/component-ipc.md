# Local component IPC contract (phase 5)

> **Status:** opt-in library only, not yet enabled by the Collector, Renew, the Agent engine or Policy Controller. This phase adds no new listening socket and does not change existing `desktop_session`, wrapper UDS, HTTP, kernel maps or privileges. It prepares a narrow authenticated boundary before deploying multiple processes.

## Existing vs proposed paths

Current production paths remain:
- Privileged backend owns a **single** eBPF loader, attached programs/maps and ringbuf reader.
- `backend/udsframe` is already shared by wrapper and desktop-native communication, which have their **own** compatible protocols.
- Existing Renew uses its desktop lifetime socket, private API UDS and native frame stream.

The separate `backend/internal/componentipc` package defines a **new** private inter-component protocol. Do not connect it to the existing desktop or wrapper socket: this would break protocol compatibility and trust boundaries.

## Handshake v1 and identity

1. Each side provides an explicit `VerifyPeer(net.Conn)` callback. On Linux use `VerifyUnixPeerUIDs(allowed...)`, which checks `SO_PEERCRED` and fails closed for non-Unix connections or empty UID allowlists. Root is not implicitly trusted; the caller must explicitly allow its UID.
2. The server sends protocol version (16-bit big-endian), server role (1 byte) and 32 cryptographically random nonce bytes.
3. Client checks version, expected server role and peer UID; responds with protocol version, client role and HMAC-SHA256 over a domain-separated transcript containing nonce + both roles + protocol version.
4. Server checks its role allowlist, verifier and client HMAC using constant-time comparison. It replies with a **different**, server-labeled HMAC of the same transcript. Client verifies to authenticate the server's possession of the secret.
5. Each side rejects invalid/short/oversized frames; handshake has a 5-second deadline and closes the connection on failure.

The shared secret is a *deployment credential*. Future launchers must issue a per-session random secret of at least 32 bytes over a protected inherited descriptor, not arguments, logs, a world-readable settings file or ambient environment. The current package does **not** generate, store, rotate or distribute secrets; callers must not introduce insecure distribution when integrating it.

SO_PEERCRED verifies Linux UID, not an executable identity. HMAC proves possession of the ephemeral capability; it does not protect against a compromised authorized process. Future deployment must also ensure a private, owner-controlled `0700` parent directory, restricted socket permissions, safe unlink-by-inode, reconnect/lifecycle handling and no remote TCP fallback.

## Roles and message permissions

| Authenticated sending role | Allowed message kinds |
| --- | --- |
| Collector | event, status, ack, heartbeat |
| Engine | alert, status, ack, heartbeat |
| Controller | status, ack, heartbeat |
| Gateway / UI | status, ack, heartbeat |

The protocol **does not expose policy apply, BPF map mutation, shell execution, plugin loading or arbitrary commands**. Those require a separate review, authorized request semantics, transactional apply/rollback and audit trail before enabling.

Each post-handshake message uses existing `udsframe` length prefix + type byte + opaque payload, at most 1 MiB per message. The decoder validates the sender's authenticated role before returning borrowed payload bytes. Payloads are not protobuf-versioned yet; the higher-layer canonical event and health schemas must be added before attaching production components.

## Queue and backpressure

`componentipc.Outbox` is a one-shot single-sender worker, with nonblocking `TryEnqueue`, a hard limit of 64 pending messages, maximum 1 MiB/message, a 1-second bounded write, and accepted/dropped/sent/failed counters. Accepted payloads are **copied** so the producer can reuse its buffer. On saturation, callers receive `QueueSaturated`; drops must be surfaced in collector health and cannot be interpreted as successful delivery. Cancellation stops accepting; it does not guarantee that pending records reached the receiver.

`KindAck` is reserved for explicit sequencing/credits but **credit-based flow control and replay/recovery are not yet implemented**. Before process separation, implement receive acknowledgments, sequence IDs, reset/resume handling, priority for DENY/ALERT and persisted drop reasons. Until then, treat this transport as best-effort telemetry only; no enforcement action may depend on event delivery succeeding.

## Validation

Rootless Linux tests include:
- mutual handshake over a real Unix socket with SO_PEERCRED, rejection of wrong UID and non-UDS connections;
- wrong key, role, version, and missing verifier rejection;
- role-based frame validation and size limits;
- copied payload ownership and bounded nonblocking queue under cancellation;
- `go test -race`, `go vet` and `gofmt` in CI.

```sh
cd backend
go test -race ./internal/componentipc -count=1
go vet ./internal/componentipc
```

## Implementation order after this PR

1. Implement isolated typed **event/status schemas**, transport sequence IDs and durable drop accounting.
2. Wire a single privileged Collector *publisher* behind an **off-by-default** feature gate; retain current in-process event handling as default.
3. Run a non-privileged Engine subprocess against the private UDS with explicit session-scoped credentials, UID verification and bounded queues, then compare event parity against embedded mode.
4. Introduce controller policy apply only after versioned requests, readback verification, transactional rollback, auditability and privileged kernel smoke tests.
5. Preserve Renew one-click launch and single privileged BPF owner; the app supervisor must own every child process and cleanly join on shutdown.
