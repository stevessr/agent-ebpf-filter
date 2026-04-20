# Agent eBPF Filter Framework

A specialized eBPF-based observation framework designed to trace application paths (e.g., file opens, executions) of AI Agents written in Python or Node.js.

## Architecture

- **Frontend:** Vue3 + TypeScript + Ant Design Vue
- **Backend:** Go (using Gin + Gorilla WebSocket)
- **eBPF:** C-based `sys_enter_openat` and `sys_enter_execve` tracepoints
- **Adapters:** SDKs for Python and JavaScript to register their PIDs

## Prerequisites

- Linux kernel with eBPF support and BTF enabled (typical on modern distros)
- Clang/LLVM for eBPF compilation
- Go 1.21+
- Bun
- Python 3.8+ (for Python agents)
- Root/sudo access to run the Go backend (eBPF requires privileges)

## Usage

### 1. Development Mode

```bash
make dev
```
Starts the Go backend and the frontend Vite development server (`http://localhost:5173`) concurrently. Changes to the frontend will be reflected in real-time.

### 2. Production Mode

```bash
make run
```
Builds the frontend production assets, compiles the eBPF code, and starts the Go server which then serves the frontend from `http://localhost:8080`.

### 3. Manual Control

#### Build and Run Backend Only
```bash
make backend
make run-backend
```

#### Run Frontend Dev Server Only
```bash
make run-frontend
```

### 3. CLI Interceptor (Wrapper)

The framework includes a specialized `agent-wrapper` that acts as a secure shim for sensitive commands.

```bash
make wrapper
# Usage: ./agent-wrapper <command> [args...]
./agent-wrapper rm -rf /important/data
```

**Features:**
- **UDS Integration**: Communicates with the backend via `/tmp/agent-ebpf.sock` for zero-latency policy checks.
- **Security Policies**:
  - **BLOCK**: Completely prevent command execution.
  - **ALERT**: Allow command but log a high-priority security event.
  - **REWRITE**: Dynamically modify arguments (e.g., automatically add `-i` to `rm`).
- **Real-time Monitoring**: Every command run through the wrapper is intercepted and shown in the "Wrapper" tag on the dashboard.

### 4. Run Agents (Adapters)

**Python Agent:**
```bash
cd adapters/python
pip install requests
python agent_tracker.py
```

**JavaScript Agent:**
```bash
cd adapters/js
node agentTracker.js
```

Once an agent starts, its PID is registered with the eBPF map. Any file reads/opens (`sys_openat`) or command executions (`sys_execve`) by that PID will be intercepted by the eBPF program, sent to the ringbuffer, consumed by the Go backend, and streamed via WebSocket to the Vue frontend.
