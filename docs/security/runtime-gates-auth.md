# Runtime Gates 与 Auth

本页解释 build feature、runtime gate 和 auth 的关系。

## 三层模型

```text
Build feature compiled in?
  → Runtime setting enabled?
    → Release mode auth passed?
      → Handler executes
```

任何一层不满足，都可能导致功能不可用。

## Build feature

由 `AGENT_BUILD_FEATURES` 和 Go build tags 控制。

示例：

```bash
AGENT_BUILD_FEATURES=all make backend
AGENT_BUILD_FEATURES=core make backend
AGENT_BUILD_FEATURES=tls_capture,ml make backend
```

## Runtime gate

由 `/config/runtime`、环境变量和 `runtime.json` 控制。

典型环境变量：

- `AGENT_RUNTIME_SHELL_SESSIONS_ENABLED`
- `AGENT_RUNTIME_SYSTEM_RUN_ENABLED`
- `AGENT_RUNTIME_HOOK_MANAGEMENT_ENABLED`
- `AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED`
- `AGENT_RUNTIME_TLS_CAPTURE_ENABLED`
- `AGENT_RUNTIME_OTLP_ENABLED`
- `AGENT_RUNTIME_DOMAIN_FORWARD_ENABLED`
- `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENABLED`
- `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MIN_SCORE`
- `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_NETWORK`
- `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_FILE_NAMES`
- `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_ENFORCE_EXEC`
- `AGENT_RUNTIME_KERNEL_RISK_FEEDBACK_MAX_ACTIONS_PER_MINUTE`

常见 gate 与关联文档：

| Gate / setting | 控制面 | 相关文档 |
| --- | --- | --- |
| `ShellSessionsEnabled` | `/shell-sessions*`、`/ws/shell*` | [前端工作台](/frontend/workbench)、[路由与 API](/backend/routes-api) |
| `SystemRunEnabled` | `/system/run` | [安全模型](/security/model)、[路由与 API](/backend/routes-api) |
| `HookManagementEnabled` | hook install / raw hook writes | [Native Hooks](/integrations/native-hooks) |
| `PolicyManagementEnabled` | wrapper / cgroup / LSM policy mutation | [策略语义](/security/policy-semantics)、[eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement) |
| `TlsCaptureEnabled` | TLS / Codex capture | [脱敏与隐私](/security/redaction-privacy)、[TLS Quickstart](../backend/TLS_QUICKSTART.md) |
| `OtlpEnabled` | OTLP export | [MCP、External API 与 OTLP](/integrations/mcp-external-otlp) |
| `DomainForwardProxy.Enabled` | 80/443 Host/SNI forwarder | [部署与安装](/operations/deployment)、[README](../../README.md) |
| `KernelRiskFeedback.Enabled` | 用户态风险评分写回 cgroup / LSM map | [ML、Plugins 与扩展能力](/backend/ml-plugins)、[安全模型](/security/model) |

::: warning 双 gate 能力
Kernel risk feedback 写内核策略时需要 `PolicyManagementEnabled=true` 且 `KernelRiskFeedback.Enabled=true`。只打开 ML 或只打开 policy management 都不应被描述为“自动写入内核阻断规则”。
:::

## Auth

release mode 使用 runtime access token。token 存储：

```text
~/.config/agent-ebpf-filter/runtime.json
```

也可通过：

```text
AGENT_API_KEY
AGENT_ACCESS_TOKEN
```

覆盖。

认证方式：

- `X-API-KEY`；
- `Authorization: Bearer`；
- `?key=`。

release mode 下，敏感入口默认需要 token，包括：

- `/config/**`
- `/system/**`
- `/ws*`
- `/metrics`
- `/register` / `/unregister`
- `/agentsight/**`
- `/api/**` 与 `/api/v1/**`
- `/events/recent`、`/events/graph`
- `/sandbox/**`
- `/shell-sessions*`
- `/mcp`

修改这些范围时，同步 [路由与 API](/backend/routes-api)、[External API](../external-api.md)、[MCP、External API 与 OTLP](/integrations/mcp-external-otlp) 和 [文档地图](/reference/documentation-map)。

## Hooks 特例

`/hooks/event` 可使用：

- runtime token；
- per-hook `X-Agent-Hook-Secret`。

这允许 hook relay script 使用 provider-specific secret，而不是暴露全局 token。
