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

`/api/v1/health.featureManifest` mirrors `GET /system/features` and reports each
optional module's `compiledIn` and `runtimeEnabled` state. Use it to distinguish
a build that omitted a feature from a build where the feature exists but its
runtime gate is disabled.

`/api/v1/health.features.kernelRiskFeedbackEnabled` and the nested
`collector.kernelRisk*` counters show whether the optional closed loop from
zero-copy decoded kernel events into cgroup / BPF LSM policy maps is enabled and
whether recent feedback actions were applied, dropped, or failed.

## Event and graph APIs

| Method | Path | Purpose |
| --- | --- | --- |
| `GET` | `/api/v1/events/recent?limit=100&type=execve` | Recent captured records with normalized envelopes. Supports `type`, `event_type`, `source`, `pid`, `comm`, `trace_id`, `span_id`, `since`, `until`, and `redaction_state`. |
| `GET` | `/api/v1/events/graph?...` | Execution graph nodes and edges for retained events. |
| `GET` | `/api/v1/agentsight/events?format=json\|array\|jsonl` | AgentSight-compatible merged export of retained EventEnvelope records, uploaded AgentSight traces, and TLS capture history. Supports `limit`, `include_tls`, `type`, `event_type`, semantic `source` (`file`, `process`, `http_parser`, `ssl`, `stdio`, `system`, `policy`), `pid`, `comm`, `trace_id`, `span_id`, `since`, `until`, `redaction_state`, and `filter`. |
| `POST` | `/api/v1/agentsight/events` | Import AgentSight JSON, JSON arrays, `{ "events": [...] }`, or JSONL text into the in-memory AgentSight compatibility store. |
| `GET` | `/api/v1/agentsight/runners` | Logical AgentSight runner status for process/eBPF, TLS, stdio, system, wrapper/policy/OTel, and uploaded traces. |
| `GET` | `/api/v1/agentsight/events/stats` | AgentSight storage statistics by source, event type, logical runner, and command. |
| `GET` | `/api/v1/agentsight/events/runners/{id}/stats` | Same statistics filtered to one logical runner (`process`, `tls`, `stdio`, `system`, `agent`, or `uploaded`). |
| `POST` | `/api/v1/agentsight/events/query` | Advanced AgentSight query with JSON body filters such as `sources`, `event_types`, `pids`, `runner`, `since`, `until`, and `filter`. |
| `GET` | `/api/v1/agentsight/events/stream` / `/api/v1/agentsight/stream/merged` | Server-sent AgentSight-compatible stream for clients that cannot use WebSockets. |

Example:

```bash
curl -H "X-API-KEY: $AGENT_API_KEY" \
  "http://127.0.0.1:8080/api/v1/events/recent?limit=25"

curl -H "X-API-KEY: $AGENT_API_KEY" \
  "http://127.0.0.1:8080/api/v1/events/recent?limit=50&source=ebpf_ringbuf&event_type=TLS_PLAINTEXT&redaction_state=sanitized"

curl -H "X-API-KEY: $AGENT_API_KEY" \
  "http://127.0.0.1:8080/api/v1/agentsight/events?format=jsonl&limit=200&source=http_parser"

curl -X POST -H "X-API-KEY: $AGENT_API_KEY" \
  -H "Content-Type: application/x-ndjson" \
  --data-binary @agentsight-trace.jsonl \
  http://127.0.0.1:8080/api/v1/agentsight/events

curl -H "X-API-KEY: $AGENT_API_KEY" \
  http://127.0.0.1:8080/api/v1/agentsight/runners

curl -X POST -H "X-API-KEY: $AGENT_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"runner":"tls","event_types":["HTTP_MESSAGE","SSE_MESSAGE"],"filter":"anthropic","limit":100}' \
  http://127.0.0.1:8080/api/v1/agentsight/events/query
```

For compatibility with the original AgentSight frontend in
`docs/ref/agentsight`, the authenticated root aliases `GET /api/events` and
`GET /api/events/stream`, `GET /api/runners`, and `GET /api/stream/merged`
return the same semantic AgentSight event shape/status, and `POST /api/events`
plus `POST /api/events/query` accept the same import/query payloads. The plain
`/api/events` endpoint returns JSONL text.

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
require `policyManagementEnabled` in `/config/runtime`. The optional
kernel-risk feedback worker uses the same map mutation helpers after a scored
event crosses `kernelRiskFeedback.minRiskScore`; it additionally requires
`kernelRiskFeedback.enabled`, respects `enforceNetwork`, `enforceFileNames`,
`enforceExec`, and rate-limits with `maxActionsPerMinute`.

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

---

## 相关导航

- [路由与 API](../backend/routes-api.md)
- [MCP、External API 与 OTLP](mcp-external-otlp.md)
- [Kubernetes](../operations/kubernetes.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [部署与安装](../operations/deployment.md)

