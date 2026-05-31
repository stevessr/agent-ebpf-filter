# 03 — 前端层

本层用于定位 Vue 3 前端文件、页面结构、组件/组合式函数分层、路由和验证方式。

## 技术栈

- Vue：`^3.5.32`
- Router：`vue-router@4`
- Build：Vite `^8.0.9`
- TypeScript：`5.9.3`
- Typecheck：`vue-tsc`
- UI：`ant-design-vue`、`@ant-design/icons-vue`
- Charts / graph：`apexcharts`、`vue3-apexcharts`、`d3`
- Editor / docs：`monaco-editor`、`markdown-it`、`shiki`
- Protobuf：`protobufjs`

构建脚本在 `frontend/package.json`：

```bash
cd frontend && bun run build
```

实际执行：

```text
vue-tsc -b && vite build
```

## 前端目录分层

```text
frontend/src/
  main.ts                    # Vue app bootstrap
  App.vue                    # App shell / navigation
  router/index.ts            # 路由表
  style.css                  # 全局样式
  views/                     # 页面级容器
  components/                # 领域 UI 组件
  composables/               # 可复用状态、API、WS、数据处理
  types/                     # TypeScript 类型
  data/                      # 静态 catalog / reference data
  utils/                     # 通用工具函数
  pb/                        # generated protobuf JS/TS，不手改
```

## 路由到页面映射

路由定义在 `frontend/src/router/index.ts`：

| 路由 | 页面 | 说明 |
| --- | --- | --- |
| `/` | redirect `/dashboard` | 默认入口 |
| `/dashboard/:tab?` | `views/dashboard/Dashboard.vue` | live event stream / dashboard tabs |
| `/monitor/:tab?/:subtab?` | `views/monitor/Monitor.vue` | CPU/Mem/GPU/IO/page-fault/sensors/systemd/tracing |
| `/network` | `views/network/Network.vue` | syscall-derived network events |
| `/network-flow/:tab?` | `views/network/NetworkFlow.vue` | flow table / details / graph |
| `/tls-capture` | `views/network/TLSCapture.vue` | TLS/Codex capture UI |
| `/agentsight/:tab?` | redirect ExecutionGraph behavior | AgentSight old route compatibility |
| `/execution-graph/:tab?` | `views/execution-graph/ExecutionGraph.vue` | agent/process/tool/syscall graph |
| `/explorer` | `views/explorer/Explorer.vue` | file browser / tracked paths |
| `/executor/:tab?` | `views/executor/Executor.vue` | PTY shell / wrapper execution / launcher |
| `/hooks` | `views/hooks/Hooks.vue` | AI CLI hook management |
| `/ml/:subtab?` | `views/ml/ML.vue` | ML status/training/tuning/dataset |
| `/plugins/:tab?` | `views/plugins/Plugins.vue` | plugin registry / visual builder |
| `/config/ml/:subtab?` | redirect ML | legacy route |
| `/config/:tab?/:subtab?/:subsubtab?` | `views/config/Config.vue` | runtime/config/security/docs |

新增页面时：

1. 在 `views/<domain>/` 放页面容器。
2. 在 `router/index.ts` 增加 lazy route。
3. 在 `App.vue` 或对应导航中增加入口（如需要）。
4. 业务逻辑放 composable，UI 子块放 components。
5. 添加类型到 `types/`，避免 view 内堆大量 inline shape。

## Views 层规则

`views/` 放“页面容器”，负责：

- 组合 domain composables。
- 管理当前页面 tab / route param。
- 连接页面级 loading/error 状态。
- 组织 components 布局。
- 做少量页面专属 glue code。

避免：

- 在 view 内堆大量 fetch / WebSocket / 数据转换逻辑。
- 在 view 内定义可复用类型和算法。
- 在 view 内直接复制另一个页面的状态管理。

## Components 层规则

`components/<domain>/` 放可复用 UI：

- 接收 props。
- emit 用户动作。
- 尽量不直接做全局 API 副作用。
- 复杂 UI 拆小组件：toolbar、modal、panel、table、drawer、chart。

领域目录：

- `components/agentsight/`
- `components/config/`
- `components/config/ml/`
- `components/dashboard/`
- `components/docs/`
- `components/execution-graph/`
- `components/executor/`
- `components/explorer/`
- `components/hooks/`
- `components/monitor/`
- `components/network/`
- `components/plugins/`
- `components/terminal/`

## Composables 层规则

`composables/<domain>/` 放状态、API、WebSocket、数据转换、可复用行为：

- 命名使用 `useXxx.ts`。
- 返回 refs/computed/actions。
- 负责 cleanup（WebSocket、interval、event listener）。
- 对 API endpoint、auth token、error handling 做封装。
- 复杂 domain 分多个 composables，避免一个巨大 composable。

重要 composables：

