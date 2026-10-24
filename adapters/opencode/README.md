# OpenCode execution bridge (opt-in, v1 API)

This bridge calls an explicitly configured local policy command **before** OpenCode tools run. It implements `tool.execute.before` and `tool.execute.after`, tested with OpenCode 1.19.3. It is a cooperative CLI integration, not a kernel boundary or a sandbox for a malicious agent. It does not install hooks globally, load a model, or change the current backend policy.

Create a project-local `.opencode/plugins/agent-filter.js` entry (the **`.js` suffix matters** for auto-discovery in the tested release):

```js
export { AgentFilter as default } from 'file:///absolute/repo/adapters/opencode/agent-filter.mjs';
```

Only this one-export loader belongs in the plugin directory. `agent-filter.mjs` also exports test helpers, so directly auto-loading every export as a plugin is unsupported.

Set environment variables for this process:

- `AGENT_OPENCODE_DECISION_ARGV`: JSON array of absolute executable + literal arguments; executed directly, never passed through a shell.
- `AGENT_OPENCODE_MODE`: explicitly `observe` or `enforce`.
- `AGENT_OPENCODE_AUDIT`: supervisor-owned JSONL path outside the agent's editable workspace; keep its parent private. The plugin writes args hashes and decision metadata, not command/file contents. Hashing is not anonymity.

The decision program reads one JSON object from stdin: `tool`, `args`, `session_id`, `call_id`. It must exit 0 and return exactly one JSON value with `action: ALLOW | ALERT | BLOCK`. `ALERT` permits the action; `BLOCK` throws before execution only in enforce mode. A shadow model result cannot be used as an enforcement result. Invalid response, failure or a 2-second timeout becomes a transport failure; enforce aborts the call, observe records it and continues. JSON input and output are limited to 256 KiB. No REWRITE support is implied.

If loading the plugin fails, OpenCode itself may continue without it. **Checking the configured file exists is insufficient.** Run a supervised canary before accepting a session and reconcile every native call ID with a before-audit entry. The research harness reports unobserved calls, invalid runs and control effects, rather than calling zero audit events a 0% false-positive result. A live production supervisor that terminates uncovered sessions is not implemented here.

The callback and audit files must remain outside the agent's authority. Same-UID filesystem separation is not a security boundary. The plugin cannot prevent a shell command or another tool from performing arbitrary nested syscalls or bypassing literal-string policies. Use OS isolation and an authenticated backend policy transport for hostile workloads.

See [disposable-fixture evaluation](../../scripts/research/opencode-interception-20261018/README.md). Its literal `protected-canary` policy is **only an integration control**. Do not deploy it as a security classifier. The existing HTTP model remains shadow-only and its known cross-source failure is unchanged.

References: https://opencode.ai/docs/plugins/ ; https://opencode.ai/docs/cli/ . Actual compatibility is pinned to the executed version, not a promise about later OpenCode v2 APIs.
