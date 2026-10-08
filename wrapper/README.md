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
3. send a length-prefixed `pb.WrapperRequest`,
4. receive a length-prefixed `pb.WrapperResponse`,
5. apply the decision,
6. `exec()` the final command.

## Optional lightweight sandbox (Linux)

`agent-wrapper` can now execute an approved command inside a **Bubblewrap
namespace/mount sandbox**, rather than directly on the host. This feature is
explicitly opt-in. Existing wrapper invocations are unchanged.

```bash
# Read-only project tree; no network and a private /tmp.
agent-wrapper --sandbox=readonly --cwd "$PWD" -- /usr/bin/python3 script.py

# Permit writes only inside the chosen project workspace.
agent-wrapper --sandbox=workspace --sandbox-workspace "$PWD" \
  --cwd "$PWD" -- /usr/bin/python3 script.py

# Deliberately grant outbound networking and one additional read-only path.
agent-wrapper --sandbox=workspace --sandbox-network \
  --sandbox-ro-bind /etc/ssl --cwd "$PWD" -- /usr/bin/python3 script.py
```

Requirements: install `bwrap` (Bubblewrap) on Linux. Unprivileged user
namespaces must be supported. Sandboxed commands **must not run as host root**;
if the wrapper is started as root, specify a non-root `--user` to drop to.
If Bubblewrap, namespace creation, mounts or policy response fail, sandbox mode
refuses to execute; there is **no unsandboxed fallback**. Without `--sandbox`,
the existing backend-unavailable behavior is retained for compatibility.

### Isolation profile

- A fresh mount namespace with **no bind of host `/`**. Standard executables
  and libraries under `/usr`, `/bin`, `/sbin`, `/lib`, `/lib64` are
  read-only. A small set of standard files under `/etc` is read-only.
- Workspace defaults to the effective command cwd. `readonly` mounts it
  read-only; `workspace` makes only that tree writable, plus private `/tmp`.
  A working directory outside the declared workspace is rejected.
- PID, IPC, UTS and user namespace isolation, dropped capabilities, detached
  terminal session and lifecycle-bound child processes. Network is isolated by
  default. `--sandbox-network` **shares host networking** (not a filtered
  network); existing host eBPF network policies remain a separate control.
- A cleared environment: only a safe PATH/LANG, temporary HOME/cache paths,
  runtime marker and a sandbox correlation ID are forwarded. Host API tokens,
  `LD_PRELOAD`, SSH agents, and arbitrary environment values are **not**
  forwarded. Do not pass secrets through command arguments.
- `--sandbox-ro-bind /absolute/path` is repeatable for additional read-only
  dependencies (for example a tool installation or certificates). It explicitly
  expands visibility; exposing host `/` is refused.
- The wrapper generates a unique `wrapper-bwrap-...` **logical** sandbox ID
  (unless an upstream `AGENT_EBPF_CONTAINER_ID` was supplied) and forwards it
  to the child, allowing registration and wrapper events to correlate. This is
  not an OCI container ID.

This is lightweight filesystem/namespace isolation, **not** a complete gVisor
Sentry, VM, seccomp syscall allowlist, or untrusted-code security boundary.
Access to files made visible by explicit binds is still governed by host file
permissions; network access when granted can reach the host network. Existing
Agent eBPF cgroup/BPF-LSM policies can be layered independently.

### Examples and limits

The profile intentionally hides home directories, credentials, and most of
`/etc`. Network-capable AI CLIs requiring authentication or dynamic system
libraries may need a more explicit deployment profile; an accidental missing
resource must not cause an automatic wider mount or policy bypass. For a
production sandbox with stronger syscall isolation, run the workload under
gVisor/Kata and let Agent eBPF provide host-boundary monitoring; see
[Sandbox runtime integration](../docs/integrations/sandbox-runtimes.md).

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
- Each protobuf message uses a 4-byte big-endian length prefix and a 4 MiB payload limit, so requests are not truncated by Unix stream-socket packet boundaries.
- The wrapper applies a 2-second exchange deadline; the backend applies per-read and per-write deadlines.
- The backend path to the wrapper can be overridden with `AGENT_WRAPPER_PATH`.
- If present, the wrapper forwards runtime context from environment variables such as `AGENT_EBPF_AGENT_RUN_ID`, `AGENT_EBPF_TASK_ID`, `AGENT_EBPF_TOOL_CALL_ID`, `AGENT_EBPF_TRACE_ID`, `AGENT_EBPF_SPAN_ID`, `AGENT_EBPF_ROOT_AGENT_PID`, and `AGENT_EBPF_CWD`.
- The socket is expected to be owned by root or the original invoking user; arbitrary local users should no longer be able to connect.
- Invalid `--cwd`, unknown users, or failed group/GID/UID transitions abort the launch instead of executing with the caller's current privileges.
- A wrapper process running as root must receive an explicit `--user` value; use `--user root` only when root execution is intentional.

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

## DeepSeek Harness exact argv semantics

`dsh-exec` is an opt-in provider hook independent of the existing native Cordis session/tool metadata hook. Provider-spawned child commands preserve every argument (including blank and whitespace-significant values) through `--dsh-exec --verbatim --`. Other CLI commands retain their historical normalization. Wrapper argument digests use NUL-delimited command/argv boundaries to avoid collapsing distinct commands.
