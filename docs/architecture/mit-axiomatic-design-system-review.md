# MIT 公理设计全系统审查与验收总表（第四轮）

基线 `86357e5a912b3a4b294c9cf4b94474fbc3481bd7`；关联 PR #172。整合 [一级分析](mit-axiomatic-design.md)、[结构验证](mit-axiomatic-design-validation.md)、[动态反馈分析](mit-axiomatic-design-feedback.md)。本轮覆盖产品、数据面、控制面、研究面、呈现面与工程交付，按功能域给出可追踪验收；不是所有仓库文件逐行审计，也不是生产安全认证。读取架构/安全/验证等文档和关键源码，追加 13/13 文本锚点核查（12 个不同源文件），未执行生产 Go/BPF/性能测试。

## 1. CA、约束与设计域

| CA | 客户需要 | 对应 FR |
| --- | --- | --- |
| CA1 | 普通操作者看得懂 Agent 做过什么、何时、凭什么归属 | F01–F04、F11 |
| CA2 | 管理员授权限制行为，并可撤销、回滚及复核 | F05–F07、F12 |
| CA3 | 研究人员可重复、独立评估检测和性能 | F08、F13 |
| CA4 | 被监控主体、部署方的数据和权限受保护 | F07、F09、F10 |
| CA5 | 系统可安装、升级、运行、恢复，支持不同宿主能力 | F10–F12、F14 |

硬约束：无未授权权限扩大；未知/不支持显式报告；默认生产路径兼容；数据最小化；保持一套真实 BPF owner；实验不得修改日常宿主的生产 map。Linux 内核、BTF/LSM、cgroup v2 和特权是适用能力条件，而非全部平台普遍具有的能力；Windows/UI 或无特权端仅按实际实现测试。性能、成本、可维护性为预先冻结的设计范围，不得替代安全门禁。

Zigzag：CA → 一级 FR1–FR6（主文）→ 现有设计选择/边界 → 下表的验收级子 FR/DP/PV → 实验。一级 FR 与子 FR 不是互斥模块数量；例如 F09 同时影响 FR5 隐私与 FR6 审计，F10 影响 FR4 权限和 FR6 可用性。下表以主责任归类，不将接口依赖自动等同于功能耦合。

## 2. 全域需求追踪与证据地图

| 子 FR / 一级归属 | 可独立验收结果 | 现状 DP / 源码锚点 | 目标 DP → PV/验收 |
| --- | --- | --- | --- |
| F01 / FR1 | 采集声明范围内的内核事实并区分不支持 | [collector ingress](../../backend/app/jobs_background.go)、[BPF ABI](../../backend/ebpf/agent_tracker_common.h) | 单 owner、能力 manifest、解码契约 → 支持探针集合、fixture 真值、错误分类 |
| F02 / FR1 | 时间、序号、原始丢失与主动过滤可核对 | [kernel sequence](../../backend/app/kernel_sequence_audit.go)、[clock bridge](../../backend/app/kernelriskbridge.go) | 原始观察前置、epoch 账本 → 乱序/reload/丢失注入、残差 |
| F03 / FR2 | 有证据归属、推断、未知可区分 | [context](../../backend/app/events/context_event.go)、[observed root](../../backend/app/agent_scope_root.go) | 主体 start identity + run/cgroup evidence provenance → PID reuse、缺 hook、冲突 |
| F04 / FR1/2 | capture/monitor 规则可独立选择并具有单事件版本 | [scope](../../backend/app/agent_scope_policy.go)、[broadcaster](../../backend/app/ws_api.go) | 不可变 scope snapshot、过滤原因 → 四种 capture/monitor 组合及配置交错 |
| F05 / FR3 | 风险结果具来源、版本与证据，识别质量可测 | [kernel risk](../../backend/app/events/kernel_risk.go)、[semantic alerts](../../backend/app/events/alerts_semantic.go)、[ML runtime](../../backend/app/ml/ml_runtime_state.go) | 分层 detector、版本化输入/输出 → 混淆矩阵、calibration、超时和成本 |
| F06 / FR4 | 手工策略独立有效；自动提案有界且可撤销 | [feedback](../../backend/app/events/kernel_risk_feedback.go)、[cgroup](../../backend/app/sandbox/cgroup_ops.go)、[LSM](../../backend/app/sandbox/lsm_control.go) | 来源账本、授权代际、有效 map readback → V03/V05/M01–M04 |
| F07 / FR4/5 | HTTP/WS/MCP/UDS/hook 的身份与能力校验、跨入口等价 | [auth](../../backend/app/helpers_auth.go)、[routes](../../backend/app/routes.go)、[wrapper UDS](../../backend/app/server_uds.go)、[HTTP security](../../backend/app/http_security.go) | 主体绑定、能力授权、每个 mutator 统一 commit 契约 → 未授权、禁用 gate、撤销负例 |
| F08 / FR3/6 | 研究回放、标签和训练/评估可独立复现，不自动激活策略 | [replay](../../backend/internal/detectionengineering/engine.go)、[transition](../../backend/internal/agentboundary/transition.go) | 数据 lineage、holdout 冻结、只读研究 → 标签独立性、重复实验、无 map 写入 |
| F09 / FR5 | 全部外部出口按分类保护数据和凭据 | [redaction](../../backend/app/redaction_engine.go)、[TLS broadcaster](../../backend/app/tls/capturehandlerstls.go)、[config DTO](../../backend/app/helpers_auth.go) | 各域最小输出 DTO、策略版本、secret write-only → synthetic canary 和导出 schema |
| F10 / FR4/6 | 组件权限和资源隔离，传输状态诚实 | [peer auth](../../backend/internal/componentipc/peer_linux.go)、[IPC phase contract](component-ipc.md) | 认证的窄接口、有界 outbox、处理确认与恢复 → 假角色、断链、饱和、重连 |
| F11 / FR6 | UI、历史、导出一致呈现事实/告警/实际执行与缺失 | [event persistence](../../backend/app/statepersistence_events.go)、[broadcaster](../../backend/app/ws_api.go)、[architecture](overview.md) | read-only projection、source-aware IDs、状态词汇 → 跨出口同一事件/动作对账 |
| F12 / FR6/4 | 生命周期、重启、配置持久化与部分故障可恢复 | [runtime config](../../backend/app/handlers/runtimeconfig.go)、[background jobs](../../backend/app/jobs_background.go)、[IPC phase contract](component-ipc.md) | 显式 supervisor、desired/applied 状态、reconcile → crash、shutdown、reopen、map reload |
| F13 / FR6 | 负载与资源开销可量化，慢消费者不破坏控制 | [broadcast](../../backend/app/broadcast_queue.go)、[collector ingress](../../backend/app/jobs_background.go)、[benchmark](../operations/verification-benchmark.md) | 预算、独立队列、饱和指标 → 吞吐/延迟/RSS/CPU/损失曲线 |
| F14 / FR6 | 构建、版本、部署与升级可复现且兼容 | [README](../../README.md)、[implementation map](../reference/codebase-implementation-map.md)、[verification](../operations/verification-benchmark.md) | schema/ABI/feature 版本、发布与回退契约 → 构建矩阵、兼容测试、可信产物 |

