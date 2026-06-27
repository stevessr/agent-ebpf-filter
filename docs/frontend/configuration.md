# ⚙️ Configuration（系统配置与策略中心）

Configuration 页面是 **Agent eBPF Filter** 的中央控制台，负责对后端运行期配置进行调整、管理安全阻断规则、监控系统健康状态以及维护 AI 客户端注册信任列表。

---

## 1. 核心配置面

配置中心主要分为以下五个功能标签页（Tabs）：

### 1.1 运行时门控 (Runtime Gates)
* **动态特性开关**：
  * **Shell Sessions Enabled**：控制是否允许前端拉起交互式 PTY。
  * **System Run Enabled**：控制是否启用后端 API 任意命令执行。
  * **TLS Capture Enabled**：一键开启或关闭可选的 TLS 明文捕获高风险诊断模块。
  * **Policy Management Enabled**：控制安全策略的读写热更新。
  * **OTLP / Prometheus Enabled**：遥测导出与标准指标监控接口开关。

### 1.2 安全策略规则库 (Security Policies)
* 提供可视化规则编辑器，对以下两类内核阻断策略进行维护：
  * **网络规则 (cgroup network blocks)**：支持手动输入目标 IP 和端口，或者指定特定的 cgroup 进程切片进行网络四层流量硬切断。
  * **文件/执行规则 (BPF LSM blocks)**：添加被拦截的可执行路径（如 `/bin/ncat`）或者文件 basename（如 `id_rsa`）。

### 1.3 数据脱敏中心 (Redaction & Privacy)
* 快速选择脱敏级别：`None` / `Basic` / `Standard` / `Strict`。
* 配置全局敏感字段模式（如 API Token 字段名正则）。
* 支持设置差异化出口可见性（如仅在日志中脱敏，而在 WS 实时通道中保留，或者反之）。

### 1.4 AI 客户端信任注册列表 (Agent Registry)
* 显示当前正在运行并已通过 PID 注册接口向后端报备的合法 AI Agent 进程树及其运行元数据（如 `agent_run_id`, `tool_call_id`）。
* 支持管理员在界面上撤销特定进程的信任标记，将其移出白名单并触发安全警报。

### 1.5 系统健康诊断 (System Health)
* 展示 eBPF 探针的运行状态（事件接收速率、丢失速率、环形缓冲区使用率）。
* 展示 Go 后端资源占用、CUDA 加速显存状态（若启用 kernel-ml）以及 Systemd 守护进程状态。

---

## 相关导航

- [🎨 前端工作台总览](workbench.md) —— 整体架构与微服务划分
- [🛡️ 安全模型](../security/model.md) —— Runtime Gate 五层控制原理
- [🔑 Runtime Gates 与认证机制](../security/runtime-gates-auth.md) —— API Token 安全细节
- [🛠️ 故障排查与常见问题指南](../operations/troubleshooting.md) —— 配置修改生效故障排查
