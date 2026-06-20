# 阅读路线

不同角色关注点不同。本站提供四条推荐路线。

## 新开发者路线

1. [项目是什么](/guide/what-is-agent-ebpf-filter)
2. [快速开始](/guide/quick-start)
3. [总体架构](/architecture/overview)
4. [数据流](/architecture/data-flow)
5. [启动链路](/backend/runtime-startup)
6. [路由与 API](/backend/routes-api)
7. [前端工作台](/frontend/workbench)
8. [代码入口索引](/reference/code-entrypoints)

## 后端 / eBPF 开发路线

1. [后端启动链路](/backend/runtime-startup)
2. [事件管线](/backend/event-pipeline)
3. [eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement)
4. [协议与事件模型](/architecture/protocol-events)
5. [Runtime Settings 与 Feature Manifest](/backend/runtime-settings-features)
6. [生成文件边界](/reference/generated-files)

## 前端开发路线

1. [前端工作台总览](/frontend/workbench)
2. [路由与功能页](/frontend/routes-and-pages)
3. [组件与 Composables](/frontend/components-composables)
4. [构建与 Feature Flags](/frontend/build-feature-flags)
5. [维护检查清单](/reference/maintenance-checklists)

## 安全审查路线

1. [安全模型](/security/model)
2. [Runtime Gates 与 Auth](/security/runtime-gates-auth)
3. [策略语义](/security/policy-semantics)
4. [脱敏与隐私](/security/redaction-privacy)
5. [Wrapper 命令策略](/integrations/wrapper)
6. [Native Hooks](/integrations/native-hooks)
7. [eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement)

## 比赛答辩路线

1. [比赛答辩主线](/delivery/competition-defense)
2. [总体架构](/architecture/overview)
3. [功能总览](/guide/capabilities)
4. [演示脚本](/delivery/demo-script)
5. [评测报告](/delivery/evaluation)
6. [第三方与 AI 使用披露](/delivery/compliance)

## 任务型阅读路线

如果你不是“从头阅读”，而是在维护某个功能，按下面的影响链跳转。

| 任务 | 推荐顺序 | 结束前检查 |
| --- | --- | --- |
| 新增 / 修改后端 API | [路由与 API](/backend/routes-api) → [Runtime Gates 与 Auth](/security/runtime-gates-auth) → [External API](../external-api.md) → [前端路由与功能页](/frontend/routes-and-pages) | 是否需要 auth、runtime gate、MCP / `/api/v1` alias、前端 composable |
| 修改事件字段或 protobuf | [协议与事件模型](/architecture/protocol-events) → [事件管线](/backend/event-pipeline) → [生成文件边界](/reference/generated-files) → [组件与 Composables](/frontend/components-composables) | `make proto` 后 backend / frontend / adapters / docs 是否同步 |
| 调整 cgroup / BPF LSM 策略 | [eBPF 与 OS Enforcement](/backend/ebpf-os-enforcement) → [策略语义](/security/policy-semantics) → [安全模型](/security/model) → [验证页](/operations/verification-benchmark) | exact key 语义、pin path、map permission、policy gate 是否仍准确 |
| 调整 runtime gate / build feature | [Runtime Settings 与 Feature Manifest](/backend/runtime-settings-features) → [Runtime Gates 与 Auth](/security/runtime-gates-auth) → [前端 Feature Flags](/frontend/build-feature-flags) | 不要把“编译进来”写成“运行时已启用” |
| 调整 ML / kernel risk feedback | [ML、Plugins 与扩展能力](/backend/ml-plugins) → [ML 模型完整指南](/backend/ml-models-complete-guide) → [内核 ML README](../../kernel-ml/README.md) → [评测报告](/delivery/evaluation) | 后端 registry、前端 catalog、kernel-ml UAPI / README / tests 是否一致 |
| 调整 TLS / Codex capture / redaction | [脱敏与隐私](/security/redaction-privacy) → [Sanitization](../sanitization.md) → [TLS Quickstart](../backend/TLS_QUICKSTART.md) → [安全模型](/security/model) | 默认关闭、认证、脱敏、密钥移除、body 截断是否都写清楚 |
| 只改文档 | [文档地图](/reference/documentation-map) → [维护检查清单](/reference/maintenance-checklists) → [验证页](/operations/verification-benchmark) | 页面是否进 sidebar、相对链接是否可点击、是否需要更新专项文档 |

## 跨页校验习惯

- 每读一条“能力说明”，顺手确认它是否能在 [代码入口索引](/reference/code-entrypoints) 找到当前源码入口。
- 每读一条“安全 / 默认值说明”，顺手确认 [Runtime Gates 与 Auth](/security/runtime-gates-auth) 与 [策略语义](/security/policy-semantics) 是否一致。
- 每读一条“构建 / 验证命令”，顺手确认 [验证、测试与 Benchmark](/operations/verification-benchmark) 是否有对应命令。
- 每次文档改动后至少运行 `python3 scripts/check-doc-links.py`；导航、frontmatter 或 mermaid 变化再运行 `bun run docs:build`。
