---
title: Agent eBPF Filter 在线填报字段草稿
author: 负责人待填
lang: zh-CN
---

# 中国国际大学生创新大赛（2026）在线填报字段草稿

> 用途：复制到全国大学生创业服务网或学校校赛系统。不同学校字段名称可能不同，提交前以系统页面为准。所有 `【待填】` 必须替换为真实信息。

## 1. 项目基础字段

| 字段 | 建议填写 |
| --- | --- |
| 项目名称 | Agent eBPF Filter：面向 AI Agent 的内核级运行时安全与可观测平台 |
| 项目简称 | Agent eBPF Filter |
| 参赛赛道 | 高教主赛道 |
| 组别 | 创意组（未注册公司时）；如已有公司与股权材料，改为创业组 |
| 项目类别 | 新工科类 / 人工智能+ / 网络空间安全 |
| 项目所属领域 | 人工智能安全、系统安全、eBPF、开发者工具治理 |
| 是否已注册公司 | 否【如已注册，请改为“是”并补企业信息】 |
| 项目负责人 | 【待填】 |
| 负责人所在学校/学院/专业 | 【待填】 |
| 负责人手机号/QQ/邮箱 | 【待填】 |
| 指导教师 | 【待填】 |
| 团队成员数 | 【待填，3-15人】 |
| 项目阶段 | 已有可运行原型，正在校赛/省赛材料完善与试点验证阶段 |
| 项目关键词 | AI Agent 安全；eBPF；BPF LSM；cgroup；进程树追踪；运行时审计；语义-事实一致性；执行图谱 |

## 2. 项目简介（短版，约 200 字）

Agent eBPF Filter 是面向 AI Agent 与开发者 CLI 的内核级运行时安全与可观测平台。项目通过 eBPF tracepoints、cgroup/connect、UDP sendmsg 和 BPF LSM 等机制，实时捕获 Agent 及其子进程的进程执行、文件访问、网络连接和策略命中行为；再结合 wrapper、CLI hooks 和 Python/Node 适配器获取 Agent run、task、tool call 等语义信息，建立“Agent 声称要做什么”与“操作系统实际发生了什么”的跨层证据链。平台可识别敏感文件访问、异常外联、工作区逃逸、下载即执行、多 Agent 文件竞争等风险，并提供告警、阻断、执行图谱、回放与训练样本导出能力。

## 3. 项目简介（系统常见长文本版，约 500-800 字）

随着 Codex、Claude Code、Gemini CLI、GitHub Copilot 等 AI Agent 进入软件研发流程，Agent 已不再只是生成文本，而是能够读取仓库、修改代码、运行测试、安装依赖、启动 shell、访问网络并调用外部工具。传统 Prompt 审查只能看到模型输入输出，传统 EDR 又缺少 Agent 任务语义，导致企业和高校很难回答一次 Agent 执行中“哪个任务触发、哪个工具调用、哪个子进程访问了哪些文件和网络端点、是否偏离用户意图”等关键问题。

Agent eBPF Filter 以 Linux eBPF 为底层事实采集与策略阻断基础，构建内核事实层、语义声明层、关联分析层和控制展示层。内核事实层通过 syscall tracepoints 捕获 exec/open/connect/send/recv/fork/unlink 等行为，通过 cgroup/connect 与 UDP sendmsg 对网络目的地进行内核级拒绝，通过 BPF LSM 对可执行文件、文件/目录名和高风险操作进行同步阻断。语义声明层通过 agent-wrapper、原生 CLI hooks、Python/Node 适配器接入 Agent run、task_id、tool_call_id、trace_id 等上下文。关联分析层将内核事件、wrapper 事件、hook 事件归一到 EventEnvelope，形成从 Agent Run 到 Tool Call、Process、File、Network、Policy Decision 的执行图谱。控制展示层提供 Dashboard、Network Flow、Execution Graph、Security Policies、低代码 eBPF 策略编辑、JSONL 回放、OTLP/Prometheus/MCP 导出等能力。

