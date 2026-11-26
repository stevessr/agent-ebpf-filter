# 全系统 14×14 公理设计矩阵（第五轮）

基线：`86357e5a912b3a4b294c9cf4b94474fbc3481bd7`。前置：[全系统审查](mit-axiomatic-design-system-review.md)、[动态反馈](mit-axiomatic-design-feedback.md)。本报告逐格填写 196 个当前结构单元和 196 个目标契约单元，矩阵数据、理由、证据及计算均可复现。**没有真实运行时干预数据，数值 Jacobian 仍全部未测。**

## 1. FR/DP 编号及读法

行 F01–F14 对应功能结果，列 D01–D14 对应上一轮同编号设计参数族。每列是一组设计选择，非已证明独立的单一可调旋钮；测量时必须进一步固定一个干预、工作负载、模式、能力和指标，不能把一次变更的影响外推至整列。

| ID | 功能/设计域 |
| --- | --- |
| F01 / D01 | 内核采集 |
| F02 / D02 | 完整性与时间 |
| F03 / D03 | Agent 归属 |
| F04 / D04 | 捕获/监控范围 |
| F05 / D05 | 风险检测 |
| F06 / D06 | 策略执行 |
| F07 / D07 | 认证授权 |
| F08 / D08 | 研究与评估 |
| F09 / D09 | 隐私与凭据 |
| F10 / D10 | 组件通信与隔离 |
| F11 / D11 | 证据呈现 |
| F12 / D12 | 生命周期恢复 |
| F13 / D13 | 性能与资源 |
| F14 / D14 | 交付兼容 |

现状符号：D = 对角主责任映射（未证实达标）；X = 核查到结构连接（非非零灵敏度实测）；W = Wrapper 路径适用时；C = 内核风险反馈适用且获授权时；B = W 或 C；I = opt-in IPC 实验；H = 待验证因果/资源/权限影响；? = 证据不足，不能当成零。X 也可能有正常运行的局部门控（例如 monitor、build gate）。这些模式是分析投影，不宣称存在一个开关可实现所有组合。I 不代表当前生产使用 IPC。

目标符号：D = 主责任；A = 明确允许的结构依赖；W/C/B/I = 对应条件下允许；Z = 在冻结支持范围内应隔离到预设容差，**是要求而非已实测零**。不支持、模式不同、主动策略改变行为分布均需另分层。目标 A 也不表示任意跨域影响都被允许，例如 D07 对 F06 仅允许合法授权限制。

## 2. 现状结构矩阵

| FR \\ DP | D01 | D02 | D03 | D04 | D05 | D06 | D07 | D08 | D09 | D10 | D11 | D12 | D13 | D14 |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| F01 | D | ? | W | ? | X | H | W | ? | ? | H | ? | ? | H | H |
| F02 | X | D | ? | H | ? | H | ? | ? | ? | ? | ? | H | H | ? |
| F03 | X | ? | D | ? | W | ? | W | ? | ? | ? | ? | X | ? | ? |
| F04 | X | ? | X | D | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? |
| F05 | X | ? | X | X | D | ? | ? | H | X | ? | ? | X | H | X |
| F06 | C | ? | H | H | B | D | X | H | ? | ? | ? | C | C | X |
| F07 | ? | ? | ? | ? | ? | H | D | ? | ? | ? | ? | X | ? | X |
| F08 | X | ? | X | X | W | ? | ? | D | X | ? | ? | X | H | ? |
| F09 | X | ? | ? | X | ? | ? | X | ? | D | ? | X | X | ? | ? |
| F10 | ? | ? | ? | ? | ? | ? | H | ? | ? | D | ? | ? | I | H |
| F11 | X | X | X | X | X | H | X | H | X | H | D | X | X | X |
| F12 | ? | ? | ? | ? | ? | ? | ? | ? | ? | I | ? | D | H | X |
| F13 | H | H | ? | H | H | ? | ? | H | H | I | H | H | D | H |
| F14 | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | D |

