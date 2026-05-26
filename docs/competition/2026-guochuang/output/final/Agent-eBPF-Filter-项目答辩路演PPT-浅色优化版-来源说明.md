# Agent-eBPF-Filter 项目答辩路演 PPT（浅色优化版）来源说明

生成日期：2026-05-26

## 输出文件

- `Agent-eBPF-Filter-项目答辩路演PPT-浅色优化版.pptx`
- `Agent-eBPF-Filter-项目答辩路演PPT-浅色优化版.pdf`
- 生成脚本：`../generate_light_roadshow_ppt.js`
- 网络图片/报告素材目录：`../ppt_assets_light/`
- 静态预览图目录：`../validation/light-ppt-preview-template/`

## 线上模板参考

本版没有直接套用下载模板文件，而是参考公开模板站中“大学生创新创业 / 科技风 / 商业路演 / 浅色答辩”的常见版式语言，重新用 PptxGenJS 生成：

| 参考方向 | 页面/模板 | 采用到本 PPT 的版式特征 |
|---|---|---|
| 大学生科技创新创业答辩 | 天天素材库《大学生科技创新创业项目答辩汇报PPT模板》 | 高校答辩结构、项目背景—方案—落地的叙事节奏、蓝色科技商务基调 |
| 浅色科技创新创业 | 第一PPT《以科技为引擎助力乡村振兴》类浅绿/浅蓝科技模板 | 浅色底、蓝绿辅助色、少量科技色带而非大面积渐变背景 |
| 商业计划书/创业路演 | iSlide 蓝色科技风商业计划书、蓝色商务风创业路演模板 | 封面右栏指标、数据页指标卡、商业模式表格、路线图 |
| 创业大赛路演结构 | 路演制作逻辑线资料 | 痛点、产品服务、技术概况、运营模式、未来展望等内容顺序 |

## 关键外部数据与案例

| PPT 位置 | 用途 | 数据/案例 | 来源 |
|---|---|---|---|
| Slide 1/3/14/18 | AI 开发工具采用率 | 84% 受访者正在使用或计划使用 AI 开发工具；专业开发者日用比例 51% | Stack Overflow Developer Survey 2025: https://survey.stackoverflow.co/2025/ai |
| Slide 1/3/4/14/18 | 密钥泄露规模、AI-assisted commit 风险、MCP 配置密钥 | 2025 年 public GitHub 新增 28.65M 硬编码密钥，YoY +34%；public commits 约 1.94B，YoY +43%；AI service secrets 1,275,105，YoY +81%；Claude Code-assisted commits 泄露率 3.2% vs 全站 1.5%；MCP 配置中 24,008 个唯一密钥、2,117 个有效 | GitGuardian State of Secrets Sprawl 2026: https://blog.gitguardian.com/the-state-of-secrets-sprawl-2026/ |
| Slide 3/14/18 | 数据泄露成本与 AI 治理缺口 | 全球平均数据泄露成本约 $4.44M；97% 报告 AI 相关安全事件的组织缺少适当 AI 访问控制；63% 缺少 AI governance policy；shadow AI 相关 breach 增加约 $670k 成本 | IBM Cost of a Data Breach Report 2025: https://www.ibm.com/reports/data-breach and IBM newsroom release: https://newsroom.ibm.com/2025-07-30-ibm-report-13-of-organizations-reported-breaches-of-ai-models-or-applications%2C-97-of-which-reported-lacking-proper-ai-access-controls |
| Slide 4/18 | Copilot / EchoLeak 真实案例 | M365 Copilot 相关信息泄露链；EchoLeak CVE-2025-32711 CVSS 9.3；2026 CSA 报告将多起披露归纳为跨 12 个月的系统性边界问题 | CSA research note: https://labs.cloudsecurityalliance.org/wp-content/uploads/2026/05/CSA_research_note_M365_Copilot_CVE_2026_24299_20260505-csa-styled.pdf ; NVD: https://nvd.nist.gov/vuln/detail/CVE-2025-32711 |
| Slide 4/5/18 | LLM / Agentic 风险框架 | OWASP Top 10 for LLM Applications / Agentic AI Threats，覆盖 excessive agency、tool misuse、untraceability 等 | OWASP GenAI Security Project: https://owasp.org/www-project-top-10-for-large-language-model-applications/ ; OWASP Agentic AI Threats PDF: https://genai.owasp.org/download/45674/?tmstv=1739819891 |
| Slide 4/6/18 | MCP Tool Poisoning 案例机制 | 恶意 MCP server/tool response 将隐藏指令注入 LLM context，导致 restricted tool 调用、数据泄露或绕过 system prompt | OWASP MCP Tool Poisoning: https://owasp.org/www-community/attacks/MCP_Tool_Poisoning |

## 下载/生成的网络图片素材

- `stackoverflow_ai_usage.png` — Stack Overflow 2025 AI tools usage chart
- `gitguardian_state_2026.png` — GitGuardian State of Secrets Sprawl 2026 hero image
- `gitguardian_commits_developers.png` — public commits/developer growth chart
- `gitguardian_ai_detectors.png` — AI detector growth chart
- `gitguardian_mcp_valid_secrets.png` — MCP valid unique secrets chart
- `gitguardian_secrets_per_1k_commits.png` — secrets per 1000 commits chart
- `csa_copilot_2026.pdf` and rendered thumbnails — CSA Copilot CVE-2026-24299 research note
- `owasp_llm_top10_2025.pdf` and rendered thumbnails — OWASP LLM Applications & Generative AI Top 10
- `owasp_agentic_threats.pdf` and rendered thumbnails — OWASP Agentic AI Threats and Mitigations

## 本轮去 AIGC 化设计调整

- 删除上一版大面积彩色抽象背景、半透明圆形、网格粒子、弧线装饰，避免“AI 自动生成科技背景”观感。
- 删除脚本中的 OpenXML 动画注入，减少播放兼容风险，也避免对象逐个飞入带来的模板感过强问题。
- 统一为白底 + 顶部蓝绿细色带 + 淡灰分区，接近高校答辩和商务路演模板的静态版式。
- 保留少量蓝、绿、橙、红、紫作为语义强调色，只用于指标、风险、技术层级和表格标识。
- 将封面改为左侧项目定位、右侧指标栏；将商业模式改为表格页；将风险链和路线图改为低装饰流程页。
- 保留真实报告截图和公开数据，降低纯图标/概念图比例，便于答辩时解释“依据从哪里来”。

## 验收记录

- 已用 `node ../generate_light_roadshow_ppt.js` 重新生成 PPTX。
- 已用 LibreOffice headless 导出 PDF。
- 已用 `pdftoppm` 生成 18 张 PNG 静态预览图到 `../validation/light-ppt-preview-template/`。
- 抽查 Slide 1、3、4、6：版式已从彩色抽象背景转为白底商务科技模板风格，数据/案例图片正常显示。
