# MCP、External API 与 OTLP

Agent eBPF Filter 对外提供 MCP、External API v1、Prometheus 和 OTLP 四种集成接口，适用于 AI IDE 接入、自动化运维、监控告警和分布式追踪场景。

---

## MCP (Model Context Protocol)

### ```text
/mcp
```

MCP 使用 Streamable HTTP 传输，认证方式与后端 config API 一致。

### ```bash
# Header 方式
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/mcp

# Bearer 方式
curl -H "Authorization: Bearer <token>" http://127.0.0.1:8080/mcp

# Query 方式
curl "http://127.0.0.1:8080/mcp?key=<token>"
```

### MCP 工具列表

| 工具名称 | 描述 | 参数 | 需要 policy gate |
| --- | --- | --- | --- |
| `tail_events` | 获取最近捕获事件 | `limit`（可选，默认 50，最大 500） | 否 |
| `config_snapshot` | 获取完整配置快照 | 无 | 否 |
| `add_tracked_command` | 添加追踪命令 | `command`, `tag` | 否 |
| `add_tracked_path` | 添加追踪路径 | `path`, `tag` | 否 |
| `query_events` | 按条件查询事件 | `eventType`, `comm`, `pid`, `limit` | 否 |
| `get_network_flows` | 获取网络流量摘要 | 无 | 否 |
| `get_system_health` | 获取系统健康状态 | 无 | 否 |
| `block_network_destination` | 阻止网络目标 | `ip` 或 `port` | **是** |
| `block_process_cgroup` | 阻止进程 cgroup 网络 | `pid` | **是** |
| `block_file_access` | BPF LSM 阻止文件访问 | `path`, `basename`, `isExec` | **是** |

::: warning
`block_*` 工具需要在 Runtime Config 中启用 `policyManagementEnabled` 标志。
:::

### Claude Code Skills

项目提供三个 Claude Code skills 用于操作 MCP 服务：

- **configure-security**：配置安全策略（tracked commands/paths、wrapper 规则、网络/文件拦截）
- **analyze-network**：分析网络流量，识别异常连接
- **monitor-process**：深度监控特定进程的行为（文件访问、网络、子进程）

使用方式：在 Claude Code 中直接调用这些 skills，它们会自动使用项目的 MCP 工具。

---

## External API v1

外部自动化应优先使用 `/api/v1` 下的版本化别名，认证方式与 MCP 一致。

### ```bash
# 健康检查
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/health

# OpenAPI 规范
curl http://127.0.0.1:8080/api/v1/openapi.json
```

### ```bash
# 最近事件
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/events/recent?limit=50

# 执行图谱
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/events/graph
```

### ```bash
# 网络流列表
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/network/flows

# DNS 缓存
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/network/dns-cache

# 接口统计
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/network/interfaces

# JSONL 导出
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/network/export/jsonl
```

### OS Enforcement 状态

```bash
# cgroup sandbox 状态
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/sandbox/cgroup/status

# BPF LSM 状态
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/sandbox/lsm/status
```

### policyManagementEnabled）

```bash
# 阻止 IP
curl -X POST -H "X-API-KEY: <token>" -H "Content-Type: application/json" \
  -d '{"ip":"10.0.0.1"}' http://127.0.0.1:8080/api/v1/policies/network/block-ip

# 阻止端口
curl -X POST -H "X-API-KEY: <token>" -H "Content-Type: application/json" \
  -d '{"port":8443,"protocol":"tcp"}' http://127.0.0.1:8080/api/v1/policies/network/block-port

# 阻止文件名
curl -X POST -H "X-API-KEY: <token>" -H "Content-Type: application/json" \
  -d '{"name":"secret.txt"}' http://127.0.0.1:8080/api/v1/policies/lsm/block-file-name
```

### AgentSight 兼容接口

```bash
# 事件导出（JSON / JSONL）
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/agentsight/events?format=jsonl

# 存储统计
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/agentsight/events/stats

# Runner 状态
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/api/v1/agentsight/runners
```

### Agent 注册

```bash
# 注册 PID
curl -X POST -H "X-API-KEY: <token>" -H "Content-Type: application/json" \
  -d '{"pid":12345,"tag":"AI Agent","agent_run_id":"run-001"}' \
  http://127.0.0.1:8080/api/v1/agents/register

# 注销 PID
curl -X POST -H "X-API-KEY: <token>" -H "Content-Type: application/json" \
  -d '{"pid":12345}' http://127.0.0.1:8080/api/v1/agents/unregister
```

---

## OTLP 导出

OTLP export 将 Agent 行为转换为 OpenTelemetry tracing spans：

| Span 类型 | 描述 |
| --- | --- |
| `agent.run` | Agent 运行会话 |
| `codex.task` | Codex 任务 |
| `tool.call` | 工具调用 |
| `mcp.call` | MCP 调用 |
| 子 spans | 子进程 / 文件 / 网络 / 策略派生 |

### 通过 Runtime Config（`/config/runtime`）或环境变量配置：

| 配置项 | 环境变量 | 说明 |
| --- | --- | --- |
| `OtlpEnabled` | `AGENT_RUNTIME_OTLP_ENABLED` | 启用 OTLP 导出 |
| `OtlpEndpoint` | `AGENT_RUNTIME_OTLP_ENDPOINT` | OTLP HTTP endpoint |
| `OtlpServiceName` | `AGENT_RUNTIME_OTLP_SERVICE_NAME` | 服务名称 |
| `OtlpHeaders` | `AGENT_RUNTIME_OTLP_HEADERS` | 自定义 HTTP headers |

### Jaeger / Grafana 集成

```bash
# 启动 Jaeger all-in-one
docker run -d --name jaeger \
  -p 4318:4318 -p 16686:16686 \
  jaegertracing/jaeger:latest

# 配置后端 OTLP endpoint
# Runtime Config 中设置:
# OtlpEnabled: true
# OtlpEndpoint: http://localhost:4318
# OtlpServiceName: agent-ebpf-filter
```

### ```bash
curl -H "X-API-KEY: <token>" http://127.0.0.1:8080/system/otel-health
```

返回：exporter readiness、queue length、active span counts、exported/dropped totals。

---

## Prometheus 指标

`GET /metrics` 暴露 Prometheus 格式指标，release mode 下需要认证。

### | 指标 | 描述 |
| --- | --- |
| `agent_ebpf_ringbuf_events_total` | ringbuf 事件总数 |
| `agent_ebpf_ringbuf_reserve_fail` | ringbuf 预留失败次数 |
| `agent_ebpf_zerocopy_decode_total` | zero-copy 解码次数 |
| `agent_ebpf_kernel_risk_*` | 内核风险评分与反馈计数 |
| `agent_ebpf_ws_clients` | WebSocket 客户端数 |
| `agent_ebpf_events_by_type` | 按类型的事件计数 |

### Grafana 集成

项目提供现成的 AgentSight Grafana 配置：

```bash
docker compose -f docs/agentsight-grafana-compose.yml up -d
```

包含 Loki/Grafana 对 JSONL 事件日志的可视化。

---

## - [External API 详细文档](external-api.md)
- [路由与 API](../backend/routes-api.md)
- [OTLP 导出配置](otel-export.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [Agents、Adapters 与 PID 注册](agents.md)