计数：D=14；?=103；W=5；X=37；H=30；C=3；B=1；I=3（合计 196）。每个非默认非对角单元的证据和限定理由见 [机器可读矩阵](../../scripts/axiomatic_design_matrix.json)。? 未填直接依赖；这不是遗漏，也不是零。

特别注意：D05→F06 为 B 而非仅 C：Wrapper 直接使用模型/规则融合并形成执行决策，关闭内核反馈不会自动消除 Wrapper 路径的影响。D03→F01 为 W，具体是 Wrapper 注册跟踪对象，不能由此声称固定范围内的采集召回一定变化。D09→F05 为 X 只限后置 loop/signal 等消费脱敏记录的路径，不涵盖前置风险/语义检测。

## 3. 目标契约矩阵（建议，尚未实现/验证）

| FR \\ DP | D01 | D02 | D03 | D04 | D05 | D06 | D07 | D08 | D09 | D10 | D11 | D12 | D13 | D14 |
| --- | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: | :---: |
| F01 | D | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z |
| F02 | A | D | Z | Z | Z | Z | Z | Z | Z | Z | Z | A | Z | Z |
| F03 | A | Z | D | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z |
| F04 | A | Z | A | D | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z |
| F05 | A | Z | A | A | D | Z | Z | Z | Z | Z | Z | Z | Z | Z |
| F06 | C | Z | B | Z | B | D | A | Z | Z | Z | Z | Z | Z | Z |
| F07 | Z | Z | Z | Z | Z | Z | D | Z | Z | Z | Z | Z | Z | Z |
| F08 | A | Z | A | A | A | Z | Z | D | A | Z | Z | Z | Z | Z |
| F09 | Z | Z | Z | Z | Z | Z | A | Z | D | Z | Z | Z | Z | Z |
| F10 | Z | Z | Z | Z | Z | Z | Z | Z | Z | D | Z | Z | Z | Z |
| F11 | A | A | A | A | A | A | A | Z | A | I | D | A | Z | Z |
| F12 | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | D | Z | Z |
| F13 | Z | Z | Z | Z | Z | Z | Z | Z | Z | I | Z | A | D | Z |
| F14 | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | Z | D |

计数：D=14；Z=152；A=25；C=1；B=2；I=2（合计 196）。Z 是设计团队必须证明的隔离要求；若某一 Z 被实测拒绝，要么改造 DP，要么明确变更 FR/适用范围并重新验证，不能直接将其涂为 A。

F01 的固定采集范围覆盖与 F04 的动态 scope/注册必须分开，D03 变更不应在相同声明范围下改变 F01 的完整性验收。D05 与 D09 的顺序区分内部可信分析、脱敏后研究视图；若研究任务依赖被移除的字段，须定义不可用/降级，不能默默补造真值。F06 拆成静态控制与授权动态反馈，B 边只允许明确授权模式下的候选效应，不允许模型更改手工规则或扩大授权。资源预算在固定 SLA/能力内验证 Z；饱和实验单独报告降级、不可用与损失，不保证物理资源无限。

## 4. 图计算与敏感性

对非对角结构边 j→i，计算 SCC（仅显示非平凡分量）以及满足所有先后约束的共同 FR/DP 重排数。14! = 87178291200，使用 bitmask DP 精确计数，不需要穷举所有排列。此图只反映声明的边：既不把未知当零，也不将图循环等同于已实测耦合。

