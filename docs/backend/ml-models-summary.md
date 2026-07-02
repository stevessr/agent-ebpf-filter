# ML 模型速查表

快速查找 agent-ebpf-filter 中所有可用的机器学习模型。

## | 需求 | 推荐模型 | 链接 |
|------|---------|------|
| **生产环境默认** | `random_forest` / `random_forest_stable` | [详情](#random-forest-系列) |
| **极速响应 (<2μs)** | `logistic` / `svm` | [详情](#内核态模型概览) |
| **最高准确率** | `ensemble` / `neural_network` | [详情](#ensemble-系列) |
| **数据不平衡** | `logistic_balanced` / `ensemble_stacked` | [详情](#logistic-regression-系列) |
| **少样本合成扩增** | `gan_transformer` | [详情](#gan--transformer) |
| **可解释性** | `logistic_l1` / `random_forest` | [详情](#使用场景) |
| **内存受限 (<5KB)** | `ridge` / `svm` / `nearest_centroid` | [详情](#内核态模型概览) |

---

## 在 `kernel-ml/` 中实现，DKMS 内核模块，微秒级推理。

| 模型 | 延迟 | 内存 | 准确率 | 复杂度 | 用途 |
|------|------|------|--------|--------|------|
| **Random Forest** | ~10 μs | 50 KB | ★★★★☆ | O(T×log N) | 通用稳健 |
| **SVM** | ~2 μs | 1 KB | ★★★☆☆ | O(D) | 低延迟 |
| **Logistic Regression** | ~1 μs | 1 KB | ★★☆☆☆ | O(D) | 极速响应 |
| **Neural Network** | ~5 μs | 16 KB | ★★★★★ | O(H×D) | 高准确率 |

**特性**:
- ✅ 定点数运算（无浮点）
- ✅ 热切换（运行时动态加载）
- ✅ CUDA 后端（可选 GPU 加速）
- ✅ 64 项 LRU 缓存

---

## (47 种)

基于 scikit-learn，用于训练和评估。

### Random Forest 系列 (6 种)

| ID | 描述 | 参数 | 标签 |
|----|------|------|------|
| `random_forest` ⭐ | 稳健默认 | trees=31, depth=8 | `stable` |
| `random_forest_fast` | 快速推理 | trees=5, depth=4 | `fast` |
| `random_forest_shallow` | 浅层防过拟合 | trees=15, depth=4 | `low-overfit` |
| `random_forest_stable` ⭐ | 高稳定性 | trees=51, depth=10 | `stable` |
| `random_forest_deep` | 深层高容量 | trees=31, depth=12 | `high-capacity` |
| `random_forest_wide` | 宽森林 | trees=71, depth=8 | `wide` |

### Extra Trees 系列 (4 种)

| ID | 描述 | 参数 | 标签 |
|----|------|------|------|
| `extra_trees` | 极随机树 | trees=31, depth=8 | `randomized` |
| `extra_trees_fast` | 快速版 | trees=9, depth=5 | `fast` |
| `extra_trees_deep` | 深层版 | trees=31, depth=12 | `high-capacity` |
| `extra_trees_wide` | 宽森林版 | trees=71, depth=10 | `wide` |

### Logistic Regression 系列 (6 种)

| ID | 描述 | 正则化 | 标签 |
|----|------|--------|------|
| `logistic` | L2 正则 | L2, C=1.0 | `interpretable` |
| `logistic_fast` | 快速训练 | L2, max_iter=20 | `fast` |
| `logistic_none` | 无正则 | penalty='none' | `ablation` |
| `logistic_l1` ⭐ | L1 稀疏 | L1, C=0.1 | `interpretable`, `sparse` |
| `logistic_balanced` | 类别加权 | class_weight='balanced' | `balanced` |
| `logistic_l1_balanced` | L1+ 平衡 | L1, balanced | `balanced`, `sparse` |

### SVM 系列 (3 种)

| ID | 描述 | 核函数 | 标签 |
|----|------|--------|------|
| `svm` | 线性 SVM | linear | `margin` |
| `svm_long` | 长迭代 | linear, iter=4000 | `long` |
| `svm_balanced` | 类别加权 | linear, balanced | `balanced` |

### Ridge 系列 (3 种)

| ID | 描述 | 正则强度 | 标签 |
|----|------|---------|------|
| `ridge` | 标准 | alpha=1.0 | `linear` |
| `ridge_light` | 弱正则 | alpha=0.1 | `light` |
| `ridge_strong` | 强正则 | alpha=10.0 | `regularized` |

### Perceptron 系列 (3 种)

| ID | 描述 | 迭代 | 标签 |
|----|------|------|------|
| `perceptron` | 在线感知机 | iter=20 | `online` |
| `perceptron_long` | 长迭代 | iter=150 | `long` |
| `perceptron_balanced` | 类别加权 | balanced | `online`, `balanced` |

### Passive-Aggressive 系列 (3 种)

| ID | 描述 | 迭代 | 标签 |
|----|------|------|------|
| `passive_aggressive` | PA 在线 | iter=10 | `online` |
| `passive_aggressive_long` | 长迭代 | iter=20 | `long` |
| `passive_aggressive_balanced` | 类别加权 | balanced | `online`, `balanced` |

### KNN 系列 (4 种)

| ID | 描述 | 距离度量 | K |
|----|------|---------|---|
| `knn` | 欧氏距离 | euclidean | 5 |
| `knn_manhattan` | 曼哈顿距离 | manhattan | 7 |
| `knn_cosine` | 余弦距离 | cosine | 7 |
| `knn_distance` | 距离加权 | euclidean, weighted | 5 |

### Nearest Centroid 系列 (4 种)

| ID | 描述 | 距离 | 标签 |
|----|------|------|------|
| `nearest_centroid` | 欧氏质心 | euclidean | `fast`, `interpretable` |
| `nearest_centroid_balanced` | 均匀先验 | euclidean | `balanced`, `fast` |
| `nearest_centroid_cosine` | 余弦质心 | cosine | `fast` |
| `nearest_centroid_manhattan` | 曼哈顿质心 | manhattan | `fast` |

### Naive Bayes 系列 (2 种)

| ID | 描述 | 先验 | 标签 |
|----|------|------|------|
| `naive_bayes` | 高斯 NB | 经验先验 | `probabilistic` |
| `naive_bayes_balanced` | 均匀先验 | 均匀先验 | `balanced`, `probabilistic` |

### AdaBoost 系列 (3 种)

| ID | 描述 | 估计器数 | 标签 |
|----|------|---------|------|
| `adaboost` | AdaBoost | n=100 | `boosting` |
| `adaboost_fast` | 快速版 | n=25 | `fast` |
| `adaboost_large` | 大容量 | n=200 | `large` |

### Ensemble 系列 (4 种)

| ID | 描述 | 成员模型 | 标签 |
|----|------|---------|------|
| `ensemble` ⭐ | 软投票集成 | RF+LR+NB+KNN | `ensemble`, `stable` |
| `ensemble_soft` | 显式软投票 | 加权概率融合 | `soft-vote` |
| `ensemble_hard` | 硬投票 | 多数投票 | `hard-vote` |
| `ensemble_stacked` ⭐ | 风险堆叠 | 少数高风险优先 | `risk-stacked` |

### GAN + Transformer

| ID | 描述 | 默认参数 | 标签 |
|----|------|----------|------|
| `gan_transformer` | Class-conditioned GAN 合成样本 + Transformer 编码判别器，用于少样本和边界样本增强对照 | latent=16, epochs≈24, synthetic/class=8 | `gan`, `transformer`, `synthetic` |

---

## ### 1: 生产环境部署

```bash
# 训练
python3 train_model.py --model random_forest_stable --output model.bin

# 加载到内核
sudo cat model.bin > /proc/ml_load
cat /proc/ml_stats
```

### 2: 低延迟实时防御

```bash
# 推理 < 2 μs
python3 train_model.py --model logistic_l1 --output lr.bin
sudo cat lr.bin > /proc/ml_load
```

### 3: 最高准确率

```bash
# 训练集成模型
python3 train_ensemble.py --output ensemble.bin
sudo cat ensemble.bin > /proc/ml_load
```

### 4: 数据不平衡

```bash
# 类别加权
python3 train_model.py --model logistic_balanced --output model.bin
```

---

## ### ```
LR  ▌ 1 μs
SVM ▌▌ 2 μs
NN  ▌▌▌▌▌ 5 μs
RF  ▌▌▌▌▌▌▌▌▌▌ 10 μs
```

### vs 速度

![准确率 vs 速度权衡](./ml-accuracy-vs-speed.svg)

### ![推理延迟对比](./ml-latency-comparison.svg)

### ![内存占用对比](./ml-memory-comparison.svg)

---

## ```bash
# 查看模型状态
cat /proc/ml_stats
cat /sys/kernel/kernel_ml/model_info

# 切换推理后端
echo auto > /proc/ml_backend

# 启用 CUDA
echo cuda > /proc/ml_backend
sudo ./kernel_ml_cuda_helper &

# 查看缓存统计
cat /sys/kernel/kernel_ml/cache_stats

# 热切换模型
sudo cat model_new.bin > /proc/ml_load
```

---

## - **[ML 模型完整指南](./ml-models-complete-guide.md)** - 详细结构、示例、API
- **[内核态多模型实现](/backend/multi-model-complete)** - 实现细节
- **[实验框架使用](/backend/ml-experiments)** - 批量评估
- **[内核 ML 实现](/backend/kernel-ml-implementation)** - 内核模块架构

---

**总计**: 4 种内核态模型 + 48+ 种用户态模型变体（含 `gan_transformer`）= **52+ 种 ML 模型**
