# 06 — 功能域索引层

本层按用户可见功能域组织文件入口。需要“我要改某个页面/功能，应该看哪些文件”时优先读这里。

## Dashboard / live events

前端：

- `frontend/src/views/dashboard/Dashboard.vue`
- `frontend/src/components/dashboard/DashboardToolbar.vue`
- `frontend/src/components/dashboard/DashboardEventModal.vue`
- `frontend/src/composables/dashboard/useDashboard.ts`
- `frontend/src/composables/dashboard/useDashboardStream.ts`
- `frontend/src/composables/dashboard/dashboardConstants.ts`
- `frontend/src/composables/dashboard/dashboardHelpers.ts`

后端：

- `backend/routes.go`：`/ws`、`/events/recent`。
- `backend/api_ws.go`
- `backend/event_recording.go`
- `backend/event_envelope.go`
- `backend/runtime_state*.go`
- `backend/network_events.go`（event type/string 相关）

常见改动：

- 新增 dashboard filter。
- 改 event modal 展示。
- 改实时事件流处理。
- 增加 event type option。

验证：

- `cd frontend && bun run build`
- `cd backend && go test ./...`（若改 backend event/API）

## Monitor / system telemetry

前端：

- `frontend/src/views/monitor/Monitor.vue`
- `frontend/src/components/monitor/HealthCpu.vue`
- `frontend/src/components/monitor/HealthMemory.vue`
- `frontend/src/components/monitor/HealthGpu.vue`
- `frontend/src/components/monitor/HealthIO.vue`
- `frontend/src/components/monitor/HealthFaults.vue`
- `frontend/src/components/monitor/HealthProcMem.vue`
- `frontend/src/components/monitor/ProcessTable.vue`
- `frontend/src/components/monitor/ProcessPickerModal.vue`
- `frontend/src/components/monitor/SensorsPanel.vue`
- `frontend/src/components/monitor/SystemdPanel.vue`
- `frontend/src/components/monitor/TracingPanel.vue`
- `frontend/src/components/monitor/HistoryChartModal.vue`
- `frontend/src/composables/monitor/useMonitorData.ts`
- `frontend/src/composables/monitor/useSensors.ts`
- `frontend/src/composables/monitor/useSystemd.ts`
- `frontend/src/composables/monitor/useInterfaceMonitor.ts`

后端：

- `backend/system_handlers*.go`
- `backend/metrics.go`
- `backend/collector_metrics.go`
- `backend/prometheus_metrics.go`
- `backend/camera_manager.go`
- `backend/api_ws.go`：`/ws/system` 等。

常见改动：

- 新增硬件/系统指标。
- 新增 sensor/systemd/tracing UI。
- 修改 metric history。

验证：

- frontend build。
- backend tests。
- 如涉及真实硬件/系统接口，需手动运行 app 验证。

## Network / flows / enrichment

前端：

- `frontend/src/views/network/Network.vue`
- `frontend/src/views/network/NetworkFlow.vue`
- `frontend/src/views/network/useFlowFilters.ts`
- `frontend/src/views/network/useInterfaceMonitor.ts`
- `frontend/src/components/network/NetworkEventModal.vue`
- `frontend/src/components/network/NetworkFlowPanel.vue`
- `frontend/src/components/network/FlowDetailModal.vue`
- `frontend/src/components/network/NetworkStatsCards.vue`
- `frontend/src/components/network/TrafficGraph.vue`
- `frontend/src/composables/network/useNetworkInterfaces.ts`
- `frontend/src/composables/network/useNetworkEnrichment.ts`
- `frontend/src/composables/network/useTrafficGraph.ts`

后端：

- `backend/network_events.go`
- `backend/network_syscalls.go`
- `backend/network_syscalls_ext.go`
- `backend/network_event_flows.go`
- `backend/network_flow_aggregator.go`
- `backend/network_flow_store.go`
- `backend/network_audit.go`
- `backend/network_enrichment*.go`
- `backend/network_enrichment_tcp.go`
- `backend/network_enrichment_net.go`
- `backend/pcap_export.go`
- `backend/protocol_detection.go`
- `backend/routes.go`：`/network/**`

常见改动：

- 新增 flow 字段。
- 新增 DNS/TCP/GeoIP/protocol enrichment。
- 修改 flow aggregation。
- 导出 JSONL/PCAP。

验证：

- `cd backend && go test ./...`
- frontend build。
- 网络行为最好结合 live app/manual capture 验证。

## TLS Capture / Codex Capture

前端：

- `frontend/src/views/network/TLSCapture.vue`
- `frontend/src/components/agentsight/AgentSightTracePanel.vue`（如显示 TLS/Codex 事件）
- `frontend/src/composables/agentsight/useAgentSightEvents.ts`

后端：