| 模式 | 现状：边数 / SCC / 排列数 | 加入全部 H：边数 / SCC / 排列数 | 目标：边数 / SCC / 排列数 |
| --- | --- | --- | --- |
| observe | 37 / {F01,F02,F03,F04,F05,F09,F11} / 0 | 67 / {F01,F02,F03,F04,F05,F06,F07,F08,F09,F10,F11,F12,F13} / 0 | 25 / 无 / 9075066 |
| wrapper | 43 / {F01,F02,F03,F04,F05,F09,F11} / 0 | 73 / {F01,F02,F03,F04,F05,F06,F07,F08,F09,F10,F11,F12,F13} / 0 | 27 / 无 / 3332966 |
| feedback | 41 / {F01,F02,F03,F04,F05,F09,F11} / 0 | 71 / {F01,F02,F03,F04,F05,F06,F07,F08,F09,F10,F11,F12,F13} / 0 | 28 / 无 / 3332966 |
| combined | 46 / {F01,F02,F03,F04,F05,F09,F11} / 0 | 76 / {F01,F02,F03,F04,F05,F06,F07,F08,F09,F10,F11,F12,F13} / 0 | 28 / 无 / 3332966 |
| experimental_ipc | 40 / {F01,F02,F03,F04,F05,F09,F11}; {F10,F13} / 0 | 70 / {F01,F02,F03,F04,F05,F06,F07,F08,F09,F10,F11,F12,F13} / 0 | 27 / 无 / 5479082 |

- observe 目标示例顺序：D01 → D03 → D04 → D05 → D07 → D06 → D09 → D08 → D10 → D12 → D02 → D11 → D13 → D14
- wrapper 目标示例顺序：D01 → D03 → D04 → D05 → D07 → D06 → D09 → D08 → D10 → D12 → D02 → D11 → D13 → D14
- feedback 目标示例顺序：D01 → D03 → D04 → D05 → D07 → D06 → D09 → D08 → D10 → D12 → D02 → D11 → D13 → D14
- combined 目标示例顺序：D01 → D03 → D04 → D05 → D07 → D06 → D09 → D08 → D10 → D12 → D02 → D11 → D13 → D14
- experimental_ipc 目标示例顺序：D01 → D03 → D04 → D05 → D07 → D06 → D09 → D08 → D10 → D12 → D02 → D11 → D13 → D14

解释：图有环说明在给定结构约束下不存在同时重排成严格下三角的排列；没有环仅说明本图可以拓扑排序。H 全部激活是压力敏感性假设，不是统计概率。? 任一新边仍可能改变结果。目标是设计契约图，不是已经满足独立公理的证书；静态三角图不能替代动态时序/控制安全验证。

## 5. 逐列证据账本

以下只列显式非对角项，其余现状 ?、目标 Z 的共同解释见第 1 节。各证据锚点固定于 baseline；E22 为 PR 分析补充，非 baseline 源码。证据表行号为核查入口，非完整调用图证明。

### D01 内核采集

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F02 | X | E01 E03 | 采集路径提供序号与时钟输入 |
| F03 | X | E01 E04 E06 | 事件供 PID/父子/cgroup 归属 |
| F04 | X | E01 E06 | 事件输入 scope admission |
| F05 | X | E01 E02 | 原始事件进入同步评分 |
| F06 | C | E02 E09 | 内核风险反馈启用时，采集提供控制候选输入 |
| F08 | X | E06 E07 | 记录进入研究队列（监控开启） |
| F09 | X | E06 E07 | 采集内容进入脱敏出口；不证明隐私指标变化 |
| F11 | X | E06 E07 | 原始事实成为归档和呈现输入 |
| F13 | H | E01 E13 | 采集速率/样本成本可能改变资源预算 |

### D02 完整性与时间

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F11 | X | E03 E07 | 时钟及序号字段随记录进入呈现 |
| F13 | H | E03 | 审计同步成本待测 |

### D03 Agent 归属

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F01 | W | E08 | Wrapper 归属/注册路径写 agent_pids 和 tracked comms；改变声明采集范围，非固定范围召回实测 |
| F04 | X | E04 E05 | 归属 root 参与 scope |
| F05 | X | E04 E06 | 语义检测使用归属后事件 |
| F06 | H | E04 E08 | 主体身份是否改变授权执行需专项验证 |
| F08 | X | E06 E07 | 研究记录包含归属上下文 |
| F11 | X | E04 E07 | 归属字段进入记录 |

