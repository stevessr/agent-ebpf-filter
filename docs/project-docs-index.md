# 项目文档索引

> 项目：Agent eBPF Filter  
> 目的：将仓库中分散的技术文档、比赛答辩文档、规划文档、合规文档和演示材料统一索引，便于开发、评审、答辩和后续维护。  
> 推荐阅读顺序：先读“比赛交付文档”，再读“核心架构文档”，最后按功能域阅读专项文档。

---

## 1. 比赛交付文档

| 文档 | 状态 | 用途 |
| --- | --- | --- |
| `docs/os-competition-defense.md` | 已建立草案 | 操作系统设计赛答辩项目主文档，覆盖项目概述、技术方案、创新点、演示、合规和 AI 使用披露 |
| `docs/project-structure-deep-dive.md` | 已建立 | 项目结构深度说明，覆盖顶层目录、分层架构、后端、前端、proto/eBPF、构建验证和安全边界 |
| `docs/codebase-implementation-map.md` | 已建立 | 从当前代码实现出发的深挖地图，覆盖 backend/app 启动链、路由、事件上下文、feature manifest、wrapper、前端路由和文档风险 |
| `docs/project-roadmap.md` | 已建立 | 项目规划与交付路线图，覆盖阶段规划、测试、风险、交付包和后续任务 |
| `docs/project-docs-index.md` | 已建立 | 本索引文档，用于集中导航所有项目文档 |
| `docs/ai-usage/README.md` | 已建立 | AI 工具使用披露记录模板 |
| `docs/development-timeline.md` | 已建立草案 | 从 git log 整理开发过程与提交记录，满足赛事过程合规要求 |
| `docs/demo-script.md` | 已建立草案 | 答辩现场演示脚本、权限要求、预期现象和失败兜底方案 |
| `docs/evaluation-report.md` | 已建立模板 | 构建、smoke、benchmark、性能和评测结果汇总模板 |
| `docs/third-party-notices.md` | 已建立草案 | 第三方依赖、复制 / 借鉴来源、许可证说明 |

---

## 2. 核心架构与安全文档

| 文档 | 主题 | 何时阅读 |
| --- | --- | --- |
| `docs/architecture.md` | 运行时架构、数据流、组件职责 | 需要理解系统整体架构、写答辩架构页、改跨层数据流时 |
| `docs/security-model.md` | 当前安全模型、权限拆分、auth、runtime gate、内核阻断 | 涉及安全边界、release mode、危险能力、Kubernetes 部署时 |
| `docs/threat-model.md` | 保护资产、威胁来源、检测重点、非目标 | 写答辩安全章节、审查系统边界、回应评委安全问题时 |
| `docs/policy-semantics.md` | 策略匹配语义和限制 | 修改 wrapper、cgroup、LSM、tracked paths/commands 时 |
| `docs/external-api.md` | 外部 API 与兼容接口 | 维护 `/api/**`、`/api/v1/**`、AgentSight API、外部接入时 |
| `docs/kubernetes.md` | Kubernetes 部署 | 准备节点级部署、DaemonSet、集群演示时 |
| `docs/otel-export.md` | OTLP export | 需要把事件导出到 tracing / observability 系统时 |
| `docs/benchmark.md` | runtime replay benchmark | 做评测、性能指标、异常场景回放时 |

---

## 3. eBPF、内核和 OS enforcement 文档

| 文档 | 主题 |
| --- | --- |
| `docs/ebpf-optimization-summary.md` | eBPF syscall handler 宏优化和代码维护性优化结果 |
| `docs/ebpf-ml-feasibility.md` | eBPF ML 可行性分析 |
| `docs/ebpf-ml-implementation.md` | eBPF ML 实现说明 |
| `docs/kernel-load-manual.md` | 内核加载 / 手动操作说明 |
| `docs/kernel-ml-implementation.md` | DKMS 内核态 ML 推理模块实现总结 |
| `docs/btf-fix-guide.md` | BTF 相关问题修复指南 |
| `OPTIMIZATION_CHECKLIST.md` | 优化检查清单 |

