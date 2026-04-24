# Agent eBPF Filter

Linux-first observability and control plane for AI agents and developer CLIs.

This project combines:

- **kernel-space eBPF tracing** for selected syscalls,
- **user-space PID registration** for agent opt-in,
- **command/path tagging** through pinned BPF maps,
- **wrapper- and hook-based interception** for AI CLIs,
- **a Go + Vue dashboard** for live inspection and control.

It is designed for local development workstations and lab environments where you want to see what an agent is opening, executing, connecting to, or attempting to modify.

---

## What the project currently does

### Kernel-side telemetry

The eBPF program listens to these syscall tracepoints:

- `sys_enter_execve`
- `sys_enter_openat`
- `sys_enter_connect`
- `sys_enter_mkdirat`
- `sys_enter_unlinkat`
- `sys_enter_ioctl`
- `sys_enter_bind`
- `sys_enter_sendto`
- `sys_enter_recvfrom`

Events are written to a ring buffer and consumed by the Go backend.

### User-space telemetry and control

- **PID registration**: Python / Node adapters call `/register` and `/unregister`.
- **Tracked command names**: common CLIs plus user-defined commands are tagged through `tracked_comms`.
- **Tracked paths**: exact path matches are tagged through `tracked_paths`.
- **Wrapper interception**: `agent-wrapper` asks the backend for `ALLOW`, `BLOCK`, `ALERT`, or `REWRITE`.
- **Native AI CLI hooks**: the backend can install hook config for Claude Code, Gemini CLI, Codex, and GitHub Copilot, or wrapper aliases for Cursor / any CLI routed through the wrapper.
  Hook callbacks resolve against the backend's current port instead of assuming `8080`.

### UI surfaces

- **Dashboard**: live event stream with tag / type / PID / command / path filters
- **Monitor**: process / CPU / memory / GPU / IO / page-fault telemetry
- **Network**: syscall-derived network flow table with direction / endpoint filters
- **Explorer**: browse the host filesystem and add tracked paths
- **Executor**: run commands via `agent-wrapper` and manage interactive PTY sessions
- **Hooks**: install or edit native hook configs / wrapper aliases
- **Configuration**: manage tags, tracked commands, tracked paths, and wrapper rules

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
   - The eBPF program only emits events when a PID, command, or path matches a tracked rule.

4. **Event fan-out**
   - Kernel events, wrapper events, and native hook events are normalized into protobuf messages.
   - The backend broadcasts them over `/ws` to the Vue frontend.

5. **Policy enforcement**
   - `agent-wrapper` connects to `/tmp/agent-ebpf.sock`.
   - The backend evaluates wrapper rules and returns `ALLOW`, `BLOCK`, `ALERT`, or `REWRITE`.

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
make dev
```

What it does:

- generates protobuf bindings,
- builds `agent-wrapper`,
- regenerates eBPF bindings,
- builds the backend binary,
- starts the backend, which then self-elevates when needed,
- writes the chosen backend port to `backend/.port`,
- starts Vite for the frontend.

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

### Public event / control endpoints

- `GET /ws` — live event stream
- `GET /ws/system?interval=2000` — process/system telemetry stream
- `POST /register` — register a PID
- `POST /unregister` — unregister a PID
- `POST /hooks/event` — receive native hook events
- `POST /shell-sessions` — create a persistent PTY session
- `GET /shell-sessions` — list PTY sessions
- `DELETE /shell-sessions/:id` — close a PTY session
- `GET /ws/shell?session_id=...` — attach to a PTY session

### Config and system endpoints

Protected by `authMiddleware()` in release mode:

- `/config/tags`
- `/config/comms`
- `/config/paths`
- `/config/rules`
- `/config/export`
- `/config/import`
- `/config/hooks`
- `/system/ls`
- `/system/run`

---

## Important behavior and limitations

### PID registration is per process

Registering a PID adds **that process** to `agent_pids`.

- `execve` in-place keeps the PID and remains tracked.
- child processes created later do **not** automatically inherit registration unless they are also registered or matched by tracked command/path rules.

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

### Auth model

- In non-release mode, auth is disabled by default.
- In release mode, `X-API-KEY` is required for `/config/**` and `/system/**`.
- The current frontend does **not** expose an API-key login flow.
- WebSocket endpoints, shell session endpoints, `/register`, `/unregister`, and `/hooks/event` are not wrapped by `authMiddleware()` today.

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
