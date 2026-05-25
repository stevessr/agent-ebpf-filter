const fs = require("fs");
const path = require("path");
const pptxgen = require("pptxgenjs");

const ROOT = path.resolve(__dirname, "..");
const OUT_DIR = path.join(ROOT, "docs/competition/2026-guochuang/output");
const OUT = path.join(OUT_DIR, "Agent-eBPF-Filter-项目答辩路演PPT.pptx");
fs.mkdirSync(OUT_DIR, { recursive: true });

const pptx = new pptxgen();
pptx.layout = "LAYOUT_16x9";
pptx.author = "Agent eBPF Filter Team";
pptx.subject = "中国国际大学生创新大赛（2026）项目答辩/路演";
pptx.title = "Agent eBPF Filter：面向 AI Agent 的内核级运行时安全与可观测平台";
pptx.company = "负责人待填";
pptx.lang = "zh-CN";
pptx.theme = {
  headFontFace: "Microsoft YaHei",
  bodyFontFace: "Microsoft YaHei",
  lang: "zh-CN"
};

const C = {
  ink: "0B1220", ink2: "111827", muted: "64748B", blue: "2563EB", cyan: "0891B2",
  green: "16A34A", red: "DC2626", gold: "F59E0B", purple: "7C3AED",
  bg: "F8FAFC", card: "FFFFFF", line: "CBD5E1", sky: "DBEAFE", amber: "FEF3C7", mint: "DCFCE7", rose: "FEE2E2", violet: "EDE9FE"
};
let idx = 0;

function slide(bg = C.bg) { const s = pptx.addSlide(); idx += 1; s.background = { color: bg }; return s; }
function tx(s, text, x, y, w, h, opt = {}) {
  s.addText(text, {
    x, y, w, h, margin: opt.margin ?? 0.03,
    fontFace: opt.fontFace || "Microsoft YaHei", fontSize: opt.size ?? 12,
    bold: !!opt.bold, italic: !!opt.italic, color: opt.color || C.ink,
    align: opt.align || "left", valign: opt.valign || "top", fit: opt.fit || "shrink",
    breakLine: opt.breakLine
  });
}
function rect(s, x, y, w, h, fill, line = fill, opt = {}) {
  s.addShape(pptx.ShapeType.rect, { x, y, w, h, fill: { color: fill, transparency: opt.fillT ?? 0 }, line: { color: line, transparency: opt.lineT ?? 0, width: opt.lw ?? 1 } });
}
function card(s, x, y, w, h, opt = {}) {
  s.addShape(pptx.ShapeType.roundRect, {
    x, y, w, h, rectRadius: opt.r ?? 0.12,
    fill: { color: opt.fill || C.card, transparency: opt.fillT ?? 0 },
    line: { color: opt.line || C.line, width: opt.lw ?? 1, transparency: opt.lineT ?? 0 },
    shadow: opt.shadow === false ? undefined : { type: "outer", color: "000000", opacity: 0.10, blur: 1, angle: 45, distance: 1 }
  });
}
function pill(s, x, y, w, text, color = C.blue) {
  s.addShape(pptx.ShapeType.roundRect, { x, y, w, h: 0.28, rectRadius: 0.14, fill: { color }, line: { color, transparency: 100 } });
  tx(s, text, x, y + 0.055, w, 0.12, { size: 8.5, color: "FFFFFF", bold: true, align: "center", margin: 0 });
}
function badge(s) { if (idx > 1) pill(s, 9.15, 5.12, 0.55, String(idx).padStart(2, "0"), C.blue); }
function header(s, sec, title, sub) {
  pill(s, 0.42, 0.32, 0.82, sec, C.blue);
  tx(s, title, 1.38, 0.23, 7.75, 0.34, { size: 22, bold: true, margin: 0 });
  if (sub) tx(s, sub, 0.47, 0.70, 8.65, 0.24, { size: 10.5, color: C.muted, margin: 0 });
  rect(s, 0.46, 1.02, 9.05, 0.03, C.blue, C.blue, { lineT: 100 });
  badge(s);
}
function bullets(s, arr, x, y, w, h, size = 10.2, color = C.ink) { tx(s, arr.map(v => `• ${v}`).join("\n"), x, y, w, h, { size, color, margin: 0.02 }); }
function bcard(s, x, y, w, h, title, arr, color = C.blue) {
  card(s, x, y, w, h, { line: color });
  rect(s, x + 0.14, y + 0.15, 0.08, 0.32, color, color, { lineT: 100 });
  tx(s, title, x + 0.29, y + 0.13, w - 0.38, 0.24, { size: 13, color, bold: true, margin: 0 });
  bullets(s, arr, x + 0.18, y + 0.55, w - 0.32, h - 0.62, 10.1);
}
function metric(s, x, y, w, h, num, label, color = C.blue) {
  card(s, x, y, w, h, { line: "E2E8F0" });
  tx(s, num, x + 0.08, y + 0.10, w - 0.16, 0.32, { fontFace: "Arial", size: 20, bold: true, color, align: "center", margin: 0 });
  tx(s, label, x + 0.08, y + 0.48, w - 0.16, h - 0.50, { size: 8.6, color: C.muted, align: "center", margin: 0 });
}
function line(s, x1, y1, x2, y2, color = C.line, dashed = false) {
  s.addShape(pptx.ShapeType.line, { x: x1, y: y1, w: x2 - x1, h: y2 - y1, line: { color, width: 1.6, beginArrowType: "none", endArrowType: "triangle", dash: dashed ? "dash" : "solid" } });
}
function source(s, text) { tx(s, text, 0.52, 5.36, 8.5, 0.11, { size: 6.8, color: C.muted, margin: 0 }); }

