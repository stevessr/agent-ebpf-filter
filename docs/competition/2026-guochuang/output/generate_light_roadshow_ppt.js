const pptxgen = require("pptxgenjs");
const imageSize = require("image-size");
const fs = require("fs");
const path = require("path");
const { spawnSync } = require("child_process");

const OUT_DIR = path.join(__dirname, "final");
const ASSET_DIR = path.join(__dirname, "ppt_assets_light");
const OUT = path.join(OUT_DIR, "Agent-eBPF-Filter-项目答辩路演PPT-浅色优化版.pptx");
fs.mkdirSync(OUT_DIR, { recursive: true });

const pptx = new pptxgen();
pptx.author = "Agent eBPF Filter";
pptx.company = "Agent eBPF Filter";
pptx.subject = "国创赛 2026 高教主赛道创意组路演";
pptx.title = "Agent eBPF Filter 项目答辩路演 PPT 技术深度与应用前景版";
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
const PALE = [C.paleBlue, C.paleGreen, C.paleOrange, C.paleRed, C.palePurple, C.paleCyan];
const img = (name) => path.join(ASSET_DIR, name);
let textObjectSeq = 0;

function shouldTypewriter(text, opts) {
  if (opts.typewriter === false) return false;
  if (opts.typewriter === true) return true;
  if (!text || opts.rotate) return false;
  const s = String(text).trim();
  if (s.length < 10) return false;
  if (s.startsWith("SECTION ") || s.startsWith("资料来源：") || s.startsWith("说明：")) return false;
  if (/^\[\d+\]$/.test(s) || /^\d{2}$/.test(s)) return false;
  return /[一-龥A-Za-z]/.test(s);
}

function addText(slide, text, x, y, w, h, opts = {}) {
  const useTypewriter = shouldTypewriter(text, opts);
  const objectName = opts.objectName || `${useTypewriter ? "tw_text" : "static_text"}_${String(++textObjectSeq).padStart(4, "0")}`;
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
    objectName,
  });
}

function rect(slide, x, y, w, h, fill = C.white, line = C.line, opts = {}) {
  slide.addShape(opts.shape || pptx.ShapeType.roundRect, {
    x, y, w, h,
    rectRadius: opts.radius ?? 0.035,
    fill: { color: fill, transparency: opts.transparency || 0 },
    line: { color: line, width: opts.lineWidth ?? 0.8, transparency: opts.lineTransparency ?? 0 },
    shadow: opts.shadow ? { type: "outer", color: "BED0DD", opacity: 0.08, blur: 1, angle: 45, distance: 0.8 } : undefined,
  });
}

function line(slide, x1, y1, x2, y2, color = C.line, width = 1) {
  slide.addShape(pptx.ShapeType.line, { x: x1, y: y1, w: x2 - x1, h: y2 - y1, line: { color, width } });
}

function arrow(slide, x1, y1, x2, y2, color = C.muted, width = 1.4) {
  slide.addShape(pptx.ShapeType.line, { x: x1, y: y1, w: x2 - x1, h: y2 - y1, line: { color, width, beginArrowType: "none", endArrowType: "triangle" } });
}

function pageBadge(slide, index) {
  rect(slide, 9.25, 5.14, 0.48, 0.27, C.white, C.line2, { radius: 0.04 });
  addText(slide, String(index).padStart(2, "0"), 9.25, 5.205, 0.48, 0.10, { fontFace: EN, fontSize: 7.5, bold: true, color: C.blue, align: "center" });
}

function source(slide, text) {
  line(slide, 0.56, 5.06, 8.95, 5.06, C.line2, 0.8);
  addText(slide, text, 0.58, 5.16, 8.25, 0.16, { fontSize: 6.6, color: C.lightText });
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
  if (subtitle) addText(slide, subtitle, 0.75, 1.05, 8.3, 0.22, { fontSize: 9.0, color: C.muted });
}

function pill(slide, text, x, y, w, color = C.blue, fill = C.paleBlue) {
  rect(slide, x, y, w, 0.28, fill, color, { radius: 0.08, lineTransparency: 55 });
  addText(slide, text, x + 0.06, y + 0.075, w - 0.12, 0.10, { fontSize: 7.5, color, bold: true, align: "center" });
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
  addText(slide, heading, x + 0.18, y + 0.16, w - 0.32, 0.22, { fontSize: 11.0, color: C.ink, bold: true });
  addText(slide, body, x + 0.18, y + 0.50, w - 0.34, h - 0.60, { fontSize: 7.8, color: C.text, fit: "shrink" });
}

function codeBox(slide, text, x, y, w, h) {
  rect(slide, x, y, w, h, "F6F9FC", "DCE8F0", { radius: 0.02 });
  addText(slide, text, x + 0.11, y + 0.11, w - 0.22, h - 0.22, { fontFace: EN, fontSize: 6.7, color: "38566A", fit: "shrink" });
}

function splitRow(slide, x, y, w, h, left, right, color = C.blue) {
  rect(slide, x, y, w, h, C.white, C.line, { radius: 0.02 });
  slide.addShape(pptx.ShapeType.rect, { x, y, w: 1.48, h, fill: { color }, line: { color } });
  addText(slide, left, x + 0.16, y + h / 2 - 0.06, 1.16, 0.10, { fontSize: 8.0, color: C.white, bold: true, align: "center" });
  addText(slide, right, x + 1.70, y + 0.13, w - 1.90, h - 0.18, { fontSize: 7.6, color: C.text, fit: "shrink" });
}

function axisCard(slide, x, y, w, h, num, heading, body, color, fill) {
  rect(slide, x, y, w, h, fill, color, { radius: 0.03, lineTransparency: 35 });
  addText(slide, num, x + 0.14, y + 0.14, 0.44, 0.20, { fontFace: EN, fontSize: 12, color, bold: true });
  addText(slide, heading, x + 0.68, y + 0.16, w - 0.82, 0.18, { fontSize: 10.0, color: C.ink, bold: true });
  addText(slide, body, x + 0.20, y + 0.52, w - 0.38, h - 0.62, { fontSize: 7.4, color: C.text, fit: "shrink" });
}

