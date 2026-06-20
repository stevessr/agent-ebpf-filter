# 答辩演示脚本

> 项目：Agent eBPF Filter  
> 目标：为操作系统设计赛答辩准备可重复、可解释、可兜底的现场演示流程。  
> 关联文档：`docs/os-competition-defense.md`、`docs/project-structure-deep-dive.md`、`docs/project-roadmap.md`、`docs/evaluation-report.md`。  
> 安全提醒：涉及 root、eBPF attach、cgroup / BPF LSM enforcement、hook 安装、TLS capture、domain forward、命令执行等操作时，必须在授权环境中进行。

---

## 1. 演示目标

本演示要向评委证明：

1. 项目能在 Linux 上通过 eBPF 观察 AI Agent / CLI 的真实 OS 行为；
2. 项目能把 syscall、网络、文件、进程、hook 语义统一到可视化证据链；
3. 项目不仅能观测，还能通过 wrapper、cgroup、BPF LSM 实施策略控制；
4. 项目具备录制、回放、benchmark、合规披露和工程化交付能力；
5. 项目清楚区分安全基线与高风险诊断能力，不夸大安全边界。

---

## 2. 演示环境准备

### 2.1 推荐环境

| 项目 | 建议 |
| --- | --- |
| OS | Linux，启用 BTF 的现代内核 |
| 权限 | 可使用 root / sudo，或 privileged devcontainer |
| cgroup | cgroup v2 可用 |
| BPF LSM | 内核启用 BPF LSM；若不可用，准备 replay 兜底 |
| 前端工具 | Bun / Node.js 可用 |
| 后端工具 | Go、clang、llvm、bpftool 或相关 eBPF 构建依赖 |
| 浏览器 | Chrome / Chromium / Firefox 均可 |
| 网络 | 尽量准备本地 loopback 演示，避免依赖外网 |

### 2.2 启动前检查

```bash
make predev
make os-enforcement-preflight
make os-enforcement-check
```

预期：

- `make predev` 完成开发依赖检查；
- `make os-enforcement-preflight` 给出当前机器是否适合 live kernel deny validation；
- `make os-enforcement-check` 完成 rootless static check。

如果 preflight 显示 BPF LSM 或 cgroup 条件不足：

- 仍可演示 Dashboard、Network、Execution Graph、AgentSight、wrapper、runtime replay；
- OS enforcement live demo 改用录屏或 replay 兜底。

### 2.3 启动项目

开发模式：

```bash
make dev
```

或分开启动：

```bash
make dev-backend
make dev-frontend
```

生产 / 后端-only 演示：

```bash
make backend
sudo -E ./backend/agent-ebpf-filter
```

说明：

- 后端会在 `8080..8089` 中选择端口，并写入 `backend/.port`；
- Vite dev proxy 会读取 `backend/.port`；
- eBPF / cgroup / LSM attach 通常需要特权；
- release mode 下需要 runtime access token。

---

## 3. 演示总流程

建议现场按 8 个环节推进，总时长 10–15 分钟。

| 环节 | 时间 | 目标 |
| --- | --- | --- |
| 1. 项目总览 | 1 分钟 | 说明 AI Agent OS 行为观测与控制问题 |
| 2. 系统启动与健康状态 | 1 分钟 | 展示 backend / frontend / eBPF collector 状态 |
| 3. 实时 syscall 观测 | 2 分钟 | 展示 exec/open/connect 等事件 |
| 4. 网络流分析 | 1–2 分钟 | 展示 process-level network flow |
| 5. Execution Graph / AgentSight | 2 分钟 | 展示进程树、时间线、行为证据链 |
| 6. cgroup 网络阻断 | 1–2 分钟 | 展示内核侧网络 deny |
| 7. BPF LSM 文件 / 执行阻断 | 1–2 分钟 | 展示内核侧文件 / exec deny |
| 8. replay / benchmark / 合规总结 | 2 分钟 | 展示可回放、可评测、可合规交付 |

---

## 4. 演示 1：系统启动与健康状态

### 4.1 操作

1. 打开前端页面；
2. 进入 Config / System Health；
3. 展示 collector health、eBPF bootstrap、runtime config、feature gates；
4. 展示 Dashboard 初始事件流。

### 4.2 讲解要点

- 后端负责加载 eBPF、读取 ringbuf、维护 runtime state；
- 前端通过 WebSocket / REST 接收事件；
- 安全敏感能力由 runtime gates 控制；
- release mode 下敏感 API 需要 runtime token。

### 4.3 预期现象

- 前端能显示 backend 在线；
- collector health 有事件计数或 attach 状态；
- runtime config 能看到 shell / policy / TLS / OTLP 等 gate。

### 4.4 兜底

如果现场 eBPF attach 失败：

