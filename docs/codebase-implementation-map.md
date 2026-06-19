# 代码深挖实现地图

> 项目：Agent eBPF Filter  
> 生成日期：2026-06-19  
> 用途：把当前代码实现中的关键入口、真实链路、跨层同步点和文档维护边界整理成一份“从代码出发”的项目文档，供答辩、开发交接、审查和后续重构使用。

---

## 1. 阅读范围与结论摘要

本次阅读覆盖以下代码与文档入口：

- 构建与运行：`Makefile`、`CLAUDE.md`、`AGENTS.md`。
- 后端启动与路由：`backend/app/main.go`、`backend/app/routes__routes.go`。
- 后端运行时状态：`backend/core/state_types.go`、`backend/app/feature_manifest.go`、`backend/app/runtime__jobs_background.go`。
- 事件上下文：`backend/app/events__context_event.go`、`proto/tracker_events.proto`。
- 前端路由：`frontend/src/router/index.ts`。
- wrapper：`wrapper/main.go`。
- 项目级导航：`.claude/skills/project-structure/references/*.md`。
- 现有文档：`README.md`、`docs/project-structure-deep-dive.md`、`docs/project-docs-index.md`、`docs/architecture.md`、`docs/security-model.md`、`docs/threat-model.md` 等。

总体结论：

1. 项目已经从早期“`backend/*.go` 平铺”演进为更明显的 `backend/app/`、`backend/core/`、`backend/ebpf/` 等分层结构；部分历史文档仍用旧文件名表达，需要在后续维护中同步到新路径。
2. 系统主线是 **eBPF 事实采集 → Go 后端归一化与控制 → Vue 工作台展示 → wrapper / hooks / adapters 形成 Agent 语义关联 → cgroup / LSM / policy 形成控制闭环**。
3. 高风险能力已经通过 **build feature、runtime feature gate、release-mode auth、脱敏、默认关闭** 多层边界约束，文档中必须避免把诊断能力描述成默认采集或默认阻断。
4. 项目文档应分成三类维护：
   - 面向用户 / 答辩的产品文档；
   - 面向开发者的代码导航文档；
   - 面向审查的安全、策略、协议和验证文档。

---

## 2. 当前实现的顶层运行链

### 2.1 后端启动链

当前后端主入口位于 `backend/app/main.go`，不是旧文档中常见的 `backend/main.go`。

启动顺序可概括为：

```text
backend/app.Main()
  → bootstrap mode 检查
  → ensureBackendPrivileges()
  → runtimeSettingsStore.LoadOrCreate()
  → killPreviousBackendProcesses()
  → ensureTrackerMapsLoaded()
  → newFeatureRegistry()
  → domain forward / TLS runtime 初始化
  → ringbuf.NewReader(trackerMaps.Events)
  → startKernelEventReader()
  → startRuntimeBackgroundJobs()
  → ApplySandbox()
  → gin.Default()
  → registerRoutes()
  → seedDefaultTrackedCommands()
  → chooseBackendPort()
  → configureRuntimePort()
  → deferred ML / plugin runtime
  → r.Run(:port)
```

这条链路体现了几个关键边界：

- eBPF 组件必须先完成权限和 pinned map 初始化；
- runtime settings 在路由和后台任务前加载；
- TLS、ML、Plugins、Sandbox、Domain forward 均受 build feature / runtime settings 影响；
- 端口通过 `chooseBackendPort()` 选择，并写入运行时端口 handoff 文件，供前端 dev proxy 和本地集成读取。

### 2.2 后台任务链

`backend/app/runtime__jobs_background.go` 负责把内核事件和后台服务串起来：

```text
startKernelEventReader(ringbuf.Reader)
  → rd.Read()
  → decodeBPFEventRecord()
  → self PID / disabled comm / disabled event type 过滤
  → buildKernelEventFromRaw()
  → broadcast channel

startRuntimeBackgroundJobs(features)
  → startEventBroadcaster()
  → startKernelRiskFeedbackWorker()
  → startUDSServer(broadcast)
  → cgroup / DNS / TCP / flow GC
  → exfil detection loop
  → GeoIP init
  → optional cgroup sandbox loader
  → optional LSM enforcer loader
```

这里的重点是：

- ringbuf 解码走 mmap-backed zero-copy fast path，必要时回退到 `binary.Read`；
- 内核事件不会直接进 UI，而是先进入 backend broadcast / archive / envelope 处理；
- UDS wrapper server 是后台任务的一部分，和 kernel event broadcaster 共享事件输出；
- cgroup 和 LSM enforcement 是可选加载，失败时记录 warning，而不是让普通观测链路完全不可用。

