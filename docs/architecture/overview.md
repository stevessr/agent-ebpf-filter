# 总体架构

Agent eBPF Filter 采用 L0-L5 六层分层架构设计，从产品战略目标向下延伸，贯穿运行时边界、通信协议、后端领域引擎、前端工作台，直至底层工程交付流水线。

## L0: 产品目标层

核心目标是为 AI Agent、开发者 CLI、本地自动化脚本提供完整的 OS 级别可观测、可关联、可约束、可导出的行为证据链。

```
AI Agent / CLI  -->  捕获 OS 行为事实  -->  深度因果关联  -->  运行时多层控制
                                    -->  开放式审计导出
```

## L1: 运行时边界层

系统运行时由七个核心边界组件构成：

| 运行时边界 | 核心组件 | 职责 |
|-----------|---------|------|
| 内核采集面 | eBPF Tracepoints, Ring Buffer, Pinned BPF Maps | 零侵入捕获底层系统调用事实 |
| 内核控制面 | cgroup eBPF, BPF LSM | 网络流量、文件系统及进程执行的硬阻断 |
| 后端控制面 | Go Backend, Gin Engine, WebSocket Server | 高频事件汇聚、状态管理、策略下发、动态认证 |
| 命令策略面 | agent-wrapper, Unix Domain Socket (UDS) | 拦截应用层指令，执行 ALLOW/BLOCK/ALERT/REWRITE 决策 |
| Agent 语义面 | Process Adapters, Native Hooks Relay | 绑定 PID 信任树，注入 Tool Metadata 及大模型 Trace 上下文 |
| 前端工作台 | Vue 3, Vite, Pinia | 实时事件流、因果拓扑图谱、策略可视化编排 |
| 外部生态接口 | MCP Server, OTLP Exporter, Prometheus Metrics | 对接 AI IDE、分布式链路追踪及标准监控看板 |

## L2: 协议与事件层

通信数据结构收敛于 `proto/` 目录，通过强类型 Schema 保证跨语言一致性：

### Protocol Buffer 核心定义

- `tracker.proto`: 顶层网关桩，聚合全量子模块
- `tracker_common.proto`: 通用元数据、进程树凭证及基础类型
- `tracker_events.proto`: 统一事件模型，系统调用与网络事件
- `tracker_registration.proto`: AI Agent PID 树注册协议
- `tracker_config.proto`: 运行时门控、安全阻断字典及 ML 评分配置
- `tracker_shell.proto`: PTY 交互式 Shell 会话流式通信
- `tracker_system.proto`: 宿主机 CPU、内存、GPU 遥测指标

### 多语言自动生成

- Go Backend: `backend/pb/*.pb.go`
- Vue Frontend: `frontend/src/pb/*`
- Python Adapter: `adapters/python/`
- Node.js Adapter: `adapters/js/`

## L3: 后端领域层

后端采用领域驱动设计（DDD），核心引擎收敛于 `backend/app/` 及 `backend/` 下级子包。重构后的目录结构如下：

### 应用入口与组装层 (`backend/app/`)

| 模块 | 路径 | 职责 |
|------|------|------|
| 系统引导 | `main.go` | 特权校验、环境基架审计、服务平滑引导 |
| 依赖注入容器 | `appcontext.go` | 全局 AppContext 聚合所有子系统管理器 |
| 路由注册 | `routes.go` | REST 接口、WebSocket、认证门禁 |
| 特性矩阵 | `feature_manifest.go`, `feature_registry.go` | 编译期/运行期 Feature Gate 管理 |
| 后台任务 | `jobs_background.go` | Ringbuf Reader 与后台审计消费协程 |
| 类型桥接 | `typebridge.go` | 跨包类型别名（app/tls, app/network, app/runtime） |

### 子包分层结构

| 子包 | 路径 | 核心职责 |
|------|------|---------|
| `handlers/` | `backend/app/handlers/` (24 文件) | HTTP 路由 handler，按功能模块拆分 |
| `events/` | `backend/app/events/` | 事件归一化、语义告警、上下文组装、Kernel Risk |
| `network/` | `backend/app/network/` | 网络流 TCP/DNS/GeoIP/带宽聚合与分析 |
| `tls/` | `backend/app/tls/` | TLS 明文捕获、HTTP/SSE 解析、AI 元数据富化 |
| `observability/` | `backend/app/observability/` | 采集器指标、Prometheus 导出 |
| `shell/` | `backend/app/shell/` | Shell 会话生命周期管理 |
| `sandbox/` | `backend/app/handlers/cgroup_sandbox.go`, `backend/app/handlers/lsm_enforcer.go`, `backend/internal/sandbox/` | cgroup/BPF LSM 沙箱管控 |
| `platform/` | `backend/app/platform/` | 宿主平台抽象（uid/gid、文件系统操作） |
| `types/` | `backend/app/types/` | 特性 ID、插件类型等共享常量定义 |
| `runtime/` | `backend/app/runtime/` | 运行时配置持久化与状态管理 |
| `export/` | `backend/app/export/` | OTel 导出、PCAP 导出 |

### handlers/ 子包结构

