---
name: project-structure
description: Understand the agent-ebpf-filter repository structure and choose the right files, commands, and docs when developing agents or features in this project.
---

# Agent eBPF Filter 项目结构导航

使用本 skill 辅助 Agent 在 `agent-ebpf-filter` 仓库中快速定位代码、规划修改范围、选择验证命令，并保持后端、前端、protobuf、eBPF 与文档的一致性。

## 何时使用

当任务涉及以下任意内容时使用：

- 需要理解项目结构、模块职责、数据流或运行链路
- 需要定位某个功能应该改哪些文件
- 需要为新功能、修复、重构或代码审查选择入口文件
- 需要判断是否要更新 protobuf、eBPF 生成物、前端类型或文档
- 需要为 AI Agent / wrapper / hook / AgentSight / TLS capture / ML / sandbox 功能开发做上下文收集

## 总体架构

核心链路：

```text
eBPF ringbuf → Go backend → WebSocket / HTTP / MCP → Vue frontend
agent-wrapper → Unix socket → backend policy engine → ALLOW/BLOCK/ALERT/REWRITE
Python / Node adapters → HTTP register/unregister → backend agent PID tracking
AI CLI hooks → curl POST /hooks/event → backend native hook ingest → events/envelopes
```

主要模块：

- `backend/`：Go 后端，特权运行时、HTTP/WS API、事件归档、策略、hooks、shell sessions、ML、AgentSight、TLS capture、sandbox、domain forward、OTLP/Prometheus。
- `backend/ebpf/`：eBPF C 程序和生成出的 Go/.o 绑定。不要手改生成文件。
- `frontend/`：Vue 3 + TypeScript + Vite 前端，全部优先使用 Composition API 与 `<script setup lang="ts">`。
- `wrapper/`：`agent-wrapper` CLI，通过 `/tmp/agent-ebpf.sock` 向后端请求命令策略决策。
- `adapters/`：Python / Node Agent PID 注册 helper。
- `proto/`：protobuf 源文件。事件、配置、shell、system、registration 等协议定义的源头。
- `docs/`：架构、策略语义、外部 API、Kubernetes、benchmark、OTLP、安全模型等文档。
- `scripts/`：开发、构建、smoke、ML report、Grafana 等辅助脚本。
- `tools/dev-env-tui/`：Go TUI，本地开发环境向导。

## 常用入口文件

### 后端 API / 路由

优先从这些文件开始：

- `backend/routes.go`：总路由注册入口。
- `backend/main.go`：启动、运行时 wiring、历史遗留的大入口。
- `backend/config_handlers*.go`：`/config/**`。
- `backend/system_handlers*.go`：`/system/**`。
- `backend/api_ws.go`：WebSocket 辅助逻辑。
- `backend/data_handlers.go`：清空/管理事件数据。
- `backend/registration_handlers.go`：`/register`、`/unregister`。
- `backend/external_api.go`：`/api/v1/**` 外部兼容 API。
- `backend/mcp_server.go`：MCP SSE / HTTP 入口。

### 事件、执行图与 AgentSight

- `backend/event_envelope.go`、`backend/event_context*.go`：规范化事件 envelope 与上下文。
- `backend/execution_graph.go`：执行图数据。
- `backend/agentsight_*.go`：AgentSight ingest / query / analyzer。
- `frontend/src/views/execution-graph/ExecutionGraph.vue`：执行图页面。
- `frontend/src/components/agentsight/`：AgentSight 展示组件。
- `frontend/src/composables/agentsight/`：AgentSight 前端状态与 i18n。

### 网络、TLS capture、Codex capture

- `backend/network_events.go`、`backend/network_syscalls*.go`：内核网络事件映射。
- `backend/network_flow_*.go`：网络 flow 聚合与存储。
- `backend/network_enrichment*.go`：DNS / TCP / GeoIP 等 enrichment。
- `backend/tls_capture_*.go`、`backend/tls_*parser*.go`、`backend/tls_fragment_assembler.go`：TLS 明文捕获、HTTP 解析和分片拼装。
- `backend/codex_capture_handlers.go`：Codex capture ingest。
- `frontend/src/views/network/Network.vue`、`NetworkFlow.vue`、`TLSCapture.vue`：网络相关页面。
- `frontend/src/components/network/`：网络 UI 组件。
- `frontend/src/composables/network/`：网络前端 composables。

### eBPF / OS enforcement / sandbox

- `backend/ebpf/agent_tracker.c`：主 syscall tracing 程序。
- `backend/ebpf/cgroup_sandbox.c`：cgroup 网络阻断。
- `backend/ebpf/lsm_enforcer.c`：BPF LSM enforcement。
- `backend/ebpf_runtime.go`：eBPF bootstrap、pin maps/links、自提权。
- `backend/cgroup_sandbox_*.go`：cgroup sandbox 状态与 API。
- `backend/lsm_enforcer_*.go`：LSM enforcement 状态与 API。
- `backend/path_policy.go`：路径策略相关逻辑。

