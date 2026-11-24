# 快速开始

根据目标选择启动入口。**日常监控首选原生 Renew；深度研究、策略与 API 管理使用 Vue Web 工作台。** 文档站使用 MkDocs Material，与监控桌面和 Web 工作台独立。

## 环境要求

| 用途 | 最低前提 |
| --- | --- |
| 文档站 | Python 3.12 + `docs/requirements.txt` |
| Go / MyGo 原生桌面编译 | Linux、项目声明的 Go **1.27.3** 工具链、MyGo 版本以 [`desktop/renew/go.mod`](https://github.com/stevessr/agent-ebpf-filter/blob/master/desktop/renew/go.mod) 为准（截至 2026-10-18 为 v0.5.5，模块自动拉取）与对应本机 GUI 开发依赖 |
| 嵌入式后端/eBPF 构建 | clang/LLVM、`protoc`/`protoc-gen-go`、eBPF 对象构建依赖 |
| 真实 eBPF 采集 | Linux eBPF/BTF、授权的 BPF 加载能力和受支持内核 |
| 内核级 cgroup / BPF LSM 阻断 | 额外需要 cgroup v2 / 已启用的 BPF LSM、程序成功附着与 Runtime Policy Gate |
| Vue Web 工作台 | Bun、Go 后端与 Vite 前端工具链 |

若只阅读文档，不需要 root、eBPF、BPF LSM 或安装桌面。

## 方式 A：明镜高悬原生桌面（推荐）

```bash
# 仓库根目录
make predev
make os-enforcement-preflight
make renew-desktop-dev
```

此命令会准备 protobuf/eBPF 对象并通过 MyGo 启动桌面开发版。打开 **我的设备**，观察 backend、collector、事件与系统流状态；若有错误，进入 **系统诊断**。

**制作桌面产物**：

```bash
make renew-desktop-build

# 独立检验原生 Go 模块
cd desktop/renew
go test ./...
go tool mygo vet .
```

Linux 桌面常规内置模式把后台作为同一可执行文件的授权子进程启动，使用两个私有 Unix Socket 通信，**不会因为启动桌面就额外暴露端口 8080**。选择连接已有后端时，遵从那个服务本身的地址和认证设置。

详细能力见 [原生桌面指南](/frontend/renew)，实际点按路径见 [演示导览](/guide/demo-tour)。

## 方式 B：Vue Web 研发工作台

```bash
make predev
make dev
```

`make dev` 使用开发会话运行后端与 Vite；前端一般位于 `http://localhost:5173`，后端在 `8080..8089` 之间选择可用端口，并写入 `backend/.port`，供开发代理读取。

也可拆分运行：

```bash
make dev-backend
make dev-frontend
```

此入口提供 Dashboard、Network、Execution Graph、Hooks、Config、ML 等**研究和管理**页面；老的浏览器 `/renew` 路由已移除。不要把这个端口模型套用到内置 Renew 桌面。

## 方式 C：终端查看事件

```bash
make tui
```

TUI 订阅后端的事件接口，适合无桌面的测试环境。采集事实与权限约束不因使用终端而改变。

## 文档站本地预览

```bash
python3 -m pip install -r docs/requirements.txt
python3 docs/build_site.py serve
python3 docs/build_site.py build
```

从仓库根目录运行；配置位于 `docs/mkdocs.yml`，与前端的 Bun 依赖独立。

## 最小验证清单

| 改动范围 | 建议检查 |
| --- | --- |
| MkDocs 页面 / 导航 | `python3 docs/build_site.py build`；有条件时运行 `python3 scripts/check-doc-links.py` |
| Renew MyGo 桌面 | `cd desktop/renew && go test ./...` 与 `go tool mygo vet .` |
| Go 后端 | `cd backend && go test ./...` |
| Vue Web | `cd frontend && bun run build` |
| eBPF / OS 控制 | `make os-enforcement-check`；另在隔离内核测试机上执行 preflight / live smoke |
| Protobuf | `make proto` 并同步校验 Go/前端消费者 |

::: warning 不要在生产主机直接做的事
Hook 安装会修改目标 CLI 的配置；BPF LSM 与 cgroup 策略可能真的阻断所有相关进程；TLS 明文采集、80/443 域名转发和 Shell 执行可能影响隐私与宿主行为。开启前请明确授权、确认 Gate、保留回滚方法。**文档演示默认先用只读操作。**
:::

## 遇到问题？

- 只有 UI，没有事件：检查内核权限、collector 健康、跟踪登记与忽略规则；没有数据不等于没有风险。
- Renew 无法访问后端：先区分内置私有 UDS 模式与显式 `--backend` / `AGENT_BACKEND_URL` 连接已有服务的模式。
- 文档缺少依赖：执行 `python3 -m pip install -r docs/requirements.txt`；Vue 前端的 Bun 依赖另行安装。

[桌面详解](/frontend/renew) · [构建与运行](/operations/build-and-run) · [安全模型](/security/model)。