- `composables/dashboard/useDashboard.ts`
- `composables/dashboard/useDashboardStream.ts`
- `composables/monitor/useMonitorData.ts`
- `composables/network/useNetworkInterfaces.ts`
- `composables/network/useNetworkEnrichment.ts`
- `composables/network/useTrafficGraph.ts`
- `composables/executor/useShellSessions.ts`
- `composables/executor/useLaunchEnv.ts`
- `composables/config/useConfigRuntime.ts`
- `composables/config/useConfigSecurity.ts`
- `composables/config/useConfigRegistry.ts`
- `composables/config/useConfigML*.ts`
- `composables/agentsight/useAgentSightEvents.ts`
- `composables/plugins/usePlugins.ts`

## Types / data / utils

### `types/`

用于跨组件共享类型：

- `types/config.ts`
- `types/executionGraph.ts`
- `types/filePreview.ts`
- `types/hooks.ts`
- `types/shell.ts`

新增 API response / config / UI model 时，优先放到这里或 colocated domain type 文件。

### `data/`

静态 catalog：

- `data/hookCatalog.ts`：AI CLI hook catalog。
- `data/linuxReferenceCatalog.ts`：Linux reference catalog。
- `data/mlModelCatalog.ts`：ML model catalog。

新增 provider/model/catalog 选项优先检查这里。

### `utils/`

通用工具：

- `utils/agentsight.ts`
- `utils/requestContext.ts`
- `utils/tmux.ts`

不要把 domain-specific 大逻辑塞进 `utils/`，更适合放 domain composable。

## Protobuf 前端生成物

路径：

- `frontend/src/pb/tracker_pb.js`
- `frontend/src/pb/tracker_pb.d.ts`

这些由 `make proto` 生成，不要手改。需要新字段时从 `proto/*.proto` 改起。

## 样式组织

- 全局样式：`frontend/src/style.css`。
- 页面专属样式：如 `views/execution-graph/execution-graph.css`。
- 复杂组件专属 CSS：如 `components/plugins/*.css`。
- 维持现有 class 命名与布局风格，不引入无关 CSS 框架。

## 页面与文件快速索引

### Dashboard

- View：`views/dashboard/Dashboard.vue`
- Components：`components/dashboard/DashboardEventModal.vue`、`DashboardToolbar.vue`
- Composables：`composables/dashboard/useDashboard.ts`、`useDashboardStream.ts`
- Helpers：`dashboardConstants.ts`、`dashboardHelpers.ts`

### Monitor

- View：`views/monitor/Monitor.vue`
- Components：`components/monitor/*`
- Composables：`composables/monitor/useMonitorData.ts`、`useSensors.ts`、`useSystemd.ts`

### Network / TLS

- Views：`views/network/Network.vue`、`NetworkFlow.vue`、`TLSCapture.vue`
- Components：`components/network/*`
- Composables：`composables/network/*`

### ExecutionGraph / AgentSight

- View：`views/execution-graph/ExecutionGraph.vue`
- CSS：`views/execution-graph/execution-graph.css`
- Components：`components/execution-graph/*`、`components/agentsight/*`
- Composables：`composables/execution-graph/*`、`composables/agentsight/*`

### Explorer

- View：`views/explorer/Explorer.vue`
- Components：`components/explorer/FileBrowserPanel.vue`、`FilePreviewDrawer.vue`、`PathNavigatorDrawer.vue`
- Types：`types/filePreview.ts`

### Executor

- View：`views/executor/Executor.vue`
- Components：`components/executor/ExecutorLaunchEnvTab.vue`、`components/terminal/*`
- Composables：`composables/executor/*`
- Types：`types/shell.ts`

### Hooks

- View：`views/hooks/Hooks.vue`
- Components：`components/hooks/*`
- Data：`data/hookCatalog.ts`
- Types：`types/hooks.ts`

### Config / ML

- Config View：`views/config/Config.vue`
- Config Components：`components/config/*`
- ML View：`views/ml/ML.vue`
- ML Components：`components/config/ml/*`
- Composables：`composables/config/*`
- Types：`types/config.ts`

### Plugins

- View：`views/plugins/Plugins.vue`
- Components：`components/plugins/*`
- Composables：`composables/plugins/*`

## Vue 开发硬性约定

- 使用 Composition API。
- 使用 `<script setup lang="ts">`。
- 保持现有 Ant Design Vue 组件风格。
- API/WS 状态逻辑优先放 composable。
- 复杂 UI 拆组件，不在单个 `.vue` 中无限膨胀。
- 修改 Vue 项目时可结合 `vue-best-practices` 和 `vue-development-guides` skills。

## 前端验证

最小验证：

```bash
cd frontend && bun run build
```

若只改静态 data/types，也仍建议跑 build，因为它包含 `vue-tsc`。

若改 Vite 配置：

- 检查 `frontend/vite.config.ts`。
- 关注 `backend/.port` dev proxy 读取逻辑。
- 可运行 `make frontend` 或 `cd frontend && bun run build`。
