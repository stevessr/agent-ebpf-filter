# AGENTS.md

Repository-specific operating manual for coding agents and maintainers.

This repo combines a privileged Go/eBPF runtime, a Vue 3 dashboard, a CLI
wrapper, language adapters, VitePress docs, and a separate `kernel-ml` DKMS
module. Use this file as the first stop before changing code.

## 0. Golden rules

- **Prefix shell commands with `rtk`** in this environment.
- Start every non-trivial edit with `rtk git status --short`; do not overwrite
  unrelated user work. `docs/ref/**` is often a dirty reference checkout.
- Prefer targeted reads with `rg`, `grep`, `sed`, `find`, and `cat`; avoid
  opening huge generated files unless the task explicitly needs them.
- Use `js_repl` for quick JavaScript/Node inspection when the tool is
  available; otherwise use repo scripts instead of ad-hoc experiments.
- Do not hand-edit generated artifacts. Change the source and regenerate.
- Keep build-time feature tags separate from runtime gates: compiled-in
  dangerous features still require `/config/runtime` or `AGENT_RUNTIME_*`
  enablement, and release-mode auth still applies.

## 1. Project map

| Area | Main files | Responsibility |
| --- | --- | --- |
| Backend runtime | `backend/app/` | HTTP/WS APIs, feature gates, auth, hooks, shell sessions, ML, plugins, policy APIs |
| eBPF programs | `backend/ebpf/` | tracker, TLS capture, cgroup sandbox, BPF LSM generated bindings |
| Protobuf | `proto/` | split domain protos plus `tracker.proto` aggregator for JS tooling |
| Frontend | `frontend/src/` | Vue 3 + TypeScript + Vite dashboard/workbench |
| Wrapper | `wrapper/` | `agent-wrapper` binary and UDS control client |
| Adapters | `adapters/python/`, `adapters/js/` | PID registration helpers and generated protobuf clients |
| Kernel ML | `kernel-ml/` | separate DKMS module, proc/sysfs UAPI, optional CUDA userspace helper |
| Deploy | `deploy/kubernetes/` | Kubernetes DaemonSet/Service manifests |
| Docs | `README.md`, `docs/`, `agents.md`, component READMEs | product docs, VitePress docs, registration semantics |

Go workspace facts:

- `go.work` uses `./backend`, `./wrapper`, and `./tools/dev-env-tui`.
- Current Go version is `1.26.2` in `go.work`, `backend/go.mod`, and the
  devcontainer image args.

## 2. Quick task routing

| If you change... | Primary files to inspect/update | Regenerate/build/verify |
| --- | --- | --- |
| Protobuf contracts | `proto/tracker_*.proto`, `proto/tracker.proto` | `rtk make proto` |
| Main tracker eBPF | `backend/ebpf/agent_tracker.c` | `rtk bash -lc 'cd backend/ebpf && go generate'`; then backend build/test |
| TLS capture eBPF | `backend/ebpf/agent_tls_capture.c`, `backend/ebpf/gen_tls.go` | `rtk make ebpf-tls`; backend build/test |
| cgroup sandbox eBPF | `backend/ebpf/cgroup_sandbox.c`, sandbox handlers/tests | `rtk make ebpf-cgroup`; `rtk make os-enforcement-check` |
| BPF LSM eBPF | `backend/ebpf/lsm_enforcer.c`, LSM handlers/tests | `rtk make ebpf-lsm`; `rtk make os-enforcement-check` |
| HTTP/WS route | `backend/app/routes__routes.go`, matching handler file | focused `rtk bash -lc 'cd backend && go test ...'` |
| Auth/runtime gate | `backend/app/runtime__helpers_auth.go`, `runtime__state*.go`, docs | auth tests + docs sync |
| MCP tool | `backend/app/server__server_mcp.go`, skills/docs | verify auth and return shape |
| Feature flag/build tag | `backend/app/feature_build_*.go`, `feature_manifest.go`, frontend `featureFlags.ts` | backend tests + frontend build if UI affected |
| Vue page/composable | `frontend/src/views/`, `frontend/src/components/`, `frontend/src/composables/` | `rtk make frontend` or focused TS/Vite check |
| VitePress docs | `docs/**/*.md`, `docs/.vitepress/config.ts` | `rtk bun run docs:build` |
| External API/K8s behavior | `docs/external-api.md`, `docs/kubernetes.md`, `deploy/kubernetes/agent-ebpf-filter.yaml`, README summaries | API/K8s docs and manifest review |
| Kernel ML UAPI | `kernel-ml/*`, `kernel-ml/README.md` | `rtk bash -lc 'cd kernel-ml && make dkms-smoke'` when applicable |

