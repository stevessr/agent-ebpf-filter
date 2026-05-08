const fs = require('fs');
const path = require('path');
const pptxgen = require('pptxgenjs');

const ROOT = path.resolve(__dirname, '..');
const OUT_ASCII = path.join(ROOT, 'proposal-output', 'projectbook-presentation.pptx');
const OUT_CN = path.join(ROOT, 'proposal-output', '项目书公开展演.pptx');

const theme = {
  primary: '0A0A0A',
  secondary: '0070F3',
  accent: 'D4AF37',
  light: 'F5F5F5',
  bg: 'FFFFFF',
};

const colors = {
  ink: theme.primary,
  blue: theme.secondary,
  gold: theme.accent,
  paper: theme.bg,
  soft: theme.light,
  muted: '6B7280',
  line: 'E5E7EB',
  lineDark: 'CBD5E1',
};

const assets = {
  cover: path.join(ROOT, 'proposal-output', 'images', 'cover.png'),
  evaluation: path.join(ROOT, 'proposal-output', 'images', 'evaluation.png'),
  roadmap: path.join(ROOT, 'proposal-output', 'images', 'roadmap.png'),
  generatedPipeline: path.join(ROOT, 'proposal-output', 'images', 'generated', 'pipeline-visual.png'),
  generatedDashboard: path.join(ROOT, 'proposal-output', 'images', 'generated', 'dashboard-visual.png'),
  reportDir: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249'),
  best: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'best.json'),
  overallBest: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'overall_best.svg'),
  overallSpeed: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'overall_speed.svg'),
  stabilityBest: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'stability_best.svg'),
  stabilitySpeed: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'stability_speed.svg'),
  svmLong: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'svm-long.svg'),
  svmInference: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'svm-long-inference.svg'),
  rfDeep: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'random-forest-deep.svg'),
  rfInference: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'random-forest-inference.svg'),
  rfFast: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'random-forest-fast.svg'),
  logisticBalanced: path.join(ROOT, 'reports', 'ml-sweep-20260506-160249', 'logistic-balanced.svg'),
};

const report = JSON.parse(fs.readFileSync(assets.best, 'utf8'));
const best = report.best || {};
const stableBest = report.stableBest || {};
const screenBest = report.screenBest || {};

function ensureDir(filePath) {
  fs.mkdirSync(path.dirname(filePath), { recursive: true });
}

function fmtPct(value) {
  return `${(value * 100).toFixed(2)}%`;
}

function fmtK(value) {
  if (!Number.isFinite(value)) return '-';
  if (Math.abs(value) >= 1e6) return `${(value / 1e6).toFixed(2)}M/s`;
  if (Math.abs(value) >= 1e3) return `${(value / 1e3).toFixed(2)}k/s`;
  return `${value.toFixed(0)}/s`;
}

function pageBadge(pres, slide, num) {
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 9.1, y: 5.12, w: 0.6, h: 0.34,
    rectRadius: 0.17,
    fill: { color: colors.gold },
    line: { color: colors.gold, transparency: 100 },
  });
  slide.addText(String(num).padStart(2, '0'), {
    x: 9.1, y: 5.12, w: 0.6, h: 0.34,
    fontFace: 'Arial', fontSize: 11,
    color: 'FFFFFF', bold: true,
    align: 'center', valign: 'middle',
    margin: 0,
  });
}

function addHeader(pres, slide, num, tag, title, subtitle, dark = false) {
  const titleColor = dark ? 'FFFFFF' : colors.ink;
  const subtitleColor = dark ? 'E5E7EB' : colors.muted;
  const tagFill = dark ? colors.gold : colors.blue;
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 0.4, y: 0.33, w: 1.1, h: 0.34,
    rectRadius: 0.17,
    fill: { color: tagFill },
    line: { color: tagFill, transparency: 100 },
  });
  slide.addText(tag, {
    x: 0.4, y: 0.33, w: 1.1, h: 0.34,
    fontFace: 'Arial', fontSize: 11,
    color: 'FFFFFF', bold: true, align: 'center', valign: 'middle', margin: 0,
  });
  slide.addText(title, {
    x: 1.62, y: 0.22, w: 7.6, h: 0.3,
    fontFace: 'Microsoft YaHei', fontSize: 24,
    color: titleColor, bold: true, margin: 0,
  });
  if (subtitle) {
    slide.addText(subtitle, {
      x: 0.42, y: 0.68, w: 8.4, h: 0.34,
      fontFace: 'Microsoft YaHei', fontSize: 11.5,
      color: subtitleColor, margin: 0,
    });
  }
  slide.addShape(pres.shapes.RECTANGLE, {
    x: 0.4, y: 1.0, w: 9.1, h: 0.03,
    fill: { color: dark ? colors.gold : colors.blue },
    line: { color: dark ? colors.gold : colors.blue, transparency: 100 },
  });
  if (num > 1) pageBadge(pres, slide, num);
}