现状证据分级：S=已核查代码路径；D=文档声明（不是实测）；M=抽象模型；T=实际测试执行。本轮 F01–F12 至少具有局部 S 或 D，F13–F14 主要依赖局部路径/现有文档；覆盖十四项是**需求清单覆盖率**，不表示十四项通过。无 T 数据不得填入真实通过率。历史报告、默认配置和 opt-in 实验必须按各自运行模式区分。

## 3. 跨域新增问题与正向证据

| ID | 源码/文档事实 | 未决风险与验证 |
| --- | --- | --- |
| F11a | wrapper UDS 先验证 peer（server_uds.go L204），再解码及校验请求 L223；peer 验证 L458–486 检查 SO_PEERCRED UID；server_uds_limits.go L26–83 检查 PID 非零、长度和 score 有限，server_uds.go L369–375 使用 req.Pid 注册上下文和 map | 在这条直接路径未见 peer PID 与请求主体的授权绑定。UID 可信不等于同 UID 每个进程声明都可信；代理合法代注册也不能简单禁止。隔离 fixture 验证同 UID/不同进程、真实父子、PID reuse、授权代理。是否构成漏洞取决于明确的本机信任模型；不提供攻击操作。 |
| F12a | helpers_auth.go L145–158 清空 LlmAPIKey，却复制 OTLP header 值并返回 RuntimeSettings；core/state_types.go L22、32–33 有 accessToken、otlpHeaders、hookSecrets JSON 字段；runtimeconfig.go L305–313 返回该 DTO | 配置展示与秘密管理耦合。认证 API 本身不等于公开泄露；需要明确不同主体允许读取哪些凭据，并对 JSON/protobuf/config export/错误日志执行 synthetic canary。尤其应区分需要一次性展示/轮换的凭据与仅写入的集成凭据；不要盲目删除现有合法工作流。 |
| F13a | ml_runtime_state.go L21–54 有锁保护模型与元数据 snapshot，wrapper L251–278 直接使用同一 snapshot；model_ensemble.go L418–419、500–527 的优化辅助函数 key 仅含 comm/args 且 cache lookup 早于 snapshot；模型发布在 unlock 后 Clear | 直接 wrapper 路径没有使用该 cache；搜索只返回 helper 定义，未确认线上可达性。若今后接入，模型版本/特征上下文与 cache 一致性须测试：旧推断完成后写回、新模型读旧 key、禁用后 cache hit、相同命令不同特征。优先用 generation namespace；未证实当前生产误判。 |
| F14a | TLSBroadcaster.publish 在 capturehandlerstls.go L181–182 直接 JSON 序列化 TLSPlaintextEvent；这是区别于通用 Event record 的发布路径，另有 TLS 专用 sanitizer | 需按全部入口（不同 capture source）验证发布前保证；不能由普通 event redaction 顺序推导 TLS 安全，也不据此宣称 TLS 明文泄漏。 |
| F15a | detectionengineering/engine.go L81–83、177 要求人工标签并明确不能证明 holdout 独立；agentboundary/transition.go L19–27 重做 CheckUpdate，返回非变更 proposal | 现有设计正向符合研究与控制分离。需要验证回放不修改任何运行时策略、训练语料与评估按 run/家族/时间隔离，proposal 不能被当作执行证据。 |
| F16a | componentipc/peer_linux.go L16–49 UID allowlist、非 UDS/空 allowlist 拒绝；[IPC 文档](component-ipc.md) 描述 HMAC/role、read-only memfd、私有 cursor 和 ACK | 正向设计：显式身份与数据面权限分离。文档明确 ACK 不提供重放/持久处理或可信安全证明，恶意授权 Engine 仍可发出 ACK；同 UID、跨 UID 及 production 未部署状态必须分列。 |

