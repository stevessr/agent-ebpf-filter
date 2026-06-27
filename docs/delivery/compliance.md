# 第三方与 AI 使用披露

比赛和交付材料需要说明第三方依赖、参考资料、许可证和 AI 辅助情况。

## 第三方依赖

### Go 后端依赖

主要依赖：

- **Gin** - HTTP Web 框架
- **gorilla/websocket** - WebSocket 实现
- **cilium/ebpf** - eBPF Go 库
- **OpenTelemetry** - 可观测性标准库
- **protobuf** - 协议缓冲区
- **MCP SDK** - Model Context Protocol

### 前端依赖

- **Vue** `^3.5.32` - 响应式框架
- **Vue Router** 4 - 路由
- **Vite** `^8.0.9` - 构建工具
- **Ant Design Vue** - UI 组件库
- **ApexCharts** / **D3** - 图表库
- **Monaco Editor** - 代码编辑器
- **Shiki** - 代码语法高亮

### 文档站依赖

- **VitePress** `^1.6.4` - 静态站点生成器
- **vitepress-plugin-mermaid** `^2.0.17` - Mermaid 图表插件
- **markdown-it-mathjax3** `^5.2.0` - LaTeX 数学公式插件

### 开发工具

- **Bun** - JavaScript 运行时与包管理器
- **Go** 1.26.2 - 后端编译器
- **Python** 3.13+ / **uv** - 脚本和 adapters
- **clang** / **LLVM** - eBPF 编译器

### 依赖信息维护

- Go: `go.mod`, `go.sum`
- Frontend: `frontend/package.json`, `frontend/bun.lock`
- Docs: `package.json`, `bun.lock`
- Python: `adapters/python/pyproject.toml`
- 第三方声明：`docs/third-party-notices.md`

## AgentSight 项目引用

### 项目关系