function addCard(pres, slide, x, y, w, h, opts = {}) {
  const fill = opts.fill || colors.soft;
  const line = opts.line || colors.line;
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x, y, w, h,
    rectRadius: opts.radius ?? 0.12,
    fill: { color: fill },
    line: { color: line, transparency: opts.lineTransparency ?? 0.0, width: opts.lineWidth ?? 1 },
    shadow: opts.shadow === false ? undefined : {
      type: 'outer',
      color: '000000',
      blur: 2,
      offset: 1,
      angle: 45,
      opacity: 0.10,
    },
  });
}

function addImageCard(pres, slide, imagePath, x, y, w, h, opts = {}) {
  addCard(pres, slide, x, y, w, h, {
    fill: opts.fill || colors.paper,
    line: opts.line || colors.line,
    radius: opts.radius ?? 0.1,
    shadow: opts.shadow !== false,
  });
  const pad = opts.pad ?? 0.09;
  slide.addImage({
    path: imagePath,
    x: x + pad,
    y: y + pad,
    w: w - pad * 2,
    h: h - pad * 2 - (opts.caption ? 0.25 : 0),
    sizing: { type: 'contain', w: w - pad * 2, h: h - pad * 2 - (opts.caption ? 0.25 : 0) },
    altText: opts.alt || path.basename(imagePath),
  });
  if (opts.caption) {
    slide.addText(opts.caption, {
      x: x + 0.08, y: y + h - 0.23, w: w - 0.16, h: 0.16,
      fontFace: 'Microsoft YaHei', fontSize: 9.5,
      color: colors.muted, align: 'center', margin: 0,
    });
  }
}

function metricCard(pres, slide, x, y, w, h, number, label, accent = colors.blue) {
  addCard(pres, slide, x, y, w, h, { fill: colors.paper, line: colors.line, radius: 0.12 });
  slide.addShape(pres.shapes.RECTANGLE, {
    x: x + 0.08, y: y + 0.08, w: 0.12, h: h - 0.16,
    fill: { color: accent }, line: { color: accent, transparency: 100 },
  });
  slide.addText(number, {
    x: x + 0.24, y: y + 0.14, w: w - 0.32, h: 0.35,
    fontFace: 'Arial', fontSize: 22, color: colors.ink, bold: true, margin: 0,
  });
  slide.addText(label, {
    x: x + 0.24, y: y + 0.48, w: w - 0.32, h: h - 0.56,
    fontFace: 'Microsoft YaHei', fontSize: 10.5, color: colors.muted, margin: 0,
  });
}

function bulletCard(pres, slide, x, y, w, h, title, bullets, accent = colors.blue) {
  addCard(pres, slide, x, y, w, h, { fill: colors.paper, line: colors.line, radius: 0.12 });
  slide.addShape(pres.shapes.RECTANGLE, {
    x: x + 0.12, y: y + 0.14, w: 0.08, h: 0.28,
    fill: { color: accent }, line: { color: accent, transparency: 100 },
  });
  slide.addText(title, {
    x: x + 0.28, y: y + 0.10, w: w - 0.38, h: 0.28,
    fontFace: 'Microsoft YaHei', fontSize: 13.5, bold: true, color: colors.ink, margin: 0,
  });
  slide.addText(bullets.map((b) => `• ${b}`).join('\n'), {
    x: x + 0.18, y: y + 0.44, w: w - 0.36, h: h - 0.52,
    fontFace: 'Microsoft YaHei', fontSize: 10.5, color: colors.ink,
    margin: 0, breakLine: false, fit: 'shrink',
  });
}

function smallPill(pres, slide, x, y, w, text, fill = colors.blue) {
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x, y, w, h: 0.28,
    rectRadius: 0.14, fill: { color: fill }, line: { color: fill, transparency: 100 },
  });
  slide.addText(text, {
    x, y: y + 0.01, w, h: 0.24,
    fontFace: 'Microsoft YaHei', fontSize: 9.5, color: 'FFFFFF',
    bold: true, align: 'center', valign: 'middle', margin: 0,
  });
}

function flowNode(pres, slide, cfg) {
  const {
    x, y, w, h,
    step, title, subtitle,
    fill = colors.paper,
    line = colors.line,
    accent = colors.blue,
    dark = false,
  } = cfg;
  addCard(pres, slide, x, y, w, h, {
    fill,
    line,
    radius: 0.13,
    lineWidth: 1.2,
    shadow: true,
  });
  slide.addShape(pres.shapes.RECTANGLE, {
    x: x + 0.02, y: y + 0.02, w: 0.08, h: h - 0.04,
    fill: { color: accent }, line: { color: accent, transparency: 100 },
  });
  slide.addShape(pres.shapes.OVAL, {
    x: x + 0.12, y: y + 0.12, w: 0.28, h: 0.28,
    fill: { color: accent }, line: { color: accent, transparency: 100 },
  });
  slide.addText(step, {
    x: x + 0.12, y: y + 0.12, w: 0.28, h: 0.28,
    fontFace: 'Arial', fontSize: 9.5, color: 'FFFFFF', bold: true,
    align: 'center', valign: 'middle', margin: 0,
  });
  slide.addText(title, {
    x: x + 0.46, y: y + 0.10, w: w - 0.56, h: 0.24,
    fontFace: 'Microsoft YaHei', fontSize: 11.2, color: dark ? 'FFFFFF' : colors.ink,
    bold: true, margin: 0, fit: 'shrink',
  });
  slide.addText(subtitle, {
    x: x + 0.12, y: y + 0.44, w: w - 0.24, h: h - 0.54,
    fontFace: 'Microsoft YaHei', fontSize: 9.1, color: dark ? 'E5E7EB' : colors.muted,
    margin: 0, fit: 'shrink',
  });
}

