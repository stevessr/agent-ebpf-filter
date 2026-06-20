# 项目结构深度说明

> 项目：Agent eBPF Filter  
> 用途：作为操作系统设计赛答辩、后续研发协作、代码审查和交付验收的结构导航文档。  
> 关联文档：`docs/os-competition-defense.md`、`docs/project-roadmap.md`、`docs/architecture.md`、`docs/security-model.md`、`docs/threat-model.md`。

---

## 1. 项目定位

Agent eBPF Filter 是一个面向 Linux 本地工作站、实验节点和开发环境的 **AI Agent 行为观测与安全控制系统**。它把内核态 eBPF 采集、cgroup / BPF LSM 内核阻断、Go 后端控制面、Vue 3 可视化前端、AI CLI hook、命令 wrapper、多语言 Agent adapter、MCP / OTLP / Prometheus 外部接口和可选内核态 ML 模块组合成一个完整的操作系统级工具。

项目的核心能力可以归纳为：

1. **看见事实**：通过 eBPF tracepoint、ringbuf、network flow、EventEnvelope 记录真实 OS 行为。
2. **关联语义**：通过 native hooks、wrapper、PID registration、tool_call_id、trace_id 关联 AI Agent 声明意图与系统调用事实。
3. **形成图谱**：通过 Execution Graph / AgentSight 把进程、文件、网络、工具调用、策略命中组织成可查询、可导出、可回放的证据链。
4. **实施控制**：通过 wrapper policy、cgroup 网络阻断、BPF LSM 文件 / 执行阻断实施用户态与内核态双层控制。
5. **安全交付**：通过 runtime feature gates、release-mode token、数据脱敏、危险能力默认关闭降低敏感功能误用风险。

---

## 2. 仓库顶层结构

当前仓库受版本控制的顶层结构大致如下：

| 路径 | 主要职责 | 当前规模 / 备注 |
| --- | --- | --- |
| `backend/` | Go 后端、eBPF runtime、API、事件处理、策略、ML、MCP、AgentSight、sandbox | 约 481 个受控文件，其中顶层 Go 文件约 415 个 |
| `backend/ebpf/` | eBPF C 程序、go:generate 入口、生成绑定 | 约 19 个受控文件；生成物不要手改 |
| `frontend/` | Vue 3 + Vite + TypeScript 前端 | 约 282 个受控文件，`frontend/src` 约 183 个文件 |
| `proto/` | protobuf 源定义 | 7 个 proto 源文件，是后端 / 前端 / adapter 协议源头 |
| `wrapper/` | `agent-wrapper` 命令拦截 CLI | Go module，负责 ALLOW/BLOCK/ALERT/REWRITE 执行链 |
| `adapters/` | Python / Node Agent PID 注册适配器 | 约 17 个受控文件 |
| `kernel-ml/` | 可选 DKMS 内核态 ML 推理模块 | 约 23 个受控文件，含 C、CUDA helper、Python loader、README |
| `docs/` | 架构、安全、benchmark、ML、TLS、答辩文档 | 当前已有 50+ 文档，新增赛事答辩与规划文档建议集中于此 |
| `scripts/` | 开发、安装、smoke、benchmark、ML 报告脚本 | 约 14 个 shell 脚本 |
| `benchmarks/` | runtime replay 场景 | 用于 agent-security 离线回放评测 |
| `deploy/` | Kubernetes / 部署清单 | 节点级部署入口 |
| `.devcontainer/` | 开发容器配置 | privileged devcontainer，便于 eBPF 开发 |
| `.claude/skills/` | 项目级 Claude Code skills | 内置项目结构、安全配置、网络分析、进程监控技能 |
| `Makefile` | 统一构建、运行、验证入口 | 所有研发和答辩演示应优先引用这里的命令 |
| `README.md` / `README_cn.md` | 产品说明 | 行为变化应同步更新 |
| `AGENTS.md` / `agents.md` | Agent / contributor 约定 | 开发约束、注册语义、生成物边界 |
| `LICENSE` | 源代码开源协议 | 当前为 GPL-3.0 |

---