- `backend/tls_capture_controller.go`
- `backend/tls_capture_handlers.go`
- `backend/tls_capture_rules.go`
- `backend/tls_capture_startup.go`
- `backend/tls_capture_store.go`
- `backend/tls_capture_types.go`
- `backend/tls_fragment_assembler.go`
- `backend/tls_http_parser.go`
- `backend/tls_http_stream_assembler.go`
- `backend/tls_probe_discovery.go`
- `backend/tls_probe_manager.go`
- `backend/tls_agent_stream.go`
- `backend/tls_agent_stream_loop.go`
- `backend/codex/capture/handlers/handlers.go`
- `backend/codex_capture_sink.go`
- `backend/ebpf/agent_tls_capture.c`
- `backend/ebpf/gen_tls.go`

文档：

- `docs/architecture.md`
- `docs/codex-workflows.md`
- `docs/security-model.md`

安全注意：

- 默认关闭。
- 属于高风险明文诊断能力。
- 主事件应只携带 metadata/digest。

验证：

- TLS parser/assembler tests。
- backend tests。
- 如改 eBPF TLS：`make ebpf-tls`。

## Execution Graph / AgentSight

前端：

- `frontend/src/views/execution-graph/ExecutionGraph.vue`
- `frontend/src/views/execution-graph/execution-graph.css`
- `frontend/src/views/execution-graph/useGraphFilters.ts`
- `frontend/src/views/execution-graph/useGraphWebSocket.ts`
- `frontend/src/components/execution-graph/ExecutionGraphCanvas.vue`
- `frontend/src/components/execution-graph/useDisplayGraphBuilder.ts`
- `frontend/src/components/execution-graph/useExecutionGraphHelpers.ts`
- `frontend/src/composables/execution-graph/useExecutionGraph.ts`
- `frontend/src/composables/execution-graph/useExecutionGraphRecording.ts`
- `frontend/src/types/executionGraph.ts`
- `frontend/src/components/agentsight/*`
- `frontend/src/composables/agentsight/*`
- `frontend/src/utils/agentsight.ts`

后端：

- `backend/execution_graph.go`
- `backend/event_envelope.go`
- `backend/event_context*.go`
- `backend/agentsight_handlers.go`
- `backend/agentsight_analyzers.go`
- `backend/otel_export*.go`
- `backend/otel_span_helpers.go`

常见改动：

- 新增节点/边类型。
- 修改 agent/tool/process/syscall 关联。
- 修改 AgentSight import/export/query。
- 修改 OTLP span derivation。

验证：

- `backend/execution_graph_test.go`
- `backend/event_envelope_test.go`
- `backend/agentsight_handlers_test.go`
- frontend build。

## Explorer / file browser / tracked paths

前端：

- `frontend/src/views/explorer/Explorer.vue`
- `frontend/src/components/explorer/FileBrowserPanel.vue`
- `frontend/src/components/explorer/FilePreviewDrawer.vue`
- `frontend/src/components/explorer/PathNavigatorDrawer.vue`
- `frontend/src/types/filePreview.ts`

后端：

- `backend/helpers_fs.go`
- `backend/config_handlers*.go`
- `backend/path_policy.go`

常见改动：

- 文件浏览/预览。
- 添加 tracked path。
- 路径策略 UI。

重要事实：

- `tracked_paths` 是 exact 256-byte path match，不递归。

## Executor / shell sessions / launchers

前端：

- `frontend/src/views/executor/Executor.vue`
- `frontend/src/components/executor/ExecutorLaunchEnvTab.vue`
- `frontend/src/components/terminal/LocalShellTerminal.vue`
- `frontend/src/components/terminal/RemoteWrapperTerminal.vue`
- `frontend/src/components/terminal/ShellTerminalPane.vue`
- `frontend/src/composables/executor/useShellSessions.ts`
- `frontend/src/composables/executor/useLaunchEnv.ts`
- `frontend/src/composables/executor/useCodingLauncher.ts`
- `frontend/src/composables/executor/useScriptLauncher.ts`
- `frontend/src/composables/executor/useLauncherUtils.ts`
- `frontend/src/types/shell.ts`
- `frontend/src/utils/tmux.ts`

后端：

- `backend/shell_session_*.go`
- `backend/shell_ws.go`
- `backend/launch_env.go`
- `backend/privileges.go`
- `backend/process_cleanup.go`
- `backend/uds_server.go`

常见改动：

- 新 launcher。
- shell sessions lifecycle。
- PTY attach/detach。
- wrapper terminal behavior。

安全：

- shell sessions 和 `/system/run` 受 runtime gate 控制。

## Hooks

前端：

- `frontend/src/views/hooks/Hooks.vue`
- `frontend/src/components/hooks/HookCard.vue`
- `frontend/src/components/hooks/HookConfigModal.vue`
- `frontend/src/data/hookCatalog.ts`
- `frontend/src/types/hooks.ts`

