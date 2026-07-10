# 构建与 Feature Flags

前端 build 会受 `AGENT_FRONTEND_BUILD_FEATURES` 影响，用于裁剪或隐藏某些功能页面。

## 构建命令

```bash
cd frontend && bun run build
```

实际执行：

```text
vue-tsc -b && vite build
```

根 Makefile 中：

```bash
make frontend
```

会执行 Bun install，并传递：

```text
VITE_AGENT_BUILD_FEATURES="$(AGENT_FRONTEND_BUILD_FEATURES)"
```

## Route feature meta

路由中通过：

```ts
meta: featureMeta('tls_capture')
```

标记功能页。常见 feature：

- `tls_capture`
- `shell_sessions`
- `hooks`
- `ml`
- `plugins`

如果 feature 未包含在当前 build 中，router guard 会跳转到 FeatureUnavailable 页面。

## 与后端 feature 的关系

前端 feature flags 控制 UI 可见性；后端 build tags 和 runtime gates 控制 API 与能力可用性。

不要假设：

- 前端页面可见 = 后端能力启用；
- 后端 compiled in = runtime gate 已开启；
- runtime gate 开启 = release mode 不需要 auth。

## 验证

- 修改 Vue / TS：`cd frontend && bun run build`；
- 修改 route / feature flags：测试 feature-unavailable 跳转；
- 修改 API schema：同步 types / composables / backend docs。

---

## 相关导航

- [前端工作台](workbench.md)
- [路由与功能页](routes-and-pages.md)
- [Runtime Settings 与 Feature Manifest](../backend/runtime-settings-features.md)
- [Runtime Gates 与 Auth](../security/runtime-gates-auth.md)
- [维护检查清单](../reference/maintenance-checklists.md)
