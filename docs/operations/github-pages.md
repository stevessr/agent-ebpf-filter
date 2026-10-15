# GitHub Pages 自动部署

项目的 VitePress 文档站可以通过 GitHub Actions 发布为静态网站；不需要在 Pages 主机上运行 Go 后端、MyGo 桌面或 eBPF。

- **预期地址**：[https://stevessr.github.io/agent-ebpf-filter/](https://stevessr.github.io/agent-ebpf-filter/)
- **构建输入**：仓库根目录的 `package.json`、`bun.lock` 与 `docs/`。
- **构建输出**：`docs/.vitepress/dist`。
- **发布工作流**：[pages-vitepress.yml](https://github.com/stevessr/agent-ebpf-filter/blob/master/.github/workflows/pages-vitepress.yml)。
- **独立文档检查**：`vitepress-docs.yml` 保留原有的本地根路径构建验证。

## 第一次启用：仓库 Pages 设置

站点首次发布前，仓库管理员需在 [Settings → Pages](https://github.com/stevessr/agent-ebpf-filter/settings/pages) 中，将 **Build and deployment → Source** 设为 **GitHub Actions**。

> 提交工作流文件**不会自动修改仓库的 Pages 发布源**。本仓库的部署工作流仅使用自动颁发的 `GITHUB_TOKEN`，不会要求长期 PAT 或把凭据写进仓库；如未开启 Pages，`configure-pages` 可能失败。这一步仅需管理员配置一次。

## 自动运行规则

| 事件 | 会发生什么 |
| --- | --- |
| 向 `master` 推送文档、锁文件或部署配置更改 | 安装锁定依赖、查链接、构建 Pages 版本、上传静态文件并发布 |
| 创建/更新涉及相关文件的 PR | 执行同样的 Pages 路径构建与检验，**不发布、不上传部署产物** |
| Actions 页面手动触发 `workflow_dispatch` | 在默认分支运行构建和发布，适合初次开启 Pages 后重新部署 |
| 无关源代码变更 | 不重复构建文档，减少不必要的 Actions 运行 |

发布工作流设置了 `github-pages` 环境。部署作业只有 `contents: read`、`pages: write`、`id-token: write` 这些必需权限，不需要提交 Pages 分支或配置 GitHub PAT。

## 子路径、导航和本地开发

仓库 Pages 地址以 `/agent-ebpf-filter/` 开头，因此**仅在 Pages Actions 构建时**设置：

```bash
VITEPRESS_BASE=/agent-ebpf-filter/ bun run docs:build
```

VitePress 的 `base` 会自动应用于站内导航和资产链接。普通开发仍使用 `/`：

```bash
bun install --frozen-lockfile
bun run docs:dev
bun run docs:build
```

在本地验证 Pages 静态文件时，构建后应检查 `docs/.vitepress/dist/index.html` 中的 JS/CSS URL 指向 `/agent-ebpf-filter/assets/`，否则 GitHub Pages 上可能出现首页可打开、但样式和脚本 404 的问题。

如果以后改为自定义域名的根目录托管，需要同时把工作流里的 `VITEPRESS_BASE` 更新为 `/`，并在 Settings → Pages 配置域名；不要只修改 DNS。

## 排查部署问题

1. **Actions 构建失败**：检查 [Pages 工作流运行记录](https://github.com/stevessr/agent-ebpf-filter/actions/workflows/pages-vitepress.yml) 中的 Bun 安装、`check-doc-links.py` 或 VitePress 构建步骤。
2. **`configure-pages` 失败**：确认 Pages 已启用、发布源是 GitHub Actions，且仓库允许 GitHub Actions 使用相应权限。
3. **部署返回 403**：检查部署作业的 `pages: write` / `id-token: write`、`github-pages` 环境审批及分支规则。
4. **静态资源 404**：检查 `VITEPRESS_BASE` 是否等于 `/agent-ebpf-filter/`，并重新触发发布。
5. **首次部署仍显示 404**：先以成功的 `Deploy artifact` 作业为准核对 Pages URL；不能把 PR 的绿色构建等同于网站已经发布。

## 关联

[文档入口](/) · [快速开始](/guide/quick-start) · [构建与运行](/operations/build-and-run)。
