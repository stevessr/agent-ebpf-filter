# 开发历史与实现记录

本目录包含项目开发过程中的实现记录、技术决策和优化总结。这些文档记录了关键特性的演进过程，适合深入了解项目实现细节的开发者。

## 🗂️ 文档分类

### 架构与设计

| 文档 | 说明 |
| --- | --- |
| [architecture.md](architecture.md) | 早期架构设计文档 |
| [codebase-implementation-map.md](codebase-implementation-map.md) | 完整代码库实现地图 |
| [project-structure-deep-dive.md](project-structure-deep-dive.md) | 项目结构深度分析 |

### AgentSight 集成

| 文档 | 说明 |
| --- | --- |
| [agentsight-optimization-guide.md](agentsight-optimization-guide.md) | AgentSight 优化指南 |
| [agentsight-optimization-summary.md](agentsight-optimization-summary.md) | 优化总结 |

### eBPF 与内核

| 文档 | 说明 |
| --- | --- |
| [ebpf-optimization-summary.md](ebpf-optimization-summary.md) | eBPF 性能优化总结 |
| [ebpf-ml-feasibility.md](ebpf-ml-feasibility.md) | eBPF ML 可行性分析 |
| [ebpf-ml-implementation.md](ebpf-ml-implementation.md) | ML 实现记录 |
| [btf-fix-guide.md](btf-fix-guide.md) | BTF 问题修复指南 |

### TLS 与 Codex 捕获

| 文档 | 说明 |
| --- | --- |
| [codex-implementation-complete.md](codex-implementation-complete.md) | Codex 捕获完整实现 |
| [codex-rustls-fix.md](codex-rustls-fix.md) | RustLS 捕获修复 |
| [codex-stripped-analysis.md](codex-stripped-analysis.md) | Stripped binary 分析 |
| [codex-workflows.md](codex-workflows.md) | Codex 工作流 |

### 安全与脱敏

| 文档 | 说明 |
| --- | --- |
| [SANITIZATION_IMPLEMENTATION_SUMMARY.md](SANITIZATION_IMPLEMENTATION_SUMMARY.md) | 脱敏实现完整总结 |

### Execution Graph

| 文档 | 说明 |
| --- | --- |
| [execution-graph-behavior-tracking-fix.md](execution-graph-behavior-tracking-fix.md) | 行为追踪修复 |

### 测试与评估

| 文档 | 说明 |
| --- | --- |
| [benchmark.md](benchmark.md) | 性能基准测试 |
| [evaluation-report.md](evaluation-report.md) | 项目评估报告 |
| [demo-script.md](demo-script.md) | 演示脚本 |

### 模型与 ML

| 文档 | 说明 |
| --- | --- |
| [all-models-complete.md](all-models-complete.md) | 所有模型实现完成 |

### 其他

| 文档 | 说明 |
| --- | --- |
| [development-timeline.md](development-timeline.md) | 开发时间线 |
| [external-api.md](external-api.md) | External API 设计 |

## 📌 与 VitePress 文档站的关系

这些开发记录是项目演进过程的原始材料，部分内容已整合到 VitePress 文档站中：

| 开发记录 | 文档站对应页面 |
| --- | --- |
| architecture.md | [总体架构](/architecture/overview) |
| ebpf-optimization-summary.md | [eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement) |
| agentsight-optimization-*.md | [AgentSight 项目致敬](/reference/agentsight-acknowledgment) |
| SANITIZATION_*.md | [脱敏与隐私](/security/redaction-privacy) |
| evaluation-report.md | [评测报告](/delivery/evaluation) |
| demo-script.md | [演示脚本](/delivery/demo-script) |

## 🔍 如何使用这些文档

### 新开发者

建议先阅读 VitePress 文档站的结构化内容：
- [项目是什么](/guide/what-is-agent-ebpf-filter)
- [快速开始](/guide/quick-start)
- [总体架构](/architecture/overview)

需要深入某个特性时，再回到对应的开发记录查看实现细节。

### 维护者

当需要：
- 理解某个优化决策的背景
- 追溯某个特性的演进过程
- 查找历史实现细节
- 调试复杂问题

可在这些开发记录中搜索关键词。

### 研究者

这些文档记录了：
- 技术选型的权衡
- 性能优化的迭代
- 问题分析与解决方案
- 实验结果与评估

适合作为技术报告、论文或答辩材料的素材来源。

## 🚀 快速导航

**想了解项目整体？**
→ 访问 [VitePress 文档站](/) 或查看 [codebase-implementation-map.md](codebase-implementation-map.md)

**想了解某个具体特性？**
→ 在上方分类表格中找到对应文档

**想了解开发历程？**
→ 查看 [development-timeline.md](development-timeline.md)

**想了解性能表现？**
→ 查看 [benchmark.md](benchmark.md) 和 [evaluation-report.md](evaluation-report.md)

**想了解安全设计？**
→ 查看 [SANITIZATION_IMPLEMENTATION_SUMMARY.md](SANITIZATION_IMPLEMENTATION_SUMMARY.md) 和 [安全模型](/security/model)

## 📝 维护建议

### 新增开发记录时

1. 在本 README 中添加链接和简短说明
2. 归类到合适的分类下
3. 如果与 VitePress 文档站有关联，注明对应页面
4. 考虑是否需要将内容整合到文档站

### 更新 VitePress 文档站时

1. 检查相关的开发记录是否需要同步更新
2. 保持两边信息一致性
3. 开发记录可以更详细，文档站应更结构化

---

**注意**: 这些开发记录是历史快照，部分内容可能已过时。以 VitePress 文档站的内容为准。