答辩中建议从这些文档中提炼：

- syscall tracepoint 覆盖范围；
- ringbuf / pinned maps 数据路径；
- cgroup 和 BPF LSM 阻断；
- eBPF 代码可维护性优化；
- kernel-ml 定点数推理、sysfs/proc、CUDA helper。

---

## 4. AgentSight、Execution Graph 与行为追踪文档

| 文档 | 主题 |
| --- | --- |
| `docs/agentsight-optimization-guide.md` | AgentSight 性能优化指南 |
| `docs/agentsight-optimization-summary.md` | AgentSight 优化结果总结 |
| `docs/execution-graph-behavior-tracking-fix.md` | 执行图行为追踪修复说明 |

相关代码入口：

- `frontend/src/views/execution-graph/ExecutionGraph.vue`
- `frontend/src/components/agentsight/`
- `frontend/src/composables/agentsight/`
- `backend/execution_graph.go`
- `backend/event_envelope.go`
- `backend/agentsight_handlers.go`

答辩中建议突出：

1. Agent 行为从 syscall 事实转换为进程树 / 时间线 / 图谱；
2. 支持 JSON / JSONL / CSV 导出；
3. 支持 recording / replay；
4. 前端针对 10,000 events 级别数据做了缓存、过滤和渲染优化。

---

## 5. ML 与模型相关文档

| 文档 | 主题 |
| --- | --- |
| `docs/ml-opening-report.md` | ML 方向开题 / 规划材料 |
| `docs/ml-benchmark-report.md` | ML benchmark 报告 |
| `docs/ml-experiments.md` | ML 实验记录 |
| `docs/ml-attention.md` | attention 模型相关说明 |
| `docs/multi-model-support.md` | 多模型支持设计 |
| `docs/multi-model-complete.md` | 内核态多模型实现 |
| `docs/all-models-complete.md` | 全模型实现概览 |
| `docs/kernel-ml-implementation.md` | 内核态 ML 推理模块实现 |

相关代码入口：

- `backend/ml_*.go`
- `backend/ml/`
- `frontend/src/views/ml/ML.vue`
- `frontend/src/components/config/ml/`
- `kernel-ml/`

答辩中建议把 ML 作为增强点，而不是压过 eBPF / OS enforcement 主线。

---

## 6. TLS、Codex 与数据捕获文档

| 文档 | 主题 |
| --- | --- |
| `docs/ssl-implementation-summary.md` | SSL/TLS capture 实现说明 |
| `docs/ssl-final-summary.md` | SSL/TLS capture 能力说明 |
| `docs/ssl-claude-codex-support.md` | Claude Code / Codex TLS 支持 |
| `docs/codex-workflows.md` | Codex workflow 说明 |
| `docs/codex-stripped-analysis.md` | Codex stripped binary 分析 |
| `docs/codex-implementation-complete.md` | Codex syscall-level tracing 实现 |
| `docs/codex-rustls-fix.md` | Codex rustls 修复说明 |

安全提醒：

- TLS 明文捕获是高风险诊断能力；
- 默认关闭；
- 需要 runtime gate 显式启用；
- 普通事件流应使用 metadata / digest / redaction 结果；
- 答辩中应主动说明“不默认采集敏感明文”。

---

## 7. 数据脱敏与隐私文档

| 文档 | 主题 |
| --- | --- |
| `docs/sanitization.md` | 英文脱敏文档 |
| `docs/sanitization_zh.md` | 中文脱敏使用指南 |
| `docs/SANITIZATION_IMPLEMENTATION_SUMMARY.md` | 脱敏机制实现说明 |
| `backend/redaction/README.md` | redaction 模块说明 |
| `backend/redaction/docs/SANITIZATION_ENHANCEMENTS_v2.md` | redaction 增强说明 |

答辩中建议突出：

- None / Basic / Standard / Strict 四级脱敏；
- path、command args、network、credential 多类别处理；
- TLS / Codex ingest 也复用脱敏链路；
- 保护隐私是系统设计的一部分，而非事后补丁。

