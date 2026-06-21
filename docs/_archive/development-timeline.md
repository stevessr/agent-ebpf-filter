# 开发时间线与提交记录

> 项目：Agent eBPF Filter  
> 目的：整理阶段性开发过程，用于操作系统设计赛对“逐步改进版本、多次提交记录、详细注释说明、真实开发迭代过程”的要求。  
> 注意：本文档基于当前仓库 `git log` 自动提取的近期提交草案，最终提交前应由队伍人工补充完整日期、阶段目标、成员分工和关键验证结果。

---

## 1. 赛事过程合规要求映射

用户提供的赛事规则要求包括：

- 初赛阶段不少于 8 次提交记录；
- 决赛阶段不少于 4 次提交记录；
- 每次提交间隔建议 3–7 天；
- 每次提交应包含详细注释说明，例如新增功能和修复关键 bug；
- 禁止无注释批量代码提交；
- 鼓励频繁提交并全程详细真实记录开发迭代过程；
- 使用 AI 工具时，需要在 commit 记录、开发文档、设计文档、答辩 PPT 中说明使用场景、成果和交互记录。

本项目建议用本文档和 `docs/ai-usage/` 共同满足过程说明要求。

---

## 2. 近期提交记录摘录

| Commit | 日期 | 说明 |
| --- | --- | --- |
| `0020848` | 2026-06-19 | feat: 修复执行图行为追踪功能，支持 PID 和进程树过滤器 |
| `7dc1b1e` | 2026-06-16 | feat: Enhance AgentSight performance with optimizations and new components |
| `828421b` | 2026-06-16 | feat: Refactor redaction policy types and update related components |
| `6acaeaf` | 2026-06-12 | kernel-ml: Enhance model loading and inference with caching and sysfs interface |
| `198284f` | 2026-06-12 | Add CUDA inference support and backend selection to kernel-ml module |
| `44cc97c` | 2026-06-12 | feat: Complete ML model implementation - 9 algorithms total |
| `d982a88` | 2026-06-12 | docs: Add multi-model implementation summary |
| `27d5775` | 2026-06-12 | feat: Add multi-model support to kernel ML module |
| `cf1cf18` | 2026-06-12 | docs: Add kernel ML module implementation summary |
| `6c603e3` | 2026-06-12 | feat: Add kernel-space ML inference module (DKMS) |
| `e4dee65` | 2026-06-12 | chore: Add eBPF optimization checklist |
| `b0f9582` | 2026-06-12 | docs: Add eBPF optimization summary |
| `aea85a7` | 2026-06-12 | refactor: Optimize eBPF code for efficiency (-85% syscall handler code) |
| `0ebd2c1` | 2026-06-12 | feat: Replace Codex syscall tracepoint with rustls offset-based uprobe |
| `187a4c2` | 2026-06-12 | feat: Enhance MCP service with new tools and skills for network analysis, process monitoring, and security configuration |
| `978f38b` | 2026-06-12 | feat: Enhance TLS probe discovery for Node.js and Rustls |
| `0ad3690` | 2026-06-12 | feat: Add SSL/TLS capture support for Claude Code and Codex |
| `91a9094` | 2026-06-09 | feat: add MLStatus message to tracker_system.proto and update related code |
| `0fc54aa` | 2026-06-09 | feat: Add new attention mechanisms and enhanced models |
| `933671d` | 2026-06-09 | Add path mapping tests, enhance session module dependencies, and improve logging permissions |
| `aed0842` | 2026-06-08 | feat(tls): implement KeyRemover for sensitive data detection and removal |
| `8f4415d` | 2026-06-08 | feat: add redaction functionality across various views and update proto definitions |
| `e747c07` | 2026-06-07 | feat: 添加 eBPF ML 模型拦截功能自动测试脚本及简化测试脚本 |
| `ca00164` | 2026-06-07 | feat: 添加 eBPF ML 模型加载与测试脚本及相关文档 |
| `c1b6b19` | 2026-06-07 | Refactor code structure for improved readability and maintainability |
| `8fb4060` | 2026-06-07 | feat: add comprehensive test set evaluation and attention mechanism documentation |
| `b6ae31e` | 2026-06-07 | feat: add attention-based models and training scripts |
| `5c560a8` | 2026-06-05 | feat: add kernel risk feedback mechanism and ensemble model types |
| `07d3e44` | 2026-06-03 | feat: introduce feature registry and manifest for build-time feature management |
| `55b8849` | 2026-06-03 | feat: 添加内置可执行文件附加功能，更新相关接口和前端实现 |
| `a8512c1` | 2026-06-01 | Refactor code structure for improved readability and maintainability |
| `4041654` | 2026-06-01 | merge |
| `d71bbe1` | 2026-06-01 | feat: 重构行为分类逻辑，移除旧的实现并更新相关引用 |
| `e27a1d4` | 2026-06-01 | lint |
| `816a9da` | 2026-06-01 | mv |
| `4a391fb` | 2026-06-01 | Add visual LLM parsing and plugin for kernel-defense policy compilation |
| `760b7f2` | 2026-05-31 | feat: 重构 Codex 捕获处理，添加新的处理程序和测试，移除旧的处理逻辑 |
| `1cde484` | 2026-05-31 | lint |
| `a7aa371` | 2026-05-31 | lint |
| `a7293a1` | 2026-05-31 | feat: 添加可执行文件挂载支持，优化 TLS 捕获功能，更新前端展示 |

