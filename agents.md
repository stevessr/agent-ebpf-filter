# Agents Specialization: Python & JS Support

This framework is specialized for AI Agents. It provides SDK-like adapters to allow agents to "opt-in" to eBPF monitoring by registering their Process IDs (PIDs) with the backend.

## Why this approach?
Tracing *every* process in the system generates massive noise. By requiring agents to register themselves, we ensure:
1. **Zero Noise**: Only the agent's file/process activity is captured.
2. **Contextual Observation**: We know exactly which agent produced which system call.
3. **Efficiency**: eBPF map lookups are extremely fast ($O(1)$), minimizing overhead for the agent.

## Adapters Usage

### Python Adapter
The Python adapter uses `requests` to notify the Go backend and `atexit` to ensure clean unregistration.

```python
from agent_tracker import AgentTracker

tracker = AgentTracker(backend_url="http://localhost:8080")
tracker.start() # Registers current PID

# Any file/exec calls from here on are traced by eBPF
with open("agent_workspace/config.yaml", "r") as f:
    pass
```

### Node.js Adapter
The JS adapter uses native `http` and handles process signals (`SIGINT`, `SIGTERM`) for unregistration.

```javascript
const AgentTracker = require('./agentTracker');

const tracker = new AgentTracker('http://localhost:8080');
tracker.start(); // Registers current process.pid

// eBPF will now capture fs calls
const fs = require('fs');
fs.readFileSync('./secrets.env');
```

## How eBPF Filters Agents
The Go backend maintains a `BPF_MAP_TYPE_HASH` called `agent_pids`. 
When `tracker.start()` is called, the Go server performs:
```go
objs.AgentPids.Put(&pid, &val)
```
The eBPF kernel program then checks this map on every syscall entry:
```c
u8 *is_agent = bpf_map_lookup_elem(&agent_pids, &pid);
if (!is_agent) return 0; // Ignore non-agent processes
```