// 01 Cover
{
  const s = slide(C.ink);
  rect(s, 0.34, 0.32, 9.32, 0.08, C.gold, C.gold, { lineT: 100 });
  pill(s, 0.58, 0.62, 1.10, "国创赛2026", C.gold);
  pill(s, 1.84, 0.62, 1.12, "高教主赛道", C.blue);
  pill(s, 3.12, 0.62, 0.96, "创意组", C.purple);
  tx(s, "Agent eBPF Filter", 0.64, 1.28, 5.2, 0.40, { fontFace: "Arial", size: 29, bold: true, color: "FFFFFF", margin: 0 });
  tx(s, "面向 AI Agent 的\n内核级运行时安全与可观测平台", 0.64, 1.78, 5.6, 1.05, { size: 29, bold: true, color: "FFFFFF", margin: 0 });
  tx(s, "让 Agent 的“承诺”和操作系统里的“事实”对齐起来", 0.68, 3.05, 5.5, 0.28, { size: 14, color: "E5E7EB", margin: 0 });
  metric(s, 0.70, 4.12, 1.18, 0.78, "eBPF", "内核事实", C.gold);
  metric(s, 2.05, 4.12, 1.18, 0.78, "LSM", "同步阻断", C.red);
  metric(s, 3.40, 4.12, 1.18, 0.78, "Graph", "执行归因", C.green);
  card(s, 6.28, 1.04, 2.78, 3.70, { fill: "111827", line: C.gold, shadow: false });
  tx(s, "负责人/学院/联系方式", 6.55, 1.38, 2.24, 0.24, { size: 13, color: C.gold, bold: true, align: "center", margin: 0 });
  tx(s, "【待填】\n推荐学院：待填\n负责人：待填\n手机号：待填\nQQ：待填", 6.63, 1.95, 2.10, 1.30, { size: 15, color: "FFFFFF", align: "center", margin: 0 });
  tx(s, "项目答辩 / 路演版 · 2026-05-25", 6.50, 4.18, 2.36, 0.18, { size: 9.5, color: "CBD5E1", align: "center", margin: 0 });
}

