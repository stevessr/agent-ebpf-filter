# 构建与运行

`Makefile` 是项目统一入口，管理所有构建、开发和部署流程。

---

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `make predev` | 并行安装开发依赖 |
| `make dev` | 后端 + 前端开发会话（Zellij） |
| `make dev-backend` | 仅后端热加载 |
| `make dev-frontend` | 仅前端 Vite dev server |
| `make run` | production build + run |
| `make backend` | 构建 backend + eBPF generate |
| `make frontend` | 构建 Vue frontend |
| `make wrapper` | 构建 agent-wrapper |
| `make proto` | 生成 protobuf bindings |
| `make all` | proto + backend + frontend + wrapper |
| `make install` | 生产安装为系统服务 |
| `make uninstall` | 卸载系统服务 |
| `make clean` | 清理构建产物 |

---

## 开发环境设置

### 首次设置

```bash
# 1. 可选：交互式环境配置向导
make dev-env          # Go TUI，写入 .env.dev 和 .env.dev.mk

# 2. 安装开发依赖
make predev           # 并行安装 Go/Python/Frontend/TUI 依赖

# 3. 启动开发环境
make dev              # Zellij 会话：后端 + 前端分离 pane
```

### 环境配置详情

`make dev-env` 启动 `tools/dev-env-tui` 中的 Go TUI 程序，交互式配置：

- 核心开发设置
- ML/LLM 行为（`AGENT_LLM_*` 和 OpenAI 兼容设置）
- 运行时开关
- sandbox / cluster 设置
- devcontainer image 覆盖
- CUDA 配置
- smoke test / replay / ML sweep 设置

配置文件写入 `.env.dev`（shell exports）和 `.env.dev.mk`（Makefile 变量），**不应提交到 Git**。

```bash
# 查看生效配置（secrets 脱敏）
make dev-env-doctor

# 在 shell 中加载配置
set -a; . ./.env.dev; set +a
```

---

## 端口模型

后端监听 `8080..8089` 中第一个可用端口，并写入 `backend/.port`。

以下组件自动读取该端口：
- Vite dev proxy（`frontend/vite.config.ts`）
- Adapters
- Hook callback URL 推导
- CLI relay scripts

---

## 编译期功能选择

通过 `AGENT_BUILD_FEATURES` 控制后端功能模块：

| 值 | 说明 |
| --- | --- |
| `all`（默认） | 完整功能 |
| `core` | 仅核心事件与运行时控制面 |
| `tls_capture,ml,plugins` | 逗号分隔，映射为 Go build tag |

```bash
# 示例：仅构建核心模块
AGENT_BUILD_FEATURES=core make backend

# 示例：选择性功能
AGENT_BUILD_FEATURES=tls_capture,ml make backend
```

前端通过 `AGENT_FRONTEND_BUILD_FEATURES` 控制可见功能声明。

::: tip
编译期选择只决定"当前构建是否包含该模块"。危险能力仍需 `/config/runtime` 运行时 gate 和 release mode token 才能使用。后端通过 `GET /system/features` 暴露当前构建与运行时状态。
:::

---

## eBPF 构建

```bash
make ebpf-bootstrap    # 主 tracker
make ebpf-tls          # TLS capture
make ebpf-cgroup       # cgroup sandbox
make ebpf-lsm          # BPF LSM enforcer
```

手动重建主 tracker：

```bash
cd backend/ebpf && go generate
cd .. && go build ./...
```

---

## 文档站

```bash
bun install            # 安装文档站依赖（根目录 package.json）
bun run docs:dev       # 本地实时预览
bun run docs:build     # 生产构建
bun run docs:preview   # 预览构建产物
```

::: warning 依赖隔离
前端应用的依赖位于 `frontend/package.json`，文档站的依赖位于根目录 `package.json`，两者相互独立。
:::

---

## 常见构建问题排查

| 问题 | 解决方案 |
| --- | --- |
| `GOPATH=/go` 不可写 | `make predev` 自动 fallback 到 `$HOME/go` |
| `protobufjs-cli` 报错 | 必须通过 Node.js 运行 `pbjs`/`pbts`，不能用 `bunx` |
| CUDA 编译失败 | 无 CUDA 时使用 CPU-only stub，不影响正常构建 |
| 前端找不到 `vitepress` | 在根目录执行 `bun install` |
| eBPF 编译需要 clang | 安装 `clang` / LLVM |

---

## 相关导航

- [开发容器](devcontainer.md)
- [部署与安装](deployment.md)
- [验证、测试与 Benchmark](verification-benchmark.md)
- [后端启动链路](../backend/runtime-startup.md)
- [生成文件边界](../reference/generated-files.md)