- 展示 `make os-enforcement-preflight` 输出；
- 展示已有截图 / 录屏；
- 使用 JSONL replay 展示 UI 功能。

---

## 5. 演示 2：实时 syscall 观测

### 5.1 操作方式 A：普通命令触发

在终端执行安全、可控的本地命令：

```bash
pwd
ls
uname -a
python3 -c 'import os; print("demo pid", os.getpid()); open("/tmp/agent-ebpf-demo.txt", "w").write("hello")'
python3 -c 'open("/tmp/agent-ebpf-demo.txt").read(); print("read ok")'
```

### 5.2 操作方式 B：通过 Agent adapter 注册 PID

如使用 Python adapter，可准备一个短脚本：

```python
import os
import time

# 伪代码：实际按 adapters/python/README.md 引入 agent_tracker
print("agent demo pid", os.getpid())
open("/tmp/agent-ebpf-agent-demo.txt", "w").write("agent demo")
time.sleep(1)
```

### 5.3 UI 展示

在 Dashboard 中筛选：

- `execve`
- `openat`
- `connect`
- `mkdirat`
- `unlinkat`
- command / PID / path

### 5.4 讲解要点

- eBPF tracepoint 是事实来源；
- 事件通过 ringbuf 到 Go 后端，再到 WebSocket 和前端；
- tracked command / tracked path 是 exact match；
- PID registration 是 per-process，子进程通过 fork/clone lineage 和 userspace parent fallback 关联。

### 5.5 预期现象

- Dashboard 出现命令执行、文件打开等事件；
- 事件中可看到 PID、comm、path、event type、timestamp；
- 如果启用 EventEnvelope，可进一步进入图谱关联。

---

## 6. 演示 3：网络流分析

### 6.1 推荐本地网络演示

为了避免外网不稳定，建议使用本地 HTTP server：

终端 1：

```bash
python3 -m http.server 18080 --bind 127.0.0.1
```

终端 2：

```bash
curl http://127.0.0.1:18080/
```

### 6.2 UI 展示

打开：

- Network；
- Network Flow；
- Dashboard 中的 connect / sendto / recvfrom 事件。

筛选：

- `process:curl`
- `dport:18080`
- `state:` 或对应 flow state；
- PID / comm。

### 6.3 讲解要点

- 网络事件由 syscall-derived network events 和 flow aggregator 组织；
- 支持 per-process TCP / UDP flow attribution；
- 可结合 DNS / SNI / HTTP Host / ALPN 等 metadata；
- Network 页面用于从 OS 行为角度回答“Agent 连接了哪里”。

### 6.4 预期现象

- Network 页面显示 127.0.0.1:18080 相关连接；
- 能关联到 `curl` 或对应进程；
- flow detail 中显示时间、方向、端口和进程信息。

---

## 7. 演示 4：Execution Graph / AgentSight 行为证据链

### 7.1 操作

执行一个多步骤脚本：

```bash
python3 - <<'PY'
import os, subprocess, pathlib, urllib.request
p = pathlib.Path('/tmp/agent-ebpf-graph-demo')
p.mkdir(exist_ok=True)
(p / 'input.txt').write_text('demo')
subprocess.run(['sh', '-c', 'cat /tmp/agent-ebpf-graph-demo/input.txt > /tmp/agent-ebpf-graph-demo/output.txt'])
try:
    urllib.request.urlopen('http://127.0.0.1:18080/', timeout=2).read(64)
except Exception as exc:
    print('network demo skipped:', exc)
print('graph demo pid', os.getpid())
PY
```

### 7.2 UI 展示

打开：

- Execution Graph；
- 行为追踪 / AgentSight tab；
- Timeline；
- Process Tree；
- Metrics；
- Log。

### 7.3 讲解要点

- EventEnvelope 将 syscall、wrapper、hook、TLS、policy 等来源归一化；
- Execution Graph 组织 agent / process / tool / syscall / file / network / policy；
- AgentSight 兼容视图提供 Log、Timeline、Process Tree、Metrics；
- 支持导出 JSON / JSONL / CSV，便于赛后审计和复现。

### 7.4 预期现象

- 可看到 Python 进程及其子进程；
- 可看到文件创建、读取、网络连接；
- 时间线上能展示事件顺序；
- 进程树能体现父子关系。

---

## 8. 演示 5：cgroup 网络阻断

> 该演示需要 cgroup sandbox attach 成功，且 policy mutation gate 已开启。请只在授权环境中执行。

### 8.1 准备

确认 Runtime Config 中：

- `policyManagementEnabled = true`；
- cgroup sandbox status 正常；
- 当前用户有权限调用 policy API。

### 8.2 操作方式 A：通过 UI

1. 打开 Config → Security Policies；
2. 添加 destination port block，例如 `18080`；
3. 再次执行：

