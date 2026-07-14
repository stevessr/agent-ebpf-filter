# Runtime Settings 与 Feature Manifest

本项目把功能可用性拆成三层：build feature、runtime setting、auth。三者不要混淆。

## RuntimeSettings

定义：`backend/core/state_types.go`

核心字段：

| 字段 | 作用 |
| --- | --- |
| `LogPersistenceEnabled` | 是否写 JSONL |
| `LogFilePath` | 持久化文件路径；必须是 `~/.config/agent-ebpf-filter/` 下的直接子文件，以 `0600` 安全打开 |
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
| `LoopDetection` | 重复上下文/重复读取检测 worker 配置 |
| `ResearchProcessing` | 后端研究视图、时间线和会话任务 worker 配置 |
| `SignalProcessing` | 信号规则、TTL 衰减、cron 清理和选中程序 protobuf 二进制日志配置 |
| `DomainForwardProxy` | 80/443 Host/SNI forward config |

### 事件日志持久化语义

启用 `LogPersistenceEnabled` 后，捕获线程只向 4096 项有界队列执行非阻塞提交；
单消费者 writer 使用 256 KiB 缓冲区，按 128 条或 250 ms 批量刷盘。队列满、单条
JSON 编码失败和文件 I/O 失败都会进入健康指标，但不会让内核事件读取线程等待磁盘。

`LogFilePath` 仍只允许 runtime settings 目录下的直接子普通文件，并拒绝符号链接、
硬链接和特殊文件。配置更新采用 prepare/drain/swap：相同路径保持当前 writer，
路径变更排空旧 generation，持久化配置写入失败时回滚原配置。禁用、清空和停机也会在
有界期限内排空已接受记录。

读取近期事件会先等待 flush barrier，然后从文件尾部反向读取；单行、扫描行数、
扫描字节数、返回条数和取消信号都有明确边界。writer 的 generation 计数会在重启或
路径切换后重置，而 `capturedPersistedTotal` / `capturedPersistErrorsTotal` 是进程级
累计计数。

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

---

## 相关导航

- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [路由与 API](routes-api.md)
- [前端 Feature Flags](../frontend/build-feature-flags.md)
- [安全模型](../security/model.md)
- [维护检查清单](../reference/maintenance-checklists.md)
