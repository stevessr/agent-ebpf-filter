# Agent 决策模型训练：困难负例、低误报与 tag 归一化（2026-10-22）

状态：**已完成 v5 两候选各 6 epoch 的真实公开数据重训。** 选中 baseline 第 5 轮，旧测试集回归 Recall 2.16%、FPR 0/151、AP 0.7913；困难负例方案未胜出，仍未达到生产门槛。见[完整结果与权重证据](RESULTS-20261022.md)。PR #116 保持 draft、shadow-only，不修改线上内核策略、模型权重及旧实验成绩。

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

将产生 `protocol.json`、两组 `*.safetensors`、`trials.json`、`freeze.json`、`results.json` 与脱敏预测标识。模型结构仍使用历史实验的 `PairBert` 与 `encode`，避免隐式改变 tokenization。输出应按 model SHA-256、数据 digest 和运行环境归档；**不得覆盖 2026-10-18 旧权重/冻结分数**。本目录提供程序与协议测试；v5 全量运行产物已单独归档并在结果报告中链接。

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


## v3 低误报识别能力训练（历史代码阶段）

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

## v4：tag 归一化、来源隔离与错误分层（2026-10-22）

这次修复数据与选模链路中的可验证缺陷；尚无新训练权重或模型准确率提升结果。训练协议升级为 `privacy_low_fpr_automl.v4`，审核报告升级为 `privacy_automl_review.v2`，不能与旧协议结果混为同一实验。

`tag_protocol.py` 定义 `behavior_tags.v1`：合并 `tags`、`tag`、`category`；NFKC、大小写、camelCase、连字符/空白/下划线归一化；排序去重。`FILE_READ`、`file-read`、`FileRead`、`ReadFile`、`文件读取` 都得到 `file_read`。`privacy_leakage`/`隐私泄露` 得到 `privacy_leak`；一般网络请求仍归入 `network`，不冒充完整 HTTP 请求。缺失为 `unknown`；合法自定义英文标识保留，已知中文别名单独映射；其他非法格式、非字符串、超限或未知版本明确拒绝。最多 32 个输入 tag，每个原值最多 128 字符、规范值最多 64 字符。

**tag 仅用于误差诊断，不产生二分类标签，也不进入模型输入。** `label`、上下文、候选动作、source/group/id/domain 均不做 tag 规范化；带 `privacy_leak` 或 `BLOCK` tag 的人工负例仍为负例。旧数据没有 tag 时只能报告 `unknown`，不能从分数或策略结论推造语义标签。

训练批次、pair loss、验证折、bootstrap 和影子评估统一按 `(source, group)` 隔离；同名 group 和 action ID 可以属于不同来源，同一来源内重用 ID 仍受审计。source 缺失保留缺失，不把身份字段改成小写。泄漏检查额外拒绝跨分区的 NFKC/空白规范化近重复，以及同一输入或 ID 的标签冲突；原始模型输入保持原样。

模型选择要求**总体和每个验证折**均满足经验误报预算；全部候选失败时，优先减小最差折 FPR，再比较召回下界/召回/AP。修复之前会在全部不合格候选中仍优先选高召回的问题。独立影子审核同时检查每个来源的行级/场景级 FPR 上界，避免大来源稀释小来源的误报；样本不足也会保留不合格。

冻结后的 `error_slices.json` 及审核报告 `metrics.slices` 提供按 source/tag 的 FP/FN/TP/TN、Accuracy/Precision/Recall/F1/FPR 和 FPR 上界，缺少正例或负例时相应指标为 null。多 tag 样本参与多个切片，**不可把切片计数相加当总体**。这是固定阈值诊断，不回调阈值，不使用 regression 标签选模；报告只包含元数据聚合，不包含上下文/候选动作。协议增加训练、归一化、评估及模型定义源码 SHA-256。

验证：本目录共发现 **49 项测试，46 项通过、3 项 PyTorch 梯度测试因依赖缺失跳过**；`py_compile` 通过。新增 16 项标准库回归覆盖 tag 等价/幂等/边界、标签不变、跨来源身份与场景隔离、近重复和冲突标签、严格阈值、NaN 拒绝、分折/分来源误报及不合格候选排序。CLI 审核集成测试同时核对归一化切片。合成测试验证软件行为，不能解释为真实模型拦截率。完整 BERT 重训和独立人工标注评估仍待具备训练依赖与资产的环境执行。