// 02 Agenda
{
  const s = slide(); header(s, "00", "路演结构：从真实痛点到代码证据，再到商业落地", "遵循高教主赛道创意组：个人成长、项目创新、产业价值、团队协作。")
  const items = [
    ["调研痛点", "OWASP / Five Eyes / GitGuardian 都指向：Agent 高权限执行链需要 runtime guardrail。"],
    ["用户画像", "开发者怕密钥外泄，安全团队怕不可审计，平台团队怕推广 Agent 后无法追责。"],
    ["技术闭环", "eBPF 事实层 + hooks 语义层 + BPF LSM/cgroup 阻断 + 执行图谱。"],
    ["代码证据", "lsm_enforcer、cgroup_sandbox、event_context、semantic_alerts、execution_graph 已有实现。"],
    ["商业落地", "高校/实验室试点切入，扩展 AI coding 团队、企业私有化与教育服务。"],
  ];
  items.forEach((it, i) => { const y = 1.32 + i * 0.69; card(s, 0.76, y, 8.35, 0.50, { fill: i % 2 ? "FFFFFF" : "F1F5F9", shadow: false }); pill(s, 0.95, y + 0.12, 0.72, String(i + 1).padStart(2, "0"), i % 2 ? C.gold : C.blue); tx(s, it[0], 1.92, y + 0.10, 1.15, 0.20, { size: 13.2, bold: true, margin: 0 }); tx(s, it[1], 3.20, y + 0.12, 5.6, 0.18, { size: 10.2, color: C.muted, margin: 0 }); });
}

// 03 Problem
{
  const s = slide(); header(s, "01", "问题：Agent 安全已从“回答错”变成“三个不可见”", "编码 Agent 会启动终端、修改文件、安装依赖、访问网络，安全边界必须落到运行时。")
  bcard(s, 0.58, 1.22, 2.75, 2.95, "意图不可见", ["EDR 看得到进程，却不知道哪个 run/task/tool_call 触发", "Prompt 网关看得到文本，看不到 OS 行为"], C.red);
  bcard(s, 3.62, 1.22, 2.75, 2.95, "链路不可见", ["危险动作常发生在 shell/python/node/git/npm/curl 子进程", "MCP 配置、skills、依赖脚本扩大本机攻击面"], C.blue);
  bcard(s, 6.66, 1.22, 2.75, 2.95, "阻断不可见", ["事后日志难证明动作是否已被内核拒绝", "安全团队需要可回放、可提交的证据链"], C.gold);
  tx(s, "结论：用户真正要的不是“再加一个大屏”，而是把 Agent 语义与操作系统事实合成可验证证据链。", 0.72, 4.46, 8.55, 0.30, { size: 13.5, bold: true, align: "center", margin: 0 });
  source(s, "调研依据：OWASP LLM06:2025 Excessive Agency；Five Eyes/CISA/NSA/NCSC Agentic AI 指南；GitGuardian 2026 secrets sprawl。");
}

// 04 Scenario chain
{
  const s = slide(); header(s, "02", "典型风险链：Prompt 注入 → 工具误用 → 子进程漂移 → 数据外泄", "项目从执行链而不是单条日志看待 Agent 风险。")
  const nodes = [["Prompt 注入", C.red, "网页 / Issue / README"], ["工具误用", C.gold, "shell / MCP / browser"], ["子进程漂移", C.purple, "python / node / curl"], ["数据外泄", C.blue, "secret / network / TLS"], ["安全闭环", C.green, "告警 / 阻断 / 回放"]];
  nodes.forEach((n, i) => { const x = 0.62 + i * 1.82; card(s, x, 1.60, 1.32, 1.08, { fill: i === 4 ? C.mint : "FFFFFF", line: n[1] }); tx(s, n[0], x + 0.08, 1.78, 1.16, 0.22, { size: 12.5, bold: true, color: n[1], align: "center", margin: 0 }); tx(s, n[2], x + 0.08, 2.16, 1.16, 0.22, { size: 8.4, color: C.muted, align: "center", margin: 0 }); if (i < nodes.length - 1) line(s, x + 1.34, 2.12, x + 1.78, 2.12, C.muted); });
  bcard(s, 0.86, 3.35, 2.55, 0.86, "观测对象", ["进程、文件、网络、策略、工具调用"], C.blue);
  bcard(s, 3.72, 3.35, 2.55, 0.86, "判断依据", ["意图是否与事实行为一致"], C.gold);
  bcard(s, 6.58, 3.35, 2.55, 0.86, "处置动作", ["ALLOW / ALERT / BLOCK / REWRITE"], C.green);
  source(s, "调研案例覆盖：开源 Agent/skills 供应链、MCP 配置密钥、只读任务漂移与异常外联。");
}