function flowArrow(pres, slide, x1, y1, x2, y2, accent = colors.blue) {
  slide.addShape(pres.shapes.LINE, {
    x: x1, y: y1, w: x2 - x1, h: y2 - y1,
    line: {
      color: accent,
      width: 2,
      beginArrowType: 'none',
      endArrowType: 'triangle',
    },
  });
}

function slideCover(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.ink };

  slide.addShape(pres.shapes.RECTANGLE, {
    x: 0, y: 0, w: 10, h: 5.625,
    fill: { color: colors.ink },
    line: { color: colors.ink, transparency: 100 },
  });
  slide.addShape(pres.shapes.RECTANGLE, {
    x: 0.25, y: 0.35, w: 4.65, h: 4.9,
    fill: { color: '111827', transparency: 8 },
    line: { color: '111827', transparency: 100 },
  });
  slide.addShape(pres.shapes.RECTANGLE, {
    x: 5.05, y: 0.35, w: 4.65, h: 4.9,
    fill: { color: colors.paper },
    line: { color: colors.paper, transparency: 100 },
  });
  slide.addShape(pres.shapes.RECTANGLE, {
    x: 0.3, y: 0.3, w: 9.35, h: 0.08,
    fill: { color: colors.gold },
    line: { color: colors.gold, transparency: 100 },
  });

  smallPill(pres, slide, 0.45, 0.55, 0.92, '金种子');
  smallPill(pres, slide, 1.45, 0.55, 0.92, '投稿版', colors.blue);
  smallPill(pres, slide, 2.45, 0.55, 1.08, '公开展演', colors.gold);

  slide.addText('项目书', {
    x: 0.48, y: 1.05, w: 1.8, h: 0.42,
    fontFace: 'Microsoft YaHei', fontSize: 24, color: 'D1D5DB',
    bold: true, margin: 0,
  });
  slide.addText('面向高风险稀缺样本识别的\n轻量级可解释表格数据模型研究与系统实现', {
    x: 0.48, y: 1.48, w: 4.0, h: 1.25,
    fontFace: 'Microsoft YaHei', fontSize: 24, color: 'FFFFFF',
    bold: true, margin: 0, breakLine: false, fit: 'shrink',
  });
  slide.addText('围绕 ALLOW 放行率、少数类召回、误伤率、训练耗时与推理速度，\n构建可投稿、可演示、可持续扩展的研究与系统闭环。', {
    x: 0.48, y: 2.78, w: 4.15, h: 0.75,
    fontFace: 'Microsoft YaHei', fontSize: 12.5, color: 'E5E7EB', margin: 0,
  });

  metricCard(pres, slide, 0.48, 3.68, 1.16, 0.92, '949', '条样本', colors.gold);
  metricCard(pres, slide, 1.74, 3.68, 1.2, 0.92, '31', '个模型族', colors.blue);
  metricCard(pres, slide, 3.04, 3.68, 1.32, 0.92, '10x', '稳定复核', colors.gold);

  slide.addText('2026 年项目书 + 展演版', {
    x: 0.48, y: 4.94, w: 2.25, h: 0.22,
    fontFace: 'Arial', fontSize: 10.5, color: 'C7CDD7', margin: 0,
  });

  addImageCard(pres, slide, assets.cover, 5.35, 0.55, 3.95, 4.25, {
    fill: colors.paper,
    caption: '封面图 / 研究主题视觉',
    alt: 'cover image',
  });
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 6.1, y: 3.9, w: 2.55, h: 0.62,
    rectRadius: 0.12,
    fill: { color: colors.ink, transparency: 10 },
    line: { color: colors.gold, width: 1 },
  });
  slide.addText('图文并茂 · 可直接投屏展示', {
    x: 6.1, y: 4.06, w: 2.55, h: 0.18,
    fontFace: 'Microsoft YaHei', fontSize: 10.5,
    color: 'FFFFFF', bold: true, align: 'center', margin: 0,
  });

  return slide;
}

