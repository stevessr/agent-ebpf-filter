# MCP、External API 与 OTLP

Agent eBPF Filter 对外提供 MCP、External API、Prometheus 和 OTLP。

## MCP

入口：

```text
/mcp
```

认证：

- `X-API-KEY`
- `Authorization: Bearer`
- `?key=`

典型 tools：

- tail events；
- config snapshot；
- add tracked command/path；
- query events；
- get network flows；
- system health；
- block network destination；
- block process cgroup；
- block file access。

## External API

兼容入口：

- `/api/**`
- `/api/v1/**`
- `/api/v1/agentsight/**`

修改 external API 时应同步：

- `docs/external-api.md`；
- route auth；
- backward compatibility；
- frontend / external clients。

## OTLP

OTLP export 用于把 Agent 行为转换为 tracing spans：

- `agent.run`
- `codex.task`
- `tool.call`
- network / syscall / policy 派生 span

配置：

- `OtlpEnabled`
- `OtlpEndpoint`
- `OtlpServiceName`
- `OtlpHeaders`

## Prometheus

`GET /metrics` 暴露 Prometheus metrics。release mode 下需要 auth。

---

## 相关导航

- [External API](../external-api.md)
- [路由与 API](../backend/routes-api.md)
- [OTLP export](../otel-export.md)
- [MCP Skills 增强](../mcp-skills-enhancement.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