### D04 捕获/监控范围

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F02 | H | E01 E06 | 需区分早期过滤与后置 scope，原始账本影响待测 |
| F05 | X | E06 | monitor 门控语义分析 |
| F06 | H | E06 E09 | 后置 scope 不覆盖早期反馈；明确反馈授权语义待测 |
| F08 | X | E06 E07 | capture/monitor 决定研究队列输入 |
| F09 | X | E06 E07 | capture 决定进入保护/发布的记录集合 |
| F11 | X | E06 E07 | capture 决定普通证据可见范围 |
| F13 | H | E06 E07 | 分析负荷随监控范围变化，未测 |

### D05 风险检测

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F01 | X | E01 E02 E03 | 风险计算在采集 reader 热路径内；吞吐影响未量化 |
| F03 | W | E08 | Wrapper 风险/decision 参与构造 process context；不等于 PID 真实性已改变 |
| F06 | B | E08 E09 | Wrapper 决策融合或内核反馈任一开启可形成控制路径 |
| F08 | W | E08 | Wrapper 分类和异常分数进入训练样本/历史；不等于独立真值 |
| F11 | X | E06 E07 | 语义告警及风险注释进入记录 |
| F13 | H | E02 E08 E18 | 检测成本及资源竞争待测 |

### D06 策略执行

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F01 | H | E09 E20 | 策略改变后续行为分布；固定尝试集合下采集影响未测 |
| F02 | H | E09 E20 | 阻断事件/序号账本关联待测 |
| F07 | H | E09 E10 | 撤销与授权代际是否一致待测 |
| F11 | H | E09 E07 | 实际 map 生效与证据关联仍待核查 |

### D07 认证授权

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F01 | W | E08 | Wrapper 授权后请求可注册跟踪对象 |
| F03 | W | E08 | UID 验证后处理声明 PID 与上下文 |
| F06 | X | E09 E10 E23 | 策略 API gate 和 feedback broad gates 影响控制许可 |
| F09 | X | E10 | 认证限制配置/数据出口；最小输出仍待验证 |
| F11 | X | E10 E23 | 路由认证影响证据访问 |
| F10 | H | E10 E21 | HTTP 与实验 IPC 的主体域不同，不能假定同一授权语义 |

### D08 研究与评估

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F05 | H | E15 E18 | 离线评估到已部署模型需要独立发布；未证实直接变更 |
| F06 | H | E15 E09 | 回放应保持只读，训练/发布到控制的边界需负例 |
| F11 | H | E15 | 研究导出和实时 UI 的一致性未核查 |
| F13 | H | E07 E15 | 研究负载可能竞争资源 |

### D09 隐私与凭据

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F05 | X | E07 E17 | 部分 loop/signal 分析消费已脱敏记录；不等于早期风险/语义评分受脱敏影响 |
| F08 | X | E07 E17 | 研究队列消费脱敏记录 |
| F11 | X | E07 E17 E19 | 普通出口依赖脱敏；TLS 独立保护需另验 |
| F13 | H | E07 E17 | 脱敏成本可能影响广播吞吐 |

### D10 组件通信与隔离

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F01 | H | E21 | 生产 Collector 不依赖实验 IPC；未来切换传输需做对照 |
| F11 | H | E21 | 实验 Engine 尚非生产 UI 数据链 |
| F12 | I | E21 | 实验子进程需独立启动/关闭 |
| F13 | I | E14 E21 | 实验有界 outbox/共享内存影响资源与背压 |

### D11 证据呈现

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F09 | X | E10 | 输出 DTO 是否暴露凭据决定隐私边界 |
| F13 | H | E07 | 呈现/序列化负担待测 |