function slideAgenda(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 2, '01', '目录 / 展演路线', '用 5 个部分把“问题—方法—结果—系统—产出”讲清楚');

  const items = [
    ['01', '为什么做', '高风险稀缺样本的现实痛点'],
    ['02', '怎么做', '数据清洗、训练 pipeline 与模型闭环'],
    ['03', '结果怎样', '稳定性复核与综合最优模型'],
    ['04', '系统是什么', '原型系统与前端展示能力'],
    ['05', '怎么展示', '投稿材料包与公开展演提纲'],
  ];

  items.forEach((item, i) => {
    const rowY = 1.35 + i * 0.65;
    addCard(pres, slide, 0.48, rowY, 4.65, 0.52, { fill: i % 2 === 0 ? colors.paper : 'FAFAFA', line: colors.line, radius: 0.10, shadow: false });
    slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
      x: 0.62, y: rowY + 0.11, w: 0.32, h: 0.32,
      rectRadius: 0.08,
      fill: { color: i % 2 === 0 ? colors.blue : colors.gold },
      line: { color: i % 2 === 0 ? colors.blue : colors.gold, transparency: 100 },
    });
    slide.addText(item[0], {
      x: 0.62, y: rowY + 0.11, w: 0.32, h: 0.32,
      fontFace: 'Arial', fontSize: 9.5, color: 'FFFFFF', bold: true, align: 'center', valign: 'middle', margin: 0,
    });
    slide.addText(item[1], {
      x: 1.05, y: rowY + 0.07, w: 1.5, h: 0.17,
      fontFace: 'Microsoft YaHei', fontSize: 12.5, bold: true, color: colors.ink, margin: 0,
    });
    slide.addText(item[2], {
      x: 1.05, y: rowY + 0.27, w: 3.55, h: 0.14,
      fontFace: 'Microsoft YaHei', fontSize: 10.2, color: colors.muted, margin: 0,
    });
  });

  addImageCard(pres, slide, assets.cover, 5.45, 1.28, 2.05, 1.55, { caption: '封面图', fill: colors.paper });
  addImageCard(pres, slide, assets.evaluation, 7.58, 1.28, 1.95, 1.55, { caption: '评价图', fill: colors.paper });
  addImageCard(pres, slide, assets.roadmap, 5.45, 2.98, 4.08, 1.72, { caption: '路线图', fill: colors.paper });
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 5.45, y: 1.28, w: 4.08, h: 1.55,
    rectRadius: 0.1,
    fill: { color: 'FFFFFF', transparency: 100 },
    line: { color: colors.line, transparency: 100 },
  });

  return slide;
}

function slideProblem(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 3, '02', '为什么要做这个项目', '不是只追求“识别出来”，还要保证“该放行的命令真的放行”');

  bulletCard(pres, slide, 0.48, 1.34, 2.85, 1.28, '痛点 1：高风险样本稀缺', [
    '真正危险的命令/样本少，但漏判代价高。',
    '只看 accuracy 容易掩盖少数类召回问题。',
  ], colors.gold);
  bulletCard(pres, slide, 0.48, 2.78, 2.85, 1.28, '痛点 2：正常命令会被误伤', [
    '工具调用、运维审批和自动化链路不能被过度拦截。',
    'ALLOW 放行率直接影响系统可用性。',
  ], colors.blue);
  bulletCard(pres, slide, 0.48, 4.22, 2.85, 0.82, '痛点 3：单次结果不可靠', [
    '需要重复复核、速度评估和稳定性统计。',
  ], colors.gold);

  addImageCard(pres, slide, assets.evaluation, 3.54, 1.34, 6.0, 3.70, {
    fill: colors.paper,
    caption: '评价视图：不仅看准确率，也看放行率与速度',
  });
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 3.82, y: 4.77, w: 5.38, h: 0.52,
    rectRadius: 0.14,
    fill: { color: colors.ink, transparency: 6 },
    line: { color: colors.blue, width: 1 },
  });
  slide.addText('项目的核心价值：把“安全”和“可用”放在同一套评价体系里。', {
    x: 4.02, y: 4.91, w: 4.95, h: 0.16,
    fontFace: 'Microsoft YaHei', fontSize: 11.5, color: 'FFFFFF', bold: true, align: 'center', margin: 0,
  });

  return slide;
}