// 05 Solution architecture
{
  const s = slide(); header(s, "03", "解决方案：看得见、说得清、拦得住", "以 eBPF 为事实底座，用 hooks/wrapper 补齐 Agent 语义，再进入图谱和策略闭环。")
  const rows = [
    ["Agent / CLI", "Codex、Claude、Gemini、Copilot、脚本与子进程", C.purple],
    ["说得清", "wrapper / native hooks / adapters：run、task、tool_call、intent", C.gold],
    ["看得见", "eBPF tracepoints / cgroup / BPF LSM：exec、file、net、fork", C.blue],
    ["拦得住", "BPF LSM、cgroup、wrapper 策略：ALLOW / ALERT / BLOCK / REWRITE", C.red],
    ["可复盘", "EventEnvelope、semantic_alerts、Execution Graph、JSONL、OTLP、MCP", C.green],
  ];
  rows.forEach((r, i) => { const y = 1.22 + i * 0.68; card(s, 1.0, y, 8.0, 0.50, { fill: i % 2 ? "FFFFFF" : "F1F5F9", line: r[2], shadow: false }); pill(s, 1.20, y + 0.12, 1.25, r[0], r[2]); tx(s, r[1], 2.72, y + 0.13, 5.85, 0.18, { size: 10.4, color: C.ink, margin: 0 }); if (i < rows.length - 1) line(s, 5.0, y + 0.52, 5.0, y + 0.66, C.line); });
}

// 06 Current product
{
  const s = slide(); header(s, "04", "当前原型：已不是概念图，而是可运行工程", "仓库已包含 Go 后端、eBPF 程序、Vue 前端、wrapper、适配器与部署脚本。")
  metric(s, 0.72, 1.32, 1.36, 0.86, "Go", "特权后端/API/策略", C.blue);
  metric(s, 2.30, 1.32, 1.36, 0.86, "eBPF", "tracepoints/LSM/cgroup", C.red);
  metric(s, 3.88, 1.32, 1.36, 0.86, "Vue", "Dashboard/Graph/UI", C.green);
  metric(s, 5.46, 1.32, 1.36, 0.86, "Wrapper", "命令控制/UDS", C.gold);
  metric(s, 7.04, 1.32, 1.36, 0.86, "ML", "风险评分/训练集", C.purple);
  bcard(s, 0.72, 2.68, 2.75, 1.40, "后台能力", ["event_envelope.go：统一事件模型", "semantic_alerts.go：语义风险告警", "execution_graph.go：证据图谱"], C.blue);
  bcard(s, 3.62, 2.68, 2.75, 1.40, "内核能力", ["lsm_enforcer.c：exec/file/inode 阻断", "cgroup_sandbox.c：connect/sendmsg 阻断", "agent_tracker.c：进程事实采集"], C.red);
  bcard(s, 6.52, 2.68, 2.75, 1.40, "产品能力", ["Vue Dashboard / Execution Graph / Config", "低代码 recipe 与 transpiler", "systemd / rc.local / K8s 部署文档"], C.green);
  tx(s, "答辩要点：每个卖点都能指到代码文件、演示页面和验证脚本。", 0.82, 4.68, 8.2, 0.22, { size: 13, bold: true, align: "center", margin: 0 });
}

