# 🗺️ 图表与示例索引

本页作为全站的**全局导航枢纽**，集成了分布在各个关键章节中的 Mermaid 架构图、时序图、数据流向以及代码/配置示例，旨在帮助开发者快速定位技术蓝图与实现样本。


## 📐 1. 架构与流程图表索引

### 🧱 总体架构与数据流

#### [架构总览](/architecture/overview)

* 🗺️ **系统级总架构图**：全面透视用户空间（Agent/CLI）、内核空间（eBPF/LSM）与表现层（Vue）的宏观联动。
* 🥞 **L0-L5 分层视图**：垂直解构从裸金属内核、安全阻断层到数据归一化、外部协议集成的六层架构。
* 🔌 **组件交互与依赖关系**：明晰核心 Go 后端与外部微服务、底层的拓扑依赖。

#### [数据流向拓扑](/architecture/data-flow)

* ⏱️ **eBPF 事件流时序图**：追踪系统调用从内核 `ringbuf` 异步上报至 Go 内存零拷贝解码的完整生命周期。
* 🛡️ **Wrapper 策略流时序图**：拦截 AI CLI 进程，实施同步阻断或动态改写的上下游握手流。
* 🪝 **Native Hook 流时序图**：应用层原生钩子（Hooks）主动上报工具语义与 Payload 的时序链。
* 🆔 **PID Registration 流时序图**：Agent 启动时向系统注册双向信任树的认证流。
* ⚙️ **前端配置流组件图**：Vue Workbench 与后端配置同步、热加载落盘的拓扑结构。
* 📦 **导出流组件图**：多源并发事件流向 JSONL、PCAP、Prometheus、OTLP Spans 的分流管线。


### 🚀 后端、内核与安全模型

#### [后端启动链路](/backend/runtime-startup)

* ⚙️ **`Main()` 启动流程图**：后端主引擎初始化、参数解析与退化机制的确定性状态机。
* 🔑 **Bootstrap 与特权提升时序图**：系统特权检查（CAP_SYS_ADMIN/CAP_MAC_ADMIN）与安全性校验。
* 🐙 **eBPF 初始化依赖图**：BTF 加载、Maps 创建、Links 固定（Pinning）的拓扑先后依赖。
* 📝 **配置实例**：`Runtime Settings JSON` 线上动态生效样本。
* 🤖 **后台任务**：核心异步审计任务的调度伪代码。

#### [安全模型与防御纵深](/security/model)

* 🛡️ **五层安全模型总图**：涵盖环境层、特权层、认证层、门控层、执行层的纵深防御矩阵。
* 👮 **权限层时序图**：基于 `CAP check` 与 `sudo` 行为的动态安全隔离。
* 🎫 **认证层时序图**：基于 `Runtime Access Token` 的 WebSocket 与 MCP 越权校验流。
* 🌳 **Runtime Gate 决策树**：多维条件（Shell 敏感度、明文拦截开关等）并发评估的树状引擎。
* 🔄 **内核控制层状态机**：由 Wrapper、cgroup、BPF LSM 联合驱动的异步控制与阻断状态流转换。
* 🧪 **数据保护层流水线**：敏感数据本地脱敏（Redaction）与哈希摘要计算的管道。


### 🔌 外部集成与前端

#### [Wrapper 命令策略](/integrations/wrapper)

* 🧠 **Wrapper 执行时序图**：挂钩机器学习模型，进行运行时风险评分（ML Risk Scoring）的全链路时序。
* 🚥 **决策类型流程图**：`ALLOW`（放行）/ `BLOCK`（阻断）/ `ALERT`（告警）/ `REWRITE`（动态参数改写）的分支判定逻辑。
* 🛠️ **代码与配置**：包含元数据提取（Metadata Extraction）代码示例、Wrapper Rule 配置 JSON 样例以及安全边界依赖拓扑。

#### [前端工作台总览](/frontend/workbench)

* 📐 **技术栈依赖图**：Vue 3、Pinia、Vite 与组件库的核心依赖蓝图。
* 📂 **目录分层架构图**：工作台组件、资产、状态机与通信层的清晰目录树解析。
* 🗺️ **工作台页面分类图**：Dashboard、Monitor、Network、Explorer 等页面的职能边界图。
* 🌊 **设计原则数据流图**：高频数据下的前端零防抖渲染与视窗虚拟化逻辑。
* ⏱️ **典型数据流时序图**：Dashboard 接收大并发 WebSocket 实时事件流的高能渲染时序。