## v5：召回证据、严格输入与真实梯度 CI（2026-10-22）

审核不能仅凭召回率点估计通过。例如 400 个独立正常场景零误报、仅 1 个泄露正例命中时，点召回为 100%，但单侧 95% 精确召回下界仅 **5%**；重复同一正例场景的 100 个候选，也不能形成 100 个独立场景证据。

- 统一指标增加 `recall_lower95`（精确 Clopper–Pearson 单侧下界）；总体、每个来源和每个归一化 tag 切片增加正例场景数、全部正例动作命中的场景数、场景召回率及 `scenario_recall_lower95`。一个场景只要漏掉任一人工正例，就不计为完整检出；严格 `score > threshold` 保持一致。
- `research_gate` 要求总体及每个来源的**动作/场景召回下界**都达到既有 20% 门槛，同时保留 FPR 上界检查。来源缺正例、正例场景不足、仅提供旧版点估计或缺少分来源证据，均不能通过审核候选门槛。tag 是诊断切片，不用任意事后筛选的 tag 声称统计资格。各置信区间是边际区间，未声称跨所有切片的同时 95% 覆盖。
- 审核报告版本变为 `privacy_automl_review.v3`；训练协议变为 `privacy_low_fpr_automl.v5`。旧报告需从原始冻结预测重新计算，不能直接补造置信界。训练前数据/阈值分工保持原设计；v5 实际重训与冻结回归已完成，详见结果报告。
- `json_contract.py` 在训练数据、搜索网格、freeze 和审核预测入口拒绝重复 JSON 键、NaN/Infinity/指数溢出、非对象及无效 UTF-8。训练 JSONL 限 64 MiB / 单行 1 MiB / 100,000 行；审核预测限 16 MiB / 单行 64 KiB / 100,000 行；freeze 和搜索网格限 1 MiB。冲突 `label`/tag 不再由 JSON 的最后一个同名键悄悄覆盖；统计预算按实际 UTF-8 字节计算。
- 损失函数移至 `training_objectives.py`，实现保持原公式，仅依赖 PyTorch，不再要求 Transformers、完整模型或语料才能运行梯度测试。新增 CPU CI，使用与既有训练相同的 `torch==2.14.1+cpu`。`REQUIRE_TORCH_TESTS=1` 下缺 Torch 直接失败；Torch 已存在时的代码导入错误也不再被吞成 skip。六项实际测试覆盖排序损失、有限梯度、跨来源不配对、梯度方向、真实 SGD 单步下降和极端 logits 数值稳定性。这些小张量测试不代表真实数据训练或模型拦截率。

运行轻量梯度验证：

```bash
python -m pip install -r scripts/research/decision-training-20261021/requirements-objectives.txt
REQUIRE_TORCH_TESTS=1 python -m unittest discover \
  -s scripts/research/decision-training-20261021 \
  -p 'test_training_objectives_torch.py' -v
```