注意：

- `tracked_comms` 是 16-byte command exact match。
- `tracked_paths` 是 256-byte path exact match，不是递归路径树。
- cgroup destination blocking 使用 exact IPv4 / IPv6 / port map，不是 CIDR/range。
- LSM 文件名策略是 basename-based；exec 策略按 exact path 或 executable basename。

### Shell / Executor / wrapper

- `backend/shell_session_*.go`、`backend/shell_ws.go`：PTY shell sessions。
- `backend/privileges.go`：子进程降权逻辑。
- `backend/uds_server.go`：wrapper UDS policy server。
- `wrapper/main.go`：CLI wrapper。
- `frontend/src/views/executor/Executor.vue`：Executor 页面。
- `frontend/src/components/terminal/`：terminal UI 组件。
- `frontend/src/composables/executor/`：shell sessions 与 launcher composables。

### Hooks

- `backend/hooks*.go`：AI CLI hook 检测、安装、事件 ingest。
- `frontend/src/views/hooks/Hooks.vue`：Hooks 页面。
- `frontend/src/components/hooks/`：hook card / modal。
- `frontend/src/data/hookCatalog.ts`：hook catalog。

注意：native hook relay 脚本依赖 host 上的 `curl`。

### Config / Runtime / Registry / Security

- `frontend/src/views/config/Config.vue`：Config 页面主容器。
- `frontend/src/components/config/`：各配置 tab。
- `frontend/src/composables/config/`：各配置领域的 API 与状态。
- `frontend/src/types/config.ts`：配置类型。
- `backend/config_handlers*.go`：后端配置 API。
- `backend/runtime_state*.go`：运行时状态、token、持久化。
- `backend/features.go`、`backend/feature_helpers.go`：runtime feature gates。

### ML

- `backend/ml_*.go`、`backend/ml/*.go`：模型、训练、sweep、dataset、LLM scoring、auto-tune。
- `frontend/src/views/ml/ML.vue`：ML 页面。
- `frontend/src/components/config/ml/`：ML 子 tab。
- `frontend/src/composables/config/useConfigML*.ts`、`useAutoTune.ts`、`useMLStatusStream.ts`：ML 前端状态。
- `docs/ml-*.md`、`scripts/ml-*.sh`：报告和 benchmark 辅助。

### Plugins / visual eBPF builder

- `backend/plugin*.go`：插件 API、visual LLM 解析。
- `frontend/src/views/plugins/Plugins.vue`：Plugins 页面。
- `frontend/src/components/plugins/`：visual designer、flow canvas、pseudo compiler、transpiler、runtime、recipes。
- `frontend/src/composables/plugins/`：canvas / NLP compiler / plugins 状态。

重要约束：visual eBPF plugins 使用 `attachKind: "lsm"`；仅 `unlink` / `do_unlinkat` 流程使用 `attachKind: "kprobe"`。不要为非 unlink 插件序列化 `attachKind: "none"`。

## 前端路由与页面

路由定义在 `frontend/src/router/index.ts`：

- `/dashboard/:tab?` → `frontend/src/views/dashboard/Dashboard.vue`
- `/monitor/:tab?/:subtab?` → `frontend/src/views/monitor/Monitor.vue`
- `/network` → `frontend/src/views/network/Network.vue`
- `/network-flow/:tab?` → `frontend/src/views/network/NetworkFlow.vue`
- `/tls-capture` → `frontend/src/views/network/TLSCapture.vue`
- `/execution-graph/:tab?` → `frontend/src/views/execution-graph/ExecutionGraph.vue`
- `/explorer` → `frontend/src/views/explorer/Explorer.vue`
- `/executor/:tab?` → `frontend/src/views/executor/Executor.vue`
- `/hooks` → `frontend/src/views/hooks/Hooks.vue`
- `/ml/:subtab?` → `frontend/src/views/ml/ML.vue`
- `/plugins/:tab?` → `frontend/src/views/plugins/Plugins.vue`
- `/config/:tab?/:subtab?/:subsubtab?` → `frontend/src/views/config/Config.vue`

前端开发规则：

- 使用 Vue 3 Composition API 与 `<script setup lang="ts">`。
- 视图放在 `frontend/src/views/`。
- 可复用 UI 放在 `frontend/src/components/<domain>/`。
- 可复用状态/API 逻辑放在 `frontend/src/composables/<domain>/`。
- 类型放在 `frontend/src/types/`。
- 修改 Vue 代码时使用项目已有样式、命名和 composable 模式。

## Protobuf 与生成文件

源文件在 `proto/`：

- `proto/tracker.proto` 是聚合文件，只 import 功能域 proto。
- `proto/tracker_common.proto`
- `proto/tracker_events.proto`
- `proto/tracker_registration.proto`
- `proto/tracker_system.proto`
- `proto/tracker_config.proto`
- `proto/tracker_shell.proto`

