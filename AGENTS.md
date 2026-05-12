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
rtk make predev
rtk make proto
rtk make backend
rtk make wrapper
rtk make frontend
rtk make runtime-benchmark
rtk make ebpf-cgroup
rtk make ebpf-lsm
rtk make os-enforcement-preflight
rtk make os-enforcement-check
rtk make os-enforcement-smoke
rtk env OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start
rtk make dev
```

`make predev` installs the helper dependencies in parallel. `make dev` assumes those dependencies are already present and opens the backend/frontend dev session in Zellij instead of tmux.

If you change `proto/tracker.proto`, regenerate:

```bash
rtk make proto
```

If you change `backend/ebpf/agent_tracker.c`, regenerate/build:

```bash
rtk bash -lc 'cd backend/ebpf && go generate'
rtk bash -lc 'cd backend && go build ./...'
```

If you change `backend/ebpf/cgroup_sandbox.c`, regenerate/build:

```bash
rtk make ebpf-cgroup
rtk bash -lc 'cd backend && go build ./...'
```

If you change `backend/ebpf/lsm_enforcer.c`, regenerate/build:

```bash
rtk make ebpf-lsm
rtk bash -lc 'cd backend && go build ./...'
```

## 4. Generated files

Do not hand-edit generated artifacts unless the task explicitly requires it.

Generated / derived files include:

- `backend/ebpf/agenttracker_bpfel.go`
- `backend/ebpf/agenttracker_bpfeb.go`
- `backend/ebpf/agenttracker_bpfel.o`
- `backend/ebpf/agenttracker_bpfeb.o`
- `backend/ebpf/agentcgroupsandbox_bpfel.go`
- `backend/ebpf/agentcgroupsandbox_bpfeb.go`
- `backend/ebpf/agentcgroupsandbox_bpfel.o`
- `backend/ebpf/agentcgroupsandbox_bpfeb.o`
- `backend/ebpf/agentlsmenforcer_bpfel.go`
- `backend/ebpf/agentlsmenforcer_bpfeb.go`
- `backend/ebpf/agentlsmenforcer_bpfel.o`
- `backend/ebpf/agentlsmenforcer_bpfeb.o`
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
- cgroup/connect + UDP sendmsg OS-level network blocking pins under:
  - `/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps`
  - `/sys/fs/bpf/agent-ebpf/cgroup_sandbox/links`
- PID-based cgroup sandbox actions resolve the PID's cgroup v2 inode id and
  then write the same `cgroup_blocklist` map.
- Destination blocking uses exact `ip_blocklist` (IPv4), `ip6_blocklist`
  (IPv6), and TCP/UDP `port_blocklist` maps; do not describe it as CIDR/range based.
- BPF LSM exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename blocking pins under:
  - `/sys/fs/bpf/agent-ebpf/lsm_enforcer/maps`
  - `/sys/fs/bpf/agent-ebpf/lsm_enforcer/links`
- BPF LSM file-name policy is basename-based and applies to `file_open`, `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`,
  `inode_create`, `inode_link`, `inode_symlink`, `inode_unlink`, `inode_mkdir`, `inode_rmdir`, `inode_mknod`, and `inode_rename`; executable policy applies to
  `bprm_check_security` by exact path or executable basename.
- OS-level cgroup/LSM policy maps should remain restrictive (`0600`) and be
  mutated through authenticated backend APIs, not direct unprivileged map writes.
- Wrapper control uses the Unix socket:
  - `/tmp/agent-ebpf.sock`

### Port handoff

- The backend chooses the first free port in `8080..8089`.
- It writes the result into `backend/.port`.
- `frontend/vite.config.ts` reads that file to build dev proxies.

### Matching model

- PID tracking is seeded from the registered process and now inherits to descendants through fork/clone lineage.
- `tracked_comms` is an exact 16-byte command-name map.
- `tracked_paths` is an exact 256-byte path map.

Avoid describing path tracking as recursive or policy-tree based unless you also change the implementation.

### Auth model

- In release mode, the backend now requires the runtime access token for:
  - `/config/**`
  - `/system/**`
  - `/ws*`
  - `/metrics`
  - `/register`
  - `/unregister`
  - `/shell-sessions*`
  - `/events/recent`
  - `/events/graph`
  - `/sandbox/**`
- Dev mode disables auth by default.
- `/hooks/event` accepts either the normal access token or a per-hook secret via `X-Agent-Hook-Secret`.
- Shell sessions, `/system/run`, hook installation / raw hook writes, and policy mutations are runtime-gated and default to disabled until explicitly enabled in `/config/runtime`.

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
- `ExecutionGraph.vue` — agent run / tool / process / syscall / file / network / policy graph
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
- The frontend runtime page now stores the access token locally and appends it to WebSocket URLs as `?key=...`.
- The wrapper UDS socket is expected to stay restrictive (`0600`) and validate peer credentials against root / the original invoking user.
