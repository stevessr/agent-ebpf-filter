# Agent eBPF Filter

Linux-first observability and control plane for AI agents and developer CLIs.

This project combines:

- **kernel-space eBPF tracing** for selected syscalls,
- **kernel-side cgroup/connect + UDP sendmsg blocking** for selected cgroups, IPv4/IPv6 destinations, and TCP/UDP destination ports,
- **BPF LSM file/exec blocking** for selected executable paths, executable basenames, and file/directory basenames,
- **user-space PID registration** for agent opt-in,
- **command/path tagging** through pinned BPF maps,
- **wrapper- and hook-based interception** for AI CLIs,
- **a Go + Vue dashboard** for live inspection and control.

It is designed for local development workstations and lab environments where you want to see what an agent is opening, executing, connecting to, or attempting to modify.

---

## What the project currently does

### Kernel-side telemetry

The eBPF program listens to these core syscall tracepoints:

- `sys_enter_execve`
- `sys_enter_openat`
- `sys_enter_connect`
- `sys_enter_mkdirat`
- `sys_enter_unlinkat`
- `sys_enter_ioctl`
- `sys_enter_bind`
- `sys_enter_sendto`
- `sys_enter_recvfrom`

The kernel event payload now also carries syscall exit duration so the UI can render strace-style one-line summaries with timing context.

The runtime now auto-attaches the extended tracepoints compiled from `backend/ebpf/agent_tracker.c`; on kernels that do not expose a specific tracepoint, the backend skips that one and continues booting.

Events are written to a ring buffer and consumed by the Go backend.

### OS-level network interception

The backend also loads `backend/ebpf/cgroup_sandbox.c` as cgroup/connect4 + connect6 plus cgroup/sendmsg4 + sendmsg6 eBPF programs. It attaches at the cgroup v2 root by default (`/sys/fs/cgroup`, or `AGENT_CGROUP_SANDBOX_PATH` when set) and pins maps/links under `/sys/fs/bpf/agent-ebpf/cgroup_sandbox`. Its policy maps are kept restrictive (`0600`) and should be mutated through the authenticated backend API rather than direct local map writes.

Unlike wrapper or native CLI hooks, this path rejects matching TCP/UDP connects and UDP sends in the kernel before the matching operation completes. IPv4 destination blocks are also honored for IPv4-mapped IPv6 sockets such as `::ffff:127.0.0.1`, so AF_INET6 clients cannot bypass an IPv4 block for the same endpoint; mapped inputs normalize to the equivalent IPv4 key. It starts with empty policy maps; blocks are only added through the Security Policies UI/API (or existing pinned-map state from a previous privileged run), not by automatic default deny rules. The Configuration → Security Policies page exposes status, active block entries, counters, and controls for blocking/unblocking cgroup ids, a PID's current cgroup, IPv4/IPv6 destinations, and TCP/UDP destination ports. Mutating routes are protected by the same policy-management runtime gate as wrapper-rule edits.

### BPF LSM file and exec interception

When the running kernel supports BPF LSM, the backend also loads `backend/ebpf/lsm_enforcer.c` and attaches:

- `bprm_check_security` — rejects configured executable paths or executable basenames before `execve` completes.
- `file_open` — rejects configured file basenames before the open succeeds.
- `file_permission` — rejects configured basenames on existing file descriptors before read/write I/O continues.
- `mmap_file` — rejects configured basenames before a new mmap is created from an existing fd.
- `file_mprotect` — rejects configured basenames before an existing file-backed mapping can gain new protections.
- `inode_setattr` — rejects configured basenames before chmod/chown/truncate-style metadata changes succeed.
- `inode_create` / `inode_link` / `inode_symlink` / `inode_mkdir` / `inode_mknod` — reject configured basenames before creating files, hard links, symlinks, directories, FIFOs, or device nodes.
- `inode_unlink` / `inode_rmdir` / `inode_rename` — reject configured file or directory basenames before delete/rmdir/rename succeeds.