### D12 生命周期恢复

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F02 | H | E03 E12 | 生命周期切换和序号 generation 的统一账本待测 |
| F03 | X | E12 | cgroup attribution GC 属于运行时生命周期 |
| F05 | X | E12 | 启动/关闭 loop 与 signal worker、ML autotune |
| F06 | C | E09 E12 | 反馈 worker 生命周期影响自动执行（不推断静态已附着规则失效） |
| F07 | X | E10 E11 | runtime settings 替换改变门控条件 |
| F08 | X | E12 | 研究 processing worker 生命周期 |
| F09 | X | E11 E12 | 启动脱敏及配置 TLS 副作用 |
| F11 | X | E11 E12 | 归档保留、广播生命周期 |
| F13 | H | E12 | 关闭预算、恢复开销待测 |

### D13 性能与资源

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F01 | H | E01 E13 | 负载、reader 及队列容量影响覆盖的程度待测 |
| F02 | H | E03 E13 | 资源饱和是否造成不可解释残差待测 |
| F05 | H | E02 E07 | 检测延迟及模型预算待测 |
| F06 | C | E09 | 自动反馈 rate limit/dedup/队列容量影响执行尝试 |
| F08 | H | E07 | 研究队列饱和及隔离待测 |
| F10 | I | E14 E21 | 实验 transport outbox 资源上限直接决定 admission |
| F11 | X | E13 E06 | 广播拒绝改变下游普通记录输入 |
| F12 | H | E12 | 故障与关闭时间预算待测 |

### D14 交付兼容

| 受影响 FR | 现状 | 证据 | 限定理由 |
| --- | --- | --- | --- |
| F01 | H | E16 | 功能构建范围与真实 probe 支持需逐宿主验证 |
| F05 | X | E12 E16 | ML build gate 影响 auto-tune 启动；非模型质量证明 |
| F06 | X | E12 E16 | sandbox build gate 影响启动能力；非真实阻断证明 |
| F07 | X | E16 E23 | build/runtime feature 决定可用能力/路由契约 |
| F10 | H | E21 | 实验 IPC 兼容与版本迁移待测 |
| F11 | X | E16 | manifest 暴露编译和运行时能力字段 |
| F12 | X | E12 | build gate 决定特定启动任务 |
| F13 | H | E12 E16 | 不同构建的资源预算需比较 |

## 6. 证据索引

