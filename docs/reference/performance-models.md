# 性能分析与数学模型

本页展示项目中涉及的性能指标计算、风险评分算法和统计模型。

## eBPF 事件处理性能

### 零拷贝优化收益

ringbuf 解码在满足条件时使用零拷贝：

<div>

<div>


$$
T_{decode} = \begin{cases}
T_{view} & \text{if aligned and little-endian} \\
T_{copy} + T_{parse} & \text{otherwise}
\end{cases}
$$


</div>

</div>

其中：
- $T_{view}$：直接构造指针视图的时间（$\sim$ 1-2 ns）
- $T_{copy}$：内存拷贝时间（$\sim$ 50-100 ns）
- $T_{parse}$：`binary.Read` 解析时间（$\sim$ 20-30 ns）

性能提升比例：

<div>


$$
\text{Speedup} = \frac{T_{copy} + T_{parse}}{T_{view}} \approx 35-65\times
$$


</div>

### Ringbuf 容量与事件丢失率

设系统事件到达率为 $\lambda$ events/s，ringbuf 容量为 $C$ bytes，平均事件大小为 $s$ bytes，后端处理速率为 $\mu$ events/s。

队列占用率：

<div>


$$
\rho = \frac{\lambda}{\mu}
$$


</div>

当 $\rho < 1$ 时系统稳定。事件丢失率：

<div>


$$
P_{loss} = \begin{cases}
0 & \text{if } \rho \leq 1 \\
1 - \frac{1}{\rho} & \text{if } \rho > 1
\end{cases}
$$


</div>

推荐 ringbuf 大小：

<div>


$$
C \geq k \cdot s \cdot \frac{\lambda}{\mu} \cdot T_{burst}
$$


</div>

其中 $k$ 为安全系数（推荐 2-3），$T_{burst}$ 为预期最大突发时长。

## ML 风险评分模型

### 命令风险评分

wrapper policy engine 使用加权多因子模型：

<div>


$$
R_{command} = \sum_{i=1}^{n} w_i \cdot f_i(c, a, m)
$$


</div>

其中：
- $R_{command}$：最终风险评分（0-1）
- $w_i$：第 $i$ 个因子权重，$\sum w_i = 1$
- $f_i$：特征提取函数（命令 $c$、参数 $a$、元数据 $m$）

典型因子：

| 因子 | 权重 | 计算方式 |
| --- | --- | --- |
| 命令危险度 | 0.4 | $f_{cmd}(c) \in \{0, 0.5, 1\}$ |
| 参数模式 | 0.3 | ML classifier output |
| Agent 历史 | 0.2 | Bayesian update |
| 上下文异常 | 0.1 | Isolation Forest score |

### 阈值决策

策略决策使用分段阈值：

<div>


$$
\text{Decision}(R) = \begin{cases}
\text{ALLOW} & \text{if } R < \theta_{alert} \\
\text{ALERT} & \text{if } \theta_{alert} \leq R < \theta_{block} \\
\text{BLOCK} & \text{if } R \geq \theta_{block}
\end{cases}
$$


</div>

默认阈值：$\theta_{alert} = 0.5$，$\theta_{block} = 0.8$。

### 贝叶斯 Agent 信誉更新

Agent 历史信誉使用贝叶斯更新：

<div>


$$
P(trustworthy | evidence) = \frac{P(evidence | trustworthy) \cdot P(trustworthy)}{P(evidence)}
$$


</div>

初始先验：$P(trustworthy) = 0.8$（假设新 Agent 默认可信）。

经过 $n$ 次观测后：

<div>


$$
P_n(trustworthy) = \frac{\alpha + n_{safe}}{\alpha + \beta + n}
$$


</div>

其中：
- $\alpha, \beta$：Beta 分布先验参数
- $n_{safe}$：安全行为次数
- $n$：总观测次数

## 网络流聚合

### Flow 超时与状态机

TCP flow 状态转移概率：

<div>


$$
P(s_{t+1} = j | s_t = i) = \begin{bmatrix}
0.9 & 0.08 & 0.02 \\
0 & 0.95 & 0.05 \\
0 & 0 & 1
\end{bmatrix}
$$


</div>

其中状态：$i, j \in \{\text{ACTIVE}, \text{IDLE}, \text{CLOSED}\}$。

Flow 被标记为 stale 的概率（经过 $t$ 时间）：

<div>


$$
P_{stale}(t) = 1 - e^{-\lambda \cdot t}
$$


</div>

其中 $\lambda$ 为 GC 速率参数，默认 $\lambda = \frac{1}{300}$ s$^{-1}$（5 分钟半衰期）。

### DNS 缓存命中率

设 DNS 查询到达率为 $\lambda_q$，缓存 TTL 为 $T$，unique domain 数为 $N_d$。

缓存命中率（假设 Zipf 分布访问模式）：

<div>


$$
H = 1 - \frac{1}{1 + \frac{\lambda_q \cdot T}{N_d}}
$$


</div>

推荐缓存大小：

<div>


$$
C_{dns} = N_d \cdot (1 + \epsilon)
$$


</div>

其中 $\epsilon = 0.2$ 为安全余量。

## Event Archive 与内存管理

### Ring Buffer 占用

Event archive 使用 bounded ring buffer，内存占用：

<div>


$$
M_{archive} = n \cdot (s_{event} + s_{overhead})
$$


</div>

其中：
- $n$：最大事件数（默认 10,000）
- $s_{event}$：平均事件大小（$\sim$ 512 bytes）
- $s_{overhead}$：Go slice/struct overhead（$\sim$ 64 bytes）

总内存估算：

<div>


$$
M_{total} \approx 10000 \times 576 \text{ bytes} \approx 5.76 \text{ MB}
$$


</div>

### Eviction 策略

时间窗口驱逐：

<div>


$$
\text{evict}(e) = \begin{cases}
\text{true} & \text{if } t_{now} - t_e > T_{max} \\
\text{false} & \text{otherwise}
\end{cases}
$$


</div>

其中 $T_{max}$ 默认为 24 小时。

容量驱逐（FIFO）：

<div>


$$
\text{evict}(e_i) = \text{true} \iff i < n - n_{max}
$$


</div>

## 性能基准

### Ringbuf 吞吐量

测试结果（10 万事件 replay）：

<div>


$$
\text{Throughput} = \frac{N_{events}}{T_{total}} \approx 25000-30000 \text{ events/s}
$$


</div>

P99 延迟：

<div>


$$
L_{p99} = \mu + 2.33\sigma \approx 150 \mu\text{s}
$$


</div>

其中 $\mu \approx 40 \mu\text{s}$，$\sigma \approx 47 \mu\text{s}$（实测数据）。

### WebSocket 广播延迟

客户端数量为 $n$ 时，广播延迟：

<div>


$$
T_{broadcast}(n) = T_{serialize} + n \cdot T_{write}
$$


</div>

实测：
- $T_{serialize} \approx 10 \mu\text{s}$
- $T_{write} \approx 50 \mu\text{s}$

对于 $n = 10$ 客户端：

<div>


$$
T_{broadcast}(10) \approx 10 + 10 \times 50 = 510 \mu\text{s}
$$


</div>

## 参考

- [事件管线](/backend/event-pipeline)
- [Runtime Settings](/backend/runtime-settings-features)
- [Wrapper 命令策略](/integrations/wrapper)
- [验证与 Benchmark](/operations/verification-benchmark)
