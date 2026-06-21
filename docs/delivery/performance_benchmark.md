# eBPF Performance Overhead Benchmark - 交付文档

## 项目概述

本项目为 agent-ebpf-filter 添加了完整的性能基准测试框架，用于量化 eBPF hook 引入的性能开销。

## 已完成的工作

### 1. 核心基准测试工具

#### `scripts/benchmark-syscalls.go`
- **功能**: 精确测量系统调用执行时间
- **测试覆盖**: 10 种常见系统调用
  - 进程信息：getpid, getppid, gettid, getuid
  - 文件操作：open/close, stat, access, getcwd, read/write  
  - 网络操作：socket/close
- **特性**:
  - 支持多次运行和预热迭代
  - 统计分析（平均值、最小值、最大值、标准差）
  - JSON 格式输出

#### `scripts/run-benchmark-manual.sh`
- **功能**: 手动运行基准测试的脚本
- **特性**:
  - 自动检测 eBPF 是否加载
  - 智能选择运行模式（baseline 或 ebpf）
  - 自动生成对比报告
  - 支持会话保持（通过 BENCH_STAMP 环境变量）

#### `scripts/ebpf-overhead-benchmark.sh`
- **功能**: 全自动基准测试（需要 root 权限）
- **特性**:
  - 自动加载/卸载 eBPF 程序
  - 运行完整的对比测试
  - 生成 Python 报告
  - 恢复原始系统状态

#### `scripts/benchmark-quickstart.sh`
- **功能**: 交互式向导，引导用户完成测试
- **特性**:
  - 检测现有测试结果
  - 分步骤提示用户操作
  - 尝试自动启动后端（如有 passwordless sudo）
  - 清晰的进度反馈

#### `scripts/visualize-benchmark.py`
- **功能**: 可视化基准测试结果
- **特性**:
  - ASCII 条形图展示
  - 开销百分比可视化
  - 颜色编码（绿色/黄色/红色）
  - 分类统计分析
  - CSV 导出功能

### 2. 文档

#### `scripts/README_BENCHMARK.md`
- 完整的基准测试文档
- 工具使用说明
- 结果解读指南
- 故障排除
- CI/CD 集成示例

#### `BENCHMARK_STATUS.md`
- 当前测试状态
- 已完成的基线测试结果摘要
- 下一步操作指南
- 预期结果说明

### 3. Makefile 集成

添加了以下 make 目标：

```makefile
make benchmark-tool      # 构建基准测试工具
make benchmark-baseline  # 运行基线测试
make benchmark-quick     # 运行交互式向导
make benchmark-clean     # 清理测试报告
```

## 已完成的测试结果

### 基线测试（无 eBPF）

✅ 已成功完成，结果保存在：`reports/ebpf-overhead-20260621-034100/baseline.json`

**关键数据**:

| 系统调用 | 平均时间 (μs) | 变异系数 |
|----------|---------------|----------|
| getpid | 0.065 | 低 |
| getppid | 0.068 | 低 |
| gettid | 0.064 | 低 |
| open_close | 0.623 | 中 |
| stat | 0.330 | 中 |
| read_write | 5.295 | 低 |
| socket_close | 1.262 | 低 |
| getcwd | 0.940 | 中 |
| getuid | 0.135 | 中 |
| access | 0.263 | 低 |

### 待完成：eBPF 测试

由于需要 sudo 权限启动后端，eBPF 测试需要用户手动完成。

## 使用指南

### 快速开始（推荐）

```bash
# 运行交互式向导
./scripts/benchmark-quickstart.sh
```

向导会：
1. 检查现有测试结果
2. 指导运行缺失的测试
3. 提示何时启动/停止后端
4. 自动生成完整报告

### 手动运行（完整流程）

#### 步骤 1: 运行基线测试

```bash
# 确保后端未运行
./scripts/run-benchmark-manual.sh
```

记录输出的时间戳（例如：20260621-034100）

#### 步骤 2: 启动后端

在新终端中：

```bash
cd backend
sudo go run ./app
```

等待看到 "Listening on port 8080"

#### 步骤 3: 运行 eBPF 测试

在原终端中：

```bash
BENCH_STAMP=20260621-034100 ./scripts/run-benchmark-manual.sh
```

#### 步骤 4: 查看结果

