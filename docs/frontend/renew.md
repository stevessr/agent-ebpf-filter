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

Renew 不新增数据库状态，也不会改变事件模型。

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