编号 F11a–F16a 是发现编号，不与上表子 FR 等同。上一轮 F05–F10 继续有效；本轮优先审查配置 DTO、主体绑定和证据守恒，避免将“身份已认证”写成“每项声明都真实”。

## 4. 功能独立性：全系统干预矩阵

一级 6×6 静态图继续用于结构假设；动态闭环见第三轮。十四项子 FR 的独立性由以下**干预契约**检验，而非人为填入 196 个未经测量的数字。非列出的单元是 unknown，不是零。针对固定工作负载与模式，逐项改变 DPj，记录全部 FRi 指标；用事前容差判断交叉影响。

| DP 干预 | 允许的主效应 | 应保持不变量（失败即耦合/门禁问题） |
| --- | --- | --- |
| D01/D02 采集与序号审计 | F01/F02 的覆盖和精度 | F06 静态授权不扩大，F09 不泄露，F11 不误报 loss |
| D03/D04 归属与 capture/monitor | F03/F04，明确范围内 F05/F11 | F06 自动授权仅由明确 feedback scope 决定，F02 原始计数不改义 |
| D05 detector 版本/阈值 | F05，授权闭环内 F06b | F06a 固定手工策略、F07 授权、F09 保护不变 |
| D06 策略及来源撤销 | F06、后续真实行为 | F02 不把被阻断未发生行为当丢失，F07 不能越权 |
| D07 认证/能力配置 | F07 | F09 仍按出口最小化；未授权调用不改变 F06 |
| D08 replay/训练数据 | F08/F05 的离线结果 | F06 生产策略和已部署模型不变，除非有独立批准发布 |
| D09 redaction | F09/F11 可见表示 | F02 原始账本、F06 控制决定不变，已授权分析输入保持定义 |
| D10 IPC 传输/进程边界 | F10/F13 | F06a 控制独立于 telemetry 成功；F11 不能伪称已处理 |
| D11 UI/export schema | F11 | F01–F08 事实、模型和控制不受 UI 状态影响 |
| D12 生命周期/故障恢复 | F12/F13 | F06/F07 权限不扩大；F02/F11 新 epoch 明确 |
| D13 资源预算 | F13 | 硬门禁 F06/F07/F09 不因压力降级为未授权或明文发布 |
| D14 构建与版本 | F14/能力范围 | 同等支持配置的 F01–F13 语义兼容；明确 unsupported |

可量化定义（非实测）：对每个 FR 指标 Y_i、每个 DP 干预 j，报告配对差值 Δ_ij = Y_i(intervention) − Y_i(baseline)，预设最小实际重要差 δ_i。用适合指标的置信区间和等效性检验判断是否落于 [-δ_i,+δ_i]；区间跨界为 inconclusive，而不是“没有显著差异所以独立”。二元安全不变量要求每个预先指定用例通过；不把硬门禁折为平均分。反馈改变 workload 的实验另记反事实或固定尝试动作 ID；因果解释只限受控干预。

## 5. 信息公理：联合可靠性而非打分

采用同一宿主能力、数据划分、负载、授权语义和统计单位。定义 S_i 为第 i 项 FR 的事前验收事件，方案 k 的信息量 I_k = -log2 P_k(∩ S_i)。只有在有证据支持独立时才可将信息量相加。不要以十四个检查表的完成比例代替联合成功概率；不将已知未知、unsupported、未运行计为成功。共享资源、反馈和故障使 FR 相关，应保存每次 run 的完整结果向量。采用配对/分层的区间；报告最差适用宿主与失败模式，区间重叠不自动判优劣。

