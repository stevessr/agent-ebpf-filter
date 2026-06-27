# 项目评测报告

> 项目：Agent eBPF Filter  
> 用途：记录操作系统设计赛答辩前的构建、测试、OS enforcement、runtime benchmark、AgentSight 性能、eBPF 优化和 kernel-ml 推理验证结果。  
> 状态：评测报告模板 + 初始证据汇总。最终提交前必须在固定环境中重新运行命令并填入真实输出。  
> 关联文档：`docs/demo-script.md`、`docs/project-roadmap.md`、`docs/benchmark.md`、`docs/security-model.md`。

---

## 1. 报告摘要

| 项目 | 当前状态 | 说明 |
| --- | --- | --- |
| 构建验证 | 待运行 | 需记录 `make backend`、`make frontend`、`make wrapper`、`make all` |
| proto 生成验证 | 待运行 | 需记录 `make proto` 是否更新生成物 |
| OS enforcement static check | 待运行 | 需记录 `make os-enforcement-check` |
| OS enforcement preflight | 待运行 | 需记录当前机器 cgroup / BPF LSM / 权限情况 |
| OS enforcement smoke | 待授权运行 | 需要 root / privileged backend |
| runtime replay benchmark | 待运行 | 需记录 `make runtime-benchmark` 输出 summary |
| AgentSight 性能 | 文档已有历史数据，需复测 | 历史文档称 10,000 events 下事件处理从约 450ms 降至 180ms |
| eBPF 优化 | 文档已有历史数据，需复核 | 历史文档称 eBPF 总源码行数从 4,570 减少到 2,555 |
| kernel-ml 推理 | 文档已有设计指标，需实测 | 历史文档称目标延迟约 5–10 μs，需在当前环境验证 |

结论模板：

```text
在 <OS / kernel / hardware> 环境下，项目完成了 backend/frontend/wrapper/proto 构建验证，OS enforcement 静态检查 <通过/失败>，特权 smoke <通过/跳过/失败>，runtime replay benchmark 覆盖 <N> 个场景。主要风险为 <...>。
```

---

## 2. 评测环境

最终提交前填写：

| 项目 | 值 |
| --- | --- |
| 评测日期 | TBD |
| 评测人员 | TBD |
| Git commit | TBD |
| Git branch | `master` / TBD |
| 工作区是否干净 | TBD |
| OS 发行版 | TBD |
| Kernel version | TBD |
| CPU | TBD |
| GPU | TBD |
| Memory | TBD |
| Go version | TBD |
| Bun / Node version | TBD |
| clang / llvm version | TBD |
| 是否 devcontainer | TBD |
| 是否 privileged | TBD |
| cgroup v2 | TBD |
| BPF LSM | TBD |
| BTF 可用性 | TBD |

建议采集命令：

```bash
git rev-parse --short HEAD
git status --short
uname -a
go version
bun --version
node --version
clang --version
mount | grep cgroup
cat /sys/kernel/security/lsm 2>/dev/null || true
ls /sys/kernel/btf/vmlinux 2>/dev/null || true
```

---

## 3. 构建验证

### 3.1 依赖准备

命令：

```bash
make predev
```

记录：

| 项目 | 结果 |
| --- | --- |
| 是否通过 | TBD |
| 耗时 | TBD |
| 失败原因 | TBD |
| 日志路径 | TBD |

### 3.2 Proto 生成

命令：

```bash
make proto
```

检查点：

- `backend/pb/*.pb.go` 是否生成；
- `frontend/src/pb/tracker_pb.js` / `.d.ts` 是否生成；
- `adapters/python/tracker_*_pb2.py` 是否生成；
- `adapters/js/tracker_pb.js` 是否生成；
- 是否出现非预期 diff。

结果：TBD。

### 3.3 Backend 构建 / 测试

命令：

```bash
make backend
# 或
cd backend && go test ./...
```

记录：

| 项目 | 结果 |
| --- | --- |
| `make backend` | TBD |
| `cd backend && go test ./...` | TBD |
| 失败包 | TBD |
| 关键错误 | TBD |

### 3.4 Frontend 构建