function slidePipeline(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 4, '03', '数据—模型—反馈闭环', '从数据导入到误判回流，再到模型复训，形成可持续迭代的项目闭环');

  const steps = [
    ['1', '数据导入', '公开数据 / 本地文件 / 归档展开'],
    ['2', '清洗脱敏', '去重、标签模式、敏感字段屏蔽'],
    ['3', '模型训练', '多模型 sweep + 稳定性复核'],
    ['4', '结果展示', '图表、报告、HTML 演示页'],
    ['5', '误判回流', '人工复核后再训练'],
  ];

  steps.forEach((s, i) => {
    const x = 0.5 + i * 1.9;
    const fill = i % 2 === 0 ? colors.paper : 'FAFAFA';
    addCard(pres, slide, x, 1.38, 1.65, 1.28, { fill, line: colors.line, radius: 0.12, shadow: false });
    slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
      x: x + 0.18, y: 1.55, w: 0.36, h: 0.36,
      rectRadius: 0.1,
      fill: { color: i < 2 ? colors.blue : i < 4 ? colors.gold : colors.ink },
      line: { color: i < 2 ? colors.blue : i < 4 ? colors.gold : colors.ink, transparency: 100 },
    });
    slide.addText(s[0], {
      x: x + 0.18, y: 1.55, w: 0.36, h: 0.36,
      fontFace: 'Arial', fontSize: 10, color: 'FFFFFF', bold: true, align: 'center', valign: 'middle', margin: 0,
    });
    slide.addText(s[1], {
      x: x + 0.6, y: 1.50, w: 0.9, h: 0.18,
      fontFace: 'Microsoft YaHei', fontSize: 11.5, bold: true, color: colors.ink, margin: 0,
    });
    slide.addText(s[2], {
      x: x + 0.18, y: 1.96, w: 1.25, h: 0.42,
      fontFace: 'Microsoft YaHei', fontSize: 9.8, color: colors.muted, margin: 0, fit: 'shrink',
    });
  });

  // connectors
  for (let i = 0; i < 4; i++) {
    slide.addShape(pres.shapes.LINE, {
      x: 2.16 + i * 1.9, y: 2.0, w: 0.25, h: 0,
      line: { color: colors.blue, width: 2, beginArrowType: 'none', endArrowType: 'triangle' },
    });
  }

  addImageCard(pres, slide, assets.roadmap, 0.5, 2.96, 5.2, 2.08, { caption: '路线图：当前项目已经形成从问题定义到闭环迭代的完整链路' });
  bulletCard(pres, slide, 5.92, 2.96, 3.58, 2.08, '闭环内核', [
    '公开数据、合成样本、已有缓存样本共同构成训练集。',
    '脱敏与去重确保样本可直接用于训练和展示。',
    '实验结果会回流到报告、图表和前端页面。',
  ], colors.gold);

  return slide;
}

function slideTrainingFlow(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 5, '03', '训练 pipeline 流程图', '把“输入—处理—训练—评估—输出—回流”一次画清楚');

  const nodeY = 1.42;
  const nodeH = 1.18;
  const nodeW = 1.1;
  const xs = [0.56, 1.81, 3.06, 4.31, 5.56, 6.81, 8.06];
  const titles = [
    '数据源',
    '导入展开',
    '清洗脱敏',
    '去重标注',
    '训练集构建',
    '多模型 sweep',
    '稳定复核',
  ];
  const subtitles = [
    '公开数据、实验样本、历史归档',
    'URL / 文件 / 压缩包统一接入',
    '敏感字段屏蔽，统一字段格式',
    '批内与存量去重，保留来源标记',
    '特征矩阵与标签分离，形成可训集',
    'SVM / RF / 线性模型并行比较',
    '10x 重复复核，筛选稳定最优',
  ];
  const fills = ['F8FBFF', 'FFFDF6', 'F8FBFF', 'FFFDF6', 'F8FBFF', 'FFFDF6', '0A0A0A'];
  const accents = [colors.blue, colors.gold, colors.blue, colors.gold, colors.blue, colors.gold, colors.gold];
  const darkFlags = [false, false, false, false, false, false, true];

  xs.forEach((x, i) => {
    flowNode(pres, slide, {
      x,
      y: nodeY,
      w: nodeW,
      h: nodeH,
      step: String(i + 1),
      title: titles[i],
      subtitle: subtitles[i],
      fill: fills[i],
      line: i === 6 ? colors.gold : colors.line,
      accent: accents[i],
      dark: darkFlags[i],
    });
    if (i < xs.length - 1) {
      flowArrow(
        pres,
        slide,
        x + nodeW + 0.03,
        nodeY + nodeH / 2,
        xs[i + 1] - 0.05,
        nodeY + nodeH / 2,
        i % 2 === 0 ? colors.blue : colors.gold,
      );
    }
  });

  slide.addText('反馈回流：人工复核后的误判样本会重新进入训练集，下一轮 sweep 再次参与比较。', {
    x: 0.58, y: 2.78, w: 8.78, h: 0.18,
    fontFace: 'Microsoft YaHei', fontSize: 10.8, color: colors.ink, align: 'center', margin: 0,
  });
  slide.addShape(pres.shapes.RECTANGLE, {
    x: 0.86, y: 3.06, w: 8.28, h: 0.03,
    fill: { color: colors.blue },
    line: { color: colors.blue, transparency: 100 },
  });
  slide.addShape(pres.shapes.LINE, {
    x: 1.08, y: 3.07, w: 7.07, h: 0,
    line: {
      color: colors.gold,
      width: 2,
      dashType: 'dash',
      beginArrowType: 'triangle',
      endArrowType: 'none',
    },
  });
  slide.addText('回流 / 再训练', {
    x: 1.22, y: 2.90, w: 1.2, h: 0.16,
    fontFace: 'Microsoft YaHei', fontSize: 9.6, color: colors.gold, bold: true, margin: 0,
  });
  slide.addText('导入后的样本不会被简单丢弃，而是回到训练闭环中继续迭代。', {
    x: 6.02, y: 2.90, w: 2.6, h: 0.16,
    fontFace: 'Microsoft YaHei', fontSize: 9.6, color: colors.muted, align: 'right', margin: 0,
  });

  const cardY = 3.44;
  const cardH = 1.55;
  const cardW = 2.92;
  const gaps = 0.17;
  const cardXs = [0.48, 0.48 + cardW + gaps, 0.48 + (cardW + gaps) * 2];

  bulletCard(pres, slide, cardXs[0], cardY, cardW, cardH, '输入约束', [
    '支持 URL、文件与压缩包导入。',
    '导入后自动脱敏，避免敏感字段泄漏。',
    '保留来源标签，便于追踪与复核。',
  ], colors.blue);

  bulletCard(pres, slide, cardXs[1], cardY, cardW, cardH, '训练策略', [
    'SVM / Random Forest / 线性模型并行对比。',
    '重复复核 10 次，强调稳定性而非偶然最优。',
    '同时关注准确率、ALLOW 放行率与速度。',
  ], colors.gold);

  bulletCard(pres, slide, cardXs[2], cardY, cardW, cardH, '输出形态', [
    'best.json 记录稳定结果。',
    'SVG 图表与 HTML 报告可直接投屏。',
    'PPTX 与文稿同步输出，方便展示。',
  ], colors.ink);

  return slide;
}

