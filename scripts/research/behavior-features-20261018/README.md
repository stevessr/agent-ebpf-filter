# 行为清洗与自动特征工程实验（2026-10-18）

对应 issue #112。本轮交付离线 Go 清洗器、现有 archive/TLS 适配器和真实公开数据实验。
**自动筛选候选未通过误报率门槛，不能替换或进入自动阻断。**

## 实现

独立 `kernel-http-behavior.v1` 共 137 个带 present mask 的特征。覆盖 syscall 读写执行/路径/结果、HTTP 请求目标/参数/头/请求体，以及同一进程生命周期下 1/30/300 秒历史统计。旧 128 维契约不变。

详细语义、输入示例、字段缺口见 [架构说明](../../../docs/architecture/kernel-http-behavior-features.md)。
清洗后的输出不含原始路径、主机名、参数、请求体、凭据或 scope ID。

## 数据与隔离

原作者 [HttpParamsDataset](https://github.com/Morzeux/HttpParamsDataset)，revision `926670a710283f87c05b554680facf3f9530548c`。
`payload_full.csv` SHA256 `a4e62ba13435ad3dd583c5790db2175629020fc41c56e1b26baa02a9ae45f03d`；仓库 MIT，源数据包括 CSIC2010/sqlmap/xssya/Vega/FuzzDB，详见原 README 与本目录 `DATASET_LICENSE.txt`。

原始 31,067 条：19,304 正常、11,763 异常；其中 SQLi 10,852、XSS 532、命令注入 89、路径穿越 290。
它们只有 **HTTP 参数值**，不是完整网站请求、syscall 轨迹或成功攻击记录；只能评估参数异常代理任务。
两层解码规范化后检查重复/标签冲突，此版本均为 0；进一步词法模板形成 10,982 组，组间不重叠。
数字/标识符/大小写变体尽可能同组，但不宣称消除了全部语义近重复。分组导致比例偏移，禁止用随机行切分掩盖。

| 分区 | 样本 | 模板组 | 异常 | 正常 |
|---|---:|---:|---:|---:|
| train | 18143 | 6589 | 7077 | 11066 |
| validation | 2845 | 1647 | 1743 | 1102 |
| calibration | 1775 | 1098 | 1196 | 579 |
| test | 8304 | 1648 | 1747 | 6557 |

## 自动特征工程与模型

- Go 提取器生成 23 个参数结构特征；训练集内扩展到 48 个候选（log1p、每字节计数、hint 交互）。
- 仅训练集拟合方差过滤、互信息及相关系数去冗余，保留 24 个。标签、攻击类型、ID、来源不进入特征。
- 每个特征视图比较 Logistic Regression（C=0.1/1）与 HistGradientBoosting（7/15 叶），共 8 次搜索。
- 验证集 AP 选择配置；两个视图均选择 HGB 15 叶、180 轮。每个胜出配置跑 3 seeds，共 14 fits/次实验。
- 首轮 14 fits，修正互信息离散掩码（低基数连续变量不能误当离散类别）并补固定阈值诊断后再跑 14 fits，总计 28 fits。保留 `initial_results.json`，不覆盖首轮失败结论。
- 三个 seeds 的 HGB 结果一致，不当作三个独立数据集。JSON 模型导出后全部测试集预测与 sklearn 最大绝对误差 0。

## 真实结果

以下为 8,304 条分组测试样本的结果。测试标签未进入拟合/特征筛选，但首轮已经查看；修正与补充诊断后的表是**复用测试集确认**，不是新盲测。

| 特征 | 阈值规则 | Accuracy | Balanced Accuracy | F1 | Recall | FPR | AP |
|---|---|---:|---:|---:|---:|---:|---:|
| structural | 独立校准集负例 95% 分位阈值 | 28.52% | 54.74% | 37.05% | 100.00% | 90.53% | 0.9978 |
| structural | 固定 0.5（后补诊断） | 89.01% | 92.95% | 79.25% | 99.77% | 13.86% | 0.9978 |
| auto_selected | 独立校准集负例 95% 分位阈值 | 28.77% | 54.90% | 37.13% | 100.00% | 90.21% | 0.9976 |
| auto_selected | 固定 0.5（后补诊断） | 85.21% | 90.55% | 73.95% | 99.77% | 18.67% | 0.9976 |

校准集负例阈值分别为 0.00366605 / 0.00430335，校准 FPR 均为 4.84%，测试 FPR 却超过 90%。
验证集固定 0.5 时两者 FPR 都为 0，但测试集 13.86% / 18.67%。这是明显的分组分布偏移，排序 AP 接近 1 不能替代阈值质量。
自动筛选版在固定阈值下比基础结构版 FPR 高 4.80 个百分点，故保留基础结构版作对照，拒绝提升候选。
按 test FPR ≤5% 且 Recall ≥70% 的离线门槛，两种视图、两种阈值均失败。
不再根据这个测试集反复改阈值；下一轮需要新来源、独立保留的完整请求/真实行为数据。

## 验证与证据缺口

- Go race 测试：27 个命名测试/子测试通过；两个包 `go vet` 通过；Python 6 个协议测试通过。
- 覆盖 fd 路径缺失、返回值存在性、mode/flags、O_PATH、openat/renameat、截断/NUL、编码边界、重复头、PID 复用、跨 run/clock 隔离、同时间戳、重复 ID 冲突、原文不输出、JSON 重复键/标签拒绝。
- 合成 HTTP fixture 的离线 microbenchmark：40.45–43.80 µs/op，约 30 KB/op；不是实时内核端到端延迟。
- 之前真实 OpenCode 的 24 个运行日志共 146 条工具事件全部判为“不具备 syscall/HTTP 请求证据”。不会将这些白数据伪装为内核记录。
- 当前环境没有本轮真实 eBPF syscall 采集；因此没有内核隐私泄露或破坏行为准确率结果。现有 archive 缺少稳定进程身份/路径截断与 FD 解析依据时，相关特征保留未知。
- TLS/内核采集开关、policy gate、运行时模型均未接入本轮候选。

## 重现

```sh
# Go 1.27.1; 在仓库根目录
(cd backend && go build -o /tmp/behavior-features ./cmd/behavior-features)
python3 -m pip install -r scripts/research/behavior-features-20261018/requirements.txt
git clone https://github.com/Morzeux/HttpParamsDataset /tmp/http-params-data
(cd /tmp/http-params-data && git checkout 926670a710283f87c05b554680facf3f9530548c)
OPENBLAS_NUM_THREADS=1 OMP_NUM_THREADS=2 python3 scripts/research/behavior-features-20261018/train_http.py --dataset /tmp/http-params-data/payload_full.csv --extractor /tmp/behavior-features --out /tmp/behavior-results
(cd scripts/research/behavior-features-20261018 && python3 -m unittest -v test_pipeline.py)
```

输出 `results.json` 为可发布聚合结果；`private_replay.json` 保存分组和行索引，仅用于本地核验；`clean_features.npz` 是数值特征矩阵。
六个 JSON 模型无需 pickle，使用 `portable_model.predict`；自动视图先用 `train_http.engineer`，再按模型 `selected_indices` 取列。
模型和数值矩阵作为独立实验包提供，原始公开语料不重复发布。源代码与报告留在 Git。
