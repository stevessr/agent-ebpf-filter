# 文档地图

本站是项目网站入口，仓库中还保留了组件 README、专项实现记录、比赛交付材料和第三方参考快照。本文档用于把这些材料串起来：先告诉读者该从哪里进入，再说明改动某个功能时要同步哪些页面。

## 本轮扫描摘要

- 扫描对象：根 README、`docs/**/*.md`（默认排除 `docs/ref/**` 参考快照）、组件 README、`kernel-ml/README.md`、devcontainer 与 dev-env 文档。
- 链接守卫：新增 [`scripts/check-doc-links.py`](../../scripts/check-doc-links.py)，用于检查仓库内 Markdown 链接、VitePress 绝对路径和指向源码的相对路径。
- 使用方式：`python3 scripts/check-doc-links.py`；该脚本不替代 `bun run docs:build`，而是补上 VitePress `ignoreDeadLinks` 对仓库外源码链接不敏感的问题。
- 权威性原则：当前代码和 [`AGENTS.md`](../../AGENTS.md) 优先；网站页负责稳定入口；历史/专项文档必须通过本页或 `DEV_DOCS_INDEX.md` 标注它与当前实现的关系。

## 网站章节

| 章节 | 用途 | 常用入口 |
| --- | --- | --- |
| Guide | 项目定位、快速开始、功能总览、阅读路线 | [项目是什么](/guide/what-is-agent-ebpf-filter)、[快速开始](/guide/quick-start)、[阅读路线](/guide/reading-paths) |
| Architecture | 总体架构、数据流、运行时边界、协议事件 | [总体架构](/architecture/overview)、[数据流](/architecture/data-flow)、[协议与事件模型](/architecture/protocol-events) |
| Backend | 启动链、路由、事件管线、eBPF、runtime settings、ML/plugins | [启动链路](/backend/runtime-startup)、[事件管线](/backend/event-pipeline)、[eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement) |
| Frontend | 工作台、路由、组件、build feature flags | [工作台总览](/frontend/workbench)、[路由与功能页](/frontend/routes-and-pages) |
| Security | 安全模型、策略语义、auth/gates、redaction | [安全模型](/security/model)、[Runtime Gates 与 Auth](/security/runtime-gates-auth)、[策略语义](/security/policy-semantics) |
| Integrations | adapters、wrapper、hooks、MCP/API/OTLP | [Agents](/integrations/agents)、[Wrapper](/integrations/wrapper)、[Native Hooks](/integrations/native-hooks)、[MCP/External API/OTLP](/integrations/mcp-external-otlp) |
| Operations | 构建运行、devcontainer、部署、验证 benchmark | [构建与运行](/operations/build-and-run)、[部署与安装](/operations/deployment)、[验证、测试与 Benchmark](/operations/verification-benchmark) |
| Delivery | 答辩、演示、评测、合规 | [比赛答辩主线](/delivery/competition-defense)、[演示脚本](/delivery/demo-script)、[评测报告](/delivery/evaluation) |
| Reference | 文档地图、关系审计、代码入口、生成文件、检查清单 | [文档关系审计](/reference/documentation-audit)、[代码入口索引](/reference/code-entrypoints)、[生成文件边界](/reference/generated-files)、[维护检查清单](/reference/maintenance-checklists) |

## 功能域互联矩阵

