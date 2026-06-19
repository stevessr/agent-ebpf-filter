# 后端启动链路

当前后端主入口位于 `backend/app/main.go`。这点很重要：部分历史文档仍写 `backend/main.go`，维护时应以当前代码为准。

## Main() 启动顺序

```text
Main()
  → isBootstrapMode()
  → bootstrapTrackerMaps()
  → ensureBackendPrivileges()
  → refreshHooksPaths()
  → runtimeSettingsStore.LoadOrCreate()
  → otelExporterStore.Close() defer
  → killPreviousBackendProcesses()
  → ensureTrackerMapsLoaded()
  → runtimeSettingsStore.Snapshot()
  → newFeatureRegistry()
  → optional domainForwardProxyService.Activate()
  → TLS runtime setup
  → ringbuf.NewReader(trackerMaps.Events)
  → startKernelEventReader(rd)
  → startRuntimeBackgroundJobs(features)
  → ApplySandbox()
  → gin.Default()
  → clusterGatewayMiddleware()
  → startArchiveEvictionLoop(ctx)
  → registerRoutes(...)
  → seedDefaultTrackedCommands()
  → chooseBackendPort()
  → configureRuntimePort(actualPort)
  → optional startDeferredMLRuntime()
  → optional startDeferredPluginRuntime()
  → r.Run(:actualPort)
```

## Bootstrap 与特权

启动最先处理两件事：

1. bootstrap mode：只初始化 eBPF components 后退出；
2. 特权检查：如果当前进程能力不足，后端可通过 `sudo` / `pkexec` 自提权重启。

需要特权的原因：

- 加载 eBPF object；
- attach tracepoint / cgroup / LSM；
- pin maps / links；
- 可选绑定 80/443；
- 管理 bpffs 下的 restrictive map permissions。

## Runtime settings 加载

`runtimeSettingsStore.LoadOrCreate()` 会加载或创建运行时配置，默认位置：

```text
~/.config/agent-ebpf-filter/runtime.json
```

配置会影响：

- auth token；
- event archive 大小和过期时间；
- JSONL persistence；
- shell sessions；
- system run；
- hook management；
- policy management；
- TLS capture；
- OTLP；
- ML；
- domain forward；
- kernel risk feedback。

## eBPF 初始化

`ensureTrackerMapsLoaded()` 负责主 tracker maps 和 links 的加载 / 打开 / pinning。

主 pin 路径：

```text
/sys/fs/bpf/agent-ebpf/maps
/sys/fs/bpf/agent-ebpf/links
```

cgroup 与 LSM 有各自子目录：

```text
/sys/fs/bpf/agent-ebpf/cgroup_sandbox/maps
/sys/fs/bpf/agent-ebpf/cgroup_sandbox/links
/sys/fs/bpf/agent-ebpf/lsm_enforcer/maps
/sys/fs/bpf/agent-ebpf/lsm_enforcer/links
```

## 后台任务

`startRuntimeBackgroundJobs(features)` 启动：

- event broadcaster；
- kernel risk feedback worker；
- UDS server；
- cgroup attribution GC；
- DNS cache GC；
- TCP state tracker GC；
- flow aggregator GC；
- exfil detection loop；
- GeoIP init；
- optional cgroup sandbox loader；
- optional LSM enforcer loader。

## 端口选择

后端会在 `8080..8089` 选择可用端口，随后写入运行时端口 handoff 文件。前端 Vite dev proxy、adapters 和 hook endpoint 推导会依赖该端口。
