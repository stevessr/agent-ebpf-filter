# Agent eBPF Filter：公开危险行为证据与逐条研读标注（2026-10-23）

本目录是**可审查的威胁特征研究语料**，不是预训练标签集，也不是生产阻断验证。资料来源包括 MITRE ATT&CK、Falco、OWASP Agentic Top 10、AgentDojo 和 PrivacyAlign。收集 11 条公开资料索引、对其逐条研读形成 33 条**分析者构造的对照场景**（危险候选 11、明确授权的正常对照 11、证据不足 11），另从 MITRE ATT&CK 的 *Procedure Examples* 摘要转录 11 条**有出处的恶意技术案例**。案例为独立简述，不复制攻击代码、第三方样本原文、凭据或可识别个人数据。

## 文件

- [sources.json](sources.json)：核查来源、准确 URL、来源类型与支持范围。
- [cases.jsonl](cases.jsonl)：33 条独立构造的纸面特征场景，**不是从互联网下载的真实 event trace**。
- [documented_cases.jsonl](documented_cases.jsonl)：8 条从 ATT&CK Procedure Examples 核对的已报告恶意行为，**不包含 syscall、HTTP request 或 Agent provenance 原始日志**。
- [audit.py](audit.py) / [test_audit.py](test_audit.py)：无额外依赖的血缘和字段约束测试；拒绝将语义推断涂改为人工标注或已证实的执行结果。

## 人工式研读 / 标注边界

逐条以「公开来源描述 → 可能观测信号 → 相反的正常业务解释 → 必要授权证据 → 缺失证据」的顺序审阅，写出独立中文场景、证据需求和误报反例。每个 `CUR-*` 案例都显式标记 `provenance=model_assisted_source_review`、`telemetry_origin=constructed_hypothetical`、`human_verified=false`、`ground_truth_label=null`、`actual_outcome=not_observed`。因此 **“risk_candidate” 只能理解为给出完整情境后的研究风险候选，绝不是恶意样本的真值 1**；`benign_control` 也不是现场认证的生产白样本。若缺少上下文、实际成功调用或授权信息，标 `insufficient_evidence` 而不是强迫二分类。

| 分析类 | 研读重点 | 能否由 syscall 单独证明 |
|---|---|---|
| 凭据访问与对外发送 | 成功的敏感文件访问、目标域/账户、上传内容关联、用户批准 | 否；无数据流关联或报文证据时只属风险 |
| 大规模破坏/加密 | 操作成功率、文件可重建性、工作空间、权限、备份与可恢复性 | 只能确认局部文件效果，不能只凭调用判定意图 |
| 权限提升/日志删除 | 变更前后权限、行动者、审计保留策略、事件先后关系 | OS 信号常不足，还需 IAM/DB/Win audit |
| 非预期 Shell / 容器越界 | Web worker 进程身份、子进程实际 exec、容器入口与允许列表 | 有效进程身份可观测；是否越权仍需应用策略 |
| Prompt injection/MCP 工具滥用 | 原始用户目标、非可信工具内容、调用参数、审批与真实副作用 | **不能**；必须有可信 harness/tool 层记录 |
| 隐私上下文共享 | 来源敏感度、接收者与目的、用户分享规范、实际传出内容 | **不能**；网络连接不是泄露证明 |

## 来源案例和限制

例如：
- ATT&CK [T1485](https://attack.mitre.org/techniques/T1485/) 的 C0034、C0063 分别报告擦除相关文件和覆盖破坏文件，提取「跨目标覆盖/删除异常」而不伪造某个 `unlink()` 的返回值；
- [T1486](https://attack.mitre.org/techniques/T1486/) 的 S1133、S1129 报告恶意加密现象，但加密意图/结果不能仅通过高写入量来识别；
- [T1567](https://attack.mitre.org/techniques/T1567/) 的 C0062、C0059、S0622 包含 Agent 辅助资料整理及利用合法 Web/API 通道外传的真实报告；C0062 的报告描述攻击者确认外传，不等于用户合法批准，也未证明 Agent 自主执行外传；
- [T1098](https://attack.mitre.org/techniques/T1098/) 的 C0025、G0125 是账户/权限操作证据，Linux 网络流量不足以等价认定；
- [T1070.004](https://attack.mitre.org/techniques/T1070/004/) 的 S0504、S1167 说明自删除存在于恶意软件中，但普通临时文件清理是必须保留的困难负例；
- [Falco default rules](https://falco.org/docs/reference/rules/default-rules/) 的规则有范围和噪声注意事项。Shell、敏感文件、外连请求均应结合应用身份；
- [AgentDojo](https://arxiv.org/abs/2406.13352) 与 [workspace attack tasks](https://github.com/ethz-spylab/agentdojo/blob/main/src/agentdojo/default_suites/v1/workspace/injection_tasks.py) 支持将不可信内容诱导未授权工具调用列为研究场景，并**不**提供 eBPF syscall 真值；
- [PrivacyAlign](https://github.com/ServiceNow/PrivacyAlign) 本身以**合成情境**和独立的人类偏好标注研究 Agent 隐私；本目录没有复制其匿名的人类标签，也没有声称这里是人工众包标注；
- [OWASP Agentic Top 10 2026](https://genai.owasp.org/resource/owasp-top-10-for-agentic-applications-for-2026/) 仅做威胁分类参考，不能作为任何特定执行事件的事实依据。

## 可用与不可用

这份语料可以用于：构建 feature hypothesis checklist、来源/Tag 归一化检查、训练数据审计规范、拒绝模棱两可信号的 abstain 测试、下一轮独立审核问卷。**不得**合并到 PrivacyAlign 训练分区，不得作为标为 0/1 的独立攻击样本，不得由本语料计算攻击检测 FPR / Recall / PR-AUC，更不得用它授权 production BLOCK。不存在任何核验过的 Agent 真实 syscall 拦截率提升。

下一阶段必须采集来自批准测试环境的脱敏 Hook + syscall/HTTP 真实事件：保存 `(host,run,tool_call,process_start)` 稳定身份、event time、source、evidence level、返回结果、数据流证据和审批状态，另由**独立人类复核**目标动作是否违反实际任务授权；按来源/场景/时间封存分组并建立不共享原始事件的 train / calibration / held-out。真实 malicious/benign 标注只有具备额外可复核证据时才创建。

## 复核与测试

```bash
python3 scripts/research/behavior-annotation-20261023/audit.py
python3 -m unittest discover -s scripts/research/behavior-annotation-20261023 -p 'test_*.py' -v
```

验证报告应显示 `real_system_call_traces: 0`、`independent_human_ground_truth: 0`、`production_training_authorized: false`；无法通过这些标志推断模型效果。专用 CI 位于 `.github/workflows/decision-training-contracts.yml`。
