# Architecture

This document describes the runtime architecture of **Agent eBPF Filter**.

## High-level view

```text
┌─────────────────────────────────────────────────────────────────────┐
│                           Linux host                               │
│                                                                     │
│  ┌────────────────────┐        ringbuf events       ┌────────────┐  │
│  │ eBPF tracepoints   │ ─────────────────────────▶ │ Go backend │  │
│  │ exec/open/connect  │                            │            │  │
│  │ sendto/recvfrom    │ ◀──── pinned maps ───────▶ │            │  │
│  │ mkdir/unlink/ioctl │                            └─────┬──────┘  │
│  │ bind               │                                           │  │
│  └────────────────────┘                                  │         │
│                                                           │         │
│                                WebSocket / HTTP           │         │
│                                                           ▼         │
│                                                   ┌──────────────┐  │
│                                                   │ Vue frontend │  │
│                                                   └──────────────┘  │
│                                                                     │
│  ┌────────────────────┐        UDS protobuf         ┌────────────┐  │
│  │ agent-wrapper      │ ─────────────────────────▶ │ policy      │  │
│  │ command shim       │ ◀───────────────────────── │ engine      │  │
│  └────────────────────┘                            └────────────┘  │
│                                                                     │
│  ┌────────────────────┐        HTTP register         ┌────────────┐  │
│  │ Python / Node      │ ───────────────────────────▶ │ agent_pids │  │
│  │ adapters           │                              │ map        │  │
│  └────────────────────┘                              └────────────┘  │
│                                                                     │
│  ┌────────────────────┐        HTTP hook callback    ┌────────────┐  │
│  │ Claude / Gemini /  │ ───────────────────────────▶ │ hook event │  │
│  │ Codex / Copilot    │                              │ ingest     │  │
│  └────────────────────┘                              └────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
```

## Main components

### 1. eBPF program

Source:

- `backend/ebpf/agent_tracker.c`

Responsibilities:

- attach to syscall tracepoints
- inspect current PID / command / path
- look up matching tags from pinned BPF maps
- emit normalized kernel events into the ring buffer, including syscall exit timing for strace-style summaries

Tracked syscall entrypoints:

- `execve`
- `openat`
- `connect`
- `sendto`
- `recvfrom`
- `mkdirat`
- `unlinkat`
- `ioctl`
- `bind`

### 2. Pinned BPF state

Pinned under:

- `/sys/fs/bpf/agent-ebpf/maps`
- `/sys/fs/bpf/agent-ebpf/links`

Important maps:

- `agent_pids`
- `tracked_comms`
- `tracked_paths`
- `events`

### 3. Go backend

Main files:

- `backend/main.go`
- `backend/ebpf_runtime.go`
- `backend/shell_sessions.go`
- `backend/privileges.go`

Responsibilities:

- bootstrap / attach / reopen pinned BPF objects
- read ring-buffer events
- convert events to protobuf
- host HTTP + WebSocket API
- manage tags, tracked commands, tracked paths, wrapper rules
- receive native AI CLI hook callbacks
- train and tune local ML policy models, including parameter-grid tuning and cross-model selection over built-in profiles
- expose PTY shell sessions
- enforce wrapper policy over Unix socket

### 4. Frontend

Location:

- `frontend/src`

Responsibilities:

- show live event stream
- show process / system telemetry
- show syscall-derived network flows in a dedicated tab
- browse host filesystem
- edit tracking config
- manage AI CLI hooks
- attach to backend PTY sessions

### 5. Adapters

Locations:

- `adapters/python/agent_tracker.py`
- `adapters/js/agentTracker.js`

Responsibilities:

- register current process with backend
- unregister on shutdown when possible

### 6. Wrapper

Location:

- `wrapper/main.go`

Responsibilities:

- intercept command execution requests
- ask backend for a decision over `/tmp/agent-ebpf.sock`
- apply `ALLOW`, `BLOCK`, `ALERT`, or `REWRITE`
- exec the final command

## Data flows

### A. Kernel event flow

```text
tracked PID / command / path
        │
        ▼
eBPF tracepoint handler
        │
        ▼
ring buffer event
        │
        ▼
Go backend reader
        │
        ▼
protobuf pb.Event
        │
        ▼
/ws
        │
        ▼
Dashboard
```

### B. Wrapper policy flow

