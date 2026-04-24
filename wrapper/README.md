# agent-wrapper

`agent-wrapper` is a small command shim that asks the backend whether a command should run, be blocked, be rewritten, or be allowed with an alert.

## Behavior

Input:

```bash
./agent-wrapper <command> [args...]
```

Runtime flow:

1. sanitize command arguments,
2. connect to `/tmp/agent-ebpf.sock`,
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
