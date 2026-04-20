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

### 1. Quick Start (Run Both)

```bash
make run
```
This will compile the eBPF code, build the Go backend, and start both the backend (using `sudo`) and the frontend concurrently.

### 2. Manual Start

#### Build and Run the Backend
```bash
make backend
make run-backend
```
*Note: sudo is required for attaching eBPF programs.*

#### Run the Frontend
```bash
make run-frontend
```
Navigate to `http://localhost:5173` to view the UI.

### 3. Run Agents (Adapters)

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