---

## 3. 路由与 API 分层

当前路由总入口是 `backend/app/routes__routes.go`。

### 3.1 路由注册顺序

```text
registerRoutes()
  → registerWebSocketRoutes()
  → registerShellSessionRoutes()
  → registerEventRoutes()
  → registerNetworkRoutes()
  → registerSandboxRoutes()
  → registerUtilityRoutes()
  → registerAuthenticatedAPIRoutes()
  → registerCompatibilityRoutes()
  → registerStaticRoutes()
```

### 3.2 主要路由域

| 路由域 | 代表路径 | 安全 / feature 约束 | 主要用途 |
| --- | --- | --- | --- |
| WebSocket | `/ws`、`/ws/system`、`/ws/envelopes`、`/ws/events/graph`、`/ws/tls-capture` | `authMiddleware()`；TLS/ML 受 compiled feature 影响 | 实时事件、系统指标、执行图、TLS 流 |
| Shell sessions | `/shell-sessions`、`/ws/shell` | `authMiddleware()` + `shellSessionsEnabledMiddleware()` | PTY session 生命周期与交互 |
| Events | `/events/recent`、`/events/graph`、`/events/recording/**` | `authMiddleware()` | 事件查询、执行图、录制回放 |
| Network | `/network/flows`、`/network/analyze`、`/network/interfaces`、`/network/export*` | `authMiddleware()`；export 受 `FeatureNetworkExport` | 网络流、TCP/DNS/GeoIP、导出 |
| Sandbox | `/sandbox/cgroup/**`、`/sandbox/lsm/**` | `authMiddleware()`；mutation 还需 policy management gate | cgroup / BPF LSM 状态与策略变更 |
| Utility | `/metrics`、`/hooks/event`、`/register`、`/unregister` | metrics/register/unregister 走 auth；hook 走 hook ingress auth | Prometheus、hooks、PID 注册 |
| Authenticated API | `/config/**`、`/system/**`、`/tls-capture/**`、`/agentsight/**`、`/plugins/**`、`/mcp` | 统一 `authMiddleware()` | 配置、系统操作、TLS、MCP、插件 |
| Compatibility | `/api/**`、`/api/v1/**` | `authMiddleware()` | AgentSight / external API 兼容 |
| Static | `/`、`/assets`、fallback | 生产静态资源 | 前端 dist |

### 3.3 文档维护提示

如果新增路由，文档中应同时回答：

1. 它属于哪个 route group？
2. 是否 release mode 需要 token？
3. 是否需要 runtime feature gate？
4. 是否需要同步 `frontend/src/composables/**`？
5. 是否属于外部 API，需更新 `docs/external-api.md`？
6. 是否涉及危险能力，需更新 `docs/security-model.md` / `docs/threat-model.md` / `docs/policy-semantics.md`？

---

## 4. 事件、协议与上下文

### 4.1 protobuf 事件字段

`proto/tracker_events.proto` 的 `Event` 已经不只是 syscall 基础字段，而是统一事件事实模型。关键字段包括：

- 进程基础：`pid`、`ppid`、`uid`、`gid`、`tgid`、`comm`、`path`。
- 网络：`net_direction`、`net_endpoint`、`flow_id`、`src_ip`、`src_port`、`dst_ip`、`dst_port`、`transport`、`app_protocol`、`dns_name`、`sni`、`http_host`、`tls_alpn`、`geo_*`、`ip_scope`。
- Agent 语义：`root_agent_pid`、`agent_run_id`、`conversation_id`、`turn_id`、`tool_call_id`、`tool_name`、`trace_id`、`span_id`、`task_id`、`cwd`。
- 策略与风险：`decision`、`risk_score`、`behavior`。
- 隐私与脱敏：`argv_digest`、`redaction_level`、`sanitized_fields`。
- 兼容和性能：`schema_version`、`duration_ns`、`first_seen_ms`、`last_seen_ms`。

这说明项目的核心事件模型已经从“系统调用日志”升级为“可关联 Agent 行为、网络流、策略决策和脱敏状态的统一事实记录”。

### 4.2 process context

`backend/app/events__context_event.go` 维护 `processContext`，它把注册、wrapper、hook 中的上下文归一化到事件上：

```text
register payload / wrapper request / hook payload
  → buildProcessContextFromRegister()
  → buildProcessContextFromWrapperRequest()
  → buildProcessContextFromHookPayload()
  → normalizeProcessContext()
  → enrichEventContext()
  → pb.Event / EventEnvelope
```

关键事实：

