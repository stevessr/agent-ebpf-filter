# 2025–2026 顶会前沿全景调研与 MingJing v3 下一代研究构想

**生成日期**：2026-11-20  
**立足基线**：Agent eBPF Filter（明镜 / MingJing v2：`v2` 已具备三模态接入、Pre-Reserve 丢包序号链、哈希链取证、结果感知语义告警、LPM-Trie CIDR + `(s_dev, i_ino)` 14-Hook BPF LSM 及 HTTP 隐私网关）

---

## 一、2025–2026 顶会安全与系统研究前沿脉络

对照近期 **IEEE S&P、USENIX Security、ACM CCS、NDSS、OSDI/SOSP、ICML/NeurIPS、DSN、ESEC/FSE、IEEE TPS** 等会议与最新预印本成果，当前自主 AI 智能体安全研究集中呈现出四条主线与一个关键断层：

### 1. 应用层信息流控制（IFC）与能力租约窄化
- **FIDES**（Microsoft Research, arXiv:2505.23643, 2025）与 **CaMeL**（arXiv:2503.18813, 2025）：在 LLM 规划器/解释器内部跟踪机密性（Confidentiality）与完整性（Integrity）标签，尝试在设计层面隔离“受信任控制流”与“不可信外部数据”。
- **Progent**（arXiv:2504.11703, 2025/2026）：提出可编程工具权限 DSL，结合 SMT 验证器保证权限在执行过程中的**单调收紧（Monotonic Confinement）**。
- **IntentCap**（AgenticOS 2026, arXiv:2609.14631）与 **AgentFlow**（2026）：强调能力（Capability）必须随当前任务意图（Task Intent）与上下文来源动态收窄，约束数据流边上的跨工具发布权限。
- **核心盲区（OS-Drop-Down Gap）**：上述系统全部停留在 **LLM 编排器 / Python 运行时工具封装边界**。一旦编码智能体执行 `bash -c`、构建脚本（`npm install` / `pytest` / `make`）、或者运行本地 MCP `stdio` 子进程，应用层 IFC 标签立即断裂——非信任文本写入临时文件、派生孙子进程或硬链接/重命名别名读取凭证后的操作系统真实外传完全脱离编排器视野。

### 2. 多工具跨域污染、因果归因与资源耗尽攻击
- **Les Dissonances / XTHP**（NDSS 2026）：系统揭示了多工具智能体中的**跨工具收割与污染（Cross-Tool Harvesting and Polluting, XTHP）**攻击——恶意外部观测数据污染上游工具输出或临时状态，借后续合法工具之手外发机密。
- **AttriGuard**（USENIX Security 2026）：提出基于**反事实影子重放（Counterfactual Shadow Replay）**的工具调用因果归因防御间接提示词注入，但需要对候选动作并发触发多路 LLM 重放推理，计算与延迟成本高昂，且仍停留在应用层。
- **AgentDoS**（USENIX Security 2026）：揭示智能体工作流中的递归进程派生与资源耗尽（Fork/API DoS）攻击面。

### 3. eBPF 内核可观测性与虚拟化
- **Agent-Warden**（IEEE TPS 2026, arXiv:2609.38245）：利用 `BPF_MAP_TYPE_TASK_STORAGE` 与 `BPF_MAP_TYPE_INODE_STORAGE` 在内核构建进程-普通文件因果溯源图（Provenance DAG），开销仅 0.2%–3.5%，但其架构聚焦于**事后异步溯源建图**，未实现同步拦截、能力租约窄化或应用层意图编译。
- **vBPF**（OSDI 2026）与 **AgentSight**（arXiv:2508.02736, 2025）：分别聚焦多租户 eBPF 延迟绑定虚拟化与基于 TLS uprobe 的行为观测（后者因常驻明文解密存在研发隐私风险）。

### 4. MCP 协议生态攻击与契约治理
- **A First Look at Security Issues in the MCP Ecosystem / MCPInspect**（DSN 2026）：对 67,000+ 公开 MCP 服务器的大规模审计暴露出广泛的元数据劫持与服务端污染漏洞。
- **AgentBound**（ESEC/FSE 2026）与 **MCP Threat Modeling**（2026）：指出 **Tool Poisoning**（在工具描述/参数 schema 中嵌入 `<IMPORTANT>`、隐藏指令或不可见 Unicode Tag-Plane `U+E0000–U+E007F` / BiDi 控制符劫持模型）以及 **Mid-Session Rug Pulls**（会话建立后动态篡改工具定义实现权限逃逸）正成为最紧迫的客户端侧威胁。

