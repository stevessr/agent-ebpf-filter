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
- `deploy/kubernetes/` — DaemonSet / Service manifests for Kubernetes node-agent deployment
- `proto/tracker.proto` — protobuf source of truth

## 2. Shell / tooling rules

- Per local instructions, **prefix shell commands with `rtk`**.
- Use `js_repl` for quick Node/JavaScript inspection instead of ad-hoc `node -e` when practical.
- Prefer small targeted reads with `rg`, `sed`, `find`, and `cat` over opening huge generated files.

## 3. Build and regeneration workflow

Typical commands:

```bash
rtk make help
rtk make dev-env     # Interactive local/devcontainer development env wizard
rtk make dev-env-doctor
rtk make predev
rtk make dev-image    # Print the GHCR devcontainer image for the current branch
rtk make dev-image-tag
rtk make docker       # Pull GHCR devcontainer image for the current branch; no local image build
rtk make exec         # Start/attach to the privileged devcontainer shell
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
rtk make install       # Install as a system service: systemd first, rc.local fallback
rtk make uninstall
```

`make dev-env` opens the standalone Go TUI in `tools/dev-env-tui` and writes
local-only `.env.dev` / `.env.dev.mk` files for core dev settings, ML/LLM
configuration, runtime app behavior, sandbox/cluster options, devcontainer
overrides, and tooling/benchmark variables; do not commit those generated files.
Use `make dev-env-build` to build `bin/dev-env-tui`, or `make dev-env-cli` for
the legacy shell prompt. `make dev-env-doctor` checks the effective values and
local tooling with secrets redacted.
`make predev` installs the helper dependencies in parallel. It normalizes an
unwritable Go workspace such as a host-side stale `GOPATH=/go` to `$HOME/go`
before installing Go helper binaries. `make dev` assumes those dependencies are
already present and opens the backend/frontend dev session in Zellij instead of
tmux.
JS/TS protobuf generation should run `protobufjs-cli` through Node.js, not
`bunx`, because `pbts` depends on Node module loader behavior. The devcontainer
gets Node from the official `nodejs.org` binary tarball and should not install
Debian `nodejs` / `npm`; keep dependency installation on Bun.
CUDA acceleration is build-tagged: only build with the `cuda` tag when
`/opt/cuda/bin/nvcc` and CUDA runtime libraries are present; otherwise keep the
CPU-only stub as the default so devcontainers without CUDA still compile.
`make exec` and VS Code Dev Containers must mount container-local volumes over
`frontend/node_modules` and `adapters/python/.venv` so the bind-mounted
workspace stays writable without reusing host-only dependency trees.

`make install` runs a production build and installs the backend, compiled
frontend, and wrapper under `/opt/agent-ebpf-filter` plus public binaries under
`/usr/local/bin`. The installer writes
`/etc/agent-ebpf-filter/agent-ebpf-filter.env`, sets `GIN_MODE=release` and
`AGENT_WRAPPER_PATH=/usr/local/bin/agent-wrapper`, and records the invoking
user's home in `AGENT_REAL_HOME` so runtime config stays in
`~/.config/agent-ebpf-filter`. Service registration is systemd-first when a
running systemd manager exists, otherwise it writes an `rc.local` managed block
and `/usr/local/sbin/agent-ebpf-filter-service`. Use
`INSTALL_METHOD=systemd|rc.local`, `INSTALL_START=0`, `INSTALL_ENABLE=0`, or
`INSTALL_PREFIX=...` to override defaults; keep the service privileged/root
because the backend loads eBPF, cgroup, and BPF LSM programs and may bind
80/443 when domain forwarding is enabled.

