# GitHub Pages 自动部署

本站采用 **MkDocs Material** 生成静态文档，使用 GitHub Actions artifact 部署到 GitHub Pages，不再使用 VitePress。无需在 Pages 上运行 Go、MyGo 桌面或 eBPF。

- 网站预期地址：https://stevessr.github.io/agent-ebpf-filter/
- 唯一发布工作流：[Documentation / GitHub Pages](https://github.com/stevessr/agent-ebpf-filter/actions/workflows/docs-pages.yml)
- 工作流路径：`.github/workflows/docs-pages.yml`
- 文档站配置：`docs/mkdocs.yml`
- 生成目录：`site/`（不需要提交进 Git）
- Python 依赖：`docs/requirements.txt`

## 自动触发：仅文档变更

| 事件 | 条件 | 行为 |
| --- | --- | --- |
| 推送到 `master` | 此次推送至少修改一个 `docs/**` 路径 | 构建并发布 GitHub Pages |
| 新建/更新 PR | PR 中至少修改一个 `docs/**` 路径 | 检查文档并构建，**不发布** |
| 其他源码、依赖或工作流文件变动 | 未修改 `docs/**` | **不触发此文档工作流** |

不启用 `workflow_dispatch`、定时任务或其它事件。文档站配置本身存放在 `docs/`，因此对该配置的修改也计入文档变更。

## 第一次启用 Pages

仓库管理员需要在 [Settings → Pages](https://github.com/stevessr/agent-ebpf-filter/settings/pages) 中选择 **Build and deployment → Source → GitHub Actions**。

仅提交工作流 **不能自动修改仓库 Pages 发布源设置**。CI 使用 GitHub 自动颁发的 `GITHUB_TOKEN`，部署 job 申请 `pages: write` 和 `id-token: write`，不需要自定义 PAT。

## 本地验证

从仓库根目录执行：

```bash
python3 -m pip install -r docs/requirements.txt
python3 scripts/check-doc-links.py docs
python3 docs/build_site.py build
python3 docs/build_site.py serve
```

MkDocs 不允许配置文件直接位于其 `docs_dir` 中，因此 `docs/build_site.py` 会短暂创建仓库根目录下的临时配置，构建或预览结束后自动清除。这样配置和构建脚本依然属于 `docs/**`，符合触发约束。

文档站在仓库子路径 `/agent-ebpf-filter/` 下运行。文档钩子会将旧站的站内根路径链接转换成相对 Markdown 链接，由 MkDocs 生成正确子路径。Mermaid 图表、LaTeX 公式和旧的 `::: tip` 提示块也予以兼容。

## 故障排查

1. 如果构建失败，请查看 Python 依赖安装、链接校验和 MkDocs 日志。
2. 如果 `configure-pages` 失败，请确认 Pages Source 设置为 GitHub Actions。
3. 如果部署失败并返回 403，请确认 `github-pages` 环境规则与 `pages: write` / `id-token: write` 权限。
4. 如果站点返回 404，请核对 `site_url` 和构建产物中的路径。
5. PR 检查通过不代表网站已更新，只有包含文档变更的 `master` push 会发布。

[文档入口](/) · [快速开始](/guide/quick-start) · [构建与运行](/operations/build-and-run)
