# 比赛答辩主线

Agent eBPF Filter 适合作为操作系统设计赛项目，从“内核观测 + 安全控制 + 可视化工作台 + 工程交付”四条线讲述。

## 推荐叙事

1. 问题背景：AI Agent 行为不可见、不可审计、难关联；
2. 核心方案：eBPF 捕获事实，hooks/wrapper/adapters 提供语义；
3. 内核能力：tracepoint、ringbuf、cgroup、BPF LSM；
4. 后端控制面：runtime settings、auth、feature gates、EventEnvelope；
5. 前端工作台：Dashboard、Network、Execution Graph、Config；
6. 安全边界：默认关闭高风险能力，release token，redaction；
7. 演示与评测：live events、network flow、policy blocking、record/replay；
8. 工程交付：Makefile、devcontainer、Kubernetes、docs、AI usage disclosure。

## 创新点

- 将 Agent 意图和内核事实合并；
- 用户态 wrapper 与内核态 cgroup/LSM 双层控制；
- EventEnvelope / Execution Graph 证据链；
- TLS / Codex capture 作为显式高风险诊断，而非默认采集；
- ML / Plugins 作为可扩展增强层；
- 文档、benchmark、演示脚本和合规材料完整。

## 答辩中必须说清

- `tracked_paths` 是 exact path，不是递归目录；
- destination blocking 是 exact IP/port，不是 CIDR；
- LSM file policy 是 basename-based；
- TLS capture 默认关闭；
- wrapper 不是完整 sandbox；
- release mode 需要 token；
- 高风险能力需要 runtime gate。

---

## 相关导航

- [OS competition defense 草案](../os-competition-defense.md)
- [演示脚本](demo-script.md)
- [评测报告](evaluation.md)
- [第三方与 AI 使用披露](compliance.md)
- [文档地图](../reference/documentation-map.md)
