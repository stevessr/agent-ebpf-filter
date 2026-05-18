## 项目简介

  Agent eBPF Filter 是一个面向 Linux 本地开发与实验环境的 AI Agent 可观测与安全控制平台。项目通过 Go 后端 + eBPF 内核探针 + Vue 3 前端 + CLI Wrapper + 多语言适配器，实时
  追踪 AI Agent、开发者 CLI 与子进程的文件访问、进程执行、网络连接和策略命中行为，并提供可视化分析、运行时配置与内核级阻断能力。

  系统核心由特权 Go 后端加载并管理 eBPF 程序，利用 syscall tracepoint、cgroup/connect、UDP sendmsg 与 BPF LSM 等机制采集和拦截关键操作。前端仪表盘提供事件流、系统监控、执
  行拓扑、网络流量、文件浏览、命令执行、Hook 管理、ML/LLM 风险评分和运行时配置等页面，帮助开发者直观看到 Agent 正在读取什么、执行什么、连接哪里，以及哪些行为触发了安全策
  略。

  项目支持 Python、Node 等适配器进行 PID 注册，也支持通过 agent-wrapper 和原生 AI CLI Hook 接入 Claude Code、Gemini CLI、Codex、GitHub Copilot 等工具链。除观测外，系统还
  提供基于 cgroup 的精确 IP/端口阻断、基于 BPF LSM 的可执行文件与文件名策略阻断、TLS 明文片段捕获、Host/SNI 域名转发、事件记录回放、网络流富化、OTLP/Prometheus 导出和
  Kubernetes 节点部署能力。

  该项目适用于 AI Agent 安全研究、开发者工具行为审计、本地沙箱实验、企业内网工具治理、CTF/攻防演练环境以及需要对自动化编码代理进行细粒度监控与约束的场景。它的目标不是简单
  记录日志，而是构建一个可观测、可解释、可回放、可策略化控制的 AI Agent 运行时安全平面。