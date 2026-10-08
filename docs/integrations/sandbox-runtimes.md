# Sandbox runtime integration

Agent eBPF treats sandbox runtimes as an **attribution boundary**, not as a
reason to overstate what host eBPF can see.

## Supported runtimes

The host-side runtime detector recognizes:

- gVisor / `runsc`
- Kata Containers (`containerd-shim-kata-v2`, `kata-runtime`)
- Firecracker / jailer / the AWS Firecracker containerd shim
- Bubblewrap
- nsjail
- OCI runtimes such as runc, crun, youki and `containerd-shim-runc-v2`

It correlates `/proc/<pid>/comm`, `/proc/<pid>/cmdline` and
`/proc/<pid>/cgroup`, then reuses the existing `container_id` process
context. Registration also fills an empty `container_id` when a stable ID is
discoverable from the host-visible PID.

Read-only diagnostics:

```bash
curl -H "X-API-KEY: $TOKEN" \
  http://127.0.0.1:8080/sandbox/runtime/status

curl -H "X-API-KEY: $TOKEN" \
  "http://127.0.0.1:8080/sandbox/runtime/detect?pid=1234"

curl -H "X-API-KEY: $TOKEN" \
  "http://127.0.0.1:8080/sandbox/runtime/active?limit=128"
```

The same endpoints are exposed below `/api/v1/sandbox/runtime/*`.

## gVisor boundary

gVisor is a special case. Application syscalls are handled by the Sentry
user-space kernel, so a host tracepoint does **not** represent a one-to-one
stream of guest syscalls. Agent eBPF therefore separates:

1. **Host boundary attribution** — identify `runsc`/Sentry/Gofer processes,
   their cgroups and container IDs, and correlate those host-visible operations
   with existing Agent eBPF process/container context.
2. **Host enforcement** — continue using the existing cgroup eBPF and BPF LSM
   controls. Exact coverage depends on gVisor's platform and networking mode.
3. **Guest telemetry** — use gVisor's own tracing channel when guest-level
   syscall/trace-point detail is required.

Current gVisor exposes `--strace`, `--strace-syscalls` and
`--strace-event`, but structured production-oriented telemetry is better
matched by gVisor's seccheck trace sessions. The `remote` sink connects to a
Unix-domain socket and can include context such as container ID. A future
`gvisor-seccheck` receiver can feed those records into the same event
normalization layer without changing the host eBPF collector.

A minimal gVisor trace-session shape is:

```json
{
  "trace_session": {
    "name": "Default",
    "points": [
      {
        "name": "syscall/execve/enter",
        "context_fields": ["container_id", "group_id", "process_name"]
      }
    ],
    "sinks": [
      {
        "name": "remote",
        "config": {
          "endpoint": "/run/agent-ebpf/gvisor-events.sock"
        }
      }
    ]
  }
}
```

The remote sink is **not enabled automatically** by Agent eBPF in this change:
the gVisor stream is a separate trust/input boundary and needs authenticated
framing, versioned protobuf decoding, backpressure/drop accounting and explicit
redaction before it is merged with kernel evidence.

## PID namespaces and VMs

`/sandbox/runtime/detect` accepts a **host-visible PID**. A PID reported from
inside a PID namespace may not name the same task in host `/proc`. For agents
running inside a sandbox, prefer propagating the existing
`AGENT_EBPF_CONTAINER_ID` / `container_id` metadata from the launcher.

Kata and Firecracker place a VM boundary between host and workload. The host
adapter can attribute the shim/VMM, but guest syscall observability and
guest-side policy require a collector inside the VM or a runtime-specific
telemetry channel.

## Security semantics

Runtime detection is read-only and best-effort. It does not weaken the existing
authentication or `policyManagementEnabled` gates. Runtime names and cgroup
paths are attribution metadata, not proof that a workload is isolated, and
detection failure never silently widens a policy scope.