命令：

```bash
make frontend
# 或
cd frontend && bun run build
```

记录：

| 项目 | 结果 |
| --- | --- |
| TypeScript / vue-tsc | TBD |
| Vite build | TBD |
| 输出目录 | TBD |
| 失败原因 | TBD |

### 3.5 Wrapper 构建 / 测试

命令：

```bash
make wrapper
cd wrapper && go test ./...
```

记录：TBD。

### 3.6 全量构建

命令：

```bash
make all
```

记录：TBD。

---

## 4. OS Enforcement 验证

### 4.1 静态检查

命令：

```bash
make os-enforcement-check
```

用途：rootless static check 编译后的 enforcement objects 和 smoke script。

记录：

| 项目 | 结果 |
| --- | --- |
| cgroup object | TBD |
| LSM object | TBD |
| smoke script | TBD |
| 是否通过 | TBD |

### 4.2 环境预检

命令：

```bash
make os-enforcement-preflight
```

用途：判断宿主机是否适合 live kernel-deny validation。

记录：

| 检查项 | 结果 |
| --- | --- |
| root / sudo 可用 | TBD |
| cgroup v2 root | TBD |
| BTF | TBD |
| BPF LSM | TBD |
| bpffs | TBD |
| clang / bpftool | TBD |
| 结论 | TBD |

### 4.3 特权 smoke

> 需要 root 或等价特权，只在授权环境中运行。

命令：

```bash
sudo -E env DISABLE_AUTH=true ./backend/agent-ebpf-filter
make os-enforcement-smoke
```

或：

```bash
OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start
```

应验证的能力：

- BPF LSM exec-path denial.
- BPF LSM exec-name denial.
- file-open denial.
- existing-fd read/write denial.
- mmap / mprotect denial.
- ftruncate / fchmod / setattr denial.
- create / link / symlink denial.
- unlink / mkdir / rmdir / mknod / rename denial.
- cgroup PID-cgroup block.
- IPv4 / IPv6 destination block.
- IPv4-mapped IPv6 block.
- TCP destination-port block.
- UDP connected-socket connect / send block.
- UDP sendto / sendmsg destination / port block.

记录：TBD。

---

## 5. Runtime Replay Benchmark

### 5.1 命令

```bash
make runtime-benchmark
```

或：

```bash
cd backend
RUNTIME_REPLAY_OUT=../reports/runtime-replay-manual/summary.json go test ./... -run TestRuntimeReplaySuite -count=1 -v
```

### 5.2 场景范围

根据 `docs/benchmark.md`，scenario catalog 包括：

- benign flows:`git status`、`npm install`、`pip install`、`pytest`、`cargo build`、PR review read-only scans；
- malicious flows:`curl|bash`、secret read、reverse shell、workspace escape、`chmod +x` then exec, suspicious SSH, hidden network egress, lightweight fork storm.
- agentic flows: prompt-injection file exfiltration, malicious MCP tool, unexpected browser/network behavior, suspicious remote-devbox action, resource-wasting loop.

### 5.3 指标记录

| 指标 | 值 |
| --- | --- |
| 输出目录 | TBD |
| scenario count | TBD |
| pass count | TBD |
| failed count | TBD |
| false positives | TBD |
| false negatives | TBD |
| replay-event latency p50 / p95 / p99 | TBD |
| wrapper-decision latency p50 / p95 / p99 | TBD |
| first-alert latency p50 / p95 / p99 | TBD |
| first-block latency p50 / p95 / p99 | TBD |
| memory allocation delta | TBD |
| trace-correlation accuracy | TBD |

### 5.4 解释边界

Runtime replay 是 offline replay：

- replay summary 中 ringbuf drop rate 不能代表 live kernel drop；
- live kernel collection lag 仍需从 `/system/collector-health` 或 `/metrics` 验证；
- replay 适合验证 context inheritance、semantic alert generation、wrapper decision path、expected-vs-observed classification。

---

## 6. AgentSight / Execution Graph 性能评测

### 6.1 历史文档数据

`docs/agentsight-optimization-summary.md` 中记录过基于 10,000 events 测试数据的优化结果：

