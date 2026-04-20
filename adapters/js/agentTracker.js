const http = require('http');

class AgentTracker {
  constructor(backendUrl = 'http://localhost:8080') {
    this.backendUrl = new URL(backendUrl);
    this.pid = process.pid;
    this.registered = false;
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
    const data = JSON.stringify({ pid: this.pid });
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