```
backend/app/handlers/
  agentsight.go        AgentSight 兼容 API (事件导出/查询/流/上传/统计)
  api_external.go      外部 API (health/openapi)
  benchmark.go         基准测试
  cgroup_sandbox.go    Cgroup 网络沙箱 (9 handler)
  config.go            配置 CRUD (tags/comms/paths/rules)
  data.go              数据管理
  deps.go              依赖注入定义 (300+ 行接口与类型)
  doc.go               包文档
  enrichment.go        网络流/TCP/DNS (9 handler)
  exportconfig.go      配置导入导出
  feature_manifest.go  特性清单
  hardware.go          摄像头/传感器/麦克风
  hooksconfig.go       Hook 配置
  lsm_enforcer.go      BPF LSM 执行控制 (7 handler)
  ml_ws.go             ML WebSocket 状态推送
  mlconfig.go          ML 训练/评估/采样
  native_hook.go       AI CLI 原生钩子
  plugin.go            插件注册
  registration.go      PID 注册
  runtimeconfig.go     运行时配置
  shell_sessions.go    Shell 会话管理 (6 handler)
  stats.go             系统统计 WebSocket
  system.go            系统操作
```

### 桥接模式

子包通过 **桥接文件** 与 app 层解耦：

| 桥接文件 | 桥接目标 | 模式 |
|---------|---------|------|
| `handlersbridge.go` | `handlers/` | Dep 闭包 + 适配器接口 + Fat Bridge |
| `eventsbridge.go` | `events/` | Deps 结构体注入 |
| `networkflowsbridge.go` | `events/` + `network/` | 函数指针注入 |
| `shellbridge.go` | `shell/` | 全局变量兼容 |
| `observabilitybridge.go` | `observability/` | 接口适配 |
| `collectormetricsbridge.go` | metrics 采集 | 桥接适配 |
| `contextbridge.go` | 进程上下文 | 函数桥接 |

### 内部基础设施

| 包 | 路径 | 职责 |
|---|------|------|
| `ebpf/` | `backend/ebpf/` | eBPF C 源码与 bpf2go 生成的 Go 绑定 |
| `pb/` | `backend/pb/` | 自动生成的 Protobuf Go 桩代码 |
| `internal/network/` | `backend/internal/network/` | 网络核心算法（Flow/TCP/DNS/Scope） |
| `internal/behavior/` | `backend/internal/behavior/` | 行为分类与向量嵌入 |
| `internal/geoip/` | `backend/internal/geoip/` | GeoIP 解析与风险国家判定 |
| `ml/` | `backend/ml/` | 机器学习模型定义与训练器 |
| `redaction/` | `backend/redaction/` | 数据脱敏引擎 |
| `probe/manager/` | `backend/probe/manager/` | TLS 探针管理器 |

### 专用后端服务

| 服务 | 路径 | 职责 |
|------|------|------|
| agent-wrapper | `wrapper/main.go` | CLI 命令拦截与策略执行 |
| SSL 节点测试 | `backend/cmd/test_node_ssl_attach/` | SSL/TLS 库探测 |
| AI 工具扫描 | `backend/cmd/scan_ai_tools/` | AI CLI 工具发现 |

## L4: 前端领域层

前端引擎位于 `frontend/src/`，基于 Vue 3 Composition API、`<script setup lang="ts">` 语法与 Vite 构建：

```
frontend/src/
  main.ts              全局入口
  App.vue              根容器
  router/index.ts       动态路由与权限守护
  views/                视窗级容器 (Dashboard, Network, Graph, Config, AgentSight)
  components/           原子级/块级可复用组件
  composables/          业务逻辑抽离 (API, WS, 状态机)
  types/                前端视图类型定义
  utils/                纯函数与工具集
  pb/                   [自动生成] Protobuf TS 桩
```

## L5: 工程交付层

| 分类 | 核心入口 | 职责 |
|------|---------|------|
| 统一编译 | `Makefile` | 全量及分项组件构建 |
| 开发就绪 | `make predev`, `make dev` | 依赖拉取，热加载引擎 |
| 容器沙箱 | `.devcontainer/`, `make exec` | 非 Linux 主机开发环境 |
| 系统集成 | `make install`, `scripts/install-service.sh` | Systemd 守护进程注册 |
| 云原生 | `deploy/kubernetes/` | 特权 DaemonSet 集群监控 |
| 文档体系 | `docs/` (VitePress) | 技术文档站 |

## 分层依赖视图

```
L0: 产品目标层 (可观测/可关联/可约束/可导出)
    |
L1: 运行时边界层 (内核控制/后端控制/命令策略/Agent 语义/UI)
    |
L2: 协议与事件层 (proto 定义 -> 多语言代码生成)
    |
L3: 后端领域层 (启动引导/动态状态/零拷贝管线/拓扑图谱/内核 C)
    |
L4: 前端领域层 (视图容器/原子组件/组合式状态机)
    |
L5: 工程交付层 (Makefile/K8s/Systemd/VitePress)
```

## 相关导航

- [数据流](data-flow.md) -- 内核 Ringbuf 解码至前端渲染时序
- [运行时边界](runtime-boundaries.md) -- 各组件安全防护隔离
- [协议与事件模型](protocol-events.md) -- Protobuf 序列化与事件封包
- [后端 API 路由参考](../backend/routes-api.md) -- 完整 HTTP/WS API 索引
- [运行时启动流程](../backend/runtime-startup.md) -- Main 函数引导序列
- [TLS 快速入门](../backend/TLS_QUICKSTART.md) -- TLS 明文捕获配置指南
