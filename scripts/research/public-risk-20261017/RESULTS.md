# 公开数据深度训练结果（2026-10-17）

本轮已实际执行全参数 BERT 微调；结果与此前 4,800 条本地策略合成样本完全分开。训练目标是公开数据定义的分类标签，**不是已验证的生产环境 Agent 行为判断或内核阻断能力**。

## 结果判断

- 辅助隐私违规：加权训练 F1 从 0.674 到 0.702，Accuracy 从 97.12% 到 97.38%。这只是对应公开任务的改善。
- 辅助有害请求：加权训练 F1 从 0.737 到 0.800，Accuracy 从 73.21% 到 80.36%。这只是对应公开任务的改善。
- **漏洞代理任务未达到可用水平**：加权 BERT 的 balanced accuracy 50.40%、FPR 98.24%，F1 最优阈值近似全判正类。不能把约 0.63 的 F1 当成有效漏洞判断能力。词法与 MLP 对照也未解决这个问题。
- 直接工具动作隐私任务按验证集选择 `agent_privacy_transfer_balanced`：测试 balanced accuracy 49.68%、F1 0.749、AP 0.6736；常数对照的 F1 为 0.754。需要结合下表的误报率判断，不能只看 F1。

**直接动作隐私决策同样未通过可用性检查。** 融合模型排序 AP 为 0.7672，但 F1 运行点 FPR 为 98.68%；采用验证集低误报阈值后，测试 FPR 3.31%、Recall 仅 9.52%。这不是可部署的泄露阻断器。

## 数据划分与独立性

| 评测任务 | Train | Validation | Calibration | Test | 测试正例 | 测试 group |
|---|---:|---:|---:|---:|---:|---:|
| 隐私违规辅助 / BeaverTails | 13947 | 2554 | 2490 | 3019 | 155 | 2525 |
| 代码缺陷辅助 / Devign | 10163 | 1736 | 1676 | 2730 | 1254 | 2067 |
| 有害请求辅助 / AgentHarm | 248 | 48 | 0 | 56 | 28 | 7 |
| Agent 候选动作隐私 / PrivacyAlign | 1524 | 253 | 236 | 382 | 231 | 199 |

所有同一原始任务的变体按 group 隔离。BeaverTails 原始训练中删除 8151 条测试问题重叠样本。Devign 删除训练/验证/测试间相同 commit 或代码重叠，因此这不是其原始 leaderboard 划分。AgentHarm 在公开 test 部分做本地重划分，只测请求识别，不是官方 Agent 执行评测。PrivacyAlign 将两候选与同一上下文绑定分组，排除无严格多数的泄露标签。

## 直接任务：PrivacyAlign 工具动作泄露

| 模型 | Accuracy | Balanced acc. | Precision | Recall | F1(正类) | Macro-F1 | AP | FPR | F1 95% CI |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| agent_privacy_base | 60.99% | 50.66% | 60.79% | 100.00% | 75.61% | 39.11% | 0.6562 | 98.68% | [0.727, 0.784] |
| agent_privacy_transfer_balanced | 59.95% | 49.68% | 60.32% | 98.70% | 74.88% | 38.08% | 0.6736 | 99.34% | [0.718, 0.777] |
| agent_privacy_frozen_auxiliary | 60.47% | 50.00% | 60.47% | 100.00% | 75.37% | 37.68% | 0.6388 | 100.00% | [0.725, 0.782] |
| agent_privacy_tfidf | 60.73% | 50.45% | 60.69% | 99.57% | 75.41% | 39.00% | 0.6658 | 98.68% | [0.725, 0.782] |
| agent_privacy_candidate_only_tfidf | 60.73% | 50.33% | 60.63% | 100.00% | 75.49% | 38.40% | 0.7595 | 99.34% | [0.726, 0.783] |
| agent_privacy_validation_selected_blend | 60.73% | 50.45% | 60.69% | 99.57% | 75.41% | 39.00% | 0.7672 | 98.68% | [0.726, 0.784] |
| agent_privacy_constant_prevalence | 60.47% | 50.00% | 60.47% | 100.00% | 75.37% | 37.68% | 0.6047 | 100.00% | [0.725, 0.782] |

