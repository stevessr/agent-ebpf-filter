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

## Limitations

- Registration is asynchronous.
- Exit-time unregister is best-effort.
- The helper currently uses the backend default tag (`AI Agent`) and does not expose custom tags.
- Registration is per process; spawned child processes are not auto-registered.

## Smoke test

```bash
node agentTracker.js
```

The built-in example writes `/tmp/agent_test_js.txt` after registration.
