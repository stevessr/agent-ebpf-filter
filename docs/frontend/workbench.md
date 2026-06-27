# 前端工作台总览

前端是 Vue 3 + Vite + TypeScript 工作台，位于 `frontend/src/`。

## ```mermaid
graph TB
    subgraph "Core"
        Vue["Vue 3.5.32<br/>Composition API + script setup"]
        Router["Vue Router 4"]
        Vite["Vite 8.0.9<br/>Rolldown + Oxc"]
        TS["TypeScript 5.9.3"]
    end
    
    subgraph "UI"
        AntD["Ant Design Vue<br/>组件库"]
        Charts["ApexCharts + D3<br/>图表"]
        Monaco["Monaco Editor<br/>代码编辑"]
    end
    
    subgraph "Data"
        Proto["protobufjs<br/>Event/EventEnvelope"]
        WS["WebSocket<br/>实时事件流"]
        HTTP["axios<br/>REST API"]
    end
    
    subgraph "Content"
        MD["markdown-it<br/>Markdown 渲染"]
        Shiki["Shiki<br/>代码高亮"]
        TOML["smol-toml<br/>配置解析"]
    end
    
    Vue --> Router
    Vue --> AntD
    Vue --> Charts
    Vue --> Monaco
    Router --> WS
    Router --> HTTP
    WS --> Proto
    HTTP --> Proto
    
    style Vue fill:#bfb
    style Vite fill:#bbf
    style Proto fill:#fbb
```

- Vue `^3.5.32`
- Vue Router 4
- Vite `^8.0.9`
- TypeScript `5.9.3`
- Ant Design Vue
- ApexCharts / D3
- Monaco Editor
- markdown-it / Shiki
- protobufjs

## ```mermaid
graph TB
    subgraph "frontend/src/"
        Main["main.ts<br/>app bootstrap"]
        App["App.vue<br/>root shell"]
        Router["router/index.ts<br/>route definitions"]
        
        Views["views/<br/>页面容器"]
        Components["components/<br/>可复用 UI"]
        Composables["composables/<br/>API/WS/状态"]
        
        Types["types/<br/>共享类型"]
        Data["data/<br/>静态 catalog"]
        Utils["utils/<br/>通用工具"]
        
        PB["pb/<br/>generated protobuf<br/>⚠️ 不手改"]
        Style["style.css"]
    end
    
    Main --> App
    App --> Router
    Router --> Views
    Views --> Components
    Views --> Composables
    Components --> Types
    Composables --> PB
    Composables --> Types
    Data --> Types
    
    style Main fill:#bfb
    style PB fill:#fbb
    style Views fill:#bbf
    style Composables fill:#bbf
```

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

## ```mermaid
graph LR
    subgraph "观测"
        Dashboard["Dashboard<br/>事件流"]
        Monitor["Monitor<br/>系统指标"]
        Network["Network<br/>网络流"]
        Graph["ExecutionGraph<br/>进程图谱"]
    end
    
    subgraph "控制"
        Config["Config<br/>运行时配置"]
        Security["Security<br/>策略管理"]
        Hooks["Hooks<br/>AI CLI 集成"]
        Executor["Executor<br/>Shell/Wrapper"]
    end
    
    subgraph "诊断"
        TLS["TLSCapture<br/>明文诊断"]
        AgentSight["AgentSight<br/>录制回放"]
        Explorer["Explorer<br/>文件浏览"]
    end
    
    subgraph "扩展"
        ML["ML<br/>训练/评分"]
        Plugins["Plugins<br/>eBPF 扩展"]
    end
    
    Dashboard -->|filter by| Network
    Network -->|trace flow| Graph
    Graph -->|drill down| Dashboard
    Config -->|enable| TLS
    Security -->|policy| Executor
    Hooks -->|events| Dashboard
    
    style Dashboard fill:#bbf
    style Config fill:#fbb
    style TLS fill:#fbb
    style ML fill:#bfb
```

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

## ```mermaid
graph TB
    User[User Action] --> View[View<br/>页面容器]
    View --> Composable[Composable<br/>API/WS/状态]
    Composable --> API[Backend API]
    API --> Composable
    Composable --> State[Reactive State]
    State --> View
    View --> Component[Component<br/>可复用 UI]
    Component --> Props[Props]
    Component --> Emit[Emit Events]
    Emit --> View
    
    Types[types/<br/>共享类型] -.-> View
    Types -.-> Composable
    Types -.-> Component
    
    Data[data/<br/>静态 catalog] -.-> View
    Utils[utils/<br/>通用工具] -.-> Composable
    
    style View fill:#bbf
    style Composable fill:#bfb
    style Component fill:#ffb
    style Types fill:#ddd
```

- view 是页面容器；
- component 是可复用 UI；
- composable 封装 API / WebSocket / 状态 / 转换；
- types 放共享类型；
- data 放静态 catalog；
- utils 放通用工具；
- 不在单个 `.vue` 文件堆过多业务逻辑。

## ```mermaid
sequenceDiagram
    participant User as User
    participant View as Dashboard.vue
    participant Comp as useDashboardStream
    participant WS as WebSocket /ws
    participant Backend as Backend
    participant Component as EventCard.vue
    
    User->>View: 打开 Dashboard
    View->>Comp: const { events, filters } = useDashboardStream()
    Comp->>WS: connect()
    WS->>Backend: WebSocket handshake
    Backend-->>WS: connected
    
    loop Real-time Events
        Backend->>WS: pb.Event message
        WS->>Comp: onmessage(data)
        Comp->>Comp: decode protobuf
        Comp->>Comp: apply filters
        Comp->>View: events.value updated
        View->>Component: v-for event in events
        Component->>User: render event card
    end
    
    User->>View: change filter
    View->>Comp: updateFilter(type)
    Comp->>Comp: filters.value = {...}
    Comp->>View: filtered events
```

---

## - [路由与功能页](routes-and-pages.md)
- [组件与 Composables](components-composables.md)
- [构建与 Feature Flags](build-feature-flags.md)
- [事件管线](../backend/event-pipeline.md)
- [前端 README](../../frontend/README.md)