| 读者问题 | 先读 | 深入 / 权威页 | 验证或源码入口 |
| --- | --- | --- | --- |
| 项目整体如何工作？ | [项目是什么](/guide/what-is-agent-ebpf-filter) | [总体架构](/architecture/overview)、[数据流](/architecture/data-flow) | [代码入口索引](/reference/code-entrypoints) |
| 如何启动后端、理解自提权和端口 handoff？ | [构建与运行](/operations/build-and-run) | [后端启动链路](/backend/runtime-startup)、[运行时边界](/architecture/runtime-boundaries) | `backend/app/main.go`、`backend/.port` |
| 一条 syscall 如何变成 UI / AgentSight / OTLP 数据？ | [事件管线](/backend/event-pipeline) | [协议与事件模型](/architecture/protocol-events)、[前端工作台](/frontend/workbench)、[MCP/External API/OTLP](/integrations/mcp-external-otlp) | `proto/tracker.proto`、`backend/app/events__*.go`、`frontend/src/pb/` |
| cgroup / BPF LSM 到底拦截什么？ | [eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement) | [策略语义](/security/policy-semantics)、[安全模型](/security/model)、[Runtime Gates 与 Auth](/security/runtime-gates-auth) | `make ebpf-cgroup`、`make ebpf-lsm`、[验证页](/operations/verification-benchmark) |
| Agent、wrapper、hooks 如何把语义关联到 PID？ | [Agents、Adapters 与 PID 注册](/integrations/agents) | [Wrapper 命令策略](/integrations/wrapper)、[Native Hooks](/integrations/native-hooks)、[事件管线](/backend/event-pipeline) | `adapters/*/README.md`、`wrapper/README.md`、`backend/app/hooks__*.go` |
| release mode 为什么 401/403？ | [Runtime Gates 与 Auth](/security/runtime-gates-auth) | [Runtime Settings 与 Feature Manifest](/backend/runtime-settings-features)、[路由与 API](/backend/routes-api) | `/config/runtime`、`backend/app/runtime__helpers_auth.go` |
| ML、内核风险反馈、kernel-ml 怎样串起来？ | [ML、Plugins 与扩展能力](/backend/ml-plugins) | [ML 模型完整指南](/backend/ml-models-complete-guide)、[内核 ML README](../../kernel-ml/README.md)、[安全模型](/security/model) | `backend/app/events__kernel_risk*.go`、`backend/app/ml__*.go`、`kernel-ml/Makefile` |
| TLS / Codex capture 与脱敏边界在哪里？ | [脱敏与隐私](/security/redaction-privacy) | [TLS Quickstart](../backend/TLS_QUICKSTART.md)、[Sanitization](../sanitization.md)、[Redaction 模块](../../backend/redaction/README.md) | `backend/app/tls_*`、`backend/codex/capture/` |
| 前端路由、页面和后端 API 如何对齐？ | [前端工作台](/frontend/workbench) | [路由与功能页](/frontend/routes-and-pages)、[组件与 Composables](/frontend/components-composables)、[路由与 API](/backend/routes-api) | `frontend/src/router/index.ts`、`frontend/src/composables/` |
| Kubernetes / 外部 API / MCP 如何对外交付？ | [部署与安装](/operations/deployment) | [Kubernetes](../kubernetes.md)、[External API](../external-api.md)、[MCP/External API/OTLP](/integrations/mcp-external-otlp) | `deploy/kubernetes/`、`backend/app/api__api_external.go`、`backend/app/server__server_mcp.go` |
| 答辩或评测材料如何复用技术页？ | [比赛答辩主线](/delivery/competition-defense) | [演示脚本](/delivery/demo-script)、[评测报告](/delivery/evaluation)、[合规披露](/delivery/compliance) | [验证、测试与 Benchmark](/operations/verification-benchmark) |

## 变更影响链

| 如果你修改了…… | 必须同步检查 | 常见漏项 |
| --- | --- | --- |
| `proto/tracker.proto` 或事件字段 | [协议与事件模型](/architecture/protocol-events)、[事件管线](/backend/event-pipeline)、[生成文件边界](/reference/generated-files)、前端 `pb/` 使用点 | 只改 Go 结构，忘记前端过滤器、AgentSight / OTLP 投影和 adapters |
| 后端 route、auth 或 compatibility alias | [路由与 API](/backend/routes-api)、[Runtime Gates 与 Auth](/security/runtime-gates-auth)、[External API](../external-api.md)、[MCP/External API/OTLP](/integrations/mcp-external-otlp) | `/api/**`、`/api/v1/**`、MCP tool 返回格式未同步 |
| eBPF C 程序或 map key/value | [eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement)、[策略语义](/security/policy-semantics)、[生成文件边界](/reference/generated-files)、[验证页](/operations/verification-benchmark) | 手改 generated 文件、忘记 pin path / permissions / exact-match 语义 |
| runtime setting 或 build feature | [Runtime Settings 与 Feature Manifest](/backend/runtime-settings-features)、[安全模型](/security/model)、[前端 Feature Flags](/frontend/build-feature-flags)、[维护清单](/reference/maintenance-checklists) | 把 compiled-in 写成 runtime enabled，或漏掉 release-mode auth |
| kernel risk feedback / ML catalog / kernel-ml UAPI | [ML、Plugins 与扩展能力](/backend/ml-plugins)、[ML 模型指南](/backend/ml-models-complete-guide)、[内核 ML README](../../kernel-ml/README.md)、[评测报告](/delivery/evaluation) | 后端模型 registry、前端 catalog、kernel-ml README / tests 不一致 |
| TLS / Codex capture / redaction | [脱敏与隐私](/security/redaction-privacy)、[Sanitization](../sanitization.md)、[TLS Quickstart](../backend/TLS_QUICKSTART.md)、[安全模型](/security/model) | 忘记说明默认关闭、body 截断、密钥移除和 EventEnvelope 输出 |
| 文档站页面或导航 | 本页、[阅读路线](/guide/reading-paths)、[维护检查清单](/reference/maintenance-checklists)、`docs/.vitepress/config.ts` | 页面存在但未进 sidebar；相对链接在仓库浏览时失效 |

## ML 模型文档

机器学习模型的完整文档：

