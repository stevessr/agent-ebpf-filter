# Dashboard Activity Insights

Dashboard 的 **Activity Insights** 是一层基于当前实时事件缓冲区的轻量聚合视图，设计灵感来自 CC-Monitor 的会话统计、工具/MCP 调用统计、文件/软件安装/Git 操作统计、AI 网络轨迹，以及应用层与系统层交叉验证思路。

它不是 CC-Monitor 的移植层，也不依赖 Claude Code 私有配置、transcript 或 PTY。所有统计都直接消费 Agent eBPF Filter 现有的 `AgentEvent` 数据，因此适用于已经接入本项目事件协议的不同 AI agent / developer CLI。

## 展示内容

- **Agent Runs**：优先按 `agentRunId` 聚合；缺失时回退到 `conversationId`，再回退到 `rootAgentPid`。
- **Tool Calls**：优先对 `toolCallId` 去重；没有 `toolCallId` 的 `wrapper_intercept` / `native_hook` 事件按事件次数计数。
- **MCP Calls / Servers**：识别 `mcp__<server>__<tool>` 命名格式并按 server 聚合。
- **Alerts / Blocks / High Risk**：结合 `semantic_alert`、`agentsight_alert`、`decision` 与 `riskScore` 汇总。
- **Sensitive Files**：统计对 `.ssh`、`.aws/credentials`、`.kube/config`、`.env`、常见 credentials/secrets/token 文件以及关键 `/etc` 路径的文件事件。
- **AI Trajectory**：从 `domain` 或 `netEndpoint` 归一化出网络目的地，并显示当前缓冲区内的 Top targets。
- **文件操作**：读、写、删除、改名/权限/链接/目录等元数据修改。
- **软件安装**：Python/uv、系统包管理器、Node 全局安装和其它常见安装命令。
- **Git / GitHub 操作**：`git push`、`clone`、`commit`、`pull/fetch`、`gh` CLI 与其它 git 操作。
- **最近 Agent Runs**：展示每个 run 的事件量、工具调用、告警/阻断、网络目标数与最后活跃时间。

## 双层交叉验证：Semantic Gap

项目原本就同时具备语义层事件（wrapper / native hook）与内核层 eBPF 事件，因此可以直接实现类似 CC-Monitor “系统层兜底验证应用层”的能力。

当前实现只对带有至少一个关联字段的 `execve` / `process_exec` 事件进行验证：

- `toolCallId`
- `spanId`
- `traceId`

如果一个内核执行事件的任一关联字段能在 `wrapper_intercept` / `native_hook` 事件中找到匹配，则记为 **paired**；否则记为 **Semantic Gap**。

```text
coverage = paired correlated kernel execs / all correlated kernel execs
```

这里刻意忽略没有任何语义关联字段的普通内核事件，避免把无法关联的系统噪声误报成 “hook 被绕过”。该指标仍然是当前事件缓冲区内的在线启发式结果，不等价于取证级证明。

## 实现位置

- 聚合逻辑：`frontend/src/composables/dashboard/dashboardInsights.ts`
- UI：`frontend/src/components/dashboard/DashboardInsights.vue`
- 页面接入：`frontend/src/views/dashboard/Dashboard.vue`
- 单元测试：`frontend/tests/dashboard-insights.test.ts`

运行测试：

```bash
cd frontend
bun run test:dashboard-insights
```

完整前端类型检查和构建：

```bash
cd frontend
bun run build
```

## 后续可扩展方向

当前实现故意保持为无后端状态、无新增协议字段的低侵入版本。后续若要继续吸收 CC-Monitor 的思路，优先级建议是：

1. 把 Semantic Gap 聚合下沉到后端，允许跨 Dashboard 缓冲区和持久化窗口做验证；
2. 增加可持久化的审计快照/归档，而不是只看当前浏览器缓冲区；
3. 在现有策略引擎上增加带 TTL 的临时批准（allow once / allow for N minutes / session allow）；
4. 把网络 destination 的本地 GeoIP enrichment 与现有 Network 页面联动；
5. 对支持 transcript / OTEL / agent-native hooks 的适配器增加会话级语义回放，而不是绑定某一种 AI CLI。