function slideGeneratedVisuals(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 6, '04', '生图视觉稿 / 训练与展示', '把项目流程和系统效果用更直观的视觉方式补强');

  addImageCard(pres, slide, assets.generatedPipeline, 0.48, 1.36, 4.46, 2.92, {
    caption: '生图 1：数据接入—训练—反馈闭环',
    fill: colors.paper,
    line: colors.line,
    pad: 0.08,
  });
  addImageCard(pres, slide, assets.generatedDashboard, 5.06, 1.36, 4.46, 2.92, {
    caption: '生图 2：系统看板—评估—输出—回流',
    fill: colors.paper,
    line: colors.line,
    pad: 0.08,
  });

  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 0.58, y: 4.62, w: 8.84, h: 0.34,
    rectRadius: 0.14,
    fill: { color: colors.ink, transparency: 4 },
    line: { color: colors.gold, width: 1 },
  });
  slide.addText('这些图像由生图功能生成，用作项目书 / PPT 的视觉增强页，突出“训练流程”和“系统展示”两个核心场景。', {
    x: 0.72, y: 4.71, w: 8.56, h: 0.18,
    fontFace: 'Microsoft YaHei', fontSize: 10.3, color: 'FFFFFF', align: 'center', margin: 0,
  });

  return slide;
}

function slideExperimentOverview(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 7, '05', '实验规模与综合结果', '当前数据集与 sweep 已达到可以公开展示的完整度');

  metricCard(pres, slide, 0.48, 1.34, 1.35, 0.88, '949', '条样本', colors.blue);
  metricCard(pres, slide, 1.92, 1.34, 1.35, 0.88, '31', '个模型族', colors.gold);
  metricCard(pres, slide, 3.36, 1.34, 1.50, 0.88, '106', '个代表变体', colors.blue);
  metricCard(pres, slide, 4.96, 1.34, 1.32, 0.88, '10x', '稳定复核', colors.gold);
  metricCard(pres, slide, 6.38, 1.34, 1.45, 0.88, '100%', '最佳验证均值', colors.blue);
  metricCard(pres, slide, 7.93, 1.34, 1.45, 0.88, '813.77k/s', '稳定推理速度', colors.gold);

  bulletCard(pres, slide, 0.48, 2.42, 2.95, 2.48, '当前稳定最优', [
    `模型：${stableBest.profile || 'svm_long'}`,
    `配置：${stableBest.configSummary || 'lr=0.100 iter=8000'}`,
    `验证均值：${fmtPct(stableBest.validationMean ?? 1)}`,
    `ALLOW：${fmtPct(stableBest.allowMean ?? 1)}`,
    `平均吞吐：${fmtK(stableBest.inferenceMean ?? 813774.58)}`,
  ], colors.blue);

  addImageCard(pres, slide, assets.overallBest, 3.62, 2.42, 6.0, 1.78, { caption: '整体准确率 / 放行率对比', fill: colors.paper });
  addImageCard(pres, slide, assets.overallSpeed, 3.62, 4.30, 6.0, 1.04, { caption: '整体推理吞吐对比', fill: colors.paper });

  return slide;
}

