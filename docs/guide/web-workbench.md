# IDE 风格 Web 工作台

WebUI 采用深色 VS Code 风格布局：顶部功能搜索、左侧活动栏与功能导航、中央可停靠工作区、底部状态栏。原有功能页和后端认证、运行时功能开关保持不变。

## 打开与分屏

- 单击左侧功能，在当前聚焦面板中打开或切换对应标签。
- 拖动标签或左侧功能到面板**左／右／上／下边缘**，创建对应方向的分屏；蓝色预览显示投放位置。
- 拖到面板**中央或标签栏**，合并到该面板。空源面板会自动移除。
- 标签栏右侧按钮或左下角按钮可直接左右／上下分屏。按钮分屏会在新面板打开同一功能页；之后可以切换为其他功能。
- 拖动分隔线调整比例；也可用 Tab 聚焦分隔线，再用方向键每次调整 5%。
- 标签的关闭按钮移除该标签；关闭面板最后一个标签后，空分屏自动合并。

这里的拖拽单位是**功能页面板组件**，不是页面内部的任意按钮、表格列或卡片，也不是弹出独立浏览器窗口。为避免无限递归和过多并行数据订阅，布局最多支持 12 个面板、8 层分屏、每面板 30 个标签；超限时状态栏会提示。各面板有独立的路由上下文，例如右侧配置页切换子标签不会改变左侧监控页。

## 布局恢复

面板树、标签路径、当前标签和分屏比例自动保存在当前浏览器的 `localStorage`（`agent.ide.layout.v1`）。刷新后恢复布局，懒加载页面会重新加载。禁用浏览器存储时仍可使用，但刷新不会恢复。

仅保存布局，不保存事件数据、认证令牌、未提交表单或终端现场。关闭／切换页面可能重新建立数据订阅；不要把布局保存视为终端会话恢复。后端不可用时原功能页会显示未连接／错误状态，工作台不伪造采集数据。

顶部重置按钮和底部“重置布局”会在确认后回到当前功能的单面板布局，不更改后端配置。

## 验证

```bash
cd frontend
bun run test:workbench
VITE_AGENT_BUILD_FEATURES=core bun run test:workbench
bun run build
```

浏览器交互回归需要已安装的 `agent-browser` 和运行中的前端：

```bash
# 一个终端启动前端
bun run dev --host 127.0.0.1
# 另一个终端验证（不要求特权后端，也不会修改后端策略）
CHROMIUM_PATH=/usr/bin/chromium bun run test:workbench-browser
```

可设置 `AGENT_BROWSER_CLI` 指定 CLI 可执行路径、`WORKBENCH_URL` 指定前端地址。脚本使用独立浏览器会话，验证原生标签拖拽合并、HTML5 边缘投放、分屏折叠、鼠标／键盘尺寸调整、刷新恢复、独立页面路由、功能搜索和窄屏布局。不替代后端 API、特权 eBPF 或真实设备验证。

## Dashboard 接收、暂停与重连

- Dashboard 使用 `/ws/envelopes` 的稳定 `eventId`，与 `/events/recent?limit=1000` 返回的历史记录去重；内容相同但 ID 不同的系统调用不会因此合并为同一证据。
- “暂停显示 / Pause Display”不关闭接收：当前窗口冻结，待显示队列仍受当前容量限制；恢复时合并。队列超限会丢弃最早待显示记录并持续提示数量，与已显示窗口的容量淘汰分开统计。
- 每次 WebSocket 连接成功后补取最近记录；补取失败不清空本地数据，提供重试按钮。离开页面或切换集群时取消旧请求，旧响应不会写入新会话。清空后端失败也不会清空本地证据。
- **补取不是完整恢复协议**：每次最多 1000 条，没有连续游标/保留期证明。断线、解码失败和缓存淘汰提示不因重新连接就消失；需要完整证据时核对后端持久化审计记录。旧历史若无 ID，不按内容猜测去重。

流恢复浏览器测试用本地合成 WebSocket / protobuf / HTTP 响应替代全部 API 请求，不会改变真实后端数据。需要运行中的 Vite 前端、Chromium 和可用的 `playwright-core`：

```bash
cd frontend
bun test tests/dashboard-stream.test.ts
# 已在模块搜索路径安装 playwright-core 时无需 PLAYWRIGHT_MODULE；否则指定其 index.mjs。
PLAYWRIGHT_MODULE=/path/to/playwright-core/index.mjs \
  CHROMIUM_PATH=/usr/bin/chromium DASHBOARD_URL=http://127.0.0.1:5173 \
  bun run test:dashboard-stream-browser
```

覆盖暂停/恢复、自动重连补取、稳定 ID 去重、HTTP 失败重试、清空失败保留、明细模式和窄屏提示。不是实际 eBPF 采集、长期过载或持久化完整性的验收。
