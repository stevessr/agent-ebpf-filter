# 前端工作台总览

前端是 Vue 3 + Vite + TypeScript 工作台，位于 `frontend/src/`。

## 技术栈

- Vue `^3.5.32`
- Vue Router 4
- Vite `^8.0.9`
- TypeScript `5.9.3`
- Ant Design Vue
- ApexCharts / D3
- Monaco Editor
- markdown-it / Shiki
- protobufjs

## 目录分层

```text
frontend/src/
  main.ts
  App.vue
  router/index.ts
  style.css
  views/
  components/
  composables/
  types/
  data/
  utils/
  pb/          # generated，不手改
```

## 工作台页面

| 页面 | 目标 |
| --- | --- |
| Dashboard | 事件流、过滤、详情、strace-style summaries |
| Monitor | CPU、内存、GPU、IO、faults、sensors、systemd、tracing |
| Network | 网络事件、flow table、traffic graph、enrichment |
| TLSCapture | TLS / Codex capture 高风险诊断面 |
| ExecutionGraph | agent / process / tool / syscall / file / network / policy 图谱 |
| Explorer | 文件浏览、preview、tracked paths |
| Executor | shell sessions、wrapper terminal、tmux / script launcher |
| Hooks | AI CLI hook 检测和管理 |
| ML | ML status、training、tuning、dataset、LLM scoring |
| Plugins | plugin registry、visual builder、pseudocode builder |
| Config | runtime、security、registry、cluster、docs、system health |

## 设计原则

- view 是页面容器；
- component 是可复用 UI；
- composable 封装 API / WebSocket / 状态 / 转换；
- types 放共享类型；
- data 放静态 catalog；
- utils 放通用工具；
- 不在单个 `.vue` 文件堆过多业务逻辑。
