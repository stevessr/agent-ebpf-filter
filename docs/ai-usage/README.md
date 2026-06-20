# AI 工具使用披露记录

> 本目录用于满足操作系统设计赛关于 AI 工具使用披露、交互记录留存和成果说明的要求。  
> 建议本目录文档按 CC-BY-SA 4.0 许可发布，并在最终答辩 PPT 中引用本目录。

## 1. 披露原则

参赛过程中允许合理使用 AI 工具，但需要真实、完整地说明：

1. 使用了哪些 AI 工具和模型；
2. 在哪些环节使用；
3. AI 产生了哪些内容；
4. 哪些内容被人工采纳、修改或放弃；
5. 采用前做了哪些人工复核、测试和合规检查；
6. 对应到哪些 git commit 或文档修订记录。

## 2. 建议登记表

| 日期 | AI 工具 / 模型 | 使用场景 | 输入摘要 | 输出摘要 | 采纳内容 | 人工复核 | 关联 commit |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 2026-06-19 | Claude Code / gpt-5.5 会话 | 操作系统设计赛答辩文档草案 | 根据赛事规则、官网链接和仓库文档构建答辩材料 | 生成 `docs/os-competition-defense.md` 草案 | 项目概述、架构、创新点、合规与 AI 披露章节 | 待队员逐节核验官网规则、依赖许可证、评测数据 | TBD |

## 3. 单次交互记录模板

```markdown
# YYYY-MM-DD - <主题>

- AI 工具 / 模型：
- 使用人：
- 使用场景：
- 输入摘要：
- 输出摘要：
- 采纳内容：
- 未采纳内容及原因：
- 人工修改：
- 验证命令 / 结果：
- 关联文件：
- 关联 commit：
```

## 4. Commit message 建议

可在需要披露的 commit footer 中加入：

```text
AI-Assisted-By: Claude Code
AI-Usage: drafted documentation outline; human reviewed and edited
```

若赛事平台不适合在每条 commit 中加入较长说明，也可在本目录集中记录，并在设计文档和答辩 PPT 中说明“完整 AI 使用记录见 `docs/ai-usage/`”。

## 5. 当前已生成材料

- `docs/os-competition-defense.md`：操作系统设计赛答辩项目文档草案，包含项目概述、架构、关键技术、演示流程、开源合规、AI 使用披露和答辩 PPT 结构。

---

## 相关导航

- [合规披露](../delivery/compliance.md)
- [OS competition defense 草案](../os-competition-defense.md)
- [第三方声明草案](../third-party-notices.md)
- [项目文档索引](../project-docs-index.md)
- [文档关系审计](../reference/documentation-audit.md)
