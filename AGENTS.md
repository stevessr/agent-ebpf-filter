# AGENTS.md

Repository-specific guidance for coding agents and maintainers.

## 1. Project shape

This repo is a **Go + eBPF backend**, **Vue 3 frontend**, **CLI wrapper**, and **language adapters** project.

Main responsibilities:

- `backend/` — privileged runtime, HTTP/WS APIs, hooks, wrapper policy engine
- `backend/ebpf/` — kernel tracing program and generated BPF bindings
- `frontend/` — Vue 3 + TypeScript dashboard
- `wrapper/` — `agent-wrapper` binary
- `adapters/` — Python / Node PID registration helpers
- `proto/tracker.proto` — protobuf source of truth

## 2. Shell / tooling rules

- Per local instructions, **prefix shell commands with `rtk`**.
- Use `js_repl` for quick Node/JavaScript inspection instead of ad-hoc `node -e` when practical.
- Prefer small targeted reads with `rg`, `sed`, `find`, and `cat` over opening huge generated files.

## 3. Build and regeneration workflow

Typical commands:

```bash
rtk make help
rtk make proto
rtk make backend
rtk make wrapper
rtk make frontend
rtk make dev
```

If you change `proto/tracker.proto`, regenerate:

```bash
rtk make proto
```

If you change `backend/ebpf/agent_tracker.c`, regenerate/build:

```bash
rtk bash -lc 'cd backend/ebpf && go generate'
rtk bash -lc 'cd backend && go build ./...'
```

## 4. Generated files

Do not hand-edit generated artifacts unless the task explicitly requires it.

Generated / derived files include:

- `backend/ebpf/agenttracker_bpfel.go`
- `backend/ebpf/agenttracker_bpfeb.go`
- `backend/ebpf/agenttracker_bpfel.o`
- `backend/ebpf/agenttracker_bpfeb.o`
- `backend/pb/tracker.pb.go`
- `adapters/python/tracker_pb2.py`
- `adapters/js/tracker_pb.js`
- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`

Repo-root binaries such as `agent-wrapper` and `backend/agent-ebpf-filter` are build outputs, not source files.

## 5. Runtime facts that matter while editing

### Privilege model

- The backend self-elevates via `sudo` / `pkexec`.
- eBPF maps and links are pinned under:
  - `/sys/fs/bpf/agent-ebpf/maps`
  - `/sys/fs/bpf/agent-ebpf/links`
- Wrapper control uses the Unix socket:
  - `/tmp/agent-ebpf.sock`

### Port handoff

- The backend chooses the first free port in `8080..8089`.
- It writes the result into `backend/.port`.
- `frontend/vite.config.ts` reads that file to build dev proxies.

### Matching model

- PID tracking is **per registered process**.
- `tracked_comms` is an exact 16-byte command-name map.
- `tracked_paths` is an exact 256-byte path map.

Avoid describing path tracking as recursive or policy-tree based unless you also change the implementation.

### Auth model

- `authMiddleware()` protects `/config/**` and `/system/**` in release mode.
- Dev mode disables auth by default.
- Several endpoints are currently outside that middleware (`/ws`, `/ws/system`, `/register`, `/unregister`, `/shell-sessions`, `/ws/shell`, `/hooks/event`).

If you change auth or deployment docs, keep this nuance accurate.

## 6. Frontend conventions

- Vue 3 + TypeScript + Vite
- Prefer / keep **Composition API** with `<script setup lang="ts">`
- Routes live in `frontend/src/views/`
- Shared terminal UI lives in:
  - `frontend/src/components/LocalShellTerminal.vue`
  - `frontend/src/components/RemoteWrapperTerminal.vue`
  - `frontend/src/components/ShellTerminalPane.vue`

Important pages:

- `Dashboard.vue` — live event stream
- `Monitor.vue` — system/process metrics
- `Explorer.vue` — filesystem browser and path tagging
- `Executor.vue` — wrapper execution + PTY shell manager
- `Hooks.vue` — AI CLI hook management
- `Config.vue` — tags, comms, paths, wrapper rules

## 7. Backend conventions

- Keep route additions near existing groups in `backend/main.go`.
- Keep protobuf event naming aligned across:
  - eBPF event type mapping,
  - protobuf messages,
  - frontend filters/tables.
- Shell-session logic belongs in `backend/shell_sessions.go`, not inlined into `main.go`, unless the change is tiny.
- Privilege dropping for child commands belongs in `backend/privileges.go`.

## 8. Documentation expectations

When behavior changes, update the matching docs:

- root `README.md` for product-level behavior
- `agents.md` for agent registration semantics
- component READMEs for local details
- this `AGENTS.md` for contributor gotchas

Especially keep these accurate:

- supported syscall types
- supported AI CLI hooks
- auth scope
- generated-file workflow
- Make targets

## 9. Nice-to-know gotchas

- Native hook installation injects a `curl` command into CLI config files, so docs should mention `curl` as a runtime dependency.
- Hook callbacks resolve to the current backend port via `.port` unless `AGENT_HOOK_ENDPOINT` overrides it.
- The current frontend has no API-key UX; if release-mode auth matters, you may need proxy/header work as part of the change.