项目的目标不是再造一个 Agent，而是为 Agent 运行时提供“黑匣子 + 刹车系统”：看得见真实行为，说得清因果链路，必要时拦得住高风险动作。它适用于高校 AI 安全与 eBPF 教学、企业 AI coding 工具治理、研发效能平台安全审计、CTF/攻防演练和本地 Agent 沙箱实验。

## 4. 项目背景/痛点

1. AI Agent 正从聊天助手变成具有终端、文件、网络、浏览器、MCP 工具和 skills 权限的执行型系统，风险从“回答错”延伸为“执行错”。
2. OWASP LLM06 将过度工具、过度权限和过度自治列为关键风险；Five Eyes / CISA / NSA / NCSC 指南强调 Agent 采用需要持续可见性、最小权限、隔离和运行期保证。
3. GitGuardian 2026 secrets sprawl 调研显示，AI 辅助开发、AI service key 和 MCP 配置正在放大密钥泄露风险，开发者本机凭据成为 Agent 时代的新攻击面。
4. Prompt/内容安全无法证明 Agent 实际启动了哪些子进程、访问了哪些路径、连接了哪些外部地址；传统 EDR 又不知道这些行为属于哪个 Agent run、哪个 task、哪个 tool call。
5. 用户真正需要解决三个“不可见”：Agent 意图不可见、子进程链路不可见、阻断效果不可见。

## 5. 解决方案/产品功能

项目把用户痛点落成“看得见、说得清、拦得住”的三层闭环：

- **看得见**：实时采集 Agent 及子进程的进程执行、文件访问、网络连接、策略命中事件。
- **说得清**：通过 PID 注册、wrapper、native hooks、父子进程继承和 cgroup 归因，关联 Agent run、task、tool_call、trace 等上层语义。
- **拦得住**：使用 BPF LSM 和 cgroup hook 对高风险文件、可执行程序、网络目的地和端口进行确定性阻断。
- 通过 Execution Graph 展示 Agent Run、Tool Call、Process、File、Network、Policy Decision 节点和因果边。
- 识别 SECRET_ACCESS、UNEXPECTED_NETWORK_EGRESS、UNEXPECTED_CHILD_PROCESS、WORKSPACE_ESCAPE、RESOURCE_WASTING_LOOP、MULTI_AGENT_FILE_CONTENTION 等风险。
- 支持事件录制、JSONL 回放、训练样本导出、OTLP/Prometheus/MCP 集成。
- 提供低代码可视化 eBPF/策略编辑器，降低教学和策略配置门槛。

## 6. 创新点

1. **BPF LSM 策略执行**：将关键阻断能力下沉到 `bprm_check_security`、`file_open`、`file_permission`、`inode_*` 等 hook，能够在高风险行为完成前返回 `EACCES` 或拒绝操作。
2. **内核态对象匹配**：对可执行路径、可执行名、文件/目录 basename、cgroup、IP、IPv6、TCP/UDP 端口等进行低延迟确定性匹配。
3. **进程树归因**：注册 Agent 进程后自动关联 shell、python、node、git、npm、curl 等子进程，继承 agent_run_id、task_id、tool_call_id 和 trace_id。
4. **语义-事实一致性检测**：把 hook/wrapper 中的工具意图与 eBPF 事实事件对齐，发现只读任务中的敏感文件访问、异常外联、下载即执行等偏离行为。
5. **可回放执行图谱**：把平铺日志升级为 Agent 执行图，支持事故复盘、答辩展示、策略生成和训练数据沉淀。
6. **教育与商业兼容**：既能用于 eBPF/AI 安全课程实验，也能面向企业 AI coding 治理提供私有化部署。

## 7. 技术路线