---

## 3. 阶段归纳

### 3.1 TLS / Codex / Hook 捕获增强阶段

代表提交：

- `a7293a1`
- `760b7f2`
- `0ad3690`
- `978f38b`
- `0ebd2c1`

阶段目标：

- 增强 TLS / Codex 捕获能力；
- 支持 Claude Code、Codex 等 AI CLI 的捕获与展示；
- 优化 rustls / Node.js / executable attach 等场景。

答辩可引用贡献：

- AI CLI hook 和 TLS 诊断能力；
- 高风险能力默认关闭；
- 与脱敏和 EventEnvelope 链路集成。

### 3.2 ML / 风险反馈阶段

代表提交：

- `5c560a8`
- `b6ae31e`
- `8fb4060`
- `e747c07`
- `91a9094`
- `44cc97c`

阶段目标：

- 建立 command safety / anomaly / classifier 模型；
- 引入 attention、ensemble、多模型和 MLStatus；
- 支持训练、评测、自动测试和前端状态展示。

答辩可引用贡献：

- 从系统调用事实到风险评分；
- 用户态 ML 与内核态 ML 的分工；
- replay benchmark 和训练集管理。

### 3.3 内核态 ML / CUDA 探索阶段

代表提交：

- `6c603e3`
- `cf1cf18`
- `27d5775`
- `198284f`
- `6acaeaf`

阶段目标：

- 新增 `kernel-ml` DKMS 模块；
- 支持定点数 Random Forest 推理；
- 增加 sysfs / proc 控制面；
- 增加 CUDA helper offload、LRU cache、backend selection。

答辩可引用贡献：

- 操作系统与硬件结合；
- 内核态低延迟推理；
- CUDA 用户态 helper 与内核 fallback。

### 3.4 eBPF 维护性与性能优化阶段

代表提交：

- `aea85a7`
- `b0f9582`
- `e4dee65`

阶段目标：

- 使用宏减少 syscall handler 重复代码；
- 移除未使用实验代码；
- 输出优化总结和 checklist。

答辩可引用贡献：

- eBPF 代码总源码行数显著减少；
- 降低 verifier / 维护认知复杂度；
- 保持运行时功能不退化。

### 3.5 Redaction / 隐私保护阶段

代表提交：

- `8f4415d`
- `aed0842`
- `828421b`

阶段目标：

- 建立跨视图 redaction 能力；
- 支持 key removal；
- 重构 policy types。

答辩可引用贡献：

- None / Basic / Standard / Strict 多级脱敏；
- 保护路径、命令参数、网络地址、凭证；
- TLS / Codex 数据进入 UI 和持久化前进行脱敏。

### 3.6 AgentSight / Execution Graph 阶段

代表提交：

- `7dc1b1e`
- `0020848`

阶段目标：

- 优化 AgentSight 性能和组件；
- 修复执行图行为追踪；
- 支持 PID 和进程树过滤器。

答辩可引用贡献：

- 行为图谱；
- 进程树 / 时间线 / metrics；
- 10,000 events 级别 UI 性能优化；
- Agent 行为证据链展示。

---

## 4. 后续提交建议

为了满足比赛提交记录的真实性和可读性，建议后续按以下主题分别提交：

1. `docs: add OS competition defense materials`
   - 包含答辩主文档、结构文档、规划文档、文档索引。
2. `docs: add AI usage disclosure records`
   - 包含 `docs/ai-usage/` 交互记录。
3. `docs: add third-party notices and source attribution`
   - 包含依赖许可证和引用来源清单。
4. `docs: add demo script and evaluation report`
   - 包含演示脚本、benchmark、smoke 结果。
5. `test: update runtime replay evidence for defense`
   - 如补充 replay 场景或报告。
6. `fix:` / `feat:`
   - 若根据演示预演修复实际功能，再独立提交。

建议在使用 AI 辅助生成文档或代码的提交中加入 footer，例如：

```text
AI-Assisted-By: Claude Code
AI-Usage: drafted documentation outline; human reviewed and edited
```

---

## 5. 待人工补充项

- [ ] 每个阶段的队员分工；
- [ ] 每个阶段的测试命令和结果；
- [ ] 比赛平台实际提交日期；
- [ ] 官方题目编号和题目名称；
- [ ] AI 工具交互记录与具体 commit 的映射；
- [ ] 如果参考往届作品或开源项目，补充基础版本、增量贡献和许可证。

---

## 相关导航

- [项目文档索引](project-docs-index.md)
- [AI 使用记录](ai-usage/README.md)
- [第三方与 AI 使用披露](delivery/compliance.md)
- [项目路线图](project-roadmap.md)
- [文档关系审计](reference/documentation-audit.md)
