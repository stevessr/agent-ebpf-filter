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
