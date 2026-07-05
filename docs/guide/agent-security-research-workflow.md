# Agent 安全研究工作流

本工作流把运行时事件、Research Session、ML 训练数据和可复现实验包串成一条闭环，适合安全审计、论文实验和 Agent 行为基准构建。

## 1. 采集与构建会话

1. 在运行时启用需要的观测能力：wrapper/native hooks、eBPF 事件、AgentSight 上传、可选 TLS 诊断。
2. 在 Research 页面创建 Session，配置 source/eventType/comm/query/time range。
3. 执行 `build_session`，后端会把实时事件归一化为 `ResearchEvent`，并生成 timeline、process tree、trace summary、risk alerts、loop findings 等聚合结果。

> Research 导出不会修改 wrapper/cgroup/LSM/kernel policy，只做观察、分析和导出。

## 2. 预览训练集

在 Research 的 **Training Dataset** 标签页选择 label policy：

- `decision`：只信任已有 ALLOW/BLOCK/ALERT/REWRITE 决策，适合导入监督训练库。
- `heuristic`：decision 缺失时用风险评分和行为分类补齐标签。
- `unlabeled`：只用于 JSONL/CSV 导出和离线标注，不直接导入监督训练库。

预览结果会展示：

- sample/labeled/importable/unlabeled 数量；
- label/category/source 分布；
- 128 维 feature vector 与 normalization report；
- class imbalance、重复命令、out-of-range feature 等质量提示。

## 3. 扩充训练数据

在 ML Training 页可混合导入以下数据：

- 内置合法 Agent 行为样本：常见 git/search/build/test/package-manager/read-only ops。
- 内置 SELinux policy 样本：`allow/neverallow/dontaudit/auditallow/permissive/type_transition`。
- 本地或远程数据集：JSON/JSONL/CSV/TSV/text/zip/tar/gz/bz2。
- SELinux `.te/.cil` 或 JSON `rules[].rule` / `rules[].selinuxRule`：自动转为 `selinux-rule ...` 样本。

导入响应会返回 `imported/skipped/skipReasons`，并提供 `byLabel/byCategory/bySource/quality/normalization` 供前端检查训练数据质量。


## 3.1 训练前 Readiness 门槛

`GET /config/ml/status` 会返回 `trainingReadiness`，后端统一计算：

- `ready/labeledCount/minSamples/classCount/minClasses`：是否满足最小监督训练门槛。
- `byLabel/byCategory`：快速发现单类别、类别不均衡或缺少高风险样本。
- `normalization/quality`：检查非有限值、超出 `[0,1]` 的特征、重复命令和未标注样本。
- `blockingReasons/warnings/suggestedActions`：前端训练页会据此提示或阻断手动训练，避免把低质量数据直接写入模型。

建议顺序：先导入 Agent legal / SELinux / Research Session 样本，再处理 `single_class_training_data`、`insufficient_labeled_samples`、`feature_values_out_of_range` 等阻断项，最后训练或调参。

## 4. 训练、调优与导出

1. 确认训练库中 ALLOW/BLOCK/ALERT 分布和 normalization 状态。
2. 运行 ML train 或 auto-tune，优先关注 `balancedAccuracy` 和 `allowRecall`，避免只看整体 accuracy。
3. 运行 Research security evaluation，对比预期标签与实际风险/决策；报告中的 `posture` 会汇总 `pass/needs_review/critical`、风险分、阻断项、warnings、suggested actions 和结构化 remediation plan。
4. 导出 bundle，产物包含：
   - `events.jsonl/csv`
   - `training.jsonl/csv`
   - `training-manifest.json`
   - `results.json`
   - `session.json`
   - 如已运行安全评测，还包含 `security-evaluation.*`

`training-manifest.json` 会记录 schema version、label policy、feature space/version、redaction level 分布、label/category/source 分布、normalization 与 quality，便于复现实验。`security-evaluation.json` 同时记录 posture、remediationPlan 与 top failing categories，方便把评测结果转成后续规则、阈值或训练集修复任务。