function injectTypewriterAnimations(fileName) {
  const script = String.raw`
import os
import re
import sys
import tempfile
import zipfile
from xml.sax.saxutils import escape

pptx_path = sys.argv[1]
tmp_path = pptx_path + ".tmp"
slide_re = re.compile(r"ppt/slides/slide(\d+)\.xml$")
shape_re = re.compile(r'<p:cNvPr[^>]*id="(\d+)"[^>]*name="(tw_text_\d+)"[^>]*>')


def timing_xml(spids):
    tid = 1
    def next_id():
        nonlocal tid
        tid += 1
        return tid

    items = []
    blds = []
    for pos, spid in enumerate(spids):
        outer_id = next_id()
        effect_id = next_id()
        anim_id = next_id()
        dur = 520 + min(1300, pos * 45)
        delay = 0 if pos == 0 else 90
        items.append(f'''<p:par><p:cTn id="{outer_id}" fill="hold"><p:stCondLst><p:cond delay="{delay}"/></p:stCondLst><p:childTnLst><p:par><p:cTn id="{effect_id}" presetID="1" presetClass="entr" presetSubtype="0" fill="hold" nodeType="clickEffect"><p:stCondLst><p:cond delay="0"/></p:stCondLst><p:childTnLst><p:animEffect transition="in" filter="typewriter"><p:cBhvr><p:cTn id="{anim_id}" dur="{dur}" fill="hold"/><p:tgtEl><p:spTgt spid="{escape(spid)}"/></p:tgtEl></p:cBhvr></p:animEffect></p:childTnLst><p:iterate type="lt"><p:tmAbs val="35"/></p:iterate></p:cTn></p:par></p:childTnLst></p:cTn></p:par>''')
        blds.append(f'<p:bldP spid="{escape(spid)}" grpId="0" build="p"/>')

    return f'''<p:timing><p:tnLst><p:par><p:cTn id="1" dur="indefinite" restart="never" nodeType="tmRoot"><p:childTnLst><p:seq concurrent="1" nextAc="seek"><p:cTn id="2" dur="indefinite" nodeType="mainSeq"><p:childTnLst>{''.join(items)}</p:childTnLst></p:cTn><p:prevCondLst><p:cond evt="onPrev" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:prevCondLst><p:nextCondLst><p:cond evt="onNext" delay="0"><p:tgtEl><p:sldTgt/></p:tgtEl></p:cond></p:nextCondLst></p:seq></p:childTnLst></p:cTn></p:par></p:tnLst><p:bldLst>{''.join(blds)}</p:bldLst></p:timing>'''


def patch_slide(xml):
    spids = [m.group(1) for m in shape_re.finditer(xml)]
    if not spids:
        return xml, 0
    xml = re.sub(r'<p:timing[\s\S]*?</p:timing>', '', xml)
    timing = timing_xml(spids)
    if '<p:extLst>' in xml:
        xml = xml.replace('<p:extLst>', timing + '<p:extLst>', 1)
    else:
        xml = xml.replace('</p:sld>', timing + '</p:sld>', 1)
    return xml, len(spids)

count = 0
slides = 0
with zipfile.ZipFile(pptx_path, 'r') as zin, zipfile.ZipFile(tmp_path, 'w', zipfile.ZIP_DEFLATED) as zout:
    for item in zin.infolist():
        data = zin.read(item.filename)
        if slide_re.match(item.filename):
            text = data.decode('utf-8')
            text, added = patch_slide(text)
            data = text.encode('utf-8')
            if added:
                slides += 1
                count += added
        zout.writestr(item, data)

os.replace(tmp_path, pptx_path)
print(f"typewriter animations: {count} objects across {slides} slides")
`;
  const result = spawnSync("python3", ["-c", script, fileName], { encoding: "utf8" });
  if (result.stdout) process.stdout.write(result.stdout);
  if (result.status !== 0) {
    if (result.stderr) process.stderr.write(result.stderr);
    throw new Error("failed to inject typewriter animations");
  }
}

{
  const slide = pptx.addSlide();
  base(slide, 1);
  slide.addShape(pptx.ShapeType.rect, { x: 6.72, y: 0.08, w: 3.28, h: 5.545, fill: { color: C.panel }, line: { color: C.panel } });
  addText(slide, "国创赛2026 · 高教主赛道 · 创意组", 0.64, 0.56, 3.9, 0.22, { fontSize: 9.6, color: C.blue, bold: true });
  line(slide, 0.64, 1.04, 4.1, 1.04, C.blue, 2.1);
  addText(slide, "Agent eBPF Filter", 0.62, 1.33, 5.85, 0.52, { fontFace: EN, fontSize: 34, color: C.ink, bold: true });
  addText(slide, "面向 AI Agent 的\n内核级运行时安全与可观测平台", 0.64, 2.02, 5.65, 0.88, { fontSize: 23, color: C.ink, bold: true });
  addText(slide, "不是罗列更多功能，而是把一个关键问题做深：\nAgent 的语义承诺，如何在操作系统事实里被验证和约束。", 0.66, 3.08, 5.78, 0.42, { fontSize: 10.2, color: C.text });
  pill(slide, "内核事实采集", 0.66, 3.78, 1.28, C.blue, C.paleBlue);
  pill(slide, "LSM/cgroup 前置阻断", 2.08, 3.78, 1.78, C.green, C.paleGreen);
  pill(slide, "语义-事实一致性", 4.02, 3.78, 1.62, C.purple, C.palePurple);
  addText(slide, "负责人/学院/联系方式：\n【待填】\n推荐学院：【待填】\n负责人：【待填】\n手机号/QQ：【待填】", 0.66, 4.55, 4.05, 0.52, { fontSize: 7.9, color: C.muted });
  addText(slide, "技术深度与应用前景版 · 2026-05-26", 4.34, 5.03, 2.0, 0.16, { fontSize: 7.5, color: C.lightText, align: "right" });
  rect(slide, 7.16, 0.70, 2.34, 1.03, C.white, C.line, { radius: 0.04 });
  addText(slide, "核心主张", 7.38, 0.90, 1.90, 0.18, { fontSize: 12.0, color: C.ink, bold: true, align: "center" });
  addText(slide, "从“看得到”升级到“拦得住、讲得清、能落地”", 7.34, 1.22, 1.98, 0.20, { fontSize: 7.8, color: C.muted, align: "center" });
  metric(slide, "4 层", "采集、归因、判定、阻断", 7.16, 2.02, 2.34, C.blue);
  metric(slide, "3 类", "高校/团队/企业落地场景", 7.16, 2.96, 2.34, C.green);
  metric(slide, "1 条", "可复盘 Agent 证据链", 7.16, 3.90, 2.34, C.purple);
}

