# 现场演示脚本：Agent eBPF Filter（2026-10 版）

> 适用：项目展示 / 操作系统设计赛答辩 / 真实 Linux 开发机产品演示  
> 基线：`master@10e9766`（2026-10-15）  
> 主线：**内核事实 → Agent 归因 → 原生桌面调查 → 受控 OS Enforcement → 工程验证**  
> 安全原则：全程在授权环境进行；策略拒绝、Hook 安装、Shell、TLS 明文采集等副作用操作只在隔离测试机展示。

与旧版纯 Web 演示不同，本版分为**必备的 Renew 桌面演示**和**依条件开放的 Web/内核高级演示**。现场不具备 BPF LSM/cgroup 时，明确说明未执行阻断，不用普通事件冒充内核拒绝。

## 0. 演示前必须核对的能力

| 检查项 | 操作 / 判断 | 若不满足 |
| --- | --- | --- |
| 内核与加载前提 | `make os-enforcement-preflight` | 保留只读桌面、文档与离线评测路径 |
| 源码一致性 | 演示前记录 `git rev-parse --short HEAD` | 不把其它提交功能写成本次可展示能力 |
| 原生桌面 | `make renew-desktop-dev` 或已打包的 Renew | 提前保存可核对版本的截图和日志，不声称实时运行 |
| eBPF collector | 桌面「系统诊断」实际显示健康与 event/system 流 | 展示故障状态与 fallback，不将空白页视为安全 |
| Agent 识别/生命周期 | 已有受支持 Agent 进程；可选 Hook 分别安装成功 | 只展示普通进程图，不标为已证实 SubAgent |
| cgroup/BPF LSM | 相应 hook 真正附着、gate 开启、隔离环境授权 | 省略现场策略变更，解释机制与验证命令 |
| 隐私数据 | 演示目录和 Agent 会话不含真实秘密 | 不打开完整凭据、TLS body 或用户私人配置 |

## 1. 时间线（约 12–15 分钟）

| 段落 | 建议用时 | 画面 | 要证实的观点 |
| --- | --- | --- | --- |
| 1. 目标与架构 | 1 分钟 | 文档站首页 / Mermaid 图 | 事实、语义、策略是不同层 |
| 2. 原生桌面启动与健康 | 2 分钟 | 「我的设备」和「系统诊断」 | 连通性、采集状态与丢弃计数可见 |
| 3. Agent 事件归因 | 2 分钟 | 「事件」→ 详情 / 关联 | syscall PID 与 root Agent 区分 |
| 4. Agent 域名外联 | 1.5 分钟 | 「Agent 域名监控」 | 网络元数据可以定位目标，但不等于明文 API 账单 |
| 5. Agent → SubAgent | 2 分钟 | 放射图 / 瀑布图 / 生命周期信号 | 进程关系与逻辑调用证据有清楚界限 |
| 6. 原生文件工具与路径策略 | 2 分钟 | 文件管理 / 编辑器 / 路径权限 | 普通用户文件操作与内核策略授权隔离 |
| 7. Web 深度功能和安全控制 | 2 分钟 | Dashboard、Hooks、Config、内核状态 | 多层控制必须有运行时证据 |
| 8. 工程验证与收尾 | 1–2 分钟 | 预检 / 回放 / 版本记录 | 可验证、可复现、不夸大 |

### 讲解开场（约 30 秒）

> AI Agent 可以声称它运行了什么命令，但这不等于系统真实发生的行为。我们用 eBPF 获得内核事实，再用 Hook/Wrapper/注册上下文关联一次 Agent 会话，最后通过 Renew 桌面和 Web 工作台呈现可核查的证据。只有需要主动拦截时，我们才在授权条件下启用 wrapper、cgroup 或 BPF LSM 策略。

## 2. 桌面健康：必须先展示「采集到底有没有发生」

**操作**

1. 启动 Renew，打开「我的设备」。
2. 展示后端、collector、系统/事件流状态以及最近活动。
3. 进入「系统诊断」，说明 loss/drop/queue 等指标与连接异常提示。
4. 可演示「暂停事件」后状态刷新仍持续；**暂停仅影响 UI**，不暂停 BPF 采集或策略。

**必须说清**

- UI 没有新事件 ≠ BPF 挂载正常；更不等于主机安全。
- 独立 Renew 后端通过私有 Unix Socket 传递摘要/鉴权请求；本地内置模式并不默认打开 HTTP 8080。
- 事件完整详情需要显式查询，列表只保存有界摘要。

