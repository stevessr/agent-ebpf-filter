# Agent Registration & Tracking Guide

This document explains how **agents** are observed by this project.

It is about runtime behavior, not contributor workflow. For repo-maintainer instructions, see [`AGENTS.md`](./AGENTS.md).

---

## 1. The tracking model

The project can surface agent activity through four complementary mechanisms:

1. **PID registration** — adapters register a process in the `agent_pids` BPF map.
2. **Tracked command names** — commands such as `git`, `python`, or `node` are tagged in `tracked_comms`.
3. **Tracked paths** — exact path strings are tagged in `tracked_paths`.
4. **Wrapper / native hook events** — user-space telemetry emitted by `agent-wrapper` and AI CLI hooks.

The eBPF program emits a kernel event when **any** of these match:

- registered PID
- tracked command name
- tracked path

---

## 2. What the eBPF filter actually checks

The kernel program stores:

- `agent_pids`: `u32 pid -> u32 tag_id`
- `tracked_comms`: `char[16] -> u32 tag_id`
- `tracked_paths`: `char[256] -> u32 tag_id`

The matching logic is:

```c
u32 tag_id = get_tag_id(pid, comm, path);
if (tag_id == 0) return 0;
```

So the event is ignored unless at least one map returns a tag.

### Important limitations

- `tracked_comms` is an **exact** command-name match.
- `tracked_paths` is an **exact** path match.
- PID registration is **per process**.

That means:

- `python`, `node`, `git`, `bun`, `npm` are good command keys.
- `/tmp/foo.txt` is a good path key.
- “watch everything under `/workspace` recursively” is **not** what the current code does.
- child processes are **not** auto-registered just because the parent registered itself.

---

## 3. Event types

Kernel-space event types currently mapped by the backend:

- `execve`
- `openat`
- `network_connect`
- `mkdir`
- `unlink`
- `ioctl`
- `network_bind`

Additional user-space event types:

- `wrapper_intercept`
- `native_hook`

---

## 4. Python adapter

Location:

- `adapters/python/agent_tracker.py`

Behavior:

- sends `POST /register` with the current PID,
- registers an `atexit` hook,
- sends `POST /unregister` on shutdown.

Example:

```python
from agent_tracker import AgentTracker

tracker = AgentTracker("http://127.0.0.1:8080")
tracker.start()

# from here on, matching syscalls from this process can be observed
with open("/tmp/agent-demo.txt", "w") as f:
    f.write("hello")
```

Notes:

- the helper currently registers with the backend default tag (`AI Agent`);
- if you need a custom tag today, extend the helper or call `/register` directly;
- subprocesses created later are not automatically added to `agent_pids`.

---

## 5. Node.js adapter

Location:

- `adapters/js/agentTracker.js`

Behavior:

- sends `POST /register`,
- installs `exit`, `SIGINT`, and `SIGTERM` handlers,
- attempts best-effort unregister on shutdown.

Example:

```javascript
const AgentTracker = require('./agentTracker');
const fs = require('fs');

const tracker = new AgentTracker('http://127.0.0.1:8080');
tracker.start();

fs.writeFileSync('/tmp/agent-demo-js.txt', 'hello');
```

Notes:

- registration is asynchronous;
- unregister on exit is best-effort;
- like the Python helper, this helper does not expose custom tag selection yet.

---

## 6. Tracking without adapters

You can still observe interesting agent activity without modifying the agent code:

### Track command names

Examples:

- `git`
- `node`
- `python`
- `bun`
- `npm`
- `cargo`

This is useful when an agent shells out to well-known tools.

### Track exact paths

Examples:

- `/etc/passwd`
- `/tmp/secret.txt`
- `/home/user/.ssh/config`

This is useful when you care about a specific file or directory entry string.

For directories, the current implementation is still **exact-match**, not recursive subtree matching.

### Use `agent-wrapper`

Commands run through the wrapper always produce a `wrapper_intercept` event and may be blocked, alerted, or rewritten by policy.

### Use native hooks

For supported AI CLIs, hook callbacks produce `native_hook` events even when there is no matching kernel event.

---

## 7. Register / unregister API

### `POST /register`

Payload:

```json
{
  "pid": 12345,
  "tag": "AI Agent"
}
```

`tag` is optional; the backend defaults to `AI Agent`.

### `POST /unregister`

Payload:

```json
{
  "pid": 12345
}
```

---

## 8. Recommended usage patterns

### Best for real agents

Use the adapter and register the long-lived process that actually performs the work.

### Best for shell-heavy workflows

Combine:

- adapter registration for the main agent process,
- tracked command names for subprocesses,
- wrapper rules for commands that should be blocked or rewritten.

### Best for AI CLI monitoring

Use the **Hooks** page to install native hooks for supported CLIs, then combine that with tracked commands / paths for kernel-level detail.

---

## 9. Related docs

- [`README.md`](./README.md) — project overview
- [`AGENTS.md`](./AGENTS.md) — contributor / coding-agent workflow
- [`wrapper/README.md`](./wrapper/README.md) — wrapper policy protocol
- [`backend/README.md`](./backend/README.md) — backend internals
