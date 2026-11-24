# Renew 桌面界面演示视频（MyGo render 插件）

`desktop/renew` 的 `--render-ui-demo` 用官方
[render 插件](https://mygo.run/zh/docs/plugins/render) 把**真实 Renew 桌面界面**录成
MP4：compositon 的 View 直接调用应用自己的视图函数，没有任何重绘或仿制的界面。

```go
View: func(c *ui.Context, f render.Frame) {
    uiDemoApplyFrame(app, f, now, fixedScene) // 只推进数据与页面状态
    app.view(c)                               // 真实的界面代码
}
```

`app.view` 就是窗口使用的同一条链路：
`native_ui.go` 的 `renewApp.view` → `native_workspace.go` 的 `workspaceView`
（活动栏 / 侧栏 / 页头 / 页脚 / 研判栏）→ `native_pages_*.go` 的各页面。
界面里出现的每一个像素、文案、图标、表格与动画都由这些真实代码产生。

## 渲染

```sh
# 在仓库根目录运行
go run ./desktop/renew --render-ui-demo artifacts/renew-ui-demo.mp4

# 或在 desktop/renew 内运行：输出路径相对当前工作目录
go run . --render-ui-demo ../../artifacts/renew-ui-demo.mp4   # 也接受 --render-ui-demo=<path>
go run . --ui-demo-stills ../../artifacts/ui-demo-stills      # 每个页面导出一张 PNG
go test ./...
```

- 视频：1480×860、30 fps、420 帧（14 秒）、H.264。
  `Scale: 1` 让视频像素等于窗口 DIP，因此布局与真实的 1480×860 主窗口完全一致。
- 不需要窗口、显示服务器、浏览器、后端或 root 权限。
- 帧由 GPU 渲染器（Linux 上是无需显示器的 EGL）绘制，没有 GPU 时用 CPU 渲染器；
  编码交给系统编码器（VA-API / NVENC，否则 x264）。
- 渲染前会拦截命令行并直接返回，不会启动 `mygo.App`。

## 镜头表

| 秒 | 页面 | 画面内容 |
| --- | --- | --- |
| 0–2.3 | 概览 | “我的设备”监控状态、风险计数、优先核查列表 |
| 2.3–4.7 | 事件 | eBPF 实时事件流表格、筛选、行选中 |
| 4.7–7.0 | 行为 | 行为轨道（四类同心轨道）、窗口事件与速率 |
| 7.0–9.3 | 研判 | 风险研判工作台、事件上下文、固定证据 |
| 9.3–11.7 | 网络 | 按外联目标聚合的访问目标表 |
| 11.7–14 | 进程 | 实时进程快照视图与选中进程详情（演示中为示例进程数据） |

每个镜头内事件窗口按“收集器持续写入”的方式逐条展开，页面切换使用界面自身的
过渡动画（`ui` 的动画跟随视频时钟，见 render 插件文档）。

## 数据是示例数据

**界面是真的，数据是示例的。** 为了在不连接后端、不提权的情况下渲染，
`ui_demo_video.go` 用代表性数据填充 `renewApp` 状态：

- `uiDemoEvents`：10 条紧凑事件摘要（`openat` / `connect` / `write` /
  `process_exec` / `TLS_PLAINTEXT` / `semantic_alert`），带 `Decision`、
  `RiskScore`、Agent 归属（Claude Code / Codex / DSH）；
- `uiDemoProcesses`：5 行实时进程快照；
- 事件接收时间按“渲染开始时刻往前 4 秒起、每 27 秒一条”生成，因此落在界面自己的
  5 分钟行为窗口内。

这些不是真实采集、攻击回放或基准测试证据。视频画面里界面自带的免责声明
（例如“数量只统计当前摘要窗口，不代表设备所有历史活动”“事件数和字节数不是完整
连接/流量统计”）保持原样显示，未被移除或改写。

## 可复现性

- 时间来自 `render.Frame`（`Progress`/`Seconds`）与渲染开始时刻，页面状态由帧号决定；
- 每帧完整构建并绘制，没有缓存的 `ui.Element`，也没有 goroutine 触碰 `ui.Context`；
- `--ui-demo-stills` 对每个页面单独渲染一张 PNG，便于人工核验或接入 CI。