---

## 二、四个面向下一代顶会的具体研究 Idea 构想

基于“MingJing 已有 14-Hook BPF LSM + `agentboundary` 策略边界引擎 + `domainforwardproxy`”的工程底座，我们提出四个互补且目标会议明确的下一代研究构想（**本轮工程原型与实证重点落地 Idea 1 + Idea 2，并覆盖 Idea 4 的 `spawn_budget` 原语**）：

| 编号 | 研究构想名称 | 目标会议 | 直击的 SOTA 断层 | 核心机制设计 | 验证与评测路线 |
| :--- | :--- | :--- | :--- | :--- | :--- |
| **Idea 1 (旗舰)** | **`K-IFC`：内核原生信息流格与单调能力租约** | IEEE S&P / USENIX Security / OSDI | CaMeL/FIDES/Progent/IntentCap 无法穿透 `execve`/Shell/临时文件；Agent-Warden 仅审计不阻断 | 基于 `BPF_MAP_TYPE_TASK_STORAGE` + `INODE_STORAGE` 的 8-Hook 同步 LSM 执行器；应用层 `agentboundary`/Prompt 自动编译不可递增的 `(conf_taint, integ_taint, cap_mask, spawn_budget)` 租约；内核实时阻断“致命三要素（Lethal Trifecta）”外连、XTHP 跨进程/硬链接/重命名 inode 污点外泄与非授权破坏性文件操作 | 编译 `ifc_enforcer.c` ELF 核验 + ABI 字节对齐测试 + AgentDojo（66 任务 / 427 攻击对）+ XTHP/AgentDoS 20 场景跨进程对抗 |
| **Idea 2 (配套)** | **`MCP-ContractGuard`：防投毒与防中途变脸的契约编译器** | ACM CCS / DSN / ESEC-FSE | MCPInspect 侧重静态预检；运行时缺乏对“描述投毒（含不可见 Tag/BiDi 字符）+ 会话中途 Rug Pull + 能力越权编译到内核”的闭环防护 | 为每个 MCP 工具生成 `(schema_sha256, desc_sha256)` 密码学摘要锚定；扫描 `<IMPORTANT>` 覆写指令、Unicode Tag-Plane (`U+E0000–U+E007F`) 与 BiDi 重排符；检测到 Rug Pull 立即零权限Fail-Closed隔离并同步回收内核租约 | 24 组对抗契约集（良性工具、指令投毒、不可见字符走私、Rug Pull、权限越权）精确率/召回率/延迟评测 |
| **Idea 3** | **`LightAttri`：微秒级确定性因果差分归因** | USENIX Security / NDSS | AttriGuard（USENIX'26）依赖多路反事实 LLM 影子重放，耗时数秒且成本高 | 结合 K-IFC 内核任务/inode 污点边与工具参数绑定集合，在不重放 LLM 的前提下实时判定敏感/外连行为是否具有用户原始 Prompt 因果必需性 | 与纯能力位规则在 AgentDojo 跨域写入/删除场景对比误报与漏报边界 |
| **Idea 4** | **`eBPF AgentDoS Quotas`：每运行实例内核资源配额** | USENIX Security / OSDI | AgentDoS 揭露递归派生与高频套接字/写入耗尽风险，传统 cgrouppids 无法按单次 `agent_run_id` 窄化 | 在 `lsm/task_alloc` 维护沿 `TASK_STORAGE` 递减传播的 `spawn_budget`（并在后续可扩展套接字/写令牌桶），配额归零时同步 `-EACCES` | 在 `task_alloc` 中落地 `spawn_budget` 并验证递归 Fork Storm / DoS 拦截 |

---

## 三、本轮聚焦落地规范：`K-IFC`（Idea 1）+ `MCP-ContractGuard`（Idea 2）

### 3.1 形式化格（Lattice）与单调能力租约定义