**兜底**：采集未就绪时保留诊断画面，用先前准备的可识别来源的日志/录屏说明预期；不伪装实时事件。

## 3. 内核事实与 Agent 归因

**事前准备**：已有被检测/跟踪的目标进程，以及符合当前 capture/monitor 规则的本地测试文件。注意新的默认噪声规则会降低 `/tmp` 和 `/proc` 的普通事件，因此**测试内容不要完全依赖这些路径出现的低风险记录**。

可在你控制的工作目录准备：

```bash
mkdir -p "$HOME/agent-ebpf-demo"
printf 'demo\n' > "$HOME/agent-ebpf-demo/input.txt"
python3 -c 'from pathlib import Path; p=Path.home()/"agent-ebpf-demo"/"input.txt"; print(p.read_text())'
```

**操作**

1. 进入「事件」，按实际执行程序、PID、类型、风险过滤。
2. 选中一次记录，查看真实 syscall PID、时间、文件/网络目标、可用的 root Agent/run 上下文。
3. 使用「查看进程树」「父进程」或「Agent 会话」跳转；如是 shell/python 间接编辑文件，比较**实际执行器**与**有证据的 Agent 归属**。

**必须说清**

- PID 注册和 fork/clone 证据帮助关联后代；不会把所有 `python`、`bash` 默认识别成 Agent。
- 一条 `write` 只有满足文件相关证据时才算文件编辑，不能把套接字写入误算为文件改动。
- 已退出进程、时间戳缺失、PID 重用无法事后准确恢复所有关系。

## 4. Agent 域名目标：展示「外联证据」的边界

**操作**：打开「Agent 域名监控」，挑选实际采到的目标，演示 Agent/域名/IP 筛选、风险优先级、目标跳转事件。可在功能需要时查看 Claude/Codex 本地**只读配置候选**，不要打开真实密钥或用户私有数据。

**必须说清**：域名或 IP 线索来自既有事件/网络元数据，不能自动得出每个模型调用的 token、请求 body 或账单。没有主机名时可能只是 IP-only，不能用反查域名臆造 HTTPS Host。配置文件声明也不是一次网络访问已成功的证据。

## 5. 新功能：Agent → SubAgent 可视化

**前提**：有在线 `/ws/system` 进程快照和可识别的 Agent 根进程。演示前应确认正在运行的 Agent 确实产生了至少一个 OS 子进程，否则展示「无当前后代」是正常结果。

**操作**

1. 在原生桌面选「Agent 派生」，选定一个 root Agent。
2. 切换「放射图」与「瀑布图」，选中同一节点，展示事件/进程详情交叉跳转。
3. 解释普通 shell 执行器、其它 Agent 可执行程序的**子 Agent 候选**和 Hook 生命周期信号之间的区别。
4. 若已显式启用 Claude Code `SubagentStart`、`SubagentStop`，展示带有实际时间的信号；未启用时明确不展示该信号。

**关键边界**

- **放射图**：实时 PID/PPID，不是 SDK 的 parent span。
- **瀑布图**：进程创建到快照时刻的区间，不是任务执行耗时、也不是退出时间。
- **限制**：最多 72 节点与 6 层；重用 PID、过期快照不能拼成所谓历史全景图。
- **独立 Hook**：两个生命周期事件可分别安装/卸载；不保存原始 `agent_id` / prompt，也不凭两个孤立事件凑出跨 RPC 逻辑调用链。

