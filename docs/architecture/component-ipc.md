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
5. Each side rejects invalid/short/oversized frames; handshake has a 5-second deadline, explicitly clears it before returning a live session and closes the connection if clearing fails.

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

Each post-handshake message uses existing `udsframe` length prefix + type byte + opaque payload, at most 1 MiB per message. The decoder validates the sender's authenticated role before returning borrowed payload bytes. Concurrent receive calls are serialized; callers must consume or copy borrowed bytes before the next receive. `Session.ReceiveCopy()` returns caller-owned bytes that remain valid across later receives, including from another goroutine. A failure to clear a post-send deadline closes the session. Payloads are not protobuf-versioned yet; the higher-layer canonical event and health schemas must be added before attaching production components.

## Queue and backpressure

`componentipc.Outbox` is a one-shot single-sender worker, with nonblocking `TryEnqueue`, a hard limit of 64 pending messages (plus at most one in-flight frame), maximum 1 MiB/message, a 1-second bounded write, and accepted/dropped/sent/failed counters. `Rejected` tracks offers refused before admission; `Abandoned` counts admitted frames never passed to Send, including dequeue/shutdown races. `Dropped = Rejected + Abandoned`. Attempted writes that fail count as `Failed`, not `Abandoned`. Accepted payloads are **copied** so the producer can reuse its buffer. On saturation, callers receive `QueueSaturated`; drops must be surfaced in collector health and cannot be interpreted as successful delivery. Cancellation stops accepting and counts undelivered queued frames as drops; an explicit `Stop` additionally wakes an idle `Run(context.Background())` worker and returns normally. Concurrent stop is idempotent and never closes the producer queue. In-flight writes remain subject to their one-second deadline, and neither accepted nor in-flight records are guaranteed to have reached the receiver.

The optional `Sequenced` envelope and `Ack` receipt codec now define a versioned **transport metadata** schema, but **automatic ACKs, credit-based flow control and replay/recovery are not implemented**. An ACK must be sent only after application processing; no ACK is generated by `Receive`. Before process separation, implement durable replay/resume and persisted drop reasons. Before process separation, implement receive acknowledgments, sequence IDs, reset/resume handling, priority for DENY/ALERT and persisted drop reasons. Until then, treat this transport as best-effort telemetry only; no enforcement action may depend on event delivery succeeding.

## Validation

Rootless Linux tests include:
- mutual handshake over a real Unix socket with SO_PEERCRED, rejection of wrong UID and non-UDS connections;
- wrong key, role, version, and missing verifier rejection;
- role-based frame validation and size limits;
- copied payload ownership, rejected/abandoned/failed write accounting, and bounded nonblocking queue under cancellation;
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

## Opt-in sequenced transport metadata (phase 5 extension)

This is a **library-only opt-in** wire contract and does not change existing `Session.Send`, `Session.Receive`, `udsframe` or desktop/native IPC messages. Both sides must deliberately opt into `SendSequenced` and the strict `ReceiveRecordCopy` decoder (or explicitly pair `ReceiveCopy` with `DecodeRecord`); mixing a raw body with a sequenced body is invalid. The strict receiver closes the session on an invalid schema, malformed ACK, or raw heartbeat; it never guesses how to resynchronize. It is deliberately not yet a canonical domain Event/Status protobuf schema.

After the outer `udsframe` 4-byte big-endian length and one-byte message kind, an event, alert or status body is:

| Offset | Width | Type | Requirement |
| --- | --- | --- | --- |
| 0 | 2 | uint16 big-endian | `SequencedSchemaVersion = 1` |
| 2 | 16 | random stream ID | nonzero, freshly generated for each new stream |
| 18 | 8 | uint64 big-endian | sequence starts at 1; zero is invalid |
| 26 | 1..(1 MiB - 26) | opaque payload | nonempty, bounded |

An ACK uses the same 26-byte metadata header **without any payload**. `EncodeAck`/`DecodeAck` reject extra bytes, invalid versions, zero IDs and zero sequence values. Only `KindEvent`, `KindAlert` and `KindStatus` accept sequenced application bodies; role restrictions are still enforced by `Session.Send` and `Session.Receive`. The stream ID is **not an authentication secret** and does not replace HMAC or `SO_PEERCRED`.

`SequenceReceiver.Check` rejects a foreign stream, stale/duplicate sequence or a gap; it does **not** mark a record processed. The owning application must serialize processing, verify the domain payload, apply any safe read-only side effects, call `Commit` only after success, then optionally `SendAck`. `Commit` produces the cumulative processed ACK, but neither sends it nor persists it. `AckTracker.MarkSent` is called **only after a successful socket write**; `ApplyAck` rejects foreign, duplicate, stale or future ACKs and records the last confirmed sequence. These helpers do not implement retransmission, process-failure recovery, persistent checkpointing or exactly-once semantics; socket writes alone never imply that the peer processed a record.

