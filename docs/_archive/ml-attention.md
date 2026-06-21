# 注意力机制技术文档

## 1. 概述

本项目已实现并注册了多种注意力机制与注意力增强模型，主要用于固定维度特征向量 `[FeatureDim]float64` 的特征重加权、上下文建模与分类增强。

当前实现的核心思路是：

- 先对输入特征做注意力变换
- 再将变换后的特征送入基础模型进行预测
- 部分注意力层支持序列式前向、缓存中间值与简单反向更新

> 说明：以下数学表达以代码实现为准，结合了标准注意力公式与项目中的固定向量实现方式。

---

## 2. 支持的注意力类型

### 2.1 Additive Attention (Bahdanau Attention)

文件：`/home/steve/文档/vibe coding/agent-ebpf-filiter/backend/app/ml__attention_additive attention (bahdanau).go`

#### 数学原理

给定输入向量 `x ∈ R^d`，项目中的 Additive Attention 采用如下变换：

1. 线性投影

\[
 h_i = \tanh(W_i x + b_i)
\]

2. 打分

\[
 s = v^T h
\]

3. 注意力系数

在该实现中，单输入向量的 softmax 退化为恒等系数：

\[
 \alpha = 1
\]

4. 输出

\[
 y = \alpha x = x
\]

也就是说，当前代码更接近一个带门控信息缓存的残差投影层，保留了 Bahdanau 风格的非线性打分结构，但在单向量场景中输出与输入同维。

#### 实现特征

- 初始化时 `W` 使用单位矩阵
- `V` 初始为全 1
- 支持缓存 `LastInput / LastHidden / LastScore / LastAlpha`
- 支持返回梯度：`gradInput, gradW, gradB, gradV`
- 支持序列化与反序列化

---

### 2.2 Self-Attention

当前代码中，Self-Attention 逻辑以 `CrossAttentionLayer` 的同输入退化形式体现。

#### 数学原理

标准 self-attention 公式为：

\[
 Q = XW_Q, \quad K = XW_K, \quad V = XW_V
\]

\[
 A = \text{softmax}\left(\frac{QK^T}{\sqrt{d_k}}\right)
\]

\[
 Y = AV
\]

在本项目固定向量实现中，输入是单个特征向量而非完整序列，因此使用逐维点积与逐维 softmax 近似：

\[
 s_i = \frac{q_i k_i}{\sqrt{d}}
\]

\[
 a = \text{softmax}(s)
\]

\[
 y_i = a_i v_i
\]

#### 实现特征

- 依赖 `CrossAttentionLayer` 的同输入调用方式
- 支持缓存 `LastQ / LastK / LastV / LastA / LastY`
- 支持 `Backward(dY)` 做最小 SGD 更新

---

### 2.3 Cross-Attention

文件：`/home/steve/文档/vibe coding/agent-ebpf-filiter/backend/app/ml__attention_cross-attention.go`

#### 数学原理

标准 cross-attention 由查询、键和值三部分组成：

\[
 Q = X_Q W_Q, \quad K = X_K W_K, \quad V = X_V W_V
\]

\[
 A = \text{softmax}\left(\frac{QK^T}{\sqrt{d_k}}\right)
\]

\[
 Y = AV
\]

本项目中采用固定维度向量版本：

- `Q/K/V` 都是 `[FeatureDim]float64`
- 按维度计算得分
- 使用一维 softmax 得到注意力权重
- 输出按维度缩放 `V`

#### 实现特征

- `Wq / Wk / Wv` 默认是单位矩阵
- `Predict()` 内部使用 `Output(features, features, features)` 作为兼容实现
- `Backward(dY)` 会更新 `Wq / Wk / Wv`
- 支持二进制序列化，文件头标记为 `CATN`

---

### 2.4 Multi-Head Attention

文件：`/home/steve/文档/vibe coding/agent-ebpf-filiter/backend/app/ml__attention_multi-head attention.go`

#### 数学原理

多头注意力将多个单头注意力并行计算：

\[
 head_h = \text{Attention}(QW_Q^{(h)}, KW_K^{(h)}, VW_V^{(h)})
\]

\[
 H = [head_1, head_2, \dots, head_n]
\]

\[
 Y = HW_O
\]

在本项目中实现为：

- 每个 head 独立拥有 `WQ/WK/WV`
- 各 head 输出后求平均作为拼接向量
- 再通过输出投影 `WO`

#### 实现特征

- 默认 `NumHeads = 4`
- `WQ/WK/WV/WO` 初始均为单位矩阵
- 支持缓存 `LastHeads / LastConcat / LastY`
- 支持 `Backward(dY)` 更新 `WO` 和 `WV`
- 支持二进制序列化，文件头标记为 `MHA1`

---

## 3. 注意力增强模型列表

文件：`/home/steve/文档/vibe coding/agent-ebpf-filiter/backend/app/ml__attention_models.go`

当前注册的注意力增强模型如下：

1. `ModelRandomForestAttention`
   - 基础模型：`DecisionForest(31, 8, 4)`
   - 包装器：`attentionEnhancedModel`
   - 注意力层：`NewAdditiveAttention()`