## 3. 分层架构

项目可按 L0–L5 分层理解。

```text
L0 产品目标层
  └─ AI Agent 行为观测、运行时可视化、安全策略、证据回放、答辩展示

L1 运行时边界层
  ├─ privileged Go backend
  ├─ eBPF maps / links / ringbuf
  ├─ cgroup / BPF LSM 内核阻断
  ├─ Unix socket wrapper policy
  ├─ HTTP / WebSocket / MCP / OTLP / Prometheus
  ├─ Vue frontend
  └─ adapters / native hooks / external API

L2 协议与事件层
  ├─ proto/*.proto
  ├─ backend/pb/*.pb.go
  ├─ frontend/src/pb/*
  ├─ adapters/*/tracker_*
  └─ EventEnvelope / Execution Graph / AgentSight aliases

L3 后端领域层
  ├─ routes + handlers
  ├─ runtime state + auth + feature gates
  ├─ event ingest / archive / persistence / recording
  ├─ network / TLS / Codex capture
  ├─ shell sessions + wrapper UDS
  ├─ hooks + MCP + external API
  ├─ ML + plugins
  └─ cgroup / LSM sandbox + eBPF bootstrap

L4 前端领域层
  ├─ views
  ├─ components
  ├─ composables
  ├─ types / data / utils
  ├─ router
  └─ generated protobuf JS / TS

L5 构建、测试、部署、文档层
  ├─ Makefile
  ├─ scripts/
  ├─ tools/dev-env-tui/
  ├─ deploy/
  ├─ docs/
  └─ README / AGENTS / component READMEs
```

这种分层适合用于答辩：先讲产品目标，再讲运行边界，然后深入协议、内核、后端、前端和工程交付。

---

## 4. 核心运行链路

### 4.1 eBPF 事件链路

```text
tracked PID / comm / path
  → eBPF tracepoint / cgroup / LSM program
  → pinned BPF maps + ringbuf
  → Go backend reader / decoder
  → pb.Event / EventEnvelope
  → archive / persistence / WebSocket / OTLP / MCP / AgentSight
  → Vue Dashboard / Network / ExecutionGraph / AgentSight
```

关键文件：

- `backend/ebpf/agent_tracker.c`
- `backend/ebpf/agent_tracker_common.h`
- `backend/ebpf/cgroup_sandbox.c`
- `backend/ebpf/lsm_enforcer.c`
- `backend/app/runtime__runtime_ebpf.go`
- `backend/app/events__events_network.go`
- `backend/app/runtime__envelope_event.go`
- `backend/app/events__graph_execution.go`
- `frontend/src/views/dashboard/Dashboard.vue`
- `frontend/src/views/network/Network.vue`
- `frontend/src/views/execution-graph/ExecutionGraph.vue`

### 4.2 Wrapper 策略链路

```text
用户 / 前端 / Agent 触发命令
  → agent-wrapper
  → Unix socket /tmp/agent-ebpf.sock
  → backend policy engine
  → ALLOW / BLOCK / ALERT / REWRITE
  → wrapper intercept event
  → 执行、阻断或改写命令
```

关键文件：

- `wrapper/main.go`
- `backend/uds_server.go`
- `backend/behavior.go`
- `backend/path_policy.go`
- `frontend/src/views/executor/Executor.vue`
- `frontend/src/components/terminal/RemoteWrapperTerminal.vue`

### 4.3 Native Hook 链路

```text
AI CLI hook payload
  → generated relay script
  → curl POST /hooks/event
  → hook ingress auth / secret check
  → normalize payload
  → native_hook event / EventEnvelope
  → Dashboard / AgentSight / OTLP / persistence
```

支持方向包括 Claude Code、Gemini、Codex、Copilot、Kiro、Cursor 等 AI CLI 或 wrapper alias。

关键文件：

- `backend/hooks.go`
- `backend/hooks_detection.go`
- `backend/hooks_events.go`
- `backend/config_handlers_hooks.go`
- `frontend/src/views/hooks/Hooks.vue`
- `frontend/src/components/hooks/`
- `frontend/src/data/hookCatalog.ts`
- `frontend/src/types/hooks.ts`

