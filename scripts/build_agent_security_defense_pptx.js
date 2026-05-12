const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
const pptxgen = require('pptxgenjs');

const ROOT = path.resolve(__dirname, '..');
const OUT = path.join(ROOT, 'proposal-output', 'agent-security-opening-defense.pptx');
const OUT_CN = path.join(ROOT, 'proposal-output', 'Agent安全立项答辩.pptx');
const DOT_DIR = path.join(ROOT, 'proposal-output', 'graphviz', 'agent-security');
const GRAPH_DIR = path.join(ROOT, 'proposal-output', 'images', 'agent-security-graphviz');
const C = {
  ink: '0B1220', ink2: '111827', blue: '2563EB', gold: 'F59E0B', bg: 'F8FAFC',
  card: 'FFFFFF', muted: '64748B', line: 'CBD5E1', red: 'DC2626', green: '16A34A', purple: '7C3AED', sky: 'DBEAFE', amber: 'FEF3C7',
};
const GRAPHS = {
  threat: path.join(GRAPH_DIR, '01-threat-chain.png'),
  governance: path.join(GRAPH_DIR, '02-governance-loop.png'),
  position: path.join(GRAPH_DIR, '03-position-map.png'),
  architecture: path.join(GRAPH_DIR, '04-runtime-architecture.png'),
  roadmap: path.join(GRAPH_DIR, '05-implementation-roadmap.png'),
};
let slideNo = 0;

function ensureDir(file) { fs.mkdirSync(path.dirname(file), { recursive: true }); }
function text(slide, value, x, y, w, h, o = {}) {
  slide.addText(value, {
    x, y, w, h,
    fontFace: o.fontFace || 'Microsoft YaHei',
    fontSize: o.size ?? 12,
    color: o.color || C.ink,
    bold: !!o.bold,
    italic: !!o.italic,
    align: o.align || 'left',
    valign: o.valign || 'top',
    margin: o.margin ?? 0.03,
    fit: o.fit || 'shrink',
    breakLine: o.breakLine,
  });
}
function card(pptx, slide, x, y, w, h, o = {}) {
  slide.addShape(pptx.ShapeType.roundRect, {
    x, y, w, h, rectRadius: o.r ?? 0.12,
    fill: { color: o.fill || C.card, transparency: o.fillT ?? 0 },
    line: { color: o.line || C.line, width: o.lw ?? 1, transparency: o.lt ?? 0 },
    shadow: o.shadow === false ? undefined : { type: 'outer', color: '000000', opacity: 0.10, blur: 1, angle: 45, distance: 1 },
  });
}
function pill(pptx, slide, x, y, w, value, color = C.blue) {
  slide.addShape(pptx.ShapeType.roundRect, { x, y, w, h: 0.28, rectRadius: 0.14, fill: { color }, line: { color, transparency: 100 } });
  text(slide, value, x, y + 0.055, w, 0.12, { size: 8.5, color: 'FFFFFF', bold: true, align: 'center', margin: 0 });
}
function badge(pptx, slide, n) { pill(pptx, slide, 9.1, 5.13, 0.62, String(n).padStart(2, '0'), C.blue); }
function baseSlide(pptx, bg = C.bg) { const s = pptx.addSlide(); slideNo += 1; s.background = { color: bg }; return { s, n: slideNo }; }
function header(pptx, slide, n, sec, title, sub, dark = false) {
  text(slide, sec, 0.46, 0.32, 0.85, 0.2, { fontFace: 'Arial', size: 10, bold: true, color: dark ? C.gold : C.blue, margin: 0 });
  text(slide, title, 1.35, 0.22, 7.9, 0.34, { size: 22, bold: true, color: dark ? 'FFFFFF' : C.ink, margin: 0 });
  if (sub) text(slide, sub, 0.48, 0.68, 8.65, 0.24, { size: 10.5, color: dark ? 'CBD5E1' : C.muted, margin: 0 });
  slide.addShape(pptx.ShapeType.rect, { x: 0.48, y: 1.02, w: 9.02, h: 0.03, fill: { color: dark ? C.gold : C.blue }, line: { transparency: 100 } });
  if (n > 1) badge(pptx, slide, n);
}
function contentSlide(pptx, sec, title, sub) { const { s, n } = baseSlide(pptx); header(pptx, s, n, sec, title, sub); return s; }
function bulletText(slide, arr, x, y, w, h, size = 10.5, color = C.ink) { text(slide, arr.map(v => `• ${v}`).join('\n'), x, y, w, h, { size, color, margin: 0.02 }); }
function bulletCard(pptx, slide, x, y, w, h, title, arr, color = C.blue) {
  card(pptx, slide, x, y, w, h, { line: color });
  slide.addShape(pptx.ShapeType.rect, { x: x + 0.13, y: y + 0.14, w: 0.08, h: 0.32, fill: { color }, line: { transparency: 100 } });
  text(slide, title, x + 0.28, y + 0.12, w - 0.38, 0.24, { size: 13, bold: true, color, margin: 0 });
  bulletText(slide, arr, x + 0.18, y + 0.54, w - 0.32, h - 0.62, 10.2);
}
function source(slide, value) { text(slide, value, 0.52, 5.30, 8.35, 0.12, { size: 7.2, color: C.muted, margin: 0 }); }
function metric(pptx, slide, x, y, w, h, num, label, color = C.blue) {
  card(pptx, slide, x, y, w, h, { line: 'E2E8F0' });
  text(slide, num, x + 0.08, y + 0.12, w - 0.16, 0.28, { fontFace: 'Arial', size: 20, color, bold: true, align: 'center', margin: 0 });
  text(slide, label, x + 0.08, y + 0.48, w - 0.16, h - 0.52, { size: 8.6, color: C.muted, align: 'center', margin: 0 });
}
function addGraph(slide, file, x, y, w, h) { slide.addImage({ path: file, x, y, w, h, sizing: { type: 'contain', w, h }, altText: path.basename(file) }); }

