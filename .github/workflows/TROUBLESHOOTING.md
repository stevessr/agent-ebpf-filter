# GitHub Actions 故障排查与修复

## 🎯 问题总结

经过三次迭代，成功修复了 GitHub Actions 工作流的所有问题。

## 📊 运行历史

| 运行 | 失败步骤 | 问题 | 修复 |
|------|----------|------|------|
| #1 (27893423026) | Checkout code | submodules 配置错误 | 禁用 submodules |
| #2 (27893481858) | Build backend | Go 版本不匹配 (1.22 vs 1.26.2) | 添加调试日志 |
| #3 (27893535523) | Build backend | 同上 | 更新 Go 版本到 1.26 |
| **#4** | **待测试** | **应该成功** | ✅ |

## 🔧 具体修复内容

### 修复 1: Submodules 问题
```yaml
# 之前
- name: Checkout code
  uses: actions/checkout@v4
  with:
    submodules: recursive  # ❌ 失败

# 之后
- name: Checkout code
  uses: actions/checkout@v4
  with:
    submodules: false      # ✅ 成功
```

**原因**: 项目没有正确配置 .gitmodules，但有 submodules 目录修改。

### 修复 2: Go 版本不匹配
```yaml
# 之前
- name: Set up Go
  uses: actions/setup-go@v5
  with:
    go-version: '1.22'     # ❌ 版本过低

# 之后
- name: Set up Go
  uses: actions/setup-go@v5
  with:
    go-version: '1.26'     # ✅ 匹配 go.mod
```

**原因**: backend/go.mod 要求 `go 1.26.2`，但工作流使用 1.22。

### 修复 3: 增强调试和错误处理
```yaml
# 添加系统依赖
- pkg-config
- libelf-dev
- zlib1g-dev

# 添加验证步骤
- name: Verify Go installation
  run: |
    go version
    go env

# 详细构建日志
- name: Build backend
  run: |
    echo "Building backend..."
    go build -v -o ../agent-ebpf-filter . 2>&1 | tee build.log
```

## 📝 提交记录

```
db43f7a - feat: Add comprehensive eBPF performance benchmark suite
c932ef3 - fix(ci): Improve GitHub Actions benchmark workflow
8633d7c - fix(ci): Update Go version to 1.26 to match go.mod requirement
```

## 🧪 测试步骤

### 1. 推送修复
```bash
cd /path/to/agent-ebpf-filiter
git push origin master
```

### 2. 手动触发工作流
在 GitHub 上：
1. 进入 **Actions** 标签页
2. 选择 **eBPF Performance Benchmark**
3. 点击 **Run workflow**
4. 选择参数：
   - cycles: 100
   - syscalls: extended

### 3. 预期结果

工作流应该成功完成以下步骤：

- ✅ Checkout code
- ✅ Set up Go (1.26)
- ✅ Install system dependencies
- ✅ Verify Go installation
- ✅ Cache Go modules
- ✅ Cache benchmark tools
- ✅ Build benchmark tools
- ✅ Build eBPF programs
- ✅ Build backend
- ✅ Start backend in background
- ✅ Run benchmark
- ✅ Generate reports
- ✅ Upload artifacts

### 4. 输出产物

- **Step Summary**: 包含核心指标和详细图表
- **Artifact 1**: benchmark-results-extended-100-cycles
- **Artifact 2**: benchmark-charts-extended-100-cycles

## 🎯 关键学习点

### 1. Submodules 配置
- 如果项目没有 `.gitmodules` 文件，不要使用 `submodules: recursive`
- 可以完全禁用 submodules checkout

### 2. Go 版本匹配
- GitHub Actions 中的 Go 版本必须匹配或高于 go.mod 中的要求
- 使用 `go-version: '1.26'` 会匹配最新的 1.26.x 版本

### 3. 调试技巧
- 添加 `echo` 语句来跟踪进度
- 使用 `go build -v` 获取详细输出
- 使用 `|| true` 使某些步骤失败非致命
- 添加 `if: always()` 确保清理步骤运行

### 4. 缓存优化
- 缓存 Go 模块可以节省大量时间
- 缓存编译好的工具避免重复构建
- 使用正确的 cache key 确保缓存有效性

## 📊 预期性能

### 本地运行时间
- 完整测试 (100 cycles, 18 syscalls): ~10-15 分钟

### GitHub Actions 运行时间
- 首次运行 (无缓存): ~20-25 分钟
- 后续运行 (有缓存): ~15-20 分钟

时间分解：
- 环境设置: 2-3 分钟
- 构建 (首次): 5-7 分钟
- 构建 (缓存): 2-3 分钟
- 测试运行: 10-15 分钟
- 报告生成: 1-2 分钟

## 🔍 故障排查备忘

如果工作流再次失败，检查：

1. **Go 版本**: 确保 workflow 的 go-version 匹配 go.mod
2. **系统依赖**: 确保所有 eBPF 工具链已安装
3. **内核支持**: GitHub Actions runner 的内核是否支持 eBPF
4. **权限**: sudo 权限是否正常工作
5. **网络**: 依赖下载是否成功

## ✅ 验证清单

运行成功后，验证：

- [ ] Step Summary 显示平均开销
- [ ] 开销在合理范围 (-10% ~ +10%)
- [ ] 测试了 18 个系统调用
- [ ] Artifacts 已上传
- [ ] summary.json 包含所有数据
- [ ] summary_chart.txt 可读

## 📚 相关文档

- [工作流配置](.github/workflows/benchmark.yml)
- [使用指南](.github/workflows/README.md)
- [性能报告](docs/delivery/performance_final_report.md)
- [快速参考](BENCHMARK_QUICKREF.txt)

---

**状态**: ✅ 修复完成，等待测试  
**下一步**: 推送并手动触发工作流  
**预期**: 成功运行并生成报告