本地共发现 **69 项测试，63 项标准库测试通过、6 项 Torch 测试因当前环境缺依赖跳过**；Python 编译检查通过。[远端 CPU CI](https://github.com/stevessr/agent-ebpf-filter/actions/runs/39723968550) 已实际执行全部 6 项 Torch 测试、零跳过；结合标准库 job，69 项测试均得到实际通过证据。PR 保持 Draft / shadow-only。

### 有限预算公开数据运行

`privacy-public-training.yml` 仅在研究分支的 `public-training-request.json` 改动或手动触发时运行；其他代码提交不会自动重训。本次请求固定默认两候选、每组 6 epoch、batch 16、seed 20261021、2 CPU 线程和 40 分钟总超时，不扩大 AutoML 网格。`fetch_privacy_assets.py` 仅下载原实验固定 revision 的 PrivacyAlign、Mini 配置/权重和词表，逐个检查既有 `source_hashes.json`；单文件限 96 MiB，损坏/超限下载不安装，已存在文件也重新校验。HTTP 数据集、PiSAs、在线 Agent/API 均不在本次下载或运行中。

运行产物包含两组权重、每 epoch 日志、参数/输入/源码摘要、选模榜、独立 calibration 冻结阈值、已消费 test 回归及 tag/source 错误切片；本轮 Actions artifact 已上传成功，保留至 2026-11-21。语料原文不上传 artifact。全量训练、失败门槛及具体指标见 [RESULTS-20261022.md](RESULTS-20261022.md)。统计审核继续要求新的独立人工标注数据，不自动安装模型或开启 BLOCK。训练步骤另设 35 分钟超时，在 40 分钟 job 预算内为失败日志和部分产物归档预留时间。


## v6: finite-sample selection correction and controlled evidence ablation

The v5 per-fold empirical FPR feasibility test is unsuitable for the current
validation sample size: folds have 51/57 negatives. A single error is 1.96% or
1.75%. With strict `>` and no tied negative maxima, calibration against the
opposite fold's maximum necessarily produces at least one fold error, even if
positives rank perfectly above negatives. The v5 fits/results remain historical
records; their per-fold failure is not evidence that the classifier cannot
separate positives from negatives. Independent calibration/review gates still
fail because negative support and recall are insufficient.

`privacy_low_fpr_ablation.v6` selects epochs/candidates by validation standardized
partial ROC AUC (`max_fpr=0.01`, tied-score groups, linear ROC interpolation), then
empirical validation recall, AP and lower empirical FPR. Crossfit results remain
diagnostics. This interpolated ranking is **not** a population FPR certificate.
Separate calibration, strict thresholds and independent human-review gates stay
unchanged. A full-environment preflight checks partial AUC against sklearn.

The fixed request `public-v6-request.json` specifies four equal-budget 2x2 cells:
legacy/evidence token selection crossed with baseline/source-local negative-tail
ranking. Each uses the same BERT-Mini architecture, pretrained initialization,
seed, 384-token budget, batch size 16, AdamW learning rate 3e-5, within-scenario
pair weight 0.2 and six epochs. Tail ranking adds weight 0.2 against the highest
10% of benign logits in each training batch/source; at small batches this is one
negative, not an estimator of the population 1% tail. Only public privacy-action
labels are supported.

`evidence_pair.v1` reserves at most 64 tokens for the goal (head/tail) and the
remaining context budget for at most two original memory/trajectory chunks.
Deterministic token overlap and within-row document frequencies choose windows;
subject anchors and source order are preserved. A long action keeps first 126,
last 63 and an overlap-selected middle 64 tokens. No labels, tags, identities,
model scores or corpus-fitted vocabulary enter token selection. This joint
context/action view is one feature axis; it does not isolate just goal truncation.

The new workflow runs four independent CPU workers, verifies identical initial
state/source/assets/budgets, and selects using validation only. All four weight
digests and calibration thresholds are written to `freeze.json` **before** any
consumed-test evaluation. All four then receive the same regression evaluation;
test performance never changes selection or thresholds. Worker/final artifacts
include checkpoints, provenance, encoding aggregates, prediction/error slices and
logs. Only the v6 request file change (or manual dispatch) triggers this training;
the old v5 request is unchanged. Mode remains Draft/shadow-only.

### Tag coverage and v6 independent-review compatibility

`normalize_with_audit` retains the same `behavior_tags.v1` canonical IDs and
returns aggregate accounting: annotation coverage, missing/explicit unknowns,
known-taxonomy/custom-tag rows, canonicalized spellings, duplicate values and
unknowns suppressed by a concrete tag. Custom IDs remain valid metadata;
known/custom row counts can overlap. Repeated delimiter-only tags now consistently
normalize to `unknown`. Zero-width controls, unsupported Unicode IDs, overlong
canonical expansions and merged-input budgets still fail closed.

`error_slices` includes this audit overall and per source/tag; raw contexts,
actions and identities are absent from coverage. Public PrivacyAlign remains 0%
annotated / all unknown, rather than silently appearing to have runtime tag
coverage. Tags do not enter fitting or alter reviewed labels.

`automl_review.py` accepts both legacy single-checkpoint freezes and the v6
multi-candidate envelope. It resolves only the validation-selected checkpoint and
its already frozen threshold/calibration. Unknown envelope versions, ambiguous
candidate sets and invalid thresholds fail. Model digest checks, independent
provenance requirements and production denial remain unchanged.
