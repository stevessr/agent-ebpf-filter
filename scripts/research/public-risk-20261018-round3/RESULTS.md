# Round 3：前端/后端网络搜索与 OpenCode 白样本实验

完成 34 组预定架构/超参数试验：30 组冻结编码器+判别头，4 组 TinyBERT 全参数训练。另对胜出配置和 TinyBERT 对照各补 2 个随机种子，并训练 3 个加入白样本的对照模型。所有分数来自实际运行。

## 数据与评估边界

audit 为本轮训练前新划出的组隔离留出集，但来自此前使用过的公开训练语料；不是全新外部测试集。旧 confirmation/test 仅作为回归集。验证集选结构，独立 calibration 选阈值（经验 FPR≤5%），然后冻结再评估。所有模型从公开预训练底座或新判别头开始，没有继承之前微调的头。

| 任务 | train | validation | calibration | 本轮 audit | 旧 confirmation | 旧 test |
|---|---:|---:|---:|---:|---:|---:|
| privacy | 1003 | 253 | 236 | 218 | 303 | 382 |
| code | 3656 | 800 | 800 | 844 | 1955 | 2730 |

隐私为 PrivacyAlign 人工泄露标签；代码为 Devign 缺陷代理标签，不能称为真实 Agent 漏洞利用或破坏结果准确率。

## 每个前端的验证集最佳结构（单搜索种子）

| 任务 | 前端 | 后端 | Val BA | Val AP | Val FPR |
|---|---|---|---:|---:|---:|
| privacy | finetuned_tiny | cnn | 50.10% | 0.5869 | 4.63% |
| privacy | tiny | linear | 52.63% | 0.6092 | 3.70% |
| privacy | new | residual lr=0.001 | 53.32% | 0.6690 | 3.70% |
| privacy | guard | residual lr=0.0003 | 52.18% | 0.6593 | 1.85% |
| privacy | hybrid | linear | 51.82% | 0.6409 | 4.63% |
| code | tiny | linear | 53.42% | 0.5362 | 4.68% |
| code | new | gated lr=0.0003 | 56.65% | 0.6079 | 3.04% |
| code | finetuned_tiny | cnn | 51.28% | 0.5203 | 2.81% |

privacy new=all-MiniLM-L6-v2；code new=CodeT5-small；guard=AstrLink Guard INT8 token证据；hybrid=MiniLM+Guard；finetuned_tiny 为所有参数可训练。

## 本轮 audit（3 种子均值）

| 任务 / 模型 | Accuracy | BA | F1 | AP | Recall | FPR |
|---|---:|---:|---:|---:|---:|---:|
| privacy / control | 44.04% | 50.27% | 17.57% | 0.6363 | 10.32% | 9.78% |
| privacy / selected | 44.65% | 51.63% | 12.55% | 0.6296 | 6.88% | 3.62% |
| code / control | 48.70% | 51.51% | 15.92% | 0.5694 | 9.11% | 6.09% |
| code / selected | 53.32% | 55.91% | 27.72% | 0.6809 | 16.81% | 4.99% |

control 为本轮验证集选出的 TinyBERT 前端最佳后端；selected 为全部候选中的验证集胜出配置。表格使用同一数据划分、同一阈值规则。

| 任务 | 胜出配置 | 相对 control 的 audit BA 差值 | 配对组 bootstrap 95% CI |
|---|---|---:|---|
| privacy | {'front': 'new', 'back': 'residual', 'lr': 0.001} | +1.36 pp | [-2.57, +4.90] pp |
| code | {'front': 'new', 'back': 'gated', 'lr': 0.0003} | +4.40 pp | [+1.85, +7.10] pp |

CI 按场景/commit 组配对重采样 1000 次；反映这些已训练种子条件下的数据抽样不确定性。均值/标准差、混淆矩阵、阈值及各模型结果保存在 JSON。这里只做阈值选择，未做概率校准；sigmoid 分值不能解释为已校准风险概率。

## 旧 test 回归集（3 种子均值，已被前轮查看）

| 任务 / 模型 | BA | AP | Recall | FPR |
|---|---:|---:|---:|---:|
| privacy / control | 54.75% | 0.6991 | 10.82% | 1.32% |
| privacy / selected | 53.28% | 0.7146 | 10.53% | 3.97% |
| code / control | 50.03% | 0.4783 | 7.58% | 7.52% |
| code / selected | 53.81% | 0.5679 | 12.79% | 5.17% |

## OpenCode 白样本与加入训练的对照

OpenCode 1.19.3，GLM-5.3-Flash-Free 和 DeepSeek-V4.1-Flash-Free：24 次正式任务，22 条验收通过；API 汇报费用合计 0。另有 2 次早期权限探测失败，未混入训练。15 条来自 4 个训练任务族，7 条来自 2 个留出任务族。两条正式任务分别因读取证据不合格/答案不符剔除。

只读 fixture 任务，实际路径/工具/完成状态/文件不变性/答案均检查。标签仅覆盖这些观测行为，漏洞标签未知。未把安全任务描述直接当作安全行为标签。读权限限制在独立任务目录，外部目录和非 read 工具被拒绝；没有 OS 沙箱保证。

| 隐私配置 | 白样本留出 FP / 7（逐种子） | 公开 audit BA | 公开 audit Recall | 公开 audit FPR |
|---|---|---:|---:|---:|
| base | 0, 0, 0 / 7 | 51.63% | 6.88% | 3.62% |
| white | 0, 0, 0 / 7 | 52.09% | 6.35% | 2.17% |

white 仅添加 15 条经过验收的负样本，每条权重 1；保持所选结构、阈值规则和训练预算。7 条白样本只有 2 个任务族，不能支持一般安全准确率结论；这里仅报告误报数量，不能把三个种子当成 21 条独立样本。没有基于白样本留出结果挑选新结构。

## 参考、验证与限制

- 参考 AstrLink Guard v0.1.2 的多领域数据、难负样本、实体证据和误报统计。公开来源包括 Nemotron-PII、nym-pii-multilingual-data、CredData；其完整训练数据和配方未公开，本轮没有声称重训复现 AstrLink。详见 README 的固定 revision 与来源链接。
- AstrLink 是 token 实体识别，不能把公开的实体 F1 当作本实验 Agent 泄露决策准确率。我们的 token 聚合特征没有复刻其完整生产 BIO/URL decoder。
- Guard 最多采样三个窗口；原有语义缓存和端到端模型也有截断，覆盖率和规则见源码/manifest。中英文模型能力不等于本实验已完成中文独立验证。
- 5 项协议测试通过：组隔离/缓存标签一致、FPR 阈值并列值、试验矩阵数量、全参数编码器梯度、后端梯度。另执行冻结权重/结果混淆矩阵/数据哈希/白样本轨迹检查。
- 初始 ONNX Runtime 执行因遥测连接被自动审核拒绝；已按官方文档在初始化前设置 ORT_DISABLE_TELEMETRY=1 并重跑。独立 strace 检查因环境 ptrace 限制无法执行，不把空日志视为无外联证明。
- 部署门槛仍为 Recall≥70% 且 FPR≤5%；仅研究分支，无内核/运行时策略变更。真实破坏性行为尚缺匹配的带结果执行轨迹，本轮白样本不能填补正类证据。

胜出模型本轮 audit 部署门槛逐种子：{'privacy': [False, False, False], 'code': [False, False, False]}。
