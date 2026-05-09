# agent-wrapper

`agent-wrapper` is a small command shim that asks the backend whether a command should run, be blocked, be rewritten, or be allowed with an alert.

## Behavior

Input:

```bash
./agent-wrapper <command> [args...]
```

Runtime flow:

1. sanitize command arguments,
2. connect to `/tmp/agent-ebpf.sock` (restricted `0600`, peer-credential checked),
3. send `pb.WrapperRequest`,
4. receive `pb.WrapperResponse`,
5. apply the decision,
6. `exec()` the final command.

## Backend decisions

- `ALLOW` — run command as-is
- `BLOCK` — print message and exit non-zero
- `ALERT` — print warning, then run
- `REWRITE` — replace command + args with `rewritten_args`

## Why it exists

The wrapper gives you a policy point for commands that may not be captured well enough by PID-only tracking:

- destructive filesystem commands,
- package managers,
- network tools,
- AI CLIs you want to route through a single entrypoint.

It also generates a `wrapper_intercept` event for the dashboard.

## Notes

- The current implementation prints debug output to stdout.
- If the backend socket is unavailable, the wrapper falls back to executing the original command.
- The backend path to the wrapper can be overridden with `AGENT_WRAPPER_PATH`.
- If present, the wrapper forwards runtime context from environment variables such as `AGENT_EBPF_AGENT_RUN_ID`, `AGENT_EBPF_TASK_ID`, `AGENT_EBPF_TOOL_CALL_ID`, `AGENT_EBPF_TRACE_ID`, `AGENT_EBPF_SPAN_ID`, `AGENT_EBPF_ROOT_AGENT_PID`, and `AGENT_EBPF_CWD`.
- The socket is expected to be owned by root or the original invoking user; arbitrary local users should no longer be able to connect.

## Build

From the repo root:

```bash
make wrapper
```

Or directly:

```bash
cd wrapper
go build -o ../agent-wrapper
```