- `RootAgentPid` 为空时会 fallback 到当前 pid；
- `Decision` 会规范成大写；
- `RiskScore` 负数会归零；
- `ArgvDigest` 用 `\x00` 分隔字段后做 SHA-256，避免直接保存完整 argv；
- hook payload 会兼容 snake_case 和 camelCase 字段。

### 4.3 新事件字段同步链

新增或修改事件字段时，不应只改一层：

```text
proto/tracker_events.proto
  → make proto
  → backend event construction / context enrichment
  → network_events / execution_graph / AgentSight / OTLP
  → frontend generated pb + types / filters / table / modal
  → docs / tests
```

---

## 5. Runtime settings、feature manifest 与安全边界

### 5.1 RuntimeSettings

`backend/core/state_types.go` 中 `RuntimeSettings` 覆盖：

- 日志持久化：`LogPersistenceEnabled`、`LogFilePath`、`MaxEventCount`、`MaxEventAge`。
- access token：`AccessToken`。
- 危险能力 gate：`ShellSessionsEnabled`、`SystemRunEnabled`、`HookManagementEnabled`、`PolicyManagementEnabled`、`TlsCaptureEnabled`、`OtlpEnabled`。
- OTLP：`OtlpEndpoint`、`OtlpServiceName`、`OtlpHeaders`。
- hooks：`HookSecrets`。
- ML：`MLConfig`。
- kernel feedback：`KernelRiskFeedback`。
- domain forward：`DomainForwardProxy`。

### 5.2 Feature manifest

`backend/app/feature_manifest.go` 把 build feature、runtime gate、auth、route prefixes 和危险级别统一成 manifest。当前关键 feature 包括：

| Feature | Runtime gate | Danger level | 代表 route |
| --- | --- | --- | --- |
| `shell_sessions` | `shell_sessions` | high | `/shell-sessions`、`/ws/shell` |
| `system_run` | `system_run` | critical | `/system/run` |
| `hooks` | `hook_management` | high | `/hooks/event`、`/config/hooks` |
| `policy_management` | `policy_management` | high | `/config/rules`、`/sandbox`、`/api/v1/policies` |
| `tls_capture` | `tls_capture` | critical | `/tls-capture`、`/ws/tls-capture`、`/codex/capture` |
| `otlp` | `otlp` | medium | `/system/otel-health` |
| `domain_forward` | `domain_forward` | high | `/system/domain-forward/status` |
| `ml` | ML config enabled | medium | `/config/ml`、`/ws/ml-status` |
| `plugins` | compiled/runtime availability | high | `/plugins` |
| `sandbox_cgroup` | compiled availability | high | `/sandbox/cgroup` |
| `sandbox_lsm` | compiled availability | high | `/sandbox/lsm` |
| `network_export` | compiled availability | medium | `/network/export` |
| `agentsight` | compiled availability | low | `/agentsight`、`/api/v1/agentsight` |

文档中要避免把 `compiledIn` 和 `runtimeEnabled` 混为一谈：

- build feature 决定代码是否编进二进制；
- runtime setting 决定危险能力是否在运行时启用；
- release-mode auth 决定敏感 API 是否需要 runtime access token。

---

## 6. eBPF、cgroup、LSM 和匹配语义

### 6.1 主 tracker

主 eBPF 追踪程序在 `backend/ebpf/agent_tracker.c`，通过 ringbuf 把选定 syscall 事件交给 Go 后端。当前产品文档应维持以下表述：

- 支持核心 syscall：`execve`、`openat`、`connect`、`mkdirat`、`unlinkat`、`ioctl`、`bind`、`sendto`、`recvfrom`。
- 事件包含 syscall exit duration，可用于 strace-style UI 展示。
- tracked PID / comm / path 是过滤和标记入口，不代表完整 sandbox。

### 6.2 cgroup sandbox

`backend/ebpf/cgroup_sandbox.c` 实现 cgroup connect / UDP sendmsg 级网络阻断。

准确语义：

- cgroup blocklist 基于 cgroup v2 inode id；
- destination blocking 是 exact IPv4 / exact IPv6 / exact TCP/UDP port；
- IPv4-mapped IPv6 输入会归一化到 IPv4 key；
- 不应写成 CIDR、range、递归网络策略。

### 6.3 BPF LSM enforcer

`backend/ebpf/lsm_enforcer.c` 实现 exec / file / directory 相关 BPF LSM enforcement。

准确语义：

- exec policy 支持 exact path 或 executable basename；
- file-name policy 是 basename-based；
- 覆盖 open、permission、mmap、mprotect、setattr、create、link、symlink、unlink、mkdir、rmdir、mknod、rename 等 LSM hooks；
- policy maps 应保持 restrictive permissions，经 authenticated backend API 修改。