On reconnection, callers must create a fresh stream ID and explicitly establish a new baseline. Never reset an existing ID to sequence 1 or claim old unacknowledged records have been delivered. Events with security significance must not be discarded or considered enforced solely because this opt-in transport produced an ACK.

Validation adds codec bounds/version/role tests, gap/duplicate/foreign-stream handling and an authenticated `net.Pipe` event-to-ACK round trip. Kernel privilege, persistent replay and production process supervision remain outside this phase.


// Operator convenience: one-command rootless IPC doctor

The optional `component-ipc-doctor` command is an **isolated self-test**, not a
live production backend status probe. It creates a private temporary `0700`
directory and an owner-only `0600` Unix socket, generates a fresh 32-byte
in-memory secret, performs a bidirectionally UID-verified HMAC handshake,
sends one versioned event, checks its sequence, and receives an explicit ACK.
The socket and directory are removed when the process exits. No eBPF hooks,
policy maps, existing Renew sockets, or privileged commands are touched.

From the repository root, on Linux:

```sh
make ipc-doctor
make ipc-doctor IPC_DOCTOR_ARGS="--json"
# Or without Make: cd backend && go run ./cmd/component-ipc-doctor --json
```

Outputs include per-stage **pass/fail**, a bounded execution time, and
machine-readable `status`, `scope: isolated_local_self_test`,
`checks`, and `duration_ms`. Exit code 0 means the isolated test passed;
1 indicates a failed check, and 2 means invalid CLI arguments. The `--timeout`
flag overrides the 8-second default. Failure checks are reported to the
terminal or JSON output without exposing the secret or raw event body.

This is useful when working on IPC protocol or diagnosing Linux runtime
permissions. A successful self-test **does not mean Collector/Engine are
running**, does not verify an active desktop's health, and does not prove that
durable replay or kernel enforcement works.

## Phase 6: separate Engine process boundary (rootless opt-in)

`backend/internal/componentwire` now defines a **minimal typed and bounded
JSON event projection** with schema `component.event.v1` and only whitelisted
fields (event ID, source, timestamp, event type, PID, decision and risk score).
Unknown JSON fields, trailing content, invalid versions and malformed records
are rejected. In particular raw argv, prompts, paths, secrets and packet
contents cannot be represented in this projection. The future privileged
Collector adapter will map **already-redacted** `pb.EventEnvelope` values
into this deliberately smaller independent schema. Unlike a direct protobuf
import, this projection can compile without generated `pb` dependencies.
The original event normalization, policy maps, risk feedback, HTTP APIs,
wrapper sockets and Renew startup behavior remain unchanged.

`backend/internal/componentproc` contains a Linux-only demonstration of a
**real, supervised child process**, rather than an in-process goroutine.
`RunEngine(ctx, absoluteExecutable, publisher)` creates a private `0700`
temporary directory, a `0600` UDS and a 32-byte random session secret,
starts `backend/cmd/component-engine-worker` with a minimal environment,
and supplies the secret only through inherited **FD 3**. The socket path is
not a secret. The Collector and Engine both verify Linux `SO_PEERCRED` and
complete the mutual v1 HMAC handshake before accepting traffic. The
supervisor owns shutdown, closes its sockets, joins/kills the worker on
cancellation and removes its private directory.

This **does not yet start from `app.Main()` or Renew**. A root supervisor is
explicitly rejected rather than accidentally launching a root Engine;
the current demonstration only supports a *same-UID non-root* supervisor and
worker. A real privileged Collector must introduce explicitly configured
UID/GID dropping, a separate private socket accessibility policy and a
reviewed launcher before production opt-in. Same-UID subprocess isolation is
a process-lifecycle boundary, **not** a hard OS security boundary.

The worker currently validates and consumes normalized EventEnvelope records
and returns cumulative ACKs only after successful decode/check/commit.
It never issues alerts, runs AI/ML inference, mutates BPF maps or implements
controller RPCs. Repeated or skipped sequences, malformed projection envelopes
and unexpected kinds terminate the worker; there is **no automatic replay,
deduplication across restarts, credit flow control, durable cursors or policy
decision equivalence**. A manual publisher uses `SendEvent` (which itself validates and encodes
only the whitelisted projection) to wait for an explicit ACK, which means the envelope was decoded/validated, not evaluated
by any security engine.

An unprivileged Linux end-to-end test compiles the child executable, launches
it as a separate OS process, sends two typed event projections and checks ACKs,
then verifies malformed protobuf causes worker failure with no ACK.
Run locally:

```sh
cd backend
go test -race -count=1 ./internal/componentwire ./internal/componentproc
go vet ./internal/componentwire ./internal/componentproc
```