后端：

- `backend/hooks.go`
- `backend/hooks_detection.go`
- `backend/hooks_events.go`
- `backend/hooks_kiro_antigravity.go`
- `backend/config_handlers_hooks.go`

常见改动：

- 新 provider。
- 新 hook lifecycle event。
- install/uninstall UI。
- per-hook secret。

安全：

- hook install/raw writes 受 runtime gate 控制。
- relay script 依赖 `curl`。

## Config / Runtime / Security / Registry / Docs tab

前端：

- `frontend/src/views/config/Config.vue`
- `frontend/src/components/config/ConfigRuntimeTab.vue`
- `frontend/src/components/config/ConfigSecurityTab.vue`
- `frontend/src/components/config/ConfigRegistryTab.vue`
- `frontend/src/components/config/ConfigClusterTab.vue`
- `frontend/src/components/config/ConfigDocsTab.vue`
- `frontend/src/components/config/ConfigSystemHealthTab.vue`
- `frontend/src/components/config/ConfigVisualFilterTab.vue`
- `frontend/src/components/config/CoreRuleMonitorBoard.vue`
- `frontend/src/components/config/QuickCoreRulePanel.vue`
- `frontend/src/composables/config/useConfigRuntime.ts`
- `frontend/src/composables/config/useConfigSecurity.ts`
- `frontend/src/composables/config/useConfigRegistry.ts`
- `frontend/src/composables/config/useConfigCluster.ts`
- `frontend/src/composables/config/useConfigVisualFilter.ts`
- `frontend/src/composables/config/useBehaviorClassifier.ts`
- `frontend/src/types/config.ts`

后端：

- `backend/config_handlers*.go`
- `backend/runtime_state*.go`
- `backend/features.go`
- `backend/feature_helpers.go`
- `backend/bootstrap_health.go`
- `backend/cluster_*.go`

常见改动：

- 新 runtime config。
- 新 policy/registry field。
- 新 health/status card。
- security feature gate。

验证：

- proto sync if config schema changed。
- backend tests。
- frontend build。

## ML

前端：

- `frontend/src/views/ml/ML.vue`
- `frontend/src/components/config/ml/ConfigMLStatusTab.vue`
- `frontend/src/components/config/ml/ConfigMLParamsTab.vue`
- `frontend/src/components/config/ml/ConfigMLModelTab.vue`
- `frontend/src/components/config/ml/ConfigMLTrainingTab.vue`
- `frontend/src/components/config/ml/ConfigMLLLMTab.vue`
- `frontend/src/components/config/ml/useAutoTuneElapsed.ts`
- `frontend/src/components/config/ml/useModelTypeDisplay.ts`
- `frontend/src/composables/config/useConfigML.ts`
- `frontend/src/composables/config/useConfigMLDataset.ts`
- `frontend/src/composables/config/useAutoTune.ts`
- `frontend/src/composables/config/useMLStatusStream.ts`
- `frontend/src/composables/config/mlPresets.ts`
- `frontend/src/composables/config/mlUtils.ts`
- `frontend/src/data/mlModelCatalog.ts`

后端：

- `backend/ml_*.go`
- `backend/ml/*.go`
- `backend/config_ml_handlers.go`
- `backend/ml_ws.go`

文档/脚本：

- `docs/benchmark.md`
- `docs/ml-benchmark-report.md`
- `docs/ml-opening-report.md`
- `scripts/ml-report.sh`
- `scripts/ml-sweep.sh`

验证：

- backend ML tests。
- `make runtime-benchmark` 如涉及 replay。
- frontend build。

## Plugins / visual builder

前端：

- `frontend/src/views/plugins/Plugins.vue`
- `frontend/src/views/plugins/usePluginBuilder.ts`
- `frontend/src/views/plugins/usePluginList.ts`
- `frontend/src/components/plugins/*`
- `frontend/src/composables/plugins/*`

后端：

- `backend/plugins.go`
- `backend/plugin_handlers.go`
- `backend/plugin_visual_llm.go`
- `backend/plugin_visual_llm_parse.go`

关键约束：

- visual eBPF plugins 使用 `attachKind: "lsm"`。
- 仅 unlink/do_unlinkat 流程使用 `attachKind: "kprobe"`。
- 不要对非 unlink 插件序列化 `attachKind: "none"`。

## External API / Kubernetes / Deployment

后端：

- `backend/external_api.go`
- `backend/routes.go`
- `backend/server_startup.go`
- `backend/domain_forward_proxy.go`

文档/部署：

- `docs/external-api.md`
- `docs/kubernetes.md`
- `deploy/kubernetes/agent-ebpf-filter.yaml`
- `scripts/install-service.sh`

改外部行为时同步这些文档和清单。