`make dev-image` prints the image ref. `make docker` is pull-only: it derives `ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>-<branch-hash>` from the GitHub origin remote and the current branch, where the branch hash is the first 12 hex characters of the branch name's SHA-256 digest. If the branch cannot be inferred on a detached HEAD, set `DEV_BRANCH=<branch>` or pass a full `DEV_IMAGE=...`. If the image is not available yet, wait for it to publish or run the GitHub Actions devcontainer image workflow; do not add a local build fallback. The workflow-built image must run `make predev` during the Docker build, publish a multi-arch manifest for `linux/amd64` and `linux/arm64` (aarch64), and keep a copy of workspace-local dependencies under `/opt/agent-ebpf-predev` so VS Code post-create and `make exec` can seed bind-mounted workspaces without reinstalling from the network. Post-create should run `make predev-check` only; if an old image is missing dependencies, tell the user to rebuild/pull the workflow image instead of silently running online installs, unless `DEVCONTAINER_POSTCREATE_INSTALL=1` is explicitly set. VS Code Dev Containers and `make exec` must pass through the host Git config (`~/.gitconfig` and `~/.config/git`) read-only, but must not mount credentials, SSH keys, or Git credential stores. VS Code Dev Containers must use the `image` field in `.devcontainer/devcontainer.json`; keep `.devcontainer/Dockerfile` only as the GitHub Actions build input. Keep `updateRemoteUserUID` disabled so VS Code does not generate a local `updateUID.Dockerfile`; keep the Podman user namespace mapping aligned with the image's `vscode` UID/GID `1001:1001`; and do not combine `--init` with `--pid=host` because Podman rejects that startup shape. For branch/fork Dev Containers, launch VS Code with `DEV_IMAGE_REPOSITORY` and `DEV_IMAGE_TAG` from the matching Make targets.

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

If you change external API or Kubernetes deployment behavior, update:

- `docs/external-api.md`
- `docs/kubernetes.md`
- `deploy/kubernetes/agent-ebpf-filter.yaml`
- root `README.md` / `backend/README.md` endpoint summaries

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
- User-authored visual eBPF plugins use `attachKind: "lsm"` for generated
  `SEC("lsm/...")` programs and `attachKind: "kprobe"` only for the
  `unlink` / `do_unlinkat` flow. Do not serialize non-`unlink` visual plugins
  with `attachKind: "none"` because the backend loader requires a real attach
  kind and `programName`.

### Port handoff

- The backend chooses the first free port in `8080..8089`.
- It writes the result into `backend/.port`.
- `frontend/vite.config.ts` reads that file to build dev proxies.
- This is separate from the optional domain forwarder. When
  `runtime.domainForwardProxy.enabled` is true, the backend also binds the
  configured public HTTP/HTTPS ports (default `80` / `443`) and routes traffic
  by request `Host` / TLS SNI. HTTPS needs a default cert/key or route-level
  cert/key paths.

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
- The domain forwarder is disabled by default and configured through
  `/config/runtime`; proxied data-plane traffic on 80/443 is not API-token
  protected, but its config and `/system/domain-forward/status` are.

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
- `ML.vue` — ML status, parameters, model tuning, LLM scoring, and training-set management
- `Plugins.vue` — plugin registry, online eBPF builder, visual block editor, and the independent TS pseudocode builder; the NLP Blocks Compiler calls `/plugins/visual/llm-compile` and falls back to local parsing when LLM config is unavailable. Keep visual-canvas drafts and TS-pseudocode drafts in separate browser storage slots, and do not reintroduce canvas ↔ TS pseudocode bidirectional syncing.
- `Config.vue` — tags, comms, paths, wrapper rules, Runtime Config, System Health

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
- `/api/v1` external API aliases and Kubernetes manifests
- 80/443 domain-forwarding behavior and certificate requirements
- generated-file workflow
- Make targets

## 9. Nice-to-know gotchas

- Native hook installation injects generated relay scripts (the scripts call `curl`), so docs should mention `curl` as a runtime dependency.
- Hook callbacks resolve to the current backend port via `.port` unless `AGENT_HOOK_ENDPOINT` overrides it.
- The frontend Runtime Config tab stores the access token locally and appends it to WebSocket URLs as `?key=...`.
- The wrapper UDS socket is expected to stay restrictive (`0600`) and validate peer credentials against root / the original invoking user.
