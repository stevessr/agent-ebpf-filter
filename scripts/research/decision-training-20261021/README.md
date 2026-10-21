# Agent 决策模型训练 v2：困难负例与低误报评估（2026-10-21）

状态：**研究分支中的可复现实验代码；尚未重新完成公开数据训练，未产生本轮模型权重或新准确率。** 保持 PR #116 为 draft、shadow-only，不修改线上内核策略、模型权重及旧实验成绩。

本目录在 [2026-10-18 冻结实验](../deep-optimization-20261018/README.md) 的基础上新增一套**独立**训练/校准协议，以解决当前 PrivacyAlign AP 上升但低误报召回仍为 1.73%、PiSAs 迁移召回为 0 的问题。改善这些指标是**待检验假设**，不是已经取得的效果。HTTP 高误报留待独立的真实良性流量采集及专门实验；PrivacyAlign 隐私标签不能冒充 HTTP/syscall 攻击标签。

## 实验设计

- **同计算预算消融**：BERT-Mini 两组都从同一原始预训练权重、相同随机种子、数据和训练轮数开始。对照 `mini_pair_baseline` 使用 BCE + 同场景正负动作 pairwise loss；实验组 `mini_hard_negative` 在此基础上把训练 batch 中评分最高的前 25% 良性动作 BCE 权重设为 3 倍。这个训练批次的负例集合只读取原始人工标签，不用现有风险评分、策略输出或测试集。
- **场景完整 batch**：同一 group 的候选动作一起进入 batch，以保证 pair loss 只发生在同任务上下文内；超大 group 明确报错而不默默拆分。每个 epoch 使用固定派生种子打乱 group。
- **验证集选 epoch/模型**：按场景聚类重采样 400 次，优先最大化 recall 的 bootstrap 95% 下界，再以验证集经验 FPR≤1% 的召回率、AP 破平局。bootstrap 本身不是新的独立验证集；多模型搜索造成的选择偏差仍需独立数据检验。
- **校准与统计门槛**：独立 calibration 的负例按分数降序，严格使用 `score > threshold`，允许最多 `floor(0.01 × negative_count)` 个经验 FP。发布 FP、TP、FN、TN 以及单侧 95% Clopper–Pearson FPR 上界。现有 99 条隐私校准负例即使 0 FP，上界仍约 2.98%，不构成 1% 真实 FPR 资格。
- **封存与回归**：`freeze.json` 写入选中 epoch、参数、模型 SHA-256 和校准结果后才打开已消费的 PrivacyAlign test；它只能用于回归，不能声称独立 held-out。训练前先审计 train/validation/calibration 的 group/完整输入重复；冻结后再打开 test 完成跨 split 审计。输出同时保存数据来源 SHA-256。

## 运行

先按 [历史实验步骤](../deep-optimization-20261018/README.md) 准备 Python 3.12 虚拟环境、公开 `agent_privacy_data/{train,validation,calibration,test}.jsonl`、本地 BERT-Mini 和 tokenizer 资产。只读取 PrivacyAlign；不下载/处理危险 payload，不调用任何生产执行 API。

```bash
python3 -m unittest discover -s scripts/research/decision-training-20261021 -p 'test_*.py' -v
python3 -m py_compile scripts/research/decision-training-20261021/*.py

export OMP_NUM_THREADS=2 OPENBLAS_NUM_THREADS=2
python3 scripts/research/decision-training-20261021/train_privacy_v2.py \
  --assets /tmp/risk-assets \
  --mini /tmp/risk-round/base_mini \
  --out /tmp/risk-v2 \
  --epochs 6 --batch-size 16
```

将产生 `protocol.json`、两组 `*.safetensors`、`trials.json`、`freeze.json`、`results.json` 与脱敏预测标识。模型结构仍使用历史实验的 `PairBert` 与 `encode`，避免隐式改变 tokenization。输出应按 model SHA-256、数据 digest 和运行环境归档；**不得覆盖 2026-10-18 旧权重/冻结分数**。本目录提供程序与 11 项纯标准库协议测试，当前不包含实际 v2 全量训练产物。

## 下一道真实评测门槛