The maps/links are pinned under `/sys/fs/bpf/agent-ebpf/lsm_enforcer`; policy maps use restrictive (`0600`) permissions and are changed via the authenticated backend API. Like the cgroup sandbox, the LSM enforcer starts with empty policy maps unless a previous privileged run left pinned entries. The Configuration → Security Policies page exposes attach state, counters, active block entries, and controls for adding/removing executable-path, executable-name, and file/directory-name blocks. This is intentionally a fast deterministic kernel decision path; wrapper/hook and ML/LLM policy can suggest entries, but they are not in the synchronous LSM decision loop.

### TLS 明文捕获

后端可以通过 eBPF uprobes 挂载 OpenSSL、GnuTLS、NSS 和手动注册的 Go TLS 二进制，在加密发送前或解密接收后捕获 HTTPS 明文片段。片段在 Go 后端拼装后解析 HTTP request/response，并通过 `GET /ws/tls-capture`、`GET /tls-capture/recent`、`GET /tls-capture/libraries` 暴露给前端。

Go 进程可通过 `POST /tls-capture/go-binary` 手动注册，或由后端每 60 秒自动扫描 `/proc` 发现的 Go TLS 进程。

安全边界：不做 MITM、不注入证书、不修改目标进程内存或控制流；Authorization、X-API-KEY、Cookie、Set-Cookie、Proxy-Authorization 在后端脱敏；body 截断至 16 KiB。

### User-space telemetry and control

- **PID registration**: Python / Node adapters call `/register` and `/unregister`, optionally attaching `agent_run_id` / `task_id` / `tool_call_id` / `trace_id` / `cwd` style metadata.
- **Tracked command names**: common CLIs plus user-defined commands are tagged through `tracked_comms`.
- **Tracked paths**: exact path matches are tagged through `tracked_paths`.
- **Wrapper interception**: `agent-wrapper` asks the backend for `ALLOW`, `BLOCK`, `ALERT`, or `REWRITE`.
- **Native AI CLI hooks**: the backend can install hook config for Claude Code, Gemini CLI, Codex, and GitHub Copilot, or wrapper aliases for Cursor / any CLI routed through the wrapper.
- **Derived semantic alerts**: the backend can emit `semantic_alert` records such as `SECRET_ACCESS`, `UNEXPECTED_NETWORK_EGRESS`, `UNEXPECTED_CHILD_PROCESS`, and `SEMANTIC_MISMATCH` when observed behavior drifts from read-only style tool intent.
  Hook callbacks resolve against the backend's current port instead of assuming `8080`.

### UI surfaces

- **Dashboard**: live event stream with tag / type / PID / command / path filters, strace-style trace summaries with syscall timing, log-flow ordering, and an optional no-pagination mode
- **Monitor**: process / CPU / memory / GPU / IO / page-fault telemetry
- **Network**: RustNet-style flow workspace with per-process TCP / UDP flow attribution, DNS / SNI / HTTP Host / ALPN enrichment, interface traffic charts, staleness / historic flow indicators, and `process:` / `dport:` / `sni:` / `state:` style filters
- **Execution Graph**: a first-pass agent execution graph with filters for run / tool / trace / pid / path / domain / risk / time, force-layout topology, file-backed and browser-memory recording / replay / export, node details, and one-click rule / training-sample actions
- **Explorer**: browse the host filesystem and add tracked paths
- **Executor**: open a temporary wrapper-backed PTY tab for ad-hoc commands, keep shell PTY sessions separate from tmux, and let the Remote tab self-destruct when you leave it
- **Executor**: launch coding CLIs in tmux, start Python/Node/Ruby/sh/pwsh/Deno/Bun scripts with optional virtualenv selection, and manage shared launch environment variables in a dedicated config tab with backend-detected env suggestions
- **TLS 捕获**: TLS 明文日志，支持实时 WebSocket、进程/库/方向/域名过滤、body 搜索、body 和 curl 一键复制、库挂载状态查看
- **Hooks**: install or edit native hook configs / wrapper aliases
- **Configuration**: manage tags, tracked commands, tracked paths, wrapper rules, OS-level cgroup network blocking, BPF LSM exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename blocking, runtime log persistence, the backend access token, OTLP trace export settings / health, a quick Linux 6.18 LTS syscall / eBPF docs popup preview backed by local snapshots, and ML subtabs for status / parameters / model management / training-set management, including a 42-profile local built-in model catalog, native C runtime inference timing with CUDA / Intel iGPU capability detection, OpenAI-compatible LLM scoring that auto-saves to browser storage and syncs to the backend before scoring, validation split controls, square-grid auto parameter tuning with selectable granularity, live progress, and a heatmap preview
- **Configuration**: the ML training-set manager now includes synthetic expansion presets, batch import of downloadable internet datasets, and the LLM subtab can still pull a cleaned production training set directly from the current training store and export it as OpenAI chat JSONL
- **Cluster control**: master/slave routing, node switching, and forwarded inspection requests through the master backend