可将页面与 [Agent 派生功能说明](/frontend/renew#4-agent-subagent-派生图-当前新增) 一起展示。

## 6. 原生工具与访问控制

**操作**

1. 在「工具 → 文件管理」浏览用户演示目录，新建/重命名临时文本。
2. 双击文本，用 MyGo 原生编辑器修改、保存；可展示多标签、未保存提醒。演示冲突保护时不要对真实文件进行外部并发修改。
3. 打开「跟踪范围」与「路径权限」界面作对比，不直接对系统凭据下发危险策略。

**讲解边界**

| 功能 | 主体与效果 |
| --- | --- |
| 文件管理 / 编辑器 | Renew **未提权的普通桌面进程**执行，仍受用户文件权限约束 |
| 跟踪规则 | 决定采集目标或递归路径前缀，不等同文件拒绝 |
| 忽略路径 | 减少低风险噪声，不抹掉 ALERT/BLOCK/DENY |
| BPF LSM 精确路径保护 | 明确授权的内核读写阻断，受 gate、支持的 hook 和精确路径约束 |

如需 live LSM smoke，先阅读 [路径权限说明](/security/mingjing-path-access)，在隔离主机用临时普通文件完成，**不要**测试 `/etc/shadow`、用户 SSH 私钥等真实文件。

## 7. Web 研究工作台、策略和 IPC

```bash
# 与原生桌面独立的 Web 开发流程
make dev
```

在 Vue 工作台中选取 **Dashboard → Network → Execution Graph → Hooks / Config** 的一条真实事件，说明网络/进程/工具链如何穿透不同视图。只有授权与 gate 条件满足时，才展示以下可选验证：

- **Wrapper**：明确让命令经 wrapper 执行，演示 ALLOW/BLOCK/ALERT/REWRITE；命令不经过 wrapper 的直接执行不受它保证。
- **cgroup eBPF**：在隔离范围验证精确 IP/端口规则、attach 状态、连接失败和计数器，不能说是任意 CIDR 防火墙。
- **BPF LSM**：选临时精确路径验证文件/exec deny，核对拒绝返回及内核指标；保存一条规则不意味着成功生效。
- **组件化阶段**：说明 `agentscope`、`eventnoise`、`collectorcodec` 等可测库与实验性 `componentipc`、`componentproc`、`componentshm`。这些**不是**默认替换生产 Collector/Engine/Controller 的多服务拓扑。

::: warning 实验边界
分组件 Unix Socket/HMAC、memfd/eventfd、共享内存 ACK 和 opt-in 子进程路径用于验证隔离与传输机制；不能把 schema 验证的 ACK 误称为 ML 风险判断或执行策略成功。核对 [组件 IPC 设计](/architecture/component-ipc)。
:::

## 8. 验证证据：用真实结果代替修辞

演示可以引用运行在当前环境的命令和报告，不应单纯用「零开销」「完美隔离」「绝对安全」等无法验证的表达。

```bash
make os-enforcement-preflight
make os-enforcement-check
make runtime-benchmark
cd desktop/renew && go test ./...
```

| 证据 | 可以支持的结论 | 不能代替 |
| --- | --- | --- |
| `go test` / 前端构建通过 | 对应代码可编译或断言通过 | 实际内核挂载及现场策略执行 |
| rootless component IPC / SHM 测试 | 协议/队列/ACK 原型满足测试条件 | 默认线上组件分离与可靠重放 |
| runtime replay summary | 离线数据集上的延迟、召回/误报或上下文回归指标 | 当前主机真实 ringbuf loss / kernel drop |
| collector health | 实际采集链路和丢弃状态 | 主机没有恶意行为 |
| isolated OS smoke | 指定 hook/规则/内核组合下发生过真实 deny | 任意平台或全路径/全网络默认覆盖 |

**实测性能数字**请连同提交、内核、CPU、工作负载、采样规模和统计口径一起给出。无法现场复现时应明确标注「历史评测」或「预先录制」，不插入未经本次确认的新百分比。

## 9. 结束时的三句话

1. **可观测**：系统调用事实与 Agent 语义被合并为可检查的关联证据，而不是只信任终端文本。
2. **可调查**：MyGo 原生桌面降低日常监控复杂度，同时保留 Vue Web 工作台作深入分析。
3. **可约束但不夸大**：用户态与内核策略在正确前提下起作用；捕获、关联、进程图、Hook 信号、实验 IPC 各自有明确的证据与权限边界。

## 10. 演示结束检查

- [ ] 记录演示的 Git 提交、内核版本、实际 collector 状态与截图来源。
- [ ] 移除所有仅为演示创建的 wrapper/cgroup/LSM 规则。
- [ ] 停止本地临时 HTTP/Agent 服务，清理用户演示目录。
- [ ] 如安装 Hook，检查是否要移除本次新增的 SubagentStart/Stop 配置。
- [ ] 不保存明文凭据、真实用户 prompt/TLS body 或不必要的敏感事件详情。
- [ ] 将真实测试输出与离线演示素材清晰区分。

## 关联资料

[产品演示导览](/guide/demo-tour) · [Renew 用户指南](/frontend/renew) · [系统架构](/architecture/overview) · [OS Enforcement](/backend/ebpf-os-enforcement) · [评测报告](/delivery/evaluation) · [第三方和 AI 使用披露](/delivery/compliance)。
