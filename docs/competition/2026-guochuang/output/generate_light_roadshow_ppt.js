const pptxgen = require("pptxgenjs");
const imageSize = require("image-size");
const fs = require("fs");
const path = require("path");

const OUT_DIR = path.join(__dirname, "final");
const ASSET_DIR = path.join(__dirname, "ppt_assets_light");
const OUT = path.join(OUT_DIR, "Agent-eBPF-Filter-项目答辩路演PPT-浅色优化版.pptx");
fs.mkdirSync(OUT_DIR, { recursive: true });

const pptx = new pptxgen();
pptx.author = "Agent eBPF Filter";
pptx.company = "Agent eBPF Filter";
pptx.subject = "国创赛 2026 高教主赛道创意组路演";
pptx.title = "Agent eBPF Filter 项目答辩路演 PPT 浅色模板版";
pptx.lang = "zh-CN";
pptx.defineLayout({ name: "LIGHT_16_9", width: 10, height: 5.625 });
pptx.layout = "LIGHT_16_9";
pptx.theme = {
  headFontFace: "Microsoft YaHei",
  bodyFontFace: "Microsoft YaHei",
  lang: "zh-CN"
};
pptx.margin = 0;
pptx.keywords = "AI Agent,eBPF,BPF LSM,cgroup,Runtime Security,Observability,Guochuang";
pptx.presentationLayout = "wide";

const W = 10;
const H = 5.625;
const CN = "Microsoft YaHei";
const EN = "Arial";
const C = {
  bg: "FBFDFF",
  panel: "F3F8FC",
  panel2: "EDF5FB",
  white: "FFFFFF",
  ink: "102A43",
  text: "243B53",
  muted: "6B7C8E",
  lightText: "8293A3",
  line: "D9E6EF",
  line2: "E7EEF4",
  blue: "1677B8",
  blueDark: "0B4F78",
  cyan: "0A9FB5",
  green: "23865F",
  orange: "C66A1D",
  red: "C74343",
  purple: "5B55A3",
  paleBlue: "EAF5FC",
  paleGreen: "EAF7F1",
  paleOrange: "FFF3E6",
  paleRed: "FDEEEE",
  palePurple: "F1F0FA",
  paleCyan: "E9F8FA"
};
const A = [C.blue, C.green, C.orange, C.red, C.purple, C.cyan];
const P = [C.paleBlue, C.paleGreen, C.paleOrange, C.paleRed, C.palePurple, C.paleCyan];

const img = (name) => path.join(ASSET_DIR, name);

function addText(slide, text, x, y, w, h, opts = {}) {
  slide.addText(text, {
    x, y, w, h,
    fontFace: opts.fontFace || CN,
    fontSize: opts.fontSize || 12,
    color: opts.color || C.text,
    bold: opts.bold || false,
    italic: opts.italic || false,
    align: opts.align || "left",
    valign: opts.valign || "top",
    margin: opts.margin ?? 0.01,
    fit: opts.fit || "shrink",
    breakLine: opts.breakLine || false,
    paraSpaceAfterPt: opts.paraSpaceAfterPt || 0,
    paraSpaceBeforePt: opts.paraSpaceBeforePt || 0,
    lineSpacingMultiple: opts.lineSpacingMultiple || 0.92,
    transparency: opts.transparency || 0,
    rotate: opts.rotate || 0,
  });
}

function rect(slide, x, y, w, h, fill = C.white, line = C.line, opts = {}) {
  slide.addShape(opts.shape || pptx.ShapeType.roundRect, {
    x, y, w, h,
    rectRadius: opts.radius ?? 0.04,
    fill: { color: fill, transparency: opts.transparency || 0 },
    line: { color: line, width: opts.lineWidth ?? 0.8, transparency: opts.lineTransparency ?? 0 },
    shadow: opts.shadow ? { type: "outer", color: "BED0DD", opacity: 0.08, blur: 1, angle: 45, distance: 0.8 } : undefined,
  });
}

function line(slide, x1, y1, x2, y2, color = C.line, width = 1) {
  slide.addShape(pptx.ShapeType.line, { x: x1, y: y1, w: x2 - x1, h: y2 - y1, line: { color, width } });
}

function pageBadge(slide, index) {
  rect(slide, 9.25, 5.14, 0.48, 0.27, C.white, C.line2, { radius: 0.04 });
  addText(slide, String(index).padStart(2, "0"), 9.25, 5.205, 0.48, 0.10, { fontFace: EN, fontSize: 7.5, bold: true, color: C.blue, align: "center" });
}

function source(slide, text) {
  line(slide, 0.56, 5.06, 8.95, 5.06, C.line2, 0.8);
  addText(slide, text, 0.58, 5.16, 8.25, 0.16, { fontSize: 6.8, color: C.lightText });
}

function base(slide, index) {
  slide.background = { color: C.bg };
  slide.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: W, h: H, fill: { color: C.bg }, line: { color: C.bg } });
  slide.addShape(pptx.ShapeType.rect, { x: 0, y: 0, w: W, h: 0.08, fill: { color: C.blue }, line: { color: C.blue } });
  slide.addShape(pptx.ShapeType.rect, { x: 8.35, y: 0, w: 1.65, h: 0.08, fill: { color: C.green }, line: { color: C.green } });
  if (index > 1) pageBadge(slide, index);
}

