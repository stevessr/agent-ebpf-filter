# Agent eBPF Filter 调研与优化报告

> 调研日期：2026-10-11  
> 仓库基线：`master` / `29e15a53750a9ca19fbcba07ac543919f65fa09a`  
> 方法：当前源码与 CI 审阅、非特权测试、局部微基准、官方文档对照。  
> 初次调研仅新增报告；后续已实施安全与流恢复改进，见下方实施记录。提交记录见本地 Git；未推送。

## 实施记录：安全、验证与 Dashboard 流恢复改进

以下“一”至“六”保留初次调研的基线证据，不代表后续代码仍保持原行为。

### 已实施

- **O1**：TCP 默认 `127.0.0.1`；新增 `AGENT_BACKEND_HOST`（字面 IP），免认证与非回环监听组合启动即拒绝；认证不再取决于 Gin mode。主 WS 升级器和集群转发前统一检查来源；原生无 Origin 客户端仍可使用认证访问。TLS 兼容 upgrader 也恢复安全默认来源检查。
- **O2 止血部分**：内置访问日志不输出 query、header 或 handler error；panic 日志只保留通用提示和调用栈，不转储请求/异常值。保留现有 query token 兼容，**未实现短期 ticket**，外部代理日志也需单独核查。
- **O3 部分**：修复 Vite 代理断言，补充 URL override 和非法端口回退测试；frontend CI 执行所有单测及 core 工作台测试。热路径 CI 扩大路径触发范围，运行专项测试、race 检查及 5 次微基准，上传原始结果。
- **O4 部分**：Dashboard 改用 envelope ID 去重；先订阅后补取最近最多 1000 条，重连重新补取；暂停仅暂停显示并保留有界队列，显示队列淘汰和不可证明完整的断线缺口；补取失败可重试，切换会话取消旧请求，清空失败保留本地证据。**未实现连续游标恢复/保留期证明**。
- **O5 部分**：广播基准每次使用新事件，停止循环复用已被脱敏和保留的对象；新基准包含 clone 成本，不可直接与旧数字做同比结论。
- 同步开发脚本/提权环境透传、服务配置提示、Kubernetes 显式网络监听和认证文档。未启动/重启用户服务，未加载或改动内核策略。

### 升级注意

原先依靠 `GIN_MODE=debug` 免认证的直接启动方式，现在必须发送 token，或仅在本机开发中显式设置 `DISABLE_AUTH=true`。`make dev` 已显式设置该变量。远程部署需设置 `AGENT_BACKEND_HOST` 并保留认证；反向代理需保留 Host 或设置 `AGENT_WS_ALLOWED_ORIGINS`。详见 [认证与迁移说明](../security/runtime-gates-auth.md)。

### 已完成验证

