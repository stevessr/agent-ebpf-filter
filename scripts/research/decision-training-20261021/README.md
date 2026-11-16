# Agent 决策模型：低误报训练与 tag 归一化

最新 v6 已完成 **4 次真实拟合 / 24 个 epoch**，新证据编码和尾部排序均未胜出。验证集选中旧编码基线第 6 轮，旧 test 回归 TP=4/FN=227、FP=0/TN=151，Recall=1.73%、AP=0.787996；上一轮是 5 个命中。本轮未取得质量提升，PR #116 仍为 Draft / shadow-only。

[最新完整报告](RESULTS-V6-20261022.md) · [结构化结果](RESULTS-V6-20261022.json) · [训练与四组权重](https://github.com/stevessr/agent-ebpf-filter/actions/runs/39732615408) · [归档复核](https://github.com/stevessr/agent-ebpf-filter/actions/runs/39734823246) · [历史 v5 报告与勘误](RESULTS-20261022.md)

这里只研究公开 PrivacyAlign **隐私动作代理任务**。它的人工标签不能冒充 HTTP/syscall 攻击标签，已有 test 也不能再次成为独立验证集。tag 是诊断元数据，不是风险标签或模型输入。

## 当前训练协议

`privacy_low_fpr_ablation.v6` 是固定的 2×2 消融：legacy/evidence 编码交叉 baseline/source-local negative-tail 排序。四组均使用同一 BERT-Mini 结构、词表与预训练资产、初始状态摘要、seed 20261021、384-token 上限、batch 16、AdamW 3e-5、场景 pair 权重 0.2、6 epoch、2 CPU 线程。Tail 方案额外使用权重 0.2。

- 训练批次按 `(source, group)` 保持场景完整。场景内 pair 只比较同场景正负动作；tail surrogate 只比较同来源的 privacy-action 标签，使用 batch 内最高 10% 负例。小 batch 通常只有一个负例，这不是总体 1% 尾部分布估计。
- Epoch/候选按 validation 标准化部分 ROC AUC（max_fpr=0.01）排序，再按经验 Recall、AP、更低 FPR 破平局。并列分数组和 ROC 线性插值已与 sklearn 对照。该指标只用于选模。
- 独立 calibration 固定阈值，严格 `score > threshold`，最多允许 `floor(0.01 × negatives)` 个经验 FP。四组权重摘要、epoch 和阈值先一起写入 freeze，再打开旧 test。
- 交叉阈值验证仍按场景隔离，但仅作诊断。旧 v5 每折 FPR≤1% 硬条件在 51/57 负例下存在不可达问题，已在历史报告勘误，不能继续作为硬质量判据。
- Test 只做冻结后的回归；它不参与训练、选模或阈值更新。所有新项即使未胜出也完整归档，不按旧 test 重新挑选模型。

`evidence_pair.v1` 将目标压到至多 64 token（保留头尾），为至多两个原始记忆/工具轨迹片段保留剩余上下文预算。候选 token 重叠和行内文档频率选择窗口，保留片段主体锚点和原始顺序。长动作保留头 126、尾 63、相关中间窗口 64 token。只读取 context/candidate，不读取 label/tag/ID/split 或模型分数。上下文与动作取窗共同构成一个实验轴，不能声称已孤立某一种截断原因。当前实验不支持采用此新视图。

## 文件与运行入口

| 文件 | 职责 |
|---|---|
| `decision_protocol.py` / `quality_protocol.py` | 来源/场景隔离、近重复冲突审计、严格阈值、精确单侧区间、聚类 bootstrap、交叉阈值诊断 |
| `automl_protocol.py` / `low_fpr_ranking.py` | 有界候选计划、版本化排序、独立审核门槛、部分 ROC AUC |
| `training_objectives.py` | 真实 Torch BCE、场景 pair、训练负例尾部损失 |
| `evidence_encoding.py` / `v6_protocol.py` | 无标签有界 token 规划、固定四组预算及初始化/来源一致性检查 |
| `train_privacy_v6.py` | 独立 worker 拟合与 aggregate 全组冻结/回归 |
| `tag_protocol.py` / `error_analysis.py` | tag 归一化与覆盖审计，固定阈值的来源/tag/场景错误切片 |
| `freeze_contract.py` / `automl_review.py` | 旧/v6 冻结格式解析及独立人工审核入口 |
| `json_contract.py` / `fetch_privacy_assets.py` | 严格有界 JSON/JSONL、固定 revision 和 SHA-256 的公开资产下载 |
| `ablation_analysis.py` | 归档权重/选模/预测重算，成对错误与场景 bootstrap 差值，实际 tag 覆盖率 |

GitHub 工作流只在研究分支对应请求文件改动或手动触发时执行：

- `privacy-public-v6.yml` + `public-v6-request.json`：四组并行 worker，各 40 分钟 job / 35 分钟训练步骤上限，后接汇总。普通代码/报告提交不会重复训练。
- `privacy-v6-archive-audit.yml` + `public-v6-analysis-request.json`：下载指定已完成运行的产物，以标准库复核；不拟合模型、不执行产物内代码。四组旧 test 的分数和各自阈值保持冻结。
- `privacy-public-training.yml` + `public-training-request.json`：保留旧 v5 两候选入口与历史协议，不作为当前 v6 实验定义。

完整模型依赖固定在 `../deep-optimization-20261018/requirements.txt`，损失测试的轻量 Torch 依赖在 `requirements-objectives.txt`。资产下载重现历史固定 PrivacyAlign/BERT revision，逐文件核对既有 source_hashes，单文件限 96 MiB，临时下载后原子安装。语料原文不重新上传产物。

手动 worker 入口为 `train_privacy_v6.py worker --candidate <固定名称> --assets <资产目录> --out <空输出目录>`，aggregate 为同脚本的 `aggregate --assets <资产目录> --workers <四组产物根目录> --out <空输出目录>`。要求不可变源码 `GITHUB_SHA` 与固定请求相符，四组训练/验证/校准数据、初始状态和代码摘要必须一致。候选名见 `public-v6-request.json`；已有 checkpoint 或试验记录不覆写。

## tag 归一化与覆盖率

`behavior_tags.v1` 合并 tags/tag/category，执行 NFKC、camel/acronym/snake/kebab/空格与大小写归一化，使用明确语义别名，排序去重；存在具体标签时抑制 unknown。最多合并 32 个原始标签，原始串最多 128 字符，规范 ID 最多 64 字符。非法类型/语法、零宽控制字符、不支持的版本与超限输入直接拒绝。合法自定义 ASCII ID 保留，不凭空增加语义标签。

| 输入示例 | 规范结果 |
|---|---|
| `FileRead`、`read-file`、`文件读取`、全角 `ＦＩＬＥ＿ＲＥＡＤ` | `file_read` |
| `HTTPRequest`、`https_request` | `http_request` |
| `NetworkRequest`、`网络请求` | `network` |
| `Privacy Leakage`、`隐私泄露` | `privacy_leak` |
| 缺失、空白、连续下划线/连字符、`未知` | `unknown` |
| `customAction` | `custom_action`，自定义分类元数据 |

`normalize_with_audit` 同时返回复制后的行及聚合计数：覆盖率、隐式/显式 unknown、规范化数量、重复值、被抑制的 unknown、已知分类/自定义标签行数。已知与自定义覆盖可重叠。`error_slices` 整体及每个来源/tag 都公开这些数据，不输出原始上下文、动作或身份。当前公开回归 **382 行全为 unknown，覆盖率 0%**，没有按模型分数推造 tag。

归一化不改写 label、context、candidate、domain、source/group/id。来源和场景名称保持原始身份；近重复审计只计算保守摘要，不改写真正模型输入。

## 冻结审核与产物复核

```bash
python scripts/research/decision-training-20261021/automl_review.py \
  --freeze freeze.json --review-predictions independent_predictions.jsonl \
  --output new_review.json
```

审核输入须包含独立来源、场景/动作身份、人工标注来源、privacy_action domain、independent_review 分区、人工二元 label、冻结分数与 artifact_sha256。V6 额外要求精确 freeze 文件的 `freeze_sha256` 和所选 `encoding_view` 声明。模型摘要、冻结选择与阈值均不得按审核结果修改；声明的执行真实性仍需人审。

资格同时检查动作和完整正例场景的单侧 95% Recall 下界≥20%，动作/场景 FPR 上界≤1%，并对各来源检查。任一缺失类别、重复身份、来源完全漏检、未知冻结格式或无独立人工来源都会失败。现在 calibration 只有 99 条负例，零误报仍对应动作级名义 FPR 上界约 2.98%，且缺少独立人审；任何统计通过也不自动授权安装模型或开启 BLOCK。

`ablation_analysis.py` 从归档重建 validation 选模、校验四组权重和 freeze 字节摘要、对齐 source/ID/group/label，重算回归 confusion/AP，并按场景做 2,000 次成对 bootstrap。区间只描述已消费语料，不能充当独立泛化证据；零观测误报的差值区间 [0,0] 也不是总体 FPR 保证。

初始 v6 导出误将缺省 source 写成空串。后续导出已省略缺省字段；旧归档仅按精确 writer commit 与已知 test 摘要恢复缺省，并记录修复次数。其他空来源仍拒绝，原始产物、分数/标签/组/阈值不改。工作流显式使用带 pipefail 的 Bash，避免 tee 掩盖失败；实际复核必须产出预期结果记录。

## 验证与历史

```bash
python3 -m py_compile scripts/research/decision-training-20261021/*.py
python3 -m unittest discover -s scripts/research/decision-training-20261021 -p 'test_*.py' -v
# 安装 requirements-objectives.txt 后，强制真实梯度测试：
REQUIRE_TORCH_TESTS=1 python3 -m unittest discover \
  -s scripts/research/decision-training-20261021 -p 'test_training_objectives_torch.py' -v
```

[最新专项 CI](https://github.com/stevessr/agent-ebpf-filter/actions/runs/39734831954)实际通过 **88 项标准库测试 + 10 项 Torch 测试，共 98 项不同测试**。缺 Torch 时的 10 项跳过不计作通过，独立 CPU job 将缺依赖视为失败。测试覆盖数据/来源隔离、严格 JSON、标签、置信门槛、选模、梯度/实际 SGD、token 预算与无标签不变性、完整产物复核及篡改拒绝。

历史 [2026-10-18 冻结研究](../deep-optimization-20261018/README.md) 与 [v5 实测报告](RESULTS-20261022.md) 的旧权重/指标继续保留。下一道质量证据需要新的独立人工审核，不能继续用这份旧 test 调阈值；真实 HTTP、破坏性 syscall 和内核拦截需要各自的数据与评测。