The backend can optionally persist captured events as JSONL under `~/.config/agent-ebpf-filter/events.jsonl`, now normalizes live events into versioned `EventEnvelope` records for REST / WebSocket / MCP consumers, exposes `/ws/envelopes` for protobuf envelope streaming, `/metrics` for Prometheus scraping, can export `agent.run` / `codex.task` / `tool.call` derived spans over OTLP HTTP, and provides an authenticated MCP SSE endpoint at `/mcp` using the runtime access token generated from the Configuration page. MCP clients may authenticate with `X-API-KEY`, `Authorization: Bearer`, or `?key=<token>`.

For a rootless static check of the compiled enforcement objects and smoke script, run `rtk make os-enforcement-check`; to diagnose whether a host is ready for live kernel-deny validation, run `rtk make os-enforcement-preflight`. For a privileged live check of both OS-level enforcement paths, start the backend as root (for example with `rtk sudo -E env DISABLE_AUTH=true ./backend/agent-ebpf-filter`) and run `rtk make os-enforcement-smoke`; or set `OS_SMOKE_PRIVILEGE_CMD='sudo -E'` / another root command prefix and run `rtk make os-enforcement-smoke-start`. The smoke script verifies BPF LSM exec-path, exec-name, file-open, existing-fd read/write, mmap, mprotect, ftruncate/fchmod/setattr, create, link, symlink, unlink, mkdir, rmdir, mknod, and rename denial plus cgroup/connect PID-cgroup, IPv4/IPv6 destination, IPv4-mapped IPv6 destination, TCP destination-port, UDP connected-socket connect, existing connected UDP sends, and UDP sendto/sendmsg destination/port denial through the public APIs.

## Security and workflow docs

- `docs/threat-model.md`
- `docs/security-model.md`
- `docs/policy-semantics.md`
- `docs/otel-export.md`
- `docs/benchmark.md`
- `docs/codex-workflows.md`

## Cluster mode

The backend can run in either **master** or **slave** mode:

- **Default**: master
- **Slave mode**: set all of the following environment variables:
  - `AGENT_CLUSTER_MASTER_URL`
  - `AGENT_CLUSTER_ACCOUNT`
  - `AGENT_CLUSTER_PASSWORD`

Optional node identity overrides:

- `AGENT_CLUSTER_NODE_URL`
- `AGENT_CLUSTER_NODE_ID`
- `AGENT_CLUSTER_NODE_NAME`

When a master is selected in the web UI, it can forward supported requests to a slave backend. The currently selected target is stored in the browser and drives HTTP/WS routing through the master.

---

## Architecture

1. **eBPF bootstrap**
   - `backend/ebpf/agent_tracker.c` is compiled through `bpf2go`.
   - Maps and links are pinned under `/sys/fs/bpf/agent-ebpf`.

2. **Privileged backend**
   - `backend/main.go` self-elevates through `sudo` / `pkexec` if needed.
   - It opens pinned maps, consumes the ring buffer, and serves HTTP/WebSocket APIs.

3. **Agent registration**
   - Adapters register the current PID into the `agent_pids` BPF hash map.
   - Registered process context is mirrored in user space and inherited across child processes so descendants can carry `root_agent_pid`, `agent_run_id`, `tool_call_id`, and trace IDs.
   - The eBPF program only emits events when a PID, command, or path matches a tracked rule.

4. **Event fan-out**
   - Kernel events, wrapper events, and native hook events are normalized into protobuf messages.
   - The backend broadcasts them over `/ws` to the Vue frontend.