## 💻 2. 代码与配置示例索引

### 🐹 Go 后端代码核心片断

| 位置 | 核心示例内容 | 技术要点 |
| --- | --- | --- |
| **[后端启动链路](/backend/runtime-startup)** | 动态端口自动选择机制、Runtime Settings 热加载逻辑 | 解决端口冲突、并发安全配置锁 |
| **[数据流](/architecture/data-flow)** | Kernel Ringbuf 解码零拷贝（Zero-copy View）实现 | `mmap` 对齐、Native-endian 高效转换 |
| **[Wrapper](/integrations/wrapper)** | 命令行元数据（Metadata）提取、`argv digest` 哈希计算 | 参数防篡改、轻量级快照生成 |

### 📄 JSON 结构化配置样本

| 位置 | 配置对象名称 | 主要用途 |
| --- | --- | --- |
| **[后端启动链路](/backend/runtime-startup)** | `runtime.json` 完整工程配置 | 统筹运行时网络端口、存储路径、安全等级、遥测网关 |
| **[Wrapper](/integrations/wrapper)** | `wrapper-rules.json` 规则拦截字典 | 声明敏感命令、阻断阈值、黑白名单与参数改写模板 |

### 🐚 Bash 效能与构建命令

| 位置 | 命令集合 | 执行目标 |
| --- | --- | --- |
| **[快速开始](/guide/quick-start)** | `make predev` / `make dev` / `make all` | 并行拉取依赖、全栈热加载启动、端到端全量打包 |
| **[构建与运行](/operations/build-and-run)** | eBPF 内核构建编译命令、LSM 挂载命令 | `clang` 编译内核态 C 代码并输出 BPF 字节码 |


## 🛠️ 3. Mermaid 图表规范与标准

本站的所有技术图表均基于标准 **Mermaid** 引擎构建，在 VitePress 运行环境中支持交互式缩放与响应式渲染：

::: info 📊 视窗图表分类标准

* 🟢 **`graph TB / LR` (流程拓扑)**：专门用于系统组件解耦、宏观层级划分、目录分层及依赖分类。
* 🟣 **`sequenceDiagram` (交互时序)**：专门用于跨层级（例如内核态到应用态、后端到前端）的高并发异步事件传输与授权握手流程。
* 🔴 **`flowchart TD` (分支控制)**：专门用于安全门控决策、内核阻断状态机切换以及动态改写的逻辑流。
:::


## 🎯 4. 开发者阅读导引建议

根据你在项目开发中所扮演的角色，我们建议你重点攻克对应的图表组合：

* 🚀 **新进贡献者**：优先研读 [架构总览](/architecture/overview) 宏观图与 [数据流向拓扑](/architecture/data-flow) 系统调用时序图。
* 💻 **后端与内核开发者**：紧密锁定 [后端启动链路](/backend/runtime-startup)、初始化依赖图以及 Ringbuf 零拷贝解码的 Go 代码片段。
* 🎨 **前端与可观测性开发者**：深入分析 [前端工作台总览](/frontend/workbench) 目录架构图和 WebSocket 实时高频渲染时序图。
* 🛡️ **安全审计官与红蓝对抗专家**：重点拆解 [安全模型与防御纵深](/security/model) 的五层架构总图、LSM 状态机以及 Gate 决策树。
* 🔌 **外围生态集成商**：关注 [Wrapper 命令策略](/integrations/wrapper) 的执行时序、JSON 字典及 PID 注册时序。


## 📋 5. 后续图表扩展路线图

以下架构与流水线图表正在设计中，将在后续迭代版本中逐步补充：

* [ ] **Execution Graph 构建算法流程图** —— 深度解析因果网络节点合并与裁剪算法。
* [ ] **Network Flow 流量异步聚合逻辑图** —— 揭秘四层流控向七层应用协议（HTTP/DNS）富化的状态机。
* [ ] **ML 本地异常训练与推理管线图** —— 展现流式特征工程、数据窗口提取到实时 Scoring 的闭环。
* [ ] **Plugin 动态编译与热加载流程图** —— 内核态 eBPF 插件动态编译、注入与安全退出的机制。
* [ ] **Cluster 集群分布式拓扑图** —— 多节点心跳同步、集中式策略管控与大面积流转发拓扑。
* [ ] **OTLP Span 泛化与派生规则图** —— 离散 Syscall 事件如何优雅降噪并折叠为标准追踪 Span 树的演算逻辑。