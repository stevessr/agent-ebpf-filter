# 构建与运行

`Makefile` 是项目统一入口。

## 常用命令

| 命令 | 作用 |
| --- | --- |
| `make predev` | 并行安装开发依赖 |
| `make dev` | 后端 + 前端开发会话 |
| `make dev-backend` | 后端热加载 |
| `make dev-frontend` | 前端 Vite dev server |
| `make run` | production build + run |
| `make backend` | 构建 backend + eBPF generate |
| `make frontend` | 构建 Vue frontend |
| `make wrapper` | 构建 agent-wrapper |
| `make proto` | 生成 protobuf bindings |
| `make all` | proto + backend + frontend + wrapper |
| `make clean` | 清理构建产物 |

## eBPF 构建

```bash
make ebpf-bootstrap
make ebpf-tls
make ebpf-cgroup
make ebpf-lsm
```

主 tracker 修改：

```bash
cd backend/ebpf && go generate
cd ../.. && cd backend && go build ./...
```

## 文档站

```bash
bun install
bun run docs:dev
bun run docs:build
bun run docs:preview
```

## 端口模型

后端监听 `8080..8089` 中第一个可用端口，并写入 `backend/.port`。Vite dev proxy、adapters 和 hook callback 推导会读取该端口。