三个备选设计：A 保留嵌入式并增加证据/测试；B 嵌入式窄接口、来源化控制与独立资源预算；C 特权 Collector + 非特权 Engine + 经审查的控制面。反对 C 的核心证据是尚无持久处理确认、隔离成本、兼容和恢复数据；支持 C 的核心理由是权限与故障域隔离潜力。现阶段推荐 B 作为实验优先级，不宣称信息公理已证明 B 最优。若 B 无法满足预设权限/故障隔离预算而 C 满足所有硬门禁和性能/可靠性条件，推荐反转。保留 A 作为对照、可回退路径。

## 6. 全域验证计划和发布门禁

| 验证族 | 对应 FR | 必须实际取得的证据 | 当前状态 |
| --- | --- | --- | --- |
| T01 kernel ABI/探针/clock/sequence | F01,F02 | rootless fixture + 隔离特权宿主 ground truth，按 CPU/epoch 对账 | 待执行 |
| T02 identity & scope | F03,F04,F07 | PID/生命周期/代理授权/配置交错，未知和误归属分别统计 | 待执行 |
| T03 detection & ML | F05,F08 | 独立 benign/holdout run，混淆矩阵、PR 曲线、版本一致性、超时；校准与控制效果分开 | 待执行 |
| T04 policy & dynamic feedback | F06,F07,F12 | 现有 helper/mock/真实返回、两阶段撤销、来源重叠、部分失败、重启 | 抽象反例已执行；真实待执行 |
| T05 ingress authorization | F07,F09 | REST root/API alias、MCP、WS、UDS、hook、desktop，各入口负例与零未经授权状态变化 | 待执行 |
| T06 privacy | F09,F11 | 合成凭据 canary：配置/事件/历史/TLS/WS/导出/OTLP/日志，各主体出口分别记录 | 待执行 |
| T07 transport & recovery | F10,F12,F13 | 同/跨 UID、角色、disconnect/ACK/重放语义、压力、kill/restart，无自动权限扩大 | 待执行 |
| T08 projections & schema | F11,F14 | proto/JSON/WS/UI/record ID 与 status 一致，老版本读写、脱敏标志与 unsupported | 待执行 |
| T09 performance | F01,F06,F10,F13 | 基线/嵌入改造/可选 IPC 同 workload：p50/p95/p99、吞吐、RSS、CPU、丢失、恢复时间 | 待执行 |
| T10 delivery | F12,F14 | 构建矩阵、依赖锁、产物身份、安装升级回退；不在 untrusted PR 运行 root smoke | 文档/完整性 CI 已通过上一轮；实际交付待测 |

追踪检查器：[verify_axiomatic_traceability.mjs](../../scripts/verify_axiomatic_traceability.mjs)，从仓库根目录执行 `bun scripts/verify_axiomatic_traceability.mjs`（或 Node）。核心检查已对本报告文本执行：14/14 子 FR 均被 10 类测试覆盖，无未知引用；删除 T10 行的负向对照被检出。这里的覆盖率是文档追踪完整性，而非运行时测试覆盖率或通过率。

测试入口参考仓库现有 [验证、测试与 Benchmark](../operations/verification-benchmark.md)，以及前两轮脚本。先运行 rootless 测试，再由可信操作者在隔离 Linux VM 执行特权 preflight/smoke；真实硬门禁不允许被 docs CI 替代。所有实验记录 commit、config hash、kernel/CPU/arch、probe 支持、模型与数据版本、seed、重复数、失败原文（脱敏）、适用性、未运行原因。

## 7. 行动优先级与停止条件

- P0：统一认证主体绑定与秘密输出契约；静态/动态策略来源和代际；原始序号审计；真实安全回归测试。确认 API 兼容语义后才修改行为。
- P1：归属生命周期、单事件 scope snapshot、部分成功状态、检测/预测版本契约、脱敏出口覆盖；可量化资源隔离。
- P2：在相同负载下比较 B/C，先 read-only Engine，再讨论有权控制协议；必须有认证、作用域、可追溯动作、失败恢复和兼容性。
- 阻止发布：任何未经授权写入、秘密 canary 在不允许出口出现、人工规则误撤、将 unsupported/未执行当作成功、将 ACK/告警当作真实 enforcement、必要内核能力缺失仍报告已生效。

审查完成定义：全域需求、现状证据、假设、目标契约、干预、验收和阻塞条件都已列出；**实际全系统验证未完成**，不得合并后将此文当成安全认证。前述部分已有设计优点：纯逻辑包、单 BPF owner、只读提案、角色鉴权、保守身份缓存、事件序号和专用脱敏组件，保留并验证这些机制比不加区别地重写更合理。
