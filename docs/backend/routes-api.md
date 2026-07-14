# 路由与 API 参考

路由总入口为 `backend/app/routes.go` 中的 `registerRoutes()` 函数。路由注册按功能分组，各组由独立的 `register*Routes` 函数负责。

## 注册顺序

```
registerRoutes()
  registerWebSocketRoutes()
  registerShellSessionRoutes()
  registerEventRoutes()
  registerNetworkRoutes()
  registerSandboxRoutes()
  registerUtilityRoutes()
  registerAuthenticatedAPIRoutes()
  registerCompatibilityRoutes()
  registerDocsRoutes()
  registerStaticRoutes()
```

## WebSocket 实时流 (`/ws`)

所有 WebSocket 路由需要 `authMiddleware()` 认证。

| 路径 | 用途 | 特性门控 |
|------|------|---------|
| `/ws` | Protobuf 事件流 | -- |
| `/ws/system` | 系统遥测 (CPU/内存/GPU/Zram) | -- |
| `/ws/camera` | 摄像头实时流 | -- |
| `/ws/sensors` | 传感器数据流 | -- |
| `/ws/microphone` | 麦克风流 | -- |
| `/ws/ml-status` | ML 训练状态推送 | `FeatureML` |
| `/ws/envelopes` | EventEnvelope 格式流 | -- |
| `/ws/events/graph` | 执行图谱流 | -- |
| `/ws/tls-capture` | TLS 明文捕获流 | `FeatureTLSCapture` |
| `/ws/shell` | Shell 会话终端 | `FeatureShellSessions` |
| `/ws/shell-sessions` | Shell 会话列表 | `FeatureShellSessions` |

## Shell 会话 (`/shell-sessions`)

需要 `FeatureShellSessions` 编译特性、`authMiddleware()` 认证和 `shellSessionsEnabledMiddleware()` 运行时门控。

| 方法 | 路径 | handler |
|------|------|--------|
| `POST` | `/shell-sessions` | `handleCreateShellSession` |
| `GET` | `/shell-sessions` | `handleListShellSessions` |
| `DELETE` | `/shell-sessions/:id` | `handleDeleteShellSession` |
| `POST` | `/shell-sessions/:id/input` | `handleSendShellSessionInput` |
| `POST` | `/shell-sessions/cleanup` | `handleShellSessionsCleanup` |

## 事件路由 (`/events`)

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/events/recent` | 近期捕获事件 (支持 limit/type/pid/comm 查询参数，`limit` 最大 1000) |
| `GET` | `/events/graph` | 聚合执行图谱 (支持 agent_run_id/trace_id/since/until) |
| `GET` | `/events/recording` | 录制状态 |
| `POST` | `/events/recording/start` | 启动事件录制（仅允许运行时 `recordings/` 目录下的直接子文件，文件权限 `0600`） |
| `POST` | `/events/recording/stop` | 停止事件录制 |
| `POST` | `/events/recording/replay` | 回放录制（安全普通单链接文件；最大 128 MiB、单行 4 MiB、返回 10000 条） |
| `POST` | `/events/recording/browser/save` | 原子保存浏览器录制（原始 export 最大 16 MiB，格式化输出最大 32 MiB） |

> 录制文件路径可使用文件名，或使用位于
> `~/.config/agent-ebpf-filter/recordings/` 下的绝对路径。父目录跳转、嵌套目录、
> 符号链接、硬链接与 FIFO/设备等特殊文件会被拒绝。
>
> 事件录制采用 2048 项有界单消费者队列和 256 KiB 缓冲区，按 128 条或
> 250 ms 批量刷盘；未启用录制时不会执行事件 JSON 编码。`Stop`、录制替换和
> 后端停机会停止接收并排空已接受事件，状态响应包含
> `pending`/`queueLen`/`queueCap`/`failedTotal`/`droppedTotal` 指标。单条 JSONL
> 记录最大约 4 MiB；录制文件达到与回放一致的 128 MiB 上限后会停止该 writer，
> 避免生成无法回放的文件。
>
> 回放从文件尾部按 256 KiB 块反向读取，只解析满足 `limit` 所需的最新有效 JSONL，
> 再恢复时间顺序，避免为了 200 条尾部事件扫描完整 128 MiB 文件。一次请求最多检查
> 250000 行，并受 15 秒处理期限和客户端取消信号约束；活动 writer 增长时使用请求开始
> 时的文件大小快照。执行图的进程树使用邻接表 BFS，避免反序 PID 链触发重复全表扫描。
>
> 执行图构建器最多处理最新 10000 条输入，输出最多 12000 个节点、24000 条边，
> 并使用 32 MiB 的保守 JSON 编码预算。节点 ID/标签/副标题分别限制为
> 512/512/1024 字节，单个元数据值限制为 4096 字节，边 ID 限制为 1024 字节。
> 超长资源 ID 保留可读前缀并附加 SHA-256，避免冲突与巨型边 ID。
> 响应中的 `truncated`、`omittedEventCount`、`omittedNodeCount`、
> `omittedEdgeCount` 和 `truncatedFieldCount` 会显式标记被边界化的结果，前端会显示
> `bounded output` 提示。空元数据字段不再输出。

> 运行时持久化日志与录制 writer 相互独立：它使用 4096 项非阻塞队列、256 KiB
> 缓冲区，并按 128 条或 250 ms 刷盘。`/events/recent` 会先等待 flush barrier，
> 再按 256 KiB 块从 JSONL 尾部读取；单行最多 4 MiB、最多检查 250000 行和
> 128 MiB，并继承 HTTP 取消信号和统一的 10 秒处理期限。文件读取失败时回退到内存 archive，客户端取消
> 则直接终止。AgentSight 查询/SSE、执行图、信号扫描以及 MCP tail_events /
> query_events 同样向该读取路径传播请求 context。

## 网络路由 (`/network`)

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/network/flows` | 聚合网络流量视图 (支持 filter/sort/limit/cursor/pid/domain) |
| `GET` | `/network/flows/:flowID` | 按流 ID 查询单条流 |
| `GET` | `/network/tcp-state` | TCP 连接状态追踪 |
| `GET` | `/network/analyze` | 端点分析 (IP 范围/服务/域名/风险评分) |
| `GET` | `/network/dns-lookup` | DNS 缓存 IP 查询 |
| `GET` | `/network/dns-cache` | DNS 缓存全量快照 |
| `GET` | `/network/interfaces` | 网卡 RX/TX 计数器 |
| `GET` | `/network/export/jsonl` | 流快照 JSONL 导出 (FeatureNetworkExport) |
| `POST` | `/network/export-pcap` | PCAP 导出 (FeatureNetworkExport；生成唯一的 `0600` PCAP 与 JSONL sidecar) |
| `GET` | `/network/geoip` | GeoIP 查询 (IP -> 国家/ASN) |

