# Python adapter

The Python adapter is a tiny helper that registers the current process ID with the backend so eBPF events from that process can be tagged and surfaced in the UI.

## Files

- `agent_tracker.py` — adapter implementation
- `tracker_pb2.py` — generated protobuf bindings

## Install

### Minimal

If you only want to use the adapter helper:

```bash
pip install requests
```

### Repo-managed environment

```bash
uv sync
```

> The checked-in `pyproject.toml` currently targets Python `>=3.13`.

## Usage

```python
from agent_tracker import AgentTracker

tracker = AgentTracker("http://127.0.0.1:8080")
tracker.start()

# do work here
with open("/tmp/python-agent-demo.txt", "w") as f:
    f.write("hello")
```

When the process exits normally, the adapter attempts to unregister through `atexit`.

## Behavior

- `start()` sends `POST /register` with `{"pid": os.getpid()}`
- `stop()` sends `POST /unregister`
- the backend defaults the tag to `AI Agent`
- if no URL is passed, the helper resolves backend address in this order:
  - explicit constructor argument
  - `AGENT_BACKEND_URL`
  - repo-local `backend/.port`
  - fallback `http://127.0.0.1:8080`
- in release mode, set `AGENT_API_KEY` or `AGENT_EBPF_ACCESS_TOKEN` so `/register` and `/unregister` can authenticate
- the helper can also forward optional run / trace metadata from the constructor `context` dict or env vars such as `AGENT_EBPF_AGENT_RUN_ID`, `AGENT_EBPF_TASK_ID`, `AGENT_EBPF_TOOL_CALL_ID`, `AGENT_EBPF_TRACE_ID`, and `AGENT_EBPF_CWD`

## Limitations

- The helper does **not** currently expose a custom tag parameter.
- Descendant processes now inherit the registered context automatically.
- If the backend is unavailable during shutdown, unregister is best-effort.

## Smoke test

```bash
python agent_tracker.py
```

The example in `__main__` registers the current PID, writes `/tmp/agent_test.txt`, waits briefly, and exits.