```bash
curl http://127.0.0.1:18080/
```

4. 查看连接失败和 counters 增加。

### 8.3 操作方式 B：通过 MCP / API

若使用 MCP 工具，可调用 `block_network_destination`，参数为 `port: 18080`。  
若使用 REST API，按当前 `/sandbox/cgroup/**` 文档和 token 调用。

### 8.4 讲解要点

- 阻断发生在 cgroup/connect 或 cgroup/sendmsg hook；
- 规则是 exact IP / IPv6 / TCP/UDP port map，不是 CIDR / range；
- kernel path 在连接完成前返回失败；
- policy map mutation 通过 authenticated backend API 进行。

### 8.5 预期现象

- `curl` 连接失败；
- UI 中 cgroup decision counters 的 blocked 数增加；
- Dashboard / policy event 可显示阻断事实。

### 8.6 兜底

如果现场 cgroup 不可用：

- 展示 `make os-enforcement-check` 静态检查结果；
- 展示 smoke 脚本内容和历史报告；
- 使用录屏或 replay 展示阻断效果。

---

## 9. 演示 6：BPF LSM 文件 / 执行阻断

> 该演示需要内核支持 BPF LSM，且 LSM enforcer attach 成功。请只在授权环境中执行。

### 9.1 文件阻断演示

准备文件：

```bash
printf 'secret demo\n' > /tmp/agent-ebpf-secret-demo.txt
cat /tmp/agent-ebpf-secret-demo.txt
```

通过 UI Config → Security Policies 添加 basename block：

```text
agent-ebpf-secret-demo.txt
```

再次读取：

```bash
cat /tmp/agent-ebpf-secret-demo.txt
```

### 9.2 执行阻断演示

准备一个临时脚本：

```bash
cat > /tmp/agent-ebpf-blocked-exec.sh <<'SH'
#!/usr/bin/env sh
echo should-not-run
SH
chmod +x /tmp/agent-ebpf-blocked-exec.sh
/tmp/agent-ebpf-blocked-exec.sh
```

通过 UI 或 API 添加 exec path / basename block：

```text
/tmp/agent-ebpf-blocked-exec.sh
```

再次执行：

```bash
/tmp/agent-ebpf-blocked-exec.sh
```

### 9.3 讲解要点

- BPF LSM 覆盖 exec、open、read/write、mmap、mprotect、setattr、create/link/symlink/delete/mkdir/rmdir/mknod/rename 等 hook；
- file-name policy 是 basename-based；
- exec policy 是 exact path 或 executable basename；
- 阻断通常表现为 `EACCES`；
- LSM 是确定性 map lookup，不把 LLM / ML 放在同步内核决策路径。

### 9.4 预期现象

- 读取被阻断文件返回 Permission denied；
- 执行被阻断脚本返回 Permission denied；
- UI status / counters / event 中能看到策略命中。

### 9.5 兜底

如果 BPF LSM 不可用：

- 展示 preflight 输出；
- 展示 `docs/security-model.md` 中的边界说明；
- 使用录屏或 smoke 报告展示。

---

## 10. 演示 7：Wrapper 策略控制

### 10.1 操作

确认 wrapper 可用：

```bash
make wrapper
./agent-wrapper --help || true
```

在 UI Config / Wrapper Rules 中配置规则，例如：

- 对某个命令返回 ALERT；
- 对某个危险命令返回 BLOCK；
- 对某个命令参数做 REWRITE。

通过 Executor 的 Remote / Wrapper terminal 运行命令。

### 10.2 讲解要点

- wrapper 是命令 shim / policy layer，不是完整 shell sandbox；
- 决策通过 UDS `/tmp/agent-ebpf.sock`；
- 支持 ALLOW / BLOCK / ALERT / REWRITE；
- UDS socket 应保持 restrictive，backend 校验 peer credentials；
- policy mutation 受 runtime gate 控制。

### 10.3 预期现象

- UI 显示 wrapper intercept event；
- BLOCK 命令不执行；
- ALERT 命令执行但产生告警；
- REWRITE 命令参数被替换后执行。

---

## 11. 演示 8：Runtime replay / Benchmark

### 11.1 操作

```bash
make runtime-benchmark
```

或：

```bash
cd backend
RUNTIME_REPLAY_OUT=../reports/runtime-replay-manual/summary.json go test ./... -run TestRuntimeReplaySuite -count=1 -v
```

### 11.2 展示内容

展示 `reports/runtime-replay-*/summary.json`，重点包括：

- scenario coverage；
- pass count；
- false positives / false negatives；
- p50 / p95 / p99 replay-event latency；
- p50 / p95 / p99 wrapper-decision latency；
- p50 / p95 / p99 first-alert / block latency；
- memory allocation delta；
- trace-correlation accuracy。

