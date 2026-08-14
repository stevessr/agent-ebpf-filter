# Agent eBPF Filter

**Linux-first observability and control plane for AI agents and developer CLIs.**

[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![Documentation](https://img.shields.io/badge/docs-VitePress-green.svg)](docs/index.md)

> Real-time monitoring, semantic correlation, and kernel-level enforcement for AI agent behaviors on Linux workstations.

---

## What is Agent eBPF Filter?

Agent eBPF Filter combines **eBPF kernel tracing**, **Go backend**, **Vue.js dashboard**, and **multiple enforcement layers** to provide comprehensive observability and control over AI agents and developer tools running on Linux systems.

**Core Question:** When an AI agent or CLI executes on your machine, how do you know:
- What commands it actually ran?
- Which files it accessed, modified, or deleted?
- What network connections it established?
- Which agent run, tool call, or trace these actions belong to?
- Whether sensitive data was properly redacted?

**Agent eBPF Filter provides the answers.**

---

## Key Features

### 🔍 **Kernel-Level Observability**
- eBPF tracepoints capture `execve`, `openat`, `connect`, `sendto`, `recvfrom`, `bind`, `ioctl`, `mkdir`, `unlink` syscalls
- Zero-copy ringbuffer decoding for high-performance event collection
- Automatic handling of missing kernel tracepoints for compatibility

### 🎯 **Semantic Correlation**
- PID registration via Python/Node.js adapters
- Native hooks for Claude Code, Gemini CLI, Codex, Pi, Oh My Pi, GitHub Copilot, Kiro CLI, Augment, and Antigravity CLI
- Wrapper-based command interception for Cursor and DeepSeek Harness (`dsh`)
- Context tracking: `agent_run_id`, `tool_call_id`, `trace_id`, `cwd`, `argv_digest`

### 🛡️ **Multi-Layer Enforcement**
- **User-space:** `agent-wrapper` provides ALLOW/BLOCK/ALERT/REWRITE decisions
- **Kernel-space cgroup:** Block network connections at kernel level (TCP/UDP, IPv4/IPv6)
- **Kernel-space BPF LSM:** Block file access and process execution at LSM hooks
- **ML-based risk scoring:** Optional machine learning classification

### 📊 **Rich Web Dashboard**
- **Dashboard:** Real-time event stream with strace-style summaries
- **Network:** Flow attribution with DNS/SNI/HTTP enrichment
- **Execution Graph:** Process topology with behavior tracking
- **AgentSight Integration:** Compatible with AgentSight event format
- **Recording/Replay:** Capture and replay execution sessions

### 🔐 **Security-First Design**
- Release-mode authentication with runtime access tokens
- Four-level data redaction (None/Basic/Standard/Strict)
- Runtime feature gates for dangerous capabilities
- TLS capture **disabled by default** (opt-in diagnostic tool)
- Automatic key/credential removal from captured data

### 🔌 **Integration Ready**
- **MCP Server:** Tools and resources for AI agent integration
- **External API v1:** OpenAPI-documented REST endpoints
- **OTLP Export:** Span telemetry for observability platforms
- **Prometheus Metrics:** Standard monitoring integration
- **Kubernetes:** DaemonSet manifests included

---

## Quick Start

### Prerequisites

- **Linux** with eBPF and BTF support
- **Go 1.26.2+**
- **Bun** (frontend build tool)
- **clang/LLVM** (eBPF compilation)
- **protoc** (Protocol Buffers compiler)
- **sudo** or **pkexec** (privilege elevation)

### Development Mode

```bash
# Install dependencies
make predev

# Start backend + frontend in Zellij session
make dev
```

The frontend will be available at `http://localhost:5173`, and the backend API at the port specified in `backend/.port`.

### Production Build

```bash
# Build and run (backend serves compiled frontend)
make run
```

### System Service Installation

```bash
# Install as systemd service (or rc.local fallback)
make install

# Check status
systemctl status agent-ebpf-filter

# Uninstall
make uninstall
```

### Docker Development Container

```bash
# Pull GitHub-built devcontainer image
make docker

# Start or attach to privileged container shell
make exec
```

---

## Usage Examples

### Monitor a Python Agent

```python
from agent_tracker import AgentTracker

tracker = AgentTracker("http://127.0.0.1:8080")
tracker.start()

# All syscalls from this process are now tracked
with open("/tmp/example.txt", "w") as f:
    f.write("hello from agent")
```

### Monitor a Node.js Agent

```javascript
const AgentTracker = require('./agentTracker');

const tracker = new AgentTracker('http://127.0.0.1:8080');
tracker.start();

// Agent activities are now visible in the dashboard
```

### Track Commands Without Code Changes

From the **Configuration** page in the web UI:
- Add command names: `git`, `npm`, `python`, `curl`, etc.
- Add exact file paths: `/etc/passwd`, `~/.ssh/config`
- Assign tags to organize tracked resources

### Install AI CLI Hooks

From the **Hooks** page, install native or wrapper integrations for:
- Claude Code
- Gemini CLI
- Codex
- DeepSeek Harness (`dsh`, wrapper-only)
- Pi (TypeScript extension)
- Oh My Pi (`omp`, TypeScript extension)
- GitHub Copilot CLI
- Kiro CLI
- Augment/Auggie CLI
- Antigravity CLI (`agy`)
- Cursor (via wrapper alias)

### Block Network Destinations (Kernel-Level)

```bash
# Via MCP tool or REST API
curl -X POST http://localhost:8080/sandbox/cgroup/block-ip \
  -H "X-API-KEY: your-token" \
  -d '{"ip": "203.0.113.42"}'
```

### Block File Access (BPF LSM)

```bash
# Block specific executable
curl -X POST http://localhost:8080/sandbox/lsm/block-exec-path \
  -H "X-API-KEY: your-token" \
  -d '{"path": "/usr/bin/dangerous-tool"}'
```

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    AI Agent / Developer CLI                  │
└────────┬─────────────────────────────────┬─────────────────┘
         │                                 │
         │ syscalls                        │ HTTP callbacks
         ▼                                 ▼
┌─────────────────────┐          ┌──────────────────────────┐
│   eBPF Tracepoints  │          │  Native Hooks / Wrapper  │
│   (kernel-space)    │          │    (user-space)          │
└──────────┬──────────┘          └────────────┬─────────────┘
           │                                  │
           │ ringbuf events                   │ JSON events
           ▼                                  ▼
    ┌────────────────────────────────────────────────┐
    │           Go Backend (privileged)              │
    │  • Event normalization & enrichment            │
    │  • Risk scoring & policy evaluation            │
    │  • cgroup/LSM map management                   │
    │  • Data redaction & archival                   │
    └────┬──────────────────────────────────────┬────┘
         │                                      │
         │ WebSocket/REST                       │ MCP/OTLP
         ▼                                      ▼
┌─────────────────────┐              ┌────────────────────────┐
│   Vue.js Dashboard  │              │  External Integrations │
│  • Real-time events │              │  • MCP clients         │
│  • Network flows    │              │  • Grafana/Loki        │
│  • Execution graph  │              │  • Prometheus          │
│  • Configuration    │              │  • Kubernetes          │
└─────────────────────┘              └────────────────────────┘
```

For detailed architecture, see [docs/architecture/overview.md](docs/architecture/overview.md).

---

## Documentation

Comprehensive documentation is available in the [`docs/`](docs/) directory, organized by topic:

| Section | Description |
|---------|-------------|
| [**Guide**](docs/guide/what-is-agent-ebpf-filter.md) | What it is, quick start, capabilities, reading paths |
| [**Architecture**](docs/architecture/overview.md) | System design, data flow, component interactions |
| [**Backend**](docs/backend/runtime-startup.md) | Startup sequence, API routes, event pipeline, ML models |
| [**Frontend**](docs/frontend/workbench.md) | Dashboard overview, routing, component structure |
| [**Security**](docs/security/model.md) | Security model, policy semantics, runtime gates |
| [**Integration**](docs/integrations/agents.md) | Agent adapters, wrappers, hooks, MCP/OTLP |
| [**Operations**](docs/operations/build-and-run.md) | Build process, deployment, validation |
| [**Delivery**](docs/delivery/competition-defense.md) | Competition defense materials, demo scripts |
| [**Reference**](docs/reference/code-entrypoints.md) | Code entry points, AgentSight acknowledgment |

**Start here:**
- New developers: [What is Agent eBPF Filter?](docs/guide/what-is-agent-ebpf-filter.md)
- Security review: [Security Model](docs/security/model.md)
- Integration: [External API](docs/integrations/external-api.md)
- Deployment: [Kubernetes Guide](docs/operations/kubernetes.md)

**Preview documentation site:**
```bash
cd docs
bun install
bun run docs:dev
```

---

## Project Structure

```
agent-ebpf-filter/
├── backend/              # Go backend with eBPF integration
│   ├── ebpf/             # eBPF C programs (tracepoint, cgroup, LSM)
│   ├── app/              # HTTP/WS API, routing, privilege management
│   ├── core/             # State management, configuration
│   ├── redaction/        # Data sanitization engine
│   └── pb/               # Generated protobuf bindings
├── frontend/             # Vue 3 + TypeScript dashboard
│   ├── src/views/        # Dashboard, Network, Graph, Config, etc.
│   └── src/components/   # Reusable UI components
├── wrapper/              # agent-wrapper command interceptor
├── adapters/             # Python and Node.js PID registration helpers
├── proto/                # Protobuf definitions (source of truth)
├── kernel-ml/            # Optional DKMS kernel-space ML module
├── docs/                 # VitePress documentation site
├── deploy/kubernetes/    # Kubernetes manifests
└── scripts/              # Demo and validation scripts
```

---

## Configuration

### Environment Variables

Configure via `.env.dev` (created by `make dev-env`):

```bash
# Backend
AGENT_BACKEND_PORT=8080
GIN_MODE=debug

# Features
AGENT_BUILD_FEATURES=all  # or: core,tls_capture,ml,plugins
AGENT_RUNTIME_TLS_CAPTURE_ENABLED=false
AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED=false

# ML/LLM
AGENT_LLM_PROVIDER=openai
AGENT_LLM_MODEL=gpt-4o-mini
OPENAI_API_KEY=sk-...

# Security
AGENT_REDACTION_LEVEL=standard  # none/basic/standard/strict
DISABLE_AUTH=true  # dev-only, never in production
```

### Runtime Configuration

Access runtime settings via the **Configuration → Runtime Config** tab in the web UI:

- Authentication & access tokens
- Feature gates (shell, hooks, policy management)
- Data retention & archival
- Redaction levels
- TLS capture (disabled by default)
- Kernel-risk feedback
- Domain forwarding (80/443 proxy)
- OTLP export

---

## MCP Integration

Agent eBPF Filter exposes an MCP server at `/mcp` (authenticated via `X-API-KEY` or `Authorization: Bearer`):

### Available Tools

- `tail_events` — Get recent captured events
- `query_events` — Search events by type/comm/pid
- `get_network_flows` — Network flow summary
- `add_tracked_command` / `add_tracked_path` — Add tracking rules
- `block_network_destination` / `block_process_cgroup` — Kernel-level blocking (requires `policyManagementEnabled`)
- `block_file_access` — BPF LSM file/exec blocking

### Example Usage with Claude Code

The project includes three Claude Code skills:
- `configure-security` — Manage security policies
- `analyze-network` — Analyze network traffic
- `monitor-process` — Deep process behavior monitoring

---

## Validation & Testing

### Rootless Static Check

```bash
make os-enforcement-check
```

### Privileged Preflight Check

```bash
make os-enforcement-preflight
```

### Live OS Enforcement Smoke Test

```bash
# Start backend as root with auth disabled
sudo -E env DISABLE_AUTH=true ./backend/agent-ebpf-filter &

# Run smoke test
make os-enforcement-smoke
```

Validates:
- BPF LSM exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/unlink/mkdir/rmdir/mknod/rename denial
- cgroup connect/sendmsg blocking for IPv4/IPv6 destinations and TCP/UDP ports

---

## Performance

- **Zero-copy ringbuffer:** mmap-backed aligned decoding
- **Low-latency risk scoring:** User-space kernel-risk evaluation before broadcast
- **Optional CUDA acceleration:** Kernel-space ML inference with CUDA helper
- **Efficient event filtering:** Only emit events for registered PIDs, tracked commands, or tracked paths

Benchmark:
```bash
make runtime-benchmark
```

---

## Security Considerations

### Default Security Posture

✅ **Safe by default:**
- TLS capture **disabled** by default
- Policy management **disabled** by default
- Shell/system commands **disabled** by default
- Hook installation **disabled** by default
- Release-mode authentication **enabled** in production
- Four-level redaction with automatic key removal

⚠️ **Dangerous capabilities require explicit opt-in:**
- TLS plaintext capture (diagnostic tool only)
- OS-level network/file blocking (requires authentication for OS sandbox (`/sandbox/**`))
- PTY session creation
- Hook configuration editing

### OS Sandbox Status & Enforcement

The system monitors and exposes the sandbox status via the API:
- `GET /sandbox/cgroup/status` returns the kernel state, active blocks, and decision counters as `checked` / `blocked` / `allowed`.
- It supports legacy `connect*` aliases for backward compatibility.
- It handles network filtering for IPv4-mapped IPv6-destination sockets and existing connected UDP sends.

### Threat Model

See [docs/security/threat-model.md](docs/security/threat-model.md) for comprehensive security analysis.

---

## Known Limitations

1. **Path matching is exact-match**, not recursive subtree matching
2. **Command matching is exact basename**, limited to 16 bytes
3. **Domain forwarding is a reverse proxy**, not transparent eBPF NAT
4. **TLS capture** requires explicit library/binary registration for non-system libraries
5. **cgroup blocking** requires cgroup v2
6. **BPF LSM blocking** requires kernel with BPF LSM enabled

---

## Troubleshooting

### Backend fails to start

Check:
- Kernel supports eBPF + BTF: `uname -r` and `/sys/kernel/btf/vmlinux`
- `/sys/fs/bpf` is mounted: `mount | grep bpf`
- `clang` is installed: `clang --version`
- Privilege elevation works: `sudo -v` or `pkexec --version`

### Frontend cannot connect to backend

Check:
- `backend/.port` file exists and contains valid port
- Backend is running: `ps aux | grep agent-ebpf-filter`
- Firewall allows local connections

### Native hooks not working

Check:
- Target CLI config or extension file contains the `agent-ebpf-hook-active` marker
- Pi/Oh My Pi are loading the generated TypeScript extension from their active extension directory
- DeepSeek Harness is invoked through the `agent-wrapper` alias; dsh profile/plugin configuration remains dsh-owned
- Hook callback URL is reachable from CLI process
- `curl` is available in PATH
- Backend is running and auth token is valid (if in release mode)

### OS enforcement smoke test fails

Check:
- Backend running as root
- `DISABLE_AUTH=true` environment variable set
- cgroup v2 available: `mount | grep cgroup2`
- BPF LSM enabled: `cat /sys/kernel/security/lsm | grep bpf`

---

## Contributing

See [AGENTS.md](AGENTS.md) for developer and coding-agent workflow guidance.

---

## License

[GPL-3.0](LICENSE)

---

## Acknowledgments

This project was inspired by [AgentSight](https://github.com/eunomia-bpf/agentsight), an open-source AI agent tracing tool developed by the eunomia-bpf team. AgentSight pioneered the use of eBPF for agent observability.

Agent eBPF Filter extends this foundation with:
- **Go + Vue.js** tech stack (vs Rust + Next.js)
- **Enforcement capabilities** (wrapper/cgroup/LSM blocking)
- **Security-first design** (TLS capture disabled by default)
- **Production-ready features** (MCP/OTLP/Prometheus integration)

See [docs/reference/agentsight-acknowledgment.md](docs/reference/agentsight-acknowledgment.md) for detailed acknowledgment.

---

## Support

- **Documentation:** [docs/](docs/)
- **Issues:** Use GitHub Issues for bug reports and feature requests
- **Discussions:** Use GitHub Discussions for questions and ideas

---

**Made with ❤️ for the AI agent ecosystem**
