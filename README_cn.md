# Agent eBPF Filter

**面向 Linux 的 AI Agent 观测与控制平面**

[![许可证: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)
[![文档](https://img.shields.io/badge/docs-VitePress-green.svg)](docs/index.md)

> Linux 工作站上 AI Agent 行为的实时监控、语义关联与内核级强制执行。

---

## Agent eBPF Filter 是什么？

Agent eBPF Filter 结合 **eBPF 内核追踪**、**Go 后端**、**Vue.js 仪表板** 和 **多层强制执行机制**，为运行在 Linux 系统上的 AI Agent 和开发者工具提供全面的可观测性与控制能力。

**核心问题：** 当 AI Agent 或 CLI 工具在你的机器上执行时，你如何知道：
- 它实际运行了什么命令？
- 它访问、修改或删除了哪些文件？
- 它建立了哪些网络连接？
- 这些操作属于哪个 Agent 运行、工具调用或追踪？
- 敏感数据是否已正确脱敏？

**Agent eBPF Filter 提供答案。**

---

## 核心特性

### 🔍 **内核级可观测性**
- eBPF tracepoint 捕获 `execve`、`openat`、`connect`、`sendto`、`recvfrom`、`bind`、`ioctl`、`mkdir`、`unlink` 等系统调用
- 零拷贝 ringbuffer 解码，实现高性能事件采集
- 自动处理缺失的内核 tracepoint，确保兼容性

### 🎯 **语义关联**
- 通过 Python/Node.js 适配器进行 PID 注册
- 原生 Hook 支持：Claude Code、Gemini CLI、Codex、GitHub Copilot、Kiro CLI、Augment、Antigravity CLI
- 基于 Wrapper 的命令拦截
- 上下文追踪：`agent_run_id`、`tool_call_id`、`trace_id`、`cwd`、`argv_digest`

### 🛡️ **多层强制执行**
- **用户态：** `agent-wrapper` 提供 ALLOW/BLOCK/ALERT/REWRITE 决策
- **内核态 cgroup：** 在内核层面阻断网络连接（TCP/UDP、IPv4/IPv6）
- **内核态 BPF LSM：** 在 LSM hook 层面阻断文件访问和进程执行
- **基于 ML 的风险评分：** 可选的机器学习分类

### 📊 **丰富的 Web 仪表板**
- **Dashboard：** 实时事件流，strace 风格的摘要
- **Network：** 流量归因，带 DNS/SNI/HTTP 增强
- **Execution Graph：** 进程拓扑与行为追踪
- **AgentSight 集成：** 兼容 AgentSight 事件格式
- **录制/回放：** 捕获和回放执行会话

### 🔐 **安全优先设计**
- 发布模式认证，运行时访问令牌
- 四级数据脱敏（None/Basic/Standard/Strict）
- 危险功能的运行时开关
- TLS 捕获**默认禁用**（可选诊断工具）
- 自动从捕获数据中移除密钥/凭证

### 🔌 **集成就绪**
- **MCP Server：** 为 AI Agent 集成提供工具和资源
- **External API v1：** OpenAPI 文档化的 REST 端点
- **OTLP 导出：** 向可观测性平台导出 Span 遥测
- **Prometheus 指标：** 标准监控集成
- **Kubernetes：** 包含 DaemonSet 清单

---

## 快速开始

### 前置要求

- **Linux**，支持 eBPF 和 BTF
- **Go 1.26.2+**
- **Bun**（前端构建工具）
- **clang/LLVM**（eBPF 编译）
- **protoc**（Protocol Buffers 编译器）
- **sudo** 或 **pkexec**（权限提升）

### 开发模式

```bash
# 安装依赖
make predev

# 在 Zellij 会话中启动后端 + 前端
make dev
```

前端将在 `http://localhost:5173` 可用，后端 API 在 `backend/.port` 指定的端口。

### 生产构建

```bash
# 构建并运行（后端提供编译后的前端）
make run
```

### 系统服务安装

```bash
# 安装为 systemd 服务（或 rc.local 回退）
make install

# 检查状态
systemctl status agent-ebpf-filter

# 卸载
make uninstall
```

### Docker 开发容器

```bash
# 拉取 GitHub 构建的 devcontainer 镜像
make docker

# 启动或附加到特权容器 shell
make exec
```

---

## 使用示例

### 监控 Python Agent

```python
from agent_tracker import AgentTracker

tracker = AgentTracker("http://127.0.0.1:8080")
tracker.start()

# 此进程的所有系统调用现在都会被追踪
with open("/tmp/example.txt", "w") as f:
    f.write("hello from agent")
```

### 监控 Node.js Agent

```javascript
const AgentTracker = require('./agentTracker');

const tracker = new AgentTracker('http://127.0.0.1:8080');
tracker.start();

// Agent 活动现在在仪表板中可见
```

### 无需代码修改追踪命令

在 Web UI 的 **Configuration** 页面：
- 添加命令名称：`git`、`npm`、`python`、`curl` 等
- 添加精确文件路径：`/etc/passwd`、`~/.ssh/config`
- 分配标签以组织追踪的资源

### 安装 AI CLI Hook

在 **Hooks** 页面，为以下工具安装原生 hook：
- Claude Code
- Gemini CLI
- Codex
- GitHub Copilot CLI
- Kiro CLI
- Augment/Auggie CLI
- Antigravity CLI (`agy`)
- Cursor（通过 wrapper 别名）

### 阻断网络目标（内核级）

```bash
# 通过 MCP 工具或 REST API
curl -X POST http://localhost:8080/sandbox/cgroup/block-ip \
  -H "X-API-KEY: your-token" \
  -d '{"ip": "203.0.113.42"}'
```

### 阻断文件访问（BPF LSM）

```bash
# 阻断特定可执行文件
curl -X POST http://localhost:8080/sandbox/lsm/block-exec-path \
  -H "X-API-KEY: your-token" \
  -d '{"path": "/usr/bin/dangerous-tool"}'
```

---

## 架构概览

```
┌─────────────────────────────────────────────────────────────┐
│                  AI Agent / 开发者 CLI                       │
└────────┬─────────────────────────────────┬─────────────────┘
         │                                 │
         │ 系统调用                        │ HTTP 回调
         ▼                                 ▼
┌─────────────────────┐          ┌──────────────────────────┐
│   eBPF Tracepoints  │          │  原生 Hook / Wrapper     │
│     (内核态)        │          │      (用户态)            │
└──────────┬──────────┘          └────────────┬─────────────┘
           │                                  │
           │ ringbuf 事件                     │ JSON 事件
           ▼                                  ▼
    ┌────────────────────────────────────────────────┐
    │            Go 后端（特权进程）                 │
    │  • 事件归一化与增强                            │
    │  • 风险评分与策略评估                          │
    │  • cgroup/LSM map 管理                         │
    │  • 数据脱敏与归档                              │
    └────┬──────────────────────────────────────┬────┘
         │                                      │
         │ WebSocket/REST                       │ MCP/OTLP
         ▼                                      ▼
┌─────────────────────┐              ┌────────────────────────┐
│   Vue.js 仪表板     │              │      外部集成          │
│  • 实时事件         │              │  • MCP 客户端          │
│  • 网络流           │              │  • Grafana/Loki        │
│  • 执行图           │              │  • Prometheus          │
│  • 配置             │              │  • Kubernetes          │
└─────────────────────┘              └────────────────────────┘
```

详细架构请参阅 [docs/architecture/overview.md](docs/architecture/overview.md)。

---

## 文档

完整文档位于 [`docs/`](docs/) 目录，按主题组织：

| 章节 | 描述 |
|------|------|
| [**指南**](docs/guide/what-is-agent-ebpf-filter.md) | 项目介绍、快速开始、功能、阅读路径 |
| [**架构**](docs/architecture/overview.md) | 系统设计、数据流、组件交互 |
| [**后端**](docs/backend/runtime-startup.md) | 启动序列、API 路由、事件管线、ML 模型 |
| [**前端**](docs/frontend/workbench.md) | 仪表板概览、路由、组件结构 |
| [**安全**](docs/security/model.md) | 安全模型、策略语义、运行时开关 |
| [**集成**](docs/integrations/agents.md) | Agent 适配器、wrapper、hook、MCP/OTLP |
| [**运维**](docs/operations/build-and-run.md) | 构建流程、部署、验证 |
| [**交付**](docs/delivery/competition-defense.md) | 比赛答辩材料、演示脚本 |
| [**参考**](docs/reference/code-entrypoints.md) | 代码入口点、AgentSight 致谢 |

**从这里开始：**
- 新开发者：[Agent eBPF Filter 是什么？](docs/guide/what-is-agent-ebpf-filter.md)
- 安全审查：[安全模型](docs/security/model.md)
- 集成：[外部 API](docs/integrations/external-api.md)
- 部署：[Kubernetes 指南](docs/operations/kubernetes.md)

**预览文档站：**
```bash
cd docs
bun install
bun run docs:dev
```

---

## 项目结构

```
agent-ebpf-filter/
├── backend/              # Go 后端，集成 eBPF
│   ├── ebpf/             # eBPF C 程序（tracepoint、cgroup、LSM）
│   ├── app/              # HTTP/WS API、路由、权限管理
│   ├── core/             # 状态管理、配置
│   ├── redaction/        # 数据脱敏引擎
│   └── pb/               # 生成的 protobuf 绑定
├── frontend/             # Vue 3 + TypeScript 仪表板
│   ├── src/views/        # Dashboard、Network、Graph、Config 等
│   └── src/components/   # 可重用 UI 组件
├── wrapper/              # agent-wrapper 命令拦截器
├── adapters/             # Python 和 Node.js PID 注册助手
├── proto/                # Protobuf 定义（单一事实来源）
├── kernel-ml/            # 可选 DKMS 内核态 ML 模块
├── docs/                 # VitePress 文档站
├── deploy/kubernetes/    # Kubernetes 清单
└── scripts/              # 演示和验证脚本
```

---

## 配置

### 环境变量

通过 `.env.dev` 配置（由 `make dev-env` 创建）：

```bash
# 后端
AGENT_BACKEND_PORT=8080
GIN_MODE=debug

# 功能
AGENT_BUILD_FEATURES=all  # 或：core,tls_capture,ml,plugins
AGENT_RUNTIME_TLS_CAPTURE_ENABLED=false
AGENT_RUNTIME_POLICY_MANAGEMENT_ENABLED=false

# ML/LLM
AGENT_LLM_PROVIDER=openai
AGENT_LLM_MODEL=gpt-4o-mini
OPENAI_API_KEY=sk-...

# 安全
AGENT_REDACTION_LEVEL=standard  # none/basic/standard/strict
DISABLE_AUTH=true  # 仅开发环境，生产环境切勿使用
```

### 运行时配置

通过 Web UI 的 **Configuration → Runtime Config** 标签访问运行时设置：

- 认证与访问令牌
- 功能开关（shell、hook、策略管理）
- 数据保留与归档
- 脱敏级别
- TLS 捕获（默认禁用）
- 内核风险反馈
- 域名转发（80/443 代理）
- OTLP 导出

---

## MCP 集成

Agent eBPF Filter 在 `/mcp` 暴露 MCP 服务器（通过 `X-API-KEY` 或 `Authorization: Bearer` 认证）：

### 可用工具

- `tail_events` — 获取最近捕获的事件
- `query_events` — 按类型/命令/PID 搜索事件
- `get_network_flows` — 网络流摘要
- `add_tracked_command` / `add_tracked_path` — 添加追踪规则
- `block_network_destination` / `block_process_cgroup` — 内核级阻断（需要 `policyManagementEnabled`）
- `block_file_access` — BPF LSM 文件/执行阻断

### Claude Code 示例用法

项目包含三个 Claude Code skill：
- `configure-security` — 管理安全策略
- `analyze-network` — 分析网络流量
- `monitor-process` — 深度进程行为监控

---

## 验证与测试

### 无 Root 静态检查

```bash
make os-enforcement-check
```

### 特权预检

```bash
make os-enforcement-preflight
```

### 实时 OS 强制执行冒烟测试

```bash
# 以 root 启动后端，禁用认证
sudo -E env DISABLE_AUTH=true ./backend/agent-ebpf-filter &

# 运行冒烟测试
make os-enforcement-smoke
```

验证：
- BPF LSM exec/open/read-write/mmap/mprotect/setattr/create/link/symlink/unlink/mkdir/rmdir/mknod/rename 拒绝
- cgroup connect/sendmsg 阻断 IPv4/IPv6 目标和 TCP/UDP 端口

---

## 性能

- **零拷贝 ringbuffer：** 基于 mmap 的对齐解码
- **低延迟风险评分：** 广播前的用户态内核风险评估
- **可选 CUDA 加速：** 带 CUDA 助手的内核态 ML 推理
- **高效事件过滤：** 仅为已注册 PID、追踪命令或追踪路径发出事件

基准测试：
```bash
make runtime-benchmark
```

---

## 安全考虑

### 默认安全态势

✅ **默认安全：**
- TLS 捕获**默认禁用**
- 策略管理**默认禁用**
- Shell/系统命令**默认禁用**
- Hook 安装**默认禁用**
- 生产环境发布模式认证**启用**
- 四级脱敏，自动移除密钥

⚠️ **危险功能需要显式开启：**
- TLS 明文捕获（仅诊断工具）
- OS 级网络/文件阻断
- PTY 会话创建
- Hook 配置编辑

### 威胁模型

详见 [docs/security/threat-model.md](docs/security/threat-model.md) 的全面安全分析。

---

## 已知限制

1. **路径匹配是精确匹配**，而非递归子树匹配
2. **命令匹配是精确基本名**，限制为 16 字节
3. **域名转发是反向代理**，而非透明 eBPF NAT
4. **TLS 捕获** 需要为非系统库显式注册库/二进制文件
5. **cgroup 阻断** 需要 cgroup v2
6. **BPF LSM 阻断** 需要启用 BPF LSM 的内核

---

## 故障排除

### 后端启动失败

检查：
- 内核支持 eBPF + BTF：`uname -r` 和 `/sys/kernel/btf/vmlinux`
- `/sys/fs/bpf` 已挂载：`mount | grep bpf`
- `clang` 已安装：`clang --version`
- 权限提升工作：`sudo -v` 或 `pkexec --version`

### 前端无法连接后端

检查：
- `backend/.port` 文件存在且包含有效端口
- 后端正在运行：`ps aux | grep agent-ebpf-filter`
- 防火墙允许本地连接

### 原生 hook 不工作

检查：
- 目标 CLI 配置文件包含 `agent-ebpf-hook-active` 标记
- Hook 回调 URL 可从 CLI 进程访问
- `curl` 在 PATH 中可用
- 后端正在运行且认证令牌有效（如果在发布模式）

### OS 强制执行冒烟测试失败

检查：
- 后端以 root 运行
- 设置了 `DISABLE_AUTH=true` 环境变量
- cgroup v2 可用：`mount | grep cgroup2`
- BPF LSM 已启用：`cat /sys/kernel/security/lsm | grep bpf`

---

## 贡献

开发者和编码 Agent 工作流指南请参阅 [AGENTS.md](AGENTS.md)。

---

## 许可证

[GPL-3.0](LICENSE)

---

## 致谢

本项目受 [AgentSight](https://github.com/eunomia-bpf/agentsight) 启发，这是由 eunomia-bpf 团队开发的开源 AI Agent 追踪工具。AgentSight 开创了使用 eBPF 进行 Agent 可观测性的先河。

Agent eBPF Filter 在此基础上扩展：
- **Go + Vue.js** 技术栈（vs Rust + Next.js）
- **强制执行能力**（wrapper/cgroup/LSM 阻断）
- **安全优先设计**（TLS 捕获默认禁用）
- **生产就绪功能**（MCP/OTLP/Prometheus 集成）

详见 [docs/reference/agentsight-acknowledgment.md](docs/reference/agentsight-acknowledgment.md)。

---

## 支持

- **文档：** [docs/](docs/)
- **问题：** 使用 GitHub Issues 报告 bug 和功能请求
- **讨论：** 使用 GitHub Discussions 提问和交流想法

---

**❤️ 为 AI Agent 生态打造**
