# Runtime Settings 与 Feature Manifest

本项目把功能可用性拆成三层：build feature、runtime setting、auth。三者不要混淆。

## RuntimeSettings

定义：`backend/core/state_types.go`

核心字段：

| 字段 | 作用 |
| --- | --- |
| `LogPersistenceEnabled` | 是否写 JSONL |
| `LogFilePath` | 持久化文件路径 |
| `AccessToken` | runtime access token |
| `MaxEventCount` | archive 最大事件数 |
| `MaxEventAge` | archive 最大保留时间 |
| `ShellSessionsEnabled` | shell sessions gate |
| `SystemRunEnabled` | `/system/run` gate |
| `HookManagementEnabled` | hook install / raw write gate |
| `PolicyManagementEnabled` | wrapper / sandbox policy mutation gate |
| `OtlpEnabled` | OTLP export gate |
| `TlsCaptureEnabled` | TLS capture gate |
| `MLConfig` | ML runtime config |
| `KernelRiskFeedback` | 用户态风险评分写回内核 map 的闭环配置 |
| `DomainForwardProxy` | 80/443 Host/SNI forward config |

## Feature manifest

定义：`backend/app/feature_manifest.go`

每个 feature 包含：

- `id`
- `name`
- `compiledIn`
- `runtimeEnabled`
- `runtimeGate`
- `authRequired`
- `routePrefixes`
- `dangerLevel`
- `buildTag`
- `compatibilityAliases`

## 关键 feature

| ID | Danger | Runtime gate | 说明 |
| --- | --- | --- | --- |
| `shell_sessions` | high | `shell_sessions` | PTY sessions |
| `system_run` | critical | `system_run` | 系统命令执行 |
| `hooks` | high | `hook_management` | Native hook 管理 |
| `policy_management` | high | `policy_management` | 策略修改 |
| `tls_capture` | critical | `tls_capture` | TLS / Codex capture |
| `otlp` | medium | `otlp` | OpenTelemetry export |
| `domain_forward` | high | `domain_forward` | 80/443 Host/SNI forward |
| `ml` | medium | ML config | ML / LLM scoring |
| `plugins` | high | compiled availability | plugin registry / visual builder |
| `sandbox_cgroup` | high | compiled availability | cgroup enforcement |
| `sandbox_lsm` | high | compiled availability | BPF LSM enforcement |
| `network_export` | medium | compiled availability | JSONL / PCAP export |
| `agentsight` | low | compiled availability | AgentSight compatibility |

## Build tags

`AGENT_BUILD_FEATURES` 会生成 `agentfeat_*` tags。

示例：

```bash
AGENT_BUILD_FEATURES=core make backend
AGENT_BUILD_FEATURES=tls_capture,ml make backend
AGENT_BUILD_FEATURES=all make backend
```

## 文档表达规范

- “编译进来”不等于“运行时启用”；
- “运行时启用”不等于“无需认证”；
- 高风险功能应写明默认关闭；
- release mode 敏感 API 必须写明 token；
- feature route prefix 变更应同步安全文档和文档站。