| 指标 | 优化前 | 优化后 | 提升 |
| --- | --- | --- | --- |
| 事件处理速度 | ~450ms | ~180ms | 约 60% 下降 |
| 过滤操作速度 | ~120ms | ~50ms | 约 58% 下降 |
| 进程树构建 | ~800ms | ~480ms | 约 40% 下降 |
| 内存占用 | ~45MB | ~32MB | 约 29% 下降 |

### 6.2 最终复测要求

最终提交前应补充：

| 项目 | 值 |
| --- | --- |
| 测试数据来源 | TBD |
| 事件数量 | TBD |
| 浏览器 | TBD |
| 前端构建模式 | dev / production，TBD |
| 事件处理耗时 | TBD |
| 过滤耗时 | TBD |
| 进程树构建耗时 | TBD |
| 内存占用 | TBD |
| 截图 / 录屏 | TBD |

### 6.3 需要验证的页面

- `frontend/src/views/execution-graph/ExecutionGraph.vue`
- `frontend/src/components/agentsight/AgentSightTracePanel.vue`
- `frontend/src/composables/agentsight/useAgentSightEvents.ts`
- `frontend/src/utils/agentsight.ts`

---

## 7. eBPF 优化评测

### 7.1 历史文档数据

`docs/ebpf-optimization-summary.md` 中记录：

| 指标 | 优化前 | 优化后 | 变化 |
| --- | --- | --- | --- |
| eBPF 总源码行数 | 4,570 | 2,555 | -2,015 (-44%) |
| `agent_tracker_syscalls.h` | 997 | 164 | -833 (-83%) |
| `_ml_model_ebpf.c` | 1,182 | 0 | -1,182 (-100%) |
| `agenttracker_bpfel.o` | 1.8 MB | 1.8 MB | 持平 |
| `agent-ebpf-filter` binary | 49.9 MB | 49.9 MB | 持平 |

历史验证：

- `make backend` 成功；
- 后端正常启动；
- 19 个 syscall tracepoint 全部附加成功。

### 7.2 最终复测命令

```bash
cd backend/ebpf && go generate
cd ../.. && make backend
make os-enforcement-check
```

记录：TBD。

---

## 8. Kernel ML 评测

### 8.1 能力范围

`kernel-ml/README.md` 与 `docs/backend/kernel-ml-implementation.md` 描述的能力包括：

- DKMS 内核模块 `kernel_ml.ko`；
- 定点数推理，无浮点；
- Random Forest v1 / v2 模型格式；
- `/proc/ml_load`、`/proc/ml_predict`、`/proc/ml_stats`；
- `/sys/kernel/kernel_ml/*`；
- `kernel` / `cuda` / `auto` backend；
- CUDA userspace helper；
- LRU exact-match cache；
- `model_generation`；
- 多分类输出。

### 8.2 测试命令

```bash
cd kernel-ml
make test
make dkms-smoke
make cuda-helper-self-test CUDA_HOME=/opt/cuda
```

如具备特权：

```bash
sudo dkms add .
sudo dkms build kernel-ml/1.1
sudo dkms install kernel-ml/1.1
sudo insmod kernel_ml.ko
cat /sys/kernel/kernel_ml/model_info
cat /sys/kernel/kernel_ml/stats
cat /sys/kernel/kernel_ml/cache_stats
```

### 8.3 指标记录

| 指标 | 值 |
| --- | --- |
| `make test` | TBD |
| `make dkms-smoke` | TBD |
| CUDA helper self-test | TBD |
| 模块加载 | TBD |
| model_generation | TBD |
| backend | TBD |
| cache hit / miss | TBD |
| 推理延迟 | TBD |
| fallback 次数 | TBD |

### 8.4 解释边界

- CUDA runtime 不能直接链接进 Linux 内核模块；本项目采用 userspace CUDA helper；
- helper 不存在、超时或 GPU 报错时 fallback 到内核 CPU 推理；
- kernel-ml 是可选扩展亮点，不是 eBPF 采集主链路的必要条件。

---

## 9. 安全与合规验证

