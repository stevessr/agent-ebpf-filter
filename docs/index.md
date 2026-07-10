---
layout: home

hero:
  name: Agent eBPF Filter
  text: AI Agent 的 Linux 观测与控制平面
  tagline: eBPF 事实采集、Go 后端控制、Vue 工作台、Wrapper / Hooks / Adapters 语义关联、cgroup / BPF LSM 内核阻断。
  actions:
    - theme: brand
      text: 快速开始
      link: /guide/quick-start
    - theme: alt
      text: 架构总览
      link: /architecture/overview
    - theme: alt
      text: 安全模型
      link: /security/model
    - theme: alt
      text: 研究工作流
      link: /guide/agent-security-research-workflow

features:
  - title: 内核事实采集
    details: 通过 eBPF tracepoint、ringbuf、pinned maps 记录 exec/open/connect/sendto/recvfrom/bind/ioctl 等系统调用事实。
  - title: Agent 语义关联
    details: 通过 PID registration、native hooks、wrapper request、trace_id、tool_call_id 和 EventEnvelope 把 Agent 意图与 OS 行为合并。
  - title: 用户态 + 内核态控制
    details: wrapper 提供 ALLOW/BLOCK/ALERT/REWRITE 命令策略，cgroup 与 BPF LSM 提供网络、文件和执行阻断。
  - title: 可视化工作台
    details: Dashboard、Network、Execution Graph、AgentSight、Config、Executor、Hooks、ML 和 Plugins 组成统一观察与配置界面。
  - title: 安全默认值
    details: release-mode auth、runtime feature gates、脱敏、危险能力默认关闭，避免把诊断能力变成默认数据暴露。
  - title: 可交付文档体系
    details: 本站按指南、架构、后端、前端、安全、集成、运维、答辩和参考组织，可直接作为项目网站。
---

## 项目定位

Agent eBPF Filter 是面向 Linux 本地工作站、实验节点和开发环境的 **AI Agent 行为观测与安全控制系统**。它并不只是一组 syscall 日志，也不是单一的命令拦截器，而是把内核态事实、用户态语义、Web 控制面和可选内核阻断组合成一条完整证据链。

核心问题是：当 AI Agent 或开发者 CLI 在本机执行任务时，系统如何回答以下问题？

- 它实际执行了什么命令？
- 它打开、修改或删除了哪些文件？
- 它连接了哪些网络目标？
- 这些行为属于哪个 Agent run、哪个 tool call、哪个 trace？
- 是否触发了 wrapper / cgroup / LSM / ML / semantic policy？
- 高风险诊断数据是否经过脱敏？
- 这些证据能否被查询、导出、回放和用于答辩？

本站文档按“能直接用于项目网站”的粒度组织，既服务新开发者，也服务比赛答辩、安全审查和后续维护。

## 推荐阅读

::: tip 新开发者
从 [项目是什么](/guide/what-is-agent-ebpf-filter) 和 [快速开始](/guide/quick-start) 开始，再进入 [总体架构](/architecture/overview) 与 [代码入口索引](/reference/code-entrypoints)。
:::

::: tip 安全审查
优先阅读 [安全模型](/security/model)、[策略语义](/security/policy-semantics)、[Runtime Gates 与 Auth](/security/runtime-gates-auth)。
:::

::: tip 答辩准备
优先阅读 [比赛答辩主线](/delivery/competition-defense)、[演示脚本](/delivery/demo-script)、[评测报告](/delivery/evaluation)。
:::

## 致谢

本项目在架构设计和技术选型上受到 [AgentSight](https://github.com/eunomia-bpf/agentsight) 项目的启发。AgentSight 是由 eunomia-bpf 团队开发的开源 AI Agent 系统级追踪工具，验证了 eBPF + TLS capture 对 Agent 观测的可行性。

Agent eBPF Filter 在以下方面进行了扩展和差异化：
- **技术栈**: Go + Vue 3（vs Rust + Next.js）
- **产品定位**: 观测 + 控制（vs 纯观测）
- **控制能力**: wrapper / cgroup / BPF LSM enforcement
- **安全模型**: TLS capture 默认关闭，作为高风险诊断能力
- **目标场景**: 包含操作系统课程答辩交付

详见 [AgentSight 项目致敬](/reference/agentsight-acknowledgment)。

感谢 AgentSight 项目对开源社区的贡献！