### 11.3 讲解要点

- replay suite 是 offline replay，不依赖现场 kernel attach；
- 它用于回归测试 context inheritance、semantic alert、wrapper latency、expected-vs-observed classification；
- live kernel drop rate 仍需通过 `/system/collector-health` 或 `/metrics` 验证。

---

## 12. 可选演示：kernel-ml 内核态推理

> 可选。若现场环境不适合加载 DKMS 模块，可用 README、测试输出或录屏展示。

### 12.1 编译 / 测试

```bash
cd kernel-ml
make test
make dkms-smoke
```

如果具备权限：

```bash
sudo dkms add .
sudo dkms build kernel-ml/1.1
sudo dkms install kernel-ml/1.1
sudo insmod kernel_ml.ko
```

### 12.2 展示接口

```bash
cat /sys/kernel/kernel_ml/model_info
cat /sys/kernel/kernel_ml/stats
cat /sys/kernel/kernel_ml/cache_stats
cat /sys/kernel/kernel_ml/backend
```

### 12.3 讲解要点

- 内核禁止浮点，因此使用定点数；
- Random Forest 适合可解释、低延迟推理；
- CUDA runtime 不能直接进入内核，因此采用 userspace helper；
- helper 不存在或超时时 fallback 到内核 CPU 路径。

---

## 13. 不建议现场开启的能力

除非评委明确要求且环境授权充分，否则不建议现场开启：

| 能力 | 原因 |
| --- | --- |
| TLS 明文捕获 | 高风险诊断能力，涉及敏感数据；建议用截图或脱敏样例展示 |
| domain forward 80/443 | 可能影响本机网络服务，需要 root / CAP_NET_BIND_SERVICE |
| 真实 AI CLI hook 安装 | 会修改本机 AI CLI 配置；除非已准备隔离环境 |
| 删除 / 清空持久化事件 | 不可逆操作，可能影响演示数据 |
| 外网攻击 / 扫描 / DoS 类演示 | 不符合安全边界和比赛合规 |

---

## 14. 现场问答准备

### Q1：这是不是一个完整沙箱？

不是。项目是 Agent 行为观测与控制平面，提供 wrapper、cgroup、BPF LSM 等精确策略能力，但不声称替代容器沙箱、VM 或完整 MAC 系统。

### Q2：为什么不防 root 攻击者？

root 可以卸载 BPF program、修改 map、停止服务或直接绕过用户态控制。防 root 需要可信启动、内核硬化、外部隔离等更大范围能力。

### Q3：TLS 明文捕获是否侵犯隐私？

TLS capture 默认关闭，是高风险诊断能力。安全基线依赖 syscall、网络元数据、digest、长度和脱敏事件，不依赖默认采集明文。

### Q4：ML 是否在内核同步决策路径中？

cgroup / LSM 同步阻断是确定性 map lookup。ML / LLM 可以用于风险评分、训练样本、规则建议，但不应把不确定模型放进同步内核阻断路径。

### Q5：策略匹配是不是递归路径或 CIDR？

不是。tracked command 是 16-byte exact match；tracked path 是 256-byte exact path；cgroup destination blocking 是 exact IP / IPv6 / port；LSM file policy 是 basename-based。

---

## 15. 演示前检查清单

- [ ] `make backend` 通过。
- [ ] `make frontend` 通过。
- [ ] `make wrapper` 通过。
- [ ] `make os-enforcement-preflight` 已记录。
- [ ] `make os-enforcement-check` 已记录。
- [ ] 如需 live enforcement，已确认 root / cgroup v2 / BPF LSM 可用。
- [ ] 已准备本地 HTTP server 演示，避免外网依赖。
- [ ] 已准备 JSONL replay 或录屏兜底。
- [ ] 已确认不会误删用户文件、修改真实 AI CLI 配置或开启高风险 TLS capture。
- [ ] 已打开 `docs/os-competition-defense.md`、`docs/project-structure-deep-dive.md`、`docs/evaluation-report.md` 备用说明。

---

## 16. 演示后收尾

演示结束后建议恢复环境：

- 移除临时 policy block entries；
- 删除 `/tmp/agent-ebpf-*` 临时文件；
- 停止本地 HTTP server；
- 如安装了 hook，按 UI / 文档卸载；
- 如加载了 kernel module，按需 `sudo rmmod kernel_ml`；
- 保存录屏、截图和 benchmark summary 到 `reports/` 或答辩材料目录。

---

## 相关导航

- [演示脚本](delivery/demo-script.md)
- [比赛答辩主线](delivery/competition-defense.md)
- [评测报告](delivery/evaluation.md)
- [验证、测试与 Benchmark](operations/verification-benchmark.md)
- [OS competition defense 草案](os-competition-defense.md)