// 07 Core innovation 1
{
  const s = slide(); header(s, "05", "创新一：BPF LSM + cgroup 形成确定性内核阻断", "不是只做事后日志，而是在 exec/open/read/write/mmap/rename/connect 等路径前置决策。")
  bcard(s, 0.72, 1.28, 2.75, 2.60, "BPF LSM", ["bprm_check_security：执行前检查", "file_open / file_permission：打开与读写", "inode_*：创建、删除、重命名、链接", "命中策略返回 -EACCES"], C.red);
  bcard(s, 3.62, 1.28, 2.75, 2.60, "cgroup 网络", ["connect4/connect6", "sendmsg4/sendmsg6", "按 cgroup、IP、IPv6、端口阻断", "覆盖 TCP/UDP 外联场景"], C.blue);
  bcard(s, 6.52, 1.28, 2.75, 2.60, "策略原则", ["确定性 map lookup", "ML/LLM 只建议不直接内核阻断", "策略通过认证 API 修改", "阻断证据进入执行图谱"], C.gold);
  card(s, 1.10, 4.38, 7.80, 0.50, { fill: C.ink, line: C.ink, shadow: false });
  tx(s, "评委可感知亮点：真实 OS 级拒绝，而不是 UI 上标红。", 1.25, 4.55, 7.5, 0.14, { size: 12.5, color: "FFFFFF", bold: true, align: "center", margin: 0 });
  source(s, "代码证据：backend/ebpf/lsm_enforcer.c；backend/ebpf/cgroup_sandbox.c；scripts/os-enforcement-smoke.sh。");
}

// 08 Core innovation 2
{
  const s = slide(); header(s, "06", "创新二：进程树追踪让 Agent 子进程不再“失踪”", "真实 Agent 行为经常发生在 /bin/sh、python、node、git、npm、curl 等子进程。")
  const y = 2.06;
  const xs = [0.70, 2.15, 3.60, 5.05, 6.50, 7.95];
  const labels = ["Agent", "Tool Call", "shell", "python/node", "file/net", "policy"];
  const cols = [C.purple, C.gold, C.blue, C.cyan, C.green, C.red];
  xs.forEach((x, i) => { card(s, x, y, 1.08, 0.70, { fill: i % 2 ? "FFFFFF" : "F1F5F9", line: cols[i] }); tx(s, labels[i], x + 0.06, y + 0.22, 0.96, 0.16, { size: 10.2, bold: true, color: cols[i], align: "center", margin: 0 }); if (i < xs.length - 1) line(s, x + 1.10, y + 0.35, xs[i + 1] - 0.04, y + 0.35, C.muted); });
  bcard(s, 0.82, 3.42, 2.55, 1.10, "继承字段", ["agent_run_id / task_id / tool_call_id / trace_id"], C.purple);
  bcard(s, 3.72, 3.42, 2.55, 1.10, "事实字段", ["PID/TGID/PPID、comm、cwd、cgroup_id"], C.blue);
  bcard(s, 6.62, 3.42, 2.55, 1.10, "复盘结果", ["一条链路回放所有子进程与对象"], C.green);
  source(s, "代码证据：backend/event_context.go 从父 PID 或 cgroup 继承 Agent 上下文。");
}

// 09 Core innovation 3
{
  const s = slide(); header(s, "07", "创新三：语义-事实一致性检测", "把工具声明的意图与 eBPF 事实对比，识别“说是读文件，实际在外联/读密钥”。")
  bcard(s, 0.60, 1.24, 2.72, 2.90, "语义输入", ["tool_name / task / prompt 摘要", "wrapper 决策请求", "native hook 元数据"], C.gold);
  bcard(s, 3.62, 1.24, 2.72, 2.90, "事实输入", ["exec/open/connect/send", "进程树、路径、网络端点", "BPF LSM/cgroup 决策"], C.blue);
  bcard(s, 6.64, 1.24, 2.72, 2.90, "输出告警", ["SECRET_ACCESS", "UNEXPECTED_NETWORK_EGRESS", "WORKSPACE_ESCAPE", "TOKEN_EXFIL_RISK"], C.red);
  tx(s, "示例：任务声明为只读代码审查，但子进程访问 ~/.ssh 或向未知公网地址发送数据 → 触发高风险告警/阻断。", 0.84, 4.65, 8.25, 0.22, { size: 12.2, bold: true, align: "center", margin: 0 });
  source(s, "代码证据：backend/semantic_alerts.go；backend/event_envelope.go；backend/execution_graph.go。");
}

