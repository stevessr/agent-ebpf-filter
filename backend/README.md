# Backend

The backend is the privileged runtime of the project.

It is responsible for:

- loading / pinning eBPF maps and links,
- consuming ring-buffer events from the kernel,
- serving HTTP and WebSocket APIs,
- aggregating process / system telemetry,
- managing wrapper rules,
- receiving native AI CLI hook callbacks,
- hosting PTY shell sessions,
- routing cluster traffic through a master backend when cluster targets are selected.

## Key files

- `main.go` — routes, event broadcasting, system metrics, hook management, wrapper UDS
- `ebpf_runtime.go` — bootstrap / pin / privilege escalation flow; auto-attaches every tracepoint program compiled from `ebpf/agent_tracker.c` and skips tracepoints the running kernel does not expose
- `shell_sessions.go` — persistent PTY session manager
- `privileges.go` — drop spawned commands back to the invoking user
- `ebpf/agent_tracker.c` — eBPF source
- `ebpf/gen.go` — `bpf2go` generation entrypoint

## Privilege model

The backend needs elevated privileges to create and attach eBPF programs.

Runtime behavior:

1. start backend normally,
2. backend checks whether it is already privileged,
3. if not, it relaunches itself, preferring desktop/polkit elevation (`pkexec`) when a graphical session is available, otherwise falling back to `sudo`,
4. eBPF maps and links are pinned under `/sys/fs/bpf/agent-ebpf`, and compiled tracepoint programs are attached when the running kernel exposes the matching tracepoint.

Spawned shells and wrapper-launched commands attempt to drop privileges back to the original invoking user using `SUDO_UID` / `SUDO_GID`.

## Pinned objects

Pinned map directory:

- `/sys/fs/bpf/agent-ebpf/maps`

Pinned link directory:

- `/sys/fs/bpf/agent-ebpf/links`

Required maps:

- `agent_pids`
- `events`
- `tracked_comms`
- `tracked_paths`

## WebSocket streams

### `/ws`

Broadcasts `pb.Event` messages sourced from:

- kernel eBPF ring-buffer events, including syscall-derived network flow records,
- wrapper interceptions,
- native AI CLI hook callbacks.

Kernel event payloads include syscall exit duration so the dashboard can render strace-style summaries without requiring a separate tracer.
They also carry `schema_version`, `gid`, `cgroup_id`, and inherited agent-run context when available. The backend now also normalizes them into versioned `EventEnvelope` records with `task_id` / `cwd` support for downstream consumers and can translate those envelopes into OTLP spans (`agent.run`, `codex.task`, `tool.call`, `mcp.call`, plus child process / file / network / policy spans).

### `/ws/system`

Broadcasts `pb.SystemStats` messages that include:

- process list
- CPU usage
- memory stats
- GPU stats
- network and disk IO
- VM page-fault / swap counters

### `/ws/shell-sessions`

Broadcasts the full shell session list as JSON text messages whenever the session list changes.

Uses a pub/sub pattern:

- clients subscribe on connect, unsubscribe on disconnect,
- the server sends the current `shellSessions.List()` immediately and re-sends on every `Create`, `Delete`, or session state change,
- the broadcast is driven by `shellSession.onChange` callbacks and `shellSessionManager.notify()`.

### `/ws/shell`

Attaches to a persistent PTY session created through `/shell-sessions`.

Current behavior:

- one backend session may have **one active WebSocket attachment at a time**,
- the backend keeps a bounded output backlog so reconnecting clients can receive recent output.

`POST /shell-sessions` accepts either a normal shell launch, a wrapper-backed temporary terminal, or a custom command + args payload, which is what the Executor page uses for the Remote tab, tmux-backed coding CLIs, script runners, and shared launch environment overrides.
`GET /system/env` returns a filtered list of the backend process environment so the Executor launch-env tab can suggest already-present variables without leaking backend-only config such as `AGENT_*`, `GIN_MODE`, or `DISABLE_AUTH`.
`POST /shell-sessions/:id/input` can inject raw bytes into an existing PTY session, which the tmux quick manager uses to send `Ctrl-b` shortcuts.

