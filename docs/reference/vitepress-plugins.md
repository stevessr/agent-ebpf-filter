# 文档渲染配置（MkDocs）

该文档站已迁移到 **MkDocs Material**，不再使用 VitePress 构建。此文件保留原有路径 `vitepress-plugins.md`，仅用于兼容历史文档链接。

## 配置位置

- `docs/mkdocs.yml`：站点 URL、导航、主题、Markdown 扩展及排除目录。
- `docs/requirements.txt`：Python 文档依赖。
- `docs/site_hooks.py`：旧式绝对站内链接和 VitePress 风格提示容器的兼容层。
- `docs/javascripts/mathjax.js`：LaTeX 数学公式渲染配置。
- `.github/workflows/docs-pages.yml`：文档目录变更时运行的部署工作流。

## 支持的功能

- **Mermaid 图表**：标准 `mermaid` 代码块，通过 Material for MkDocs 原生渲染。
- **LaTeX 数学公式**：通过 `pymdownx.arithmatex` 和 MathJax 处理行内、块级公式。
- **Markdown 提示**：支持 `!!! tip`，并兼容已有 `::: tip`、`::: warning` 等提示块。
- **子路径**：使用 `site_url` 和文档钩子，适配 GitHub Pages 的 `/agent-ebpf-filter/` 子路径。

## 本地构建

```bash
python3 -m pip install -r docs/requirements.txt
python3 docs/build_site.py serve
python3 docs/build_site.py build
```

参见 [GitHub Pages 自动部署](/operations/github-pages) 了解触发范围和首次启用步骤。