### 4.4 前端配置链路

```text
Vue view
  → domain composable
  → HTTP / WebSocket API
  → backend handler
  → runtime state / BPF map / feature gate / persistence
  → UI state refresh
```

前端修改时应保持页面容器、组件、composable、types 分层，不把大量 API / WS / 数据转换逻辑堆在单个 `.vue` 文件中。

---

## 5. 后端结构

### 5.1 技术栈

- Go module：`backend/go.mod`
- Go 版本：`1.26.2`
- HTTP：`github.com/gin-gonic/gin`
- WebSocket：`github.com/gorilla/websocket`
- eBPF：`github.com/cilium/ebpf`
- PTY：`github.com/creack/pty/v2`
- MCP：`github.com/modelcontextprotocol/go-sdk`
- OTLP：`go.opentelemetry.io/otel/*`
- protobuf：`google.golang.org/protobuf`

### 5.2 目录 / 文件职责

```text
backend/
  main.go                      # 启动、wiring、历史大入口
  routes.go                    # 路由注册总入口
  *_handlers.go                # HTTP handler 层
  *_ws.go                      # WebSocket handler / broadcaster
  runtime_state*.go            # runtime state、token、persistence
  event_*.go                   # event archive、context、envelope、recording
  network_*.go                 # network events / flows / enrichment
  tls_*.go                     # TLS capture、parser、assembler、store
  agentsight_*.go              # AgentSight API / analyzers
  ml_*.go + ml/                # ML model / training / sweep / LLM scoring
  cgroup_sandbox_*.go          # cgroup sandbox control / API
  lsm_enforcer_*.go            # BPF LSM enforcer control / API
  shell_session_*.go           # PTY session lifecycle
  hooks*.go                    # AI CLI hooks
  plugin*.go                   # plugin registry / visual LLM
  ebpf_runtime.go              # privileged eBPF bootstrap
  ebpf/                        # eBPF C + generated bindings
  pb/                          # generated protobuf Go files
```

### 5.3 路由分组

`backend/app/routes__routes.go` 中按领域注册：

- WebSocket：`/ws`、`/ws/system`、`/ws/envelopes`、`/ws/events/graph`、`/ws/tls-capture` 等。
- Shell：`/shell-sessions`、`/ws/shell`。
- Events：`/events/recent`、`/events/graph`、`/events/recording/**`。
- Network：`/network/flows`、`/network/analyze`、`/network/interfaces`、`/network/export*` 等。
- Sandbox：`/sandbox/cgroup/**`、`/sandbox/lsm/**`。
- Utility：`/metrics`、`/hooks/event`、`/register`、`/unregister`。
- Authenticated API：`/config/**`、`/system/**`、`/tls-capture/**`、`/agentsight/**`、`/plugins/**`、`/mcp` 等。
- Compatibility：`/api/**`、`/api/v1/**`。
- Static：生产模式前端 dist。

### 5.4 后端模块边界

| 模块 | 主要文件 | 边界要求 |
| --- | --- | --- |
| Auth / feature gates | `helpers_auth.go`、`features.go`、`feature_helpers.go` | 新危险能力必须默认关闭并受 gate 控制 |
| Runtime state | `runtime_state*.go` | runtime token、config、persistence 统一从这里进入 |
| Event / Envelope | `event_envelope.go`、`event_context*.go`、`execution_graph.go` | 新事件字段要同步 AgentSight / ExecutionGraph / OTLP |
| Network | `network_*.go` | network type mapping 要与 proto / frontend 对齐 |
| TLS / Codex capture | `tls_*.go`、`codex/capture` | 默认关闭，必须脱敏，主事件携带 digest / metadata |
| AgentSight | `agentsight_*.go` | 保持兼容 API 和 import / export 行为 |
| Shell / Executor | `shell_session_*.go`、`privileges.go` | shell session 逻辑不要塞回 `main.go` |
| Wrapper / UDS | `uds_server.go`、`behavior.go`、`path_policy.go` | UDS 权限和 peer credentials 不能削弱 |
| Hooks | `hooks*.go` | hook install 是危险能力；relay 依赖 curl |
| ML | `ml_*.go`、`ml/` | 训练、扫参、LLM scoring、status WS 分层维护 |
| Plugins | `plugin*.go` | visual eBPF attachKind 语义要稳定 |
| Sandbox | `cgroup_sandbox_*.go`、`lsm_enforcer_*.go` | exact map / basename / path 语义不能误写成递归 / CIDR |