## HTTP endpoints

### Release-mode authenticated routes

The runtime access token protects:

- `GET /events/recent?type=&limit=` — historical events (used for initial WS load); each record now also includes a normalized `Envelope`
- `GET /events/graph?...` — aggregated execution graph API for the current event retention window
- `GET /ws/envelopes` — live `pb.EventEnvelopeBatch` stream for normalized event consumers
- `GET /metrics` — Prometheus exposition for collector / queue / WS / per-type / per-pid counters
- `GET /system/otel-health` — OTLP exporter readiness / queue / active span counts
- `GET /ws/shell-sessions` — live shell session list (WebSocket JSON push)
- `POST /register`
- `POST /unregister`
- `POST /shell-sessions`
- `GET /shell-sessions`
- `DELETE /shell-sessions/:id`
- `POST /shell-sessions/:id/input`
- `GET /ws/shell`
- `GET /ws`
- `GET /ws/system`
- `GET /ws/camera`
- `GET /ws/sensors`
- `GET /ws/microphone`
- `GET /ws/ml-status`

`POST /hooks/event` accepts either the normal token or a per-hook secret via `X-Agent-Hook-Secret` + `X-Agent-CLI`.

### Routes behind `authMiddleware()` in release mode

Config routes:

- `/config/tags`
- `/config/comms`
- `/config/paths`
- `/config/rules`
- `/config/export`
- `/config/import`
- `/config/runtime`
- `/config/access-token`
- `/config/hooks`
- `/config/hooks/:id/raw`
- `/config/ml/existing-commands`, `/config/ml/import-existing`, `/config/ml/assess`
- `/config/ml/llm/production-dataset/pull` — pull a cleaned OpenAI chat-style JSONL preview from the current training store for LLM fine-tuning
- `/config/ml/datasets/pull`, `/config/ml/datasets/import`, `/config/ml/datasets/export`, `DELETE /config/ml/datasets`
- the ML config also supports OpenAI-compatible LLM scoring and post-training review; the frontend persists the LLM base URL, model, API key, timeout, temperature, max tokens, and validation split ratio
- the dataset importer accepts raw HTTP/HTTPS payloads or local file uploads, and will recursively expand common archives / compressed payloads such as zip, tar, gzip, bzip2, and xz before parsing rows
- the frontend also exposes a curated classic OS-security dataset catalog for reference; one-click presets carry their own import format/label mode, and archival pages still need you to download or extract the actual data first

`authMiddleware()` accepts `?key=<token>`, `X-API-KEY`, or `Authorization: Bearer <token>`.
The token is generated and stored by the runtime settings file at:

- `~/.config/agent-ebpf-filter/runtime.json`

Runtime feature flags in `/config/runtime` default dangerous capabilities to off:

- `shellSessionsEnabled`
- `systemRunEnabled`
- `hookManagementEnabled`
- `policyManagementEnabled`

That means shell sessions, `/system/run`, hook installation / raw hook writes, and policy mutations must be explicitly enabled before their mutating routes succeed.

### Cluster control

Cluster mode is configured entirely through environment variables:

- `AGENT_CLUSTER_MASTER_URL`
- `AGENT_CLUSTER_ACCOUNT`
- `AGENT_CLUSTER_PASSWORD`

If all three are present, the backend starts in **slave** mode and heartbeats to `AGENT_CLUSTER_MASTER_URL`. Otherwise it stays in **master** mode.

Optional identity overrides:

- `AGENT_CLUSTER_NODE_URL`
- `AGENT_CLUSTER_NODE_ID`
- `AGENT_CLUSTER_NODE_NAME`

Cluster state routes:

- `GET /cluster/state`
- `GET /cluster/nodes`

