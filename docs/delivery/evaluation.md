# 评测报告

> 用途：记录答辩前的构建、测试、OS enforcement、runtime benchmark 验证结果。
> 状态：评测报告模板，最终提交前必须在固定环境中重新运行命令并填入真实输出。

---

## 评测环境模板

最终提交前填写：

| 项目 | 值 |
| --- | --- |
| 评测日期 | _(填写)_ |
| Git commit | _(填写)_ |
| OS 发行版 | _(填写)_ |
| Kernel version | _(填写)_ |
| CPU / Memory | _(填写)_ |
| Go version | _(填写)_ |
| Bun / Node version | _(填写)_ |
| clang / LLVM version | _(填写)_ |
| cgroup v2 | _(填写)_ |
| BPF LSM | _(填写)_ |
| BTF 可用性 | _(填写)_ |
| 是否 devcontainer | _(填写)_ |

环境信息采集命令：

```bash
git rev-parse --short HEAD
uname -a
go version
bun --version
clang --version
mount | grep cgroup
cat /sys/kernel/security/lsm 2>/dev/null || true
ls /sys/kernel/btf/vmlinux 2>/dev/null || true
```

---

## 功能验证矩阵

| 验证项 | 命令 | 预期结果 | 实际结果 |
| --- | --- | --- | --- |
| 依赖准备 | `make predev` | 无错误退出 | _(填写)_ |
| Proto 生成 | `make proto` | Go/JS/TS/Python 桩文件更新 | _(填写)_ |
| 后端编译 | `make backend` | 生成 `backend/agent-ebpf-filter` | _(填写)_ |
| 前端构建 | `cd frontend && bun run build` | `dist/` 下有完整静态资源 | _(填写)_ |
| Wrapper 编译 | `make wrapper` | 生成 `agent-wrapper` | _(填写)_ |
| 文档站构建 | `bun run docs:build` | 无构建错误，`docs/.vitepress/dist/` 存在 | _(填写)_ |
| 后端测试 | `cd backend && go test ./...` | 全部通过 | _(填写)_ |
| Wrapper 测试 | `cd wrapper && go test ./...` | 全部通过 | _(填写)_ |
| eBPF 生成 | `cd backend/ebpf && go generate` | 更新 `*_bpfel.go` / `*_bpfeb.go` | _(填写)_ |

---

## OS Enforcement 验证

### 静态检查（rootless）

```bash
make os-enforcement-check
```

预期：cgroup/LSM ELF sections 正确，Go 非特权测试通过。

| 检查项 | 结果 |
| --- | --- |
| cgroup_sandbox.o sections | _(填写)_ |
| lsm_enforcer.o sections | _(填写)_ |
| Go 测试 | _(填写)_ |

### Preflight 检查

```bash
make os-enforcement-preflight
```

记录当前机器能力：

| 检查项 | 结果 |
| --- | --- |
| bpffs 可写 | _(填写)_ |
| cgroup v2 可用 | _(填写)_ |
| BPF LSM 可用 | _(填写)_ |
| 权限方式 | root / sudo / OS_SMOKE_PRIVILEGE_CMD |
| cgroup attach path | _(填写)_ |

### Live Smoke 测试（需特权）

```bash
# 需要 root 或 sudo
make os-enforcement-smoke-start
```

验证覆盖项：

| 类别 | 测试点 | 结果 |
| --- | --- | --- |
| BPF LSM | exec-path denial | _(填写)_ |
| BPF LSM | exec-name denial | _(填写)_ |
| BPF LSM | file-open denial | _(填写)_ |
| BPF LSM | existing-fd read/write denial | _(填写)_ |
| BPF LSM | mmap denial | _(填写)_ |
| BPF LSM | mprotect denial | _(填写)_ |
| BPF LSM | setattr denial | _(填写)_ |
| BPF LSM | create / link / symlink / unlink denial | _(填写)_ |
| BPF LSM | mkdir / rmdir / mknod / rename denial | _(填写)_ |
| cgroup | PID-cgroup denial | _(填写)_ |
| cgroup | IPv4 destination denial | _(填写)_ |
| cgroup | IPv6 destination denial | _(填写)_ |
| cgroup | IPv4-mapped IPv6 denial | _(填写)_ |
| cgroup | TCP destination-port denial | _(填写)_ |
| cgroup | UDP connected-socket denial | _(填写)_ |
| cgroup | UDP sendto/sendmsg denial | _(填写)_ |

---

## Runtime Replay Benchmark

```bash
make runtime-benchmark
```

报告应包含：

| 指标 | 值 |
| --- | --- |
| 输入场景数量 | _(填写)_ |
| 总事件数 | _(填写)_ |
| p50 延迟 | _(填写)_ |
| p95 延迟 | _(填写)_ |
| p99 延迟 | _(填写)_ |
| 峰值内存 | _(填写)_ |
| 是否启用 ML | _(填写)_ |
| 是否启用 TLS | _(填写)_ |
| 是否启用 OTLP | _(填写)_ |

---

## 安全模型验证

| 验证项 | 方法 | 预期 | 结果 |
| --- | --- | --- | --- |
| Release mode auth | 不带 token 调用 `/config/tags` | 返回 401 | _(填写)_ |
| Runtime gates | 未启用 shellSessionsEnabled 时创建 PTY | 返回 403 | _(填写)_ |
| Policy gate | 未启用 policyManagementEnabled 时修改规则 | 返回 403 | _(填写)_ |
| Redaction | Standard 级别下事件中敏感路径 | 脱敏为 `~` 或 `<CONFIG>` | _(填写)_ |
| Hook auth | 发送无 secret 的 hook event | Release 模式下返回 401 | _(填写)_ |

---

## 文档站验证

```bash
bun run docs:build
```

| 检查项 | 结果 |
| --- | --- |
| VitePress 构建通过 | _(填写)_ |
| nav / sidebar 无断链 | _(填写)_ |
| 页面标题和 outline 正常 | _(填写)_ |
| 代码块高亮正常 | _(填写)_ |
| Mermaid 图表渲染正常 | _(填写)_ |

---

## 评分参考

根据功能挑战赛道评分标准，关注以下维度：

| 维度 | 本项目对应展示 |
| --- | --- |
| 操作系统功能与性能 | eBPF tracepoint、ringbuf zero-copy、内核态阻断 |
| 操作系统与硬件结合 | kernel-ml DKMS 模块、CUDA helper offload |
| 操作系统调试工具 | Dashboard、Network、Execution Graph、AgentSight |
| 操作系统安全工具 | cgroup/BPF LSM enforcement、wrapper policy |
| 工程完整性 | 构建系统、文档、测试、devcontainer、K8s 部署 |
| 可答辩展示 | 实时 UI、事件回放、策略配置、证据链展示 |

---

## 相关导航

- [验证、测试与 Benchmark](../operations/verification-benchmark.md)
- [演示脚本](demo-script.md)
- [比赛答辩主线](competition-defense.md)
- [安全模型](../security/model.md)
- [构建与运行](../operations/build-and-run.md)
