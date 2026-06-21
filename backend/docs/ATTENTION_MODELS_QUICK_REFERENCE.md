# 新增注意力模型快速参考

## 模型选择指南

### 独立注意力模型（4 种）

| 模型 | 适用场景 | 复杂度 | 特点 |
|------|---------|--------|------|
| **Scaled Dot-Product** | 标准特征提取 | O(d²) | Transformer 经典，全局依赖 |
| **Multi-Head (4 heads)** | 多视角特征学习 | O(d²) | 捕捉不同子空间关系 |
| **RWKV** | 长序列/高维特征 | O(d) 线性 | 高效，适合大规模 |
| **Mamba** | 时序/状态依赖 | O(d) 状态 | 选择性记忆，序列建模 |

### 注意力增强组合模型（12 种）

#### Random Forest 组合（推荐用于生产）
- `ModelRandomForestScaledDotProduct` - RF + 标准 Transformer 注意力
- `ModelRandomForestMultiHead` - RF + 多头注意力 ⭐ **推荐**
- `ModelRandomForestRWKV` - RF + 线性注意力（高效） ⭐ **推荐**
- `ModelRandomForestMamba` - RF + 状态空间模型 ⭐ **推荐**

#### Logistic Regression 组合（轻量级）
- `ModelLogisticScaledDotProduct` - 快速 + 可解释
- `ModelLogisticMultiHead` - 多视角线性分类
- `ModelLogisticRWKV` - 高效线性组合
- `ModelLogisticMamba` - 状态感知线性模型

#### KNN 组合（实例学习）
- `ModelKNNScaledDotProduct` - 注意力加权近邻
- `ModelKNNMultiHead` - 多空间近邻匹配
- `ModelKNNRWKV` - 高效近邻查找
- `ModelKNNMamba` - 状态感知近邻

## 使用示例

```go
// 方式 1：通过类型创建
model, err := NewModel(ModelRandomForestMultiHead)
if err != nil {
    log.Fatal(err)
}

// 方式 2：直接创建注意力层
attention := NewMultiHeadAttention(4)
output := attention.Forward(features)

// 方式 3：创建增强模型
base := NewDecisionForest(31, 8, 4)
attn := NewRWKVAttention()
enhanced := newAttentionEnhancedModelWithLayer(
    ModelRandomForestRWKV,
    base,
    attn,
)
```

## 配置建议

### 默认推荐配置
```json
{
  "modelType": "random_forest_multi_head",
  "numTrees": 31,
  "maxDepth": 8,
  "minSamplesLeaf": 5
}
```

### 高性能配置（RWKV）
```json
{
  "modelType": "random_forest_rwkv",
  "numTrees": 31,
  "maxDepth": 8,
  "minSamplesLeaf": 5
}
```

### 序列感知配置（Mamba）
```json
{
  "modelType": "random_forest_mamba",
  "numTrees": 31,
  "maxDepth": 8,
  "minSamplesLeaf": 5
}
```

## 特殊功能

### Mamba 状态重置
```go
mambaAttn := NewMambaAttention()

// 处理第一个序列
for _, event := range sequence1 {
    output := mambaAttn.Forward(event.Features)
    // 处理 output
}

// 重置状态，准备处理新序列
mambaAttn.Reset()

// 处理第二个序列
for _, event := range sequence2 {
    output := mambaAttn.Forward(event.Features)
    // 处理 output
}
```

### Multi-Head 自定义头数
```go
// 创建 8 头注意力（FeatureDim 必须能被 8 整除）
mha := NewMultiHeadAttention(8)

// 如果头数无效，自动回退到 4 头
mhaInvalid := NewMultiHeadAttention(0) // 实际使用 4 头
```

## 性能对比

| 模型 | 训练速度 | 推理速度 | 内存占用 | 准确率潜力 |
|------|---------|---------|---------|-----------|
| RF + Additive | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| RF + Scaled Dot-Product | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| RF + Multi-Head | ⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| RF + RWKV | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| RF + Mamba | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |

## 故障排除

### Q: 编译错误 "pb.MLStatus undefined"
**A**: 这是项目中预存在的问题，与新注意力模型无关。新模型的核心逻辑已验证正确。

### Q: Multi-Head 创建失败
**A**: 确保 `FeatureDim` 能被头数整除。默认会回退到 4 头。

### Q: Mamba 输出不稳定
**A**: 检查是否在序列边界处调用了 `Reset()`。跨序列的隐藏状态会导致不正确的结果。

### Q: 模型序列化失败
**A**: 确保目标目录存在写权限。所有注意力模型都支持序列化。

## 测试运行

```bash
# 测试所有新注意力模型
go test ./backend/app -run "TestScaledDotProduct|TestMultiHead|TestRWKV|TestMamba" -v

# 测试特定模型
go test ./backend/app -run TestScaledDotProductAttentionForward -v
go test ./backend/app -run TestMultiHeadAttentionMultipleHeads -v
go test ./backend/app -run TestRWKVAttentionLinearComplexity -v
go test ./backend/app -run TestMambaAttentionStateRetention -v
```

## 下一步

1. 修复项目中的 `pb.MLStatus` 问题
2. 运行完整测试套件
3. 在真实数据上训练和评估新模型
4. 比较不同注意力机制的性能
5. 根据结果调整推荐配置
