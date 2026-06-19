# 快速开始

本页给出开发、运行、构建和文档站启动的最短路径。

## 环境前提

Agent eBPF Filter 面向 Linux。完整能力需要：

- Linux kernel 支持 eBPF、BTF、cgroup v2、BPF LSM（视功能而定）；
- Go、Bun、Python / uv、clang / LLVM；
- 特权运行环境，用于加载 eBPF、pin maps/links、绑定可选 80/443；
- 前端开发需要 Bun；
- native hooks relay 依赖 host 安装 `curl`。

## 准备开发依赖

```bash
make predev
```

`make predev` 会并行准备 Go、Python、frontend 和 dev-env TUI 依赖，并处理不可写 GOPATH 的情况。

## 启动开发环境

```bash
make dev
```

`make dev` 会启动后端热加载和前端 Vite dev server。后端会在 `8080..8089` 选择可用端口并写入 `backend/.port`，前端 dev proxy 和 adapters 可读取该端口。

也可单独启动：

```bash
make dev-backend
make dev-frontend
```

## 构建全部组件

```bash
make all
```

等价于：

```text
proto → backend → frontend → wrapper
```

常用分项：

```bash
make proto
make backend
make frontend
make wrapper
```

## 运行文档站

本站使用 VitePress。根目录脚本：

```bash
bun install
bun run docs:dev
```

生产构建：

```bash
bun run docs:build
bun run docs:preview
```

::: warning 依赖说明
如果本地尚未安装 `vitepress`，请先运行 `bun install` 更新根目录依赖。当前仓库前端应用依赖位于 `frontend/package.json`，文档站依赖位于根目录 `package.json`。
:::

## 最小验证命令

| 改动类型 | 最小验证 |
| --- | --- |
| Markdown / VitePress 文档 | `bun run docs:build` |
| 后端 Go | `cd backend && go test ./...` |
| wrapper | `cd wrapper && go test ./...` |
| 前端 Vue / TS | `cd frontend && bun run build` |
| proto | `make proto`，再 backend/frontend 验证 |
| 主 eBPF tracker | `cd backend/ebpf && go generate` + `cd backend && go build ./...` |
| cgroup / LSM | `make ebpf-cgroup` / `make ebpf-lsm` |

## 安全提示

以下操作具有外向或高风险效果，应在明确授权下执行：

- `make install` 安装系统服务；
- 启动特权 eBPF / cgroup / BPF LSM enforcement；
- 修改 AI CLI hook 配置；
- 启用 TLS 明文捕获；
- 启用 80/443 domain forward；
- 清空持久化事件；
- 运行 `/system/run` 或交互式 shell sessions。
