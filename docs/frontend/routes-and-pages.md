# 路由与功能页

前端路由定义在 `frontend/src/router/index.ts`。

## | Route | Name | Component | Feature meta |
| --- | --- | --- | --- |
| `/` | redirect | `/dashboard` | - |
| `/dashboard/:tab?` | Dashboard | `views/dashboard/Dashboard.vue` | - |
| `/monitor/:tab?/:subtab?` | Monitor | `views/monitor/Monitor.vue` | - |
| `/network` | Network | `views/network/Network.vue` | - |
| `/network-flow/:tab?` | NetworkFlow | `views/network/NetworkFlow.vue` | - |
| `/tls-capture` | TLSCapture | `views/network/TLSCapture.vue` | `tls_capture` |
| `/agentsight/:tab?` | redirect | ExecutionGraph behavior | - |
| `/execution-graph/:tab?` | ExecutionGraph | `views/execution-graph/ExecutionGraph.vue` | - |
| `/explorer` | Explorer | `views/explorer/Explorer.vue` | - |
| `/executor/:tab?/:subtab?` | Executor | `views/executor/Executor.vue` | `shell_sessions` |
| `/hooks` | Hooks | `views/hooks/Hooks.vue` | `hooks` |
| `/ml/:subtab?` | ML | `views/ml/ML.vue` | `ml` |
| `/plugins/:tab?` | Plugins | `views/plugins/Plugins.vue` | `plugins` |
| `/config/ml/:subtab?` | redirect | ML legacy route | - |
| `/config/:tab?/:subtab?/:subsubtab?` | Config | `views/config/Config.vue` | - |
| `/feature-unavailable` | FeatureUnavailable | `views/config/FeatureUnavailable.vue` | - |

## Feature unavailable guard

`router.beforeEach()` 会读取 route meta 中的 feature，并调用 `isFeatureIncludedInFrontendBuild(feature)`。如果当前前端 build 未包含该 feature，则跳转到：

```text
/feature-unavailable?feature=<id>&from=<originalPath>
```

## 1. 在 `views/<domain>/` 创建页面容器；
2. 在 `router/index.ts` 增加 lazy route；
3. 需要时在 `App.vue` / navigation 中添加入口；
4. API / WS 状态放 composable；
5. UI 子块放 `components/<domain>/`；
6. 类型放 `types/`；
7. 运行 `cd frontend && bun run build`。

---

## - [前端工作台](workbench.md)
- [组件与 Composables](components-composables.md)
- [构建与 Feature Flags](build-feature-flags.md)
- [路由与 API](../backend/routes-api.md)
- [代码入口索引](../reference/code-entrypoints.md)
