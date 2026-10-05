# Renew 日常监控前端

Renew 是 Agent eBPF Filter 的轻量前端变种，面向把服务常驻在个人 Linux 工作站上的日常使用场景。

它不会替代原来的专业工作台，也不会引入新的后端协议。Renew 直接复用现有的事件 WebSocket、系统指标 WebSocket、鉴权和请求上下文；当用户需要查看 syscall、完整网络流、执行图或策略细节时，再跳转回原工作台。

## 入口

- 路由：`/renew`
- 导航：观测分析 → Renew
- 组件：`frontend/src/views/renew/Renew.vue`

进入 `/renew` 时，`App.vue` 会根据路由的 `meta.uiVariant = "renew"` 使用独立壳层，因此不会显示研究型工作台的多标签页和完整功能树。

## 设计目标

1. **先回答“现在正常吗”**：首页只突出采集连接、CPU、内存、待处理异常。
2. **把 syscall 翻译成动作**：默认显示“读取文件”“连接到目标”“调用工具”等自然语言摘要，原始字段留在专业工作台。
3. **颜色只用于需要处理的状态**：普通运行保持中性色；告警、拒绝和高风险事件才使用黄/红色。
4. **Agent 优先**：默认只展示带 Agent run / conversation / tool call / root PID 关联，或已标记 tag 的事件。
5. **渐进披露**：事件、网络、进程、规则都有一键深挖入口，但不会在首页同时展开。
6. **不复制参考项目代码**：只借鉴 CC Switch v4 的低噪声卡片/侧栏信息架构，以及 ArgusBPF 的“普通语言优先”监控思路。

## 数据来源

| Renew 区域 | 现有数据源 |
| --- | --- |
| 实时活动 | `/ws`，通过 `useDashboard()` |
| CPU / 内存 / 进程 / 网络吞吐 | `/ws/system`，通过 `useMonitorData()` |
| Agent 会话摘要 | 当前事件缓冲区中的 `agentRunId` / `conversationId` / `rootAgentPid` |
| 待处理 | `decision`、`riskScore`、`semantic_alert`、`agentsight_alert` |
| 已配置跟踪进程 | `/system/tracked-comms` |

Renew 不改变事件模型；日常界面只消费紧凑摘要，完整事件历史由后端持久化层统一管理并按需读取。

## 模块结构

Renew 不把业务继续堆进单个 view：

```text
frontend/src/
  views/renew/
    Renew.vue                 # 只做页面组装
    renew.css                 # Renew 视觉系统
  components/renew/
    RenewSidebar.vue
    RenewHeader.vue
    RenewMetrics.vue
    RenewToolbar.vue
    RenewActivityPanel.vue
    RenewAttentionPanel.vue
    RenewAgentSessions.vue
    RenewSystemSnapshot.vue
  composables/renew/
    useRenewDashboard.ts      # 生命周期与页面编排
    useRenewEventSummary.ts   # 聚合统计 / Agent 会话 / 网络目标
    eventPresentation.ts      # syscall -> 日常语言、风险/标签规则
    types.ts                  # Renew view model
```

其中 `eventPresentation.ts` 保持纯函数，便于通过 Bun 单元测试验证，不依赖页面或 WebSocket。


## 监控中心与默认开销

监控中心把常驻观测拆成可独立开关的进程行为、文件变更、基础网络、高频文件访问、网络细节与 generic syscall 组，并把循环检测、行为信号、研究分析、TLS 明文捕获和本地持久化作为独立运行时开关。Lite / 日常 / 深度三个档位只是一键组合，单个模块仍可继续细调。

新安装采用“日常”对应的事件采集基线：进程、文件变更与基础网络保持开启；高频 `open/read/ioctl`、详细 send/recv/socket 与 generic syscall 默认关闭。已有安装如果运行时配置中还没有 `disabledEventTypes` 字段，则保留历史全采集行为，避免升级时静默缩小观测范围。之后通过 Renew 或旧 `/config/event-types` API 修改的禁用列表都会持久化到 `runtime.json`，重启后继续生效。

这个交互只借鉴终端安全软件“模块化防护中心、默认合理开启、需要时再打开高成本能力”的产品思路，不复制第三方代码、规则或界面资源。


### 默认路径降噪

日常监控还会在**语义分析之后、归档 / Pebble / WebSocket 之前**应用运行时 `ignoredPaths`。默认值为 `["/proc"]`，等价于忽略普通 `/proc/**` 遥测；匹配使用路径边界，因此不会把 `/procfs` 误判为 `/proc` 子路径。

该规则只用于降噪，不改变内核采集和安全判断顺序。原始事件仍先进入语义分析；带 `BLOCK` / `DENY` / `ALERT` 决策、`riskScore >= 60`、`semantic_alert` 或 `agentsight_alert` 的事件始终绕过路径忽略并正常进入历史与实时流。

