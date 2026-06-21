# 🌐 Network Flow（网络流分析）

Network Flow 页面用于可视化展示 AI Agent 的所有网络行为。由于 AI Agent 有可能被提示词注入攻击（Prompt Injection）并尝试外发敏感凭证（如访问恶意 C2 服务器或发起网络逃逸），网络行为的精准观测与控制是系统纵深防御的重要一环。

---

## 1. 核心观测面

网络流分析页面包含三大核心观测板块：

### 1.1 实时网络连接列表 (Network Flow Table)
* 记录每一次网络系统调用（`connect`, `sendto`, `bind`）的物理连接事实。
* 展示字段包括：
  * **发起进程**：进程名（comm）、PID、父进程关系。
  * **源地址 / 目的地址**：源 IP 和端口、目的 IP、目的端口及传输协议（TCP/UDP）。
  * **拦截状态**：展示当前连接是否已被内核态 `cgroup sandbox` 过滤硬阻断（显示为 `BLOCKED` 或 `ALLOWED`）。

### 1.2 DNS / SNI 元数据富化 (DNS & SNI Enrichment)
* 后端通过捕获 DNS 解析流量，自动在内存中富化目的 IP 的关联域名（DNS Name）。
* 对于 HTTPS 加密流量，解析 TLS 握手报文中的 Client Hello 字段，提取服务器名称指示（SNI），在前端清晰呈现具体的访问域名，帮助审计人员快速识别第三方 API 或外部未授权服务的访问。

### 1.3 实时流量网络拓扑图 (Traffic Graph)
* 使用 D3.js 绘制节点拓扑图，直观展现进程与外部网络节点（IP 或域名）之间的连通关系与数据吞吐趋势。

---

## 2. 内核态网络策略干预

用户可以通过 Network 页面对异常流量进行实时干预：
1. **直接阻断 IP / 端口**：在流量条目右侧点击“Block”按钮。
2. **策略下发**：前端调用后端 API，将该 IP/端口或 cgroup ID 写入内核的 pinned eBPF Maps。
3. **内核态即时截断**：后续的 connect 或 sendmsg 系统调用将在内核直接被拒，并在 Dashboard 和 Network 页面上报 `BLOCKED` 状态。

---

## 🔗 相关导航

- [🎨 前端工作台总览](workbench.md) —— 前端技术栈与组件分层
- [📊 Dashboard 事件流](dashboard.md) —— 系统事件流瀑布
- [🛡️ 策略语义与拦截机制](../security/policy-semantics.md) —— cgroup 内核拦截细节
- [🔌 MCP、External API 与 OTLP](../integrations/mcp-external-otlp.md) —— 遥测指标导出