5. **Policy enforcement**
   - `agent-wrapper` connects to `/tmp/agent-ebpf.sock`.
   - The backend evaluates wrapper rules and returns `ALLOW`, `BLOCK`, `ALERT`, or `REWRITE`.
   - The cgroup sandbox evaluates cgroup/IP/port maps inside cgroup/connect and cgroup/sendmsg hooks and rejects matching outbound TCP connects, UDP connected-socket connects, existing connected UDP sends, unconnected UDP sendto/sendmsg, and IPv4-mapped IPv6 traffic for blocked IPv4 destinations at the kernel boundary.
   - The BPF LSM enforcer evaluates executable-path, executable-name, and file-name maps inside LSM hooks and rejects matching exec/open/read-write/mmap/mprotect/ftruncate/fchmod/setattr/create/link/symlink/unlink/mkdir/rmdir/mknod/rename attempts with `EACCES`.

---

## Repository layout

```text
.
├── README.md
├── AGENTS.md                  # contributor / coding-agent guide
├── agents.md                  # runtime guide for agent registration and tracking
├── Makefile
├── proto/
│   └── tracker.proto          # source of truth for protobuf messages
├── backend/
│   ├── main.go                # HTTP API, WS streams, hooks, wrapper UDS, config
│   ├── ebpf_runtime.go        # pinned map/link bootstrap and privilege handoff
│   ├── shell_sessions.go      # persistent PTY session manager
│   ├── privileges.go          # privilege drop for spawned shells/commands
│   ├── ebpf/
│   │   ├── agent_tracker.c    # eBPF program
│   │   └── gen.go             # bpf2go generation entrypoint
│   └── pb/                    # generated Go protobufs
├── wrapper/
│   └── main.go                # agent-wrapper entrypoint
├── adapters/
│   ├── python/
│   │   └── agent_tracker.py   # Python PID registration helper
│   └── js/
│       └── agentTracker.js    # Node.js PID registration helper
└── frontend/
    └── src/
        ├── views/             # Dashboard / Monitor / Explorer / Executor / Hooks / Config
        ├── components/        # shell terminal UI
        └── pb/                # generated frontend protobuf bindings
```

---

## Requirements

### Host requirements

- Linux with eBPF support
- BTF available (modern distro kernels usually have this)
- bpffs mounted at `/sys/fs/bpf`
- `clang` / LLVM for eBPF code generation
- `protoc`
- `sudo` or `pkexec`

### Toolchain used by this repo

- **Go**: the repo currently declares **Go 1.26.2** in `go.work` / `go.mod`
- **Bun**: used for the frontend
- **Python**: `adapters/python/pyproject.toml` currently targets **Python 3.13+**
- `uv` for the Python adapter environment

> `make deps` installs some helper tools, but it does **not** install `protoc` for you.

---

## Quick start

### Development mode

```bash
make predev
make dev
```

`make predev` installs the development dependencies and helper tools. `make dev` assumes those are already present and opens a Zellij session with backend and frontend in separate panes.

What it does:

- generates protobuf bindings,
- builds `agent-wrapper`,
- starts the backend hot-reload script, which self-elevates when needed and rebuilds the backend and eBPF program as needed,
- writes the chosen backend port to `backend/.port`,
- starts Vite for the frontend inside a Zellij session with a separate backend pane.

The frontend reads `backend/.port` and proxies API / WebSocket traffic automatically.
In desktop sessions, the backend prefers the system's graphical elevation flow (for example `pkexec`) before falling back to `sudo`.

### Production-style run

```bash
make run
```

This builds everything and runs the backend, which serves the compiled frontend from the same process.

### Useful targets

```bash
make help
make proto
make backend
make wrapper
make frontend
make runtime-benchmark
make docker      # Pull the GitHub-built devcontainer image for this branch
make exec        # Start or attach to the privileged devcontainer shell
make run-backend
make run-frontend
make clean
```

---

## Common workflows

### 1) Monitor a Python agent

```python
from agent_tracker import AgentTracker

tracker = AgentTracker("http://127.0.0.1:8080")
tracker.start()

with open("/tmp/example.txt", "w") as f:
    f.write("hello")
```