---

## 6. 前端结构

### 6.1 技术栈

- Vue `^3.5.32`
- Vue Router 4
- Vite `^8.0.9`
- TypeScript `5.9.3`
- Ant Design Vue
- ApexCharts / D3
- Monaco Editor / Markdown-it / Shiki
- protobufjs

### 6.2 目录分层

```text
frontend/src/
  main.ts                    # Vue app bootstrap
  App.vue                    # App shell / navigation
  router/index.ts            # 路由表
  style.css                  # 全局样式
  views/                     # 页面级容器
  components/                # 领域 UI 组件
  composables/               # 状态、API、WS、数据处理
  types/                     # TypeScript 类型
  data/                      # 静态 catalog / reference data
  utils/                     # 通用工具函数
  pb/                        # generated protobuf JS/TS，不手改
```

### 6.3 用户可见页面

| 路由 | 页面 | 说明 |
| --- | --- | --- |
| `/dashboard/:tab?` | Dashboard | live event stream / dashboard tabs |
| `/monitor/:tab?/:subtab?` | Monitor | CPU、内存、GPU、IO、page fault、sensor、systemd、tracing |
| `/network` | Network | syscall-derived network events |
| `/network-flow/:tab?` | NetworkFlow | flow table / details / graph |
| `/tls-capture` | TLSCapture | TLS / Codex capture UI |
| `/agentsight/:tab?` | redirect ExecutionGraph behavior | AgentSight 兼容入口 |
| `/execution-graph/:tab?` | ExecutionGraph | agent / process / tool / syscall graph |
| `/explorer` | Explorer | 文件浏览与 tracked paths |
| `/executor/:tab?` | Executor | PTY shell、wrapper execution、launcher |
| `/hooks` | Hooks | AI CLI hook management |
| `/ml/:subtab?` | ML | ML status、training、tuning、dataset、LLM scoring |
| `/plugins/:tab?` | Plugins | plugin registry / visual builder |
| `/config/:tab?/:subtab?/:subsubtab?` | Config | runtime、security、registry、docs、system health |

### 6.4 前端边界原则

1. `views/` 是页面容器，负责组合组件和 composable。
2. `components/<domain>/` 放可复用 UI，不直接做大量全局副作用。
3. `composables/<domain>/` 放 API、WebSocket、状态和数据转换。
4. `types/` 放共享响应类型、配置类型和 UI model。
5. `data/` 放 hook catalog、Linux reference catalog、ML model catalog 等静态资料。
6. `utils/` 放通用工具，不承载巨大领域逻辑。
7. `frontend/src/pb/` 是生成物，不手改。

---

## 7. 协议、eBPF 与生成物

### 7.1 Protobuf 源文件

```text
proto/
  tracker.proto              # 聚合 import
  tracker_common.proto       # 通用类型
  tracker_events.proto       # 事件 enum / payload
  tracker_registration.proto # Agent register / unregister
  tracker_system.proto       # system stats / runtime system
  tracker_config.proto       # config / runtime / security / ML
  tracker_shell.proto        # shell session
```

修改 proto 后必须运行：

```bash
make proto
```

### 7.2 生成物禁止手改

不要手工编辑：

