# 组件与 Composables

前端维护质量取决于是否保持 views、components、composables、types 的边界。

## Views

`views/` 是页面容器，负责：

- 读取 route params；
- 组合 composables；
- 管理页面级 loading / error；
- 组织布局；
- 少量 glue code。

避免在 view 中：

- 直接堆大量 fetch；
- 维护复杂 WebSocket 生命周期；
- 定义大量可复用类型；
- 复制另一个页面的状态逻辑。

## Components

`components/<domain>/` 放可复用 UI。

常见领域：

- `components/dashboard/`
- `components/network/`
- `components/execution-graph/`
- `components/agentsight/`
- `components/executor/`
- `components/hooks/`
- `components/config/`
- `components/config/ml/`
- `components/plugins/`
- `components/monitor/`
- `components/terminal/`

组件应：

- 接收 props；
- emit 用户动作；
- 少做全局副作用；
- 拆 toolbar、modal、drawer、table、chart、panel。

## Composables

`composables/<domain>/` 放：

- API 请求；
- WebSocket；
- state；
- computed；
- data transform；
- cleanup。

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
- `composables/agentsight/useAgentSightEvents.ts`
- `composables/plugins/usePlugins.ts`

## Types / Data / Utils

| 目录 | 责任 |
| --- | --- |
| `types/` | API response、config、UI model、shared type |
| `data/` | hookCatalog、linuxReferenceCatalog、mlModelCatalog |
| `utils/` | requestContext、agentsight、tmux 等通用工具 |
| `pb/` | generated protobuf JS/TS，不手改 |

## WebSocket cleanup

任何 composable 创建 WebSocket、interval、event listener 时，都必须在 unmount 或 stop action 中 cleanup。

---

## 相关导航

- [前端工作台](workbench.md)
- [路由与功能页](routes-and-pages.md)
- [构建与 Feature Flags](build-feature-flags.md)
- [事件管线](../backend/event-pipeline.md)
- [代码入口索引](../reference/code-entrypoints.md)
