const http = require('http');
const fs = require('fs');
const path = require('path');

function firstEnv(...keys) {
  for (const key of keys) {
    const value = (process.env[key] || '').trim();
    if (value) {
      return value;
    }
  }
  return '';
}

function parseEnvNumber(...keys) {
  const raw = firstEnv(...keys);
  if (!raw) {
    return 0;
  }
  const parsed = Number(raw);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : 0;
}

function buildArgvDigest(parts) {
  const normalized = parts.map((part) => String(part || '').trim()).filter(Boolean);
  if (normalized.length === 0) {
    return '';
  }
  return require('crypto').createHash('sha256').update(normalized.join('\x00')).digest('hex');
}

function resolveBackendUrl(explicitUrl) {
  if (explicitUrl) {
    return explicitUrl;
  }

  if (process.env.AGENT_BACKEND_URL) {
    return process.env.AGENT_BACKEND_URL;
  }

  try {
    const portFile = path.resolve(__dirname, '../../backend/.port');
    if (fs.existsSync(portFile)) {
      const port = fs.readFileSync(portFile, 'utf-8').trim();
      if (/^\d+$/.test(port)) {
        return `http://127.0.0.1:${port}`;
      }
    }
  } catch (err) {
    // Fall through to the default URL below.
  }

  return 'http://127.0.0.1:8080';
}

class AgentTracker {
  constructor(backendUrl, context = {}) {
    this.backendUrl = new URL(resolveBackendUrl(backendUrl));
    this.pid = process.pid;
    this.registered = false;
    this.context = context;
  }

  start() {
    this._sendRequest('/register', (err, res) => {
      if (err) {
        console.error(`AgentTracker: Error connecting to backend - ${err.message}`);
      } else if (res.statusCode === 200) {
        console.log(`AgentTracker: successfully registered PID ${this.pid}`);
        this.registered = true;
        
        // Handle process exit to unregister
        process.on('exit', () => this.stopSync());
        process.on('SIGINT', () => { this.stopSync(); process.exit(); });
        process.on('SIGTERM', () => { this.stopSync(); process.exit(); });
      } else {
        console.error(`AgentTracker: failed to register PID ${this.pid}. Status: ${res.statusCode}`);
      }
    });
  }

  stopSync() {
    if (!this.registered) return;
    
    // Use synchronous request on exit to ensure it sends
    const data = JSON.stringify({ pid: this.pid });
    const options = {
      hostname: this.backendUrl.hostname,
      port: this.backendUrl.port,
      path: '/unregister',
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': data.length
      }
    };

    // For synchronous exit, child_process.spawnSync can be used, 
    // or just relying on standard async if not exiting immediately.
    // Here we just do a quick fire-and-forget
    const req = http.request(options);
    req.write(data);
    req.end();
    this.registered = false;
  }

  _sendRequest(path, callback) {
    const payload = {
      pid: this.pid,
      root_agent_pid: this.context.root_agent_pid || parseEnvNumber('AGENT_EBPF_ROOT_AGENT_PID', 'ROOT_AGENT_PID'),
      agent_run_id: this.context.agent_run_id || firstEnv('AGENT_EBPF_AGENT_RUN_ID', 'AGENT_RUN_ID'),
      conversation_id: this.context.conversation_id || firstEnv('AGENT_EBPF_CONVERSATION_ID', 'AGENT_CONVERSATION_ID'),
      turn_id: this.context.turn_id || firstEnv('AGENT_EBPF_TURN_ID', 'AGENT_TURN_ID'),
      tool_call_id: this.context.tool_call_id || firstEnv('AGENT_EBPF_TOOL_CALL_ID', 'AGENT_TOOL_CALL_ID'),
      tool_name: this.context.tool_name || firstEnv('AGENT_EBPF_TOOL_NAME', 'AGENT_TOOL_NAME'),
      trace_id: this.context.trace_id || firstEnv('AGENT_EBPF_TRACE_ID', 'TRACE_ID'),
      span_id: this.context.span_id || firstEnv('AGENT_EBPF_SPAN_ID', 'SPAN_ID'),
      decision: this.context.decision || firstEnv('AGENT_EBPF_DECISION', 'AGENT_DECISION'),
      risk_score: this.context.risk_score || parseEnvNumber('AGENT_EBPF_RISK_SCORE', 'AGENT_RISK_SCORE'),
      container_id: this.context.container_id || firstEnv('AGENT_EBPF_CONTAINER_ID', 'CONTAINER_ID'),
      argv_digest: this.context.argv_digest || buildArgvDigest([
        this.context.tool_name || firstEnv('AGENT_EBPF_TOOL_NAME', 'AGENT_TOOL_NAME'),
        this.context.tool_call_id || firstEnv('AGENT_EBPF_TOOL_CALL_ID', 'AGENT_TOOL_CALL_ID'),
        this.context.agent_run_id || firstEnv('AGENT_EBPF_AGENT_RUN_ID', 'AGENT_RUN_ID'),
      ]),
    };
    const data = JSON.stringify(payload);
    const options = {
      hostname: this.backendUrl.hostname,
      port: this.backendUrl.port,
      path: path,
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Content-Length': data.length
      }
    };

    const req = http.request(options, (res) => {
      callback(null, res);
    });

    req.on('error', (err) => {
      callback(err, null);
    });

    req.write(data);
    req.end();
  }
}

module.exports = AgentTracker;

// Test
if (require.main === module) {
  const tracker = new AgentTracker();
  tracker.start();
  const fs = require('fs');
  setTimeout(() => {
    fs.writeFileSync('/tmp/agent_test_js.txt', 'test');
    console.log("File written");
  }, 1000);
}
