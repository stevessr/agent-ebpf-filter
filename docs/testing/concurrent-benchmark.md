# 并发性能测试

## 概述

并发性能测试用于模拟真实场景下多个线程/进程同时执行系统调用的情况，评估 eBPF 钩子在高并发环境下的开销。

## 快速开始

### 1. 启动后端

```bash
cd backend
sudo go run ./app
```

### 2. 运行并发测试

```bash
# 测试 1, 4, 8, 16, 32 个并发级别，每级别 10 个周期
BENCH_CYCLES=10 ./scripts/benchmark-concurrent.sh

# 自定义并发级别
BENCH_CONCURRENCY="1 8 32 64" BENCH_CYCLES=20 ./scripts/benchmark-concurrent.sh
```

### 3. 查看结果

```bash
# 查看最新的报告
cat reports/ebpf-concurrent-*/concurrent_report.txt
```

## 测试参数

### 环境变量

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `BENCH_CONCURRENCY` | `1 4 8 16 32` | 要测试的并发级别列表 |
| `BENCH_CYCLES` | `10` | 每个并发级别的测试周期数 |
| `BENCH_STAMP` | 当前时间戳 | 报告目录时间戳 |
| `EBPF_BENCH_OUTDIR` | `reports/ebpf-concurrent-*` | 输出目录 |

### 内部参数

每个测试运行使用以下参数：
- **Runs**: 3（每个操作重复 3 次）
- **Warmup**: 1（预热 1 次）
- **Iterations**: 5000（每次运行 5000 次迭代）

总测量次数 = `Cycles × Concurrency_Levels × Operations × Runs × Iterations`

示例：`10 × 5 × 18 × 3 × 5000 = 13,500,000` 次测量

## 并发级别说明

- **C=1**: 单线程（基准）
- **C=4**: 轻度并发（典型桌面应用）
- **C=8**: 中度并发（多核服务器）
- **C=16**: 高并发（繁忙服务器）
- **C=32**: 极高并发（高吞吐量服务）
- **C=64**: 压力测试（极端场景）

## 输出格式

### 目录结构

```
reports/ebpf-concurrent-<timestamp>/
├── raw_baseline_c1.jsonl    # 并发=1 的 baseline 原始数据，每个 cycle 一行
├── raw_ebpf_c1.jsonl        # 并发=1 的 eBPF 原始数据，每个 cycle 一行
├── raw_baseline_c4.jsonl    # 并发=4 的 baseline 原始数据
├── raw_ebpf_c4.jsonl
├── ...
├── agg_baseline_c1.json     # 并发=1 的 baseline 聚合数据
├── agg_ebpf_c1.json
├── summary_c1.json          # 并发=1 的最终摘要
├── summary_c4.json
├── ...
└── concurrent_report.txt    # 综合对比报告
```

### 报告内容

1. **Overall Average Overhead by Concurrency**
   - 每个并发级别的总体平均开销
   - 最小/最大开销
   - 操作数量

2. **Per-Operation Overhead Trends**
   - 每个系统调用在不同并发级别下的开销变化
   - 可视化条形图

3. **Scalability Analysis**
   - 开销增长率分析
   - 可扩展性分类（优秀/良好/中等/差）
   - 趋势图

4. **Recommendations**
   - 最佳/最差并发级别
   - 生产环境使用建议

## 解读结果

### 理想情况

```
Concurrency     Avg Overhead    Min        Max        Operations
---------------------------------------------------------------
1               ✨ -0.15%      -2.40%    +0.26%     18
4               ✨ -0.12%      -2.35%    +0.30%     18
8               ✓  +0.45%      -1.80%    +0.85%     18
16              ✓  +0.82%      -1.20%    +1.45%     18
32              ✓  +1.15%      -0.50%    +2.10%     18

Scalability: ✅ Excellent - Nearly constant overhead regardless of concurrency
```

**解读**：
- 开销范围 < 1.5% → 可扩展性优秀
- 即使在 C=32 时开销也 < 2% → 适合高并发场景

### 需要关注的情况