function dotBase(body, extra = '') {
  return `digraph G {
    graph [bgcolor="transparent", pad="0.16", nodesep="0.50", ranksep="0.55", splines=ortho, ${extra}];
    node [shape=box, style="rounded,filled", fontname="Noto Sans CJK SC", fontsize=13, penwidth=2, margin="0.18,0.10", width=1.85, height=0.72, color="#CBD5E1", fillcolor="#FFFFFF", fontcolor="#0B1220"];
    edge [fontname="Noto Sans CJK SC", fontsize=9, color="#64748B", fontcolor="#475569", arrowsize=0.8, penwidth=2];
    ${body}
  }`;
}
function renderGraph(name, dot) {
  fs.mkdirSync(DOT_DIR, { recursive: true });
  fs.mkdirSync(GRAPH_DIR, { recursive: true });
  const dotFile = path.join(DOT_DIR, `${name}.dot`);
  const pngFile = path.join(GRAPH_DIR, `${name}.png`);
  const svgFile = path.join(GRAPH_DIR, `${name}.svg`);
  fs.writeFileSync(dotFile, dot);
  // Keep an SVG copy for editing, but insert PNG into PPTX because LibreOffice/PPT
  // can distort Graphviz SVG CJK text during conversion.
  execFileSync('dot', ['-Tsvg', dotFile, '-o', svgFile], { stdio: 'pipe' });
  execFileSync('dot', ['-Tpng', '-Gdpi=220', dotFile, '-o', pngFile], { stdio: 'pipe' });
  return pngFile;
}
function writeGraphvizCharts() {
  renderGraph('01-threat-chain', dotBase(`
    rankdir=TB;
    p [label="Prompt 注入\n网页 / Issue / README", color="#DC2626", fillcolor="#FEF2F2", fontcolor="#DC2626"];
    t [label="工具误用\nshell / MCP / 浏览器", color="#F59E0B", fillcolor="#FFFBEB", fontcolor="#B45309"];
    pr [label="进程漂移\n子进程 / 依赖脚本", color="#7C3AED", fillcolor="#F5F3FF", fontcolor="#7C3AED"];
    d [label="数据外泄\n密钥 / 网络 / TLS", color="#2563EB", fillcolor="#EFF6FF", fontcolor="#2563EB"];
    c [label="运行时安全平面\neBPF 事实 + hooks 语义 + wrapper 控制\n告警 / 阻断 / 回放 / 训练样本", color="#16A34A", fillcolor="#ECFDF5", fontcolor="#16A34A", width=2.9];
    {rank=same; p; t; pr;}
    {rank=same; d; c;}
    p -> t -> pr;
    pr -> d;
    d -> c [label="回落到控制闭环", color="#F59E0B", style=dashed];
  `));
  renderGraph('02-governance-loop', dotBase(`
    rankdir=TB;
    id [label="强身份\n非人身份 / 短期凭证", color="#2563EB", fillcolor="#EFF6FF", fontcolor="#2563EB"];
    perm [label="最小权限\n按任务授权 / 可撤销", color="#0EA5E9", fillcolor="#F0F9FF", fontcolor="#0369A1"];
    run [label="运行时治理\n观测 / 告警 / 阻断", color="#F59E0B", fillcolor="#FFFBEB", fontcolor="#B45309"];
    audit [label="审计复盘\n回放 / 证据 / 责任链", color="#7C3AED", fillcolor="#F5F3FF", fontcolor="#7C3AED"];
    eval [label="持续评估\n红队 / 基准 / 合规", color="#16A34A", fillcolor="#ECFDF5", fontcolor="#16A34A"];
    {rank=same; id; perm; run;}
    {rank=same; eval; audit;}
    id -> perm -> run -> audit -> eval -> id [label="反馈改进", color="#64748B"];
  `));
  renderGraph('03-position-map', dotBase(`
    rankdir=TB;
    subgraph cluster_high { label="高执行控制"; color="#CBD5E1"; style="rounded";
      log [label="普通日志 / 审计\n事后可看，难以及时干预", color="#94A3B8", fillcolor="#F8FAFC", fontcolor="#64748B"];
      ours [label="本项目定位\nOS 事实 + Agent 语义 + 策略控制\n黑匣子 + 刹车系统", color="#2563EB", fillcolor="#DBEAFE", fontcolor="#2563EB", width=2.65];
      {rank=same; log; ours;}
    }
    subgraph cluster_low { label="低执行控制"; color="#CBD5E1"; style="rounded";
      prompt [label="单纯 Prompt 防护\n依赖模型遵循，缺少运行时事实", color="#DC2626", fillcolor="#FEF2F2", fontcolor="#DC2626"];
      edr [label="传统 EDR / 沙箱\n强控制但不理解 Agent 任务语义", color="#F59E0B", fillcolor="#FFFBEB", fontcolor="#B45309"];
      {rank=same; prompt; edr;}
    }
    log -> prompt [style=invis]; ours -> edr [style=invis]; prompt -> edr [label="OS 事实更强", color="#CBD5E1"]; log -> ours [label="OS 事实更强", color="#CBD5E1"];
  `));
  renderGraph('04-runtime-architecture', dotBase(`
    rankdir=TB;
    agent [label="Agent / CLI\nClaude、Codex、Gemini\n脚本与子进程", color="#2563EB", fillcolor="#EFF6FF", fontcolor="#2563EB", width=2.35];
    sem [label="语义声明层\nhooks / wrapper / metadata\nrun、task、tool_call、intent", color="#7C3AED", fillcolor="#F5F3FF", fontcolor="#7C3AED", width=2.55];
    fact [label="内核事实层\neBPF tracepoints / uprobes\nfile、net、process、TLS", color="#2563EB", fillcolor="#DBEAFE", fontcolor="#2563EB", width=2.55];
    policy [label="关联与策略引擎\nsemantic mismatch\nALLOW / ALERT / BLOCK / REWRITE", color="#F59E0B", fillcolor="#FFFBEB", fontcolor="#B45309", width=2.75];
    view [label="展示与复盘\nDashboard / Execution Graph\nJSONL / OTLP / MCP", color="#16A34A", fillcolor="#ECFDF5", fontcolor="#16A34A", width=2.65];
    {rank=same; sem; fact;}
    agent -> sem; agent -> fact; sem -> policy; fact -> policy; policy -> view;
  `));
  renderGraph('05-implementation-roadmap', dotBase(`
    rankdir=TB;
    a [label="阶段一：基础观测\neBPF 事件模型\nPID 继承 / Dashboard", color="#2563EB", fillcolor="#EFF6FF", fontcolor="#2563EB", width=2.25];
    b [label="阶段二：控制闭环\nwrapper / hooks\nruntime gate / 告警解释", color="#F59E0B", fillcolor="#FFFBEB", fontcolor="#B45309", width=2.25];
    c [label="阶段三：图谱复盘\nExecution Graph\nJSONL 回放 / 导出", color="#7C3AED", fillcolor="#F5F3FF", fontcolor="#7C3AED", width=2.25];
    d [label="阶段四：评估优化\n良性/恶意工作流\n误报漏报与开销统计", color="#16A34A", fillcolor="#ECFDF5", fontcolor="#16A34A", width=2.25];
    {rank=same; a; b;}
    {rank=same; c; d;}
    a -> b -> c -> d;
    d -> a [label="数据回流", color="#F59E0B", style=dashed, constraint=false];
  `));
}