## 沙箱路由 (`/sandbox`)

### Cgroup 沙箱 (`/sandbox/cgroup`)

需要 `FeatureSandboxCgroup` 编译特性。写操作额外需要 `FeaturePolicyManagement`。

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/sandbox/cgroup/status` | Cgroup 沙箱状态/计数/阻断列表 |
| `POST` | `/sandbox/cgroup/block-cgroup` | 阻断 cgroup ID |
| `POST` | `/sandbox/cgroup/unblock-cgroup` | 解除 cgroup 阻断 |
| `POST` | `/sandbox/cgroup/block-pid` | 阻断 PID (解析其 cgroup) |
| `POST` | `/sandbox/cgroup/unblock-pid` | 解除 PID 阻断 |
| `POST` | `/sandbox/cgroup/block-ip` | 阻断目标 IP (v4/v6) |
| `POST` | `/sandbox/cgroup/unblock-ip` | 解除 IP 阻断 |
| `POST` | `/sandbox/cgroup/block-port` | 阻断目标端口 |
| `POST` | `/sandbox/cgroup/unblock-port` | 解除端口阻断 |

### BPF LSM 沙箱 (`/sandbox/lsm`)

需要 `FeatureSandboxLSM` 编译特性。写操作额外需要 `FeaturePolicyManagement`。

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/sandbox/lsm/status` | LSM 状态/计数/阻断列表 |
| `POST` | `/sandbox/lsm/block-exec-path` | 阻断可执行文件路径 |
| `POST` | `/sandbox/lsm/unblock-exec-path` | 解除路径阻断 |
| `POST` | `/sandbox/lsm/block-exec-name` | 阻断可执行文件 basename |
| `POST` | `/sandbox/lsm/unblock-exec-name` | 解除 basename 阻断 |
| `POST` | `/sandbox/lsm/block-file-name` | 阻断文件/目录 basename (open/read/write/mmap/mprotect/setattr/create/link/symlink/delete/mkdir/rmdir/mknod/rename) |
| `POST` | `/sandbox/lsm/unblock-file-name` | 解除文件 basename 阻断 |