- `backend/pb/*.pb.go`
- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`
- `adapters/python/tracker_*_pb2.py`
- `adapters/js/tracker_pb.js`
- `backend/ebpf/*_bpfel.go`
- `backend/ebpf/*_bpfeb.go`
- `backend/ebpf/*.o`

### 7.3 eBPF 源文件

| 文件 | 职责 |
| --- | --- |
| `agent_tracker.c` | 主 syscall tracing 程序 |
| `agent_tracker_common.h` | 共享结构和常量 |
| `agent_tracker_syscalls.h` | syscall handler helper / definitions |
| `agent_tracker_tail.h` | tail call / 分段逻辑 |
| `cgroup_sandbox.c` | cgroup connect / UDP sendmsg network blocking |
| `lsm_enforcer.c` | BPF LSM exec / file enforcement |
| `agent_tls_capture.c` | TLS uprobe capture |
| `gen*.go` | go:generate 入口 |

### 7.4 内核匹配语义

这些语义必须在文档、UI 和答辩中准确表达：

- `agent_pids`：PID match，注册进程种子 + fork/clone lineage + userspace parent fallback。
- `tracked_comms`：16-byte command-name exact match。
- `tracked_paths`：256-byte path exact match，不是递归路径树。
- cgroup blocklist：基于 cgroup v2 inode id。
- destination blocking：exact IPv4 / IPv6 / TCP/UDP port maps，不是 CIDR / range。
- LSM file-name policy：basename-based。
- LSM exec policy：exact path 或 executable basename。

---

## 8. 构建、运行与验证入口

### 8.1 准备与开发

```bash
make predev
make deps
make dev
make dev-backend
make dev-frontend
make run-backend
make run-frontend
```

`make dev` 使用 Zellij 启动后端热加载和前端 Vite dev server。后端在 `8080..8089` 中选择可用端口，写入 `backend/.port`，前端 dev proxy 和 adapters 可读取该端口。

### 8.2 构建

```bash
make backend
make frontend
make wrapper
make proto
make all
make build
```

`make all` 包括 proto、backend、frontend、wrapper。

### 8.3 eBPF / OS enforcement

```bash
make ebpf-bootstrap
make ebpf-tls
make ebpf-cgroup
make ebpf-lsm
make os-enforcement-preflight
make os-enforcement-check
make os-enforcement-smoke
make os-enforcement-smoke-start
```

### 8.4 安装 / 部署

```bash
make install
make uninstall
make dev-image
make docker
make exec
```

安装路径和行为：

- production backend / frontend / wrapper 构建；
- 安装到 `/opt/agent-ebpf-filter`；
- public binaries 到 `/usr/local/bin`；
- 写入 `/etc/agent-ebpf-filter/agent-ebpf-filter.env`；
- systemd 优先，无 systemd 时回落 rc.local managed block。

### 8.5 最小验证决策表

| 改动类型 | 最小验证 | 更完整验证 |
| --- | --- | --- |
| Markdown / docs | 检查文件存在与内容 | 链接检查、答辩预演 |
| Go backend | `cd backend && go test ./...` | `make backend` |
| wrapper | `cd wrapper && go test ./...` | `make wrapper` |
| frontend | `cd frontend && bun run build` | `make frontend` |
| proto | `make proto` | `make all` |
| eBPF tracker | `cd backend/ebpf && go generate` + `cd backend && go build ./...` | `make backend` |
| cgroup / LSM | `make ebpf-cgroup` / `make ebpf-lsm` | OS enforcement smoke |
| runtime replay | `make runtime-benchmark` | 分析 `reports/runtime-replay-*` |
| Kubernetes | manifest review / lint | 真实节点部署验证 |

---

## 9. 安全边界结构

### 9.1 Auth

Release mode 中敏感 API 需要 runtime access token。典型受保护范围包括：

- `/config/**`
- `/system/**`
- `/ws*`
- `/metrics`
- `/register`
- `/unregister`
- `/agentsight/**`
- `/api/events*`
- `/api/runners*`
- `/api/stream*`
- `/shell-sessions*`
- `/events/recent`
- `/events/graph`
- `/sandbox/**`

Dev mode 默认关闭 auth，便于本地开发。

### 9.2 Runtime feature gates

以下危险能力默认关闭，需要 runtime config 显式启用：

- shell sessions
- `/system/run`
- hook installation / raw hook writes
- policy mutation
- TLS capture
- OTLP export
- domain forward proxy

### 9.3 OS enforcement

- cgroup sandbox：exact cgroup id、IPv4、IPv6、TCP/UDP port map。
- BPF LSM enforcer：exec exact path / executable basename、file / directory basename。
- policy maps 使用 restrictive permissions，建议通过 authenticated backend API 修改。

### 9.4 TLS capture

TLS 明文捕获是高风险诊断能力，不是默认安全基线：

- 默认关闭；
- 只在 Runtime Config 显式启用；
- HTTP parser 和 Codex ingest 必须共用脱敏逻辑；
- 主事件应只携带 metadata / digest，避免敏感明文进入普通事件流。

---

## 10. 功能域索引

| 功能域 | 前端入口 | 后端入口 | 主要文档 / 验证 |
| --- | --- | --- | --- |
| Dashboard | `views/dashboard/Dashboard.vue` | `/ws`、`/events/recent`、`event_envelope.go` | frontend build、backend tests |
| Monitor | `views/monitor/Monitor.vue` | `system_handlers*.go`、`metrics.go` | system metrics 手动验证 |
| Network | `views/network/Network.vue`、`NetworkFlow.vue` | `network_*.go` | backend tests、live network capture |
| TLS Capture | `views/network/TLSCapture.vue` | `tls_*.go`、`agent_tls_capture.c` | TLS parser tests、`make ebpf-tls` |
| Execution Graph / AgentSight | `views/execution-graph/ExecutionGraph.vue` | `execution_graph.go`、`agentsight_*.go` | graph / envelope / agentsight tests |
| Explorer | `views/explorer/Explorer.vue` | `helpers_fs.go`、`path_policy.go` | frontend build |
| Executor | `views/executor/Executor.vue` | `shell_session_*.go`、`uds_server.go` | wrapper/backend tests、手动 PTY 验证 |
| Hooks | `views/hooks/Hooks.vue` | `hooks*.go` | backend tests、frontend build；真实安装需授权 |
| Config / Security | `views/config/Config.vue` | `config_handlers*.go`、`runtime_state*.go` | backend tests、frontend build |
| ML | `views/ml/ML.vue` | `ml_*.go`、`config_ml_handlers.go` | backend ML tests、runtime benchmark |
| Plugins | `views/plugins/Plugins.vue` | `plugin*.go` | frontend build、backend tests |
| External API / Deploy | N/A | `external_api.go`、`routes.go` | `docs/external-api.md`、`docs/kubernetes.md` |

---

## 11. 答辩材料中的结构表达建议

答辩中建议把结构讲成四张图：

1. **产品总览图**：AI Agent → eBPF / wrapper / hook → Go backend → Vue UI / JSONL / MCP / OTLP。
2. **内核能力图**：tracepoint、cgroup/connect、cgroup/sendmsg、BPF LSM、pinned maps、ringbuf。
3. **后端控制面图**：routes、runtime state、auth、feature gates、event archive、policy API、MCP、OTLP。
4. **前端工作台图**：Dashboard、Network、Execution Graph、AgentSight、Config、ML、Plugins、Hooks、Executor。

同时应突出三条边界：

- 观测路径：低侵入、事实可信；
- 控制路径：wrapper 用户态控制 + cgroup / LSM 内核态控制；
- 安全路径：auth、runtime gate、脱敏、危险能力默认关闭。

---

## 12. 后续维护规则

1. 新事件字段从 proto 改起，不能只改前端显示。
2. 新 runtime config 字段必须同步后端 state、env override、Config UI 和 docs。
3. 新危险能力必须默认关闭，且 release mode 受 auth 保护。
4. 新 eBPF map 必须同步 C、Go binding、bootstrap、pin path、status API 和 docs。
5. 新 hook provider 必须同步 detection、install/uninstall、payload parser、frontend catalog 和 docs。
6. 修改 Vue 页面时保持 Composition API 与 `<script setup lang="ts">`。
7. 修改行为边界时更新 `docs/security-model.md`、`docs/threat-model.md`、`docs/policy-semantics.md` 或本结构文档。
8. 答辩材料引用性能数据前必须重新记录测试环境、命令和结果。
