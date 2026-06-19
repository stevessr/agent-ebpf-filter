# 文档地图

本站是项目网站入口，仓库中仍保留更细的专项文档。

## 网站章节

| 章节 | 用途 |
| --- | --- |
| Guide | 项目定位、快速开始、功能总览、阅读路线 |
| Architecture | 总体架构、数据流、运行时边界、协议事件 |
| Backend | 启动链、路由、事件管线、eBPF、runtime settings、ML/plugins |
| Frontend | 工作台、路由、组件、build feature flags |
| Security | 安全模型、策略语义、auth/gates、redaction |
| Integrations | adapters、wrapper、hooks、MCP/API/OTLP |
| Operations | 构建运行、devcontainer、部署、验证 benchmark |
| Delivery | 答辩、演示、评测、合规 |
| Reference | 文档地图、代码入口、生成文件、检查清单 |

## 仓库内专项文档

- `docs/project-docs-index.md`
- `docs/project-structure-deep-dive.md`
- `docs/codebase-implementation-map.md`
- `docs/architecture.md`
- `docs/security-model.md`
- `docs/threat-model.md`
- `docs/policy-semantics.md`
- `docs/external-api.md`
- `docs/kubernetes.md`
- `docs/otel-export.md`
- `docs/benchmark.md`
- `backend/README.md`
- `frontend/README.md`
- `wrapper/README.md`
- `adapters/python/README.md`
- `adapters/js/README.md`
- `kernel-ml/README.md`

## 维护策略

- 网站页面提供结构化入口；
- 专项文档保留详细实验 / 设计 / 历史记录；
- 行为变化优先同步网站对应页面和专项源文档；
- 旧路径表述需要定期校正到当前代码。