function slideStability(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 8, '06', '稳定性复核', '这不是单次偶然最好，而是经过重复复核仍然稳定的结果');

  bulletCard(pres, slide, 0.48, 1.34, 2.86, 2.2, '稳定性结论', [
    `10 次重复：${report.repeats || 10} 次`,
    `稳定最优：${stableBest.profile || 'svm_long'}`,
    `均值 / 方差：${fmtPct(stableBest.validationMean ?? 1)} ± ${fmtPct(stableBest.validationStd ?? 0)}`,
    `ALLOW：${fmtPct(stableBest.allowMean ?? 1)} ± ${fmtPct(stableBest.allowStd ?? 0)}`,
    `成功率：${fmtPct(stableBest.successRate ?? 1)}`,
  ], colors.gold);

  addImageCard(pres, slide, assets.stabilityBest, 3.58, 1.34, 5.95, 2.06, { caption: '稳定性均值与方差', fill: colors.paper });
  addImageCard(pres, slide, assets.stabilitySpeed, 3.58, 3.56, 5.95, 1.66, { caption: '稳定性下的吞吐 / 延迟', fill: colors.paper });

  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 0.48, y: 3.82, w: 2.86, h: 1.12,
    rectRadius: 0.12,
    fill: { color: colors.ink, transparency: 4 },
    line: { color: colors.blue, width: 1 },
  });
  slide.addText('可公开展示的关键说法：\n“当前综合最优模型已经通过重复复核，而不是只赢了一次。”', {
    x: 0.66, y: 4.05, w: 2.5, h: 0.6,
    fontFace: 'Microsoft YaHei', fontSize: 11.2, color: colors.ink, margin: 0,
  });

  return slide;
}

function slideDeepDive(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 9, '07', '模型家族深挖', 'SVM / Random Forest / 线性模型各有长处，但综合最优和展示价值并不相同');

  addImageCard(pres, slide, assets.svmLong, 0.48, 1.38, 5.05, 3.48, { caption: 'SVM Long：稳定综合最优', fill: colors.paper });
  addImageCard(pres, slide, assets.rfDeep, 5.68, 1.38, 3.82, 1.62, { caption: 'Random Forest Deep：单次性能强', fill: colors.paper });
  addImageCard(pres, slide, assets.rfInference, 5.68, 3.16, 3.82, 1.62, { caption: 'Random Forest 推理吞吐', fill: colors.paper });

  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 0.72, y: 4.98, w: 8.52, h: 0.4,
    rectRadius: 0.12,
    fill: { color: colors.paper, transparency: 0 },
    line: { color: colors.lineDark, width: 1 },
  });
  slide.addText('综合展示口径：SVM Long 在稳定性、验证均值和放行率上更适合答辩主讲；Random Forest 更适合展示“单次峰值速度”对照。', {
    x: 0.86, y: 5.08, w: 8.2, h: 0.16,
    fontFace: 'Microsoft YaHei', fontSize: 10.7, color: colors.ink, align: 'center', margin: 0,
  });

  return slide;
}

function slideSystem(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 10, '08', '系统原型与前端展示', '项目已经不是纯文稿，而是一个能演示、能导入、能训练的完整原型');

  bulletCard(pres, slide, 0.48, 1.34, 3.2, 3.88, '当前原型能力', [
    '后端支持数据集 pull / import / export / clear。',
    '导入时自动去重、脱敏并保留来源信息。',
    '前端支持训练样本浏览、批量打分与参数调整。',
    '已有 PPTX 风格 HTML 报告页可直接投屏。',
    '可以在答辩现场解释“数据—模型—回流”的闭环。',
  ], colors.blue);

  addImageCard(pres, slide, assets.cover, 3.92, 1.34, 2.1, 1.55, { caption: '封面图', fill: colors.paper });
  addImageCard(pres, slide, assets.evaluation, 6.16, 1.34, 3.36, 1.55, { caption: '评价图', fill: colors.paper });
  addImageCard(pres, slide, assets.roadmap, 3.92, 3.04, 5.6, 2.18, { caption: '路线图：用于解释系统如何从数据流转到展示', fill: colors.paper });

  return slide;
}

function slideMaterials(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.bg };
  addHeader(pres, slide, 11, '09', '投稿与公开展演材料包', '把“提交什么、展示什么、证明什么”一次性整理清楚');

  const cards = [
    {
      title: '投稿主文稿',
      img: assets.cover,
      bullets: ['proposal-output/01_重点课题版.md', '完整申报书主文稿', '适合投前最终修改'],
      accent: colors.blue,
    },
    {
      title: '短版摘要',
      img: assets.evaluation,
      bullets: ['proposal-output/02_短版.md', '适合摘要填报 / 快速审阅', '1 分钟讲述版'],
      accent: colors.gold,
    },
    {
      title: '展演与支撑',
      img: assets.roadmap,
      bullets: ['proposal-output/05_公开展演提纲.md', 'docs/ml-benchmark-presentation.html', 'reports/ml-sweep-20260506-160249/'],
      accent: colors.blue,
    },
  ];
  cards.forEach((c, i) => {
    const x = 0.5 + i * 3.15;
    addCard(pres, slide, x, 1.40, 2.85, 3.92, { fill: colors.paper, line: colors.line, radius: 0.12 });
    slide.addShape(pres.shapes.RECTANGLE, {
      x: x + 0.14, y: 1.56, w: 0.08, h: 0.28,
      fill: { color: c.accent }, line: { color: c.accent, transparency: 100 },
    });
    slide.addText(c.title, {
      x: x + 0.28, y: 1.50, w: 1.9, h: 0.2,
      fontFace: 'Microsoft YaHei', fontSize: 12.5, bold: true, color: colors.ink, margin: 0,
    });
    slide.addImage({
      path: c.img,
      x: x + 0.18, y: 1.88, w: 2.49, h: 1.4,
      sizing: { type: 'contain', w: 2.49, h: 1.4 },
      altText: c.title,
    });
    slide.addText(c.bullets.map((b) => `• ${b}`).join('\n'), {
      x: x + 0.18, y: 3.36, w: 2.45, h: 1.2,
      fontFace: 'Microsoft YaHei', fontSize: 10.2, color: colors.ink, margin: 0,
      fit: 'shrink',
    });
  });

  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 0.56, y: 5.08, w: 8.88, h: 0.24,
    rectRadius: 0.12,
    fill: { color: colors.blue, transparency: 15 },
    line: { color: colors.blue, transparency: 100 },
  });
  slide.addText('展示目标：让评审一眼看出“问题真实、指标完整、结果稳定、材料齐备”。', {
    x: 0.6, y: 5.12, w: 8.8, h: 0.13,
    fontFace: 'Microsoft YaHei', fontSize: 10.8, color: colors.ink, align: 'center', margin: 0,
  });

  return slide;
}