---

## 8. MCP、外部接口和技能文档

| 文档 | 主题 |
| --- | --- |
| `docs/mcp-skills-enhancement.md` | MCP skills 增强 |
| `docs/mcp-sse-to-streamable-migration.md` | MCP SSE 到 streamable migration |
| `docs/mcp-streamable-verification.md` | MCP streamable 验证 |
| `docs/mcp-migration-complete.md` | MCP Streamable HTTP 迁移说明 |
| `docs/external-api.md` | 外部 API 文档 |

相关代码入口：

- `backend/mcp_server.go`
- `backend/routes.go`
- `.claude/skills/analyze-network/`
- `.claude/skills/configure-security/`
- `.claude/skills/monitor-process/`
- `.claude/skills/project-structure/`

---

## 9. 组件 README

| 文档 | 主题 |
| --- | --- |
| `backend/README.md` | 后端说明 |
| `frontend/README.md` | 前端说明 |
| `wrapper/README.md` | wrapper 说明 |
| `adapters/python/README.md` | Python adapter 使用说明 |
| `adapters/js/README.md` | JavaScript adapter 使用说明 |
| `kernel-ml/README.md` | kernel-ml 模块说明 |
| `.devcontainer/README.md` | devcontainer 使用说明 |
| `tools/dev-env-tui/README.md` | 开发环境 TUI 说明 |

---

## 10. 推荐阅读路径

### 10.1 评委 / 答辩阅读路径

1. `docs/os-competition-defense.md`
2. `docs/project-structure-deep-dive.md`
3. `docs/project-roadmap.md`
4. `docs/security-model.md`
5. `docs/threat-model.md`
6. `docs/benchmark.md`
7. `docs/ai-usage/README.md`

### 10.2 新开发者阅读路径

1. `README.md` 或 `README_cn.md`
2. `docs/project-structure-deep-dive.md`
3. `docs/codebase-implementation-map.md`
4. `docs/architecture.md`
5. `AGENTS.md`
6. `.claude/skills/project-structure/references/00-layered-map.md`
7. 对应功能域文档和代码入口

### 10.3 安全审查阅读路径

1. `docs/security-model.md`
2. `docs/threat-model.md`
3. `docs/policy-semantics.md`
4. `docs/codebase-implementation-map.md` 的 feature manifest、路由和安全边界章节
5. `docs/project-structure-deep-dive.md` 的安全边界章节
6. `backend/app/feature_manifest.go`、`backend/core/state_types.go`、`backend/app/routes__routes.go`
7. `backend/app/*cgroup*`、`backend/app/*lsm*`、`backend/ebpf/cgroup_sandbox.c`、`backend/ebpf/lsm_enforcer.c`

### 10.4 比赛材料准备路径

1. `docs/os-competition-defense.md`
2. `docs/project-roadmap.md`
3. `docs/project-structure-deep-dive.md`
4. `docs/development-timeline.md`
5. `docs/third-party-notices.md`
6. `docs/evaluation-report.md`
7. `docs/demo-script.md`
8. `docs/ai-usage/README.md`

---

## 11. 文档维护规则

1. 行为变化优先更新 `README.md`、`docs/architecture.md` 和对应专项文档。
2. 代码入口、路由、feature gate 或目录分层变化时同步 `docs/codebase-implementation-map.md` 与 `docs/project-structure-deep-dive.md`。
3. 安全边界变化必须更新 `docs/security-model.md` / `docs/threat-model.md` / `docs/policy-semantics.md`。
4. 比赛相关变化同步更新 `docs/os-competition-defense.md`、`docs/project-roadmap.md` 和本索引。
5. 新增 AI 辅助成果时更新 `docs/ai-usage/`。
6. 引入第三方复制代码 / 文档时更新 `docs/third-party-notices.md`。
7. 新增演示流程或评测结果时更新 `docs/demo-script.md` / `docs/evaluation-report.md`。
8. 文档中引用性能数据前必须写明测试环境、命令和日期。