// 10 Demo
{
  const s = slide(); header(s, "08", "演示场景：三分钟证明“看得见、说得清、拦得住”", "建议现场按 3 个最硬核场景演示，避免只展示大屏。")
  const demos = [
    ["敏感路径", "Agent 尝试读取 .env / ssh key → SECRET_ACCESS + 任务归因", C.red],
    ["异常外联", "curl / python 子进程连接未知 IP:port → UNEXPECTED_NETWORK_EGRESS", C.blue],
    ["内核阻断", "BPF LSM 返回 EACCES，cgroup 拒绝 connect/sendmsg", C.green],
    ["图谱回放", "Agent Run → Tool Call → Process → File/Network → Policy", C.purple],
  ];
  demos.forEach((d, i) => { const x = 0.62 + (i % 2) * 4.55; const y = 1.36 + Math.floor(i / 2) * 1.48; bcard(s, x, y, 4.04, 1.05, d[0], [d[1]], d[2]); });
  card(s, 0.82, 4.58, 8.35, 0.42, { fill: C.amber, line: C.gold, shadow: false });
  tx(s, "答辩话术：我们不是只做可视化，而是在内核执行路径上获取证据并形成策略闭环。", 1.02, 4.72, 7.95, 0.12, { size: 11.2, bold: true, align: "center", margin: 0 });
}

// 11 Review mapping
{
  const s = slide(); header(s, "09", "与评审规则对齐：四个维度逐项回应", "高教主赛道创意组重点：个人成长30、项目创新30、产业价值25、团队协作15。")
  const dims = [["个人成长 30", "课程知识到 eBPF 原型；调研 Agent 安全真实问题；专创融合。", C.purple], ["项目创新 30", "BPF LSM、内核态阻断、进程树、语义-事实一致性。", C.red], ["产业价值 25", "AI coding 团队、高校实验室、企业安全审计强需求。", C.green], ["团队协作 15", "内核、后端、前端、算法、商业分工清晰。", C.gold]];
  dims.forEach((d, i) => { const y = 1.35 + i * 0.82; card(s, 0.78, y, 8.42, 0.60, { fill: i % 2 ? "FFFFFF" : "F1F5F9", line: d[2], shadow: false }); pill(s, 0.98, y + 0.16, 1.28, d[0], d[2]); tx(s, d[1], 2.52, y + 0.15, 6.28, 0.18, { size: 10.8, color: C.ink, margin: 0 }); });
  source(s, "规则依据：2025 高教主赛道创意组评审规则；2026 正式通知发布后需复核。")
}

// 12 Market
{
  const s = slide(); header(s, "10", "目标市场：AI Agent 落地越快，运行时安全越刚需", "从教育科研切入，扩展到 AI coding 团队和企业私有化部署。")
  bcard(s, 0.58, 1.28, 2.10, 2.70, "高校实验室", ["eBPF/AI安全教学", "课程实验与竞赛", "软著/论文/项目成果"], C.blue);
  bcard(s, 2.92, 1.28, 2.10, 2.70, "AI coding 团队", ["本地/云端 Agent 审计", "敏感文件与外联治理", "团队策略模板"], C.green);
  bcard(s, 5.26, 1.28, 2.10, 2.70, "企业安全", ["私有化部署", "审计报表与 SIEM", "合规留存"], C.red);
  bcard(s, 7.60, 1.28, 1.88, 2.70, "培训/靶场", ["CTF/攻防演练", "Agent 安全训练", "案例数据集"], C.gold);
  tx(s, "先低门槛开源获客，再用专业版/私有化/培训完成转化。", 0.92, 4.52, 8.0, 0.24, { size: 13, bold: true, align: "center", margin: 0 });
}