2. `ModelLogisticAttention`
   - 基础模型：`LogisticModel(0.01, "l2", 1000)`
   - 包装器：`attentionEnhancedModel`
   - 注意力层：`NewAdditiveAttention()`

3. `ModelKNNAttention`
   - 基础模型：`KNNModel(5, "euclidean", "uniform")`
   - 包装器：`attentionEnhancedModel`
   - 注意力层：`NewAdditiveAttention()`

### 包装器机制

文件：`/home/steve/文档/vibe coding/agent-ebpf-filiter/backend/app/ml__attention_model_wrapper.go`

包装器流程如下：

\[
 x' = Attention(x)
\]

\[
 \hat{y} = BaseModel(x')
\]

即先对特征向量做注意力增强，再送入基础分类器。

---

## 4. 性能对比

### 4.1 结构复杂度对比

| 类型 | 计算方式 | 参数规模 | 典型开销 | 适用场景 |
|---|---:|---:|---:|---|
| Additive Attention | `tanh + v^T` | `O(d^2)` | 低 | 单向量特征重加权 |
| Cross-Attention | `Q/K/V + softmax` | `O(3d^2)` | 中 | 特征间对齐、上下文交互 |
| Self-Attention | 同输入 cross-attention | `O(3d^2)` | 中 | 单样本内部依赖建模 |
| Multi-Head Attention | 多头并行 attention + `WO` | `O(h·3d^2 + d^2)` | 较高 | 多视角特征建模 |

### 4.2 当前实现层面的特点

- **Additive Attention**
  - 最轻量
  - 在当前单向量场景下输出近似恒等映射
  - 更适合作为前置特征门控层

- **Cross-Attention**
  - 提供 Q/K/V 完整结构
  - 带简单 SGD 反向更新
  - 适合演示式或轻量研究型使用

- **Multi-Head Attention**
  - 表达能力更强
  - 适合组合多个子空间视角
  - 当前反向传播仅更新部分参数，训练能力偏基础

### 4.3 选择建议

- 追求速度与稳定性：选 Additive Attention
- 追求结构完整性：选 Cross-Attention
- 追求更强表示能力：选 Multi-Head Attention
- 若已有树模型、线性模型或 KNN：优先使用注意力增强包装器

> 注：本项目未在代码中提供统一 benchmark 结果表，因此“性能对比”这里以结构复杂度与实现特征为主。若后续补充实验框架，可扩展为 Accuracy / F1 / AUC / 推理耗时 / 训练耗时对比表。

---

## 5. 使用示例代码

### 5.1 直接使用 Additive Attention

```go
package main

import (
    "fmt"
    "backend/app"
)

func main() {
    attn := app.NewAdditiveAttention()

    var x [app.FeatureDim]float64
    x[0] = 1.2
    x[1] = 0.7
    x[2] = -0.3

    y := attn.Forward(x)
    fmt.Println(y)
}
```

### 5.2 直接使用 Cross-Attention

```go
package main

import (
    "fmt"
    "backend/app"
)

func main() {
    attn := app.NewCrossAttentionLayer()

    var q, k, v [app.FeatureDim]float64
    q[0] = 1.0
    k[0] = 0.8
    v[0] = 0.5

    y := attn.Output(q, k, v)
    fmt.Println(y)

    attn.Backward(y)
}
```

### 5.3 使用 Multi-Head Attention

```go
package main

import (
    "fmt"
    "backend/app"
)

func main() {
    attn := app.NewMultiHeadAttentionLayer(4)

    var x [app.FeatureDim]float64
    x[0] = 1.0
    x[1] = 0.2
    x[2] = 0.6

    y := attn.Output(x, x, x)
    fmt.Println(y)

    attn.Backward(y)
}
```

### 5.4 使用注意力增强模型

```go
package main

import (
    "fmt"
    "backend/app"
)

func main() {
    model := app.NewAttentionEnhancedModelForDemo() // 示例：按项目实际构造方式替换

    var x [app.FeatureDim]float64
    x[0] = 0.9
    x[1] = 0.1

    pred := model.Predict(x)
    fmt.Printf("action=%d confidence=%.4f anomaly=%.4f\n", pred.Action, pred.Confidence, pred.AnomalyScore)
}
```

### 5.5 保存与加载模型

```go
err := attn.Serialize("/tmp/attn.bin")
if err != nil {
    panic(err)
}

loaded, err := app.DeserializeAdditiveAttention("/tmp/attn.bin")
if err != nil {
    panic(err)
}

_ = loaded
```

---

## 6. 小结

本项目当前已支持四类注意力相关能力：

- Additive Attention
- Self-Attention
- Cross-Attention
- Multi-Head Attention

并提供了注意力增强模型包装器，将注意力层用于传统机器学习模型前的特征变换。整体实现更偏向轻量、可序列化、可嵌入现有推理流程的工程实践。

后续若补充实验框架，可以将本页扩展为完整的模型评测文档，包括数据集、指标、超参数和结果图表。

---

## 相关导航

- [ML、Plugins 与扩展能力](backend/ml-plugins.md)
- [ML 模型完整指南](backend/ml-models-complete-guide.md)
- [ML 实验框架](/backend/ml-experiments)
- [ML benchmark](ml-benchmark-report.md)
- [多模型支持设计](multi-model-support.md)