`ignoredPaths` 由 `/config/runtime` 和 `runtime.json` 持久化管理。用户可追加其他绝对路径前缀，也可以显式设置 `"ignoredPaths": []` 关闭默认路径过滤；空数组会被保留到配置文件中，因此重启后不会重新套用 `/proc` 默认值。

## 持久化与前端内存模型

Renew 默认启用本地事件持久化。完整事件由后端写入：

```text
~/.config/agent-ebpf-filter/events.pebble
```

这里使用 **Pebble**（纯 Go 的 LSM KV 数据库），而不是在 Renew 前端长期保存完整事件对象。RocksDB 技术上完全可用，但常见 Go bindings 会引入 cgo、原生 RocksDB 及压缩库的构建/分发负担；Pebble 提供同类 LSM、批量写入与顺序迭代能力，同时保持纯 Go，更适合当前 Go 1.27.1、Linux 桌面包和 CI 部署方式。若未来出现必须依赖 RocksDB 特性的场景，可再把存储层抽象为可替换 backend，而不是现在为桌面常驻场景承担额外原生依赖。

数据路径为：

```text
eBPF / hooks / wrapper
        │
        ▼
backend normalize + redact
        ├──► Pebble：完整事件、ID 索引、审计链
        ├──► /events/summaries：紧凑历史摘要（Pebble 游标分页）
        ├──► /ws/event-summaries：紧凑实时摘要
        └──► /events/detail/:id：用户点开时读取一条完整事件
                         │
                         ▼
                       Renew
```

Renew 浏览器/桌面前端只保留一个有界摘要窗口；用户需要更早记录时通过后端返回的不透明 `nextCursor` 逐页加载。完整详情只在用户点开单条事件时从后端读取，并在详情关闭后释放，因此磁盘历史增长不会把完整事件复制进 WebView/浏览器内存。

默认 Pebble 历史策略为 250,000 条完整事件或 168h，先达到的限制触发最旧记录淘汰；它由 `eventStoreMaxRecords` / `eventStoreMaxAge` 独立控制。后端内存 hot archive 默认 1,500 条，由 `maxEventCount` / `maxEventAge` 单独控制，因此扩大磁盘历史不会同比扩大常驻内存。

显式配置的旧 `.jsonl` 路径仍保持兼容；新安装默认使用 Pebble。

## MyGo 桌面版

桌面壳位于 `desktop/renew/`，使用 MyGo 0.2.7，并作为**独立 Go module** 维护。

它不复制 Vue 产物、不另起 API 代理，而是直接让系统 WebView 加载后端真实地址，例如：

```text
http://127.0.0.1:8080/renew
```

因此 REST、protobuf、WebSocket、认证 localStorage 都继续使用与 Web 版完全一致的同源路径。

桌面壳只负责：

- 单实例；
- 原生窗口和窗口状态记忆；
- `AGENT_BACKEND_URL` / `--backend` 后端地址选择；
- 后端未启动时的提示页；
- MyGo 的 Linux / Windows / macOS 打包。

MyGo 要求 Go 1.27.1+。仓库现已统一到 Go 1.27.1，并将 `desktop/renew` 纳入根 `go.work`；桌面端仍保持独立 `go.mod`，避免 MyGo 依赖进入特权后端 module。

运行和打包：

```bash
make renew-desktop-dev
make renew-desktop-build
```

或直接：

```bash
cd desktop/renew
go tool mygo dev
go tool mygo build -platform linux/amd64
```

Linux 运行时使用系统 WebKitGTK 4.1。桌面壳的详细说明见 `desktop/renew/README.md`。

## 与专业工作台的边界

Renew 适合：

- 常驻桌面监控；
- 快速判断 Agent 是否正在活动；
- 查看最近文件/网络/工具动作；
- 集中查看 BLOCK / DENY / ALERT / 高风险事件；
- 快速进入网络、进程、规则页面。

原工作台继续承担：

- syscall 和原始字段分析；
- 复杂过滤与导出；
- Execution Graph / Research；
- TLS capture；
- ML、Plugins、Executor；
- 全量配置与诊断。

## 验证

前端改动应至少通过：

```bash
cd frontend
bun install
bun run build
```

日常手工验证：

1. 打开 `/renew`，确认不显示原工作台侧栏和 tabs。
2. 启动已接入 Agent，确认“最近活动”和“Agent 会话”实时更新。
3. 触发带 `riskScore >= 60` 或 `BLOCK` 的事件，确认进入“需要关注”。
4. 点击事件、网络、进程、规则入口，确认能返回专业工作台对应页面。
5. 暂停事件流后确认系统指标仍可继续刷新。