// 13 Business model
{
  const s = slide(); header(s, "11", "商业模式：开源核心 + 专业版 + 私有化 + 教育服务", "保留技术影响力，同时形成可收费的团队、企业与教学版本。")
  const rows = [["Community", "免费/开源", "单机观测、基础告警、教学实验"], ["Team Pro", "3.98万/年起", "团队策略、报表、训练样本、多用户"], ["Enterprise", "19.8万/年起", "私有化、多节点、SSO、审计留存、集成"], ["Education Kit", "3万/套起", "课程实验、靶场、竞赛训练营"], ["Consulting", "5-20万/项目", "PoC、红队 replay、策略调优"]];
  rows.forEach((r, i) => { const y = 1.22 + i * 0.65; card(s, 0.72, y, 8.60, 0.48, { fill: i % 2 ? "FFFFFF" : "F1F5F9", shadow: false }); pill(s, 0.92, y + 0.10, 1.25, r[0], [C.blue, C.green, C.red, C.gold, C.purple][i]); tx(s, r[1], 2.45, y + 0.13, 1.50, 0.16, { size: 10.5, bold: true, color: C.ink, margin: 0 }); tx(s, r[2], 4.15, y + 0.13, 4.80, 0.16, { size: 10.2, color: C.muted, margin: 0 }); });
  source(s, "注：价格为校赛版测算，提交前用真实客户访谈和报价校准。")
}

// 14 Financials
{
  const s = slide(); header(s, "12", "三年财务测算：从试点到专业版规模化", "测算逻辑：高校/实验室试点 → 团队专业版 → 企业私有化与咨询。")
  const data = [["第1年", "80万", "5个高校试点 + 3个企业PoC + 2套教育包", C.blue], ["第2年", "420万", "20个Team Pro + 8个Enterprise + 8个服务项目", C.green], ["第3年", "1600万", "60个Team Pro + 25个Enterprise + 20个项目", C.gold]];
  data.forEach((d, i) => { const x = 0.82 + i * 2.95; card(s, x, 1.48, 2.45, 2.35, { fill: "FFFFFF", line: d[3] }); tx(s, d[0], x + 0.12, 1.74, 2.20, 0.24, { size: 15, bold: true, color: d[3], align: "center", margin: 0 }); tx(s, d[1], x + 0.10, 2.20, 2.24, 0.45, { fontFace: "Arial", size: 28, bold: true, color: d[3], align: "center", margin: 0 }); tx(s, d[2], x + 0.25, 3.03, 1.95, 0.36, { size: 9.4, color: C.muted, align: "center", margin: 0 }); });
  card(s, 1.10, 4.42, 7.80, 0.44, { fill: C.rose, line: C.red, shadow: false });
  tx(s, "提交前必须用真实调研、试点意向或合同报价替换测算，避免被评委质疑数据来源。", 1.25, 4.56, 7.50, 0.12, { size: 10.8, bold: true, color: C.red, align: "center", margin: 0 });
}

// 15 Roadmap
{
  const s = slide(); header(s, "13", "实施路线：材料提交后继续把原型做成可验证产品", "围绕竞赛、试点、知识产权和商业化四条线推进。")
  const steps = [["2026.05-06", "校赛材料", "申报书/PPT/演示视频/稳定 demo"], ["2026.06-08", "场景验证", "良性/恶意 replay、检测率/阻断率报告"], ["2026.08-10", "试点应用", "课程/实验室/企业 PoC 反馈"], ["2026.10-2027.03", "成果固化", "软著/专利/论文/合作证明"], ["2027", "产品化", "专业版、多节点、企业私有化"]];
  steps.forEach((st, i) => { const x = 0.62 + i * 1.80; card(s, x, 1.55, 1.42, 2.48, { fill: i % 2 ? "FFFFFF" : "F1F5F9", line: [C.blue, C.green, C.gold, C.purple, C.red][i] }); tx(s, st[0], x + 0.08, 1.78, 1.25, 0.20, { size: 8.8, color: C.muted, align: "center", margin: 0 }); tx(s, st[1], x + 0.10, 2.18, 1.22, 0.25, { size: 12.8, bold: true, color: [C.blue, C.green, C.gold, C.purple, C.red][i], align: "center", margin: 0 }); tx(s, st[2], x + 0.12, 2.72, 1.18, 0.52, { size: 8.7, color: C.ink, align: "center", margin: 0 }); if (i < steps.length - 1) line(s, x + 1.44, 2.80, x + 1.75, 2.80, C.muted); });
}

