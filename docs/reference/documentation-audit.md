# 文档关系审计

本页记录当前文档体系的互链健康度、扫描命令和后续补强策略。它不是替代 [文档地图](documentation-map.md)，而是用于持续发现“有内容但不容易被读到”或“读到后没有下一跳”的页面。

## 截至 2026-06-20，本轮扫描覆盖：

- 根 README、`AGENTS.md`、组件 README、`kernel-ml/README.md`；
- `docs/**/*.md`，默认排除 `docs/ref/**` 外部参考快照；
- VitePress `nav` / `sidebar` 中的 `link:`；
- Markdown 中指向本仓库文档和源码的相对链接。

检查命令：

```bash
python3 scripts/check-doc-links.py
python3 scripts/check-doc-links.py --report
bun run docs:build
```

当前结果：

- `checked_files=122 missing_links=0`；
- VitePress 构建通过；
- 弱出链页面已从“多批当前专题页”收敛到少量历史 / 低变更页面；当前权威页和多数专项总结页已补“相关导航”。

## ```mermaid
graph TB
    Home[docs/index.md] --> Guide[Guide]
    Home --> Architecture[Architecture]
    Home --> Backend[Backend]
    Home --> Security[Security]
    Home --> Integrations[Integrations]
    Home --> Operations[Operations]
    Home --> Delivery[Delivery]
    Home --> Reference[Reference]

    Reference --> Map[documentation-map.md]
    Reference --> Audit[documentation-audit.md]
    Reference --> Entrypoints[code-entrypoints.md]
    Reference --> Checklists[maintenance-checklists.md]

    Map --> ProjectIndex[project-docs-index.md]
    Map --> DevIndex[DEV_DOCS_INDEX.md]
    Map --> ComponentReadmes[component READMEs]
    Map --> Historical[historical / specialty docs]

    Backend --> Events[event-pipeline.md]
    Backend --> EBPF[ebpf-os-enforcement.md]
    Backend --> ML[ml-plugins.md]
    Security --> Gates[runtime-gates-auth.md]
    Security --> Policy[policy-semantics.md]
    Integrations --> MCP[mcp-external-otlp.md]
    Delivery --> Compliance[compliance.md]
```

## | 类别 | 现象 | 处理策略 |
| --- | --- | --- |
| 首页 / 入口页 | `docs/index.md` 作为 VitePress home，入链低是正常现象 | 由 VitePress nav 负责入口，不强求普通 Markdown 入链 |
| 历史计划 / superpowers | 计划和规格页通常只有索引入链、正文少出链 | 由 [文档地图](documentation-map.md) 主题表承接；必要时在页尾补“历史上下文” |
| 专项实现总结 | 例如 TLS、MCP、ML、脱敏总结页容易成为孤岛 | 在页尾补“相关导航”，指向当前权威页、代码入口和验证页 |
| 旧版深度文档 | `DEEP_CODE_ANALYSIS.md`、`project-structure-deep-dive.md` 等篇幅大且历史路径多 | 保留历史价值，同时在开头或结尾指向当前网站页与代码入口索引 |
| 交付 / 合规材料 | `os-competition-defense.md`、`third-party-notices.md`、`ai-usage/` 等答辩材料与工程页分离 | 从 Delivery、文档地图和项目索引反向链接，并补充到验证/维护清单 |

## - 文档地图增加功能域互联矩阵、变更影响链、历史/专项材料主题入口。
- 项目文档索引中的路径改为可点击 Markdown 链接。
- VitePress 顶部导航加入“答辩交付”。
- Runtime gates、事件管线、eBPF enforcement、ML/plugins 等核心专题页补了跨页影响链。
- 新增 `scripts/check-doc-links.py --report`，用于后续持续发现弱入链/弱出链页面。
- 给当前权威页、专项总结、交付材料、MCP/TLS/ML/eBPF 历史页和 Superpowers 计划页批量补了“相关导航”，让读者能跳回当前专题、代码入口或验证命令。

## 1. 跑 `python3 scripts/check-doc-links.py --report`，记录弱入链 / 弱出链前 25 个页面。
2. 先判断页面类型：入口页、历史计划、当前权威页、专项总结、交付材料。
3. 当前权威页和专项总结页优先补“相关导航”；历史计划页可由文档地图集中承接。
4. 如果新增专题页，必须同步：
   - [文档地图](documentation-map.md)；
   - [阅读路线](/guide/reading-paths)；
   - [维护检查清单](maintenance-checklists.md)；
   - `docs/.vitepress/config.ts`（如果需要 nav / sidebar）。
5. 完成后运行 `python3 scripts/check-doc-links.py`、`git diff --check`、`bun run docs:build`。

## “文档互链完成”不等于每个历史页都有大量链接，而是满足：

- 当前权威页面可从 nav/sidebar 或文档地图到达；
- 读者从任一核心功能页能跳到安全边界、代码入口和验证命令；
- 历史/专项页至少能被某个索引或主题表发现；
- 文档变更有链接扫描和 VitePress 构建证据；
- 旧路径或历史说法不会冒充当前实现。
