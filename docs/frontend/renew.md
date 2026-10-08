# 明镜高悬 · MyGo 原生桌面端

**察微知著 · 守护 Agent 边界。** Renew 曾是 Agent eBPF Filter 的浏览器日常监控页面，现已由 MyGo 原生桌面端「明镜高悬」取代。Web UI 中的 `/renew` 路由、组件和独立前端测试已删除；研究与管理功能仍由主工作台提供。

桌面端位于 `desktop/renew/`，以 Go + MyGo 构建，无 WebView、Vue 页面或 Vite 运行时。窗口展示「明镜高悬」品牌，构建产物、应用标识符和原有命令 `agent-ebpf-renew` 暂保持兼容。

## 启动与通信

内置后端通过授权运行，但不监听 HTTP TCP 端口。桌面创建一个私有 0700 Unix Socket 目录：一个 lifetime/Native IPC socket 用于事件与系统状态、一个 API socket 用于已鉴权的配置、历史和详情请求。权限边界沿用原有 token 与 runtime policy gate。关闭桌面会结束**自己**启动的后端。连接用户已有的后端时可继续复用原有网络 API（不会由桌面额外开启监听）。

## 跟踪范围

跟踪命令支持按标签、启用/禁用状态筛选。文件、文件夹规则支持标签筛选；目前没有禁用状态字段，只能删除。MyGo 原生文件选择器区分三种输入：

- **文件夹内文件**：登记当前一级目录中的普通文件，不递归且不自动追踪未来新文件。
- **文件**：精确文件路径。
- **文件夹及其子文件**：递归路径前缀，覆盖子目录及新文件。

## 默认降噪

新配置默认忽略 `/proc` 和 `/tmp` 的低风险事件，保留明确的告警、阻断和高风险事件。另对 `/usr/bin` 下的低风险读写事件降噪，但**不**忽略执行、改名或策略风险事件。已有持久化的 `ignoredPaths` 保持不变；显式设置 `[]` 可以关闭自动噪声规则。

## 开发与验证

```bash
make renew-desktop-dev
make renew-desktop-build
cd desktop/renew && go test ./...
```

详细工程说明见 [desktop/renew/README.md](../../desktop/renew/README.md)。
