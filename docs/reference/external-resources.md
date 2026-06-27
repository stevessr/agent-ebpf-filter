# 外部资源与最佳实践

本页面汇总 eBPF、AI Agent 监控、安全控制等领域的优秀项目、文档和最佳实践，为深入学习提供参考。

## ### AgentSight

**系统级 AI Agent 追踪与监控**

- **GitHub**: https://github.com/eunomia-bpf/agentsight
- **论文**: [arXiv:2508.02736](https://arxiv.org/abs/2508.02736)
- **许可证**: MIT
- **技术栈**: Rust + eBPF + Next.js
- **核心特性**: 零插桩、TLS 捕获、系统级可观测性

**学习价值**:
- eBPF + TLS uprobe 技术组合
- HTTP/SSE 解析算法
- 系统级 Agent 可观测性理念
- Rust 框架设计模式

**本地文档**: `docs/ref/agentsight/`

**我们的改进**:
- Go 生态重写（更简单的部署）
- 扩展控制能力（wrapper/cgroup/LSM）
- TLS capture 默认关闭（安全优先）

### Cilium

**eBPF 驱动的云原生网络、安全和可观测性**

- **GitHub**: https://github.com/cilium/cilium
- **官网**: https://cilium.io/
- **许可证**: Apache 2.0

**学习价值**:
- 生产级 eBPF 应用
- 网络策略 enforcement
- Hubble 可观测性
- cilium/ebpf Go 库（我们使用的核心依赖）

**相关技术**:
- eBPF maps 管理
- XDP 数据平面
- BPF LSM 安全钩子
- Cilium Tetragon - 运行时安全观测

### Falco

**云原生运行时安全**

- **GitHub**: https://github.com/falcosecurity/falco
- **官网**: https://falco.org/
- **许可证**: Apache 2.0

**学习价值**:
- 规则引擎设计
- 威胁检测模式
- Syscall 追踪
- 运行时策略 enforcement

**与我们的区别**:
- Falco: 通用容器/主机安全
- Agent eBPF Filter: AI Agent 特化

### BPFTrace

**eBPF 高级追踪工具**

- **GitHub**: https://github.com/iovisor/bpftrace
- **文档**: https://bpftrace.org/

**学习价值**:
- eBPF 程序动态生成
- 脚本语言设计
- 追踪点使用模式

### Tracee

**Linux 运行时安全与取证**

- **GitHub**: https://github.com/aquasecurity/tracee
- **官网**: https://aquasec.com/tracee/

**学习价值**:
- 安全事件检测
- 签名规则系统
- 取证数据收集

## eBPF 学习资源

### **eBPF.io**:
- 官网：https://ebpf.io/
- 本地镜像：`docs/ref/ebpf-docs/`
- 内容：入门指南、程序类型、Map 类型、Helper 函数

**Kernel 文档**:
- BPF: https://docs.kernel.org/bpf/
- LSM BPF: https://docs.kernel.org/bpf/prog_lsm.html
- Cgroup BPF: https://docs.kernel.org/bpf/prog_cgroup_sysctl.html

### **《BPF Performance Tools》** - Brendan Gregg
- 系统性能分析
- eBPF 工具集
- 生产环境实践

**《Linux Observability with BPF》** - David Calavera, Lorenzo Fontana
- eBPF 基础
- libbpf 编程
- 可观测性模式

### **Linux Foundation**:
- eBPF Fundamentals
- Advanced BPF Programming

**Isovalent Academy** (Cilium 团队):
- eBPF 免费课程
- 实践练习

## AI Agent 安全与治理

### LangSmith (LangChain)

- **官网**: https://docs.langchain.com/langsmith/
- **定位**: 应用级 LLM 可观测性
- **特点**: Traces、Datasets、Evaluations

**与我们的区别**:
- LangSmith: SDK 集成，应用内部
- Agent eBPF Filter: 系统级，无需 SDK

### Langfuse

- **GitHub**: https://github.com/langfuse/langfuse
- **官网**: https://langfuse.com/
- **许可证**: MIT

**特点**: 开源、Traces、Prompt 管理、Evaluation

### Phoenix (Arize AI)

- **GitHub**: https://github.com/Arize-ai/phoenix
- **官网**: https://arize.com/docs/phoenix/

**特点**: ML 可观测性、Trace 分析

### Agent Protocol

- **GitHub**: https://github.com/AI-Engineer-Foundation/agent-protocol
- **规范**: https://agentprotocol.ai/

**价值**: Agent 标准化接口

## ### OWASP Top 10 for LLM Applications

- **官网**: https://owasp.org/www-project-top-10-for-large-language-model-applications/
- **内容**: LLM 应用安全风险

**相关风险**:
- Prompt Injection
- Insecure Output Handling
- Excessive Agency
- Supply Chain Vulnerabilities

### NIST AI Risk Management Framework

- **官网**: https://www.nist.gov/itl/ai-risk-management-framework

### CIS Benchmarks

- **官网**: https://www.cisecurity.org/cis-benchmarks

## ### eBPF 技术

**Brendan Gregg's Blog**:
- http://www.brendangregg.com/blog/

**Cilium Blog**:
- https://cilium.io/blog/

**内容**: eBPF 深度技术、性能分析

### AI Agent 安全

**Trail of Bits**:
- AI 安全研究
- LLM 漏洞分析

**Google DeepMind**:
- AI 安全论文
- Agent 行为研究

## ### - **perf**: Linux 性能分析工具
- **flamegraph**: 火焰图生成
- **bcc**: BPF Compiler Collection

### - **Prometheus**: 指标收集（我们已集成）
- **Grafana**: 可视化
- **OpenTelemetry**: 追踪标准（我们已支持）

### - **Tetragon**: eBPF 运行时安全（Cilium 项目）
- **Kubearmor**: Kubernetes 安全策略

## ### - **eBPF Summit**: 年度 eBPF 大会
- **KubeCon + CloudNativeCon**: CNCF 旗舰会议
- **Black Hat / DEF CON**: 安全会议

### - **eBPF Slack**: https://ebpf.io/slack
- **Cilium Slack**: https://cilium.io/slack
- **LangChain Discord**: AI Agent 开发社区

## ### eBPF 编程

**从我们的代码学习**:
- `backend/ebpf/agent_tracker.c` - 多 syscall 追踪
- `backend/ebpf/cgroup_sandbox.c` - cgroup 网络控制
- `backend/ebpf/lsm_enforcer.c` - LSM 文件控制

**外部参考**:
- Cilium/ebpf examples: https://github.com/cilium/ebpf/tree/main/examples
- libbpf-bootstrap: https://github.com/libbpf/libbpf-bootstrap

### Go + eBPF

**参考项目**:
- Cilium (大规模生产)
- Tetragon (安全监控)
- Hubble (网络可观测性)

**我们的模式**:
- cilium/ebpf 库使用
- Map pinning 管理
- Ringbuf 零拷贝解码

### Vue 3 + TypeScript

**参考项目**:
- Vite 官方示例
- Ant Design Vue Pro
- Element Plus Admin

**我们的模式**:
- Composition API + `<script setup>`
- Composables 分层
- Protobuf 集成

### **参考标准**:
- OWASP ASVS (Application Security Verification Standard)
- CWE Top 25 (Common Weakness Enumeration)
- MITRE ATT&CK (Agent 相关 TTPs)

**我们的实践**:
- 最小权限原则
- 默认安全配置
- 多层防御（wrapper + cgroup + LSM）
- 数据脱敏分级

## ### 1. **eBPF 基础**: 
   - 阅读 ebpf.io 入门指南
   - 运行 bcc 工具示例
   - 理解 Map 和 Helper 函数

2. **Go 编程**:
   - 学习 Go 基础语法
   - 理解 goroutine 和 channel
   - 学习 cilium/ebpf 库

3. **前端基础**:
   - Vue 3 官方教程
   - Composition API 文档
   - TypeScript 基础

### 1. **深入 eBPF**:
   - CO-RE (Compile Once, Run Everywhere)
   - BTF (BPF Type Format)
   - 性能优化技巧

2. **系统设计**:
   - Protobuf 协议设计
   - WebSocket 实时通信
   - 事件驱动架构

3. **安全实践**:
   - LSM hook 点选择
   - cgroup 策略设计
   - 脱敏算法实现

### 1. **论文阅读**:
   - AgentSight 论文
   - eBPF 相关学术论文
   - AI 安全研究

2. **性能优化**:
   - 零拷贝技术
   - 内存管理优化
   - 并发模型设计

3. **生产部署**:
   - Kubernetes 集成
   - 监控告警
   - 故障恢复

## 我们从开源社区学到很多，也鼓励贡献回馈：

- **Bug 报告**: GitHub Issues
- **功能建议**: Discussions
- **代码贡献**: Pull Requests
- **文档改进**: 文档站 PR
- **经验分享**: 技术博客、会议演讲

## **订阅资源**:
- eBPF Newsletter
- Cilium Newsletter
- LangChain Blog
- 相关项目 GitHub Releases

**定期检查**:
- 依赖更新
- 安全公告
- 最佳实践演进

---

**本页面持续更新**

有新的优秀资源推荐？欢迎提 PR 或 Issue！

---

## - [文档地图](documentation-map.md)
- [合规披露](../delivery/compliance.md)
- [技术对比与差异化](technical-comparison.md)
- [AgentSight 项目致谢](agentsight-acknowledgment.md)