如果改 proto，运行：

```bash
make proto
```

生成文件不要手改，包括但不限于：

- `backend/pb/*.pb.go`
- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`
- `adapters/python/tracker_*_pb2.py`
- `adapters/js/tracker_pb.js`

## eBPF 生成与构建

改 `backend/ebpf/agent_tracker.c` 后：

```bash
cd backend/ebpf && go generate
cd ../.. && cd backend && go build ./...
```

或使用项目 Make target：

```bash
make backend
```

改 cgroup/LSM eBPF 程序时优先使用：

```bash
make ebpf-cgroup
make ebpf-lsm
```

生成文件不要手改，包括：

- `backend/ebpf/*_bpfel.go`
- `backend/ebpf/*_bpfeb.go`
- `backend/ebpf/*.o`

## 选择修改文件的决策表

| 任务类型 | 首先查看 | 常见改动位置 | 验证 |
| --- | --- | --- | --- |
| 新增/修改 HTTP API | `backend/routes.go`、对应 `*_handlers.go` | `backend/*_handlers.go`、前端 composable | `cd backend && go test ./...` |
| 新增事件字段/类型 | `proto/tracker_events.proto`、`backend/network_events.go` | proto、backend mapping、frontend filters/tables | `make proto`、backend/frontend build |
| 新增配置项 | `proto/tracker_config.proto`、`backend/config_handlers*.go`、`frontend/src/types/config.ts` | backend config state、frontend composable/tab | `make proto`、frontend typecheck/build |
| 修改 eBPF tracing | `backend/ebpf/agent_tracker.c` | eBPF C、Go event decode、proto、frontend display | `go generate`、`cd backend && go build ./...` |
| 修改 wrapper policy | `wrapper/main.go`、`backend/uds_server.go` | wrapper、UDS server、rules UI | wrapper/backend tests/build |
| 修改 shell sessions | `backend/shell_session_*.go` | shell handlers、WS、frontend Executor | backend tests、manual Executor check |
| 修改 Vue 页面 | 相关 `frontend/src/views/**` | components/composables/types | `cd frontend && bun run build` 或项目脚本 |
| 修改 hooks | `backend/hooks*.go`、`frontend/src/data/hookCatalog.ts` | backend handlers、hook UI、docs | backend tests、manual hook install if safe |
| 修改 docs/API 行为 | `docs/`、`README.md`、`backend/README.md` | 对应文档 | 文档一致性检查 |

## 构建与验证命令

常用命令：

```bash
make help
make predev
make dev
make backend
make wrapper
make frontend
make proto
make all
```

针对性验证：

```bash
cd backend && go test ./...
cd wrapper && go test ./...
cd tools/dev-env-tui && go test ./...
cd frontend && bun run build
```

项目级注意事项：

- 本环境有 RTK hook，普通 shell 命令可能自动经 `rtk` 改写；直接使用 `rtk gain` / `rtk discover` 等 meta 命令时显式调用 `rtk`。
- 后端会选择 `8080..8089` 的首个可用端口并写入 `backend/.port`，Vite dev proxy 会读取它。
- Release mode 中多数敏感 API 受 runtime access token 保护；dev mode 默认关闭 auth。
- Shell sessions、`/system/run`、hook 安装和 policy mutation 受 runtime feature gate 保护，默认关闭。
- 后端特权部分会通过 `sudo` / `pkexec` 自提权；测试 eBPF、cgroup、LSM 行为时要考虑宿主权限与内核能力。

## 文档同步规则

行为变化时更新相关文档：

- `README.md`：产品级能力、支持 syscall、hook、auth 范围。
- `agents.md`：Agent PID 注册语义。
- `AGENTS.md`：贡献者 gotchas 与约定。
- `docs/architecture.md`：组件与数据流。
- `docs/external-api.md`：外部 API / `/api/v1` 行为。
- `docs/kubernetes.md` 与 `deploy/kubernetes/agent-ebpf-filter.yaml`：部署行为。
- `backend/README.md`、`frontend/README.md`、`wrapper/README.md`、adapter README：组件级细节。

## Agent 工作流建议

1. 先确定任务影响域：backend / frontend / proto / eBPF / wrapper / docs。
2. 从上面的入口文件读取最小上下文，不要优先打开生成文件或超大文件。
3. 如果涉及 Vue，遵守 Composition API、`<script setup lang="ts">`、现有 composable 结构。
4. 如果涉及 API 或事件类型，检查 proto、backend mapping、frontend display/filter 是否需要同步。
5. 如果涉及 runtime 安全、auth、shell、policy mutation 或 OS enforcement，检查 feature gate 与文档是否一致。
6. 改完后运行最小充分验证，并如实报告成功、失败或跳过原因。