function sectionDivider(pptx, chapter, title, subtitle, tags = []) {
  const { s, n } = baseSlide(pptx, C.ink);
  s.addShape(pptx.ShapeType.rect, { x: 0.35, y: 0.34, w: 9.3, h: 0.07, fill: { color: C.gold }, line: { transparency: 100 } });
  text(s, `CHAPTER ${chapter}`, 0.62, 0.78, 2.3, 0.28, { fontFace: 'Arial', size: 14, color: C.gold, bold: true, margin: 0 });
  text(s, title, 0.62, 1.38, 6.7, 0.78, { size: 30, color: 'FFFFFF', bold: true, margin: 0 });
  text(s, subtitle, 0.66, 2.46, 5.7, 0.58, { size: 13, color: 'CBD5E1', margin: 0 });
  tags.forEach((tag, i) => pill(pptx, s, 0.68 + i * 1.32, 3.36, 1.15, tag, i % 2 ? C.blue : C.gold));
  card(pptx, s, 6.75, 1.22, 2.35, 2.75, { fill: '111827', line: C.gold, shadow: false });
  text(s, String(chapter).padStart(2, '0'), 7.14, 1.62, 1.55, 0.74, { fontFace: 'Arial', size: 46, color: C.gold, bold: true, align: 'center', margin: 0 });
  text(s, '多章节答辩结构', 7.02, 2.62, 1.85, 0.26, { size: 13, color: 'FFFFFF', bold: true, align: 'center', margin: 0 });
  badge(pptx, s, n);
}