// 16 Team
{
  const s = slide(); header(s, "14", "团队分工：硬技术、产品、算法与商业材料协同", "提交前将【待填】替换为真实姓名、学号、年级、学院与贡献证据。")
  const roles = [["负责人", "总体架构 / 答辩 / 进度", C.purple], ["eBPF", "LSM / cgroup / tracepoints", C.red], ["后端", "Go API / EventEnvelope / 策略", C.blue], ["前端", "Vue / 执行图谱 / 低代码", C.green], ["算法评测", "semantic_alerts / ML / replay", C.gold], ["商业调研", "客户访谈 / 财务 / 路演", C.cyan]];
  roles.forEach((r, i) => { const x = 0.58 + (i % 3) * 3.05; const y = 1.34 + Math.floor(i / 3) * 1.35; bcard(s, x, y, 2.68, 0.95, r[0], [`【待填】${r[1]}`], r[2]); });
  card(s, 0.90, 4.45, 8.15, 0.46, { fill: C.mint, line: C.green, shadow: false });
  tx(s, "团队证明建议：commit 记录、模块负责人截图、导师指导记录、测试报告、软著/论文/专利分工。", 1.08, 4.59, 7.80, 0.12, { size: 10.8, bold: true, color: C.green, align: "center", margin: 0 });
}

// 17 Risks
{
  const s = slide(); header(s, "15", "风险与合规：高权限工具必须讲清楚边界", "评委会关注安全工具自身是否安全，需主动说明默认关闭、鉴权和脱敏。")
  bcard(s, 0.62, 1.28, 2.78, 2.75, "权限风险", ["后端需要加载 eBPF，保持 root/特权边界", "写 API、shell、策略变更均有 token 与 runtime gate"], C.red);
  bcard(s, 3.62, 1.28, 2.78, 2.75, "隐私风险", ["TLS 明文捕获默认关闭", "事件流只保留摘要/长度/角色/厂商", "演示数据必须脱敏"], C.blue);
  bcard(s, 6.62, 1.28, 2.78, 2.75, "误报风险", ["先观察模式再阻断", "回放 benchmark 调优", "ML/LLM 建议需人工确认"], C.gold);
  tx(s, "合规表述：本项目用于本地/授权环境的 Agent 安全观测与控制，不采集未授权数据。", 0.92, 4.62, 8.1, 0.22, { size: 12.5, bold: true, align: "center", margin: 0 });
}

// 18 Ask & close
{
  const s = slide(C.ink);
  rect(s, 0.34, 0.32, 9.32, 0.08, C.gold, C.gold, { lineT: 100 });
  tx(s, "总结：让 AI Agent 可观测、可解释、可回放、可控制", 0.62, 1.10, 7.80, 0.42, { size: 27, bold: true, color: "FFFFFF", margin: 0 });
  tx(s, "我们用 eBPF 把系统事实拿到，用 hooks/wrapper 把 Agent 语义接上，\n再用 BPF LSM / cgroup / 执行图谱完成安全闭环。", 0.66, 1.92, 7.80, 0.62, { size: 16, color: "E5E7EB", margin: 0 });
  bcard(s, 0.70, 3.05, 2.45, 0.95, "需要支持", ["导师/学院资源、试点用户、软著/专利指导"], C.gold);
  bcard(s, 3.55, 3.05, 2.45, 0.95, "提交前", ["补真实团队信息、联系方式、调研数据与证明"], C.blue);
  bcard(s, 6.40, 3.05, 2.45, 0.95, "目标", ["校赛晋级、省赛打磨、形成可转化产品"], C.green);
  tx(s, "谢谢各位老师，请批评指正", 0.62, 4.78, 8.70, 0.30, { size: 19, bold: true, color: C.gold, align: "center", margin: 0 });
  badge(s);
}

pptx.writeFile({ fileName: OUT });
console.log(OUT);
