# Node.js adapter

The Node.js adapter registers the current `process.pid` with the backend so matching eBPF events from that process can be tagged and shown in the dashboard.

## Files

- `agentTracker.js` — adapter implementation
- `tracker_pb.js` — generated protobuf bindings used elsewhere in the repo

## Usage

```javascript
const AgentTracker = require('./agentTracker');
const fs = require('fs');

const tracker = new AgentTracker('http://127.0.0.1:8080');
tracker.start();

fs.writeFileSync('/tmp/node-agent-demo.txt', 'hello');
```

## Behavior

- `start()` sends `POST /register`
- on success it installs:
  - `exit`
  - `SIGINT`
  - `SIGTERM`
- shutdown uses a best-effort unregister request
- if no URL is passed, the helper resolves backend address in this order:
  - explicit constructor argument
  - `AGENT_BACKEND_URL`
  - repo-local `backend/.port`
  - fallback `http://127.0.0.1:8080`
- in release mode, set `AGENT_API_KEY` or `AGENT_EBPF_ACCESS_TOKEN` so `/register` and `/unregister` can authenticate
- the helper can also forward optional run / trace metadata from constructor `context` or env vars such as `AGENT_EBPF_AGENT_RUN_ID`, `AGENT_EBPF_TASK_ID`, `AGENT_EBPF_TOOL_CALL_ID`, `AGENT_EBPF_TRACE_ID`, and `AGENT_EBPF_CWD`

## Limitations

- Registration is asynchronous.
- Exit-time unregister is best-effort.
- The helper currently uses the backend default tag (`AI Agent`) and does not expose custom tags.
- Descendant processes now inherit the registered context automatically.

## Smoke test

```bash
node agentTracker.js
```

The built-in example writes `/tmp/agent_test_js.txt` after registration.
