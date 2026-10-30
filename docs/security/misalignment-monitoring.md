# Agent 越权逃逸与对齐失范监控：证据优先的分层方案

参考：[OpenAI《我们如何检测前沿模型的对齐失范行为》，2026-10-28](https://openai.com/zh-Hans-CN/index/how-we-detect-frontier-model-misalignment/)。

## 本 PR 已实现的范围

这是一组 **研究端点中的 L0 元数据筛查扩展**，不是 OpenAI 内部多层系统的复现。保留现有 `/research/safety/evaluate` 的只读语义；不改变 BPF LSM、cgroup、wrapper 的运行时阻断策略。

- 原有精确 Grant 和祖先策略校验保持不变。
- Watchdog 新增 `network_attempt`、`network_connected`、`dns_query`、`filesystem_access`、`tool_exec`、`monitor_tamper`、`evaluation_artifact_mutation` 信号。
- 明确区分 **未批准的联网尝试**（P2 复核）和 **采集器报告已成功的未授权连接**（P1 调查）；成功与否必须由独立且可信的 syscall 返回值或等价遥测证明。
- DNS 解析不等同于成功连接；`dns_query.grant` 表示实际连接的 DNS 解析器端点，而不是查询的域名。
- 评估文件变更可能属于合法测试，因此进入 P2 上下文复核，而非直接判定破坏。
- `monitoring` 输出 `layer0`、`highestPriority`、`layer1Status`、是否需要 L2/L3、证据是否已验证和执行状态。
- 提供 `ShouldAuditUnflagged`：基于独立密钥的稳定近似 5% 未标记样本审计选择器。**它尚未接入实际事件采集调度**。

## 信号与严重度

| 采集器观察的事件 | 前提 | 初筛行为 |
| --- | --- | --- |
| `network_attempt` | 目标不属于 Agent 的精确网络授权 | P2；不宣称逃逸成功 |
| `network_connected` | 返回值证实连接成功且 Grant 未授权 | P1；需要独立日志验证 |
| `dns_query` | 解析器端点没有匹配的网络授权 | P2；仅代表疑似越界路径 |
| `filesystem_access`, `tool_exec` | 精确资源操作不在有效 Grant 中 | P1；要求安全调查 |
| `monitor_tamper` | 可信采集器报告监控遭修改的迹象 | P1；调查控制面完整性 |
| `evaluation_artifact_mutation` | 评测文件发生改动 | P2；检查任务上下文及授权 |

本端点最大建议级别为 **P1**；由于输入来自 API 调用方、没有独立证明链，**不会输出 P0 或实际隔离**。`monitoring.layer1Status=pending_not_run` 表示需要模型审查，**并不是已运行 LLM 模型**。

## 借鉴 OpenAI 的四阶段思想（拟议，不是已部署）

- **L0**：使用 eBPF、tool trace 和精确授权构建高召回候选，包含独立抽样审计。此 PR 只扩展研究 API 中的元数据判断。
- **L1**：模型审查需读取用户原始任务约束、工具调用和可用的行为摘要；不可凭独立的单行命令判定意图。该层尚未接入。
- **L2**：调查 Agent 的授权范围、连续行为、上下文窗口和因果证据；不得把理由说明当作系统事件。该层尚未接入。
- **L3**：将内核返回值、网络流量、文件变更、独立日志和历史案例交叉验证后，由经授权的控制器决定是否告警、暂停或隔离。该层尚未接入。

## 安全不变量

1. 用户提交的 JSON 不是可信系统证据。必须由独立采集器绑定 Agent Run ID、PID / 进程启动时间、namespace/cgroup、策略代际、内核事件序号及返回值。
2. 不能因观察到 `dns_query`、socket 尝试或模型描述就声称成功突破沙箱。
3. `monitor_tamper` 的 L0 信号只是调查线索；没有外部审计链，不能确认恶意或正确的 P0 升级。
4. 模型置信分数不等同于概率；任何模型建议不得绕过已有的 kernel / wrapper 授权执行路径。
5. L1 审查过程应异步，避免训练奖励或采样速度因模型主动隐藏违规意图而形成选择压力；不要把“是否被审查器识破”直接当成 RL 奖励。
6. 审计密钥必须存放在 Agent 不可访问的独立域。无私钥时抽样函数拒绝采用可预测的伪随机回退。
7. 仅保留必要元数据，避免上传 secrets、TLS 正文、私密 prompt 或未授权的 CoT 内容。

## 回归与评估

`go test ./internal/agentboundary -count=1` 覆盖授权连接与合法文件读取、未授权尝试/成功的区分、DNS 负例、监控篡改、评测文件变更、乱序信号、密钥抽样及只读 API 行为。

下一阶段需要独立带标签 replay 数据，以 **按 Agent Run、代码仓库和时间** 分组切分。比较 L0 单独、L0 + 模型审核、L0 + 模型审核 + 独立日志验证，分别报告 P0/P1/P2 召回、正常流量每万事件误报、首次报警延迟、p95 处理时延、模型调用预算、隐蔽逃逸测试和监控失效测试。当前 PR **没有训练新的分类器，也没有证明召回率改善**。

## 实时 Agent 可观测性需求（2026-10-29 研究补充）

MIT Technology Review 的 [实时 Agent 行为监控文章](https://www.technologyreview.com/2026/10/29/1147824/monitor-ai-agents-actions-real-time/) 与 [Partnership on AI 的六类遥测缺口研究](https://partnershiponai.org/resource/the-observability-gap-in-ai-agents/) 提醒我们：`agent_run_id`、PID、单行命令和风险分值不足以回答**谁拥有持续身份、谁批准了动作、权限或记忆何时改变**。系统必须区分可核实的执行结果与 Agent 自述的决策解释。

详见 **[Agent 实时行为监控：可观测性需求与验收规范](real-time-agent-observability.md)**。该规范将持久 Agent 身份、人工审批、权限/记忆变更、审计完整性、OTLP 映射和回放标准定义为待开发契约，并单独标出 CoT/logprob 的研究限制。

**状态强调**：本 PR 当前实现的 `monitoring` 仍只是只读 L0 研究初筛；新的审批/Memory 事件、持久 Agent ID、独立采集器、L1 模型审查及自动阻断均**尚未落地**。约 5% 的审计函数也尚未接入实际调度；关键审批与权限事件一旦实现应全量记录，绝不参与抽样。
