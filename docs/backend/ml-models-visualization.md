# ML 模型对比可视化

本文档通过图表和可视化展示所有模型的性能特征。

## 📊 模型分布图

### 准确率 vs 推理速度

```mermaid
graph TD
    subgraph "高准确率区 (★★★★★)"
        NN[Neural Network<br/>5μs, 16KB]
        ENS[Ensemble<br/>50μs, varies]
    end

    subgraph "稳健区 (★★★★☆)"
        RF[Random Forest<br/>10μs, 50KB]
        KNN[KNN<br/>50μs, 10KB]
    end

    subgraph "平衡区 (★★★☆☆)"
        SVM[SVM<br/>2μs, 1KB]
        ET[Extra Trees<br/>8μs, 45KB]
        NB[Naive Bayes<br/>10μs, 3KB]
    end

    subgraph "快速区 (★★☆☆☆)"
        LR[Logistic Regression<br/>1μs, 1KB]
        RIDGE[Ridge<br/>1μs, 1KB]
        CENT[Nearest Centroid<br/>0.5μs, 3KB]
    end

    style NN fill:#e74c3c,color:#fff
    style ENS fill:#e74c3c,color:#fff
    style RF fill:#3498db,color:#fff
    style SVM fill:#2ecc71,color:#fff
    style LR fill:#f39c12,color:#fff
```

## 🎯 模型选择决策树

```mermaid
flowchart TD
    START([开始选择模型])

    START --> LATENCY{需要微秒级延迟?}

    LATENCY -->|是| ULTRA{延迟 < 2μs?}
    LATENCY -->|否| MEMORY{内存 < 5KB?}

    ULTRA -->|是| LR[Logistic Regression<br/>✓ 1μs<br/>✓ 1KB<br/>★★☆☆☆]
    ULTRA -->|否| SVM[SVM<br/>✓ 2μs<br/>✓ 1KB<br/>★★★☆☆]

    MEMORY -->|是| SMALL[Ridge / Centroid<br/>✓ 1-3KB<br/>✓ 极快]
    MEMORY -->|否| ACCURACY{需要最高准确率?}

    ACCURACY -->|是| HIGH[Ensemble / NN<br/>★★★★★<br/>⚠ 较慢]
    ACCURACY -->|否| INTERPRET{需要可解释?}

    INTERPRET -->|是| EXPLAIN[Logistic L1 / RF<br/>✓ 特征权重<br/>✓ 决策路径]
    INTERPRET -->|否| DEFAULT[Random Forest<br/>✓ 稳健<br/>✓ 通用<br/>★★★★☆]

    style LR fill:#f39c12,color:#fff
    style SVM fill:#2ecc71,color:#fff
    style HIGH fill:#e74c3c,color:#fff
    style DEFAULT fill:#3498db,color:#fff
```

## 🏗️ 模型架构图谱

### Random Forest 架构

```mermaid
graph LR
    INPUT[Feature Vector<br/>128维] --> T1[Tree 1]
    INPUT --> T2[Tree 2]
    INPUT --> T3[Tree 3]
    INPUT --> T4[...]
    INPUT --> T15[Tree 15]

    T1 --> V1[Vote: ALLOW]
    T2 --> V2[Vote: BLOCK]
    T3 --> V3[Vote: ALLOW]
    T4 --> V4[Vote: ALLOW]
    T15 --> V15[Vote: ALERT]

    V1 --> AGG[投票聚合]
    V2 --> AGG
    V3 --> AGG
    V4 --> AGG
    V15 --> AGG

    AGG --> RESULT[最终预测<br/>ALLOW: 8票<br/>BLOCK: 4票<br/>ALERT: 3票<br/><br/>→ ALLOW]

    style INPUT fill:#3498db,color:#fff
    style RESULT fill:#2ecc71,color:#fff
    style AGG fill:#9b59b6,color:#fff
```

### Neural Network 架构

```mermaid
graph LR
    I[输入层<br/>128维] --> H1[隐藏层<br/>32神经元<br/>ReLU激活]
    H1 --> O[输出层<br/>3类<br/>Argmax]
    O --> R[ALLOW/BLOCK/ALERT]

    subgraph "权重"
        W1[W_input<br/>128×32=4096]
        W2[W_output<br/>32×3=96]
    end

    I -.W1.-> H1
    H1 -.W2.-> O

    style I fill:#3498db,color:#fff
    style H1 fill:#9b59b6,color:#fff
    style O fill:#e74c3c,color:#fff
    style R fill:#2ecc71,color:#fff
```