每个内核任务标签 $L_t = (C_t, I_t, P_t, B_t)$ 驻留在 `BPF_MAP_TYPE_TASK_STORAGE` 中，每个 inode 标签 $L_i = (C_i, I_i, r_i)$ 驻留在 `BPF_MAP_TYPE_INODE_STORAGE` 中：
1. **机密性污点格 $C \subseteq \{\text{CONF\_SECRET}, \text{CONF\_PRIVATE}\}$**（按位或单调递增：$C \leftarrow C_{\text{parent}} \cup C_{\text{inode}}$）。
2. **完整性污点格 $I \subseteq \{\text{INTEG\_UNTRUSTED}, \text{INTEG\_EXTERNAL\_NET}, \text{INTEG\_TOOL\_OUTPUT}\}$**（一旦读取不可信文件、网络响应或外部工具返回内容，对应比特位在任务及其所有后代中永久单调置位）。
3. **能力掩码 $P \subseteq \{\text{CAP\_NET\_EGRESS}, \text{CAP\_UNTRUSTED\_EGRESS}, \text{CAP\_FILE\_WRITE}, \text{CAP\_FILE\_UNLINK}, \text{CAP\_EXEC\_UNTRUSTED}, \text{CAP\_SECRET\_READ}\}$**：每次子智能体派生、工具调用或 MCP 契约绑定必须满足单调窄化不等式：
   $$P_{\text{child}} = P_{\text{parent}} \cap P_{\text{requested}}, \quad C_{\text{child}} = C_{\text{parent}} \cup C_{\text{requested}}, \quad B_{\text{child}} \le \min(B_{\text{parent}}, B_{\text{requested}})$$
4. **同步内核裁决不变量（`ifc_enforcer.c`, 8 个 BPF LSM 钩子）**：
   - **进程继承与 AgentDoS 遏制（`lsm/task_alloc`）**：自动沿 `fork/clone` 向子进程 `TASK_STORAGE` 拷贝 `(conf_taint, integ_taint, cap_mask)`；启用强制模式且 `spawn_budget == 0` 时返回 `-EACCES`。
   - **双向跨进程 Inode 污染与文件防越权（`lsm/file_open`, `lsm/file_permission`）**：读操作将静态密级映射 `ifc_classified_inodes` 与动态 `ifc_inode_labels` 污点单调并入任务；写操作将任务污点写入目标文件 `INODE_STORAGE`（即使后续经过 `hardlink` 或 `rename` 仍绑定同一内核 `struct inode` 实体），并在未授权写入或非 `CAP_SECRET_READ` 访问机密时同步拦截。
   - **不可信载荷执行阻断（`lsm/bprm_check_security`）**：禁止已污染（或来自不可信 inode）的脚本/二进制在缺乏 `CAP_EXEC_UNTRUSTED` 时执行。
   - **破坏性删除/覆写拦截（`lsm/inode_unlink`, `lsm/inode_rename`）**：当任务携带 `INTEG_UNTRUSTED` 且缺乏 `CAP_FILE_UNLINK` 时同步拒绝 `unlink` 与 `rename`。
   - **致命三要素（Lethal Trifecta）外连断路器（`lsm/socket_connect`, `lsm/socket_sendmsg`）**：对外部非本地（跳过 `AF_UNIX`/`AF_NETLINK`/`127.0.0.0/8` 以保障 `agent-wrapper` UDS 与本地服务）套接字连接/数据报发送执行硬检查：只要同时满足 $(\text{CONF\_SECRET} \in C_t) \land (\text{INTEG\_UNTRUSTED} \in I_t)$ 即无条件返回 `-EACCES` 阻断；此外若缺乏外连或带污点外连权限亦同步阻断。

### 3.2 `MCP-ContractGuard` 契约锚定协议
- **接入阶段**：对 `Description`、`ParameterDescriptions` 与 `InputSchemaJSON` 扫描隐藏控制指令（如 `<IMPORTANT>`、`<SYSTEM>`、`ignore previous` 等）以及不可见 Unicode Tag-Plane (`U+E0000–U+E007F`) / BiDi 双向文本重排控制符；校验证明 MCP 工具 `DeclaredCaps` 未超出用户原始任务租约 $P_{\text{parent}}$；计算 SHA-256 `(schemaHash, descHash)` 锚定记录。
- **调用阶段**：每次工具触发前核对现行 Manifest 摘要是否与锚定快照完全一致；若发生任何变更（Mid-Session Rug Pull），立即将合约标记为已损坏并返回零能力隔离租约（`cap_mask = 0`, `spawn_budget = 0`）。