## 3. Common commands

```bash
rtk make help
rtk make dev-env              # TUI for local/devcontainer env files
rtk make dev-env-doctor
rtk make predev               # install/check helper deps
rtk make dev-image            # print GHCR devcontainer image ref
rtk make dev-image-repository
rtk make dev-image-tag
rtk make docker               # pull GHCR devcontainer image only; no local fallback
rtk make exec                 # start/attach privileged devcontainer shell
rtk make proto
rtk make backend
rtk make wrapper
rtk make frontend
rtk make runtime-benchmark
rtk make ml-sweep
rtk make ebpf-cgroup
rtk make ebpf-lsm
rtk make os-enforcement-preflight
rtk make os-enforcement-check
rtk make os-enforcement-smoke
rtk env OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start
rtk make dev                  # Zellij backend/frontend dev session
rtk make install              # production build + system service install
rtk make uninstall
rtk bun run docs:build
```

Development environment notes:

- `make dev-env` opens the Go TUI in `tools/dev-env-tui` and writes local-only
  `.env.dev` / `.env.dev.mk`; never commit those files. Use
  `make dev-env-build`, `make dev-env-cli`, `make dev-env-print`, and
  `make dev-env-doctor` as needed.
- `make predev` installs helper dependencies in parallel and normalizes an
  unwritable or stale `GOPATH=/go` to `$HOME/go` before installing Go helpers.
- `make dev` assumes `predev` is done, regenerates protobuf/wrapper as needed,
  and opens the backend/frontend session in Zellij.
- JS/TS protobuf generation must use `protobufjs-cli` through Node.js, not
  `bunx`; `pbts` depends on Node module-loader behavior. Dependency install is
  still Bun-based.
- CUDA acceleration is build-tagged. Build with the `cuda` tag only when
  `/opt/cuda/bin/nvcc` and CUDA runtime libraries are present; otherwise keep
  the CPU-only stub default so non-CUDA devcontainers compile.
- Build selection uses `AGENT_BUILD_FEATURES`: `all` for the full workbench,
  `core` for optional modules removed, or comma-separated names such as
  `tls_capture,ml` mapping to `agentfeat_tls_capture agentfeat_ml` tags.

## 4. Generated artifacts and source of truth

Do not hand-edit generated/derived artifacts unless the task explicitly demands
it. Regenerate from sources instead.

Generated families include:

