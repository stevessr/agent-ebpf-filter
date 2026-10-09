# Agent eBPF Filter：在线异常检测模型与证据分层（2026-10）

> 状态：第一阶段已实现，属于**工程性研究启发**，不是论文复现。模型分数是启发式风险下限，**未经统计校准**，不能解读为攻击概率；本阶段不修改内核同步阻断策略。

## 1. 现实问题与不变量

旧版 Tool Baseline 在 16 次事件、3 种行为后成熟：首次出现任意 `(comm, event_type)` 立即以相同的 `TOOL_BEHAVIOR_DRIFT` 严重度告警，随后把该行为加入基线。普通 `read/write/open`、短时间突发以及真正的网络传输工具因此无法区分。

此外，`write(2)` 在 tracepoint 中只有 FD、没有可信路径，不允许把 `file write` 当成文件路径；不同的普通 PID 不能证明不同 Agent。语义检测必须先满足证据有效性，再评估行为偏差。

安全不变量：
- 不因模型的不确定性而推断不存在的进程、文件、因果关系。
- 规则与模型给出的风险等级必须分开看待；没有标签验证的数据不得用于宣称“准确率”。
- 模型建议不得直接升级为 BPF LSM、cgroup 或 wrapper 的不可逆自动阻断。
- 仅处理元数据；不因训练而保存凭据、prompt 明文或 TLS 明文。

## 2. 已实现：轻量级双阶段在线基线

**输入**：工具名称 `tool_name` 与被观察进程的 `comm`、`event_type`。保持原有存储上限：512 工具、每工具 128 个行为样本、24h TTL。

**成熟基线**：至少 16 个历史样本和 3 种行为；未成熟时只学习，不发漂移告警。

**高信号分支**：首次出现典型外部传输命令（如 `curl` / `ssh` / `socat`）的 `execve/process_exec` 或网络连接、传输行为时，立即发出一次带原因的告警。高信号集合是可审计的专家先验，不是训练所得概率。

**普通新行为分支**：第一次观察将其标记为 `pending`；第二个观测至少在 1s 后出现才确认偏离。如果同一毫秒内被捕获上千次，不能算作两个独立时间证据。确认后只提醒一次，防止告警风暴；过期样本按既有 TTL 淘汰。候选计数直接存储在有界样本中，不额外分配无界事件历史。

**风险层级**：高信号新行为继承现有 91/100 风险下限；经过时间确认的普通新行为使用 60/100 风险下限。这是告警优先级而非概率校准，`TOOL_BEHAVIOR_DRIFT` 说明文本列出模型的时间证据。风险仍可以因为原始事件已有更高风险而提高。

## 3. 已实现：独立 Agent + 真实目标的语义证据门

跨 Agent 文件争用只对实际解析的文件路径建立窗口，不对 `file write`、`file writev`、FD、socket/pipe 描述符、脱敏占位符或无绝对 CWD 的相对路径建立共享目标。需要可比较的 Agent 根 PID 或明确 Agent Run ID。两种身份来源不能直接相互判定不同 Agent。

这只证明在一定时间窗口内观测到两个 Agent 的文件操作，不证明发生了 inode 级同时写入，也不能直接推定恶意或破坏性数据竞争。界面应保留语义类别、来源事件、历史进程和证据缺失说明。

## 4. 论文依据和与本项目的关系

1. Zhu et al., **The Case for Learned Provenance-based System Behavior Baseline**, ICML 2025, https://proceedings.mlr.press/v267/zhu25k.html 。提出动态系统行为基线、压缩图嵌入和 OOV 适应。本项目当前仅借鉴基线随事件更新的思想，**没有实现其图模型**。
2. Jiang et al., **TFLAG: Towards Practical APT Detection via Deviation-Aware Learning on Temporal Provenance Graph**, 2025, https://arxiv.org/abs/2501.06997 。强调时间上下文与行为偏离的联合分析；本项目当前用时间确认消除批量事件假阳性，**并非复现其神经网络**。
3. Dhanuka & Rastogi, **PROVEX: Enhancing SOC Analyst Trust with Explainable Provenance-Based IDS**, 2025, https://arxiv.org/abs/2512.18199 。强调关键因果证据展示；本项目只实现理由/来源字段说明，**尚未实现 GNN 子图解释**。
4. **Conformal prediction for labelling and updating online models in the presence of concept drift in cybersecurity**, *JISA* 2025, https://doi.org/10.1016/j.jisa.2025.104120 。论文明确报告伪标签在不同数据集上的效果不一致，因此本项目不将新异常自动标为恶意，也不宣称已实现 conformal 保证。
5. **Tidal: Tackling Concept Drift in Provenance-Based APT Detection**, NINeS 2026, https://drops.dagstuhl.de/entities/document/10.4230/OASIcs.NINeS.2026.1 。提出共享与特定任务层处理概念漂移，作为将来升级为分场景适应式表示的参考。
6. **From Agent Traces to Trust: Evidence Tracing and Execution Provenance in LLM Agents**, 2026, https://arxiv.org/abs/2606.04990 。综述从 Agent 事件、工具调用、证据与行为关系进行追踪的需求；本项目的 Agent run / root PID / tool call 作为后续因果关联输入。

## 5. 评估协议（尚未完成，不要当成项目成绩）

在带标签的回放集里按**时间、Agent run、项目仓库**拆分训练、验证、测试，防止同一进程或同一文件路径跨数据集泄漏。设置如下对照：

- 旧版“首次新行为即告警”基线。
- 本阶段双阶段时间证据模型。
- 仅高信号规则模型（消融实验）。
- 后续实现时可比较静态图异常检测与漂移适应模型，须使用相同测试集。

报告 `FPR` / 每万条正常事件误报数、`TPR`、`precision`、`F1`、PR-AUC、平均与 P95 检测延迟、突发/正常升级时表现、累计状态条目、内存峰值和 p95 单事件处理耗时。分别给出无明确路径、相同 Agent 不同 PID、重定向写入、时钟漂移、进程退出和 PID 复用的对抗性负例。

实际部署时先走 **shadow-only** 比较，若无法确定恶意标签，则报告告警事件数和人工复核结果，不报告不存在的准确率提升。阈值 `1s`、风险 `60/91` 须用验证集再校准，不得仅凭论文中的数字挪用本项目。

## 6. 后续阶段

在稳定的带标签回放集和误报复核渠道建成之后，引入仅针对同一 Agent run 的短窗口事件序列统计、跨 Agent 的有限因果边（进程→工具→文件/网络），并使用时间分段评估是否存在概念漂移。可选的 conformal 或重训练机制必须满足校准集的适用假设和影子运行验证；不要把未经核验的系统事件直接写成恶意训练样本。
