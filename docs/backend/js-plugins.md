# moejs JavaScript plugins (experimental)

The existing plugin registry continues to support eBPF / webhook / command.
Kind `javascript` is an opt-in **metadata transformation** extension, not
a substitute for the kernel/wrapper/cgroup/LSM enforcement boundary.

## Lifecycle

Use the existing authenticated `/plugins` routes. The existing
`policy_management` runtime gate is required for writes and execution.

Build and start an independently restricted worker before enabling scripts.
The backend deliberately **rejects JavaScript invocation if the socket is not configured**.

```sh
cd backend
go build -o /usr/local/libexec/agent-ebpf-jsplugin-worker ./cmd/jsplugin-worker
# Linux: deploy systemd unit deploy/systemd/agent-ebpf-jsplugin-worker.service,
# enable agent-ebpf-jsplugin-worker.service and configure the backend with:
export AGENT_EBPF_JS_WORKER_SOCKET=/run/agent-ebpf-jsplugin/worker.sock
```

The supplied systemd example starts a non-root DynamicUser worker with a private
network namespace, restricted filesystem access and CPU/memory/task quotas.
Its socket is mode 0600 under a 0700 service directory: the backend must have
explicit permission to connect (typically the root collector). Do not loosen
these permissions just to make a desktop build work; for development run a
worker under the same user and a private 0700 runtime directory.

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
- Fresh runtime for each invocation **inside the worker process**; only the worker caches immutable modules by SHA-256 (32-entry bound). Both client and worker cap concurrent invocations at four; overload is rejected without queueing.
- Enabling, disabling, replacing sources and deletion are synchronized with running hooks; disabling waits for in-flight hooks to finish or time out.
- Dynamic code (`eval` and Function) disabled, module imports unsupported.
- No injected host functions, filesystem, network, process or shell APIs.
- Hook execution **and JavaScript syntax validation** run in the separate worker process via bounded, versioned Unix-socket JSON IPC. The backend only enforces size/metadata checks and verifies persisted source hashes. Disabled drafts may be stored even if malformed; enabling invokes remote validation. Isolation depends on the operator deploying the worker under a separate unprivileged OS identity and the specified service sandbox; a socket alone is not a sandbox. The runtime remains experimental and needs independent security review.
- Malformed, disabled, or modified plugins fail without affecting eBPF policy.
- moejs is alpha; its API and VM security need ongoing audit before broader use.

When updating dependencies:

```sh
cd backend
go mod tidy
go test ./app/jsplugins ./app/handlers ./app
```

- Promise results are supported only when settled synchronously by moejs microtasks; pending or rejected Promises produce explicit errors. There is no Node.js/DOM event loop or async host I/O.

The worker refuses to start as root. Worker errors or unavailable sockets
fail closed: no automatic in-process fallback. The service does not execute
scripts on the eBPF enforcement path or grant host functions to JavaScript.

## Isolation verification (GitHub Actions)

The `moejs JavaScript plugins` workflow supports `workflow_dispatch` and pull-request triggers. It builds the real worker executable, exercises a non-root worker in a separate process, asserts graceful SIGTERM shutdown and socket cleanup, rejects a worker started by root, and verifies the restricted systemd unit. IPC tests additionally cover malformed frames, concurrent requests, client-disconnect cancellation, and VM deadlines. The backend must never silently fall back to evaluating JavaScript itself.

The worker rejects group/world-accessible, symlinked, or foreign-owned socket directories. Prepare a worker-owned `0700` runtime directory (as in the systemd service); the process sets `umask 077` before binding the Unix socket. The JavaScript interpreter does not require executable writable memory, so the unit enables `MemoryDenyWriteExecute=yes`. Protocol framing budgets account for JSON escape expansion while still enforcing the original 128 KiB source / 32 KiB payload limits.

On restart, the Worker takes an advisory lock in its private runtime directory. If a previous crash left a Unix socket inode, it only removes an owned socket after a connection attempt reports `ECONNREFUSED`. A live socket, foreign-owned socket, symlink, or regular file is never overwritten. SIGKILL restart/recovery is tested with the actual worker executable in GitHub Actions.

## Live systemd integration test

GitHub Actions also starts the provided service as a real systemd unit on the Ubuntu runner, probes it from a root process (the collector's access model), asserts the worker UID is non-root with zero effective Linux capabilities and a distinct network namespace, checks the 0700 runtime directory and 0600 socket, and exercises real JS validation/invocation and malformed-source refusal. It stops and removes the temporary unit at the end. This complements but does not replace production-host deployment testing and security auditing.
