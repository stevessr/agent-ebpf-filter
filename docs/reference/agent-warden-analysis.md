# Agent-Warden 论文解读：内核原生进程—文件溯源与本项目的边界

> **文献性质**：外部相关研究 / 先有技术分析（2026-10-14）。**不是** Agent eBPF Filter 已实现功能说明，也不意味着引入了 Agent-Warden 源码。本文所指的是 Cui 等人的 eBPF 溯源论文，而非同名 MCP 扫描器、编程 Agent 编排器。

## 1. 论文信息与核心问题

- **论文**：Dongxu Cui、Zhichao Gu、Ping Zheng、Simeng Han、Yong Liao, *Agent-Warden: eBPF-Based Kernel-Native Process-File Provenance Tracking for LLM Agents*。
- **时间**：2026-09-29 提交到 arXiv，编号 [2609.38245](https://arxiv.org/abs/2609.38245)（cs.CR / cs.OS）。
- **问题**：Agent 的应用层 trace 通常止步于其 SDK、Harness 或工具调用；派生 shell、脚本以及随后由其他进程读取的文件可能不在这条 trace 中。因此“Agent 调用了什么工具”不等于“这些操作在主机上形成了什么后续影响”。
- **定位**：Agent-Warden 以 Linux 内核中的**进程与普通文件**为基本实体，维护保守的溯源状态，增量产生因果关联边；它不是直接理解 Prompt 意图的模型，也不是论文已证明的通用数据泄漏阻断系统。

一句话：**从逐条 syscall 记录进一步走向“谁影响了哪个文件、该文件又被哪个进程使用”的内核事实关联**。这里的“影响/因果”是按观测规则构造的 *conservative provenance*，不等于精确字节级信息流，也不等于证明恶意意图。

## 2. 工作原理：状态传播而不只是 PID 树

论文描述的关键机制可以归纳为：

| 内核生命周期或文件事件 | Agent-Warden 的溯源语义 | 阅读时必须注意 |
| --- | --- | --- |
| `fork/clone` | 子任务继承父任务的溯源状态 | 进程后代关联不同于跨文件重新进入因果链 |
| `exec` | 延续任务状态，并为进程镜像变更记录边 | 不能因为二进制名称变化就断开上下文 |
| 成功的普通文件写入 | 被跟踪的写入者给相应 inode 留下关联状态 | 失败的写入不可产生“已修改”结论；正向字节传输要求返回值大于 0 |
| 成功的普通文件读取 | 文件已有标记时，可将关联传播给原先未标记的读取任务 | 表示潜在信息依赖，而非证明具体字节被复制 |
| `rename` | 记录命名空间/路径变更边，保留 inode 上原有状态 | **rename 自身不是内容传播**，不能错误生成 read/write 边 |
| 受控子任务退出 | 在退出时保守汇聚子任务最终的溯源影响，补全短生命周期代理任务的上下文 | 会扩大可能的关联范围，因此存在过度近似（over-approximation） |

这些规则以 syscall **返回结果**约束传播：失败操作不应生成成功的因果边，rename 的成功判定为返回 0。事件在内核更新局部状态后，沿 BPF ring buffer 异步送往用户态重建图谱；**异步图构建不是执行前阻断**。

### 论文演示的文件介导链

论文使用受控样例：Agent 启动脚本写入 `intermediate_config.tmp`；`mv` 重命名该文件；独立的主机监视脚本稍后调用 `cat` 读取重命名后的文件，并产生 `monitor.log`。这种跨独立进程的文件介导关联，在论文选取的应用层 Langfuse trace 中没有被完整还原。

~~~mermaid
flowchart LR
    A["Agent 发起的脚本"] -->|"成功写入"| F["普通文件 inode：溯源标记"]
    F -.->|"mv / rename：名称变化，不传播内容"| N["同一 inode 的新路径"]
    N -->|"独立进程成功读取"| P["监视脚本 / cat"]
    P -->|"后续写入"| L["monitor.log"]
~~~

这是**受控场景的溯源展示**，不能外推为任意 Agent、任意文件系统、跨主机网络行为均已被完整追踪。

## 3. 两套内核状态后端

| 后端 | 论文描述 | 工程取舍 |
| --- | --- | --- |
| **Warden-Hash** | 使用 PID 为键的 eBPF hash map 保存任务状态，以兼容缺乏 BPF local storage 支持的内核 | 需要严谨处理生命周期、PID 重用、map 容量与垃圾状态；具体处理情况须以原实现为准，不能凭后端名字假定已消除全部风险 |
| **Warden-Local** | 将状态关联到 task 和 inode 的 BPF local storage，回收与对应内核对象生命周期绑定 | 有利于降低独立状态回收的复杂度，但依赖目标内核/BTF/helper 和挂载能力；部署时应逐项做能力探测 |

论文在 x86-64 与 ARM64 裸机、Linux 6.8 的实验场景评估了两套后端；这不构成其他内核版本和发行版上即插即用的保证。

## 4. 实验结果与解释边界

论文报告的受测工作负载包含重复 **1,000 次**的合成 Agent 行为原语周期（进程创建、命令、文件读写及重命名）：

- **端到端运行时间开销：0.2%–3.5%**。
- **额外系统 CPU 时间：0.6%–3.7%**。
- 论文描述在两种硬件架构上进行重复测量。这里是**论文自身受控负载**的观测值，不是本项目的 benchmark，不能未经同机同工况对照就与本仓库的 syscall 微基准直接排名。
- 示例证明“某条先前在应用 trace 中缺失的跨进程文件链在其受控条件下被重建”，**不等于**大规模真实攻击数据集的召回率、误报率或主动阻断成功率已被证明。

尤其不能把“观察到 `open/read`”直接称为“密钥已经外传”。若需证明外传，还要链接具体读取、可归因的出站网络事实和结果；若需证明阻断成功，还要观测策略判定及操作失败的可核验证据。

## 5. 与 Agent eBPF Filter 的异同

下面仅依据**当前仓库文档所描述的能力**比较，不宣称完成了逐行审计或独立复现实验。

| 维度 | Agent-Warden（论文） | Agent eBPF Filter（当前） | 应避免的错误归因 |
| --- | --- | --- | --- |
| 目标 | 进程—普通文件溯源与跨进程因果关联 | Agent 行为观测、语义归因、策略控制和 Renew 日常界面 | 两者均使用 eBPF 不代表技术贡献相同 |
| 上下文来源 | 内核任务与文件的状态传播 | PID/父子关系 + adapter/native hook/wrapper 的 `agent_run_id`、`tool_call_id`、`trace_id` | Agent 语义元数据不能取代 inode 级事实 |
| 文件关联 | 成功读写驱动 inode 状态传播，重命名保持 inode 关联，退出时可保守汇聚 | 既有 syscall/event 图谱；跨 Agent 文件修改关联是**路径级、带时间窗的启发式** | 本项目现有 `MULTI_AGENT_FILE_CONTENTION` **不是** Agent-Warden 式文件溯源 DAG |
| 状态后端 | PID hash 或 task/inode local storage 两种方案 | 项目自身 BPF maps 与用户态上下文关联 | **未实现/未验证**等价的 Warden-Local、inode 污点传播与退出汇聚 |
| 输出 | 增量因果边 → 用户态异步图重建 | ringbuf → Go `EventEnvelope` / protobuf → 查询、界面和导出 | 图显示了边，不代表边具有经验证的因果证明等级 |
| 防护 | 此论文重点是观测和可重建性 | wrapper / cgroup BPF / BPF LSM 提供独立策略路径 | 不能把 Warden 的观测论文写成已提供相同内核阻断 |

可进一步阅读：[事件管线](/backend/event-pipeline)、[数据流](/architecture/data-flow)、[运行时安全框架对照](/security/runtime-framework-comparison-2026)、[策略语义](/security/policy-semantics)。特别注意现有文件争用检测明确不验证 inode 身份；相同路径名不保证指向同一文件。

## 6. 可借鉴的后续研究路线（**规划，不是已实现**）

1. **稳定对象身份先行**：为进程采用可校验的启动时间、PID namespace/boot ID 组合而非裸 PID；为普通文件引入设备号、inode、mount namespace 等有范围约束的标识。应特别验证 PID 重用、rename、hard link、bind mount 与容器隔离。
2. **溯源边与证据强度分层**：分开存储 `process_spawn`、`exec`、`file_read`、`file_write`、`file_rename`、`process_exit` 的成功事实和“影响传播”的推断边；每条边附带来源、成功状态、时间、丢包标记与是否跨边界。
3. **内核最小工作、用户态有界构图**：BPF 只做验证器允许的定长状态更新与事件发射；Go 用户态限制图窗口、内存与边数，处理 ringbuf 溢出、乱序和无法恢复的缺口。不能因丢包仍对完整因果链作确定性声称。
4. **语义层交叉核验**：以已有 native hooks / PID registration 连接任务 `run_id` 与溯源子图；无合法上下文时降级为“系统级观察”，不把未知进程强行归给某个 Agent。
5. **检测与执行控制解耦**：图关联用于离线复盘和风险解释；若将来联动 LSM/cgroup，必须单独进行误阻、策略回滚、TOCTOU 与同步决策性能验证，默认不将推断边直接转化为自动阻断。

### 建议验收场景

| 场景 | 主要验证指标 |
| --- | --- |
| Agent → 脚本写文件 → rename → 独立进程 read | 文件身份正确率、端到端链恢复率、缺失边率 |
| 子进程快速退出、PID 重用、多层 exec | 误关联率、状态泄漏、生命周期覆盖 |
| 失败 read/write、零字节 write、跨容器同路径 | 误传播次数（理想为零）、边证据完整度 |
| 并发负载与 ringbuf 丢事件 | 丢失占比、降级标记准确性、P95 延迟、峰值内存 |
| 相同工作负载在 x86-64/ARM64 与目标内核 | 端到端开销和 system CPU 开销；仅与自家基线比较 |

## 7. 学术归属与引用方式

Agent-Warden 已在 2026-09-29 公开描述内核原生进程—文件溯源、两套状态后端、退出汇聚与异步图重建。后续比赛或论文应**明确列为先有技术**。本项目若完成“意图—进程—文件—跨 Agent—策略反馈”闭环，创新主张应落在有实证支撑的语义协同、安全治理、可解释性和用户体验上，而非声称首次用 eBPF 做 Agent 文件溯源。

本文仅分析公开论文并提出独立工程建议，**未复制或移植论文的源码/图件**。具体实现及许可证以实际发布的仓库和授权文件为准。

**参考来源**

1. Cui, D., Gu, Z., Zheng, P., Han, S., & Liao, Y. (2026). *Agent-Warden: eBPF-Based Kernel-Native Process-File Provenance Tracking for LLM Agents*. arXiv:2609.38245. https://arxiv.org/abs/2609.38245
2. 本项目：[Runtime security frameworks review](/security/runtime-framework-comparison-2026)（目前文件争用规则的证据边界）；[AgentSight 致谢](/reference/agentsight-acknowledgment)（另一条 Agent 内核观测相关工作）。