In master mode, supported web/API/WS paths can be forwarded to a selected slave target by sending `X-Cluster-Target` or `?cluster=<target>`. The master injects cluster credentials internally when proxying to the slave.

Export / import currently covers:

- tags
- tracked commands
- tracked paths
- wrapper rules
- runtime settings

System routes:

- `/system/ls`
- `/system/collector-health`
- `/system/otel-health`
- `/system/run`

MCP:

- `/mcp`

The MCP server exposes event-tail and configuration-snapshot tools over SSE and uses the same runtime access token as the HTTP config routes.

Persistent event logs, when enabled from the Configuration page, are appended as JSONL at:

- `~/.config/agent-ebpf-filter/events.jsonl`

The collector health endpoint reports ringbuf event totals, reserve-fail / drop counts, backend queue length, event-stream WS client count, recent persisted-log append latency, and simple per-event-type counters so the frontend can warn when capture may be incomplete.
The OTLP health endpoint reports whether export is enabled / ready, the configured endpoint + service name, exporter queue length, active synthetic run / task / tool spans, total exported spans, dropped exporter events, and the last export error / timestamp.

Offline replay coverage now lives in the repo-level runtime benchmark suite:

- `benchmarks/runtime-replay/scenarios.json`
- `make runtime-benchmark`
- `reports/runtime-replay-*/summary.json`

## Wrapper integration

The backend exposes a Unix-domain socket at:

- `/tmp/agent-ebpf.sock`

`agent-wrapper` sends `pb.WrapperRequest`, the backend applies wrapper rules, then returns `pb.WrapperResponse`. The request can include optional run / trace metadata (`agent_run_id`, `task_id`, `tool_call_id`, `trace_id`, `span_id`, `root_agent_pid`, `argv_digest`, `cwd`, etc.) so descendant kernel events inherit the same execution context.
The socket is created with `0600` permissions and peer credentials are checked so only root or the original invoking user may connect.

Supported actions:

- `ALLOW`
- `BLOCK`
- `ALERT`
- `REWRITE`

## Hook integration

Supported hook targets:

- Claude Code
- Gemini CLI
- Codex
- GitHub Copilot CLI
- Kiro CLI
- Cursor (wrapper alias mode)

Native hook configs are resolved relative to the real user home directory:

- `~/.claude/settings.json`
- `~/.gemini/settings.json`
- `~/.codex/hooks.json`
- `~/.kiro/agents/agent-ebpf-hook.json`
- `~/.copilot/config.json`

Codex also requires the experimental feature flag below in `~/.codex/config.toml`, which the backend now enables automatically during native-hook install:

```toml
[features]
codex_hooks = true
```

Kiro native-hook install creates a managed agent cloned from `kiro_default` and temporarily points `chat.defaultAgent` in `~/.kiro/settings/cli.json` to that managed agent. On uninstall, the previous default agent is restored.

Wrapper aliases are written to:

- `~/.bashrc` or `~/.zshrc`

When native hooks are installed, the callback URL resolves from:

1. `AGENT_HOOK_ENDPOINT`, if set
2. current backend port from `.port`
3. fallback `http://127.0.0.1:8080/hooks/event`

Native hook entries call a generated relay script under the target CLI config directory's `hooks/` subdirectory instead of embedding a long inline `curl` command directly in the hook config.
Those relay scripts now send both `X-Agent-CLI` and a per-hook `X-Agent-Hook-Secret` header.
During event broadcast, the backend may also synthesize `semantic_alert` events (for example `SECRET_ACCESS`, `UNEXPECTED_NETWORK_EGRESS`, `UNEXPECTED_CHILD_PROCESS`, or `SEMANTIC_MISMATCH`) when child behavior conflicts with read-only style tool intent.

## Build notes

Regenerate eBPF bindings:

```bash
cd ebpf && go generate
```

Build backend:

```bash
go build -o agent-ebpf-filter
```

Or from the repo root:

```bash
make backend
```