Agent eBPF Filter 在架构设计和技术选型上受到 [AgentSight](https://github.com/eunomia-bpf/agentsight) 项目的启发。

**AgentSight 信息**:
- **项目**: https://github.com/eunomia-bpf/agentsight
- **许可证**: MIT License
- **团队**: eunomia-bpf
- **论文**: arXiv:2508.02736, ACM DOI:10.1145/3766882.3767169

### 受启发的设计思路

1. eBPF + TLS capture 对 AI Agent 观测的可行性
2. 系统级 Agent 可观测性的参考架构
3. 零插桩监控的产品定位

### 移植与改写的 Go 文件

以下文件的设计思路或算法受 AgentSight Rust 代码启发，但使用 Go 语言重写：

**AgentSight 兼容层**:
- `backend/app/handlers/agentsight.go`
- `backend/app/api__api_external.go`
- `backend/app/agentsight__analyzers_agentsight.go`
- `backend/app/feature_build_agentsight.go`

**TLS 流量处理**（受 AgentSight HTTPParser / SSE Processor 启发）:
- `backend/app/tls__httpparsertls.go`
- `backend/app/tls__agentstreamlooptls.go`
- `backend/http/parser/tls.go`
- `backend/agent/stream/loop/tls.go`

**指标收集**（受 AgentSight System Runner 启发）:
- `backend/app/observability__metrics_collector.go`

### 差异化说明

Agent eBPF Filter 是独立项目，具有以下重要差异：

| 维度 | AgentSight | Agent eBPF Filter |
| --- | --- | --- |
| 技术栈 | Rust + Next.js | Go + Vue 3 |
| 许可证 | MIT | GPL-3.0 |
| 产品定位 | 纯观测 | 观测 + 控制 |
| 控制能力 | 无 | wrapper / cgroup / BPF LSM |
| TLS capture | 核心能力 | 默认关闭的高风险诊断能力 |
| 集成方式 | 纯 eBPF | eBPF + adapters + hooks + wrapper |

详见：[AgentSight 项目致敬](/reference/agentsight-acknowledgment)

## AI 使用披露

### AI 工具使用

本项目在开发过程中使用了 AI 辅助工具：

- **Claude Code** - 代码生成、文档编写、架构设计
- **其他 AI 工具** - 代码审查、重构建议

### 使用范围

1. **代码生成**: 部分 Go / Vue / TypeScript 代码由 AI 生成后人工审查
2. **文档编写**: 本 VitePress 文档站的部分内容由 AI 辅助生成
3. **架构设计**: AI 提供设计建议，最终决策由人工完成
4. **重构与优化**: AI 辅助识别重复代码和优化机会

### 人工审查

- 所有 AI 生成的代码都经过人工审查和测试
- 所有 AI 生成的文档都经过事实核验
- 关键架构决策由人工确认
- 安全相关代码由人工多轮审查

### 验证方式

```bash
# 后端测试
cd backend && go test ./...

# 前端构建
cd frontend && bun run build

# 文档站构建
bun run docs:build

# eBPF 编译
cd backend/ebpf && go generate
```

### 已知限制

- AI 生成的代码可能包含未考虑的边缘情况
- AI 生成的文档可能存在过时的技术细节
- AI 对项目历史和上下文的理解有限

## 参考资料

### 开源项目

- **AgentSight**: https://github.com/eunomia-bpf/agentsight (MIT License)
- **cilium/ebpf**: https://github.com/cilium/ebpf (Apache 2.0)
- **VitePress**: https://vitepress.dev/ (MIT License)
- **Vue**: https://vuejs.org/ (MIT License)

### 技术文档

- eBPF 文档：`docs/ref/ebpf-docs/` (引用自 ebpf.io)
- AgentSight 文档：`docs/ref/agentsight/` (引用自原项目)

### 学术论文

- AgentSight 论文：arXiv:2508.02736
- eBPF 相关论文：见项目 bibliography

## 合规检查清单

### 引用与归属

- ✅ AgentSight 项目已明确致谢和归属
- ✅ 移植文件已列出并说明差异
- ✅ 第三方依赖已列出许可证
- ✅ 参考文档已注明来源

### 代码原创性

- ✅ 所有代码使用 Go/Vue/TypeScript 重写，不是直接复制
- ✅ 算法和设计思路受启发但实现独立
- ✅ 数据结构根据本项目需求重新设计
- ✅ 接口和 API 为本项目原创

### 许可证合规

- ✅ 本项目使用 GPL-3.0 许可证
- ✅ 引用的 MIT 项目已注明
- ✅ 生成文件已标注不可手改
- ✅ 第三方 vendored docs 已隔离到 `docs/ref/`

### 性能数据

- ✅ 所有 benchmark 数据注明测试环境
- ✅ 性能模型注明假设条件
- ✅ 对比数据注明来源

### 演示与展示

- ✅ 高风险能力演示需说明授权
- ✅ TLS capture 默认关闭已明确标注
- ✅ cgroup / LSM enforcement 需特权已说明
- ✅ 答辩演示脚本包含失败兜底方案

## 维护建议

### 依赖更新

定期检查依赖安全公告：

```bash
# Go 依赖
go list -u -m all

# Frontend 依赖
cd frontend && bun outdated

# Docs 依赖
bun outdated
```

### 许可证合规

添加新依赖时检查许可证兼容性：

- GPL-3.0 兼容：MIT, Apache 2.0, BSD
- 不兼容：某些专有许可证

### AI 使用记录

持续维护 AI 使用记录：

```text
docs/ai-usage/README.md
```

记录每次 AI 辅助的范围和验证结果。

## 联系与披露

如发现合规问题或需要澄清，请通过以下方式联系：

- GitHub Issues
- 项目邮件列表（如有）
- 比赛组委会（答辩场景）

---

**本文档最后更新**: 2026-06-19

**维护责任**: 项目维护者应在添加新依赖、引用新项目或使用 AI 工具时及时更新本文档。