1. 通过 `backend/ebpf/agent_tracker.c`、`cgroup_sandbox.c`、`lsm_enforcer.c` 构建内核采集与阻断程序，并由 bpf2go 生成 Go 绑定。
2. Go 后端负责自提升、加载/挂载 eBPF、pin maps/links、消费 ring buffer、提供 REST/WS API 和策略管理。
3. Python/Node 适配器与 agent-wrapper 负责 PID 注册和命令控制；原生 hooks 对接 Claude Code、Gemini CLI、Codex、GitHub Copilot 等工具。
4. 后端将内核事件、wrapper 事件、hook 事件统一为 EventEnvelope，补充 agent/task/tool/trace、PID/TGID/PPID、cgroup、cwd、risk_score 等上下文。
5. semantic_alerts 模块识别敏感文件、异常网络、工作区逃逸、资源浪费循环和多 Agent 文件竞争。
6. Vue 前端展示 Dashboard、Network、Execution Graph、Explorer、Hooks、Plugins、ML、Configuration 等页面。
7. 通过 replay benchmark、os-enforcement smoke、前后端测试和演示脚本验证功能、性能和阻断效果。


## 7.1 代码实现证据

| 能力 | 代码位置 | 证明点 |
| --- | --- | --- |
| 内核文件/执行阻断 | `backend/ebpf/lsm_enforcer.c` | `bprm_check_security`、`file_open`、`file_permission`、`inode_*` 命中策略后返回 `-EACCES`。 |
| 内核网络阻断 | `backend/ebpf/cgroup_sandbox.c` | `connect4/connect6/sendmsg4/sendmsg6` 支持 cgroup、IP、端口阻断。 |
| Agent 上下文继承 | `backend/event_context.go` | `processContextStore` 让子进程继承 agent_run_id、task_id、tool_call_id、trace_id。 |
| 语义告警 | `backend/semantic_alerts.go` | 识别密钥访问、异常外联、工作区逃逸、Token 外泄风险和多 Agent 文件竞争。 |
| 执行图谱 | `backend/execution_graph.go` + `frontend/src/views/ExecutionGraph.vue` | 将事件转换为 Agent Run → Tool Call → Process → File/Network/Policy 证据图。 |

## 8. 商业模式

| 版本 | 目标用户 | 收费/转化方式 |
| --- | --- | --- |
| Community 开源核心 | 个人开发者、高校课程 | 免费开源，扩大技术影响力与样本来源。 |
| Team Pro | 高校实验室、AI coding 小团队 | 年费订阅，提供团队策略、报表、训练样本和协作功能。 |
| Enterprise 私有化 | 企业研发效能、安全合规部门 | 私有化部署、多节点、SSO、审计留存、SIEM/DevOps 集成。 |
| Education Kit | 高校、培训机构、竞赛队 | 课程实验、靶场脚本、讲义、训练营和竞赛辅导。 |
| Consulting | 企业 PoC/专项治理 | Agent 安全基线、红队 replay、策略调优和验收报告。 |

## 9. 市场与应用场景

- 高校网络空间安全、软件工程、人工智能实验室：用于 AI Agent 安全、eBPF、系统编程课程与科研。
- 企业研发团队：治理 Codex、Claude Code、Gemini CLI、Copilot 等自动化编码工具。
- 安全运营团队：审计 Agent 是否访问敏感文件、异常外联、执行危险命令。
- 研发效能平台：将 Agent 行为证据接入 DevOps、SIEM、OTLP、Prometheus 等体系。
- CTF/攻防演练：构建可观测、可阻断、可回放的 Agent 沙箱实验环境。

## 10. 项目进展

| 模块 | 进展 |
| --- | --- |
| eBPF 内核程序 | 已覆盖 syscall tracepoints、cgroup sandbox、BPF LSM enforcer、TLS capture opt-in 路径。 |
| Go 后端 | 已实现 HTTP/WS API、事件归一化、策略配置、运行时配置、OTLP/Prometheus/MCP 导出。 |
| Vue 前端 | 已实现 Dashboard、Network Flow、Execution Graph、Explorer、Executor、Hooks、Plugins、ML、Configuration 等页面。 |
| Wrapper/Adapters | 已实现 agent-wrapper、Python/Node PID 注册辅助和多类 CLI hook 接入。 |
| 测试验证 | 已有 Go 单元测试、runtime benchmark、os-enforcement preflight/check/smoke 脚本。 |
| 竞赛材料 | 已生成申报书、商业企划书、项目答辩/路演 PPT、PDF 与提交包。 |