The backend registers the current PID with the default tag **`AI Agent`**.

### 2) Monitor a Node.js agent

```javascript
const AgentTracker = require('./agentTracker');

const tracker = new AgentTracker('http://127.0.0.1:8080');
tracker.start();
```

### 3) Track a CLI or path without adapter code

From the **Configuration** page you can:

- add a command name such as `git`, `python`, `node`, `bun`, or a custom binary,
- add an exact file or directory path,
- assign each rule to a tag.

### 4) Install AI CLI hooks

From the **Hooks** page you can manage:

- **Claude Code** native hook
- **Gemini CLI** native hook
- **Codex** native hook
- **GitHub Copilot CLI** native hook
- **Kiro CLI** native hook
- **Cursor** wrapper alias

Native hook installation edits the target CLI config file in the user home directory and injects a generated relay script that forwards hook JSON to `POST /hooks/event`.
For Codex, the backend writes `~/.codex/hooks.json` and also enables `[features].codex_hooks = true` in `~/.codex/config.toml` to match the current official hooks setup.
For Kiro CLI, the backend creates a managed agent at `~/.kiro/agents/agent-ebpf-hook.json` from `kiro_default`, injects the native hook there, and points `chat.defaultAgent` in `~/.kiro/settings/cli.json` to that managed agent while the hook is installed.

### 5) Run commands through the wrapper

```bash
make wrapper
./agent-wrapper git status
./agent-wrapper rm -rf /tmp/demo
```

The wrapper sends the command to the backend over `/tmp/agent-ebpf.sock`, receives the decision, then executes the command.

---

## API overview

### Event / control endpoints

- `GET /ws` — live legacy event stream (protobuf binary, all kernel/wrapper/hook events)
- `GET /ws/envelopes` — normalized `EventEnvelopeBatch` stream for observability consumers
- `GET /ws/system?interval=2000` — process/system telemetry stream
- `GET /ws/shell?session_id=...` — attach to a PTY session
- `GET /ws/shell-sessions` — live shell session list (WebSocket JSON push, pub/sub)
- `GET /events/recent?type=&limit=` — historical events for initial load (REST fallback), now including a normalized `Envelope` per record
- `GET /events/graph?...` — aggregated execution graph nodes / edges for the current retained event window; pass `replay_path=/path/to/events.jsonl` to render a recording file
- `GET /events/recording` / `POST /events/recording/start|stop|replay` — record live captured events to JSONL files and replay them into the execution graph
- `POST /events/recording/browser/save` — persist browser-memory execution-graph snapshots to a backend JSON file
- `GET /network/flows?filter=&sort=&showHistoric=&limit=&cursor=` — attributed TCP / UDP flow summaries with DPI fields (`dnsName`, `sni`, `httpHost`, `tlsAlpn`), process / agent context, rate counters, staleness, and risk
- `GET /network/flows/:flowID` — one enriched flow by stable 5-tuple flow ID
- `GET /network/dns-cache` — active DNS correlation cache
- `GET /network/interfaces` — per-interface RX / TX counters, packets, errors, drops, and timestamp
- `GET /network/export/jsonl` — metadata-only flow JSONL export with process / agent attribution
- `GET /sandbox/cgroup/status` — cgroup/connect + sendmsg OS-level blocking availability, pinned-map state, active block entries, and cgroup sock-address decision counters (`checked` / `blocked` / `allowed`, with legacy `connect*` aliases)
- `POST /sandbox/cgroup/block-cgroup` / `unblock-cgroup` — block or release outbound connects for a cgroup id
- `POST /sandbox/cgroup/block-pid` / `unblock-pid` — resolve a PID's cgroup v2 inode id and block or release that cgroup
- `POST /sandbox/cgroup/block-ip` / `unblock-ip` — block or release an IPv4 or IPv6 destination globally
- `POST /sandbox/cgroup/block-port` / `unblock-port` — block or release a TCP/UDP destination port globally
- `GET /sandbox/lsm/status` — BPF LSM attach state, pinned-map state, active block entries, and exec and file-operation counters
- `POST /sandbox/lsm/block-exec-path` / `unblock-exec-path` — block or release an executable path in `bprm_check_security`
- `POST /sandbox/lsm/block-exec-name` / `unblock-exec-name` — block or release an executable basename in `bprm_check_security`
- `POST /sandbox/lsm/block-file-name` / `unblock-file-name` — block or release a file/directory basename in `file_open`, `file_permission`, `mmap_file`, `file_mprotect`, `inode_setattr`, `inode_create`, `inode_link`, `inode_symlink`, `inode_unlink`, `inode_mkdir`, `inode_rmdir`, `inode_mknod`, and `inode_rename`