---

## 7. Wrapper、hooks 与 adapters

### 7.1 Wrapper

`wrapper/main.go` 的实际行为：

```text
agent-wrapper <command> [args...]
  → 清理空白参数
  → 连接 /tmp/agent-ebpf.sock，超时 500ms
  → 发送 pb.WrapperRequest
  → 等待 pb.WrapperResponse，deadline 2s
  → BLOCK: 打印并退出 1
  → ALERT: 打印告警后继续
  → REWRITE: 替换 command / args
  → syscall.Exec(final command)
```

Wrapper 同时会把 Agent 语义环境变量写入 request：

- `AGENT_EBPF_AGENT_RUN_ID` / `AGENT_RUN_ID`
- `AGENT_EBPF_TASK_ID` / `AGENT_TASK_ID`
- `AGENT_EBPF_TOOL_CALL_ID` / `AGENT_TOOL_CALL_ID`
- `AGENT_EBPF_TRACE_ID` / `TRACE_ID`
- `AGENT_EBPF_RISK_SCORE` / `AGENT_RISK_SCORE`
- `AGENT_EBPF_CWD` / `PWD`

文档中应把 wrapper 描述为 **命令 shim / policy layer**，不要描述成完整 shell sandbox。

### 7.2 Native hooks

Native hooks 通过 CLI-specific relay script 使用 `curl POST /hooks/event`。它们的价值是补充 Agent 语义和 tool metadata，而不是替代 eBPF 的事实采集。

安全事实：

- hook install / raw hook writes 是危险能力；
- `/hooks/event` 使用 runtime token 或 per-hook secret；
- relay script 依赖 host `curl`；
- callback endpoint 可由 `.port` 推导或 `AGENT_HOOK_ENDPOINT` 覆盖。

### 7.3 Adapters

Python / JS adapters 的职责是注册当前进程 PID。文档中要准确说明：

- PID registration 是 per-process；
- 子进程关联依赖 fork/clone lineage 和 userspace parent fallback；
- adapters 不是自动递归注册全部后代进程的守护程序。

---

## 8. 前端工作台结构

当前前端路由在 `frontend/src/router/index.ts`，路由级 feature gate 会在构建裁剪后把不可用功能导向 `FeatureUnavailable`。

| Route | View | Feature meta | 用途 |
| --- | --- | --- | --- |
| `/dashboard/:tab?` | `views/dashboard/Dashboard.vue` | 无 | 实时事件流 |
| `/monitor/:tab?/:subtab?` | `views/monitor/Monitor.vue` | 无 | 系统与进程指标 |
| `/network` | `views/network/Network.vue` | 无 | syscall-derived network events |
| `/network-flow/:tab?` | `views/network/NetworkFlow.vue` | 无 | flow table / details / graph |
| `/tls-capture` | `views/network/TLSCapture.vue` | `tls_capture` | TLS / Codex capture |
| `/agentsight/:tab?` | redirect `ExecutionGraph` behavior | 无 | AgentSight 兼容入口 |
| `/execution-graph/:tab?` | `views/execution-graph/ExecutionGraph.vue` | 无 | Agent / process / tool / syscall 图谱 |
| `/explorer` | `views/explorer/Explorer.vue` | 无 | 文件浏览与 tracked paths |
| `/executor/:tab?/:subtab?` | `views/executor/Executor.vue` | `shell_sessions` | PTY / launcher / wrapper terminal |
| `/hooks` | `views/hooks/Hooks.vue` | `hooks` | AI CLI hook 管理 |
| `/ml/:subtab?` | `views/ml/ML.vue` | `ml` | ML status、training、LLM scoring |
| `/plugins/:tab?` | `views/plugins/Plugins.vue` | `plugins` | eBPF plugins / visual builder |
| `/config/:tab?/:subtab?/:subsubtab?` | `views/config/Config.vue` | 无 | runtime、security、registry、docs |

前端维护原则：

- `views/` 是页面容器；
- `components/<domain>/` 放 UI 子块；
- `composables/<domain>/` 放 API、WebSocket、状态、转换逻辑；
- `types/` 放共享类型；
- `data/` 放 hook / Linux docs / ML model catalog；
- `frontend/src/pb/` 是生成物，必须由 `make proto` 更新。

---

## 9. 构建与验证地图

`Makefile` 是唯一推荐的统一入口。

### 9.1 构建入口

