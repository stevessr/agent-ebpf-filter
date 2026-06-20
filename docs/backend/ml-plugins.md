# ML、Plugins 与扩展能力

ML 与 Plugins 是增强层，服务于行为分类、风险评分、训练集管理、自定义 eBPF 扩展和可视化规则构建。

## ML 能力

主要代码：

- `backend/app/ml__*.go`
- `backend/ml/`
- `frontend/src/views/ml/ML.vue`
- `frontend/src/components/config/ml/*`
- `frontend/src/composables/config/useConfigML*.ts`
- `frontend/src/data/mlModelCatalog.ts`

功能域：

- command safety；
- anomaly / classifier；
- built-in datasets；
- remote dataset import；
- training / validation；
- parameter sweep；
- LLM scoring；
- status WebSocket；
- optional CUDA / kernel-ml 辅助路径。

## Kernel risk feedback

`KernelRiskFeedbackSettings` 控制用户态风险评分写回内核 map：

- `Enabled`
- `MinRiskScore`
- `EnforceNetwork`
- `EnforceFileNames`
- `EnforceExec`
- `MaxActionsPerMinute`

这是闭环控制能力，必须同时考虑：

- risk threshold；
- dedup；
- rate limit；
- policy management gate；
- cgroup / LSM feature availability；
- 审计可解释性。

## Plugins

主要代码：

- `backend/app/handlers__handlers_plugin.go`
- `backend/app/plugin*`
- `frontend/src/views/plugins/Plugins.vue`
- `frontend/src/components/plugins/*`
- `frontend/src/composables/plugins/*`

能力：

- plugin registry.
- raw C source plugin.
- template plugin.
- visual eBPF builder.
- pseudocode builder.
- LLM-backed NLP blocks compiler.
- generated eBPF C preview.
- compile / register / load controls.

## attachKind 约束

- visual eBPF plugins 使用 `attachKind: "lsm"`；
- 仅 `unlink` / `do_unlinkat` 流程使用 `attachKind: "kprobe"`；
- 不要为非 unlink visual plugins 序列化 `attachKind: "none"`。

## 文档建议

ML 与 Plugins 在答辩中应作为增强点，而不是压过 eBPF / OS enforcement 主线。应突出：

- 可扩展；
- 可训练；
- 可视化规则构建；
- 与 runtime gates / auth / policy map 的边界。

## 相关文档与同步点

| 主题 | 应同步文档 / 源码 |
| --- | --- |
| ML 模型目录、内置 profile、前端 catalog | [ML 模型速查表](ml-models-summary.md)、[ML 模型完整指南](ml-models-complete-guide.md)、`frontend/src/data/mlModelCatalog.ts` |
| kernel risk feedback | [事件管线](/backend/event-pipeline)、[策略语义](/security/policy-semantics)、[Runtime Gates 与 Auth](/security/runtime-gates-auth) |
| kernel-ml DKMS / CUDA helper / v2 model format | [内核 ML 实现](/backend/kernel-ml-implementation)、[kernel-ml/README](../../kernel-ml/README.md) |
| Plugins routes / visual builder | [路由与 API](/backend/routes-api)、[前端路由与功能页](/frontend/routes-and-pages)、[代码入口索引](/reference/code-entrypoints) |
| 性能或模型评测 | [验证、测试与 Benchmark](/operations/verification-benchmark)、[评测报告](/delivery/evaluation)、[ML benchmark](../ml-benchmark-report.md) |

::: warning 不要混淆三条路径
用户态 ML 打分、kernel risk feedback 写内核策略、`kernel-ml` DKMS 模块是三条不同路径：前者给事件打风险分，第二条在双 gate 下把高风险结果转成 cgroup / LSM map 条目，第三条是独立的 proc/sysfs 推理模块。改其中任一条时，要同步本页和对应专题文档，避免把能力边界写混。
:::