function cover(pptx) {
  const { s } = baseSlide(pptx, C.ink);
  s.addShape(pptx.ShapeType.rect, { x: 0.35, y: 0.32, w: 9.3, h: 0.08, fill: { color: C.gold }, line: { transparency: 100 } });
  pill(pptx, s, 0.56, 0.62, 1.0, '立项答辩', C.gold); pill(pptx, s, 1.68, 0.62, 1.28, 'Agent 安全', C.blue); pill(pptx, s, 3.08, 0.62, 1.45, 'Graphviz 图解', C.purple);
  text(s, '面向 AI Agent 的\n运行时安全观测与控制平台', 0.62, 1.28, 5.0, 1.22, { size: 30, bold: true, color: 'FFFFFF', margin: 0 });
  text(s, '先说明网上 Agent 安全的现状与未来，再明确本项目定位：\n不是再造一个 Agent，而是为 Agent 执行链提供可验证、可回放、可管控的安全底座。', 0.65, 2.72, 5.1, 0.76, { size: 13, color: 'E5E7EB', margin: 0 });
  metric(pptx, s, 0.65, 4.05, 1.18, 0.78, 'eBPF', '内核事实层', C.gold); metric(pptx, s, 1.98, 4.05, 1.18, 0.78, 'Hooks', '语义声明层', C.blue); metric(pptx, s, 3.31, 4.05, 1.18, 0.78, 'Policy', '运行时控制层', C.gold);
  card(pptx, s, 6.15, 0.90, 3.05, 3.82, { fill: '111827', line: C.gold, shadow: false });
  text(s, '定位一句话', 6.45, 1.23, 2.45, 0.24, { size: 13, color: C.gold, bold: true, align: 'center', margin: 0 });
  text(s, '把 Agent 的「承诺」\n和操作系统里的「事实」\n对齐起来', 6.44, 1.78, 2.45, 1.16, { size: 25, color: 'FFFFFF', bold: true, align: 'center', margin: 0 });
  text(s, '2026 · 项目立项答辩多章节版', 0.62, 5.08, 3.0, 0.18, { size: 10, color: 'CBD5E1', margin: 0 });
}
function agenda(pptx) {
  const s = contentSlide(pptx, '00', '目录 / 多章节答辩结构', '用 4 个章节把“现状—定位—路线—实施”讲清楚，流程图由 Graphviz 生成并插入。');
  const items = [
    ['CH1', '现状与趋势', '为什么 Agent 安全已经从内容问题变成执行链问题'],
    ['CH2', '项目定位', '本项目做什么、不做什么，以及与现有方案的边界'],
    ['CH3', '技术路线', '事实层、语义层、控制层、展示层如何闭环'],
    ['CH4', '实施与验证', '阶段计划、评测方法、风险边界和预期成果'],
  ];
  items.forEach((it, i) => { const y = 1.35 + i * 0.82; card(pptx, s, 0.75, y, 8.45, 0.60, { fill: i % 2 ? 'FFFFFF' : 'F1F5F9', shadow: false }); pill(pptx, s, 0.96, y + 0.16, 0.78, it[0], i % 2 ? C.gold : C.blue); text(s, it[1], 1.96, y + 0.11, 1.45, 0.2, { size: 13.5, bold: true, margin: 0 }); text(s, it[2], 3.56, y + 0.13, 5.2, 0.23, { size: 10.5, color: C.muted, margin: 0 }); });
}
function current(pptx) {
  const s = contentSlide(pptx, '01', '网上 Agent 安全现状：风险从“回答错”变成“执行错”', 'Agent 拥有工具、文件、网络、浏览器和代码执行权限后，攻击面变成连续运行时链路。');
  bulletCard(pptx, s, 0.52, 1.28, 2.85, 2.95, '现状 1：自主性扩大', ['LLM 应用风险已包含 Excessive Agency：过度授权会带来可靠性、隐私与信任问题。', 'Agent 的“多步计划 + 工具调用”让单点过滤难以覆盖完整后果。'], C.red);
  bulletCard(pptx, s, 3.58, 1.28, 2.85, 2.95, '现状 2：攻击入口外移', ['提示注入可以藏在网页、Issue、README、依赖脚本和本地文件中。', '攻击者不一定要突破系统，只要诱导 Agent 合法地做错事。'], C.blue);
  bulletCard(pptx, s, 6.64, 1.28, 2.85, 2.95, '现状 3：可观测性不足', ['企业通常能看到最终文件变更，却难还原每个进程、网络端点和工具调用。', '缺少运行时事实会导致误报、漏报和事故复盘困难。'], C.gold);
  text(s, '答辩引导：这不是“模型安全”单点问题，而是 Agent 执行链安全问题。', 0.78, 4.62, 8.5, 0.26, { size: 13, bold: true, align: 'center', margin: 0 });
  source(s, 'Sources: OWASP Top 10 for LLM Applications / OWASP Agentic AI Threats and Mitigations.');
}
function threats(pptx) {
  const s = contentSlide(pptx, '02', '典型威胁链：Prompt → Tool → Process → Data', 'Graphviz 流程图：把网上讨论的 Agent 安全问题翻译成可答辩的工程攻击面。');
  addGraph(s, GRAPHS.threat, 0.72, 1.38, 8.55, 1.70);
  bulletCard(pptx, s, 0.78, 3.48, 2.62, 0.80, '攻击特点', ['跨层联动：自然语言、工具、进程、网络同时参与。'], C.red);
  bulletCard(pptx, s, 3.68, 3.48, 2.62, 0.80, '防御难点', ['只拦截易误伤，只看日志又太晚。'], C.blue);
  bulletCard(pptx, s, 6.58, 3.48, 2.62, 0.80, '项目切入口', ['eBPF 事实 + hooks 语义 + wrapper 控制。'], C.gold);
  card(pptx, s, 0.92, 4.76, 8.15, 0.36, { fill: C.ink, line: C.ink, shadow: false });
  text(s, '关键判断：Agent 攻击不是单点漏洞，而是“提示注入 → 工具误用 → 进程漂移 → 数据外泄”的执行链问题。', 1.08, 4.87, 7.82, 0.12, { size: 10.8, color: 'FFFFFF', bold: true, align: 'center', margin: 0 });
}
function future(pptx) {
  const s = contentSlide(pptx, '03', '未来趋势：Agent 安全会走向“可控自治”', 'Graphviz 闭环图：未来治理重点从提示词安全扩展为身份、权限、运行期治理、审计和持续评估。');
  addGraph(s, GRAPHS.governance, 0.82, 1.35, 8.35, 2.65);
  card(pptx, s, 0.82, 4.82, 8.36, 0.30, { fill: C.ink, line: C.ink, shadow: false });
  text(s, '结论：未来不是简单“禁用 Agent”，而是让 Agent 在受约束、可解释、可追责的环境里工作。', 1.0, 4.91, 8.0, 0.12, { size: 10.8, color: 'FFFFFF', bold: true, align: 'center', margin: 0 });
  source(s, 'Sources: Five Eyes “Careful Adoption of Agentic AI Services” (2026-05-01); NIST AI RMF / GenAI Profile.');
}
function position(pptx) {
  const s = contentSlide(pptx, '04', '项目定位：Agent 执行链安全的“黑匣子 + 刹车系统”', 'Graphviz 定位图：本项目不做大模型本体，也不做完整沙箱；聚焦事实采集、语义关联和策略控制。');
  addGraph(s, GRAPHS.position, 0.72, 1.14, 8.55, 3.72);
  text(s, '一句话定位：给开发者和安全团队一个低侵入、可复盘、能干预的 Agent 运行时安全平面。', 0.82, 5.02, 8.4, 0.18, { size: 12.3, bold: true, align: 'center', margin: 0 });
}
function goals(pptx) {
  const s = contentSlide(pptx, '05', '立项目标与研究内容', '把定位落成可验收的任务，而不是停留在“安全愿景”。');
  [['目标 1', '可观测', '构建 Agent 执行链事件模型：进程、文件、网络、TLS、工具调用、策略结果。'], ['目标 2', '可解释', '关联“Agent 声称的任务意图”和“系统实际行为”，识别语义不一致。'], ['目标 3', '可控制', '通过 wrapper / hooks / runtime gating 输出 ALLOW、ALERT、BLOCK、REWRITE。'], ['目标 4', '可复盘', '提供前端图谱、JSONL 回放、OTLP/MCP 导出与训练样本回流。']].forEach((r, i) => { const y = 1.32 + i * 0.82; card(pptx, s, 0.65, y, 8.7, 0.62, { fill: i % 2 ? 'FFFFFF' : 'F1F5F9', shadow: false }); pill(pptx, s, 0.86, y + 0.17, 0.82, r[0], i % 2 ? C.gold : C.blue); text(s, r[1], 1.95, y + 0.13, 1.0, 0.22, { size: 13.5, bold: true, margin: 0 }); text(s, r[2], 3.0, y + 0.13, 5.9, 0.25, { size: 10.8, color: C.muted, margin: 0 }); });
}
function arch(pptx) {
  const s = contentSlide(pptx, '06', '技术路线：事实层 + 语义层 + 控制层 + 展示层', 'Graphviz 架构图：以 Linux 运行时事实为底座，再接入 AI CLI 语义和策略执行。');
  addGraph(s, GRAPHS.architecture, 3.15, 1.16, 3.70, 3.52);
  bulletCard(pptx, s, 0.68, 1.62, 2.05, 1.02, '语义输入', ['hooks / wrapper 描述 Agent 自称要做什么。'], C.purple);
  bulletCard(pptx, s, 0.68, 3.08, 2.05, 1.02, '事实输入', ['eBPF 记录进程、文件、网络和 TLS 事实。'], C.blue);
  bulletCard(pptx, s, 7.18, 2.20, 2.05, 1.35, '输出能力', ['策略命中、图谱复盘、JSONL/OTLP/MCP 导出。'], C.green);
  card(pptx, s, 0.92, 4.74, 8.15, 0.40, { fill: 'FFFFFF', line: C.line, shadow: false });
  text(s, '已有基础：Go/eBPF/Vue/wrapper/hooks 原型｜核心创新：语义声明 vs OS 事实一致性判定｜答辩亮点：事件流、执行图谱、策略命中和回放', 1.05, 4.86, 7.9, 0.13, { size: 9.8, color: C.ink, bold: true, align: 'center', margin: 0 });
}
function features(pptx) {
  const s = contentSlide(pptx, '07', '系统原型能力：已经能支撑立项演示', '从“论文式设想”推进到可以运行、可以截图、可以现场讲解的工程原型。');
  [['9+', '核心 syscall tracepoints', C.blue], ['4+', 'AI CLI hook 适配', C.gold], ['UDS', 'wrapper 本地控制', C.purple], ['TLS', '明文片段捕获与脱敏', C.green], ['Graph', '执行拓扑回放', C.blue], ['OTLP', '可观测性导出', C.gold]].forEach((m, i) => metric(pptx, s, 0.62 + i * 1.43, 1.30, 1.28, 0.78, m[0], m[1], m[2]));
  bulletCard(pptx, s, 0.7, 2.52, 2.7, 1.72, '观测面', ['exec/open/connect/mkdir/unlink/ioctl/bind/send/recv 等事件。', 'TLS request/response 片段脱敏、截断后展示。'], C.blue);
  bulletCard(pptx, s, 3.65, 2.52, 2.7, 1.72, '控制面', ['wrapper 规则返回 ALLOW/BLOCK/ALERT/REWRITE。', '高危功能默认 runtime-gated。'], C.gold);
  bulletCard(pptx, s, 6.6, 2.52, 2.7, 1.72, '复盘面', ['Dashboard、Execution Graph、JSONL replay、MCP/OTLP 输出。', '可转成训练样本继续优化。'], C.green);
}
function innovation(pptx) {
  const s = contentSlide(pptx, '08', '创新点：不是“多加日志”，而是建立 Agent 安全语义闭环', '答辩中应强调差异化：低侵入观测、语义事实对齐、策略闭环、可复盘证据。');
  [['1. 双层证据', ['hook/wrapper 提供意图；eBPF 提供事实。', '两者不一致时形成语义告警。'], C.blue], ['2. 低侵入', ['无需修改 Agent 模型。', '通过 PID 注册、进程继承和系统调用观测接入。'], C.gold], ['3. 可干预', ['不是事后看日志；wrapper 能在命令执行前决策。', '策略变更可在 UI 中管理。'], C.purple], ['4. 可积累', ['事件可回放、导出和转训练样本。', '为后续 ML/LLM 风险评分提供数据基础。'], C.green]].forEach((b, i) => bulletCard(pptx, s, 0.58 + i * 2.26, 1.28, 2.05, 3.05, b[0], b[1], b[2]));
  text(s, '答辩表达：项目的贡献是把 Agent 安全从“提示词防护”推进到“运行时证据与控制平面”。', 0.78, 4.78, 8.5, 0.2, { size: 12.4, bold: true, align: 'center', margin: 0 });
}
function plan(pptx) {
  const s = contentSlide(pptx, '09', '实施计划与阶段成果', 'Graphviz 路线图：按立项答辩习惯给出明确路线，强调阶段递进和评估回流。');
  addGraph(s, GRAPHS.roadmap, 0.68, 1.35, 8.65, 2.93);
  text(s, '预期成果：原型系统、威胁模型与安全模型文档、典型 Agent 工作流评测、演示视频/截图、可投稿技术报告。', 0.82, 4.94, 8.35, 0.18, { size: 11.5, bold: true, align: 'center', margin: 0 });
}
function feasibility(pptx) {
  const s = contentSlide(pptx, '10', '可行性：仓库已有工程骨架，后续重点是收敛与验证', '说明为什么该项目不是空想：核心链路已经跑通，立项后可以集中打磨边界、评测和展示。');
  bulletCard(pptx, s, 0.65, 1.30, 2.72, 2.9, '工程基础', ['Go + eBPF 后端已经具备 syscall ringbuf、pinned maps、WebSocket/API。', 'Vue 3 前端已有 Dashboard、Network、Execution Graph、Config 等页面。'], C.blue);
  bulletCard(pptx, s, 3.64, 1.30, 2.72, 2.9, '安全基础', ['release-mode token、hook secret、UDS 0600、peer credential、危险功能 gate。', '已有 threat-model/security-model/policy-semantics 文档。'], C.gold);
  bulletCard(pptx, s, 6.63, 1.30, 2.72, 2.9, '验证基础', ['事件 JSONL、图谱回放、benchmark、ML/LLM 评分链路可作为评估材料。', '后续重点补齐代表性 Agent 场景集。'], C.green);
  text(s, '风险控制：诚实说明非目标——不防 root/恶意内核，不替代容器或企业 EDR；用清晰边界换取可落地。', 0.82, 4.72, 8.4, 0.22, { size: 11.8, bold: true, color: C.red, align: 'center', margin: 0 });
}
function close(pptx) {
  const { s, n } = baseSlide(pptx, C.ink); header(pptx, s, n, '11', '结论：项目定位清晰、问题真实、路线可落地', '从网上 Agent 安全趋势出发，落到本项目可实现的运行时安全底座。', true);
  text(s, '答辩收束三句话', 0.75, 1.40, 3.4, 0.28, { size: 18, color: 'FFFFFF', bold: true, margin: 0 });
  text(s, '1. Agent 安全的核心矛盾，是自主执行能力和可控性之间的矛盾。\n2. 本项目的定位，是补齐 Agent 执行链的事实观测、语义关联和策略控制。\n3. 现有原型已经覆盖关键链路，后续工作可以围绕评测、体验和硬化持续推进。', 0.78, 2.02, 4.45, 1.55, { size: 13, color: 'E5E7EB', margin: 0 });
  card(pptx, s, 5.65, 1.45, 3.55, 2.62, { fill: '111827', line: C.gold, shadow: false });
  text(s, '项目关键词', 5.98, 1.78, 2.9, 0.24, { size: 15, color: C.gold, bold: true, align: 'center', margin: 0 });
  ['运行时证据', '最小权限', '语义一致性', '可回放审计', '策略闭环'].forEach((v, i) => pill(pptx, s, 6.1 + (i % 2) * 1.42, 2.28 + Math.floor(i / 2) * 0.50, 1.22, v, i % 2 ? C.gold : C.blue));
  text(s, '谢谢！', 0.78, 4.84, 1.1, 0.24, { size: 18, color: 'FFFFFF', bold: true, margin: 0 });
  text(s, '主要来源：Five Eyes Careful Adoption of Agentic AI Services (2026-05-01); OWASP Agentic AI Threats and Mitigations (2025); OWASP Top 10 for LLM Applications; NIST AI RMF / GenAI Profile。', 0.78, 5.24, 8.6, 0.13, { size: 7.8, color: 'CBD5E1', margin: 0 });
}