1. 完成全部预声明 epoch 并提交训练日志、硬负例/基线的同阈值比较及耗时。
2. 加入新的跨 session/harness 正常开发调用与独立人工确认的隐私外发正例；用同一冻结阈值计算 per-run FP、Recall、AP、置信区间。
3. 对 PiSAs/工具动作类型与特征缺失做跨来源分层诊断；不因单一 AP 进步就升级生产资格。
4. 真实 shadow 回放通过后另审推理部署与数据使用许可证。只有独立且足量样本通过 FPR 上界及召回门槛，才考虑审批阶段；此代码永不自动开启 BLOCK。

## AutoML 搜索与独立影子审核（增量）

保持默认两组消融方案不变；只有显式提供 JSON 搜索空间时，才进行更多 BERT-Mini 拟合。参数组合确定性排序、完整预算检查（最多 24 项），不会静默截断或读取 test/calibration 标签以选择训练超参。

示例文件 auto-grid.json：

```json
{
  "learning_rates": [0.00002, 0.00003],
  "pair_weights": [0.0, 0.2],
  "hard_negative_weights": [1, 3]
}
```

运行参数添加 `--search-space /tmp/auto-grid.json --max-trials 8`；将实际训练 **8 次 BERT-Mini 完整 epoch 配置**，资源开销约为默认方案的四倍，不会虚报执行结果。新增 `leaderboard.json` 按验证集组 Bootstrap 召回下界、召回率、AP 排序；独立 calibration 只设阈值，不参与模型选择。输出目录存在协议/冻结结果时拒绝覆盖。

`freeze.json` 添加 `shadow_review_gate`：独立人工标注影子评测缺失则自动标记不合格；现有 99 条负例的校准 FPR 上界也无法通过 1% 门槛。可选在**模型冻结后**通过离线 `automl_review.py` 读取新来源、人工确认的隐私动作评分 JSONL，计算行级及场景级 FPR 95% 上界和召回率。每条必须包含 `id/group/label/domain=privacy_action/split=independent_review/source/annotation_source/score/artifact_sha256`，摘要不能与冻结模型摘要不一致；不能将旧 regression/test/HTTP 标签充当新隐私正例。

```bash
python3 scripts/research/decision-training-20261021/automl_review.py   --freeze /tmp/risk-v2/freeze.json   --review-predictions /path/to/new_verified_shadow_actions.jsonl   --output /tmp/new-shadow-review.json
```

审查工具不接入运行时、不写权重、不自动加载新模型；即使统计门槛全部满足，也只会标记 `shadow_review_candidate`，并保留 `production_approved=false`。独立性/人工标签来源仍须实证核查。样例和协议测试均为合成数据，不代表真实性能提升。


## v3 低误报识别能力训练（代码可用，权重待实验）

使用 `--quality-search` 将原 BERT-Mini baseline 与困难负例候选保留，并增加两组**可检验的训练假设**：

- `mini_hardest_pair`：仅在同一场景内，对最容易被误报的良性动作施加更强的 pair margin，目标是在固定误报预算下提高两类动作的分离度。
- `mini_missed_positive`：同时增加漏检正例的 BCE 权重，测试是否改善高阈值召回；其副作用可能是误报上升，必须如实报告。

```bash
python3 scripts/research/decision-training-20261021/train_privacy_v2.py \
  --assets /tmp/risk-assets \
  --mini /tmp/risk-round/base_mini \
  --out /tmp/privacy-low-fpr-v3 \
  --epochs 6 --batch-size 16 --quality-search
```

旧 `--search-space` 的三个轴仍兼容；新网格还可选 `positive_weights: [1, 2]` 和 `pair_strategies: ["all", "hardest"]`；最多 24 组。质量预设与自定义网格互斥。

**模型选择偏差修复**：新增 `quality_protocol.py`。按完整场景分两个验证折，每折使用另一折负例确定阈值，再进行样本外评分；交换折后合并统计。模型排序优先满足经验 FPR≤1%，再比较场景 Bootstrap 召回下界、召回率、AP。训练依旧只使用 train；校准依旧只使用独立 calibration；此前已消费 test 仅作为回归而非新的泛化证明。

报告新增 Accuracy、Precision、Recall、Specificity、Balanced Accuracy、F1、FPR 与一侧 95% FPR 上界。Accuracy 易受 PrivacyAlign 标签比例影响；是否真正改善只能由新训练和独立影子数据支持。**无自动模型部署、无自动 BLOCK，PR 继续 Draft。**