function title(slide, section, titleText, subtitle, index) {
  base(slide, index);
  addText(slide, `SECTION ${section}`, 0.58, 0.36, 1.25, 0.18, { fontFace: EN, fontSize: 7.2, color: C.blue, bold: true });
  line(slide, 0.58, 0.65, 0.58, 1.03, C.blue, 2.2);
  addText(slide, titleText, 0.74, 0.58, 8.5, 0.42, { fontSize: 20.5, color: C.ink, bold: true });
  if (subtitle) addText(slide, subtitle, 0.75, 1.05, 8.3, 0.22, { fontSize: 9.2, color: C.muted });
}

function pill(slide, text, x, y, w, color = C.blue, fill = C.paleBlue) {
  rect(slide, x, y, w, 0.28, fill, color, { radius: 0.08, lineTransparency: 55 });
  addText(slide, text, x + 0.06, y + 0.075, w - 0.12, 0.10, { fontSize: 7.5, color, bold: true, align: "center" });
}

function label(slide, text, x, y, color = C.blue) {
  slide.addShape(pptx.ShapeType.rect, { x, y: y + 0.03, w: 0.06, h: 0.24, fill: { color }, line: { color } });
  addText(slide, text, x + 0.13, y, 2.5, 0.22, { fontSize: 10.5, color: C.ink, bold: true });
}

function containImage(slide, file, x, y, w, h, opts = {}) {
  if (!fs.existsSync(file)) {
    rect(slide, x, y, w, h, C.paleRed, "F2B6B6", { radius: 0.03 });
    addText(slide, `缺少图片：${path.basename(file)}`, x + 0.12, y + h / 2 - 0.08, w - 0.24, 0.16, { fontSize: 7.2, color: C.red, align: "center" });
    return;
  }
  const dim = imageSize(file);
  let iw = w;
  let ih = w * dim.height / dim.width;
  if (ih > h) {
    ih = h;
    iw = h * dim.width / dim.height;
  }
  if (opts.frame) rect(slide, x, y, w, h, opts.bg || C.white, opts.line || C.line, { radius: opts.radius ?? 0.03 });
  slide.addImage({ path: file, x: x + (w - iw) / 2, y: y + (h - ih) / 2, w: iw, h: ih });
}

function metric(slide, value, labelText, x, y, w, color = C.blue) {
  rect(slide, x, y, w, 0.72, C.white, C.line, { radius: 0.035 });
  addText(slide, value, x + 0.13, y + 0.10, w - 0.26, 0.25, { fontFace: EN, fontSize: 17.5, color, bold: true });
  addText(slide, labelText, x + 0.13, y + 0.42, w - 0.26, 0.18, { fontSize: 6.9, color: C.text, fit: "shrink" });
}

function card(slide, x, y, w, h, heading, body, color = C.blue, fill = C.white) {
  rect(slide, x, y, w, h, fill, C.line, { radius: 0.035 });
  slide.addShape(pptx.ShapeType.rect, { x, y, w: 0.05, h, fill: { color }, line: { color } });
  addText(slide, heading, x + 0.18, y + 0.16, w - 0.32, 0.22, { fontSize: 11.5, color: C.ink, bold: true });
  addText(slide, body, x + 0.18, y + 0.50, w - 0.34, h - 0.60, { fontSize: 8.0, color: C.text, fit: "shrink" });
}

function codeBox(slide, text, x, y, w, h) {
  rect(slide, x, y, w, h, "F6F9FC", "DCE8F0", { radius: 0.02 });
  addText(slide, text, x + 0.11, y + 0.11, w - 0.22, h - 0.22, { fontFace: EN, fontSize: 6.8, color: "38566A", fit: "shrink" });
}

function arrow(slide, x1, y1, x2, y2, color = C.muted) {
  slide.addShape(pptx.ShapeType.line, { x: x1, y: y1, w: x2 - x1, h: y2 - y1, line: { color, width: 1.4, beginArrowType: "none", endArrowType: "triangle" } });
}

{
  const slide = pptx.addSlide();
  base(slide, 1);
  slide.addShape(pptx.ShapeType.rect, { x: 6.75, y: 0.08, w: 3.25, h: 5.545, fill: { color: C.panel }, line: { color: C.panel } });
  addText(slide, "国创赛2026 · 高教主赛道 · 创意组", 0.64, 0.56, 3.9, 0.22, { fontSize: 9.6, color: C.blue, bold: true });
  line(slide, 0.64, 1.04, 4.1, 1.04, C.blue, 2.1);
  addText(slide, "Agent eBPF Filter", 0.62, 1.33, 5.85, 0.52, { fontFace: EN, fontSize: 34, color: C.ink, bold: true });
  addText(slide, "面向 AI Agent 的\n内核级运行时安全与可观测平台", 0.64, 2.02, 5.65, 0.88, { fontSize: 23, color: C.ink, bold: true });
  addText(slide, "让 Agent 的“承诺”和操作系统里的“事实”对齐起来", 0.66, 3.10, 5.7, 0.24, { fontSize: 11.4, color: C.text });
  pill(slide, "eBPF 内核事实", 0.66, 3.68, 1.35, C.blue, C.paleBlue);
  pill(slide, "BPF LSM 同步阻断", 2.15, 3.68, 1.55, C.green, C.paleGreen);
  pill(slide, "Execution Graph 执行归因", 3.85, 3.68, 1.85, C.purple, C.palePurple);
  addText(slide, "负责人/学院/联系方式：\n【待填】\n推荐学院：【待填】\n负责人：【待填】\n手机号/QQ：【待填】", 0.66, 4.55, 4.05, 0.52, { fontSize: 7.9, color: C.muted });
  addText(slide, "项目答辩 / 路演版 · 2026-05-26", 4.65, 5.03, 1.7, 0.16, { fontSize: 7.5, color: C.lightText, align: "right" });
  rect(slide, 7.18, 0.70, 2.32, 1.12, C.white, C.line, { radius: 0.04 });
  addText(slide, "一句话定位", 7.38, 0.93, 1.9, 0.20, { fontSize: 12.5, color: C.ink, bold: true, align: "center" });
  addText(slide, "Agent 运行时证据链与策略边界", 7.34, 1.27, 1.98, 0.16, { fontSize: 8.2, color: C.muted, align: "center" });
  metric(slide, "84%", "AI 开发工具采用意愿", 7.18, 2.15, 2.32, C.blue);
  metric(slide, "28.65M", "public GitHub 新增硬编码密钥", 7.18, 3.08, 2.32, C.red);
  metric(slide, "OS", "内核级事实采集与阻断", 7.18, 4.01, 2.32, C.green);
}

