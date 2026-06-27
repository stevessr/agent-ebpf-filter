# 维护检查清单

## - [ ] 属于哪个领域？
- [ ] 是否需要 API route？
- [ ] 是否需要 protobuf 字段？
- [ ] 是否需要 build feature？
- [ ] 是否需要 runtime gate？
- [ ] 是否需要 auth？
- [ ] 是否需要前端 view / component / composable / type？
- [ ] 是否影响 generated files？
- [ ] 是否影响 docs / website？
- [ ] 是否有最小验证？

## API

- [ ] route group 正确；
- [ ] authMiddleware；
- [ ] runtime gate middleware；
- [ ] request / response schema；
- [ ] external API docs；
- [ ] tests。

## - [ ] `<script setup lang="ts">`；
- [ ] API / WS 逻辑在 composable；
- [ ] UI 拆组件；
- [ ] shared type 放 types；
- [ ] cleanup WebSocket / interval；
- [ ] route param 与 router 对齐；
- [ ] `cd frontend && bun run build`。

## Proto

- [ ] 改 domain proto；
- [ ] 字段号不冲突；
- [ ] backward compatible；
- [ ] `make proto`；
- [ ] backend / frontend / adapters 生成物同步；
- [ ] docs 同步。

## eBPF

- [ ] 改 C 源而不是 generated；
- [ ] struct layout 与 Go decode 一致；
- [ ] map key/value 大小一致；
- [ ] verifier 约束；
- [ ] event type 与 proto/frontend 一致；
- [ ] pin path / permissions；
- [ ] security docs。

## - [ ] 默认关闭高风险能力；
- [ ] release mode auth；
- [ ] runtime gate；
- [ ] 不泄漏 secrets / TLS plaintext；
- [ ] policy mutation 有审计和权限；
- [ ] exact matching 没写成 recursive / CIDR / range；
- [ ] 更新 security-model / threat-model / policy-semantics。

## - [ ] 新页面加入 nav / sidebar；
- [ ] 链接可点击；
- [ ] 代码路径真实存在；
- [ ] 旧路径已校正；
- [ ] `python3 scripts/check-doc-links.py`；
- [ ] 若在做文档梳理 / 深化，查看 `python3 scripts/check-doc-links.py --report` 的弱入链和弱出链页面；
- [ ] 需要渲染 / 导航验证时运行 `bun run docs:build`；
- [ ] 与专项文档互相索引；
- [ ] 新增页面已从 [文档地图](/reference/documentation-map) 或 [阅读路线](/guide/reading-paths) 反向链接；
- [ ] 若文档描述 route、auth、runtime gate、eBPF map、protobuf 或 kernel-ml UAPI，同步更新对应专题页和组件 README。

## - [ ] `docs/ref/**` 是外部参考快照；默认不要求修复其内部断链，除非本次任务明确维护该快照。
- [ ] VitePress 绝对路径（如 `/backend/event-pipeline`）能映射到 `docs/backend/event-pipeline.md`。
- [ ] 仓库相对路径（如 `../../kernel-ml/README.md`）从当前 Markdown 文件所在目录解析后真实存在。
- [ ] 历史实现记录中若保留旧路径，应注明“历史路径 / 当前入口见……”，不要让它冒充当前代码入口。
- [ ] 重要专题至少有“上游概念页 → 当前专题页 → 源码入口 / 验证命令”的三段链路。
