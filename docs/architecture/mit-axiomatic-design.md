# Agent eBPF Filter：MIT 公理设计（Axiomatic Design）架构分析

> **全系统审查入口：** [第四轮全系统需求追踪、干预矩阵与验收总表](mit-axiomatic-design-system-review.md)。该报告覆盖十四项验收级子 FR 与十类测试族，并区分现有结构证据、模型结果及待执行的真实测试。

> **第二轮验证更新：** [源码深度验证与分模式矩阵](mit-axiomatic-design-validation.md) 已确认可选 kernel risk feedback 到真实策略 map 写入的结构链路，并穷举设计矩阵的所有排列。第 4 节为初始假设快照；关于 A43 等条件依赖，以第二轮报告为准。没有完成 Go/BPF 运行时测试。

> 状态：设计审查 / 待验证假设，2026-11-26。基于 master 的源码抽查及现有技术手册；不宣称已完成全仓调用图审计、性能测试或公理合规认证。本 PR 不修改运行时代码。

## 1. 方法与边界

采用 Nam P. Suh 的 Axiomatic Design（AD）方法：[MIT introduction](https://web.mit.edu/axiom/www/introduction.shtml)；Suh, *The Principles of Design*, Oxford University Press, 1990；*Axiomatic Design: Advances and Applications*, Oxford University Press, 2001。

四个域：客户需求 CA → 功能需求 FR → 设计参数 DP → 过程变量 PV。FR 描述要达到的可测结果，DP 描述满足 FR 的可调整设计手段，PV 描述构建、配置和验证手段。各层从 FR 选择 DP，再在已选 DP 约束下分解子 FR（zigzag），而非事先按代码目录划分 FR。

- 独立公理：维护功能需求的独立性；理想设计矩阵为对角阵，三角阵允许按指定顺序调节。无法经合理重排成为三角阵的循环依赖是耦合设计。
- 信息公理：在满足独立公理的备选设计中，选择满足全部 FR 的信息量最小者。信息量来自达到目标的概率，而非文件数量、代码行数或 Shannon 事件熵。

约束 C（不作为矩阵中的可交易性能目标）：Linux-first；明确内核能力与实际覆盖范围；授权、最小权限、危险能力默认关闭；不泄露受保护数据；兼容现有 protobuf 字段编号、HTTP/WS、wrapper UDS、Renew；不把 Hook 推断冒充内核事实。无内核支持时必须显式报告 unavailable，而非显示受保护。不可为了提高覆盖率隐式扩大采集范围或授权。

## 2. 现状证据与客户需求

| 证据 | 观察与边界 |
| --- | --- |
| [appcontext.go](../../backend/app/appcontext.go) | AppCtx 聚合事件 hub、runtime settings、BPF maps、规则、归档、关联等；Ctx 有全局回退。共享所有权/生命周期风险确实存在，但仅凭结构体不能证明功能耦合。 |
| [jobs_background.go](../../backend/app/jobs_background.go) | startKernelEventReader 在同步 Pump 回调中解码、筛选、buildKernelEventFromRaw 并入队；借用样本不能跨回调。startRuntimeBackgroundJobs 同时启动 broadcaster、risk feedback、loop detection、research 等任务。 |
| [kernel_risk.go](../../backend/app/events/kernel_risk.go) | ApplyKernelRiskDecision 在消费样本期间评分、修改 RiskScore/Decision/ExtraInfo 并提交 feedback；因此分析成本与采集热路径之间存在明确依赖。feedback 对具体 map/权限的作用仍需追踪。 |
| [redaction_engine.go](../../backend/app/redaction_engine.go) | 全局脱敏引擎；redactEvent 修改 event 字段，envelope 处理考虑 LegacyEvent 别名。应验证对关联标识、归档与各出口的影响。 |
| [routes.go](../../backend/app/routes.go)、[helpers_auth.go](../../backend/app/helpers_auth.go) | 路由认证、feature 编译条件和 runtime gate；authEnforced 只检查 DISABLE_AUTH，而非 Gin mode；clusterRequestAuthAllowed 是另一个分支。旧 [technical/security](../technical/security.md) 关于 release 的表述需校正，不能据此推断 debug 无认证。 |
| [independent-components](independent-components.md)、[component-ipc](component-ipc.md) | 已有 agentscope、eventnoise、collectorcodec、collectorstream、agentidentity、eventqueue 等独立库和阶段性 IPC。IPC 文档中的自动 ACK、耐久重放等尚非生产保障；不假定所有组件独立部署。 |
| [technical architecture](../technical/architecture.md)、[data flow](data-flow.md) | 单个特权后端、Vue Web 与原生 Renew 是不同 UI；数据与控制边界是逻辑分层，非默认进程分离。 |
| [validation-first detection](../security/nvidia-validation-first-detection.md) | 已有有界离线候选规则 replay 的文档入口，可用作独立验证思路；候选通过不等于执行授权。 |

CA1 用户知道 Agent 真正执行了什么；CA2 用户知道证据属于哪个 run/tool；CA3 用户及时识别风险且误报可控；CA4 用户可独立约束执行；CA5 用户能够安全审计且不暴露敏感信息；CA6 用户可以持续使用监控而不因 UI/导出/分析故障失去核心能力。

这些需求为分析者从当前项目目标推导的工作假设，不是已访谈的客户需求。

## 3. FR / DP / PV 一级映射

FR0：在授权和性能约束内提供可追溯、可判断、可约束的 Agent OS 行为证据。DP0：内核观测与控制 + 用户态语义关联/分析 + 安全发布的分层架构。

FR 指标分母须预先声明；下面为候选验收维度，不是已测得的 KPI。

| ID | FR（功能结果） | 当前 DP（设计手段） | 候选 PV / 验证 |
| --- | --- | --- | --- |
| 1 | 在声明的采集范围内记录内核操作事实及缺失状态 | DP1 tracepoints、ringbuf、采集范围、解码与队列 | 固定 syscall workload、attach capability、buffer 容量、事件序列及损失计数 |
| 2 | 对已采集证据进行可验证的 run/tool 归属，未知显式标注 | DP2 PID/PPID、注册、Hook、ProcessContextStore、agentidentity | 独立已标注 trace、PID reuse、乱序、Hook 缺失、跨 cgroup 场景 |
| 3 | 对归属证据产生有根据的风险评分和告警 | DP3 规则、kernel risk、可选 ML、semantic alerts | 固定规则/模型版本、独立 benign/holdout 分组、阈值、混淆矩阵 |
| 4 | 对明确授权的操作范围执行可验证的 allow/deny/rewrite 语义 | DP4 wrapper / cgroup / BPF LSM 与策略管理 | feature gate、授权主体、策略版本、map apply、真实 syscall 返回和状态 |
| 5 | 输出符合数据保护策略的证据，同时保留所需的审计关联 | DP5 redaction、受控存储/输出、认证和 gate 的数据侧应用 | 合成 secret canary、字段分类、各出口检查、关联稳定性、token 轮换 |
| 6 | 在定义的延迟、吞吐和故障范围内查询与展示已发布证据 | DP6 archive、wsfanout、REST/WS、MCP/OTLP、Vue/Renew | 固定负载、slow consumer、断连、磁盘满、P95/P99、CPU/RSS |

FR1 是对实际发生且符合采集条件的操作的事实完整性，不要求所有操作一律采集。FR4 的阻断会改变真实 workload，矩阵比较时以同一尝试动作及其结果为基准；不能把成功阻断造成的后续动作不存在算成事件丢失。FR5 约束所有 DP；其行仅评价发布数据保护结果，不能将硬性安全约束随意移除。

## 4. 独立公理：现状诊断

设计方程 {FR} = [A]{DP}。对于软件离散配置，用受控干预和结果变化近似分析 A_ij；行是 FR，列是 DP。X = 结构性依赖或直接实现证据；? = 疑似交叉影响、需干预验证；. = 在限定工作负载与约束下暂未发现直接依赖，非已证明为零。以下是保守的**定性依赖假设矩阵**，不是数值 Jacobian。

| As-is | DP1 采集 | DP2 归属 | DP3 风险 | DP4 控制 | DP5 保护 | DP6 发布 |
| --- | --- | --- | --- | --- | --- | --- |
| FR1 事实 | X | ? | X | ? | . | ? |
| FR2 归属 | X | X | . | . | ? | . |
| FR3 识别 | X | X | X | ? | ? | . |
| FR4 控制 | . | ? | ? | X | ? | . |
| FR5 保护 | ? | ? | ? | . | X | ? |
| FR6 服务 | X | X | X | ? | X | X |

关键非对角项及验证：

| 边 | 风险与证据级别 | 干预实验 |
| --- | --- | --- |
| A13 | 高优先级：源码确认 risk pass 位于样本消费热路径，是否造成 loss 尚未测 | 固定事件生产率，仅改变评分实现成本，测 decoder/sink loss 与延迟 |
| A31 | 结构依赖：检测依赖采集事件；与 A13 形成双向依赖假设 | 固定评分器，改变采集容量及范围，分离 coverage 与 recall |
| A12 / A24 / A42 | 采集范围、归属与策略主体可能交叉，尚未证实完整链路 | 禁用 Hook、构造 PID reuse、保持固定静态策略；对比实际 syscall 结果和证据 |
| A43 | risk feedback / wrapper 评分可能影响决策，但不能从名称推导其有阻断权限 | 固定静态规则，禁用或替换评分，追踪 policy / map writer，比较 syscall 与 wrapper decision |
| A25 / A35 | 变更脱敏可能影响关联或 replay 特征，尚未证明现有实现存在回环 | 固定输入、分别切换 redaction policy，检查不含 secret 的稳定关联键及特征覆盖 |
| A16 / A56 | 消费拥塞及不同出口可能影响证据可用性或数据保护 | slow subscriber、disk full、OTLP unavailable；逐出口 canary 校验，核对 drop counter |
| A45 | 授权、runtime gate 和 redaction 都在安全领域，但读写权限不应由数据脱敏等级决定 | 固定 token/gate 与策略，只变 redaction，核对授权及拒绝结果 |

现状在抽象层上至少存在 A13/A31 的结构性反馈路径，因此不能直接判为对角设计；是否在约定负载、容差下构成实质的 FR 耦合仍需实验。共享 AppCtx 不等于矩阵中每格都是 X。

## 5. Zigzag：高风险子问题分解

### 5.1 DP1 采集面
- FR1.1 适用探针与宿主能力明确可见 → DP1.1 单一 eBPF 资源所有者及 capability 状态 → PV attach/preflight fixtures。
- FR1.2 以明确所有权解码样本 → DP1.2 collectorcodec / collectorstream，借用 view 不越界 → PV 对齐、截断、大小端及缓冲复用测试。
- FR1.3 可度量交接损失 → DP1.3 有界队列、序号和独立 loss 分类 → PV 饱和、取消、重连、序列缺口测试。

先确定所有权与样本契约，再设计队列，避免为了进程拆分引入 UAF、重复 BPF attach 或未记录丢失。

### 5.2 DP4 控制面
- FR4.1 只有授权主体可改变策略 → DP4.1 认证/授权/编译期与运行时能力检查 → PV 未认证、gate disabled、unsupported 负例。
- FR4.2 策略意图可被正确转换为各执行点的能力约束 → DP4.2 版本化 policy validator/compiler → PV 覆盖矩阵、非法字段、冲突优先级。
- FR4.3 执行结果与已确认策略版本一致 → DP4.3 单一 map/link owner，事务式应用与回滚（目标） → PV 部分失败、重启、旧版本及真实 syscall。
- FR4.4 控制不可因监控 UI、出口或 ML 不可用而偷偷扩大权限 → DP4.4 独立静态基线和显式降级状态 → PV 停止消费者、模型不可用、保持固定政策的故障注入。

建议 DP4.1 → DP4.2 → DP4.3；DP4.4 是贯穿的安全不变量。wrapper、cgroup、LSM 效果范围不等价。对 ML 建议单独建立人工/授权提升策略的通道，保留现有明确配置的 wrapper 同步策略兼容性，任何行为迁移需另行审查。

### 5.3 DP5 数据保护
FR5.1 明确原始数据访问边界 → DP5.1 原始事件仅在受信处理域短生命周期存在；FR5.2 生成符合隐私策略的发布 DTO → DP5.2 有类型的 redacted event 与稳定非敏感关联键；FR5.3 各输出遵守相同策略 → DP5.3 单一发布契约、出口防御检查。PV 为合成秘密、重放、日志/错误分支、所有序列化器的 canary 测试。三项均为目标契约，不宣称已完整落地。

## 6. 目标设计：受控单向依赖而非强行微服务

拟在现有 Go 后端内先建立明确接口/所有权，保持嵌入部署，必要时再使用实验 IPC。先固定控制和采集设计参数，随后归属、风险、保护与服务。以 DP4、DP1、DP2、DP3、DP5、DP6 顺序重排后，目标矩阵：

| To-be | DP4 | DP1 | DP2 | DP3 | DP5 | DP6 |
| --- | --- | --- | --- | --- | --- | --- |
| FR4 | X | . | . | . | . | . |
| FR1 | X | X | . | . | . | . |
| FR2 | . | X | X | . | . | . |
| FR3 | . | X | X | X | . | . |
| FR5 | . | X | X | X | X | . |
| FR6 | . | X | X | X | X | X |

其中 A14（重排后第二行第一列）表示控制尝试的结果会成为观测事实；核心静态策略不依赖在线风险模型或采集投递成功，是目标而非当前断言。如果业务明确要求风险驱动的同步控制，则需把该闭环作为单独 FR/DP 组合重新建模、固定静态授权上界、规定超时和降级语义，不能伪称已消除所有耦合。采集与分析资源争用、隐私策略对证据可用性及所有权迁移等剩余影响需通过容差与实验验证。

```mermaid
flowchart LR
  I[授权策略意图] --> P[验证及版本化策略]
  P --> K[单一内核控制所有者]
  K --> S[尝试及实际结果]
  S --> C[采集 / loss accounting]
  C --> A[归属与证据]
  A --> R[规则 / ML 风险建议]
  R --> D[受控脱敏发布]
  D --> O[归档 / API / Renew / Vue]
  R -. 独立审批 .-> I
```

虚线表示跨版本治理决策，非自动在线授权反馈；策略管理、采集、发布即使共享进程也应具有独立契约。保留现有 protobuf 兼容、可选 TLS、危险能力 gate 与用户明确控制。

## 7. 信息公理：可复现实验设计

I = -log2 P(所有 FR 同时落入预先确定的设计范围)。只有 FR 成功事件相互独立时才可把单项 I 相加；否则使用联合概率或条件概率链。零成功不能推出真实概率为零；有限样本须报告置信区间，缺失数据单列。跨版本不可改变 workload、阈值和支持范围后直接比较 I。

| FR | 每次试验成功判据（建议，尚未批准） |
| --- | --- |
| FR1 | 声明覆盖范围内预置操作的事件召回 ≥ 99%，缺失可对账；给出分母及 probe coverage |
| FR2 | 已知归属样本正确率 ≥ 99%，未知不被误归属，报告 unknown 比例与 coverage |
| FR3 | 独立攻击 holdout recall ≥ 95%，独立 benign run FPR ≤ 1%，报告置信区间和 precision |
| FR4 | 支持且授权的固定阻断/放行用例全部正确；不支持状态明确；禁用 gate 和未授权操作全部拒绝 |
| FR5 | 合成 secret canary 在声明的所有外部出口/归档零泄露，关联键在同策略版本稳定 |
| FR6 | 预定负载下 P99 事件发布延迟 ≤ 250 ms、CPU/RSS/损失在预先冻结的预算内；慢客户端不阻塞核心采集 |

数值只是供 maintainer 审批的起始目标，不是项目基准。FR4/FR5 属安全硬门禁；观察零失败并不证明真实失败概率为零。FR3 分母以独立 run 为单位，不以同一 run 中相关事件充当独立样本。测试前冻结机器、内核、BTF/LSM、权限、配置、模型、语料、种子、并发、运行时长、CPU/RSS 预算及预热。支持与不支持宿主分别统计；未运行的测试不能计为成功。

每次独立运行记录六个 FR 的 pass/fail/unsupported、各项分子分母、性能、loss、策略版本与配置哈希。联合成功率 = 全部适用且预先指定 FR 同时通过的运行数 / 完成的独立运行数；报告 Wilson 95% 区间（独立 Bernoulli 运行假设），必要时分层或按 run bootstrap。I 以联合概率估计及区间换算，避免凭空打分。

纯示例：若方案 A 联合成功 80/100，方案 B 90/100，则点估计 I_A≈0.322 bit、I_B≈0.152 bit；这不是仓库测试数据，也不自动证明 B 显著优于 A。若 B 引入授权或隐私回归，直接排除。

对照三种设计：
- A 当前单体 + 已有子包，低 IPC 复杂度，但共享故障域与热路径风险需实测。
- B 单进程内窄接口、队列、所有权和预算隔离，优先实验。需要验证额外队列/拷贝成本及行为兼容。
- C 进程拆分 / opt-in IPC，隔离潜力更高，但引入重放、认证、版本偏差、启动和恢复复杂度。仅当 B 无法满足可量化隔离目标、C 经实验通过全部门禁才推广。

不能在缺少联合成功率和对照测试时宣布哪种方案信息量最低。

## 8. 实施路线与评审门禁

| 优先级 | 工作包 | 验收及停止条件 |
| --- | --- | --- |
| P0 | 固定证据语义、能力矩阵、基线与 FR 测试契约；校正认证文档 | rootless 单元 + 负例，未定义的 FR 不做架构结论 |
| P0 | 采集热路径与风险评分隔离实验，核查 feedback map writer | 保留 borrow/copy 契约；loss、延迟、拒绝语义无回归，否则回退 |
| P0 | 授权和控制生命周期独立、版本化生效状态 | gate/未授权负例、部分失败、真实内核测试；不允许权限扩大 |
| P1 | 归属置信与隐私发布 DTO；每个输出 canary | PID reuse / missing hook / redaction / export；任何泄漏停止 |
| P1 | 队列、归档、UI/OTLP 慢消费者隔离 | 有界内存、drop reason、核心控制不受影响 |
| P2 | opt-in 组件 IPC 对照验证 | 认证、角色、序列、ACK/resume、故障恢复、版本兼容；默认继续嵌入 |

复用现有测试入口（本 PR 未执行）：

```sh
cd backend
go test -race ./internal/agentscope ./internal/agentidentity ./internal/eventnoise ./internal/componentipc
go test ./internal/detectionengineering ./app/research -count=1
go test ./...
cd ..
make os-enforcement-check
# 在支持且隔离的特权 Linux 测试机上分别运行：
make os-enforcement-preflight
make os-enforcement-smoke
make runtime-benchmark
```

上述测试与基准不能替代新增的 FR 干预、独立语料及联合成功率实验。每个后续 PR 应注明影响 FR/DP、可能新增的非对角项、对应证据和测试、回退路径；API/proto 变更必须有兼容审查。禁止把未经测试的 '.' 当成已证明独立。

## 9. 结论

当前项目已具备可复用的组件边界，但不能仅凭包拆分认定满足独立公理。采集和分析的反馈路径是首要验证对象，控制授权及数据保护是不可交易的门禁。推荐先做单进程的可测契约隔离，并用真实 workload 对照方案；不要在没有量化证据时将 IPC 微服务化作为默认终局。最大未知是 risk feedback、共享运行时和拥塞在真实负载下对控制语义及证据完整性的联合影响。