### SVM 决策边界

```mermaid
graph TD
    INPUT[Feature Vector<br/>x ∈ R^128] --> DOT[点积计算<br/>w·x]
    DOT --> BIAS[加偏置<br/>decision = w·x + b]

    BIAS --> DECISION{decision值?}

    DECISION -->|< -500| BLOCK[BLOCK<br/>远离边界<br/>确定阻止]
    DECISION -->|-500 ~ 500| ALERT[ALERT<br/>接近边界<br/>需要审计]
    DECISION -->|> 500| ALLOW[ALLOW<br/>远离边界<br/>确定允许]

    style BLOCK fill:#e74c3c,color:#fff
    style ALERT fill:#f39c12,color:#fff
    style ALLOW fill:#2ecc71,color:#fff
```

## 📈 性能矩阵热图

### 内核态模型性能比较

| 指标 / 模型 | Logistic | SVM | NN | Random Forest |
|-----------|---------|-----|----|--------------|
| **推理延迟** | 🟢🟢🟢🟢🟢 | 🟢🟢🟢🟢⚪ | 🟢🟢🟢⚪⚪ | 🟢🟢⚪⚪⚪ |
| **内存占用** | 🟢🟢🟢🟢🟢 | 🟢🟢🟢🟢🟢 | 🟢🟢🟢⚪⚪ | 🟢⚪⚪⚪⚪ |
| **准确率** | 🟢🟢⚪⚪⚪ | 🟢🟢🟢⚪⚪ | 🟢🟢🟢🟢🟢 | 🟢🟢🟢🟢⚪ |
| **可解释性** | 🟢🟢🟢🟢⚪ | 🟢🟢🟢⚪⚪ | 🟢⚪⚪⚪⚪ | 🟢🟢🟢🟢⚪ |
| **训练速度** | 🟢🟢🟢🟢⚪ | 🟢🟢🟢⚪⚪ | 🟢⚪⚪⚪⚪ | 🟢🟢🟢⚪⚪ |

图例：🟢 好 | ⚪ 中

## 🔄 模型家族谱系

```mermaid
graph TD
    ML[机器学习模型<br/>51种]

    ML --> KERNEL[内核态<br/>4种]
    ML --> USER[用户态<br/>47种]

    KERNEL --> KRF[Random Forest]
    KERNEL --> KSVM[SVM]
    KERNEL --> KLR[Logistic Regression]
    KERNEL --> KNN[Neural Network]

    USER --> TREE[树模型家族<br/>18种]
    USER --> LINEAR[线性模型家族<br/>12种]
    USER --> ONLINE[在线学习<br/>6种]
    USER --> NEIGHBOR[近邻模型<br/>8种]
    USER --> OTHER[其他模型<br/>3种]

    TREE --> RF_FAMILY[Random Forest 6种]
    TREE --> ET_FAMILY[Extra Trees 4种]

    LINEAR --> LR_FAMILY[Logistic 6种]
    LINEAR --> SVM_FAMILY[SVM 3种]
    LINEAR --> RIDGE_FAMILY[Ridge 3种]

    ONLINE --> PERC_FAMILY[Perceptron 3种]
    ONLINE --> PA_FAMILY[PA 3种]

    NEIGHBOR --> KNN_FAMILY[KNN 4种]
    NEIGHBOR --> CENT_FAMILY[Centroid 4种]

    OTHER --> NB_FAMILY[Naive Bayes 2种]
    OTHER --> ADA_FAMILY[AdaBoost 3种]
    OTHER --> ENS_FAMILY[Ensemble 4种]

    style KERNEL fill:#e74c3c,color:#fff
    style USER fill:#3498db,color:#fff
    style TREE fill:#2ecc71,color:#fff
    style LINEAR fill:#9b59b6,color:#fff
```

## 🎯 使用场景地图

```mermaid
mindmap
  root((ML模型<br/>使用场景))
    生产环境
      random_forest
      random_forest_stable
      ensemble
    低延迟
      logistic
      svm
      ridge
    高准确率
      neural_network
      ensemble
      adaboost
    内存受限
      svm 1KB
      logistic 1KB
      ridge 1KB
      nearest_centroid 3KB
    数据不平衡
      logistic_balanced
      svm_balanced
      ensemble_stacked
    可解释性
      logistic_l1
      random_forest
      nearest_centroid
    在线学习
      perceptron
      passive_aggressive
```