function slideClosing(pres) {
  const slide = pres.addSlide();
  slide.background = { color: colors.ink };
  addHeader(pres, slide, 12, '10', '收束与结论', '这套材料已经可以用于投稿，也可以用于公开展演', true);

  slide.addText('项目书已具备\n投稿 + 展演 条件', {
    x: 0.52, y: 1.46, w: 3.6, h: 1.1,
    fontFace: 'Microsoft YaHei', fontSize: 28, color: 'FFFFFF', bold: true, margin: 0,
  });
  slide.addText('1. 问题真实：高风险样本少，但漏判和误伤都代价高。\n2. 指标完整：不是只看 accuracy，而是看放行率、速度和稳定性。\n3. 材料齐备：文稿、短版、调研、图表、展演提纲和 PPTX 都已准备。', {
    x: 0.56, y: 2.78, w: 4.05, h: 1.4,
    fontFace: 'Microsoft YaHei', fontSize: 13, color: 'E5E7EB', margin: 0,
  });

  metricCard(pres, slide, 0.58, 4.35, 1.38, 0.9, '100%', '验证均值', colors.gold);
  metricCard(pres, slide, 2.08, 4.35, 1.50, 0.9, '100%', 'ALLOW 放行率', colors.blue);
  metricCard(pres, slide, 3.72, 4.35, 1.52, 0.9, '813.77k/s', '稳定吞吐', colors.gold);

  addImageCard(pres, slide, assets.roadmap, 5.18, 1.18, 4.2, 3.9, {
    fill: '111827',
    line: colors.gold,
    caption: '继续迭代：补充样本、复核、再训练、再展示',
    shadow: false,
  });
  slide.addShape(pres.shapes.ROUNDED_RECTANGLE, {
    x: 5.42, y: 4.7, w: 3.72, h: 0.46,
    rectRadius: 0.12,
    fill: { color: colors.gold },
    line: { color: colors.gold, transparency: 100 },
  });
  slide.addText('可以直接用于路演、答辩和公开展演', {
    x: 5.42, y: 4.82, w: 3.72, h: 0.15,
    fontFace: 'Microsoft YaHei', fontSize: 11.4, bold: true, color: 'FFFFFF',
    align: 'center', margin: 0,
  });

  slide.addText('谢谢！', {
    x: 0.52, y: 5.18, w: 1.2, h: 0.2,
    fontFace: 'Microsoft YaHei', fontSize: 14, color: 'FFFFFF', bold: true, margin: 0,
  });

  return slide;
}

async function main() {
  const pptx = new pptxgen();
  pptx.layout = 'LAYOUT_16x9';
  pptx.author = 'Codex';
  pptx.company = 'OpenAI';
  pptx.subject = '金种子项目书与公开展演 PPTX';
  pptx.title = '面向高风险稀缺样本识别的轻量级可解释表格数据模型研究与系统实现';
  pptx.lang = 'zh-CN';
  pptx.theme = {
    headFontFace: 'Microsoft YaHei',
    bodyFontFace: 'Microsoft YaHei',
    lang: 'zh-CN',
  };

  slideCover(pptx);
  slideAgenda(pptx);
  slideProblem(pptx);
  slidePipeline(pptx);
  slideTrainingFlow(pptx);
  slideGeneratedVisuals(pptx);
  slideExperimentOverview(pptx);
  slideStability(pptx);
  slideDeepDive(pptx);
  slideSystem(pptx);
  slideMaterials(pptx);
  slideClosing(pptx);

  ensureDir(OUT_ASCII);
  await pptx.writeFile({ fileName: OUT_ASCII });
  fs.copyFileSync(OUT_ASCII, OUT_CN);
  console.log(`Wrote ${OUT_ASCII}`);
  console.log(`Copied ${OUT_CN}`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
