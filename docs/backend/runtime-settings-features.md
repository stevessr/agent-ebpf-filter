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

### 共享有界历史缓冲

TLS 明文捕获、AgentSight 上传事件、wrapper 特征历史和已关闭网络连接共用
`internal/boundedring` 的插入顺序环形缓冲。缓冲按需增长且 backing capacity 不超过
配置上限；满容量后追加为 O(1) 覆盖，快照仍按最旧到最新的逻辑顺序返回。
这避免高频采集在锁内搬移整个历史切片，也避免通过反复重分配间歇性复制大缓冲。

### Signal Processing worker

Signal Processing 的活跃状态使用侵入式 LRU 维护访问顺序。更新已有状态、插入新状态和
超过 `MaxStates` 时淘汰最久未使用状态均为 O(1)，不再在容量压力下扫描并排序整个状态
表；TTL 全表扫描仅由独立 cron 或显式 expire 任务执行。状态接口的 `expiredTotal` 保持
“累计移除”兼容语义，`capacityEvictedTotal` 是其中由容量压力造成的子集，
`expiryRunsTotal` 记录实际完成的 TTL 扫描次数。`activeStates` 返回全部未过期状态数，
而 `recentStates` 仍最多返回 50 条。

### Loop Detection worker

Loop Detection 使用单消费者有界队列处理重复上下文。活跃窗口按访问顺序维护 O(1)
LRU；容量压力只淘汰最久未使用窗口，不再在每条事件上复制并排序全部 context。
事件热路径最多每 30 秒（短窗口使用其自身时长）批量扫描一次过期窗口；每个窗口的
PID、命令、路径、工具名和事件类型集合最多各保留 12 项，长键以稳定 SHA-256 后缀截断。
`/system/loop-detection/status` 的 `windowGCRunsTotal` 与 `windowEvictedTotal`
可用于确认清理频率和容量压力。

### Research Processing worker

Research Processing 的事件历史使用按需增长的有界环形缓冲。达到 `MaxEvents` 后，
每条新样本只覆盖一个最旧槽位，不再移动整个事件切片；动态缩小上限时仍按接收顺序
保留最新样本。`/system/research-processing/status` 的 `bufferEvictedTotal` 是进程级
累计淘汰数，可区分“已消费但超出历史容量”与 worker 队列的 `droppedTotal`。

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