- **[ML 模型速查表](../backend/ml-models-summary.md)** ⚡ - 快速查找模型能力和适用场景
- **[ML 模型完整指南](../backend/ml-models-complete-guide.md)** - 详细结构、示例、性能对比
- **[ML 模型对比可视化](../backend/ml-models-visualization.md)** - 面向答辩和横向比较的图表材料
- **[内核态多模型实现](/backend/multi-model-complete)** - 内核态多模型实现细节
- **[实验框架使用指南](/backend/ml-experiments)** - 批量评估和参数扫描
- **[内核 ML 实现](/backend/kernel-ml-implementation)** 与 **[kernel-ml/README](../../kernel-ml/README.md)** - DKMS 模块、proc/sysfs UAPI、CUDA userspace helper

## 仓库内专项文档

这些文档不一定全部进入 VitePress sidebar，但应由本页或对应专题页反向索引：

- [项目文档索引](../project-docs-index.md) — 旧版集中索引，覆盖比赛材料和历史专题；
- [开发历史与实现记录](../DEV_DOCS_INDEX.md) — 开发过程、专项实现和历史决策索引；
- [项目结构深挖](../project-structure-deep-dive.md) — 顶层目录、后端、前端、proto/eBPF、构建验证和安全边界；
- [当前代码实现地图](../codebase-implementation-map.md) — 从当前代码出发的 backend/app、feature manifest、wrapper、前端路由和风险地图；
- [历史版架构](../architecture.md)、[历史版安全模型](../security-model.md)、[威胁模型](../threat-model.md)、[历史版策略语义](../policy-semantics.md) — 可用于答辩和审查，但要与网站当前页交叉核验；
- [External API](../external-api.md)、[Kubernetes](../kubernetes.md)、[OTLP export](../otel-export.md)、[Benchmark](../benchmark.md) — 外部接口、部署和评测；
- [backend README](../../backend/README.md)、[frontend README](../../frontend/README.md)、[wrapper README](../../wrapper/README.md)、[Python adapter README](../../adapters/python/README.md)、[JS adapter README](../../adapters/js/README.md)、[kernel-ml README](../../kernel-ml/README.md) — 组件级 README；
- `docs/ref/**` — 外部参考快照，默认不参与链接完整性扫描，除非用 `python3 scripts/check-doc-links.py --include-ref` 明确检查。

### 历史 / 专项材料按主题入口

::: tip 归档说明
一次性开发总结、实现完成报告、迁移验证记录等文件已归档到 `docs/_archive/`，不再参与 VitePress 构建。如需查阅历史记录，请直接访问 `docs/_archive/` 目录。
:::

| 主题 | 文档 |
| --- | --- |
| 比赛与合规 | [OS competition defense 草案](../os-competition-defense.md)、[第三方声明草案](../third-party-notices.md)、[AI 使用披露](../ai-usage/README.md)、[评测模板](../evaluation-report.md) |
| ML / 模型 | [ML benchmark](../ml-benchmark-report.md) |
| TLS / 脱敏 | [Sanitization](../sanitization.md)、[Sanitization 中文](../sanitization_zh.md) |
| 深度分析 / 规划 | [Project roadmap](../project-roadmap.md)、[Demo script 草案](../demo-script.md) |
| Superpowers 计划 / 规格历史 | [TLS plaintext capture design](../superpowers/specs/2026-05-10-tls-plaintext-capture-design.md)、[TLS plaintext capture plan](../superpowers/plans/2026-05-10-tls-plaintext-capture-implementation.md)、[GHCR devcontainer design](../superpowers/specs/2026-05-12-ghcr-devcontainer-build-design.md)、[GHCR devcontainer plan](../superpowers/plans/2026-05-12-ghcr-devcontainer-build.md)、[make default build plan](../superpowers/plans/2026-05-12-make-default-build.md) |

## 维护策略

1. 网站页面提供稳定入口，专项文档保留详细实验 / 设计 / 历史记录。
2. 行为变化优先同步网站对应页面、组件 README 和专项源文档；不要只改总结页。
3. 代码路径、route、feature gate、UAPI、构建命令变化时，同时更新 [代码入口索引](/reference/code-entrypoints)、[维护检查清单](/reference/maintenance-checklists) 和相关专题页。
4. 文档变更最小验证：`python3 scripts/check-doc-links.py`；需要看弱互链页面时加 `--report`；若改 VitePress 配置、mermaid、frontmatter 或导航，再跑 `bun run docs:build`。
5. 旧路径表述需要定期校正到当前代码；如必须保留历史路径，应明确标注“历史记录”而不是让读者误以为它是当前入口。
6. 周期性复盘 [文档关系审计](documentation-audit.md)，把弱互链页面分成“入口页正常”“历史页由索引承接”“当前专题需补相关导航”三类处理。
