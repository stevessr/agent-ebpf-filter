# External API Guide

This document describes the stable API surface intended for automation,
observability collectors, Kubernetes callers, and external control planes. The
Vue UI still uses the root routes directly, but external integrations should
prefer the versioned aliases under:

```text
http://<agent-ebpf-filter>:8080/api/v1
```

## Authentication

In release mode, every `/api/v1/**` route requires the runtime access token.
Send it using one of:

```bash
curl -H "X-API-KEY: $AGENT_API_KEY" http://127.0.0.1:8080/api/v1/health
curl -H "Authorization: Bearer $AGENT_API_KEY" http://127.0.0.1:8080/api/v1/health
curl "http://127.0.0.1:8080/api/v1/health?key=$AGENT_API_KEY"
```

The query-string form is mainly for WebSocket/SSE-style clients that cannot set
headers. Prefer headers for normal HTTP clients.

## Discovery

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/health` | Service health, runtime gates, eBPF bootstrap status, and collector counters. |
| `GET` | `/api/v1/openapi.json` | Machine-readable OpenAPI 3.0 summary for the stable external aliases. |

`/api/v1/health.features.domainForwardProxyEnabled` reports whether the
optional 80/443 Host/SNI data-plane forwarder is enabled. Manage that forwarder
through the authenticated root `/config/runtime` and inspect listener status at
`/system/domain-forward/status`.

## Event and graph APIs

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/events/recent?limit=100&type=execve` | Recent captured records with normalized envelopes. |
| `GET` | `/api/v1/events/graph?...` | Execution graph nodes and edges for retained events. |

Example:

```bash
curl -H "X-API-KEY: $AGENT_API_KEY" \
  "http://127.0.0.1:8080/api/v1/events/recent?limit=25"
```

## Network and enforcement APIs

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/network/flows?filter=process:curl%20dport:443` | Attributed TCP/UDP flows with process and agent context. |
| `GET` | `/api/v1/network/flows/{flowID}` | One enriched flow. |
| `GET` | `/api/v1/network/dns-cache` | DNS correlation cache. |
| `GET` | `/api/v1/network/interfaces` | Interface RX/TX counters and drops/errors. |
| `GET` | `/api/v1/network/export/jsonl` | Metadata-only flow JSONL export. |
| `GET` | `/api/v1/sandbox/cgroup/status` | cgroup/connect + sendmsg BPF status and active blocks. |
| `GET` | `/api/v1/sandbox/lsm/status` | BPF LSM status and active blocks. |

Policy mutation routes are deterministic control-plane operations and also
require `policyManagementEnabled` in `/config/runtime`.

| Method | Path | Body |
| --- | --- | --- |
| `POST` | `/api/v1/policies/network/block-ip` | `{ "ip": "203.0.113.10" }` |
| `POST` | `/api/v1/policies/network/unblock-ip` | `{ "ip": "203.0.113.10" }` |
| `POST` | `/api/v1/policies/network/block-port` | `{ "port": 4444 }` |
| `POST` | `/api/v1/policies/network/unblock-port` | `{ "port": 4444 }` |
| `POST` | `/api/v1/policies/network/block-pid` | `{ "pid": 1234 }` |
| `POST` | `/api/v1/policies/network/unblock-pid` | `{ "pid": 1234 }` |
| `POST` | `/api/v1/policies/lsm/block-exec-path` | `{ "path": "/usr/bin/nc" }` |
| `POST` | `/api/v1/policies/lsm/unblock-exec-path` | `{ "path": "/usr/bin/nc" }` |
| `POST` | `/api/v1/policies/lsm/block-exec-name` | `{ "name": "nc" }` |
| `POST` | `/api/v1/policies/lsm/unblock-exec-name` | `{ "name": "nc" }` |
| `POST` | `/api/v1/policies/lsm/block-file-name` | `{ "name": "id_rsa" }` |
| `POST` | `/api/v1/policies/lsm/unblock-file-name` | `{ "name": "id_rsa" }` |

## Agent registration aliases

External launchers can register through the stable aliases:

```bash
curl -X POST -H "X-API-KEY: $AGENT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"pid":1234,"tag":"AI Agent","agent_run_id":"run-1","task_id":"task-1","tool_call_id":"tool-1","cwd":"/workspace"}' \
  http://127.0.0.1:8080/api/v1/agents/register
```

Use `/api/v1/agents/unregister` with `{ "pid": 1234 }` when the launcher exits.

## Kubernetes callers

When deployed with `deploy/kubernetes/agent-ebpf-filter.yaml`, in-cluster
callers can use the ClusterIP Service:

```bash
curl -H "X-API-KEY: $AGENT_API_KEY" \
  http://agent-ebpf-filter.agent-ebpf-filter.svc.cluster.local:8080/api/v1/health
```

For node-specific debugging, port-forward a selected DaemonSet Pod instead of
using the Service, because a ClusterIP Service may load-balance to any node
agent.