async function main() {
  writeGraphvizCharts();
  const pptx = new pptxgen();
  pptx.layout = 'LAYOUT_16x9';
  pptx.author = 'Codex';
  pptx.company = 'Agent eBPF Filter';
  pptx.subject = 'Agent 安全立项答辩';
  pptx.title = '面向 AI Agent 的运行时安全观测与控制平台';
  pptx.lang = 'zh-CN';
  pptx.theme = { headFontFace: 'Microsoft YaHei', bodyFontFace: 'Microsoft YaHei', lang: 'zh-CN' };
  cover(pptx);
  agenda(pptx);
  sectionDivider(pptx, 1, '现状与趋势', '先回答为什么现在需要做 Agent 运行时安全。', ['现状', '威胁链', '未来治理']);
  current(pptx); threats(pptx); future(pptx);
  sectionDivider(pptx, 2, '项目定位', '再回答本项目做什么、不做什么、和别人有什么区别。', ['定位', '边界', '目标']);
  position(pptx); goals(pptx);
  sectionDivider(pptx, 3, '技术路线', '把定位落到系统结构：事实层、语义层、控制层、展示层。', ['Graphviz', 'eBPF', '策略闭环']);
  arch(pptx); features(pptx); innovation(pptx);
  sectionDivider(pptx, 4, '实施与验证', '最后说明阶段计划、可行性、风险边界和预期成果。', ['计划', '验证', '成果']);
  plan(pptx); feasibility(pptx); close(pptx);
  ensureDir(OUT);
  await pptx.writeFile({ fileName: OUT });
  fs.copyFileSync(OUT, OUT_CN);
  console.log(`Wrote ${OUT}`);
  console.log(`Copied ${OUT_CN}`);
  console.log(`Rendered Graphviz charts to ${GRAPH_DIR}`);
}
main().catch(err => { console.error(err); process.exit(1); });