```bash
# 查看 JSON 摘要
cat reports/ebpf-overhead-20260621-034100/summary.json

# 生成可视化报告
python3 scripts/visualize-benchmark.py \
    reports/ebpf-overhead-20260621-034100/summary.json

# 导出 CSV
python3 scripts/visualize-benchmark.py \
    reports/ebpf-overhead-20260621-034100/summary.json \
    --csv overhead.csv
```

### 使用 Makefile

```bash
# 构建工具
make benchmark-tool

# 运行基线测试
make benchmark-baseline

# 运行完整测试（交互式）
make benchmark-quick

# 清理旧报告
make benchmark-clean
```

## 输出文件结构

```
reports/ebpf-overhead-<timestamp>/
├── baseline.json      # 无 eBPF 的测试结果
├── ebpf.json         # 有 eBPF 的测试结果
└── summary.json      # 对比摘要（包含开销百分比）
```

## 结果解读

### 开销分类

- **<10% 开销**: 🟢 极佳，可忽略不计
- **10-30% 开销**: 🟡 良好，可接受范围
- **>30% 开销**: 🔴 需要注意

### 系统调用分类

1. **极快速调用** (<0.1μs)
   - 例如：getpid, getuid
   - 预期：20-100% 开销（绝对值仍很小）

2. **中速调用** (0.1-1μs)
   - 例如：stat, access
   - 预期：5-20% 开销

3. **慢速调用** (>1μs)
   - 例如：open, socket, read/write
   - 预期：2-10% 开销

### 平均开销预期

基于典型 eBPF tracepoint 实现：**10-30%**

## 技术细节

### 基准测试方法

1. **预热**: 2 次迭代，避免冷启动影响
2. **主测试**: 5 次运行，每次 10,000 迭代
3. **统计**: 计算平均值、最小值、最大值、标准差
4. **对比**: 计算百分比开销和绝对时间差

### 测试精度

- 时间精度：微秒级 (μs)
- 迭代次数：50,000 次/操作
- 统计样本：5 次运行

### eBPF 检测逻辑

脚本通过以下方式检测 eBPF 是否加载：

1. 检查 `/sys/fs/bpf/agent-ebpf/maps` 目录
2. 检查 `/sys/fs/bpf/agent-ebpf/links` 目录
3. 检查后端进程是否运行

## 故障排除

### 问题：需要 sudo 密码

**原因**: 启动 eBPF 后端需要 root 权限

**解决方案**:
1. 手动在另一个终端启动后端
2. 或配置 passwordless sudo

### 问题：benchmark-syscalls 未找到

**解决方案**:
```bash
cd scripts
go build -o benchmark-syscalls ./benchmark-syscalls.go
```

### 问题：后端无法启动

**检查**:
```bash
# 检查内核支持
cat /sys/kernel/btf/vmlinux | head

# 检查 bpffs
mount | grep bpf

# 检查 eBPF 后端编译
ls -la backend/ebpf/*.o
```

## 未来增强

### 建议的改进

1. **更多系统调用**: 添加 execve, clone, connect 等
2. **压力测试**: 并发测试多进程/多线程场景
3. **长时间测试**: 测试长期运行的性能衰减
4. **不同负载**: CPU 密集、I/O 密集、网络密集
5. **图形化报告**: 生成 HTML/PNG 图表
6. **CI 集成**: 自动化回归检测
7. **对比多版本**: 跟踪不同版本的性能变化

### 可选的自动化

如果配置了 passwordless sudo，可以完全自动化：

```bash
# 一键完成所有测试
sudo ./scripts/ebpf-overhead-benchmark.sh
```

## 总结

✅ **已完成**:
- 完整的基准测试框架
- 10 种系统调用的测试覆盖
- 自动化和手动测试模式
- 详细文档和使用指南
- 可视化工具
- Makefile 集成

📊 **测试状态**:
- 基线测试：✅ 已完成
- eBPF 测试：⏳ 待用户使用 sudo 完成

📖 **文档**:
- `scripts/README_BENCHMARK.md` - 完整文档
- `BENCHMARK_STATUS.md` - 当前状态
- 本文档 - 交付总结

🎯 **交付物**:
- 4 个可执行脚本
- 1 个 Go 测试工具
- 1 个 Python 可视化工具
- 3 个文档文件
- Makefile 集成

用户现在可以轻松地测量和监控 eBPF hook 的性能影响，为生产部署提供数据支持。
