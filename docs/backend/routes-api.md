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
| `GET` | `/events/recent` | 近期捕获事件 (支持 limit/type/pid/comm 查询参数) |
| `GET` | `/events/graph` | 聚合执行图谱 (支持 agent_run_id/trace_id/since/until) |
| `GET` | `/events/recording` | 录制状态 |
| `POST` | `/events/recording/start` | 启动事件录制 |
| `POST` | `/events/recording/stop` | 停止事件录制 |
| `POST` | `/events/recording/replay` | 回放录制 |
| `POST` | `/events/recording/browser/save` | 保存浏览器录制 |

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
| `POST` | `/network/export-pcap` | PCAP 导出 (FeatureNetworkExport) |
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

`/config/ml/datasets/pull` 与 `/config/ml/datasets/import` 支持 `json`、`jsonl`、`csv`、`tsv`、纯文本与常见压缩包；纯文本 `.te`/SELinux policy 规则以及 JSON `rules[].rule` / `rules[].selinuxRule` 字段会自动识别为 `selinux-rule ...` 训练样本，并按 `allow/type_transition=ALLOW`、`neverallow=BLOCK`、`dontaudit/auditallow/permissive=ALERT` 保留来源标签。响应会附带 `byLabel`、`byCategory`、`bySource`、`normalization`、`quality`，导入响应还会附带 `skipReasons`；压缩包中被跳过的条目或 limit 截断会出现在 `parseWarnings`。

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

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/system/ls` | 目录列表 |
| `GET` | `/system/file-preview` | 文件预览 |
| `GET` | `/system/file-preview/stream` | 文件预览流式输出 |
| `GET` | `/system/file-hex` | 文件十六进制查看 |
| `GET` | `/system/file-elf` | ELF 文件分析 |
| `GET` | `/system/home` | 用户主目录 |
| `GET` | `/system/download` | 文件下载 |
| `POST` | `/system/upload` | 文件上传 |
| `POST` | `/system/benchmark` | 运行基准测试 |
| `GET` | `/system/benchmark` | 基准测试结果 |
| `GET` | `/system/tracked-comms` | 已追踪命令列表 |
| `POST` | `/system/process/signal` | 发送进程信号 |
| `GET` | `/system/process/maps` | 进程内存映射 |
| `GET` | `/system/sensors` | 传感器列表 |
| `GET` | `/system/cameras` | 摄像头列表 |
| `GET` | `/system/camera/snapshot` | 摄像头快照 |
| `GET` | `/system/microphones` | 麦克风列表 |
| `GET` | `/system/signals/status` | 信号处理运行态：队列、TTL 状态、最近信号、可用 signal kinds |
| `POST` | `/system/signals/task` | 信号处理后台任务：`scan_recent`/`expire`/`reset` |
| `POST` | `/system/signals/rules/test` | 用最近事件 dry-run 单条 signal rule，返回匹配样本 |
| `GET` | `/system/signals/program-logs` | 选中程序的本地 protobuf 压缩日志状态、路径、frame 数 |
| `GET` | `/system/signals/program-logs/download?program=...` | 下载某个已配置选中程序的 length-framed gzip protobuf 日志 |

`/system/signals/status` 对应 `RuntimeSettings.signalProcessing`。信号规则支持 `path_access`、`child_process`、`repeated_read` 与 `custom` kind，规则内条件按 AND 组合；选中的程序会由后端写入本地 length-framed gzip protobuf (`ProgramSignalLogRecord`) 二进制日志。

### TLS 捕获路由 (`/tls-capture`)

需要 `FeatureTLSCapture` 编译特性:

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

### AgentSight 路由 (`/agentsight`)

需要 `FeatureAgentSight` 编译特性:

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

需要 `FeaturePlugins` 编译特性:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/plugins` | 插件列表 |
| `GET` | `/plugins/` | 插件列表 (别名) |
| `POST` | `/plugins` | 创建插件 |
| `GET` | `/plugins/:id` | 插件详情 |
| `PUT` | `/plugins/:id` | 更新插件 |
| `DELETE` | `/plugins/:id` | 删除插件 |
| `POST` | `/plugins/:id/toggle` | 切换插件启用状态 |

BPF 模板子路由 (`/plugins/bpf`):

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/plugins/bpf/templates` | BPF 模板列表 |
| `POST` | `/plugins/bpf/compile` | 编译用户 BPF 代码 |
| `POST` | `/plugins/bpf/load` | 加载 BPF 程序 |
| `POST` | `/plugins/bpf/unload` | 卸载 BPF 程序 |

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