### 9.1 Auth / Runtime Gates

需要检查：

- release mode 下 `/config/**`、`/system/**`、`/ws*`、`/metrics`、`/register`、`/unregister`、`/agentsight/**`、`/sandbox/**` 等是否受 token 保护；
- dev mode 是否按预期关闭 auth；
- shell sessions、`/system/run`、hook installation、policy mutation、TLS capture、OTLP、domain forward 是否默认关闭；
- UI 是否清楚标出危险能力。

结果：TBD。

### 9.2 Redaction

需要检查：

- Basic / Standard / Strict 脱敏级别；
- path、command args、network、credential；
- TLS / Codex ingest 中 header、query、body、token、key removal。

结果：TBD。

### 9.3 开源合规

关联文档：`docs/third-party-notices.md`。

需要检查：

- 根目录 GPL-3.0；
- 文档 / PPT CC-BY-SA 4.0；
- Go / frontend / kernel-ml 依赖许可证；
- Linux docs 快照来源；
- AI 使用披露；
- 往届作品 / 开源基础版本说明。

结果：TBD。

---

## 10. 当前未验证项

| 项目 | 原因 | 后续动作 |
| --- | --- | --- |
| 特权 OS enforcement smoke | 需要 root / BPF LSM / cgroup v2 授权环境 | 在演示机运行并记录 |
| live ringbuf drop rate | 需要真实负载 | 通过 `/system/collector-health` 和 `/metrics` 记录 |
| AgentSight 性能复测 | 需要固定数据集和浏览器环境 | 准备 10,000 events 样本 |
| kernel-ml CUDA helper | 需要 CUDA 环境 | 在有 NVIDIA GPU / CUDA Toolkit 的机器上运行 |
| 第三方许可证完整扫描 | 需要许可证扫描工具或人工核验 | 生成 `reports/licenses-*` |
| 赛事官网正式信息 | 当前环境直接抓取官网受限 | 人工打开官网补充正式链接和题号 |

---

## 11. 附录：命令清单

```bash
# 环境
uname -a
go version
bun --version
node --version
clang --version

# 构建
make predev
make proto
make backend
make frontend
make wrapper
make all

# 后端 / 前端独立验证
cd backend && go test ./...
cd frontend && bun run build
cd wrapper && go test ./...

# eBPF / OS enforcement
make ebpf-cgroup
make ebpf-lsm
make os-enforcement-preflight
make os-enforcement-check
make os-enforcement-smoke
OS_SMOKE_PRIVILEGE_CMD='sudo -E' make os-enforcement-smoke-start

# benchmark
make runtime-benchmark
cd backend && RUNTIME_REPLAY_OUT=../reports/runtime-replay-manual/summary.json go test ./... -run TestRuntimeReplaySuite -count=1 -v

# kernel-ml
cd kernel-ml && make test
cd kernel-ml && make dkms-smoke
cd kernel-ml && make cuda-helper-self-test CUDA_HOME=/opt/cuda
```

---

## 12. 最终结论模板

最终答辩前将本节替换为真实结论：

```text
本项目在 <环境> 上完成了完整构建验证，backend/frontend/wrapper/proto 均通过。OS enforcement 静态检查通过，特权 smoke 在 <环境> 中验证了 cgroup 网络阻断和 BPF LSM 文件/执行阻断。runtime replay benchmark 覆盖 <N> 个正常、恶意和 agentic 场景，核心延迟指标为 <...>。AgentSight 在 <N> events 数据集下完成可视化性能复测。kernel-ml 作为可选扩展在 <环境> 中完成 <test/dkms/cuda> 验证。

仍需注意：项目不防御 root 攻击者，不替代完整容器沙箱；TLS 明文捕获和 domain forward 属于默认关闭的高风险诊断能力；所有第三方依赖和 AI 使用记录已在文档中披露。
```

---

## - [评测报告](evaluation.md)
- [验证、测试与 Benchmark](../operations/verification-benchmark.md)
- [Benchmark](../operations/runtime-replay-benchmark.md)
- [Demo script 草案](demo-script.md)
- [OS competition defense 草案](competition-defense.md)

