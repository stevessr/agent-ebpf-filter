# 材料验收记录

验收时间：2026-05-25

## 本轮深化内容

- 已完成网上深度调研，并把 OWASP LLM06:2025、Five Eyes/CISA/NSA/NCSC Agentic AI 指南、GitGuardian 2026 secrets sprawl、NIST AI RMF / AI 600-1 纳入材料。
- 已按用户痛点重写表达：从泛化“AI 安全”收敛为 **Agent 意图不可见、子进程链路不可见、阻断效果不可见**。
- 已查看仓库代码实现并形成代码证据链：`backend/ebpf/lsm_enforcer.c`、`backend/ebpf/cgroup_sandbox.c`、`backend/event_context.go`、`backend/semantic_alerts.go`、`backend/event_envelope.go`、`backend/execution_graph.go`、前端 Security/Plugins 相关实现。
- 已新增独立支撑材料：`Agent-eBPF-Filter-深度调研与代码实现映射.md/.docx/.pdf`。

## 生成物

- 申报书：DOCX + PDF
- 商业企划书：DOCX + PDF
- 项目答辩/路演PPT：PPTX + PDF，共 18 页
- 在线填报字段草稿：DOCX + PDF + Markdown
- 深度调研与代码实现映射：Markdown + DOCX + PDF
- 规则核验与提交清单：Markdown
- 提交说明：Markdown
- 负责人信息采集表与 JSON 模板：Markdown + JSON
- 一键填充脚本：`scripts/fill_guochuang2026_materials.py`
- 负责人待填提交包：ZIP
- 最终版填充脚本输出包：`output/final/Agent-eBPF-Filter-国创赛2026最终提交包.zip`

## 校验结果

- DOCX/PPTX：已通过 `unzip -t` 压缩包结构校验。
- PPT：解析 `ppt/slides/slide*.xml`，确认 18 页。
- PPT 关键词：已确认包含“三个不可见”、OWASP、`lsm_enforcer`、`cgroup_sandbox`、`semantic_alerts`、“看得见、说得清、拦得住”。
- PDF：已通过 LibreOffice headless 从 DOCX/PPTX 导出。
- ZIP：负责人待填包与最终包均已通过 `unzip -t` 校验，无压缩数据错误。
- 内容关键词：申报书、商业企划书、PPT、在线填报草稿和深度调研文档均覆盖外部调研、BPF LSM、cgroup、进程树归因、语义告警、执行图谱和商业痛点。
- 一键填充脚本：已用当前模板试运行，可生成 `output/final/` 下 DOCX/PDF/PPTX/ZIP 与最终提交审计表；因真实信息为空，审计表会列出缺失项。

## 未能本地完成的在线提交项

线上系统提交需要负责人学校账号、验证码、真实个人信息、学院审核/盖章及最终学校通知字段；本地无法代替负责人完成。
