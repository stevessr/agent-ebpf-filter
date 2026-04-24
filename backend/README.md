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
- `ebpf_runtime.go` — bootstrap / pin / privilege escalation flow
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
4. eBPF maps and links are pinned under `/sys/fs/bpf/agent-ebpf`.

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

### `/ws/system`

Broadcasts `pb.SystemStats` messages that include:

- process list
- CPU usage
- memory stats
- GPU stats
- network and disk IO
- VM page-fault / swap counters

### `/ws/shell`

Attaches to a persistent PTY session created through `/shell-sessions`.

Current behavior:

- one backend session may have **one active WebSocket attachment at a time**,
- the backend keeps a bounded output backlog so reconnecting clients can receive recent output.

## HTTP endpoints

### Public / currently unauthenticated routes

- `POST /register`
- `POST /unregister`
- `POST /hooks/event`
- `POST /shell-sessions`
- `GET /shell-sessions`
- `DELETE /shell-sessions/:id`
- `GET /ws/shell`
- `GET /ws`
- `GET /ws/system`

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

`authMiddleware()` accepts `?key=<token>`, `X-API-KEY`, or `Authorization: Bearer <token>`.
The token is generated and stored by the runtime settings file at:

- `~/.config/agent-ebpf-filter/runtime.json`

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
- `/system/run`

MCP:

- `/mcp`

The MCP server exposes event-tail and configuration-snapshot tools over SSE and uses the same runtime access token as the HTTP config routes.

Persistent event logs, when enabled from the Configuration page, are appended as JSONL at:

- `~/.config/agent-ebpf-filter/events.jsonl`

## Wrapper integration

The backend exposes a Unix-domain socket at:

- `/tmp/agent-ebpf.sock`

`agent-wrapper` sends `pb.WrapperRequest`, the backend applies wrapper rules, then returns `pb.WrapperResponse`.

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