| ID | 路径 | 行号/范围 | 说明 |
| --- | --- | --- | --- |
| E01 | [backend/app/jobs_background.go](../../backend/app/jobs_background.go) | 77–90 | 解码、提前过滤、构建、广播入队 |
| E02 | [backend/app/events/events_kernel_builder.go](../../backend/app/events/events_kernel_builder.go) | 310–311 | 同步调用 KernelRisk |
| E03 | [backend/app/kernelriskbridge.go](../../backend/app/kernelriskbridge.go) | 62–103 | 时钟与序号观察、同步风险调用 |
| E04 | [backend/app/events/context_event.go](../../backend/app/events/context_event.go) | 313–379 | PID/父进程/cgroup 归属、fork/exit 状态 |
| E05 | [backend/app/agent_scope_policy.go](../../backend/app/agent_scope_policy.go) | 72–87 | root 归属参与 capture/monitor |
| E06 | [backend/app/ws_api.go](../../backend/app/ws_api.go) | 146–183 | 归属、capture、monitor、语义告警及记录顺序 |
| E07 | [backend/app/statepersistence_events.go](../../backend/app/statepersistence_events.go) | 376–410 | 脱敏、归档、持久化、OTLP、分析队列 |
| E08 | [backend/app/server_uds.go](../../backend/app/server_uds.go) | 204–278,345–416 | UDS 验证、模型/规则融合、训练样本、上下文与 PID 注册、事件 |
| E09 | [backend/app/events/kernel_risk_feedback.go](../../backend/app/events/kernel_risk_feedback.go) | 87–101,157–261,267–289 | 门控、预算、候选、实际 enforcer |
| E10 | [backend/app/helpers_auth.go](../../backend/app/helpers_auth.go) | 16–34,64–115,133–165 | 认证、runtime gate、配置响应 |
| E11 | [backend/app/handlers/runtimeconfig.go](../../backend/app/handlers/runtimeconfig.go) | 214–217,279–313 | 配置串行化、替换、保留与 TLS 副作用 |
| E12 | [backend/app/jobs_background.go](../../backend/app/jobs_background.go) | 100–210 | 生命周期启动、脱敏、后台 worker、UDS 与 build gate |
| E13 | [backend/app/broadcast_queue.go](../../backend/app/broadcast_queue.go) | 11–18 | 队列 admission 与 drop 指标 |
| E14 | [backend/internal/componentipc/outbox.go](../../backend/internal/componentipc/outbox.go) | 65 | 实验传输入队 |
| E15 | [backend/internal/detectionengineering/engine.go](../../backend/internal/detectionengineering/engine.go) | 81–83,177 | 人工标签与 replay |
| E16 | [backend/app/feature_manifest.go](../../backend/app/feature_manifest.go) | 183–204 | build/runtime feature manifest |
| E17 | [backend/app/redaction_engine.go](../../backend/app/redaction_engine.go) | 31–41,140–143 | 原地脱敏及共享 legacy event |
| E18 | [backend/app/ml/ml_runtime_state.go](../../backend/app/ml/ml_runtime_state.go) | 21–54 | 模型 snapshot 与缓存清除 |
| E19 | [backend/app/tls/capturehandlerstls.go](../../backend/app/tls/capturehandlerstls.go) | 171–187 | TLS 独立序列化 |
| E20 | [backend/app/sandbox/cgroup_ops.go](../../backend/app/sandbox/cgroup_ops.go) | 80–161 | 阻断目标 map 写入/删除 |
| E21 | [docs/architecture/component-ipc.md](../../docs/architecture/component-ipc.md) | 26–59,284–337 | 实验角色、队列、UID/只读 SHM 与 ACK 限制 |
| E22 | [docs/architecture/mit-axiomatic-design-feedback.md](../../docs/architecture/mit-axiomatic-design-feedback.md) | 第 2–4 节 | 来源、序号和代际分析（PR 补充，不属于基线源码） |
| E23 | [backend/app/routes.go](../../backend/app/routes.go) | 27–93 | 路由与认证/策略门控局部锚点 |

## 7. 定量填充与复现

```sh
node scripts/verify_axiomatic_matrix.mjs
node scripts/verify_axiomatic_matrix.mjs --template > matrix-measurements.json
```

亦可使用 Bun。模板包含 196 个单元、结构类别、目标类别，以及 status/metric/unit/delta/CI/tolerance/独立 run 数/evidence 和实验 provenance。默认全部 not_run/null。每种模式、负载、实验 commit 与指标单独生成/保存模板；未运行不可用 0 代替。

每个格子的可测效应为固定其他设计变量、基线与干预同一 workload/模式时的配对指标差，或明确的因果估计。对需要隔离的 Z，等效性区间全部落入 [-tolerance,+tolerance] 才支持该指定指标的隔离；横跨容差边界为 inconclusive，完全超出同一侧为违反。对 A/W/C/B/I，验证预期主/交叉效应及其他不变量，不要求人为非零。多个指标需独立记录，不能对相反方向变化平均抵消。安全硬门禁使用单独负例、状态变化和 canary 检查，不用浮点容差或平均分替代。统计计划与联合成功率见第四轮。

## 8. 验证与限制

矩阵生成逻辑已执行，196+196 格、证据 ID 引用和重复单元负向对照已检查；SCC/排列计数由脚本计算。未执行 Go 并发、真实 BPF、性能及统计干预实验。只改分析/验证工具，不改生产路径和默认权限。当前所有定量单元仍未测；本轮填充的是结构证据与目标隔离契约，不得作为生产安全认证。


## 9. 第六轮：独立算法验证、实验数据契约与 CI