## 11. 团队成员字段模板

| 排序 | 姓名 | 学号 | 学校/学院/专业 | 年级 | 角色 | 分工 | 手机/邮箱 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 1 | 【待填】 | 【待填】 | 【待填】 | 【待填】 | 项目负责人 | 总体架构、答辩、进度管理、竞赛材料统筹 | 【待填】 |
| 2 | 【待填】 | 【待填】 | 【待填】 | 【待填】 | eBPF/内核开发 | tracepoints、BPF LSM、cgroup hook、性能优化 | 【待填】 |
| 3 | 【待填】 | 【待填】 | 【待填】 | 【待填】 | 后端开发 | Go API、事件模型、策略引擎、测试 | 【待填】 |
| 4 | 【待填】 | 【待填】 | 【待填】 | 【待填】 | 前端开发 | Vue Dashboard、Execution Graph、低代码 UI | 【待填】 |
| 5 | 【待填】 | 【待填】 | 【待填】 | 【待填】 | 算法评测 | semantic_alerts、ML/LLM scoring、benchmark | 【待填】 |
| 6 | 【待填】 | 【待填】 | 【待填】 | 【待填】 | 商业调研 | 市场访谈、竞品分析、商业计划、路演 | 【待填】 |

## 12. 指导教师字段模板

| 排序 | 姓名 | 学校/学院 | 职称 | 研究方向 | 指导内容 | 手机/邮箱 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 【待填】 | 【待填】 | 【待填】 | 系统安全 / AI 工程 / 创新创业 | 技术路线、项目管理、竞赛材料审核 | 【待填】 |
| 2 | 【待填】 | 【待填】 | 【待填】 | 商业管理 / 网络安全 / 软件工程 | 商业计划、产品落地、试点资源 | 【待填】 |

## 13. 三年规划

| 阶段 | 时间 | 目标 | 成果 |
| --- | --- | --- | --- |
| P0 校赛准备 | 2026.05-2026.06 | 完成材料、演示和规则核验 | 申报书、商业企划书、PPT、演示视频、提交包 |
| P1 场景验证 | 2026.06-2026.08 | 建立良性/恶意 Agent replay 套件 | 检测率、阻断率、延迟、资源开销报告 |
| P2 试点应用 | 2026.08-2026.10 | 在实验室/课程/企业 PoC 中试点 | 用户反馈、试点证明、功能迭代清单 |
| P3 成果固化 | 2026.10-2027.03 | 申请知识产权并完善产品 | 软著、专利、论文、开源版本、课程实验 |
| P4 产品化 | 2027.03-2027.12 | 推出 Team Pro / Enterprise / Education Kit | 付费试点、合作协议、年度收入复盘 |

## 14. 附件/支撑材料清单

- 申报书 DOCX/PDF。
- 商业企划书 DOCX/PDF。
- 路演 PPTX/PDF。
- 项目演示视频（建议 3-5 分钟）。
- 系统截图：Dashboard、Execution Graph、Security Policies、Network Flow、Plugins 低代码编辑器。
- 测试报告：os-enforcement smoke、runtime benchmark、Go 单元测试、前端构建截图。
- 代码仓库链接、主要 commit 记录、贡献人说明。
- 软著/专利/论文/获奖证明（如有）。
- 用户访谈、问卷、试点意向书或导师/实验室推荐证明。
- 若走创业组：营业执照、统一社会信用代码、股权结构、法定代表人证明、财务/运营证明。

## 15. 系统提交注意事项

1. 项目名称、团队成员、指导教师排序必须和校内系统、全国大学生创业服务网、PPT、申报书保持一致。
2. 如果学校要求“大创网报名截图”，必须先完成全国大学生创业服务网报名，再上传截图到校内系统。
3. 若进入校复赛/省赛，很多学校不允许再修改成员和指导教师排序，提交前必须逐项核对。
4. 申报书中的 `【待填】` 必须替换；商业计划中的财务预测应尽量补真实调研依据。
5. 涉及安全演示时只使用授权本地环境和脱敏数据，不上传真实密钥、真实用户日志或敏感流量内容。