- `backend/ebpf/*_bpfel.go`, `backend/ebpf/*_bpfeb.go`, `backend/ebpf/*.o`
- `backend/pb/*.pb.go`
- `adapters/python/*_pb2.py`
- `adapters/js/tracker_pb.js`
- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`
- repo-root binaries such as `agent-wrapper` and `backend/agent-ebpf-filter`

Protobuf source facts:

- Domain definitions live in `proto/tracker_common.proto`,
  `tracker_events.proto`, `tracker_registration.proto`, `tracker_system.proto`,
  `tracker_config.proto`, and `tracker_shell.proto`.
- `proto/tracker.proto` is an aggregation/import file kept for downstream
  tooling that still references `tracker.proto`.
- If any proto changes, run `rtk make proto` and inspect all generated language
  bindings.

## 5. Runtime invariants to preserve

### Privilege, pins, and policy mutation

- The backend self-elevates with `sudo` / `pkexec` when needed.
- Main eBPF tracker maps/links are pinned under:
  - `/sys/fs/bpf/agent-ebpf/maps`
  - `/sys/fs/bpf/agent-ebpf/links`
- cgroup/connect + UDP sendmsg OS-level network blocking pins under:
  - `/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps`
  - `/sys/fs/bpf/agent-ebpf/cgroup_sandbox/links`
- BPF LSM pins under:
  - `/sys/fs/bpf/agent-ebpf/lsm_enforcer/maps`
  - `/sys/fs/bpf/agent-ebpf/lsm_enforcer/links`
- OS-level cgroup/LSM policy maps should stay restrictive (`0600`) and be
  mutated through authenticated backend APIs, not unprivileged direct map writes.
- Wrapper control uses `/tmp/agent-ebpf.sock`; keep the UDS restrictive (`0600`)
  and peer-credential checks scoped to root / original invoking user.

### OS enforcement semantics

- PID-based cgroup sandbox actions resolve the PID's cgroup v2 inode id and
  write the same `cgroup_blocklist` map.
- Destination blocking is exact-match only:
  - `ip_blocklist` for IPv4
  - `ip6_blocklist` for IPv6
  - `port_blocklist` for TCP/UDP ports
- Do **not** describe cgroup destination policy as CIDR/range based unless the
  implementation changes.
- BPF LSM file-name policy is basename-based and applies to `file_open`,
  `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`,
  `inode_create`, `inode_link`, `inode_symlink`, `inode_unlink`, `inode_mkdir`,
  `inode_rmdir`, `inode_mknod`, and `inode_rename`.
- Executable policy applies to `bprm_check_security` by exact path or executable
  basename.

### Tracking/matching model

- PID tracking is seeded from the registered process and inherits to descendants
  through fork/clone lineage.
- `tracked_comms` is an exact 16-byte command-name map.
- `tracked_paths` is an exact 256-byte path map.
- Avoid describing path tracking as recursive or policy-tree based unless you
  also change the implementation.

### Port handoff and domain forwarding

- The backend chooses the first free port in `8080..8089` and writes it to
  `backend/.port`.
- `frontend/vite.config.ts` reads `backend/.port` for dev proxies.
- This is independent from the optional domain forwarder. When
  `runtime.domainForwardProxy.enabled` is true, the backend also binds the
  configured public HTTP/HTTPS ports (default `80` / `443`) and routes by
  request `Host` / TLS SNI.
- HTTPS forwarding needs a default cert/key or route-level cert/key paths.

### Auth and runtime gates

- Dev mode disables auth by default. Release mode requires the runtime access
  token for protected surfaces including:
  - `/config/**`
  - `/system/**`
  - `/ws*`
  - `/metrics`
  - `/register`
  - `/unregister`
  - `/agentsight/**`
  - `/api/events*`
  - `/api/runners*`
  - `/api/stream*`
  - `/shell-sessions*`
  - `/events/recent`
  - `/events/graph`
  - `/sandbox/**`
  - `/mcp`
- `/mcp` is a streamable HTTP MCP endpoint authenticated with `X-API-KEY`,
  `Authorization: Bearer`, or `?key=<token>`.
- `/hooks/event` accepts either the normal access token or a per-hook secret via
  `X-Agent-Hook-Secret`.
- Shell sessions, `/system/run`, hook installation/raw hook writes, and policy
  mutations are runtime-gated and default to disabled until enabled through
  `/config/runtime` or matching `AGENT_RUNTIME_*` settings.
- The domain forwarder is disabled by default and configured through
  `/config/runtime`; proxied data-plane traffic on 80/443 is not API-token
  protected, but its config and `/system/domain-forward/status` are protected.

## 6. MCP tools and agent skills

The backend exposes MCP tools at `/mcp`.

Configuration:

- `config_snapshot` — full configuration snapshot
- `add_tracked_command` — add tracked command
- `add_tracked_path` — add tracked path

Events:

- `tail_events` — recent events
- `query_events` — filter by `eventType`, `comm`, `pid`

Monitoring:

- `get_network_flows` — network flow summary
- `get_system_health` — collector/bootstrap/OTLP/enforcement health

Security policy, gated by `policyManagementEnabled=true`:

- `block_network_destination` — block exact IP or port
- `block_process_cgroup` — block a process cgroup's network
- `block_file_access` — block file/exec through BPF LSM

Bundled Claude Code skills:

- `configure-security` — tracked commands/paths, wrapper rules, network/file blocking
- `analyze-network` — traffic analysis and anomaly identification
- `monitor-process` — process behavior, file/network/child-process monitoring

When modifying MCP tools:

1. Update `backend/app/server__server_mcp.go`.
2. Verify all tools have correct auth/runtime checks.
3. Keep skills and README/docs synchronized.
4. Test the return payload shape.

## 7. Backend conventions

- Keep route additions near existing groups in `backend/app/routes__routes.go`.
- Keep protobuf event names aligned across eBPF event mapping, protobuf
  messages, backend handlers, and frontend filters/tables.
- Shell-session logic belongs in `backend/app/shell__*.go`, not `main.go`,
  unless the change is tiny.
- Privilege dropping for child commands belongs in
  `backend/app/runtime__privileges.go`.
- Feature availability is split across build-time files
  `backend/app/feature_build_*.go`, runtime config state, and frontend build
  filtering. Update all affected layers together.
- AgentSight compatibility spans EventEnvelope history, TLS capture
  history/stream, `/ws/system` metric conversion, imported traces, bundled
  sample trace, and aliases under `/agentsight/*`, `/api/events*`,
  `/api/runners*`, and `/api/v1/agentsight/*`.

## 8. Frontend conventions

- Vue 3 + TypeScript + Vite.
- Prefer/keep Composition API with `<script setup lang="ts">`.
- Routes live in `frontend/src/router/index.ts`; workbench navigation lives in
  `frontend/src/config/navigation.ts`.
- Views live under `frontend/src/views/**`; shared UI and composables live under
  `frontend/src/components/**` and `frontend/src/composables/**`.
- Shared terminal UI lives in:
  - `frontend/src/components/terminal/LocalShellTerminal.vue`
  - `frontend/src/components/terminal/RemoteWrapperTerminal.vue`
  - `frontend/src/components/terminal/ShellTerminalPane.vue`

Important pages/workbenches:

- `Dashboard.vue` — live event stream
- `Monitor.vue` — system/process metrics
- `Network.vue`, `NetworkFlow.vue`, `TLSCapture.vue` — network views
- `ExecutionGraph.vue` — agent run/tool/process/syscall/file/network/policy graph
- `Explorer.vue` — filesystem browser and path tagging
- `Executor.vue` — wrapper execution + PTY shell manager
- `Hooks.vue` — AI CLI hook management
- `ML.vue` — ML status, parameters, model tuning, LLM scoring, training data
- `Plugins.vue` — plugin registry, online eBPF builder, visual block editor,
  and independent TypeScript pseudocode builder
- `Config.vue` — tags, comms, paths, wrapper rules, Runtime Config, System Health

Plugins gotcha:

- The NLP Blocks Compiler calls `/plugins/visual/llm-compile` and falls back to
  local parsing if LLM config is unavailable.
- Keep visual-canvas drafts and TS-pseudocode drafts in separate browser storage
  slots. Do not reintroduce canvas <-> TS pseudocode bidirectional syncing.
- User-authored visual eBPF plugins use `attachKind: "lsm"` for generated
  `SEC("lsm/...")` programs, and `attachKind: "kprobe"` only for the
  `unlink` / `do_unlinkat` flow.
- Do not serialize non-`unlink` visual plugins with `attachKind: "none"`; the
  backend loader requires a real attach kind and `programName`.

## 9. Kernel ML conventions

The `kernel-ml` module is separate from the backend eBPF programs.

- It is built through DKMS/Kbuild and must not link CUDA into kernel space.
- CUDA inference is userspace offload via `kernel-ml/kernel_ml_cuda_helper` and
  proc ABI:
  - `/proc/ml_cuda_request`
  - `/proc/ml_cuda_result`
  - `/proc/ml_cuda_model`
- `/proc/ml_backend` and `/sys/kernel/kernel_ml/backend` select `kernel`,
  `cuda`, or `auto`; helper/GPU failure falls back to kernel CPU inference.
- The module exposes `/sys/kernel/kernel_ml/*`, v2 model metadata
  (`model_generation`, dynamic tree count/depth, up to 16 classes), and a
  64-entry exact-match LRU cache.
- Keep `kernel-ml/README.md` in sync when changing those UAPI surfaces.
- The checkout path may contain spaces; stage DKMS/Kbuild work to a temporary
  tree or use existing `kernel-ml` Make targets such as `make dkms-smoke`.

## 10. Docs conventions

When behavior changes, update the matching docs in the same pass.

Always consider:

- root `README.md` for product-level behavior
- `agents.md` for agent registration/tracking semantics
- component READMEs for local behavior
- `docs/backend/*.md` for runtime/eBPF/ML/backend architecture details
- `docs/operations/*.md` for build/run/deployment operations
- this `AGENTS.md` for contributor gotchas

External API or Kubernetes changes also require:

- `docs/external-api.md`
- `docs/kubernetes.md`
- `deploy/kubernetes/agent-ebpf-filter.yaml`
- root `README.md` / `backend/README.md` endpoint summaries

VitePress facts:

- Root `package.json` scripts are `docs:dev`, `docs:build`, `docs:preview`.
- VitePress config is `docs/.vitepress/config.ts`.
- Backend docs routes belong under `/backend/*`; backend ML pages are linked
  there, e.g. `/backend/multi-model-complete`, `/backend/ml-experiments`, and
  `/backend/kernel-ml-implementation`.
- `docs/ref/**` is excluded from VitePress source and should be treated as
  vendored/reference material unless the user explicitly asks to change it.
- Mermaid/SVG-heavy docs should be checked with `rtk bun run docs:build`.

Keep these documentation claims precise:

- supported syscall/event types
- supported AI CLI hooks
- release/dev auth scope
- `/api/v1` external API aliases and Kubernetes manifests
- 80/443 domain-forwarding behavior and certificate requirements
- generated-file workflow
- Make targets and devcontainer pull-only workflow

## 11. Devcontainer and install rules

Devcontainer/GHCR:

- `make dev-image` prints the image ref.
- `make docker` is pull-only and derives
  `ghcr.io/<owner>/<repo>/devcontainer:<branch-slug>-<branch-hash>` from the
  GitHub origin remote and current branch. The branch hash is the first 12 hex
  chars of the branch-name SHA-256 digest.
- On detached HEAD, set `DEV_BRANCH=<branch>` or pass a full `DEV_IMAGE=...`.
- If the image is missing, wait for or run the GitHub Actions devcontainer image
  workflow; do not add a local build fallback.
- The workflow-built image must run `make predev`, publish a multi-arch manifest
  for `linux/amd64` and `linux/arm64` (aarch64), and keep dependency copies
  under `/opt/agent-ebpf-predev`.
- Post-create should seed from `/opt/agent-ebpf-predev` and run
  `make predev-check` only. Do not silently install online unless
  `DEVCONTAINER_POSTCREATE_INSTALL=1` is explicitly set.
- VS Code Dev Containers and `make exec` mount container-local volumes over
  `frontend/node_modules` and `adapters/python/.venv`.
- Pass through host Git config (`~/.gitconfig`, `~/.config/git`) read-only, but
  never mount credentials, SSH keys, or credential stores.
- VS Code Dev Containers must use the `image` field in
  `.devcontainer/devcontainer.json`; keep `.devcontainer/Dockerfile` only as the
  GitHub Actions build input.
- Keep `updateRemoteUserUID` disabled so VS Code does not generate a local
  `updateUID.Dockerfile`.
- Keep Podman user namespace mapping aligned with image UID/GID `1001:1001`.
- Do not combine `--init` with `--pid=host`; Podman rejects that startup shape.
- For branch/fork Dev Containers, launch VS Code with `DEV_IMAGE_REPOSITORY` and
  `DEV_IMAGE_TAG` from the matching Make targets.

Install/service:

- `make install` runs a production build and installs backend, compiled
  frontend, and wrapper under `/opt/agent-ebpf-filter`, plus public binaries
  under `/usr/local/bin`.
- The installer writes `/etc/agent-ebpf-filter/agent-ebpf-filter.env`, sets
  `GIN_MODE=release`, sets `AGENT_WRAPPER_PATH=/usr/local/bin/agent-wrapper`,
  and records the invoking user's home in `AGENT_REAL_HOME` so runtime config
  stays in `~/.config/agent-ebpf-filter`.
- Service registration is systemd-first when a running systemd manager exists;
  otherwise it writes an `rc.local` managed block and
  `/usr/local/sbin/agent-ebpf-filter-service`.
- Overrides: `INSTALL_METHOD=systemd|rc.local`, `INSTALL_START=0`,
  `INSTALL_ENABLE=0`, `INSTALL_PREFIX=...`.
- Keep the service privileged/root because the backend loads eBPF, cgroup, and
  BPF LSM programs and may bind 80/443 for domain forwarding.

## 12. Verification checklist

Pick the smallest verification that proves the change:

- Formatting only / docs only: `rtk git diff --check` plus docs build if docs
  are part of the VitePress site.
- Backend Go logic: focused `rtk bash -lc 'cd backend && go test ...'`; use
  `rtk make backend` when generated/eBPF bindings or build tags are involved.
- Wrapper logic: `rtk make wrapper` and any relevant wrapper tests.
- Frontend logic: `rtk make frontend` or focused `vue-tsc`/Vite checks.
- eBPF object section expectations: `rtk make os-enforcement-check` for
  cgroup/LSM enforcement changes.
- Live enforcement: `rtk make os-enforcement-preflight` then privileged smoke
  only when the host can load BPF programs.
- Documentation site: `rtk bun run docs:build`.

## 13. Misc gotchas

- Native hook installation injects generated relay scripts that call `curl`;
  docs should mention `curl` as a runtime dependency when hook install behavior
  is discussed.
- Hook callbacks resolve the current backend port via `.port` unless
  `AGENT_HOOK_ENDPOINT` overrides it.
- The frontend Runtime Config tab stores the access token locally and appends it
  to WebSocket URLs as `?key=...`.
- Domain forwarder data-plane traffic is intentionally separate from API-token
  protected control-plane routes.
