# 维护检查清单

## 新功能

- [ ] 属于哪个领域？
- [ ] 是否需要 API route？
- [ ] 是否需要 protobuf 字段？
- [ ] 是否需要 build feature？
- [ ] 是否需要 runtime gate？
- [ ] 是否需要 auth？
- [ ] 是否需要前端 view / component / composable / type？
- [ ] 是否影响 generated files？
- [ ] 是否影响 docs / website？
- [ ] 是否有最小验证？

## 后端 API

- [ ] route group 正确；
- [ ] authMiddleware；
- [ ] runtime gate middleware；
- [ ] request / response schema；
- [ ] external API docs；
- [ ] tests。

## 前端

- [ ] `<script setup lang="ts">`；
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

## 安全

- [ ] 默认关闭高风险能力；
- [ ] release mode auth；
- [ ] runtime gate；
- [ ] 不泄漏 secrets / TLS plaintext；
- [ ] policy mutation 有审计和权限；
- [ ] exact matching 没写成 recursive / CIDR / range；
- [ ] 更新 security-model / threat-model / policy-semantics。

## 文档站

- [ ] 新页面加入 nav / sidebar；
- [ ] 链接可点击；
- [ ] 代码路径真实存在；
- [ ] 旧路径已校正；
- [ ] `bun run docs:build`；
- [ ] 与专项文档互相索引。
