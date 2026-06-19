# 第三方与 AI 使用披露

比赛和交付材料需要说明第三方依赖、参考资料、许可证和 AI 辅助情况。

## 第三方依赖

主要依赖类型：

- Go libraries：Gin、gorilla/websocket、cilium/ebpf、OpenTelemetry、protobuf、MCP SDK；
- Frontend：Vue、Vite、Ant Design Vue、ApexCharts、D3、Monaco、Shiki；
- Tooling：Bun、Go、Python / uv、clang / LLVM；
- Docs：VitePress。

维护位置：

- `docs/third-party-notices.md`
- component READMEs
- lockfiles

## AI 使用披露

维护位置：

```text
docs/ai-usage/README.md
```

建议记录：

- 使用的 AI 工具；
- 任务范围；
- 人工审查方式；
- 生成内容是否直接进入代码 / 文档；
- 验证命令；
- 已知限制。

## 合规提醒

- 不把参考文档复制为项目原创；
- 引用性能数据写明环境；
- 高风险能力演示需说明授权和默认关闭；
- generated files 与第三方 vendored docs 不应混淆为手写实现。