### 系统健康路由

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/system/bootstrap-health` | 启动阶段健康状态 |
| `GET` | `/system/collector-health` | 捕获、广播、语义关联/工具基线状态及异步持久化 writer/queue 健康状态 |
| `GET` | `/system/otel-health` | OTLP exporter 健康状态 |

`/system/collector-health` 中的 `persistGeneration*` 字段属于当前 writer generation；
`capturedPersistedTotal` 和 `capturedPersistErrorsTotal` 为进程级累计值。最后刷盘时间、
队列长度/容量、pending、active/stopping 和最后错误可用于区分“功能未启用”“正在排空”
与“writer 因 I/O 失败停止”。/metrics 对应暴露 agent_ebpf_persist_writer_active、
agent_ebpf_persist_queue_length/capacity、agent_ebpf_persist_pending 以及当前 generation
的 failed/dropped gauges。

同一响应中的 `semanticStateEntriesByKind`、`semanticStateEntries` 和
`semanticStateMaxEntries` 展示五类有界语义关联状态的当前占用；
`semanticStateExpiredEvictionsTotal`、`semanticStateCapacityEvictionsTotal`、
`semanticStateTruncatedValuesTotal`、`semanticStateIgnoredOversizedMetadataTotal` 与
`semanticStateLastSweepAt` 用于发现 TTL 清理、容量压力和异常大输入。`/metrics` 对应暴露
`agent_ebpf_semantic_state_entries{kind=...}`、`agent_ebpf_semantic_state_max_entries`、
`agent_ebpf_semantic_state_expired_evictions_total`、
`agent_ebpf_semantic_state_capacity_evictions_total`、
`agent_ebpf_semantic_state_truncated_values_total` 和
`agent_ebpf_semantic_state_ignored_metadata_total`。

工具行为基线通过 `toolBaselineTools`、`toolBaselineSamples`、对应上限、每工具样本上限、
`toolBaselineObservationsTotal`、`toolBaselineDriftsTotal`、TTL/容量淘汰、受限值和
`toolBaselineLastSweepAt` 暴露。`/metrics` 对应提供
`agent_ebpf_tool_baseline_tools`、`agent_ebpf_tool_baseline_samples`、两个容量 gauge，
以及 `agent_ebpf_tool_baseline_observations_total`、`agent_ebpf_tool_baseline_drifts_total`、
`agent_ebpf_tool_baseline_expired_evictions_total`、
`agent_ebpf_tool_baseline_capacity_evictions_total` 和
`agent_ebpf_tool_baseline_truncated_values_total`。

## 工具路由

| 方法 | 路径 | 用途 | 特性门控 |
|------|------|------|---------|
| `GET` | `/metrics` | Prometheus 指标 | -- |
| `POST` | `/hooks/event` | 原生钩子事件上报 | `FeatureHooks` |
| `POST` | `/register` | 注册 Agent PID | -- |
| `POST` | `/unregister` | 注销 Agent PID | -- |
| `POST` | `/cluster/heartbeat` | 集群心跳 | -- |
| `POST` | `/cluster/register` | 集群注册 | -- |

## 认证 API 路由

以下路由在 `/` 前缀下注册，需要 `authMiddleware()`:

### 配置路由 (`/config`)

注册于 `registerConfigRoutes()`（位于 `handlershooksconfig.go`）:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/config/tags` | 标签列表 |
| `POST` | `/config/tags` | 创建标签 |
| `GET` | `/config/comms` | 追踪命令列表 |
| `POST` | `/config/comms` | 添加追踪命令 |
| `DELETE` | `/config/comms/:comm` | 移除追踪命令 |
| `POST` | `/config/comms/:comm/disable` | 禁用命令 |
| `DELETE` | `/config/comms/:comm/disable` | 启用命令 |
| `GET` | `/config/event-types` | 事件类型列表 |
| `POST` | `/config/event-types/:type/disable` | 禁用事件类型 |
| `DELETE` | `/config/event-types/:type/disable` | 启用事件类型 |
| `GET` | `/config/paths` | 追踪路径列表 |
| `POST` | `/config/paths` | 添加追踪路径 |
| `DELETE` | `/config/paths/*path` | 移除追踪路径 |
| `GET` | `/config/prefixes` | 追踪前缀列表 |
| `POST` | `/config/prefixes` | 添加追踪前缀 |
| `DELETE` | `/config/prefixes` | 移除追踪前缀 |
| `GET` | `/config/rules` | Wrapper 规则列表 |
| `POST` | `/config/rules` | 创建/更新规则 |
| `DELETE` | `/config/rules/:comm` | 删除规则 |
| `GET` | `/config/runtime` | 运行时配置 |
| `PUT` | `/config/runtime` | 更新运行时配置 |
| `POST` | `/config/access-token` | 轮换访问令牌 |
| `GET` | `/config/export` | 导出全量配置 |
| `POST` | `/config/import` | 导入配置 |

### ML 配置路由 (`/config/ml`)

