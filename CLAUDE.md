# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build commands

```bash
make dev          # Dev mode: parallel backend (hot-reload via scripts/dev-backend.sh) + Vite frontend via Nx
make backend      # Build Go backend + compile eBPF (requires clang, BTF)
make wrapper      # Build agent-wrapper CLI binary
make frontend     # Build Vue 3 frontend (requires bun)
make proto        # Regenerate all protobuf bindings from proto/tracker.proto
make ebpf-bootstrap  # Pre-build backend binary (bootstrap happens on first run)
make run          # Production build + run (serves compiled frontend from backend)
make all          # Build all components (proto + backend + frontend + wrapper)
make deps         # Install Go, Python (uv), and frontend (bun) dependencies
make dev-backend  # Backend only with hot-reload (scripts/dev-backend.sh)
make dev-frontend # Frontend Vite dev server only
make run-backend  # Build and run backend only
make run-frontend # Frontend dev server only
make clean        # Remove all build artifacts
make help         # List all Make targets
```

`make dev` uses Nx to run backend and frontend in parallel. Nx is installed via `bun install` at the repo root (not inside `frontend/`). The backend hot-reload is handled by `scripts/dev-backend.sh`.

After changing `backend/ebpf/agent_tracker.c`:
```bash
cd backend/ebpf && go generate
cd backend && go build ./...
```

After changing `proto/tracker.proto`:
```bash
make proto
```

## Code architecture

### Data flow

```
eBPF ringbuf → Go backend (main.go) → WebSocket (/ws) → Vue 3 frontend
                                    → JSONL file (optional persistence)
                                    → MCP SSE endpoint (/mcp)
agent-wrapper → UDS (/tmp/agent-ebpf.sock) → backend policy engine → ALLOW/BLOCK/ALERT/REWRITE
```

### Backend (`backend/`)

- `main.go` — HTTP/WS routes, event loop, wrapper UDS server, hook management, config APIs (~2000+ lines)
- `ebpf/agent_tracker.c` — BPF program tracing 9 syscalls (execve, openat, connect, mkdirat, unlinkat, ioctl, bind, sendto, recvfrom)
- `ebpf_runtime.go` — privileged bootstrap: pin maps/links under `/sys/fs/bpf/agent-ebpf/`, self-elevation via sudo/pkexec
- `cluster.go` — master/slave node forwarding with heartbeat
- `mcp_server.go` — MCP SSE endpoint exposing tools for config/event access
- `network_events.go` — kernel event type ↔ string mapping + network address formatting
- `privileges.go` — drop privileges for spawned shells (SUDO_UID/SUDO_GID handling)
- `runtime_state.go` — event archive (in-memory ring), JSONL persistence, access token management
- `shell_sessions.go` — persistent PTY session manager with WebSocket attach/detach

### Frontend (`frontend/src/`)

7 views, all Composition API `<script setup lang="ts">`:
- `Dashboard.vue` — live event stream with tag/type/PID/comm/path filters
- `Monitor.vue` — CPU/memory/GPU/IO/page-fault telemetry
- `Network.vue` — syscall-derived network flow table
- `Explorer.vue` — filesystem browser for adding tracked paths
- `Executor.vue` — PTY shell manager + tmux workbench + script launchers (Python/Node/Deno/Bun/Ruby/sh/pwsh)
- `Hooks.vue` — AI CLI hook installer (Claude Code, Gemini, Codex, Copilot, Kiro, Cursor)
- `Config.vue` — tags, tracked comms/paths, wrapper rules, logging, access token

### Wrapper (`wrapper/main.go`)

- Intercepts CLI commands via UDS (`/tmp/agent-ebpf.sock`)
- Sends `WrapperRequest` (pid, comm, args) → receives `ALLOW/BLOCK/REWRITE/ALERT`
- On REWRITE, replaces args; on BLOCK, exits without executing

### Adapters

- `adapters/python/agent_tracker.py` — PID registration for Python agents (uv-based, Python 3.13+)
- `adapters/js/agentTracker.js` — PID registration for Node.js agents

## Coding conventions

### Frontend (`frontend/src/`)

- All views use Vue 3 Composition API with `<script setup lang="ts">`
- Routes live in `frontend/src/views/`
- Shared terminal UI components: `LocalShellTerminal.vue`, `RemoteWrapperTerminal.vue`, `ShellTerminalPane.vue` in `frontend/src/components/`
- Protobuf event naming must stay aligned between backend event types, proto messages, and frontend filters/tables

### Backend (`backend/`)

- Add new HTTP routes near existing route groups in `main.go`
- Shell-session logic belongs in `shell_sessions.go`, not inlined into `main.go` (unless the change is tiny)
- Privilege-dropping logic for child commands belongs in `privileges.go`
- Keep protobuf event type mapping in `network_events.go` consistent with proto definitions and frontend display

### Documentation

When behavior changes, update the relevant docs:
- `README.md` — product-level behavior, supported syscalls, supported hooks, auth scope
- `agents.md` — agent registration semantics
- `AGENTS.md` — contributor gotchas and conventions
- `docs/architecture.md` — component and data-flow architecture
- Per-component READMEs (`backend/README.md`, `frontend/README.md`, `wrapper/README.md`, adapter READMEs)

## Key runtime facts

- Backend writes chosen port to `backend/.port`; Vite dev proxy reads it
- eBPF maps pinned at `/sys/fs/bpf/agent-ebpf/maps/{agent_pids,events,tracked_comms,tracked_paths}`
- Matching is **exact**: 16-byte command keys, 256-byte path keys (not recursive)
- PID registration is per-process, not inherited by children
- Auth: `authMiddleware()` protects `/config/**` and `/system/**` in release mode (X-API-KEY); `/ws`, `/register`, `/unregister` are unprotected
- Access token auto-generated, stored at `~/.config/agent-ebpf-filter/runtime.json`, overridable via `AGENT_API_KEY` env var
- Event persistence: optional JSONL at `~/.config/agent-ebpf-filter/events.jsonl` (toggled from Config page)
- Native hooks require `curl` installed on the host (hook callbacks use `curl` to POST to the backend)
- Deeper architecture details: see `docs/architecture.md` for component diagrams and data-flow descriptions

## Generated files (do not hand-edit)

- `backend/ebpf/agenttracker_bpf{el,eb}.go` and `.o`
- `backend/pb/tracker.pb.go`
- `adapters/python/tracker_pb2.py`, `adapters/js/tracker_pb.js`
- `frontend/src/pb/tracker_pb.js` and `.d.ts`
- `backend/agent-ebpf-filter`, `agent-wrapper` (build outputs)