The cgroup and LSM maps are loaded empty on first boot; mutating API calls are required to install block entries and are protected by the runtime policy-management gate.

For validation, `rtk make os-enforcement-preflight` checks host prerequisites such as bpffs write access directly or through passwordless sudo / `OS_SMOKE_PRIVILEGE_CMD`, root/passwordless sudo or custom privilege command, cgroup v2, the selected cgroup attach path (including temporary cgroup creation when a privilege runner is available), BPF LSM visibility, compiled cgroup/LSM object sections, and smoke-test tools (`curl` / `python3`). `rtk make os-enforcement-check` runs rootless object/script checks. `rtk make os-enforcement-smoke` expects a privileged backend that is already running; `rtk make os-enforcement-smoke-start` builds and starts one with `DISABLE_AUTH=true` when root, passwordless sudo, or an explicit `OS_SMOKE_PRIVILEGE_CMD` command prefix is available. The live smoke covers LSM exec/open/existing-fd read-write/mmap/mprotect/ftruncate/fchmod/setattr/create/link/symlink/unlink/mkdir/rmdir/mknod/rename denial and cgroup/connect PID-cgroup, TCP destination-port, UDP connected-socket destination/port, existing connected UDP sends, UDP sendto/sendmsg destination/port, IPv4-destination, IPv4-mapped IPv6-destination, and IPv6-destination denial.
- `GET /metrics` — Prometheus exposition for ringbuf / queue / WS / per-type / per-pid counters
- `GET /system/otel-health` — OTLP exporter readiness / queue / active-span counters
- `POST /register` — register a PID
- `POST /unregister` — unregister a PID
- `POST /hooks/event` — receive native hook events
- `POST /shell-sessions` — create a persistent PTY session
- `GET /shell-sessions` — list PTY sessions
- `DELETE /shell-sessions/:id` — close a PTY session
- `POST /shell-sessions/:id/input` — inject raw bytes into a PTY session

In release mode, **all endpoints above require the runtime access token** except `POST /hooks/event`, which accepts either:

- the normal access token (`X-API-KEY`, `Authorization: Bearer`, or `?key=...`), or
- a per-hook secret via `X-Agent-Hook-Secret` paired with `X-Agent-CLI`.

### Config and system endpoints

Protected by the same release-mode access token:

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
- `/config/ml/tune` — start square-grid auto parameter tuning over the current ML hyperparameters and stream progress/state via `/config/ml/status`; the result payload includes heatmap-ready scores for validation accuracy or inference throughput
- `/config/ml/existing-commands`, `/config/ml/import-existing`, `/config/ml/assess` — pull historical wrapper/hook command data into ML samples and run command safety assessment
- `/config/ml/datasets/pull`, `/config/ml/datasets/import`, `/config/ml/datasets/export`, `DELETE /config/ml/datasets` — fetch remote HTTP/HTTPS raw datasets or local file content, preview them, import them into the ML training store, export the current training set, or clear it in one step; archives are auto-expanded recursively for common zip / tar / gzip / bzip2 / xz payloads
- the Configuration UI also includes a curated classic OS-security dataset catalog for GTFOBins, LOLBAS, Claude Code Safety Net, ADFA, CERT Insider Threat, LANL host/network, and DARPA IDS references; it also exposes synthetic expansion presets and batch import of downloadable internet datasets, while archival pages still need you to download or extract the actual data files first
- `/system/ls`
- `/system/collector-health`
- `/system/otel-health`
- `/system/run`
- `/system/env`
- `/mcp` — MCP SSE endpoint (same auth as config routes)

Dangerous capabilities are also runtime-gated and default to **disabled** until explicitly enabled from `/config/runtime`:

