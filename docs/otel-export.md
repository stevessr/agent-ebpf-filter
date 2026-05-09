# OTLP Export

This project can export normalized runtime evidence to an OpenTelemetry collector over **OTLP HTTP**.

## What gets exported

The exporter consumes the same versioned `EventEnvelope` objects used by:

- `GET /events/recent`
- `GET /ws/envelopes`
- MCP `tail_events`

It currently derives these spans:

- `agent.run`
- `codex.task`
- `tool.call`
- `llm.call` (heuristic when the tool name looks like an LLM call)
- `pr.review` (heuristic when the tool / task looks like review work)
- `mcp.call`

It also emits child spans or span events for:

- process lifecycle / exec / wait / exit
- file access operations
- network activity
- wrapper intercepts
- native hook callbacks
- policy / semantic alerts

## Runtime settings

Configure OTLP from **Configuration -> Runtime** or `PUT /config/runtime`:

```json
{
  "otlpEnabled": true,
  "otlpEndpoint": "http://127.0.0.1:4318",
  "otlpServiceName": "agent-ebpf-filter",
  "otlpHeaders": {
    "Authorization": "Bearer <token>"
  }
}
```

Accepted endpoint forms:

- `http://host:4318`
- `https://collector.example.com`
- `https://collector.example.com/custom/path`

If no path is supplied, the exporter defaults to `/v1/traces`.

## Health endpoint

Check exporter status at:

- `GET /system/otel-health`

The response includes:

- `enabled` / `ready`
- `endpoint`
- `serviceName`
- `queueLen`
- `activeRunSpans`
- `activeTaskSpans`
- `activeToolSpans`
- `exportedSpans`
- `droppedEvents`
- `lastExportedAt`
- `lastError`

## Notes

- Prometheus metrics remain local at `GET /metrics`.
- OTLP export is best-effort and asynchronous; exporter queue overflow increments `droppedEvents`.
- The OTLP spans are **derived** from local runtime evidence. They preserve agent identifiers such as `agent_run_id`, `task_id`, `tool_call_id`, and the original `trace_id` as attributes even when the emitted OTel trace tree is synthesized locally.