需要 `FeatureML` 编译特性:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/config/ml/status` | ML 引擎状态；响应包含 `trainingReadiness`，用于说明 labeled/min samples、类别数、特征归一化、阻断原因、warnings 与 suggestedActions |
| `GET` | `/config/ml/logs` | 训练日志 |
| `GET` | `/config/ml/history` | 训练历史 |
| `POST` | `/config/ml/train` | 触发训练；失败响应会附带 `trainingReadiness` 便于定位样本不足、单类别或特征越界问题 |
| `POST` | `/config/ml/train/cancel` | 取消训练 |
| `POST` | `/config/ml/tune` | 超参调优 |
| `POST` | `/config/ml/tune-models` | 多模型调优 |
| `POST` | `/config/ml/feedback` | 提交反馈标签 |
| `GET` | `/config/ml/samples` | 训练样本列表 |
| `POST` | `/config/ml/samples` | 添加训练样本 |
| `PUT` | `/config/ml/samples/label` | 标注样本 |
| `PUT` | `/config/ml/samples/anomaly` | 设置异常分数 |
| `DELETE` | `/config/ml/samples/:index` | 删除样本 |
| `GET` | `/config/ml/existing-commands` | 现有命令候选 |
| `POST` | `/config/ml/import-existing` | 导入现有命令 |
| `POST` | `/config/ml/assess` | 命令安全评估 |
| `POST` | `/config/ml/llm/score` | LLM 评分 |
| `POST` | `/config/ml/llm/batch-score` | LLM 批量评分 |
| `POST` | `/config/ml/llm/production-dataset/pull` | 拉取生产数据集 |
| `GET` | `/config/ml/datasets/classic` | 经典数据集列表 |
| `GET` | `/config/ml/datasets/classic/:name` | 数据集详情 |
| `POST` | `/config/ml/datasets/classic/:name/preview` | 数据集预览 |
| `POST` | `/config/ml/datasets/pull` | 拉取远程数据集 |
| `POST` | `/config/ml/datasets/import` | 导入数据集 |
| `POST` | `/config/ml/datasets/agent-legal` | 导入内置合法 Agent 行为样本 |
| `POST` | `/config/ml/datasets/selinux-policy` | 导入内置常见 SELinux policy 规则样本（allow/neverallow/dontaudit/auditallow/permissive） |
| `GET` | `/config/ml/datasets/export` | 导出数据集 |
| `DELETE` | `/config/ml/datasets` | 清空数据集 |
| `GET` | `/config/ml/health/processes` | 健康监测进程 |
| `GET` | `/config/ml/health/generators` | 健康监测生成器 |
| `POST` | `/config/ml/health/register` | 注册健康监测 |
| `POST` | `/config/ml/health/unregister` | 注销健康监测 |
| `POST` | `/config/ml/health/run` | 运行健康检查 |
| `POST` | `/config/ml/backtest` | 回测 (同 assess) |

LLM 出站请求共享 4 个全局并发槽位，单次响应限制为 256 KiB，配置超时会被收敛到
5–120 秒。批量评分默认处理 20 条、单次最多 100 条，同时最多运行 2 个批次；每批
使用最多 4 个有界 worker，整体最长运行 10 分钟并继承 HTTP 请求取消信号；
启发式数据导入不会触发外部 LLM。

自动训练 scheduler 在启动时已初始化 ML 的节点上始终保持存活；临时关闭
`AutoTrain`、关闭 ML 或 master 身份变更只会让它跳过当次检查，不会永久退出。
每轮会重新读取 `TrainInterval`；定时 flush worker 也保持存活直到进程 context
取消。超参/模型调优使用单槽位有界任务队列，`/config/ml/status` 的
`autoTuneRuntime` 暴露队列、完成、取消、拒绝与耗时统计。

`/config/ml/datasets/pull` 与 `/config/ml/datasets/import` 支持 `json`、`jsonl`、`csv`、`tsv`、纯文本与常见压缩包；纯文本 `.te`/SELinux policy 规则以及 JSON `rules[].rule` / `rules[].selinuxRule` 字段会自动识别为 `selinux-rule ...` 训练样本，并按 `allow/type_transition=ALLOW`、`neverallow=BLOCK`、`dontaudit/auditallow/permissive=ALERT` 保留来源标签。响应会附带 `byLabel`、`byCategory`、`bySource`、`normalization`、`quality`，导入响应还会附带 `skipReasons`；压缩包成员打开/读取失败、归档流后续读取失败、嵌套压缩流解码失败、条目解析失败或 limit 截断会连同成员或归档来源出现在 `parseWarnings`。

远程 URL 下载采用 fail-closed 网络策略：仅允许解析结果全部为公网地址的 `http`/`https` URL，禁止 URL credentials、loopback、RFC1918/ULA、link-local、云 metadata 与其他 special-use 地址，且每次连接和每个重定向都会重新验证；下载不会使用进程环境代理，也禁止 HTTPS 重定向降级到 HTTP。内网或本机数据集应通过 `content` / `contentBase64` 上传。单个下载或归档成员上限为 20 MiB；归档累计展开上限为 64 MiB、4096 个成员和 4 层嵌套。所有 payload 共享 100,000 条记录的解析硬上限，即使 `importAll=true` 也不会绕过；达到硬上限时响应返回 `truncated=true`、`totalIsLowerBound=true` 和 `record_limit_truncated` warning。

Research training API：`GET /research/sessions/:id/training` 和 `POST /research/sessions/:id/training/import` 会返回 `byLabel/byCategory/bySource`、`normalization` 与 `quality`，导入响应额外返回 `skippedByReason`。Research bundle 中的 `training-manifest.json` 会记录 feature space/version、redaction level 分布和训练可用性摘要。

### Hook 配置路由 (`/config/hooks`)

需要 `FeatureHooks` 编译特性:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/config/hooks` | 可用 Hook 列表 |
| `POST` | `/config/hooks` | 安装/卸载 Hook |
| `GET` | `/config/hooks/:id/raw` | 读取原始配置 |
| `POST` | `/config/hooks/:id/raw` | 写入原始配置 |

