# 开题报告：本地 ML 命令安全模型扩展与稳定性评估

## 一、研究背景

`http://localhost:5173/config/ml` 对应的是本地命令安全模型的训练、评估与调参页面。当前系统不仅要判断“是否有错误”，更要尽量保证：

1. 正确命令尽量放行，降低误拦截；
2. 高风险命令能够稳定拦截；
3. 模型在不同数据构造、不同参数设置、不同运行后端下都保持可复现；
4. 训练、推理和数据集维护流程可持续迭代。

因此，本课题的核心不是只找一个“单次最高分模型”，而是在真实数据集、构造数据集、稳定性复核和推理性能之间找到可部署的最优解。

## 二、研究目标

- 扩展并整理训练数据集，提升覆盖面和标签质量；
- 在多模型、多参数空间内进行系统评估；
- 同时考察验证准确率、ALLOW 放行率、训练耗时和推理吞吐；
- 选择当前最优且可部署的稳定模型；
- 输出开题报告、阶段进度与后续展望，便于后续提交和复盘。

## 三、数据集构造方案

当前数据集来源主要包括：

- **本地持久化训练集**：来自运行中的样本库；
- **合成扩增样本**：用于补足罕见高危行为与边界样本；
- **互联网下载数据集**：用于横向扩展命令模式覆盖，当前已补充 `HttpParamsDataset`、`PowerShell MPSD`、`Malicious PowerShell Dataset` 等更经典的恶意/善意混合语料；
- **人工标注 / 回填样本**：用于修正误标和提升类别稳定性。

数据集维护能力已经具备：

- 添加样本
- 重标注样本
- 修改 anomaly score
- 删除样本
- 导入 / 导出数据集
- 导入时清洗敏感字段，自动屏蔽密码、token、Authorization/Bearer、邮箱、IP 与 home 路径等片段

其中，经典互联网数据集导入路径已经补齐了：

- **命令 / HTTP 参数混合攻击样本**：`HttpParamsDataset`，包含 benign(norm) 与 SQLi/XSS/Command Injection/Path Traversal(anom)；
- **PowerShell 混合语料**：`PowerShell MPSD`，包含 `malicious_pure`、`powershell_benign_dataset` 和 `mixed_malicious`，可按来源路径推断 benign / malicious；
- **恶意 PowerShell 集合**：`Malicious PowerShell Dataset`，适合与 benign 语料联动训练和清洗测试。

目前可用的标注训练样本数为 **949**，类别分布偏向 `BLOCK`，其次为 `ALLOW` 和 `ALERT`。这意味着模型评估不能只看整体准确率，还要重点关注正确命令是否被误拦。

## 四、模型测试与评估方法

本课题采用“宽扫 + 稳定复核”的方式：

1. **模型横向扩展**
   - Random Forest / Extra Trees
   - Logistic / SVM / Ridge / Perceptron / Passive-Aggressive
   - KNN / Naive Bayes / AdaBoost / Ensemble / Nearest Centroid 等

2. **参数空间构造**
   - 树模型：树数、深度、叶子最小样本数
   - 线性模型：学习率、迭代次数、正则项
   - 邻近模型：k 值、距离度量、权重方式
   - `comprehensive` 评测会把每个数值型可调参数拆成独立 axis profile；每个数值参数默认取 **1000 个离散点**，分类/固定参数则枚举全部有意义取值。
   - 综合评测默认不只跑单一持久化训练集，还会加入 `label-balanced` 等派生测试集；也可通过 `--datasets all,label-balanced,allow-block` 显式选择测试集。

3. **评估指标**
   - 验证准确率
   - ALLOW 放行率
   - 训练耗时
   - 推理吞吐 / 延迟
   - 稳定性均值与方差

4. **补充运行状态**
   - Native C runtime 推理耗时
   - CUDA / Intel iGPU 能力检测

## 五、阶段性结论

最新稳定性复核结果位于：

- `reports/ml-sweep-20260506-160249/`

当前稳定版最优模型为：

- **模型类型**：`svm_long`
- **参数**：`lr=0.100 iter=8000`
- **稳定均值**：`100.00% ± 0.00%`
- **平均放行率**：`100.00% ± 0.00%`
- **平均吞吐**：`813.8k/s ± 311.2k/s`

说明：

- 这个结果是当前最适合作为部署基线的候选；
- 早期探索中出现的 `random_forest_deep` 仍然是很强的单次探索结果；
- 更早的随机森林稳定复核说明“树模型也很强”，但最新 full sweep 已经把 `svm_long` 推到了当前第一名。

## 六、进度展望

### 阶段 1：继续扩充数据集

- 增加高危命令、系统管理命令、下载/上传命令、网络命令样本；
- 增加“看起来正常但实际危险”的灰区样本；
- 继续去重与重标注，降低标签噪声。
- 持续导入更多经典恶意/善意混合数据集，并观察清洗后标签分布是否更稳定。

### 阶段 2：稳定性复测

- 对当前最优模型做更高重复次数复核；
- 对次优模型做对照实验；
- 记录均值、标准差、最坏情况和抖动。
- 若需要满足“不同测试集 × 不同模型 × 每个参数 1000 离散点”的完整矩阵，执行：
  `./scripts/ml-sweep.sh --mode comprehensive --datasets all,label-balanced --points-per-param 1000 --workers 8 --repeats 1 --stability-top 1`；
  结果目录中的 `coverage.json` 会逐项记录每个测试集、模型、参数的覆盖情况。
- 长时间运行可追加 `--resume --outdir reports/ml-sweep-comprehensive-1000` 断点续跑；脚本会复用已经完成的每个 `*-grid.csv` profile，`--workers` 会并行执行同一 profile 内相互独立的参数点。

### 阶段 3：部署与加速

- 继续完善 C runtime / CUDA / Intel iGPU 状态展示；
- 逐步把可加速路径扩展到更多模型；
- 让前端看到的能力与后端真实运行能力一致。

### 阶段 4：形成最终提交材料

- 完成最终版开题报告；
- 完成进度总结与阶段性结论；
- 保留图表、PPTX 风格 HTML 和原始结果文件，便于复核。

## 七、当前建议

如果现在就要选部署候选，建议使用：

> `svm_long` — `lr=0.100 iter=8000`

如果要继续做研究拓展，则建议同时保留：

- `random_forest` 作为树模型对照组；
- `logistic` 作为轻量线性对照组；
- `extra_trees` 作为树模型对照组；
- `ensemble` 作为高准确率但较重的上限对照组。

## 八、参考产物

- `docs/ml-benchmark-report.md`
- `docs/ml-benchmark-presentation.html`
- `reports/ml-sweep-20260506-160249/`
- `reports/ml-sweep-20260506-150507/`
