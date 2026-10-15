# moejs JavaScript plugins (experimental)

The existing plugin registry continues to support eBPF / webhook / command.
Kind `javascript` is an opt-in **metadata transformation** extension, not
a substitute for the kernel/wrapper/cgroup/LSM enforcement boundary.

## Lifecycle

Use the existing authenticated `/plugins` routes. The existing
`policy_management` runtime gate is required for writes and execution.

Register (disabled first), then enable:

```sh
curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json'   http://127.0.0.1:8080/plugins   -d '{"id":"risk-annotation","name":"Risk annotation","kind":"javascript","enabled":false,"source":"export function onEvent(e){return {risk:e.risk,flagged:e.risk>=80}}"}'

curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json'   http://127.0.0.1:8080/plugins/risk-annotation/toggle -d '{"enabled":true}'

curl -X POST -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json'   http://127.0.0.1:8080/plugins/risk-annotation/invoke   -d '{"hook":"onEvent","payload":{"risk":91}}'
```

The input/output schema is JSON. Export `onEvent(event)` and/or
`onAlert(alert)`. These hooks currently execute **on explicit
invocation only**: automatic subscription to the eBPF event hot path is not
enabled. Results do not automatically alter allow/deny decisions.

- JavaScript source limit: 128 KiB; input/output: 32 KiB each.
- Per-invocation deadline: 100 ms (including source verification/cache lookup); JavaScript memory allocation budget: 8 MiB. HTTP returns 429 on executor overload and 504 on execution timeout.
- Fresh runtime for each invocation; compiled modules cached by SHA-256 (32-entry bound) and at most four invocations globally run concurrently. Overload is rejected without queueing.
- Enabling, disabling, replacing sources and deletion are synchronized with running hooks; disabling waits for in-flight hooks to finish or time out.
- Dynamic code (`eval` and Function) disabled, module imports unsupported.
- No injected host functions, filesystem, network, process or shell APIs.
- Scripts execute in the backend process. Resource limits are defense-in-depth,
  **not an OS-level security boundary**. Only install reviewed/trusted plugins.
- Malformed, disabled, or modified plugins fail without affecting eBPF policy.
- moejs is alpha; its API and VM security need ongoing audit before broader use.

When updating dependencies:

```sh
cd backend
go mod tidy
go test ./app/jsplugins ./app/handlers ./app
```

- Promise results are supported only when settled synchronously by moejs microtasks; pending or rejected Promises produce explicit errors. There is no Node.js/DOM event loop or async host I/O.
