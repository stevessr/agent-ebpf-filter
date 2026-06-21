# Developer & Coding Agent Guide

**Welcome to Agent eBPF Filter development!** This guide helps both human developers and AI coding assistants contribute effectively to the project.

---

## Quick Navigation

### For Human Developers
- [Project Structure](#project-structure) — Repository layout and key files
- [Development Setup](#development-setup) — Get started in minutes
- [Common Tasks](#common-tasks) — Frequently performed operations
- [Code Style](#code-style) — Conventions and patterns

### For AI Coding Assistants
- [Project Context](#project-context) — What you need to know first
- [Claude Code Skills](#claude-code-skills) — Available project-specific skills
- [Making Changes](#making-changes) — Best practices for code modifications
- [Testing & Validation](#testing--validation) — Verify your changes

---

## Project Context

Agent eBPF Filter is a **Linux observability and control plane** for AI agents and developer CLIs, combining:

- **eBPF kernel tracing** (`backend/ebpf/*.c`) — Captures syscalls via tracepoints
- **Go backend** (`backend/app/*.go`) — HTTP/WebSocket API, policy engine, privilege management
- **Vue 3 frontend** (`frontend/src/`) — Real-time dashboard and configuration UI
- **CLI wrapper** (`wrapper/main.go`) — Command interception and policy enforcement
- **Language adapters** (`adapters/python/`, `adapters/js/`) — PID registration helpers
- **Protocol definitions** (`proto/*.proto`) — Single source of truth for event schemas

### Key Concepts

1. **Event Flow:** eBPF ringbuffer → Go backend → WebSocket → Vue frontend
2. **Tracking:** PID registration, command names, exact paths, wrapper interception, native hooks
3. **Enforcement:** Wrapper policies (ALLOW/BLOCK/ALERT/REWRITE), cgroup network blocking, BPF LSM file/exec blocking
4. **Security:** Release-mode auth, runtime gates, 4-level redaction, TLS capture disabled by default

---

## Project Structure

```
agent-ebpf-filter/
├── backend/                    # Go backend
│   ├── app/                    # HTTP API, WebSocket, routing
│   │   ├── main.go             # Entry point
│   │   ├── routes__*.go        # Route groups
│   │   └── events__*.go        # Event handling
│   ├── ebpf/                   # eBPF programs
│   │   ├── agent_tracker.c     # Main tracepoint program
│   │   ├── cgroup_sandbox.c    # Network blocking (cgroup/connect + sendmsg)
│   │   ├── lsm_enforcer.c      # File/exec blocking (BPF LSM)
│   │   └── gen.go              # bpf2go generation
│   ├── core/                   # State management
│   │   └── state_types.go      # RuntimeSettings, ConfigState
│   ├── redaction/              # Data sanitization
│   └── pb/                     # Generated protobuf
├── frontend/                   # Vue 3 + TypeScript
│   ├── src/
│   │   ├── views/              # Pages (Dashboard, Network, Config, etc.)
│   │   ├── components/         # Reusable components
│   │   ├── router/             # Vue Router config
│   │   └── pb/                 # Generated protobuf (JS)
│   └── vite.config.ts
├── wrapper/                    # agent-wrapper CLI
│   └── main.go
├── adapters/
│   ├── python/                 # Python PID registration
│   │   └── agent_tracker.py
│   └── js/                     # Node.js PID registration
│       └── agentTracker.js
├── proto/                      # Protobuf schemas (source of truth)
│   ├── tracker_events.proto    # Event definitions
│   └── tracker.proto
├── docs/                       # VitePress documentation
│   ├── guide/                  # Getting started
│   ├── architecture/           # System design
│   ├── backend/                # Backend internals
│   ├── security/               # Security model
│   └── index.md                # Documentation home
├── deploy/kubernetes/          # Kubernetes manifests
├── kernel-ml/                  # Optional DKMS kernel ML module
├── scripts/                    # Demo and validation scripts
├── Makefile                    # Build orchestration
└── .claude/                    # Claude Code skills
    └── skills/
        ├── project-structure/  # Repository navigation skill
        ├── configure-security/ # Security policy management
        ├── analyze-network/    # Network traffic analysis
        └── monitor-process/    # Process behavior monitoring
```

---

## Development Setup

### Prerequisites

- Linux with eBPF + BTF support
- Go 1.26.2+
- Bun (frontend build tool)
- clang/LLVM (eBPF compilation)
- protoc (Protocol Buffers compiler)
- sudo or pkexec

### First-Time Setup

```bash
# 1. Install dependencies
make predev

# 2. Configure development environment (optional interactive TUI)
make dev-env

# 3. Start development session (backend + frontend in Zellij)
make dev
```

The `make dev-env` command launches a TUI that helps configure `.env.dev` with:
- Backend port
- Feature flags
- ML/LLM settings
- Redaction levels
- CUDA support
- Development toggles

### Manual Environment Configuration

If you prefer not to use the TUI, create `.env.dev`:

```bash
# Backend
AGENT_BACKEND_PORT=8080
GIN_MODE=debug

# Features
AGENT_BUILD_FEATURES=all
AGENT_RUNTIME_TLS_CAPTURE_ENABLED=false
AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED=false

# Security
AGENT_REDACTION_LEVEL=standard
DISABLE_AUTH=true  # dev only

# ML (optional)
AGENT_LLM_PROVIDER=openai
OPENAI_API_KEY=sk-...
```

### Using Devcontainer

```bash
# Pull GitHub-built image
make docker

# Start privileged container
make exec
```

---

## Common Tasks

### Regenerate Protobuf Bindings

```bash
make proto
```

This regenerates:
- `backend/pb/*.pb.go` (Go)
- `frontend/src/pb/*.js` and `*.d.ts` (TypeScript)

### Rebuild eBPF Programs

```bash
make backend
```

This runs `bpf2go` and regenerates `backend/ebpf/*.o` and Go bindings.

### Rebuild Wrapper

```bash
make wrapper
```

### Build Everything

```bash
make all
```

### Run Backend Only

```bash
cd backend
go run ./app
```

### Run Frontend Only

```bash
cd frontend
bun run dev
```

### Production Build

```bash
make run
```

Backend serves compiled frontend assets.

### Install as System Service

```bash
sudo make install

# Check status
systemctl status agent-ebpf-filter

# Uninstall
sudo make uninstall
```

### Run Validation Tests

```bash
# Static check (no root required)
make os-enforcement-check

# Preflight check (needs sudo)
make os-enforcement-preflight

# Live smoke test (needs running backend as root)
make os-enforcement-smoke
```

### Clean Build Artifacts

```bash
make clean
```

---

## Code Style

### Go Backend

- **Formatting:** `gofmt` (enforced)
- **Imports:** Standard lib → third-party → local, separated by blank lines
- **Naming:** 
  - Exported: `PascalCase`
  - Unexported: `camelCase`
  - Constants: `PascalCase` or `SCREAMING_SNAKE_CASE` for clarity
- **Error handling:** Always check errors, use `fmt.Errorf` with context
- **Logging:** Use structured logging (e.g., `log.Info().Str("key", value).Msg("message")`)
- **File organization:** 
  - `routes__*.go` for route groups
  - `events__*.go` for event handling
  - `*_types.go` for type definitions

### Vue Frontend

- **Framework:** Vue 3 Composition API with `<script setup>`
- **Language:** TypeScript preferred
- **Formatting:** Prettier (configured in `.prettierrc`)
- **Component naming:** `PascalCase.vue`
- **Props:** Always define with TypeScript interfaces
- **Reactivity:** Use `ref()` and `reactive()` appropriately
- **API calls:** Centralize in composables or service files

### eBPF C Code

- **Style:** Kernel style (indent with tabs)
- **Verification:** Keep verifier happy (bounded loops, no unbounded recursion)
- **Maps:** Pin to `/sys/fs/bpf/agent-ebpf/*` for persistence
- **Comments:** Explain tricky verifier workarounds

### Protobuf

- **Style:** snake_case for fields, PascalCase for messages
- **Documentation:** Document all fields with comments
- **Versioning:** Never remove or change field numbers

---

## Claude Code Skills

The project includes several skills for AI assistants working on this codebase:

### `project-structure`

Helps navigate the repository structure through layered references. Use when:
- You need to understand where a feature is implemented
- You're looking for the right file to modify
- You need to understand component relationships

**Example:**
```
/project-structure "Where is the cgroup network blocking implemented?"
```

### `configure-security`

Manages security policies via MCP tools. Use when:
- Adding tracked commands or paths
- Configuring wrapper rules
- Setting up network or file blocking

**Example:**
```
/configure-security "Block all connections to 203.0.113.0/24"
```

### `analyze-network`

Analyzes captured network traffic. Use when:
- Investigating unusual connections
- Understanding agent network behavior
- Debugging network-related issues

**Example:**
```
/analyze-network "Show all connections from PID 12345"
```

### `monitor-process`

Deep process behavior monitoring. Use when:
- Debugging agent behavior
- Understanding file access patterns
- Investigating privilege escalation

**Example:**
```
/monitor-process "Monitor PID 12345 for suspicious file access"
```

---

## Making Changes

### Adding a New API Endpoint

1. **Define the route** in appropriate `backend/app/routes__*.go`
2. **Implement handler** in same file or dedicated handler file
3. **Add auth check** if needed (check `requireAccessToken` middleware)
4. **Update OpenAPI spec** if adding to External API v1
5. **Test manually** with curl or frontend
6. **Add to documentation** in `docs/backend/routes-api.md`

### Adding a New eBPF Hook

1. **Add hook** to `backend/ebpf/agent_tracker.c` (or create new `.c` file)
2. **Define event struct** in same file
3. **Update proto** in `proto/tracker_events.proto` if adding new event type
4. **Regenerate:** `make proto && make backend`
5. **Handle in Go** in `backend/app/events__*.go`
6. **Update frontend** to display new event type
7. **Test** with target syscall

### Adding a New Frontend View

1. **Create view** in `frontend/src/views/YourView.vue`
2. **Add route** in `frontend/src/router/index.ts`
3. **Add navigation** in main menu component
4. **Connect to backend** via WebSocket or REST API
5. **Test** in browser

### Modifying Runtime Configuration

1. **Update struct** in `backend/core/state_types.go` (`RuntimeSettings`)
2. **Update default values** in initialization code
3. **Update frontend UI** in `frontend/src/views/Configuration.vue`
4. **Test** that settings persist and apply correctly

---

## Testing & Validation

### Manual Testing

```bash
# Start backend (will self-elevate if needed)
cd backend
go run ./app

# In another terminal, start frontend
cd frontend
bun run dev

# Access UI at http://localhost:5173
```

### Testing eBPF Programs

```bash
# Check loaded programs
sudo bpftool prog list

# Check pinned maps
sudo bpftool map list
ls -la /sys/fs/bpf/agent-ebpf/

# Monitor events
sudo cat /sys/kernel/debug/tracing/trace_pipe
```

### Testing Wrapper

```bash
# Build wrapper
make wrapper

# Test command interception
./agent-wrapper echo "hello"
./agent-wrapper ls -la
```

### Testing Python Adapter

```bash
cd adapters/python
uv pip install -e .

python3 -c "
from agent_tracker import AgentTracker
tracker = AgentTracker('http://127.0.0.1:8080')
tracker.start()
print('Registered!')
"
```

### Testing Network Blocking

```bash
# Requires backend running as root

# Block an IP
curl -X POST http://localhost:8080/sandbox/cgroup/block-ip \
  -H "X-API-KEY: $(cat ~/.config/agent-ebpf-filter/runtime.json | jq -r .access_token)" \
  -d '{"ip": "1.1.1.1"}'

# Try to connect (should fail)
curl https://1.1.1.1

# Unblock
curl -X POST http://localhost:8080/sandbox/cgroup/unblock-ip \
  -H "X-API-KEY: $(cat ~/.config/agent-ebpf-filter/runtime.json | jq -r .access_token)" \
  -d '{"ip": "1.1.1.1"}'
```

### Testing BPF LSM Blocking

```bash
# Block executable
curl -X POST http://localhost:8080/sandbox/lsm/block-exec-path \
  -H "X-API-KEY: $(cat ~/.config/agent-ebpf-filter/runtime.json | jq -r .access_token)" \
  -d '{"path": "/bin/cat"}'

# Try to execute (should fail with EACCES)
/bin/cat /etc/passwd

# Unblock
curl -X POST http://localhost:8080/sandbox/lsm/unblock-exec-path \
  -H "X-API-KEY: $(cat ~/.config/agent-ebpf-filter/runtime.json | jq -r .access_token)" \
  -d '{"path": "/bin/cat"}'
```

---

## Debugging Tips

### Backend Not Starting

1. Check kernel support: `cat /sys/kernel/btf/vmlinux | head`
2. Check bpffs: `mount | grep bpf`
3. Check privilege: `sudo -v`
4. Check logs: Look for eBPF bootstrap errors in stdout

### Frontend Can't Connect

1. Check `backend/.port` exists and is readable
2. Check Vite proxy config in `frontend/vite.config.ts`
3. Check backend is running: `lsof -i :8080`
4. Check browser console for WebSocket errors

### Events Not Appearing

1. Check if PID is registered: `curl http://localhost:8080/config/export | jq .`
2. Check if command/path is tracked: Same endpoint
3. Check eBPF maps: `sudo bpftool map dump name agent_pids`
4. Check event filtering logic in `backend/ebpf/agent_tracker.c`

### Permission Denied Errors

1. Check if backend is running with sufficient privileges
2. Check map permissions: `ls -la /sys/fs/bpf/agent-ebpf/`
3. Check cgroup v2: `mount | grep cgroup2`
4. Check BPF LSM: `cat /sys/kernel/security/lsm | grep bpf`

---

## Resources

### Documentation

- [Main README](README.md) — Project overview
- [Documentation Site](docs/index.md) — Comprehensive docs
- [Architecture](docs/architecture/overview.md) — System design
- [Security Model](docs/security/model.md) — Security considerations
- [API Reference](docs/backend/routes-api.md) — Backend API

### External Resources

- [eBPF Documentation](https://ebpf.io/)
- [cilium/ebpf](https://github.com/cilium/ebpf) — Go eBPF library
- [Vue 3 Docs](https://vuejs.org/)
- [BPF LSM](https://docs.kernel.org/bpf/prog_lsm.html)

### Getting Help

- **Issues:** Use GitHub Issues for bugs
- **Discussions:** Use GitHub Discussions for questions
- **Code:** Check existing implementations for patterns

---

## Contributing Guidelines

1. **Read existing code** to understand patterns before adding new features
2. **Test your changes** thoroughly before submitting
3. **Update documentation** when adding features or changing behavior
4. **Follow code style** for the language you're working in
5. **Check security implications** of changes, especially for enforcement features
6. **Verify eBPF verifier** is happy with any eBPF changes
7. **Test across contexts:** dev mode, release mode, with/without auth

---

## For AI Coding Assistants: Special Notes

When working on this codebase:

1. **Read before writing:** Always read relevant files before making changes
2. **Use project skills:** The `/project-structure`, `/configure-security`, `/analyze-network`, and `/monitor-process` skills are designed to help you
3. **Check security:** This is a security-sensitive project. Consider:
   - Is this feature properly gated by runtime config?
   - Does it need authentication?
   - Should data be redacted?
   - Is TLS capture involved? (should be opt-in)
4. **Test enforcement features carefully:** Kernel-level blocking can lock you out
5. **Respect the documentation structure:** Keep `docs/` organized by the existing taxonomy
6. **Protobuf is source of truth:** When in doubt about event schemas, check `proto/*.proto`

---

**Happy coding!** Whether you're human or AI, we're glad you're contributing to Agent eBPF Filter. 🚀