{
  const slide = pptx.addSlide();
  title(slide, "00", "路演结构：从真实痛点到代码证据，再到商业落地", "按高校项目路演常用逻辑组织：背景痛点、解决方案、创新性、可行性、落地价值。", 2);
  const items = [
    ["01", "调研痛点", "AI Agent 高权限执行链进入真实开发流程，安全边界仍依赖事后日志。"],
    ["02", "案例证据", "密钥外泄、MCP 配置泄露、Copilot 信息泄露证明风险已经发生。"],
    ["03", "技术方案", "eBPF 事实层 + hooks 语义层 + LSM/cgroup 阻断 + 图谱回放。"],
    ["04", "工程进展", "Go 后端、eBPF 程序、Vue 前端、wrapper、适配器均已有实现。"],
    ["05", "商业落地", "从高校实验室与 AI coding 团队切入，扩展企业私有化与教育服务。"],
  ];
  line(slide, 1.05, 1.58, 1.05, 4.42, C.line, 1.2);
  items.forEach((it, i) => {
    const y = 1.35 + i * 0.66;
    slide.addShape(pptx.ShapeType.ellipse, { x: 0.86, y: y + 0.02, w: 0.38, h: 0.38, fill: { color: i === 0 ? C.blue : C.white }, line: { color: C.blue, width: 1.2 } });
    addText(slide, it[0], 0.86, y + 0.15, 0.38, 0.09, { fontFace: EN, fontSize: 6.2, color: i === 0 ? C.white : C.blue, bold: true, align: "center" });
    addText(slide, it[1], 1.46, y, 1.38, 0.22, { fontSize: 12.2, color: C.ink, bold: true });
    addText(slide, it[2], 2.95, y + 0.02, 5.95, 0.20, { fontSize: 8.4, color: C.text });
  });
  rect(slide, 0.82, 4.75, 8.55, 0.38, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "展演节奏建议：前 2 分钟交代外部证据，中间 5 分钟集中讲原型和演示，最后 2 分钟说明落地路径与资源诉求。", 1.02, 4.88, 8.15, 0.10, { fontSize: 8.2, color: C.text, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "01", "数据证据：AI Agent 不是未来风险，而是当前安全缺口", "采用公开调研与安全报告，说明市场使用率、泄露规模、治理缺口和经济成本。", 3);
  metric(slide, "84%", "Stack Overflow：正在使用或计划使用 AI 开发工具", 0.62, 1.38, 1.66, C.blue);
  metric(slide, "28.65M", "GitGuardian：2025 年 public GitHub 新增硬编码密钥", 2.42, 1.38, 1.82, C.red);
  metric(slide, "$4.44M", "IBM：全球平均数据泄露成本", 4.38, 1.38, 1.52, C.purple);
  metric(slide, "97%", "IBM：AI 相关事件组织缺少适当 AI 访问控制", 6.04, 1.38, 1.54, C.orange);
  metric(slide, "24,008", "GitGuardian：公开 MCP 配置中暴露的唯一密钥", 7.72, 1.38, 1.66, C.green);
  rect(slide, 0.62, 2.42, 4.32, 2.28, C.white, C.line, { radius: 0.035 });
  containImage(slide, img("stackoverflow_ai_usage.png"), 0.84, 2.62, 3.90, 1.56, { frame: true, bg: C.white });
  addText(slide, "采用率提升叠加信任下降：Agent 一旦接入终端和文件系统，必须用运行时事实建立审计边界。", 0.86, 4.30, 3.85, 0.16, { fontSize: 7.2, color: C.muted });
  rect(slide, 5.16, 2.42, 4.22, 2.28, C.white, C.line, { radius: 0.035 });
  containImage(slide, img("gitguardian_commits_developers.png"), 5.38, 2.62, 3.80, 1.56, { frame: true, bg: C.white });
  addText(slide, "软件生产提速：public commits 与开发者规模同步增长，密钥与配置治理需要跟上生成速度。", 5.40, 4.30, 3.72, 0.16, { fontSize: 7.2, color: C.muted });
  source(slide, "数据/图片：Stack Overflow Developer Survey 2025；GitGuardian State of Secrets Sprawl 2026；IBM Cost of a Data Breach Report 2025");
}

{
  const slide = pptx.addSlide();
  title(slide, "02", "真实案例：工具误用、密钥外泄、Copilot 数据外泄已形成样本", "用报告截图替代概念化图标，降低生成感，同时让评委能追溯外部证据。", 4);
  const cases = [
    { x: 0.62, img: "csa_copilot_cover_thumb.png", t: "M365 Copilot 信息泄露", d: "CSA 研究梳理 12 个月内多起 Copilot 相关披露；EchoLeak CVSS 9.3，体现上下文边界问题。", c: C.red },
    { x: 3.70, img: "gitguardian_mcp_valid_secrets.png", t: "MCP 配置密钥泄露", d: "GitGuardian 发现 24,008 个 MCP 相关唯一密钥，其中 2,117 个仍有效；配置本身成为攻击面。", c: C.green },
    { x: 6.78, img: "owasp_llm_cover_thumb.png", t: "Agentic Tool Misuse", d: "OWASP 将工具误用、权限滥用、不可追踪列为核心风险，强调日志、沙箱和实时边界。", c: C.blue },
  ];
  cases.forEach((c) => {
    rect(slide, c.x, 1.38, 2.60, 3.40, C.white, C.line, { radius: 0.035 });
    containImage(slide, img(c.img), c.x + 0.18, 1.58, 2.24, 1.45, { frame: true, bg: "F8FAFC", line: C.line2 });
    addText(slide, c.t, c.x + 0.18, 3.28, 2.24, 0.24, { fontSize: 11.3, color: c.c, bold: true });
    addText(slide, c.d, c.x + 0.18, 3.68, 2.22, 0.60, { fontSize: 7.4, color: C.text, fit: "shrink" });
    pill(slide, "需要 runtime 证据链", c.x + 0.18, 4.40, 1.35, c.c, c.c === C.red ? C.paleRed : c.c === C.green ? C.paleGreen : C.paleBlue);
  });
  source(slide, "案例图片：Cloud Security Alliance 2026 Copilot research note；GitGuardian 2026 report；OWASP Top 10 for LLM Applications 2025 / Agentic AI Threats");
}

{
  const slide = pptx.addSlide();
  title(slide, "03", "问题：Agent 安全已从“回答错”变成“三个不可见”", "编码 Agent 会启动终端、改文件、装依赖、访问网络，安全边界必须落到运行时。", 5);
  card(slide, 0.74, 1.42, 2.62, 2.45, "意图不可见", "• EDR 看得到进程，却不知道哪个 run/task/tool_call 触发\n• Prompt 网关看得到文本，看不到 OS 行为\n• 审计口径无法对齐“承诺”与“事实”", C.blue, C.white);
  card(slide, 3.68, 1.42, 2.62, 2.45, "链路不可见", "• 危险动作常发生在 shell、python、node、git、npm、curl 子进程\n• MCP、skills、依赖脚本扩大本机攻击面\n• 父子进程上下文容易丢失", C.orange, C.white);
  card(slide, 6.62, 1.42, 2.62, 2.45, "阻断不可见", "• 事后日志难证明动作是否已被内核拒绝\n• UI 标红不等于 OS 层真实拒绝\n• 安全团队需要可回放证据链", C.red, C.white);
  rect(slide, 0.92, 4.32, 8.12, 0.48, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "结论：评委真正需要看到的不是“再加一个大屏”，而是把 Agent 语义与操作系统事实合成可验证、可阻断、可复盘的证据链。", 1.12, 4.48, 7.72, 0.12, { fontSize: 8.8, color: C.ink, bold: true, align: "center" });
  source(slide, "依据：OWASP LLM/Agentic AI 风险框架；GitGuardian 2026 secrets sprawl；IBM 2025 AI governance findings");
}

{
  const slide = pptx.addSlide();
  title(slide, "04", "典型风险链：Prompt 注入 → 工具误用 → 子进程漂移 → 数据外泄", "项目从执行链而不是单条日志看待 Agent 风险。", 6);
  const steps = [
    ["Prompt 注入", "网页 / Issue / README", C.purple],
    ["工具误用", "shell / MCP / browser", C.orange],
    ["子进程漂移", "python / node / curl", C.blue],
    ["数据外泄", "secret / network / TLS", C.red],
    ["安全闭环", "告警 / 阻断 / 回放", C.green],
  ];
  steps.forEach((s, i) => {
    const x = 0.62 + i * 1.84;
    rect(slide, x, 1.58, 1.42, 0.82, C.white, s[2], { radius: 0.03, lineTransparency: 20 });
    addText(slide, s[0], x + 0.10, 1.78, 1.22, 0.16, { fontSize: 10.0, color: s[2], bold: true, align: "center" });
    addText(slide, s[1], x + 0.10, 2.12, 1.22, 0.10, { fontSize: 6.8, color: C.text, align: "center" });
    if (i < steps.length - 1) arrow(slide, x + 1.47, 1.99, x + 1.78, 1.99, C.lightText);
  });
  const rows = [
    ["观测对象", "进程、文件、网络、策略、工具调用", C.blue],
    ["判断依据", "意图是否与事实行为一致", C.purple],
    ["处置动作", "ALLOW / ALERT / BLOCK / REWRITE", C.green],
  ];
  rows.forEach((r, i) => card(slide, 0.82 + i * 3.05, 3.12, 2.56, 1.05, r[0], r[1], r[2], C.white));
  addText(slide, "与案例的对应关系：EchoLeak 证明“外部输入可驱动内部数据泄露”；MCP 密钥证明“配置即攻击面”；GitGuardian 数据证明“AI 辅助编码让密钥泄露速度提升”。", 0.88, 4.58, 8.2, 0.22, { fontSize: 8.1, color: C.text, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "05", "解决方案：看得见、说得清、拦得住、可复盘", "以 eBPF 为事实底座，用 hooks/wrapper 补齐 Agent 语义，再进入图谱和策略闭环。", 7);
  rect(slide, 0.74, 1.35, 8.52, 3.30, C.white, C.line, { radius: 0.035 });
  const layers = [
    ["Agent / CLI", "Codex、Claude、Gemini、Copilot、脚本与子进程", C.purple],
    ["语义层", "wrapper / native hooks / adapters：run、task、tool_call、intent", C.blue],
    ["事实层", "eBPF tracepoints / cgroup / BPF LSM：exec、file、net、fork", C.green],
    ["策略层", "BPF LSM、cgroup、wrapper：ALLOW / ALERT / BLOCK / REWRITE", C.red],
    ["证据层", "EventEnvelope、semantic_alerts、Execution Graph、JSONL、OTLP", C.orange],
  ];
  layers.forEach((l, i) => {
    const y = 1.62 + i * 0.50;
    rect(slide, 1.12, y, 7.75, 0.34, i % 2 ? C.panel : C.white, C.line2, { radius: 0.02 });
    slide.addShape(pptx.ShapeType.rect, { x: 1.12, y, w: 0.06, h: 0.34, fill: { color: l[2] }, line: { color: l[2] } });
    addText(slide, l[0], 1.34, y + 0.095, 1.25, 0.10, { fontSize: 8.4, color: l[2], bold: true });
    addText(slide, l[1], 2.72, y + 0.095, 5.90, 0.10, { fontSize: 7.8, color: C.text });
  });
  rect(slide, 1.12, 4.20, 7.75, 0.28, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "核心差异：不是替代模型安全，而是在模型调用工具后，用操作系统事实证明它实际做了什么，并在危险动作发生前拦截。", 1.30, 4.285, 7.40, 0.08, { fontSize: 7.7, bold: true, color: C.ink, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "06", "当前原型：已不是概念图，而是可运行工程", "仓库包含 Go 后端、eBPF 程序、Vue 前端、wrapper、适配器与部署脚本。", 8);
  const modules = [
    ["Go 后端", "特权 runtime / HTTP+WS API / hooks / policy engine", "backend/main.go\nbackend/event_envelope.go\nbackend/semantic_alerts.go\nbackend/execution_graph.go", C.blue],
    ["eBPF 内核", "tracepoints / BPF LSM / cgroup 网络阻断", "backend/ebpf/agent_tracker.c\nbackend/ebpf/lsm_enforcer.c\nbackend/ebpf/cgroup_sandbox.c", C.green],
    ["Vue 前端", "Dashboard / Execution Graph / Config / Plugins", "frontend/src/views/Dashboard.vue\nfrontend/src/views/ExecutionGraph.vue\nfrontend/src/views/Config.vue\nfrontend/src/views/Plugins.vue", C.purple],
    ["部署与验证", "systemd / rc.local / K8s / smoke tests", "Makefile install/uninstall\nscripts/os-enforcement-smoke.sh\ndocs/kubernetes.md\ndeploy/kubernetes/*.yaml", C.orange],
  ];
  modules.forEach((m, i) => {
    const x = 0.70 + (i % 2) * 4.46;
    const y = 1.38 + Math.floor(i / 2) * 1.62;
    card(slide, x, y, 3.86, 1.24, m[0], m[1], m[3], C.white);
    codeBox(slide, m[2], x + 2.05, y + 0.18, 1.55, 0.86);
  });
  rect(slide, 1.06, 4.76, 7.90, 0.26, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "答辩要点：每个卖点都能指到代码文件、演示页面和验证脚本，避免只停留在概念图。", 1.26, 4.845, 7.50, 0.08, { fontSize: 7.8, color: C.text, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "07", "创新一：BPF LSM + cgroup 形成确定性内核阻断", "不是只做事后日志，而是在 exec/open/read/write/mmap/rename/connect 等路径前置决策。", 9);
  card(slide, 0.70, 1.36, 2.72, 2.48, "BPF LSM", "• bprm_check_security：执行前检查\n• file_open / file_permission：打开与读写\n• inode_*：创建、删除、重命名、链接\n• 命中策略返回 -EACCES", C.green, C.white);
  card(slide, 3.66, 1.36, 2.72, 2.48, "cgroup 网络", "• connect4/connect6\n• sendmsg4/sendmsg6\n• 按 cgroup、IP、IPv6、端口阻断\n• 覆盖 TCP/UDP 外联场景", C.blue, C.white);
  card(slide, 6.62, 1.36, 2.72, 2.48, "策略原则", "• 确定性 map lookup\n• ML/LLM 只建议，不直接内核阻断\n• 策略通过认证 API 修改\n• 阻断证据进入执行图谱", C.red, C.white);
  rect(slide, 0.92, 4.28, 8.14, 0.42, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "评委可感知亮点：真实 OS 级拒绝，而不是 UI 上标红。代码证据：backend/ebpf/lsm_enforcer.c；backend/ebpf/cgroup_sandbox.c；scripts/os-enforcement-smoke.sh。", 1.08, 4.42, 7.82, 0.10, { fontSize: 7.6, color: C.text, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "08", "创新二：进程树追踪让 Agent 子进程不再“失踪”", "真实 Agent 行为经常发生在 /bin/sh、python、node、git、npm、curl 等子进程。", 10);
  const nodes = [
    ["Agent", 0.82, C.purple],
    ["Tool Call", 2.28, C.blue],
    ["shell", 3.86, C.orange],
    ["python/node", 5.30, C.orange],
    ["file/net", 6.96, C.red],
    ["policy", 8.22, C.green],
  ];
  nodes.forEach((n, i) => {
    rect(slide, n[1], 1.66, 1.05, 0.48, C.white, n[2], { radius: 0.03 });
    addText(slide, n[0], n[1] + 0.06, 1.825, 0.93, 0.08, { fontFace: EN, fontSize: 7.6, color: n[2], bold: true, align: "center" });
    if (i < nodes.length - 1) arrow(slide, n[1] + 1.08, 1.90, nodes[i + 1][1] - 0.08, 1.90, C.lightText);
  });
  card(slide, 0.84, 3.00, 2.60, 1.18, "继承字段", "agent_run_id / task_id / tool_call_id / trace_id", C.blue, C.white);
  card(slide, 3.70, 3.00, 2.60, 1.18, "事实字段", "PID/TGID/PPID、comm、cwd、cgroup_id", C.green, C.white);
  card(slide, 6.56, 3.00, 2.60, 1.18, "复盘结果", "一条链路回放所有子进程与对象", C.purple, C.white);
  source(slide, "代码证据：backend/event_context.go 从父 PID 或 cgroup 继承 Agent 上下文；执行图谱将进程、文件、网络和策略关联起来。");
}

{
  const slide = pptx.addSlide();
  title(slide, "09", "创新三：语义-事实一致性检测", "把工具声明的意图与 eBPF 事实对比，识别“说是读文件，实际在外联/读密钥”。", 11);
  card(slide, 0.72, 1.38, 2.45, 2.40, "语义输入", "• tool_name / task / prompt 摘要\n• wrapper 决策请求\n• native hook 元数据\n• 预期资源与只读/可写声明", C.blue, C.white);
  arrow(slide, 3.28, 2.55, 3.62, 2.55, C.lightText);
  card(slide, 3.74, 1.38, 2.45, 2.40, "事实输入", "• exec/open/connect/send\n• 进程树、路径、网络端点\n• BPF LSM/cgroup 决策\n• 阻断/告警/放行结果", C.green, C.white);
  arrow(slide, 6.30, 2.55, 6.64, 2.55, C.lightText);
  card(slide, 6.76, 1.38, 2.45, 2.40, "输出告警", "• SECRET_ACCESS\n• UNEXPECTED_NETWORK_EGRESS\n• WORKSPACE_ESCAPE\n• TOKEN_EXFIL_RISK", C.red, C.white);
  rect(slide, 0.96, 4.28, 8.08, 0.46, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "示例：任务声明为只读代码审查，但子进程访问 ~/.ssh 或向未知公网地址发送数据 → 触发高风险告警/阻断。", 1.16, 4.43, 7.68, 0.10, { fontSize: 8.0, color: C.text, align: "center" });
  source(slide, "代码证据：backend/semantic_alerts.go；backend/event_envelope.go；backend/execution_graph.go。");
}

{
  const slide = pptx.addSlide();
  title(slide, "10", "演示场景：三分钟证明“看得见、说得清、拦得住”", "现场优先展示硬核闭环，避免只展示静态大屏。", 12);
  const demos = [
    ["00:00", "敏感路径", "Agent 尝试读取 .env / ssh key → SECRET_ACCESS + 任务归因", C.red],
    ["00:50", "异常外联", "curl / python 子进程连接未知 IP:port → UNEXPECTED_NETWORK_EGRESS", C.orange],
    ["01:40", "内核阻断", "BPF LSM 返回 EACCES，cgroup 拒绝 connect/sendmsg", C.green],
    ["02:30", "图谱回放", "Agent Run → Tool Call → Process → File/Network → Policy", C.blue],
  ];
  demos.forEach((d, i) => {
    const y = 1.40 + i * 0.78;
    rect(slide, 0.78, y, 8.52, 0.50, i % 2 ? C.panel : C.white, C.line, { radius: 0.025 });
    addText(slide, d[0], 1.02, y + 0.18, 0.60, 0.08, { fontFace: EN, fontSize: 7.3, color: d[3], bold: true, align: "center" });
    line(slide, 1.78, y + 0.12, 1.78, y + 0.38, d[3], 2);
    addText(slide, d[1], 2.02, y + 0.13, 1.12, 0.13, { fontSize: 10.4, color: C.ink, bold: true });
    addText(slide, d[2], 3.32, y + 0.14, 5.45, 0.12, { fontSize: 7.8, color: C.text });
  });
  rect(slide, 1.00, 4.70, 8.00, 0.30, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "答辩话术：我们不是只做可视化，而是在内核执行路径上获取证据并形成策略闭环。", 1.20, 4.795, 7.60, 0.08, { fontSize: 7.8, color: C.ink, bold: true, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "11", "与评审规则对齐：四个维度逐项回应", "高教主赛道创意组重点：个人成长 30、项目创新 30、产业价值 25、团队协作 15。", 13);
  const evals = [
    ["个人成长 30", "课程知识到 eBPF 原型；调研 Agent 安全真实问题；专创融合。", C.blue],
    ["项目创新 30", "BPF LSM、内核态阻断、进程树、语义-事实一致性。", C.green],
    ["产业价值 25", "AI coding 团队、高校实验室、企业安全审计强需求。", C.orange],
    ["团队协作 15", "内核、后端、前端、算法、商业分工清晰。", C.purple],
  ];
  evals.forEach((e, i) => {
    const x = 0.78 + (i % 2) * 4.42;
    const y = 1.44 + Math.floor(i / 2) * 1.38;
    card(slide, x, y, 3.86, 0.98, e[0], e[1], e[2], C.white);
  });
  rect(slide, 0.96, 4.62, 8.08, 0.30, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "规则依据：2025 高教主赛道创意组评审规则；2026 正式通知发布后需复核。", 1.18, 4.72, 7.64, 0.08, { fontSize: 7.8, color: C.muted, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "12", "目标市场：AI Agent 落地越快，运行时安全越刚需", "从教育科研切入，扩展到 AI coding 团队和企业私有化部署。", 14);
  rect(slide, 0.70, 1.36, 3.22, 3.20, C.white, C.line, { radius: 0.035 });
  containImage(slide, img("gitguardian_state_2026.png"), 0.90, 1.56, 2.82, 1.56, { frame: true, bg: C.white });
  addText(slide, "风险信号", 0.94, 3.34, 1.2, 0.20, { fontSize: 12.2, color: C.red, bold: true });
  addText(slide, "GitGuardian：AI service secrets 2025 年达到 1,275,105，YoY +81%；AI 辅助提交泄露率高于全站基线。", 0.94, 3.70, 2.72, 0.36, { fontSize: 7.0, color: C.text, fit: "shrink" });
  const markets = [
    ["高校实验室", "eBPF/AI安全教学\n课程实验与竞赛\n软著/论文/项目成果", C.blue],
    ["AI coding 团队", "本地/云端 Agent 审计\n敏感文件与外联治理\n团队策略模板", C.green],
    ["企业安全", "私有化部署\n审计报表与 SIEM\n合规留存", C.orange],
  ];
  markets.forEach((m, i) => card(slide, 4.28 + i * 1.70, 1.50, 1.48, 2.56, m[0], m[1], m[2], C.white));
  rect(slide, 4.34, 4.46, 4.72, 0.30, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "打法：先低门槛开源获客，再用专业版/私有化/培训完成转化。", 4.50, 4.555, 4.40, 0.08, { fontSize: 7.8, bold: true, color: C.ink, align: "center" });
  source(slide, "市场证据：Stack Overflow 2025 AI adoption；GitGuardian 2026 secrets sprawl；IBM 2025 AI oversight gap。");
}

{
  const slide = pptx.addSlide();
  title(slide, "13", "商业模式：开源核心 + 专业版 + 私有化 + 教育服务", "保留技术影响力，同时形成可收费的团队、企业与教学版本。", 15);
  const plans = [
    ["Community", "免费/开源", "单机观测、基础告警、教学实验"],
    ["Team Pro", "3.98万/年起", "团队策略、报表/训练样本、多用户"],
    ["Enterprise", "19.8万/年起", "私有化/多节点、SSO、审计留存、SIEM"],
    ["Education Kit", "3万/套起", "课程实验、靶场、竞赛训练营"],
    ["Consulting", "5-20万/项目", "PoC、红队 replay、策略调优"],
  ];
  rect(slide, 0.72, 1.42, 8.56, 3.02, C.white, C.line, { radius: 0.035 });
  const widths = [1.72, 1.62, 4.72];
  addText(slide, "版本", 1.02, 1.72, widths[0], 0.12, { fontSize: 8.2, color: C.blue, bold: true });
  addText(slide, "价格", 2.74, 1.72, widths[1], 0.12, { fontSize: 8.2, color: C.blue, bold: true });
  addText(slide, "核心价值", 4.38, 1.72, widths[2], 0.12, { fontSize: 8.2, color: C.blue, bold: true });
  line(slide, 0.94, 1.98, 9.06, 1.98, C.line, 0.8);
  plans.forEach((p, i) => {
    const y = 2.18 + i * 0.40;
    if (i % 2 === 1) slide.addShape(pptx.ShapeType.rect, { x: 0.92, y: y - 0.05, w: 8.16, h: 0.30, fill: { color: C.panel }, line: { color: C.panel } });
    addText(slide, p[0], 1.02, y, widths[0], 0.10, { fontFace: EN, fontSize: 7.5, color: A[i], bold: true });
    addText(slide, p[1], 2.74, y, widths[1], 0.10, { fontSize: 7.4, color: C.text });
    addText(slide, p[2], 4.38, y, widths[2], 0.10, { fontSize: 7.4, color: C.text });
  });
  addText(slide, "注：价格为校赛版测算，提交前建议用真实客户访谈、试点意向或合同报价校准。", 1.04, 4.78, 7.92, 0.10, { fontSize: 7.4, color: C.muted, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "14", "实施路线：把原型做成可验证产品", "围绕竞赛、试点、知识产权和商业化四条线推进。", 16);
  const mile = [
    ["2026.05-06", "校赛材料", "申报书 / PPT / 演示视频 / 稳定 demo", C.blue],
    ["2026.06-08", "场景验证", "良性/恶意 replay、检测率/阻断率报告", C.green],
    ["2026.08-10", "试点应用", "课程/实验室/企业 PoC 反馈", C.orange],
    ["2026.10-2027.03", "成果固化", "软著/专利/论文/合作证明", C.purple],
    ["2027", "产品化", "专业版、多节点、企业私有化", C.red],
  ];
  line(slide, 1.05, 2.42, 8.80, 2.42, C.line, 2.2);
  mile.forEach((m, i) => {
    const x = 0.66 + i * 1.84;
    slide.addShape(pptx.ShapeType.ellipse, { x: x + 0.62, y: 2.23, w: 0.36, h: 0.36, fill: { color: m[3] }, line: { color: C.white, width: 1 } });
    rect(slide, x, 2.90, 1.48, 1.28, C.white, C.line, { radius: 0.03 });
    addText(slide, m[0], x + 0.11, 3.12, 1.26, 0.10, { fontFace: EN, fontSize: 6.7, color: m[3], bold: true, align: "center" });
    addText(slide, m[1], x + 0.11, 3.43, 1.26, 0.14, { fontSize: 9.6, color: C.ink, bold: true, align: "center" });
    addText(slide, m[2], x + 0.11, 3.78, 1.26, 0.20, { fontSize: 6.5, color: C.text, align: "center", fit: "shrink" });
  });
  addText(slide, "阶段目标都对应可验收证据：脚本结果、演示录屏、试点反馈、知识产权材料和可复现 benchmark。", 0.92, 4.76, 8.15, 0.10, { fontSize: 7.6, color: C.muted, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "15", "团队分工与合规边界：高权限工具必须讲清楚", "提交前将【待填】替换为真实姓名、学号、年级、学院与贡献证据。", 17);
  card(slide, 0.72, 1.38, 2.72, 2.70, "团队分工", "负责人：【待填】总体架构 / 答辩 / 进度\neBPF：【待填】LSM / cgroup / tracepoints\n后端：【待填】Go API / EventEnvelope / 策略\n前端：【待填】Vue / 执行图谱 / 低代码\n算法评测：【待填】ML / replay\n商业调研：【待填】客户访谈 / 财务", C.blue, C.white);
  card(slide, 3.64, 1.38, 2.72, 2.70, "权限与隐私", "• 后端加载 eBPF，保持 root/特权边界\n• 写 API、shell、策略变更均有 token 与 runtime gate\n• TLS 明文捕获默认关闭\n• 事件流只保留摘要/长度/角色/厂商", C.green, C.white);
  card(slide, 6.56, 1.38, 2.72, 2.70, "误报与合规", "• 先观察模式再阻断\n• 回放 benchmark 调优\n• ML/LLM 建议需人工确认\n• 用于本地/授权环境，不采集未授权数据\n• 演示数据必须脱敏", C.red, C.white);
  addText(slide, "团队证明建议：commit 记录、模块负责人截图、导师指导记录、测试报告、软著/论文/专利分工。", 0.96, 4.70, 8.1, 0.10, { fontSize: 7.6, color: C.muted, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "16", "总结：让 AI Agent 可观测、可解释、可回放、可控制", "用公开数据说明刚需，用真实工程证明可行，用可复盘演示争取支持。", 18);
  card(slide, 0.74, 1.30, 2.62, 1.18, "证明 1：真实需求", "Stack Overflow、GitGuardian、IBM 数据说明 AI 开发工具普及、密钥泄露和治理缺口已经同时出现。", C.blue, C.white);
  card(slide, 3.68, 1.30, 2.62, 1.18, "证明 2：真实风险", "EchoLeak、MCP Tool Poisoning、Copilot 信息泄露案例说明工具链和上下文边界已是攻击面。", C.red, C.white);
  card(slide, 6.62, 1.30, 2.62, 1.18, "证明 3：真实原型", "eBPF + BPF LSM/cgroup + hooks/wrapper + Execution Graph，形成可演示、可验证闭环。", C.green, C.white);
  rect(slide, 0.88, 2.94, 8.24, 0.48, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "需要支持：导师/学院资源、试点用户、软著/专利指导。目标：校赛晋级、省赛打磨、形成可转化产品。", 1.10, 3.10, 7.80, 0.10, { fontSize: 8.8, color: C.ink, bold: true, align: "center" });
  rect(slide, 0.88, 3.75, 8.24, 0.70, C.white, C.line, { radius: 0.03 });
  addText(slide, "主要来源：Stack Overflow Developer Survey 2025；GitGuardian State of Secrets Sprawl 2026；IBM Cost of a Data Breach Report 2025；OWASP Top 10 for LLM Applications / Agentic AI Threats；CSA M365 Copilot CVE-2026-24299 research note；NVD CVE-2025-32711。", 1.08, 3.92, 7.84, 0.30, { fontSize: 6.8, color: C.text, align: "center", fit: "shrink" });
  addText(slide, "谢谢各位老师，请批评指正", 3.10, 4.76, 3.8, 0.20, { fontSize: 14.2, bold: true, color: C.blue, align: "center" });
}

pptx.writeFile({ fileName: OUT }).then(() => {
  console.log(OUT);
}).catch((err) => {
  console.error(err);
  process.exit(1);
});