## 📊 训练时间分布

```mermaid
gantt
    title 模型训练时间对比 (1000 样本)
    dateFormat X
    axisFormat %s

    section 极快 <0.2s
    Nearest Centroid :0, 1
    Naive Bayes      :0, 2

    section 快速 0.2-1s
    Logistic         :0, 5
    Ridge            :0, 4
    Perceptron       :0, 6

    section 中等 1-5s
    SVM              :0, 15
    Random Forest    :0, 30
    Extra Trees      :0, 20

    section 较慢 5-30s
    Neural Network   :0, 150
    AdaBoost         :0, 80

    section 慢 >30s
    Ensemble         :0, 400
```

## 🔬 特征工程流程

```mermaid
flowchart LR
    RAW[原始系统调用<br/>syscall_nr<br/>pid<br/>comm<br/>args] --> EXTRACT[特征提取]

    EXTRACT --> F1[特征 0-9<br/>系统调用类型]
    EXTRACT --> F2[特征 10-19<br/>进程属性]
    EXTRACT --> F3[特征 20-39<br/>参数编码]
    EXTRACT --> F4[特征 40-127<br/>上下文特征]

    F1 --> FV[Feature Vector<br/>128维]
    F2 --> FV
    F3 --> FV
    F4 --> FV

    FV --> MODEL[ML模型]
    MODEL --> RESULT[ALLOW/BLOCK/ALERT]

    style RAW fill:#3498db,color:#fff
    style FV fill:#9b59b6,color:#fff
    style RESULT fill:#2ecc71,color:#fff
```

## 🚀 推理管线

```mermaid
sequenceDiagram
    participant User as 用户空间进程
    participant eBPF as eBPF 程序
    participant Kernel as 内核模块
    participant GPU as CUDA Helper

    User->>eBPF: 系统调用 (execve)
    eBPF->>eBPF: 提取特征 (128维)
    eBPF->>Kernel: /proc/ml_predict

    alt CUDA 后端
        Kernel->>GPU: /proc/ml_cuda_request
        GPU->>GPU: GPU 推理 (3.8μs)
        GPU->>Kernel: /proc/ml_cuda_result
    else CPU 后端
        Kernel->>Kernel: CPU 推理 (10μs)
    end

    Kernel->>eBPF: ALLOW/BLOCK/ALERT

    alt BLOCK
        eBPF->>User: -EPERM (拒绝执行)
    else ALLOW
        eBPF->>User: 0 (允许执行)
    else ALERT
        eBPF->>User: 0 (允许但记录)
    end
```

## 📦 模型文件格式

```mermaid
graph TD
    PKL[sklearn model<br/>.pkl] --> EXPORT[model_loader.py]

    EXPORT --> BIN[二进制格式<br/>.bin]

    BIN --> HEADER[Header<br/>24 bytes]
    BIN --> TREE1[Tree 1<br/>nodes × 32B]
    BIN --> TREE2[Tree 2<br/>nodes × 32B]
    BIN --> TREE_N[Tree N<br/>nodes × 32B]

    HEADER --> V[version: u32]
    HEADER --> NT[num_trees: u32]
    HEADER --> FD[feature_dim: u32]
    HEADER --> TN[total_nodes: u32]
    HEADER --> NC[num_classes: u32]
    HEADER --> MD[max_depth: u32]

    TREE1 --> NODE[Node Structure<br/>32 bytes]

    NODE --> FI[feature_idx: u32]
    NODE --> TH[threshold: s64]
    NODE --> LC[left_child: s32]
    NODE --> RC[right_child: s32]
    NODE --> LV[leaf_value: s32]
    NODE --> IL[is_leaf: u8]

    style PKL fill:#3498db,color:#fff
    style BIN fill:#2ecc71,color:#fff
    style NODE fill:#e74c3c,color:#fff
```

---

## 📚 相关文档

- [ML 模型速查表](./ml-models-summary.md)
- [ML 模型完整指南](./ml-models-complete-guide.md)
- [内核态多模型实现](/backend/multi-model-complete)