- PTY / shell session creation and attachment
- `/system/run`
- hook installation / raw hook writes
- policy mutations (tags / comms / paths / prefixes / wrapper rules / cgroup sandbox maps / BPF LSM path/name maps for exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename / config import)

### Cluster endpoints

- `GET /cluster/state` — current node role and cluster mode
- `GET /cluster/nodes` — discovered slave nodes
- `POST /cluster/heartbeat` — slave heartbeat / registration (internal)

---

## Important behavior and limitations

### PID registration seeds process lineage

Registering a PID adds the process to `agent_pids` and seeds a process-context record.

- `execve` in-place keeps the PID and remains tracked.
- child processes created later now inherit tracking through `sched_process_fork` / `clone` plus parent-PID context fallback in user space.
- descendants can carry `root_agent_pid`, `agent_run_id`, `task_id`, `conversation_id`, `turn_id`, `tool_call_id`, `tool_name`, `trace_id`, `span_id`, `decision`, `argv_digest`, and `cwd` when the caller provides them.

### Command and path matching are exact-match maps

- `tracked_comms` uses a fixed 16-byte command key.
- `tracked_paths` uses a fixed 256-byte path key.

That means the current implementation is best for:

- short executable names like `git`, `node`, `python`, `bun`
- exact absolute paths you care about

It is **not** a recursive path policy engine.

### Export / import scope

`/config/export` and `/config/import` currently cover:

- tags
- tracked commands
- tracked paths

They currently include:

- tags
- tracked commands
- tracked paths
- wrapper rules

They still do **not** include raw native hook config files.

### Privilege model

- The backend must run with elevated privileges to bootstrap eBPF maps and links.
- Spawned shells and wrapper-launched commands attempt to drop back to the invoking user with `SUDO_UID` / `SUDO_GID`.
- The wrapper UDS socket at `/tmp/agent-ebpf.sock` is created with restrictive permissions and peer-credential checks for root / the original invoking user.

### Auth model

- In non-release mode, auth is disabled by default.
- In release mode, the runtime access token protects config, system, WebSocket, shell-session, register / unregister, metrics, event-history / graph, network-inspection, and OS sandbox (`/sandbox/**`) routes.
- `POST /hooks/event` accepts either that token or a per-hook secret.
- The runtime page persists the token locally and appends it to WebSocket URLs via `?key=...`.
- The runtime page also shows collector health, including ringbuf reserve-fail counters and per-event-type totals.

For anything beyond local use, put the app behind a trusted reverse proxy and tighten auth coverage.

---

## Documentation map

- [`AGENTS.md`](./AGENTS.md) — contributor / coding-agent guide
- [`agents.md`](./agents.md) — agent registration and tracking guide
- [`docs/architecture.md`](./docs/architecture.md) — component and data-flow architecture
- [`backend/README.md`](./backend/README.md) — backend internals and API surface
- [`frontend/README.md`](./frontend/README.md) — frontend structure and route map
- [`wrapper/README.md`](./wrapper/README.md) — wrapper protocol and behavior
- [`adapters/python/README.md`](./adapters/python/README.md) — Python adapter usage
- [`adapters/js/README.md`](./adapters/js/README.md) — Node adapter usage

---

## Troubleshooting

### Backend fails to start eBPF components

Check:

- kernel supports eBPF + BTF,
- `/sys/fs/bpf` is mounted,
- `clang` is installed,
- the backend can elevate via `sudo` or `pkexec`.

### Frontend cannot reach backend in dev mode

- confirm `backend/.port` exists,
- confirm Vite is running from `frontend/`,
- confirm the backend actually started on the chosen port.

### Native hooks show installed but no events arrive

Check:

- the target CLI is reading the config file you edited,
- `curl` is installed,
- the backend is reachable at the current hook callback URL (typically `http://127.0.0.1:<port>/hooks/event`),
- the hook config file still contains the injected `agent-ebpf-hook-active` marker.

### Wrapper commands do not enforce policy

Check:

- the backend is running,
- `/tmp/agent-ebpf.sock` exists,
- `agent-wrapper` can connect to that socket,
- the command rule exists under **Configuration → Wrapper Rules**.