- 前端全量 **59 pass / 0 fail**（含 12 个流生命周期/协议测试）；core 工作台 **10 pass**；显式远程 URL 代理测试 **4 pass**；前端生产构建通过。
- 后端 handlers、observability、events、wsfanout、tls、core、redaction、recording 八个包测试通过；app 中安全、广播和持久化等专项测试通过。
- 新安全专项 race 检测通过；`go build ./...` 通过；有真实本机 WebSocket handshake 测试（仅测试服务器，无 eBPF），覆盖来源白名单、未认证拒绝、debug 下认证和原生客户端。
- 日志回归覆盖成功、401、panic，确认虚构 query/header/Bearer/error/panic 标记均不出现在捕获日志中。
- Chromium 实际操作 Dashboard，验证初次补取、暂停/恢复、自动重连、protobuf 去重、补取失败/重试、清空失败保留以及明细/窄屏缺口提示，无未捕获浏览器异常。使用合成传输 fixture，不是真实 eBPF 端到端。可复现命令见 [工作台指南](../guide/web-workbench.md#dashboard-接收暂停与重连)。
- Shell 语法检查通过。事件构造微基准可执行，**不据此宣称性能提升**。
- Renew `go test ./...` 通过；VitePress 文档构建通过；修改的 workflow 和 Kubernetes YAML 可解析。没有远程运行 GitHub Actions。

### 已确认的基线失败与剩余范围

扩大 app 全量测试发现两项已有失败：

1. `TestLsmPolicySourceUsesCurrentHookArguments`：mmap_file 源码空指针检查断言失败。
2. `TestOSFrontendSecuritySurfaceWiresSandboxEndpoints`：旧 UI 标题字符串断言失败。

使用 Go `-overlay` 将本轮修改的 Go 文件映射回原基线 `29e15a53`，并排除新增安全实现/测试后，上述两项仍失败，确认不是本轮引入。未删除、跳过或放宽这两项测试；新的热路径 job 保持专项测试范围，**不能称为后端全量通过**。

额外执行 `scripts/codex-cloud/maintain.sh verify`（报告 `reports/codex-cloud/latest.md`，被 Git 忽略）后，全套脚本有 **4 类失败**：版本声明精确匹配、既有 Go 文件格式、`app/events/attribution_ancestry_test.go` 复制 protobuf 锁的 vet 警告、上述 app 测试。其余检查包括离线 eBPF 生成、Go 构建、Renew/wrapper/TUI、前端构建、文档和 Python adapter 均通过。未批量格式化无关文件。

该脚本的 protobuf 生成还产生了两份无关 JS 大量重排；浏览器暴露生成版本的嵌套 history decoder `ReferenceError: C is not defined`。已撤销这两份纯生成副产物，保留原有已跟踪 bindings，并补充 history protobuf roundtrip 单测防止“生成成功/编译成功”掩盖解码失败。**生成工具链问题仍需另行修复**；最终全量前端测试和浏览器回归针对实际提交的 bindings 重跑。

仍待实施：O2 ticket 迁移、O3 浏览器测试接入 CI 与手动内核 benchmark fail-fast、O4 连续游标补读与完整性证明、O5 阶段直方图和端到端基线，以及 O6–O10 的结构优化。CI 配置仅本地检查，未远程运行；未进行真实 eBPF、生产部署或桌面 GUI 验收。

## 一、执行摘要

**建议下一阶段从“增加功能”转为“安全默认值、证据完整性、可重复性能验证”。** 当前项目并不是缺少基本优化的原型：已有内核丢事件统计、批量广播、慢客户端隔离、异步持久化、Pebble 游标分页、前端有界缓冲以及 Renew 原生 UI 批处理。

本轮最值得优先处理的事项：

1. **收紧独立后端网络入口**：TCP 路径监听所有地址，认证又与 Gin release 模式绑定，WebSocket Origin 放行全部来源。此组合需要优先整改，不能依靠操作者记得设置生产环境变量。
2. **阻止访问令牌进入日志**：WebSocket URL 携带长期令牌；当前 Gin 默认 logger 保留 query。本轮用虚构令牌验证了日志保留行为。
3. **修复测试与 CI 脱节**：前端全量测试实际为 43 通过、1 失败，但常规 frontend workflow 只跑图标测试和构建；热路径 workflow 没有执行 benchmark。
4. **明确实时流的断档与恢复语义**：Dashboard 暂停时直接丢弃新消息，断线重连未触发补读；“连接正常”和“证据完整”不是同一件事。
5. **先量化再优化存储、热路径和 UI**：读请求与同步刷盘存在耦合；后端串行处理和多面板重复订阅值得分析，但尚不能宣布它们是已测得的瓶颈。

优先级含义：P0 为发布/暴露网络前处理；P1 为下一迭代；P2 为基线建立后的结构优化。工作量为熟悉项目的开发者的粗略人日估计，包含相关测试，不是交付承诺。

| 编号 | 优先级 | 项目 | 证据性质 | 估计工作量 |
| --- | --- | --- | --- | --- |
| O1 | P0 | 安全监听默认值、独立认证开关、Origin 白名单 | 源码确认；未做网络利用测试 | 2–3 人日 |
| O2 | P0 | 日志令牌脱敏与短期 WS ticket | 默认日志行为已复现 | 止血 1 人日；ticket 2–4 人日 |
| O3 | P1 | 修复前端失败测试并补齐 CI 门禁 | 测试失败与 workflow 配置确认 | 1–2 人日 |
| O4 | P1 | 流断档标记、重连补读、去重 | Dashboard 源码确认 | 3–5 人日 |
| O5 | P1 | 修正性能基准、完善分阶段指标 | 基准和 CI 缺口确认 | 2–4 人日 |
| O6 | P1 | 分离历史查询和强一致刷盘 | 耦合确认；收益待压测 | 3–5 人日 |
| O7 | P2 | 事件热路径与按需摘要优化 | 优化假设，需 profile | 3–5 人日 |
| O8 | P2 | Web 多面板共享订阅和按需加载 | 结构及包大小确认；卡顿未实测 | 3–5 人日 |
| O9 | P2 | 缩小特权后端职责 | 架构风险评估 | 原型 5–10 人日 |
| O10 | P2 | 文档、协议、能力声明一致性 | 部分导航已落后于源码 | 2–3 人日 |

## 二、调研范围与当前架构

审阅覆盖 eBPF 采集公共代码、Go 事件广播/认证/持久化/指标、Vue Dashboard 与工作台、Renew IPC/UI、相关 GitHub Actions。不是逐文件安全审计，也不是所有功能的验收。

当前主链路可概括为：

```text
eBPF ringbuf
  → kernel reader / event construction
  → bounded broadcast queue
  → attribution / capture admission / semantic alerts
  → redaction / archive / persistence / optional analysis
  → WebSocket batches 或 Renew native summaries
  → bounded UI buffers / user investigation
```

### 已有能力，应该保留而非重复建设

| 能力 | 当前证据 | 对后续优化的约束 |
| --- | --- | --- |
| 内核 reserve 失败计数、CPU 序列和 generation 关联 | `backend/ebpf/agent_tracker_common.h:202–229,460–517` | 不应简单建议“增加丢包统计”；应补充分阶段解释与验收 |
| 广播按 50 条或 50ms 刷新；无订阅者跳过相应 protobuf 序列化 | `backend/app/ws_api.go:27–133` | 不应重做基本批处理 |
| 每客户端独立、有界队列，慢客户端断开，共享 prepared message | `backend/internal/wsfanout/hub.go` | 保留采集不被慢客户端拖住的行为 |
| 持久化队列 4096，128 条或 250ms 刷新；Pebble 使用同步提交 | `runtime_event_log_writer.go:15–22`、`runtime_event_store_pebble.go:382–430` | 不能用关闭持久性保证来制造性能提升 |
| Pebble 时间键、事件 ID 索引与游标分页 | `backend/app/runtime_event_store_pebble.go:118–147,511–685` | 无需先迁移到另一种数据库 |
| Dashboard shallowRef、有界缓存、80ms UI 批次、分页 | `frontend/src/composables/dashboard/useDashboard.ts`、`useDashboardStream.ts` | 不应笼统声称“每条消息都触发完整 DOM 渲染” |
| Renew 16ms 批处理、有界覆盖队列、事件选择身份保持 | `desktop/renew/interaction_perf.go` | 桌面端已有独立于 Web 的优化路径 |
| Renew 私有 Unix API，无 TCP listener | `backend/app/startup_server.go:20–40` | 独立后端的网络风险不能泛化为全部桌面会话暴露 |

官方资料也支持先保留现有 ringbuf 设计：ringbuf 空间不足时 reserve 失败而非阻塞；默认唤醒机制通常足够，分片和手动唤醒应在实测证明必要后采用。[Linux BPF ring buffer](https://docs.kernel.org/bpf/ringbuf.html)

## 三、本轮验证结果

环境：Linux `7.2.9-1-cachyos`，Go `go1.27.1-X:nodwarf5 linux/amd64`，Bun `1.4.2`，CPU 为 Intel Core i7-12800HX。执行前工作区干净。

| 验证 | 结果 | 说明 |
| --- | --- | --- |
| `cd frontend && bun test tests` | **43 pass / 1 fail**，9 个文件、44 个测试 | 失败为 `tests/config-proxy.test.ts:13` |
| `cd frontend && bun run build` | **通过** | 包含 vue-tsc；仍有大于 500kB 的产物警告 |
| 仓库根目录 `bun run docs:build` | **通过** | 新报告已通过 VitePress 构建与页面渲染；有大 chunk 警告 |
| 后端 6 个专项包 `go test … -count=1` | **全部通过** | wsfanout、observability、events、recording、redaction、core |
| 上述 6 个包 `go test -race … -count=1` | **全部通过** | 不代表全仓库没有数据竞争 |
| 后端 app 包选定测试 | **通过** | 认证、事件存储/日志等名称筛选，命令见附录 |
| `cd desktop/renew && go test ./... -count=1` | **通过** | 不等于原生窗口、提权和实际内核联动验收 |
| Gin 默认访问日志虚构令牌验证 | **确认记录 query 令牌** | 内存 httptest，无真实服务、真实令牌或外部请求 |

**前端失败详情：** 测试要求 `http://localhost:<port>`，当前 Vite 实际默认值为 `http://127.0.0.1:8080`。这是测试约定与实现不一致，不足以证明代理功能失效。修复应覆盖回环默认值和显式 `VITE_AGENT_BACKEND_URL`，不要为了通过旧断言盲目回退实现。

### 局部微基准

命令：`go test ./app/events -run '^$' -bench '^BenchmarkBuildKernelEvent(Openat|Connect|Sendto)$' -benchmem -benchtime=200ms -count=3`。

| 构造路径 | 3 次 ns/op 范围 | B/op | allocs/op |
| --- | --- | --- | --- |
| Openat | 380.6–402.0 | 1376 | 3 |
| Connect | 501.2–565.1 | 1424 | 5 |
| Sendto | 664.1–683.2 | 1432 | 5 |

这是已有测试依赖配置下的 **Go 事件构造微基准**，不是 eBPF syscall 开销、整个广播链路吞吐，也不是用户交互延迟。不能将倒数换算成产品的“每秒可采集事件数”。本轮未运行完整采集/阻断、浏览器压测、磁盘故障注入或真实桌面 GUI 验收。

## 四、逐项优化建议

### O1：独立后端默认仅本机监听，认证与 debug 解耦

**证据。** `backend/app/startup_server.go:42–60` 用 `:<port>` 监听 TCP；`helpers_auth.go:16–34` 仅在 release 且 `DISABLE_AUTH != true` 时要求令牌；`state_api.go:15–19` 的 `CheckOrigin` 恒为 true。相关 WS 路由确实挂了认证 middleware，因此不是“所有模式均无认证”，而是默认策略组合存在风险。

**影响边界。** 在使用 TCP 且处于免认证模式、网络可达时，观测 API/WS 的暴露面扩大；高危操作还受到各自 runtime gate 约束，不能直接断言所有执行接口可用。桌面私有 Unix socket 分支应独立评估。

**建议。**

- 默认绑定 `127.0.0.1`，需要远程访问时显式设置 bind address。
- 认证独立于日志模式；免认证开发模式仅允许回环，并对非回环配置直接拒绝启动。
- WebSocket 实施同源或显式来源白名单；为 Vite dev 和无 Origin 的原生客户端制定明确策略。
- 远程部署统一通过 TLS 入口，保留功能门控；Origin 是浏览器侧纵深防御，不是替代 token 的身份认证。

**验收。** 覆盖 debug/release、auth on/off、loopback/non-loopback、可信/非可信 Origin；未认证网络访问拒绝，合法 Web、CLI 和 Renew 均不回归。Gorilla 的官方文档明确要求服务器决定 Origin 策略，并提供默认的 Host/Origin 检查。[Gorilla Origin considerations](https://pkg.go.dev/github.com/gorilla/websocket#hdr-Origin_Considerations)

### O2：先阻止日志泄露，再迁移长期 WS query token

**证据。** `frontend/src/utils/requestContext.ts` 的 `buildWebSocketUrl` 把 localStorage 中的 token 放进 `?key=`；`backend/app/helpers_auth.go:116–129` 接受 query token；`main.go:186` 使用 `gin.Default()`。当前 Gin `v1.12.0` logger 在未启用 `SkipQueryString` 时追加原始 query。[Gin logger 源码](https://github.com/gin-gonic/gin/blob/v1.12.0/logger.go)

本轮使用 `RESEARCH_DUMMY_TOKEN` 和 `httptest`，结果：`dummy_token_in_default_access_log=true`。这确认了默认 logger 行为，**没有读取或断言现有用户日志已泄露真实令牌**。

**建议。**

1. 立即使用不记录 query 或按字段脱敏的访问日志；同时覆盖错误日志、网关日志和诊断包。
2. 普通 HTTP 不再接受长期凭证 query，保留标准认证 header；制定客户端兼容窗口。
3. 浏览器 WS 改为由已认证 HTTP 接口签发短时、单次、限定用途的 ticket；ticket 也不落日志。原生客户端可继续使用 header。
4. 提供撤销/轮换方式；只在确认真实 token 曾写入日志后按事件范围安排轮换与清理，不自动删除用户日志。

**验收。** 成功、失败、重连和关闭连接均不输出凭证；过期/复用 ticket 拒绝；普通 API 不能用 WS ticket；多标签页兼容。

### O3：把实际测试纳入持续集成

**证据。** `frontend-build.yml` 只执行图标测试和 build；本轮全量测试发现 proxy 断言失败。`perf-hotpath.yml` 的路径筛选没有覆盖 `backend/app/events/**`、`internal/wsfanout/**`、`ws_api.go` 等关键路径，执行命令也只有 `-run`，没有 `-bench`。这说明特定 workflow 有覆盖缺口，不代表整个仓库没有 CI。

**建议。**

- 修复 proxy 测试，frontend workflow 执行 `bun test tests`。
- 增加 core/all feature 对应工作台测试，定期执行已有 workbench browser 脚本。
- 用领域依赖组维护 workflow path filter，覆盖协议、生成步骤和跨模块依赖。
- 将竞态检测扩展到共享状态、队列和持久化，性能任务实际执行 benchmark 并保存可比较结果。
- `benchmark.yml:150` 不应吞掉 eBPF 生成失败；`:181–190` 不能仅以 curl 有响应/JSON 可解析认定采集可用。使用 `curl -f`，并断言正确的 bootstrap 状态、目标程序挂载与计数增长。

**验收。** 人为破坏 proxy、广播或事件构造测试的 PR 必须触发并失败；内核程序不可用时 benchmark 明确 failed/skipped，不产生“有效开销”结论。GitHub 路径过滤会影响 workflow 是否运行，必须按实际依赖设计。[GitHub Actions workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax#onpushpull_requestpull_request_targetpathspaths-ignore)

### O4：将“实时连接”升级为“可解释的事件连续性”

**证据。** `frontend/src/composables/dashboard/useDashboardStream.ts:190–228` 在暂停时直接返回，重连的 onopen 仅修改连接状态；初始历史加载只有 200 条。该实现未在重连时补读断开期间事件，`evictedRecords` 也主要统计本地队列/窗口淘汰。

**建议。**

- 区分采集丢失、入队失败、持久化失败、传输断档、UI 窗口淘汰、用户暂停六类状态。
- 使用稳定 event ID 和服务端恢复游标；内核 CPU/sequence/generation 只用于对应来源的连续性，不能混作所有 hook/derived 事件的全局序列。
- 重连先订阅并缓冲 live，再按水位补读、去重合并；超出保留期明确显示不可恢复区间。
- 明确“暂停显示”还是“暂停接收”：前者继续有界接收，后者展示缺口并允许恢复。不得无限缓存。

**验收。** 在有持久化的测试环境中断开 10 秒，历史与 live 重叠时不重复、不遗漏可恢复事件；超过保留期显示缺口而非假装完整；暂停恢复语义可见。服务端慢客户端断开机制应保留。

### O5：建立可信的性能基线，而不是只看平均开销

**证据。** 已有 metrics 和 benchmark，但 `metrics_prometheus.go:72–73,112–113` 对关键延迟暴露的是最近值 gauge，无法还原 P95/P99。`broadcast_pipeline_bench_test.go:19–31` 循环复用 1024 个事件指针，而 `statepersistenceruntime.go:760–769` 明确事件会原地脱敏并被保留；基准输入不再持续代表新鲜事件，可能造成测量偏差。

**建议。**

- 修正该基准的事件所有权：每次交付新对象，或按批次在非计时区准备新对象；另设包含构造成本的端到端子基准。
- 在 reader→queue、归因/分析、持久化提交、广播、客户端呈现等阶段建立 histogram；不要把异步入队耗时当成落盘耗时。
- 保留已有 drop counters，增加重启/generation 说明与跨阶段对账；过滤、派生告警、采样要单独计数，否则简单相减会误报丢失。
- 用 Go CPU/heap/block/mutex profile 定位占用后再修改。不要将未受保护的 pprof 暴露到网络。[Go Diagnostics](https://go.dev/doc/diagnostics)
- 参考 Falco 的事件率、资源、内核和输出队列指标分类完善健康面板，不必替换现有指标系统。[Falco metrics](https://falco.org/docs/concepts/metrics/)

**验收。** 固定 fixture、构建配置、硬件和 feature 集；基线与候选版本交替测量至少 5 次，保存 raw 数据与差异分析。任何“更快”都需同时报告丢失、内存和持久性是否退化。

### O6：历史查询不要无条件承担新的持久化屏障

**证据。** `runtime_event_store_pebble.go` 的 `Recent`、`Page`、`GetByID` 分别在 `523`、`596`、`661` 行先调用 `FlushContext`，writer 通过 `batch.Commit(pebble.Sync)` 提交。已有 ID 索引和游标，不是全库扫描式设计。

**判断。** 持续写入时，高频查看历史或详情可能导致更多小批同步提交，读延迟也可能等待写队列。耦合是确认的，性能影响尚未通过磁盘混合负载验证；无待写数据时不会必然触发实际同步写。

**建议。**

- 为普通历史浏览定义 committed snapshot / persisted watermark 语义，避免每次翻页请求新的 flush。
- 对“刚产生事件的详情”先查有界内存归档，必要时显式请求 read-after-write。
- 保留录制停止、审计导出、配置切换等必须等待持久化的屏障。
- 合并并发 barrier 请求；只有查询模式证明确有收益时再添加 agent/time 等二级索引，评估额外写放大。

**验收。** 在持续写入 + 10 个历史查询客户端下比较读 P95、sync 次数、队列长度与丢弃数；新旧接口明确数据新鲜度。不得通过将 `pebble.Sync` 全面替换成 `NoSync` 作为“优化”。

### O7：保持有序主链路，按 profile 优化热点

**证据。** `ws_api.go:153–179` 在同一消费循环内执行归因、capture admission、语义告警和记录；记录阶段再分发到归档、持久化、录制、OTLP、分析队列。已有异步下游，不应误判为全部同步写盘。

**建议。**

- 先补阶段计时；只有昂贵且不影响 admission/顺序/脱敏的 enrichment 才考虑移到有界 worker。
- 无 summary 订阅者时，评估跳过 Web summary 构造；不能影响独立 Renew native summary 路径。
- 为可选分析明确过载行为，保持核心采集和审计状态可观测；不使用无界 goroutine。
- 内核按来源过滤和事件类型紧凑布局仅在 profile 证明有效时实施；调整 C ABI 必须同步 proto/绑定及 verifier 测试。

**验收。** 同一 Agent 的因果顺序、事件不可变所有权、派生告警归因、脱敏先后顺序不变；慢下游不能阻塞采集；吞吐收益必须附 P99 与丢失率。当前微基准已经较小，不建议凭直觉重写所有字符串处理。

### O8：Web 工作台优先减少重复数据处理，而不是简单加大缓存

**证据。** Dashboard 和 AgentSight 等 composable 各自建立 WS；工作台允许独立 pane 路由。Dashboard 每次 flush 合并已有窗口，多个 computed 在数据变化后可能重新扫描。已使用 shallowRef、批处理和分页，因此实际成本需用多面板浏览器 profile 确认。

本轮构建部分产物大小（minified / gzip，**不等于首页实际下载量**）：

| 产物 | 大小 |
| --- | --- |
| `editor.api2-*.js` | 2557.19 / 657.91 kB |
| `es-*.js` | 1354.55 / 409.39 kB |
| `monaco.contribution-*.js` | 1081.88 / 273.61 kB |
| `tracker_pb-*.js` | 443.40 / 64.78 kB |

**建议。**

- 按后端目标、认证会话、stream 类型共享连接与解码；面板保留自己的筛选和选中状态。认证或 cluster 切换必须隔离，不能跨目标共享数据。
- 根据 browser profile 决定增量聚合、共享 selector、Worker 解码或虚拟列表；Worker 应计算传输/复制成本。
- 分析初始路由和功能路由依赖图，编辑器、语言包、图表按需加载；不要仅提高 chunk warning 阈值。
- 后台 pane 降低派生计算频率；保留输入焦点、事件选中身份和 pane 路由隔离。

**验收。** 固定 1/4/8 pane 场景、5000/50000 条窗口和相同事件 fixture，记录连接数、解码次数、长任务、交互 P95 和堆内存；关闭 pane 后订阅释放。Vue 官方建议基于测量采用代码拆分、列表虚拟化和浅层响应性。[Vue performance](https://vuejs.org/guide/best-practices/performance.html)

### O9：渐进式缩小特权后端职责

**证据。** `backend/app/main.go:43–64` 在应用启动阶段先确保特权，后续同一运行时承载 API 和多个功能域。现有 runtime gate、编译 feature gate 与桌面私有 IPC 已提供边界，但不是进程级故障隔离。

**建议。** 先画出 capability/操作矩阵，再将需要特权的 BPF 加载、map/策略操作收敛到小 helper；普通查询、分析与 UI 数据服务以低权限运行。复用现有私有 IPC 经验，但必须校验 peer、请求类型、参数和授权范围，避免变成任意命令代理。

**验收。** 在目标内核上分别验证 tracing、cgroup、LSM、TLS opt-in 的权限需求和失败降级；helper 崩溃、升级和权限拒绝具有明确状态。不要假设“设置某几个 capability”就能在所有内核上替代 root。

这项是中期架构改进，优先级低于 O1/O2；不建议为了形式上的微服务化立即拆成多个网络服务。

### O10：用当前能力契约约束文档与功能扩张

**证据。** 当前 Vue views 已按领域分目录，后端有 `app/events`、`app/handlers`，Renew 已具备原生 Go UI/IPC；部分导航仍描述扁平 views、旧路由文件组织。`frontend/src/composables/dashboard/useDashboard.ts` 仍有 920 行，说明拆目录不等于职责已完全分离。

**建议。**

- 更新仓库导航和 skill references，明确 Web 与 Renew 的数据接口、测试入口和真实边界。
- 将 API/feature/auth/runtime gate 建成可测试矩阵，校验路由、Vite proxy、前端导航和文档一致性；新增字段始终从 proto 出发。
- Dashboard 只按筛选、聚合、选择状态等稳定职责增量拆分，不进行一次性全仓重命名。
- 保留并扩展 Renew 对历史进程、PID 复用和归因的测试；跨节点/重启身份不能仅靠 PID。Tetragon 的事件模型包含独立 exec identity、父关系和时间，可作为契约设计参考，不代表本项目需复制其全部架构。[Tetragon events](https://tetragon.io/docs/concepts/events/)
- 产品表述继续区分“观测到”“策略决定”“内核实际阻断”“无法判断”；不将 hook 接入覆盖、跨编译或离线测试写成全平台验证。

**验收。** 文档路径检查、feature matrix 测试和协议兼容性检查通过；典型用户可从事件追溯 Agent→工具调用→进程→文件/网络结果，并看到证据缺口，而不是只有更多图表。

## 五、落地顺序与验收实验

### 推荐迭代

| 阶段 | 内容 | 出口条件 |
| --- | --- | --- |
| 第 1 阶段 | O1、O2 日志止血、O3 测试修复 | 安全默认值生效；凭证不进日志；前端测试全部通过且 CI 真正运行 |
| 第 2 阶段 | O4、O5，完成 ticket 迁移设计 | 重连/暂停缺口可解释；形成可信基准与阶段耗时图 |
| 第 3 阶段 | 依据第 2 阶段证据选择 O6–O8 | 至少一个高价值瓶颈改善且持久性、丢失和内存不退化 |
| 后续 | O9 原型、O10 持续治理 | 特权边界与能力契约可测试，按风险逐项迁移 |

按单人顺序推进，前三阶段建议预留约 4–6 周，具体取决于协议迁移和故障测试；不应把表中的独立估计误认为可以全部同时交付。

### 实验矩阵

- **负载**：合成回放逐档增加到 1k/5k/10k events/s，每档稳定运行 10 分钟；这是实验输入，不是当前吞吐承诺。真实 syscall 场景另做内核基线对照。
- **订阅者**：0/1/10/50，至少一个慢消费者；Web 再覆盖 1/4/8 pane，Renew 使用相同事件来源。
- **持久化**：关闭、JSONL、Pebble；查询并发 0/1/10；临时测试目录中模拟慢盘、写失败和退出恢复。
- **故障**：网络断开、暂停、令牌轮换、服务重启、保留期跨越；内核阻断实验只在可恢复隔离环境执行。
- **产物**：commit、内核、CPU、构建 feature、配置摘要、脱敏 fixture、原始指标、profile、浏览器 trace 和有效/无效判定理由。

建议起始验收目标如下，**均为待校准目标，不是本轮实测指标**：

| 指标 | 建议门槛 |
| --- | --- |
| 凭证泄露 | 测试日志/诊断包中真实或测试 token 出现次数为 0 |
| 证据完整性 | 设计负载内无未解释丢失；过载时所有丢弃阶段可定位 |
| WS 恢复 | 在保留期内完成补读去重；保留期外明确显示不可恢复缺口 |
| 用户交互 | 固定硬件/fixture 下交互 P95 先以 200ms 为目标，再按基线校准 |
| 稳态资源 | 预热后持续负载 30 分钟内存形成平台，无与累计事件数线性增长 |
| 性能变更 | 同条件重复结果有明确改善，同时 drop、tail latency、持久性不恶化 |

## 六、不建议本轮启动的事项

- 为追求“高并发架构”引入 Kafka/Redis 或拆成大量网络微服务：目前证据未证明必要。
- 把 ringbuf、Go queue、UI cache 全部扩大：只会延后过载暴露，可能扩大内存和尾延迟。
- 关闭同步落盘、脱敏、权限检查或 TLS opt-in 来提高 benchmark。
- 未定位开销就重写为 Rust、全面启用 GPU/LLM，或宣称 AI 检测替代确定性策略。
- 用 unit/build 成功替代浏览器、桌面、eBPF verifier 和实际阻断验收。

## 附录：复现命令与证据边界

以下为本轮执行命令；测试日志临时保存在 `/tmp/agent-research-*.log`，不是长期交付件，关键结果已记录在正文。

```bash
# Web
cd frontend
bun test tests
bun run build

# Backend
cd ../backend
go test ./internal/wsfanout ./app/observability ./app/events \
  ./app/recording ./redaction ./core -count=1
go test -race ./internal/wsfanout ./app/observability ./app/events \
  ./app/recording ./redaction ./core -count=1
go test ./app \
  -run 'Test.*(Auth|RuntimeEventStore|RuntimeEventLog|EventBroadcast|RecentEvent|Collector|KernelCapabilities)' \
  -count=1
go test ./app/events -run '^$' \
  -bench '^BenchmarkBuildKernelEvent(Openat|Connect|Sendto)$' \
  -benchmem -benchtime=200ms -count=3

# Renew native tests
cd ../desktop/renew
go test ./... -count=1
```

日志行为验证使用内存 `httptest.NewRecorder()`，向 `gin.Default()` 的临时 `/probe` handler 发送 `/probe?key=RESEARCH_DUMMY_TOKEN`，捕获 `gin.DefaultWriter` 并检查虚构令牌是否存在；没有启动监听端口。

Race detector 通过只覆盖本轮实际执行的代码路径，不能证明未执行路径不存在竞态。[Go race detector](https://go.dev/doc/articles/race_detector)

**最终边界：上方实施记录列出已落地改进，其余正文保留初次调研基线。未产生端到端吞吐提升、完整流恢复或生产安全验收结论；未推送、部署或执行特权内核测试。**
