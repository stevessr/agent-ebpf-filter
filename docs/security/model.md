# 安全模型

Agent eBPF Filter 的安全模型由五层组成：权限、认证、runtime gates、策略语义、数据脱敏。

## 安全目标

- 只在授权环境中加载和管理 eBPF；
- release mode 保护敏感 API；
- 高风险能力默认关闭；
- policy mutation 通过认证后端 API；
- 避免普通事件流泄漏 secrets / TLS plaintext；
- 将观测、诊断、控制的边界说清楚。

## 权限层

后端需要特权进行：

- eBPF load / attach；
- map / link pinning；
- cgroup / LSM attach；
- restrictive map permissions；
- 可选 80/443 binding。

子 shell / 命令应尽量 drop privileges 回调用用户。

## Auth 层

release mode 下敏感 API 使用 runtime access token。常见受保护面：

- `/config/**`
- `/system/**`
- `/ws*`
- `/metrics`
- `/register`
- `/unregister`
- `/events/recent`
- `/events/graph`
- `/shell-sessions*`
- `/sandbox/**`
- `/agentsight/**`
- `/api/**`
- `/api/v1/**`
- `/mcp`

## Runtime gate 层

危险能力默认关闭：

- shell sessions；
- `/system/run`；
- hook management；
- policy management；
- TLS capture；
- OTLP export；
- domain forward；
- kernel risk feedback。

## 内核控制层

| 控制 | 语义 | 边界 |
| --- | --- | --- |
| cgroup sandbox | exact cgroup id、IPv4、IPv6、port | 不是 CIDR/range firewall |
| BPF LSM | exact exec path/name、file basename | 不是递归目录策略 |
| wrapper | command shim | 只覆盖经 wrapper 执行的命令 |

## 数据保护层

- argv digest；
- prompt / response digest + length；
- redaction levels；
- TLS / Codex body 截断；
- sanitized_fields；
- secrets / tokens / headers / query / JSON body redaction。

## 高风险能力清单

| 能力 | 风险 | 防护 |
| --- | --- | --- |
| TLS capture | 明文敏感数据 | 默认关闭、auth、runtime gate、redaction |
| system run | 任意命令执行 | critical feature、auth、runtime gate |
| shell sessions | 交互式 PTY | auth、runtime gate、privilege dropping |
| hook install | 修改用户 CLI 配置 | auth、runtime gate、确认授权 |
| policy mutation | 阻断网络/文件/执行 | auth、runtime gate、restrictive maps |
| domain forward | 80/443 public data plane | 默认关闭、auth-protected config |