### 系统路由 (`/system`)

注册于 `handlers.RegisterSystemRoutes()`（位于 `backend/app/handlers/system.go`）：

`/system/ls`、`/system/file-*`、`/system/download` 和 `/system/upload` 属于高风险主机文件能力，除认证外还需要 `system_run` 编译特性与 runtime gate。

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/system/ls` | 目录列表 |
| `GET` | `/system/file-preview` | 文件预览 |
| `GET` | `/system/file-preview/stream` | 文件预览流式输出 |
| `GET` | `/system/file-hex` | 文件十六进制查看 |
| `GET` | `/system/file-elf` | ELF 文件分析（文件最大 256 MiB，符号元数据最大 8 MiB/65536 项，`objdump` 输出最大 2 MiB） |
| `GET` | `/system/home` | 用户主目录 |
| `GET` | `/system/download` | 文件下载 |
| `POST` | `/system/upload` | 文件上传（需启用 `system_run` runtime gate；单文件最大 64 MiB；文件名会归一化；不覆盖已有目标） |
| `POST` | `/system/benchmark` | 运行基准测试 |
| `GET` | `/system/benchmark` | 基准测试结果 |
| `GET` | `/system/tracked-comms` | 已追踪命令列表 |
| `POST` | `/system/process/signal` | 发送进程信号 |
| `GET` | `/system/process/maps` | 进程内存映射 |
| `GET` | `/system/sensors` | 传感器列表 |
| `GET` | `/system/cameras` | 摄像头列表 |
| `GET` | `/system/camera/snapshot` | 摄像头快照 |
| `GET` | `/system/microphones` | 麦克风列表 |
| `POST` | `/system/run` | 执行命令；需要 `FeatureSystemRun` 编译特性且 `SystemRunEnabled=true` |
| `GET` | `/system/signals/status` | 信号处理运行态：队列、TTL 状态、最近信号、可用 signal kinds |
| `POST` | `/system/signals/task` | 信号处理后台任务：`scan_recent`/`expire`/`reset` |
| `POST` | `/system/signals/rules/test` | 用最近事件 dry-run 单条 signal rule，返回匹配样本 |
| `GET` | `/system/signals/program-logs` | 选中程序的本地 protobuf 压缩日志状态、路径、frame 数 |
| `GET` | `/system/signals/program-logs/download?program=...` | 下载某个已配置选中程序的 length-framed gzip protobuf 日志 |

`/system/signals/status` 对应 `RuntimeSettings.signalProcessing`。信号规则支持 `path_access`、`child_process`、`repeated_read` 与 `custom` kind，规则内条件按 AND 组合；选中的程序会由后端写入本地 length-framed gzip protobuf (`ProgramSignalLogRecord`) 二进制日志。状态响应的 `capacityEvictedTotal` 区分 LRU 容量淘汰，`expiryRunsTotal` 记录 TTL 扫描次数。

选中程序日志只允许位于 `~/.config/agent-ebpf-filter/signals/program-logs/` 目录的直接子文件；`path` 可留空使用按程序名生成的默认文件名，或填写该目录下的单一文件名。后端会拒绝越界/嵌套路径、symlink、多硬链接和 FIFO/设备等特殊文件，并将单个日志的追加与下载上限固定为 128 MiB。事件热路径通过有界单消费者队列异步追加，停机时停止接收并排空已接受任务；状态响应的 `writer` 字段提供队列、完成、失败和丢弃计数。frame 计数按文件大小/修改时间缓存，成功追加会常数时间推进计数，轮询状态不会反复扫描完整日志。读取时最多接受 100000 个 frame，单个 gzip frame 最大 8 MiB，解压后的 protobuf payload 最大 4 MiB，以阻止异常文件和压缩炸弹造成资源耗尽。

`/system/run` 在 app 路由层单独注册，编译时未包含 `FeatureSystemRun` 返回 `501`，运行时 gate 关闭返回 `403`；它不会随其他普通 system 路由无条件暴露。

### TLS 捕获路由 (`/tls-capture`)

需要 `FeatureTLSCapture` 编译特性，并受 `TlsCaptureEnabled` 运行时 gate 保护；gate 关闭时 `/tls-capture/**`、`/codex/capture` 与 `/ws/tls-capture` 返回 `403`。
近期明文历史使用有界环形缓冲，容量满后每条新事件只覆盖一个最旧槽位，不在高吞吐 TLS 捕获热路径移动完整历史。

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/tls-capture/recent` | 近期捕获的 TLS 明文事件 |
| `GET` | `/tls-capture/libraries` | 已附加 TLS 库状态列表 |
| `GET` | `/tls-capture/status` | 捕获运行时状态 |
| `POST` | `/tls-capture/start` | 启动捕获并附加默认库 |
| `POST` | `/tls-capture/attach-defaults` | 扫描并附加系统默认 TLS 库 |
| `POST` | `/tls-capture/attach-builtins` | 附加内置 AI 工具可执行文件 |
| `GET` | `/tls-capture/rules` | 捕获规则列表 |
| `PUT` | `/tls-capture/rules` | 更新捕获规则 (批量) |
| `POST` | `/tls-capture/library` | 手动附加自定义 TLS 库路径 |
| `POST` | `/tls-capture/go-binary` | 手动附加 Go 编译的二进制文件 |
| `POST` | `/tls-capture/executable` | 附加指定路径的可执行文件 |

#### `GET /tls-capture/status` 广播状态

`GET /tls-capture/status` 响应中的 `broadcast` 对象用于观测 `/ws/tls-capture` 客户端的有界异步队列和写入故障。可以只投影运行态与广播字段：

```bash
curl -s http://localhost:8080/tls-capture/status | \
  jq '{enabled, available, readStarted, goDiscoveryStarted, broadcast}'
```

```json
{
  "enabled": true,
  "available": true,
  "readStarted": true,
  "goDiscoveryStarted": true,
  "broadcast": {
    "activeClients": 1,
    "queuedEvents": 0,
    "queueCapacity": 64,
    "queueFullDropsTotal": 0,
    "writeFailuresTotal": 0,
    "writeDeadlineFailuresTotal": 0
  }
}
```

| `broadcast` 字段 | 语义 |
|--------------------|------|
| `activeClients` | 当前连接到 `/ws/tls-capture` 的客户端数 |
| `queuedEvents` | 快照时所有活跃客户端队列中待写事件的总数 |
| `queueCapacity` | **每个客户端**的有界队列容量；判断总使用率时上限为 `activeClients * queueCapacity` |
| `queueFullDropsTotal` | 因某个客户端队列已满而未入队的累计投递数；后端会关闭该过载连接 |
| `writeFailuresTotal` | WebSocket JSON 写入失败的累计次数 |
| `writeDeadlineFailuresTotal` | 设置 WebSocket 单次写入截止时间失败的累计次数 |

三个 `*Total` 计数器在当前后端进程内单调累加，重启后重置。`activeClients` 为 `0` 在没有打开 TLS Capture 页面或其他 WebSocket 消费者时是正常现象。

### AgentSight 路由 (`/agentsight`)

需要 `FeatureAgentSight` 编译特性:

AgentSight 事件上传端点（`POST /agentsight/events`及兼容路由）单次最多接收 16 MiB 或 10,000 条事件；超限返回 `413` 且不导入部分数据，已接受的 10,000 条事件均可保留在上传事件环形库中。环形库按需增长且永不超过上限，单条满容量追加为 O(1)，大批次可直接替换为最新 10,000 条。前端本地文件/粘贴导入使用相同的 16 MiB 和 10,000 条限制，并始终将导入缓存、WebSocket 事件和系统采样缓冲区保持在 10,000 条以内。SSE 流每个连接的事件 ID 去重状态限制为请求 `limit` 的两倍（最多 10,000 个 ID），长时间连接不会无界积累历史 ID。

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/agentsight/runners` | Runner 列表与状态 |
| `GET` | `/agentsight/events` | 合并事件导出 (支持 format/jsonl/array) |
| `POST` | `/agentsight/events` | 上传 AgentSight 事件 |
| `GET` | `/agentsight/events.jsonl` | JSONL 格式导出 |
| `GET` | `/agentsight/events/stats` | 事件统计 (按 source/type/runner/comm) |
| `GET` | `/agentsight/events/runners/:id/stats` | 单 Runner 统计 |
| `POST` | `/agentsight/events/query` | 高级查询 (JSON body) |
| `GET` | `/agentsight/events/stream` | SSE 事件流 |
| `GET` | `/agentsight/stream/merged` | 合并 SSE 流 |
| `GET` | `/agentsight/stream/runner/:id` | 单 Runner SSE 流 |

### Research Processing v2 路由 (`/research`)

所有路由使用运行时访问 token。研究会话只保存归一化/脱敏后的研究视图与导出产物，任务通过单 worker 有界队列异步执行。
前端 `/research` 工作台封装了会话创建、`build_session`/`scan_recent`/`compare_windows`/`security_eval`/`export_bundle` 任务提交、事件/聚合结果浏览、安全评测、训练样本预览/导入和 JSONL/CSV/Bundle 下载；ML 页面也可直接从 Research Session 导入 128 维结构化训练样本。

任务运行时只按完成顺序以 O(1) 淘汰最旧终态历史；排队中和运行中的任务不会因保留上限被删除，因此高峰期 `trackedTotal` 可暂时超过上限。已取消的排队任务会保留到 worker 实际排空，取消若先于终态发布发生会覆盖成功/失败结果，且每个任务的完成统计只记一次。

Research 控制请求最大 64 KiB；会话最多保留 1024 个，单会话事件上限最大可配置为 100000。持久化文件通过 dirfd/`openat2` 限定在专用 research 目录，拒绝 symlink、硬链接和特殊文件并原子替换。`session.json`、`results.json`、`events.jsonl` 和单个 artifact 分别限制为 1/32/256/256 MiB；bundle 另限制单 payload 64 MiB、未压缩总量 256 MiB、ZIP 输出 128 MiB。

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/research/sessions` | 会话列表与摘要 |
| `POST` | `/research/sessions` | 创建研究会话，可带 sourceFilter/timeRange/tags |
| `GET` | `/research/sessions/:id` | 会话详情、summary、artifactRefs |
| `DELETE` | `/research/sessions/:id` | 删除会话和 artifacts |
| `POST` | `/research/sessions/:id/tasks` | 提交异步任务 (`scan_recent`/`build_session`/`compare_windows`/`security_eval`/`export_bundle`/`reset_session`) |
| `GET` | `/research/tasks/status` | 查询有界队列、任务保留量、完成/失败/取消/恐慌计数、拒绝原因及排队/运行耗时 |
| `GET` | `/research/tasks/:taskId` | 查询任务状态、进度、错误和 resultRef |
| `POST` | `/research/tasks/:taskId/cancel` | 幂等取消排队中或运行中的任务 |
| `GET` | `/research/sessions/:id/events` | 分页查询归一化 ResearchEvent |
| `GET` | `/research/sessions/:id/results` | 查询时间线、进程树、trace、top-K、loop/risk 关联结果与 securityEvaluation 安全评测报告；安全评测包含 `posture` pass/needs_review/critical、阻断项、warnings、suggestedActions 与 remediationPlan |
| `GET` | `/research/sessions/:id/training?format=json|jsonl|csv&labelPolicy=heuristic|decision|unlabeled` | 将会话事件结构化为训练样本：固定 128 维特征、标签策略、feature names、归一化报告 |
| `POST` | `/research/sessions/:id/training/import` | 将带标签训练样本导入 ML training store；默认使用 `decision` 标签策略以避免无决策事件被误标 |
| `GET` | `/research/sessions/:id/export?format=jsonl|csv|json|bundle|security-json|security-jsonl|security-csv` | 下载研究产物或安全评测明细；bundle 会包含 security-evaluation artifacts 与 manifest 哈希 |

### 插件路由 (`/plugins`)

需要 `FeaturePlugins` 编译特性。所有写操作还需要通过
`policyManagementEnabledMiddleware`；列表、详情和模板接口保持只读。

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/plugins` | 插件列表 |
| `GET` | `/plugins/` | 插件列表 (别名) |
| `POST` | `/plugins` | 创建插件 |
| `GET` | `/plugins/:id` | 插件详情 |
| `PUT` | `/plugins/:id` | 更新插件 |
| `DELETE` | `/plugins/:id` | 删除插件 |
| `POST` | `/plugins/:id/toggle` | 切换插件启用状态 |
| `POST` | `/plugins/visual/llm-compile` | 将自然语言编译为有界的可视化规则图 |

BPF 模板子路由 (`/plugins/bpf`):

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/plugins/bpf/templates` | BPF 模板列表 |
| `POST` | `/plugins/bpf/compile` | 编译用户 BPF 代码 |
| `POST` | `/plugins/bpf/load` | 加载 BPF 程序 |
| `POST` | `/plugins/bpf/unload` | 卸载 BPF 程序 |

在线编译会继承 HTTP 请求取消信号，并限制源码/诊断/对象大小、15 秒执行时间和
全局并发槽位。产物使用 FD 相对路径原子发布；加载前会校验 SHA-256、程序类型、
attach target、指令数、map 数量及估算内存，只实例化选定程序。Visual LLM 编译
同样具备请求/响应大小、并发和最长 120 秒超时限制。

### 数据管理 (`/data`)

| 方法 | 路径 | 用途 |
|------|------|------|
| `POST` | `/data/clear-events` | 清除内存事件 |
| `POST` | `/data/clear-events-memory` | 清除内存事件 |
| `POST` | `/data/clear-events-persisted` | 清除持久化事件 |

### 集群路由 (`/cluster`)

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/cluster/state` | 集群状态 |
| `GET` | `/cluster/nodes` | 集群节点列表 |

### MCP 协议 (`/mcp`)

| 方法 | 路径 | 用途 |
|------|------|------|
| `ANY` | `/mcp` | MCP 协议端点 |

## 兼容性路由 (`/api`)

### AgentSight 兼容 (`/api`)

需要 `FeatureAgentSight`:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/api/runners` | Runner 列表 |
| `GET` | `/api/events` | 事件导出 (强制 JSONL) |
| `POST` | `/api/events` | 上传事件 |
| `GET` | `/api/events/stats` | 事件统计 |
| `GET` | `/api/events/runners/:id/stats` | Runner 统计 |
| `POST` | `/api/events/query` | 高级查询 |
| `GET` | `/api/events/stream` | SSE 流 |
| `GET` | `/api/stream/merged` | 合并流 |
| `GET` | `/api/stream/runner/:id` | Runner 流 |

## 外部 API v1 (`/api/v1`)

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/api/v1/health` | 服务健康检查 |
| `GET` | `/api/v1/openapi.json` | OpenAPI 3.0.3 规范文档 |
| `GET` | `/api/v1/events/recent` | 近期事件 |
| `GET` | `/api/v1/events/graph` | 执行图谱 |
| `GET` | `/api/v1/research/sessions` | Research 会话列表 |
| `POST` | `/api/v1/research/sessions` | 创建 Research 会话 |
| `GET` | `/api/v1/research/sessions/:id` | Research 会话详情 |
| `DELETE` | `/api/v1/research/sessions/:id` | 删除 Research 会话 |
| `POST` | `/api/v1/research/sessions/:id/tasks` | 提交 Research 异步任务 |
| `GET` | `/api/v1/research/tasks/:taskId` | 查询 Research 任务 |
| `POST` | `/api/v1/research/tasks/:taskId/cancel` | 取消 Research 任务 |
| `GET` | `/api/v1/research/sessions/:id/events` | Research 事件分页 |
| `GET` | `/api/v1/research/sessions/:id/results` | Research 聚合结果 |
| `GET` | `/api/v1/research/sessions/:id/training` | Research 训练数据集导出 |
| `POST` | `/api/v1/research/sessions/:id/training/import` | Research 训练样本导入 |
| `GET` | `/api/v1/research/sessions/:id/export` | Research 导出下载 |
| `GET` | `/api/v1/agentsight/runners` | AgentSight runners |
| `GET` | `/api/v1/agentsight/events` | AgentSight 事件 |
| `POST` | `/api/v1/agentsight/events` | 上传事件 |
| `GET` | `/api/v1/agentsight/events.jsonl` | JSONL 导出 |
| `GET` | `/api/v1/agentsight/events/stats` | 事件统计 |
| `GET` | `/api/v1/agentsight/events/runners/:id/stats` | Runner 统计 |
| `POST` | `/api/v1/agentsight/events/query` | 高级查询 |
| `GET` | `/api/v1/agentsight/events/stream` | SSE 流 |
| `GET` | `/api/v1/agentsight/stream/merged` | 合并流 |
| `GET` | `/api/v1/agentsight/stream/runner/:id` | Runner 流 |
| `GET` | `/api/v1/network/flows` | 网络流 |
| `GET` | `/api/v1/network/flows/:flowID` | 按 ID 流 |
| `GET` | `/api/v1/network/dns-cache` | DNS 缓存 |
| `GET` | `/api/v1/network/interfaces` | 网卡计数器 |
| `GET` | `/api/v1/network/export/jsonl` | JSONL 导出 (FeatureNetworkExport) |
| `GET` | `/api/v1/sandbox/cgroup/status` | Cgroup 沙箱状态 (FeatureSandboxCgroup) |
| `GET` | `/api/v1/sandbox/lsm/status` | LSM 状态 (FeatureSandboxLSM) |
| `POST` | `/api/v1/policies/network/block-ip` | 阻断 IP |
| `POST` | `/api/v1/policies/network/unblock-ip` | 解除 IP 阻断 |
| `POST` | `/api/v1/policies/network/block-port` | 阻断端口 |
| `POST` | `/api/v1/policies/network/unblock-port` | 解除端口阻断 |
| `POST` | `/api/v1/policies/network/block-pid` | 阻断 PID |
| `POST` | `/api/v1/policies/network/unblock-pid` | 解除 PID 阻断 |
| `POST` | `/api/v1/policies/lsm/block-exec-path` | 阻断执行路径 |
| `POST` | `/api/v1/policies/lsm/unblock-exec-path` | 解除路径阻断 |
| `POST` | `/api/v1/policies/lsm/block-exec-name` | 阻断执行 basename |
| `POST` | `/api/v1/policies/lsm/unblock-exec-name` | 解除 basename 阻断 |
| `POST` | `/api/v1/policies/lsm/block-file-name` | 阻断文件名 |
| `POST` | `/api/v1/policies/lsm/unblock-file-name` | 解除文件名阻断 |
| `POST` | `/api/v1/agents/register` | 注册 Agent PID |
| `POST` | `/api/v1/agents/unregister` | 注销 Agent PID |
| `GET` | `/api/v1/config/export` | 导出配置 |
