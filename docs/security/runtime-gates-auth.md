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

## Hooks 特例

`/hooks/event` 可使用：

- runtime token；
- per-hook `X-Agent-Hook-Secret`。

这允许 hook relay script 使用 provider-specific secret，而不是暴露全局 token。
