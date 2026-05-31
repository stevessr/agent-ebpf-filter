# 07 — Agent 工作流与检查清单层

本层提供 Agent 执行开发任务时的操作模板、修改路径选择、验证和文档同步清单。

## 通用工作流

1. **界定影响域**
   - backend / frontend / proto / eBPF / wrapper / adapters / docs / deploy。
   - 是否涉及安全边界、runtime gate、auth、特权执行或外向发布。

2. **读取最小上下文**
   - 先读本 skill 对应 reference。
   - 再读目标文件附近代码。
   - 避免优先打开生成文件、二进制、超大参考目录。

3. **找跨层同步点**
   - proto 改动是否需要 regeneration？
   - backend event/API 是否要改 frontend composable？
   - runtime config 是否要改 env/docs/UI？
   - 安全行为是否要改 security docs？

4. **实现时遵守本项目风格**
   - Go：小函数、贴近现有 handler/test 风格。
   - Vue：Composition API、`<script setup lang="ts">`、domain composable。
   - eBPF：保持 struct/map/layout 与 Go decode 一致。
   - docs：同步产品行为，而不是只写实现细节。

5. **运行最小充分验证**
   - 按改动域选择命令。
   - 如果跳过验证，说明原因。

6. **报告结果**
   - 说明改了哪些文件。
   - 说明验证命令和结果。
   - 明确任何未完成/风险/后续建议。

## 新功能开发清单

- [ ] 功能属于哪个领域？
- [ ] 是否需要新 API route？
- [ ] 是否需要 protobuf 字段？
- [ ] 是否需要 runtime feature gate？
- [ ] 是否需要前端页面/组件/composable？
- [ ] 是否需要持久化或导入导出？
- [ ] 是否影响 auth/release mode？
- [ ] 是否涉及特权/eBPF/OS enforcement？
- [ ] 是否要更新 README/docs/AGENTS/component README？
- [ ] 是否有测试或最小手动验证方式？

推荐步骤：

1. 写 backend data model / handler / route。
2. 写 frontend composable / type / UI。
3. 同步 docs。
4. 跑 backend tests + frontend build。

如果功能从 proto 开始：

1. 改 proto。
2. `make proto`。
3. 改 backend。
4. 改 frontend。
5. 验证。

## Bug 修复清单

- [ ] 能否定位到单一层，还是跨层 bug？
- [ ] 是否有现有 test 覆盖？
- [ ] 是否能写 regression test？
- [ ] bug 是否来自生成文件过期？
- [ ] bug 是否来自 release/dev auth 差异？
- [ ] bug 是否只在特权/eBPF 环境复现？
- [ ] 修复后是否需要文档更新？

调试路线：

- UI 错误：先看 frontend view/composable/types，再看 backend response schema。
- API 错误：先看 `routes.go` + handler + auth/gate，再看 frontend request。
- 事件缺字段：先看 proto/generated，再看 backend construction，再看 frontend display。
- eBPF 不出事件：先看 tracking maps、pin path、bootstrap、ringbuf decode、kernel support。
- shell/wrapper 问题：先看 feature gate、UDS socket、peer credentials、privilege dropping。

## Code review 清单

- [ ] 是否手改了生成文件？如果是，是否应该改源头？
- [ ] 是否遗漏 proto/backend/frontend/docs 同步？
- [ ] 是否引入 release mode auth bypass？
- [ ] 是否绕过 runtime feature gate？
- [ ] 是否把危险能力默认打开？
- [ ] 是否把 exact matching 错写成 recursive/CIDR/range？
- [ ] 是否在 Vue view 内堆过多业务逻辑？
- [ ] 是否在 `main.go` 内继续膨胀 shell/session 逻辑？
- [ ] 是否泄漏 TLS 明文或敏感 header/body？
- [ ] 是否破坏 UDS socket 权限/peer validation？
- [ ] 是否更新相关 tests/docs？

## Vue 改动清单

- [ ] 使用 `<script setup lang="ts">`。
- [ ] 业务状态/API 是否放到了 composable？
- [ ] UI 子块是否拆到 `components/<domain>/`？
- [ ] 共享类型是否放到 `types/` 或 domain colocated type？
- [ ] WebSocket/interval/event listener 是否 cleanup？
- [ ] route param/tab 状态是否和 `router/index.ts` 对齐？
- [ ] API token / WebSocket `?key=` 是否沿用现有工具？
- [ ] `cd frontend && bun run build` 是否通过？

适用时加载：

- `vue-best-practices`
- `vue-development-guides`
- `vue-router-best-practices`
- `vue-pinia-best-practices`（若引入/修改 Pinia，当前项目未显式使用 Pinia）

## Backend API 改动清单