## 辅助任务：BeaverTails 隐私违规

| 模型 | Accuracy | Balanced acc. | Precision | Recall | F1(正类) | Macro-F1 | AP | FPR | F1 95% CI |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| bert_unweighted | 97.12% | 78.65% | 80.36% | 58.06% | 67.42% | 82.95% | 0.7249 | 0.77% | [0.610, 0.741] |
| bert_balanced | 97.38% | 79.70% | 84.55% | 60.00% | 70.19% | 84.41% | 0.8002 | 0.59% | [0.639, 0.766] |
| tfidf_logistic | 97.12% | 82.61% | 74.64% | 66.45% | 70.31% | 84.40% | 0.7874 | 1.22% | [0.642, 0.763] |
| validation_selected_blend | 97.38% | 79.70% | 84.55% | 60.00% | 70.19% | 84.41% | 0.8002 | 0.59% | [0.639, 0.766] |
| constant_prevalence | 5.13% | 50.00% | 5.13% | 100.00% | 9.77% | 4.88% | 0.0513 | 100.00% | [0.082, 0.114] |

## 辅助任务：Devign 代码缺陷

| 模型 | Accuracy | Balanced acc. | Precision | Recall | F1(正类) | Macro-F1 | AP | FPR | F1 95% CI |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| bert_unweighted | 46.45% | 50.36% | 46.12% | 98.48% | 62.82% | 33.57% | 0.4727 | 97.76% | [0.602, 0.660] |
| bert_balanced | 46.45% | 50.40% | 46.14% | 99.04% | 62.95% | 33.19% | 0.4781 | 98.24% | [0.602, 0.660] |
| tfidf_logistic | 47.14% | 51.03% | 46.46% | 98.80% | 63.20% | 34.72% | 0.4978 | 96.75% | [0.604, 0.664] |
| validation_selected_blend | 47.47% | 51.19% | 46.55% | 96.89% | 62.89% | 36.52% | 0.4951 | 94.51% | [0.602, 0.663] |
| constant_prevalence | 45.93% | 50.00% | 45.93% | 100.00% | 62.95% | 31.48% | 0.4593 | 100.00% | [0.603, 0.661] |
| code_SVD_MLP | 46.48% | 50.47% | 46.17% | 99.52% | 63.08% | 32.94% | 0.5033 | 98.58% | [0.604, 0.662] |

## 辅助任务：AgentHarm 有害请求

| 模型 | Accuracy | Balanced acc. | Precision | Recall | F1(正类) | Macro-F1 | AP | FPR | F1 95% CI |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---|
| bert_unweighted | 73.21% | 73.21% | 72.41% | 75.00% | 73.68% | 73.21% | 0.8720 | 28.57% | [0.485, 0.906] |
| bert_balanced | 80.36% | 80.36% | 81.48% | 78.57% | 80.00% | 80.35% | 0.9117 | 17.86% | [0.667, 0.929] |
| tfidf_logistic | 80.36% | 80.36% | 79.31% | 82.14% | 80.70% | 80.35% | 0.9195 | 21.43% | [0.667, 0.943] |
| validation_selected_blend | 73.21% | 73.21% | 72.41% | 75.00% | 73.68% | 73.21% | 0.8929 | 28.57% | [0.485, 0.906] |
| constant_prevalence | 50.00% | 50.00% | 50.00% | 100.00% | 66.67% | 33.33% | 0.5000 | 100.00% | [0.667, 0.667] |

## 低误报运行点（阈值仅由验证集选择）

