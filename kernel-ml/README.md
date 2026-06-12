# Kernel ML Inference Module

内核态机器学习推理引擎 - 用于实时行为分类的 DKMS 内核模块。

## 架构

```
┌─────────────────────────────────────────┐
│  Userspace (Agent eBPF Filter)          │
│  - 训练模型 (RandomForest)               │
│  - 导出为二进制格式                      │
└─────────────┬───────────────────────────┘
              │ /proc/ml_load
              ↓
┌─────────────────────────────────────────┐
│  Kernel Module (kernel_ml.ko)           │
│  - 加载决策树模型                        │
│  - 定点数推理引擎                        │
│  - O(log N) 树遍历                       │
└─────────────┬───────────────────────────┘
              │ /proc/ml_predict
              ↓
┌─────────────────────────────────────────┐
│  Classification Result                   │
│  - ALLOW (0)                             │
│  - BLOCK (1)                             │
│  - ALERT (2)                             │
└─────────────────────────────────────────┘
```

## 核心特性

### 1. 定点数运算
- **无浮点**: 内核禁止 FPU，使用整数运算
- **精度**: 1000x 缩放 (0.001 分辨率)
- **性能**: 纯整数比较，CPU 缓存友好

### 2. 决策树推理
- **算法**: Random Forest (15 棵树)
- **复杂度**: O(log N) 每棵树
- **可解释性**: 比神经网络更透明

### 3. Proc 接口
- `/proc/ml_load` - 加载模型 (write-only)
- `/proc/ml_predict` - 推理请求 (write-only)
- `/proc/ml_stats` - 统计信息 (read-only)

## 编译

```bash
# 方法 1: 直接编译
cd kernel-ml
make CC=clang LD=ld.lld

# 方法 2: DKMS 安装 (推荐)
sudo dkms add .
sudo dkms build kernel-ml/1.0
sudo dkms install kernel-ml/1.0
```

**注意**: 路径不能包含空格（内核构建系统限制）

## 使用

### 1. 加载模块
```bash
sudo insmod kernel_ml.ko
dmesg | tail -5  # 查看加载信息
```

### 2. 训练并导出模型
```python
from sklearn.ensemble import RandomForestClassifier
import pickle

# 训练模型
model = RandomForestClassifier(n_estimators=15, max_depth=7)
model.fit(X_train, y_train)

# 保存
with open('model.pkl', 'wb') as f:
    pickle.dump(model, f)

# 转换为内核格式
python model_loader.py model.pkl model.bin
```

### 3. 加载模型到内核
```bash
cat model.bin > /proc/ml_load
cat /proc/ml_stats  # 验证加载成功
```

### 4. 推理
```c
#include <fcntl.h>
#include <unistd.h>
#include "ml_inference.h"

struct feature_vector fv;
extract_features(&fv, syscall_nr, pid, comm, args);

int fd = open("/proc/ml_predict", O_WRONLY);
write(fd, &fv, sizeof(fv));
close(fd);

// 查看 dmesg 获取结果
```

## 性能

- **推理延迟**: ~5-10 μs (15 棵深度 7 的树)
- **内存占用**: ~300 KB 模块 + ~50 KB 模型
- **吞吐量**: >100k 推理/秒 (单核)

## 限制

1. **特征维度**: 固定 128 维
2. **树数量**: 最多 15 棵
3. **树深度**: 建议 ≤ 10 (verifier 限制)
4. **模型大小**: ≤ 10 MB

## DKMS 配置

`dkms.conf`:
```ini
PACKAGE_NAME="kernel-ml"
PACKAGE_VERSION="1.0"
CLEAN="make clean"
MAKE[0]="make all KVERSION=$kernelver CC=clang LD=ld.lld"
BUILT_MODULE_NAME[0]="kernel_ml"
DEST_MODULE_LOCATION[0]="/extra"
AUTOINSTALL="yes"
```

## 文件结构

```
kernel-ml/
├── ml_inference.h       - 推理引擎头文件
├── ml_inference.c       - 核心推理实现
├── kernel_ml_main.c     - 模块入口 + proc 接口
├── Makefile             - 构建脚本
├── dkms.conf            - DKMS 配置
├── model_loader.py      - 模型转换工具
├── test_module.sh       - 测试脚本
└── README.md            - 本文件
```

## 与 eBPF 对比

| 特性 | 内核模块 | eBPF |
|------|---------|------|
| 复杂度 | 无限制 | Verifier 严格限制 |
| 性能 | 极高 | 高 |
| 安全性 | 需人工审计 | Verifier 保证 |
| 动态加载 | 需 root | 普通用户 (CAP_BPF) |
| 调试 | printk/kgdb | bpftool |

**推荐**: eBPF 用于数据捕获，内核模块用于复杂 ML 推理

## 安全考虑

- ✅ 无用户输入直接执行
- ✅ 模型从 proc 加载，需 root
- ✅ 推理结果仅用于日志/统计
- ⚠️  未启用内存保护（将来可添加）

## 故障排除

### 编译错误: "unrecognized emulation mode: llvm"
```bash
# 使用 LLVM 链接器
make CC=clang LD=ld.lld
```

### 浮点数错误: "__muldf3 undefined"
```bash
# 检查代码中的浮点运算
grep -r "\.0" *.c
# 替换为整数运算
```

### 路径空格错误
```bash
# 复制到无空格路径
cp -r kernel-ml /tmp/
cd /tmp/kernel-ml && make
```

## TODO

- [ ] 添加 sysfs 接口（替代 proc）
- [ ] 支持动态树数量/深度
- [ ] 实现模型版本控制
- [ ] 添加推理缓存（LRU）
- [ ] 支持多分类（当前仅 3 类）
- [ ] 集成 perf 性能分析
- [ ] 添加单元测试

## 许可证

GPL v2 - 与 Linux 内核兼容

## 作者

Agent eBPF Filter Project
