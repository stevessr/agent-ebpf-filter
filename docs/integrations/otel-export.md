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

Configure OTLP from **Configuration -> Runtime Config** or `PUT /config/runtime`:

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
- `queueLen` / `queueCap`
- `enqueuedEvents` / `processedEvents`
- `activeRunSpans`
- `activeTaskSpans`
- `activeToolSpans`
- `maxRunSpans` / `maxTaskSpans` / `maxToolSpans`
- `evictedRunSpans` / `evictedTaskSpans` / `evictedToolSpans`
- `exportedSpans`
- `droppedEvents`
- `lastExportedAt`
- `lastError`

## Notes

- Prometheus metrics remain local at `GET /metrics`.
- OTLP export is best-effort and asynchronous. Disabled or unready exporters reject work before envelope cloning/attribute derivation; the 2048-item ingress queue is non-blocking and queue overflow increments `droppedEvents`.
- Active hierarchy state is bounded to 1024 run spans, 4096 task spans, and 8192 tool spans. When a limit is reached, the least-recently-used span is ended first; reclaiming a run or task also ends its active descendants, and the corresponding `evicted*Spans` counters increase.
- SDK span limits remain configurable through the standard `OTEL_*_LIMIT` environment variables, but unsafe unlimited/high values are capped: string attributes at 4096 characters, 128 span attributes/events/event attributes, and 32 links/link attributes. Stricter operator values are preserved. Dynamic event/span names are capped at 256 characters, and oversized identity components are SHA-256 compacted before hierarchy-key construction.
- Event handling never performs a synchronous collector flush. The SDK batch processor exports up to 256 spans with a two-second batch timeout; provider shutdown drains the remaining batch with a five-second deadline.
- The OTLP spans are **derived** from local runtime evidence. They preserve agent identifiers such as `agent_run_id`, `task_id`, `tool_call_id`, and the original `trace_id` as attributes even when the emitted OTel trace tree is synthesized locally.

---

## 相关导航

- [MCP、External API 与 OTLP](mcp-external-otlp.md)
- [事件管线](../backend/event-pipeline.md)
- [External API](external-api.md)
- [验证、测试与 Benchmark](../operations/verification-benchmark.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
