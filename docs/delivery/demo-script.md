# 演示脚本

> 目标：为操作系统设计赛答辩准备可重复、可解释、可兜底的现场演示流程。
> 安全提醒：涉及 root、eBPF attach、cgroup / BPF LSM enforcement、hook 安装等操作时，必须在授权环境中进行。

---

## 演示环境准备

### 推荐环境

| 项目 | 建议 |
| --- | --- |
| OS | Linux，启用 BTF 的现代内核 |
| 权限 | 可使用 root / sudo，或 privileged devcontainer |
| cgroup | cgroup v2 可用 |
| BPF LSM | 内核启用 BPF LSM；若不可用，准备 replay 兜底 |
| 前端工具 | Bun / Node.js |
| 后端工具 | Go、clang / LLVM |
| 浏览器 | Chrome / Chromium / Firefox |

### 启动前检查

```bash
make predev                        # 开发依赖检查
make os-enforcement-preflight      # 内核能力与权限检查
make os-enforcement-check          # rootless 静态检查
```

### 启动项目

```bash
make dev                           # 开发模式：Zellij 会话（后端 + 前端）
# 或分开启动
make dev-backend                   # 仅后端
make dev-frontend                  # 仅前端
```

后端会在 `8080..8089` 中选择端口并写入 `backend/.port`，前端 Vite dev proxy 自动读取。

---

## 演示总流程

建议现场按 8 个环节推进，总时长 **10–15 分钟**。

| 环节 | 时间 | 目标 |
| --- | --- | --- |
| 1. 项目总览 | 1 分钟 | 说明 AI Agent OS 行为观测与控制问题 |
| 2. 系统启动与健康状态 | 1 分钟 | 展示 backend / frontend / eBPF collector 状态 |
| 3. 实时 syscall 观测 | 2 分钟 | 展示 exec/open/connect 等事件 |
| 4. 网络流分析 | 1–2 分钟 | 展示 process-level network flow |
| 5. Execution Graph / AgentSight | 2 分钟 | 展示进程树、时间线、行为证据链 |
| 6. cgroup 网络阻断 | 1–2 分钟 | 展示内核侧网络 deny |
| 7. BPF LSM 文件/执行阻断 | 1–2 分钟 | 展示内核侧文件/exec deny |
| 8. replay / benchmark / 合规总结 | 2 分钟 | 展示可回放、可评测、可合规交付 |

---

## 演示 1：系统启动与健康状态

### 操作

1. 打开前端页面
2. 进入 **Config → System Health**
3. 展示 collector health、eBPF bootstrap、runtime config、feature gates

### 讲解要点

- 后端负责加载 eBPF、读取 ringbuf、维护 runtime state
- 前端通过 WebSocket / REST 接收事件
- 安全敏感能力由 runtime gates 控制
- release mode 下敏感 API 需要 runtime token

### 预期现象

- 前端显示 backend 在线
- collector health 有事件计数或 attach 状态
- runtime config 能看到 shell / policy / TLS / OTLP 等 gate

### 兜底

如果 eBPF attach 失败：展示 `make os-enforcement-preflight` 输出，使用 JSONL replay 展示 UI 功能。

---

## 演示 2：实时 syscall 观测

### 操作方式 A：普通命令触发

```bash
pwd
ls
uname -a
python3 -c 'import os; print("demo pid", os.getpid()); open("/tmp/agent-ebpf-demo.txt", "w").write("hello")'
python3 -c 'open("/tmp/agent-ebpf-demo.txt").read(); print("read ok")'
```

### 操作方式 B：通过 Agent adapter 注册 PID

```python
from agent_tracker import AgentTracker

tracker = AgentTracker("http://127.0.0.1:8080")
tracker.start()

import os
open("/tmp/agent-ebpf-demo.txt", "w").write("agent demo")
print("agent demo pid", os.getpid())
```

### UI 展示

在 Dashboard 中筛选 `execve`、`openat`、`connect`、`mkdirat`、`unlinkat`、command / PID / path。

### 讲解要点

- eBPF tracepoint 是事实来源
- 事件通过 ringbuf → Go 后端 → WebSocket → 前端
- tracked command / tracked path 是 exact match
- PID registration 是 per-process，子进程通过 fork/clone lineage 关联

---

## 演示 3：网络流分析

### 推荐本地网络演示

