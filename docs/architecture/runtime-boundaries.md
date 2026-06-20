# 运行时边界

Agent eBPF Filter 有多层运行时边界。理解这些边界能避免把观测、控制、诊断和生产安全语义混淆。

## 权限边界

后端需要特权完成：

- 加载 eBPF programs；
- pin maps / links 到 bpffs；
- attach cgroup programs；
- attach BPF LSM programs；
- 可选绑定 80/443 domain forward 端口。

启动链中会通过 `ensureBackendPrivileges()` 检查能力，不足时可经 `sudo` / `pkexec` 自提权。

## Build feature 边界

`AGENT_BUILD_FEATURES` 会影响 Go build tags：

```text
shell_sessions → agentfeat_shell_sessions
system_run → agentfeat_system_run
tls_capture → agentfeat_tls_capture
ml → agentfeat_ml
plugins → agentfeat_plugins
sandbox_cgroup → agentfeat_sandbox_cgroup
sandbox_lsm → agentfeat_sandbox_lsm
...
```

默认 build 与 `agentfeat_all` 会启用全部 optional features。`core` 或逗号列表可裁剪能力。

## Runtime gate 边界

即使功能编译进二进制，高风险能力仍可能默认关闭。典型 runtime gates：

- shell sessions；
- `/system/run`；
- hook management；
- policy management；
- TLS capture；
- OTLP export；
- domain forward proxy；
- kernel risk feedback。

## Auth 边界

dev mode 默认关闭 auth；release mode 中敏感 API 需要 runtime access token。

常见认证方式：

- `X-API-KEY: <token>`；
- `Authorization: Bearer <token>`；
- WebSocket / MCP 可使用 `?key=<token>`。

## 数据边界

普通事件流不应携带未脱敏敏感明文。敏感数据处理原则：

- 命令 argv 使用 digest；
- prompt / response 使用 sha256 digest + length；
- TLS / Codex body 走 redaction 和截断；
- Event 中包含 `redaction_level` 与 `sanitized_fields`；
- TLS capture 默认关闭。

## 控制边界

| 控制路径 | 覆盖范围 | 非目标 |
| --- | --- | --- |
| wrapper | 经 wrapper 执行的命令 | 不是完整 shell sandbox |
| cgroup eBPF | cgroup id、exact IP/port 网络阻断 | 不是 CIDR/range 防火墙 |
| BPF LSM | exact exec path/name、file basename | 不是递归目录策略 |
| runtime gates | 后端危险能力开关 | 不是内核级访问控制 |
| hooks | AI CLI hook payload | 不保证覆盖未接入的 CLI |

---

## 相关导航

- [总体架构](overview.md)
- [数据流](data-flow.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [安全模型](../security/model.md)
- [Runtime Settings 与 Feature Manifest](../backend/runtime-settings-features.md)