- [ ] route 是否放在正确 `register*Routes` 分组？
- [ ] 是否需要 `authMiddleware()`？
- [ ] 是否需要 runtime gate middleware？
- [ ] request/response struct 是否稳定？
- [ ] handler 是否复用现有 helper/state？
- [ ] 是否需要 frontend composable？
- [ ] 是否需要 external API docs？
- [ ] 是否有 test？

验证：

```bash
cd backend && go test ./...
```

## Protobuf 改动清单

- [ ] 改的是正确的 domain proto 文件？
- [ ] `tracker.proto` 是否只需保持 import 聚合？
- [ ] 字段号是否避免冲突？
- [ ] 是否保留 backward compatibility？
- [ ] 是否运行 `make proto`？
- [ ] backend generated pb 是否更新？
- [ ] frontend generated pb 是否更新？
- [ ] adapters generated pb 是否更新？
- [ ] frontend types/display 是否同步？
- [ ] docs 是否同步？

验证：

```bash
make proto
cd backend && go test ./...
cd frontend && bun run build
```

## eBPF 改动清单

- [ ] 是否修改了源 C 文件而不是生成 `.go`/`.o`？
- [ ] struct layout 是否与 Go decode 匹配？
- [ ] map key/value 大小是否与 Go control/bootstrap 匹配？
- [ ] pin path / permissions 是否正确？
- [ ] verifier 约束是否考虑？
- [ ] event type 是否与 proto/frontend 一致？
- [ ] 是否需要更新 docs/security-model/threat-model？

验证：

```bash
cd backend/ebpf && go generate
cd ../.. && cd backend && go build ./...
```

cgroup/LSM：

```bash
make ebpf-cgroup
make ebpf-lsm
```

## Wrapper / shell / command execution 清单

- [ ] 是否触及危险执行路径？
- [ ] 是否保留 runtime gate？
- [ ] UDS socket 权限是否 restrictive？
- [ ] peer credentials 验证是否保留？
- [ ] privilege dropping 是否仍在 `privileges.go`？
- [ ] shell-session 逻辑是否仍在 `shell_session_*.go`？
- [ ] UI 是否清楚展示 BLOCK/ALERT/REWRITE？

验证：

```bash
cd wrapper && go test ./...
cd backend && go test ./...
```

必要时手动验证 Executor，但执行外向/破坏性命令前要确认授权。

## Hooks 改动清单

- [ ] provider catalog 是否更新？
- [ ] detection 是否覆盖目标 CLI 配置路径？
- [ ] install/uninstall 是否可逆？
- [ ] relay script 是否使用 `curl` 并处理 hook stdout 需求？
- [ ] `/hooks/event` parser 是否兼容 payload？
- [ ] per-hook secret / runtime token 是否处理？
- [ ] hook management gate 是否保留？
- [ ] docs 是否提到 runtime dependency `curl`？

验证：

- backend tests。
- frontend build。
- 如需真实安装 hook，先确认用户授权并说明会修改本机 CLI 配置。

## Security / runtime gate 清单

任何新增危险能力问：

- [ ] 默认是否关闭？
- [ ] release mode 是否受 auth 保护？
- [ ] 是否需要 runtime config 开关？
- [ ] UI 是否清楚标注风险？
- [ ] 是否会暴露 secrets、tokens、TLS plaintext、body/header？
- [ ] 是否有审计事件或状态 endpoint？
- [ ] docs/security-model 和 threat-model 是否更新？

## 文档同步清单

行为变化时检查：

- [ ] `README.md`
- [ ] `README_cn.md`（若中文产品说明受影响）
- [ ] `agents.md`
- [ ] `AGENTS.md`
- [ ] `docs/architecture.md`
- [ ] `docs/external-api.md`
- [ ] `docs/kubernetes.md`
- [ ] `docs/security-model.md`
- [ ] `docs/threat-model.md`
- [ ] `docs/policy-semantics.md`
- [ ] `backend/README.md`
- [ ] `frontend/README.md`
- [ ] `wrapper/README.md`
- [ ] adapter READMEs

## 最小验证决策表

| 改动 | 最小验证 |
| --- | --- |
| 只改 Markdown skill/docs | 文件存在性/内容检查即可 |
| 后端普通 Go | `cd backend && go test ./...` |
| 前端 Vue/TS | `cd frontend && bun run build` |
| wrapper | `cd wrapper && go test ./...` |
| proto | `make proto` + backend/frontend build |
| eBPF tracker | `go generate` + `cd backend && go build ./...` |
| cgroup/LSM | `make ebpf-cgroup` / `make ebpf-lsm` |
| dev-env TUI | `cd tools/dev-env-tui && go test ./...` |
| deploy/Kubernetes | manifest lint/review + docs sync |

## 报告模板

完成后向用户报告：

```text
已完成：
- 修改 A：...
- 修改 B：...

验证：
- `命令`：通过/失败/跳过（原因）

注意：
- 风险或后续事项（如有）
```

不要在有任务仍 `in_progress` 时给最终总结；先更新任务状态。