This is phase **6a** only. Next: explicit dedicated engine UID, safe
supervisor integration, production-compatible event parity tests, and later
controlled cutover of the current in-process engine behind an off-by-default
feature flag. Do not advertise subprocess schema validation as a replacement
for existing security decisions.


## Phase 6b: opt-in shared-memory data-plane prototype (not production)

The Linux-only `backend/internal/componentshm` package adds a bounded
single-producer/single-consumer **memfd + MAP_SHARED** ring with an **eventfd**
wakeup descriptor. This stage is an independently testable data-plane prototype:
the production Collector/Engine still use their existing paths, and
`componentproc.RunEngine` is **not** automatically converted to shared memory.
A later supervised integration must hand descriptors only to authenticated
participants and preserve the existing HMAC/UID-checked UDS control channel.

- The writer creates a size-sealed anonymous memfd
  (`F_SEAL_GROW | F_SEAL_SHRINK | F_SEAL_SEAL`), fixed-width bounded slots,
  and a nonblocking CLOEXEC eventfd. Memory size limits are 2..4096
  power-of-two slots with 64..16384 bytes per event. There are no external
  shared-memory pathnames or persistence.
- Writer `TryWrite` rejects oversized/empty data, never blocks and increments
  the rejection count on saturation. The reader `ReadCopy` validates the
  encoded slot length before copying, then publishes the consumer cursor.
  The ring never overwrites unread data.
- 64-bit publication/consumption cursors are aligned to separate cache lines.
  Producer and consumer use Go sequentially consistent atomics to publish
  ownership across MAP_SHARED. This layout currently targets Linux platforms
  where 64-bit atomics on shared, aligned mappings are supported; mixing
  architectures/endianness or using multiple writers/readers is unsupported.
- A publisher calls `Notify` once per batch, rather than per event.
  Reader `WaitReady` checks ring occupancy before polling eventfd; it uses
  bounded 100-ms polling to allow cancellation if a publisher disappears.
  `Finish` marks end-of-stream and wakes the reader, but does **not** mean
  validation, ACK or durable receipt.
- Descriptors may be handed to an explicitly supervised child as inherited
  files (or later over an authenticated UDS with SCM_RIGHTS). `OpenReader`
  validates seals, layout magic/version, file size and bounded dimensions.
  Both writer and reader currently require writable mappings for their
  respective head/tail cursors, so **same-UID shared memory is not an adversarial
  OS isolation boundary**. The security design must separate read-only event
  pages from writable consumer cursor pages and constrain the Engine UID
  before privileged production rollout.

Tests cover slot boundaries, buffer wraparound, saturation/overflow counting,
owned-buffer semantics, malformed record and unsupported header rejection,
timeouts, termination, and a real second process with inherited FD3/FD4
that consumes 250 ordered events. A microbenchmark exercises grouped 64-event
writes + one eventfd notify; it is **not** an eBPF-to-AI end-to-end latency
measurement and makes no claim of zero copying.

```sh
cd backend
go test -race -count=1 ./internal/componentshm
go test ./internal/componentshm -run '^$' -bench BenchmarkRingBatch64 -benchmem
```

Next steps before adopting this data plane: one stable end-to-end mode switch,
control-plane authenticated FD transfer, multi-batch ACK checkpoints,
PID/UID process isolation, explicit reconnect and loss reasons, benchmark
comparison versus UDS using equivalent payloads, and actual kernel-event
parity tests. No security enforcement action may depend on this prototype.


### Rootless Engine integration of the shared data plane (experimental)

Phase 6b now also has **opt-in, real-process** integration:
`componentproc.RunEngineShared(ctx, executable, slots, payload, publisher)`
launches the existing rootless Engine binary with `--shared`. The parent
passes the already-authenticated session's secret through FD3 and the
sealed-size event memfd plus eventfd through FD4/FD5. The Engine verifies the
parent's UID and HMAC on the original private UDS, then validates shared
`component.event.v1` projections without receiving one socket frame per
event. Only cumulative per-batch validation ACKs go back over UDS.

`SendSharedBatch` encodes and validates all events before writing, rejects
oversized or unavailable-capacity batches, emits one eventfd wakeup, and waits
for authenticated cumulative ACKs with matching stream identity. On failure,
treat an incomplete batch as ambiguous; **never silently retry it**. Engine
exits with an error and does not ACK malformed projections. On success the
supervisor marks the ring closed, wakes the reader, closes its socket and
joins the Engine. This supports comparing UDS-only vs memfd modes, but does
not yet offer automatic mode switching or a production data-plane toggle.

End-to-end CI tests launch an actual Engine subprocess, send **200 events
in eight batches** with ACKs on the authenticated UDS, verify the shared
consumer tail and reject malformed events. The production embedded
Collector/Engine, Renew, and kernel policy paths still do not call this
prototype. Privileged process separation and crash replay are not implied.