```bash
# 终端 1：启动本地 HTTP server
python3 -m http.server 18080 --bind 127.0.0.1

# 终端 2：发起请求
curl http://127.0.0.1:18080/
```

### UI 展示

打开 **Network** 页面，筛选 `process:curl`、`dport:18080`。

### 讲解要点

- 网络事件由 syscall-derived network events 和 flow aggregator 组织
- 支持 per-process TCP / UDP flow attribution
- 可结合 DNS / SNI / HTTP Host / ALPN 等 metadata
- Network 页面回答"Agent 连接了哪里"

---

## 演示 4：Execution Graph / AgentSight 行为证据链

### 操作

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

### UI 展示

打开 **Execution Graph → 行为追踪**，展示：

- 进程树（Process Tree）
- 时间线（Timeline）
- 执行拓扑图（Execution Graph）
- Log / Metrics 视图
- JSON / JSONL / CSV 导出

### 讲解要点

- EventEnvelope 将 syscall、wrapper、hook、TLS、policy 等来源归一化
- Execution Graph 组织 Agent → Process → Tool → Syscall → File / Network → Policy
- 支持导出，便于赛后审计和复现

---

## 演示 5：cgroup 网络阻断

::: warning 安全提醒
该演示需要 cgroup sandbox attach 成功，且 `policyManagementEnabled = true`。请只在授权环境中执行。
:::

### 操作

1. 打开 **Config → Security Policies**
2. 添加 destination port block，例如 `18080`
3. 再次执行 `curl http://127.0.0.1:18080/`
4. 查看连接失败和 counters 增加

### 讲解要点

- 阻断发生在 cgroup/connect 或 cgroup/sendmsg hook，在连接完成前返回失败
- 规则是 **exact IP / IPv6 / TCP/UDP port map**，不是 CIDR / range
- policy map mutation 通过 authenticated backend API 进行

### 兜底

如果 cgroup 不可用：展示 `make os-enforcement-check` 静态检查结果，或使用录屏 / replay。

---

## 演示 6：BPF LSM 文件/执行阻断

::: warning 安全提醒
该演示需要内核支持 BPF LSM，且 LSM enforcer attach 成功。请只在授权环境中执行。
:::

### 文件阻断演示

```bash
# 准备文件
printf 'secret demo\n' > /tmp/agent-ebpf-secret-demo.txt
cat /tmp/agent-ebpf-secret-demo.txt    # 应当成功

# 添加 LSM 规则（通过 UI 或 API）
# POST /sandbox/lsm/block-file-name {"name": "agent-ebpf-secret-demo.txt"}

cat /tmp/agent-ebpf-secret-demo.txt    # 应当返回 EACCES
```

### 执行阻断演示

```bash
# 添加 exec 规则
# POST /sandbox/lsm/block-exec-name {"name": "whoami"}

whoami    # 应当返回 Permission denied
```

### 讲解要点

- BPF LSM hook 在内核中返回 `-EACCES`
- file policy 是 **basename-based**，覆盖 open / read-write / mmap / mprotect / setattr / create / link / symlink / delete / mkdir / rmdir / mknod / rename
- exec policy 支持 exact path 和 executable basename

---

## 演示 7：录制与回放

### 操作

1. 在 **Execution Graph** 点击"开始录制"
2. 执行一些操作
3. 停止录制
4. 进入 replay 模式，重放刚才的事件
5. 展示 JSONL 导出

### 讲解要点

- 支持 file-backed 和 browser-memory 两种录制方式
- 可导出 JSON / JSONL / CSV
- runtime benchmark 可覆盖多个预定义场景

---

## 失败兜底策略

| 场景 | 兜底方案 |
| --- | --- |
| eBPF 不可用 | 展示 recorded replay |
| BPF LSM 不可用 | 展示 wrapper / cgroup 或截图 |
| TLS capture 不启用 | 强调默认关闭并展示 metadata / digest |
| 端口冲突 | 查看 `backend/.port` |
| 权限失败 | 切换 devcontainer 或 sudo/pkexec 环境 |
| 网络不通 | 使用 loopback 本地演示 |

---

## 相关导航

- [比赛答辩主线](competition-defense.md)
- [评测报告](evaluation.md)
- [第三方与 AI 使用披露](compliance.md)
- [验证、测试与 Benchmark](../operations/verification-benchmark.md)
- [构建与运行](../operations/build-and-run.md)
