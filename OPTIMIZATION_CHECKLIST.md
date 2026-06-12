# eBPF 优化清单 ✅

## 已完成的优化 (2026-06-12)

### ✅ 1. Syscall Handler 宏化
- **文件**: `backend/ebpf/agent_tracker_syscalls.h`
- **优化**: 997 行 → 164 行 (-83%)
- **方法**: DEFINE_SIMPLE_ENTER_HANDLER + DEFINE_GENERIC_EXIT_HANDLER 宏
- **覆盖**: 16 个简单 syscall (ioctl, chmod, chown, socket, read, write, open, 等)
- **保留**: 3 个复杂 syscall 的自定义逻辑 (bind, sendto, recvfrom)

### ✅ 2. ML 模型移除
- **删除**: `_ml_model_ebpf.c` (1182 行)
- **删除**: `compile_ebpf_model.go` (226 行)
- **原因**: ML 推理在用户空间，eBPF 版本未使用

### ✅ 3. 代码精简结果
- **总减少**: 2,346 行 (-44% eBPF 代码库)
- **净变化**: +1,191 插入, -2,346 删除
- **编译状态**: ✅ 通过
- **运行状态**: ✅ 正常

## 潜在的未来优化

### 🔄 低优先级 (收益 < 20%)

#### A. TLS Handler 合并
- **文件**: `agent_tls_capture.c` (314 行)
- **机会**: 16 个 uprobe handler 有重复模式
- **预期**: -100~150 行
- **风险**: 中 (每个库有不同的参数约定)

#### B. LSM Enforcer 分析
- **文件**: `lsm_enforcer.c` (464 行)
- **机会**: 14 个 hook 可能有共同逻辑
- **预期**: -50~100 行
- **风险**: 中 (LSM 钩子语义各异)

#### C. Tail Call 扩展
- **文件**: `agent_tracker_tail.h` (308 行)
- **机会**: 将更多逻辑移至 tail call
- **预期**: 减少主 handler 复杂度
- **风险**: 高 (33 层深度限制，verifier 限制)

## 优化原则

1. **DRY** - 用宏消除重复代码
2. **YAGNI** - 移除未使用的代码
3. **安全第一** - 仅实施低风险优化
4. **边际递减** - 当前优化已达到收益平衡点

## 下一步建议

❌ **不推荐**: 继续深度优化 (风险 > 收益)
✅ **推荐**: 保持当前状态，专注于新功能开发