{
  const slide = pptx.addSlide();
  title(slide, "00", "路演结构：少讲技术广度，深讲一条闭环", "前半段证明为什么必须做，主体部分讲清技术深水区，后半段说明能在哪些场景形成价值。", 2);
  const items = [
    ["01", "为什么是刚需", "AI Agent 使用率、密钥泄露和 Copilot/MCP 案例说明运行时边界已经成为现实问题。"],
    ["02", "深水区一：内核事实", "围绕 exec/file/net/fork 采集、ringbuf、map、上下文字段，而不是泛泛讲“监控”。"],
    ["03", "深水区二：前置阻断", "BPF LSM 和 cgroup 把策略放到危险动作发生前，形成确定性拒绝证据。"],
    ["04", "深水区三：语义归因", "把 run/task/tool_call 与 PID、PPID、cwd、cgroup、路径、网络端点连成证据链。"],
    ["05", "应用前景", "高校实验室、AI coding 团队、企业私有化和教育服务，分别给出落地价值。"],
  ];
  line(slide, 1.05, 1.52, 1.05, 4.46, C.line, 1.2);
  items.forEach((it, i) => {
    const y = 1.28 + i * 0.68;
    slide.addShape(pptx.ShapeType.ellipse, { x: 0.86, y: y + 0.02, w: 0.38, h: 0.38, fill: { color: i < 3 ? C.blue : C.white }, line: { color: C.blue, width: 1.2 } });
    addText(slide, it[0], 0.86, y + 0.15, 0.38, 0.09, { fontFace: EN, fontSize: 6.2, color: i < 3 ? C.white : C.blue, bold: true, align: "center" });
    addText(slide, it[1], 1.46, y, 1.62, 0.22, { fontSize: 11.8, color: C.ink, bold: true });
    addText(slide, it[2], 3.26, y + 0.02, 5.90, 0.20, { fontSize: 8.1, color: C.text });
  });
  rect(slide, 0.82, 4.76, 8.55, 0.34, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "答辩策略：避免“我们还做了 X/Y/Z”的横向铺陈，改为反复回答“为什么这个内核闭环难、为什么能落地”。", 1.02, 4.865, 8.15, 0.09, { fontSize: 7.9, color: C.text, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "01", "数据证据：Agent 普及越快，运行时事实越稀缺", "公开调研和安全报告共同指向同一个缺口：AI 工具进入执行链，但审计和控制仍停留在外围。", 3);
  metric(slide, "84%", "Stack Overflow 2025：正在使用或计划使用 AI 开发工具 [1]", 0.62, 1.38, 1.66, C.blue);
  metric(slide, "28.65M", "GitGuardian 2026：2025 年 public GitHub 新增硬编码密钥 [2]", 2.42, 1.38, 1.82, C.red);
  metric(slide, "$4.44M", "IBM 2025：全球平均数据泄露成本 [3]", 4.38, 1.38, 1.52, C.purple);
  metric(slide, "97%", "IBM 2025：AI 相关事件组织缺少适当访问控制 [3]", 6.04, 1.38, 1.54, C.orange);
  metric(slide, "24,008", "GitGuardian 2026：公开 MCP 配置中暴露的唯一密钥 [2]", 7.72, 1.38, 1.66, C.green);
  rect(slide, 0.62, 2.42, 4.32, 2.28, C.white, C.line, { radius: 0.035 });
  containImage(slide, img("stackoverflow_ai_usage.png"), 0.84, 2.62, 3.90, 1.56, { frame: true, bg: C.white });
  addText(slide, "采用率提升不是重点，重点是 Agent 已经开始触发终端、文件和网络行为。安全边界必须落到运行时 [1]。", 0.86, 4.30, 3.85, 0.16, { fontSize: 7.0, color: C.muted });
  rect(slide, 5.16, 2.42, 4.22, 2.28, C.white, C.line, { radius: 0.035 });
  containImage(slide, img("gitguardian_commits_developers.png"), 5.38, 2.62, 3.80, 1.56, { frame: true, bg: C.white });
  addText(slide, "软件生产提速后，密钥、依赖脚本、MCP 配置的扩散速度也变快；只看 prompt 已经不够 [2]。", 5.40, 4.30, 3.72, 0.16, { fontSize: 7.0, color: C.muted });
  source(slide, "资料来源：[1] Stack Overflow Developer Survey 2025；[2] GitGuardian State of Secrets Sprawl 2026；[3] IBM Cost of a Data Breach Report 2025。详见参考文献页。");
}

{
  const slide = pptx.addSlide();
  title(slide, "02", "真实案例：风险不是模型幻觉，而是执行链越界", "报告截图保留为外部证据，但结论聚焦到一个工程问题：如何把越界行为在 OS 层归因和拦截。", 4);
  const cases = [
    { x: 0.62, img: "csa_copilot_cover_thumb.png", t: "上下文越权 [5][6]", d: "Copilot / EchoLeak 类案例说明：外部输入可能触发内部知识检索或数据泄露，风险发生在工具链边界。", c: C.red },
    { x: 3.70, img: "gitguardian_mcp_valid_secrets.png", t: "配置即攻击面 [2]", d: "MCP 配置中的有效密钥进入公共仓库，说明 Agent 生态的本机配置、工具描述和凭据都需要治理。", c: C.green },
    { x: 6.78, img: "owasp_llm_cover_thumb.png", t: "工具误用 [4]", d: "OWASP 将 tool misuse、excessive agency、untraceability 列为重点，正对应本项目的运行时证据链。", c: C.blue },
  ];
  cases.forEach((c) => {
    rect(slide, c.x, 1.38, 2.60, 3.40, C.white, C.line, { radius: 0.035 });
    containImage(slide, img(c.img), c.x + 0.18, 1.58, 2.24, 1.45, { frame: true, bg: "F8FAFC", line: C.line2 });
    addText(slide, c.t, c.x + 0.18, 3.28, 2.24, 0.24, { fontSize: 11.3, color: c.c, bold: true });
    addText(slide, c.d, c.x + 0.18, 3.68, 2.22, 0.60, { fontSize: 7.2, color: C.text, fit: "shrink" });
    pill(slide, "归因 + 拦截 + 回放", c.x + 0.18, 4.40, 1.38, c.c, c.c === C.red ? C.paleRed : c.c === C.green ? C.paleGreen : C.paleBlue);
  });
  source(slide, "资料来源：[2] GitGuardian 2026；[4] OWASP LLM/Agentic AI Threats；[5] CSA Copilot research note；[6] NVD CVE-2025-32711。详见参考文献页。");
}

{
  const slide = pptx.addSlide();
  title(slide, "03", "核心问题：不是“多做几个监控项”，而是补上证据链断点", "技术深度集中在一个闭环：Agent 声称要做什么、系统实际做了什么、危险动作是否被前置拦截。", 5);
  card(slide, 0.72, 1.36, 2.68, 2.46, "断点一：意图到进程", "Agent 的 run/task/tool_call 通常进入 shell，再派生 python/node/git/npm/curl。传统日志看到 PID，却丢失了意图来源。", C.blue, C.white);
  card(slide, 3.66, 1.36, 2.68, 2.46, "断点二：进程到资源", "危险行为落在 open/connect/sendmsg/rename 等 OS 事件上，必须记录路径、端点、cwd、cgroup 和父子关系。", C.orange, C.white);
  card(slide, 6.60, 1.36, 2.68, 2.46, "断点三：告警到阻断", "事后告警无法证明动作没有发生。对高风险路径和外联，需要在 LSM/cgroup 决策点返回拒绝。", C.red, C.white);
  rect(slide, 0.92, 4.28, 8.12, 0.48, C.panel, C.line2, { radius: 0.02 });
  addText(slide, "因此本项目不追求“覆盖所有安全功能”，而是把 Agent 运行时安全最难的一段——语义归因 + 内核事实 + 前置阻断——做深。", 1.12, 4.44, 7.72, 0.12, { fontSize: 8.6, color: C.ink, bold: true, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "04", "总体架构：四层闭环，每层只解决一个深问题", "从语义入口到内核决策再到证据回放，架构强调深度链路而不是横向功能堆叠。", 6);
  const layers = [
    ["语义入口", "wrapper / native hooks / adapters 捕获 run、task、tool_call、intent", C.blue],
    ["上下文继承", "agent_run_id / task_id / tool_call_id / trace_id 随 PID、PPID、cgroup 传播", C.purple],
    ["内核事实", "eBPF 采集 exec、file、net、fork，形成不可伪造的 OS 行为证据", C.green],
    ["前置处置", "BPF LSM 与 cgroup 在动作发生前做 ALLOW / ALERT / BLOCK / REWRITE", C.red],
  ];
  layers.forEach((l, i) => {
    const y = 1.42 + i * 0.68;
    rect(slide, 0.84, y, 8.32, 0.50, i % 2 ? C.panel : C.white, C.line, { radius: 0.02 });
    slide.addShape(pptx.ShapeType.rect, { x: 0.84, y, w: 0.08, h: 0.50, fill: { color: l[2] }, line: { color: l[2] } });
    addText(slide, l[0], 1.12, y + 0.16, 1.10, 0.12, { fontSize: 9.4, color: l[2], bold: true });
    addText(slide, l[1], 2.48, y + 0.16, 6.30, 0.12, { fontSize: 8.0, color: C.text });
    if (i < layers.length - 1) arrow(slide, 5.00, y + 0.52, 5.00, y + 0.66, C.lightText, 1.0);
  });
  rect(slide, 1.08, 4.58, 7.84, 0.34, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "可答辩表达：我们不是做一个“安全看板”，而是把 Agent 意图和内核事实接成一条可验证、可阻断、可回放的闭环。", 1.28, 4.69, 7.44, 0.08, { fontSize: 7.7, color: C.ink, bold: true, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "05", "技术深水区一：eBPF 事实采集不是日志替代，而是可信证据底座", "重点解释为什么选 eBPF：低侵入、内核态事件源、可关联进程树和资源对象。", 7);
  card(slide, 0.70, 1.34, 2.55, 2.36, "采集点", "execve / openat / connect / sendto / recvfrom / bind / unlink / mkdir / ioctl 等关键 syscall 与网络路径。", C.blue, C.white);
  card(slide, 3.44, 1.34, 2.55, 2.36, "数据路径", "BPF map 维护 Agent 进程、关注命令与关注路径；ringbuf 将事件送到用户态 runtime。", C.green, C.white);
  card(slide, 6.18, 1.34, 3.10, 2.36, "证据字段", "PID/TGID/PPID、comm、cwd、路径、网络端点、cgroup_id、时间戳、策略结果，统一进入 EventEnvelope。", C.purple, C.white);
  codeBox(slide, "内核事件采集模块\n事件归一化与分发模块\n运行时状态管理模块\n证据信封模型模块", 0.92, 4.10, 3.10, 0.62);
  splitRow(slide, 4.30, 4.08, 4.68, 0.66, "技术难点", "既要拿到足够细的事实字段，又不能把每个 syscall 都变成高开销日志；未来可扩展为可配置采集策略和行业模板。", C.blue);
}

{
  const slide = pptx.addSlide();
  title(slide, "06", "技术深水区二：BPF LSM + cgroup 把安全决策前置到内核路径", "真正的亮点不是“发现风险”，而是在危险动作发生前给出确定性拒绝。", 8);
  const blocks = [
    ["文件/执行", "bprm_check_security、file_open、file_permission、inode_*：在执行、读写、创建、删除、重命名前判断策略。", C.green],
    ["网络外联", "connect4/connect6、sendmsg4/sendmsg6：按 cgroup、IP、IPv6、端口对 TCP/UDP 外联做前置约束。", C.blue],
    ["决策原则", "内核态只做确定性 map lookup；ML/LLM 只输出建议，不直接进入内核阻断路径。", C.red],
  ];
  blocks.forEach((b, i) => card(slide, 0.72 + i * 2.94, 1.36, 2.60, 2.42, b[0], b[1], b[2], C.white));
  splitRow(slide, 0.92, 4.18, 8.14, 0.52, "答辩亮点", "评委能听懂的差异：UI 标红只是提醒，LSM/cgroup 返回 -EACCES 或拒绝 connect 才是 OS 层真实拦截。", C.red);
  source(slide, "工程证据：内核文件/执行策略模块；cgroup 网络策略模块；OS 阻断冒烟验证流程。未来可沉淀为多场景策略模板。");
}

{
  const slide = pptx.addSlide();
  title(slide, "07", "技术深水区三：进程上下文继承解决 Agent 子进程“失踪”", "真实风险常发生在 shell/python/node/git/npm/curl 子进程，必须把语义上下文随进程树传播。", 9);
  const nodes = [
    ["Agent Run", 0.78, C.purple],
    ["Tool Call", 2.16, C.blue],
    ["/bin/sh", 3.56, C.orange],
    ["python/node", 4.92, C.orange],
    ["file/net", 6.48, C.red],
    ["Policy", 7.82, C.green],
  ];
  nodes.forEach((n, i) => {
    rect(slide, n[1], 1.58, 1.08, 0.48, C.white, n[2], { radius: 0.03 });
    addText(slide, n[0], n[1] + 0.06, 1.745, 0.96, 0.08, { fontFace: EN, fontSize: 7.1, color: n[2], bold: true, align: "center" });
    if (i < nodes.length - 1) arrow(slide, n[1] + 1.10, 1.82, nodes[i + 1][1] - 0.08, 1.82, C.lightText);
  });
  axisCard(slide, 0.82, 2.72, 2.62, 1.42, "A", "继承字段", "agent_run_id、task_id、tool_call_id、trace_id 从父 PID 或 cgroup 继承，避免只看到孤立子进程。", C.blue, C.paleBlue);
  axisCard(slide, 3.70, 2.72, 2.62, 1.42, "B", "事实字段", "PID/TGID/PPID、comm、cwd、cgroup_id 记录真实执行链，支撑后续图谱回放。", C.green, C.paleGreen);
  axisCard(slide, 6.58, 2.72, 2.62, 1.42, "C", "归因结果", "把 file/net/policy 事件归到具体 Agent run 和 tool call，而不是只归到 python 或 curl。", C.purple, C.palePurple);
  source(slide, "工程证据：Agent 上下文继承模块；执行图谱模块；交互式会话管理模块。未来可扩展为跨 IDE、跨云端 Agent 的统一追踪层。");
}

{
  const slide = pptx.addSlide();
  title(slide, "08", "技术深水区四：语义-事实一致性，不靠模型猜测做最终裁决", "让 LLM/规则负责解释和建议，让内核事实负责证据，让确定性策略负责阻断。", 10);
  card(slide, 0.72, 1.34, 2.45, 2.38, "语义输入", "工具名、任务摘要、hook 元数据、wrapper 请求、只读/可写声明、预期访问范围。", C.blue, C.white);
  arrow(slide, 3.28, 2.52, 3.62, 2.52, C.lightText);
  card(slide, 3.74, 1.34, 2.45, 2.38, "事实输入", "exec/open/connect/send、路径、端点、进程树、cgroup、LSM/cgroup 决策。", C.green, C.white);
  arrow(slide, 6.30, 2.52, 6.64, 2.52, C.lightText);
  card(slide, 6.76, 1.34, 2.45, 2.38, "输出结果", "SECRET_ACCESS、UNEXPECTED_NETWORK_EGRESS、WORKSPACE_ESCAPE、TOKEN_EXFIL_RISK。", C.red, C.white);
  splitRow(slide, 0.96, 4.08, 8.08, 0.58, "安全边界", "模型可以辅助解释风险，但不能直接下发不可解释的内核阻断；真正拦截由认证策略和确定性 map 决策完成。", C.purple);
  source(slide, "工程证据：语义风险检测模块；事件信封模型；执行图谱；命令策略代理。未来可扩展为组织级 Agent 安全策略中心。");
}

{
  const slide = pptx.addSlide();
  title(slide, "09", "工程深度：性能、权限和误报边界必须讲清楚", "高权限安全工具不能只展示能力，还要说明开销、权限、误报和隐私边界如何控制。", 11);
  splitRow(slide, 0.78, 1.34, 8.44, 0.58, "性能边界", "只采关键 syscall 与网络路径；tracked_comm/path、agent_pids、cgroup 过滤降低噪声；ringbuf 异步上报避免阻塞正常路径。", C.blue);
  splitRow(slide, 0.78, 2.08, 8.44, 0.58, "权限边界", "eBPF 加载需要特权；配置 API 使用 token；shell 与策略变更有 runtime gate；TLS 明文捕获默认关闭。", C.green);
  splitRow(slide, 0.78, 2.82, 8.44, 0.58, "误报边界", "先观察后阻断；高风险路径默认告警，确认后进入 LSM/cgroup 阻断；ML/LLM 输出进入人工确认或策略模板。", C.orange);
  splitRow(slide, 0.78, 3.56, 8.44, 0.58, "合规边界", "用于本地/授权环境；事件流以摘要、路径、端点、长度、角色为主；演示数据必须脱敏。", C.red);
  rect(slide, 1.04, 4.54, 7.92, 0.32, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "这一页用于回应评委追问：能不能跑、会不会拖慢、权限是否过大、误报怎么处理。", 1.24, 4.645, 7.52, 0.08, { fontSize: 7.7, color: C.ink, bold: true, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "10", "验证方式：用三类实验证明深度闭环，而不是只放 UI 截图", "现场演示围绕“采集到、归因准、拦得住”三件事设计。", 12);
  const demos = [
    ["实验 A", "敏感路径读取", "Agent 声称只读项目文件，但子进程访问 .env / ssh key → SECRET_ACCESS + 任务归因", C.red],
    ["实验 B", "异常外联", "curl/python 子进程连接未知 IP:port → UNEXPECTED_NETWORK_EGRESS，并关联到 tool_call", C.orange],
    ["实验 C", "前置阻断", "命中 LSM/cgroup 策略后返回 EACCES 或拒绝 connect/sendmsg，证据进入图谱", C.green],
    ["实验 D", "图谱回放", "Agent Run → Tool Call → Process → File/Network → Policy，形成可提交证据链", C.blue],
  ];
  demos.forEach((d, i) => {
    const y = 1.34 + i * 0.74;
    rect(slide, 0.78, y, 8.52, 0.50, i % 2 ? C.panel : C.white, C.line, { radius: 0.025 });
    addText(slide, d[0], 1.02, y + 0.18, 0.72, 0.08, { fontFace: EN, fontSize: 7.2, color: d[3], bold: true, align: "center" });
    line(slide, 1.88, y + 0.12, 1.88, y + 0.38, d[3], 2);
    addText(slide, d[1], 2.14, y + 0.13, 1.20, 0.13, { fontSize: 9.6, color: C.ink, bold: true });
    addText(slide, d[2], 3.54, y + 0.14, 5.30, 0.12, { fontSize: 7.5, color: C.text });
  });
  splitRow(slide, 1.00, 4.56, 8.00, 0.38, "验收指标", "检测率/阻断率、误报样例、平均事件延迟、CPU/内存开销、证据链完整率。", C.purple);
}

{
  const slide = pptx.addSlide();
  title(slide, "11", "未来前景总览：从单机工具走向 Agent 安全基础设施", "应用前景不再泛泛讲市场，而是把内核级证据链扩展成教育、团队、企业三类长期基础能力。", 13);
  const apps = [
    ["高校实验室", "教学/科研/竞赛", "沉淀为 AI 安全与系统安全实验平台，支撑课程、论文、软著、竞赛和校企联合实践。", C.blue, C.paleBlue],
    ["AI coding 团队", "开发治理", "发展为团队级 Agent 安全工作台，持续管理敏感文件、依赖脚本、MCP 工具和异常外联。", C.green, C.paleGreen],
    ["企业安全", "合规与私有化", "演进为企业 Agent runtime security 基础设施，接入审计、SIEM/OTLP、策略审批和合规留存。", C.orange, C.paleOrange],
  ];
  apps.forEach((a, i) => {
    const x = 0.72 + i * 2.92;
    rect(slide, x, 1.42, 2.58, 2.92, C.white, C.line, { radius: 0.035 });
    addText(slide, a[0], x + 0.20, 1.70, 2.16, 0.24, { fontSize: 12.5, color: a[3], bold: true, align: "center" });
    pill(slide, a[1], x + 0.62, 2.14, 1.34, a[3], a[4]);
    addText(slide, a[2], x + 0.26, 2.72, 2.04, 0.88, { fontSize: 7.8, color: C.text, align: "center", fit: "shrink" });
    splitRow(slide, x + 0.22, 3.82, 2.14, 0.34, "价值", i === 0 ? "成果转化" : i === 1 ? "降低事故" : "合规落地", a[3]);
  });
  source(slide, "资料来源：[1] Stack Overflow 2025 AI adoption；[2] GitGuardian 2026 secrets sprawl；[3] IBM 2025 AI oversight gap。详见参考文献页。");
}

{
  const slide = pptx.addSlide();
  title(slide, "12", "应用前景一：高校实验室，从竞赛项目变成教学与科研平台", "国创赛场景下，最容易先落地的是可复现实验、课程实践和安全竞赛训练。", 14);
  axisCard(slide, 0.78, 1.36, 2.62, 1.45, "1", "课程实验", "eBPF syscall 观测、LSM 阻断、Agent 风险 replay，可做成 AI 安全/系统安全课程实验。", C.blue, C.paleBlue);
  axisCard(slide, 3.70, 1.36, 2.62, 1.45, "2", "科研题目", "Agentic AI runtime guardrail、语义-事实一致性、执行图谱和误报控制，可沉淀论文/专利方向。", C.purple, C.palePurple);
  axisCard(slide, 6.62, 1.36, 2.62, 1.45, "3", "竞赛训练", "用良性/恶意任务 replay 构造靶场，训练学生识别 prompt 注入、MCP 密钥、异常外联。", C.green, C.paleGreen);
  rect(slide, 0.86, 3.38, 8.28, 0.92, C.white, C.line, { radius: 0.035 });
  addText(slide, "落地路径", 1.12, 3.62, 0.92, 0.18, { fontSize: 10.8, color: C.ink, bold: true });
  addText(slide, "先以开源 demo + 实验文档进入课程/实验室；用竞赛演示和测试报告证明可复现；再争取软著、导师课题、校企联合实践。", 2.18, 3.62, 6.55, 0.22, { fontSize: 8.0, color: C.text });
  splitRow(slide, 1.00, 4.62, 8.00, 0.36, "近期成果", "演示视频、实验指导书、benchmark 数据集、软著/专利材料、导师推荐或试点证明。", C.blue);
}

{
  const slide = pptx.addSlide();
  title(slide, "13", "应用前景二：AI coding 团队，解决“能用但不敢放开用”", "团队不是缺 AI 工具，而是缺本地执行链的边界、审计和复盘能力。", 15);
  rect(slide, 0.70, 1.32, 3.10, 3.10, C.white, C.line, { radius: 0.035 });
  containImage(slide, img("gitguardian_state_2026.png"), 0.92, 1.54, 2.66, 1.40, { frame: true, bg: C.white });
  addText(slide, "付费理由", 0.96, 3.18, 1.10, 0.18, { fontSize: 11.2, color: C.red, bold: true });
  addText(slide, "AI-assisted commits 与 MCP 配置泄露会直接影响代码资产、凭据和客户数据；团队负责人需要可追责证据。", 0.96, 3.54, 2.54, 0.34, { fontSize: 7.1, color: C.text, fit: "shrink" });
  const vals = [
    ["策略模板", "只读审查、依赖安装、网络访问、敏感路径等团队级策略包", C.blue],
    ["审计报表", "按 run/task/tool_call 输出证据链，支持复盘与责任界定", C.green],
    ["训练样本", "将真实告警转成 replay 数据，持续降低误报和漏报", C.purple],
  ];
  vals.forEach((v, i) => card(slide, 4.18, 1.34 + i * 0.96, 4.86, 0.74, v[0], v[1], v[2], i % 2 ? C.panel : C.white));
  splitRow(slide, 4.18, 4.48, 4.86, 0.40, "商业入口", "Team Pro：团队策略、多人审计、报表导出、告警训练集。", C.green);
  source(slide, "资料来源：[2] GitGuardian State of Secrets Sprawl 2026；[1] Stack Overflow Developer Survey 2025。详见参考文献页。");
}

{
  const slide = pptx.addSlide();
  title(slide, "14", "未来前景三：企业私有化，成为 Agent 安全基础设施", "企业价值不只是 UI，而是把 Agent 执行证据送进合规、审计、安全运营和策略治理流程。", 16);
  const cols = [
    ["部署形态", "单机开发机\n团队网关\nK8s / 多节点\n离线私有化", C.blue],
    ["集成对象", "SIEM / OTLP\n审计留存\nSSO / token\n策略审批", C.green],
    ["交付价值", "外联约束\n敏感路径保护\n事故复盘\n合规报告", C.orange],
    ["服务收入", "PoC\n私有化部署\n策略调优\n培训与靶场", C.purple],
  ];
  cols.forEach((c, i) => card(slide, 0.64 + i * 2.28, 1.38, 2.02, 2.90, c[0], c[1], c[2], C.white));
  rect(slide, 0.94, 4.54, 8.10, 0.36, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "长期前景：当 Agent 成为企业标准生产力工具，runtime guardrail 会像日志、EDR、CI 安全一样成为基础设施。", 1.14, 4.655, 7.70, 0.08, { fontSize: 7.8, color: C.ink, bold: true, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "15", "商业模式：围绕深度能力收费，而不是围绕页面数量收费", "开源用于建立信任，付费集中在团队治理、私有化交付和教育内容。", 17);
  const plans = [
    ["Community", "免费/开源", "单机观测、基础告警、教学实验", "开发者获客与课程传播"],
    ["Team Pro", "3.98万/年起", "团队策略、报表、训练样本、多用户", "AI coding 团队治理"],
    ["Enterprise", "19.8万/年起", "私有化、多节点、SSO、SIEM、审计留存", "企业合规与安全运营"],
    ["Education Kit", "3万/套起", "课程实验、靶场、竞赛训练营", "高校实验室与培训"],
    ["Consulting", "5-20万/项目", "PoC、红队 replay、策略调优", "落地服务收入"],
  ];
  rect(slide, 0.62, 1.34, 8.76, 3.36, C.white, C.line, { radius: 0.035 });
  addText(slide, "版本", 0.94, 1.62, 1.25, 0.12, { fontSize: 8.0, color: C.blue, bold: true });
  addText(slide, "价格", 2.18, 1.62, 1.30, 0.12, { fontSize: 8.0, color: C.blue, bold: true });
  addText(slide, "交付内容", 3.52, 1.62, 3.10, 0.12, { fontSize: 8.0, color: C.blue, bold: true });
  addText(slide, "付费动机", 6.88, 1.62, 1.65, 0.12, { fontSize: 8.0, color: C.blue, bold: true });
  line(slide, 0.86, 1.90, 9.12, 1.90, C.line, 0.8);
  plans.forEach((p, i) => {
    const y = 2.10 + i * 0.42;
    if (i % 2 === 1) slide.addShape(pptx.ShapeType.rect, { x: 0.84, y: y - 0.05, w: 8.30, h: 0.32, fill: { color: C.panel }, line: { color: C.panel } });
    addText(slide, p[0], 0.94, y, 1.18, 0.10, { fontFace: EN, fontSize: 7.2, color: A[i], bold: true });
    addText(slide, p[1], 2.18, y, 1.22, 0.10, { fontSize: 7.1, color: C.text });
    addText(slide, p[2], 3.52, y, 3.05, 0.10, { fontSize: 7.1, color: C.text });
    addText(slide, p[3], 6.88, y, 1.88, 0.10, { fontSize: 7.1, color: C.text });
  });
  addText(slide, "注：价格为校赛版测算，提交前建议补充真实访谈、试点意向或导师/企业推荐证明。", 1.02, 4.88, 7.96, 0.10, { fontSize: 7.2, color: C.muted, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "16", "实施路线：先把深度闭环跑稳，再扩展场景", "路线图从“证明内核闭环可靠”开始，而不是先堆平台功能。", 18);
  const mile = [
    ["2026.05-06", "深度演示", "敏感路径、异常外联、LSM/cgroup 阻断、证据链回放", C.blue],
    ["2026.06-08", "量化验证", "检测率、阻断率、误报、事件延迟、CPU/内存开销", C.green],
    ["2026.08-10", "高校试点", "课程实验、靶场、实验室试用、导师反馈", C.orange],
    ["2026.10-2027.03", "成果固化", "软著、专利、论文、合作证明、开源社区", C.purple],
    ["2027", "产品化", "Team Pro、企业私有化、教育套件、咨询服务", C.red],
  ];
  line(slide, 1.05, 2.34, 8.80, 2.34, C.line, 2.2);
  mile.forEach((m, i) => {
    const x = 0.66 + i * 1.84;
    slide.addShape(pptx.ShapeType.ellipse, { x: x + 0.62, y: 2.15, w: 0.36, h: 0.36, fill: { color: m[3] }, line: { color: C.white, width: 1 } });
    rect(slide, x, 2.82, 1.48, 1.34, C.white, C.line, { radius: 0.03 });
    addText(slide, m[0], x + 0.11, 3.04, 1.26, 0.10, { fontFace: EN, fontSize: 6.5, color: m[3], bold: true, align: "center" });
    addText(slide, m[1], x + 0.11, 3.34, 1.26, 0.14, { fontSize: 9.2, color: C.ink, bold: true, align: "center" });
    addText(slide, m[2], x + 0.11, 3.66, 1.26, 0.25, { fontSize: 6.3, color: C.text, align: "center", fit: "shrink" });
  });
  splitRow(slide, 0.92, 4.66, 8.15, 0.36, "阶段判断", "如果内核闭环和低误报验证不充分，不急于扩大 UI 功能；先把最难的技术证明做扎实。", C.blue);
}

{
  const slide = pptx.addSlide();
  title(slide, "17", "团队分工与合规边界：围绕深度闭环组织，而不是按页面分工", "提交前将【待填】替换为真实姓名、学号、年级、学院与贡献证据。", 19);
  card(slide, 0.72, 1.38, 2.72, 2.70, "技术主线", "eBPF/LSM/cgroup：【待填】\n上下文继承/执行图谱：【待填】\n语义风险检测：【待填】\n性能与误报验证：【待填】", C.blue, C.white);
  card(slide, 3.64, 1.38, 2.72, 2.70, "应用主线", "高校实验与课程：【待填】\n团队/企业访谈：【待填】\n商业模式与报价：【待填】\n演示视频与材料：【待填】", C.green, C.white);
  card(slide, 6.56, 1.38, 2.72, 2.70, "合规边界", "• 只在本地/授权环境使用\n• 先观察后阻断\n• 不采集未授权数据\n• TLS 明文捕获默认关闭\n• 演示数据必须脱敏", C.red, C.white);
  addText(slide, "团队证明建议：commit 记录、模块负责人截图、导师指导记录、测试报告、软著/论文/专利分工。", 0.96, 4.70, 8.1, 0.10, { fontSize: 7.6, color: C.muted, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "18", "总结：技术深度解决信任问题，应用前景决定项目价值", "用公开数据说明刚需，用内核闭环证明技术深度，用明确场景说明落地前景。", 20);
  card(slide, 0.74, 1.30, 2.62, 1.20, "深度 1：可信事实", "eBPF 采集关键 OS 行为，EventEnvelope 统一语义和事实字段，形成不可只靠 prompt 替代的证据底座。", C.blue, C.white);
  card(slide, 3.68, 1.30, 2.62, 1.20, "深度 2：前置控制", "BPF LSM/cgroup 把策略放到危险动作发生前，证明项目不是事后看板，而是 runtime guardrail。", C.green, C.white);
  card(slide, 6.62, 1.30, 2.62, 1.20, "前景：三类场景", "高校实验室、AI coding 团队、企业私有化分别对应成果转化、团队治理和合规运营。", C.orange, C.white);
  rect(slide, 0.88, 2.94, 8.24, 0.48, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "需要支持：导师/学院资源、试点用户、软著/专利指导。目标：校赛晋级、省赛打磨、形成可转化产品。", 1.10, 3.10, 7.80, 0.10, { fontSize: 8.8, color: C.ink, bold: true, align: "center" });
  rect(slide, 0.88, 3.75, 8.24, 0.70, C.white, C.line, { radius: 0.03 });
  addText(slide, "本 PPT 中统计数据、案例与外部框架已用 [1]–[6] 标注来源；参考文献按 GB/T 7714 常用格式列于下一页。", 1.08, 3.98, 7.84, 0.16, { fontSize: 8.6, color: C.text, align: "center", fit: "shrink" });
  addText(slide, "谢谢各位老师，请批评指正", 3.10, 4.76, 3.8, 0.20, { fontSize: 14.2, bold: true, color: C.blue, align: "center" });
}

{
  const slide = pptx.addSlide();
  title(slide, "REF", "参考文献", "按中国大陆论文常用著录习惯整理；网络资源类型标注为 [EB/OL]。", 21);
  const refs = [
    ["[1]", "Stack Overflow. Stack Overflow Developer Survey 2025: AI tools adoption and usage[EB/OL].", "[2026-05-26]. https://survey.stackoverflow.co/2025/ai", C.blue],
    ["[2]", "GitGuardian. The State of Secrets Sprawl 2026[EB/OL].", "2026 [2026-05-26]. https://blog.gitguardian.com/the-state-of-secrets-sprawl-2026/", C.red],
    ["[3]", "IBM. Cost of a Data Breach Report 2025[EB/OL].", "2025 [2026-05-26]. https://www.ibm.com/reports/data-breach", C.purple],
    ["[4]", "OWASP Foundation. OWASP Top 10 for LLM Applications and Agentic AI Threats[EB/OL].", "2025 [2026-05-26]. https://owasp.org/www-project-top-10-for-large-language-model-applications/", C.green],
    ["[5]", "Cloud Security Alliance. M365 Copilot command injection at scale: security guidance for CVE-2026-24299[EB/OL].", "2026 [2026-05-26]. https://labs.cloudsecurityalliance.org/", C.orange],
    ["[6]", "National Vulnerability Database. CVE-2025-32711 Detail[EB/OL].", "2025 [2026-05-26]. https://nvd.nist.gov/vuln/detail/CVE-2025-32711", C.cyan],
  ];
  refs.forEach((r, i) => {
    const y = 1.25 + i * 0.58;
    rect(slide, 0.72, y, 8.68, 0.43, i % 2 ? C.panel : C.white, C.line, { radius: 0.02 });
    addText(slide, r[0], 0.88, y + 0.14, 0.42, 0.08, { fontFace: EN, fontSize: 7.8, color: r[3], bold: true, align: "center" });
    addText(slide, r[1], 1.36, y + 0.075, 7.62, 0.12, { fontSize: 6.8, color: C.ink, bold: true, fit: "shrink" });
    addText(slide, r[2], 1.36, y + 0.245, 7.70, 0.08, { fontFace: EN, fontSize: 5.7, color: C.muted, fit: "shrink" });
  });
  rect(slide, 0.92, 4.88, 8.12, 0.28, C.panel2, C.line2, { radius: 0.02 });
  addText(slide, "说明：PPT 正文采用顺序编码制标注资料来源；网络文献访问日期统一按本材料生成日期记录。", 1.10, 4.965, 7.78, 0.08, { fontSize: 6.9, color: C.text, align: "center" });
}

pptx.writeFile({ fileName: OUT }).then(() => {
  injectTypewriterAnimations(OUT);
  console.log(OUT);
}).catch((err) => {
  console.error(err);
  process.exit(1);
});
