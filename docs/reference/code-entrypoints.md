# 代码入口索引

## 后端

| 领域 | 入口 |
| --- | --- |
| 启动 | `backend/app/main.go` |
| 路由 | `backend/app/routes__routes.go` |
| runtime settings | `backend/core/state_types.go` |
| feature manifest | `backend/app/feature_manifest.go` |
| ringbuf jobs | `backend/app/runtime__jobs_background.go` |
| eBPF runtime | `backend/app/runtime__runtime_ebpf.go` |
| event context | `backend/app/events__context_event.go` |
| network events | `backend/app/events__events_network.go` |
| network flows | `backend/app/events__event_flows.go` |
| execution graph | `backend/app/events__graph_execution.go` |
| hooks | `backend/app/hooks__hooks.go` |
| registration | `backend/app/handlers__handlers_registration.go` |
| config | `backend/app/handlers__handlers_config.go` |
| system | `backend/app/handlers__handlers_system.go` |
| ML | `backend/app/ml__*.go`、`backend/ml/` |
| plugins | `backend/app/handlers__handlers_plugin.go` |

## eBPF

| 领域 | 入口 |
| --- | --- |
| main tracker | `backend/ebpf/agent_tracker.c` |
| common structs | `backend/ebpf/agent_tracker_common.h` |
| syscall helpers | `backend/ebpf/agent_tracker_syscalls.h` |
| tail calls | `backend/ebpf/agent_tracker_tail.h` |
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
| dashboard | `frontend/src/views/dashboard/Dashboard.vue` |
| monitor | `frontend/src/views/monitor/Monitor.vue` |
| network | `frontend/src/views/network/*.vue` |
| execution graph | `frontend/src/views/execution-graph/ExecutionGraph.vue` |
| explorer | `frontend/src/views/explorer/Explorer.vue` |
| executor | `frontend/src/views/executor/Executor.vue` |
| hooks | `frontend/src/views/hooks/Hooks.vue` |
| ML | `frontend/src/views/ml/ML.vue` |
| plugins | `frontend/src/views/plugins/Plugins.vue` |
| config | `frontend/src/views/config/Config.vue` |

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