```text
user / frontend
   │
   ├─ POST /system/run
   │
   ▼
backend starts agent-wrapper
   │
   ▼
agent-wrapper → /tmp/agent-ebpf.sock
   │
   ▼
backend rule lookup
   │
   ├─ ALLOW
   ├─ BLOCK
   ├─ ALERT
   └─ REWRITE
   │
   ▼
wrapper emits wrapper_intercept event
   │
   ▼
command exec or block
```

### C. Native AI CLI hook flow

```text
AI CLI hook payload on stdin
        │
        ▼
curl POST /hooks/event
        │
        ▼
backend normalizes payload
        │
        ▼
pb.Event(type=native_hook)
        │
        ▼
/ws
```

### D. PTY shell flow

```text
frontend Executor
   │
   ├─ POST /shell-sessions
   │
   ▼
backend creates PTY-backed shell
   │
   ├─ GET /shell-sessions
   └─ GET /ws/shell?session_id=...
       │
       ▼
interactive WebSocket terminal
```

## TLS 明文捕获

```
eBPF uprobes -> tls_events ringbuf -> TLSProbeManager -> FragmentAssembler -> HTTP parser -> TLSCaptureStore -> /ws/tls-capture -> Vue TLSCapture
```

- OpenSSL/GnuTLS/NSS 静态库通过 `link.OpenExecutable` 挂载 uprobe/uretprobe
- Go `crypto/tls` 二进制通过 ELF 符号解析和自动 /proc 扫描挂载
- 分片在 `FragmentAssembler` 中按 TGID+TimestampNS+Direction 拼装
- HTTP 解析器检测 request/response，敏感 header 脱敏，非 HTTP 负载 hex dump 回退

## Matching model

The kernel filter currently uses **exact-match** maps:

- `agent_pids`: process ID match
- `tracked_comms`: executable name match
- `tracked_paths`: exact path string match

This means:

- PID tracking starts at the registered process, then inherits to descendants through fork / clone lineage plus user-space parent fallback
- command tracking works best for short executable names
- path tracking is not recursive subtree tracking

## Privilege model

The backend must be privileged to manage eBPF objects.

Runtime pattern:

1. start backend
2. detect whether backend already has needed privileges
3. relaunch via `sudo` or `pkexec` if needed
4. open or bootstrap pinned maps and links

Child shells / commands then attempt to drop back to the invoking user.

## Port model

- Backend listens on the first free port in `8080..8089`
- Selected port is written to `backend/.port`
- Frontend dev proxy reads that file
- adapters can also use that file as a local fallback
- native hook callback installation also resolves callback URL from the current port

## Auth model

In release mode, the runtime access token now protects:

- `/config/**`
- `/system/**`
- `/ws*`
- `/metrics`
- `/events/recent`
- `/events/graph`
- `/register`
- `/unregister`
- `/shell-sessions*`

`/hooks/event` accepts either that token or a per-hook `X-Agent-Hook-Secret`.
Dangerous features such as PTY sessions, `/system/run`, hook installation, and policy mutation are also runtime-gated and default to disabled until enabled in `/config/runtime`.

Treat the app as a local workstation tool unless you also harden auth and deployment boundaries.

## Export model

- `GET /ws/envelopes` exposes the normalized `EventEnvelope` stream for downstream consumers.
- `GET /metrics` exposes local Prometheus counters and gauges for ringbuf health, queue depth, WS clients, persist latency, and per-type / per-pid event totals.
- OTLP HTTP export is configured from `/config/runtime` and currently derives:
  - `agent.run` spans from `agent_run_id` / `root_agent_pid`
  - `codex.task` spans from `task_id` or conversation+turn fallback
  - `tool.call`, `llm.call`, `pr.review`, or `mcp.call` spans from the normalized tool context
  - child spans / span events for process, file, network, wrapper, hook, and policy events
- `GET /system/otel-health` reports exporter readiness, queue depth, active synthetic span counts, dropped exporter events, and the last export timestamp / error.

## Benchmark model

- `benchmarks/runtime-replay/scenarios.json` is the offline replay scenario catalog.
- `make runtime-benchmark` runs `TestRuntimeReplaySuite` and writes a JSON summary under `reports/runtime-replay-*`.
- The replay suite checks:
  - semantic alert coverage
  - benign false positives
  - wrapper decision latency
  - first-alert / block latency
  - child-context correlation accuracy
- Live collector metrics such as ringbuf drops still come from `/system/collector-health` and `/metrics`, not from the offline replay harness.
