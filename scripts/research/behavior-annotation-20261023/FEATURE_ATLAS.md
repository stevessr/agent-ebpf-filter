# 危险行为特征拆解：现有 v2 可观测量与不可证实环节

研究范围：本表是基于 [公开来源](sources.json)、[33 条构造对照](cases.jsonl) 及 [11 条 ATT&CK 报告案例](documented_cases.jsonl) 的**特征假设审计**。不评估概率，不推导新的模型准确率。现有实现以 [`backend/internal/behaviorfeatures`](../../../backend/internal/behaviorfeatures/types.go) 的 `kernel-http-behavior.v2` 137 项 feature + present mask 为准，未改动旧 128-dim slot。

## 从攻击步骤到可证实事实

| 危险模式 | 优先观测链 | v2 已具备的特征（仅作 proxy） | 不足以推出的结论 | 新增证据最优先 |
|---|---|---|---|---|
| **凭据收集** T1552.001 | 准确进程身份 → 凭据位置 → 成功读取与字节数 | `join_context_complete`、`path.sensitive_hint`、`sys.op_read`、`sys.success`、`sys.actual_bytes` | 敏感路径词面不保证文件的真实 inode，也不保证内容是凭据 | 权威 FD→inode/路径归因、完整采集标记、进程授权范围 |
| **外传** T1567 | 已确认读取 → 明确 HTTP 上传 → 同一工具和来源 → 目标与授权冲突 | `history.30s.prior_sensitive_reads`、`http.outgoing`、`http.method_post`、`http.body_bytes` | 先读后 POST ≠ 相同内容被发送；TLS connect 不是 HTTP | 工具级 request correlation、接收方授权、可验证数据流/脱敏内容类别、响应状态 |
| **数据删除** T1485 | 目标边界 → 成功 unlink/rename → 多目标和恢复能力 | `sys.op_delete`、`sys.success`、`path.truncated`、`path.temporary_hint` | 文件指针删除 ≠ 不可恢复；构建缓存清理常为正常行为 | 目标唯一 inode/目录、备份状态、累计成功删除数、合法任务预期 |
| **大规模加密/覆盖** T1486 | 工作负载基线 → 跨文件写入 → 文件内容/可用性变化 | `sys.op_write`、`sys.actual_bytes`、`history.300s.prior_writes` | 写入与高熵都不足以证明勒索/不可用 | 新旧内容受控摘要、可用性变化、授权的加密备份状态 |
| **账户持久化/提权** T1098 | 角色/权限前后差异 → 行动者/工具 → 授权记录 | `sys.op_permission`（仅少数本地权限 syscall） | `chmod` 无法认定云 IAM、数据库账号或 Windows 域角色变更 | IAM/DB/Windows 审计和事件结果映射 |
| **删除入侵痕迹** T1070.004 | 先前可疑执行 → 删除对应恶意投递物/审计事件 → 可信对象关联 | `sys.op_exec`、`sys.op_delete`、`sys.success` | 删除临时文件并不自动等于 anti-forensics | 文件创建来源、日志对象类型、执行时序和调用链 |
| **Web worker 派生 Shell** Falco | 已核实的服务器进程身份 → 不可信请求入口 → 实际子进程 exec | `sys.op_exec`，少量 `join_context_complete` | 当前 v2 只保存单条进程上下文，不能自行可靠还原 Web worker 父链和恶意意图 | 可信 process ancestry、服务运行约定、用户操作和 allowlist |
| **Prompt injection / MCP 越权** AgentDojo、OWASP | 用户任务授权 → 不可信 tool result → 新的工具动作 → 参数偏离 → 实际效果 | syscall/HTTP 只能证明其中部分 OS effect | **不能仅凭 eBPF 判断提示注入**；MCP annotations 不可当执行约束 | 原生 Harness Hook、user approval、tool call ID、evidence provenance 和副作用 |
| **上下文隐私泄露** PrivacyAlign | 信息敏感度/来源 → 接收方 → 分享同意 → 实际输出 | `http.body_present` 与网络方向仅为外围元数据 | 不能把公开语料的文本分类分数当成实际个人信息泄露 | 独立隐私人工判断和有 consent 的脱敏输出证据 |

## 最能减少误报的变量（研究优先级）

1. **授权冲突，比危险关键词更重要**：用户允许的根目录/接收方/工具调用参数与实际操作逐项核对。MCP 标注、自报目的及进程命令行都不是信任来源。
2. **动作结果，比动作名称更重要**：read/open/write/unlink 必须区分尝试、失败、成功和真实 I/O 字节数；不存在的返回值要保留 missing mask。
3. **对象身份，比路径相似更重要**：`/tmp`、`/.env` 只可做 lexical hint；路径可能经 symlink/容器挂载转换。没有可信 FD 解析，不能从文件名猜实际读写对象。
4. **可信时序关联，比时间相邻更重要**：历史窗口按 `host + boot + run + PID namespace + TGID + start_time + clock` 关联；同一 tool call 的源重试去重。即使时间相邻也**不构成**文件→HTTP 的内容因果。
5. **可信缺失和采集范围，比 0 值更重要**：`capture_loss` 和每项 `present` 必须保留；缺少应用级 API / TLS 明文 / Windows 审计时直接 abstain。
6. **难负例，比纯正例扩量更重要**：正常构建目录清理、CI 发布、授权备份、开发者 Shell、经批准的账号变更、隐私脱敏回复应该与同种 syscall/HTTP 操作的越权案例进行成对复核。

## 下一阶段实施顺序

- P0：先收集**真实、授权测试环境**的脱敏 Hook+syscall/HTTP trace，保存 `source`、`evidence_level`、`tool_call_id`、权限/结果、`capture_loss`；旧数据不足时不编造 provenance。
- P1：对每个动作建立三独立目标：`intent_boundary`（超不超任务授权）、`effect_verified`（实际成功/失败/未知）、`harmful_outcome`（明确危害/明确正常/不可确定）。每一轴人审，不由既有 score / BLOCK 决策反推。
- P2：先做规则/透明特征 baseline + ablation，并对 **benign hard-negative** 单独报告 FPR、每个完整 run 的误报数和置信区间，再考虑微调。训练/选模/校准/盲测按来源、场景、版本分组；绝不将这 44 条语义参考案例混入目标模型测试。
- P3：所有危险候选默认 shadow-only。仅在独立人审及各来源误报上界与召回下界达标后，讨论是否进入**需要单独批准**的 enforcement 流程。

## 验证约束

当前 `kernel-http-behavior.v2` 的 `history.30s.prior_sensitive_reads` 只计算 batch 内**成功、读取字节数为正且路径能归类为敏感**的前序事件；它不是完整采集，也不是数据泄露的证据。未完成的族包括 mmap、splice、sendfile、io_uring、可信目录/inode 解析及真正的 HTTP↔文件内容匹配。不要据此修改生产阻断策略。

参考：[ATT&CK T1552.001](https://attack.mitre.org/techniques/T1552/001/) · [T1567](https://attack.mitre.org/techniques/T1567/) · [T1485](https://attack.mitre.org/techniques/T1485/) · [T1486](https://attack.mitre.org/techniques/T1486/) · [Falco 默认规则](https://falco.org/docs/reference/rules/default-rules/) · [AgentDojo](https://arxiv.org/abs/2406.13352)。