本轮新增验证层，不改变上述矩阵分类，不把抽象检查当作实测。上一轮 head `e7f1c83433617ec39c8eb7b4aec885c80b0dcd2a` 的 Documentation / GitHub Pages 与 Repository integrity 已成功。

### 独立图算法对照

运行 `node scripts/test_axiomatic_matrix.mjs`。使用独立的全排列计数和 BFS 可达性参考算法对全部四节点无自环有向图进行交叉验证（12 条可能有向边，2^12 = 4096 个图）。验证边数、强连通分量、精确排列数及给出的示例排序；另检查六类边在五种模式下的 30 个条件组合、392 个 Markdown/JSON 对应单元及六种非法矩阵输入。本轮实际执行同一核心逻辑，全部通过。此结果支持图计算正确性，不验证边的物理因果真实性或 14 节点模型的独立公理成立。

### 实验数据验收契约

```sh
node scripts/verify_axiomatic_matrix.mjs --template > matrix-measurements.json
node scripts/verify_axiomatic_measurements.mjs --self-test
node scripts/verify_axiomatic_measurements.mjs matrix-measurements.json
# 严格检查全部目标 Z 单元具有等效区间证据：
node scripts/verify_axiomatic_measurements.mjs matrix-measurements.json --require-complete-isolation
```

每份文件必须有全部 196 个唯一 FR/DP 键，并与结构分类及目标分类一致。已填写记录要求实验 commit、模式、workload、host、config SHA-256、metricsVersion、预注册计划 SHA-256、置信水平（0 到 1 之间）、区间方法、多重比较策略。测量记录还要求指标/单位、估计值、有序且包含估计值的有限区间、非负容差、至少两个独立 run、证据索引。这里至少两个仅为输入合法性最低限制，不是统计功效要求。

| 输入状态/区间 | 判定 |
| --- | --- |
| not_run | 未执行；所有测量字段必须为 null |
| unsupported | 不适用/能力不可用，必须说明原因，不能冒充等效 |
| measured，Z，区间完全在 [-δ,+δ] 内 | equivalent |
| measured，Z，区间完全位于容差区间同一侧之外 | violated |
| measured，Z，其余区间 | inconclusive |
| measured，非 Z | measured_not_isolation，不能凭区间判定主功能通过 |

存在 violated 时程序退出非零；严格模式下任何一个 Z 缺测、不适用或不确定也退出非零。若多个宿主或指标，须分别保留结果；不得跨指标平均抵消违反。置信区间的选择、真实独立样本数、数据内容、计划是否事前冻结、证据真实性、统计功效和家族错误率仍由实验及独立审查确认。当前工具检查声明的数据契约，不重新计算原始样本置信区间，不把 equivalent 当作全部 FR/安全门禁通过。

实际执行合成验证：空模板 196/196 not_run；混合 fixture 1 equivalent、2 violated、1 inconclusive、1 measured_not_isolation、1 unsupported、190 not_run；17 种非法输入全部拒绝。上述数值只属于合成 fixture，不写入真实定量矩阵。

### 自动化验证

- Repository integrity 新增独立的 `axiomatic-design` job，运行一级源码锚点、抽象反例、需求追踪、独立图 oracle、实验数据自测与空模板往返校验。
- [Axiomatic design rootless validation](../../.github/workflows/axiomatic-design-rootless.yml) 仅在相关文档/脚本/工作流变更时触发（也支持手动触发），使用仓库 Go toolchain 对已有 agentscope、agentidentity、eventnoise、collectorcodec、eventqueue、taskgroup、detectionengineering、agentboundary、componentipc 运行真实 race 回归；不加载 BPF、不使用 sudo、不修改系统策略。不是完整 app 或特权集成测试。

本轮局部核心 JS 算法与数据自测已执行；新 CI job 是否完成应以最新 commit 的实际 workflow 结果为准。真实定量矩阵仍有 196 个 not_run；Go race、特权 enforcement、隐私全出口、性能及故障干预不能用本轮合成测试替代。
