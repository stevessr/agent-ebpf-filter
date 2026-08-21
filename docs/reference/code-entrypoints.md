# 代码入口索引

## 后端

| 领域 | 入口 |
| --- | --- |
| 启动 | `backend/app/main.go` |
| 路由 | `backend/app/routes.go` |
| 依赖注入容器 | `backend/app/appcontext.go` |
| 桥接层 | `backend/app/handlersbridge.go`, `eventsbridge.go`, `networkflowsbridge.go`, `shellbridge.go` |
| runtime settings | `backend/core/state_types.go` |
| feature manifest | `backend/app/feature_manifest.go`, `backend/app/feature_registry.go` |
| ringbuf jobs | `backend/app/jobs_background.go` |
| 类型别名 | `backend/app/typebridge.go`, `backend/app/types.go` |
| HTTP handlers | `backend/app/handlers/` (24 文件，按模块拆分) |
| 事件处理 | `backend/app/events/` (事件归一化/语义告警/上下文组装/Kernel Risk) |
| 网络分析 | `backend/app/network/` (TCP/DNS/GeoIP/带宽/流聚合) |
| TLS 捕获 | `backend/app/tls/` (明文捕获/HTTP解析/SSL过滤器/AI元数据富化) |
| 采集器指标 | `backend/app/observability/` |
| Shell 会话 | `backend/app/shell/` |
| 沙箱管控 | `backend/app/handlers/cgroup_sandbox.go`, `backend/app/handlers/lsm_enforcer.go`, `backend/internal/sandbox/` |
| 平台抽象 | `backend/app/platform/` |
| 运行时状态 | `backend/app/runtime/` |
| 导出 | `backend/app/export/` |
| AgentSight 兼容 | `backend/app/handlers/agentsight.go` |
| 外部 API v1 | `backend/app/api_external.go` |
| ML 引擎 | `backend/ml/` |
| 数据脱敏 | `backend/redaction/` |
| 网络核心算法 | `backend/internal/network/` (Flow/TCP/DNS/Scope) |
| 行为分类 | `backend/internal/behavior/` |
| GeoIP | `backend/internal/geoip/` |
| 进程上下文 | `backend/app/events/context_event.go` |
| 插件系统 | `backend/app/handlers/plugin.go`, `backend/app/plugins.go` |

## eBPF

| 领域 | 入口 |
| --- | --- |
| main tracker | `backend/ebpf/agent_tracker.c` |
| cgroup sandbox | `backend/ebpf/cgroup_sandbox.c` |
| BPF LSM | `backend/ebpf/lsm_enforcer.c` |
| TLS capture | `backend/ebpf/agent_tls_capture.c` |
| go generate | `backend/ebpf/gen*.go` |

## 前端

| 领域 | 入口 |
| --- | --- |
| app bootstrap | `frontend/src/main.ts` |
| shell | `frontend/src/App.vue` |
| routes | `frontend/src/router/index.ts` |
| dashboard | `frontend/src/views/dashboard/` |
| monitor | `frontend/src/views/monitor/` |
| network | `frontend/src/views/network/` (Network.vue, NetworkFlow.vue, TLSCapture.vue) |
| execution graph | `frontend/src/views/execution-graph/` |
| explorer | `frontend/src/views/explorer/` |
| executor | `frontend/src/views/executor/` |
| hooks | `frontend/src/views/hooks/` |
| AgentSight | `frontend/src/views/agentsight/` |
| config | `frontend/src/views/config/` |
| ML | `frontend/src/views/ml/` |
| plugins | `frontend/src/views/plugins/` |

## 集成

| 领域 | 入口 |
| --- | --- |
| wrapper | `wrapper/main.go` |
| Python adapter | `adapters/python/agent_tracker.py` |
| JS adapter | `adapters/js/agentTracker.js` |
| proto | `proto/*.proto` |
| deploy | `deploy/kubernetes/` |
| devcontainer | `.devcontainer/` |
| docs site | `docs/.vitepress/config.ts` |

## 相关导航

- [文档地图](documentation-map.md)
- [维护检查清单](maintenance-checklists.md)
- [生成文件边界](generated-files.md)
- [代码实现模式与最佳实践](implementation-patterns.md)
- [文档关系审计](documentation-audit.md)