| 目标 | 作用 |
| --- | --- |
| `make predev` | 并行安装 Go / Python / frontend / TUI 开发依赖并检查 |
| `make dev` | 启动 backend hot-reload + frontend dev server 的 Zellij 会话 |
| `make backend` | go generate eBPF + build backend |
| `make frontend` | Bun install + `vue-tsc -b && vite build` |
| `make wrapper` | build `agent-wrapper` |
| `make proto` | 从 `proto/*.proto` 生成 Go / JS / Python 绑定 |
| `make all` | proto + backend + frontend + wrapper |
| `make install` | production build 并安装 service / binaries |
| `make exec` | 启动 privileged devcontainer |

### 9.2 最小验证

| 改动 | 最小验证 |
| --- | --- |
| docs only | 检查 Markdown 文件与链接目标 |
| Go backend | `cd backend && go test ./...` |
| wrapper | `cd wrapper && go test ./...` |
| frontend | `cd frontend && bun run build` |
| proto | `make proto`，再 backend/frontend 验证 |
| main eBPF tracker | `cd backend/ebpf && go generate` + `cd backend && go build ./...` |
| cgroup / LSM | `make ebpf-cgroup` / `make ebpf-lsm` |
| runtime replay / ML | `make runtime-benchmark` 或相关 `scripts/ml-*.sh` |

---

## 10. 文档体系建议

### 10.1 推荐文档树

| 层级 | 文档 | 维护重点 |
| --- | --- | --- |
| 产品入口 | `README.md` / `README_cn.md` | 用户能看到什么、怎样运行、关键安全边界 |
| 总索引 | `docs/project-docs-index.md` | 所有文档的阅读路径和状态 |
| 结构深挖 | `docs/project-structure-deep-dive.md` | 分层架构、目录职责、代码入口 |
| 实现地图 | `docs/codebase-implementation-map.md` | 当前代码事实、入口文件、跨层同步点 |
| 架构 | `docs/architecture.md` | 数据流、组件关系、运行链路 |
| 安全 | `docs/security-model.md`、`docs/threat-model.md`、`docs/policy-semantics.md` | auth、runtime gate、策略语义、非目标 |
| API | `docs/external-api.md`、`docs/otel-export.md` | 外部接口、兼容 API、导出 |
| 部署 | `docs/kubernetes.md`、`.devcontainer/README.md` | 节点级部署、开发容器 |
| 组件 | `backend/README.md`、`frontend/README.md`、`wrapper/README.md`、adapter READMEs | 子系统局部使用与维护 |
| 答辩 | `docs/os-competition-defense.md`、`docs/demo-script.md`、`docs/evaluation-report.md`、`docs/development-timeline.md` | 比赛材料、演示、评测、合规 |

### 10.2 需要持续修正的文档风险

1. **路径风险**：旧文档常写 `backend/main.go`、`backend/routes.go`；当前实现大量入口在 `backend/app/*.go`，需要逐步同步。
2. **安全风险**：TLS capture、domain forward、system run、hook install 等高风险能力必须描述为默认关闭、显式启用。
3. **策略语义风险**：不要把 exact matching 写成 recursive / CIDR / range。
4. **生成物风险**：不要引导开发者手改 `backend/pb/`、`frontend/src/pb/`、adapter pb、BPF generated Go / object 文件。
5. **build vs runtime 风险**：不要把 compiled-in feature 等同于 runtime enabled。
6. **性能数据风险**：答辩或评测文档引用数字前必须附测试日期、机器环境和命令。

---

## 11. 后续文档任务建议

优先级从高到低：

1. 用本文件中的当前代码事实校正 `docs/project-structure-deep-dive.md` 的旧路径。
2. 更新 `docs/architecture.md`，补充 `backend/app/`、feature manifest、zero-copy ringbuf decode、runtime jobs。
3. 更新 `docs/security-model.md`，把 feature manifest 中的 danger level 和 route prefix 纳入安全说明。
4. 为 `backend/app/` 的文件命名方案补一段说明，解释 `domain__original.go` 形式来自合并 / 拆分历史，避免新贡献者误判。
5. 在 `docs/evaluation-report.md` 中补真实验证结果：`go test`、frontend build、wrapper test、benchmark、手动演示截图 / 日志。
6. 在 `docs/demo-script.md` 中明确高风险演示的准备命令、授权前提和失败兜底。

---

## 12. 快速审查清单

每次改代码或文档时，至少检查：

- 是否引用了真实存在的当前路径？
- 是否误把危险能力说成默认开启？
- 是否误把 exact matching 说成递归、CIDR 或 range？
- 是否漏掉 release mode auth？
- 是否需要 runtime feature gate？
- 是否触及 generated files？
- 是否需要同步 proto / backend / frontend / docs？
- 是否提供了最小验证命令？
- 是否明确说明验证结果或跳过原因？
