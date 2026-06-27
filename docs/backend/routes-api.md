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
| `GET` | `/config/ml/status` | ML 引擎状态 |
| `GET` | `/config/ml/logs` | 训练日志 |
| `GET` | `/config/ml/history` | 训练历史 |
| `POST` | `/config/ml/train` | 触发训练 |
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
| `GET` | `/config/ml/datasets/export` | 导出数据集 |
| `DELETE` | `/config/ml/datasets` | 清空数据集 |
| `GET` | `/config/ml/health/processes` | 健康监测进程 |
| `GET` | `/config/ml/health/generators` | 健康监测生成器 |
| `POST` | `/config/ml/health/register` | 注册健康监测 |
| `POST` | `/config/ml/health/unregister` | 注销健康监测 |
| `POST` | `/config/ml/health/run` | 运行健康检查 |
| `POST` | `/config/ml/backtest` | 回测 (同 assess) |

### Hook 配置路由 (`/config/hooks`)

需要 `FeatureHooks` 编译特性:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/config/hooks` | 可用 Hook 列表 |
| `POST` | `/config/hooks` | 安装/卸载 Hook |
| `GET` | `/config/hooks/:id/raw` | 读取原始配置 |
| `POST` | `/config/hooks/:id/raw` | 写入原始配置 |

### 系统路由 (`/system`)

| 方法 | 路径 | 用途 | 特性门控 |
|------|------|------|---------|
| `GET` | `/system/features` | 特性清单 | -- |
| `GET` | `/system/sensors` | 传感器列表 | -- |
| `GET` | `/system/benchmark` | 运行基准测试 | -- |
| `GET` | `/system/benchmark/results` | 基准测试结果 | -- |
| `POST` | `/system/run` | 运行系统命令 | `FeatureSystemRun` |
| `GET` | `/system/otel-health` | OTel 导出健康状态 | `FeatureOTLP` |
| `GET` | `/system/file/preview` | 文件预览 | -- |
| `POST` | `/system/clear-events` | 清除全部事件 | -- |

### TLS 捕获路由 (`/tls-capture`)

需要 `FeatureTLSCapture` 编译特性:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/tls-capture/status` | 捕获状态 |
| `PUT` | `/tls-capture/status` | 启用/禁用捕获 |
| `GET` | `/tls-capture/rules` | 规则列表 |
| `POST` | `/tls-capture/rules` | 创建规则 |
| `DELETE` | `/tls-capture/rules/:id` | 删除规则 |
| `POST` | `/tls-capture/attach-executable` | 附加可执行文件 |

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

### 插件路由 (`/plugins`)

需要 `FeaturePlugins` 编译特性:

| 方法 | 路径 | 用途 |
|------|------|------|
| `GET` | `/plugins` | 插件列表 |
| `POST` | `/plugins` | 创建/更新插件 |
| `GET` | `/plugins/:id` | 插件详情 |
| `DELETE` | `/plugins/:id` | 删除插件 |
| `POST` | `/plugins/:id/enable` | 启用插件 |
| `POST` | `/plugins/:id/disable` | 禁用插件 |
| `POST` | `/plugins/:id/reload` | 重新加载插件 |
| `GET` | `/plugins/templates` | BPF 模板列表 |

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