```
Concurrency     Avg Overhead    Min        Max        Operations
---------------------------------------------------------------
1               ✨ -0.15%      -2.40%    +0.26%     18
4               ✓  +1.20%      -1.50%    +2.80%     18
8               ⚠️ +3.45%      -0.30%    +5.20%     18
16              ⚠️ +6.80%      +1.50%    +9.30%     18
32              ⚠️ +12.50%     +4.20%    +15.80%    18

Scalability: ❌ Poor - Significant overhead growth with concurrency
```

**解读**：
- 开销随并发级别显著增长
- 可能原因：
  - eBPF map 竞争（多个 CPU 访问同一个 map）
  - 自旋锁开销
  - 缓存失效
- 建议：检查 eBPF 程序中的锁竞争和 map 访问模式

## 真实场景模拟

### Web 服务器

```bash
# 模拟 Nginx/Apache (通常 8-32 worker 进程)
BENCH_CONCURRENCY="8 16 32" BENCH_CYCLES=50 ./scripts/benchmark-concurrent.sh
```

### 数据库

```bash
# 模拟 PostgreSQL/MySQL (通常 16-128 并发连接)
BENCH_CONCURRENCY="16 32 64 128" BENCH_CYCLES=30 ./scripts/benchmark-concurrent.sh
```

### 容器编排

```bash
# 模拟 Kubernetes 节点 (大量并发 Pod)
BENCH_CONCURRENCY="32 64 128 256" BENCH_CYCLES=20 ./scripts/benchmark-concurrent.sh
```

### AI Agent 运行时

```bash
# 模拟多个 AI agent 并发执行工具调用
BENCH_CONCURRENCY="4 8 16" BENCH_CYCLES=100 ./scripts/benchmark-concurrent.sh
```

## 常见问题

### Q: 为什么并发越高，开销反而降低？

A: 这是测量噪声和缓存效应。真实开销应该：
- 保持相对稳定（理想情况）
- 或轻微增长（正常情况）

如果看到"负开销增长"，说明：
1. 测量精度不足以检测真实开销
2. CPU 缓存在高并发下更高效
3. 实际开销确实很低（< 1%）

### Q: 如何判断 eBPF 实现是否适合生产？

A: 看以下指标：
1. **平均开销 < 3%**（所有并发级别）
2. **开销范围 < 5%**（C=1 到 C=max 的差异）
3. **最大开销 < 10%**（最坏情况）

全部满足 → ✅ 适合生产

### Q: 单个系统调用显示高开销怎么办？

A: 检查：
1. 该系统调用是否真的需要追踪？
2. eBPF 程序是否对该调用做了复杂处理？
3. 是否可以通过 tail call 或其他优化减少开销？

## 与 CI/CD 集成

`.github/workflows/benchmark.yml` 已支持手动触发并发测试：

- `test_mode=concurrent`：启用并发基准测试
- `use_matrix=true`：将每个并发级别拆成独立 matrix job 并行运行
- `concurrency_matrix='["1","4","8","16","32"]'`：matrix 模式下的并发级别列表
- `matrix_max_parallel=5`：最多同时运行的 matrix job 数
- `cycles=10`：每个并发级别的 cycle 数

matrix 模式下，每个 job 只测试一个 concurrency level，并在同一 job 内完成该 level 的 baseline 和 eBPF 对比，避免把 `C=16` 的 eBPF 与其他 batch 的 baseline 混合比较。所有 matrix job 完成后，`combine-concurrent-results` 会下载各 job 的 `summary_c*.json` 并生成合并报告。

也可以在自定义 workflow 中直接运行脚本：

```yaml
- name: Run concurrent benchmark
  run: |
    BENCH_CONCURRENCY="1 4 8 16" BENCH_CYCLES=10 ./scripts/benchmark-concurrent.sh
  
- name: Check performance regression
  run: |
    LATEST=$(ls -td reports/ebpf-concurrent-* | head -1)
    python3 scripts/check-regression.py "$LATEST/concurrent_report.txt"
```

## 参考

- 单线程测试：`./scripts/benchmark-extended-full.sh`
- 基准测试工具：`./scripts/benchmark-syscalls-extended`
- 可视化工具：`./scripts/visualize-concurrent.py`
