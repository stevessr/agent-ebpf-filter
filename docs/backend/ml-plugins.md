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

- plugin registry；
- raw C source plugin；
- template plugin；
- visual eBPF builder；
- pseudocode builder；
- LLM-backed NLP blocks compiler；
- generated eBPF C preview；
- compile / register / load controls。

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