| 任务 | 模型 | 测试 Recall | 测试 FPR |
|---|---|---:|---:|
| PrivacyAlign | agent_privacy_transfer_balanced | 6.93% | 2.65% |
| PrivacyAlign | agent_privacy_tfidf | 11.69% | 8.61% |
| PrivacyAlign | agent_privacy_validation_selected_blend | 9.52% | 3.31% |
| privacy | bert_balanced | 87.74% | 5.83% |
| privacy | tfidf_logistic | 87.10% | 5.97% |
| privacy | validation_selected_blend | 87.74% | 5.83% |
| vulnerability | bert_balanced | 5.98% | 6.57% |
| vulnerability | tfidf_logistic | 8.77% | 5.01% |
| vulnerability | validation_selected_blend | 7.34% | 4.95% |
| harmful_request | bert_balanced | 10.71% | 0.00% |
| harmful_request | tfidf_logistic | 28.57% | 0.00% |
| harmful_request | validation_selected_blend | 25.00% | 0.00% |
| vulnerability | code_SVD_MLP | 6.54% | 4.95% |

验证集 FPR≤5% 不保证独立测试集也≤5%，因此两项实测结果均保留。

## 训练与优化

- `bert_unweighted`：4,394,371 参数，完成 3 epochs，验证集选择 epoch 3，LR 0.0003，训练 534.6 秒。
- `bert_balanced`：4,394,371 参数，完成 5 epochs，验证集选择 epoch 3，LR 0.0001，训练 855.9 秒。
- `agent_privacy_base`：完成 5 epochs，验证集选择 epoch 5，训练 136.9 秒。
- `agent_privacy_transfer_balanced`：完成 5 epochs，验证集选择 epoch 5，训练 129.4 秒。
- `code_SVD_MLP`：41,217 参数，验证集选择 `code_mlp_0.001`，含训练集词法降维与两组学习率搜索共 9.3 秒。这是观察到代码验证表现偏弱后增加的架构试验。

共同优化包括 AdamW、weight decay、gradient clipping、验证集 checkpoint 选择、独立 sigmoid 校准与验证集 F1 阈值选择。TF-IDF 的 C 也只在验证集调优。没有根据测试集改参数或阈值。

按验证集选出的神经模型：辅助 `bert_balanced`，直接隐私 `agent_privacy_transfer_balanced`。这个选择不代表神经模型必然优于 TF-IDF，也不代表测试集最高分。

## 必须保留的局限

- 单 seed、CPU 小模型；没有声称完成 7B 模型、GPU 或 LoRA 训练。CI 为按问题/commit/原始任务/场景的 500 次 bootstrap，只描述测试采样不确定性。
- PrivacyAlign 是公开、人工标注的合成场景；它的标签存在标注分歧。BeaverTails 的隐私类别不等于实际外传成功。Devign 是代码缺陷代理标签，不等于漏洞由 Agent 引入。
- AgentHarm 只有 7 个独立测试任务、56 条改写请求，不能因改写行数看起来多而宣称稳健。包含 Cybercrime 等广义危害，尚不能替代破坏性动作结果标签。
- 使用 F1 最优阈值，FPR 可能很高，不适合直接封禁。误报、漏报与简单基线更好的结果均原样报告。
- 文本截断、英语偏重、预训练污染、跨不同 group 的近重复都仍是限制。PrivacyAlign context/candidate 截断统计：`{"train": {"context": 1524, "candidate": 1435}, "validation": {"context": 253, "candidate": 243}, "calibration": {"context": 236, "candidate": 226}, "test": {"context": 382, "candidate": 357}}`。
- 生产方向仍需带真实授权、时序 cutoff、采集覆盖率和独立标签的 Agent/eBPF 回放数据；本次没有改变旧 128 维向量、内核策略或默认采集隐私设置。

完整配置、分割散列、逐样本预测、训练曲线见同目录的 manifests / artifacts / logs。数据来源、固定 revision、许可和复现命令见 README.md。非商用与 safety-only 数据限制必须随模型一起保留。
